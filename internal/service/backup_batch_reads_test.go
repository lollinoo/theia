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

type batchReadJobs struct {
	domain.BackupJobRepository
	jobs  []domain.BackupJob
	calls int
	err   error
}

func (r *batchReadJobs) GetByDeviceID(uuid.UUID) ([]domain.BackupJob, error) { return r.jobs, nil }
func (r *batchReadJobs) GetLatestByDeviceIDsContext(ctx context.Context, ids []uuid.UUID) (map[uuid.UUID]domain.BackupJob, error) {
	r.calls++
	if r.err != nil {
		return nil, r.err
	}
	out := make(map[uuid.UUID]domain.BackupJob)
	for _, id := range ids {
		for _, job := range r.jobs {
			if job.DeviceID == id {
				out[id] = job
			}
		}
	}
	return out, ctx.Err()
}

type batchReadFiles struct {
	domain.BackupFileRepository
	files                 map[uuid.UUID][]domain.BackupFile
	totals                map[uuid.UUID]domain.BackupFileTotals
	fileCalls, totalCalls int
	ids                   []uuid.UUID
	err                   error
}

func (r *batchReadFiles) GetByJobIDsContext(ctx context.Context, ids []uuid.UUID) (map[uuid.UUID][]domain.BackupFile, error) {
	r.fileCalls++
	r.ids = ids
	if r.err != nil {
		return nil, r.err
	}
	return r.files, ctx.Err()
}
func (r *batchReadFiles) GetTotalsByJobIDsContext(ctx context.Context, ids []uuid.UUID) (map[uuid.UUID]domain.BackupFileTotals, error) {
	r.totalCalls++
	r.ids = ids
	if r.err != nil {
		return nil, r.err
	}
	return r.totals, ctx.Err()
}

type batchReadDevices struct {
	domain.DeviceRepository
	devices []domain.Device
	calls   int
	err     error
}

func (r *batchReadDevices) GetBackupDownloadDevicesContext(ctx context.Context, ids []uuid.UUID) ([]domain.Device, error) {
	r.calls++
	if r.err != nil {
		return nil, r.err
	}
	return r.devices, ctx.Err()
}

func TestBackupListsAndTotalsUseBatchReads(t *testing.T) {
	first, second := uuid.New(), uuid.New()
	jobs := &batchReadJobs{jobs: []domain.BackupJob{{ID: first}, {ID: second}}}
	files := &batchReadFiles{files: map[uuid.UUID][]domain.BackupFile{first: {{ID: uuid.New(), JobID: first}}}, totals: map[uuid.UUID]domain.BackupFileTotals{first: {FileCount: 2, ByteCount: 20}}}
	svc := &BackupService{jobRepo: jobs, fileRepo: files}
	listed, err := svc.GetBackupJobs(context.Background(), uuid.New())
	if err != nil || files.fileCalls != 1 || len(listed[0].Files) != 1 || listed[1].Files != nil {
		t.Fatalf("batch list=%#v calls=%d error=%v", listed, files.fileCalls, err)
	}
	run := &domain.BulkBackupRun{Items: []domain.BulkBackupRunItem{{BackupJobID: &first}, {BackupJobID: &second}, {BackupJobID: &first}, {FileCount: 99, ByteCount: 99}}}
	run, err = svc.hydrateBulkBackupRunFileTotals(context.Background(), run)
	if err != nil || files.totalCalls != 1 || files.fileCalls != 1 || len(files.ids) != 2 || run.FileCount != 4 || run.ByteCount != 40 || run.Items[3].FileCount != 0 {
		t.Fatalf("batch totals=%#v error=%v", run, err)
	}
	files.err = errors.New("batch query failed")
	if _, err := svc.GetBackupJobs(context.Background(), uuid.New()); !errors.Is(err, files.err) {
		t.Fatalf("list query failure=%v", err)
	}
	if _, err := svc.hydrateBulkBackupRunFileTotals(context.Background(), run); !errors.Is(err, files.err) {
		t.Fatalf("total query failure=%v", err)
	}
}

func TestBulkDownloadBatchesMetadataAndRetainsFileValidation(t *testing.T) {
	root := t.TempDir()
	name := filepath.Join(root, "config.rsc")
	if err := os.WriteFile(name, []byte("abc"), 0600); err != nil {
		t.Fatal(err)
	}
	did, jobID := uuid.New(), uuid.New()
	devices := &batchReadDevices{devices: []domain.Device{{ID: did, Hostname: "router", Tags: map[string]string{"display_name": "Display Name"}}}}
	jobs := &batchReadJobs{jobs: []domain.BackupJob{{ID: jobID, DeviceID: did}}}
	files := &batchReadFiles{files: map[uuid.UUID][]domain.BackupFile{jobID: {{ID: uuid.New(), JobID: jobID, FileName: "config.rsc", FilePath: name}}}}
	svc := &BackupService{deviceRepo: devices, jobRepo: jobs, fileRepo: files, backupDir: root}
	entries, err := svc.GetBulkDownloadFiles(context.Background(), []uuid.UUID{did, uuid.New(), did})
	if err != nil || len(entries) != 1 || entries[0].DeviceDir != "Display_Name" || entries[0].SizeBytes != 3 || devices.calls != 1 || jobs.calls != 1 || files.fileCalls != 1 {
		t.Fatalf("entries=%#v error=%v calls=%d/%d/%d", entries, err, devices.calls, jobs.calls, files.fileCalls)
	}
	files.files[jobID][0].FilePath = filepath.Join(filepath.Dir(root), "outside.rsc")
	if _, err := svc.GetBulkDownloadFiles(context.Background(), []uuid.UUID{did}); err == nil {
		t.Fatal("batch metadata bypassed path validation")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := svc.GetBulkDownloadFiles(ctx, []uuid.UUID{did}); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled download=%v", err)
	}
	for _, stage := range []string{"devices", "jobs", "files"} {
		injected := errors.New("failed " + stage)
		devices.err, jobs.err, files.err = nil, nil, nil
		switch stage {
		case "devices":
			devices.err = injected
		case "jobs":
			jobs.err = injected
		case "files":
			files.err = injected
		}
		if _, err := svc.GetBulkDownloadFiles(context.Background(), []uuid.UUID{did}); !errors.Is(err, injected) {
			t.Fatalf("%s query failure=%v", stage, err)
		}
	}
}
