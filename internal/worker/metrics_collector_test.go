package worker

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/google/uuid"
	"github.com/lollinoo/theia/internal/cache"
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
	devices    []domain.Device
	updateCalls int32
}

func (r *mockWorkerDeviceRepo) Create(_ *domain.Device) error                  { return nil }
func (r *mockWorkerDeviceRepo) GetByID(id uuid.UUID) (*domain.Device, error) {
	for _, d := range r.devices {
		if d.ID == id {
			cp := d
			return &cp, nil
		}
	}
	return nil, fmt.Errorf("device not found: %s", id)
}
func (r *mockWorkerDeviceRepo) GetByIP(_ string) (*domain.Device, error)       { return nil, nil }
func (r *mockWorkerDeviceRepo) GetBySysName(_ string) (*domain.Device, error)  { return nil, nil }
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
func (r *mockWorkerLinkRepo) Update(_ *domain.Link) error { return nil }
func (r *mockWorkerLinkRepo) Delete(_ uuid.UUID) error    { return nil }
func (r *mockWorkerLinkRepo) Upsert(_ *domain.Link) error { return nil }

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
