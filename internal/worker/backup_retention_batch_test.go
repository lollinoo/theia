package worker

import (
	"context"
	"fmt"
	"testing"

	"github.com/google/uuid"
	"github.com/lollinoo/theia/internal/domain"
)

type retentionBatchJobs struct {
	mockBackupJobRepo
	jobs    []domain.BackupJob
	cursors []uuid.UUID
}

func (r *retentionBatchJobs) ListRetentionCandidatesContext(ctx context.Context, keep int, after uuid.UUID, limit int) ([]domain.BackupJob, error) {
	r.cursors = append(r.cursors, after)
	if limit != backupRetentionPageSize {
		panic("wrong page bound")
	}
	out := make([]domain.BackupJob, 0, limit)
	for _, job := range r.jobs {
		if job.ID.String() > after.String() {
			out = append(out, job)
			if len(out) == limit {
				break
			}
		}
	}
	return out, ctx.Err()
}

type retentionBatchService struct {
	mockRetentionBackupService
	cancel   context.CancelFunc
	cancelAt int
	ids      []uuid.UUID
}

func (s *retentionBatchService) DeleteBackupJob(ctx context.Context, id uuid.UUID) error {
	s.ids = append(s.ids, id)
	if s.cancel != nil && len(s.ids) == s.cancelAt {
		s.cancel()
		return ctx.Err()
	}
	return nil
}

func TestRetentionUsesPagesAndResumesCancelledDeletion(t *testing.T) {
	repo := &retentionBatchJobs{}
	for i := 1; i <= 120; i++ {
		repo.jobs = append(repo.jobs, domain.BackupJob{ID: uuid.MustParse(fmt.Sprintf("00000000-0000-0000-0000-%012d", i))})
	}
	service := &retentionBatchService{}
	scheduler := &DeviceBackupScheduler{jobRepo: repo, backupService: service, settingsRepo: newMockWorkerSettingsRepo()}
	scheduler.runRetentionSweep(context.Background())
	if len(repo.cursors) != 2 || len(service.ids) != 120 || repo.listSuccessfulCalls.Load() != 0 || scheduler.retentionJobCursor != uuid.Nil {
		t.Fatalf("page work=%d deletions=%d cursor=%s", len(repo.cursors), len(service.ids), scheduler.retentionJobCursor)
	}
	ctx, cancel := context.WithCancel(context.Background())
	service.ids = nil
	repo.cursors = nil
	service.cancel = cancel
	service.cancelAt = 3
	scheduler.runRetentionSweep(ctx)
	if scheduler.retentionJobCursor != repo.jobs[1].ID {
		t.Fatalf("cursor passed cancelled deletion: %s", scheduler.retentionJobCursor)
	}
	service.cancel = nil
	scheduler.runRetentionSweep(context.Background())
	if repo.cursors[1] != repo.jobs[1].ID || service.ids[3] != repo.jobs[2].ID {
		t.Fatal("cancelled batch did not resume at unfinished job")
	}
}
