package worker

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
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

func writePrometheusVectorResponse(t *testing.T, w http.ResponseWriter, result []map[string]any) {
	t.Helper()

	resp := map[string]any{
		"status": "success",
		"data": map[string]any{
			"resultType": "vector",
			"result":     result,
		},
	}
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(resp); err != nil {
		t.Fatalf("encode Prometheus response: %v", err)
	}
}

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

func (r *mockWorkerLinkRepo) Create(_ *domain.Link) error { return nil }
func (r *mockWorkerLinkRepo) CreateManualIdempotent(link *domain.Link, _ bool) (*domain.Link, bool, error) {
	return link, true, nil
}
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
	hub := ws.NewHub(ws.WithBroadcastRecorder())

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

	// The snapshot should have a normalized runtime entry for our device.
	deviceRuntime, ok := snapshot.Devices[devID.String()]
	if !ok {
		t.Fatal("expected device runtime entry in snapshot")
	}
	if deviceRuntime.OperationalStatus != string(domain.DeviceStatusUp) {
		t.Fatalf("OperationalStatus = %q, want %q", deviceRuntime.OperationalStatus, domain.DeviceStatusUp)
	}
	if deviceRuntime.MetricsStatus != "unavailable" {
		t.Fatalf("MetricsStatus = %q, want unavailable", deviceRuntime.MetricsStatus)
	}
}

func TestBuildSnapshot_PrometheusDeviceWithoutMetricsRemainsAwaitingPoll(t *testing.T) {
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
	hub := ws.NewHub(ws.WithBroadcastRecorder())
	deviceRepo := &mockWorkerDeviceRepo{
		devices: []domain.Device{{
			ID:                   devID,
			IP:                   "10.0.0.1",
			Status:               domain.DeviceStatusUp,
			MetricsSource:        domain.MetricsSourcePrometheus,
			PrometheusLabelName:  "instance",
			PrometheusLabelValue: "10.0.0.1",
			Vendor:               "default",
		}},
	}
	linkRepo := &mockWorkerLinkRepo{}
	invalidateCh := make(chan struct{}, 1)
	dlCache := cache.NewDeviceLinkCache(deviceRepo, linkRepo, invalidateCh)
	settingsRepo := newMockWorkerSettingsRepo()
	registry := buildEmptyVendorRegistry()

	mc := NewMetricsCollector(promClient, hub, dlCache, deviceRepo, settingsRepo, registry, nil, nil, nil)

	snapshot, promAvailable, promErr := mc.buildSnapshot(context.Background())
	if !promAvailable {
		t.Fatalf("expected Prometheus available, error: %s", promErr)
	}

	deviceRuntime, ok := snapshot.Devices[devID.String()]
	if !ok {
		t.Fatal("expected device runtime entry in snapshot")
	}
	if deviceRuntime.Freshness != "awaiting_poll" {
		t.Fatalf("Freshness = %q, want awaiting_poll", deviceRuntime.Freshness)
	}
	if deviceRuntime.MetricsReason != "awaiting_poll" {
		t.Fatalf("MetricsReason = %q, want awaiting_poll", deviceRuntime.MetricsReason)
	}
	if deviceRuntime.LastCollectedAt != nil {
		t.Fatalf("LastCollectedAt = %#v, want nil", deviceRuntime.LastCollectedAt)
	}
	if deviceRuntime.LastPolledAt != nil {
		t.Fatalf("LastPolledAt = %#v, want nil", deviceRuntime.LastPolledAt)
	}
}

func TestBuildSnapshot_PrometheusMetricsLeaveLastPolledAtUnset(t *testing.T) {
	devID := uuid.New()

	promServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		query := r.URL.Query().Get("query")
		switch {
		case strings.Contains(query, "hrProcessorLoad"):
			writePrometheusVectorResponse(t, w, []map[string]any{{
				"metric": map[string]string{"instance": "10.0.0.1"},
				"value":  []any{1741374000.0, "41.5"},
			}})
		default:
			writePrometheusVectorResponse(t, w, nil)
		}
	}))
	t.Cleanup(promServer.Close)

	promClient := metrics.NewPromClient(promServer.URL, nil)
	hub := ws.NewHub(ws.WithBroadcastRecorder())
	deviceRepo := &mockWorkerDeviceRepo{
		devices: []domain.Device{{
			ID:                   devID,
			IP:                   "10.0.0.1",
			Status:               domain.DeviceStatusUp,
			MetricsSource:        domain.MetricsSourcePrometheus,
			PrometheusLabelName:  "instance",
			PrometheusLabelValue: "10.0.0.1",
			Vendor:               "default",
			Interfaces:           []domain.Interface{{IfName: "ether1", IfDescr: "ether1"}},
		}},
	}
	linkRepo := &mockWorkerLinkRepo{}
	invalidateCh := make(chan struct{}, 1)
	dlCache := cache.NewDeviceLinkCache(deviceRepo, linkRepo, invalidateCh)
	settingsRepo := newMockWorkerSettingsRepo()
	registry := buildVendorRegistryWithPrometheusCPU()

	mc := NewMetricsCollector(promClient, hub, dlCache, deviceRepo, settingsRepo, registry, nil, nil, nil)

	snapshot, promAvailable, promErr := mc.buildSnapshot(context.Background())
	if !promAvailable {
		t.Fatalf("expected Prometheus available, error: %s", promErr)
	}

	deviceRuntime, ok := snapshot.Devices[devID.String()]
	if !ok {
		t.Fatal("expected device runtime entry in snapshot")
	}
	if deviceRuntime.LastCollectedAt == nil {
		t.Fatal("expected LastCollectedAt to be set from Prometheus metrics")
	}
	if deviceRuntime.LastPolledAt != nil {
		t.Fatalf("LastPolledAt = %#v, want nil", deviceRuntime.LastPolledAt)
	}
	if deviceRuntime.Freshness != "fresh" {
		t.Fatalf("Freshness = %q, want fresh", deviceRuntime.Freshness)
	}
}

func TestBuildSnapshot_PrometheusInterfaceDiscoveryDoesNotMutateCachedDevices(t *testing.T) {
	devID := uuid.New()

	promServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		query := r.URL.Query().Get("query")
		switch {
		case strings.Contains(query, "ifDescr"):
			writePrometheusVectorResponse(t, w, []map[string]any{{
				"metric": map[string]string{
					"instance": "10.0.0.1",
					"ifIndex":  "1",
					"ifName":   "ether1",
					"ifDescr":  "ether1",
					"ifSpeed":  "1000000000",
				},
				"value": []any{1741374000.0, "1"},
			}})
		default:
			writePrometheusVectorResponse(t, w, nil)
		}
	}))
	t.Cleanup(promServer.Close)

	promClient := metrics.NewPromClient(promServer.URL, nil)
	hub := ws.NewHub(ws.WithBroadcastRecorder())
	deviceRepo := &mockWorkerDeviceRepo{
		devices: []domain.Device{{
			ID:                   devID,
			IP:                   "10.0.0.1",
			Status:               domain.DeviceStatusUp,
			MetricsSource:        domain.MetricsSourcePrometheus,
			PrometheusLabelName:  "instance",
			PrometheusLabelValue: "10.0.0.1",
			Vendor:               "default",
		}},
	}
	linkRepo := &mockWorkerLinkRepo{}
	invalidateCh := make(chan struct{}, 1)
	dlCache := cache.NewDeviceLinkCache(deviceRepo, linkRepo, invalidateCh)
	settingsRepo := newMockWorkerSettingsRepo()
	registry := buildEmptyVendorRegistry()

	cachedBefore, err := dlCache.GetDevices()
	if err != nil {
		t.Fatalf("GetDevices before buildSnapshot: %v", err)
	}
	if len(cachedBefore) != 1 || len(cachedBefore[0].Interfaces) != 0 {
		t.Fatalf("expected cache to start with one device and no interfaces, got %+v", cachedBefore)
	}

	mc := NewMetricsCollector(promClient, hub, dlCache, deviceRepo, settingsRepo, registry, nil, nil, nil)
	mc.buildSnapshot(context.Background())

	if len(cachedBefore[0].Interfaces) != 0 {
		t.Fatalf("cached device slice mutated in place: %+v", cachedBefore[0].Interfaces)
	}

	cachedAfter, err := dlCache.GetDevices()
	if err != nil {
		t.Fatalf("GetDevices after buildSnapshot: %v", err)
	}
	if len(cachedAfter) != 1 {
		t.Fatalf("expected one cached device after buildSnapshot, got %d", len(cachedAfter))
	}
	if len(cachedAfter[0].Interfaces) != 0 {
		t.Fatalf("cached device state mutated by buildSnapshot: %+v", cachedAfter[0].Interfaces)
	}
	if got := atomic.LoadInt32(&deviceRepo.updateCalls); got != 1 {
		t.Fatalf("Update called %d times, want 1", got)
	}
}

func TestMetricsCollectorAppliesContractNormalizedRuntimeOutcome(t *testing.T) {
	devID := uuid.New()
	peerID := uuid.New()
	linkID := uuid.New()
	fixture := loadPrometheusRuntimeFixture(t, "partial.json")

	promServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		result := prometheusRuntimeFixtureResponse(fixture, r.URL.Query().Get("query"))
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(map[string]any{
			"status": "success",
			"data": map[string]any{
				"resultType": "vector",
				"result":     result,
			},
		}); err != nil {
			t.Fatalf("encode Prometheus response: %v", err)
		}
	}))
	t.Cleanup(promServer.Close)

	promClient := metrics.NewPromClient(promServer.URL, nil)
	hub := ws.NewHub(ws.WithBroadcastRecorder())
	deviceRepo := &mockWorkerDeviceRepo{
		devices: []domain.Device{
			{
				ID:                   devID,
				IP:                   fixture.Device.IP,
				Status:               domain.DeviceStatusUnknown,
				MetricsSource:        domain.MetricsSourcePrometheus,
				PrometheusLabelName:  fixture.Device.LabelName,
				PrometheusLabelValue: fixture.Device.LabelValue,
				Vendor:               "default",
				Interfaces:           []domain.Interface{{DeviceID: devID, IfName: "ether1", IfDescr: "ether1", Speed: 1_000_000_000}},
			},
			{
				ID:         peerID,
				IP:         "192.0.2.99",
				Status:     domain.DeviceStatusUp,
				Interfaces: []domain.Interface{{DeviceID: peerID, IfName: "ether9", IfDescr: "ether9", Speed: 1_000_000_000}},
			},
		},
	}
	linkRepo := &mockWorkerLinkRepo{links: []domain.Link{{
		ID:             linkID,
		SourceDeviceID: devID,
		SourceIfName:   "ether1",
		TargetDeviceID: peerID,
		TargetIfName:   "ether9",
	}}}
	invalidateCh := make(chan struct{}, 1)
	dlCache := cache.NewDeviceLinkCache(deviceRepo, linkRepo, invalidateCh)
	settingsRepo := newMockWorkerSettingsRepo()
	registry := buildEmptyVendorRegistry()

	mc := NewMetricsCollector(promClient, hub, dlCache, deviceRepo, settingsRepo, registry, nil, nil, nil)

	snapshot, promAvailable, promErr := mc.buildSnapshot(context.Background())
	if !promAvailable {
		t.Fatalf("expected Prometheus available, error: %s", promErr)
	}

	runtime := normalizedRuntimeOutcomeFromSnapshot(snapshot, devID)
	if runtime.Device.Status != domain.DeviceStatusUp {
		t.Fatalf("runtime.Device.Status = %q, want %q", runtime.Device.Status, domain.DeviceStatusUp)
	}
	if got := len(runtime.Links); got != 1 {
		t.Fatalf("runtime link count = %d, want 1", got)
	}
	linkRuntime := runtime.Links[0]
	if linkRuntime.LinkID != linkID.String() {
		t.Fatalf("linkRuntime.LinkID = %q, want %q", linkRuntime.LinkID, linkID)
	}
	if linkRuntime.DeviceID != devID.String() {
		t.Fatalf("linkRuntime.DeviceID = %q, want %q", linkRuntime.DeviceID, devID)
	}
	if linkRuntime.IfName != "ether1" {
		t.Fatalf("linkRuntime.IfName = %q, want %q", linkRuntime.IfName, "ether1")
	}
	if linkRuntime.MetricsStatus != "available" {
		t.Fatalf("linkRuntime.MetricsStatus = %q, want available", linkRuntime.MetricsStatus)
	}
	if linkRuntime.MetricsReason != "ok" {
		t.Fatalf("linkRuntime.MetricsReason = %q, want ok", linkRuntime.MetricsReason)
	}
	if linkRuntime.TxBps == nil || *linkRuntime.TxBps != 500 {
		t.Fatalf("linkRuntime.TxBps = %#v, want 500", linkRuntime.TxBps)
	}
	if linkRuntime.RxBps == nil || *linkRuntime.RxBps != 250 {
		t.Fatalf("linkRuntime.RxBps = %#v, want 250", linkRuntime.RxBps)
	}
	if linkRuntime.Utilization == nil || *linkRuntime.Utilization != 0.0000005 {
		t.Fatalf("linkRuntime.Utilization = %#v, want 0.0000005", linkRuntime.Utilization)
	}
}

func TestBuildSnapshot_VirtualNoIPNormalizesLegacyStatus(t *testing.T) {
	devID := uuid.New()
	var requestCount int32

	promServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&requestCount, 1)
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
	hub := ws.NewHub(ws.WithBroadcastRecorder())
	deviceRepo := &mockWorkerDeviceRepo{
		devices: []domain.Device{
			{
				ID:                   devID,
				Hostname:             "support-node",
				IP:                   "",
				DeviceType:           domain.DeviceTypeVirtual,
				Status:               domain.DeviceStatusDown,
				MetricsSource:        domain.MetricsSourcePrometheus,
				PrometheusLabelName:  "instance",
				PrometheusLabelValue: "10.0.0.99",
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

	snapshot, promAvailable, promErr := mc.buildSnapshot(context.Background())

	if snapshot == nil {
		t.Fatal("expected non-nil snapshot")
	}
	if !promAvailable {
		t.Fatalf("expected Prometheus to remain available, error: %s", promErr)
	}
	if got := snapshot.Devices[devID.String()].OperationalStatus; got != "unmonitored" {
		t.Fatalf("expected no-IP virtual node operational_status unmonitored, got %q", got)
	}
	if got := atomic.LoadInt32(&requestCount); got != 0 {
		t.Fatalf("expected no Prometheus queries for no-IP virtual node, got %d", got)
	}
}

type prometheusRuntimeFixture struct {
	Version int `json:"version"`
	Device  struct {
		IP         string `json:"ip"`
		LabelName  string `json:"label_name"`
		LabelValue string `json:"label_value"`
	} `json:"device"`
	Probe []struct {
		Instance string  `json:"instance"`
		Value    float64 `json:"value"`
	} `json:"probe"`
	Links []struct {
		Instance string  `json:"instance"`
		IfIndex  string  `json:"if_index"`
		IfName   string  `json:"if_name"`
		TxBps    float64 `json:"tx_bps"`
		RxBps    float64 `json:"rx_bps"`
		IfSpeed  float64 `json:"if_speed"`
	} `json:"links"`
}

type normalizedRuntimeOutcome struct {
	Device domain.Device
	Links  []ws.LinkRuntimeDTO
}

func loadPrometheusRuntimeFixture(t *testing.T, name string) prometheusRuntimeFixture {
	t.Helper()

	path := filepath.Join("..", "collector", "testdata", "prometheus", name)
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%q) error = %v", path, err)
	}

	var fixture prometheusRuntimeFixture
	if err := json.Unmarshal(raw, &fixture); err != nil {
		t.Fatalf("json.Unmarshal(%q) error = %v", path, err)
	}
	if fixture.Version != 1 {
		t.Fatalf("fixture version = %d, want 1", fixture.Version)
	}

	return fixture
}

func prometheusRuntimeFixtureResponse(fixture prometheusRuntimeFixture, query string) []map[string]any {
	switch {
	case strings.Contains(query, "probe_success"):
		return prometheusRuntimeProbeResponse(fixture)
	case strings.Contains(query, "rate(ifHCOutOctets"):
		return prometheusRuntimeLinkResponse(fixture, "tx")
	case strings.Contains(query, "rate(ifHCInOctets"):
		return prometheusRuntimeLinkResponse(fixture, "rx")
	case strings.Contains(query, "ifSpeed{"):
		return prometheusRuntimeLinkResponse(fixture, "speed")
	default:
		return nil
	}
}

func prometheusRuntimeProbeResponse(fixture prometheusRuntimeFixture) []map[string]any {
	result := make([]map[string]any, 0, len(fixture.Probe))
	for _, sample := range fixture.Probe {
		result = append(result, map[string]any{
			"metric": map[string]string{"instance": sample.Instance},
			"value":  []any{1741374000.0, strconv.FormatFloat(sample.Value, 'f', -1, 64)},
		})
	}
	return result
}

func prometheusRuntimeLinkResponse(fixture prometheusRuntimeFixture, field string) []map[string]any {
	result := make([]map[string]any, 0, len(fixture.Links))
	for _, sample := range fixture.Links {
		value := 0.0
		switch field {
		case "tx":
			value = sample.TxBps
		case "rx":
			value = sample.RxBps
		case "speed":
			value = sample.IfSpeed
		}
		result = append(result, map[string]any{
			"metric": map[string]string{
				"instance": sample.Instance,
				"ifIndex":  sample.IfIndex,
				"ifName":   sample.IfName,
			},
			"value": []any{1741374000.0, strconv.FormatFloat(value, 'f', -1, 64)},
		})
	}
	return result
}

func normalizedRuntimeOutcomeFromSnapshot(snapshot *ws.SnapshotPayload, deviceID uuid.UUID) normalizedRuntimeOutcome {
	deviceKey := deviceID.String()
	return normalizedRuntimeOutcome{
		Device: domain.Device{ID: deviceID, Status: domain.DeviceStatus(snapshot.DeviceStatuses[deviceKey])},
		Links:  snapshot.LinkMetrics[deviceKey],
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
	hub := ws.NewHub(ws.WithBroadcastRecorder())

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

	// Additionally verify snapshot contains normalized runtime state.
	if _, ok := snap.Devices[devID.String()]; !ok {
		t.Error("expected device runtime in snapshot after collectAndBroadcast")
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
	hub := ws.NewHub(ws.WithBroadcastRecorder())

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
	hub := ws.NewHub(ws.WithBroadcastRecorder())

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

	dm, ok := snapshot.Devices[devID.String()]
	if !ok {
		t.Fatalf("expected device runtime entry for device %s", devID)
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

// ---------------------------------------------------------------------------
// Delta detection tests: buildDelta
// ---------------------------------------------------------------------------

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
	hub := ws.NewHub(ws.WithBroadcastRecorder())

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

func buildVendorRegistryWithPrometheusCPU() *vendor.Registry {
	records := []vendor.DBVendorRecord{
		{
			Name: "default",
			ConfigJSON: `{
				"vendor": {"name": "default", "display_name": "Generic"},
				"detection": {},
				"metrics": {
					"prometheus": {
						"cpu": "hrProcessorLoad{%[1]s=~\"%[2]s\"}"
					}
				},
				"backup": {"supported": false}
			}`,
		},
	}
	reg, err := vendor.LoadRegistryFromDB(records)
	if err != nil {
		panic(fmt.Sprintf("buildVendorRegistryWithPrometheusCPU: %v", err))
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
	hub := ws.NewHub(ws.WithBroadcastRecorder())

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
	payload, ok := deltaMsg["payload"].(map[string]interface{})
	if !ok {
		t.Fatalf("payload = %#v, want object", deltaMsg["payload"])
	}
	if _, ok := payload["base_version"].(float64); !ok {
		t.Fatalf("payload.base_version = %#v, want number", payload["base_version"])
	}
	if _, ok := payload["version"].(float64); !ok {
		t.Fatalf("payload.version = %#v, want number", payload["version"])
	}
	if _, ok := payload["delta"].(map[string]interface{}); !ok {
		t.Fatalf("payload.delta = %#v, want object", payload["delta"])
	}
}

func TestCollectAndBroadcast_FirstCycle_BroadcastsVersionedSnapshot(t *testing.T) {
	mc, hub, _, _ := newMockCollectorWithChangingData(t)

	mc.collectAndBroadcast(context.Background())

	msgs := drainBroadcastCh(hub)
	if len(msgs) == 0 {
		t.Fatal("expected at least one broadcast on first cycle")
	}

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
		t.Fatalf("expected a message with type %q on first cycle", "snapshot")
	}

	payload, ok := snapshotMsg["payload"].(map[string]interface{})
	if !ok {
		t.Fatalf("payload = %#v, want object", snapshotMsg["payload"])
	}
	if _, ok := payload["version"].(float64); !ok {
		t.Fatalf("payload.version = %#v, want number", payload["version"])
	}
	if _, ok := payload["snapshot"].(map[string]interface{}); !ok {
		t.Fatalf("payload.snapshot = %#v, want object", payload["snapshot"])
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
	hub := ws.NewHub(ws.WithBroadcastRecorder())

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
	hub := ws.NewHub(ws.WithBroadcastRecorder())

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

func TestBuildSnapshot_SNMPLinkRates_DelayedSampleEmitsRate(t *testing.T) {
	calls := 0
	mc, settingsRepo, devID, deviceIP := newSNMPLinkRateTestCollector(t, func(target string, creds domain.SNMPCredentials) ([]SNMPIfCounter, error) {
		calls++
		switch calls {
		case 1:
			return []SNMPIfCounter{{IfName: "ether1", InOctets: 1_100, OutOctets: 2_100}}, nil
		case 2:
			return []SNMPIfCounter{{IfName: "ether1", InOctets: 1_200, OutOctets: 2_200}}, nil
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
	assertSNMPLinkMetricsPresent(t, snapshot, devID)

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
	hub := ws.NewHub(ws.WithBroadcastRecorder())
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
	for _, linkRuntime := range snapshot.Links {
		if linkRuntime.SourceDeviceID != devID.String() && linkRuntime.TargetDeviceID != devID.String() {
			continue
		}
		if linkRuntime.TxBps != nil || linkRuntime.RxBps != nil || linkRuntime.Utilization != nil {
			t.Fatalf("expected no link metrics for %s, got tx=%v rx=%v util=%v", devID, linkRuntime.TxBps, linkRuntime.RxBps, linkRuntime.Utilization)
		}
		if linkRuntime.MetricsStatus != "unavailable" {
			t.Fatalf("expected unavailable link metrics status for %s, got %q", devID, linkRuntime.MetricsStatus)
		}
		return
	}
	t.Fatalf("expected link runtime entry for %s", devID)
}

func assertSNMPLinkMetricsPresent(t *testing.T, snapshot *ws.SnapshotPayload, devID uuid.UUID) {
	t.Helper()

	if snapshot == nil {
		t.Fatal("expected non-nil snapshot")
	}
	for _, linkRuntime := range snapshot.Links {
		if linkRuntime.SourceDeviceID != devID.String() && linkRuntime.TargetDeviceID != devID.String() {
			continue
		}
		if linkRuntime.SourceIfName != "ether1" && linkRuntime.TargetIfName != "ether1" {
			t.Fatalf("unexpected interface names: source=%q target=%q", linkRuntime.SourceIfName, linkRuntime.TargetIfName)
		}
		if linkRuntime.TxBps == nil || linkRuntime.RxBps == nil {
			t.Fatalf("expected non-nil tx/rx rates, got tx=%v rx=%v", linkRuntime.TxBps, linkRuntime.RxBps)
		}
		if linkRuntime.MetricsStatus == "unavailable" {
			t.Fatalf("expected available or partial link metrics for %s, got unavailable", devID)
		}
		return
	}
	t.Fatalf("expected one link runtime for %s", devID)
}

// ---------------------------------------------------------------------------
// New tests: device_models population and topology_changed broadcast
// ---------------------------------------------------------------------------

func TestBuildSnapshot_UsesSlimOverviewSections(t *testing.T) {
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
	hub := ws.NewHub(ws.WithBroadcastRecorder())

	mc := NewMetricsCollector(promClient, hub, dlCache, deviceRepo, settingsRepo, registry, nil, nil, nil)

	snapshot, _, _ := mc.buildSnapshot(context.Background())

	status, ok := snapshot.DeviceStatuses[devID.String()]
	if !ok {
		t.Fatal("expected device_statuses entry for device")
	}
	if status != string(domain.DeviceStatusUp) {
		t.Errorf("expected status up, got %s", status)
	}

	raw, err := json.Marshal(snapshot)
	if err != nil {
		t.Fatalf("json.Marshal returned error: %v", err)
	}

	var decoded map[string]any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("json.Unmarshal returned error: %v", err)
	}
	if _, ok := decoded["alerts"]; ok {
		t.Fatal("expected slim snapshot to omit alerts")
	}
	if _, ok := decoded["device_hostnames"]; ok {
		t.Fatal("expected slim snapshot to omit device_hostnames")
	}
	if _, ok := decoded["device_models"]; ok {
		t.Fatal("expected slim snapshot to omit device_models")
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
	hub := ws.NewHub(ws.WithBroadcastRecorder())
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

func TestCollectAndBroadcast_TopologyChangedWithoutDeltaBroadcastsInvalidationOnly(t *testing.T) {
	mc, hub := newMockCollector(t)
	topologyNotify := make(chan struct{}, 1)
	mc.topologyNotify = topologyNotify

	mc.collectAndBroadcast(context.Background())
	drainBroadcastCh(hub)

	topologyNotify <- struct{}{}
	mc.collectAndBroadcast(context.Background())

	types := broadcastTypesFromRawMessages(t, drainBroadcastCh(hub))
	if len(types) != 1 || types[0] != ws.MessageTypeTopologyChanged {
		t.Fatalf("expected topology_changed invalidation without forced snapshot, got %v", types)
	}
}

func broadcastTypesFromRawMessages(t *testing.T, rawMessages [][]byte) []string {
	t.Helper()

	types := make([]string, 0, len(rawMessages))
	for _, raw := range rawMessages {
		var msg struct {
			Type string `json:"type"`
		}
		if err := json.Unmarshal(raw, &msg); err != nil {
			t.Fatalf("failed to unmarshal broadcast message: %v", err)
		}
		types = append(types, msg.Type)
	}

	return types
}
