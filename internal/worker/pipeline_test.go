package worker

// This file exercises pipeline behavior so refactors preserve the documented contract.

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
	"github.com/lollinoo/theia/internal/observability"
	"github.com/lollinoo/theia/internal/polling"
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
	startErr    error
	health      polling.HealthSnapshot
}

func newPipelineTestScheduler() *pipelineTestScheduler {
	return &pipelineTestScheduler{
		status: "stopped",
		tasks:  make(chan scheduler.PollTask, 16),
	}
}

func (s *pipelineTestScheduler) Start(context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.startErr != nil {
		return s.startErr
	}
	s.status = "running"
	s.startCalls++
	return nil
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

func (s *pipelineTestScheduler) PollingHealth() polling.HealthSnapshot {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.health
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

func TestPipelineTaskRunnerPersistStaticDiscoveryPropagatesNeighborDiscoveryFailures(t *testing.T) {
	deviceID := uuid.New()
	topologyService := &fakeTopologyService{}
	pipeline := &PipelineOrchestrator{topologyService: topologyService}
	runner := &pipelineTaskRunner{pipeline: pipeline}
	failures := []snmp.NeighborDiscoveryFailure{
		{
			Protocol: domain.DiscoveryProtocolCDP,
			OID:      snmp.OidCDPDeviceID,
			Critical: true,
			Error:    "cdp walk failed",
		},
	}

	runner.persistStaticDiscovery(domain.Device{ID: deviceID}, collector.StaticResult{
		SysName:                    "edge-sw",
		NeighborDiscoveryProtocols: []domain.DiscoveryProtocol{domain.DiscoveryProtocolLLDP, domain.DiscoveryProtocolCDP},
		NeighborDiscoveryFailures:  append([]snmp.NeighborDiscoveryFailure(nil), failures...),
	})

	topologyService.mu.Lock()
	defer topologyService.mu.Unlock()
	if topologyService.calls != 1 {
		t.Fatalf("expected ApplyStaticDiscovery once, got %d", topologyService.calls)
	}
	if topologyService.lastID != deviceID {
		t.Fatalf("device ID = %s, want %s", topologyService.lastID, deviceID)
	}
	if len(topologyService.lastIn.NeighborDiscoveryFailures) != 1 {
		t.Fatalf("failure count = %d, want 1", len(topologyService.lastIn.NeighborDiscoveryFailures))
	}
	if len(topologyService.lastIn.NeighborDiscoveryProtocols) != 2 ||
		topologyService.lastIn.NeighborDiscoveryProtocols[0] != domain.DiscoveryProtocolLLDP ||
		topologyService.lastIn.NeighborDiscoveryProtocols[1] != domain.DiscoveryProtocolCDP {
		t.Fatalf("protocols = %v, want [lldp cdp]", topologyService.lastIn.NeighborDiscoveryProtocols)
	}
	failure := topologyService.lastIn.NeighborDiscoveryFailures[0]
	if failure.Protocol != domain.DiscoveryProtocolCDP || failure.OID != snmp.OidCDPDeviceID || !failure.Critical || failure.Error != "cdp walk failed" {
		t.Fatalf("propagated failure = %#v, want CDP device ID critical failure", failure)
	}
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
	connectDelay  time.Duration
}

type spyPipelineTaskRunner struct {
	runTaskCalls []scheduler.PollTask
}

func (s *spyPipelineTaskRunner) runWorker(context.Context) {}

func (s *spyPipelineTaskRunner) runTask(_ context.Context, task scheduler.PollTask) {
	s.runTaskCalls = append(s.runTaskCalls, task)
}

func (s *spyPipelineTaskRunner) topologyDiscoveryMode(domain.Device) domain.TopologyDiscoveryMode {
	return domain.TopologyDiscoveryModeOff
}

func (s *spyPipelineTaskRunner) publishSubscribedDetailDelta(domain.Device) {}

func (c *fakeSNMPClient) Connect() error {
	if c.connectDelay > 0 {
		time.Sleep(c.connectDelay)
	}
	return nil
}

func (c *fakeSNMPClient) Close() error { return nil }

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

func TestPipelineRunsEssentialTaskWithEssentialTimeoutProfile(t *testing.T) {
	device := domain.Device{
		ID:        uuid.New(),
		Hostname:  "edge-essential",
		IP:        "10.0.0.1",
		Managed:   true,
		PollClass: domain.PollClassCore,
	}
	stateStore := state.NewStore()
	var gotTimeout time.Duration
	gotRetries := -1
	settingsRepo := newMockWorkerSettingsRepo()
	_ = settingsRepo.Set(domain.SettingPollingEssentialTimeoutMillis, "800")
	_ = settingsRepo.Set(domain.SettingPollingEssentialRetries, "0")
	pipeline := NewPipelineOrchestrator(nil, stateStore, nil, nil, nil, nil, nil, nil, nil, nil, settingsRepo, nil, nil, nil)
	pipeline.essential = collector.NewEssentialCollector(buildEmptyVendorRegistry(), func(_ string, _ domain.SNMPCredentials, timeout time.Duration, retries int) (collector.SNMPClient, error) {
		gotTimeout = timeout
		gotRetries = retries
		return &fakeSNMPClient{}, nil
	})

	task := scheduler.PollTask{
		Key:              scheduler.NewEssentialTaskKey(device.ID),
		Kind:             polling.TaskKindEssential,
		Lane:             polling.LaneEssential,
		Device:           device,
		ExpectedInterval: 10 * time.Second,
	}

	pipeline.runTask(context.Background(), task)
	if gotTimeout != 800*time.Millisecond {
		t.Fatalf("essential timeout = %v, want 800ms", gotTimeout)
	}
	if gotRetries != 0 {
		t.Fatalf("essential retries = %d, want 0", gotRetries)
	}
	if _, ok := stateStore.GetDevice(device.ID); !ok {
		t.Fatalf("expected essential task to update state")
	}
}

func TestPipelineRunsPerformanceTaskWithBackgroundTimeoutProfile(t *testing.T) {
	device := domain.Device{
		ID:            uuid.New(),
		Hostname:      "edge-performance",
		IP:            "10.0.0.2",
		Managed:       true,
		PollClass:     domain.PollClassCore,
		MetricsSource: domain.MetricsSourceSNMP,
		Vendor:        "default",
	}
	stateStore := state.NewStore()
	var gotTimeout time.Duration
	gotRetries := -1
	settingsRepo := newMockWorkerSettingsRepo()
	_ = settingsRepo.Set(domain.SettingSNMPTimeout, "10")
	_ = settingsRepo.Set(domain.SettingSNMPRetries, "2")
	pipeline := NewPipelineOrchestrator(nil, stateStore, nil, nil, nil, nil, nil, nil, nil, nil, settingsRepo, nil, nil, nil)
	pipeline.performance = collector.NewPerformanceCollector(buildEmptyVendorRegistry(), func(_ string, _ domain.SNMPCredentials, timeout time.Duration, retries int) (collector.SNMPClient, error) {
		gotTimeout = timeout
		gotRetries = retries
		return &fakeSNMPClient{
			getResponses: map[string][]gosnmp.SnmpPDU{
				snmp.OidSysUpTime: {{Name: snmp.OidSysUpTime, Value: uint32(3_000)}},
			},
		}, nil
	})

	task := scheduler.PollTask{
		Key:              scheduler.NewTaskKey(device.ID, domain.VolatilityClassPerformance),
		Kind:             polling.TaskKindBackground,
		Lane:             polling.LaneBackground,
		VolatilityClass:  domain.VolatilityClassPerformance,
		Device:           device,
		ExpectedInterval: 30 * time.Second,
	}

	pipeline.runTask(context.Background(), task)
	if gotTimeout != 5*time.Second {
		t.Fatalf("performance timeout = %v, want 5s background profile", gotTimeout)
	}
	if gotRetries != 1 {
		t.Fatalf("performance retries = %d, want 1 background retry", gotRetries)
	}
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
		ws.NewHub(ws.WithBroadcastRecorder()),
		nil,
		newPerformanceTestCollector(t),
		newOperationalTestCollector(t),
		newStaticTestCollector(t),
		collector.NewPrometheusCollector(promClient),
		&fakeTopologyService{},
		settingsRepo,
		make(chan struct{}, 1),
		nil,
		nil,
	)
	pipeline.runtime.prevCounters[deviceID] = map[string]collector.CounterBaseline{
		"ether1": {
			InOctets:  1_000,
			OutOctets: 2_000,
			SampledAt: time.Now().Add(-30 * time.Second),
		},
	}

	pipeline.taskRunner.runTask(context.Background(), task)

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

	pipeline.runtime.mu.RLock()
	hostname := pipeline.runtime.hostnames[deviceID]
	pipeline.runtime.mu.RUnlock()
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

func TestPipelineOrchestratorPerformanceTaskCompletionUsesWallClockFinish(t *testing.T) {
	deviceID := uuid.New()
	task := scheduler.PollTask{
		RunID:            17,
		Key:              scheduler.NewTaskKey(deviceID, domain.VolatilityClassPerformance),
		VolatilityClass:  domain.VolatilityClassPerformance,
		ExpectedInterval: 30 * time.Second,
		Device: domain.Device{
			ID:                  deviceID,
			IP:                  "192.0.2.17",
			Managed:             true,
			Status:              domain.DeviceStatusUnknown,
			MetricsSource:       domain.MetricsSourceSNMP,
			Vendor:              "default",
			PrometheusLabelName: "instance",
			SNMPCredentials: domain.SNMPCredentials{
				Version: domain.SNMPVersionV2c,
				V2c:     &domain.SNMPv2cCredentials{Community: "public"},
			},
		},
	}
	client := &fakeSNMPClient{
		connectDelay: 30 * time.Millisecond,
		getResponses: map[string][]gosnmp.SnmpPDU{
			snmp.OidSysUpTime: {{Name: snmp.OidSysUpTime, Value: uint32(3_000)}},
		},
		walkResponses: map[string][]gosnmp.SnmpPDU{
			snmp.OidHrProcessorLoad: {
				{Name: snmp.OidHrProcessorLoad + ".1", Value: 55},
			},
		},
	}
	performance := collector.NewPerformanceCollector(buildEmptyVendorRegistry(), func(string, domain.SNMPCredentials, time.Duration, int) (collector.SNMPClient, error) {
		return client, nil
	})
	sched := newPipelineTestScheduler()
	pipeline := NewPipelineOrchestrator(
		sched,
		state.NewStore(),
		newPipelineTestCache([]domain.Device{task.Device}, nil),
		ws.NewHub(ws.WithBroadcastRecorder()),
		nil,
		performance,
		newOperationalTestCollector(t),
		newStaticTestCollector(t),
		collector.NewPrometheusCollector(&fakePrometheusClient{}),
		&fakeTopologyService{},
		newMockWorkerSettingsRepo(),
		make(chan struct{}, 1),
		nil,
		nil,
	)

	startedAt := time.Now().UTC()
	pipeline.taskRunner.runTask(context.Background(), task)

	sched.mu.Lock()
	defer sched.mu.Unlock()
	if len(sched.completions) != 1 {
		t.Fatalf("expected one scheduler completion, got %d", len(sched.completions))
	}
	if !sched.completions[0].FinishedAt.After(startedAt.Add(20 * time.Millisecond)) {
		t.Fatalf("FinishedAt = %s, want wall-clock completion after delayed SNMP connect", sched.completions[0].FinishedAt.Format(time.RFC3339Nano))
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
		ws.NewHub(ws.WithBroadcastRecorder()),
		nil,
		newPerformanceTestCollector(t),
		newOperationalTestCollector(t),
		newStaticTestCollector(t),
		collector.NewPrometheusCollector(&fakePrometheusClient{}),
		topologyService,
		newMockWorkerSettingsRepo(),
		topologyNotify,
		nil,
		nil,
	)

	pipeline.taskRunner.runTask(context.Background(), task)

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

func TestPipelineOrchestratorBootstrapTaskUsesBootstrapLaneAndPersistsTopology(t *testing.T) {
	deviceID := uuid.New()
	task := scheduler.PollTask{
		RunID:            78,
		Key:              scheduler.NewBootstrapTaskKey(deviceID),
		Kind:             polling.TaskKindBootstrap,
		Lane:             polling.LaneBootstrap,
		VolatilityClass:  domain.VolatilityClassStatic,
		ExpectedInterval: domain.StaticClassInterval,
		Device: domain.Device{
			ID:                     deviceID,
			IP:                     "192.0.2.21",
			Status:                 domain.DeviceStatusProbing,
			Vendor:                 "default",
			TopologyDiscoveryMode:  domain.TopologyDiscoveryModeBootstrapOnce,
			TopologyBootstrapState: domain.TopologyBootstrapStatePending,
			SNMPCredentials: domain.SNMPCredentials{
				Version: domain.SNMPVersionV2c,
				V2c:     &domain.SNMPv2cCredentials{Community: "public"},
			},
		},
	}

	var gotTimeout time.Duration
	var gotRetries int
	staticCollector := collector.NewStaticCollector(buildEmptyVendorRegistry(), func(_ string, _ domain.SNMPCredentials, timeout time.Duration, retries int) (collector.SNMPClient, error) {
		gotTimeout = timeout
		gotRetries = retries
		return &fakeSNMPClient{
			getResponses: map[string][]gosnmp.SnmpPDU{
				snmp.OidSysName:     {{Name: snmp.OidSysName, Value: "edge-sw-bootstrap"}},
				snmp.OidSysDescr:    {{Name: snmp.OidSysDescr, Value: "SwitchOS bootstrap"}},
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
		}, nil
	})
	sched := newPipelineTestScheduler()
	store := state.NewStore()
	topologyNotify := make(chan struct{}, 1)
	topologyService := &fakeTopologyService{
		result: service.StaticPersistenceResult{TopologyChanged: true, LinksCreated: 1},
	}
	pipeline := NewPipelineOrchestrator(
		sched,
		store,
		nil,
		ws.NewHub(ws.WithBroadcastRecorder()),
		nil,
		nil,
		nil,
		staticCollector,
		nil,
		topologyService,
		newMockWorkerSettingsRepo(),
		topologyNotify,
		nil,
		nil,
	)

	pipeline.taskRunner.runTask(context.Background(), task)

	if gotTimeout != 10*time.Second {
		t.Fatalf("bootstrap timeout = %s, want 10s", gotTimeout)
	}
	if gotRetries != 1 {
		t.Fatalf("bootstrap retries = %d, want 1", gotRetries)
	}
	topologyService.mu.Lock()
	if topologyService.calls != 1 {
		t.Fatalf("expected ApplyStaticDiscovery once, got %d", topologyService.calls)
	}
	if topologyService.lastIn.SysName != "edge-sw-bootstrap" {
		t.Fatalf("expected bootstrap sysName edge-sw-bootstrap, got %q", topologyService.lastIn.SysName)
	}
	if len(topologyService.lastIn.Neighbors) != 1 {
		t.Fatalf("expected bootstrap topology discovery to include 1 neighbor, got %d", len(topologyService.lastIn.Neighbors))
	}
	topologyService.mu.Unlock()
	select {
	case <-topologyNotify:
	default:
		t.Fatal("expected topology notification after bootstrap discovery")
	}
}

func TestPipelineOrchestratorTopologyDiscoveryMode_TreatsBootstrapOnceAsOffForRegularPolls(t *testing.T) {
	settingsRepo := newMockWorkerSettingsRepo()
	if err := settingsRepo.Set(domain.SettingTopologyDiscoveryDefaultMode, string(domain.TopologyDiscoveryModeBootstrapOnce)); err != nil {
		t.Fatalf("Set setting failed: %v", err)
	}

	pipeline := NewPipelineOrchestrator(nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, settingsRepo, nil, nil, nil)

	tests := []struct {
		name   string
		device domain.Device
		want   domain.TopologyDiscoveryMode
	}{
		{
			name: "default bootstrap once is disabled for periodic polls",
			device: domain.Device{
				TopologyDiscoveryMode:  domain.TopologyDiscoveryModeInherit,
				TopologyBootstrapState: domain.TopologyBootstrapStatePending,
			},
			want: domain.TopologyDiscoveryModeOff,
		},
		{
			name: "explicit bootstrap once is disabled for periodic polls",
			device: domain.Device{
				TopologyDiscoveryMode:  domain.TopologyDiscoveryModeBootstrapOnce,
				TopologyBootstrapState: domain.TopologyBootstrapStatePending,
			},
			want: domain.TopologyDiscoveryModeOff,
		},
		{
			name: "continuous lldp mode is preserved",
			device: domain.Device{
				TopologyDiscoveryMode: domain.TopologyDiscoveryModeLLDP,
			},
			want: domain.TopologyDiscoveryModeLLDP,
		},
		{
			name: "continuous lldp cdp mode is preserved",
			device: domain.Device{
				TopologyDiscoveryMode: domain.TopologyDiscoveryModeLLDPCDP,
			},
			want: domain.TopologyDiscoveryModeLLDPCDP,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := pipeline.taskRunner.topologyDiscoveryMode(tt.device); got != tt.want {
				t.Fatalf("topologyDiscoveryMode() = %s, want %s", got, tt.want)
			}
		})
	}
}

func TestPipelineOrchestratorRunTask_DelegatesToWiredTaskRunner(t *testing.T) {
	pipeline := NewPipelineOrchestrator(nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	if pipeline.taskRunner == nil {
		t.Fatal("expected NewPipelineOrchestrator to wire taskRunner")
	}
	if concrete, ok := pipeline.taskRunner.(*pipelineTaskRunner); !ok || concrete.pipeline != pipeline {
		t.Fatal("expected wired taskRunner to retain orchestrator back-reference")
	}

	spy := &spyPipelineTaskRunner{}
	pipeline.taskRunner = spy

	task := scheduler.PollTask{
		RunID:           123,
		Key:             scheduler.NewTaskKey(uuid.New(), domain.VolatilityClassPerformance),
		VolatilityClass: domain.VolatilityClassPerformance,
	}

	pipeline.runTask(context.Background(), task)

	if len(spy.runTaskCalls) != 1 {
		t.Fatalf("expected pipeline.runTask to delegate once, got %d call(s)", len(spy.runTaskCalls))
	}
	if spy.runTaskCalls[0].RunID != task.RunID || spy.runTaskCalls[0].Key != task.Key || spy.runTaskCalls[0].VolatilityClass != task.VolatilityClass {
		t.Fatalf("delegated task = %#v, want %#v", spy.runTaskCalls[0], task)
	}
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
		ws.NewHub(ws.WithBroadcastRecorder()),
		nil,
		newPerformanceTestCollector(t),
		newOperationalTestCollector(t),
		newStaticTestCollector(t),
		collector.NewPrometheusCollector(promClient),
		&fakeTopologyService{},
		settingsRepo,
		make(chan struct{}, 1),
		nil,
		nil,
	)
	now := time.Date(2026, 4, 18, 12, 0, 0, 0, time.UTC)
	pipeline.runtime.now = func() time.Time { return now }

	pipeline.prometheusMonitor.refreshOnce(context.Background())

	pipeline.runtime.mu.RLock()
	if len(pipeline.runtime.alerts[deviceID]) != 1 {
		t.Fatalf("expected alert cache to update, got %d alert(s)", len(pipeline.runtime.alerts[deviceID]))
	}
	pipeline.runtime.mu.RUnlock()
	if !pipeline.IsPromAvailable() {
		t.Fatal("expected prometheus to remain available after successful refresh")
	}

	pipeline.runtime.mu.Lock()
	pipeline.runtime.hostnames[deviceID] = "edge-prom-host"
	pipeline.runtime.hostnameObservedAt[deviceID] = now.Add(-31 * time.Second)
	pipeline.runtime.mu.Unlock()

	promClient.mu.Lock()
	promClient.alertsErr = errors.New("prometheus unavailable")
	promClient.mu.Unlock()
	now = now.Add(5 * time.Second)

	pipeline.prometheusMonitor.refreshOnce(context.Background())

	if pipeline.IsPromAvailable() {
		t.Fatal("expected prometheus availability to flip false on refresh failure")
	}
	if pipeline.GetPrometheusStatus().Error == "" {
		t.Fatal("expected prometheus error message to be recorded")
	}

	pipeline.runtime.mu.RLock()
	defer pipeline.runtime.mu.RUnlock()
	if alerts := pipeline.runtime.alerts[deviceID]; len(alerts) != 0 {
		t.Fatalf("expected alerts to clear on refresh failure, got %d alert(s)", len(alerts))
	}
	if hostname := pipeline.runtime.hostnames[deviceID]; hostname != "" {
		t.Fatalf("expected stale hostname override to expire, got %q", hostname)
	}
}

func TestPipelineOrchestratorWorkerCount_UsesVolatilityBudgets(t *testing.T) {
	settingsRepo := newMockWorkerSettingsRepo()
	if err := settingsRepo.Set(domain.SettingSNMPWorkerPoolPerformance, "4"); err != nil {
		t.Fatalf("set performance workers: %v", err)
	}
	if err := settingsRepo.Set(domain.SettingSNMPWorkerPoolOperational, "2"); err != nil {
		t.Fatalf("set operational workers: %v", err)
	}
	if err := settingsRepo.Set(domain.SettingSNMPWorkerPoolStatic, "1"); err != nil {
		t.Fatalf("set static workers: %v", err)
	}

	pipeline := NewPipelineOrchestrator(
		newPipelineTestScheduler(),
		state.NewStore(),
		nil,
		ws.NewHub(ws.WithBroadcastRecorder()),
		nil,
		newPerformanceTestCollector(t),
		newOperationalTestCollector(t),
		newStaticTestCollector(t),
		collector.NewPrometheusCollector(&fakePrometheusClient{}),
		&fakeTopologyService{},
		settingsRepo,
		make(chan struct{}, 1),
		nil,
		nil,
	)

	if got := pipeline.workerCount(); got != 71 {
		t.Fatalf("workerCount() = %d, want background plus essential workers", got)
	}
}

func TestPipelineOrchestratorStatusReflectsLifecycle(t *testing.T) {
	sched := newPipelineTestScheduler()
	pipeline := NewPipelineOrchestrator(
		sched,
		state.NewStore(),
		newPipelineTestCache(nil, nil),
		ws.NewHub(ws.WithBroadcastRecorder()),
		nil,
		newPerformanceTestCollector(t),
		newOperationalTestCollector(t),
		newStaticTestCollector(t),
		collector.NewPrometheusCollector(&fakePrometheusClient{}),
		&fakeTopologyService{},
		newMockWorkerSettingsRepo(),
		make(chan struct{}, 1),
		nil,
		nil,
	)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := pipeline.Start(ctx); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	t.Cleanup(pipeline.Stop)

	if pipeline.Status() != "running" {
		t.Fatalf("expected running status after Start, got %q", pipeline.Status())
	}

	pipeline.Stop()

	if pipeline.Status() != "stopped" {
		t.Fatalf("expected stopped status after Stop, got %q", pipeline.Status())
	}
}

func TestPipelineOrchestratorStartReturnsErrAlreadyStarted(t *testing.T) {
	sched := newPipelineTestScheduler()
	pipeline := NewPipelineOrchestrator(
		sched,
		state.NewStore(),
		newPipelineTestCache(nil, nil),
		ws.NewHub(ws.WithBroadcastRecorder()),
		nil,
		newPerformanceTestCollector(t),
		newOperationalTestCollector(t),
		newStaticTestCollector(t),
		collector.NewPrometheusCollector(&fakePrometheusClient{}),
		&fakeTopologyService{},
		newMockWorkerSettingsRepo(),
		make(chan struct{}, 1),
		nil,
		nil,
	)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := pipeline.Start(ctx); err != nil {
		t.Fatalf("first Start() error = %v", err)
	}
	defer pipeline.Stop()

	if err := pipeline.Start(ctx); !errors.Is(err, ErrAlreadyStarted) {
		t.Fatalf("second Start() error = %v, want ErrAlreadyStarted", err)
	}
}

func TestPipelineOrchestratorStopIsIdempotent(t *testing.T) {
	sched := newPipelineTestScheduler()
	pipeline := NewPipelineOrchestrator(
		sched,
		state.NewStore(),
		newPipelineTestCache(nil, nil),
		ws.NewHub(ws.WithBroadcastRecorder()),
		nil,
		newPerformanceTestCollector(t),
		newOperationalTestCollector(t),
		newStaticTestCollector(t),
		collector.NewPrometheusCollector(&fakePrometheusClient{}),
		&fakeTopologyService{},
		newMockWorkerSettingsRepo(),
		make(chan struct{}, 1),
		nil,
		nil,
	)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := pipeline.Start(ctx); err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	pipeline.Stop()

	secondStopReturned := make(chan struct{})
	go func() {
		pipeline.Stop()
		close(secondStopReturned)
	}()

	select {
	case <-secondStopReturned:
	case <-time.After(2 * time.Second):
		t.Fatal("second Stop() did not return within 2 seconds")
	}

	if pipeline.Status() != "stopped" {
		t.Fatalf("pipeline status = %q, want stopped", pipeline.Status())
	}
	if sched.stopCalls != 1 {
		t.Fatalf("scheduler Stop() calls = %d, want 1", sched.stopCalls)
	}
}

func TestPipelineOrchestratorStartRollsBackStoreWhenSchedulerStartFails(t *testing.T) {
	sched := newPipelineTestScheduler()
	sched.startErr = errors.New("boom")
	store := state.NewStore()
	pipeline := NewPipelineOrchestrator(
		sched,
		store,
		newPipelineTestCache(nil, nil),
		ws.NewHub(ws.WithBroadcastRecorder()),
		nil,
		newPerformanceTestCollector(t),
		newOperationalTestCollector(t),
		newStaticTestCollector(t),
		collector.NewPrometheusCollector(&fakePrometheusClient{}),
		&fakeTopologyService{},
		newMockWorkerSettingsRepo(),
		make(chan struct{}, 1),
		nil,
		nil,
	)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := pipeline.Start(ctx); err == nil {
		t.Fatal("expected Start() to fail when scheduler start fails")
	}
	if pipeline.Status() != "stopped" {
		t.Fatalf("pipeline status = %q, want stopped", pipeline.Status())
	}
	if err := store.Start(ctx); err != nil {
		t.Fatalf("store should be restartable after rollback, got %v", err)
	}
	store.Stop()
}

func TestPipelineOrchestratorRunTask_VirtualOperationalUsesPrometheusReachability(t *testing.T) {
	deviceID := uuid.New()
	task := scheduler.PollTask{
		RunID:            91,
		Key:              scheduler.NewTaskKey(deviceID, domain.VolatilityClassOperational),
		VolatilityClass:  domain.VolatilityClassOperational,
		ExpectedInterval: domain.OperationalClassInterval,
		Device: domain.Device{
			ID:                   deviceID,
			DeviceType:           domain.DeviceTypeVirtual,
			IP:                   "192.0.2.90",
			Status:               domain.DeviceStatusUnknown,
			PrometheusLabelName:  "instance",
			PrometheusLabelValue: "192.0.2.90",
		},
	}

	sched := newPipelineTestScheduler()
	store := state.NewStore()
	promClient := &fakePrometheusClient{
		hostnames:     map[string]string{"192.0.2.90": "cloud-vpn"},
		probeStatuses: map[string]bool{"192.0.2.90": true},
	}
	settingsRepo := newMockWorkerSettingsRepo()
	if err := settingsRepo.Set(domain.SettingPrometheusURL, "http://prometheus.test"); err != nil {
		t.Fatalf("set prometheus_url: %v", err)
	}

	var snmpCalls int
	operational := collector.NewOperationalCollector(
		buildEmptyVendorRegistry(),
		func(string, domain.SNMPCredentials, time.Duration, int) (collector.SNMPClient, error) {
			snmpCalls++
			return nil, errors.New("virtual operational task should not create an SNMP client")
		},
	)

	pipeline := NewPipelineOrchestrator(
		sched,
		store,
		newPipelineTestCache([]domain.Device{task.Device}, nil),
		ws.NewHub(ws.WithBroadcastRecorder()),
		nil,
		newPerformanceTestCollector(t),
		operational,
		newStaticTestCollector(t),
		collector.NewPrometheusCollector(promClient),
		&fakeTopologyService{},
		settingsRepo,
		make(chan struct{}, 1),
		nil,
		nil,
	)
	pipeline.prometheusMonitor.publishStatus(ws.PrometheusStatusPayload{
		Enabled:   true,
		Available: true,
	})

	pipeline.taskRunner.runTask(context.Background(), task)

	deviceState, ok := store.GetDevice(deviceID)
	if !ok {
		t.Fatal("expected virtual operational task to update state store")
	}
	if deviceState.Reachability != state.ReachabilityUp {
		t.Fatalf("Reachability = %q, want %q", deviceState.Reachability, state.ReachabilityUp)
	}
	if deviceState.LastPolledAt.IsZero() {
		t.Fatal("expected virtual operational task to stamp last poll time")
	}
	if deviceState.ExpectedInterval != domain.OperationalClassInterval {
		t.Fatalf("ExpectedInterval = %v, want %v", deviceState.ExpectedInterval, domain.OperationalClassInterval)
	}
	if snmpCalls != 0 {
		t.Fatalf("expected virtual operational task to bypass SNMP collector, got %d SNMP call(s)", snmpCalls)
	}

	pipeline.runtime.mu.RLock()
	hostname := pipeline.runtime.hostnames[deviceID]
	pipeline.runtime.mu.RUnlock()
	if hostname != "cloud-vpn" {
		t.Fatalf("hostname override = %q, want %q", hostname, "cloud-vpn")
	}

	sched.mu.Lock()
	defer sched.mu.Unlock()
	if len(sched.completions) != 1 {
		t.Fatalf("expected one scheduler completion, got %d", len(sched.completions))
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

	hub := ws.NewHub(ws.WithBroadcastRecorder())
	topologyNotify := make(chan struct{}, 4)
	pipeline := NewPipelineOrchestrator(
		newPipelineTestScheduler(),
		store,
		newPipelineTestCache([]domain.Device{device}, []domain.Link{link}),
		hub,
		nil,
		nil,
		nil,
		nil,
		collector.NewPrometheusCollector(&fakePrometheusClient{}),
		&fakeTopologyService{},
		newMockWorkerSettingsRepo(),
		topologyNotify,
		nil,
		nil,
	)

	return pipeline, hub, store, topologyNotify, deviceID
}

func TestPipelineWorkerCountIncludesEssentialWorkers(t *testing.T) {
	settingsRepo := newMockWorkerSettingsRepo()
	_ = settingsRepo.Set(domain.SettingPollingEssentialWorkers, "12")
	_ = settingsRepo.Set(domain.SettingSNMPWorkerPoolPerformance, "3")
	_ = settingsRepo.Set(domain.SettingSNMPWorkerPoolOperational, "2")
	_ = settingsRepo.Set(domain.SettingSNMPWorkerPoolStatic, "1")

	pipeline := NewPipelineOrchestrator(
		newPipelineTestScheduler(),
		state.NewStore(),
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		settingsRepo,
		nil,
		nil,
		nil,
	)

	if got := pipeline.workerCount(); got != 18 {
		t.Fatalf("workerCount() = %d, want essential workers plus background workers", got)
	}
}

func TestPipelineBroadcastCoalesceWindowUsesPollingPolicySetting(t *testing.T) {
	settingsRepo := newMockWorkerSettingsRepo()
	_ = settingsRepo.Set(domain.SettingPollingWebSocketCoalesceMS, "750")

	pipeline := NewPipelineOrchestrator(
		newPipelineTestScheduler(),
		state.NewStore(),
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		settingsRepo,
		nil,
		nil,
		nil,
	)

	if pipeline.broadcastCoalesceWindow != 750*time.Millisecond {
		t.Fatalf("broadcastCoalesceWindow = %v, want 750ms", pipeline.broadcastCoalesceWindow)
	}
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

type wsVersionedSnapshotMessage struct {
	Type    string `json:"type"`
	Payload struct {
		Version  uint64              `json:"version"`
		Snapshot *ws.SnapshotPayload `json:"snapshot"`
	} `json:"payload"`
}

type wsVersionedSnapshotDeltaMessage struct {
	Type    string `json:"type"`
	Payload struct {
		BaseVersion uint64              `json:"base_version"`
		Version     uint64              `json:"version"`
		Delta       *ws.SnapshotPayload `json:"delta"`
	} `json:"payload"`
}

type wsVersionedRuntimeDeltaMessage struct {
	Type    string `json:"type"`
	Payload struct {
		BaseVersion uint64                 `json:"base_version"`
		Version     uint64                 `json:"version"`
		Delta       ws.RuntimeDeltaPayload `json:"delta"`
	} `json:"payload"`
}

type wsVersionedAlertMessage struct {
	Type    string `json:"type"`
	Payload struct {
		Version uint64        `json:"version"`
		Alerts  []ws.AlertDTO `json:"alerts"`
	} `json:"payload"`
}

type wsTopologyChangedMessage struct {
	Type    string `json:"type"`
	Payload struct {
		TopologyVersion     uint64 `json:"topology_version"`
		Reason              string `json:"reason"`
		RecommendedEndpoint string `json:"recommended_endpoint"`
	} `json:"payload"`
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

func attachDetailSubscriptionTopology(device domain.Device) (*cache.DeviceLinkCache, domain.Link) {
	peer := domain.Device{
		ID:     uuid.New(),
		IP:     "192.0.2.81",
		Status: domain.DeviceStatusUp,
		Vendor: "default",
		Interfaces: []domain.Interface{
			{IfName: "ether2", IfDescr: "downlink", Speed: 1_000_000_000},
		},
	}
	link := domain.Link{
		ID:             uuid.New(),
		SourceDeviceID: device.ID,
		SourceIfName:   "ether1",
		TargetDeviceID: peer.ID,
		TargetIfName:   "ether2",
	}
	return newPipelineTestCache([]domain.Device{device, peer}, []domain.Link{link}), link
}

func newDetailSubscriptionTestPipeline(t *testing.T, hub *ws.Hub) *PipelineOrchestrator {
	t.Helper()

	return NewPipelineOrchestrator(
		newPipelineTestScheduler(),
		state.NewStore(),
		nil,
		hub,
		nil,
		newPerformanceTestCollector(t),
		newOperationalTestCollector(t),
		newStaticTestCollector(t),
		collector.NewPrometheusCollector(&fakePrometheusClient{}),
		&fakeTopologyService{},
		newMockWorkerSettingsRepo(),
		make(chan struct{}, 1),
		nil,
		nil,
	)
}

func newDetailSubscriptionTestServer(t *testing.T, hub *ws.Hub) string {
	t.Helper()

	go hub.Run()

	server := httptest.NewServer(ws.NewHandler(
		hub,
		func() (*ws.SnapshotPayload, uint64) { return ws.EmptySnapshot(), 0 },
		func() ws.AlertMessagePayload { return ws.AlertMessagePayload{Alerts: []ws.AlertDTO{}} },
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

	types := make([]string, 0, 3)
	for i := 0; i < 3; i++ {
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

	if len(types) != 3 || types[0] != ws.MessageTypeSnapshot || types[1] != ws.MessageTypeAlert || types[2] != ws.MessageTypePrometheusStatus {
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

func readSnapshotDeltaMessage(t *testing.T, conn *websocket.Conn) wsVersionedSnapshotDeltaMessage {
	t.Helper()

	conn.SetReadDeadline(time.Now().Add(time.Second))
	defer conn.SetReadDeadline(time.Time{})

	_, raw, err := conn.ReadMessage()
	if err != nil {
		t.Fatalf("failed to read websocket detail delta: %v", err)
	}

	var message wsVersionedSnapshotDeltaMessage
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

func waitForBroadcastMessages(t *testing.T, hub *ws.Hub, timeout time.Duration) [][]byte {
	t.Helper()

	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if messages := drainBroadcastCh(hub); len(messages) > 0 {
			return messages
		}
		time.Sleep(10 * time.Millisecond)
	}

	t.Fatal("timed out waiting for broadcast message")
	return nil
}

func TestPipelineOrchestratorBroadcastLoopSendsSnapshotThenDelta(t *testing.T) {
	pipeline, hub, store, _, deviceID := newBroadcastTestPipeline(t)

	pipeline.broadcaster.broadcastOnce(context.Background())

	firstMessages := drainBroadcastCh(hub)
	firstTypes := broadcastMessageTypes(t, firstMessages)
	if len(firstTypes) == 0 || firstTypes[0] != ws.MessageTypeSnapshot {
		t.Fatalf("expected first broadcast to be snapshot, got %v", firstTypes)
	}
	var firstSnapshot wsVersionedSnapshotMessage
	if err := json.Unmarshal(firstMessages[0], &firstSnapshot); err != nil {
		t.Fatalf("decode first snapshot: %v", err)
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

	pipeline.broadcaster.broadcastOnce(context.Background())

	secondMessages := drainBroadcastCh(hub)
	secondTypes := broadcastMessageTypes(t, secondMessages)
	if len(secondTypes) == 0 || secondTypes[0] != ws.MessageTypeRuntimeDelta {
		t.Fatalf("expected second broadcast to be runtime_delta, got %v", secondTypes)
	}
	var delta wsVersionedSnapshotDeltaMessage
	if err := json.Unmarshal(secondMessages[0], &delta); err != nil {
		t.Fatalf("decode second delta: %v", err)
	}
	if delta.Payload.BaseVersion != firstSnapshot.Payload.Version {
		t.Fatalf("delta base_version = %d, want %d", delta.Payload.BaseVersion, firstSnapshot.Payload.Version)
	}
	if delta.Payload.Version != firstSnapshot.Payload.Version+1 {
		t.Fatalf("delta version = %d, want %d", delta.Payload.Version, firstSnapshot.Payload.Version+1)
	}
}

func TestPipelineOrchestratorBroadcastLoop_EventDrivenStateChangeSendsDelta(t *testing.T) {
	pipeline, hub, store, _, deviceID := newBroadcastTestPipeline(t)
	pipeline.broadcastCoalesceWindow = 10 * time.Millisecond
	pipeline.fullResyncInterval = time.Hour

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go pipeline.broadcaster.broadcastLoop(ctx)

	initialTypes := broadcastMessageTypes(t, waitForBroadcastMessages(t, hub, time.Second))
	if len(initialTypes) == 0 || initialTypes[0] != ws.MessageTypeSnapshot {
		t.Fatalf("expected initial event-driven broadcast to be snapshot, got %v", initialTypes)
	}

	store.Update(state.StateUpdate{
		DeviceID:        deviceID,
		VolatilityClass: domain.VolatilityClassPerformance,
		Metrics: &domain.DeviceMetrics{
			DeviceID:    deviceID,
			CPUPercent:  floatPtr(61),
			CollectedAt: time.Date(2026, 4, 13, 12, 0, 8, 0, time.UTC),
		},
		PollSuccess:      true,
		ExpectedInterval: 30 * time.Second,
		Timestamp:        time.Date(2026, 4, 13, 12, 0, 8, 0, time.UTC),
	})

	types := broadcastMessageTypes(t, waitForBroadcastMessages(t, hub, time.Second))
	if len(types) == 0 || types[0] != ws.MessageTypeRuntimeDelta {
		t.Fatalf("expected state change to broadcast runtime_delta, got %v", types)
	}
}

func TestPipelineOrchestratorBroadcastLoop_DrainsStaleQueuedStateChangesBeforeInitialSnapshot(t *testing.T) {
	pipeline, hub, store, _, deviceID := newBroadcastTestPipeline(t)
	pipeline.broadcastCoalesceWindow = 10 * time.Millisecond
	pipeline.fullResyncInterval = time.Hour

	store.Update(state.StateUpdate{
		DeviceID:        deviceID,
		VolatilityClass: domain.VolatilityClassPerformance,
		Metrics: &domain.DeviceMetrics{
			DeviceID:    deviceID,
			CPUPercent:  floatPtr(62),
			CollectedAt: time.Date(2026, 4, 25, 9, 30, 0, 0, time.UTC),
		},
		PollSuccess:      true,
		ExpectedInterval: 30 * time.Second,
		Timestamp:        time.Date(2026, 4, 25, 9, 30, 0, 0, time.UTC),
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go pipeline.broadcaster.broadcastLoop(ctx)

	initialTypes := broadcastMessageTypes(t, waitForBroadcastMessages(t, hub, time.Second))
	if len(initialTypes) == 0 || initialTypes[0] != ws.MessageTypeSnapshot {
		t.Fatalf("expected initial broadcast to be snapshot, got %v", initialTypes)
	}

	time.Sleep(50 * time.Millisecond)
	if messages := drainBroadcastCh(hub); len(messages) != 0 {
		t.Fatalf("expected stale pre-start state change to be included in initial snapshot only, got %v", broadcastMessageTypes(t, messages))
	}
}

func TestPipelineOrchestratorBuildDirtyOverviewDelta_AlertOnlyChangeIncludesAlertRuntimeFields(t *testing.T) {
	pipeline, _, _, _, deviceID := newBroadcastTestPipeline(t)
	pipeline.runtime.lastSnapshot = ws.EmptySnapshot()
	pipeline.runtime.prevHashes = computeSnapshotHashes(pipeline.runtime.lastSnapshot)
	pipeline.runtime.alerts = map[uuid.UUID][]domain.AlertState{
		deviceID: {{
			DeviceID:  deviceID,
			Severity:  "critical",
			AlertName: "HighCPU",
			State:     "firing",
		}},
	}

	previousSnapshotAll := snapshotAllPipelineState
	previousSnapshotFor := snapshotPipelineStateFor
	fullSnapshotCalls := 0
	narrowSnapshotCalls := 0
	var requestedIDs []uuid.UUID
	snapshotAllPipelineState = func(store *state.Store) map[uuid.UUID]state.DeviceState {
		fullSnapshotCalls++
		return store.Snapshot()
	}
	snapshotPipelineStateFor = func(store *state.Store, ids []uuid.UUID) map[uuid.UUID]state.DeviceState {
		narrowSnapshotCalls++
		requestedIDs = append([]uuid.UUID(nil), ids...)
		return store.SnapshotFor(ids)
	}
	t.Cleanup(func() {
		snapshotAllPipelineState = previousSnapshotAll
		snapshotPipelineStateFor = previousSnapshotFor
	})

	delta, requireFull, err := pipeline.buildDirtyOverviewDelta(nil, true)
	if err != nil {
		t.Fatalf("buildDirtyOverviewDelta returned error: %v", err)
	}
	if requireFull {
		t.Fatal("requireFull = true, want false")
	}
	if delta == nil {
		t.Fatal("expected alert-only dirty delta")
	}

	deviceRuntime, ok := delta.Devices[deviceID.String()]
	if !ok {
		t.Fatalf("expected devices[%s] in alert-only delta", deviceID)
	}
	if deviceRuntime.AlertStatus != string(domain.AlertStatusDown) {
		t.Fatalf("AlertStatus = %q, want %q", deviceRuntime.AlertStatus, domain.AlertStatusDown)
	}
	if deviceRuntime.FiringAlertCount != 1 {
		t.Fatalf("FiringAlertCount = %d, want 1", deviceRuntime.FiringAlertCount)
	}
	if fullSnapshotCalls != 0 {
		t.Fatalf("full state snapshot calls = %d, want 0", fullSnapshotCalls)
	}
	if narrowSnapshotCalls != 1 {
		t.Fatalf("narrow state snapshot calls = %d, want 1", narrowSnapshotCalls)
	}
	assertUUIDSliceSetEqual(t, requestedIDs, map[uuid.UUID]struct{}{deviceID: {}})
}

func TestPipelineOrchestratorBuildDirtyOverviewDelta_AlertResolutionUsesNarrowStateSnapshotForPreviouslyAlertingDevice(t *testing.T) {
	pipeline, _, store, _, deviceID := newBroadcastTestPipeline(t)

	devices, err := pipeline.cache.GetDevices()
	if err != nil {
		t.Fatalf("GetDevices returned error: %v", err)
	}
	links, err := pipeline.cache.GetLinks()
	if err != nil {
		t.Fatalf("GetLinks returned error: %v", err)
	}

	firingAlerts := map[uuid.UUID][]domain.AlertState{
		deviceID: {{
			DeviceID:  deviceID,
			Severity:  "critical",
			AlertName: "HighCPU",
			State:     "firing",
		}},
	}
	firingSnapshot := buildPipelineSnapshot(devices, links, store.Snapshot(), firingAlerts, ws.PrometheusStatusPayload{})
	pipeline.runtime.lastSnapshot = firingSnapshot
	pipeline.runtime.prevHashes = computeSnapshotHashes(firingSnapshot)
	pipeline.runtime.alerts = map[uuid.UUID][]domain.AlertState{}

	previousSnapshotAll := snapshotAllPipelineState
	previousSnapshotFor := snapshotPipelineStateFor
	fullSnapshotCalls := 0
	narrowSnapshotCalls := 0
	var requestedIDs []uuid.UUID
	snapshotAllPipelineState = func(store *state.Store) map[uuid.UUID]state.DeviceState {
		fullSnapshotCalls++
		return store.Snapshot()
	}
	snapshotPipelineStateFor = func(store *state.Store, ids []uuid.UUID) map[uuid.UUID]state.DeviceState {
		narrowSnapshotCalls++
		requestedIDs = append([]uuid.UUID(nil), ids...)
		return store.SnapshotFor(ids)
	}
	t.Cleanup(func() {
		snapshotAllPipelineState = previousSnapshotAll
		snapshotPipelineStateFor = previousSnapshotFor
	})

	delta, requireFull, err := pipeline.buildDirtyOverviewDelta(nil, true)
	if err != nil {
		t.Fatalf("buildDirtyOverviewDelta returned error: %v", err)
	}
	if requireFull {
		t.Fatal("requireFull = true, want false")
	}
	if delta == nil {
		t.Fatal("expected alert resolution dirty delta")
	}

	deviceRuntime, ok := delta.Devices[deviceID.String()]
	if !ok {
		t.Fatalf("expected devices[%s] in alert resolution delta", deviceID)
	}
	if deviceRuntime.AlertStatus != string(domain.AlertStatusNormal) {
		t.Fatalf("AlertStatus = %q, want %q", deviceRuntime.AlertStatus, domain.AlertStatusNormal)
	}
	if deviceRuntime.FiringAlertCount != 0 {
		t.Fatalf("FiringAlertCount = %d, want 0", deviceRuntime.FiringAlertCount)
	}
	if fullSnapshotCalls != 0 {
		t.Fatalf("full state snapshot calls = %d, want 0", fullSnapshotCalls)
	}
	if narrowSnapshotCalls != 1 {
		t.Fatalf("narrow state snapshot calls = %d, want 1", narrowSnapshotCalls)
	}
	assertUUIDSliceSetEqual(t, requestedIDs, map[uuid.UUID]struct{}{deviceID: {}})
}

func TestPipelineOrchestratorBuildDirtyOverviewDelta_AlertResolutionRebuildsMissingHashesFromPreviousSnapshot(t *testing.T) {
	pipeline, _, store, _, deviceID := newBroadcastTestPipeline(t)

	devices, err := pipeline.cache.GetDevices()
	if err != nil {
		t.Fatalf("GetDevices returned error: %v", err)
	}
	links, err := pipeline.cache.GetLinks()
	if err != nil {
		t.Fatalf("GetLinks returned error: %v", err)
	}

	firingAlerts := map[uuid.UUID][]domain.AlertState{
		deviceID: {{
			DeviceID:  deviceID,
			Severity:  "critical",
			AlertName: "HighCPU",
			State:     "firing",
		}},
	}
	firingSnapshot := buildPipelineSnapshot(devices, links, store.Snapshot(), firingAlerts, ws.PrometheusStatusPayload{})
	pipeline.runtime.lastSnapshot = firingSnapshot
	pipeline.runtime.prevHashes = nil
	pipeline.runtime.alerts = map[uuid.UUID][]domain.AlertState{}

	previousSnapshotAll := snapshotAllPipelineState
	previousSnapshotFor := snapshotPipelineStateFor
	fullSnapshotCalls := 0
	narrowSnapshotCalls := 0
	var requestedIDs []uuid.UUID
	snapshotAllPipelineState = func(store *state.Store) map[uuid.UUID]state.DeviceState {
		fullSnapshotCalls++
		return store.Snapshot()
	}
	snapshotPipelineStateFor = func(store *state.Store, ids []uuid.UUID) map[uuid.UUID]state.DeviceState {
		narrowSnapshotCalls++
		requestedIDs = append([]uuid.UUID(nil), ids...)
		return store.SnapshotFor(ids)
	}
	t.Cleanup(func() {
		snapshotAllPipelineState = previousSnapshotAll
		snapshotPipelineStateFor = previousSnapshotFor
	})

	delta, requireFull, err := pipeline.buildDirtyOverviewDelta(nil, true)
	if err != nil {
		t.Fatalf("buildDirtyOverviewDelta returned error: %v", err)
	}
	if requireFull {
		t.Fatal("requireFull = true, want false")
	}
	if delta == nil {
		t.Fatal("expected alert resolution dirty delta")
	}

	deviceRuntime, ok := delta.Devices[deviceID.String()]
	if !ok {
		t.Fatalf("expected devices[%s] in alert resolution delta", deviceID)
	}
	if deviceRuntime.AlertStatus != string(domain.AlertStatusNormal) {
		t.Fatalf("AlertStatus = %q, want %q", deviceRuntime.AlertStatus, domain.AlertStatusNormal)
	}
	if deviceRuntime.FiringAlertCount != 0 {
		t.Fatalf("FiringAlertCount = %d, want 0", deviceRuntime.FiringAlertCount)
	}
	if fullSnapshotCalls != 0 {
		t.Fatalf("full state snapshot calls = %d, want 0", fullSnapshotCalls)
	}
	if narrowSnapshotCalls != 1 {
		t.Fatalf("narrow state snapshot calls = %d, want 1", narrowSnapshotCalls)
	}
	assertUUIDSliceSetEqual(t, requestedIDs, map[uuid.UUID]struct{}{deviceID: {}})
}

func TestPipelineOrchestratorBuildDirtyOverviewDelta_UsesNarrowStateSnapshotForDeviceOnlyChange(t *testing.T) {
	pipeline, _, _, _, deviceID := newBroadcastTestPipeline(t)

	previousSnapshotAll := snapshotAllPipelineState
	previousSnapshotFor := snapshotPipelineStateFor
	fullSnapshotCalls := 0
	narrowSnapshotCalls := 0
	var requestedIDs []uuid.UUID
	snapshotAllPipelineState = func(store *state.Store) map[uuid.UUID]state.DeviceState {
		fullSnapshotCalls++
		return store.Snapshot()
	}
	snapshotPipelineStateFor = func(store *state.Store, ids []uuid.UUID) map[uuid.UUID]state.DeviceState {
		narrowSnapshotCalls++
		requestedIDs = append([]uuid.UUID(nil), ids...)
		return store.SnapshotFor(ids)
	}
	t.Cleanup(func() {
		snapshotAllPipelineState = previousSnapshotAll
		snapshotPipelineStateFor = previousSnapshotFor
	})

	delta, requireFull, err := pipeline.buildDirtyOverviewDelta(map[uuid.UUID]struct{}{deviceID: {}}, false)
	if err != nil {
		t.Fatalf("buildDirtyOverviewDelta returned error: %v", err)
	}
	if requireFull {
		t.Fatal("requireFull = true, want false")
	}
	if delta == nil {
		t.Fatal("expected dirty overview delta")
	}
	if fullSnapshotCalls != 0 {
		t.Fatalf("full state snapshot calls = %d, want 0", fullSnapshotCalls)
	}
	if narrowSnapshotCalls != 1 {
		t.Fatalf("narrow state snapshot calls = %d, want 1", narrowSnapshotCalls)
	}

	links, err := pipeline.cache.GetLinks()
	if err != nil {
		t.Fatalf("GetLinks returned error: %v", err)
	}
	wantIDs := map[uuid.UUID]struct{}{deviceID: {}}
	for _, link := range links {
		if link.SourceDeviceID == deviceID || link.TargetDeviceID == deviceID {
			wantIDs[link.SourceDeviceID] = struct{}{}
			wantIDs[link.TargetDeviceID] = struct{}{}
		}
	}
	assertUUIDSliceSetEqual(t, requestedIDs, wantIDs)
}

func TestPipelineOrchestratorBuildDirtyOverviewDelta_PreservesPeerContextForLinks(t *testing.T) {
	pipeline, _, store, _, deviceID := newBroadcastTestPipeline(t)
	peerID := uuid.New()
	pipeline.cache = newPipelineTestCache([]domain.Device{
		{
			ID:            deviceID,
			IP:            "192.0.2.40",
			MetricsSource: domain.MetricsSourcePrometheus,
			Interfaces:    []domain.Interface{{IfName: "ether1", Speed: 1_000_000_000}},
		},
		{
			ID:            peerID,
			IP:            "192.0.2.41",
			MetricsSource: domain.MetricsSourcePrometheus,
			Interfaces:    []domain.Interface{{IfName: "ether2", Speed: 1_000_000_000}},
		},
	}, []domain.Link{{
		ID:             uuid.New(),
		SourceDeviceID: deviceID,
		SourceIfName:   "ether1",
		TargetDeviceID: peerID,
		TargetIfName:   "ether2",
	}})
	devices, err := pipeline.cache.GetDevices()
	if err != nil {
		t.Fatalf("GetDevices returned error: %v", err)
	}
	if len(devices) != 2 {
		t.Fatalf("expected 2 devices, got %d", len(devices))
	}
	links, err := pipeline.cache.GetLinks()
	if err != nil {
		t.Fatalf("GetLinks returned error: %v", err)
	}
	if len(links) != 1 {
		t.Fatalf("expected 1 link, got %d", len(links))
	}
	linkID := links[0].ID

	store.Update(state.StateUpdate{
		DeviceID:        peerID,
		VolatilityClass: domain.VolatilityClassOperational,
		PollSuccess:     true,
		Timestamp:       time.Date(2026, 4, 13, 12, 0, 0, 0, time.UTC),
	})
	pipeline.runtime.promStatus = ws.PrometheusStatusPayload{Enabled: true, Available: false}

	delta, requireFull, err := pipeline.buildDirtyOverviewDelta(map[uuid.UUID]struct{}{deviceID: {}}, false)
	if err != nil {
		t.Fatalf("buildDirtyOverviewDelta returned error: %v", err)
	}
	if requireFull {
		t.Fatal("requireFull = true, want false")
	}
	if delta == nil {
		t.Fatal("expected dirty overview delta")
	}

	linkRuntime, ok := delta.Links[linkID.String()]
	if !ok {
		t.Fatalf("expected links[%s] in dirty delta", linkID)
	}
	if linkRuntime.MetricsReason != normalizedReasonUpstreamUnavailable {
		t.Fatalf("MetricsReason = %q, want %q", linkRuntime.MetricsReason, normalizedReasonUpstreamUnavailable)
	}
}

func TestMergeSnapshotPayload_RefreshesCompatibilityViewsAfterDelta(t *testing.T) {
	deviceID := uuid.New().String()
	linkID := uuid.New().String()
	updatedCollectedAt := "2026-04-20T10:30:00Z"

	base := &ws.SnapshotPayload{
		Devices: map[string]ws.DeviceRuntimeDTO{
			deviceID: {
				DeviceID:          deviceID,
				OperationalStatus: string(domain.DeviceStatusDown),
				MetricsStatus:     "missing",
			},
		},
		Links: map[string]ws.LinkRuntimeDTO{
			linkID: {
				LinkID:         linkID,
				SourceDeviceID: deviceID,
				SourceIfName:   "ether1",
				MetricsStatus:  "missing",
			},
		},
		DeviceMetrics: map[string]ws.DeviceRuntimeDTO{
			deviceID: {
				DeviceID:          deviceID,
				OperationalStatus: string(domain.DeviceStatusDown),
				MetricsStatus:     "missing",
			},
		},
		LinkMetrics: map[string][]ws.LinkRuntimeDTO{
			deviceID: {{
				LinkID:          linkID,
				DeviceID:        deviceID,
				IfName:          "ether1",
				MetricsStatus:   "missing",
				LastCollectedAt: nil,
			}},
		},
		DeviceStatuses: map[string]string{deviceID: string(domain.DeviceStatusDown)},
	}

	delta := &ws.SnapshotPayload{
		Devices: map[string]ws.DeviceRuntimeDTO{
			deviceID: {
				DeviceID:          deviceID,
				OperationalStatus: string(domain.DeviceStatusUp),
				MetricsStatus:     "available",
			},
		},
		Links: map[string]ws.LinkRuntimeDTO{
			linkID: {
				LinkID:          linkID,
				SourceDeviceID:  deviceID,
				SourceIfName:    "ether1",
				DeviceID:        deviceID,
				IfName:          "ether1",
				MetricsStatus:   "available",
				LastCollectedAt: &updatedCollectedAt,
			},
		},
	}

	merged := mergeSnapshotPayload(base, delta)

	if got := merged.DeviceMetrics[deviceID].MetricsStatus; got != "available" {
		t.Fatalf("DeviceMetrics[%s].MetricsStatus = %q, want available", deviceID, got)
	}
	if got := merged.DeviceStatuses[deviceID]; got != string(domain.DeviceStatusUp) {
		t.Fatalf("DeviceStatuses[%s] = %q, want %q", deviceID, got, domain.DeviceStatusUp)
	}
	legacyLinks, ok := merged.LinkMetrics[deviceID]
	if !ok {
		t.Fatalf("expected LinkMetrics[%s] entry after merge", deviceID)
	}
	if len(legacyLinks) != 1 {
		t.Fatalf("LinkMetrics[%s] length = %d, want 1", deviceID, len(legacyLinks))
	}
	if got := legacyLinks[0].MetricsStatus; got != "available" {
		t.Fatalf("LinkMetrics[%s][0].MetricsStatus = %q, want available", deviceID, got)
	}
	if legacyLinks[0].LastCollectedAt == nil || *legacyLinks[0].LastCollectedAt != updatedCollectedAt {
		t.Fatalf("LinkMetrics[%s][0].LastCollectedAt = %#v, want %q", deviceID, legacyLinks[0].LastCollectedAt, updatedCollectedAt)
	}
}

func TestPipelineOrchestratorBroadcastLoop_LinkChangeBroadcastsTopologyInvalidationOnly(t *testing.T) {
	pipeline, hub, _, _, _ := newBroadcastTestPipeline(t)
	pipeline.broadcastCoalesceWindow = 10 * time.Millisecond
	pipeline.fullResyncInterval = time.Hour
	linkChanges := make(chan domain.LinkChangeEvent, 1)
	pipeline.linkChangeNotify = linkChanges

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go pipeline.broadcaster.broadcastLoop(ctx)

	_ = waitForBroadcastMessages(t, hub, time.Second)

	linkChanges <- domain.LinkChangeEvent{
		Kind:   domain.ChangeKindCreated,
		LinkID: uuid.New(),
	}

	messages := waitForBroadcastMessages(t, hub, time.Second)
	types := broadcastMessageTypes(t, messages)
	if len(types) != 1 || types[0] != ws.MessageTypeTopologyChanged {
		t.Fatalf("expected topology_changed invalidation only after link change, got %v", types)
	}

	var message wsTopologyChangedMessage
	if err := json.Unmarshal(messages[0], &message); err != nil {
		t.Fatalf("decode topology_changed: %v", err)
	}
	if message.Payload.TopologyVersion == 0 {
		t.Fatal("expected topology_changed payload to include a topology version")
	}
	if message.Payload.Reason != refreshReloadReasonTopologyDirty {
		t.Fatalf("topology_changed reason = %q, want %q", message.Payload.Reason, refreshReloadReasonTopologyDirty)
	}
	if message.Payload.RecommendedEndpoint != "/api/v1/topology/canvas" {
		t.Fatalf("recommended endpoint = %q, want /api/v1/topology/canvas", message.Payload.RecommendedEndpoint)
	}
}

func TestPipelineOrchestratorBroadcastLoop_AlertRefreshBroadcastsAlertMessage(t *testing.T) {
	pipeline, hub, _, _, deviceID := newBroadcastTestPipeline(t)
	pipeline.broadcastCoalesceWindow = 10 * time.Millisecond
	pipeline.fullResyncInterval = time.Hour

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go pipeline.broadcaster.broadcastLoop(ctx)

	_ = waitForBroadcastMessages(t, hub, time.Second)

	pipeline.prometheusMonitor.setAlerts(map[uuid.UUID][]domain.AlertState{
		deviceID: {{
			DeviceID:  deviceID,
			Severity:  "critical",
			AlertName: "DeviceDown",
			State:     "firing",
			Summary:   "device down",
		}},
	})

	messages := waitForBroadcastMessages(t, hub, time.Second)
	types := broadcastMessageTypes(t, messages)
	if len(types) != 2 || types[0] != ws.MessageTypeRuntimeDelta || types[1] != ws.MessageTypeAlert {
		t.Fatalf("expected alert-only refresh to broadcast runtime_delta then alert, got %v", types)
	}

	var deltaMessage wsVersionedSnapshotDeltaMessage
	if err := json.Unmarshal(messages[0], &deltaMessage); err != nil {
		t.Fatalf("decode alert-driven delta: %v", err)
	}
	deviceRuntime, ok := deltaMessage.Payload.Delta.Devices[deviceID.String()]
	if !ok {
		t.Fatalf("expected alert-driven delta for device %s", deviceID)
	}
	if deviceRuntime.AlertStatus != string(domain.AlertStatusDown) {
		t.Fatalf("AlertStatus = %q, want %q", deviceRuntime.AlertStatus, domain.AlertStatusDown)
	}
	if deviceRuntime.FiringAlertCount != 1 {
		t.Fatalf("FiringAlertCount = %d, want 1", deviceRuntime.FiringAlertCount)
	}

	var alertMessage wsVersionedAlertMessage
	if err := json.Unmarshal(messages[1], &alertMessage); err != nil {
		t.Fatalf("decode alert message: %v", err)
	}
	if alertMessage.Payload.Version == 0 {
		t.Fatal("expected alert broadcast version to increment from zero")
	}
	if len(alertMessage.Payload.Alerts) != 1 || alertMessage.Payload.Alerts[0].DeviceID != deviceID.String() {
		t.Fatalf("alert payload = %#v, want single alert for %s", alertMessage.Payload.Alerts, deviceID)
	}
}

func TestPipelineOrchestratorBroadcastDirty_AlertResolutionWithoutRuntimeBaseUsesNarrowPatch(t *testing.T) {
	pipeline, hub, _, _, deviceID := newBroadcastTestPipeline(t)
	pipeline.runtime.setAlerts(map[uuid.UUID][]domain.AlertState{
		deviceID: {{
			DeviceID:  deviceID,
			Severity:  "critical",
			AlertName: "DeviceDown",
			State:     "firing",
			Summary:   "device down",
		}},
	})

	pipeline.broadcaster.broadcastOnce(context.Background())
	drainBroadcastCh(hub)

	pipeline.runtime.mu.Lock()
	pipeline.runtime.lastSnapshot = nil
	pipeline.runtime.prevHashes = nil
	pipeline.runtime.mu.Unlock()
	pipeline.runtime.setAlerts(map[uuid.UUID][]domain.AlertState{})

	previousSnapshotAll := snapshotAllPipelineState
	previousSnapshotFor := snapshotPipelineStateFor
	fullSnapshotCalls := 0
	narrowSnapshotCalls := 0
	var requestedIDs []uuid.UUID
	snapshotAllPipelineState = func(store *state.Store) map[uuid.UUID]state.DeviceState {
		fullSnapshotCalls++
		return store.Snapshot()
	}
	snapshotPipelineStateFor = func(store *state.Store, ids []uuid.UUID) map[uuid.UUID]state.DeviceState {
		narrowSnapshotCalls++
		requestedIDs = append([]uuid.UUID(nil), ids...)
		return store.SnapshotFor(ids)
	}
	t.Cleanup(func() {
		snapshotAllPipelineState = previousSnapshotAll
		snapshotPipelineStateFor = previousSnapshotFor
	})

	if err := pipeline.broadcaster.broadcastDirty(context.Background(), nil, true, false, false); err != nil {
		t.Fatalf("broadcastDirty returned error: %v", err)
	}

	messages := drainBroadcastCh(hub)
	types := broadcastMessageTypes(t, messages)
	if len(types) != 2 || types[0] != ws.MessageTypeRuntimeDelta || types[1] != ws.MessageTypeAlert {
		t.Fatalf("expected alert resolution to broadcast runtime_delta then alert, got %v", types)
	}

	var deltaMessage wsVersionedRuntimeDeltaMessage
	if err := json.Unmarshal(messages[0], &deltaMessage); err != nil {
		t.Fatalf("decode alert resolution delta: %v", err)
	}
	deviceRuntime, ok := deltaMessage.Payload.Delta.Devices[deviceID.String()]
	if !ok {
		t.Fatalf("expected alert resolution delta for device %s", deviceID)
	}
	if got, ok := deviceRuntime["alert_status"]; !ok || got != string(domain.AlertStatusNormal) {
		t.Fatalf("alert_status patch = %#v, want %q", deviceRuntime["alert_status"], domain.AlertStatusNormal)
	}
	if got, ok := deviceRuntime["firing_alert_count"]; !ok || got != float64(0) {
		t.Fatalf("firing_alert_count patch = %#v, want explicit 0", deviceRuntime["firing_alert_count"])
	}
	if fullSnapshotCalls != 0 {
		t.Fatalf("full state snapshot calls = %d, want 0", fullSnapshotCalls)
	}
	if narrowSnapshotCalls != 1 {
		t.Fatalf("narrow state snapshot calls = %d, want 1", narrowSnapshotCalls)
	}
	assertUUIDSliceSetEqual(t, requestedIDs, map[uuid.UUID]struct{}{deviceID: {}})
}

func TestPipelineOrchestratorBroadcastDirty_DeviceOnlyWithoutRuntimeBaseFallsBackToFullSnapshot(t *testing.T) {
	pipeline, hub, store, _, deviceID := newBroadcastTestPipeline(t)
	peerID := uuid.New()
	pipeline.cache = newPipelineTestCache([]domain.Device{
		{
			ID:            deviceID,
			IP:            "192.0.2.40",
			Status:        domain.DeviceStatusProbing,
			SysName:       "dist-sw-1",
			HardwareModel: "CRS328-24P-4S+",
			Interfaces:    []domain.Interface{{IfName: "ether1", IfDescr: "uplink", Speed: 1_000_000_000}},
		},
		{
			ID:            peerID,
			IP:            "192.0.2.41",
			Status:        domain.DeviceStatusProbing,
			SysName:       "edge-sw-1",
			HardwareModel: "CRS326-24G-2S+",
			Interfaces:    []domain.Interface{{IfName: "ether1", IfDescr: "uplink", Speed: 1_000_000_000}},
		},
	}, nil)
	store.Update(state.StateUpdate{
		DeviceID:        peerID,
		VolatilityClass: domain.VolatilityClassPerformance,
		Metrics: &domain.DeviceMetrics{
			DeviceID:    peerID,
			CPUPercent:  floatPtr(18),
			CollectedAt: time.Date(2026, 4, 19, 12, 0, 0, 0, time.UTC),
		},
		PollSuccess:      true,
		ExpectedInterval: 30 * time.Second,
		Timestamp:        time.Date(2026, 4, 19, 12, 0, 0, 0, time.UTC),
	})

	pipeline.runtime.mu.Lock()
	pipeline.runtime.lastSnapshot = nil
	pipeline.runtime.prevHashes = nil
	pipeline.runtime.mu.Unlock()

	previousSnapshotAll := snapshotAllPipelineState
	previousSnapshotFor := snapshotPipelineStateFor
	fullSnapshotCalls := 0
	narrowSnapshotCalls := 0
	snapshotAllPipelineState = func(store *state.Store) map[uuid.UUID]state.DeviceState {
		fullSnapshotCalls++
		return store.Snapshot()
	}
	snapshotPipelineStateFor = func(store *state.Store, ids []uuid.UUID) map[uuid.UUID]state.DeviceState {
		narrowSnapshotCalls++
		return store.SnapshotFor(ids)
	}
	t.Cleanup(func() {
		snapshotAllPipelineState = previousSnapshotAll
		snapshotPipelineStateFor = previousSnapshotFor
	})

	if err := pipeline.broadcaster.broadcastDirty(context.Background(), map[uuid.UUID]struct{}{deviceID: {}}, false, false, false); err != nil {
		t.Fatalf("broadcastDirty returned error: %v", err)
	}

	messages := drainBroadcastCh(hub)
	types := broadcastMessageTypes(t, messages)
	if len(types) != 1 || types[0] != ws.MessageTypeSnapshot {
		t.Fatalf("expected dirty device without runtime base to broadcast full snapshot, got %v", types)
	}

	var snapshotMessage wsVersionedSnapshotMessage
	if err := json.Unmarshal(messages[0], &snapshotMessage); err != nil {
		t.Fatalf("decode dirty-device fallback snapshot: %v", err)
	}
	if snapshotMessage.Payload.Snapshot == nil {
		t.Fatal("expected fallback snapshot payload")
	}
	if _, ok := snapshotMessage.Payload.Snapshot.Devices[deviceID.String()]; !ok {
		t.Fatalf("expected fallback snapshot to include dirty device %s", deviceID)
	}
	if _, ok := snapshotMessage.Payload.Snapshot.Devices[peerID.String()]; !ok {
		t.Fatalf("expected fallback snapshot to include non-dirty peer device %s", peerID)
	}
	if len(snapshotMessage.Payload.Snapshot.Devices) != 2 {
		t.Fatalf("fallback snapshot device count = %d, want 2", len(snapshotMessage.Payload.Snapshot.Devices))
	}
	if fullSnapshotCalls != 1 {
		t.Fatalf("full state snapshot calls = %d, want 1", fullSnapshotCalls)
	}
	if narrowSnapshotCalls != 0 {
		t.Fatalf("narrow state snapshot calls = %d, want 0", narrowSnapshotCalls)
	}
}

func TestPipelineOrchestratorBroadcastDirty_DeviceAndAlertWithoutRuntimeBaseFallsBackToFullSnapshotAndAlert(t *testing.T) {
	pipeline, hub, _, _, deviceID := newBroadcastTestPipeline(t)
	pipeline.runtime.setAlerts(map[uuid.UUID][]domain.AlertState{
		deviceID: {{
			DeviceID:  deviceID,
			Severity:  "critical",
			AlertName: "DeviceDown",
			State:     "firing",
			Summary:   "device down",
		}},
	})

	pipeline.runtime.mu.Lock()
	pipeline.runtime.lastSnapshot = nil
	pipeline.runtime.prevHashes = nil
	pipeline.runtime.mu.Unlock()

	previousSnapshotAll := snapshotAllPipelineState
	previousSnapshotFor := snapshotPipelineStateFor
	fullSnapshotCalls := 0
	narrowSnapshotCalls := 0
	snapshotAllPipelineState = func(store *state.Store) map[uuid.UUID]state.DeviceState {
		fullSnapshotCalls++
		return store.Snapshot()
	}
	snapshotPipelineStateFor = func(store *state.Store, ids []uuid.UUID) map[uuid.UUID]state.DeviceState {
		narrowSnapshotCalls++
		return store.SnapshotFor(ids)
	}
	t.Cleanup(func() {
		snapshotAllPipelineState = previousSnapshotAll
		snapshotPipelineStateFor = previousSnapshotFor
	})

	if err := pipeline.broadcaster.broadcastDirty(context.Background(), map[uuid.UUID]struct{}{deviceID: {}}, true, false, false); err != nil {
		t.Fatalf("broadcastDirty returned error: %v", err)
	}

	messages := drainBroadcastCh(hub)
	types := broadcastMessageTypes(t, messages)
	if len(types) != 2 || types[0] != ws.MessageTypeSnapshot || types[1] != ws.MessageTypeAlert {
		t.Fatalf("expected dirty device and alert without runtime base to broadcast snapshot then alert, got %v", types)
	}
	if fullSnapshotCalls != 1 {
		t.Fatalf("full state snapshot calls = %d, want 1", fullSnapshotCalls)
	}
	if narrowSnapshotCalls != 0 {
		t.Fatalf("narrow state snapshot calls = %d, want 0", narrowSnapshotCalls)
	}
}

func TestPipelineOrchestratorBroadcastLoop_DisabledFullResyncDoesNotSendSnapshot(t *testing.T) {
	pipeline, hub, _, _, _ := newBroadcastTestPipeline(t)
	pipeline.broadcastCoalesceWindow = 10 * time.Millisecond
	pipeline.fullResyncInterval = 0

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go pipeline.broadcaster.broadcastLoop(ctx)

	initialTypes := broadcastMessageTypes(t, waitForBroadcastMessages(t, hub, time.Second))
	if len(initialTypes) == 0 || initialTypes[0] != ws.MessageTypeSnapshot {
		t.Fatalf("expected initial snapshot, got %v", initialTypes)
	}

	time.Sleep(80 * time.Millisecond)

	if messages := drainBroadcastCh(hub); len(messages) != 0 {
		t.Fatalf("expected no periodic full resync snapshot when interval disabled, got %v", broadcastMessageTypes(t, messages))
	}
}

func TestPipelineOrchestratorBroadcastLoop_PeriodicFullResyncSendsSnapshot(t *testing.T) {
	pipeline, hub, _, _, _ := newBroadcastTestPipeline(t)
	pipeline.broadcastCoalesceWindow = 10 * time.Millisecond
	pipeline.fullResyncInterval = 40 * time.Millisecond

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go pipeline.broadcaster.broadcastLoop(ctx)

	initialTypes := broadcastMessageTypes(t, waitForBroadcastMessages(t, hub, time.Second))
	if len(initialTypes) == 0 || initialTypes[0] != ws.MessageTypeSnapshot {
		t.Fatalf("expected initial snapshot, got %v", initialTypes)
	}

	types := broadcastMessageTypes(t, waitForBroadcastMessages(t, hub, time.Second))
	if len(types) == 0 || types[0] != ws.MessageTypeSnapshot {
		t.Fatalf("expected periodic full resync snapshot, got %v", types)
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

	hub := ws.NewHub(ws.WithBroadcastRecorder())
	store := state.NewStore()
	pipeline := NewPipelineOrchestrator(
		newPipelineTestScheduler(),
		store,
		newPipelineTestCache([]domain.Device{device}, []domain.Link{link}),
		hub,
		nil,
		newPerformanceTestCollector(t),
		newOperationalTestCollector(t),
		newStaticTestCollector(t),
		collector.NewPrometheusCollector(&fakePrometheusClient{}),
		&fakeTopologyService{},
		newMockWorkerSettingsRepo(),
		make(chan struct{}, 1),
		nil,
		nil,
	)

	pipeline.taskRunner.runTask(context.Background(), scheduler.PollTask{
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
	performanceCollectedAt := performanceState.Metrics.CollectedAt.UTC().Format(time.RFC3339)

	pipeline.taskRunner.runTask(context.Background(), scheduler.PollTask{
		RunID:            11,
		Key:              scheduler.NewTaskKey(deviceID, domain.VolatilityClassOperational),
		VolatilityClass:  domain.VolatilityClassOperational,
		ExpectedInterval: 60 * time.Second,
		Device:           device,
	})
	pipeline.taskRunner.runTask(context.Background(), scheduler.PollTask{
		RunID:            12,
		Key:              scheduler.NewTaskKey(deviceID, domain.VolatilityClassStatic),
		VolatilityClass:  domain.VolatilityClassStatic,
		ExpectedInterval: 300 * time.Second,
		Device:           device,
	})

	pipeline.broadcaster.broadcastOnce(context.Background())

	var snapshotMessage wsVersionedSnapshotMessage
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

	if snapshotMessage.Payload.Snapshot == nil {
		t.Fatal("expected versioned snapshot payload")
	}
	metric, ok := snapshotMessage.Payload.Snapshot.Devices[deviceID.String()]
	if !ok {
		t.Fatalf("expected snapshot metric for device %s", deviceID)
	}
	if metric.LastCollectedAt == nil || *metric.LastCollectedAt != performanceCollectedAt {
		t.Fatalf("LastCollectedAt = %#v, want performance poll timestamp %q", metric.LastCollectedAt, performanceCollectedAt)
	}
	if metric.Reachability != string(state.ReachabilityUp) {
		t.Fatalf("Reachability = %q, want %q", metric.Reachability, state.ReachabilityUp)
	}
	if snapshotMessage.Payload.Snapshot.Devices[deviceID.String()].OperationalStatus != string(domain.DeviceStatusUp) {
		t.Fatalf("DeviceStatus = %q, want %q", snapshotMessage.Payload.Snapshot.Devices[deviceID.String()].OperationalStatus, domain.DeviceStatusUp)
	}
}

func TestPipelineOrchestratorTopologyChangedFiresAfterSnapshot(t *testing.T) {
	pipeline, hub, _, topologyNotify, _ := newBroadcastTestPipeline(t)
	topologyNotify <- struct{}{}

	pipeline.broadcaster.broadcastOnce(context.Background())

	types := broadcastMessageTypes(t, drainBroadcastCh(hub))
	if len(types) < 2 {
		t.Fatalf("expected snapshot and topology_changed messages, got %v", types)
	}
	if types[0] != ws.MessageTypeSnapshot || types[1] != ws.MessageTypeTopologyChanged {
		t.Fatalf("expected snapshot before topology_changed, got %v", types)
	}
}

func TestPipelineOrchestratorTopologyChangedInvalidatesWithoutForcedSnapshotWhenOverviewDeltaNil(t *testing.T) {
	pipeline, hub, _, topologyNotify, _ := newBroadcastTestPipeline(t)

	pipeline.broadcaster.broadcastOnce(context.Background())
	drainBroadcastCh(hub)

	topologyNotify <- struct{}{}
	pipeline.broadcaster.broadcastOnce(context.Background())

	types := broadcastMessageTypes(t, drainBroadcastCh(hub))
	if len(types) != 1 || types[0] != ws.MessageTypeTopologyChanged {
		t.Fatalf("expected topology_changed invalidation without forced snapshot, got %v", types)
	}
}

func TestPipelineOrchestratorBroadcastOnceRecordsStartupReloadMetrics(t *testing.T) {
	registry := observability.ResetDefaultForTest()
	t.Cleanup(func() {
		observability.ResetDefaultForTest()
	})

	pipeline, hub, _, _, _ := newBroadcastTestPipeline(t)
	pipeline.broadcaster.broadcastOnce(context.Background())

	types := broadcastMessageTypes(t, drainBroadcastCh(hub))
	if len(types) == 0 || types[0] != ws.MessageTypeSnapshot {
		t.Fatalf("expected startup snapshot broadcast, got %v", types)
	}

	metrics := string(registry.MarshalPrometheus())
	if !strings.Contains(metrics, `theia_refresh_snapshot_build_seconds_count{mode="full",result="success"} 1`) {
		t.Fatalf("expected full snapshot build metric, got:\n%s", metrics)
	}
	if !strings.Contains(metrics, `theia_refresh_topology_reload_total{reason="startup"} 1`) {
		t.Fatalf("expected startup reload reason metric, got:\n%s", metrics)
	}
}

func TestPipelineOrchestratorBroadcastDirtyRecordsFullSnapshotReasons(t *testing.T) {
	pipeline, hub, _, topologyNotify, _ := newBroadcastTestPipeline(t)

	pipeline.broadcaster.broadcastOnce(context.Background())
	drainBroadcastCh(hub)

	topologyRegistry := observability.ResetDefaultForTest()
	topologyNotify <- struct{}{}
	if err := pipeline.broadcaster.broadcastDirty(context.Background(), nil, false, true, false); err != nil {
		t.Fatalf("broadcastDirty topology refresh: %v", err)
	}

	topologyTypes := broadcastMessageTypes(t, drainBroadcastCh(hub))
	if len(topologyTypes) != 1 || topologyTypes[0] != ws.MessageTypeTopologyChanged {
		t.Fatalf("expected topology refresh invalidation only, got %v", topologyTypes)
	}
	topologyMetrics := string(topologyRegistry.MarshalPrometheus())
	if !strings.Contains(topologyMetrics, `theia_refresh_topology_reload_total{reason="topology_dirty"} 1`) {
		t.Fatalf("expected topology_dirty reload reason metric, got:\n%s", topologyMetrics)
	}
	if strings.Contains(topologyMetrics, `theia_refresh_snapshot_build_seconds_count{mode="full",result="success"} 1`) {
		t.Fatalf("expected topology invalidation not to build a full snapshot, got:\n%s", topologyMetrics)
	}

	fullResyncRegistry := observability.ResetDefaultForTest()
	if err := pipeline.broadcaster.broadcastDirty(context.Background(), nil, false, false, true); err != nil {
		t.Fatalf("broadcastDirty forced refresh: %v", err)
	}

	fullResyncTypes := broadcastMessageTypes(t, drainBroadcastCh(hub))
	if len(fullResyncTypes) == 0 || fullResyncTypes[0] != ws.MessageTypeSnapshot {
		t.Fatalf("expected forced full resync snapshot, got %v", fullResyncTypes)
	}
	if len(fullResyncTypes) > 1 && fullResyncTypes[1] == ws.MessageTypeTopologyChanged {
		t.Fatalf("forced full resync should not emit topology_changed, got %v", fullResyncTypes)
	}
	fullResyncMetrics := string(fullResyncRegistry.MarshalPrometheus())
	if !strings.Contains(fullResyncMetrics, `theia_refresh_topology_reload_total{reason="full_resync"} 1`) {
		t.Fatalf("expected full_resync reload reason metric, got:\n%s", fullResyncMetrics)
	}

	t.Cleanup(func() {
		observability.ResetDefaultForTest()
	})
}

func TestPipelineOrchestratorBroadcastDirty_DuplicateStateBurstBroadcastsRuntimeDelta(t *testing.T) {
	pipeline, hub, store, _, deviceID := newBroadcastTestPipeline(t)

	pipeline.broadcaster.broadcastOnce(context.Background())
	drainBroadcastCh(hub)

	for i := 0; i < 40; i++ {
		cpu := float64(70 + i)
		at := time.Date(2026, 4, 18, 12, 0, i, 0, time.UTC)
		store.Update(state.StateUpdate{
			DeviceID:        deviceID,
			VolatilityClass: domain.VolatilityClassPerformance,
			Metrics: &domain.DeviceMetrics{
				DeviceID:    deviceID,
				CPUPercent:  &cpu,
				CollectedAt: at,
			},
			PollSuccess:      true,
			ExpectedInterval: 30 * time.Second,
			Timestamp:        at,
		})
	}

	if err := pipeline.broadcaster.broadcastDirty(context.Background(), map[uuid.UUID]struct{}{deviceID: {}}, false, false, false); err != nil {
		t.Fatalf("broadcastDirty overflow recovery: %v", err)
	}

	messages := drainBroadcastCh(hub)
	types := broadcastMessageTypes(t, messages)
	if len(types) != 1 || types[0] != ws.MessageTypeRuntimeDelta {
		t.Fatalf("expected duplicate state burst to broadcast runtime_delta, got %v", types)
	}
}

func TestPipelineDuplicateStateBurstSequenceStaysStableAcrossReplay(t *testing.T) {
	pipeline, hub, store, _, deviceID := newBroadcastTestPipeline(t)

	pipeline.broadcaster.broadcastOnce(context.Background())
	drainBroadcastCh(hub)

	first := forcePipelineDuplicateStateBurst(t, pipeline, store, deviceID, 12, time.Date(2026, 4, 18, 12, 20, 0, 0, time.UTC))
	second := forcePipelineDuplicateStateBurst(t, pipeline, store, deviceID, 12, time.Date(2026, 4, 18, 12, 21, 0, 0, time.UTC))

	if len(first) != 1 {
		t.Fatalf("first duplicate-burst sequence = %v, want [runtime_delta]", first)
	}
	if len(second) != 1 {
		t.Fatalf("second duplicate-burst sequence = %v, want [runtime_delta]", second)
	}
	if first[0] != ws.MessageTypeRuntimeDelta {
		t.Fatalf("first duplicate-burst ordering = %v, want [runtime_delta]", first)
	}
	if second[0] != ws.MessageTypeRuntimeDelta {
		t.Fatalf("second duplicate-burst ordering = %v, want [runtime_delta]", second)
	}
	if first[0] != second[0] {
		t.Fatalf("duplicate-burst ordering changed across bursts: first=%v second=%v", first, second)
	}
}

func forcePipelineDuplicateStateBurst(t *testing.T, pipeline *PipelineOrchestrator, store *state.Store, deviceID uuid.UUID, updates int, startedAt time.Time) []string {
	t.Helper()

	// Issue more updates than the buffered channel can hold. Repeated updates
	// for one device should coalesce to one dirty ID instead of forcing resync.
	for i := 0; i < 32+updates; i++ {
		cpu := float64(110 + i)
		at := startedAt.Add(time.Duration(i) * time.Second)
		store.Update(state.StateUpdate{
			DeviceID:        deviceID,
			VolatilityClass: domain.VolatilityClassPerformance,
			Metrics: &domain.DeviceMetrics{
				DeviceID:    deviceID,
				CPUPercent:  &cpu,
				CollectedAt: at,
			},
			PollSuccess:      true,
			ExpectedInterval: 30 * time.Second,
			Timestamp:        at,
		})
	}

	if err := pipeline.broadcaster.broadcastDirty(context.Background(), map[uuid.UUID]struct{}{deviceID: {}}, false, false, false); err != nil {
		t.Fatalf("broadcastDirty overflow recovery: %v", err)
	}

	sequence := broadcastMessageTypes(t, drainBroadcastCh(pipeline.hub))
	clearBufferedStateChanges(store)
	return sequence
}

func clearBufferedStateChanges(store *state.Store) {
	for {
		select {
		case <-store.Changes():
		default:
			return
		}
	}
}

func assertUUIDSliceSetEqual(t *testing.T, got []uuid.UUID, want map[uuid.UUID]struct{}) {
	t.Helper()

	if len(got) != len(want) {
		t.Fatalf("UUID set length = %d, want %d: got=%v want=%v", len(got), len(want), got, want)
	}
	seen := make(map[uuid.UUID]struct{}, len(got))
	for _, id := range got {
		if _, ok := want[id]; !ok {
			t.Fatalf("unexpected UUID %s in set %v, want %v", id, got, want)
		}
		seen[id] = struct{}{}
	}
	for id := range want {
		if _, ok := seen[id]; !ok {
			t.Fatalf("missing UUID %s from set %v", id, got)
		}
	}
}

func TestPipelineOrchestratorBroadcastDirty_DuplicateStateBurstWithTopologyDirtyInvalidatesTopology(t *testing.T) {
	pipeline, hub, store, _, deviceID := newBroadcastTestPipeline(t)

	pipeline.broadcaster.broadcastOnce(context.Background())
	drainBroadcastCh(hub)

	for i := 0; i < 40; i++ {
		cpu := float64(90 + i)
		at := time.Date(2026, 4, 18, 12, 10, i, 0, time.UTC)
		store.Update(state.StateUpdate{
			DeviceID:        deviceID,
			VolatilityClass: domain.VolatilityClassPerformance,
			Metrics: &domain.DeviceMetrics{
				DeviceID:    deviceID,
				CPUPercent:  &cpu,
				CollectedAt: at,
			},
			PollSuccess:      true,
			ExpectedInterval: 30 * time.Second,
			Timestamp:        at,
		})
	}

	if err := pipeline.broadcaster.broadcastDirty(context.Background(), map[uuid.UUID]struct{}{deviceID: {}}, false, true, false); err != nil {
		t.Fatalf("broadcastDirty overflow + topology recovery: %v", err)
	}

	types := broadcastMessageTypes(t, drainBroadcastCh(hub))
	if len(types) != 1 {
		t.Fatalf("expected topology_changed only for topology-dirty duplicate burst, got %v", types)
	}
	if types[0] != ws.MessageTypeTopologyChanged {
		t.Fatalf("expected topology_changed for topology-dirty duplicate burst, got %v", types)
	}
}

func TestPipelineOrchestratorBroadcastDirty_TopologyAndAlertsDirtyAlsoBroadcastsAlert(t *testing.T) {
	pipeline, hub, _, _, deviceID := newBroadcastTestPipeline(t)

	pipeline.broadcaster.broadcastOnce(context.Background())
	drainBroadcastCh(hub)
	pipeline.prometheusMonitor.setAlerts(map[uuid.UUID][]domain.AlertState{
		deviceID: {{
			DeviceID:  deviceID,
			Severity:  "critical",
			AlertName: "DeviceDown",
			State:     "firing",
			Summary:   "device down",
		}},
	})
	drainBroadcastCh(hub)

	if err := pipeline.broadcaster.broadcastDirty(context.Background(), nil, true, true, false); err != nil {
		t.Fatalf("broadcastDirty topology + alerts recovery: %v", err)
	}

	types := broadcastMessageTypes(t, drainBroadcastCh(hub))
	if len(types) < 2 {
		t.Fatalf("expected topology_changed and alert, got %v", types)
	}
	if types[0] != ws.MessageTypeTopologyChanged || types[1] != ws.MessageTypeAlert {
		t.Fatalf("expected topology_changed before alert, got %v", types)
	}
}

func TestPipelineOrchestratorPrometheusStatusOnlyBroadcastsOnTransition(t *testing.T) {
	pipeline, hub, _, _, _ := newBroadcastTestPipeline(t)

	pipeline.prometheusMonitor.publishStatus(ws.PrometheusStatusPayload{})
	if messages := drainBroadcastCh(hub); len(messages) != 0 {
		t.Fatalf("expected no broadcast for unchanged disabled state, got %d message(s)", len(messages))
	}

	pipeline.prometheusMonitor.publishStatus(ws.PrometheusStatusPayload{
		Enabled:   true,
		Available: false,
		Error:     "prometheus down",
	})
	firstTypes := broadcastMessageTypes(t, drainBroadcastCh(hub))
	if len(firstTypes) != 1 || firstTypes[0] != ws.MessageTypePrometheusStatus {
		t.Fatalf("expected one prometheus_status message on failure transition, got %v", firstTypes)
	}

	pipeline.prometheusMonitor.publishStatus(ws.PrometheusStatusPayload{
		Enabled:   true,
		Available: false,
		Error:     "prometheus down",
	})
	if messages := drainBroadcastCh(hub); len(messages) != 0 {
		t.Fatalf("expected no duplicate broadcast without state transition, got %d message(s)", len(messages))
	}

	pipeline.prometheusMonitor.publishStatus(ws.PrometheusStatusPayload{
		Enabled:   true,
		Available: true,
	})
	secondTypes := broadcastMessageTypes(t, drainBroadcastCh(hub))
	if len(secondTypes) != 1 || secondTypes[0] != ws.MessageTypePrometheusStatus {
		t.Fatalf("expected one prometheus_status message on recovery transition, got %v", secondTypes)
	}
}

func TestPipelineOrchestratorRunTask_PerformancePollSendsOnlySelectedDeviceLinkMetricsToSubscribedClient(t *testing.T) {
	hub := ws.NewHub(ws.WithBroadcastRecorder())
	pipeline := newDetailSubscriptionTestPipeline(t, hub)
	device := newDetailSubscriptionTestDevice()
	device.MetricsSource = domain.MetricsSourcePrometheus
	pipeline.cache, _ = attachDetailSubscriptionTopology(device)
	pipeline.stateStore.Update(state.StateUpdate{
		DeviceID:         device.ID,
		VolatilityClass:  domain.VolatilityClassOperational,
		PollSuccess:      true,
		ExpectedInterval: 30 * time.Second,
		Timestamp:        time.Now().Add(-time.Second),
	})
	pipeline.runtime.prevCounters[device.ID] = map[string]collector.CounterBaseline{
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

	pipeline.taskRunner.runTask(context.Background(), scheduler.PollTask{
		RunID:            1,
		Key:              scheduler.NewTaskKey(device.ID, domain.VolatilityClassPerformance),
		VolatilityClass:  domain.VolatilityClassPerformance,
		ExpectedInterval: 30 * time.Second,
		Device:           device,
	})

	message := readSnapshotDeltaMessage(t, subscriber)
	metric, ok := message.Payload.Delta.Devices[device.ID.String()]
	if !ok {
		t.Fatalf("expected detail delta for device %s", device.ID)
	}
	if len(message.Payload.Delta.Links) != 1 {
		t.Fatalf("expected targeted detail delta to contain 1 links entry, got %d", len(message.Payload.Delta.Links))
	}
	var linkMetrics ws.LinkRuntimeDTO
	for _, linkRuntime := range message.Payload.Delta.Links {
		linkMetrics = linkRuntime
	}
	if linkMetrics.LinkID == "" {
		t.Fatalf("expected targeted detail delta link for device %s", device.ID)
	}
	if linkMetrics.SourceDeviceID != device.ID.String() {
		t.Fatalf("Links[*].SourceDeviceID = %q, want %q", linkMetrics.SourceDeviceID, device.ID)
	}
	if linkMetrics.SourceIfName != "ether1" {
		t.Fatalf("Links[*].SourceIfName = %q, want ether1", linkMetrics.SourceIfName)
	}
	if linkMetrics.TxBps == nil {
		t.Fatalf("Links[*].TxBps = nil, want value")
	}
	if linkMetrics.RxBps == nil {
		t.Fatalf("Links[*].RxBps = nil, want value")
	}
	if metric.Health == "" {
		t.Fatal("expected health field in detail delta")
	}
	if metric.Reachability != string(state.ReachabilityUp) {
		t.Fatalf("Reachability = %q, want %q", metric.Reachability, state.ReachabilityUp)
	}
	if metric.LastCollectedAt == nil || *metric.LastCollectedAt == "" {
		t.Fatal("expected last_collected_at in detail delta")
	}
	if messages := drainBroadcastCh(hub); len(messages) != 0 {
		t.Fatalf("expected no overview broadcast during targeted detail send, got %d message(s)", len(messages))
	}
}

func assertOperationalDetailDeltaKeepsPerformanceMetricTimestamp(t *testing.T) {
	t.Helper()

	hub := ws.NewHub(ws.WithBroadcastRecorder())
	pipeline := newDetailSubscriptionTestPipeline(t, hub)
	device := newDetailSubscriptionTestDevice()
	pipeline.cache, _ = attachDetailSubscriptionTopology(device)
	pipeline.runtime.prevCounters[device.ID] = map[string]collector.CounterBaseline{
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

	pipeline.taskRunner.runTask(context.Background(), scheduler.PollTask{
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
	performanceCollectedAt := performanceState.Metrics.CollectedAt.UTC().Format(time.RFC3339)

	performanceMessage := readSnapshotDeltaMessage(t, subscriber)
	performanceMetric, ok := performanceMessage.Payload.Delta.Devices[device.ID.String()]
	if !ok {
		t.Fatalf("expected performance detail delta for device %s", device.ID)
	}
	if performanceMetric.LastCollectedAt == nil || *performanceMetric.LastCollectedAt == "" {
		t.Fatal("expected performance detail delta to include last_collected_at")
	}

	pipeline.taskRunner.runTask(context.Background(), scheduler.PollTask{
		RunID:            21,
		Key:              scheduler.NewTaskKey(device.ID, domain.VolatilityClassOperational),
		VolatilityClass:  domain.VolatilityClassOperational,
		ExpectedInterval: 60 * time.Second,
		Device:           device,
	})

	message := readSnapshotDeltaMessage(t, subscriber)
	metric, ok := message.Payload.Delta.Devices[device.ID.String()]
	if !ok {
		t.Fatalf("expected detail delta for device %s", device.ID)
	}
	if len(message.Payload.Delta.Links) != 1 {
		t.Fatalf("expected operational detail delta to keep 1 link for %s, got %d", device.ID, len(message.Payload.Delta.Links))
	}
	for _, linkMetrics := range message.Payload.Delta.Links {
		if linkMetrics.SourceIfName != "ether1" {
			t.Fatalf("Links[*].SourceIfName = %q, want ether1", linkMetrics.SourceIfName)
		}
		if linkMetrics.TxBps == nil {
			t.Fatalf("Links[*].TxBps = nil, want value")
		}
		if linkMetrics.RxBps == nil {
			t.Fatalf("Links[*].RxBps = nil, want value")
		}
	}
	if metric.Reachability != string(state.ReachabilityUp) {
		t.Fatalf("Reachability = %q, want %q", metric.Reachability, state.ReachabilityUp)
	}
	if metric.LastCollectedAt == nil || *metric.LastCollectedAt != performanceCollectedAt {
		t.Fatalf("LastCollectedAt = %#v, want performance poll timestamp %q", metric.LastCollectedAt, performanceCollectedAt)
	}
}

func TestPipelineOrchestratorRunTask_DetailDeltaKeepsPerformanceMetricTimestampAfterOperationalPoll(t *testing.T) {
	assertOperationalDetailDeltaKeepsPerformanceMetricTimestamp(t)
}

func TestPipelineOrchestratorRunTask_OperationalPollSendsDetailDeltaToSubscribedClient(t *testing.T) {
	assertOperationalDetailDeltaKeepsPerformanceMetricTimestamp(t)
}

func TestPipelineOrchestratorRunTask_DetailDeltaDoesNotReachUnsubscribedClient(t *testing.T) {
	hub := ws.NewHub(ws.WithBroadcastRecorder())
	pipeline := newDetailSubscriptionTestPipeline(t, hub)
	device := newDetailSubscriptionTestDevice()
	pipeline.cache, _ = attachDetailSubscriptionTopology(device)
	pipeline.stateStore.Update(state.StateUpdate{
		DeviceID:         device.ID,
		VolatilityClass:  domain.VolatilityClassOperational,
		PollSuccess:      true,
		ExpectedInterval: 30 * time.Second,
		Timestamp:        time.Now().Add(-time.Second),
	})
	pipeline.runtime.prevCounters[device.ID] = map[string]collector.CounterBaseline{
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

	pipeline.taskRunner.runTask(context.Background(), scheduler.PollTask{
		RunID:            3,
		Key:              scheduler.NewTaskKey(device.ID, domain.VolatilityClassPerformance),
		VolatilityClass:  domain.VolatilityClassPerformance,
		ExpectedInterval: 30 * time.Second,
		Device:           device,
	})

	message := readSnapshotDeltaMessage(t, subscriber)
	if len(message.Payload.Delta.Links) == 0 {
		t.Fatalf("expected subscribed client detail delta links for device %s", device.ID)
	}
	assertNoWebSocketMessage(t, unsubscribed)
}

func TestPipelineOrchestratorPublishSubscribedDetailDeltaJSONIncludesDetailOnlyDeviceMetricFields(t *testing.T) {
	hub := ws.NewHub(ws.WithBroadcastRecorder())
	pipeline := newDetailSubscriptionTestPipeline(t, hub)
	device := newDetailSubscriptionTestDevice()
	pipeline.cache, _ = attachDetailSubscriptionTopology(device)
	collectedAt := time.Date(2026, 4, 13, 12, 0, 0, 0, time.UTC)
	expectedInterval := 45 * time.Second

	pipeline.stateStore.Update(state.StateUpdate{
		DeviceID:        device.ID,
		VolatilityClass: domain.VolatilityClassPerformance,
		Metrics: &domain.DeviceMetrics{
			DeviceID:    device.ID,
			CPUPercent:  floatPtr(55),
			MemPercent:  floatPtr(62),
			TempCelsius: floatPtr(48),
			UptimeSecs:  floatPtr(3600),
			CollectedAt: collectedAt,
		},
		PollSuccess:      true,
		ExpectedInterval: expectedInterval,
		Timestamp:        collectedAt,
	})
	pipeline.runtime.promStatus = ws.PrometheusStatusPayload{Enabled: true, Available: false}
	pipeline.runtime.alerts = map[uuid.UUID][]domain.AlertState{
		device.ID: {{
			DeviceID:  device.ID,
			Severity:  "critical",
			AlertName: "DeviceDown",
			State:     "firing",
		}},
	}
	pipeline.runtime.overviewVersion = 7

	wsURL := newDetailSubscriptionTestServer(t, hub)
	subscriber := connectDetailSubscriptionClient(t, wsURL)
	drainBootstrapMessages(t, subscriber)
	subscribeDetail(t, subscriber, device.ID)
	waitForDetailSubscribers(t, hub, device.ID, 1)

	pipeline.publishSubscribedDetailDelta(device)

	subscriber.SetReadDeadline(time.Now().Add(time.Second))
	defer subscriber.SetReadDeadline(time.Time{})

	_, raw, err := subscriber.ReadMessage()
	if err != nil {
		t.Fatalf("failed to read websocket detail delta: %v", err)
	}

	var decoded map[string]any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("failed to decode websocket detail delta: %v", err)
	}
	if got := decoded["type"]; got != ws.MessageTypeSnapshotDelta {
		t.Fatalf("message type = %#v, want %q", got, ws.MessageTypeSnapshotDelta)
	}

	payload, ok := decoded["payload"].(map[string]any)
	if !ok {
		t.Fatalf("payload = %#v, want object", decoded["payload"])
	}
	if got := payload["base_version"]; got != float64(7) {
		t.Fatalf("base_version = %#v, want 7", got)
	}
	if got := payload["version"]; got != float64(7) {
		t.Fatalf("version = %#v, want 7", got)
	}
	deltaPayload, ok := payload["delta"].(map[string]any)
	if !ok {
		t.Fatalf("payload.delta = %#v, want object", payload["delta"])
	}
	deviceMetrics, ok := deltaPayload["devices"].(map[string]any)
	if !ok {
		t.Fatalf("payload.delta.devices = %#v, want object", deltaPayload["devices"])
	}
	metric, ok := deviceMetrics[device.ID.String()].(map[string]any)
	if !ok {
		t.Fatalf("payload.delta.devices[%s] = %#v, want object", device.ID, deviceMetrics[device.ID.String()])
	}

	if got, ok := metric["temp_celsius"]; !ok {
		t.Fatal("expected detail subscription devices to include temp_celsius")
	} else if got != 48.0 {
		t.Fatalf("temp_celsius = %#v, want 48", got)
	}
	if got, ok := metric["uptime_secs"]; !ok {
		t.Fatal("expected detail subscription devices to include uptime_secs")
	} else if got != 3600.0 {
		t.Fatalf("uptime_secs = %#v, want 3600", got)
	}
	if got, ok := metric["last_polled_at"]; !ok {
		t.Fatal("expected detail subscription devices to include last_polled_at")
	} else if got != collectedAt.Format(time.RFC3339) {
		t.Fatalf("last_polled_at = %#v, want %q", got, collectedAt.Format(time.RFC3339))
	}
	if got, ok := metric["expected_poll_interval_seconds"]; !ok {
		t.Fatal("expected detail subscription devices to include expected_poll_interval_seconds")
	} else if got != expectedInterval.Seconds() {
		t.Fatalf("expected_poll_interval_seconds = %#v, want %v", got, expectedInterval.Seconds())
	}
	if got, ok := metric["alert_status"]; !ok {
		t.Fatal("expected detail subscription devices to include alert_status")
	} else if got != string(domain.AlertStatusDown) {
		t.Fatalf("alert_status = %#v, want %q", got, domain.AlertStatusDown)
	}
	if got, ok := metric["firing_alert_count"]; !ok {
		t.Fatal("expected detail subscription devices to include firing_alert_count")
	} else if got != float64(1) {
		t.Fatalf("firing_alert_count = %#v, want 1", got)
	}
}

func TestPipelineOrchestratorPublishSubscribedDetailDelta_UsesPrometheusStatusAndAlerts(t *testing.T) {
	hub := ws.NewHub(ws.WithBroadcastRecorder())
	pipeline := newDetailSubscriptionTestPipeline(t, hub)
	device := newDetailSubscriptionTestDevice()
	device.MetricsSource = domain.MetricsSourcePrometheus
	pipeline.cache, _ = attachDetailSubscriptionTopology(device)
	pipeline.stateStore.Update(state.StateUpdate{
		DeviceID:        device.ID,
		VolatilityClass: domain.VolatilityClassOperational,
		PollSuccess:     true,
		Timestamp:       time.Date(2026, 4, 13, 12, 0, 0, 0, time.UTC),
	})
	pipeline.runtime.promStatus = ws.PrometheusStatusPayload{Enabled: true, Available: false}
	pipeline.runtime.alerts = map[uuid.UUID][]domain.AlertState{
		device.ID: {{
			DeviceID:  device.ID,
			Severity:  "critical",
			AlertName: "DeviceDown",
			State:     "firing",
		}},
	}

	wsURL := newDetailSubscriptionTestServer(t, hub)
	subscriber := connectDetailSubscriptionClient(t, wsURL)
	drainBootstrapMessages(t, subscriber)
	subscribeDetail(t, subscriber, device.ID)
	waitForDetailSubscribers(t, hub, device.ID, 1)

	pipeline.publishSubscribedDetailDelta(device)
	message := readSnapshotDeltaMessage(t, subscriber)
	metric, ok := message.Payload.Delta.Devices[device.ID.String()]
	if !ok {
		t.Fatalf("expected detail delta for device %s", device.ID)
	}
	if metric.PrimaryReason != normalizedReasonUpstreamUnavailable {
		t.Fatalf("PrimaryReason = %q, want %q", metric.PrimaryReason, normalizedReasonUpstreamUnavailable)
	}
	if metric.MetricsReason != normalizedReasonUpstreamUnavailable {
		t.Fatalf("MetricsReason = %q, want %q", metric.MetricsReason, normalizedReasonUpstreamUnavailable)
	}
	if metric.AlertStatus != string(domain.AlertStatusDown) {
		t.Fatalf("AlertStatus = %q, want %q", metric.AlertStatus, domain.AlertStatusDown)
	}
	if metric.FiringAlertCount != 1 {
		t.Fatalf("FiringAlertCount = %d, want 1", metric.FiringAlertCount)
	}
}
