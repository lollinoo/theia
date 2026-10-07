package service

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/google/uuid"
	"github.com/lollinoo/theia/internal/domain"
)

func TestDeleteBackupJobKeepsFilesWhenMetadataDeletionFails(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "backup.rsc")
	if err := os.WriteFile(path, []byte("backup"), 0600); err != nil {
		t.Fatal(err)
	}
	id := uuid.New()
	want := errors.New("database deletion failed")
	jobs := &contextualDeleteJobs{
		get: func(context.Context, uuid.UUID) (*domain.BackupJob, error) {
			return &domain.BackupJob{ID: id, Status: domain.BackupStatusSuccess}, nil
		},
		delete: func(context.Context, uuid.UUID) error { return want },
	}
	files := &contextualDeleteFiles{get: func(context.Context, uuid.UUID) ([]domain.BackupFile, error) {
		return []domain.BackupFile{{JobID: id, FilePath: path}}, nil
	}}
	svc := &BackupService{jobRepo: jobs, fileRepo: files, backupDir: root}
	if err := svc.DeleteBackupJob(context.Background(), id); !errors.Is(err, want) {
		t.Fatalf("delete error=%v, want database failure", err)
	}
	if data, err := os.ReadFile(path); err != nil || string(data) != "backup" {
		t.Fatalf("failed metadata deletion changed disk file: data=%q error=%v", data, err)
	}
}

func TestDeleteBackupJobValidatesAllPathsBeforeDeleting(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "backup.rsc")
	if err := os.WriteFile(path, []byte("backup"), 0600); err != nil {
		t.Fatal(err)
	}
	id := uuid.New()
	jobs := &contextualDeleteJobs{get: func(context.Context, uuid.UUID) (*domain.BackupJob, error) {
		return &domain.BackupJob{ID: id, Status: domain.BackupStatusSuccess}, nil
	}}
	files := &contextualDeleteFiles{get: func(context.Context, uuid.UUID) ([]domain.BackupFile, error) {
		return []domain.BackupFile{{FilePath: path}, {FilePath: filepath.Join(t.TempDir(), "outside.rsc")}}, nil
	}}
	svc := &BackupService{jobRepo: jobs, fileRepo: files, backupDir: root}
	if err := svc.DeleteBackupJob(context.Background(), id); !IsBulkPathError(err) {
		t.Fatalf("delete error=%v, want unsafe-path rejection", err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("earlier valid file was removed before validation finished: %v", err)
	}
}

func TestDeleteBackupJobQueuesDiskCleanupAfterMetadataCommit(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "backup.rsc")
	if err := os.WriteFile(path, []byte("backup"), 0600); err != nil {
		t.Fatal(err)
	}
	job := &domain.BackupJob{ID: uuid.New(), DeviceID: uuid.New(), Status: domain.BackupStatusSuccess}
	storedJobs := newMockBackupJobRepo()
	if err := storedJobs.Create(job); err != nil {
		t.Fatal(err)
	}
	files := &cleanupFileRepo{mockBackupFileRepo: newMockBackupFileRepo()}
	file := &domain.BackupFile{ID: uuid.New(), JobID: job.ID, FilePath: path}
	if err := files.Create(file); err != nil {
		t.Fatal(err)
	}
	jobs := &contextualDeleteJobs{
		BackupJobRepository: storedJobs,
		get:                 func(_ context.Context, id uuid.UUID) (*domain.BackupJob, error) { return storedJobs.GetByID(id) },
		delete: func(_ context.Context, id uuid.UUID) error {
			// Simulate the database cascade and cleanup trigger on a successful commit.
			files.pending = append(files.pending, *file)
			if err := files.DeleteByJobID(id); err != nil {
				return err
			}
			return storedJobs.Delete(id)
		},
	}
	svc := NewBackupService(jobs, files, nil, nil, nil, nil, nil, nil, root, nil)
	if err := svc.DeleteBackupJob(context.Background(), job.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := storedJobs.GetByID(job.ID); err == nil {
		t.Fatal("job metadata survived deletion")
	}
	if remaining, err := files.GetByJobID(job.ID); err != nil || len(remaining) != 0 || len(files.pending) != 1 {
		t.Fatalf("file metadata=%v pending=%v error=%v", remaining, files.pending, err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("disk cleanup ran before the worker: %v", err)
	}
	if count, err := svc.CleanupDeletedBackupFiles(context.Background()); err != nil || count != 1 || len(files.pending) != 0 {
		t.Fatalf("cleanup count=%d pending=%v error=%v", count, files.pending, err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("worker did not remove the file: %v", err)
	}
}
