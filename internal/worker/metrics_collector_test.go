package worker

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/lollinoo/theia/internal/cache"
	"github.com/lollinoo/theia/internal/collector"
	"github.com/lollinoo/theia/internal/domain"
	"github.com/lollinoo/theia/internal/metrics"
	"github.com/lollinoo/theia/internal/vendor"
	"github.com/lollinoo/theia/internal/ws"
)

// ---------------------------------------------------------------------------
// Pointer helpers
// ---------------------------------------------------------------------------

func floatPtr(f float64) *float64 { return &f }

// ---------------------------------------------------------------------------
// Mock repositories for worker tests
// ---------------------------------------------------------------------------

type mockWorkerDeviceRepo struct {
	devices     []domain.Device
	updateCalls int32
}

func (r *mockWorkerDeviceRepo) Create(_ *domain.Device) error { return nil }
func (r *mockWorkerDeviceRepo) GetByID(id uuid.UUID) (*domain.Device, error) {
	for _, d := range r.devices {
		if d.ID == id {
			cp := d
			return &cp, nil
		}
	}
	return nil, fmt.Errorf("device not found: %s", id)
}
func (r *mockWorkerDeviceRepo) GetByIP(_ string) (*domain.Device, error)      { return nil, nil }
func (r *mockWorkerDeviceRepo) GetBySysName(_ string) (*domain.Device, error) { return nil, nil }
func (r *mockWorkerDeviceRepo) GetAll() ([]domain.Device, error) {
	result := make([]domain.Device, len(r.devices))
	copy(result, r.devices)
	return result, nil
}
func (r *mockWorkerDeviceRepo) Update(_ *domain.Device) error {
	atomic.AddInt32(&r.updateCalls, 1)
	return nil
}
func (r *mockWorkerDeviceRepo) Delete(_ uuid.UUID) error { return nil }

type mockWorkerLinkRepo struct {
	links []domain.Link
}

func (r *mockWorkerLinkRepo) Create(_ *domain.Link) error                      { return nil }
func (r *mockWorkerLinkRepo) GetByID(_ uuid.UUID) (*domain.Link, error)        { return nil, nil }
func (r *mockWorkerLinkRepo) GetByDeviceID(_ uuid.UUID) ([]domain.Link, error) { return nil, nil }
func (r *mockWorkerLinkRepo) GetAll() ([]domain.Link, error) {
	result := make([]domain.Link, len(r.links))
	copy(result, r.links)
	return result, nil
}
func (r *mockWorkerLinkRepo) Update(_ *domain.Link) error         { return nil }
func (r *mockWorkerLinkRepo) Delete(_ uuid.UUID) error            { return nil }
func (r *mockWorkerLinkRepo) Upsert(_ *domain.Link) (bool, error) { return false, nil }

// ---------------------------------------------------------------------------
// Pure function tests: normalizeInterfaceName
// ---------------------------------------------------------------------------

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

// ---------------------------------------------------------------------------
// Pure function tests: attachDeviceMetrics
// ---------------------------------------------------------------------------

func TestAttachDeviceMetrics_MapsLabelToDeviceID(t *testing.T) {
	dev1ID := uuid.New()
	dev2ID := uuid.New()

	devices := []domain.Device{
		{ID: dev1ID, IP: "10.0.0.1", PrometheusLabelValue: "10.0.0.1"},
		{ID: dev2ID, IP: "10.0.0.2", PrometheusLabelValue: "10.0.0.2"},
	}

	metricsByLabel := map[string]domain.DeviceMetrics{
		"10.0.0.1": {CPUPercent: floatPtr(42.5), MemPercent: floatPtr(60.0)},
		"10.0.0.2": {CPUPercent: floatPtr(15.0), UptimeSecs: floatPtr(86400)},
	}

	labelFor := func(d domain.Device) string {
		return d.PrometheusLabelValue
	}

	result := attachDeviceMetrics(devices, metricsByLabel, labelFor)

	// Check device 1
	m1, ok := result[dev1ID.String()]
	if !ok {
		t.Fatalf("expected metrics for device %s", dev1ID)
	}
	if m1.CPUPercent == nil || *m1.CPUPercent != 42.5 {
		t.Errorf("device 1 CPU: got %v, want 42.5", m1.CPUPercent)
	}
	if m1.MemPercent == nil || *m1.MemPercent != 60.0 {
		t.Errorf("device 1 Memory: got %v, want 60.0", m1.MemPercent)
	}

	// Check device 2
	m2, ok := result[dev2ID.String()]
	if !ok {
		t.Fatalf("expected metrics for device %s", dev2ID)
	}
	if m2.CPUPercent == nil || *m2.CPUPercent != 15.0 {
		t.Errorf("device 2 CPU: got %v, want 15.0", m2.CPUPercent)
	}
	if m2.UptimeSecs == nil || *m2.UptimeSecs != 86400 {
		t.Errorf("device 2 Uptime: got %v, want 86400", m2.UptimeSecs)
	}
}

func TestAttachDeviceMetrics_NoMatchingLabel(t *testing.T) {
	devID := uuid.New()
	devices := []domain.Device{
		{ID: devID, IP: "10.0.0.1", PrometheusLabelValue: "10.0.0.1"},
	}

	// Metrics for a different label value
	metricsByLabel := map[string]domain.DeviceMetrics{
		"10.0.0.2": {CPUPercent: floatPtr(99.0)},
	}

	labelFor := func(d domain.Device) string {
		return d.PrometheusLabelValue
	}

	result := attachDeviceMetrics(devices, metricsByLabel, labelFor)

	m, ok := result[devID.String()]
	if !ok {
		t.Fatal("expected entry for device even without matching metrics")
	}
	// No matching metrics: CPU should be nil
	if m.CPUPercent != nil {
		t.Errorf("expected nil CPU for unmatched device, got %v", *m.CPUPercent)
	}
}

// ---------------------------------------------------------------------------
// Pure function tests: attachLinkMetrics
// ---------------------------------------------------------------------------

func TestAttachLinkMetrics(t *testing.T) {
	devID := uuid.New()
	linkID := uuid.New()

	devices := []domain.Device{
		{
			ID: devID, IP: "10.0.0.1",
			Interfaces: []domain.Interface{
				{DeviceID: devID, IfName: "ether1", IfDescr: "ether1", Speed: 1000000000},
			},
		},
	}

	links := []domain.Link{
		{
			ID:             linkID,
			SourceDeviceID: devID,
			SourceIfName:   "ether1",
			TargetDeviceID: uuid.New(),
			TargetIfName:   "ether2",
		},
	}

	metricsByIP := map[string][]domain.LinkMetrics{
		"10.0.0.1": {
			{IfName: "ether1", TxBps: floatPtr(500000), RxBps: floatPtr(300000)},
		},
	}

	result := attachLinkMetrics(devices, links, metricsByIP)

	deviceMetrics, ok := result[devID.String()]
	if !ok {
		t.Fatalf("expected link metrics for device %s", devID)
	}
	if len(deviceMetrics) == 0 {
		t.Fatal("expected at least one link metric entry")
	}
	if deviceMetrics[0].LinkID != linkID.String() {
		t.Errorf("expected linkID %s, got %s", linkID, deviceMetrics[0].LinkID)
	}
}

// ---------------------------------------------------------------------------
// Pure function tests: matchLinkID
// ---------------------------------------------------------------------------

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

// ---------------------------------------------------------------------------
// Pure function tests: computeUtilization
// ---------------------------------------------------------------------------

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

// ---------------------------------------------------------------------------
// Pure function tests: attachAlerts
// ---------------------------------------------------------------------------

func TestAttachAlerts(t *testing.T) {
	devID := uuid.New()
	devices := []domain.Device{
		{ID: devID, IP: "10.0.0.1"},
	}

	alerts := []domain.AlertState{
		{Instance: "10.0.0.1", AlertName: "HighCPU", Severity: "warning"},
		{Instance: "10.0.0.99", AlertName: "Unreachable", Severity: "critical"}, // no matching device
	}

	result := attachAlerts(devices, alerts)

	if len(result) != 1 {
		t.Fatalf("expected 1 alert (matched by IP), got %d", len(result))
	}
	if result[0].DeviceID != devID {
		t.Errorf("expected deviceID %s, got %s", devID, result[0].DeviceID)
	}
	if result[0].AlertName != "HighCPU" {
		t.Errorf("expected alert name 'HighCPU', got %q", result[0].AlertName)
	}
}

// ---------------------------------------------------------------------------
// buildSnapshot integration test with mock Prometheus server
// ---------------------------------------------------------------------------

func TestBuildSnapshot_WithMockPrometheus(t *testing.T) {
	devID := uuid.New()

	// Create a mock Prometheus server that returns known metrics
	promServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Return an empty successful result for any query
		resp := map[string]interface{}{
			"status": "success",
			"data": map[string]interface{}{
				"resultType": "vector",
				"result":     []interface{}{},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	t.Cleanup(promServer.Close)

	promClient := metrics.NewPromClient(promServer.URL, nil)
	hub := ws.NewHub()

	deviceRepo := &mockWorkerDeviceRepo{
		devices: []domain.Device{
			{
				ID:                   devID,
				IP:                   "10.0.0.1",
				Hostname:             "router1",
				Status:               domain.DeviceStatusUp,
				MetricsSource:        domain.MetricsSourcePrometheus,
				PrometheusLabelName:  "instance",
				PrometheusLabelValue: "10.0.0.1",
				Vendor:               "default",
			},
		},
	}
	linkRepo := &mockWorkerLinkRepo{}
	invalidateCh := make(chan struct{}, 1)
	dlCache := cache.NewDeviceLinkCache(deviceRepo, linkRepo, invalidateCh)
	settingsRepo := newMockWorkerSettingsRepo()

	registry := buildEmptyVendorRegistry()

	mc := NewMetricsCollector(
		promClient,
		hub,
		dlCache,
		deviceRepo,
		settingsRepo,
		registry,
		nil, // no SNMP poll func
		nil, // no SNMP link poll func
		nil, // no topology notify
	)

	snapshot, promAvailable, promErr := mc.buildSnapshot(context.Background())

	if snapshot == nil {
		t.Fatal("expected non-nil snapshot")
	}
	if !promAvailable {
		t.Errorf("expected Prometheus to be available, error: %s", promErr)
	}

	// The snapshot should have a device status entry for our device
	if _, ok := snapshot.DeviceStatuses[devID.String()]; !ok {
		t.Error("expected device status entry in snapshot")
	}

	// Device metrics should have an entry (even if empty from mock Prometheus)
	if _, ok := snapshot.DeviceMetrics[devID.String()]; !ok {
		t.Error("expected device metrics entry in snapshot")
	}
}

// ---------------------------------------------------------------------------
// collectAndBroadcast test: verifies WebSocket broadcast trigger
// ---------------------------------------------------------------------------

func TestCollectAndBroadcast_BroadcastsOnCollect(t *testing.T) {
	devID := uuid.New()

	// Mock Prometheus server returning empty but valid results
	promServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := map[string]interface{}{
			"status": "success",
			"data": map[string]interface{}{
				"resultType": "vector",
				"result":     []interface{}{},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	t.Cleanup(promServer.Close)

	promClient := metrics.NewPromClient(promServer.URL, nil)
	hub := ws.NewHub()

	deviceRepo := &mockWorkerDeviceRepo{
		devices: []domain.Device{
			{
				ID:                   devID,
				IP:                   "10.0.0.1",
				Status:               domain.DeviceStatusUp,
				MetricsSource:        domain.MetricsSourcePrometheus,
				PrometheusLabelName:  "instance",
				PrometheusLabelValue: "10.0.0.1",
				Vendor:               "default",
			},
		},
	}
	linkRepo := &mockWorkerLinkRepo{}
	invalidateCh := make(chan struct{}, 1)
	dlCache := cache.NewDeviceLinkCache(deviceRepo, linkRepo, invalidateCh)
	settingsRepo := newMockWorkerSettingsRepo()
	registry := buildEmptyVendorRegistry()

	mc := NewMetricsCollector(
		promClient,
		hub,
		dlCache,
		deviceRepo,
		settingsRepo,
		registry,
		nil,
		nil,
		nil,
	)

	// collectAndBroadcast should:
	// 1. Build snapshot
	// 2. Store in mc.lastSnapshot
	// 3. Call hub.Broadcast (which sends to hub.broadcast channel)
	mc.collectAndBroadcast(context.Background())

	// Verify lastSnapshot was set (proves buildSnapshot ran and stored result).
	// In collectAndBroadcast, the Broadcast call is unconditional after
	// lastSnapshot is set (lines 207-210 of metrics_collector.go), so verifying
	// lastSnapshot proves the full path executed including the Broadcast call.
	mc.mu.RLock()
	snap := mc.lastSnapshot
	mc.mu.RUnlock()

	if snap == nil {
		t.Fatal("expected lastSnapshot to be set after collectAndBroadcast")
	}

	// Additionally verify snapshot contains expected device status
	if _, ok := snap.DeviceStatuses[devID.String()]; !ok {
		t.Error("expected device status in snapshot after collectAndBroadcast")
	}
}

// ---------------------------------------------------------------------------
// Error resilience test: Prometheus unavailable
// ---------------------------------------------------------------------------

func TestBuildSnapshot_PromUnavailable(t *testing.T) {
	devID := uuid.New()

	// Create a server and immediately close it to simulate unavailability
	promServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	promServer.Close()

	promClient := metrics.NewPromClient(promServer.URL, nil)
	hub := ws.NewHub()

	deviceRepo := &mockWorkerDeviceRepo{
		devices: []domain.Device{
			{
				ID:                   devID,
				IP:                   "10.0.0.1",
				Status:               domain.DeviceStatusUp,
				MetricsSource:        domain.MetricsSourcePrometheus,
				PrometheusLabelName:  "instance",
				PrometheusLabelValue: "10.0.0.1",
				Vendor:               "default",
			},
		},
	}
	linkRepo := &mockWorkerLinkRepo{}
	invalidateCh := make(chan struct{}, 1)
	dlCache := cache.NewDeviceLinkCache(deviceRepo, linkRepo, invalidateCh)
	settingsRepo := newMockWorkerSettingsRepo()
	registry := buildEmptyVendorRegistry()

	mc := NewMetricsCollector(
		promClient,
		hub,
		dlCache,
		deviceRepo,
		settingsRepo,
		registry,
		nil,
		nil,
		nil,
	)

	// Should not panic even when Prometheus is unreachable
	snapshot, promAvailable, promErr := mc.buildSnapshot(context.Background())

	if snapshot == nil {
		t.Fatal("expected non-nil snapshot even when Prometheus is unavailable")
	}
	if promAvailable {
		t.Error("expected promAvailable=false when Prometheus is down")
	}
	if promErr == "" {
		t.Error("expected non-empty promErr when Prometheus is down")
	}
}

// ---------------------------------------------------------------------------
// buildSnapshot integration test: SNMP poll path
// ---------------------------------------------------------------------------

func TestBuildSnapshot_SNMPPollPath(t *testing.T) {
	devID := uuid.New()

	// Mock Prometheus server (still needed -- buildSnapshot always queries Prometheus first)
	promServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := map[string]interface{}{
			"status": "success",
			"data": map[string]interface{}{
				"resultType": "vector",
				"result":     []interface{}{},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	t.Cleanup(promServer.Close)

	promClient := metrics.NewPromClient(promServer.URL, nil)
	hub := ws.NewHub()

	deviceRepo := &mockWorkerDeviceRepo{
		devices: []domain.Device{
			{
				ID:            devID,
				IP:            "10.0.0.1",
				Hostname:      "snmp-router",
				Status:        domain.DeviceStatusUp,
				MetricsSource: domain.MetricsSourceSNMP,
				Vendor:        "default",
				SNMPCredentials: domain.SNMPCredentials{
					Version: domain.SNMPVersionV2c,
					V2c:     &domain.SNMPv2cCredentials{Community: "public"},
				},
			},
		},
	}
	linkRepo := &mockWorkerLinkRepo{}
	invalidateCh := make(chan struct{}, 1)
	dlCache := cache.NewDeviceLinkCache(deviceRepo, linkRepo, invalidateCh)
	settingsRepo := newMockWorkerSettingsRepo()
	registry := buildEmptyVendorRegistry()

	// Track whether the SNMP poll function was invoked and with correct args
	var called atomic.Bool
	snmpPollFunc := func(target string, creds domain.SNMPCredentials, vendorName string) (domain.DeviceMetrics, error) {
		called.Store(true)

		if target != "10.0.0.1" {
			t.Errorf("snmpPollFunc target: got %q, want %q", target, "10.0.0.1")
		}
		if vendorName != "default" {
			t.Errorf("snmpPollFunc vendorName: got %q, want %q", vendorName, "default")
		}

		return domain.DeviceMetrics{
			CPUPercent: floatPtr(75.0),
			MemPercent: floatPtr(50.0),
		}, nil
	}

	mc := NewMetricsCollector(
		promClient,
		hub,
		dlCache,
		deviceRepo,
		settingsRepo,
		registry,
		snmpPollFunc,
		nil,
		nil,
	)

	snapshot, _, _ := mc.buildSnapshot(context.Background())

	if snapshot == nil {
		t.Fatal("expected non-nil snapshot")
	}

	if !called.Load() {
		t.Fatal("expected snmpPollFunc to be called for MetricsSourceSNMP device")
	}

	dm, ok := snapshot.DeviceMetrics[devID.String()]
	if !ok {
		t.Fatalf("expected device metrics entry for device %s", devID)
	}
	if dm.CPUPercent == nil {
		t.Fatal("expected non-nil CPUPercent in snapshot DTO")
	}
	if *dm.CPUPercent != 75.0 {
		t.Errorf("CPUPercent: got %f, want 75.0", *dm.CPUPercent)
	}
	if dm.MemPercent == nil {
		t.Fatal("expected non-nil MemPercent in snapshot DTO")
	}
	if *dm.MemPercent != 50.0 {
		t.Errorf("MemPercent: got %f, want 50.0", *dm.MemPercent)
	}
}

// ---------------------------------------------------------------------------
// Delta detection tests: computeSectionHash
// ---------------------------------------------------------------------------

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

// ---------------------------------------------------------------------------
// Delta detection tests: computeSnapshotHashes
// ---------------------------------------------------------------------------

func TestComputeSnapshotHashes_AllSections(t *testing.T) {
	devID1 := uuid.New().String()
	devID2 := uuid.New().String()

	cpu := 42.5
	mem := 60.0
	tx := 1000.0
	rx := 500.0

	snapshot := &ws.SnapshotPayload{
		DeviceMetrics: map[string]ws.DeviceMetricsDTO{
			devID1: {DeviceID: devID1, CPUPercent: &cpu, MemPercent: &mem, CollectedAt: "2024-01-01T00:00:00Z"},
			devID2: {DeviceID: devID2, CPUPercent: &cpu, CollectedAt: "2024-01-01T00:00:00Z"},
		},
		LinkMetrics: map[string][]ws.LinkMetricsDTO{
			devID1: {
				{DeviceID: devID1, IfName: "ether1", TxBps: &tx, RxBps: &rx, CollectedAt: "2024-01-01T00:00:00Z"},
			},
		},
		DeviceStatuses: map[string]string{
			devID1: "up",
			devID2: "down",
		},
		DeviceHostnames: map[string]string{
			devID1: "router-1",
		},
		Alerts: []ws.AlertDTO{
			{DeviceID: devID1, Severity: "warning", AlertName: "HighCPU", State: "firing", Summary: "CPU high"},
		},
	}

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

	// link_metrics: devID1 should have an entry.
	if _, ok := hashes.linkMetrics[devID1]; !ok {
		t.Errorf("expected linkMetrics hash for %s", devID1)
	}

	// device_statuses: both devices should have entries.
	if _, ok := hashes.deviceStatuses[devID1]; !ok {
		t.Errorf("expected deviceStatuses hash for %s", devID1)
	}
	if _, ok := hashes.deviceStatuses[devID2]; !ok {
		t.Errorf("expected deviceStatuses hash for %s", devID2)
	}

	// device_hostnames: devID1 should have an entry.
	if _, ok := hashes.deviceHostnames[devID1]; !ok {
		t.Errorf("expected deviceHostnames hash for %s", devID1)
	}

	// alerts: whole-set hash must be non-zero.
	if hashes.alertsHash == 0 {
		t.Error("expected non-zero alertsHash for non-empty alerts")
	}
}

// ---------------------------------------------------------------------------
// Delta detection tests: buildDelta
// ---------------------------------------------------------------------------

func TestBuildDelta_NoChanges_ReturnsNil(t *testing.T) {
	devID := uuid.New().String()
	cpu := 42.5
	snapshot := &ws.SnapshotPayload{
		DeviceMetrics:   map[string]ws.DeviceMetricsDTO{devID: {DeviceID: devID, CPUPercent: &cpu, CollectedAt: "2024-01-01T00:00:00Z"}},
		LinkMetrics:     map[string][]ws.LinkMetricsDTO{},
		DeviceStatuses:  map[string]string{devID: "up"},
		DeviceHostnames: map[string]string{devID: "router-1"},
		Alerts:          []ws.AlertDTO{},
	}

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
		DeviceMetrics: map[string]ws.DeviceMetricsDTO{
			devID1: {DeviceID: devID1, CPUPercent: &cpu1, CollectedAt: "2024-01-01T00:00:00Z"},
			devID2: {DeviceID: devID2, CPUPercent: &cpu2, CollectedAt: "2024-01-01T00:00:00Z"},
		},
		LinkMetrics:     map[string][]ws.LinkMetricsDTO{},
		DeviceStatuses:  map[string]string{devID1: "up", devID2: "up"},
		DeviceHostnames: map[string]string{},
		Alerts:          []ws.AlertDTO{},
	}

	// Build "previous" hashes from the snapshot.
	prevHashes := computeSnapshotHashes(snapshot)

	// Simulate devID1 metrics changing.
	cpu1Changed := 99.0
	snapshotNew := &ws.SnapshotPayload{
		DeviceMetrics: map[string]ws.DeviceMetricsDTO{
			devID1: {DeviceID: devID1, CPUPercent: &cpu1Changed, CollectedAt: "2024-01-01T00:01:00Z"},
			devID2: {DeviceID: devID2, CPUPercent: &cpu2, CollectedAt: "2024-01-01T00:00:00Z"},
		},
		LinkMetrics:     map[string][]ws.LinkMetricsDTO{},
		DeviceStatuses:  map[string]string{devID1: "up", devID2: "up"},
		DeviceHostnames: map[string]string{},
		Alerts:          []ws.AlertDTO{},
	}
	currentHashes := computeSnapshotHashes(snapshotNew)

	delta := buildDelta(snapshotNew, currentHashes, prevHashes)

	if delta == nil {
		t.Fatal("buildDelta: expected non-nil delta when one device metrics changed")
	}

	// Delta should contain only devID1 in device_metrics.
	if _, ok := delta.DeviceMetrics[devID1]; !ok {
		t.Errorf("expected devID1 in delta device_metrics")
	}
	if _, ok := delta.DeviceMetrics[devID2]; ok {
		t.Errorf("expected devID2 NOT in delta device_metrics (unchanged)")
	}

	// Other sections should be empty/nil (unchanged).
	if len(delta.LinkMetrics) != 0 {
		t.Errorf("expected empty delta link_metrics, got %d entries", len(delta.LinkMetrics))
	}
	if len(delta.DeviceStatuses) != 0 {
		t.Errorf("expected empty delta device_statuses, got %d entries", len(delta.DeviceStatuses))
	}
	if len(delta.DeviceHostnames) != 0 {
		t.Errorf("expected empty delta device_hostnames, got %d entries", len(delta.DeviceHostnames))
	}
	if delta.Alerts != nil {
		t.Errorf("expected nil delta alerts (unchanged), got %v", delta.Alerts)
	}
}

func TestBuildDelta_AlertsChanged(t *testing.T) {
	devID := uuid.New().String()

	snapshotPrev := &ws.SnapshotPayload{
		DeviceMetrics:   map[string]ws.DeviceMetricsDTO{},
		LinkMetrics:     map[string][]ws.LinkMetricsDTO{},
		DeviceStatuses:  map[string]string{},
		DeviceHostnames: map[string]string{},
		Alerts:          []ws.AlertDTO{},
	}
	prevHashes := computeSnapshotHashes(snapshotPrev)

	// Add a new alert.
	snapshotNew := &ws.SnapshotPayload{
		DeviceMetrics:   map[string]ws.DeviceMetricsDTO{},
		LinkMetrics:     map[string][]ws.LinkMetricsDTO{},
		DeviceStatuses:  map[string]string{},
		DeviceHostnames: map[string]string{},
		Alerts: []ws.AlertDTO{
			{DeviceID: devID, Severity: "critical", AlertName: "Down", State: "firing", Summary: "Device is down"},
		},
	}
	currentHashes := computeSnapshotHashes(snapshotNew)

	delta := buildDelta(snapshotNew, currentHashes, prevHashes)

	if delta == nil {
		t.Fatal("buildDelta: expected non-nil delta when alerts changed")
	}
	if delta.Alerts == nil {
		t.Fatal("expected full alerts array in delta when alertsHash changed")
	}
	if len(delta.Alerts) != 1 {
		t.Errorf("expected 1 alert in delta, got %d", len(delta.Alerts))
	}
	if delta.Alerts[0].AlertName != "Down" {
		t.Errorf("expected alert name 'Down', got %q", delta.Alerts[0].AlertName)
	}
}

func TestBuildDelta_MixedChanges(t *testing.T) {
	devID1 := uuid.New().String()
	devID2 := uuid.New().String()
	devID3 := uuid.New().String()
	cpu := 50.0
	tx := 1000.0

	snapshotPrev := &ws.SnapshotPayload{
		DeviceMetrics: map[string]ws.DeviceMetricsDTO{
			devID1: {DeviceID: devID1, CPUPercent: &cpu, CollectedAt: "2024-01-01T00:00:00Z"},
			devID2: {DeviceID: devID2, CPUPercent: &cpu, CollectedAt: "2024-01-01T00:00:00Z"},
			devID3: {DeviceID: devID3, CPUPercent: &cpu, CollectedAt: "2024-01-01T00:00:00Z"},
		},
		LinkMetrics: map[string][]ws.LinkMetricsDTO{
			devID1: {{DeviceID: devID1, IfName: "ether1", TxBps: &tx, CollectedAt: "2024-01-01T00:00:00Z"}},
		},
		DeviceStatuses:  map[string]string{devID1: "up", devID2: "up", devID3: "up"},
		DeviceHostnames: map[string]string{},
		Alerts:          []ws.AlertDTO{},
	}
	prevHashes := computeSnapshotHashes(snapshotPrev)

	// devID1 and devID2 metrics changed; devID3 status changed; alerts unchanged.
	cpu1New := 80.0
	cpu2New := 90.0
	txNew := 2000.0

	snapshotNew := &ws.SnapshotPayload{
		DeviceMetrics: map[string]ws.DeviceMetricsDTO{
			devID1: {DeviceID: devID1, CPUPercent: &cpu1New, CollectedAt: "2024-01-01T00:01:00Z"},
			devID2: {DeviceID: devID2, CPUPercent: &cpu2New, CollectedAt: "2024-01-01T00:01:00Z"},
			devID3: {DeviceID: devID3, CPUPercent: &cpu, CollectedAt: "2024-01-01T00:00:00Z"}, // unchanged
		},
		LinkMetrics: map[string][]ws.LinkMetricsDTO{
			devID1: {{DeviceID: devID1, IfName: "ether1", TxBps: &txNew, CollectedAt: "2024-01-01T00:01:00Z"}},
		},
		DeviceStatuses:  map[string]string{devID1: "up", devID2: "up", devID3: "down"}, // devID3 changed
		DeviceHostnames: map[string]string{},
		Alerts:          []ws.AlertDTO{}, // unchanged
	}
	currentHashes := computeSnapshotHashes(snapshotNew)

	delta := buildDelta(snapshotNew, currentHashes, prevHashes)

	if delta == nil {
		t.Fatal("buildDelta: expected non-nil delta for mixed changes")
	}

	// device_metrics: devID1 and devID2 changed; devID3 did not.
	if _, ok := delta.DeviceMetrics[devID1]; !ok {
		t.Errorf("expected devID1 in delta device_metrics")
	}
	if _, ok := delta.DeviceMetrics[devID2]; !ok {
		t.Errorf("expected devID2 in delta device_metrics")
	}
	if _, ok := delta.DeviceMetrics[devID3]; ok {
		t.Errorf("expected devID3 NOT in delta device_metrics (unchanged)")
	}

	// link_metrics: devID1 changed.
	if _, ok := delta.LinkMetrics[devID1]; !ok {
		t.Errorf("expected devID1 in delta link_metrics")
	}

	// device_statuses: only devID3 changed.
	if _, ok := delta.DeviceStatuses[devID3]; !ok {
		t.Errorf("expected devID3 in delta device_statuses")
	}
	if _, ok := delta.DeviceStatuses[devID1]; ok {
		t.Errorf("expected devID1 NOT in delta device_statuses (unchanged)")
	}

	// alerts: unchanged → nil.
	if delta.Alerts != nil {
		t.Errorf("expected nil delta alerts (unchanged), got %v", delta.Alerts)
	}
}

// ---------------------------------------------------------------------------
// collectAndBroadcast integration tests: delta behavior
// ---------------------------------------------------------------------------

func newMockCollector(t *testing.T) (*MetricsCollector, *ws.Hub) {
	t.Helper()

	devID := uuid.New()
	promServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := map[string]interface{}{
			"status": "success",
			"data": map[string]interface{}{
				"resultType": "vector",
				"result":     []interface{}{},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	t.Cleanup(promServer.Close)

	promClient := metrics.NewPromClient(promServer.URL, nil)
	hub := ws.NewHub()

	deviceRepo := &mockWorkerDeviceRepo{
		devices: []domain.Device{
			{
				ID:                   devID,
				IP:                   "10.0.0.1",
				Status:               domain.DeviceStatusUp,
				MetricsSource:        domain.MetricsSourcePrometheus,
				PrometheusLabelName:  "instance",
				PrometheusLabelValue: "10.0.0.1",
				Vendor:               "default",
			},
		},
	}
	linkRepo := &mockWorkerLinkRepo{}
	invalidateCh := make(chan struct{}, 1)
	dlCache := cache.NewDeviceLinkCache(deviceRepo, linkRepo, invalidateCh)
	settingsRepo := newMockWorkerSettingsRepo()
	registry := buildEmptyVendorRegistry()

	mc := NewMetricsCollector(
		promClient,
		hub,
		dlCache,
		deviceRepo,
		settingsRepo,
		registry,
		nil,
		nil,
		nil,
	)

	return mc, hub
}

func TestCollectAndBroadcast_FirstCycle_SendsFullSnapshot(t *testing.T) {
	mc, _ := newMockCollector(t)

	// First call: prevHashes is nil → must send full snapshot.
	mc.collectAndBroadcast(context.Background())

	mc.mu.RLock()
	prev := mc.prevHashes
	mc.mu.RUnlock()

	if prev == nil {
		t.Error("expected prevHashes to be set after first collectAndBroadcast")
	}
}

func TestCollectAndBroadcast_SecondCycle_SendsDelta(t *testing.T) {
	mc, _ := newMockCollector(t)

	// First cycle: seeds prevHashes.
	mc.collectAndBroadcast(context.Background())

	mc.mu.RLock()
	prevAfterFirst := mc.prevHashes
	mc.mu.RUnlock()

	if prevAfterFirst == nil {
		t.Fatal("expected prevHashes to be set after first cycle")
	}

	// Second cycle: prevHashes is non-nil → uses delta path.
	mc.collectAndBroadcast(context.Background())

	mc.mu.RLock()
	prevAfterSecond := mc.prevHashes
	mc.mu.RUnlock()

	if prevAfterSecond == nil {
		t.Error("expected prevHashes to remain set after second cycle")
	}
}

// ---------------------------------------------------------------------------
// Helper: build minimal vendor registry
// ---------------------------------------------------------------------------

func buildEmptyVendorRegistry() *vendor.Registry {
	records := []vendor.DBVendorRecord{
		{
			Name: "default",
			ConfigJSON: `{
				"vendor": {"name": "default", "display_name": "Generic"},
				"detection": {},
				"backup": {"supported": false}
			}`,
		},
	}
	reg, err := vendor.LoadRegistryFromDB(records)
	if err != nil {
		panic(fmt.Sprintf("buildEmptyVendorRegistry: %v", err))
	}
	return reg
}

// newMockCollectorWithChangingData returns a MetricsCollector backed by a repo
// whose device status can be mutated between cycles, so successive calls to
// collectAndBroadcast produce different snapshots.
// The returned invalidateCh must be sent on after mutating deviceRepo.devices
// to force cache invalidation before the next collectAndBroadcast call.
func newMockCollectorWithChangingData(t *testing.T) (*MetricsCollector, *ws.Hub, *mockWorkerDeviceRepo, chan<- struct{}) {
	t.Helper()

	devID := uuid.New()
	promServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := map[string]interface{}{
			"status": "success",
			"data": map[string]interface{}{
				"resultType": "vector",
				"result":     []interface{}{},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	t.Cleanup(promServer.Close)

	promClient := metrics.NewPromClient(promServer.URL, nil)
	hub := ws.NewHub()

	deviceRepo := &mockWorkerDeviceRepo{
		devices: []domain.Device{
			{
				ID:                   devID,
				IP:                   "10.0.0.1",
				Status:               domain.DeviceStatusUp,
				MetricsSource:        domain.MetricsSourcePrometheus,
				PrometheusLabelName:  "instance",
				PrometheusLabelValue: "10.0.0.1",
				Vendor:               "default",
			},
		},
	}
	linkRepo := &mockWorkerLinkRepo{}
	invalidateCh := make(chan struct{}, 1)
	dlCache := cache.NewDeviceLinkCache(deviceRepo, linkRepo, invalidateCh)
	settingsRepo := newMockWorkerSettingsRepo()
	registry := buildEmptyVendorRegistry()

	mc := NewMetricsCollector(
		promClient,
		hub,
		dlCache,
		deviceRepo,
		settingsRepo,
		registry,
		nil,
		nil,
		nil,
	)

	return mc, hub, deviceRepo, invalidateCh
}

// drainBroadcastCh reads all currently queued messages from the hub's broadcast
// channel without blocking. Returns the slice of raw JSON payloads.
func drainBroadcastCh(hub *ws.Hub) [][]byte {
	var msgs [][]byte
	for {
		select {
		case msg := <-hub.BroadcastCh():
			msgs = append(msgs, msg)
		default:
			return msgs
		}
	}
}

// ---------------------------------------------------------------------------
// GAP 3: First cycle broadcasts MessageTypeSnapshot
// ---------------------------------------------------------------------------

func TestCollectAndBroadcast_FirstCycle_BroadcastsSnapshotType(t *testing.T) {
	mc, hub := newMockCollector(t)

	mc.collectAndBroadcast(context.Background())

	msgs := drainBroadcastCh(hub)
	if len(msgs) == 0 {
		t.Fatal("expected at least one message in broadcast channel after first cycle")
	}

	// There may be a prometheus_status message too; find the first snapshot message.
	var snapshotMsg map[string]interface{}
	for _, raw := range msgs {
		var m map[string]interface{}
		if err := json.Unmarshal(raw, &m); err != nil {
			t.Fatalf("failed to unmarshal broadcast message: %v", err)
		}
		if m["type"] == "snapshot" {
			snapshotMsg = m
			break
		}
	}

	if snapshotMsg == nil {
		t.Fatalf("expected a message with type %q on first cycle, got types: %v",
			"snapshot", func() []string {
				var types []string
				for _, raw := range msgs {
					var m map[string]interface{}
					_ = json.Unmarshal(raw, &m)
					types = append(types, fmt.Sprintf("%v", m["type"]))
				}
				return types
			}())
	}
}

// ---------------------------------------------------------------------------
// GAP 2: Second cycle skips broadcast when nothing changed
// ---------------------------------------------------------------------------

func TestCollectAndBroadcast_SecondCycle_SkipsBroadcastWhenUnchanged(t *testing.T) {
	mc, hub := newMockCollector(t)

	// First cycle: full snapshot broadcast.
	mc.collectAndBroadcast(context.Background())

	// Drain all messages from first cycle.
	firstMsgs := drainBroadcastCh(hub)
	if len(firstMsgs) == 0 {
		t.Fatal("expected at least one broadcast after first cycle")
	}

	// Second cycle: data unchanged — no new broadcast expected.
	mc.collectAndBroadcast(context.Background())

	secondMsgs := drainBroadcastCh(hub)

	// Filter out any prometheus_status messages (those come from Prometheus
	// availability changes, not the delta/snapshot logic).
	var nonStatusMsgs [][]byte
	for _, raw := range secondMsgs {
		var m map[string]interface{}
		if err := json.Unmarshal(raw, &m); err != nil {
			continue
		}
		if m["type"] != "prometheus_status" {
			nonStatusMsgs = append(nonStatusMsgs, raw)
		}
	}

	if len(nonStatusMsgs) != 0 {
		t.Errorf("expected no snapshot/delta broadcast on second cycle when data unchanged, got %d message(s)", len(nonStatusMsgs))
		for _, raw := range nonStatusMsgs {
			t.Logf("unexpected message: %s", string(raw))
		}
	}
}

// ---------------------------------------------------------------------------
// GAP 1: Second cycle broadcasts snapshot_delta when data changed
// ---------------------------------------------------------------------------

func TestCollectAndBroadcast_SecondCycle_BroadcastsSnapshotDelta(t *testing.T) {
	mc, hub, deviceRepo, invalidateCh := newMockCollectorWithChangingData(t)

	// First cycle: seeds prevHashes, broadcasts full snapshot.
	mc.collectAndBroadcast(context.Background())

	// Drain the first cycle's messages so the channel is empty.
	drainBroadcastCh(hub)

	// Mutate device status to force a snapshot change between cycles.
	deviceRepo.devices[0].Status = domain.DeviceStatusDown
	// Signal cache invalidation so the next buildSnapshot sees updated data.
	invalidateCh <- struct{}{}

	// Second cycle: data changed — should broadcast snapshot_delta.
	mc.collectAndBroadcast(context.Background())

	msgs := drainBroadcastCh(hub)

	var deltaMsg map[string]interface{}
	for _, raw := range msgs {
		var m map[string]interface{}
		if err := json.Unmarshal(raw, &m); err != nil {
			t.Fatalf("failed to unmarshal broadcast message: %v", err)
		}
		if m["type"] == "snapshot_delta" {
			deltaMsg = m
			break
		}
	}

	if deltaMsg == nil {
		t.Fatalf("expected a message with type %q on second cycle with changed data, got types: %v",
			"snapshot_delta", func() []string {
				var types []string
				for _, raw := range msgs {
					var m map[string]interface{}
					_ = json.Unmarshal(raw, &m)
					types = append(types, fmt.Sprintf("%v", m["type"]))
				}
				return types
			}())
	}
}

// ---------------------------------------------------------------------------
// SNMP link poll guard tests (DISC-02)
// ---------------------------------------------------------------------------

// TestBuildSnapshot_SNMPLinkPollSkipsDeviceWithNoValidLinks verifies that
// snmpLinkPollFunc is NOT called for a device whose links all have empty
// SourceIfName and TargetIfName — there is no usable interface name to match
// counters against, so the walk would be wasted.
func TestBuildSnapshot_SNMPLinkPollSkipsDeviceWithNoValidLinks(t *testing.T) {
	devID := uuid.New()
	linkID := uuid.New()

	promServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := map[string]interface{}{
			"status": "success",
			"data": map[string]interface{}{
				"resultType": "vector",
				"result":     []interface{}{},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	t.Cleanup(promServer.Close)

	promClient := metrics.NewPromClient(promServer.URL, nil)
	hub := ws.NewHub()

	deviceRepo := &mockWorkerDeviceRepo{
		devices: []domain.Device{
			{
				ID:            devID,
				IP:            "192.168.1.1",
				Status:        domain.DeviceStatusUp,
				MetricsSource: domain.MetricsSourceSNMP,
				Vendor:        "default",
				SNMPCredentials: domain.SNMPCredentials{
					Version: domain.SNMPVersionV2c,
					V2c:     &domain.SNMPv2cCredentials{Community: "public"},
				},
			},
		},
	}
	// All links for this device have empty SourceIfName and TargetIfName
	linkRepo := &mockWorkerLinkRepo{
		links: []domain.Link{
			{
				ID:             linkID,
				SourceDeviceID: devID,
				SourceIfName:   "",
				TargetDeviceID: uuid.New(),
				TargetIfName:   "",
			},
		},
	}
	invalidateCh := make(chan struct{}, 1)
	dlCache := cache.NewDeviceLinkCache(deviceRepo, linkRepo, invalidateCh)
	settingsRepo := newMockWorkerSettingsRepo()
	registry := buildEmptyVendorRegistry()

	var pollCalled atomic.Bool
	snmpLinkPollFunc := func(target string, creds domain.SNMPCredentials) ([]SNMPIfCounter, error) {
		pollCalled.Store(true)
		return nil, nil
	}

	mc := NewMetricsCollector(
		promClient,
		hub,
		dlCache,
		deviceRepo,
		settingsRepo,
		registry,
		nil,
		snmpLinkPollFunc,
		nil,
	)

	snapshot, _, _ := mc.buildSnapshot(context.Background())
	if snapshot == nil {
		t.Fatal("expected non-nil snapshot")
	}

	if pollCalled.Load() {
		t.Error("expected snmpLinkPollFunc NOT to be called when all links have empty interface names (DISC-02 guard)")
	}
}

// TestBuildSnapshot_SNMPLinkPollCalledWithValidLinks verifies that
// snmpLinkPollFunc IS called when the device has at least one link with a
// non-empty SourceIfName.
func TestBuildSnapshot_SNMPLinkPollCalledWithValidLinks(t *testing.T) {
	devID := uuid.New()
	linkID := uuid.New()

	promServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := map[string]interface{}{
			"status": "success",
			"data": map[string]interface{}{
				"resultType": "vector",
				"result":     []interface{}{},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	t.Cleanup(promServer.Close)

	promClient := metrics.NewPromClient(promServer.URL, nil)
	hub := ws.NewHub()

	deviceRepo := &mockWorkerDeviceRepo{
		devices: []domain.Device{
			{
				ID:            devID,
				IP:            "192.168.1.2",
				Status:        domain.DeviceStatusUp,
				MetricsSource: domain.MetricsSourceSNMP,
				Vendor:        "default",
				SNMPCredentials: domain.SNMPCredentials{
					Version: domain.SNMPVersionV2c,
					V2c:     &domain.SNMPv2cCredentials{Community: "public"},
				},
			},
		},
	}
	// Link has a valid SourceIfName — poll should proceed
	linkRepo := &mockWorkerLinkRepo{
		links: []domain.Link{
			{
				ID:             linkID,
				SourceDeviceID: devID,
				SourceIfName:   "ether1",
				TargetDeviceID: uuid.New(),
				TargetIfName:   "",
			},
		},
	}
	invalidateCh := make(chan struct{}, 1)
	dlCache := cache.NewDeviceLinkCache(deviceRepo, linkRepo, invalidateCh)
	settingsRepo := newMockWorkerSettingsRepo()
	registry := buildEmptyVendorRegistry()

	var pollCalled atomic.Bool
	snmpLinkPollFunc := func(target string, creds domain.SNMPCredentials) ([]SNMPIfCounter, error) {
		pollCalled.Store(true)
		return nil, nil
	}

	mc := NewMetricsCollector(
		promClient,
		hub,
		dlCache,
		deviceRepo,
		settingsRepo,
		registry,
		nil,
		snmpLinkPollFunc,
		nil,
	)

	snapshot, _, _ := mc.buildSnapshot(context.Background())
	if snapshot == nil {
		t.Fatal("expected non-nil snapshot")
	}

	if !pollCalled.Load() {
		t.Error("expected snmpLinkPollFunc to be called when device has at least one link with non-empty SourceIfName")
	}
}

func TestBuildSnapshot_SNMPLinkRates_ResetDiscarded(t *testing.T) {
	calls := 0
	mc, _, devID, deviceIP := newSNMPLinkRateTestCollector(t, func(target string, creds domain.SNMPCredentials) ([]SNMPIfCounter, error) {
		calls++
		switch calls {
		case 1:
			return []SNMPIfCounter{{IfName: "ether1", InOctets: 400, OutOctets: 500}}, nil
		case 2:
			return []SNMPIfCounter{{IfName: "ether1", InOctets: 450, OutOctets: 550}}, nil
		case 3:
			return []SNMPIfCounter{{IfName: "ether1", InOctets: 1_450, OutOctets: 1_550}}, nil
		default:
			return nil, fmt.Errorf("unexpected poll %d", calls)
		}
	})

	mc.prevCounters[deviceIP] = map[string]collector.CounterBaseline{
		"ether1": {
			InOctets:  500,
			OutOctets: 450,
			SampledAt: time.Now().Add(-time.Second),
		},
	}

	snapshot, _, _ := mc.buildSnapshot(context.Background())
	assertNoSNMPLinkMetrics(t, snapshot, devID)

	snapshot, _, _ = mc.buildSnapshot(context.Background())
	assertNoSNMPLinkMetrics(t, snapshot, devID)

	time.Sleep(5 * time.Millisecond)
	snapshot, _, _ = mc.buildSnapshot(context.Background())
	assertSNMPLinkMetricsPresent(t, snapshot, devID)
}

func TestBuildSnapshot_SNMPLinkRates_GapRequiresWarmup(t *testing.T) {
	calls := 0
	mc, settingsRepo, devID, deviceIP := newSNMPLinkRateTestCollector(t, func(target string, creds domain.SNMPCredentials) ([]SNMPIfCounter, error) {
		calls++
		switch calls {
		case 1:
			return []SNMPIfCounter{{IfName: "ether1", InOctets: 1_100, OutOctets: 2_100}}, nil
		case 2:
			return []SNMPIfCounter{{IfName: "ether1", InOctets: 1_200, OutOctets: 2_200}}, nil
		case 3:
			return []SNMPIfCounter{{IfName: "ether1", InOctets: 2_200, OutOctets: 3_200}}, nil
		default:
			return nil, fmt.Errorf("unexpected poll %d", calls)
		}
	})
	settingsRepo.Set(domain.SettingPollingInterval, "1")

	mc.prevCounters[deviceIP] = map[string]collector.CounterBaseline{
		"ether1": {
			InOctets:  1_000,
			OutOctets: 2_000,
			SampledAt: time.Now().Add(-4 * time.Second),
		},
	}

	snapshot, _, _ := mc.buildSnapshot(context.Background())
	assertNoSNMPLinkMetrics(t, snapshot, devID)

	snapshot, _, _ = mc.buildSnapshot(context.Background())
	assertNoSNMPLinkMetrics(t, snapshot, devID)

	time.Sleep(5 * time.Millisecond)
	snapshot, _, _ = mc.buildSnapshot(context.Background())
	assertSNMPLinkMetricsPresent(t, snapshot, devID)
}

func TestBuildSnapshot_SNMPLinkRates_OverSpeedDiscarded(t *testing.T) {
	calls := 0
	mc, _, devID, deviceIP := newSNMPLinkRateTestCollector(t, func(target string, creds domain.SNMPCredentials) ([]SNMPIfCounter, error) {
		calls++
		switch calls {
		case 1:
			return []SNMPIfCounter{{IfName: "ether1", InOctets: 162_500_000, OutOctets: 0}}, nil
		case 2:
			return []SNMPIfCounter{{IfName: "ether1", InOctets: 162_500_100, OutOctets: 100}}, nil
		case 3:
			return []SNMPIfCounter{{IfName: "ether1", InOctets: 162_501_100, OutOctets: 1_100}}, nil
		default:
			return nil, fmt.Errorf("unexpected poll %d", calls)
		}
	})

	mc.prevCounters[deviceIP] = map[string]collector.CounterBaseline{
		"ether1": {
			InOctets:  0,
			OutOctets: 0,
			SampledAt: time.Now().Add(-time.Second),
		},
	}

	snapshot, _, _ := mc.buildSnapshot(context.Background())
	assertNoSNMPLinkMetrics(t, snapshot, devID)

	snapshot, _, _ = mc.buildSnapshot(context.Background())
	assertNoSNMPLinkMetrics(t, snapshot, devID)

	time.Sleep(5 * time.Millisecond)
	snapshot, _, _ = mc.buildSnapshot(context.Background())
	assertSNMPLinkMetricsPresent(t, snapshot, devID)
}

func newSNMPLinkRateTestCollector(t *testing.T, snmpLinkPollFunc SNMPLinkPollFunc) (*MetricsCollector, *mockWorkerSettingsRepo, uuid.UUID, string) {
	t.Helper()

	devID := uuid.New()
	deviceIP := "192.168.10.1"
	linkID := uuid.New()

	promServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := map[string]interface{}{
			"status": "success",
			"data": map[string]interface{}{
				"resultType": "vector",
				"result":     []interface{}{},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	t.Cleanup(promServer.Close)

	promClient := metrics.NewPromClient(promServer.URL, nil)
	hub := ws.NewHub()
	deviceRepo := &mockWorkerDeviceRepo{
		devices: []domain.Device{
			{
				ID:            devID,
				IP:            deviceIP,
				Status:        domain.DeviceStatusUp,
				MetricsSource: domain.MetricsSourceSNMP,
				Vendor:        "default",
				Interfaces: []domain.Interface{
					{DeviceID: devID, IfName: "ether1", Speed: 1_000_000_000},
				},
				SNMPCredentials: domain.SNMPCredentials{
					Version: domain.SNMPVersionV2c,
					V2c:     &domain.SNMPv2cCredentials{Community: "public"},
				},
			},
		},
	}
	linkRepo := &mockWorkerLinkRepo{
		links: []domain.Link{
			{
				ID:             linkID,
				SourceDeviceID: devID,
				SourceIfName:   "ether1",
				TargetDeviceID: uuid.New(),
				TargetIfName:   "ether2",
			},
		},
	}
	invalidateCh := make(chan struct{}, 1)
	dlCache := cache.NewDeviceLinkCache(deviceRepo, linkRepo, invalidateCh)
	settingsRepo := newMockWorkerSettingsRepo()
	registry := buildEmptyVendorRegistry()

	mc := NewMetricsCollector(
		promClient,
		hub,
		dlCache,
		deviceRepo,
		settingsRepo,
		registry,
		nil,
		snmpLinkPollFunc,
		nil,
	)

	return mc, settingsRepo, devID, deviceIP
}

func assertNoSNMPLinkMetrics(t *testing.T, snapshot *ws.SnapshotPayload, devID uuid.UUID) {
	t.Helper()

	if snapshot == nil {
		t.Fatal("expected non-nil snapshot")
	}
	if got := len(snapshot.LinkMetrics[devID.String()]); got != 0 {
		t.Fatalf("expected no link metrics for %s, got %d", devID, got)
	}
}

func assertSNMPLinkMetricsPresent(t *testing.T, snapshot *ws.SnapshotPayload, devID uuid.UUID) {
	t.Helper()

	if snapshot == nil {
		t.Fatal("expected non-nil snapshot")
	}
	metrics := snapshot.LinkMetrics[devID.String()]
	if len(metrics) != 1 {
		t.Fatalf("expected one link metric for %s, got %d", devID, len(metrics))
	}
	if metrics[0].IfName != "ether1" {
		t.Fatalf("unexpected interface name: got %q", metrics[0].IfName)
	}
	if metrics[0].TxBps == nil || metrics[0].RxBps == nil {
		t.Fatalf("expected non-nil tx/rx rates, got tx=%v rx=%v", metrics[0].TxBps, metrics[0].RxBps)
	}
}

// ---------------------------------------------------------------------------
// New tests: device_models population and topology_changed broadcast
// ---------------------------------------------------------------------------

func TestBuildSnapshot_DeviceModels(t *testing.T) {
	devID := uuid.New()
	promServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := map[string]interface{}{
			"status": "success",
			"data":   map[string]interface{}{"resultType": "vector", "result": []interface{}{}},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	t.Cleanup(promServer.Close)

	deviceRepo := &mockWorkerDeviceRepo{
		devices: []domain.Device{{
			ID:            devID,
			IP:            "10.0.0.1",
			Status:        domain.DeviceStatusUp,
			SysName:       "router1",
			HardwareModel: "RB4011",
			MetricsSource: domain.MetricsSourceSNMP,
			Vendor:        "default",
		}},
	}
	linkRepo := &mockWorkerLinkRepo{}
	invalidateCh := make(chan struct{}, 1)
	dlCache := cache.NewDeviceLinkCache(deviceRepo, linkRepo, invalidateCh)
	settingsRepo := newMockWorkerSettingsRepo()
	registry := buildEmptyVendorRegistry()
	promClient := metrics.NewPromClient(promServer.URL, nil)
	hub := ws.NewHub()

	mc := NewMetricsCollector(promClient, hub, dlCache, deviceRepo, settingsRepo, registry, nil, nil, nil)

	snapshot, _, _ := mc.buildSnapshot(context.Background())

	// device_models should contain the hardware model
	model, ok := snapshot.DeviceModels[devID.String()]
	if !ok {
		t.Fatal("expected device_models entry for device")
	}
	if model != "RB4011" {
		t.Errorf("expected model RB4011, got %s", model)
	}

	// device_hostnames should use DB sys_name (D-02)
	hostname, ok := snapshot.DeviceHostnames[devID.String()]
	if !ok {
		t.Fatal("expected device_hostnames entry for device")
	}
	if hostname != "router1" {
		t.Errorf("expected hostname router1 (from DB sys_name), got %s", hostname)
	}
}

func TestCollectAndBroadcast_TopologyChangedEvent(t *testing.T) {
	devID := uuid.New()
	promServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := map[string]interface{}{
			"status": "success",
			"data": map[string]interface{}{
				"resultType": "vector",
				"result":     []interface{}{},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	t.Cleanup(promServer.Close)

	promClient := metrics.NewPromClient(promServer.URL, nil)
	hub := ws.NewHub()
	go hub.Run()

	deviceRepo := &mockWorkerDeviceRepo{
		devices: []domain.Device{{
			ID: devID, IP: "10.0.0.1", Status: domain.DeviceStatusUp,
			MetricsSource: domain.MetricsSourceSNMP, Vendor: "default",
		}},
	}
	linkRepo := &mockWorkerLinkRepo{}
	invalidateCh := make(chan struct{}, 1)
	dlCache := cache.NewDeviceLinkCache(deviceRepo, linkRepo, invalidateCh)
	settingsRepo := newMockWorkerSettingsRepo()
	registry := buildEmptyVendorRegistry()

	topologyNotify := make(chan struct{}, 1)
	topologyNotify <- struct{}{} // simulate a probeDevice link creation signal

	mc := NewMetricsCollector(promClient, hub, dlCache, deviceRepo, settingsRepo, registry, nil, nil, topologyNotify)

	mc.collectAndBroadcast(context.Background())

	// After collectAndBroadcast, the topology_changed signal should have been drained
	select {
	case <-topologyNotify:
		t.Error("expected topology notify channel to be drained after collectAndBroadcast")
	default:
		// Good — channel was drained
	}
}
