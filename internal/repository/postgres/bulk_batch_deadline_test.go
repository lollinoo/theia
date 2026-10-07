package postgres

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/lollinoo/theia/internal/domain"
)

func TestBulkBatchDeadlineCancelsBlockedOwnershipLock(t *testing.T) {
	db := setupTestDB(t)
	repo := NewBulkBackupRunRepo(db)
	id := uuid.New()
	if err := repo.CreateRun(&domain.BulkBackupRun{ID: id, Status: domain.BulkBackupRunStatusRunning}, nil); err != nil {
		t.Fatal(err)
	}
	owner := "deadline-test"
	if ok, err := repo.TryAcquireBulkRunProcessor(id, owner, time.Now().Add(time.Minute)); err != nil || !ok {
		t.Fatalf("acquire=%v err=%v", ok, err)
	}
	lock, err := db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	defer lock.Rollback()
	if _, err := lock.Exec("SELECT id FROM backup_bulk_runs WHERE id=$1 FOR UPDATE", id); err != nil {
		t.Fatal(err)
	}
	scoped := repo.ForProcessor(id, owner).(*BulkBackupRunRepo)
	for _, operation := range []func(context.Context) error{
		func(ctx context.Context) error {
			_, err := scoped.ReconcileBulkRunBatchContext(ctx, id, []uuid.UUID{uuid.New()})
			return err
		},
		func(ctx context.Context) error {
			return scoped.RefreshBulkRunProcessorContext(ctx, id, owner, time.Now().Add(time.Minute))
		},
	} {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
		err := operation(ctx)
		cancel()
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("lock wait escaped its deadline: %v", err)
		}
	}
}
