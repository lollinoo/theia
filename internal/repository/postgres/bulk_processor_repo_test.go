package postgres

import (
	"errors"
	"github.com/google/uuid"
	"github.com/lollinoo/theia/internal/domain"
	"testing"
	"time"
)

func TestBulkProcessorFencesAStaleOwner(t *testing.T) {
	db := setupTestDB(t)
	deviceID, _ := createTestDevicePair(t, NewDeviceRepo(db, testKeyring, nil))
	first := NewBulkBackupRunRepo(db)
	second := NewBulkBackupRunRepo(db)
	runID := uuid.New()
	item := domain.BulkBackupRunItem{ID: uuid.New(), RunID: runID, DeviceID: deviceID, Status: domain.BulkBackupRunItemStatusChecking}
	run := &domain.BulkBackupRun{ID: runID, Status: domain.BulkBackupRunStatusRunning}
	if err := first.CreateRun(run, []domain.BulkBackupRunItem{item}); err != nil {
		t.Fatal(err)
	}
	if ok, err := first.TryAcquireBulkRunProcessor(runID, "first", time.Now().Add(time.Minute)); err != nil || !ok {
		t.Fatalf("ok=%v err=%v", ok, err)
	}
	owner := first.ForProcessor(runID, "first").(*BulkBackupRunRepo)
	if _, err := db.Exec("UPDATE backup_bulk_runs SET processing_lease_expires_at=NOW()-INTERVAL '1 second' WHERE id=$1", runID); err != nil {
		t.Fatal(err)
	}
	if ok, err := second.TryAcquireBulkRunProcessor(runID, "second", time.Now().Add(time.Minute)); err != nil || !ok {
		t.Fatalf("takeover ok=%v err=%v", ok, err)
	}
	checks := []func() error{
		func() error { return owner.UpdateRun(run) }, func() error { return owner.UpdateRunItem(&item) },
		func() error { _, _, err := owner.ClaimBulkRunItem(runID, item.ID); return err },
		func() error { _, err := owner.RecalculateRunCounters(runID); return err },
		func() error {
			_, err := owner.FinishBulkRun(runID, domain.BulkBackupRunStatusFailed, time.Now())
			return err
		},
		func() error { return owner.RecoverBulkRun(runID) },
		func() error {
			return owner.CreateBulkRunJob(&item, &domain.BackupJob{ID: uuid.New(), DeviceID: deviceID, Status: domain.BackupStatusPending})
		},
		func() error { return first.RefreshBulkRunProcessor(runID, "first", time.Now().Add(time.Minute)) },
	}
	for i, check := range checks {
		if err := check(); !errors.Is(err, domain.ErrBulkBackupProcessorLeaseLost) {
			t.Fatalf("mutation %d err=%v, want lost lease", i, err)
		}
	}
	stored, err := second.GetRun(runID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.Status != domain.BulkBackupRunStatusRunning || stored.Items[0].Status != domain.BulkBackupRunItemStatusChecking {
		t.Fatalf("stale processor mutated run: %+v", stored)
	}
	var jobs int
	if err := db.QueryRow("SELECT COUNT(*) FROM backup_jobs").Scan(&jobs); err != nil {
		t.Fatal(err)
	}
	if jobs != 0 {
		t.Fatal("stale processor created orphaned job")
	}
	if err := second.ForProcessor(runID, "second").(*BulkBackupRunRepo).RecoverBulkRun(runID); err != nil {
		t.Fatalf("current owner failed recovery: %v", err)
	}
}
