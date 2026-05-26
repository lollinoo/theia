package worker

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/lollinoo/theia/internal/domain"
	"github.com/lollinoo/theia/internal/service"
)

// --- Mock types for runRetention tests ---

type mockBackupJobRepo struct {
	deviceIDs           []uuid.UUID
	jobsByDevice        map[uuid.UUID][]domain.BackupJob
	deleteFailedCalled  atomic.Bool
	deleteFailedCount   int
	listSuccessfulCalls atomic.Int64
}

func (m *mockBackupJobRepo) ListAllDeviceIDs() ([]uuid.UUID, error) {
	return m.deviceIDs, nil
}

func (m *mockBackupJobRepo) ListSuccessfulByDeviceOldest(deviceID uuid.UUID) ([]domain.BackupJob, error) {
	m.listSuccessfulCalls.Add(1)
	if jobs, ok := m.jobsByDevice[deviceID]; ok {
		return jobs, nil
	}
	return nil, nil
}

func (m *mockBackupJobRepo) DeleteFailedOlderThan(cutoff time.Time) (int, error) {
	m.deleteFailedCalled.Store(true)
	return m.deleteFailedCount, nil
}

func (m *mockBackupJobRepo) Create(job *domain.BackupJob) error              { return nil }
func (m *mockBackupJobRepo) GetByID(id uuid.UUID) (*domain.BackupJob, error) { return nil, nil }
func (m *mockBackupJobRepo) GetByDeviceID(deviceID uuid.UUID) ([]domain.BackupJob, error) {
	return nil, nil
}
func (m *mockBackupJobRepo) GetLatestByDeviceID(deviceID uuid.UUID) (*domain.BackupJob, error) {
	return nil, nil
}
func (m *mockBackupJobRepo) Update(job *domain.BackupJob) error        { return nil }
func (m *mockBackupJobRepo) Delete(id uuid.UUID) error                 { return nil }
func (m *mockBackupJobRepo) DeleteByDeviceID(deviceID uuid.UUID) error { return nil }

type mockRetentionBackupService struct {
	deleteCalls atomic.Int64
}

func (m *mockRetentionBackupService) DeleteBackupJob(ctx context.Context, id uuid.UUID) error {
	m.deleteCalls.Add(1)
	return nil
}

func (m *mockRetentionBackupService) GetLatestBulkBackupRun(ctx context.Context) (*domain.BulkBackupRun, error) {
	return nil, nil
}

func (m *mockRetentionBackupService) StartBulkBackupRun(ctx context.Context, requestedDeviceIDs []uuid.UUID, createdBy string) (*domain.BulkBackupRun, error) {
	return nil, nil
}

type mockScheduledBackupService struct {
	startCalls         atomic.Int64
	deleteCalls        atomic.Int64
	requestedDeviceIDs []uuid.UUID
	createdBy          string
	startErr           error
	latestRun          *domain.BulkBackupRun
	latestErr          error
}

func (m *mockScheduledBackupService) StartBulkBackupRun(ctx context.Context, requestedDeviceIDs []uuid.UUID, createdBy string) (*domain.BulkBackupRun, error) {
	m.startCalls.Add(1)
	m.requestedDeviceIDs = append([]uuid.UUID(nil), requestedDeviceIDs...)
	m.createdBy = createdBy
	if m.startErr != nil {
		return &domain.BulkBackupRun{ID: uuid.New(), Status: domain.BulkBackupRunStatusRunning}, m.startErr
	}
	return &domain.BulkBackupRun{
		ID:         uuid.New(),
		Status:     domain.BulkBackupRunStatusRunning,
		TotalCount: 250,
	}, nil
}

func (m *mockScheduledBackupService) GetLatestBulkBackupRun(ctx context.Context) (*domain.BulkBackupRun, error) {
	return m.latestRun, m.latestErr
}

func (m *mockScheduledBackupService) DeleteBackupJob(ctx context.Context, id uuid.UUID) error {
	m.deleteCalls.Add(1)
	return nil
}

func TestRunRetention_BatchesDevices(t *testing.T) {
	// Create 250 device IDs to verify batching processes all of them
	deviceIDs := make([]uuid.UUID, 250)
	jobsByDevice := make(map[uuid.UUID][]domain.BackupJob)
	for i := range deviceIDs {
		deviceIDs[i] = uuid.New()
		// Each device has 1 job (at or below retention count) — no deletions needed
		jobsByDevice[deviceIDs[i]] = []domain.BackupJob{
			{ID: uuid.New(), DeviceID: deviceIDs[i], Status: domain.BackupStatusSuccess},
		}
	}

	jobRepo := &mockBackupJobRepo{
		deviceIDs:    deviceIDs,
		jobsByDevice: jobsByDevice,
	}
	settingsRepo := newMockWorkerSettingsRepo()
	settingsRepo.Set(domain.SettingDeviceBackupRetentionCount, "5")

	svc := &mockRetentionBackupService{}
	scheduler := &DeviceBackupScheduler{
		backupService: svc,
		jobRepo:       jobRepo,
		settingsRepo:  settingsRepo,
	}

	scheduler.runRetention(context.Background())

	// Verify all 250 devices were processed
	if got := jobRepo.listSuccessfulCalls.Load(); got != 250 {
		t.Errorf("ListSuccessfulByDeviceOldest called %d times, want 250 (all devices processed)", got)
	}

	// Verify failed cleanup ran
	if !jobRepo.deleteFailedCalled.Load() {
		t.Error("DeleteFailedOlderThan was not called — failed cleanup must always run")
	}
}

func TestRunScheduledBulkBackupStartsPersistentRun(t *testing.T) {
	svc := &mockScheduledBackupService{}
	scheduler := &DeviceBackupScheduler{backupService: svc}

	scheduler.runScheduledBulkBackup(context.Background())

	if got := svc.startCalls.Load(); got != 1 {
		t.Fatalf("StartBulkBackupRun calls = %d, want 1", got)
	}
	if svc.requestedDeviceIDs != nil {
		t.Fatalf("requestedDeviceIDs = %#v, want nil for all devices", svc.requestedDeviceIDs)
	}
	if svc.createdBy != "scheduler" {
		t.Fatalf("createdBy = %q, want scheduler", svc.createdBy)
	}
}

func TestRunScheduledBulkBackupIgnoresExistingActivePersistentRun(t *testing.T) {
	svc := &mockScheduledBackupService{startErr: service.ErrBulkBackupRunAlreadyActive}
	scheduler := &DeviceBackupScheduler{backupService: svc}

	scheduler.runScheduledBulkBackup(context.Background())

	if got := svc.startCalls.Load(); got != 1 {
		t.Fatalf("StartBulkBackupRun calls = %d, want 1", got)
	}
}

func TestRunScheduledBulkBackupHandlesGenericStartError(t *testing.T) {
	svc := &mockScheduledBackupService{startErr: errors.New("bulk backup run repository is not configured")}
	scheduler := &DeviceBackupScheduler{backupService: svc}

	scheduler.runScheduledBulkBackup(context.Background())

	if got := svc.startCalls.Load(); got != 1 {
		t.Fatalf("StartBulkBackupRun calls = %d, want 1", got)
	}
}

func TestRunRetention_TimeoutStopsProcessing(t *testing.T) {
	// Create 500 device IDs
	deviceIDs := make([]uuid.UUID, 500)
	for i := range deviceIDs {
		deviceIDs[i] = uuid.New()
	}

	jobRepo := &mockBackupJobRepo{
		deviceIDs:    deviceIDs,
		jobsByDevice: make(map[uuid.UUID][]domain.BackupJob),
	}
	settingsRepo := newMockWorkerSettingsRepo()
	settingsRepo.Set(domain.SettingDeviceBackupRetentionCount, "5")

	scheduler := &DeviceBackupScheduler{
		jobRepo:      jobRepo,
		settingsRepo: settingsRepo,
	}

	// Use an already-cancelled context to simulate timeout
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	scheduler.runRetention(ctx)

	// With a cancelled context, the first batch check should detect timeout
	// and not all 500 devices should be processed
	calls := jobRepo.listSuccessfulCalls.Load()
	if calls >= 500 {
		t.Errorf("ListSuccessfulByDeviceOldest called %d times — timeout should have stopped processing before all 500", calls)
	}
}

func TestRunRetention_FailedCleanupRunsAfterTimeout(t *testing.T) {
	// Create 500 device IDs
	deviceIDs := make([]uuid.UUID, 500)
	for i := range deviceIDs {
		deviceIDs[i] = uuid.New()
	}

	jobRepo := &mockBackupJobRepo{
		deviceIDs:    deviceIDs,
		jobsByDevice: make(map[uuid.UUID][]domain.BackupJob),
	}
	settingsRepo := newMockWorkerSettingsRepo()
	settingsRepo.Set(domain.SettingDeviceBackupRetentionCount, "5")

	scheduler := &DeviceBackupScheduler{
		jobRepo:      jobRepo,
		settingsRepo: settingsRepo,
	}

	// Use an already-cancelled context
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	scheduler.runRetention(ctx)

	// Failed cleanup MUST run even when timeout fires
	if !jobRepo.deleteFailedCalled.Load() {
		t.Error("DeleteFailedOlderThan was not called — failed cleanup must run even after timeout")
	}
}

func TestGetDeviceBackupInterval(t *testing.T) {
	t.Run("returns 0 when setting is 0 (disabled)", func(t *testing.T) {
		repo := newMockWorkerSettingsRepo()
		repo.Set(domain.SettingDeviceBackupIntervalHours, "0")

		interval := GetDeviceBackupInterval(repo)
		if interval != 0 {
			t.Errorf("GetDeviceBackupInterval = %v, want 0 (disabled)", interval)
		}
	})

	t.Run("returns correct hours-to-duration for 6", func(t *testing.T) {
		repo := newMockWorkerSettingsRepo()
		repo.Set(domain.SettingDeviceBackupIntervalHours, "6")

		interval := GetDeviceBackupInterval(repo)
		if interval != 6*time.Hour {
			t.Errorf("GetDeviceBackupInterval = %v, want %v", interval, 6*time.Hour)
		}
	})

	t.Run("returns correct hours-to-duration for 12", func(t *testing.T) {
		repo := newMockWorkerSettingsRepo()
		repo.Set(domain.SettingDeviceBackupIntervalHours, "12")

		interval := GetDeviceBackupInterval(repo)
		if interval != 12*time.Hour {
			t.Errorf("GetDeviceBackupInterval = %v, want %v", interval, 12*time.Hour)
		}
	})

	t.Run("returns correct hours-to-duration for 24", func(t *testing.T) {
		repo := newMockWorkerSettingsRepo()
		repo.Set(domain.SettingDeviceBackupIntervalHours, "24")

		interval := GetDeviceBackupInterval(repo)
		if interval != 24*time.Hour {
			t.Errorf("GetDeviceBackupInterval = %v, want %v", interval, 24*time.Hour)
		}
	})

	t.Run("returns correct hours-to-duration for 48", func(t *testing.T) {
		repo := newMockWorkerSettingsRepo()
		repo.Set(domain.SettingDeviceBackupIntervalHours, "48")

		interval := GetDeviceBackupInterval(repo)
		if interval != 48*time.Hour {
			t.Errorf("GetDeviceBackupInterval = %v, want %v", interval, 48*time.Hour)
		}
	})

	t.Run("returns correct hours-to-duration for 168", func(t *testing.T) {
		repo := newMockWorkerSettingsRepo()
		repo.Set(domain.SettingDeviceBackupIntervalHours, "168")

		interval := GetDeviceBackupInterval(repo)
		if interval != 168*time.Hour {
			t.Errorf("GetDeviceBackupInterval = %v, want %v", interval, 168*time.Hour)
		}
	})

	t.Run("returns 0 for invalid value (disabled fallback)", func(t *testing.T) {
		repo := newMockWorkerSettingsRepo()
		repo.Set(domain.SettingDeviceBackupIntervalHours, "invalid")

		interval := GetDeviceBackupInterval(repo)
		if interval != 0 {
			t.Errorf("GetDeviceBackupInterval = %v, want 0 (disabled fallback)", interval)
		}
	})

	t.Run("returns 0 for negative value (disabled fallback)", func(t *testing.T) {
		repo := newMockWorkerSettingsRepo()
		repo.Set(domain.SettingDeviceBackupIntervalHours, "-5")

		interval := GetDeviceBackupInterval(repo)
		if interval != 0 {
			t.Errorf("GetDeviceBackupInterval = %v, want 0 (disabled fallback)", interval)
		}
	})

	t.Run("returns 0 for missing setting", func(t *testing.T) {
		repo := &mockWorkerSettingsRepo{settings: map[string]string{}}

		interval := GetDeviceBackupInterval(repo)
		if interval != 0 {
			t.Errorf("GetDeviceBackupInterval = %v, want 0 (disabled fallback)", interval)
		}
	})
}

func TestGetDeviceBackupRetentionCount(t *testing.T) {
	t.Run("returns correct integer for 5", func(t *testing.T) {
		repo := newMockWorkerSettingsRepo()
		repo.Set(domain.SettingDeviceBackupRetentionCount, "5")

		count := GetDeviceBackupRetentionCount(repo)
		if count != 5 {
			t.Errorf("GetDeviceBackupRetentionCount = %d, want 5", count)
		}
	})

	t.Run("returns correct integer for 10", func(t *testing.T) {
		repo := newMockWorkerSettingsRepo()
		repo.Set(domain.SettingDeviceBackupRetentionCount, "10")

		count := GetDeviceBackupRetentionCount(repo)
		if count != 10 {
			t.Errorf("GetDeviceBackupRetentionCount = %d, want 10", count)
		}
	})

	t.Run("returns correct integer for 1", func(t *testing.T) {
		repo := newMockWorkerSettingsRepo()
		repo.Set(domain.SettingDeviceBackupRetentionCount, "1")

		count := GetDeviceBackupRetentionCount(repo)
		if count != 1 {
			t.Errorf("GetDeviceBackupRetentionCount = %d, want 1", count)
		}
	})

	t.Run("returns minimum 1 for 0", func(t *testing.T) {
		repo := newMockWorkerSettingsRepo()
		repo.Set(domain.SettingDeviceBackupRetentionCount, "0")

		count := GetDeviceBackupRetentionCount(repo)
		if count != 1 {
			t.Errorf("GetDeviceBackupRetentionCount = %d, want 1 (minimum)", count)
		}
	})

	t.Run("returns default 5 for invalid value", func(t *testing.T) {
		repo := newMockWorkerSettingsRepo()
		repo.Set(domain.SettingDeviceBackupRetentionCount, "invalid")

		count := GetDeviceBackupRetentionCount(repo)
		if count != 5 {
			t.Errorf("GetDeviceBackupRetentionCount = %d, want 5 (default)", count)
		}
	})

	t.Run("returns default 5 for missing setting", func(t *testing.T) {
		repo := &mockWorkerSettingsRepo{settings: map[string]string{}}

		count := GetDeviceBackupRetentionCount(repo)
		if count != 5 {
			t.Errorf("GetDeviceBackupRetentionCount = %d, want 5 (default)", count)
		}
	})

	t.Run("returns default 5 for negative value", func(t *testing.T) {
		repo := newMockWorkerSettingsRepo()
		repo.Set(domain.SettingDeviceBackupRetentionCount, "-3")

		count := GetDeviceBackupRetentionCount(repo)
		if count != 5 {
			t.Errorf("GetDeviceBackupRetentionCount = %d, want 5 (default for negative)", count)
		}
	})
}

// --- Stubs for checkAndRunBulkBackup tests ---

// checkBulkJobRepo is a controllable BackupJobRepository for checkAndRunBulkBackup tests.
// It lets tests specify device IDs and per-device latest jobs independently.
type checkBulkJobRepo struct {
	deviceIDs      []uuid.UUID
	latestByDevice map[uuid.UUID]*domain.BackupJob
}

func (r *checkBulkJobRepo) ListAllDeviceIDs() ([]uuid.UUID, error) {
	return r.deviceIDs, nil
}

func (r *checkBulkJobRepo) GetLatestByDeviceID(deviceID uuid.UUID) (*domain.BackupJob, error) {
	j, ok := r.latestByDevice[deviceID]
	if !ok {
		return nil, nil
	}
	return j, nil
}

func (r *checkBulkJobRepo) ListSuccessfulByDeviceOldest(deviceID uuid.UUID) ([]domain.BackupJob, error) {
	return nil, nil
}
func (r *checkBulkJobRepo) DeleteFailedOlderThan(_ time.Time) (int, error)  { return 0, nil }
func (r *checkBulkJobRepo) Create(job *domain.BackupJob) error              { return nil }
func (r *checkBulkJobRepo) GetByID(id uuid.UUID) (*domain.BackupJob, error) { return nil, nil }
func (r *checkBulkJobRepo) GetByDeviceID(deviceID uuid.UUID) ([]domain.BackupJob, error) {
	return nil, nil
}
func (r *checkBulkJobRepo) Update(job *domain.BackupJob) error        { return nil }
func (r *checkBulkJobRepo) Delete(id uuid.UUID) error                 { return nil }
func (r *checkBulkJobRepo) DeleteByDeviceID(deviceID uuid.UUID) error { return nil }

// TestCheckAndRunBulkBackup_NoBackupJobsExist verifies that the first scheduled backup
// is triggered immediately when no backup jobs exist at all (no prior runs).
func TestCheckAndRunBulkBackup_NoBackupJobsExist(t *testing.T) {
	jobRepo := &checkBulkJobRepo{
		deviceIDs:      []uuid.UUID{}, // no device IDs — no prior backup jobs
		latestByDevice: map[uuid.UUID]*domain.BackupJob{},
	}

	svc := &mockScheduledBackupService{}
	settingsRepo := newMockWorkerSettingsRepo()
	settingsRepo.Set(domain.SettingDeviceBackupIntervalHours, "24")

	scheduler := &DeviceBackupScheduler{
		backupService: svc,
		jobRepo:       jobRepo,
		settingsRepo:  settingsRepo,
	}

	interval := 24 * time.Hour
	scheduler.checkAndRunBulkBackup(context.Background(), interval)
	if got := svc.startCalls.Load(); got != 1 {
		t.Fatalf("StartBulkBackupRun calls = %d, want 1", got)
	}
}

// TestCheckAndRunBulkBackup_IntervalElapsed verifies that a bulk backup is triggered
// when the most recent job is older than the configured interval.
func TestCheckAndRunBulkBackup_IntervalElapsed(t *testing.T) {
	deviceID := uuid.New()
	oldJob := &domain.BackupJob{
		ID:        uuid.New(),
		DeviceID:  deviceID,
		Status:    domain.BackupStatusSuccess,
		CreatedAt: time.Now().Add(-25 * time.Hour), // 25h ago, interval is 24h
	}

	jobRepo := &checkBulkJobRepo{
		deviceIDs:      []uuid.UUID{deviceID},
		latestByDevice: map[uuid.UUID]*domain.BackupJob{deviceID: oldJob},
	}

	svc := &mockScheduledBackupService{}
	settingsRepo := newMockWorkerSettingsRepo()

	scheduler := &DeviceBackupScheduler{
		backupService: svc,
		jobRepo:       jobRepo,
		settingsRepo:  settingsRepo,
	}

	interval := 24 * time.Hour
	scheduler.checkAndRunBulkBackup(context.Background(), interval)
	if got := svc.startCalls.Load(); got != 1 {
		t.Fatalf("StartBulkBackupRun calls = %d, want 1", got)
	}
}

func TestCheckAndRunBulkBackup_NoSuccessfulJobsStartsPersistentRun(t *testing.T) {
	deviceID := uuid.New()
	jobRepo := &checkBulkJobRepo{
		deviceIDs:      []uuid.UUID{deviceID},
		latestByDevice: map[uuid.UUID]*domain.BackupJob{},
	}

	svc := &mockScheduledBackupService{}
	scheduler := &DeviceBackupScheduler{
		backupService: svc,
		jobRepo:       jobRepo,
		settingsRepo:  newMockWorkerSettingsRepo(),
	}

	scheduler.checkAndRunBulkBackup(context.Background(), 24*time.Hour)
	if got := svc.startCalls.Load(); got != 1 {
		t.Fatalf("StartBulkBackupRun calls = %d, want 1", got)
	}
}

func TestCheckAndRunBulkBackup_ActivePersistentRunSkipsStart(t *testing.T) {
	statuses := []domain.BulkBackupRunStatus{
		domain.BulkBackupRunStatusRunning,
		domain.BulkBackupRunStatusPausing,
		domain.BulkBackupRunStatusPaused,
		domain.BulkBackupRunStatusCancelling,
	}
	for _, status := range statuses {
		t.Run(string(status), func(t *testing.T) {
			svc := &mockScheduledBackupService{
				latestRun: &domain.BulkBackupRun{
					ID:        uuid.New(),
					Status:    status,
					CreatedAt: time.Now().Add(-25 * time.Hour),
				},
			}
			scheduler := &DeviceBackupScheduler{
				backupService: svc,
				jobRepo:       &checkBulkJobRepo{},
				settingsRepo:  newMockWorkerSettingsRepo(),
			}

			scheduler.checkAndRunBulkBackup(context.Background(), 24*time.Hour)
			if got := svc.startCalls.Load(); got != 0 {
				t.Fatalf("StartBulkBackupRun calls = %d, want 0", got)
			}
		})
	}
}

func TestCheckAndRunBulkBackup_RecentPersistentRunSkipsStart(t *testing.T) {
	completedAt := time.Now().Add(-1 * time.Hour)
	svc := &mockScheduledBackupService{
		latestRun: &domain.BulkBackupRun{
			ID:          uuid.New(),
			Status:      domain.BulkBackupRunStatusPartial,
			CreatedAt:   time.Now().Add(-2 * time.Hour),
			CompletedAt: &completedAt,
		},
	}
	scheduler := &DeviceBackupScheduler{
		backupService: svc,
		jobRepo:       &checkBulkJobRepo{},
		settingsRepo:  newMockWorkerSettingsRepo(),
	}

	scheduler.checkAndRunBulkBackup(context.Background(), 24*time.Hour)
	if got := svc.startCalls.Load(); got != 0 {
		t.Fatalf("StartBulkBackupRun calls = %d, want 0", got)
	}
}

func TestCheckAndRunBulkBackup_OldPersistentRunStartsNewRun(t *testing.T) {
	completedAt := time.Now().Add(-25 * time.Hour)
	svc := &mockScheduledBackupService{
		latestRun: &domain.BulkBackupRun{
			ID:          uuid.New(),
			Status:      domain.BulkBackupRunStatusPartial,
			CreatedAt:   time.Now().Add(-26 * time.Hour),
			CompletedAt: &completedAt,
		},
	}
	scheduler := &DeviceBackupScheduler{
		backupService: svc,
		jobRepo:       &checkBulkJobRepo{},
		settingsRepo:  newMockWorkerSettingsRepo(),
	}

	scheduler.checkAndRunBulkBackup(context.Background(), 24*time.Hour)
	if got := svc.startCalls.Load(); got != 1 {
		t.Fatalf("StartBulkBackupRun calls = %d, want 1", got)
	}
}

// TestCheckAndRunBulkBackup_IntervalNotElapsed verifies that no backup is triggered
// when the most recent job is within the configured interval.
func TestCheckAndRunBulkBackup_IntervalNotElapsed(t *testing.T) {
	deviceID := uuid.New()
	recentJob := &domain.BackupJob{
		ID:        uuid.New(),
		DeviceID:  deviceID,
		Status:    domain.BackupStatusSuccess,
		CreatedAt: time.Now().Add(-1 * time.Hour), // 1h ago, interval is 24h
	}

	jobRepo := &checkBulkJobRepo{
		deviceIDs:      []uuid.UUID{deviceID},
		latestByDevice: map[uuid.UUID]*domain.BackupJob{deviceID: recentJob},
	}

	settingsRepo := newMockWorkerSettingsRepo()
	svc := &mockScheduledBackupService{}

	scheduler := &DeviceBackupScheduler{
		backupService: svc,
		jobRepo:       jobRepo,
		settingsRepo:  settingsRepo,
	}

	interval := 24 * time.Hour
	scheduler.checkAndRunBulkBackup(context.Background(), interval)
	if got := svc.startCalls.Load(); got != 0 {
		t.Fatalf("StartBulkBackupRun calls = %d, want 0", got)
	}
}

// TestCheckAndRunBulkBackup_IntervalZeroDisabled verifies that tick() skips
// checkAndRunBulkBackup entirely when the interval is 0 (scheduling disabled).
func TestCheckAndRunBulkBackup_IntervalZeroDisabled(t *testing.T) {
	jobRepo := &checkBulkJobRepo{
		deviceIDs:      nil,
		latestByDevice: map[uuid.UUID]*domain.BackupJob{},
	}

	settingsRepo := newMockWorkerSettingsRepo()
	settingsRepo.Set(domain.SettingDeviceBackupIntervalHours, "0")
	svc := &mockScheduledBackupService{}

	scheduler := &DeviceBackupScheduler{
		backupService: svc,
		jobRepo:       jobRepo,
		settingsRepo:  settingsRepo,
	}

	scheduler.tick(context.Background())
	if got := svc.startCalls.Load(); got != 0 {
		t.Fatalf("StartBulkBackupRun calls = %d, want 0", got)
	}
}
