package service

// This file exercises bulk backup run behavior so refactors preserve the documented contract.

import (
	"testing"

	"github.com/google/uuid"
	"github.com/lollinoo/theia/internal/domain"
)

func TestBulkRunTerminalStatusHelpers(t *testing.T) {
	for _, status := range []domain.BulkBackupRunStatus{
		domain.BulkBackupRunStatusSuccess,
		domain.BulkBackupRunStatusPartial,
		domain.BulkBackupRunStatusFailed,
		domain.BulkBackupRunStatusCancelled,
	} {
		if !bulkRunTerminal(status) {
			t.Fatalf("bulkRunTerminal(%s) = false, want true", status)
		}
	}
	for _, status := range []domain.BulkBackupRunStatus{
		domain.BulkBackupRunStatusRunning,
		domain.BulkBackupRunStatusPausing,
		domain.BulkBackupRunStatusPaused,
		domain.BulkBackupRunStatusCancelling,
	} {
		if bulkRunTerminal(status) {
			t.Fatalf("bulkRunTerminal(%s) = true, want false", status)
		}
	}
}

func TestBulkRunItemTerminalStatusHelpers(t *testing.T) {
	for _, status := range []domain.BulkBackupRunItemStatus{
		domain.BulkBackupRunItemStatusSkipped,
		domain.BulkBackupRunItemStatusSuccess,
		domain.BulkBackupRunItemStatusFailed,
		domain.BulkBackupRunItemStatusCancelled,
	} {
		if !bulkRunItemTerminal(status) {
			t.Fatalf("bulkRunItemTerminal(%s) = false, want true", status)
		}
	}
	for _, status := range []domain.BulkBackupRunItemStatus{
		domain.BulkBackupRunItemStatusChecking,
		domain.BulkBackupRunItemStatusActive,
		domain.BulkBackupRunItemStatusRunning,
	} {
		if bulkRunItemTerminal(status) {
			t.Fatalf("bulkRunItemTerminal(%s) = true, want false", status)
		}
	}
}

func TestNextBulkRunBatchSkipsTerminalItemsAndUsesDefaultBatchSize(t *testing.T) {
	items := []domain.BulkBackupRunItem{
		{ID: uuid.New(), Status: domain.BulkBackupRunItemStatusSuccess},
		{ID: uuid.New(), Status: domain.BulkBackupRunItemStatusChecking},
		{ID: uuid.New(), Status: domain.BulkBackupRunItemStatusCancelled},
		{ID: uuid.New(), Status: domain.BulkBackupRunItemStatusActive},
	}

	batch := nextBulkRunBatch(items, 0)
	if len(batch) != 2 {
		t.Fatalf("len(nextBulkRunBatch) = %d, want 2", len(batch))
	}
	if batch[0].Status != domain.BulkBackupRunItemStatusChecking ||
		batch[1].Status != domain.BulkBackupRunItemStatusActive {
		t.Fatalf("batch statuses = [%s %s], want [checking active]", batch[0].Status, batch[1].Status)
	}
}

func TestNextBulkRunBatchHonorsPositiveBatchSize(t *testing.T) {
	items := []domain.BulkBackupRunItem{
		{ID: uuid.New(), Status: domain.BulkBackupRunItemStatusChecking},
		{ID: uuid.New(), Status: domain.BulkBackupRunItemStatusActive},
		{ID: uuid.New(), Status: domain.BulkBackupRunItemStatusRunning},
	}

	batch := nextBulkRunBatch(items, 2)
	if len(batch) != 2 {
		t.Fatalf("len(nextBulkRunBatch) = %d, want 2", len(batch))
	}
}
