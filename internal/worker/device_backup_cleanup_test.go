package worker

import (
	"context"
	"testing"
	"time"
)

type cleanupSchedulerBackupService struct {
	mockRetentionBackupService
	contexts chan context.Context
}

func (s *cleanupSchedulerBackupService) CleanupDeletedBackupFiles(ctx context.Context) (int, error) {
	s.contexts <- ctx
	return 1, nil
}

func TestDeviceBackupSchedulerCleansDeletedFilesAtStartupAndAfterRetention(t *testing.T) {
	svc := &cleanupSchedulerBackupService{contexts: make(chan context.Context, 2)}
	scheduler := &DeviceBackupScheduler{backupService: svc, jobRepo: &mockBackupJobRepo{}, settingsRepo: newMockWorkerSettingsRepo(), done: make(chan struct{})}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	scheduler.Start(ctx)
	t.Cleanup(scheduler.Stop)
	select {
	case cleanupCtx := <-svc.contexts:
		deadline, ok := cleanupCtx.Deadline()
		if !ok || time.Until(deadline) > 30*time.Second {
			t.Fatal("startup cleanup is not time bounded")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("scheduler did not retry pending disk cleanup at startup")
	}
	scheduler.runRetention(ctx)
	select {
	case <-svc.contexts:
	case <-time.After(5 * time.Second):
		t.Fatal("retention did not clean captured deletion paths")
	}
}
