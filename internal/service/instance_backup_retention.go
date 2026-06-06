package service

// This file defines instance backup retention backup and restore service behavior, including filesystem safety and cleanup expectations.

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// InstanceBackupRetentionResult summarizes an instance-backup retention sweep.
type InstanceBackupRetentionResult struct {
	SuccessfulDeleted          int
	SuccessfulDeleteCandidates int
	FailedDeleted              int
	TimedOut                   bool
	DeleteFailures             []InstanceBackupRetentionDeleteFailure
	FailedCleanupError         error
}

// InstanceBackupRetentionDeleteFailure records a successful-backup delete failure.
type InstanceBackupRetentionDeleteFailure struct {
	ID  uuid.UUID
	Err error
}

// CleanupRetention prunes old successful instance backups and old failed records.
func (s *InstanceBackupService) CleanupRetention(ctx context.Context, retentionCount int, failedCutoff time.Time) (InstanceBackupRetentionResult, error) {
	var result InstanceBackupRetentionResult
	if retentionCount < 1 {
		retentionCount = 1
	}

	successful, err := s.repo.ListSuccessfulOldest()
	if err != nil {
		return result, fmt.Errorf("listing successful backups: %w", err)
	}

	if len(successful) > retentionCount {
		toDelete := successful[:len(successful)-retentionCount]
		result.SuccessfulDeleteCandidates = len(toDelete)
		for _, backup := range toDelete {
			select {
			case <-ctx.Done():
				result.TimedOut = true
			default:
			}
			if result.TimedOut {
				break
			}
			if err := s.Delete(ctx, backup.ID); err != nil {
				result.DeleteFailures = append(result.DeleteFailures, InstanceBackupRetentionDeleteFailure{
					ID:  backup.ID,
					Err: err,
				})
				continue
			}
			result.SuccessfulDeleted++
		}
	}

	failedDeleted, err := s.repo.DeleteFailedOlderThan(failedCutoff)
	result.FailedDeleted = failedDeleted
	if err != nil {
		result.FailedCleanupError = err
	}
	return result, nil
}
