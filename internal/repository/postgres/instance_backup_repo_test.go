package postgres

// This file exercises instance backup repo behavior so refactors preserve the documented contract.

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/lollinoo/theia/internal/domain"
)

func TestInstanceBackupRepo(t *testing.T) {
	db := setupTestDB(t)
	repo := NewInstanceBackupRepo(db)

	t.Run("Create and GetByID roundtrip", func(t *testing.T) {
		backup := &domain.InstanceBackup{
			ID:               uuid.New(),
			FileName:         "theia-backup-20260405-120000-v1.4.0.tar.gz",
			FilePath:         "/data/instance-backups/abc/theia-backup-20260405-120000-v1.4.0.tar.gz",
			SizeBytes:        1048576,
			SHA256:           "abcdef1234567890abcdef1234567890abcdef1234567890abcdef1234567890",
			AppVersion:       "v1.4.0",
			MigrationVersion: 10,
			Status:           domain.InstanceBackupStatusSuccess,
			ErrorMessage:     "",
			CreatedAt:        time.Date(2026, 4, 5, 12, 0, 0, 0, time.UTC),
		}

		if err := repo.Create(backup); err != nil {
			t.Fatalf("Create: %v", err)
		}

		got, err := repo.GetByID(backup.ID)
		if err != nil {
			t.Fatalf("GetByID: %v", err)
		}
		if got == nil {
			t.Fatal("GetByID returned nil")
		}

		if got.ID != backup.ID {
			t.Errorf("ID = %v, want %v", got.ID, backup.ID)
		}
		if got.FileName != backup.FileName {
			t.Errorf("FileName = %q, want %q", got.FileName, backup.FileName)
		}
		if got.FilePath != backup.FilePath {
			t.Errorf("FilePath = %q, want %q", got.FilePath, backup.FilePath)
		}
		if got.SizeBytes != backup.SizeBytes {
			t.Errorf("SizeBytes = %d, want %d", got.SizeBytes, backup.SizeBytes)
		}
		if got.SHA256 != backup.SHA256 {
			t.Errorf("SHA256 = %q, want %q", got.SHA256, backup.SHA256)
		}
		if got.AppVersion != backup.AppVersion {
			t.Errorf("AppVersion = %q, want %q", got.AppVersion, backup.AppVersion)
		}
		if got.MigrationVersion != backup.MigrationVersion {
			t.Errorf("MigrationVersion = %d, want %d", got.MigrationVersion, backup.MigrationVersion)
		}
		if got.Status != backup.Status {
			t.Errorf("Status = %q, want %q", got.Status, backup.Status)
		}
		if got.ErrorMessage != backup.ErrorMessage {
			t.Errorf("ErrorMessage = %q, want %q", got.ErrorMessage, backup.ErrorMessage)
		}
		if !got.CreatedAt.Equal(backup.CreatedAt) {
			t.Errorf("CreatedAt = %v, want %v", got.CreatedAt, backup.CreatedAt)
		}
	})

	t.Run("List returns all ordered by created_at DESC", func(t *testing.T) {
		db2 := setupTestDB(t)
		repo2 := NewInstanceBackupRepo(db2)

		now := time.Now().UTC().Truncate(time.Second)
		for i := 0; i < 3; i++ {
			b := &domain.InstanceBackup{
				FileName:  "backup-" + uuid.New().String()[:8] + ".tar.gz",
				FilePath:  "/data/backups/" + uuid.New().String(),
				Status:    domain.InstanceBackupStatusSuccess,
				CreatedAt: now.Add(time.Duration(i) * time.Minute),
			}
			if err := repo2.Create(b); err != nil {
				t.Fatalf("Create backup %d: %v", i, err)
			}
		}

		list, err := repo2.List()
		if err != nil {
			t.Fatalf("List: %v", err)
		}
		if len(list) != 3 {
			t.Fatalf("expected 3 backups, got %d", len(list))
		}

		// Should be newest first (DESC)
		if !list[0].CreatedAt.After(list[1].CreatedAt) {
			t.Errorf("list[0].CreatedAt (%v) should be after list[1].CreatedAt (%v)", list[0].CreatedAt, list[1].CreatedAt)
		}
		if !list[1].CreatedAt.After(list[2].CreatedAt) {
			t.Errorf("list[1].CreatedAt (%v) should be after list[2].CreatedAt (%v)", list[1].CreatedAt, list[2].CreatedAt)
		}
	})

	t.Run("Update status and error_message", func(t *testing.T) {
		db3 := setupTestDB(t)
		repo3 := NewInstanceBackupRepo(db3)

		backup := &domain.InstanceBackup{
			FileName: "update-test.tar.gz",
			FilePath: "/data/backups/update-test",
			Status:   domain.InstanceBackupStatusRunning,
		}
		if err := repo3.Create(backup); err != nil {
			t.Fatalf("Create: %v", err)
		}

		backup.Status = domain.InstanceBackupStatusFailed
		backup.ErrorMessage = "integrity check failed"
		backup.SizeBytes = 999
		if err := repo3.Update(backup); err != nil {
			t.Fatalf("Update: %v", err)
		}

		got, err := repo3.GetByID(backup.ID)
		if err != nil {
			t.Fatalf("GetByID: %v", err)
		}
		if got.Status != domain.InstanceBackupStatusFailed {
			t.Errorf("Status = %q, want %q", got.Status, domain.InstanceBackupStatusFailed)
		}
		if got.ErrorMessage != "integrity check failed" {
			t.Errorf("ErrorMessage = %q, want %q", got.ErrorMessage, "integrity check failed")
		}
		if got.SizeBytes != 999 {
			t.Errorf("SizeBytes = %d, want 999", got.SizeBytes)
		}
	})

	t.Run("Delete removes backup", func(t *testing.T) {
		db4 := setupTestDB(t)
		repo4 := NewInstanceBackupRepo(db4)

		backup := &domain.InstanceBackup{
			FileName: "delete-test.tar.gz",
			FilePath: "/data/backups/delete-test",
			Status:   domain.InstanceBackupStatusSuccess,
		}
		if err := repo4.Create(backup); err != nil {
			t.Fatalf("Create: %v", err)
		}

		if err := repo4.Delete(backup.ID); err != nil {
			t.Fatalf("Delete: %v", err)
		}

		got, err := repo4.GetByID(backup.ID)
		if err != nil {
			t.Fatalf("GetByID after delete: %v", err)
		}
		if got != nil {
			t.Fatal("expected nil after delete, got non-nil")
		}
	})

	t.Run("GetByID for non-existent ID returns nil nil", func(t *testing.T) {
		db5 := setupTestDB(t)
		repo5 := NewInstanceBackupRepo(db5)

		got, err := repo5.GetByID(uuid.New())
		if err != nil {
			t.Fatalf("GetByID: %v", err)
		}
		if got != nil {
			t.Fatalf("expected nil for non-existent ID, got %+v", got)
		}
	})

	t.Run("Update for non-existent ID returns error", func(t *testing.T) {
		db6 := setupTestDB(t)
		repo6 := NewInstanceBackupRepo(db6)

		backup := &domain.InstanceBackup{
			ID:       uuid.New(),
			FileName: "nonexistent.tar.gz",
			FilePath: "/data/nonexistent",
			Status:   domain.InstanceBackupStatusSuccess,
		}
		err := repo6.Update(backup)
		if err == nil {
			t.Fatal("expected error for updating non-existent backup, got nil")
		}
	})

	t.Run("Delete for non-existent ID returns error", func(t *testing.T) {
		db7 := setupTestDB(t)
		repo7 := NewInstanceBackupRepo(db7)

		err := repo7.Delete(uuid.New())
		if err == nil {
			t.Fatal("expected error for deleting non-existent backup, got nil")
		}
	})

	t.Run("Create with uuid.Nil auto-generates UUID", func(t *testing.T) {
		db8 := setupTestDB(t)
		repo8 := NewInstanceBackupRepo(db8)

		backup := &domain.InstanceBackup{
			ID:       uuid.Nil,
			FileName: "auto-uuid.tar.gz",
			FilePath: "/data/backups/auto-uuid",
			Status:   domain.InstanceBackupStatusRunning,
		}
		if err := repo8.Create(backup); err != nil {
			t.Fatalf("Create: %v", err)
		}

		if backup.ID == uuid.Nil {
			t.Fatal("expected auto-generated UUID, got uuid.Nil")
		}

		got, err := repo8.GetByID(backup.ID)
		if err != nil {
			t.Fatalf("GetByID: %v", err)
		}
		if got == nil {
			t.Fatal("GetByID returned nil for auto-generated UUID")
		}
	})

	t.Run("Create with zero CreatedAt auto-sets to now", func(t *testing.T) {
		db9 := setupTestDB(t)
		repo9 := NewInstanceBackupRepo(db9)

		before := time.Now().UTC().Add(-time.Second)
		backup := &domain.InstanceBackup{
			FileName: "auto-time.tar.gz",
			FilePath: "/data/backups/auto-time",
			Status:   domain.InstanceBackupStatusRunning,
		}
		if err := repo9.Create(backup); err != nil {
			t.Fatalf("Create: %v", err)
		}
		after := time.Now().UTC().Add(time.Second)

		if backup.CreatedAt.IsZero() {
			t.Fatal("expected CreatedAt to be auto-set, got zero time")
		}
		if backup.CreatedAt.Before(before) || backup.CreatedAt.After(after) {
			t.Errorf("CreatedAt = %v, expected between %v and %v", backup.CreatedAt, before, after)
		}
	})
}
