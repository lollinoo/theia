package service

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/google/uuid"
	"github.com/lollinoo/theia/internal/domain"
	"github.com/lollinoo/theia/internal/instance"
)

type backupVerificationReceipt struct {
	ID               uuid.UUID `json:"id"`
	FileName         string    `json:"file_name"`
	Size             int64     `json:"size"`
	SHA256           string    `json:"sha256"`
	MigrationVersion int       `json:"migration_version"`
}

func writeBackupVerificationReceipt(backup *domain.InstanceBackup) error {
	data, err := json.Marshal(backupVerificationReceipt{backup.ID, backup.FileName, backup.SizeBytes, backup.SHA256, backup.MigrationVersion})
	if err != nil {
		return err
	}
	return instance.WritePrivateFile(backup.FilePath+".verified.json", data)
}

// reconcileVerifiedBackup trusts only a durable receipt made after decryption and
// actual database restore verification, and rechecks its exact local archive bytes.
func (s *InstanceBackupService) reconcileVerifiedBackup(ctx context.Context, backup *domain.InstanceBackup) error {
	if filepath.Base(backup.FileName) != backup.FileName || backup.FileName == "" {
		return fmt.Errorf("invalid backup filename")
	}
	path := filepath.Join(s.backupDir, backup.ID.String(), backup.FileName)
	data, err := os.ReadFile(path + ".verified.json")
	if err != nil {
		return err
	}
	var receipt backupVerificationReceipt
	if err := json.Unmarshal(data, &receipt); err != nil {
		return err
	}
	if receipt.ID != backup.ID || receipt.FileName != backup.FileName {
		return fmt.Errorf("verification receipt belongs to another backup")
	}
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	hash, err := computeFileHashContext(ctx, path)
	if err != nil {
		return err
	}
	if info.Size() != receipt.Size || hash != receipt.SHA256 {
		return fmt.Errorf("backup no longer matches verified bytes")
	}
	backup.FilePath, backup.SizeBytes, backup.SHA256, backup.MigrationVersion = path, receipt.Size, receipt.SHA256, receipt.MigrationVersion
	backup.Status, backup.ErrorMessage = domain.InstanceBackupStatusSuccess, ""
	if s.externalDestination != nil {
		backup.Status, backup.ErrorMessage = domain.InstanceBackupStatusPendingUpload, "Verified local backup retained; external upload pending"
	}
	return nil
}

func (s *InstanceBackupService) uploadVerifiedBackup(ctx context.Context, backup *domain.InstanceBackup) error {
	if err := s.reconcileVerifiedBackup(ctx, backup); err != nil {
		return err
	}
	if s.externalDestination != nil {
		if err := s.externalDestination.PutVerified(ctx, backup.ID.String()+"/"+backup.FileName, backup.FilePath, backup.SHA256); err != nil {
			return err
		}
	}
	backup.Status, backup.ErrorMessage = domain.InstanceBackupStatusSuccess, ""
	return nil
}

// RetryPendingUploads bounds retry work and keeps incomplete uploads outside
// retention. The ephemeral identity is unnecessary for identical verified bytes.
func (s *InstanceBackupService) RetryPendingUploads(ctx context.Context) error {
	s.createMu.Lock()
	defer s.createMu.Unlock()
	if s.hasActiveInstanceBackupOperation() {
		return nil
	}
	backups, err := s.repo.List()
	if err != nil {
		return err
	}
	count := 0
	for i := range backups {
		backup := &backups[i]
		if backup.Status != domain.InstanceBackupStatusPendingUpload {
			continue
		}
		if count == 3 {
			break
		}
		count++
		if err := s.uploadVerifiedBackup(ctx, backup); err != nil {
			backup.Status = domain.InstanceBackupStatusPendingUpload
			backup.ErrorMessage = err.Error()
		}
		if err := s.repo.Update(backup); err != nil {
			return err
		}
	}
	return ctx.Err()
}
