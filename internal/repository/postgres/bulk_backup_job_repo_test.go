package postgres

import (
	"github.com/google/uuid"
	"github.com/lollinoo/theia/internal/domain"
	"testing"
)

func TestBulkJobAssociationIsAtomic(t *testing.T) {
	for _, stage := range []string{"success", "stale item", "write failure"} {
		t.Run(stage, func(t *testing.T) {
			db := setupTestDB(t)
			deviceID, _ := createTestDevicePair(t, NewDeviceRepo(db, testKeyring, nil))
			repo := NewBulkBackupRunRepo(db)
			runID := uuid.New()
			item := domain.BulkBackupRunItem{ID: uuid.New(), RunID: runID, DeviceID: deviceID, Status: domain.BulkBackupRunItemStatusActive}
			if err := repo.CreateRun(&domain.BulkBackupRun{ID: runID, Status: domain.BulkBackupRunStatusRunning}, []domain.BulkBackupRunItem{item}); err != nil {
				t.Fatal(err)
			}
			if stage == "stale item" {
				if _, err := db.Exec("UPDATE backup_bulk_run_items SET status='failed' WHERE id=$1", item.ID); err != nil {
					t.Fatal(err)
				}
			}
			if stage == "write failure" {
				if _, err := db.Exec(`CREATE FUNCTION audit_reject_bulk_association() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'injected failure'; END $$`); err != nil {
					t.Fatal(err)
				}
				if _, err := db.Exec(`CREATE TRIGGER audit_bulk_association BEFORE UPDATE ON backup_bulk_run_items FOR EACH ROW EXECUTE FUNCTION audit_reject_bulk_association()`); err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() {
					db.Exec("DROP TRIGGER IF EXISTS audit_bulk_association ON backup_bulk_run_items")
					db.Exec("DROP FUNCTION IF EXISTS audit_reject_bulk_association()")
				})
			}
			job := &domain.BackupJob{ID: uuid.New(), DeviceID: deviceID, Status: domain.BackupStatusPending}
			err := repo.CreateBulkRunJob(&item, job)
			var count int
			if readErr := db.QueryRow("SELECT COUNT(*) FROM backup_jobs WHERE id=$1", job.ID).Scan(&count); readErr != nil {
				t.Fatal(readErr)
			}
			if stage == "success" {
				if err != nil || count != 1 || item.BackupJobID == nil {
					t.Fatalf("err=%v count=%d item=%+v", err, count, item)
				}
			} else if err == nil || count != 0 {
				t.Fatalf("orphaned job: err=%v count=%d", err, count)
			}
		})
	}
}
