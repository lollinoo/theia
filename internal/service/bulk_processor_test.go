package service

import (
	"context"
	"github.com/google/uuid"
	"github.com/lollinoo/theia/internal/domain"
	"testing"
	"time"
)

type rejectedBulkProcessorRepo struct {
	*mockBulkBackupRunRepo
	attempted chan struct{}
}

func (r *rejectedBulkProcessorRepo) TryAcquireBulkRunProcessor(uuid.UUID, string, time.Time) (bool, error) {
	close(r.attempted)
	return false, nil
}

func TestBulkRecoveryAcquiresOwnershipBeforeResettingWork(t *testing.T) {
	repo := &rejectedBulkProcessorRepo{mockBulkBackupRunRepo: newMockBulkBackupRunRepo(), attempted: make(chan struct{})}
	jobs := newMockBackupJobRepo()
	runID, jobID := uuid.New(), uuid.New()
	item := domain.BulkBackupRunItem{ID: uuid.New(), RunID: runID, DeviceID: uuid.New(), Status: domain.BulkBackupRunItemStatusRunning, BackupJobID: &jobID}
	if err := jobs.Create(&domain.BackupJob{ID: jobID, DeviceID: item.DeviceID, Status: domain.BackupStatusRunning}); err != nil {
		t.Fatal(err)
	}
	if err := repo.CreateRun(&domain.BulkBackupRun{ID: runID, Status: domain.BulkBackupRunStatusRunning}, []domain.BulkBackupRunItem{item}); err != nil {
		t.Fatal(err)
	}
	svc := &BackupService{bulkRunRepo: repo, jobRepo: jobs}
	svc.ResumeBulkBackupRuns(context.Background())
	select {
	case <-repo.attempted:
	case <-time.After(time.Second):
		t.Fatal("recovery did not check ownership")
	}
	stored, err := repo.ListRunItems(runID)
	if err != nil {
		t.Fatal(err)
	}
	job, err := jobs.GetByID(jobID)
	if err != nil {
		t.Fatal(err)
	}
	if stored[0].Status != domain.BulkBackupRunItemStatusRunning || stored[0].BackupJobID == nil || job.Status != domain.BackupStatusRunning {
		t.Fatal("recovery reset work owned by another process")
	}
}
