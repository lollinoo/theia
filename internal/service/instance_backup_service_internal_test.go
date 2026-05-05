package service

import (
	"archive/tar"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/lollinoo/theia/internal/domain"
)

type instanceBackupCancelTestRepo struct {
	backups map[uuid.UUID]*domain.InstanceBackup
}

func newInstanceBackupCancelTestRepo(backups ...*domain.InstanceBackup) *instanceBackupCancelTestRepo {
	repo := &instanceBackupCancelTestRepo{backups: make(map[uuid.UUID]*domain.InstanceBackup)}
	for _, backup := range backups {
		if backup.ID == uuid.Nil {
			backup.ID = uuid.New()
		}
		copyBackup := *backup
		repo.backups[backup.ID] = &copyBackup
	}
	return repo
}

func (r *instanceBackupCancelTestRepo) Create(backup *domain.InstanceBackup) error {
	if backup.ID == uuid.Nil {
		backup.ID = uuid.New()
	}
	copyBackup := *backup
	r.backups[backup.ID] = &copyBackup
	return nil
}

func (r *instanceBackupCancelTestRepo) GetByID(id uuid.UUID) (*domain.InstanceBackup, error) {
	backup := r.backups[id]
	if backup == nil {
		return nil, nil
	}
	copyBackup := *backup
	return &copyBackup, nil
}

func (r *instanceBackupCancelTestRepo) List() ([]domain.InstanceBackup, error) {
	backups := make([]domain.InstanceBackup, 0, len(r.backups))
	for _, backup := range r.backups {
		backups = append(backups, *backup)
	}
	return backups, nil
}

func (r *instanceBackupCancelTestRepo) Update(backup *domain.InstanceBackup) error {
	copyBackup := *backup
	r.backups[backup.ID] = &copyBackup
	return nil
}

func (r *instanceBackupCancelTestRepo) Delete(id uuid.UUID) error {
	delete(r.backups, id)
	return nil
}

func (r *instanceBackupCancelTestRepo) ListSuccessfulOldest() ([]domain.InstanceBackup, error) {
	backups := make([]domain.InstanceBackup, 0)
	for _, backup := range r.backups {
		if backup.Status == domain.InstanceBackupStatusSuccess {
			backups = append(backups, *backup)
		}
	}
	return backups, nil
}

func (r *instanceBackupCancelTestRepo) DeleteFailedOlderThan(cutoff time.Time) (int, error) {
	deleted := 0
	for id, backup := range r.backups {
		if backup.Status == domain.InstanceBackupStatusFailed && backup.CreatedAt.Before(cutoff) {
			delete(r.backups, id)
			deleted++
		}
	}
	return deleted, nil
}

func TestInstanceBackupCancelActiveOperationKeepsCapacityUntilWorkerStops(t *testing.T) {
	backup := &domain.InstanceBackup{
		ID:     uuid.New(),
		Status: domain.InstanceBackupStatusRunning,
	}
	repo := newInstanceBackupCancelTestRepo(backup)
	svc := NewInstanceBackupService(nil, repo, nil, t.TempDir(), "", "", "", "", nil)

	cancelled := false
	svc.beginInstanceBackupOperation(backup.ID, func() { cancelled = true }, domain.InstanceBackupProgress{
		Phase:   "archiving",
		Message: "Archiving backup",
	})

	got, err := svc.Cancel(context.Background(), backup.ID)
	if err != nil {
		t.Fatalf("Cancel: %v", err)
	}
	if !cancelled {
		t.Fatal("operation cancel function was not called")
	}
	if got.Status != domain.InstanceBackupStatusRunning {
		t.Fatalf("returned status = %q, want running until worker stops", got.Status)
	}
	persisted, err := repo.GetByID(backup.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if persisted.Status != domain.InstanceBackupStatusRunning {
		t.Fatalf("persisted status = %q, want running until worker stops", persisted.Status)
	}
	progress, ok := svc.GetProgress(backup.ID)
	if !ok {
		t.Fatal("expected cancellation progress for active operation")
	}
	if progress.Phase != "cancelling" {
		t.Fatalf("progress phase = %q, want cancelling", progress.Phase)
	}

	_, _, err = svc.prepareInstanceBackup(domain.InstanceBackupTriggerManual)
	if !errors.Is(err, ErrInstanceBackupAlreadyRunning) {
		t.Fatalf("prepareInstanceBackup error = %v, want ErrInstanceBackupAlreadyRunning", err)
	}
}

func TestInstanceBackupCancelPreventsLateSuccessTransition(t *testing.T) {
	backup := &domain.InstanceBackup{
		ID:     uuid.New(),
		Status: domain.InstanceBackupStatusRunning,
	}
	repo := newInstanceBackupCancelTestRepo(backup)
	svc := NewInstanceBackupService(nil, repo, nil, t.TempDir(), "", "", "", "", nil)
	svc.beginInstanceBackupOperation(backup.ID, func() {}, domain.InstanceBackupProgress{})

	if _, err := svc.Cancel(context.Background(), backup.ID); err != nil {
		t.Fatalf("Cancel: %v", err)
	}
	backup.Status = domain.InstanceBackupStatusSuccess
	backup.FilePath = filepath.Join(t.TempDir(), "archive.tar.gz")

	err := svc.completeInstanceBackupSuccess(backup, 0, false)

	if !errors.Is(err, context.Canceled) {
		t.Fatalf("completeInstanceBackupSuccess error = %v, want context.Canceled", err)
	}
	persisted, err := repo.GetByID(backup.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if persisted.Status == domain.InstanceBackupStatusSuccess {
		t.Fatal("cancelled operation must not persist success")
	}
}

func TestInstanceBackupCancelAfterSuccessTransitionIsRejected(t *testing.T) {
	backup := &domain.InstanceBackup{
		ID:     uuid.New(),
		Status: domain.InstanceBackupStatusRunning,
	}
	repo := newInstanceBackupCancelTestRepo(backup)
	svc := NewInstanceBackupService(nil, repo, nil, t.TempDir(), "", "", "", "", nil)
	svc.beginInstanceBackupOperation(backup.ID, func() {}, domain.InstanceBackupProgress{})
	backup.Status = domain.InstanceBackupStatusSuccess
	backup.FilePath = filepath.Join(t.TempDir(), "archive.tar.gz")

	if err := svc.completeInstanceBackupSuccess(backup, 0, false); err != nil {
		t.Fatalf("completeInstanceBackupSuccess: %v", err)
	}
	_, err := svc.Cancel(context.Background(), backup.ID)

	if !errors.Is(err, ErrInstanceBackupNotRunning) {
		t.Fatalf("Cancel error = %v, want ErrInstanceBackupNotRunning", err)
	}
}

func TestAddCollectedFileToTarContextRejectsGrowthAfterQuotaCollection(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "backup.rsc")
	if err := os.WriteFile(path, []byte("grew"), 0600); err != nil {
		t.Fatalf("writing source file: %v", err)
	}

	tw := tar.NewWriter(io.Discard)
	_, err := addCollectedFileToTarContext(context.Background(), tw, archiveSourceFile{
		archiveName: "backups/device/backup.rsc",
		diskPath:    path,
		sizeBytes:   1,
	}, DefaultBackupArchiveLimits)

	if err == nil {
		t.Fatal("expected quota error for source file growth")
	}
	var limitErr *RestoreLimitError
	if !errors.As(err, &limitErr) {
		t.Fatalf("error = %v, want RestoreLimitError", err)
	}
}
