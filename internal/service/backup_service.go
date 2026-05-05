package service

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/pkg/sftp"
	gossh "golang.org/x/crypto/ssh"

	"github.com/lollinoo/theia/internal/crypto"
	"github.com/lollinoo/theia/internal/domain"
	"github.com/lollinoo/theia/internal/ssh"
	"github.com/lollinoo/theia/internal/vendor"
)

var sanitizeRe = regexp.MustCompile(`[^a-zA-Z0-9_.-]`)

// BackupService orchestrates credential profile management and config backups.
type BackupService struct {
	jobRepo               domain.BackupJobRepository
	fileRepo              domain.BackupFileRepository
	credentialProfileRepo domain.CredentialProfileRepository
	deviceRepo            domain.DeviceRepository
	settingsRepo          domain.SettingsRepository
	vendorRegistry        *vendor.Registry
	sshDialer             ssh.Dialer
	encryptionKey         []byte
	backupDir             string
	hostKeyCallback       gossh.HostKeyCallback
	deviceLocks           sync.Map // per-device mutex: map[uuid.UUID]*sync.Mutex
}

// NewBackupService creates a new BackupService.
func NewBackupService(
	jobRepo domain.BackupJobRepository,
	fileRepo domain.BackupFileRepository,
	credentialProfileRepo domain.CredentialProfileRepository,
	deviceRepo domain.DeviceRepository,
	settingsRepo domain.SettingsRepository,
	vendorRegistry *vendor.Registry,
	sshDialer ssh.Dialer,
	encryptionKey []byte,
	backupDir string,
	hostKeyCallback gossh.HostKeyCallback,
) *BackupService {
	return &BackupService{
		jobRepo:               jobRepo,
		fileRepo:              fileRepo,
		credentialProfileRepo: credentialProfileRepo,
		deviceRepo:            deviceRepo,
		settingsRepo:          settingsRepo,
		vendorRegistry:        vendorRegistry,
		sshDialer:             sshDialer,
		encryptionKey:         encryptionKey,
		backupDir:             backupDir,
		hostKeyCallback:       hostKeyCallback,
	}
}

// getDeviceLock returns or creates a per-device mutex.
func (s *BackupService) getDeviceLock(deviceID uuid.UUID) *sync.Mutex {
	val, _ := s.deviceLocks.LoadOrStore(deviceID, &sync.Mutex{})
	return val.(*sync.Mutex)
}

// nowInConfiguredTZ returns the current time in the timezone configured in settings.
// Falls back to UTC if the setting is missing or invalid.
func (s *BackupService) nowInConfiguredTZ() time.Time {
	tzName, err := s.settingsRepo.Get(domain.SettingTimezone)
	if err != nil || tzName == "" {
		return time.Now().UTC()
	}
	loc, err := time.LoadLocation(tzName)
	if err != nil {
		return time.Now().UTC()
	}
	return time.Now().In(loc)
}

// BulkBackupResult describes the outcome of a bulk backup request per device.
type BulkBackupResult struct {
	DeviceID   uuid.UUID  `json:"device_id"`
	DeviceName string     `json:"device_name"`
	Status     string     `json:"status"` // "queued", "skipped"
	Reason     string     `json:"reason,omitempty"`
	JobID      *uuid.UUID `json:"job_id,omitempty"`
}

// TriggerBulkBackup validates all devices and queues backups for eligible ones.
func (s *BackupService) TriggerBulkBackup(ctx context.Context) ([]BulkBackupResult, error) {
	devices, err := s.deviceRepo.GetAll()
	if err != nil {
		return nil, fmt.Errorf("fetching devices: %w", err)
	}

	var results []BulkBackupResult
	for i := range devices {
		d := &devices[i]
		name := d.Tags["display_name"]
		if name == "" {
			name = d.SysName
		}
		if name == "" {
			name = d.IP
		}

		profile, err := s.credentialProfileRepo.GetBackupProfileForDevice(d.ID)
		if err != nil {
			results = append(results, BulkBackupResult{
				DeviceID: d.ID, DeviceName: name,
				Status: "skipped", Reason: "no credential profile assigned",
			})
			continue
		}

		backupCfg := s.vendorRegistry.ResolveBackupConfig(d.Vendor)
		if !backupCfg.Supported {
			results = append(results, BulkBackupResult{
				DeviceID: d.ID, DeviceName: name,
				Status: "skipped", Reason: "backup not supported for vendor",
			})
			continue
		}

		// Fast reachability check before creating the job
		if err := ssh.CheckReachable(d.IP, profile.Port, 5*time.Second); err != nil {
			results = append(results, BulkBackupResult{
				DeviceID: d.ID, DeviceName: name,
				Status: "skipped", Reason: "device unreachable",
			})
			continue
		}

		job := &domain.BackupJob{
			ID:       uuid.New(),
			DeviceID: d.ID,
			Status:   domain.BackupStatusPending,
		}
		if err := s.jobRepo.Create(job); err != nil {
			results = append(results, BulkBackupResult{
				DeviceID: d.ID, DeviceName: name,
				Status: "skipped", Reason: fmt.Sprintf("failed to create job: %v", err),
			})
			continue
		}

		go s.runFullBackup(d, profile, backupCfg, job.ID)

		jobID := job.ID
		results = append(results, BulkBackupResult{
			DeviceID: d.ID, DeviceName: name,
			Status: "queued", JobID: &jobID,
		})
	}

	return results, nil
}

// BulkDownloadEntry pairs a backup file with a device-derived folder name.
type BulkDownloadEntry struct {
	File      domain.BackupFile
	DeviceDir string // sanitized device name for zip folder
}

// GetBulkDownloadFiles returns file entries from the latest successful backup of each given device.
func (s *BackupService) GetBulkDownloadFiles(ctx context.Context, deviceIDs []uuid.UUID) ([]BulkDownloadEntry, error) {
	var entries []BulkDownloadEntry
	for _, did := range deviceIDs {
		device, err := s.deviceRepo.GetByID(did)
		if err != nil {
			continue
		}
		job, err := s.jobRepo.GetLatestByDeviceID(did)
		if err != nil || job == nil {
			continue
		}
		files, err := s.fileRepo.GetByJobID(job.ID)
		if err != nil {
			continue
		}
		dirName := device.Tags["display_name"]
		if dirName == "" {
			dirName = device.SysName
		}
		if dirName == "" {
			dirName = device.Hostname
		}
		if dirName == "" {
			dirName = device.IP
		}
		dirName = sanitizeHostname(dirName)

		for _, f := range files {
			entries = append(entries, BulkDownloadEntry{File: f, DeviceDir: dirName})
		}
	}
	return entries, nil
}

// TriggerBackup creates a pending backup job and runs all backup types asynchronously.
func (s *BackupService) TriggerBackup(ctx context.Context, deviceID uuid.UUID) (*domain.BackupJob, error) {
	device, err := s.deviceRepo.GetByID(deviceID)
	if err != nil {
		return nil, fmt.Errorf("getting device: %w", err)
	}

	profile, err := s.credentialProfileRepo.GetBackupProfileForDevice(device.ID)
	if err != nil {
		return nil, fmt.Errorf("no credential profile assigned to device %s", deviceID)
	}

	backupCfg := s.vendorRegistry.ResolveBackupConfig(device.Vendor)
	if !backupCfg.Supported {
		return nil, fmt.Errorf("backup not supported for vendor %s", device.Vendor)
	}

	// Fast reachability check before creating the job
	if err := ssh.CheckReachable(device.IP, profile.Port, 5*time.Second); err != nil {
		return nil, fmt.Errorf("device unreachable: %w", err)
	}

	job := &domain.BackupJob{
		ID:       uuid.New(),
		DeviceID: deviceID,
		Status:   domain.BackupStatusPending,
	}
	if err := s.jobRepo.Create(job); err != nil {
		return nil, fmt.Errorf("creating backup job: %w", err)
	}

	go s.runFullBackup(device, profile, backupCfg, job.ID)

	return job, nil
}

func (s *BackupService) runFullBackup(device *domain.Device, profile *domain.CredentialProfile, backupCfg vendor.BackupConfig, jobID uuid.UUID) {
	lock := s.getDeviceLock(device.ID)
	lock.Lock()
	defer lock.Unlock()

	// Set job to running
	s.updateJobStatus(jobID, domain.BackupStatusRunning, "")

	secret, err := s.decryptSecret(profile.EncryptedSecret)
	if err != nil {
		s.failJob(jobID, fmt.Sprintf("decrypting credentials: %v", err))
		return
	}

	// Connect via SSH
	var client *ssh.Client
	timeout := 30 * time.Second

	if profile.AuthMethod == domain.SSHAuthPassword {
		client, err = ssh.NewClient(s.sshDialer, device.IP, profile.Port, profile.Username, secret, timeout, s.hostKeyCallback)
	} else {
		client, err = ssh.NewClientWithKey(s.sshDialer, device.IP, profile.Port, profile.Username, []byte(secret), timeout, s.hostKeyCallback)
	}
	if err != nil {
		s.failJob(jobID, fmt.Sprintf("SSH connection failed: %v", err))
		return
	}
	defer client.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Second)
	defer cancel()

	// Determine file prefix — try device fields first, then SSH identity
	hostname := sanitizeHostname(device.Tags["display_name"])
	if hostname == "" {
		hostname = sanitizeHostname(device.SysName)
	}
	if hostname == "" {
		// SysName empty (SNMP may have failed) — get identity via SSH
		if identity, identErr := client.RunCommand(ctx, "/system identity print"); identErr == nil {
			identity = strings.TrimSpace(identity)
			// MikroTik returns "name: <identity>", parse it
			if strings.HasPrefix(identity, "name:") {
				identity = strings.TrimSpace(strings.TrimPrefix(identity, "name:"))
			}
			hostname = sanitizeHostname(identity)
		}
	}
	if hostname == "" && device.Hostname != device.IP {
		hostname = sanitizeHostname(device.Hostname)
	}
	if hostname == "" {
		hostname = sanitizeHostname(device.IP)
	}
	log.Printf("Backup file prefix hostname: %q (device: %s)", hostname, device.IP)
	prefix := s.nowInConfiguredTZ().Format("20060102_150405") + "_" + hostname

	// Ensure device backup directory
	deviceDir := filepath.Join(s.backupDir, device.ID.String())
	if err := os.MkdirAll(deviceDir, 0700); err != nil {
		s.failJob(jobID, fmt.Sprintf("creating backup directory: %v", err))
		return
	}
	if err := os.Chmod(deviceDir, 0700); err != nil {
		s.failJob(jobID, fmt.Sprintf("restricting backup directory permissions: %v", err))
		return
	}

	var warnings []string

	// Step A: /export (running)
	if backupCfg.SSHCommands.ExportRunning != "" {
		if err := s.runTextExport(ctx, client, jobID, deviceDir, prefix+".rsc", "running", backupCfg.SSHCommands.ExportRunning); err != nil {
			warnings = append(warnings, fmt.Sprintf("running export: %v", err))
		}
	}

	// Step B: /export verbose
	if backupCfg.SSHCommands.ExportVerbose != "" {
		if err := s.runTextExport(ctx, client, jobID, deviceDir, prefix+"_verbose.rsc", "verbose", backupCfg.SSHCommands.ExportVerbose); err != nil {
			warnings = append(warnings, fmt.Sprintf("verbose export: %v", err))
		}
	}

	// Step C: /export compact
	if backupCfg.SSHCommands.ExportCompact != "" {
		if err := s.runTextExport(ctx, client, jobID, deviceDir, prefix+"_compact.rsc", "compact", backupCfg.SSHCommands.ExportCompact); err != nil {
			warnings = append(warnings, fmt.Sprintf("compact export: %v", err))
		}
	}

	// Step D: Binary backup
	if backupCfg.SSHCommands.BinaryBackup != nil {
		if err := s.runBinaryExport(ctx, client, jobID, deviceDir, prefix+".backup", backupCfg.SSHCommands.BinaryBackup); err != nil {
			warnings = append(warnings, fmt.Sprintf("binary backup: %v", err))
		}
	}

	// Check results
	files, _ := s.fileRepo.GetByJobID(jobID)
	if len(files) == 0 {
		s.failJob(jobID, "all backup types failed: "+strings.Join(warnings, "; "))
		return
	}

	errMsg := ""
	if len(warnings) > 0 {
		errMsg = "partial: " + strings.Join(warnings, "; ")
	}
	s.updateJobStatus(jobID, domain.BackupStatusSuccess, errMsg)
}

// waitForRemoteFile polls for a remote file's existence using SFTP Stat.
func (s *BackupService) waitForRemoteFile(sshClient *gossh.Client, remotePath string, timeout time.Duration) error {
	if sshClient == nil {
		return fmt.Errorf("creating SFTP client for stat: nil SSH client")
	}
	sftpClient, err := sftp.NewClient(sshClient)
	if err != nil {
		return fmt.Errorf("creating SFTP client for stat: %w", err)
	}
	defer sftpClient.Close()

	deadline := time.Now().Add(timeout)
	pollInterval := 500 * time.Millisecond

	for time.Now().Before(deadline) {
		_, err := sftpClient.Stat(remotePath)
		if err == nil {
			return nil // File exists
		}
		if !os.IsNotExist(err) {
			return fmt.Errorf("SFTP stat %q: %w", remotePath, err)
		}
		time.Sleep(pollInterval)
	}

	return fmt.Errorf("timed out waiting for remote file %q after %v", remotePath, timeout)
}

func downloadSFTPFileToDiskAndHash(ctx context.Context, sshClient *gossh.Client, remotePath, localPath string) (string, int, error) {
	if sshClient == nil {
		return "", 0, fmt.Errorf("creating SFTP client: nil SSH client")
	}
	sftpClient, err := sftp.NewClient(sshClient)
	if err != nil {
		return "", 0, fmt.Errorf("creating SFTP client: %w", err)
	}
	defer sftpClient.Close()

	type result struct {
		hash string
		size int
		err  error
	}

	done := make(chan result, 1)
	go func() {
		remoteFile, err := sftpClient.Open(remotePath)
		if err != nil {
			done <- result{err: fmt.Errorf("opening remote file %q: %w", remotePath, err)}
			return
		}
		defer remoteFile.Close()

		dir := filepath.Dir(localPath)
		tmpFile, err := os.CreateTemp(dir, ".theia-download-*")
		if err != nil {
			done <- result{err: fmt.Errorf("creating temp file: %w", err)}
			return
		}
		tmpPath := tmpFile.Name()

		hasher := sha256.New()
		written, err := io.Copy(io.MultiWriter(tmpFile, hasher), remoteFile)
		if err != nil {
			tmpFile.Close()
			os.Remove(tmpPath)
			done <- result{err: fmt.Errorf("downloading file: %w", err)}
			return
		}
		maxInt := int64(int(^uint(0) >> 1))
		if written > maxInt {
			tmpFile.Close()
			os.Remove(tmpPath)
			done <- result{err: fmt.Errorf("downloaded file too large: %d bytes", written)}
			return
		}
		if err := tmpFile.Close(); err != nil {
			os.Remove(tmpPath)
			done <- result{err: fmt.Errorf("closing temp file: %w", err)}
			return
		}

		if err := os.Rename(tmpPath, localPath); err != nil {
			os.Remove(tmpPath)
			done <- result{err: fmt.Errorf("renaming temp file: %w", err)}
			return
		}
		done <- result{
			hash: hex.EncodeToString(hasher.Sum(nil)),
			size: int(written),
		}
	}()

	select {
	case <-ctx.Done():
		return "", 0, ctx.Err()
	case r := <-done:
		return r.hash, r.size, r.err
	}
}

func (s *BackupService) runTextExport(ctx context.Context, client *ssh.Client, jobID uuid.UUID, dir, fileName, fileType, command string) error {
	output, err := client.RunCommand(ctx, command)
	if err != nil {
		return fmt.Errorf("command %q failed: %w", command, err)
	}

	filePath := filepath.Join(dir, fileName)
	if err := os.WriteFile(filePath, []byte(output), 0600); err != nil {
		return fmt.Errorf("writing file: %w", err)
	}
	if err := os.Chmod(filePath, 0600); err != nil {
		return fmt.Errorf("restricting file permissions: %w", err)
	}

	hash := sha256.Sum256([]byte(output))
	return s.fileRepo.Create(&domain.BackupFile{
		ID:        uuid.New(),
		JobID:     jobID,
		FileType:  fileType,
		FileName:  fileName,
		FilePath:  filePath,
		FileHash:  hex.EncodeToString(hash[:]),
		SizeBytes: len(output),
	})
}

func (s *BackupService) runBinaryExport(ctx context.Context, client *ssh.Client, jobID uuid.UUID, dir, fileName string, bcfg *vendor.BinaryBackupCmd) error {
	// Step 1: Run save command
	log.Printf("Binary backup: running save command: %s", bcfg.SaveCommand)
	if _, err := client.RunCommand(ctx, bcfg.SaveCommand); err != nil {
		return fmt.Errorf("save command failed: %w", err)
	}

	// Step 2: Wait for file to appear on remote filesystem via SFTP stat polling
	if err := s.waitForRemoteFile(client.SSHClient(), bcfg.RemoteFilePath, 30*time.Second); err != nil {
		return fmt.Errorf("waiting for remote backup file: %w", err)
	}

	// Step 3: Download via SFTP to disk
	filePath := filepath.Join(dir, fileName)
	log.Printf("Binary backup: downloading file: %s -> %s", bcfg.RemoteFilePath, filePath)
	fileHash, sizeBytes, err := downloadSFTPFileToDiskAndHash(ctx, client.SSHClient(), bcfg.RemoteFilePath, filePath)
	if err != nil {
		return fmt.Errorf("SFTP download failed: %w", err)
	}
	if err := os.Chmod(filePath, 0600); err != nil {
		return fmt.Errorf("restricting downloaded file permissions: %w", err)
	}

	// Step 4: Cleanup remote file
	if bcfg.CleanupCommand != "" {
		log.Printf("Binary backup: cleaning up: %s", bcfg.CleanupCommand)
		if _, cleanErr := client.RunCommand(ctx, bcfg.CleanupCommand); cleanErr != nil {
			log.Printf("Warning: cleanup command failed: %v", cleanErr)
		}
	}

	return s.fileRepo.Create(&domain.BackupFile{
		ID:        uuid.New(),
		JobID:     jobID,
		FileType:  "binary",
		FileName:  fileName,
		FilePath:  filePath,
		FileHash:  fileHash,
		SizeBytes: sizeBytes,
	})
}

func (s *BackupService) updateJobStatus(jobID uuid.UUID, status domain.BackupStatus, errMsg string) {
	job, err := s.jobRepo.GetByID(jobID)
	if err != nil || job == nil {
		log.Printf("Failed to fetch job %s for update: %v", jobID, err)
		return
	}
	job.Status = status
	job.ErrorMessage = errMsg
	if err := s.jobRepo.Update(job); err != nil {
		log.Printf("Failed to update job %s: %v", jobID, err)
	}
}

func (s *BackupService) failJob(jobID uuid.UUID, errMsg string) {
	log.Printf("Backup job %s failed: %s", jobID, errMsg)
	s.updateJobStatus(jobID, domain.BackupStatusFailed, errMsg)
}

func sanitizeHostname(name string) string {
	s := sanitizeRe.ReplaceAllString(name, "_")
	if len(s) > 64 {
		s = s[:64]
	}
	return s
}

func (s *BackupService) decryptSecret(encrypted string) (string, error) {
	if encrypted == "" {
		return "", nil
	}

	var base64DecryptErr error
	if ciphertext, err := base64.StdEncoding.DecodeString(encrypted); err == nil {
		decrypted, err := crypto.Decrypt(ciphertext, s.encryptionKey)
		if err == nil {
			return string(decrypted), nil
		}
		base64DecryptErr = err
	}

	decrypted, err := crypto.Decrypt([]byte(encrypted), s.encryptionKey)
	if err == nil {
		return string(decrypted), nil
	}
	if base64DecryptErr != nil {
		return "", base64DecryptErr
	}
	return "", err
}

// EncryptSecret encrypts a plaintext secret for storage.
func (s *BackupService) EncryptSecret(plaintext string) (string, error) {
	if plaintext == "" {
		return "", nil
	}
	encrypted, err := crypto.Encrypt([]byte(plaintext), s.encryptionKey)
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(encrypted), nil
}

// GetWinboxCredentials retrieves the decrypted WinBox password for a device.
// It fetches the device IP from the device repository and decrypts the
// credential profile's secret in the service layer (T-24-05 mitigation).
// Returns ip, decryptedPassword, and an error. username is returned separately.
func (s *BackupService) GetWinboxCredentials(deviceID uuid.UUID, encryptedSecret, username string) (ip, password string, err error) {
	device, err := s.deviceRepo.GetByID(deviceID)
	if err != nil {
		return "", "", fmt.Errorf("device not found: %w", err)
	}
	pwd, err := s.decryptSecret(encryptedSecret)
	if err != nil {
		return "", "", fmt.Errorf("decrypting credentials: %w", err)
	}
	if pwd == "" {
		return "", "", fmt.Errorf("WinBox profile has no password configured")
	}
	return device.IP, pwd, nil
}

// GetBackupJobs returns all backup jobs for a device.
func (s *BackupService) GetBackupJobs(ctx context.Context, deviceID uuid.UUID) ([]domain.BackupJob, error) {
	jobs, err := s.jobRepo.GetByDeviceID(deviceID)
	if err != nil {
		return nil, err
	}
	// Attach file counts
	for i := range jobs {
		files, _ := s.fileRepo.GetByJobID(jobs[i].ID)
		jobs[i].Files = files
	}
	return jobs, nil
}

// GetBackupJob returns a single backup job with its files.
func (s *BackupService) GetBackupJob(ctx context.Context, id uuid.UUID) (*domain.BackupJob, error) {
	job, err := s.jobRepo.GetByID(id)
	if err != nil {
		return nil, err
	}
	if job == nil {
		return nil, nil
	}
	files, _ := s.fileRepo.GetByJobID(job.ID)
	job.Files = files
	return job, nil
}

// GetLatestBackupJob returns the latest successful backup job with files.
func (s *BackupService) GetLatestBackupJob(ctx context.Context, deviceID uuid.UUID) (*domain.BackupJob, error) {
	job, err := s.jobRepo.GetLatestByDeviceID(deviceID)
	if err != nil {
		return nil, err
	}
	if job == nil {
		return nil, nil
	}
	files, _ := s.fileRepo.GetByJobID(job.ID)
	job.Files = files
	return job, nil
}

// DeleteBackupJob removes a backup job, its files from disk and DB.
func (s *BackupService) DeleteBackupJob(ctx context.Context, id uuid.UUID) error {
	// Get files to delete from disk
	files, _ := s.fileRepo.GetByJobID(id)
	var fileWarnings []string
	for _, f := range files {
		if f.FilePath != "" {
			if err := os.Remove(f.FilePath); err != nil && !os.IsNotExist(err) {
				fileWarnings = append(fileWarnings, fmt.Sprintf("removing %s: %v", f.FilePath, err))
			}
		}
	}
	// Delete file records
	if err := s.fileRepo.DeleteByJobID(id); err != nil {
		return fmt.Errorf("deleting file records: %w", err)
	}
	// Delete job
	if err := s.jobRepo.Delete(id); err != nil {
		return fmt.Errorf("deleting job: %w", err)
	}
	if len(fileWarnings) > 0 {
		log.Printf("Warning: some backup files could not be removed for job %s: %s", id, strings.Join(fileWarnings, "; "))
	}
	return nil
}

// GetBackupFile returns a single backup file by ID.
func (s *BackupService) GetBackupFile(ctx context.Context, id uuid.UUID) (*domain.BackupFile, error) {
	return s.fileRepo.GetByID(id)
}

// GetBackupFileContent opens the backup file for streaming.
// The caller MUST close the returned io.ReadCloser when done.
func (s *BackupService) GetBackupFileContent(ctx context.Context, id uuid.UUID) (io.ReadCloser, string, error) {
	file, err := s.fileRepo.GetByID(id)
	if err != nil {
		return nil, "", err
	}
	if file == nil {
		return nil, "", fmt.Errorf("backup file not found")
	}
	f, err := os.Open(file.FilePath)
	if err != nil {
		return nil, "", fmt.Errorf("opening backup file: %w", err)
	}
	return f, file.FileName, nil
}

// TestSSHConnection tests SSH connectivity to a device using its assigned SSH profile.
func (s *BackupService) TestSSHConnection(ctx context.Context, deviceID uuid.UUID) error {
	device, err := s.deviceRepo.GetByID(deviceID)
	if err != nil {
		return fmt.Errorf("getting device: %w", err)
	}

	profile, err := s.credentialProfileRepo.GetBackupProfileForDevice(device.ID)
	if err != nil {
		return fmt.Errorf("no credential profile assigned to device %s", deviceID)
	}

	secret, err := s.decryptSecret(profile.EncryptedSecret)
	if err != nil {
		return fmt.Errorf("decrypting credentials: %w", err)
	}

	timeout := 10 * time.Second
	var client *ssh.Client

	if profile.AuthMethod == domain.SSHAuthPassword {
		client, err = ssh.NewClient(s.sshDialer, device.IP, profile.Port, profile.Username, secret, timeout, s.hostKeyCallback)
	} else {
		client, err = ssh.NewClientWithKey(s.sshDialer, device.IP, profile.Port, profile.Username, []byte(secret), timeout, s.hostKeyCallback)
	}
	if err != nil {
		return fmt.Errorf("SSH connection failed: %w", err)
	}
	defer client.Close()

	return nil
}

// TestCredentialProfile tests SSH connectivity using a credential profile against a target IP.
func (s *BackupService) TestCredentialProfile(ctx context.Context, profileID uuid.UUID, targetIP string) error {
	profile, err := s.credentialProfileRepo.GetByID(profileID)
	if err != nil {
		return fmt.Errorf("getting credential profile: %w", err)
	}

	secret, err := s.decryptSecret(profile.EncryptedSecret)
	if err != nil {
		return fmt.Errorf("decrypting credentials: %w", err)
	}

	timeout := 10 * time.Second
	var client *ssh.Client

	if profile.AuthMethod == domain.SSHAuthPassword {
		client, err = ssh.NewClient(s.sshDialer, targetIP, profile.Port, profile.Username, secret, timeout, s.hostKeyCallback)
	} else {
		client, err = ssh.NewClientWithKey(s.sshDialer, targetIP, profile.Port, profile.Username, []byte(secret), timeout, s.hostKeyCallback)
	}
	if err != nil {
		return fmt.Errorf("SSH connection failed: %w", err)
	}
	defer client.Close()

	return nil
}

// CreateCredentialProfile creates a new credential profile.
// If role is empty, it defaults to "Admin".
func (s *BackupService) CreateCredentialProfile(ctx context.Context, name, description, username string, port int, authMethod domain.SSHAuthMethod, secret string, role string) (*domain.CredentialProfile, error) {
	encryptedSecret, err := s.EncryptSecret(secret)
	if err != nil {
		return nil, fmt.Errorf("encrypting secret: %w", err)
	}

	if role == "" {
		role = "Admin"
	}

	profile := &domain.CredentialProfile{
		Name:            name,
		Description:     description,
		Username:        username,
		Port:            port,
		AuthMethod:      authMethod,
		EncryptedSecret: encryptedSecret,
		Role:            role,
	}
	if err := s.credentialProfileRepo.Create(profile); err != nil {
		return nil, fmt.Errorf("creating credential profile: %w", err)
	}
	return profile, nil
}

// GetCredentialProfile returns a credential profile by ID.
func (s *BackupService) GetCredentialProfile(ctx context.Context, id uuid.UUID) (*domain.CredentialProfile, error) {
	return s.credentialProfileRepo.GetByID(id)
}

// GetAllCredentialProfiles returns all credential profiles.
func (s *BackupService) GetAllCredentialProfiles(ctx context.Context) ([]domain.CredentialProfile, error) {
	return s.credentialProfileRepo.GetAll()
}

// UpdateCredentialProfile updates an existing credential profile. If secret is empty, the existing secret is kept.
// If role is empty, it defaults to "Admin".
func (s *BackupService) UpdateCredentialProfile(ctx context.Context, id uuid.UUID, name, description, username string, port int, authMethod domain.SSHAuthMethod, secret string, role string) (*domain.CredentialProfile, error) {
	profile, err := s.credentialProfileRepo.GetByID(id)
	if err != nil {
		return nil, fmt.Errorf("getting credential profile: %w", err)
	}

	profile.Name = name
	profile.Description = description
	profile.Username = username
	profile.Port = port
	profile.AuthMethod = authMethod

	if role == "" {
		role = "Admin"
	}
	profile.Role = role

	if secret != "" {
		encryptedSecret, err := s.EncryptSecret(secret)
		if err != nil {
			return nil, fmt.Errorf("encrypting secret: %w", err)
		}
		profile.EncryptedSecret = encryptedSecret
	}

	if err := s.credentialProfileRepo.Update(profile); err != nil {
		return nil, fmt.Errorf("updating credential profile: %w", err)
	}
	return profile, nil
}

// DeleteCredentialProfile removes a credential profile. Returns an error if any device references it.
func (s *BackupService) DeleteCredentialProfile(ctx context.Context, id uuid.UUID) error {
	return s.credentialProfileRepo.Delete(id)
}
