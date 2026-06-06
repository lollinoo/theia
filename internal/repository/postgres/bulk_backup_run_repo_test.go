package postgres

// This file exercises bulk backup run repo behavior so refactors preserve the documented contract.

import (
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
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
	} else {
		var pgErr *pgconn.PgError
		if !errors.As(err, &pgErr) || pgErr.ConstraintName != "backup_bulk_runs_one_active" {
			t.Fatalf("second active run error = %v, want backup_bulk_runs_one_active constraint", err)
		}
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

func TestBulkBackupRunRepoProcessorLeaseAllowsOneOwner(t *testing.T) {
	db := setupTestDB(t)
	repo := NewBulkBackupRunRepo(db)
	runID := uuid.New()
	if err := repo.CreateRun(&domain.BulkBackupRun{ID: runID, Status: domain.BulkBackupRunStatusRunning, BatchSize: 10}, nil); err != nil {
		t.Fatalf("CreateRun: %v", err)
	}

	acquired, err := repo.TryAcquireBulkRunProcessor(runID, "owner-1", time.Now().UTC().Add(time.Minute))
	if err != nil {
		t.Fatalf("TryAcquire owner-1: %v", err)
	}
	if !acquired {
		t.Fatal("owner-1 acquired = false, want true")
	}
	acquired, err = repo.TryAcquireBulkRunProcessor(runID, "owner-2", time.Now().UTC().Add(time.Minute))
	if err != nil {
		t.Fatalf("TryAcquire owner-2: %v", err)
	}
	if acquired {
		t.Fatal("owner-2 acquired = true, want false while owner-1 lease is active")
	}
	if err := repo.ReleaseBulkRunProcessor(runID, "owner-1"); err != nil {
		t.Fatalf("Release owner-1: %v", err)
	}
	acquired, err = repo.TryAcquireBulkRunProcessor(runID, "owner-2", time.Now().UTC().Add(time.Minute))
	if err != nil {
		t.Fatalf("TryAcquire owner-2 after release: %v", err)
	}
	if !acquired {
		t.Fatal("owner-2 acquired after release = false, want true")
	}
}

func TestBulkBackupRunRepoClaimBulkRunItemIsAtomic(t *testing.T) {
	db := setupTestDB(t)
	repo := NewBulkBackupRunRepo(db)
	runID := uuid.New()
	itemID := uuid.New()
	if err := repo.CreateRun(
		&domain.BulkBackupRun{ID: runID, Status: domain.BulkBackupRunStatusRunning, BatchSize: 10},
		[]domain.BulkBackupRunItem{{
			ID: itemID, RunID: runID, DeviceID: uuid.New(),
			Status: domain.BulkBackupRunItemStatusChecking,
		}},
	); err != nil {
		t.Fatalf("CreateRun: %v", err)
	}

	claimed, ok, err := repo.ClaimBulkRunItem(runID, itemID)
	if err != nil {
		t.Fatalf("Claim first: %v", err)
	}
	if !ok || claimed == nil || claimed.Status != domain.BulkBackupRunItemStatusActive {
		t.Fatalf("first claim = %+v/%v, want active item", claimed, ok)
	}
	claimed, ok, err = repo.ClaimBulkRunItem(runID, itemID)
	if err != nil {
		t.Fatalf("Claim second: %v", err)
	}
	if ok || claimed != nil {
		t.Fatalf("second claim = %+v/%v, want no claim", claimed, ok)
	}
}

func TestBulkBackupRunRepoTerminalStatusDoesNotRegressFromStaleUpdate(t *testing.T) {
	db := setupTestDB(t)
	repo := NewBulkBackupRunRepo(db)
	runID := uuid.New()
	startedAt := time.Now().UTC()
	stale := &domain.BulkBackupRun{
		ID:        runID,
		Status:    domain.BulkBackupRunStatusRunning,
		BatchSize: 10,
		StartedAt: &startedAt,
	}
	if err := repo.CreateRun(stale, nil); err != nil {
		t.Fatalf("CreateRun: %v", err)
	}
	completedAt := time.Now().UTC()
	updated, err := repo.FinishBulkRun(runID, domain.BulkBackupRunStatusSuccess, completedAt)
	if err != nil {
		t.Fatalf("FinishBulkRun: %v", err)
	}
	if !updated {
		t.Fatal("FinishBulkRun updated = false, want true")
	}
	stale.Status = domain.BulkBackupRunStatusPausing
	if err := repo.UpdateRun(stale); err != nil {
		t.Fatalf("stale UpdateRun: %v", err)
	}
	got, err := repo.GetRun(runID)
	if err != nil {
		t.Fatalf("GetRun: %v", err)
	}
	if got.Status != domain.BulkBackupRunStatusSuccess {
		t.Fatalf("status = %s, want success after stale pausing update", got.Status)
	}
}

func TestBulkBackupRunRepoStaleUpdateDoesNotClearCancelRequested(t *testing.T) {
	db := setupTestDB(t)
	repo := NewBulkBackupRunRepo(db)
	runID := uuid.New()
	stale := &domain.BulkBackupRun{
		ID:        runID,
		Status:    domain.BulkBackupRunStatusRunning,
		BatchSize: 10,
	}
	if err := repo.CreateRun(stale, nil); err != nil {
		t.Fatalf("CreateRun: %v", err)
	}
	if _, err := repo.RequestBulkRunCancel(runID); err != nil {
		t.Fatalf("RequestBulkRunCancel: %v", err)
	}
	stale.CancelRequested = false
	if err := repo.UpdateRun(stale); err != nil {
		t.Fatalf("stale UpdateRun: %v", err)
	}
	got, err := repo.GetRun(runID)
	if err != nil {
		t.Fatalf("GetRun: %v", err)
	}
	if !got.CancelRequested {
		t.Fatal("CancelRequested = false, want true after stale update")
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

func TestBulkBackupRunRepoTreatsPausedRunsAsActiveButNotResumable(t *testing.T) {
	db := setupTestDB(t)
	repo := NewBulkBackupRunRepo(db)
	paused := &domain.BulkBackupRun{
		ID:        uuid.New(),
		Status:    domain.BulkBackupRunStatusPaused,
		BatchSize: 10,
		CreatedAt: time.Now().UTC(),
	}
	next := &domain.BulkBackupRun{ID: uuid.New(), Status: domain.BulkBackupRunStatusRunning, BatchSize: 10}

	if err := repo.CreateRun(paused, nil); err != nil {
		t.Fatalf("CreateRun paused: %v", err)
	}
	active, err := repo.GetActiveRun()
	if err != nil {
		t.Fatalf("GetActiveRun: %v", err)
	}
	if active == nil || active.ID != paused.ID {
		t.Fatalf("active = %+v, want paused run", active)
	}
	if err := repo.CreateRun(next, nil); err == nil {
		t.Fatal("creating running run while paused exists error = nil, want unique active run violation")
	}

	resumable, err := repo.ListResumableRuns()
	if err != nil {
		t.Fatalf("ListResumableRuns paused: %v", err)
	}
	if len(resumable) != 0 {
		t.Fatalf("resumable paused = %+v, want none", resumable)
	}

	paused.Status = domain.BulkBackupRunStatusPausing
	if err := repo.UpdateRun(paused); err != nil {
		t.Fatalf("UpdateRun pausing: %v", err)
	}
	resumable, err = repo.ListResumableRuns()
	if err != nil {
		t.Fatalf("ListResumableRuns pausing: %v", err)
	}
	if len(resumable) != 1 || resumable[0].ID != paused.ID {
		t.Fatalf("resumable pausing = %+v, want pausing run", resumable)
	}
}

func TestBulkBackupRunRepoRecalculatesCounters(t *testing.T) {
	db := setupTestDB(t)
	repo := NewBulkBackupRunRepo(db)
	runID := uuid.New()
	run := &domain.BulkBackupRun{ID: runID, Status: domain.BulkBackupRunStatusRunning, BatchSize: 10}
	items := []domain.BulkBackupRunItem{
		{ID: uuid.New(), RunID: runID, DeviceID: uuid.New(), Status: domain.BulkBackupRunItemStatusActive},
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
	if recalculated.TotalCount != 6 ||
		recalculated.QueuedCount != 2 ||
		recalculated.SuccessCount != 1 ||
		recalculated.FailedCount != 1 ||
		recalculated.SkippedCount != 1 ||
		recalculated.CancelledCount != 1 {
		t.Fatalf("counters = %+v, want 6 total and active counted as queued", recalculated)
	}
}
