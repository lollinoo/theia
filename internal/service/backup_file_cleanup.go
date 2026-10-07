package service

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/google/uuid"
	"github.com/lollinoo/theia/internal/domain"
)

// CleanupDeletedBackupFiles removes at most 100 captured paths. Failed paths stay
// durable for retries; the configured root and existing path safety checks apply.
func (s *BackupService) CleanupDeletedBackupFiles(ctx context.Context) (int, error) {
	repo, ok := s.fileRepo.(domain.BackupFileDeletionRepository)
	if !ok {
		return 0, nil
	}
	files, err := repo.ListPendingFileDeletions(ctx, 100)
	if err != nil || len(files) == 0 {
		return 0, err
	}
	root, err := validatedBackupRoot(s.backupDir)
	if err != nil {
		return 0, err
	}
	removed := 0
	var failures []error
	for _, file := range files {
		if err := ctx.Err(); err != nil {
			return removed, errors.Join(append(failures, err)...)
		}
		completed, err := s.cleanupDeletedBackupFile(ctx, repo, root, file)
		if err != nil {
			failures = append(failures, fmt.Errorf("cleaning backup file %s: %w", file.ID, err))
		}
		if !completed {
			if deferErr := repo.DeferFileDeletion(ctx, file.ID); deferErr != nil {
				failures = append(failures, deferErr)
			}
			continue
		}
		removed++
	}
	return removed, errors.Join(failures...)
}

func (s *BackupService) cleanupDeletedBackupFile(ctx context.Context, repo domain.BackupFileDeletionRepository, root string, file domain.BackupFile) (bool, error) {
	path, err := validateBackupDeletionPath(root, s.resolveBackupFilePath(&file))
	if err != nil {
		return false, err
	}
	// Production artifacts live under a device UUID directory. Share the executor's
	// lock so cleanup cannot remove a file while this runtime is producing it.
	if deviceID, err := uuid.Parse(filepath.Base(filepath.Dir(path))); err == nil {
		release, err := s.lockBackupDevice(ctx, deviceID)
		if err != nil {
			return false, err
		}
		defer release()
	}
	referenced, err := repo.BackupFilePathsReferenced(ctx, []string{file.FilePath, path})
	if err != nil || referenced {
		return false, err
	}
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return false, err
	}
	if err := repo.CompleteFileDeletion(ctx, file.ID); err != nil {
		return false, err
	}
	return true, nil
}
