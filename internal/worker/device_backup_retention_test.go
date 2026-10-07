package worker

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/lollinoo/theia/internal/domain"
)

type contextualRetentionJobRepo struct {
	mockBackupJobRepo
	listDevices  func(context.Context) ([]uuid.UUID, error)
	listJobs     func(context.Context, uuid.UUID) ([]domain.BackupJob, error)
	deleteFailed func(context.Context, time.Time) (int, error)
}

func (r *contextualRetentionJobRepo) ListAllDeviceIDsContext(ctx context.Context) ([]uuid.UUID, error) {
	if r.listDevices != nil {
		return r.listDevices(ctx)
	}
	return r.deviceIDs, nil
}
func (r *contextualRetentionJobRepo) ListSuccessfulByDeviceOldestContext(ctx context.Context, id uuid.UUID) ([]domain.BackupJob, error) {
	if r.listJobs != nil {
		return r.listJobs(ctx, id)
	}
	return r.jobsByDevice[id], nil
}
func (r *contextualRetentionJobRepo) DeleteFailedOlderThanContext(ctx context.Context, cutoff time.Time) (int, error) {
	if r.deleteFailed != nil {
		return r.deleteFailed(ctx, cutoff)
	}
	return 0, nil
}

type contextualRetentionSettings struct {
	*mockWorkerSettingsRepo
	get func(context.Context) (string, error)
}

func (r *contextualRetentionSettings) GetContext(ctx context.Context, key string) (string, error) {
	if r.get != nil {
		return r.get(ctx)
	}
	return r.Get(key)
}

type contextualRetentionService struct {
	mockRetentionBackupService
	delete func(context.Context, uuid.UUID) error
}

func (s *contextualRetentionService) DeleteBackupJob(ctx context.Context, id uuid.UUID) error {
	return s.delete(ctx, id)
}

func TestRetentionStopsBlockedOperationsAtDeadline(t *testing.T) {
	for _, stage := range []string{"settings", "failed cleanup", "devices", "jobs", "deletion"} {
		t.Run(stage, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
			defer cancel()
			wait := func(received context.Context) error {
				if received != ctx {
					t.Fatal("operation lost the sweep context")
				}
				<-received.Done()
				return received.Err()
			}
			id := uuid.New()
			repo := &contextualRetentionJobRepo{mockBackupJobRepo: mockBackupJobRepo{deviceIDs: []uuid.UUID{id}, jobsByDevice: map[uuid.UUID][]domain.BackupJob{id: {{ID: uuid.New()}, {ID: uuid.New()}}}}}
			settings := &contextualRetentionSettings{mockWorkerSettingsRepo: newMockWorkerSettingsRepo()}
			settings.Set(domain.SettingDeviceBackupRetentionCount, "1")
			svc := &contextualRetentionService{delete: func(context.Context, uuid.UUID) error { return nil }}
			switch stage {
			case "settings":
				settings.get = func(ctx context.Context) (string, error) { return "", wait(ctx) }
			case "failed cleanup":
				repo.deleteFailed = func(ctx context.Context, _ time.Time) (int, error) { return 0, wait(ctx) }
			case "devices":
				repo.listDevices = func(ctx context.Context) ([]uuid.UUID, error) { return nil, wait(ctx) }
			case "jobs":
				repo.listJobs = func(ctx context.Context, _ uuid.UUID) ([]domain.BackupJob, error) { return nil, wait(ctx) }
			case "deletion":
				svc.delete = func(ctx context.Context, _ uuid.UUID) error { return wait(ctx) }
			}
			scheduler := &DeviceBackupScheduler{backupService: svc, jobRepo: repo, settingsRepo: settings}
			started := time.Now()
			scheduler.runRetentionSweep(ctx)
			if !errors.Is(ctx.Err(), context.DeadlineExceeded) || time.Since(started) > time.Second {
				t.Fatal("blocked operation exceeded sweep deadline")
			}
		})
	}
}

func TestRetentionResumesAfterAttemptedDeviceAndWraps(t *testing.T) {
	ids := make([]uuid.UUID, 205)
	for i := range ids {
		ids[i][14] = byte((i + 1) >> 8)
		ids[i][15] = byte(i + 1)
	}
	repo := &contextualRetentionJobRepo{mockBackupJobRepo: mockBackupJobRepo{deviceIDs: ids}}
	scheduler := &DeviceBackupScheduler{jobRepo: repo, settingsRepo: newMockWorkerSettingsRepo()}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var visited []uuid.UUID
	repo.listJobs = func(_ context.Context, id uuid.UUID) ([]domain.BackupJob, error) {
		visited = append(visited, id)
		if id == ids[99] {
			cancel()
		}
		return nil, nil
	}
	scheduler.runRetentionSweep(ctx)
	if len(visited) != 100 {
		t.Fatalf("visited %d devices after cancellation, want 100", len(visited))
	}
	// Query order changes, the cursor device disappears, and a new earlier
	// device is added. Resume by identity rather than an array offset.
	repo.deviceIDs = []uuid.UUID{uuid.MustParse("00000000-0000-0000-0000-000000000000")}
	for i := len(ids) - 1; i >= 0; i-- {
		if i != 99 {
			repo.deviceIDs = append(repo.deviceIDs, ids[i])
		}
	}
	visited = nil
	repo.listJobs = func(_ context.Context, id uuid.UUID) ([]domain.BackupJob, error) {
		visited = append(visited, id)
		return nil, nil
	}
	scheduler.runRetentionSweep(context.Background())
	if len(visited) != 205 || visited[0] != ids[100] {
		t.Fatalf("sweep restarted instead of resuming: visited=%d first=%s", len(visited), visited[0])
	}
	seen := make(map[uuid.UUID]bool)
	for _, id := range visited {
		if seen[id] {
			t.Fatal("device repeated within one sweep")
		}
		seen[id] = true
	}
	if scheduler.retentionCursor != uuid.Nil {
		t.Fatal("completed sweep did not reset cursor")
	}
}

func TestRetentionUsesOneBudgetIncludingFileCleanup(t *testing.T) {
	var sweepContext context.Context
	repo := &contextualRetentionJobRepo{deleteFailed: func(ctx context.Context, _ time.Time) (int, error) { sweepContext = ctx; return 0, nil }}
	svc := &cleanupSchedulerBackupService{contexts: make(chan context.Context, 1)}
	scheduler := &DeviceBackupScheduler{jobRepo: repo, backupService: svc, settingsRepo: newMockWorkerSettingsRepo()}
	scheduler.runRetention(context.Background())
	cleanupContext := <-svc.contexts
	deadline, ok := sweepContext.Deadline()
	cleanupDeadline, cleanupOK := cleanupContext.Deadline()
	if !ok || !cleanupOK || time.Until(deadline) > 60*time.Second || cleanupDeadline.After(deadline) {
		t.Fatal("retention and disk cleanup do not share the 60s budget")
	}
}

func TestRetentionKeepsCursorWhenLastDeviceTimesOut(t *testing.T) {
	id := uuid.New()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	repo := &contextualRetentionJobRepo{mockBackupJobRepo: mockBackupJobRepo{deviceIDs: []uuid.UUID{id}}, listJobs: func(context.Context, uuid.UUID) ([]domain.BackupJob, error) { cancel(); return nil, context.Canceled }}
	scheduler := &DeviceBackupScheduler{jobRepo: repo, settingsRepo: newMockWorkerSettingsRepo()}
	scheduler.runRetentionSweep(ctx)
	if scheduler.retentionCursor != id {
		t.Fatal("cancelled last device reset sweep progress")
	}
}
