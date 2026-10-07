package service

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/google/uuid"
	"github.com/lollinoo/theia/internal/domain"
)

type backupReadJobRepo struct {
	domain.BackupJobRepository
	job        *domain.BackupJob
	err        error
	failDevice uuid.UUID
}

func (r *backupReadJobRepo) GetByDeviceID(uuid.UUID) ([]domain.BackupJob, error) {
	if r.err != nil {
		return nil, r.err
	}
	return []domain.BackupJob{*r.job}, nil
}
func (r *backupReadJobRepo) GetByID(uuid.UUID) (*domain.BackupJob, error) { return r.job, r.err }
func (r *backupReadJobRepo) GetLatestByDeviceID(id uuid.UUID) (*domain.BackupJob, error) {
	if r.failDevice == uuid.Nil || id == r.failDevice {
		return r.job, r.err
	}
	return &domain.BackupJob{ID: id, DeviceID: id}, nil
}

type backupReadFileRepo struct {
	domain.BackupFileRepository
	files   []domain.BackupFile
	err     error
	failJob uuid.UUID
	calls   int
}

func (r *backupReadFileRepo) GetByJobID(id uuid.UUID) ([]domain.BackupFile, error) {
	r.calls++
	if r.failJob == uuid.Nil || id == r.failJob {
		return r.files, r.err
	}
	return r.files, nil
}

type backupReadDeviceRepo struct {
	domain.DeviceRepository
	failDevice uuid.UUID
	err        error
	missing    bool
}

func (r *backupReadDeviceRepo) GetByID(id uuid.UUID) (*domain.Device, error) {
	if id == r.failDevice && (r.err != nil || r.missing) {
		return nil, r.err
	}
	return &domain.Device{ID: id, Hostname: "router"}, nil
}

func TestBulkDownloadPreservesMissingData(t *testing.T) {
	for _, stage := range []string{"nil device", "missing device", "no job", "no files"} {
		t.Run(stage, func(t *testing.T) {
			id := uuid.New()
			devices := &backupReadDeviceRepo{failDevice: id}
			jobs := &backupReadJobRepo{job: &domain.BackupJob{ID: id}}
			files := &backupReadFileRepo{}
			switch stage {
			case "nil device":
				devices.missing = true
			case "missing device":
				devices.err = fmt.Errorf("%w: %s", domain.ErrDeviceNotFound, id)
			case "no job":
				jobs.job = nil
			}
			entries, err := (&BackupService{deviceRepo: devices, jobRepo: jobs, fileRepo: files}).GetBulkDownloadFiles(context.Background(), []uuid.UUID{id})
			if err != nil || len(entries) != 0 {
				t.Fatalf("entries=%v, error=%v; want empty successful selection", entries, err)
			}
		})
	}
}

func TestBackupJobReadsPropagateRepositoryErrors(t *testing.T) {
	id := uuid.New()
	want := errors.New("database unavailable")
	readers := map[string]func(*BackupService) (bool, error){
		"list": func(s *BackupService) (bool, error) {
			jobs, err := s.GetBackupJobs(context.Background(), id)
			return jobs == nil, err
		},
		"single": func(s *BackupService) (bool, error) {
			job, err := s.GetBackupJob(context.Background(), id)
			return job == nil, err
		},
		"latest": func(s *BackupService) (bool, error) {
			job, err := s.GetLatestBackupJob(context.Background(), id)
			return job == nil, err
		},
	}
	for name, read := range readers {
		for _, stage := range []string{"job", "files"} {
			t.Run(name+"/"+stage, func(t *testing.T) {
				jobs := &backupReadJobRepo{job: &domain.BackupJob{ID: id}}
				files := &backupReadFileRepo{}
				if stage == "job" {
					jobs.err = want
				} else {
					files.err = want
				}
				empty, err := read(&BackupService{jobRepo: jobs, fileRepo: files})
				if !empty || !errors.Is(err, want) {
					t.Fatalf("result empty=%v, error=%v; want no result and repository error", empty, err)
				}
			})
		}
	}
}

func TestBackupJobReadsPreserveMissingJobsAndEmptyFiles(t *testing.T) {
	for _, missing := range []bool{false, true} {
		jobs := &backupReadJobRepo{job: &domain.BackupJob{ID: uuid.New()}}
		if missing {
			jobs.job = nil
		}
		files := &backupReadFileRepo{}
		svc := &BackupService{jobRepo: jobs, fileRepo: files}
		for _, read := range []func(context.Context, uuid.UUID) (*domain.BackupJob, error){svc.GetBackupJob, svc.GetLatestBackupJob} {
			job, err := read(context.Background(), uuid.New())
			if err != nil || (job == nil) != missing {
				t.Fatalf("job=%v, error=%v, missing=%v", job, err, missing)
			}
		}
		if missing && files.calls != 0 {
			t.Fatal("missing job triggered a file lookup")
		}
	}
}

func TestBulkDownloadRejectsPartialSelectionOnRepositoryError(t *testing.T) {
	first, second := uuid.New(), uuid.New()
	want := errors.New("database unavailable")
	root := t.TempDir()
	filePath := filepath.Join(root, "running.cfg")
	if err := os.WriteFile(filePath, []byte("configuration"), 0600); err != nil {
		t.Fatal(err)
	}
	for _, stage := range []string{"device", "job", "files"} {
		t.Run(stage, func(t *testing.T) {
			devices := &backupReadDeviceRepo{failDevice: second}
			jobs := &backupReadJobRepo{job: &domain.BackupJob{ID: second}, failDevice: second}
			files := &backupReadFileRepo{failJob: second, files: []domain.BackupFile{{FileName: "running.cfg", FilePath: filePath}}}
			switch stage {
			case "device":
				devices.err = want
			case "job":
				jobs.err = want
			case "files":
				files.err = want
			}
			svc := &BackupService{deviceRepo: devices, jobRepo: jobs, fileRepo: files, backupDir: root}
			entries, err := svc.GetBulkDownloadFiles(context.Background(), []uuid.UUID{first, second})
			if entries != nil || !errors.Is(err, want) {
				t.Fatalf("entries=%v, error=%v; want no partial entries and repository error", entries, err)
			}
		})
	}
}
