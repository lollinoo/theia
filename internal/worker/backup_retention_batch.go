package worker

import (
	"context"
	"log"

	"github.com/google/uuid"
	"github.com/lollinoo/theia/internal/domain"
)

const backupRetentionPageSize = 100

type backupRetentionBatchRepository interface {
	ListRetentionCandidatesContext(context.Context, int, uuid.UUID, int) ([]domain.BackupJob, error)
}

func (s *DeviceBackupScheduler) runRetentionBatches(ctx context.Context, repo backupRetentionBatchRepository, keep int) {
	deleted := 0
	defer func() {
		if deleted > 0 {
			log.Printf("Device backup retention: deleted %d old jobs", deleted)
		}
	}()
	for ctx.Err() == nil {
		jobs, err := repo.ListRetentionCandidatesContext(ctx, keep, s.retentionJobCursor, backupRetentionPageSize)
		if err != nil {
			log.Printf("Device backup retention: failed to list batch: %v", err)
			return
		}
		for _, job := range jobs {
			if ctx.Err() != nil {
				return
			}
			if err := s.backupService.DeleteBackupJob(ctx, job.ID); err != nil {
				log.Printf("Device backup retention: failed to delete job %s: %v", job.ID, err)
			} else {
				deleted++
			}
			if ctx.Err() != nil {
				return
			}
			s.retentionJobCursor = job.ID
		}
		if len(jobs) < backupRetentionPageSize {
			s.retentionJobCursor = uuid.Nil
			return
		}
	}
}
