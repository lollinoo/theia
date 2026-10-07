package postgres

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"
)

func TestBulkDownloadLeasesUseOneIsolatedConnection(t *testing.T) {
	setupTestDB(t)
	app, err := OpenPrimaryDB(os.Getenv("THEIA_TEST_DB_DSN"))
	if err != nil {
		t.Fatal(err)
	}
	defer app.Close()
	app.SetMaxOpenConns(1)
	pool, err := OpenPrimaryDB(os.Getenv("THEIA_TEST_DB_DSN"))
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	pool.SetMaxOpenConns(5)
	repo := NewBulkOperationLeaseRepo(pool)
	for i := range 4 {
		lease, blocked, err := repo.TryAcquireBulkOperationLeases(context.Background(), []string{fmt.Sprintf("global:%d", i), fmt.Sprintf("actor:%d", i)})
		if err != nil || blocked != -1 {
			t.Fatalf("blocked=%d err=%v", blocked, err)
		}
		defer lease.Release()
	}
	if got := pool.Stats().InUse; got != 4 {
		t.Fatalf("held connections=%d, want 4", got)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	var n int
	if err := app.QueryRowContext(ctx, "SELECT 1").Scan(&n); err != nil {
		t.Fatalf("application pool blocked: %v", err)
	}
	// The second key conflicts. The first must be released with the transaction.
	_, blocked, err := repo.TryAcquireBulkOperationLeases(ctx, []string{"probe", "actor:0"})
	if err != nil || blocked != 1 {
		t.Fatalf("blocked=%d err=%v", blocked, err)
	}
	lease, ok, err := repo.TryAcquireBulkOperationLease(ctx, "probe")
	if err != nil || !ok {
		t.Fatalf("partial lease leaked: ok=%v err=%v", ok, err)
	}
	lease.Release()
	if got := pool.Stats().InUse; got != 4 {
		t.Fatalf("failed acquisition leaked a connection: %d", got)
	}
}
