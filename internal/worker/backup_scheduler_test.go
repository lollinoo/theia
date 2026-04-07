package worker

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/lollinoo/theia/internal/domain"
	"github.com/lollinoo/theia/internal/service"
)

// --- Mock types for BackupScheduler runRetention tests ---

type mockInstanceBackupRepo struct {
	backups            []domain.InstanceBackup
	deleteRepoCalls    atomic.Int64
	deleteFailedCalled atomic.Bool
	deleteFailedCount  int
}

func (m *mockInstanceBackupRepo) ListSuccessfulOldest() ([]domain.InstanceBackup, error) {
	return m.backups, nil
}

func (m *mockInstanceBackupRepo) DeleteFailedOlderThan(cutoff time.Time) (int, error) {
	m.deleteFailedCalled.Store(true)
	return m.deleteFailedCount, nil
}

func (m *mockInstanceBackupRepo) Create(backup *domain.InstanceBackup) error          { return nil }
func (m *mockInstanceBackupRepo) GetByID(id uuid.UUID) (*domain.InstanceBackup, error) { return nil, nil }
func (m *mockInstanceBackupRepo) List() ([]domain.InstanceBackup, error)               { return nil, nil }
func (m *mockInstanceBackupRepo) Update(backup *domain.InstanceBackup) error           { return nil }
func (m *mockInstanceBackupRepo) Delete(id uuid.UUID) error {
	m.deleteRepoCalls.Add(1)
	return nil
}

type mockInstanceBackupService struct {
	deleteCalls atomic.Int64
}

func (m *mockInstanceBackupService) Delete(ctx context.Context, id uuid.UUID) error {
	m.deleteCalls.Add(1)
	return nil
}

// newTestBackupService creates a real InstanceBackupService wired to the given repo.
// Passing nil for db is safe because Delete only calls repo.GetByID and repo.Delete,
// neither of which touches the database connection.
func newTestBackupService(repo domain.InstanceBackupRepository) *service.InstanceBackupService {
	return service.NewInstanceBackupService(nil, repo, nil, "", "", "", "", nil)
}

func TestBackupSchedulerRunRetention_Normal(t *testing.T) {
	// 3 backups with retention count of 5 — no deletions needed, tests full path
	backups := make([]domain.InstanceBackup, 3)
	for i := range backups {
		backups[i] = domain.InstanceBackup{
			ID:        uuid.New(),
			Status:    domain.InstanceBackupStatusSuccess,
			CreatedAt: time.Now().Add(-time.Duration(3-i) * time.Hour),
		}
	}

	backupRepo := &mockInstanceBackupRepo{backups: backups}
	settingsRepo := newMockWorkerSettingsRepo()
	settingsRepo.Set(domain.SettingInstanceBackupRetentionCount, "5")

	scheduler := &BackupScheduler{
		backupRepo:   backupRepo,
		settingsRepo: settingsRepo,
	}

	scheduler.runRetention(context.Background())

	// Failed cleanup must always run even when no deletions are needed
	if !backupRepo.deleteFailedCalled.Load() {
		t.Error("DeleteFailedOlderThan was not called — failed cleanup must always run")
	}
}

func TestBackupSchedulerRunRetention_Timeout(t *testing.T) {
	// Create backups beyond retention
	backups := make([]domain.InstanceBackup, 20)
	for i := range backups {
		backups[i] = domain.InstanceBackup{
			ID:        uuid.New(),
			Status:    domain.InstanceBackupStatusSuccess,
			CreatedAt: time.Now().Add(-time.Duration(20-i) * time.Hour),
		}
	}

	backupRepo := &mockInstanceBackupRepo{backups: backups}
	settingsRepo := newMockWorkerSettingsRepo()
	settingsRepo.Set(domain.SettingInstanceBackupRetentionCount, "5")

	scheduler := &BackupScheduler{
		backupRepo:   backupRepo,
		settingsRepo: settingsRepo,
	}

	// Use already-cancelled context to simulate timeout
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	scheduler.runRetention(ctx)

	// Failed cleanup MUST still run even when timeout fires
	if !backupRepo.deleteFailedCalled.Load() {
		t.Error("DeleteFailedOlderThan was not called — failed cleanup must run even after timeout")
	}
}

// TestBackupSchedulerRunRetention_ExcessDeletion verifies that when the number of
// successful backups exceeds the retention count, the oldest excess backups are deleted.
// With 8 backups and retention=3, exactly 5 deletions must be made (oldest 5).
func TestBackupSchedulerRunRetention_ExcessDeletion(t *testing.T) {
	const totalBackups = 8
	const retentionCount = 3
	const wantDeleted = totalBackups - retentionCount // 5

	backups := make([]domain.InstanceBackup, totalBackups)
	for i := range backups {
		backups[i] = domain.InstanceBackup{
			ID:        uuid.New(),
			Status:    domain.InstanceBackupStatusSuccess,
			CreatedAt: time.Now().Add(-time.Duration(totalBackups-i) * time.Hour),
		}
	}

	backupRepo := &mockInstanceBackupRepo{backups: backups}
	settingsRepo := newMockWorkerSettingsRepo()
	settingsRepo.Set(domain.SettingInstanceBackupRetentionCount, "3")

	// Wire a real InstanceBackupService using the mock repo so Delete calls are tracked.
	// InstanceBackupService.Delete calls repo.GetByID (returns nil — no FilePath, so no
	// file removal attempted) then repo.Delete. No sql.DB access occurs in this path.
	svc := newTestBackupService(backupRepo)

	scheduler := &BackupScheduler{
		backupService: svc,
		backupRepo:    backupRepo,
		settingsRepo:  settingsRepo,
	}

	scheduler.runRetention(context.Background())

	got := backupRepo.deleteRepoCalls.Load()
	if got != int64(wantDeleted) {
		t.Errorf("retention deleted %d backups, want %d (8 backups with retention=3)", got, wantDeleted)
	}

	// Failed record cleanup must also have run
	if !backupRepo.deleteFailedCalled.Load() {
		t.Error("DeleteFailedOlderThan was not called — failed cleanup must always run")
	}
}

func TestGetBackupInterval(t *testing.T) {
	t.Run("returns 0 when setting is 0 (disabled)", func(t *testing.T) {
		repo := newMockWorkerSettingsRepo()
		repo.Set(domain.SettingInstanceBackupIntervalHours, "0")

		interval := GetBackupInterval(repo)
		if interval != 0 {
			t.Errorf("GetBackupInterval = %v, want 0 (disabled)", interval)
		}
	})

	t.Run("returns correct hours-to-duration for 24", func(t *testing.T) {
		repo := newMockWorkerSettingsRepo()
		repo.Set(domain.SettingInstanceBackupIntervalHours, "24")

		interval := GetBackupInterval(repo)
		if interval != 24*time.Hour {
			t.Errorf("GetBackupInterval = %v, want %v", interval, 24*time.Hour)
		}
	})

	t.Run("returns correct hours-to-duration for 168", func(t *testing.T) {
		repo := newMockWorkerSettingsRepo()
		repo.Set(domain.SettingInstanceBackupIntervalHours, "168")

		interval := GetBackupInterval(repo)
		if interval != 168*time.Hour {
			t.Errorf("GetBackupInterval = %v, want %v", interval, 168*time.Hour)
		}
	})

	t.Run("returns 0 for invalid value (disabled fallback)", func(t *testing.T) {
		repo := newMockWorkerSettingsRepo()
		repo.Set(domain.SettingInstanceBackupIntervalHours, "invalid")

		interval := GetBackupInterval(repo)
		if interval != 0 {
			t.Errorf("GetBackupInterval = %v, want 0 (disabled fallback)", interval)
		}
	})

	t.Run("returns 0 for negative value (disabled fallback)", func(t *testing.T) {
		repo := newMockWorkerSettingsRepo()
		repo.Set(domain.SettingInstanceBackupIntervalHours, "-5")

		interval := GetBackupInterval(repo)
		if interval != 0 {
			t.Errorf("GetBackupInterval = %v, want 0 (disabled fallback)", interval)
		}
	})

	t.Run("returns 0 for missing setting", func(t *testing.T) {
		repo := &mockWorkerSettingsRepo{settings: map[string]string{}}

		interval := GetBackupInterval(repo)
		if interval != 0 {
			t.Errorf("GetBackupInterval = %v, want 0 (disabled fallback)", interval)
		}
	})
}

func TestGetRetentionCount(t *testing.T) {
	t.Run("returns correct integer for 5", func(t *testing.T) {
		repo := newMockWorkerSettingsRepo()
		repo.Set(domain.SettingInstanceBackupRetentionCount, "5")

		count := GetRetentionCount(repo)
		if count != 5 {
			t.Errorf("GetRetentionCount = %d, want 5", count)
		}
	})

	t.Run("returns correct integer for 1", func(t *testing.T) {
		repo := newMockWorkerSettingsRepo()
		repo.Set(domain.SettingInstanceBackupRetentionCount, "1")

		count := GetRetentionCount(repo)
		if count != 1 {
			t.Errorf("GetRetentionCount = %d, want 1", count)
		}
	})

	t.Run("returns minimum 1 for 0", func(t *testing.T) {
		repo := newMockWorkerSettingsRepo()
		repo.Set(domain.SettingInstanceBackupRetentionCount, "0")

		count := GetRetentionCount(repo)
		if count != 1 {
			t.Errorf("GetRetentionCount = %d, want 1 (minimum)", count)
		}
	})

	t.Run("returns default 5 for invalid value", func(t *testing.T) {
		repo := newMockWorkerSettingsRepo()
		repo.Set(domain.SettingInstanceBackupRetentionCount, "invalid")

		count := GetRetentionCount(repo)
		if count != 5 {
			t.Errorf("GetRetentionCount = %d, want 5 (default)", count)
		}
	})

	t.Run("returns default 5 for missing setting", func(t *testing.T) {
		repo := &mockWorkerSettingsRepo{settings: map[string]string{}}

		count := GetRetentionCount(repo)
		if count != 5 {
			t.Errorf("GetRetentionCount = %d, want 5 (default)", count)
		}
	})

	t.Run("returns default 5 for negative value", func(t *testing.T) {
		repo := newMockWorkerSettingsRepo()
		repo.Set(domain.SettingInstanceBackupRetentionCount, "-3")

		count := GetRetentionCount(repo)
		if count != 5 {
			t.Errorf("GetRetentionCount = %d, want 5 (default for negative)", count)
		}
	})
}
