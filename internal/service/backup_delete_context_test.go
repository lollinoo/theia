package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/lollinoo/theia/internal/domain"
)

type contextualDeleteJobs struct {
	domain.BackupJobRepository
	get    func(context.Context, uuid.UUID) (*domain.BackupJob, error)
	delete func(context.Context, uuid.UUID) error
}

func (r *contextualDeleteJobs) GetByIDContext(ctx context.Context, id uuid.UUID) (*domain.BackupJob, error) {
	return r.get(ctx, id)
}
func (r *contextualDeleteJobs) DeleteContext(ctx context.Context, id uuid.UUID) error {
	return r.delete(ctx, id)
}

type contextualDeleteFiles struct {
	domain.BackupFileRepository
	get    func(context.Context, uuid.UUID) ([]domain.BackupFile, error)
	delete func(context.Context, uuid.UUID) error
}

func (r *contextualDeleteFiles) GetByJobIDContext(ctx context.Context, id uuid.UUID) ([]domain.BackupFile, error) {
	return r.get(ctx, id)
}
func (r *contextualDeleteFiles) DeleteByJobIDContext(ctx context.Context, id uuid.UUID) error {
	return r.delete(ctx, id)
}

type contextualDeleteBulkRuns struct {
	domain.BulkBackupRunRepository
	check func(context.Context, uuid.UUID) (bool, error)
}

func (r *contextualDeleteBulkRuns) BackupJobReferencedByActiveRunContext(ctx context.Context, id uuid.UUID) (bool, error) {
	return r.check(ctx, id)
}

func TestDeleteBackupJobDatabaseOperationsRespectDeadline(t *testing.T) {
	for _, stage := range []string{"job", "references", "files", "delete files", "delete job"} {
		t.Run(stage, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
			defer cancel()
			step := func(received context.Context, name string) error {
				if received != ctx {
					t.Fatal("repository operation lost caller context")
				}
				if received.Err() != nil {
					t.Fatal("started a repository operation after cancellation")
				}
				if name == stage {
					<-received.Done()
					return received.Err()
				}
				return nil
			}
			id := uuid.New()
			jobs := &contextualDeleteJobs{
				get: func(ctx context.Context, _ uuid.UUID) (*domain.BackupJob, error) {
					return &domain.BackupJob{ID: id, Status: domain.BackupStatusSuccess}, step(ctx, "job")
				},
				delete: func(ctx context.Context, _ uuid.UUID) error { return step(ctx, "delete job") },
			}
			files := &contextualDeleteFiles{
				get:    func(ctx context.Context, _ uuid.UUID) ([]domain.BackupFile, error) { return nil, step(ctx, "files") },
				delete: func(ctx context.Context, _ uuid.UUID) error { return step(ctx, "delete files") },
			}
			runs := &contextualDeleteBulkRuns{check: func(ctx context.Context, _ uuid.UUID) (bool, error) { return false, step(ctx, "references") }}
			svc := &BackupService{jobRepo: jobs, fileRepo: files, bulkRunRepo: runs}
			started := time.Now()
			err := svc.DeleteBackupJob(ctx, id)
			if !errors.Is(err, context.DeadlineExceeded) || time.Since(started) > time.Second {
				t.Fatalf("deletion exceeded deadline or swallowed cancellation: %v", err)
			}
		})
	}
}

func TestDeleteBackupJobStopsWhenReferenceQueryFails(t *testing.T) {
	want := errors.New("database unavailable")
	id := uuid.New()
	jobs := &contextualDeleteJobs{get: func(context.Context, uuid.UUID) (*domain.BackupJob, error) {
		return &domain.BackupJob{ID: id, Status: domain.BackupStatusSuccess}, nil
	}}
	runs := &contextualDeleteBulkRuns{check: func(context.Context, uuid.UUID) (bool, error) { return false, want }}
	// Nil file repository ensures the destructive path cannot be reached.
	err := (&BackupService{jobRepo: jobs, bulkRunRepo: runs}).DeleteBackupJob(context.Background(), id)
	if !errors.Is(err, want) {
		t.Fatalf("reference failure was ignored: %v", err)
	}
}
