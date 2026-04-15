package worker

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"github.com/gosnmp/gosnmp"

	"github.com/lollinoo/theia/internal/cache"
	"github.com/lollinoo/theia/internal/collector"
	"github.com/lollinoo/theia/internal/domain"
	"github.com/lollinoo/theia/internal/scheduler"
	"github.com/lollinoo/theia/internal/service"
	"github.com/lollinoo/theia/internal/snmp"
	"github.com/lollinoo/theia/internal/state"
	"github.com/lollinoo/theia/internal/ws"
)

type pipelineTestScheduler struct {
	mu          sync.Mutex
	status      string
	tasks       chan scheduler.PollTask
	completions []scheduler.Completion
	startCalls  int
	stopCalls   int
}

func newPipelineTestScheduler() *pipelineTestScheduler {
	return &pipelineTestScheduler{
		status: "stopped",
		tasks:  make(chan scheduler.PollTask, 16),
	}
}

func (s *pipelineTestScheduler) Start(context.Context) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.status = "running"
	s.startCalls++
}

func (s *pipelineTestScheduler) Stop() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.status = "stopped"
	s.stopCalls++
}

func (s *pipelineTestScheduler) Tasks() <-chan scheduler.PollTask {
	return s.tasks
}

func (s *pipelineTestScheduler) Complete(completion scheduler.Completion) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.completions = append(s.completions, completion)
}

func (s *pipelineTestScheduler) Status() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.status
}

type fakeTopologyService struct {
	mu     sync.Mutex
	calls  int
	lastID uuid.UUID
	lastIn service.StaticDiscoveryInput
	result service.StaticPersistenceResult
	err    error
}

func (s *fakeTopologyService) ApplyStaticDiscovery(deviceID uuid.UUID, input service.StaticDiscoveryInput) (service.StaticPersistenceResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.calls++
	s.lastID = deviceID
	s.lastIn = input
	return s.result, s.err
}

type fakePrometheusClient struct {
	mu            sync.Mutex
	hostnames     map[string]string
	probeStatuses map[string]bool
	alertsByID    map[uuid.UUID][]domain.AlertState
	alertsErr     error
	hostnameErr   error
	probeErr      error
}

func (c *fakePrometheusClient) QueryHostnames(_ context.Context, _ string, labelValues []string) (map[string]string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.hostnameErr != nil {
		return nil, c.hostnameErr
	}
	result := make(map[string]string, len(labelValues))
	for _, labelValue := range labelValues {
		if hostname, ok := c.hostnames[labelValue]; ok {
			result[labelValue] = hostname
		}
	}
	return result, nil
}

func (c *fakePrometheusClient) QueryProbeStatus(_ context.Context, deviceIPs []string) (map[string]bool, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.probeErr != nil {
		return nil, c.probeErr
	}
	result := make(map[string]bool, len(deviceIPs))
	for _, deviceIP := range deviceIPs {
		if status, ok := c.probeStatuses[deviceIP]; ok {
			result[deviceIP] = status
		}
	}
	return result, nil
}

func (c *fakePrometheusClient) QueryAlerts(context.Context) ([]domain.AlertState, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.alertsErr != nil {
		return nil, c.alertsErr
	}
	var alerts []domain.AlertState
	for deviceID, grouped := range c.alertsByID {
		for _, alert := range grouped {
			mapped := alert
			if mapped.DeviceID == uuid.Nil {
				mapped.DeviceID = deviceID
			}
			alerts = append(alerts, mapped)
		}
	}
	return alerts, nil
}

type fakeSNMPClient struct {
	getResponses  map[string][]gosnmp.SnmpPDU
	walkResponses map[string][]gosnmp.SnmpPDU
	getErr        map[string]error
	walkErr       map[string]error
}

func (c *fakeSNMPClient) Connect() error { return nil }
func (c *fakeSNMPClient) Close() error   { return nil }

func (c *fakeSNMPClient) Get(oids []string) ([]gosnmp.SnmpPDU, error) {
	var out []gosnmp.SnmpPDU
	for _, oid := range oids {
		if err := c.getErr[oid]; err != nil {
			return nil, err
		}
		out = append(out, c.getResponses[oid]...)
	}
	return out, nil
}

func (c *fakeSNMPClient) BulkWalk(rootOid string) ([]gosnmp.SnmpPDU, error) {
	if err := c.walkErr[rootOid]; err != nil {
		return nil, err
	}
	return append([]gosnmp.SnmpPDU(nil), c.walkResponses[rootOid]...), nil
}

func newPipelineTestCache(devices []domain.Device, links []domain.Link) *cache.DeviceLinkCache {
	return cache.NewDeviceLinkCache(
		&mockWorkerDeviceRepo{devices: devices},
		&mockWorkerLinkRepo{links: links},
		make(chan struct{}, 1),
	)
}

func newPerformanceTestCollector(t *testing.T) *collector.PerformanceCollector {
	t.Helper()
	client := &fakeSNMPClient{
		getResponses: map[string][]gosnmp.SnmpPDU{
			snmp.OidSysUpTime: {{Name: snmp.OidSysUpTime, Value: uint32(3_000)}},
		},
		walkResponses: map[string][]gosnmp.SnmpPDU{
			snmp.OidHrProcessorLoad: {
				{Name: snmp.OidHrProcessorLoad + ".1", Value: 55},
			},
			snmp.OidIfName: {
				{Name: snmp.OidIfName + ".1", Value: "ether1"},
			},
			snmp.OidIfHCInOctets: {
				{Name: snmp.OidIfHCInOctets + ".1", Value: uint64(4_000)},
			},
			snmp.OidIfHCOutOctets: {
				{Name: snmp.OidIfHCOutOctets + ".1", Value: uint64(8_000)},
			},
		},
	}

	return collector.NewPerformanceCollector(buildEmptyVendorRegistry(), func(string, domain.SNMPCredentials, time.Duration, int) (collector.SNMPClient, error) {
		return client, nil
	})
}

func newOperationalTestCollector(t *testing.T) *collector.OperationalCollector {
	t.Helper()
	client := &fakeSNMPClient{
		getResponses: map[string][]gosnmp.SnmpPDU{
			snmp.OidSysUpTime: {{Name: snmp.OidSysUpTime, Value: uint32(6_000)}},
		},
		walkResponses: map[string][]gosnmp.SnmpPDU{
			snmp.OidIfName: {
				{Name: snmp.OidIfName + ".1", Value: "ether1"},
			},
			snmp.OidIfOperStatus: {
				{Name: snmp.OidIfOperStatus + ".1", Value: 1},
			},
		},
	}

	return collector.NewOperationalCollector(buildEmptyVendorRegistry(), func(string, domain.SNMPCredentials, time.Duration, int) (collector.SNMPClient, error) {
		return client, nil
	})
}

func newStaticTestCollector(t *testing.T) *collector.StaticCollector {
	t.Helper()
	client := &fakeSNMPClient{
		getResponses: map[string][]gosnmp.SnmpPDU{
			snmp.OidSysName:     {{Name: snmp.OidSysName, Value: "edge-sw-1"}},
			snmp.OidSysDescr:    {{Name: snmp.OidSysDescr, Value: "SwitchOS edge"}},
			snmp.OidSysObjectID: {{Name: snmp.OidSysObjectID, Value: ".1.3.6.1.4.1.14988.1"}},
		},
		walkResponses: map[string][]gosnmp.SnmpPDU{
			snmp.OidIfTable: {
				{Name: snmp.OidIfDescr + ".1", Value: "uplink"},
				{Name: snmp.OidIfSpeed + ".1", Value: uint32(1_000_000_000)},
				{Name: snmp.OidIfAdminStatus + ".1", Value: 1},
				{Name: snmp.OidIfOperStatus + ".1", Value: 1},
			},
			snmp.OidIfXTable: {
				{Name: snmp.OidIfName + ".1", Value: "ether1"},
				{Name: snmp.OidIfHighSpeed + ".1", Value: uint32(1_000)},
			},
			snmp.OidLLDPLocPortIfIndex: {
				{Name: snmp.OidLLDPLocPortIfIndex + ".1", Value: int(1)},
			},
			snmp.OidLLDPLocPortId: {
				{Name: snmp.OidLLDPLocPortId + ".1", Value: "ether1"},
			},
			snmp.OidLLDPRemChassisId: {
				{Name: snmp.OidLLDPRemChassisId + ".0.1.1", Value: "aa:bb:cc:dd:ee:ff"},
			},
			snmp.OidLLDPRemPortId: {
				{Name: snmp.OidLLDPRemPortId + ".0.1.1", Value: "ether2"},
			},
			snmp.OidLLDPRemSysName: {
				{Name: snmp.OidLLDPRemSysName + ".0.1.1", Value: "remote-switch"},
			},
		},
	}

	return collector.NewStaticCollector(buildEmptyVendorRegistry(), func(string, domain.SNMPCredentials, time.Duration, int) (collector.SNMPClient, error) {
		return client, nil
	})
}

func TestPipelineOrchestratorPerformanceTaskUpdatesStoreAndCompletesScheduler(t *testing.T) {
	deviceID := uuid.New()
	linkID := uuid.New()
	task := scheduler.PollTask{
		RunID:            42,
		Key:              scheduler.NewTaskKey(deviceID, domain.VolatilityClassPerformance),
		VolatilityClass:  domain.VolatilityClassPerformance,
		ExpectedInterval: 30 * time.Second,
		Device: domain.Device{
			ID:                   deviceID,
			IP:                   "192.0.2.10",
			Vendor:               "default",
			Status:               domain.DeviceStatusUnknown,
			PrometheusLabelName:  "instance",
			PrometheusLabelValue: "192.0.2.10",
			Interfaces: []domain.Interface{
				{IfName: "ether1", IfDescr: "uplink", Speed: 1_000_000_000},
			},
		},
	}

	sched := newPipelineTestScheduler()
	store := state.NewStore()
	promClient := &fakePrometheusClient{
		hostnames:     map[string]string{"192.0.2.10": "core-sw-1"},
		probeStatuses: map[string]bool{"192.0.2.10": true},
	}
	settingsRepo := newMockWorkerSettingsRepo()
	if err := settingsRepo.Set(domain.SettingPrometheusURL, "http://prometheus.test"); err != nil {
		t.Fatalf("set prometheus_url: %v", err)
	}
	pipeline := NewPipelineOrchestrator(
		sched,
		store,
		newPipelineTestCache([]domain.Device{task.Device}, []domain.Link{{
			ID:             linkID,
			SourceDeviceID: deviceID,
			SourceIfName:   "ether1",
			TargetDeviceID: uuid.New(),
			TargetIfName:   "ether9",
		}}),
		ws.NewHub(),
		newPerformanceTestCollector(t),
		newOperationalTestCollector(t),
		newStaticTestCollector(t),
		collector.NewPrometheusCollector(promClient),
		&fakeTopologyService{},
		settingsRepo,
		make(chan struct{}, 1),
	)
	pipeline.prevCounters[deviceID] = map[string]collector.CounterBaseline{
		"ether1": {
			InOctets:  1_000,
			OutOctets: 2_000,
			SampledAt: time.Now().Add(-30 * time.Second),
		},
	}

	pipeline.runTask(context.Background(), task)

	deviceState, ok := store.GetDevice(deviceID)
	if !ok {
		t.Fatal("expected state store update for performance task")
	}
	if deviceState.Metrics.CPUPercent == nil || *deviceState.Metrics.CPUPercent != 55 {
		t.Fatalf("expected CPU metric 55, got %#v", deviceState.Metrics.CPUPercent)
	}
	if len(deviceState.LinkMetrics) != 1 {
		t.Fatalf("expected 1 computed link metric, got %d", len(deviceState.LinkMetrics))
	}
	if deviceState.LinkMetrics[0].TxBps == nil || *deviceState.LinkMetrics[0].TxBps <= 0 {
		t.Fatalf("expected positive tx_bps, got %#v", deviceState.LinkMetrics[0].TxBps)
	}

	pipeline.snapshotMu.RLock()
	hostname := pipeline.hostnames[deviceID]
	pipeline.snapshotMu.RUnlock()
	if hostname != "core-sw-1" {
		t.Fatalf("expected hostname override from prometheus enrichment, got %q", hostname)
	}

	sched.mu.Lock()
	defer sched.mu.Unlock()
	if len(sched.completions) != 1 {
		t.Fatalf("expected one scheduler completion, got %d", len(sched.completions))
	}
	if sched.completions[0].RunID != task.RunID {
		t.Fatalf("expected completion RunID %d, got %d", task.RunID, sched.completions[0].RunID)
	}
	if sched.completions[0].Key != task.Key {
		t.Fatalf("expected completion key %+v, got %+v", task.Key, sched.completions[0].Key)
	}
}

func TestPipelineOrchestratorStaticTaskUpdatesStorePersistsTopologyAndSignalsNotify(t *testing.T) {
	deviceID := uuid.New()
	performanceAt := time.Date(2026, 4, 13, 12, 30, 0, 0, time.UTC)
	task := scheduler.PollTask{
		RunID:            77,
		Key:              scheduler.NewTaskKey(deviceID, domain.VolatilityClassStatic),
		VolatilityClass:  domain.VolatilityClassStatic,
		ExpectedInterval: 5 * time.Minute,
		Device: domain.Device{
			ID:     deviceID,
			IP:     "192.0.2.20",
			Status: domain.DeviceStatusProbing,
			Vendor: "default",
		},
	}

	sched := newPipelineTestScheduler()
	store := state.NewStore()
	store.Update(state.StateUpdate{
		DeviceID:        deviceID,
		VolatilityClass: domain.VolatilityClassPerformance,
		Metrics: &domain.DeviceMetrics{
			DeviceID:    deviceID,
			CPUPercent:  floatPtr(55),
			CollectedAt: performanceAt,
		},
		PollSuccess:      true,
		ExpectedInterval: 30 * time.Second,
		Timestamp:        performanceAt,
	})
	topologyNotify := make(chan struct{}, 1)
	topologyService := &fakeTopologyService{
		result: service.StaticPersistenceResult{TopologyChanged: true, LinksCreated: 1},
	}
	pipeline := NewPipelineOrchestrator(
		sched,
		store,
		nil,
		ws.NewHub(),
		newPerformanceTestCollector(t),
		newOperationalTestCollector(t),
		newStaticTestCollector(t),
		collector.NewPrometheusCollector(&fakePrometheusClient{}),
		topologyService,
		newMockWorkerSettingsRepo(),
		topologyNotify,
	)

	pipeline.runTask(context.Background(), task)

	deviceState, ok := store.GetDevice(deviceID)
	if !ok {
		t.Fatal("expected static task to update state store")
	}
	if !deviceState.LastPolledAt.Equal(performanceAt) {
		t.Fatalf("LastPolledAt = %s, want performance poll timestamp %s", deviceState.LastPolledAt, performanceAt)
	}
	if deviceState.ExpectedInterval != 30*time.Second {
		t.Fatalf("ExpectedInterval = %s, want 30s performance cadence", deviceState.ExpectedInterval)
	}

	topologyService.mu.Lock()
	if topologyService.calls != 1 {
		t.Fatalf("expected ApplyStaticDiscovery once, got %d", topologyService.calls)
	}
	if topologyService.lastID != deviceID {
		t.Fatalf("expected ApplyStaticDiscovery device %s, got %s", deviceID, topologyService.lastID)
	}
	if topologyService.lastIn.SysName != "edge-sw-1" {
		t.Fatalf("expected static discovery sysName edge-sw-1, got %q", topologyService.lastIn.SysName)
	}
	if len(topologyService.lastIn.Interfaces) != 1 {
		t.Fatalf("expected 1 discovered interface, got %d", len(topologyService.lastIn.Interfaces))
	}
	topologyService.mu.Unlock()

	select {
	case <-topologyNotify:
	default:
		t.Fatal("expected topology notification after topology-changing static poll")
	}

	sched.mu.Lock()
	if len(sched.completions) != 1 {
		t.Fatalf("expected one scheduler completion, got %d", len(sched.completions))
	}
	sched.mu.Unlock()
}

func TestPipelineOrchestratorPrometheusRefreshUpdatesAlertsAndStatus(t *testing.T) {
	deviceID := uuid.New()
	device := domain.Device{
		ID:                   deviceID,
		IP:                   "192.0.2.30",
		PrometheusLabelName:  "instance",
		PrometheusLabelValue: "192.0.2.30",
	}
	cache := newPipelineTestCache([]domain.Device{device}, nil)
	promClient := &fakePrometheusClient{
		alertsByID: map[uuid.UUID][]domain.AlertState{
			deviceID: {{
				Instance:  "192.0.2.30",
				Severity:  "critical",
				AlertName: "DeviceDown",
				State:     "firing",
				Summary:   "device down",
			}},
		},
	}
	settingsRepo := newMockWorkerSettingsRepo()
	if err := settingsRepo.Set(domain.SettingPrometheusURL, "http://prometheus.test"); err != nil {
		t.Fatalf("set prometheus_url: %v", err)
	}
	pipeline := NewPipelineOrchestrator(
		newPipelineTestScheduler(),
		state.NewStore(),
		cache,
		ws.NewHub(),
		newPerformanceTestCollector(t),
		newOperationalTestCollector(t),
		newStaticTestCollector(t),
		collector.NewPrometheusCollector(promClient),
		&fakeTopologyService{},
		settingsRepo,
		make(chan struct{}, 1),
	)

	pipeline.refreshPrometheusOnce(context.Background())

	pipeline.snapshotMu.RLock()
	if len(pipeline.alerts[deviceID]) != 1 {
		t.Fatalf("expected alert cache to update, got %d alert(s)", len(pipeline.alerts[deviceID]))
	}
	pipeline.snapshotMu.RUnlock()
	if !pipeline.IsPromAvailable() {
		t.Fatal("expected prometheus to remain available after successful refresh")
	}

	promClient.mu.Lock()
	promClient.alertsErr = errors.New("prometheus unavailable")
	promClient.mu.Unlock()

	pipeline.refreshPrometheusOnce(context.Background())

	if pipeline.IsPromAvailable() {
		t.Fatal("expected prometheus availability to flip false on refresh failure")
	}
	if pipeline.GetPrometheusStatus().Error == "" {
		t.Fatal("expected prometheus error message to be recorded")
	}
}

func TestPipelineOrchestratorStatusReflectsLifecycle(t *testing.T) {
	sched := newPipelineTestScheduler()
	pipeline := NewPipelineOrchestrator(
		sched,
		state.NewStore(),
		newPipelineTestCache(nil, nil),
		ws.NewHub(),
		newPerformanceTestCollector(t),
		newOperationalTestCollector(t),
		newStaticTestCollector(t),
		collector.NewPrometheusCollector(&fakePrometheusClient{}),
		&fakeTopologyService{},
		newMockWorkerSettingsRepo(),
		make(chan struct{}, 1),
	)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	pipeline.Start(ctx)
	t.Cleanup(pipeline.Stop)

	if pipeline.Status() != "running" {
		t.Fatalf("expected running status after Start, got %q", pipeline.Status())
	}

	pipeline.Stop()

	if pipeline.Status() != "stopped" {
		t.Fatalf("expected stopped status after Stop, got %q", pipeline.Status())
	}
}

func newBroadcastTestPipeline(t *testing.T) (*PipelineOrchestrator, *ws.Hub, *state.Store, chan struct{}, uuid.UUID) {
	t.Helper()

	deviceID := uuid.New()
	device := domain.Device{
		ID:            deviceID,
		IP:            "192.0.2.40",
		Status:        domain.DeviceStatusProbing,
		SysName:       "dist-sw-1",
		HardwareModel: "CRS328-24P-4S+",
		Interfaces: []domain.Interface{
			{IfName: "ether1", IfDescr: "uplink", Speed: 1_000_000_000},
		},
	}
	linkID := uuid.New()
	link := domain.Link{
		ID:             linkID,
		SourceDeviceID: deviceID,
		SourceIfName:   "ether1",
		TargetDeviceID: uuid.New(),
		TargetIfName:   "ether2",
	}

	store := state.NewStore()
	store.Update(state.StateUpdate{
		DeviceID:        deviceID,
		VolatilityClass: domain.VolatilityClassPerformance,
		Metrics: &domain.DeviceMetrics{
			DeviceID:    deviceID,
			CPUPercent:  floatPtr(25),
			CollectedAt: time.Date(2026, 4, 13, 12, 0, 0, 0, time.UTC),
		},
		LinkMetrics: []domain.LinkMetrics{{
			IfName:      "uplink",
			TxBps:       floatPtr(1200),
			RxBps:       floatPtr(2400),
			CollectedAt: time.Date(2026, 4, 13, 12, 0, 0, 0, time.UTC),
		}},
		PollSuccess:      true,
		ExpectedInterval: 30 * time.Second,
		Timestamp:        time.Date(2026, 4, 13, 12, 0, 0, 0, time.UTC),
	})
	store.Update(state.StateUpdate{
		DeviceID:         deviceID,
		VolatilityClass:  domain.VolatilityClassOperational,
		PollSuccess:      true,
		ExpectedInterval: 60 * time.Second,
		Timestamp:        time.Date(2026, 4, 13, 12, 0, 1, 0, time.UTC),
	})

	hub := ws.NewHub()
	topologyNotify := make(chan struct{}, 4)
	pipeline := NewPipelineOrchestrator(
		newPipelineTestScheduler(),
		store,
		newPipelineTestCache([]domain.Device{device}, []domain.Link{link}),
		hub,
		nil,
		nil,
		nil,
		collector.NewPrometheusCollector(&fakePrometheusClient{}),
		&fakeTopologyService{},
		newMockWorkerSettingsRepo(),
		topologyNotify,
	)

	return pipeline, hub, store, topologyNotify, deviceID
}

func broadcastMessageTypes(t *testing.T, raw [][]byte) []string {
	t.Helper()

	types := make([]string, 0, len(raw))
	for _, payload := range raw {
		var message map[string]any
		if err := json.Unmarshal(payload, &message); err != nil {
			t.Fatalf("failed to decode broadcast payload: %v", err)
		}
		if msgType, ok := message["type"].(string); ok {
			types = append(types, msgType)
		}
	}
	return types
}

type wsSnapshotMessage struct {
	Type    string             `json:"type"`
	Payload ws.SnapshotPayload `json:"payload"`
}

func newDetailSubscriptionTestDevice() domain.Device {
	return domain.Device{
		ID:     uuid.New(),
		IP:     "192.0.2.80",
		Status: domain.DeviceStatusUnknown,
		Vendor: "default",
		Interfaces: []domain.Interface{
			{IfName: "ether1", IfDescr: "uplink", Speed: 1_000_000_000},
		},
	}
}

func newDetailSubscriptionTestPipeline(t *testing.T, hub *ws.Hub) *PipelineOrchestrator {
	t.Helper()

	return NewPipelineOrchestrator(
		newPipelineTestScheduler(),
		state.NewStore(),
		nil,
		hub,
		newPerformanceTestCollector(t),
		newOperationalTestCollector(t),
		newStaticTestCollector(t),
		collector.NewPrometheusCollector(&fakePrometheusClient{}),
		&fakeTopologyService{},
		newMockWorkerSettingsRepo(),
		make(chan struct{}, 1),
	)
}

func newDetailSubscriptionTestServer(t *testing.T, hub *ws.Hub) string {
	t.Helper()

	go hub.Run()

	server := httptest.NewServer(ws.NewHandler(
		hub,
		func() *ws.SnapshotPayload { return ws.EmptySnapshot() },
		func() ws.PrometheusStatusPayload {
			return ws.PrometheusStatusPayload{Enabled: true, Available: true}
		},
	))
	t.Cleanup(server.Close)

	return "ws" + strings.TrimPrefix(server.URL, "http")
}

func connectDetailSubscriptionClient(t *testing.T, wsURL string) *websocket.Conn {
	t.Helper()

	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("failed to dial websocket test server: %v", err)
	}
	t.Cleanup(func() {
		_ = conn.Close()
	})

	return conn
}

func drainBootstrapMessages(t *testing.T, conn *websocket.Conn) {
	t.Helper()

	types := make([]string, 0, 2)
	for i := 0; i < 2; i++ {
		conn.SetReadDeadline(time.Now().Add(time.Second))
		_, raw, err := conn.ReadMessage()
		if err != nil {
			t.Fatalf("failed to read bootstrap websocket message: %v", err)
		}

		var message struct {
			Type string `json:"type"`
		}
		if err := json.Unmarshal(raw, &message); err != nil {
			t.Fatalf("failed to decode bootstrap websocket message: %v", err)
		}
		types = append(types, message.Type)
	}
	conn.SetReadDeadline(time.Time{})

	if len(types) != 2 || types[0] != ws.MessageTypeSnapshot || types[1] != ws.MessageTypePrometheusStatus {
		t.Fatalf("unexpected bootstrap message order: %v", types)
	}
}

func subscribeDetail(t *testing.T, conn *websocket.Conn, deviceID uuid.UUID) {
	t.Helper()

	if err := conn.WriteJSON(map[string]any{
		"type": ws.MessageTypeSubscribeDetail,
		"payload": map[string]string{
			"device_id": deviceID.String(),
		},
	}); err != nil {
		t.Fatalf("failed to send subscribe_detail message: %v", err)
	}
}

func waitForDetailSubscribers(t *testing.T, hub *ws.Hub, deviceID uuid.UUID, want int) {
	t.Helper()

	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if len(hub.DetailSubscribers(deviceID)) == want {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}

	t.Fatalf("expected %d detail subscribers for %s, got %d", want, deviceID, len(hub.DetailSubscribers(deviceID)))
}

func readSnapshotDeltaMessage(t *testing.T, conn *websocket.Conn) wsSnapshotMessage {
	t.Helper()

	conn.SetReadDeadline(time.Now().Add(time.Second))
	defer conn.SetReadDeadline(time.Time{})

	_, raw, err := conn.ReadMessage()
	if err != nil {
		t.Fatalf("failed to read websocket detail delta: %v", err)
	}

	var message wsSnapshotMessage
	if err := json.Unmarshal(raw, &message); err != nil {
		t.Fatalf("failed to decode websocket detail delta: %v", err)
	}
	if message.Type != ws.MessageTypeSnapshotDelta {
		t.Fatalf("message Type = %q, want %q", message.Type, ws.MessageTypeSnapshotDelta)
	}

	return message
}

func assertNoWebSocketMessage(t *testing.T, conn *websocket.Conn) {
	t.Helper()

	conn.SetReadDeadline(time.Now().Add(200 * time.Millisecond))
	defer conn.SetReadDeadline(time.Time{})

	if _, _, err := conn.ReadMessage(); err == nil {
		t.Fatal("expected websocket read to time out, but a message arrived")
	} else if netErr, ok := err.(net.Error); !ok || !netErr.Timeout() {
		t.Fatalf("expected websocket timeout, got %v", err)
	}
}

func TestPipelineOrchestratorBroadcastLoopSendsSnapshotThenDelta(t *testing.T) {
	pipeline, hub, store, _, deviceID := newBroadcastTestPipeline(t)

	pipeline.broadcastOnce(context.Background())

	firstTypes := broadcastMessageTypes(t, drainBroadcastCh(hub))
	if len(firstTypes) == 0 || firstTypes[0] != ws.MessageTypeSnapshot {
		t.Fatalf("expected first broadcast to be snapshot, got %v", firstTypes)
	}

	store.Update(state.StateUpdate{
		DeviceID:        deviceID,
		VolatilityClass: domain.VolatilityClassPerformance,
		Metrics: &domain.DeviceMetrics{
			DeviceID:    deviceID,
			CPUPercent:  floatPtr(50),
			CollectedAt: time.Date(2026, 4, 13, 12, 0, 5, 0, time.UTC),
		},
		PollSuccess:      true,
		ExpectedInterval: 30 * time.Second,
		Timestamp:        time.Date(2026, 4, 13, 12, 0, 5, 0, time.UTC),
	})

	pipeline.broadcastOnce(context.Background())

	secondTypes := broadcastMessageTypes(t, drainBroadcastCh(hub))
	if len(secondTypes) == 0 || secondTypes[0] != ws.MessageTypeSnapshotDelta {
		t.Fatalf("expected second broadcast to be snapshot_delta, got %v", secondTypes)
	}
}

func TestPipelineOrchestratorBroadcastOnce_MixedTierPollsKeepPerformanceFreshnessMetadata(t *testing.T) {
	deviceID := uuid.New()
	device := domain.Device{
		ID:            deviceID,
		IP:            "192.0.2.41",
		Status:        domain.DeviceStatusProbing,
		SysName:       "dist-sw-2",
		HardwareModel: "CRS326-24G-2S+",
		Vendor:        "default",
		Interfaces: []domain.Interface{
			{IfName: "ether1", IfDescr: "uplink", Speed: 1_000_000_000},
		},
	}
	link := domain.Link{
		ID:             uuid.New(),
		SourceDeviceID: deviceID,
		SourceIfName:   "ether1",
		TargetDeviceID: uuid.New(),
		TargetIfName:   "ether2",
	}

	hub := ws.NewHub()
	store := state.NewStore()
	pipeline := NewPipelineOrchestrator(
		newPipelineTestScheduler(),
		store,
		newPipelineTestCache([]domain.Device{device}, []domain.Link{link}),
		hub,
		newPerformanceTestCollector(t),
		newOperationalTestCollector(t),
		newStaticTestCollector(t),
		collector.NewPrometheusCollector(&fakePrometheusClient{}),
		&fakeTopologyService{},
		newMockWorkerSettingsRepo(),
		make(chan struct{}, 1),
	)

	pipeline.runTask(context.Background(), scheduler.PollTask{
		RunID:            10,
		Key:              scheduler.NewTaskKey(deviceID, domain.VolatilityClassPerformance),
		VolatilityClass:  domain.VolatilityClassPerformance,
		ExpectedInterval: 30 * time.Second,
		Device:           device,
	})

	performanceState, ok := store.GetDevice(deviceID)
	if !ok {
		t.Fatal("expected performance poll to seed device state")
	}
	performanceLastPolledAt := performanceState.LastPolledAt.UTC().Format(time.RFC3339)

	pipeline.runTask(context.Background(), scheduler.PollTask{
		RunID:            11,
		Key:              scheduler.NewTaskKey(deviceID, domain.VolatilityClassOperational),
		VolatilityClass:  domain.VolatilityClassOperational,
		ExpectedInterval: 60 * time.Second,
		Device:           device,
	})
	pipeline.runTask(context.Background(), scheduler.PollTask{
		RunID:            12,
		Key:              scheduler.NewTaskKey(deviceID, domain.VolatilityClassStatic),
		VolatilityClass:  domain.VolatilityClassStatic,
		ExpectedInterval: 300 * time.Second,
		Device:           device,
	})

	pipeline.broadcastOnce(context.Background())

	var snapshotMessage wsSnapshotMessage
	foundSnapshot := false
	for _, raw := range drainBroadcastCh(hub) {
		if err := json.Unmarshal(raw, &snapshotMessage); err != nil {
			t.Fatalf("failed to decode broadcast snapshot message: %v", err)
		}
		if snapshotMessage.Type == ws.MessageTypeSnapshot {
			foundSnapshot = true
			break
		}
	}
	if !foundSnapshot {
		t.Fatal("expected mixed-tier broadcast to send snapshot payload")
	}

	metric, ok := snapshotMessage.Payload.DeviceMetrics[deviceID.String()]
	if !ok {
		t.Fatalf("expected snapshot metric for device %s", deviceID)
	}
	if metric.LastPolledAt != performanceLastPolledAt {
		t.Fatalf("LastPolledAt = %q, want performance poll timestamp %q", metric.LastPolledAt, performanceLastPolledAt)
	}
	if metric.ExpectedPollIntervalSeconds == nil || *metric.ExpectedPollIntervalSeconds != 30 {
		t.Fatalf("ExpectedPollIntervalSeconds = %#v, want 30", metric.ExpectedPollIntervalSeconds)
	}
	if metric.Reachability != string(state.ReachabilityUp) {
		t.Fatalf("Reachability = %q, want %q", metric.Reachability, state.ReachabilityUp)
	}
	if snapshotMessage.Payload.DeviceStatuses[deviceID.String()] != string(domain.DeviceStatusUp) {
		t.Fatalf("DeviceStatus = %q, want %q", snapshotMessage.Payload.DeviceStatuses[deviceID.String()], domain.DeviceStatusUp)
	}
}

func TestPipelineOrchestratorTopologyChangedFiresAfterSnapshot(t *testing.T) {
	pipeline, hub, _, topologyNotify, _ := newBroadcastTestPipeline(t)
	topologyNotify <- struct{}{}

	pipeline.broadcastOnce(context.Background())

	types := broadcastMessageTypes(t, drainBroadcastCh(hub))
	if len(types) < 2 {
		t.Fatalf("expected snapshot and topology_changed messages, got %v", types)
	}
	if types[0] != ws.MessageTypeSnapshot || types[1] != ws.MessageTypeTopologyChanged {
		t.Fatalf("expected snapshot before topology_changed, got %v", types)
	}
}

func TestPipelineOrchestratorTopologyChangedForcesSnapshotWhenOverviewDeltaNil(t *testing.T) {
	pipeline, hub, _, topologyNotify, _ := newBroadcastTestPipeline(t)

	pipeline.broadcastOnce(context.Background())
	drainBroadcastCh(hub)

	topologyNotify <- struct{}{}
	pipeline.broadcastOnce(context.Background())

	types := broadcastMessageTypes(t, drainBroadcastCh(hub))
	if len(types) < 2 {
		t.Fatalf("expected forced snapshot and topology_changed, got %v", types)
	}
	if types[0] != ws.MessageTypeSnapshot {
		t.Fatalf("expected forced full snapshot before topology_changed, got %v", types)
	}
	if types[1] != ws.MessageTypeTopologyChanged {
		t.Fatalf("expected topology_changed after forced snapshot, got %v", types)
	}
}

func TestPipelineOrchestratorPrometheusStatusOnlyBroadcastsOnTransition(t *testing.T) {
	pipeline, hub, _, _, _ := newBroadcastTestPipeline(t)

	pipeline.publishPrometheusStatus(ws.PrometheusStatusPayload{})
	if messages := drainBroadcastCh(hub); len(messages) != 0 {
		t.Fatalf("expected no broadcast for unchanged disabled state, got %d message(s)", len(messages))
	}

	pipeline.publishPrometheusStatus(ws.PrometheusStatusPayload{
		Enabled:   true,
		Available: false,
		Error:     "prometheus down",
	})
	firstTypes := broadcastMessageTypes(t, drainBroadcastCh(hub))
	if len(firstTypes) != 1 || firstTypes[0] != ws.MessageTypePrometheusStatus {
		t.Fatalf("expected one prometheus_status message on failure transition, got %v", firstTypes)
	}

	pipeline.publishPrometheusStatus(ws.PrometheusStatusPayload{
		Enabled:   true,
		Available: false,
		Error:     "prometheus down",
	})
	if messages := drainBroadcastCh(hub); len(messages) != 0 {
		t.Fatalf("expected no duplicate broadcast without state transition, got %d message(s)", len(messages))
	}

	pipeline.publishPrometheusStatus(ws.PrometheusStatusPayload{
		Enabled:   true,
		Available: true,
	})
	secondTypes := broadcastMessageTypes(t, drainBroadcastCh(hub))
	if len(secondTypes) != 1 || secondTypes[0] != ws.MessageTypePrometheusStatus {
		t.Fatalf("expected one prometheus_status message on recovery transition, got %v", secondTypes)
	}
}

func TestPipelineOrchestratorRunTask_PerformancePollSendsOnlySelectedDeviceLinkMetricsToSubscribedClient(t *testing.T) {
	hub := ws.NewHub()
	pipeline := newDetailSubscriptionTestPipeline(t, hub)
	device := newDetailSubscriptionTestDevice()
	pipeline.stateStore.Update(state.StateUpdate{
		DeviceID:         device.ID,
		VolatilityClass:  domain.VolatilityClassOperational,
		PollSuccess:      true,
		ExpectedInterval: 30 * time.Second,
		Timestamp:        time.Now().Add(-time.Second),
	})
	pipeline.prevCounters[device.ID] = map[string]collector.CounterBaseline{
		"ether1": {
			InOctets:  1_000,
			OutOctets: 2_000,
			SampledAt: time.Now().Add(-30 * time.Second),
		},
	}

	wsURL := newDetailSubscriptionTestServer(t, hub)
	subscriber := connectDetailSubscriptionClient(t, wsURL)
	drainBootstrapMessages(t, subscriber)
	subscribeDetail(t, subscriber, device.ID)
	waitForDetailSubscribers(t, hub, device.ID, 1)

	pipeline.runTask(context.Background(), scheduler.PollTask{
		RunID:            1,
		Key:              scheduler.NewTaskKey(device.ID, domain.VolatilityClassPerformance),
		VolatilityClass:  domain.VolatilityClassPerformance,
		ExpectedInterval: 30 * time.Second,
		Device:           device,
	})

	message := readSnapshotDeltaMessage(t, subscriber)
	metric, ok := message.Payload.DeviceMetrics[device.ID.String()]
	if !ok {
		t.Fatalf("expected detail delta for device %s", device.ID)
	}
	if len(message.Payload.LinkMetrics) != 1 {
		t.Fatalf("expected targeted detail delta to contain 1 link_metrics key, got %d", len(message.Payload.LinkMetrics))
	}
	linkMetrics, ok := message.Payload.LinkMetrics[device.ID.String()]
	if !ok {
		t.Fatalf("expected targeted detail delta link_metrics for device %s", device.ID)
	}
	if len(linkMetrics) != 1 {
		t.Fatalf("expected 1 targeted link metric for device %s, got %d", device.ID, len(linkMetrics))
	}
	if linkMetrics[0].DeviceID != device.ID.String() {
		t.Fatalf("LinkMetrics[%s][0].DeviceID = %q, want %q", device.ID, linkMetrics[0].DeviceID, device.ID)
	}
	if linkMetrics[0].IfName != "ether1" {
		t.Fatalf("LinkMetrics[%s][0].IfName = %q, want ether1", device.ID, linkMetrics[0].IfName)
	}
	if linkMetrics[0].TxBps == nil {
		t.Fatalf("LinkMetrics[%s][0].TxBps = nil, want value", device.ID)
	}
	if linkMetrics[0].RxBps == nil {
		t.Fatalf("LinkMetrics[%s][0].RxBps = nil, want value", device.ID)
	}
	if metric.Health == "" {
		t.Fatal("expected health field in detail delta")
	}
	if metric.Reachability != string(state.ReachabilityUp) {
		t.Fatalf("Reachability = %q, want %q", metric.Reachability, state.ReachabilityUp)
	}
	if metric.LastPolledAt == "" {
		t.Fatal("expected last_polled_at in detail delta")
	}
	if metric.ExpectedPollIntervalSeconds == nil || *metric.ExpectedPollIntervalSeconds != 30 {
		t.Fatalf("ExpectedPollIntervalSeconds = %#v, want 30", metric.ExpectedPollIntervalSeconds)
	}
	if messages := drainBroadcastCh(hub); len(messages) != 0 {
		t.Fatalf("expected no overview broadcast during targeted detail send, got %d message(s)", len(messages))
	}
}

func assertOperationalDetailDeltaKeepsPerformanceFreshnessMetadata(t *testing.T) {
	t.Helper()

	hub := ws.NewHub()
	pipeline := newDetailSubscriptionTestPipeline(t, hub)
	device := newDetailSubscriptionTestDevice()
	pipeline.prevCounters[device.ID] = map[string]collector.CounterBaseline{
		"ether1": {
			InOctets:  1_000,
			OutOctets: 2_000,
			SampledAt: time.Now().Add(-30 * time.Second),
		},
	}

	wsURL := newDetailSubscriptionTestServer(t, hub)
	subscriber := connectDetailSubscriptionClient(t, wsURL)
	drainBootstrapMessages(t, subscriber)
	subscribeDetail(t, subscriber, device.ID)
	waitForDetailSubscribers(t, hub, device.ID, 1)

	pipeline.runTask(context.Background(), scheduler.PollTask{
		RunID:            20,
		Key:              scheduler.NewTaskKey(device.ID, domain.VolatilityClassPerformance),
		VolatilityClass:  domain.VolatilityClassPerformance,
		ExpectedInterval: 30 * time.Second,
		Device:           device,
	})

	performanceState, ok := pipeline.stateStore.GetDevice(device.ID)
	if !ok {
		t.Fatalf("expected performance task to seed device state for %s", device.ID)
	}
	performanceLastPolledAt := performanceState.LastPolledAt.UTC().Format(time.RFC3339)

	performanceMessage := readSnapshotDeltaMessage(t, subscriber)
	performanceMetric, ok := performanceMessage.Payload.DeviceMetrics[device.ID.String()]
	if !ok {
		t.Fatalf("expected performance detail delta for device %s", device.ID)
	}
	if performanceMetric.ExpectedPollIntervalSeconds == nil || *performanceMetric.ExpectedPollIntervalSeconds != 30 {
		t.Fatalf("ExpectedPollIntervalSeconds = %#v, want 30", performanceMetric.ExpectedPollIntervalSeconds)
	}

	pipeline.runTask(context.Background(), scheduler.PollTask{
		RunID:            21,
		Key:              scheduler.NewTaskKey(device.ID, domain.VolatilityClassOperational),
		VolatilityClass:  domain.VolatilityClassOperational,
		ExpectedInterval: 60 * time.Second,
		Device:           device,
	})

	message := readSnapshotDeltaMessage(t, subscriber)
	metric, ok := message.Payload.DeviceMetrics[device.ID.String()]
	if !ok {
		t.Fatalf("expected detail delta for device %s", device.ID)
	}
	linkMetrics, ok := message.Payload.LinkMetrics[device.ID.String()]
	if !ok {
		t.Fatalf("expected operational detail delta link_metrics for device %s", device.ID)
	}
	if len(linkMetrics) != 1 {
		t.Fatalf("expected operational detail delta to keep 1 link metric for %s, got %d", device.ID, len(linkMetrics))
	}
	if linkMetrics[0].IfName != "ether1" {
		t.Fatalf("LinkMetrics[%s][0].IfName = %q, want ether1", device.ID, linkMetrics[0].IfName)
	}
	if linkMetrics[0].TxBps == nil {
		t.Fatalf("LinkMetrics[%s][0].TxBps = nil, want value", device.ID)
	}
	if linkMetrics[0].RxBps == nil {
		t.Fatalf("LinkMetrics[%s][0].RxBps = nil, want value", device.ID)
	}
	if metric.Reachability != string(state.ReachabilityUp) {
		t.Fatalf("Reachability = %q, want %q", metric.Reachability, state.ReachabilityUp)
	}
	if metric.LastPolledAt != performanceLastPolledAt {
		t.Fatalf("LastPolledAt = %q, want performance poll timestamp %q", metric.LastPolledAt, performanceLastPolledAt)
	}
	if metric.ExpectedPollIntervalSeconds == nil || *metric.ExpectedPollIntervalSeconds != 30 {
		t.Fatalf("ExpectedPollIntervalSeconds = %#v, want 30", metric.ExpectedPollIntervalSeconds)
	}
}

func TestPipelineOrchestratorRunTask_DetailDeltaKeepsPerformanceFreshnessMetadataAfterOperationalPoll(t *testing.T) {
	assertOperationalDetailDeltaKeepsPerformanceFreshnessMetadata(t)
}

func TestPipelineOrchestratorRunTask_OperationalPollSendsDetailDeltaToSubscribedClient(t *testing.T) {
	assertOperationalDetailDeltaKeepsPerformanceFreshnessMetadata(t)
}

func TestPipelineOrchestratorRunTask_DetailDeltaDoesNotReachUnsubscribedClient(t *testing.T) {
	hub := ws.NewHub()
	pipeline := newDetailSubscriptionTestPipeline(t, hub)
	device := newDetailSubscriptionTestDevice()
	pipeline.stateStore.Update(state.StateUpdate{
		DeviceID:         device.ID,
		VolatilityClass:  domain.VolatilityClassOperational,
		PollSuccess:      true,
		ExpectedInterval: 30 * time.Second,
		Timestamp:        time.Now().Add(-time.Second),
	})
	pipeline.prevCounters[device.ID] = map[string]collector.CounterBaseline{
		"ether1": {
			InOctets:  1_000,
			OutOctets: 2_000,
			SampledAt: time.Now().Add(-30 * time.Second),
		},
	}

	wsURL := newDetailSubscriptionTestServer(t, hub)
	subscriber := connectDetailSubscriptionClient(t, wsURL)
	unsubscribed := connectDetailSubscriptionClient(t, wsURL)
	drainBootstrapMessages(t, subscriber)
	drainBootstrapMessages(t, unsubscribed)
	subscribeDetail(t, subscriber, device.ID)
	waitForDetailSubscribers(t, hub, device.ID, 1)

	pipeline.runTask(context.Background(), scheduler.PollTask{
		RunID:            3,
		Key:              scheduler.NewTaskKey(device.ID, domain.VolatilityClassPerformance),
		VolatilityClass:  domain.VolatilityClassPerformance,
		ExpectedInterval: 30 * time.Second,
		Device:           device,
	})

	message := readSnapshotDeltaMessage(t, subscriber)
	if _, ok := message.Payload.LinkMetrics[device.ID.String()]; !ok {
		t.Fatalf("expected subscribed client detail delta link_metrics for device %s", device.ID)
	}
	assertNoWebSocketMessage(t, unsubscribed)
}
