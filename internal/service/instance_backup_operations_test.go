package service

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/lollinoo/theia/internal/domain"
)

func TestInstanceBackupOperationTrackerTracksProgressLifecycle(t *testing.T) {
	tracker := newInstanceBackupOperationTracker()
	id := uuid.New()
	initial := domain.InstanceBackupProgress{Phase: "starting", Message: "Preparing backup"}
	updated := domain.InstanceBackupProgress{Phase: "archiving", Message: "Writing backup archive", Current: 10, Total: 20}

	if tracker.hasActive() {
		t.Fatal("new tracker hasActive = true, want false")
	}
	tracker.begin(id, func() {}, initial)
	if !tracker.hasActive() {
		t.Fatal("tracker hasActive = false after begin, want true")
	}
	got, ok := tracker.getProgress(id)
	if !ok {
		t.Fatal("getProgress() ok = false, want true")
	}
	if got != initial {
		t.Fatalf("getProgress() = %#v, want %#v", got, initial)
	}

	tracker.updateProgress(id, updated)
	got, ok = tracker.getProgress(id)
	if !ok {
		t.Fatal("getProgress() ok = false after update, want true")
	}
	if got != updated {
		t.Fatalf("getProgress() = %#v, want %#v", got, updated)
	}

	tracker.end(id)
	if tracker.hasActive() {
		t.Fatal("tracker hasActive = true after end, want false")
	}
	if _, ok := tracker.getProgress(id); ok {
		t.Fatal("getProgress() ok = true after end, want false")
	}
}

func TestInstanceBackupOperationTrackerCancelActiveOperation(t *testing.T) {
	tracker := newInstanceBackupOperationTracker()
	id := uuid.New()
	cancelled := false
	tracker.begin(id, func() { cancelled = true }, domain.InstanceBackupProgress{Phase: "archiving"})
	backup := &domain.InstanceBackup{ID: id, Status: domain.InstanceBackupStatusRunning}

	got, active, err := tracker.requestCancel(id, func() (*domain.InstanceBackup, error) {
		return backup, nil
	})
	if err != nil {
		t.Fatalf("requestCancel() error = %v", err)
	}
	if !active {
		t.Fatal("requestCancel() active = false, want true")
	}
	if !cancelled {
		t.Fatal("operation cancel function was not called")
	}
	if got != backup {
		t.Fatal("requestCancel() did not return loaded backup")
	}
	if got.Status != domain.InstanceBackupStatusRunning {
		t.Fatalf("returned status = %q, want running until worker stops", got.Status)
	}
	if got.ErrorMessage != "cancellation requested" {
		t.Fatalf("returned ErrorMessage = %q, want cancellation requested", got.ErrorMessage)
	}
	if !tracker.cancellationRequested(id) {
		t.Fatal("cancellationRequested() = false, want true")
	}
	progress, ok := tracker.getProgress(id)
	if !ok {
		t.Fatal("getProgress() ok = false, want true")
	}
	if progress.Phase != "cancelling" || progress.Message != "Cancellation requested" {
		t.Fatalf("progress = %#v, want cancellation progress", progress)
	}
}

func TestInstanceBackupOperationTrackerCompleteRejectsCancelledOperation(t *testing.T) {
	tracker := newInstanceBackupOperationTracker()
	id := uuid.New()
	tracker.begin(id, func() {}, domain.InstanceBackupProgress{})
	if _, _, err := tracker.requestCancel(id, func() (*domain.InstanceBackup, error) {
		return &domain.InstanceBackup{ID: id, Status: domain.InstanceBackupStatusRunning}, nil
	}); err != nil {
		t.Fatalf("requestCancel() error = %v", err)
	}

	persisted := false
	err := tracker.completeSuccess(id, 10, true, func() error {
		persisted = true
		return nil
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("completeSuccess() error = %v, want context.Canceled", err)
	}
	if persisted {
		t.Fatal("completeSuccess() persisted cancelled operation")
	}
	if !tracker.hasActive() {
		t.Fatal("cancelled operation should remain active until worker cleanup")
	}
}

func TestInstanceBackupOperationTrackerCompleteUpdatesProgressAndRemovesOperation(t *testing.T) {
	tracker := newInstanceBackupOperationTracker()
	id := uuid.New()
	tracker.begin(id, func() {}, domain.InstanceBackupProgress{Phase: "hashing"})

	persisted := false
	if err := tracker.completeSuccess(id, 42, true, func() error {
		persisted = true
		return nil
	}); err != nil {
		t.Fatalf("completeSuccess() error = %v", err)
	}
	if !persisted {
		t.Fatal("completeSuccess() did not persist")
	}
	if tracker.hasActive() {
		t.Fatal("completeSuccess() left operation active")
	}
}
