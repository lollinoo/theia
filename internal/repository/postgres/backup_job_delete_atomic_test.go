package postgres

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/lollinoo/theia/internal/domain"
)

func TestBackupJobDeleteRollsBackMetadataAndCleanupQueue(t *testing.T) {
	for _, stage := range []struct{ timing, table string }{
		{"BEFORE", "backup_jobs"},
		{"BEFORE", "backup_files"},
		{"AFTER", "backup_jobs"},
	} {
		t.Run(stage.timing+" "+stage.table, func(t *testing.T) {
			db := setupTestDB(t)
			device := newDeviceImportTestDevice("atomic-delete.example.net")
			if err := NewDeviceRepo(db, testKeyring, nil).Create(device); err != nil {
				t.Fatal(err)
			}
			jobs, files := NewBackupJobRepo(db), NewBackupFileRepo(db)
			job := &domain.BackupJob{ID: uuid.New(), DeviceID: device.ID, Status: domain.BackupStatusSuccess}
			if err := jobs.Create(job); err != nil {
				t.Fatal(err)
			}
			file := &domain.BackupFile{ID: uuid.New(), JobID: job.ID, FileName: "backup.rsc", FilePath: "/backups/atomic/backup.rsc"}
			if err := files.Create(file); err != nil {
				t.Fatal(err)
			}
			cleanupSQL := fmt.Sprintf(`DROP TRIGGER IF EXISTS fail_backup_deletion ON %s; DROP FUNCTION IF EXISTS fail_backup_deletion();`, stage.table)
			t.Cleanup(func() {
				if _, err := db.Exec(cleanupSQL); err != nil {
					t.Errorf("remove failure injection: %v", err)
				}
			})
			if _, err := db.Exec(fmt.Sprintf(`
				CREATE FUNCTION fail_backup_deletion() RETURNS trigger LANGUAGE plpgsql AS $$
				BEGIN RAISE EXCEPTION 'injected backup deletion failure'; END;
				$$;
				CREATE TRIGGER fail_backup_deletion %s DELETE ON %s
				FOR EACH ROW EXECUTE FUNCTION fail_backup_deletion();`, stage.timing, stage.table)); err != nil {
				t.Fatal(err)
			}
			var pgErr *pgconn.PgError
			if err := jobs.DeleteContext(context.Background(), job.ID); !errors.As(err, &pgErr) || pgErr.Code != "P0001" {
				t.Fatalf("delete error=%v, want injected PostgreSQL failure", err)
			}
			if count := importTestCount(t, db, `SELECT COUNT(*) FROM backup_jobs WHERE id=$1`, job.ID); count != 1 {
				t.Fatal("failed deletion removed job metadata")
			}
			if count := importTestCount(t, db, `SELECT COUNT(*) FROM backup_files WHERE id=$1`, file.ID); count != 1 {
				t.Fatal("failed deletion removed file metadata")
			}
			if count := importTestCount(t, db, `SELECT COUNT(*) FROM backup_file_deletions`); count != 0 {
				t.Fatal("failed deletion left a cleanup entry for a live file")
			}
			if _, err := db.Exec(cleanupSQL); err != nil {
				t.Fatal(err)
			}
			if err := jobs.DeleteContext(context.Background(), job.ID); err != nil {
				t.Fatal(err)
			}
			if count := importTestCount(t, db, `SELECT COUNT(*) FROM backup_files WHERE id=$1`, file.ID); count != 0 {
				t.Fatal("successful deletion did not cascade file metadata")
			}
			if pending, err := files.ListPendingFileDeletions(context.Background(), 100); err != nil || len(pending) != 1 || pending[0].ID != file.ID {
				t.Fatalf("successful retry pending=%v error=%v", pending, err)
			}
		})
	}
}
