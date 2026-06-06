package service

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	gossh "golang.org/x/crypto/ssh"

	"github.com/lollinoo/theia/internal/crypto"
	"github.com/lollinoo/theia/internal/domain"
	"github.com/lollinoo/theia/internal/ssh"
	"github.com/lollinoo/theia/internal/vendor"
)

var sanitizeRe = regexp.MustCompile(`[^a-zA-Z0-9_.-]`)

const defaultBulkBackupWorkerCount = 4
const defaultBulkBackupRunBatchSize = 10
const legacyBulkBackupLeaseKey = "backup.bulk_backup:legacy"
const legacyBulkBackupMetricOperation = "bulk_backup_legacy"

var ErrBulkBackupRunAlreadyActive = errors.New("bulk backup run already active")
var ErrBulkBackupAlreadyActive = errors.New("bulk backup already active")
var ErrBulkOperationLimiterUnavailable = errors.New("bulk operation limiter unavailable")

// BackupService orchestrates credential profile management and config backups.
type BackupService struct {
	jobRepo                domain.BackupJobRepository
	fileRepo               domain.BackupFileRepository
	credentialProfileRepo  domain.CredentialProfileRepository
	deviceRepo             domain.DeviceRepository
	settingsRepo           domain.SettingsRepository
	vendorRegistry         *vendor.Registry
	sshDialer              ssh.Dialer
	encryptionKeyring      *crypto.Keyring
	legacyEncryptionKey    []byte
	backupDir              string
	hostKeyCallback        gossh.HostKeyCallback
	deviceLocks            sync.Map // per-device mutex: map[uuid.UUID]*sync.Mutex
	bulkLimits             BulkOperationLimits
	bulkRunRepo            domain.BulkBackupRunRepository
	bulkOperationLeaseRepo domain.BulkOperationLeaseRepository
	legacyBulkBackupGate   bulkOperationGate
}

type BackupServiceOption func(*BackupService)

func WithBulkBackupRunRepo(repo domain.BulkBackupRunRepository) BackupServiceOption {
	return func(s *BackupService) {
		s.bulkRunRepo = repo
	}
}

func WithBulkOperationLeaseRepository(repo domain.BulkOperationLeaseRepository) BackupServiceOption {
	return func(s *BackupService) {
		s.bulkOperationLeaseRepo = repo
		if repo != nil {
			recordLegacyBulkBackupDistributedConcurrencyLimit()
		}
	}
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
	encryptionKey any,
	backupDir string,
	hostKeyCallback gossh.HostKeyCallback,
	opts ...BackupServiceOption,
) *BackupService {
	keyring, legacyKey := normalizeBackupEncryptionKey(encryptionKey)
	svc := &BackupService{
		jobRepo:               jobRepo,
		fileRepo:              fileRepo,
		credentialProfileRepo: credentialProfileRepo,
		deviceRepo:            deviceRepo,
		settingsRepo:          settingsRepo,
		vendorRegistry:        vendorRegistry,
		sshDialer:             sshDialer,
		encryptionKeyring:     keyring,
		legacyEncryptionKey:   legacyKey,
		backupDir:             backupDir,
		hostKeyCallback:       hostKeyCallback,
		bulkLimits:            DefaultBulkOperationLimits,
	}
	for _, opt := range opts {
		opt(svc)
	}
	recordLegacyBulkBackupLocalConcurrencyLimit()
	return svc
}

func normalizeBackupEncryptionKey(key any) (*crypto.Keyring, []byte) {
	switch k := key.(type) {
	case *crypto.Keyring:
		return k, nil
	case []byte:
		keyring, err := crypto.NewKeyringFromLegacyKey(k)
		if err != nil {
			return nil, k
		}
		return keyring, k
	case nil:
		return nil, nil
	default:
		return nil, nil
	}
}

// SetBulkOperationLimits overrides bulk request quotas.
func (s *BackupService) SetBulkOperationLimits(limits BulkOperationLimits) {
	s.bulkLimits = normalizeBulkOperationLimits(limits)
}

// BulkOperationLimits returns the effective bulk request quotas.
func (s *BackupService) BulkOperationLimits() BulkOperationLimits {
	return normalizeBulkOperationLimits(s.bulkLimits)
}

func (s *BackupService) BulkOperationLeaseRepositoryConfigured() bool {
	return s != nil && s.bulkOperationLeaseRepo != nil
}

func (s *BackupService) BulkBackupRunRepositoryConfigured() bool {
	return s != nil && s.bulkRunRepo != nil
}

func (s *BackupService) BulkBackupRunBatchSize() int {
	return defaultBulkBackupRunBatchSize
}

func (s *BackupService) SetBulkOperationLeaseRepository(repo domain.BulkOperationLeaseRepository) {
	s.bulkOperationLeaseRepo = repo
	if repo != nil {
		recordLegacyBulkBackupDistributedConcurrencyLimit()
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

type queuedDeviceBackup struct {
	device    domain.Device
	profile   *domain.CredentialProfile
	backupCfg vendor.BackupConfig
	jobID     uuid.UUID
}

type preparedDeviceBackup struct {
	device      domain.Device
	profile     *domain.CredentialProfile
	backupCfg   vendor.BackupConfig
	resultIndex int
}

// TriggerBulkBackup validates all devices and queues backups for eligible ones.
func (s *BackupService) TriggerBulkBackup(ctx context.Context, requestedDeviceIDs ...uuid.UUID) ([]BulkBackupResult, error) {
	lease, err := s.acquireLegacyBulkBackupLease(ctx)
	if err != nil {
		return nil, err
	}
	leaseTransferred := false
	defer func() {
		if !leaseTransferred {
			releaseBulkOperationLease("legacy backup", lease)
		}
	}()

	devices, err := s.bulkBackupDevices(ctx, requestedDeviceIDs)
	if err != nil {
		return nil, err
	}

	var results []BulkBackupResult
	preparedBackups := make([]preparedDeviceBackup, 0)
	for i := range devices {
		if err := contextError(ctx); err != nil {
			return nil, err
		}
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

		results = append(results, BulkBackupResult{
			DeviceID: d.ID, DeviceName: name,
			Status: "queued",
		})
		preparedBackups = append(preparedBackups, preparedDeviceBackup{
			device:      *d,
			profile:     profile,
			backupCfg:   backupCfg,
			resultIndex: len(results) - 1,
		})
	}

	limits := s.BulkOperationLimits()
	if len(preparedBackups) > limits.BulkBackupMaxQueuedJobs {
		return nil, &BulkLimitError{
			Operation: "bulk backup",
			Limit:     "queued jobs",
			Max:       int64(limits.BulkBackupMaxQueuedJobs),
			Actual:    int64(len(preparedBackups)),
		}
	}

	queuedBackups := make([]queuedDeviceBackup, 0, len(preparedBackups))
	for _, prepared := range preparedBackups {
		if err := contextError(ctx); err != nil {
			return nil, err
		}
		job := &domain.BackupJob{
			ID:       uuid.New(),
			DeviceID: prepared.device.ID,
			Status:   domain.BackupStatusPending,
		}
		if err := s.jobRepo.Create(job); err != nil {
			results[prepared.resultIndex].Status = "skipped"
			results[prepared.resultIndex].Reason = fmt.Sprintf("failed to create job: %v", err)
			continue
		}

		queuedBackups = append(queuedBackups, queuedDeviceBackup{
			device:    prepared.device,
			profile:   prepared.profile,
			backupCfg: prepared.backupCfg,
			jobID:     job.ID,
		})

		jobID := job.ID
		results[prepared.resultIndex].JobID = &jobID
	}

	if len(queuedBackups) == 0 {
		return results, nil
	}

	leaseTransferred = true
	s.startBulkBackupWorkers(queuedBackups, func() {
		releaseBulkOperationLease("legacy backup", lease)
	})

	return results, nil
}

func (s *BackupService) startBulkBackupWorkers(queuedBackups []queuedDeviceBackup, onComplete ...func()) {
	runCompleteCallbacks := func() {
		for _, callback := range onComplete {
			if callback != nil {
				callback()
			}
		}
	}
	if len(queuedBackups) == 0 {
		runCompleteCallbacks()
		return
	}

	workerCount := defaultBulkBackupWorkerCount
	if len(queuedBackups) < workerCount {
		workerCount = len(queuedBackups)
	}

	go func() {
		defer runCompleteCallbacks()
		jobs := make(chan queuedDeviceBackup)
		var wg sync.WaitGroup
		wg.Add(workerCount)
		for i := 0; i < workerCount; i++ {
			go func() {
				defer wg.Done()
				for job := range jobs {
					device := job.device
					s.runFullBackup(&device, job.profile, job.backupCfg, job.jobID)
				}
			}()
		}
		for _, job := range queuedBackups {
			jobs <- job
		}
		close(jobs)
		wg.Wait()
	}()
}

func contextError(ctx context.Context) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
		return nil
	}
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
	if s.encryptionKeyring != nil {
		if crypto.IsEnvelope(encrypted) {
			return s.encryptionKeyring.DecryptString(encrypted)
		}
		return "", fmt.Errorf("credential secret is not a versioned encryption envelope")
	}

	var base64DecryptErr error
	if ciphertext, err := base64.StdEncoding.DecodeString(encrypted); err == nil {
		decrypted, err := crypto.Decrypt(ciphertext, s.legacyEncryptionKey)
		if err == nil {
			return string(decrypted), nil
		}
		base64DecryptErr = err
	}

	decrypted, err := crypto.Decrypt([]byte(encrypted), s.legacyEncryptionKey)
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
	if s.encryptionKeyring != nil {
		return s.encryptionKeyring.EncryptString(plaintext)
	}
	return "", fmt.Errorf("encryption keyring is required")
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
