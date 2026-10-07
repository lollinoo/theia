package postgres

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/lollinoo/theia/internal/domain"
)

func TestBulkBatchReconcileOnlyWritesTransitions(t *testing.T) {
	db := setupTestDB(t)
	deviceID, _ := createTestDevicePair(t, NewDeviceRepo(db, testKeyring, nil))
	repo := NewBulkBackupRunRepo(db)
	runID := uuid.New()
	item := domain.BulkBackupRunItem{ID: uuid.New(), RunID: runID, DeviceID: deviceID, Status: domain.BulkBackupRunItemStatusActive}
	if err := repo.CreateRun(&domain.BulkBackupRun{ID: runID, Status: domain.BulkBackupRunStatusRunning}, []domain.BulkBackupRunItem{item}); err != nil {
		t.Fatal(err)
	}
	job := &domain.BackupJob{ID: uuid.New(), DeviceID: deviceID, Status: domain.BackupStatusRunning}
	if err := repo.CreateBulkRunJob(&item, job); err != nil {
		t.Fatal(err)
	}
	ids := []uuid.UUID{item.ID}
	done, err := repo.ReconcileBulkRunBatch(runID, ids)
	if err != nil || done {
		t.Fatalf("done=%v err=%v", done, err)
	}
	var before, after time.Time
	if err := db.QueryRow("SELECT updated_at FROM backup_bulk_run_items WHERE id=$1", item.ID).Scan(&before); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.ReconcileBulkRunBatch(runID, ids); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow("SELECT updated_at FROM backup_bulk_run_items WHERE id=$1", item.ID).Scan(&after); err != nil {
		t.Fatal(err)
	}
	if !before.Equal(after) {
		t.Fatal("unchanged running job was rewritten")
	}
	if _, err := db.Exec("UPDATE backup_jobs SET status='success' WHERE id=$1", job.ID); err != nil {
		t.Fatal(err)
	}
	done, err = repo.ReconcileBulkRunBatch(runID, ids)
	if err != nil || !done {
		t.Fatalf("done=%v err=%v", done, err)
	}
	run, err := repo.GetRun(runID)
	if err != nil {
		t.Fatal(err)
	}
	if run.SuccessCount != 1 || run.QueuedCount != 0 || run.Items[0].CompletedAt == nil {
		t.Fatalf("run=%+v", run)
	}
	if _, err := repo.ReconcileBulkRunBatch(runID, []uuid.UUID{uuid.New()}); err == nil {
		t.Fatal("missing batch item silently accepted")
	}
}
