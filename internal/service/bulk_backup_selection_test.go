package service

// This file exercises bulk backup selection behavior so refactors preserve the documented contract.

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/google/uuid"
	"github.com/lollinoo/theia/internal/domain"
)

func TestBulkBackupRunSelectionStopsOnLookupFailure(t *testing.T) {
	first, failing := uuid.New(), uuid.New()
	want := errors.New("database unavailable")
	runs := newMockBulkBackupRunRepo()
	svc := &BackupService{
		deviceRepo:  &backupReadDeviceRepo{failDevice: failing, err: want},
		bulkRunRepo: runs,
	}
	run, err := svc.StartBulkBackupRun(context.Background(), []uuid.UUID{first, failing}, "test")
	if run != nil || !errors.Is(err, want) {
		t.Fatalf("run=%v error=%v; want no run and lookup failure", run, err)
	}
	if len(runs.runs) != 0 {
		t.Fatal("lookup failure created a partial backup run")
	}
}

func TestBulkBackupRunSelectionSkipsOnlyMissingDevices(t *testing.T) {
	first, missing, last := uuid.New(), uuid.New(), uuid.New()
	for _, lookupError := range []error{nil, fmt.Errorf("lookup: %w", domain.ErrDeviceNotFound)} {
		svc := &BackupService{deviceRepo: &backupReadDeviceRepo{failDevice: missing, missing: true, err: lookupError}}
		devices, err := svc.bulkBackupRunDevices(context.Background(), []uuid.UUID{first, missing, first, last})
		if err != nil || len(devices) != 2 || devices[0].ID != first || devices[1].ID != last {
			t.Fatalf("devices=%v error=%v; want ordered, deduplicated existing devices", devices, err)
		}
	}
}

func TestBulkBackupDeviceNameUsesConfiguredFallbackOrder(t *testing.T) {
	tests := []struct {
		name   string
		device domain.Device
		want   string
	}{
		{
			name: "display name",
			device: domain.Device{
				Tags:    map[string]string{"display_name": "edge-a"},
				SysName: "sys-a",
				IP:      "10.0.0.1",
			},
			want: "edge-a",
		},
		{
			name:   "sys name",
			device: domain.Device{Tags: map[string]string{}, SysName: "sys-a", IP: "10.0.0.1"},
			want:   "sys-a",
		},
		{
			name:   "ip",
			device: domain.Device{Tags: map[string]string{}, IP: "10.0.0.1"},
			want:   "10.0.0.1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := bulkBackupDeviceName(tt.device); got != tt.want {
				t.Fatalf("bulkBackupDeviceName = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestDedupeUUIDsPreservesFirstOccurrenceOrder(t *testing.T) {
	first := uuid.New()
	second := uuid.New()
	got := dedupeUUIDs([]uuid.UUID{first, second, first, second})

	if len(got) != 2 {
		t.Fatalf("len(dedupeUUIDs) = %d, want 2", len(got))
	}
	if got[0] != first || got[1] != second {
		t.Fatalf("dedupeUUIDs order = %v, want [%s %s]", got, first, second)
	}
}
