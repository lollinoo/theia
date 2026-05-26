package postgres

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/lollinoo/theia/internal/domain"
)

func TestBulkBackupRunRepoCreatesAndReadsRunWithItems(t *testing.T) {
	db := setupTestDB(t)
	repo := NewBulkBackupRunRepo(db)
	runID := uuid.New()
	firstDeviceID := uuid.New()
	secondDeviceID := uuid.New()
	startedAt := time.Now().UTC().Truncate(time.Microsecond)
	run := &domain.BulkBackupRun{
		ID:        runID,
		Status:    domain.BulkBackupRunStatusRunning,
		BatchSize: 10,
		CreatedBy: "admin",
		StartedAt: &startedAt,
	}
	items := []domain.BulkBackupRunItem{
		{
			ID:         uuid.New(),
			RunID:      runID,
			DeviceID:   firstDeviceID,
			DeviceName: "router-01",
			Status:     domain.BulkBackupRunItemStatusChecking,
		},
		{
			ID:         uuid.New(),
			RunID:      runID,
			DeviceID:   secondDeviceID,
			DeviceName: "router-02",
			Status:     domain.BulkBackupRunItemStatusSkipped,
			Reason:     "device offline",
		},
	}

	if err := repo.CreateRun(run, items); err != nil {
		t.Fatalf("CreateRun: %v", err)
	}

	got, err := repo.GetRun(runID)
	if err != nil {
		t.Fatalf("GetRun: %v", err)
	}
	if got == nil {
		t.Fatal("GetRun returned nil")
	}
	if got.ID != runID || got.Status != domain.BulkBackupRunStatusRunning || got.BatchSize != 10 {
		t.Fatalf("run = %+v, want id/status/batch size preserved", got)
	}
	if got.CreatedBy != "admin" {
		t.Fatalf("CreatedBy = %q, want admin", got.CreatedBy)
	}
	if len(got.Items) != 2 {
		t.Fatalf("items len = %d, want 2", len(got.Items))
	}
	if got.Items[0].DeviceName != "router-01" || got.Items[1].Reason != "device offline" {
		t.Fatalf("items = %+v, want persisted names and reasons", got.Items)
	}
}

func TestBulkBackupRunRepoAllowsOnlyOneActiveRun(t *testing.T) {
	db := setupTestDB(t)
	repo := NewBulkBackupRunRepo(db)
	first := &domain.BulkBackupRun{ID: uuid.New(), Status: domain.BulkBackupRunStatusRunning, BatchSize: 10}
	second := &domain.BulkBackupRun{ID: uuid.New(), Status: domain.BulkBackupRunStatusRunning, BatchSize: 10}
	cancelling := &domain.BulkBackupRun{ID: uuid.New(), Status: domain.BulkBackupRunStatusCancelling, BatchSize: 10}

	if err := repo.CreateRun(first, nil); err != nil {
		t.Fatalf("creating first active run: %v", err)
	}
	if err := repo.CreateRun(second, nil); err == nil {
		t.Fatal("creating second active run error = nil, want unique active run violation")
	}
	if err := repo.CreateRun(cancelling, nil); err == nil {
		t.Fatal("creating cancelling run while running exists error = nil, want unique active run violation")
	}

	completedAt := time.Now().UTC()
	first.Status = domain.BulkBackupRunStatusSuccess
	first.CompletedAt = &completedAt
	if err := repo.UpdateRun(first); err != nil {
		t.Fatalf("UpdateRun first: %v", err)
	}
	if err := repo.CreateRun(second, nil); err != nil {
		t.Fatalf("creating second active run after first terminal: %v", err)
	}
}

func TestBulkBackupRunRepoReturnsLatestAndResumableRuns(t *testing.T) {
	db := setupTestDB(t)
	repo := NewBulkBackupRunRepo(db)
	oldRun := &domain.BulkBackupRun{
		ID:        uuid.New(),
		Status:    domain.BulkBackupRunStatusSuccess,
		BatchSize: 10,
		CreatedAt: time.Now().UTC().Add(-time.Hour),
	}
	activeRun := &domain.BulkBackupRun{
		ID:        uuid.New(),
		Status:    domain.BulkBackupRunStatusRunning,
		BatchSize: 10,
		CreatedAt: time.Now().UTC(),
	}

	if err := repo.CreateRun(oldRun, nil); err != nil {
		t.Fatalf("CreateRun old: %v", err)
	}
	if err := repo.CreateRun(activeRun, nil); err != nil {
		t.Fatalf("CreateRun active: %v", err)
	}

	latest, err := repo.GetLatestRun()
	if err != nil {
		t.Fatalf("GetLatestRun: %v", err)
	}
	if latest == nil || latest.ID != activeRun.ID {
		t.Fatalf("latest = %+v, want active run", latest)
	}
	resumable, err := repo.ListResumableRuns()
	if err != nil {
		t.Fatalf("ListResumableRuns: %v", err)
	}
	if len(resumable) != 1 || resumable[0].ID != activeRun.ID {
		t.Fatalf("resumable = %+v, want only active run", resumable)
	}
}

func TestBulkBackupRunRepoRecalculatesCounters(t *testing.T) {
	db := setupTestDB(t)
	repo := NewBulkBackupRunRepo(db)
	runID := uuid.New()
	run := &domain.BulkBackupRun{ID: runID, Status: domain.BulkBackupRunStatusRunning, BatchSize: 10}
	items := []domain.BulkBackupRunItem{
		{ID: uuid.New(), RunID: runID, DeviceID: uuid.New(), Status: domain.BulkBackupRunItemStatusQueued},
		{ID: uuid.New(), RunID: runID, DeviceID: uuid.New(), Status: domain.BulkBackupRunItemStatusSuccess},
		{ID: uuid.New(), RunID: runID, DeviceID: uuid.New(), Status: domain.BulkBackupRunItemStatusFailed, Reason: "boom"},
		{ID: uuid.New(), RunID: runID, DeviceID: uuid.New(), Status: domain.BulkBackupRunItemStatusSkipped, Reason: "unsupported"},
		{ID: uuid.New(), RunID: runID, DeviceID: uuid.New(), Status: domain.BulkBackupRunItemStatusCancelled},
	}
	if err := repo.CreateRun(run, items); err != nil {
		t.Fatalf("CreateRun: %v", err)
	}

	recalculated, err := repo.RecalculateRunCounters(runID)
	if err != nil {
		t.Fatalf("RecalculateRunCounters: %v", err)
	}
	if recalculated.TotalCount != 5 ||
		recalculated.QueuedCount != 1 ||
		recalculated.SuccessCount != 1 ||
		recalculated.FailedCount != 1 ||
		recalculated.SkippedCount != 1 ||
		recalculated.CancelledCount != 1 {
		t.Fatalf("counters = %+v, want 5 total and one per terminal/queued status", recalculated)
	}
}
