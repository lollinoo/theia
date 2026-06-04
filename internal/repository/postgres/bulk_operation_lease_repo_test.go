package postgres

import (
	"context"
	"testing"
)

func TestBulkOperationLeaseRepoCoordinatesLocksAcrossInstances(t *testing.T) {
	db := setupTestDB(t)
	firstRepo := NewBulkOperationLeaseRepo(db)
	secondRepo := NewBulkOperationLeaseRepo(db)
	ctx := context.Background()

	firstLease, acquired, err := firstRepo.TryAcquireBulkOperationLease(ctx, "backup.bulk_download:user:operator")
	if err != nil {
		t.Fatalf("first acquire returned error: %v", err)
	}
	if !acquired {
		t.Fatal("first acquire = false, want true")
	}
	defer firstLease.Release()

	secondLease, acquired, err := secondRepo.TryAcquireBulkOperationLease(ctx, "backup.bulk_download:user:operator")
	if err != nil {
		t.Fatalf("second acquire returned error: %v", err)
	}
	if acquired {
		secondLease.Release()
		t.Fatal("second acquire = true while first lease is held, want false")
	}

	if err := firstLease.Release(); err != nil {
		t.Fatalf("release first lease: %v", err)
	}

	thirdLease, acquired, err := secondRepo.TryAcquireBulkOperationLease(ctx, "backup.bulk_download:user:operator")
	if err != nil {
		t.Fatalf("third acquire returned error: %v", err)
	}
	if !acquired {
		t.Fatal("third acquire after release = false, want true")
	}
	if err := thirdLease.Release(); err != nil {
		t.Fatalf("release third lease: %v", err)
	}
}
