package service

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

func newInstanceBackupOperationTracker() *instanceBackupOperationTracker {
	return &instanceBackupOperationTracker{
		operations: make(map[uuid.UUID]*instanceBackupOperation),
	}
}

func (t *instanceBackupOperationTracker) hasActive() bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	return len(t.operations) > 0
}

func (t *instanceBackupOperationTracker) begin(id uuid.UUID, cancel context.CancelFunc, progress domain.InstanceBackupProgress) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.operations[id] = &instanceBackupOperation{
		cancel:   cancel,
		progress: progress,
	}
}

func (t *instanceBackupOperationTracker) end(id uuid.UUID) {
	t.mu.Lock()
	defer t.mu.Unlock()
	delete(t.operations, id)
}

func (t *instanceBackupOperationTracker) updateProgress(id uuid.UUID, progress domain.InstanceBackupProgress) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if op := t.operations[id]; op != nil {
		op.progress = progress
	}
}

func (t *instanceBackupOperationTracker) getProgress(id uuid.UUID) (domain.InstanceBackupProgress, bool) {
	t.mu.Lock()
	defer t.mu.Unlock()
	op := t.operations[id]
	if op == nil {
		return domain.InstanceBackupProgress{}, false
	}
	return op.progress, true
}

func (t *instanceBackupOperationTracker) cancellationRequested(id uuid.UUID) bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	op := t.operations[id]
	return op != nil && op.cancelRequested
}

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

	backup, err := loadBackup()
	if err != nil {
		t.mu.Unlock()
		return nil, true, fmt.Errorf("getting backup for cancel: %w", err)
	}
	if backup == nil {
		t.mu.Unlock()
		return nil, true, ErrInstanceBackupNotFound
	}
	if backup.Status != domain.InstanceBackupStatusRunning {
		t.mu.Unlock()
		return nil, true, ErrInstanceBackupNotRunning
	}

	op.cancelRequested = true
	op.cancel()
	op.progress = domain.InstanceBackupProgress{
		Phase:   "cancelling",
		Message: "Cancellation requested",
	}
	t.mu.Unlock()

	backup.ErrorMessage = "cancellation requested"
	return backup, true, nil
}

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
