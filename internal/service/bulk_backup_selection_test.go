package service

import (
	"testing"

	"github.com/google/uuid"
	"github.com/lollinoo/theia/internal/domain"
)

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
