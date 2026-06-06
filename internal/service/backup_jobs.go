package service

// This file defines backup jobs backup and restore service behavior, including filesystem safety and cleanup expectations.

import (
	"context"
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
)

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
	if err := contextError(ctx); err != nil {
		return err
	}
	job, err := s.jobRepo.GetByID(id)
	if err != nil {
		return err
	}
	if job == nil {
		return fmt.Errorf("backup job %s not found", id)
	}
	if job.Status == domain.BackupStatusPending || job.Status == domain.BackupStatusRunning {
		return ErrBackupJobActive
	}
	if s.backupJobReferencedByActiveBulkRun(id) {
		return ErrBackupJobReferencedByActiveBulkRun
	}

	files, err := s.fileRepo.GetByJobID(id)
	if err != nil {
		return fmt.Errorf("loading file records: %w", err)
	}
	backupRoot, err := validatedBackupRoot(s.backupDir)
	if err != nil && len(files) > 0 {
		return err
	}
	var fileWarnings []string
	for _, f := range files {
		if f.FilePath != "" {
			removePath, err := validateBackupDeletionPath(backupRoot, f.FilePath)
			if err != nil {
				return err
			}
			if err := os.Remove(removePath); err != nil && !os.IsNotExist(err) {
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

func (s *BackupService) backupJobReferencedByActiveBulkRun(id uuid.UUID) bool {
	if s.bulkRunRepo == nil {
		return false
	}
	run, err := s.bulkRunRepo.GetActiveRun()
	if err != nil || run == nil {
		return false
	}
	for _, item := range run.Items {
		if item.BackupJobID == nil || *item.BackupJobID != id || bulkRunItemTerminal(item.Status) {
			continue
		}
		return true
	}
	return false
}

func validateBackupDeletionPath(backupRoot, filePath string) (string, error) {
	if strings.TrimSpace(filePath) == "" {
		return "", &BulkPathError{Reason: "backup file path is empty"}
	}
	absPath, err := filepath.Abs(filePath)
	if err != nil {
		return "", fmt.Errorf("resolving backup file path: %w", err)
	}
	cleanPath := filepath.Clean(absPath)
	if !pathIsUnderDir(backupRoot, cleanPath) {
		return "", &BulkPathError{Path: filePath, Reason: "backup file path is outside backup directory"}
	}
	info, err := os.Lstat(cleanPath)
	if err != nil {
		if os.IsNotExist(err) {
			return cleanPath, nil
		}
		return "", fmt.Errorf("lstat backup file: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return "", &BulkPathError{Path: filePath, Reason: "backup file path is a symlink"}
	}
	if !info.Mode().IsRegular() {
		return "", &BulkPathError{Path: filePath, Reason: "backup file path is not a regular file"}
	}
	resolvedPath, err := filepath.EvalSymlinks(cleanPath)
	if err != nil {
		return "", fmt.Errorf("resolving backup file symlinks: %w", err)
	}
	if !pathIsUnderDir(backupRoot, resolvedPath) {
		return "", &BulkPathError{Path: filePath, Reason: "backup file path is outside backup directory"}
	}
	return cleanPath, nil
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
