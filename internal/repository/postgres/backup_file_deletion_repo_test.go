package postgres

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/lollinoo/theia/internal/domain"
)

func TestBackupFileDeletionCapturesEveryMetadataDeletion(t *testing.T) {
	for _, deletion := range []string{"failed retention", "device cascade", "job cascade", "device jobs", "file records"} {
		t.Run(deletion, func(t *testing.T) {
			db := setupTestDB(t)
			devices := NewDeviceRepo(db, testKeyring, nil)
			device := newDeviceImportTestDevice("cleanup.example.net")
			if err := devices.Create(device); err != nil {
				t.Fatal(err)
			}
			jobs, files := NewBackupJobRepo(db), NewBackupFileRepo(db)
			job := &domain.BackupJob{ID: uuid.New(), DeviceID: device.ID, Status: domain.BackupStatusFailed, CreatedAt: time.Now().Add(-8 * 24 * time.Hour)}
			if err := jobs.Create(job); err != nil {
				t.Fatal(err)
			}
			file := &domain.BackupFile{ID: uuid.New(), JobID: job.ID, FileName: "partial.rsc", FilePath: "/backups/cleanup/partial.rsc"}
			if err := files.Create(file); err != nil {
				t.Fatal(err)
			}
			var err error
			switch deletion {
			case "failed retention":
				var count int
				count, err = jobs.DeleteFailedOlderThan(time.Now().Add(-7 * 24 * time.Hour))
				if err == nil && count != 1 {
					t.Fatalf("deleted %d jobs, want 1", count)
				}
			case "device cascade":
				err = devices.Delete(device.ID)
			case "job cascade":
				err = jobs.Delete(job.ID)
			case "device jobs":
				err = jobs.DeleteByDeviceID(device.ID)
			case "file records":
				err = files.DeleteByJobID(job.ID)
			}
			if err != nil {
				t.Fatal(err)
			}
			pending, err := files.ListPendingFileDeletions(context.Background(), 100)
			if err != nil || len(pending) != 1 || pending[0].ID != file.ID || pending[0].FilePath != file.FilePath || pending[0].FileName != file.FileName {
				t.Fatalf("pending=%v error=%v, want captured file path", pending, err)
			}
			if count := importTestCount(t, db, "SELECT COUNT(*) FROM backup_files WHERE id=$1", file.ID); count != 0 {
				t.Fatal("metadata was not deleted")
			}
			if err := files.CompleteFileDeletion(context.Background(), file.ID); err != nil {
				t.Fatal(err)
			}
			if count := importTestCount(t, db, "SELECT COUNT(*) FROM backup_file_deletions"); count != 0 {
				t.Fatal("completed file deletion remained queued")
			}
		})
	}
}

func TestBackupFileDeletionRollbackAndRetry(t *testing.T) {
	db := setupTestDB(t)
	devices := NewDeviceRepo(db, testKeyring, nil)
	device := newDeviceImportTestDevice("retry.example.net")
	if err := devices.Create(device); err != nil {
		t.Fatal(err)
	}
	jobs, files := NewBackupJobRepo(db), NewBackupFileRepo(db)
	job := &domain.BackupJob{ID: uuid.New(), DeviceID: device.ID, Status: domain.BackupStatusFailed}
	if err := jobs.Create(job); err != nil {
		t.Fatal(err)
	}
	file := &domain.BackupFile{ID: uuid.New(), JobID: job.ID, FileName: "retry.rsc", FilePath: "/backups/retry/retry.rsc"}
	if err := files.Create(file); err != nil {
		t.Fatal(err)
	}
	tx, err := db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	if _, err := tx.Exec("DELETE FROM backup_jobs WHERE id=$1", job.ID); err != nil {
		t.Fatal(err)
	}
	if err := tx.Rollback(); err != nil {
		t.Fatal(err)
	}
	if count := importTestCount(t, db, "SELECT COUNT(*) FROM backup_file_deletions"); count != 0 {
		t.Fatal("rolled-back deletion leaked into the cleanup queue")
	}
	if referenced, err := files.BackupFilePathsReferenced(context.Background(), []string{file.FilePath}); err != nil || !referenced {
		t.Fatalf("live reference=%v error=%v", referenced, err)
	}
	if err := jobs.Delete(job.ID); err != nil {
		t.Fatal(err)
	}
	if err := files.DeferFileDeletion(context.Background(), file.ID); err != nil {
		t.Fatal(err)
	}
	if pending, err := files.ListPendingFileDeletions(context.Background(), 100); err != nil || len(pending) != 0 {
		t.Fatalf("deferred deletion was returned: pending=%v error=%v", pending, err)
	}
	if count := importTestCount(t, db, "SELECT COUNT(*) FROM backup_file_deletions WHERE id=$1", file.ID); count != 1 {
		t.Fatal("retry lost the captured path")
	}
}
