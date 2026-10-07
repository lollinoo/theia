package service

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestLoginAdmissionBoundsDummyPasswordWork(t *testing.T) {
	h := newAuthServiceHarness(t)
	entered := make(chan struct{}, defaultPasswordConcurrency)
	release := make(chan struct{})
	h.service.verifyPassword = func(string, string) (bool, error) { entered <- struct{}{}; <-release; return false, nil }
	var wg sync.WaitGroup
	for range defaultPasswordConcurrency {
		wg.Add(1)
		go func() { defer wg.Done(); h.service.Login(context.Background(), LoginInput{Password: "unknown"}) }()
	}
	for range defaultPasswordConcurrency {
		select {
		case <-entered:
		case <-time.After(time.Second):
			t.Fatal("password checks did not start")
		}
	}
	_, err := h.service.Login(context.Background(), LoginInput{Identifier: "other", Password: "unknown"})
	if !errors.Is(err, ErrRuntimeBusy) {
		t.Fatalf("overflow login err=%v", err)
	}
	close(release)
	wg.Wait()
	h.service.verifyPassword = func(string, string) (bool, error) { return false, nil }
	if _, err := h.service.Login(context.Background(), LoginInput{}); !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("admission slot leaked: %v", err)
	}
}

func TestManualBackupRejectsBeforeRepositoryAccessWhenBusy(t *testing.T) {
	s := &BackupService{}
	var releases []func()
	for range defaultBulkBackupWorkerCount {
		release, err := s.admitManualBackup(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		releases = append(releases, release)
	}
	defer func() {
		for _, release := range releases {
			release()
		}
	}()
	// Repositories are deliberately absent: overload must not touch them or create a job.
	if _, err := s.TriggerBackup(context.Background(), uuid.New()); !errors.Is(err, ErrRuntimeBusy) {
		t.Fatalf("err=%v", err)
	}
}

func TestBackupDeviceLockCancellationAndCleanup(t *testing.T) {
	s := &BackupService{}
	id := uuid.New()
	release, err := s.lockBackupDevice(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	if _, err := s.lockBackupDevice(ctx, id); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("wait err=%v", err)
	}
	release()
	for range 100 {
		release, err := s.lockBackupDevice(context.Background(), uuid.New())
		if err != nil {
			t.Fatal(err)
		}
		release()
	}
	if len(s.deviceLocks) != 0 {
		t.Fatalf("unused device locks retained: %d", len(s.deviceLocks))
	}
}
