package postgres

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/lollinoo/theia/internal/domain"
)

func TestBackupBatchReadsKeepLatestFilesAndPositiveTotals(t *testing.T) {
	db := setupTestDB(t)
	devices := NewDeviceRepo(db, nil, nil)
	jobs := NewBackupJobRepo(db)
	files := NewBackupFileRepo(db)
	ctx := context.Background()
	did := uuid.New()
	if err := devices.Create(&domain.Device{ID: did, IP: "192.0.2.90", Hostname: "router", SysName: "sys-router", Tags: map[string]string{"display_name": "Display"}}); err != nil {
		t.Fatal(err)
	}
	older := domain.BackupJob{ID: uuid.New(), DeviceID: did, Status: domain.BackupStatusSuccess, CreatedAt: time.Now().Add(-time.Hour)}
	latest := older
	latest.ID = uuid.New()
	latest.CreatedAt = time.Now().Add(-time.Minute)
	pending := latest
	pending.ID = uuid.New()
	pending.Status = domain.BackupStatusPending
	pending.CreatedAt = time.Now()
	for _, job := range []domain.BackupJob{older, latest, pending} {
		if err := jobs.Create(&job); err != nil {
			t.Fatal(err)
		}
	}
	for _, size := range []int{7, 11, -1} {
		if err := files.Create(&domain.BackupFile{ID: uuid.New(), JobID: latest.ID, FileType: "config", FileName: "file", FilePath: "/backup/file", SizeBytes: size}); err != nil {
			t.Fatal(err)
		}
	}
	gotJobs, err := jobs.GetLatestByDeviceIDsContext(ctx, []uuid.UUID{did, uuid.New(), did})
	if err != nil || len(gotJobs) != 1 || gotJobs[did].ID != latest.ID {
		t.Fatalf("latest=%#v error=%v", gotJobs, err)
	}
	ids := []uuid.UUID{older.ID, latest.ID, uuid.New()}
	for i := 0; i < 300; i++ {
		ids = append(ids, latest.ID)
	}
	gotFiles, err := files.GetByJobIDsContext(ctx, ids)
	if err != nil || len(gotFiles[latest.ID]) != 3 || gotFiles[older.ID] != nil {
		t.Fatalf("files=%#v error=%v", gotFiles, err)
	}
	totals, err := files.GetTotalsByJobIDsContext(ctx, ids)
	if err != nil || totals[latest.ID].FileCount != 3 || totals[latest.ID].ByteCount != 18 || totals[older.ID].FileCount != 0 {
		t.Fatalf("totals=%#v error=%v", totals, err)
	}
	gotDevices, err := devices.GetBackupDownloadDevicesContext(ctx, []uuid.UUID{did, uuid.New(), did})
	if err != nil || len(gotDevices) != 1 || gotDevices[0].Tags["display_name"] != "Display" || gotDevices[0].SysName != "sys-router" || len(gotDevices[0].Interfaces) != 0 {
		t.Fatalf("download identity=%#v error=%v", gotDevices, err)
	}
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	if _, err := files.GetByJobIDsContext(cancelled, []uuid.UUID{latest.ID}); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled batch=%v", err)
	}
}

func TestBackupRetentionCandidatesKeepNewestPerDeviceAndPage(t *testing.T) {
	db := setupTestDB(t)
	devices := NewDeviceRepo(db, nil, nil)
	jobs := NewBackupJobRepo(db)
	ctx := context.Background()
	obsolete := make(map[uuid.UUID]bool)
	for _, address := range []string{"192.0.2.91", "192.0.2.92"} {
		did := uuid.New()
		if err := devices.Create(&domain.Device{ID: did, IP: address, Hostname: address}); err != nil {
			t.Fatal(err)
		}
		for i := 0; i < 4; i++ {
			job := domain.BackupJob{ID: uuid.New(), DeviceID: did, Status: domain.BackupStatusSuccess, CreatedAt: time.Now().Add(time.Duration(i-4) * time.Hour)}
			if err := jobs.Create(&job); err != nil {
				t.Fatal(err)
			}
			if i < 2 {
				obsolete[job.ID] = true
			}
		}
		job := domain.BackupJob{ID: uuid.New(), DeviceID: did, Status: domain.BackupStatusFailed}
		if err := jobs.Create(&job); err != nil {
			t.Fatal(err)
		}
	}
	after := uuid.Nil
	seen := make(map[uuid.UUID]bool)
	for i := 0; i < 5; i++ {
		page, err := jobs.ListRetentionCandidatesContext(ctx, 2, after, 1)
		if err != nil {
			t.Fatal(err)
		}
		if len(page) == 0 {
			break
		}
		if !obsolete[page[0].ID] || seen[page[0].ID] {
			t.Fatalf("wrong retention candidate=%#v", page)
		}
		seen[page[0].ID] = true
		after = page[0].ID
	}
	if len(seen) != 4 {
		t.Fatalf("obsolete jobs selected=%d, want 4", len(seen))
	}
	if _, err := jobs.ListRetentionCandidatesContext(ctx, 2, uuid.Nil, 101); err == nil {
		t.Fatal("unbounded retention page accepted")
	}
}
