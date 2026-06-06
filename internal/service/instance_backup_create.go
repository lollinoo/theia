package service

// This file defines instance backup create backup and restore service behavior, including filesystem safety and cleanup expectations.

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/lollinoo/theia/internal/domain"
)

// Create produces a full instance backup archive synchronously with trigger set to "manual".
func (s *InstanceBackupService) Create(ctx context.Context) (*domain.InstanceBackup, error) {
	return s.CreateWithTrigger(ctx, domain.InstanceBackupTriggerManual)
}

// CreateWithTrigger produces a full instance backup archive containing the database,
// device config files, SSH known_hosts, and a manifest with integrity metadata.
// The trigger field records what initiated the backup (manual or scheduled).
func (s *InstanceBackupService) CreateWithTrigger(ctx context.Context, trigger domain.InstanceBackupTrigger) (*domain.InstanceBackup, error) {
	backup, backupSubDir, err := s.prepareInstanceBackup(trigger)
	if err != nil {
		return nil, err
	}
	return s.runPreparedInstanceBackup(ctx, backup, backupSubDir)
}

// StartCreateWithTrigger creates a running instance backup record and starts archive work in the background.
// Cancellation from the caller is bridged into an internal run context so Cancel can stop long archive steps.
func (s *InstanceBackupService) StartCreateWithTrigger(ctx context.Context, trigger domain.InstanceBackupTrigger) (*domain.InstanceBackup, error) {
	backup, backupSubDir, err := s.prepareInstanceBackup(trigger)
	if err != nil {
		return nil, err
	}
	runCtx, cancel := s.backupRunContext(ctx)
	s.beginInstanceBackupOperation(backup.ID, cancel, domain.InstanceBackupProgress{
		Phase:   "queued",
		Message: "Backup queued",
	})
	go func() {
		defer s.endInstanceBackupOperation(backup.ID)
		defer cancel()
		if _, err := s.runPreparedInstanceBackupWithContext(runCtx, backup, backupSubDir, false); err != nil {
			log.Printf("Instance backup %s failed: %v", backup.ID, err)
		}
	}()
	return backup, nil
}

// prepareInstanceBackup creates the filesystem and repository state for a new run.
// The create mutex serializes run admission and protects the invariant that only one backup is running.
func (s *InstanceBackupService) prepareInstanceBackup(trigger domain.InstanceBackupTrigger) (*domain.InstanceBackup, string, error) {
	s.createMu.Lock()
	defer s.createMu.Unlock()

	if s.hasActiveInstanceBackupOperation() {
		return nil, "", ErrInstanceBackupAlreadyRunning
	}
	if running, err := s.hasRunningInstanceBackup(); err != nil {
		return nil, "", fmt.Errorf("checking running backups: %w", err)
	} else if running {
		return nil, "", ErrInstanceBackupAlreadyRunning
	}

	backupID := uuid.New()
	now := time.Now().UTC()

	fileName := fmt.Sprintf("theia-backup-%s.tar.gz", now.Format("20060102-150405"))

	// Create backup subdirectory: {backupDir}/{backupID}/
	backupSubDir := filepath.Join(s.backupDir, backupID.String())
	if err := os.MkdirAll(backupSubDir, 0700); err != nil {
		return nil, "", fmt.Errorf("creating backup directory: %w", err)
	}

	// Create initial DB record with status "running"
	backup := &domain.InstanceBackup{
		ID:       backupID,
		FileName: fileName,
		Status:   domain.InstanceBackupStatusRunning,
		Trigger:  trigger,
	}
	if err := s.repo.Create(backup); err != nil {
		os.RemoveAll(backupSubDir)
		return nil, "", fmt.Errorf("creating backup record: %w", err)
	}

	return backup, backupSubDir, nil
}

// runPreparedInstanceBackup runs a prepared backup synchronously with its own tracker entry.
func (s *InstanceBackupService) runPreparedInstanceBackup(ctx context.Context, backup *domain.InstanceBackup, backupSubDir string) (*domain.InstanceBackup, error) {
	runCtx, cancel := s.backupRunContext(ctx)
	s.beginInstanceBackupOperation(backup.ID, cancel, domain.InstanceBackupProgress{
		Phase:   "starting",
		Message: "Preparing backup",
	})
	defer s.endInstanceBackupOperation(backup.ID)
	defer cancel()
	return s.runPreparedInstanceBackupWithContext(runCtx, backup, backupSubDir, true)
}

// runPreparedInstanceBackupWithContext performs database dump, manifest, archive, hash, and persistence steps.
// On any failure or cancellation it updates the repository row to failed/cancelled and removes partial files.
func (s *InstanceBackupService) runPreparedInstanceBackupWithContext(ctx context.Context, backup *domain.InstanceBackup, backupSubDir string, ownOperation bool) (*domain.InstanceBackup, error) {
	if err := ctx.Err(); err != nil {
		s.cleanupFailedInstanceBackup(backup, backupSubDir, "backup cancelled", err)
		return nil, err
	}

	limits := s.BackupArchiveLimits()

	cleanupOnError := func(errMsg string, err error) {
		s.cleanupFailedInstanceBackup(backup, backupSubDir, errMsg, err)
	}

	s.updateInstanceBackupProgress(backup.ID, domain.InstanceBackupProgress{
		Phase:   "database",
		Message: "Creating database snapshot",
	})

	// Step 1: Create a PostgreSQL database dump.
	dbArtifact, err := s.backupDatabase(ctx, backupSubDir)
	if err != nil {
		cleanupOnError(fmt.Sprintf("backing up database: %v", err), err)
		return nil, fmt.Errorf("backing up database: %w", err)
	}
	defer os.Remove(dbArtifact.tempPath)

	dbInfo, err := os.Stat(dbArtifact.tempPath)
	if err != nil {
		cleanupOnError(fmt.Sprintf("statting database backup: %v", err), err)
		return nil, fmt.Errorf("statting database backup: %w", err)
	}
	if err := checkBackupArchiveEntryQuota(dbArtifact.archiveEntryName, dbInfo.Size(), limits); err != nil {
		cleanupOnError(fmt.Sprintf("checking database archive quota: %v", err), err)
		return nil, err
	}

	s.updateInstanceBackupProgress(backup.ID, domain.InstanceBackupProgress{
		Phase:   "hashing",
		Message: "Hashing database snapshot",
		Current: dbInfo.Size(),
		Total:   dbInfo.Size(),
	})

	// Step 2: Compute SHA-256 of the database copy
	dbHash, err := computeFileHashContext(ctx, dbArtifact.tempPath)
	if err != nil {
		cleanupOnError(fmt.Sprintf("computing DB hash: %v", err), err)
		return nil, fmt.Errorf("computing DB hash: %w", err)
	}

	s.updateInstanceBackupProgress(backup.ID, domain.InstanceBackupProgress{
		Phase:   "collecting",
		Message: "Collecting device backup files",
	})

	deviceBackupFiles, backupFileCount, knownHostsFile, totalSourceBytes, archiveFileEntries, err := s.collectArchiveSourceFiles(ctx, limits, dbInfo.Size())
	if err != nil {
		cleanupOnError(fmt.Sprintf("collecting backup files: %v", err), err)
		return nil, err
	}
	requiredKeyIDs, err := s.collectRequiredCredentialKeyIDs(ctx)
	if err != nil {
		cleanupOnError(fmt.Sprintf("collecting credential encryption metadata: %v", err), err)
		return nil, err
	}

	// Step 5: Build manifest
	manifestPlan, err := buildInstanceBackupArchiveManifestPlan(instanceBackupArchiveManifestInput{
		dbArtifact:         dbArtifact,
		backupCreatedAt:    backup.CreatedAt,
		dbSHA256:           dbHash,
		backupFileCount:    backupFileCount,
		totalSourceBytes:   totalSourceBytes,
		archiveFileEntries: archiveFileEntries,
		encryptionKey:      s.encryptionKey,
		encryptionKeyring:  s.keyring,
		requiredKeyIDs:     requiredKeyIDs,
		limits:             limits,
	})
	if err != nil {
		errMsg := err.Error()
		if !strings.HasPrefix(errMsg, "marshaling manifest:") {
			errMsg = fmt.Sprintf("checking archive quota: %v", err)
		}
		cleanupOnError(errMsg, err)
		return nil, err
	}
	manifest := manifestPlan.manifest
	manifestJSON := manifestPlan.manifestJSON
	totalArchiveBytes := manifestPlan.totalArchiveBytes

	s.updateInstanceBackupProgress(backup.ID, domain.InstanceBackupProgress{
		Phase:   "archiving",
		Message: "Writing backup archive",
		Total:   totalArchiveBytes,
	})

	// Step 6: Create archive at temp path
	finalPath := filepath.Join(backupSubDir, backup.FileName)
	tempArchivePath := finalPath + ".tmp"

	totalSize, err := s.createArchive(ctx, tempArchivePath, dbArtifact, deviceBackupFiles, knownHostsFile, manifestJSON, &manifest, backup.ID)
	if err != nil {
		cleanupOnError(fmt.Sprintf("creating archive: %v", err), err)
		os.Remove(tempArchivePath)
		return nil, fmt.Errorf("creating archive: %w", err)
	}
	manifest.TotalSizeBytes = totalSize

	// Step 7: Rename temp archive to final path
	if err := os.Rename(tempArchivePath, finalPath); err != nil {
		cleanupOnError(fmt.Sprintf("renaming archive: %v", err), err)
		os.Remove(tempArchivePath)
		return nil, fmt.Errorf("renaming archive: %w", err)
	}
	if err := os.Chmod(finalPath, 0600); err != nil {
		cleanupOnError(fmt.Sprintf("restricting archive permissions: %v", err), err)
		return nil, fmt.Errorf("restricting archive permissions: %w", err)
	}

	s.updateInstanceBackupProgress(backup.ID, domain.InstanceBackupProgress{
		Phase:   "hashing",
		Message: "Hashing backup archive",
	})

	// Step 8: Compute archive SHA-256 and write sidecar
	archiveHash, err := computeFileHashContext(ctx, finalPath)
	if err != nil {
		cleanupOnError(fmt.Sprintf("computing archive hash: %v", err), err)
		return nil, fmt.Errorf("computing archive hash: %w", err)
	}

	sidecarContent := fmt.Sprintf("%s  %s\n", archiveHash, filepath.Base(finalPath))
	sidecarPath := finalPath + ".sha256"
	if err := os.WriteFile(sidecarPath, []byte(sidecarContent), 0600); err != nil {
		cleanupOnError(fmt.Sprintf("writing sidecar: %v", err), err)
		return nil, fmt.Errorf("writing sidecar: %w", err)
	}
	if err := ctx.Err(); err != nil {
		cleanupOnError("backup cancelled", err)
		return nil, err
	}
	if s.instanceBackupCancellationRequested(backup.ID) {
		err := context.Canceled
		cleanupOnError("backup cancelled", err)
		return nil, err
	}

	// Step 9: Get archive file size
	archiveInfo, err := os.Stat(finalPath)
	if err != nil {
		cleanupOnError(fmt.Sprintf("statting archive: %v", err), err)
		return nil, fmt.Errorf("statting archive: %w", err)
	}

	// Step 10: Update DB record to success
	backup.FilePath = finalPath
	backup.SizeBytes = archiveInfo.Size()
	backup.SHA256 = archiveHash
	backup.MigrationVersion = dbArtifact.migrationVersion
	backup.Status = domain.InstanceBackupStatusSuccess
	backup.ErrorMessage = ""

	if err := s.completeInstanceBackupSuccess(backup, totalSize, ownOperation); err != nil {
		if errors.Is(err, context.Canceled) {
			cleanupOnError("backup cancelled", err)
			return nil, err
		}
		return nil, fmt.Errorf("updating backup record: %w", err)
	}

	return backup, nil
}

// completeInstanceBackupSuccess persists successful metadata while respecting cancellation races.
func (s *InstanceBackupService) completeInstanceBackupSuccess(backup *domain.InstanceBackup, totalSize int64, ownOperation bool) error {
	return s.operations.completeSuccess(backup.ID, totalSize, ownOperation, func() error {
		return s.repo.Update(backup)
	})
}

// cleanupFailedInstanceBackup marks a failed or cancelled run and removes its working directory.
func (s *InstanceBackupService) cleanupFailedInstanceBackup(backup *domain.InstanceBackup, backupSubDir string, errMsg string, err error) {
	status := domain.InstanceBackupStatusFailed
	if errors.Is(err, context.Canceled) || s.instanceBackupCancellationRequested(backup.ID) || backupAlreadyCancelled(s.repo, backup.ID) {
		status = domain.InstanceBackupStatusCancelled
		errMsg = "cancelled by user"
	}
	backup.Status = status
	backup.ErrorMessage = errMsg
	if updateErr := s.repo.Update(backup); updateErr != nil {
		log.Printf("Failed to update backup record to %s: %v", status, updateErr)
	}
	os.RemoveAll(backupSubDir)
}

// backupAlreadyCancelled checks persisted state for cancellation that arrived during cleanup.
func backupAlreadyCancelled(repo domain.InstanceBackupRepository, id uuid.UUID) bool {
	backup, err := repo.GetByID(id)
	return err == nil && backup != nil && backup.Status == domain.InstanceBackupStatusCancelled
}

// backupRunContext applies the configured backup duration limit to a run context.
func (s *InstanceBackupService) backupRunContext(ctx context.Context) (context.Context, context.CancelFunc) {
	if ctx == nil {
		ctx = context.Background()
	}
	limits := s.BackupArchiveLimits()
	if limits.MaxDuration > 0 {
		return context.WithTimeout(ctx, limits.MaxDuration)
	}
	return context.WithCancel(ctx)
}

// hasRunningInstanceBackup checks persisted records for an already-running backup.
func (s *InstanceBackupService) hasRunningInstanceBackup() (bool, error) {
	backups, err := s.repo.List()
	if err != nil {
		return false, err
	}
	for i := range backups {
		if backups[i].Status == domain.InstanceBackupStatusRunning {
			return true, nil
		}
	}
	return false, nil
}
