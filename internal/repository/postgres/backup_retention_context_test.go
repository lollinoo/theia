package postgres

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/lollinoo/theia/internal/domain"
)

func TestBackupRetentionQueriesCancelWhilePoolIsSaturated(t *testing.T) {
	setupTestDB(t)
	db, err := sql.Open("pgx", os.Getenv("THEIA_TEST_DB_DSN"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	db.SetMaxOpenConns(1)
	setupCtx, setupCancel := context.WithTimeout(context.Background(), time.Second)
	defer setupCancel()
	conn, err := db.Conn(setupCtx)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	jobs, files := NewBackupJobRepo(db), NewBackupFileRepo(db)
	for name, operation := range map[string]func(context.Context) error{
		"settings": func(ctx context.Context) error {
			_, err := NewSettingsRepo(db).GetContext(ctx, domain.SettingDeviceBackupRetentionCount)
			return err
		},
		"devices": func(ctx context.Context) error { _, err := jobs.ListAllDeviceIDsContext(ctx); return err },
		"successful jobs": func(ctx context.Context) error {
			_, err := jobs.ListSuccessfulByDeviceOldestContext(ctx, uuid.New())
			return err
		},
		"failed cleanup": func(ctx context.Context) error {
			_, err := jobs.DeleteFailedOlderThanContext(ctx, time.Now())
			return err
		},
		"job":          func(ctx context.Context) error { _, err := jobs.GetByIDContext(ctx, uuid.New()); return err },
		"delete job":   func(ctx context.Context) error { return jobs.DeleteContext(ctx, uuid.New()) },
		"files":        func(ctx context.Context) error { _, err := files.GetByJobIDContext(ctx, uuid.New()); return err },
		"delete files": func(ctx context.Context) error { return files.DeleteByJobIDContext(ctx, uuid.New()) },
		"active references": func(ctx context.Context) error {
			_, err := NewBulkBackupRunRepo(db).BackupJobReferencedByActiveRunContext(ctx, uuid.New())
			return err
		},
	} {
		t.Run(name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
			defer cancel()
			if err := operation(ctx); !errors.Is(err, context.DeadlineExceeded) {
				t.Fatalf("pool wait ignored deadline: %v", err)
			}
		})
	}
}

func TestBackupRetentionQueriesCancelBlockedStatements(t *testing.T) {
	db := setupTestDB(t)
	tx, err := db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`LOCK TABLE settings, backup_jobs, backup_files, backup_bulk_runs, backup_bulk_run_items IN ACCESS EXCLUSIVE MODE`); err != nil {
		t.Fatal(err)
	}
	jobs, files := NewBackupJobRepo(db), NewBackupFileRepo(db)
	for _, operation := range []func(context.Context) error{
		func(ctx context.Context) error {
			_, err := NewSettingsRepo(db).GetContext(ctx, domain.SettingDeviceBackupRetentionCount)
			return err
		},
		func(ctx context.Context) error { _, err := jobs.ListAllDeviceIDsContext(ctx); return err },
		func(ctx context.Context) error {
			_, err := jobs.ListSuccessfulByDeviceOldestContext(ctx, uuid.New())
			return err
		},
		func(ctx context.Context) error {
			_, err := jobs.DeleteFailedOlderThanContext(ctx, time.Now())
			return err
		},
		func(ctx context.Context) error { _, err := jobs.GetByIDContext(ctx, uuid.New()); return err },
		func(ctx context.Context) error { return jobs.DeleteContext(ctx, uuid.New()) },
		func(ctx context.Context) error { _, err := files.GetByJobIDContext(ctx, uuid.New()); return err },
		func(ctx context.Context) error { return files.DeleteByJobIDContext(ctx, uuid.New()) },
		func(ctx context.Context) error {
			_, err := NewBulkBackupRunRepo(db).BackupJobReferencedByActiveRunContext(ctx, uuid.New())
			return err
		},
	} {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
		err := operation(ctx)
		cancel()
		if err == nil || ctx.Err() != context.DeadlineExceeded {
			t.Fatalf("blocked SQL did not stop at deadline: %v", err)
		}
	}
}

func TestBackupRetentionReferenceQueryMatchesActiveStatuses(t *testing.T) {
	db := setupTestDB(t)
	deviceID, _ := createTestDevicePair(t, NewDeviceRepo(db, testKeyring, nil))
	job := &domain.BackupJob{ID: uuid.New(), DeviceID: deviceID, Status: domain.BackupStatusSuccess}
	if err := NewBackupJobRepo(db).Create(job); err != nil {
		t.Fatal(err)
	}
	run := &domain.BulkBackupRun{ID: uuid.New(), Status: domain.BulkBackupRunStatusRunning}
	item := domain.BulkBackupRunItem{ID: uuid.New(), DeviceID: deviceID, Status: domain.BulkBackupRunItemStatusQueued, BackupJobID: &job.ID}
	repo := NewBulkBackupRunRepo(db)
	if err := repo.CreateRun(run, []domain.BulkBackupRunItem{item}); err != nil {
		t.Fatal(err)
	}
	for _, runStatus := range []string{"running", "pausing", "paused", "cancelling", "success", "partial", "failed", "cancelled"} {
		for _, itemStatus := range []string{"checking", "active", "queued", "running", "success", "failed", "skipped", "cancelled"} {
			if _, err := db.Exec(`UPDATE backup_bulk_runs SET status=$1 WHERE id=$2`, runStatus, run.ID); err != nil {
				t.Fatal(err)
			}
			if _, err := db.Exec(`UPDATE backup_bulk_run_items SET status=$1 WHERE id=$2`, itemStatus, item.ID); err != nil {
				t.Fatal(err)
			}
			got, err := repo.BackupJobReferencedByActiveRunContext(context.Background(), job.ID)
			wantRun := runStatus == "running" || runStatus == "pausing" || runStatus == "paused" || runStatus == "cancelling"
			wantItem := itemStatus == "checking" || itemStatus == "active" || itemStatus == "queued" || itemStatus == "running"
			if err != nil || got != (wantRun && wantItem) {
				t.Fatalf("run=%s item=%s referenced=%v error=%v", runStatus, itemStatus, got, err)
			}
		}
	}
}
