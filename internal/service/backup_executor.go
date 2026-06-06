package service

// This file defines backup executor backup and restore service behavior, including filesystem safety and cleanup expectations.

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/lollinoo/theia/internal/domain"
	"github.com/lollinoo/theia/internal/ssh"
	"github.com/lollinoo/theia/internal/vendor"
	"github.com/pkg/sftp"
	gossh "golang.org/x/crypto/ssh"
)

// runFullBackup executes all configured SSH/SFTP backup commands for one device.
// The per-device lock serializes access to remote backup filenames and the job row transitions
// from running to success or failed before the background worker releases its bulk lease.
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

// downloadSFTPFileToDiskAndHash streams a remote SFTP file into a temp file, hashes it, then renames atomically.
// Cancellation stops waiting for the transfer result; the worker goroutine still cleans up partial temp files.
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

// runTextExport captures a command response as a local backup file and records its SHA-256 metadata.
// The final file is written through a temp path so repository rows never point at partial content.
func (s *BackupService) runTextExport(ctx context.Context, client *ssh.Client, jobID uuid.UUID, dir, fileName, fileType, command string) error {
	filePath := filepath.Join(dir, fileName)
	tmpFile, err := os.CreateTemp(dir, ".theia-export-*")
	if err != nil {
		return fmt.Errorf("creating temp export file: %w", err)
	}
	tmpPath := tmpFile.Name()

	hasher := sha256.New()
	counter := &countingWriter{w: io.MultiWriter(tmpFile, hasher)}
	if err := client.RunCommandToWriter(ctx, command, counter); err != nil {
		tmpFile.Close()
		os.Remove(tmpPath)
		return fmt.Errorf("command %q failed: %w", command, err)
	}
	maxInt := int64(int(^uint(0) >> 1))
	if counter.n > maxInt {
		tmpFile.Close()
		os.Remove(tmpPath)
		return fmt.Errorf("export output too large: %d bytes", counter.n)
	}
	if err := tmpFile.Close(); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("closing temp export file: %w", err)
	}
	if err := os.Chmod(tmpPath, 0600); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("restricting temp file permissions: %w", err)
	}
	if err := os.Rename(tmpPath, filePath); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("renaming temp export file: %w", err)
	}
	if err := os.Chmod(filePath, 0600); err != nil {
		os.Remove(filePath)
		return fmt.Errorf("restricting file permissions: %w", err)
	}

	return s.createBackupFileOrRemoveLocal(&domain.BackupFile{
		ID:        uuid.New(),
		JobID:     jobID,
		FileType:  fileType,
		FileName:  fileName,
		FilePath:  filePath,
		FileHash:  hex.EncodeToString(hasher.Sum(nil)),
		SizeBytes: int(counter.n),
	})
}

type countingWriter struct {
	w io.Writer
	n int64
}

func (w *countingWriter) Write(p []byte) (int, error) {
	n, err := w.w.Write(p)
	w.n += int64(n)
	return n, err
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
		os.Remove(filePath)
		return fmt.Errorf("restricting downloaded file permissions: %w", err)
	}

	// Step 4: Cleanup remote file
	if bcfg.CleanupCommand != "" {
		log.Printf("Binary backup: cleaning up: %s", bcfg.CleanupCommand)
		if _, cleanErr := client.RunCommand(ctx, bcfg.CleanupCommand); cleanErr != nil {
			log.Printf("Warning: cleanup command failed: %v", cleanErr)
		}
	}

	return s.createBackupFileOrRemoveLocal(&domain.BackupFile{
		ID:        uuid.New(),
		JobID:     jobID,
		FileType:  "binary",
		FileName:  fileName,
		FilePath:  filePath,
		FileHash:  fileHash,
		SizeBytes: sizeBytes,
	})
}

func (s *BackupService) createBackupFileOrRemoveLocal(file *domain.BackupFile) error {
	if err := s.fileRepo.Create(file); err != nil {
		if file != nil && strings.TrimSpace(file.FilePath) != "" {
			if removeErr := os.Remove(file.FilePath); removeErr != nil && !os.IsNotExist(removeErr) {
				return fmt.Errorf("recording backup file metadata: %w; removing untracked file: %v", err, removeErr)
			}
		}
		return fmt.Errorf("recording backup file metadata: %w", err)
	}
	return nil
}
