package service

import (
	"context"
	"errors"

	"github.com/google/uuid"
)

// ErrRuntimeBusy signals bounded runtime admission rejection before costly work.
var ErrRuntimeBusy = errors.New("runtime capacity exhausted; retry later")

const defaultPasswordConcurrency = 4

func (s *AuthService) admitPasswordWork(ctx context.Context) (func(), error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	s.passwordAdmissionOnce.Do(func() {
		limit := s.passwordConcurrency
		if limit <= 0 {
			limit = defaultPasswordConcurrency
		}
		s.passwordAdmission = make(chan struct{}, limit)
	})
	select {
	case s.passwordAdmission <- struct{}{}:
		return func() { <-s.passwordAdmission }, nil
	default:
		return nil, ErrRuntimeBusy
	}
}

func (s *BackupService) backupSlots() chan struct{} {
	s.backupAdmissionOnce.Do(func() { s.backupAdmission = make(chan struct{}, defaultBulkBackupWorkerCount) })
	return s.backupAdmission
}

func (s *BackupService) admitManualBackup(ctx context.Context) (func(), error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	select {
	case s.backupSlots() <- struct{}{}:
		return func() { <-s.backupSlots() }, nil
	default:
		return nil, ErrRuntimeBusy
	}
}

type backupDeviceLock struct {
	slot       chan struct{}
	references int
}

// lockBackupDevice makes lock waiting cancellable and drops unused lock entries.
// Global admission bounds the number of live entries and waiters.
func (s *BackupService) lockBackupDevice(ctx context.Context, id uuid.UUID) (func(), error) {
	s.deviceLocksMu.Lock()
	if s.deviceLocks == nil {
		s.deviceLocks = make(map[uuid.UUID]*backupDeviceLock)
	}
	lock := s.deviceLocks[id]
	if lock == nil {
		lock = &backupDeviceLock{slot: make(chan struct{}, 1)}
		s.deviceLocks[id] = lock
	}
	lock.references++
	s.deviceLocksMu.Unlock()
	drop := func() {
		s.deviceLocksMu.Lock()
		lock.references--
		if lock.references == 0 {
			delete(s.deviceLocks, id)
		}
		s.deviceLocksMu.Unlock()
	}
	select {
	case lock.slot <- struct{}{}:
		return func() { <-lock.slot; drop() }, nil
	case <-ctx.Done():
		drop()
		return nil, ctx.Err()
	}
}
