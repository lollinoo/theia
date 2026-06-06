package service

// This file defines instance backup operations backup and restore service behavior, including filesystem safety and cleanup expectations.

import (
	"context"
	"fmt"
	"sync"

	"github.com/google/uuid"
	"github.com/lollinoo/theia/internal/domain"
)

type instanceBackupOperation struct {
	cancel          context.CancelFunc
	progress        domain.InstanceBackupProgress
	cancelRequested bool
}

type instanceBackupOperationTracker struct {
	mu         sync.Mutex
	operations map[uuid.UUID]*instanceBackupOperation
}

// newInstanceBackupOperationTracker creates the in-memory operation tracker.
func newInstanceBackupOperationTracker() *instanceBackupOperationTracker {
	return &instanceBackupOperationTracker{
		operations: make(map[uuid.UUID]*instanceBackupOperation),
	}
}

// hasActive reports whether any instance backup operation is currently running.
func (t *instanceBackupOperationTracker) hasActive() bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	return len(t.operations) > 0
}

// begin records a cancellable running operation and its initial progress.
func (t *instanceBackupOperationTracker) begin(id uuid.UUID, cancel context.CancelFunc, progress domain.InstanceBackupProgress) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.operations[id] = &instanceBackupOperation{
		cancel:   cancel,
		progress: progress,
	}
}

// end removes a running operation from the tracker.
func (t *instanceBackupOperationTracker) end(id uuid.UUID) {
	t.mu.Lock()
	defer t.mu.Unlock()
	delete(t.operations, id)
}

// updateProgress replaces the best-effort progress snapshot for a running operation.
func (t *instanceBackupOperationTracker) updateProgress(id uuid.UUID, progress domain.InstanceBackupProgress) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if op := t.operations[id]; op != nil {
		op.progress = progress
	}
}

// getProgress returns the best-effort progress snapshot for a running operation.
func (t *instanceBackupOperationTracker) getProgress(id uuid.UUID) (domain.InstanceBackupProgress, bool) {
	t.mu.Lock()
	defer t.mu.Unlock()
	op := t.operations[id]
	if op == nil {
		return domain.InstanceBackupProgress{}, false
	}
	return op.progress, true
}

// cancellationRequested reports whether the running operation has been asked to stop.
func (t *instanceBackupOperationTracker) cancellationRequested(id uuid.UUID) bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	op := t.operations[id]
	return op != nil && op.cancelRequested
}

// requestCancel loads the target backup outside the tracker lock, then marks an active operation as cancelling.
func (t *instanceBackupOperationTracker) requestCancel(
	id uuid.UUID,
	loadBackup func() (*domain.InstanceBackup, error),
) (*domain.InstanceBackup, bool, error) {
	t.mu.Lock()
	op := t.operations[id]
	if op == nil {
		t.mu.Unlock()
		return nil, false, nil
	}
	t.mu.Unlock()

	backup, err := loadBackup()
	if err != nil {
		return nil, true, fmt.Errorf("getting backup for cancel: %w", err)
	}
	if backup == nil {
		return nil, true, ErrInstanceBackupNotFound
	}
	if backup.Status != domain.InstanceBackupStatusRunning {
		return nil, true, ErrInstanceBackupNotRunning
	}

	t.mu.Lock()
	op = t.operations[id]
	if op == nil {
		t.mu.Unlock()
		return nil, false, nil
	}
	op.cancelRequested = true
	cancel := op.cancel
	op.progress = domain.InstanceBackupProgress{
		Phase:   "cancelling",
		Message: "Cancellation requested",
	}
	t.mu.Unlock()

	if cancel != nil {
		cancel()
	}
	backup.ErrorMessage = "cancellation requested"
	return backup, true, nil
}

// completeSuccess persists success unless cancellation won the race.
func (t *instanceBackupOperationTracker) completeSuccess(
	id uuid.UUID,
	totalSize int64,
	ownOperation bool,
	persist func() error,
) error {
	t.mu.Lock()
	defer t.mu.Unlock()

	if op := t.operations[id]; op != nil && op.cancelRequested {
		return context.Canceled
	}
	if err := persist(); err != nil {
		return err
	}
	if ownOperation {
		if op := t.operations[id]; op != nil {
			op.progress = domain.InstanceBackupProgress{
				Phase:   "complete",
				Message: "Backup complete",
				Current: totalSize,
				Total:   totalSize,
			}
		}
	}
	delete(t.operations, id)
	return nil
}

// hasActiveInstanceBackupOperation delegates active-operation checks to the tracker.
func (s *InstanceBackupService) hasActiveInstanceBackupOperation() bool {
	return s.operations.hasActive()
}

// beginInstanceBackupOperation records a service-level running backup operation.
func (s *InstanceBackupService) beginInstanceBackupOperation(id uuid.UUID, cancel context.CancelFunc, progress domain.InstanceBackupProgress) {
	s.operations.begin(id, cancel, progress)
}

// endInstanceBackupOperation removes a service-level running backup operation.
func (s *InstanceBackupService) endInstanceBackupOperation(id uuid.UUID) {
	s.operations.end(id)
}

// updateInstanceBackupProgress publishes progress for API polling.
func (s *InstanceBackupService) updateInstanceBackupProgress(id uuid.UUID, progress domain.InstanceBackupProgress) {
	s.operations.updateProgress(id, progress)
}

// instanceBackupCancellationRequested checks whether backup work should stop early.
func (s *InstanceBackupService) instanceBackupCancellationRequested(id uuid.UUID) bool {
	return s.operations.cancellationRequested(id)
}

// GetProgress returns best-effort in-memory progress for a running instance backup.
func (s *InstanceBackupService) GetProgress(id uuid.UUID) (domain.InstanceBackupProgress, bool) {
	return s.operations.getProgress(id)
}

// Cancel requests cancellation of a running instance backup.
func (s *InstanceBackupService) Cancel(ctx context.Context, id uuid.UUID) (*domain.InstanceBackup, error) {
	if ctx != nil {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
	}

	if backup, active, err := s.operations.requestCancel(id, func() (*domain.InstanceBackup, error) {
		return s.repo.GetByID(id)
	}); active || err != nil {
		return backup, err
	}

	backup, err := s.repo.GetByID(id)
	if err != nil {
		return nil, fmt.Errorf("getting backup for cancel: %w", err)
	}
	if backup == nil {
		return nil, ErrInstanceBackupNotFound
	}
	if backup.Status != domain.InstanceBackupStatusRunning {
		return nil, ErrInstanceBackupNotRunning
	}

	backup.Status = domain.InstanceBackupStatusCancelled
	backup.ErrorMessage = "cancelled by user"
	if err := s.repo.Update(backup); err != nil {
		return nil, fmt.Errorf("updating cancelled backup: %w", err)
	}
	return backup, nil
}
