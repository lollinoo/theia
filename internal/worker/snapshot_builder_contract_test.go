package worker

import (
	"fmt"

	"github.com/google/uuid"
	"github.com/lollinoo/theia/internal/domain"
	"github.com/lollinoo/theia/internal/ws"
	"testing"
)

func TestNormalizeInterfaceName(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"ether1", "ether1"},
		{"  Ether1  ", "ether1"},
		{"GigabitEthernet0/1", "gigabitethernet0/1"},
		{"", ""},
		{"  ", ""},
		{"VLAN100", "vlan100"},
	}

	for _, tc := range tests {
		t.Run(fmt.Sprintf("%q", tc.input), func(t *testing.T) {
			got := normalizeInterfaceName(tc.input)
			if got != tc.want {
				t.Errorf("normalizeInterfaceName(%q) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}

func TestMatchLinkID(t *testing.T) {
	devID := uuid.New()
	linkID := uuid.New()
	otherDevID := uuid.New()

	device := domain.Device{
		ID: devID,
		Interfaces: []domain.Interface{
			{DeviceID: devID, IfName: "ether1", IfDescr: "Ethernet 1", Speed: 1000000000},
		},
	}

	links := []domain.Link{
		{
			ID:             linkID,
			SourceDeviceID: devID,
			SourceIfName:   "ether1",
			TargetDeviceID: otherDevID,
			TargetIfName:   "ether2",
		},
	}

	// Match by exact name
	got := matchLinkID(device, links, "ether1")
	if got != linkID.String() {
		t.Errorf("matchLinkID for 'ether1': got %q, want %q", got, linkID.String())
	}

	// No match
	got = matchLinkID(device, links, "ether99")
	if got != "" {
		t.Errorf("matchLinkID for 'ether99': got %q, want empty", got)
	}

	// Empty interface name
	got = matchLinkID(device, links, "")
	if got != "" {
		t.Errorf("matchLinkID for empty: got %q, want empty", got)
	}
}

func TestComputeUtilization(t *testing.T) {
	devWithSpeed := domain.Device{
		Interfaces: []domain.Interface{
			{IfName: "ether1", Speed: 1_000_000_000}, // 1 Gbps
		},
	}
	devNoSpeed := domain.Device{
		Interfaces: []domain.Interface{
			{IfName: "ether1", Speed: 0},
		},
	}

	t.Run("with_speed_and_rates", func(t *testing.T) {
		metric := domain.LinkMetrics{
			TxBps: floatPtr(500_000_000), // 500 Mbps
			RxBps: floatPtr(300_000_000), // 300 Mbps
		}
		util := computeUtilization(devWithSpeed, "ether1", metric)
		if util == nil {
			t.Fatal("expected non-nil utilization")
		}
		// Utilization = max(tx, rx) / speed = 500M / 1G = 0.5
		if *util < 0.49 || *util > 0.51 {
			t.Errorf("expected utilization ~0.5, got %f", *util)
		}
	})

	t.Run("no_speed_returns_existing_utilization", func(t *testing.T) {
		// When interfaceSpeed returns 0, computeUtilization returns metric.Utilization
		existing := 0.75
		metric := domain.LinkMetrics{
			TxBps:       floatPtr(500_000_000),
			Utilization: &existing,
		}
		util := computeUtilization(devNoSpeed, "ether1", metric)
		if util == nil {
			t.Fatal("expected existing utilization to be returned")
		}
		if *util != 0.75 {
			t.Errorf("expected 0.75, got %f", *util)
		}
	})

	t.Run("nil_rates_returns_nil", func(t *testing.T) {
		metric := domain.LinkMetrics{
			TxBps: nil,
			RxBps: nil,
		}
		util := computeUtilization(devWithSpeed, "ether1", metric)
		if util != nil {
			t.Errorf("expected nil utilization when no rates, got %f", *util)
		}
	})
}

func TestComputeSectionHash_Deterministic(t *testing.T) {
	// Same input must produce the same hash.
	h1 := computeSectionHash("device_id|42.5|60.0|<nil>|<nil>|2024-01-01T00:00:00Z")
	h2 := computeSectionHash("device_id|42.5|60.0|<nil>|<nil>|2024-01-01T00:00:00Z")
	if h1 != h2 {
		t.Errorf("computeSectionHash: same input produced different hashes: %d vs %d", h1, h2)
	}

	// Different input must produce a different hash.
	h3 := computeSectionHash("device_id|99.0|60.0|<nil>|<nil>|2024-01-01T00:00:00Z")
	if h1 == h3 {
		t.Errorf("computeSectionHash: different inputs produced the same hash: %d", h1)
	}

	// Empty string has a defined (non-panicking) value.
	h4 := computeSectionHash("")
	h5 := computeSectionHash("")
	if h4 != h5 {
		t.Errorf("computeSectionHash: empty string is not deterministic: %d vs %d", h4, h5)
	}
}

func TestComputeSnapshotHashes_AllSections(t *testing.T) {
	devID1 := uuid.New().String()
	devID2 := uuid.New().String()

	cpu := 42.5
	mem := 60.0
	tx := 1000.0
	rx := 500.0
	linkID := uuid.New().String()

	snapshot := &ws.SnapshotPayload{
		Devices: map[string]ws.DeviceRuntimeDTO{
			devID1: {DeviceID: devID1, OperationalStatus: "up", Reachability: "up", Health: "unknown", Freshness: "fresh", PrimaryReason: "ok", MetricsStatus: "partial", MetricsReason: "ok", AlertStatus: "normal", CPUPercent: &cpu, MemPercent: &mem, LastCollectedAt: stringPtr("2024-01-01T00:00:00Z")},
			devID2: {DeviceID: devID2, OperationalStatus: "down", Reachability: "hard_down", Health: "unknown", Freshness: "awaiting_poll", PrimaryReason: "device_unreachable", MetricsStatus: "unavailable", MetricsReason: "device_unreachable", AlertStatus: "normal", CPUPercent: &cpu, LastCollectedAt: stringPtr("2024-01-01T00:00:00Z")},
		},
		Links: map[string]ws.LinkRuntimeDTO{
			linkID: {LinkID: linkID, SourceDeviceID: devID1, TargetDeviceID: devID2, SourceIfName: "ether1", TargetIfName: "ether2", MetricsStatus: "partial", MetricsReason: "ok", TxBps: &tx, RxBps: &rx, LastCollectedAt: stringPtr("2024-01-01T00:00:00Z")},
		},
	}
	syncSnapshotCompatibility(snapshot)

	hashes := computeSnapshotHashes(snapshot)

	if hashes == nil {
		t.Fatal("computeSnapshotHashes returned nil")
	}

	// device_metrics: both devices should have entries.
	if _, ok := hashes.deviceMetrics[devID1]; !ok {
		t.Errorf("expected deviceMetrics hash for %s", devID1)
	}
	if _, ok := hashes.deviceMetrics[devID2]; !ok {
		t.Errorf("expected deviceMetrics hash for %s", devID2)
	}

	// links: devID1's link should have an entry.
	for _, link := range snapshot.Links {
		if _, ok := hashes.linkMetrics[link.LinkID]; !ok {
			t.Errorf("expected linkMetrics hash for %s", link.LinkID)
		}
	}

	// device_statuses: both devices should have entries.
	if _, ok := hashes.deviceStatuses[devID1]; !ok {
		t.Errorf("expected deviceStatuses hash for %s", devID1)
	}
	if _, ok := hashes.deviceStatuses[devID2]; !ok {
		t.Errorf("expected deviceStatuses hash for %s", devID2)
	}

}

func TestBuildDelta_NoChanges_ReturnsNil(t *testing.T) {
	devID := uuid.New().String()
	cpu := 42.5
	snapshot := &ws.SnapshotPayload{
		Devices: map[string]ws.DeviceRuntimeDTO{devID: {DeviceID: devID, OperationalStatus: "up", Reachability: "up", Health: "unknown", Freshness: "fresh", PrimaryReason: "ok", MetricsStatus: "partial", MetricsReason: "ok", AlertStatus: "normal", CPUPercent: &cpu, LastCollectedAt: stringPtr("2024-01-01T00:00:00Z")}},
		Links:   map[string]ws.LinkRuntimeDTO{},
	}
	syncSnapshotCompatibility(snapshot)

	hashes := computeSnapshotHashes(snapshot)
	// Identical prev and current hashes → no changes.
	delta := buildDelta(snapshot, hashes, hashes)

	if delta != nil {
		t.Errorf("buildDelta: expected nil delta when nothing changed, got non-nil")
	}
}

func TestBuildDelta_OneDeviceMetricsChanged(t *testing.T) {
	devID1 := uuid.New().String()
	devID2 := uuid.New().String()
	cpu1 := 42.5
	cpu2 := 15.0

	snapshot := &ws.SnapshotPayload{
		Devices: map[string]ws.DeviceRuntimeDTO{
			devID1: {DeviceID: devID1, OperationalStatus: "up", Reachability: "up", Health: "unknown", Freshness: "fresh", PrimaryReason: "ok", MetricsStatus: "partial", MetricsReason: "ok", AlertStatus: "normal", CPUPercent: &cpu1, LastCollectedAt: stringPtr("2024-01-01T00:00:00Z")},
			devID2: {DeviceID: devID2, OperationalStatus: "up", Reachability: "up", Health: "unknown", Freshness: "fresh", PrimaryReason: "ok", MetricsStatus: "partial", MetricsReason: "ok", AlertStatus: "normal", CPUPercent: &cpu2, LastCollectedAt: stringPtr("2024-01-01T00:00:00Z")},
		},
		Links: map[string]ws.LinkRuntimeDTO{},
	}
	syncSnapshotCompatibility(snapshot)

	// Build "previous" hashes from the snapshot.
	prevHashes := computeSnapshotHashes(snapshot)

	// Simulate devID1 metrics changing.
	cpu1Changed := 99.0
	snapshotNew := &ws.SnapshotPayload{
		Devices: map[string]ws.DeviceRuntimeDTO{
			devID1: {DeviceID: devID1, OperationalStatus: "up", Reachability: "up", Health: "unknown", Freshness: "fresh", PrimaryReason: "ok", MetricsStatus: "partial", MetricsReason: "ok", AlertStatus: "normal", CPUPercent: &cpu1Changed, LastCollectedAt: stringPtr("2024-01-01T00:01:00Z")},
			devID2: {DeviceID: devID2, OperationalStatus: "up", Reachability: "up", Health: "unknown", Freshness: "fresh", PrimaryReason: "ok", MetricsStatus: "partial", MetricsReason: "ok", AlertStatus: "normal", CPUPercent: &cpu2, LastCollectedAt: stringPtr("2024-01-01T00:00:00Z")},
		},
		Links: map[string]ws.LinkRuntimeDTO{},
	}
	syncSnapshotCompatibility(snapshotNew)
	currentHashes := computeSnapshotHashes(snapshotNew)

	delta := buildDelta(snapshotNew, currentHashes, prevHashes)

	if delta == nil {
		t.Fatal("buildDelta: expected non-nil delta when one device metrics changed")
	}

	// Delta should contain only devID1 in devices.
	if _, ok := delta.Devices[devID1]; !ok {
		t.Errorf("expected devID1 in delta devices")
	}
	if _, ok := delta.Devices[devID2]; ok {
		t.Errorf("expected devID2 NOT in delta devices (unchanged)")
	}

	// Other sections should be empty/nil (unchanged).
	if len(delta.Links) != 0 {
		t.Errorf("expected empty delta links, got %d entries", len(delta.Links))
	}
}

func TestBuildDelta_MixedChanges(t *testing.T) {
	devID1 := uuid.New().String()
	devID2 := uuid.New().String()
	devID3 := uuid.New().String()
	cpu := 50.0
	tx := 1000.0

	linkID := uuid.New().String()
	snapshotPrev := &ws.SnapshotPayload{
		Devices: map[string]ws.DeviceRuntimeDTO{
			devID1: {DeviceID: devID1, OperationalStatus: "up", Reachability: "up", Health: "unknown", Freshness: "fresh", PrimaryReason: "ok", MetricsStatus: "partial", MetricsReason: "ok", AlertStatus: "normal", CPUPercent: &cpu, LastCollectedAt: stringPtr("2024-01-01T00:00:00Z")},
			devID2: {DeviceID: devID2, OperationalStatus: "up", Reachability: "up", Health: "unknown", Freshness: "fresh", PrimaryReason: "ok", MetricsStatus: "partial", MetricsReason: "ok", AlertStatus: "normal", CPUPercent: &cpu, LastCollectedAt: stringPtr("2024-01-01T00:00:00Z")},
			devID3: {DeviceID: devID3, OperationalStatus: "up", Reachability: "up", Health: "unknown", Freshness: "fresh", PrimaryReason: "ok", MetricsStatus: "partial", MetricsReason: "ok", AlertStatus: "normal", CPUPercent: &cpu, LastCollectedAt: stringPtr("2024-01-01T00:00:00Z")},
		},
		Links: map[string]ws.LinkRuntimeDTO{
			linkID: {LinkID: linkID, SourceDeviceID: devID1, TargetDeviceID: devID3, SourceIfName: "ether1", TargetIfName: "ether2", MetricsStatus: "partial", MetricsReason: "ok", TxBps: &tx, LastCollectedAt: stringPtr("2024-01-01T00:00:00Z")},
		},
	}
	syncSnapshotCompatibility(snapshotPrev)
	prevHashes := computeSnapshotHashes(snapshotPrev)

	// devID1 and devID2 metrics changed; devID3 status changed; alerts unchanged.
	cpu1New := 80.0
	cpu2New := 90.0
	txNew := 2000.0

	snapshotNew := &ws.SnapshotPayload{
		Devices: map[string]ws.DeviceRuntimeDTO{
			devID1: {DeviceID: devID1, OperationalStatus: "up", Reachability: "up", Health: "unknown", Freshness: "fresh", PrimaryReason: "ok", MetricsStatus: "partial", MetricsReason: "ok", AlertStatus: "normal", CPUPercent: &cpu1New, LastCollectedAt: stringPtr("2024-01-01T00:01:00Z")},
			devID2: {DeviceID: devID2, OperationalStatus: "up", Reachability: "up", Health: "unknown", Freshness: "fresh", PrimaryReason: "ok", MetricsStatus: "partial", MetricsReason: "ok", AlertStatus: "normal", CPUPercent: &cpu2New, LastCollectedAt: stringPtr("2024-01-01T00:01:00Z")},
			devID3: {DeviceID: devID3, OperationalStatus: "down", Reachability: "hard_down", Health: "unknown", Freshness: "fresh", PrimaryReason: "device_unreachable", MetricsStatus: "unavailable", MetricsReason: "device_unreachable", AlertStatus: "normal", CPUPercent: &cpu, LastCollectedAt: stringPtr("2024-01-01T00:00:00Z")},
		},
		Links: map[string]ws.LinkRuntimeDTO{
			linkID: {LinkID: linkID, SourceDeviceID: devID1, TargetDeviceID: devID3, SourceIfName: "ether1", TargetIfName: "ether2", MetricsStatus: "partial", MetricsReason: "ok", TxBps: &txNew, LastCollectedAt: stringPtr("2024-01-01T00:01:00Z")},
		},
	}
	syncSnapshotCompatibility(snapshotNew)
	currentHashes := computeSnapshotHashes(snapshotNew)

	delta := buildDelta(snapshotNew, currentHashes, prevHashes)

	if delta == nil {
		t.Fatal("buildDelta: expected non-nil delta for mixed changes")
	}

	// devices: devID1, devID2, and devID3 changed.
	if _, ok := delta.Devices[devID1]; !ok {
		t.Errorf("expected devID1 in delta devices")
	}
	if _, ok := delta.Devices[devID2]; !ok {
		t.Errorf("expected devID2 in delta devices")
	}
	if _, ok := delta.Devices[devID3]; !ok {
		t.Errorf("expected devID3 in delta devices due to status change")
	}

	// links: devID1 link changed.
	if _, ok := delta.Links[linkID]; !ok {
		t.Errorf("expected %s in delta links", linkID)
	}

}
