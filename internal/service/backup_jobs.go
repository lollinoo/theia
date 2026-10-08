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
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	release, err := s.admitManualBackup(ctx)
	if err != nil {
		return nil, err
	}
	transferred := false
	defer func() {
		if !transferred {
			release()
		}
	}()

	getDevice := s.deviceRepo.GetByID
	if repo, ok := s.deviceRepo.(interface {
		GetByIDContext(context.Context, uuid.UUID) (*domain.Device, error)
	}); ok {
		getDevice = func(id uuid.UUID) (*domain.Device, error) { return repo.GetByIDContext(ctx, id) }
	}
	device, err := getDevice(deviceID)
	if err != nil {
		return nil, fmt.Errorf("getting device: %w", err)
	}

	getProfile := s.credentialProfileRepo.GetBackupProfileForDevice
	if repo, ok := s.credentialProfileRepo.(interface {
		GetBackupProfileForDeviceContext(context.Context, uuid.UUID) (*domain.CredentialProfile, error)
	}); ok {
		getProfile = func(id uuid.UUID) (*domain.CredentialProfile, error) {
			return repo.GetBackupProfileForDeviceContext(ctx, id)
		}
	}
	profile, err := getProfile(device.ID)
	if err != nil {
		return nil, fmt.Errorf("no credential profile assigned to device %s", deviceID)
	}

	backupCfg := s.vendorRegistry.ResolveBackupConfig(device.Vendor)
	if !backupCfg.Supported {
		return nil, fmt.Errorf("backup not supported for vendor %s", device.Vendor)
	}

	// Fast reachability check before creating the job
	if err := ssh.CheckReachableContext(ctx, domain.BackupAddress(*device), profile.Port, 5*time.Second); err != nil {
		return nil, fmt.Errorf("device unreachable: %w", err)
	}

	job := &domain.BackupJob{
		ID:       uuid.New(),
		DeviceID: deviceID,
		Status:   domain.BackupStatusPending,
	}
	createJob := s.jobRepo.Create
	if repo, ok := s.jobRepo.(interface {
		CreateContext(context.Context, *domain.BackupJob) error
	}); ok {
		createJob = func(job *domain.BackupJob) error { return repo.CreateContext(ctx, job) }
	}
	if err := createJob(job); err != nil {
		return nil, fmt.Errorf("creating backup job: %w", err)
	}

	transferred = true
	go func() { defer release(); s.runFullBackupReserved(device, profile, backupCfg, job.ID) }()

	return job, nil
}

func (s *BackupService) updateJobStatus(jobID uuid.UUID, status domain.BackupStatus, errMsg string) error {
	job, err := s.jobRepo.GetByID(jobID)
	if err != nil || job == nil {
		log.Printf("Failed to fetch job %s for update: %v", jobID, err)
		if err != nil {
			return err
		}
		return fmt.Errorf("backup job %s not found", jobID)
	}
	job.Status = status
	job.ErrorMessage = errMsg
	if err := s.jobRepo.Update(job); err != nil {
		log.Printf("Failed to update job %s: %v", jobID, err)
		return err
	}
	return nil
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
	ids := make([]uuid.UUID, len(jobs))
	for i := range jobs {
		ids[i] = jobs[i].ID
	}
	files, err := s.backupFilesByJobIDs(ctx, ids)
	if err != nil {
		return nil, err
	}
	for i := range jobs {
		jobs[i].Files = files[jobs[i].ID]
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
	files, err := s.fileRepo.GetByJobID(job.ID)
	if err != nil {
		return nil, fmt.Errorf("loading files for backup job %s: %w", job.ID, err)
	}
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
	files, err := s.fileRepo.GetByJobID(job.ID)
	if err != nil {
		return nil, fmt.Errorf("loading files for backup job %s: %w", job.ID, err)
	}
	job.Files = files
	return job, nil
}

// DeleteBackupJob atomically removes job and file metadata. Durable repositories
// queue physical file removal for the cleanup worker after the deletion commits.
func (s *BackupService) DeleteBackupJob(ctx context.Context, id uuid.UUID) error {
	if err := contextError(ctx); err != nil {
		return err
	}
	getJob := s.jobRepo.GetByID
	if repo, ok := s.jobRepo.(interface {
		GetByIDContext(context.Context, uuid.UUID) (*domain.BackupJob, error)
	}); ok {
		getJob = func(id uuid.UUID) (*domain.BackupJob, error) { return repo.GetByIDContext(ctx, id) }
	}
	job, err := getJob(id)
	if err != nil {
		return err
	}
	if job == nil {
		return fmt.Errorf("backup job %s not found", id)
	}
	if job.Status == domain.BackupStatusPending || job.Status == domain.BackupStatusRunning {
		return ErrBackupJobActive
	}
	referenced := false
	if repo, ok := s.bulkRunRepo.(interface {
		BackupJobReferencedByActiveRunContext(context.Context, uuid.UUID) (bool, error)
	}); ok {
		referenced, err = repo.BackupJobReferencedByActiveRunContext(ctx, id)
		if err != nil {
			return fmt.Errorf("checking active bulk run references: %w", err)
		}
	} else {
		referenced = s.backupJobReferencedByActiveBulkRun(id)
	}
	if referenced {
		return ErrBackupJobReferencedByActiveBulkRun
	}

	getFiles := s.fileRepo.GetByJobID
	if repo, ok := s.fileRepo.(interface {
		GetByJobIDContext(context.Context, uuid.UUID) ([]domain.BackupFile, error)
	}); ok {
		getFiles = func(id uuid.UUID) ([]domain.BackupFile, error) { return repo.GetByJobIDContext(ctx, id) }
	}
	files, err := getFiles(id)
	if err != nil {
		return fmt.Errorf("loading file records: %w", err)
	}
	backupRoot, err := validatedBackupRoot(s.backupDir)
	if err != nil && len(files) > 0 {
		return err
	}
	paths := make([]string, 0, len(files))
	for _, f := range files {
		if err := contextError(ctx); err != nil {
			return err
		}
		if f.FilePath != "" {
			removePath, err := validateBackupDeletionPath(backupRoot, f.FilePath)
			if err != nil {
				return err
			}
			paths = append(paths, removePath)
		}
	}
	// The job deletion cascades to file metadata and its durable cleanup entries
	// in one database statement. Never remove disk files before this succeeds.
	if err := contextError(ctx); err != nil {
		return err
	}
	deleteJob := s.jobRepo.Delete
	if repo, ok := s.jobRepo.(interface {
		DeleteContext(context.Context, uuid.UUID) error
	}); ok {
		deleteJob = func(id uuid.UUID) error { return repo.DeleteContext(ctx, id) }
	}
	if err := deleteJob(id); err != nil {
		return fmt.Errorf("deleting job: %w", err)
	}
	if _, ok := s.fileRepo.(domain.BackupFileDeletionRepository); ok {
		return nil
	}

	// Repositories without a durable queue clean up only after metadata deletion.
	var fileWarnings []string
	for _, path := range paths {
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			fileWarnings = append(fileWarnings, fmt.Sprintf("removing %s: %v", path, err))
		}
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
	file, err := s.fileRepo.GetByID(id)
	if err != nil || file == nil {
		return file, err
	}
	resolvedPath := s.resolveBackupFilePath(file)
	if resolvedPath == file.FilePath {
		return file, nil
	}
	resolved := *file
	resolved.FilePath = resolvedPath
	return &resolved, nil
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
	filePath := s.resolveBackupFilePath(file)
	f, err := os.Open(filePath)
	if err != nil {
		return nil, "", fmt.Errorf("opening backup file: %w", err)
	}
	return f, file.FileName, nil
}

func (s *BackupService) resolveBackupFilePath(file *domain.BackupFile) string {
	if file == nil {
		return ""
	}
	if _, err := os.Stat(file.FilePath); err == nil {
		return file.FilePath
	}
	fallback := s.restoredBackupFilePath(file)
	if fallback == "" {
		return file.FilePath
	}
	if _, err := os.Stat(fallback); err == nil {
		return fallback
	}
	return file.FilePath
}

func (s *BackupService) restoredBackupFilePath(file *domain.BackupFile) string {
	if file == nil || strings.TrimSpace(s.backupDir) == "" {
		return ""
	}
	deviceDir := filepath.Base(filepath.Dir(filepath.Clean(file.FilePath)))
	fileName := filepath.Base(file.FileName)
	if deviceDir == "" || deviceDir == "." || fileName == "" || fileName == "." {
		return ""
	}
	backupRoot, err := validatedBackupRoot(s.backupDir)
	if err != nil {
		return ""
	}
	candidate := filepath.Join(backupRoot, deviceDir, fileName)
	if !pathIsUnderDir(backupRoot, candidate) {
		return ""
	}
	return candidate
}
