package service

import (
	"context"
	"errors"
	"github.com/google/uuid"
	"github.com/lollinoo/theia/internal/domain"
	"testing"
	"time"
)

type missingBulkJobRepo struct{ *mockBackupJobRepo }

func (r *missingBulkJobRepo) GetByID(uuid.UUID) (*domain.BackupJob, error) { return nil, nil }

func TestBulkWaiterFinishesOrphanedItems(t *testing.T) {
	for _, missing := range []bool{false, true} {
		t.Run(map[bool]string{false: "no association", true: "missing job"}[missing], func(t *testing.T) {
			repo := newMockBulkBackupRunRepo()
			runID := uuid.New()
			item := domain.BulkBackupRunItem{ID: uuid.New(), RunID: runID, DeviceID: uuid.New(), Status: domain.BulkBackupRunItemStatusActive}
			if missing {
				id := uuid.New()
				item.BackupJobID = &id
			}
			if err := repo.CreateRun(&domain.BulkBackupRun{ID: runID, Status: domain.BulkBackupRunStatusRunning}, []domain.BulkBackupRunItem{item}); err != nil {
				t.Fatal(err)
			}
			svc := &BackupService{bulkRunRepo: repo, jobRepo: &missingBulkJobRepo{newMockBackupJobRepo()}}
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			if err := svc.waitForBulkRunBatchContext(ctx, runID, []domain.BulkBackupRunItem{item}); err != nil {
				t.Fatal(err)
			}
			stored, err := repo.ListRunItems(runID)
			if err != nil {
				t.Fatal(err)
			}
			if stored[0].Status != domain.BulkBackupRunItemStatusFailed || stored[0].Reason == "" {
				t.Fatalf("item=%+v", stored[0])
			}
		})
	}
}

func TestBulkWaiterHasADeadline(t *testing.T) {
	repo := newMockBulkBackupRunRepo()
	jobs := newMockBackupJobRepo()
	runID, jobID := uuid.New(), uuid.New()
	item := domain.BulkBackupRunItem{ID: uuid.New(), RunID: runID, DeviceID: uuid.New(), Status: domain.BulkBackupRunItemStatusActive, BackupJobID: &jobID}
	if err := jobs.Create(&domain.BackupJob{ID: jobID, DeviceID: item.DeviceID, Status: domain.BackupStatusRunning}); err != nil {
		t.Fatal(err)
	}
	if err := repo.CreateRun(&domain.BulkBackupRun{ID: runID, Status: domain.BulkBackupRunStatusRunning}, []domain.BulkBackupRunItem{item}); err != nil {
		t.Fatal(err)
	}
	svc := &BackupService{bulkRunRepo: repo, jobRepo: jobs}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	if err := svc.waitForBulkRunBatchContext(ctx, runID, []domain.BulkBackupRunItem{item}); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("err=%v", err)
	}
}
