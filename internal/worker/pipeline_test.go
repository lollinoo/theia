package worker

// This file exercises pipeline behavior so refactors preserve the documented contract.

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log"
	"math"
	"net"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"github.com/gosnmp/gosnmp"

	theiaapi "github.com/lollinoo/theia/internal/api"
	"github.com/lollinoo/theia/internal/cache"
	"github.com/lollinoo/theia/internal/collector"
	"github.com/lollinoo/theia/internal/domain"
	"github.com/lollinoo/theia/internal/logging"
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

func assertPipelineFloatPtrEqual(t *testing.T, got *float64, want float64, label string) {
	t.Helper()
	if got == nil {
		t.Fatalf("%s = nil, want %v", label, want)
	}
	if *got != want {
		t.Fatalf("%s = %v, want %v", label, *got, want)
	}
}

type fakeTopologyService struct {
	mu     sync.Mutex
	calls  int
	lastID uuid.UUID
	lastIn service.StaticDiscoveryInput
	inputs []service.StaticDiscoveryInput
	result service.StaticPersistenceResult
	err    error
}

func (s *fakeTopologyService) ApplyStaticDiscovery(deviceID uuid.UUID, input service.StaticDiscoveryInput) (service.StaticPersistenceResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.calls++
	s.lastID = deviceID
	s.lastIn = input
	s.inputs = append(s.inputs, input)
	result := s.result
	if !input.SkipTopologyMaterialization {
		result.TopologyMaterialized = true
	}
	return result, s.err
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

func TestPipelineTaskRunnerPersistStaticDiscoverySkipsUnchangedRegularResult(t *testing.T) {
	registry := observability.ResetDefaultForTest()
	deviceID := uuid.New()
	topologyService := &fakeTopologyService{}
	pipeline := &PipelineOrchestrator{topologyService: topologyService}
	runner := &pipelineTaskRunner{pipeline: pipeline}
	result := staticDiscoveryDedupeResult()

	runner.persistStaticDiscovery(domain.Device{ID: deviceID}, result)
	runner.persistStaticDiscovery(domain.Device{ID: deviceID}, result)

	topologyService.mu.Lock()
	defer topologyService.mu.Unlock()
	if topologyService.calls != 1 {
		t.Fatalf("ApplyStaticDiscovery calls = %d, want 1 for unchanged regular static result", topologyService.calls)
	}
	metrics := string(registry.MarshalPrometheus())
	if !strings.Contains(metrics, `theia_static_persistence_skips_total{reason="unchanged"} 1`) {
		t.Fatalf("expected unchanged static persistence skip metric, got:\n%s", metrics)
	}
}

func TestPipelineTaskRunnerPersistStaticDiscoverySkipsCooldownSuppressedInterfaceFields(t *testing.T) {
	deviceID := uuid.New()
	topologyService := &fakeTopologyService{}
	pipeline := &PipelineOrchestrator{topologyService: topologyService}
	runner := &pipelineTaskRunner{pipeline: pipeline}
	full := staticDiscoveryDedupeResult()
	suppressed := staticDiscoveryDedupeResult()
	suppressed.Interfaces[0].IfDescr = ""
	suppressed.Interfaces[0].IfName = ""
	device := domain.Device{
		ID:         deviceID,
		Interfaces: append([]domain.Interface(nil), full.Interfaces...),
	}

	runner.persistStaticDiscovery(domain.Device{ID: deviceID}, full)
	runner.persistStaticDiscovery(device, suppressed)

	topologyService.mu.Lock()
	defer topologyService.mu.Unlock()
	if topologyService.calls != 1 {
		t.Fatalf("ApplyStaticDiscovery calls = %d, want 1 when current device already has cooldown-suppressed interface fields", topologyService.calls)
	}
}

func TestPipelineTaskRunnerPersistStaticDiscoveryPersistsChangedRegularResult(t *testing.T) {
	deviceID := uuid.New()
	topologyService := &fakeTopologyService{}
	pipeline := &PipelineOrchestrator{topologyService: topologyService}
	runner := &pipelineTaskRunner{pipeline: pipeline}
	first := staticDiscoveryDedupeResult()
	second := staticDiscoveryDedupeResult()
	second.Interfaces[0].Speed = 10_000_000_000

	runner.persistStaticDiscovery(domain.Device{ID: deviceID}, first)
	runner.persistStaticDiscovery(domain.Device{ID: deviceID}, second)

	topologyService.mu.Lock()
	defer topologyService.mu.Unlock()
	if topologyService.calls != 2 {
		t.Fatalf("ApplyStaticDiscovery calls = %d, want 2 after interface speed changes", topologyService.calls)
	}
}

func TestPipelineTaskRunnerPersistStaticDiscoverySkipsUnchangedTopologyMaterialization(t *testing.T) {
	deviceID := uuid.New()
	topologyService := &fakeTopologyService{}
	pipeline := &PipelineOrchestrator{topologyService: topologyService}
	runner := &pipelineTaskRunner{pipeline: pipeline}
	first := staticDiscoveryDedupeResult()
	changedInterface := staticDiscoveryDedupeResult()
	changedInterface.Interfaces[0].Speed = 10_000_000_000

	runner.persistStaticDiscovery(domain.Device{ID: deviceID}, first)
	runner.persistStaticDiscovery(domain.Device{ID: deviceID}, changedInterface)

	topologyService.mu.Lock()
	defer topologyService.mu.Unlock()
	if topologyService.calls != 2 {
		t.Fatalf("ApplyStaticDiscovery calls = %d, want 2 after interface speed changes", topologyService.calls)
	}
	if len(topologyService.inputs) != 2 {
		t.Fatalf("captured inputs = %d, want 2", len(topologyService.inputs))
	}
	if topologyService.inputs[0].SkipTopologyMaterialization {
		t.Fatal("first static persistence skipped topology materialization, want initial materialization")
	}
	if !topologyService.inputs[1].SkipTopologyMaterialization {
		t.Fatal("interface-only static change did not skip unchanged topology materialization")
	}
}

func TestPipelineTaskRunnerPersistStaticDiscoveryMaterializesChangedTopology(t *testing.T) {
	deviceID := uuid.New()
	topologyService := &fakeTopologyService{}
	pipeline := &PipelineOrchestrator{topologyService: topologyService}
	runner := &pipelineTaskRunner{pipeline: pipeline}
	first := staticDiscoveryDedupeResult()
	changedTopology := staticDiscoveryDedupeResult()
	changedTopology.Neighbors[0].RemotePortID = "ether3"

	runner.persistStaticDiscovery(domain.Device{ID: deviceID}, first)
	runner.persistStaticDiscovery(domain.Device{ID: deviceID}, changedTopology)

	topologyService.mu.Lock()
	defer topologyService.mu.Unlock()
	if topologyService.calls != 2 {
		t.Fatalf("ApplyStaticDiscovery calls = %d, want 2 after neighbor changes", topologyService.calls)
	}
	if len(topologyService.inputs) != 2 {
		t.Fatalf("captured inputs = %d, want 2", len(topologyService.inputs))
	}
	if topologyService.inputs[1].SkipTopologyMaterialization {
		t.Fatal("changed topology skipped topology materialization")
	}
}

func TestPipelineTaskRunnerPersistStaticDiscoveryMaterializesWhenPreviousTopologyHadUnresolvedNeighbors(t *testing.T) {
	deviceID := uuid.New()
	topologyService := &fakeTopologyService{
		result: service.StaticPersistenceResult{UnresolvedNeighbors: 1},
	}
	pipeline := &PipelineOrchestrator{topologyService: topologyService}
	runner := &pipelineTaskRunner{pipeline: pipeline}
	first := staticDiscoveryDedupeResult()
	changedInterface := staticDiscoveryDedupeResult()
	changedInterface.Interfaces[0].Speed = 10_000_000_000

	runner.persistStaticDiscovery(domain.Device{ID: deviceID}, first)
	runner.persistStaticDiscovery(domain.Device{ID: deviceID}, changedInterface)

	topologyService.mu.Lock()
	defer topologyService.mu.Unlock()
	if topologyService.calls != 2 {
		t.Fatalf("ApplyStaticDiscovery calls = %d, want 2 after interface speed changes", topologyService.calls)
	}
	if len(topologyService.inputs) != 2 {
		t.Fatalf("captured inputs = %d, want 2", len(topologyService.inputs))
	}
	if topologyService.inputs[1].SkipTopologyMaterialization {
		t.Fatal("previous unresolved topology allowed topology materialization skip")
	}
}

func TestPipelineTaskRunnerPersistStaticDiscoverySkipsUnchangedResultAfterOldSelfHealDeadline(t *testing.T) {
	registry := observability.ResetDefaultForTest()
	deviceID := uuid.New()
	now := time.Date(2026, 6, 17, 12, 0, 0, 0, time.UTC)
	topologyService := &fakeTopologyService{}
	pipeline := &PipelineOrchestrator{
		topologyService:      topologyService,
		staticPersistenceNow: func() time.Time { return now },
	}
	runner := &pipelineTaskRunner{pipeline: pipeline}
	result := staticDiscoveryDedupeResult()

	runner.persistStaticDiscovery(domain.Device{ID: deviceID}, result)
	now = now.Add(61 * time.Minute)
	runner.persistStaticDiscovery(domain.Device{ID: deviceID}, result)

	topologyService.mu.Lock()
	defer topologyService.mu.Unlock()
	if topologyService.calls != 1 {
		t.Fatalf("ApplyStaticDiscovery calls = %d, want 1 after old unchanged self-heal deadline", topologyService.calls)
	}
	metrics := string(registry.MarshalPrometheus())
	if !strings.Contains(metrics, `theia_static_persistence_skips_total{reason="unchanged"} 1`) {
		t.Fatalf("expected unchanged static persistence skip metric, got:\n%s", metrics)
	}
}

func TestPipelineTaskRunnerPersistStaticDiscoveryPersistsChangedResultAfterOldSelfHealDeadline(t *testing.T) {
	deviceID := uuid.New()
	now := time.Date(2026, 6, 17, 12, 0, 0, 0, time.UTC)
	topologyService := &fakeTopologyService{}
	pipeline := &PipelineOrchestrator{
		topologyService:      topologyService,
		staticPersistenceNow: func() time.Time { return now },
	}
	runner := &pipelineTaskRunner{pipeline: pipeline}
	first := staticDiscoveryDedupeResult()
	changed := staticDiscoveryDedupeResult()
	changed.Interfaces[0].Speed = 10_000_000_000

	runner.persistStaticDiscovery(domain.Device{ID: deviceID}, first)
	now = now.Add(61 * time.Minute)
	runner.persistStaticDiscovery(domain.Device{ID: deviceID}, changed)

	topologyService.mu.Lock()
	defer topologyService.mu.Unlock()
	if topologyService.calls != 2 {
		t.Fatalf("ApplyStaticDiscovery calls = %d, want 2 because changed fingerprints still persist after old self-heal deadline", topologyService.calls)
	}
}

func TestPipelineTaskRunnerPersistStaticDiscoveryDoesNotCachePersistenceFailure(t *testing.T) {
	deviceID := uuid.New()
	topologyService := &fakeTopologyService{err: errors.New("database unavailable")}
	pipeline := &PipelineOrchestrator{topologyService: topologyService}
	runner := &pipelineTaskRunner{pipeline: pipeline}
	result := staticDiscoveryDedupeResult()

	runner.persistStaticDiscovery(domain.Device{ID: deviceID}, result)
	runner.persistStaticDiscovery(domain.Device{ID: deviceID}, result)

	topologyService.mu.Lock()
	defer topologyService.mu.Unlock()
	if topologyService.calls != 2 {
		t.Fatalf("ApplyStaticDiscovery calls = %d, want 2 because failed persistence must retry", topologyService.calls)
	}
}

func TestPipelineTaskRunnerPersistStaticDiscoveryPersistsUnchangedBootstrapFollowup(t *testing.T) {
	deviceID := uuid.New()
	topologyService := &fakeTopologyService{}
	pipeline := &PipelineOrchestrator{topologyService: topologyService}
	runner := &pipelineTaskRunner{pipeline: pipeline}
	device := domain.Device{
		ID:                     deviceID,
		TopologyBootstrapState: domain.TopologyBootstrapStateFollowupScheduled,
	}
	result := staticDiscoveryDedupeResult()

	runner.persistStaticDiscovery(device, result)
	runner.persistStaticDiscovery(device, result)

	topologyService.mu.Lock()
	defer topologyService.mu.Unlock()
	if topologyService.calls != 2 {
		t.Fatalf("ApplyStaticDiscovery calls = %d, want 2 while bootstrap follow-up is pending", topologyService.calls)
	}
}

func TestPipelineTaskRunnerPersistStaticDiscoveryForcedPersistsUnchangedResult(t *testing.T) {
	deviceID := uuid.New()
	topologyService := &fakeTopologyService{}
	pipeline := &PipelineOrchestrator{topologyService: topologyService}
	runner := &pipelineTaskRunner{pipeline: pipeline}
	result := staticDiscoveryDedupeResult()

	runner.persistStaticDiscoveryForced(domain.Device{ID: deviceID}, result)
	runner.persistStaticDiscoveryForced(domain.Device{ID: deviceID}, result)

	topologyService.mu.Lock()
	defer topologyService.mu.Unlock()
	if topologyService.calls != 2 {
		t.Fatalf("ApplyStaticDiscovery calls = %d, want 2 for forced static persistence", topologyService.calls)
	}
	if len(topologyService.inputs) != 2 {
		t.Fatalf("captured inputs = %d, want 2", len(topologyService.inputs))
	}
	if topologyService.inputs[1].SkipTopologyMaterialization {
		t.Fatal("forced static persistence skipped topology materialization")
	}
}

func staticDiscoveryDedupeResult() collector.StaticResult {
	return collector.StaticResult{
		SysName:       "edge-sw",
		SysDescr:      "SwitchOS",
		SysObjectID:   ".1.3.6.1.4.1.14988.1",
		HardwareModel: "CCR",
		OSVersion:     "7.15",
		Vendor:        "mikrotik",
		DeviceType:    domain.DeviceTypeSwitch,
		Interfaces: []domain.Interface{
			{IfIndex: 1, IfName: "ether1", IfDescr: "uplink", Speed: 1_000_000_000, AdminStatus: "up", OperStatus: "up"},
		},
		Neighbors: []snmp.NeighborInfo{
			{LocalIfIndex: 1, LocalIfName: "ether1", RemoteSysName: "core", RemotePortID: "ether2", Protocol: domain.DiscoveryProtocolLLDP},
		},
		NeighborDiscoveryProtocols: []domain.DiscoveryProtocol{domain.DiscoveryProtocolLLDP},
	}
}

func TestPipelineTaskRunnerNetworkProbePortsResolvesInheritance(t *testing.T) {
	tests := []struct {
		name   string
		device domain.Device
		want   []int
	}{
		{
			name: "address override",
			device: domain.Device{
				IP:         "192.0.2.30",
				ProbePorts: []int{22, 443},
				Addresses: []domain.DeviceAddress{
					{
						Address:    "192.0.2.30",
						Role:       domain.DeviceAddressRolePrimary,
						IsPrimary:  true,
						ProbePorts: []int{2222},
					},
				},
			},
			want: []int{2222},
		},
		{
			name: "device override",
			device: domain.Device{
				IP:         "198.51.100.30",
				ProbePorts: []int{22, 443},
				Addresses: []domain.DeviceAddress{
					{
						Address:   "198.51.100.30",
						Role:      domain.DeviceAddressRolePrimary,
						IsPrimary: true,
					},
				},
			},
			want: []int{22, 443},
		},
		{
			name: "primary fallback target address override",
			device: domain.Device{
				IP:         "192.0.2.31",
				ProbePorts: []int{22, 443},
				Addresses: []domain.DeviceAddress{
					{
						Address:    "192.0.2.31",
						Role:       domain.DeviceAddressRoleManagement,
						ProbePorts: []int{9443},
					},
				},
			},
			want: []int{9443},
		},
		{
			name: "global setting",
			device: domain.Device{
				IP: "203.0.113.30",
				Addresses: []domain.DeviceAddress{
					{
						Address:   "203.0.113.30",
						Role:      domain.DeviceAddressRolePrimary,
						IsPrimary: true,
					},
				},
			},
			want: []int{8291},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			settingsRepo := newMockWorkerSettingsRepo()
			if err := settingsRepo.Set(domain.SettingNetworkProbePorts, "8291"); err != nil {
				t.Fatalf("Set() error = %v", err)
			}
			runner := &pipelineTaskRunner{pipeline: &PipelineOrchestrator{settingsRepo: settingsRepo}}

			got := runner.networkProbePorts(tt.device)

			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("networkProbePorts() = %v, want %v", got, tt.want)
			}
		})
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
	walkCalls     []string
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
	c.walkCalls = append(c.walkCalls, rootOid)
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

const (
	testIfHCInOctetsWalkOperation  = "if_hc_in_octets_walk"
	testIfHCOutOctetsWalkOperation = "if_hc_out_octets_walk"
)

func pipelineCounterCooldownTestDevice(deviceID uuid.UUID) domain.Device {
	return domain.Device{
		ID:            deviceID,
		Hostname:      "edge-cooldown",
		IP:            "192.0.2.85",
		Managed:       true,
		MetricsSource: domain.MetricsSourceSNMP,
		Vendor:        "default",
		SNMPCredentials: domain.SNMPCredentials{
			Version: domain.SNMPVersionV2c,
			V2c:     &domain.SNMPv2cCredentials{Community: "public"},
		},
		Interfaces: []domain.Interface{
			{IfIndex: 1, IfName: "ether1", Speed: 1_000_000_000},
		},
	}
}

func armPipelineCounterCooldown(runtime *pipelineRuntimeState, deviceID uuid.UUID, now time.Time, expectedInterval time.Duration) {
	for _, operation := range []string{testIfHCInOctetsWalkOperation, testIfHCOutOctetsWalkOperation} {
		runtime.RecordCounterWalkResult(deviceID, operation, "timeout", now, expectedInterval)
		runtime.RecordCounterWalkResult(deviceID, operation, "timeout", now.Add(time.Second), expectedInterval)
	}
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
			snmp.OidHrProcessorLoad: {
				{Name: snmp.OidHrProcessorLoad + ".1", Value: 40},
				{Name: snmp.OidHrProcessorLoad + ".2", Value: 50},
			},
			snmp.OidHrStorageType: {
				{Name: snmp.OidHrStorageType + ".1", Value: snmp.OidHrStorageRam},
			},
			snmp.OidHrStorageAllocUnits: {
				{Name: snmp.OidHrStorageAllocUnits + ".1", Value: int64(1)},
			},
			snmp.OidHrStorageSize: {
				{Name: snmp.OidHrStorageSize + ".1", Value: int64(200)},
			},
			snmp.OidHrStorageUsed: {
				{Name: snmp.OidHrStorageUsed + ".1", Value: int64(100)},
			},
			snmp.OidEntPhySensorType: {
				{Name: snmp.OidEntPhySensorType + ".1", Value: int64(8)},
			},
			snmp.OidEntPhySensorValue: {
				{Name: snmp.OidEntPhySensorValue + ".1", Value: int64(47)},
			},
			snmp.OidIfDescr: {
				{Name: snmp.OidIfDescr + ".1", Value: "uplink"},
			},
			snmp.OidIfSpeed: {
				{Name: snmp.OidIfSpeed + ".1", Value: uint32(1_000_000_000)},
			},
			snmp.OidIfAdminStatus: {
				{Name: snmp.OidIfAdminStatus + ".1", Value: 1},
			},
			snmp.OidIfOperStatus: {
				{Name: snmp.OidIfOperStatus + ".1", Value: 1},
			},
			snmp.OidIfName: {
				{Name: snmp.OidIfName + ".1", Value: "ether1"},
			},
			snmp.OidIfHighSpeed: {
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

func newStaticCachedContextTestCollector(t *testing.T) *collector.StaticCollector {
	t.Helper()
	client := &fakeSNMPClient{
		getResponses: map[string][]gosnmp.SnmpPDU{
			snmp.OidSysName:     {{Name: snmp.OidSysName, Value: "edge-cached-context"}},
			snmp.OidSysDescr:    {{Name: snmp.OidSysDescr, Value: "SwitchOS edge"}},
			snmp.OidSysObjectID: {{Name: snmp.OidSysObjectID, Value: ".1.3.6.1.4.1.14988.1"}},
		},
		walkResponses: map[string][]gosnmp.SnmpPDU{
			snmp.OidHrProcessorLoad: {
				{Name: snmp.OidHrProcessorLoad + ".1", Value: 40},
			},
			snmp.OidIfOperStatus: {
				{Name: snmp.OidIfOperStatus + ".7", Value: 1},
			},
			snmp.OidLLDPLocPortIfIndex: {
				{Name: snmp.OidLLDPLocPortIfIndex + ".1", Value: int(7)},
			},
			snmp.OidLLDPRemChassisId: {
				{Name: snmp.OidLLDPRemChassisId + ".1000.1.1", Value: "aa:bb:cc:dd:ee:ff"},
			},
			snmp.OidLLDPRemPortId: {
				{Name: snmp.OidLLDPRemPortId + ".1000.1.1", Value: "ether48"},
			},
			snmp.OidLLDPRemSysName: {
				{Name: snmp.OidLLDPRemSysName + ".1000.1.1", Value: "remote-switch"},
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

func TestPipelineRunsPerformanceTaskWithPerformanceCounterTimeoutProfile(t *testing.T) {
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
	if gotTimeout != 2*time.Second {
		t.Fatalf("performance timeout = %v, want capped 2s performance counter profile", gotTimeout)
	}
	if gotRetries != 0 {
		t.Fatalf("performance retries = %d, want 0 performance counter retries", gotRetries)
	}
}

func TestPipelineRunsOperationalAndStaticTasksWithBackgroundTimeoutProfile(t *testing.T) {
	stateStore := state.NewStore()
	settingsRepo := newMockWorkerSettingsRepo()
	_ = settingsRepo.Set(domain.SettingSNMPTimeout, "10")
	_ = settingsRepo.Set(domain.SettingSNMPRetries, "2")

	gotProfiles := map[domain.VolatilityClass]polling.TimeoutProfile{}
	operational := collector.NewOperationalCollector(buildEmptyVendorRegistry(), func(_ string, _ domain.SNMPCredentials, timeout time.Duration, retries int) (collector.SNMPClient, error) {
		gotProfiles[domain.VolatilityClassOperational] = polling.TimeoutProfile{Timeout: timeout, Retries: retries}
		return &fakeSNMPClient{}, nil
	})
	staticCollector := collector.NewStaticCollector(buildEmptyVendorRegistry(), func(_ string, _ domain.SNMPCredentials, timeout time.Duration, retries int) (collector.SNMPClient, error) {
		gotProfiles[domain.VolatilityClassStatic] = polling.TimeoutProfile{Timeout: timeout, Retries: retries}
		return &fakeSNMPClient{}, nil
	})
	pipeline := NewPipelineOrchestrator(nil, stateStore, nil, nil, nil, nil, operational, staticCollector, nil, nil, settingsRepo, nil, nil, nil)

	for _, volatilityClass := range []domain.VolatilityClass{domain.VolatilityClassOperational, domain.VolatilityClassStatic} {
		device := domain.Device{
			ID:            uuid.New(),
			Hostname:      "edge-" + string(volatilityClass),
			IP:            "10.0.0.3",
			Managed:       true,
			PollClass:     domain.PollClassCore,
			MetricsSource: domain.MetricsSourceSNMP,
			Vendor:        "default",
		}
		pipeline.runTask(context.Background(), scheduler.PollTask{
			Key:              scheduler.NewTaskKey(device.ID, volatilityClass),
			Kind:             polling.TaskKindBackground,
			Lane:             polling.LaneBackground,
			VolatilityClass:  volatilityClass,
			Device:           device,
			ExpectedInterval: 30 * time.Second,
		})
	}

	for _, volatilityClass := range []domain.VolatilityClass{domain.VolatilityClassOperational, domain.VolatilityClassStatic} {
		profile, ok := gotProfiles[volatilityClass]
		if !ok {
			t.Fatalf("%s task did not create SNMP client", volatilityClass)
		}
		if profile.Timeout != 10*time.Second {
			t.Fatalf("%s timeout = %v, want configured 10s background profile", volatilityClass, profile.Timeout)
		}
		if profile.Retries != 2 {
			t.Fatalf("%s retries = %d, want configured 2 background retries", volatilityClass, profile.Retries)
		}
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
				{IfIndex: 1, IfName: "ether1", IfDescr: "uplink", Speed: 1_000_000_000},
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
	if deviceState.Metrics.CPUPercent != nil || deviceState.Metrics.MemPercent != nil || deviceState.Metrics.TempCelsius != nil {
		t.Fatalf("expected performance task to skip device-health metrics, got %#v", deviceState.Metrics)
	}
	if deviceState.Metrics.UptimeSecs != nil {
		t.Fatalf("expected performance task to skip uptime metric, got %#v", deviceState.Metrics.UptimeSecs)
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

func TestPipelineOrchestratorPerformanceTaskSkipsSNMPWhenEssentialMarkedSNMPUnreachable(t *testing.T) {
	deviceID := uuid.New()
	essentialAt := time.Date(2026, 4, 28, 12, 0, 0, 0, time.UTC)
	store := state.NewStore()
	store.Update(state.StateUpdate{
		DeviceID:         deviceID,
		ExpectedInterval: 10 * time.Second,
		Timestamp:        essentialAt,
		Essential: &state.EssentialUpdate{
			PollStatus:       polling.PollStatusFailed,
			NetworkReachable: polling.TriStateTrue,
			SNMPReachable:    polling.TriStateFalse,
			Uptime:           polling.FieldStateError,
			CPU:              polling.FieldStateError,
			Memory:           polling.FieldStateError,
		},
	})

	var snmpCalls int
	performance := collector.NewPerformanceCollector(buildEmptyVendorRegistry(), func(string, domain.SNMPCredentials, time.Duration, int) (collector.SNMPClient, error) {
		snmpCalls++
		return nil, errors.New("background performance SNMP should be skipped")
	})
	sched := newPipelineTestScheduler()
	pipeline := NewPipelineOrchestrator(
		sched,
		store,
		nil,
		ws.NewHub(ws.WithBroadcastRecorder()),
		nil,
		performance,
		newOperationalTestCollector(t),
		newStaticTestCollector(t),
		nil,
		nil,
		newMockWorkerSettingsRepo(),
		make(chan struct{}, 1),
		nil,
		nil,
	)

	pipeline.taskRunner.runTask(context.Background(), scheduler.PollTask{
		RunID:            83,
		Key:              scheduler.NewTaskKey(deviceID, domain.VolatilityClassPerformance),
		VolatilityClass:  domain.VolatilityClassPerformance,
		ExpectedInterval: 30 * time.Second,
		Device: domain.Device{
			ID:            deviceID,
			IP:            "192.0.2.83",
			MetricsSource: domain.MetricsSourceSNMP,
			Vendor:        "default",
			SNMPCredentials: domain.SNMPCredentials{
				Version: domain.SNMPVersionV2c,
				V2c:     &domain.SNMPv2cCredentials{Community: "public"},
			},
		},
	})

	if snmpCalls != 0 {
		t.Fatalf("performance SNMP calls = %d, want 0 while essential SNMP state is unreachable", snmpCalls)
	}
	deviceState, ok := store.GetDevice(deviceID)
	if !ok {
		t.Fatal("expected existing essential state")
	}
	if !deviceState.LastPolledAt.Equal(essentialAt) {
		t.Fatalf("LastPolledAt = %s, want unchanged essential timestamp %s", deviceState.LastPolledAt, essentialAt)
	}
	sched.mu.Lock()
	defer sched.mu.Unlock()
	if len(sched.completions) != 1 {
		t.Fatalf("expected one scheduler completion for skipped task, got %d", len(sched.completions))
	}
}

func TestPipelineCounterCooldownResetDeviceRuntimeClearsState(t *testing.T) {
	deviceID := uuid.New()
	runtime := newPipelineRuntimeState(ws.PrometheusStatusPayload{})
	now := time.Date(2026, 6, 19, 12, 0, 0, 0, time.UTC)

	armPipelineCounterCooldown(runtime, deviceID, now, 30*time.Second)
	if !runtime.ShouldSkipCounterWalk(deviceID, testIfHCInOctetsWalkOperation, now.Add(time.Second)) {
		t.Fatal("expected armed counter cooldown to skip in-octets walk")
	}

	runtime.resetDeviceRuntime(deviceID)
	if runtime.ShouldSkipCounterWalk(deviceID, testIfHCInOctetsWalkOperation, now.Add(time.Second)) {
		t.Fatal("counter cooldown state was not cleared by resetDeviceRuntime")
	}
}

func TestPipelineCounterCooldownSkipKeepsPerformanceFreshnessAndSNMPReachability(t *testing.T) {
	device := pipelineCounterCooldownTestDevice(uuid.New())
	store := state.NewStore()
	var snmpCalls int
	fakeClient := &fakeSNMPClient{
		walkResponses: map[string][]gosnmp.SnmpPDU{
			snmp.OidIfHCInOctets: {
				{Name: snmp.OidIfHCInOctets + ".1", Value: uint64(10_000)},
			},
			snmp.OidIfHCOutOctets: {
				{Name: snmp.OidIfHCOutOctets + ".1", Value: uint64(20_000)},
			},
		},
	}
	performance := collector.NewPerformanceCollector(buildEmptyVendorRegistry(), func(string, domain.SNMPCredentials, time.Duration, int) (collector.SNMPClient, error) {
		snmpCalls++
		return fakeClient, nil
	})
	pipeline := NewPipelineOrchestrator(
		newPipelineTestScheduler(),
		store,
		newPipelineTestCache([]domain.Device{device}, nil),
		ws.NewHub(ws.WithBroadcastRecorder()),
		nil,
		performance,
		newOperationalTestCollector(t),
		newStaticTestCollector(t),
		nil,
		nil,
		newMockWorkerSettingsRepo(),
		make(chan struct{}, 1),
		nil,
		nil,
	)
	armPipelineCounterCooldown(pipeline.runtime, device.ID, time.Now().UTC(), 30*time.Second)

	pipeline.taskRunner.runTask(context.Background(), scheduler.PollTask{
		RunID:            85,
		Key:              scheduler.NewTaskKey(device.ID, domain.VolatilityClassPerformance),
		VolatilityClass:  domain.VolatilityClassPerformance,
		ExpectedInterval: 30 * time.Second,
		Device:           device,
	})

	if snmpCalls != 1 {
		t.Fatalf("performance SNMP factory calls = %d, want 1 for cooldown-aware poll", snmpCalls)
	}
	if len(fakeClient.walkCalls) != 0 {
		t.Fatalf("BulkWalk calls = %v, want no HC counter walks during cooldown", fakeClient.walkCalls)
	}
	deviceState, ok := store.GetDevice(device.ID)
	if !ok {
		t.Fatal("expected performance cooldown skip to update device freshness")
	}
	if deviceState.LastPolledAt.IsZero() {
		t.Fatal("LastPolledAt was not updated by cooldown skip")
	}
	if deviceState.Stale {
		t.Fatal("device remains stale after successful cooldown skip")
	}
	if deviceState.SNMPReachable == polling.TriStateFalse {
		t.Fatalf("SNMPReachable = %q, want not false after cooldown skip", deviceState.SNMPReachable)
	}
	if len(deviceState.LinkMetrics) != 0 {
		t.Fatalf("link metrics = %d, want none during cooldown skip", len(deviceState.LinkMetrics))
	}
}

func TestPipelinePrevCountersCounterCooldownSkipDoesNotInstallEmptyBaseline(t *testing.T) {
	device := pipelineCounterCooldownTestDevice(uuid.New())
	performance := collector.NewPerformanceCollector(buildEmptyVendorRegistry(), func(string, domain.SNMPCredentials, time.Duration, int) (collector.SNMPClient, error) {
		return &fakeSNMPClient{}, nil
	})
	pipeline := NewPipelineOrchestrator(
		newPipelineTestScheduler(),
		state.NewStore(),
		newPipelineTestCache([]domain.Device{device}, nil),
		ws.NewHub(ws.WithBroadcastRecorder()),
		nil,
		performance,
		newOperationalTestCollector(t),
		newStaticTestCollector(t),
		nil,
		nil,
		newMockWorkerSettingsRepo(),
		make(chan struct{}, 1),
		nil,
		nil,
	)
	armPipelineCounterCooldown(pipeline.runtime, device.ID, time.Now().UTC(), 30*time.Second)

	pipeline.taskRunner.runTask(context.Background(), scheduler.PollTask{
		RunID:            86,
		Key:              scheduler.NewTaskKey(device.ID, domain.VolatilityClassPerformance),
		VolatilityClass:  domain.VolatilityClassPerformance,
		ExpectedInterval: 30 * time.Second,
		Device:           device,
	})

	pipeline.runtime.mu.RLock()
	_, ok := pipeline.runtime.prevCounters[device.ID]
	pipeline.runtime.mu.RUnlock()
	if ok {
		t.Fatal("cooldown skip installed an empty prevCounters baseline")
	}
}

func TestPipelineCounterCooldownKnownSNMPUnreachableSkipRemainsStronger(t *testing.T) {
	device := pipelineCounterCooldownTestDevice(uuid.New())
	essentialAt := time.Date(2026, 6, 19, 12, 30, 0, 0, time.UTC)
	store := state.NewStore()
	store.Update(state.StateUpdate{
		DeviceID:         device.ID,
		ExpectedInterval: 10 * time.Second,
		Timestamp:        essentialAt,
		Essential: &state.EssentialUpdate{
			PollStatus:       polling.PollStatusFailed,
			NetworkReachable: polling.TriStateTrue,
			SNMPReachable:    polling.TriStateFalse,
			Uptime:           polling.FieldStateError,
			CPU:              polling.FieldStateError,
			Memory:           polling.FieldStateError,
		},
	})

	var snmpCalls int
	performance := collector.NewPerformanceCollector(buildEmptyVendorRegistry(), func(string, domain.SNMPCredentials, time.Duration, int) (collector.SNMPClient, error) {
		snmpCalls++
		return nil, errors.New("background performance SNMP should be skipped before cooldown policy")
	})
	pipeline := NewPipelineOrchestrator(
		newPipelineTestScheduler(),
		store,
		newPipelineTestCache([]domain.Device{device}, nil),
		ws.NewHub(ws.WithBroadcastRecorder()),
		nil,
		performance,
		newOperationalTestCollector(t),
		newStaticTestCollector(t),
		nil,
		nil,
		newMockWorkerSettingsRepo(),
		make(chan struct{}, 1),
		nil,
		nil,
	)
	armPipelineCounterCooldown(pipeline.runtime, device.ID, time.Now().UTC(), 30*time.Second)

	pipeline.taskRunner.runTask(context.Background(), scheduler.PollTask{
		RunID:            87,
		Key:              scheduler.NewTaskKey(device.ID, domain.VolatilityClassPerformance),
		VolatilityClass:  domain.VolatilityClassPerformance,
		ExpectedInterval: 30 * time.Second,
		Device:           device,
	})

	if snmpCalls != 0 {
		t.Fatalf("performance SNMP calls = %d, want 0 while known SNMP unreachable", snmpCalls)
	}
	deviceState, ok := store.GetDevice(device.ID)
	if !ok {
		t.Fatal("expected existing essential state")
	}
	if !deviceState.LastPolledAt.Equal(essentialAt) {
		t.Fatalf("LastPolledAt = %s, want unchanged essential timestamp %s", deviceState.LastPolledAt, essentialAt)
	}
}

func TestPipelineOrchestratorOperationalTaskSkipsSNMPWhenEssentialMarkedSNMPUnreachable(t *testing.T) {
	deviceID := uuid.New()
	essentialAt := time.Date(2026, 4, 28, 12, 5, 0, 0, time.UTC)
	store := state.NewStore()
	store.Update(state.StateUpdate{
		DeviceID:         deviceID,
		ExpectedInterval: 10 * time.Second,
		Timestamp:        essentialAt,
		Essential: &state.EssentialUpdate{
			PollStatus:       polling.PollStatusFailed,
			NetworkReachable: polling.TriStateTrue,
			SNMPReachable:    polling.TriStateFalse,
			Uptime:           polling.FieldStateError,
			CPU:              polling.FieldStateError,
			Memory:           polling.FieldStateError,
		},
	})

	var snmpCalls int
	operational := collector.NewOperationalCollector(buildEmptyVendorRegistry(), func(string, domain.SNMPCredentials, time.Duration, int) (collector.SNMPClient, error) {
		snmpCalls++
		return nil, errors.New("background operational SNMP should be skipped")
	})
	sched := newPipelineTestScheduler()
	pipeline := NewPipelineOrchestrator(
		sched,
		store,
		nil,
		ws.NewHub(ws.WithBroadcastRecorder()),
		nil,
		newPerformanceTestCollector(t),
		operational,
		newStaticTestCollector(t),
		nil,
		nil,
		newMockWorkerSettingsRepo(),
		make(chan struct{}, 1),
		nil,
		nil,
	)

	pipeline.taskRunner.runTask(context.Background(), scheduler.PollTask{
		RunID:            84,
		Key:              scheduler.NewTaskKey(deviceID, domain.VolatilityClassOperational),
		VolatilityClass:  domain.VolatilityClassOperational,
		ExpectedInterval: domain.OperationalClassInterval,
		Device: domain.Device{
			ID:            deviceID,
			IP:            "192.0.2.84",
			MetricsSource: domain.MetricsSourceSNMP,
			Vendor:        "default",
			SNMPCredentials: domain.SNMPCredentials{
				Version: domain.SNMPVersionV2c,
				V2c:     &domain.SNMPv2cCredentials{Community: "public"},
			},
		},
	})

	if snmpCalls != 0 {
		t.Fatalf("operational SNMP calls = %d, want 0 while essential SNMP state is unreachable", snmpCalls)
	}
	deviceState, ok := store.GetDevice(deviceID)
	if !ok {
		t.Fatal("expected existing essential state")
	}
	if !deviceState.LastPolledAt.Equal(essentialAt) {
		t.Fatalf("LastPolledAt = %s, want unchanged essential timestamp %s", deviceState.LastPolledAt, essentialAt)
	}
	sched.mu.Lock()
	defer sched.mu.Unlock()
	if len(sched.completions) != 1 {
		t.Fatalf("expected one scheduler completion for skipped task, got %d", len(sched.completions))
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
	assertPipelineFloatPtrEqual(t, deviceState.Metrics.CPUPercent, 45, "CPUPercent")
	assertPipelineFloatPtrEqual(t, deviceState.Metrics.MemPercent, 50, "MemPercent")
	assertPipelineFloatPtrEqual(t, deviceState.Metrics.TempCelsius, 47, "TempCelsius")

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

func TestPipelineStaticTaskUsesCachedInterfacesForDiscoveryContext(t *testing.T) {
	deviceID := uuid.New()
	taskDevice := domain.Device{
		ID:                    deviceID,
		IP:                    "192.0.2.21",
		Status:                domain.DeviceStatusProbing,
		Vendor:                "default",
		TopologyDiscoveryMode: domain.TopologyDiscoveryModeLLDP,
	}
	cachedDevice := taskDevice
	cachedDevice.Interfaces = []domain.Interface{
		{IfIndex: 7, IfName: "cached-uplink", Speed: 1_000_000_000},
	}
	topologyService := &fakeTopologyService{}
	pipeline := NewPipelineOrchestrator(
		newPipelineTestScheduler(),
		state.NewStore(),
		newPipelineTestCache([]domain.Device{cachedDevice}, nil),
		ws.NewHub(ws.WithBroadcastRecorder()),
		nil,
		newPerformanceTestCollector(t),
		newOperationalTestCollector(t),
		newStaticCachedContextTestCollector(t),
		nil,
		topologyService,
		newMockWorkerSettingsRepo(),
		make(chan struct{}, 1),
		nil,
		nil,
	)

	pipeline.taskRunner.runTask(context.Background(), scheduler.PollTask{
		RunID:            78,
		Key:              scheduler.NewTaskKey(deviceID, domain.VolatilityClassStatic),
		VolatilityClass:  domain.VolatilityClassStatic,
		ExpectedInterval: 5 * time.Minute,
		Device:           taskDevice,
	})

	topologyService.mu.Lock()
	defer topologyService.mu.Unlock()
	if topologyService.calls != 1 {
		t.Fatalf("ApplyStaticDiscovery calls = %d, want 1", topologyService.calls)
	}
	if len(topologyService.lastIn.Neighbors) != 1 {
		t.Fatalf("neighbors = %#v, want one cached-context neighbor", topologyService.lastIn.Neighbors)
	}
	if topologyService.lastIn.Neighbors[0].LocalIfName != "cached-uplink" {
		t.Fatalf("neighbor LocalIfName = %q, want cached-uplink", topologyService.lastIn.Neighbors[0].LocalIfName)
	}
	if len(topologyService.lastIn.Interfaces) != 1 {
		t.Fatalf("interfaces = %#v, want only one live-observed interface", topologyService.lastIn.Interfaces)
	}
	if topologyService.lastIn.Interfaces[0].IfName == "cached-uplink" {
		t.Fatalf("cached interface name was persisted as fresh interface data: %#v", topologyService.lastIn.Interfaces[0])
	}
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
				snmp.OidIfDescr: {
					{Name: snmp.OidIfDescr + ".1", Value: "uplink"},
				},
				snmp.OidIfSpeed: {
					{Name: snmp.OidIfSpeed + ".1", Value: uint32(1_000_000_000)},
				},
				snmp.OidIfAdminStatus: {
					{Name: snmp.OidIfAdminStatus + ".1", Value: 1},
				},
				snmp.OidIfOperStatus: {
					{Name: snmp.OidIfOperStatus + ".1", Value: 1},
				},
				snmp.OidIfName: {
					{Name: snmp.OidIfName + ".1", Value: "ether1"},
				},
				snmp.OidIfHighSpeed: {
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

func TestPipelineOrchestratorBootstrapTopologyDiscoveryMode_UsesBootstrapState(t *testing.T) {
	settingsRepo := newMockWorkerSettingsRepo()
	if err := settingsRepo.Set(domain.SettingTopologyDiscoveryDefaultMode, string(domain.TopologyDiscoveryModeLLDPCDP)); err != nil {
		t.Fatalf("Set setting failed: %v", err)
	}
	pipeline := NewPipelineOrchestrator(nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, settingsRepo, nil, nil, nil)

	tests := []struct {
		name   string
		device domain.Device
		want   domain.TopologyDiscoveryMode
	}{
		{
			name: "pending inherit forces bootstrap once",
			device: domain.Device{
				TopologyDiscoveryMode:  domain.TopologyDiscoveryModeInherit,
				TopologyBootstrapState: domain.TopologyBootstrapStatePending,
			},
			want: domain.TopologyDiscoveryModeBootstrapOnce,
		},
		{
			name: "followup scheduled overrides continuous mode",
			device: domain.Device{
				TopologyDiscoveryMode:  domain.TopologyDiscoveryModeLLDPCDP,
				TopologyBootstrapState: domain.TopologyBootstrapStateFollowupScheduled,
			},
			want: domain.TopologyDiscoveryModeBootstrapOnce,
		},
		{
			name: "idle inherit uses default continuous mode",
			device: domain.Device{
				TopologyDiscoveryMode:  domain.TopologyDiscoveryModeInherit,
				TopologyBootstrapState: domain.TopologyBootstrapStateIdle,
			},
			want: domain.TopologyDiscoveryModeLLDPCDP,
		},
		{
			name: "completed bootstrap once becomes off",
			device: domain.Device{
				TopologyDiscoveryMode:  domain.TopologyDiscoveryModeBootstrapOnce,
				TopologyBootstrapState: domain.TopologyBootstrapStateCompleted,
			},
			want: domain.TopologyDiscoveryModeOff,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			runner, ok := pipeline.taskRunner.(*pipelineTaskRunner)
			if !ok {
				t.Fatal("expected pipeline task runner to use pipelineTaskRunner")
			}
			if got := runner.bootstrapTopologyDiscoveryMode(tt.device); got != tt.want {
				t.Fatalf("bootstrapTopologyDiscoveryMode() = %s, want %s", got, tt.want)
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

func TestPipelineOrchestratorStopClearsRuntimeRecoveryTracking(t *testing.T) {
	for _, started := range []bool{false, true} {
		name := "never started"
		if started {
			name = "started"
		}
		t.Run(name, func(t *testing.T) {
			registry := observability.ResetDefaultForTest()
			t.Cleanup(func() {
				observability.ResetDefaultForTest()
			})
			pipeline := NewPipelineOrchestrator(
				newPipelineTestScheduler(),
				nil,
				nil,
				nil,
				nil,
				nil,
				nil,
				nil,
				nil,
				nil,
				nil,
				nil,
				nil,
				nil,
			)
			if started {
				if err := pipeline.Start(context.Background()); err != nil {
					t.Fatalf("Start() error = %v", err)
				}
			}

			pipeline.runtimeRecoveryTTL = 50 * time.Millisecond
			client := &ws.Client{}
			pipeline.overviewBuildMu.Lock()
			pipeline.recordRuntimeRecoveryScheduledLocked(
				client,
				ws.OverviewSyncBatch{
					Mode:            ws.OverviewSyncModeCurrent,
					RuntimeStreamID: "runtime-stream-1",
					TargetVersion:   42,
				},
				ws.ResyncReasonClientResync,
				pipeline.clockNow(),
			)
			pipeline.armRuntimeRecoveryTimerLocked(pipeline.clockNow())
			generationBeforeStop := pipeline.runtimeRecoveryTimerGen
			pipeline.overviewBuildMu.Unlock()

			pipeline.Stop()
			pipeline.Stop()

			pipeline.overviewBuildMu.Lock()
			remaining := len(pipeline.runtimeRecoveryAttempts)
			timer := pipeline.runtimeRecoveryTimer
			generationAfterStop := pipeline.runtimeRecoveryTimerGen
			pipeline.overviewBuildMu.Unlock()
			if remaining != 0 || timer != nil {
				t.Fatalf("Stop() left attempts=%d timer=%v", remaining, timer)
			}
			if generationAfterStop <= generationBeforeStop {
				t.Fatalf(
					"Stop() timer generation = %d, want newer than %d",
					generationAfterStop,
					generationBeforeStop,
				)
			}

			time.Sleep(2 * pipeline.runtimeRecoveryTTL)
			body := string(registry.MarshalPrometheus())
			if strings.Contains(body, `mode="current",outcome="failed",reason="client_resync_scheduled"`) {
				t.Fatalf("Stop() emitted a delayed runtime recovery failure\n%s", body)
			}
		})
	}
}

func TestPipelineOrchestratorContextCancellationClearsRuntimeRecoveryTracking(t *testing.T) {
	registry := observability.ResetDefaultForTest()
	t.Cleanup(func() {
		observability.ResetDefaultForTest()
	})
	pipeline := NewPipelineOrchestrator(
		newPipelineTestScheduler(), nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil,
	)
	ctx, cancel := context.WithCancel(context.Background())
	if err := pipeline.Start(ctx); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	pipeline.lifecycleMu.Lock()
	runDone := pipeline.done
	pipeline.lifecycleMu.Unlock()

	pipeline.runtimeRecoveryTTL = 50 * time.Millisecond
	pipeline.overviewBuildMu.Lock()
	pipeline.recordRuntimeRecoveryScheduledLocked(
		&ws.Client{},
		ws.OverviewSyncBatch{
			Mode:            ws.OverviewSyncModeCurrent,
			RuntimeStreamID: "runtime-stream-1",
			TargetVersion:   42,
		},
		ws.ResyncReasonClientResync,
		pipeline.clockNow(),
	)
	pipeline.armRuntimeRecoveryTimerLocked(pipeline.clockNow())
	generationBeforeCancel := pipeline.runtimeRecoveryTimerGen
	pipeline.overviewBuildMu.Unlock()

	cancel()
	select {
	case <-runDone:
	case <-time.After(2 * time.Second):
		pipeline.clearRuntimeRecoveryTracking()
		t.Fatal("pipeline run did not stop after context cancellation")
	}
	pipeline.overviewBuildMu.Lock()
	remaining := len(pipeline.runtimeRecoveryAttempts)
	timer := pipeline.runtimeRecoveryTimer
	generationAfterCancel := pipeline.runtimeRecoveryTimerGen
	pipeline.overviewBuildMu.Unlock()
	if remaining != 0 || timer != nil {
		pipeline.clearRuntimeRecoveryTracking()
		t.Fatalf("context cancellation left attempts=%d timer=%v", remaining, timer)
	}
	if generationAfterCancel <= generationBeforeCancel {
		t.Fatalf(
			"context cancellation timer generation = %d, want newer than %d",
			generationAfterCancel,
			generationBeforeCancel,
		)
	}

	time.Sleep(2 * pipeline.runtimeRecoveryTTL)
	body := string(registry.MarshalPrometheus())
	if strings.Contains(body, `mode="current",outcome="failed",reason="client_resync_scheduled"`) {
		t.Fatalf("context cancellation emitted a delayed runtime recovery failure\n%s", body)
	}
	if err := pipeline.Start(context.Background()); err != nil {
		t.Fatalf("Start() after context cancellation error = %v", err)
	}
	pipeline.Stop()
}

func TestPipelineOrchestratorGetOrBuildAfterShutdownDoesNotRestartRuntimeRecovery(t *testing.T) {
	for _, shutdown := range []string{"stop", "context_cancel"} {
		t.Run(shutdown, func(t *testing.T) {
			registry := observability.ResetDefaultForTest()
			t.Cleanup(func() {
				observability.ResetDefaultForTest()
			})
			pipeline, _, _, _, _ := newBroadcastTestPipeline(t)
			ctx, cancel := context.WithCancel(context.Background())
			t.Cleanup(cancel)
			if err := pipeline.Start(ctx); err != nil {
				t.Fatalf("Start() error = %v", err)
			}

			switch shutdown {
			case "stop":
				pipeline.Stop()
			case "context_cancel":
				pipeline.lifecycleMu.Lock()
				runDone := pipeline.done
				pipeline.lifecycleMu.Unlock()
				cancel()
				select {
				case <-runDone:
				case <-time.After(2 * time.Second):
					pipeline.Stop()
					t.Fatal("pipeline run did not stop after context cancellation")
				}
				t.Cleanup(pipeline.Stop)
			default:
				t.Fatalf("unsupported shutdown mode %q", shutdown)
			}

			pipeline.runtimeRecoveryTTL = 50 * time.Millisecond
			pipeline.overviewBuildMu.Lock()
			pipeline.runtime.mu.Lock()
			pipeline.runtime.prevHashes = nil
			pipeline.runtime.mu.Unlock()
			pipeline.overviewBuildMu.Unlock()

			state := pipeline.GetOrBuildOverviewState()
			if state.Snapshot == nil || state.StreamID == "" {
				t.Fatalf("GetOrBuildOverviewState() = %#v, want rebuilt state", state)
			}
			pipeline.overviewBuildMu.Lock()
			remaining := len(pipeline.runtimeRecoveryAttempts)
			timer := pipeline.runtimeRecoveryTimer
			pipeline.overviewBuildMu.Unlock()
			if remaining != 0 || timer != nil {
				pipeline.clearRuntimeRecoveryTracking()
				t.Fatalf("post-shutdown getter left attempts=%d timer=%v", remaining, timer)
			}

			time.Sleep(2 * pipeline.runtimeRecoveryTTL)
			body := string(registry.MarshalPrometheus())
			if strings.Contains(body, `outcome="failed",reason="client_resync_scheduled"`) {
				t.Fatalf("post-shutdown getter emitted a delayed runtime recovery failure\n%s", body)
			}
		})
	}
}

func TestPipelineOrchestratorStopPreventsRestartBeforeFinalRecoveryCleanup(t *testing.T) {
	pipeline := NewPipelineOrchestrator(
		newPipelineTestScheduler(),
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
	)
	if err := pipeline.Start(context.Background()); err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	pipeline.overviewBuildMu.Lock()
	stopReturned := make(chan struct{})
	go func() {
		pipeline.Stop()
		close(stopReturned)
	}()

	deadline := time.Now().Add(2 * time.Second)
	for !pipeline.stopping.Load() {
		if time.Now().After(deadline) {
			pipeline.overviewBuildMu.Unlock()
			t.Fatal("Stop() did not enter the stopping state")
		}
		time.Sleep(time.Millisecond)
	}
	restartErr := pipeline.Start(context.Background())
	pipeline.overviewBuildMu.Unlock()

	select {
	case <-stopReturned:
	case <-time.After(2 * time.Second):
		t.Fatal("Stop() did not return after recovery cleanup was released")
	}
	if restartErr == nil {
		pipeline.Stop()
	}
	if !errors.Is(restartErr, ErrAlreadyStarted) {
		t.Fatalf("overlapping Start() error = %v, want ErrAlreadyStarted", restartErr)
	}

	if err := pipeline.Start(context.Background()); err != nil {
		t.Fatalf("Start() after Stop() error = %v", err)
	}
	if pipeline.Status() != "running" {
		t.Fatalf("pipeline status after restart = %q, want running", pipeline.Status())
	}
	pipeline.Stop()
}

func TestPipelineOrchestratorStopRejectsRuntimeSyncAfterFinalRecoveryCleanup(t *testing.T) {
	hub := ws.NewHub()
	go hub.Run()
	pipeline := NewPipelineOrchestrator(
		newPipelineTestScheduler(), nil, nil, hub, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil,
	)
	configureRuntimeRecoveryState(t, pipeline, "runtime-stream-1", 42)
	_, client, release := attachRuntimeRecoveryClient(
		t,
		hub,
		pipeline.GetOverviewSnapshot,
		ws.RuntimeStreamProtocolVersion,
		ws.RuntimeCursor{StreamID: "runtime-stream-1", Version: 42, Known: true},
		false,
	)
	defer release()
	if err := pipeline.Start(context.Background()); err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	pipeline.overviewBuildMu.Lock()
	pipeline.recordRuntimeRecoveryScheduledLocked(
		client,
		ws.OverviewSyncBatch{
			Mode:            ws.OverviewSyncModeCurrent,
			RuntimeStreamID: "runtime-stream-1",
			TargetVersion:   42,
		},
		ws.ResyncReasonClientResync,
		pipeline.clockNow(),
	)
	pipeline.armRuntimeRecoveryTimerLocked(pipeline.clockNow())
	generationBeforeStop := pipeline.runtimeRecoveryTimerGen
	pipeline.overviewBuildMu.Unlock()

	pipeline.overviewBuildMu.Lock()
	stopReturned := make(chan struct{})
	go func() {
		pipeline.Stop()
		close(stopReturned)
	}()

	deadline := time.Now().Add(2 * time.Second)
	for !pipeline.stopping.Load() {
		if time.Now().After(deadline) {
			pipeline.overviewBuildMu.Unlock()
			t.Fatal("Stop() did not enter the stopping state")
		}
		time.Sleep(time.Millisecond)
	}
	pipeline.lifecycleMu.Lock()
	pipeline.overviewBuildMu.Unlock()

	deadline = time.Now().Add(2 * time.Second)
	for {
		pipeline.overviewBuildMu.Lock()
		generation := pipeline.runtimeRecoveryTimerGen
		pipeline.overviewBuildMu.Unlock()
		if generation >= generationBeforeStop+2 {
			break
		}
		if time.Now().After(deadline) {
			pipeline.lifecycleMu.Unlock()
			t.Fatal("pipeline shutdown did not complete its pre-final recovery cleanup passes")
		}
		time.Sleep(time.Millisecond)
	}

	syncReturned := make(chan struct{})
	go func() {
		pipeline.SyncOverviewClient(client, ws.RuntimeSyncRequest{Reason: ws.ResyncReasonClientResync})
		close(syncReturned)
	}()
	select {
	case <-syncReturned:
	case <-time.After(2 * time.Second):
		pipeline.lifecycleMu.Unlock()
		t.Fatal("runtime sync did not return while pipeline was stopping")
	}
	pipeline.overviewBuildMu.Lock()
	attemptsDuringStop := len(pipeline.runtimeRecoveryAttempts)
	timerDuringStop := pipeline.runtimeRecoveryTimer
	pipeline.overviewBuildMu.Unlock()
	pipeline.lifecycleMu.Unlock()

	select {
	case <-stopReturned:
	case <-time.After(2 * time.Second):
		t.Fatal("Stop() did not return after lifecycle transition was released")
	}
	pipeline.SyncOverviewClient(client, ws.RuntimeSyncRequest{Reason: ws.ResyncReasonClientResync})
	pipeline.overviewBuildMu.Lock()
	attemptsAfterStop := len(pipeline.runtimeRecoveryAttempts)
	timerAfterStop := pipeline.runtimeRecoveryTimer
	pipeline.overviewBuildMu.Unlock()
	if attemptsDuringStop != 0 || timerDuringStop != nil || attemptsAfterStop != 0 || timerAfterStop != nil {
		pipeline.clearRuntimeRecoveryTracking()
		t.Fatalf(
			"runtime sync recreated tracking during Stop(): during=(%d,%v) after=(%d,%v)",
			attemptsDuringStop,
			timerDuringStop,
			attemptsAfterStop,
			timerAfterStop,
		)
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

func TestPipelineOrchestratorRunTask_VirtualPrometheusUnreachableDrivesTCPReachability(t *testing.T) {
	deviceID := uuid.New()
	task := scheduler.PollTask{
		RunID:            92,
		Key:              scheduler.NewTaskKey(deviceID, domain.VolatilityClassOperational),
		VolatilityClass:  domain.VolatilityClassOperational,
		ExpectedInterval: domain.OperationalClassInterval,
		Device: domain.Device{
			ID:                   deviceID,
			DeviceType:           domain.DeviceTypeVirtual,
			IP:                   "192.0.2.91",
			Status:               domain.DeviceStatusUnknown,
			PrometheusLabelName:  "instance",
			PrometheusLabelValue: "192.0.2.91",
		},
	}

	sched := newPipelineTestScheduler()
	store := state.NewStore()
	promClient := &fakePrometheusClient{
		probeStatuses: map[string]bool{"192.0.2.91": false},
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
	if deviceState.Reachability != state.ReachabilitySoftDown {
		t.Fatalf("Reachability = %q, want %q", deviceState.Reachability, state.ReachabilitySoftDown)
	}
	if deviceState.ConsecutiveFailures != 1 {
		t.Fatalf("ConsecutiveFailures = %d, want 1", deviceState.ConsecutiveFailures)
	}
	if snmpCalls != 0 {
		t.Fatalf("expected virtual operational task to bypass SNMP collector, got %d SNMP call(s)", snmpCalls)
	}
}

func newBroadcastTestPipeline(t *testing.T) (*PipelineOrchestrator, *ws.Hub, *state.Store, chan struct{}, uuid.UUID) {
	return newBroadcastTestPipelineWithOverviewClient(t, true)
}

func newNoClientBroadcastTestPipeline(t *testing.T) (*PipelineOrchestrator, *ws.Hub, *state.Store, chan struct{}, uuid.UUID) {
	return newBroadcastTestPipelineWithOverviewClient(t, false)
}

func newBroadcastTestPipelineWithOverviewClient(t *testing.T, attachClient bool) (*PipelineOrchestrator, *ws.Hub, *state.Store, chan struct{}, uuid.UUID) {
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

	if attachClient {
		attachOverviewBroadcastTestClient(t, hub)
	}

	return pipeline, hub, store, topologyNotify, deviceID
}

func attachOverviewBroadcastTestClient(t *testing.T, hub *ws.Hub) {
	t.Helper()

	go hub.Run()
	server := httptest.NewServer(ws.NewHandler(
		hub,
		func() (*ws.SnapshotPayload, uint64) { return ws.EmptySnapshot(), 0 },
		func() ws.AlertMessagePayload { return ws.AlertMessagePayload{Alerts: []ws.AlertDTO{}} },
		func() ws.PrometheusStatusPayload { return ws.PrometheusStatusPayload{} },
	))
	t.Cleanup(server.Close)

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http") + "?runtime_version=0"
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("failed to dial overview broadcast websocket test client: %v", err)
	}
	t.Cleanup(func() {
		_ = conn.Close()
	})

	waitForOverviewBroadcastClient(t, hub)
	drainOverviewBroadcastClientBootstrap(t, conn)
}

func waitForOverviewBroadcastClient(t *testing.T, hub *ws.Hub) {
	t.Helper()

	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if hub.HasOverviewClients() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("expected overview broadcast test client to register")
}

func drainOverviewBroadcastClientBootstrap(t *testing.T, conn *websocket.Conn) {
	t.Helper()

	conn.SetReadDeadline(time.Now().Add(time.Second))
	defer conn.SetReadDeadline(time.Time{})
	for i := 0; i < 2; i++ {
		if _, _, err := conn.ReadMessage(); err != nil {
			t.Fatalf("failed to drain overview broadcast bootstrap message %d: %v", i+1, err)
		}
	}
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
			{IfIndex: 1, IfName: "ether1", IfDescr: "uplink", Speed: 1_000_000_000},
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

func TestApplySnapshotDeltaToRuntime_UpdatesSnapshotAndHashesIncrementally(t *testing.T) {
	deviceAID := uuid.New().String()
	deviceBID := uuid.New().String()
	linkAID := uuid.New().String()
	linkBID := uuid.New().String()
	updatedCollectedAt := "2026-04-20T10:30:00Z"

	base := ws.EmptySnapshot()
	base.Devices[deviceAID] = ws.DeviceRuntimeDTO{
		DeviceID:          deviceAID,
		OperationalStatus: string(domain.DeviceStatusDown),
		MetricsStatus:     "missing",
	}
	base.Devices[deviceBID] = ws.DeviceRuntimeDTO{
		DeviceID:          deviceBID,
		OperationalStatus: string(domain.DeviceStatusUp),
		RuntimeFlags:      []string{},
		MetricsStatus:     "available",
	}
	base.Links[linkAID] = ws.LinkRuntimeDTO{
		LinkID:         linkAID,
		SourceDeviceID: deviceAID,
		TargetDeviceID: deviceBID,
		DeviceID:       deviceAID,
		SourceIfName:   "ether1",
		MetricsStatus:  "missing",
	}
	base.Links[linkBID] = ws.LinkRuntimeDTO{
		LinkID:         linkBID,
		SourceDeviceID: deviceBID,
		TargetDeviceID: deviceAID,
		DeviceID:       deviceBID,
		SourceIfName:   "ether2",
		MetricsStatus:  "available",
	}
	syncSnapshotCompatibility(base)
	baseHashes := computeSnapshotHashes(base)
	expectedBase := ws.CloneSnapshot(base)

	delta := ws.EmptySnapshot()
	delta.Devices[deviceAID] = ws.DeviceRuntimeDTO{
		DeviceID:          deviceAID,
		OperationalStatus: string(domain.DeviceStatusUp),
		MetricsStatus:     "available",
	}
	delta.Links[linkAID] = ws.LinkRuntimeDTO{
		LinkID:          linkAID,
		SourceDeviceID:  deviceAID,
		TargetDeviceID:  deviceBID,
		DeviceID:        deviceAID,
		SourceIfName:    "ether1",
		MetricsStatus:   "available",
		LastCollectedAt: &updatedCollectedAt,
	}

	expected := mergeSnapshotPayload(expectedBase, delta)
	expectedHashes := computeSnapshotHashes(expected)

	got, gotHashes := applySnapshotDeltaToRuntime(base, baseHashes, delta)

	if got != base {
		t.Fatal("expected runtime snapshot to be updated in place")
	}
	if gotHashes != baseHashes {
		t.Fatal("expected section hashes to be updated in place")
	}
	if !reflect.DeepEqual(got, expected) {
		t.Fatalf("incremental snapshot mismatch\n got: %#v\nwant: %#v", got, expected)
	}
	if !reflect.DeepEqual(gotHashes, expectedHashes) {
		t.Fatalf("incremental hashes mismatch\n got: %#v\nwant: %#v", gotHashes, expectedHashes)
	}
}

func TestRemoveLinkRuntimeFromCompatibility_UsesPreviousBucketAndFallbackSourceDevice(t *testing.T) {
	linkID := uuid.New().String()
	oldDeviceID := uuid.New().String()
	otherDeviceID := uuid.New().String()
	sourceDeviceID := uuid.New().String()
	sourceFallbackLinkID := uuid.New().String()

	metrics := map[string][]ws.LinkRuntimeDTO{
		oldDeviceID: {
			{LinkID: linkID, DeviceID: oldDeviceID},
			{LinkID: "keep-old", DeviceID: oldDeviceID},
		},
		otherDeviceID: {
			{LinkID: "keep-other", DeviceID: otherDeviceID},
		},
		sourceDeviceID: {
			{LinkID: sourceFallbackLinkID, SourceDeviceID: sourceDeviceID},
		},
	}

	removeLinkRuntimeFromCompatibility(metrics, ws.LinkRuntimeDTO{LinkID: linkID, DeviceID: oldDeviceID}, linkID)

	if len(metrics[oldDeviceID]) != 1 || metrics[oldDeviceID][0].LinkID != "keep-old" {
		t.Fatalf("old bucket after targeted removal = %#v, want only keep-old", metrics[oldDeviceID])
	}
	if len(metrics[otherDeviceID]) != 1 || metrics[otherDeviceID][0].LinkID != "keep-other" {
		t.Fatalf("other bucket changed during targeted removal: %#v", metrics[otherDeviceID])
	}

	removeLinkRuntimeFromCompatibility(
		metrics,
		ws.LinkRuntimeDTO{LinkID: sourceFallbackLinkID, SourceDeviceID: sourceDeviceID},
		sourceFallbackLinkID,
	)

	if _, exists := metrics[sourceDeviceID]; exists {
		t.Fatalf("source fallback bucket still exists after removing its only link: %#v", metrics[sourceDeviceID])
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

func TestPipelineOrchestratorBroadcastDirty_DirtyDeviceNoClientSkipsNarrowOverviewBuild(t *testing.T) {
	pipeline, hub, store, _, deviceID := newNoClientBroadcastTestPipeline(t)

	pipeline.broadcaster.broadcastOnce(context.Background())
	drainBroadcastCh(hub)

	cpu := float64(72)
	store.Update(state.StateUpdate{
		DeviceID:        deviceID,
		VolatilityClass: domain.VolatilityClassPerformance,
		Metrics: &domain.DeviceMetrics{
			DeviceID:    deviceID,
			CPUPercent:  &cpu,
			CollectedAt: time.Date(2026, 4, 13, 12, 1, 0, 0, time.UTC),
		},
		PollSuccess:      true,
		ExpectedInterval: 30 * time.Second,
		Timestamp:        time.Date(2026, 4, 13, 12, 1, 0, 0, time.UTC),
	})

	hooks := installPipelineSnapshotHookCounters(t)
	if err := pipeline.broadcaster.broadcastDirty(context.Background(), map[uuid.UUID]struct{}{deviceID: {}}, false, false, false); err != nil {
		t.Fatalf("broadcastDirty returned error: %v", err)
	}

	if hooks.narrowCalls != 0 {
		t.Fatalf("narrow state snapshot calls = %d, want 0", hooks.narrowCalls)
	}
	if hooks.fullCalls != 0 {
		t.Fatalf("full state snapshot calls = %d, want 0", hooks.fullCalls)
	}
	if messages := drainBroadcastCh(hub); len(messages) != 0 {
		t.Fatalf("expected no overview broadcast messages without clients, got %v", broadcastMessageTypes(t, messages))
	}
}

func TestPipelineOrchestratorBroadcastDirty_ForcedFullResyncNoClientSkipsFullOverviewBuild(t *testing.T) {
	pipeline, hub, _, _, _ := newNoClientBroadcastTestPipeline(t)

	hooks := installPipelineSnapshotHookCounters(t)
	if err := pipeline.broadcaster.broadcastDirty(context.Background(), nil, false, false, true); err != nil {
		t.Fatalf("broadcastDirty returned error: %v", err)
	}

	if hooks.fullCalls != 0 {
		t.Fatalf("full state snapshot calls = %d, want 0", hooks.fullCalls)
	}
	if hooks.narrowCalls != 0 {
		t.Fatalf("narrow state snapshot calls = %d, want 0", hooks.narrowCalls)
	}
	if messages := drainBroadcastCh(hub); len(messages) != 0 {
		t.Fatalf("expected no overview broadcast messages without clients, got %v", broadcastMessageTypes(t, messages))
	}
}

func TestPipelineOrchestratorBroadcastDirty_NoClientDirtyWorkSetsRuntimeBaseStale(t *testing.T) {
	pipeline, _, _, _, deviceID := newNoClientBroadcastTestPipeline(t)

	pipeline.broadcaster.broadcastOnce(context.Background())
	pipeline.runtime.mu.RLock()
	seeded := pipeline.runtime.prevHashes != nil
	pipeline.runtime.mu.RUnlock()
	if !seeded {
		t.Fatal("expected broadcastOnce to seed runtime hash base")
	}

	if err := pipeline.broadcaster.broadcastDirty(context.Background(), map[uuid.UUID]struct{}{deviceID: {}}, false, false, false); err != nil {
		t.Fatalf("broadcastDirty returned error: %v", err)
	}

	pipeline.runtime.mu.RLock()
	stale := pipeline.runtime.prevHashes == nil
	pipeline.runtime.mu.RUnlock()
	if !stale {
		t.Fatal("expected no-client dirty work to clear runtime hash base")
	}
}

func TestPipelineOrchestratorBroadcastDirty_TopologyOnlyNoClientMarksRuntimeBaseStale(t *testing.T) {
	pipeline, hub, _, _, _ := newNoClientBroadcastTestPipeline(t)

	pipeline.broadcaster.broadcastOnce(context.Background())
	hooks := installPipelineSnapshotHookCounters(t)
	if err := pipeline.broadcaster.broadcastDirty(context.Background(), nil, false, true, false); err != nil {
		t.Fatalf("broadcastDirty returned error: %v", err)
	}

	pipeline.runtime.mu.RLock()
	stale := pipeline.runtime.prevHashes == nil
	pipeline.runtime.mu.RUnlock()
	if !stale {
		t.Fatal("expected topology-only no-client work to clear runtime hash base")
	}
	if hooks.fullCalls != 0 {
		t.Fatalf("full state snapshot calls = %d, want 0", hooks.fullCalls)
	}
	if hooks.narrowCalls != 0 {
		t.Fatalf("narrow state snapshot calls = %d, want 0", hooks.narrowCalls)
	}
	types := broadcastMessageTypes(t, drainBroadcastCh(hub))
	if len(types) != 1 || types[0] != ws.MessageTypeTopologyChanged {
		t.Fatalf("expected topology invalidation only, got %v", types)
	}
}

func TestPipelineOrchestratorBroadcastDirty_DirtyTopologyNoClientMarksRuntimeBaseStale(t *testing.T) {
	pipeline, hub, store, _, deviceID := newNoClientBroadcastTestPipeline(t)

	pipeline.broadcaster.broadcastOnce(context.Background())
	cpu := float64(91)
	store.Update(state.StateUpdate{
		DeviceID:        deviceID,
		VolatilityClass: domain.VolatilityClassPerformance,
		Metrics: &domain.DeviceMetrics{
			DeviceID:    deviceID,
			CPUPercent:  &cpu,
			CollectedAt: time.Date(2026, 4, 13, 12, 3, 0, 0, time.UTC),
		},
		PollSuccess:      true,
		ExpectedInterval: 30 * time.Second,
		Timestamp:        time.Date(2026, 4, 13, 12, 3, 0, 0, time.UTC),
	})

	hooks := installPipelineSnapshotHookCounters(t)
	if err := pipeline.broadcaster.broadcastDirty(context.Background(), map[uuid.UUID]struct{}{deviceID: {}}, false, true, false); err != nil {
		t.Fatalf("broadcastDirty returned error: %v", err)
	}

	pipeline.runtime.mu.RLock()
	stale := pipeline.runtime.prevHashes == nil
	pipeline.runtime.mu.RUnlock()
	if !stale {
		t.Fatal("expected dirty+topology no-client work to clear runtime hash base")
	}
	if hooks.fullCalls != 0 {
		t.Fatalf("full state snapshot calls = %d, want 0", hooks.fullCalls)
	}
	if hooks.narrowCalls != 0 {
		t.Fatalf("narrow state snapshot calls = %d, want 0", hooks.narrowCalls)
	}
	types := broadcastMessageTypes(t, drainBroadcastCh(hub))
	if len(types) != 1 || types[0] != ws.MessageTypeTopologyChanged {
		t.Fatalf("expected topology invalidation only, got %v", types)
	}
}

func TestPipelineOrchestratorBroadcastDirty_AlertOnlyNoClientStillBroadcastsAlert(t *testing.T) {
	pipeline, hub, _, _, deviceID := newNoClientBroadcastTestPipeline(t)

	pipeline.broadcaster.broadcastOnce(context.Background())
	drainBroadcastCh(hub)
	pipeline.runtime.setAlerts(map[uuid.UUID][]domain.AlertState{
		deviceID: {{
			DeviceID:  deviceID,
			Severity:  "critical",
			AlertName: "DeviceDown",
			State:     "firing",
			Summary:   "device down",
		}},
	})

	hooks := installPipelineSnapshotHookCounters(t)
	if err := pipeline.broadcaster.broadcastDirty(context.Background(), nil, true, false, false); err != nil {
		t.Fatalf("broadcastDirty returned error: %v", err)
	}

	types := broadcastMessageTypes(t, drainBroadcastCh(hub))
	if len(types) != 1 || types[0] != ws.MessageTypeAlert {
		t.Fatalf("expected alert-only no-client work to broadcast alert only, got %v", types)
	}
	if hooks.narrowCalls != 0 {
		t.Fatalf("narrow state snapshot calls = %d, want 0", hooks.narrowCalls)
	}
	if hooks.fullCalls != 0 {
		t.Fatalf("full state snapshot calls = %d, want 0", hooks.fullCalls)
	}
}

func TestPipelineOrchestratorGetOrBuildOverviewSnapshotRebuildsAfterSkippedNoClientDirtyFlush(t *testing.T) {
	pipeline, _, store, _, deviceID := newNoClientBroadcastTestPipeline(t)

	pipeline.broadcaster.broadcastOnce(context.Background())
	updatedCPU := float64(88)
	store.Update(state.StateUpdate{
		DeviceID:        deviceID,
		VolatilityClass: domain.VolatilityClassPerformance,
		Metrics: &domain.DeviceMetrics{
			DeviceID:    deviceID,
			CPUPercent:  &updatedCPU,
			CollectedAt: time.Date(2026, 4, 13, 12, 2, 0, 0, time.UTC),
		},
		PollSuccess:      true,
		ExpectedInterval: 30 * time.Second,
		Timestamp:        time.Date(2026, 4, 13, 12, 2, 0, 0, time.UTC),
	})
	if err := pipeline.broadcaster.broadcastDirty(context.Background(), map[uuid.UUID]struct{}{deviceID: {}}, false, false, false); err != nil {
		t.Fatalf("broadcastDirty returned error: %v", err)
	}

	hooks := installPipelineSnapshotHookCounters(t)
	snapshot, _ := pipeline.GetOrBuildOverviewSnapshot()

	if hooks.fullCalls != 1 {
		t.Fatalf("GetOrBuildOverviewSnapshot full state snapshot calls = %d, want 1", hooks.fullCalls)
	}
	deviceRuntime, ok := snapshot.Devices[deviceID.String()]
	if !ok {
		t.Fatalf("expected rebuilt snapshot to include device %s", deviceID)
	}
	if deviceRuntime.CPUPercent == nil || *deviceRuntime.CPUPercent != updatedCPU {
		t.Fatalf("CPUPercent = %#v, want %v", deviceRuntime.CPUPercent, updatedCPU)
	}
}

func TestPipelineOrchestratorGetOrBuildOverviewSnapshotBroadcastsRebuiltSnapshotToExistingClients(t *testing.T) {
	pipeline, hub, store, _, deviceID := newNoClientBroadcastTestPipeline(t)

	pipeline.broadcaster.broadcastOnce(context.Background())
	updatedCPU := float64(89)
	store.Update(state.StateUpdate{
		DeviceID:        deviceID,
		VolatilityClass: domain.VolatilityClassPerformance,
		Metrics: &domain.DeviceMetrics{
			DeviceID:    deviceID,
			CPUPercent:  &updatedCPU,
			CollectedAt: time.Date(2026, 4, 13, 12, 3, 0, 0, time.UTC),
		},
		PollSuccess:      true,
		ExpectedInterval: 30 * time.Second,
		Timestamp:        time.Date(2026, 4, 13, 12, 3, 0, 0, time.UTC),
	})
	if err := pipeline.broadcaster.broadcastDirty(context.Background(), map[uuid.UUID]struct{}{deviceID: {}}, false, false, false); err != nil {
		t.Fatalf("broadcastDirty returned error: %v", err)
	}

	attachOverviewBroadcastTestClient(t, hub)
	drainBroadcastCh(hub)

	hooks := installPipelineSnapshotHookCounters(t)
	snapshot, _ := pipeline.GetOrBuildOverviewSnapshot()

	if hooks.fullCalls != 1 {
		t.Fatalf("GetOrBuildOverviewSnapshot full state snapshot calls = %d, want 1", hooks.fullCalls)
	}
	deviceRuntime, ok := snapshot.Devices[deviceID.String()]
	if !ok {
		t.Fatalf("expected rebuilt snapshot to include device %s", deviceID)
	}
	if deviceRuntime.CPUPercent == nil || *deviceRuntime.CPUPercent != updatedCPU {
		t.Fatalf("CPUPercent = %#v, want %v", deviceRuntime.CPUPercent, updatedCPU)
	}
	types := broadcastMessageTypes(t, drainBroadcastCh(hub))
	if len(types) != 1 || types[0] != ws.MessageTypeSnapshot {
		t.Fatalf("expected rebuilt stale base to broadcast full snapshot to existing clients, got %v", types)
	}
}

func TestPipelineOrchestratorBroadcastDirty_DirtyBuildWithClientDoesNotLetConcurrentBootstrapReplaceBase(t *testing.T) {
	pipeline, hub, store, _, deviceID := newBroadcastTestPipeline(t)

	pipeline.broadcaster.broadcastOnce(context.Background())
	drainBroadcastCh(hub)

	updatedCPU := float64(93)
	store.Update(state.StateUpdate{
		DeviceID:        deviceID,
		VolatilityClass: domain.VolatilityClassPerformance,
		Metrics: &domain.DeviceMetrics{
			DeviceID:    deviceID,
			CPUPercent:  &updatedCPU,
			CollectedAt: time.Date(2026, 4, 13, 12, 4, 0, 0, time.UTC),
		},
		PollSuccess:      true,
		ExpectedInterval: 30 * time.Second,
		Timestamp:        time.Date(2026, 4, 13, 12, 4, 0, 0, time.UTC),
	})

	previousSnapshotAll := snapshotAllPipelineState
	previousSnapshotFor := snapshotPipelineStateFor
	dirtyBuildEntered := make(chan struct{}, 1)
	releaseDirtyBuild := make(chan struct{})
	var releaseOnce sync.Once
	fullBuildCalls := 0
	var fullBuildMu sync.Mutex
	snapshotAllPipelineState = func(store *state.Store) map[uuid.UUID]state.DeviceState {
		fullBuildMu.Lock()
		fullBuildCalls++
		fullBuildMu.Unlock()
		return store.Snapshot()
	}
	snapshotPipelineStateFor = func(store *state.Store, ids []uuid.UUID) map[uuid.UUID]state.DeviceState {
		select {
		case dirtyBuildEntered <- struct{}{}:
		default:
		}
		<-releaseDirtyBuild
		return store.SnapshotFor(ids)
	}
	t.Cleanup(func() {
		releaseOnce.Do(func() {
			close(releaseDirtyBuild)
		})
		snapshotAllPipelineState = previousSnapshotAll
		snapshotPipelineStateFor = previousSnapshotFor
	})

	dirtyErr := make(chan error, 1)
	go func() {
		dirtyErr <- pipeline.broadcaster.broadcastDirty(context.Background(), map[uuid.UUID]struct{}{deviceID: {}}, false, false, false)
	}()

	select {
	case <-dirtyBuildEntered:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for dirty overview build to enter")
	}

	bootstrapDone := make(chan struct{})
	go func() {
		_, _ = pipeline.GetOrBuildOverviewSnapshot()
		close(bootstrapDone)
	}()

	select {
	case <-bootstrapDone:
	case <-time.After(250 * time.Millisecond):
	}

	releaseOnce.Do(func() {
		close(releaseDirtyBuild)
	})

	select {
	case err := <-dirtyErr:
		if err != nil {
			t.Fatalf("broadcastDirty returned error: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for dirty overview build to finish")
	}
	select {
	case <-bootstrapDone:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for concurrent GetOrBuildOverviewSnapshot")
	}

	fullBuildMu.Lock()
	gotFullBuildCalls := fullBuildCalls
	fullBuildMu.Unlock()
	if gotFullBuildCalls != 0 {
		t.Fatalf("concurrent GetOrBuildOverviewSnapshot full state snapshot calls = %d, want 0", gotFullBuildCalls)
	}

	messages := drainBroadcastCh(hub)
	types := broadcastMessageTypes(t, messages)
	if len(types) != 1 || types[0] != ws.MessageTypeRuntimeDelta {
		t.Fatalf("expected connected client dirty flush to broadcast runtime_delta, got %v", types)
	}
}

func TestPipelineOrchestratorBroadcastDirty_DirtyDeviceWithStaleBaseUsesFullSnapshot(t *testing.T) {
	pipeline, hub, store, _, deviceAID := newNoClientBroadcastTestPipeline(t)
	deviceBID := uuid.New()
	pipeline.cache = newPipelineTestCache([]domain.Device{
		{
			ID:            deviceAID,
			IP:            "192.0.2.40",
			Status:        domain.DeviceStatusProbing,
			SysName:       "dist-sw-1",
			HardwareModel: "CRS328-24P-4S+",
			Interfaces:    []domain.Interface{{IfName: "ether1", IfDescr: "uplink", Speed: 1_000_000_000}},
		},
		{
			ID:            deviceBID,
			IP:            "192.0.2.41",
			Status:        domain.DeviceStatusProbing,
			SysName:       "dist-sw-2",
			HardwareModel: "CRS326-24G-2S+",
			Interfaces:    []domain.Interface{{IfName: "ether1", IfDescr: "uplink", Speed: 1_000_000_000}},
		},
	}, nil)
	store.Update(state.StateUpdate{
		DeviceID:        deviceBID,
		VolatilityClass: domain.VolatilityClassPerformance,
		Metrics: &domain.DeviceMetrics{
			DeviceID:    deviceBID,
			CPUPercent:  floatPtr(35),
			CollectedAt: time.Date(2026, 4, 13, 12, 0, 0, 0, time.UTC),
		},
		PollSuccess:      true,
		ExpectedInterval: 30 * time.Second,
		Timestamp:        time.Date(2026, 4, 13, 12, 0, 0, 0, time.UTC),
	})

	pipeline.broadcaster.broadcastOnce(context.Background())
	drainBroadcastCh(hub)

	cpuA := float64(74)
	store.Update(state.StateUpdate{
		DeviceID:        deviceAID,
		VolatilityClass: domain.VolatilityClassPerformance,
		Metrics: &domain.DeviceMetrics{
			DeviceID:    deviceAID,
			CPUPercent:  &cpuA,
			CollectedAt: time.Date(2026, 4, 13, 12, 5, 0, 0, time.UTC),
		},
		PollSuccess:      true,
		ExpectedInterval: 30 * time.Second,
		Timestamp:        time.Date(2026, 4, 13, 12, 5, 0, 0, time.UTC),
	})
	if err := pipeline.broadcaster.broadcastDirty(context.Background(), map[uuid.UUID]struct{}{deviceAID: {}}, false, false, false); err != nil {
		t.Fatalf("broadcastDirty no-client dirty A returned error: %v", err)
	}
	pipeline.runtime.mu.RLock()
	stale := pipeline.runtime.prevHashes == nil
	pipeline.runtime.mu.RUnlock()
	if !stale {
		t.Fatal("expected no-client dirty A work to clear runtime hash base")
	}

	attachOverviewBroadcastTestClient(t, hub)

	cpuB := float64(82)
	store.Update(state.StateUpdate{
		DeviceID:        deviceBID,
		VolatilityClass: domain.VolatilityClassPerformance,
		Metrics: &domain.DeviceMetrics{
			DeviceID:    deviceBID,
			CPUPercent:  &cpuB,
			CollectedAt: time.Date(2026, 4, 13, 12, 6, 0, 0, time.UTC),
		},
		PollSuccess:      true,
		ExpectedInterval: 30 * time.Second,
		Timestamp:        time.Date(2026, 4, 13, 12, 6, 0, 0, time.UTC),
	})

	hooks := installPipelineSnapshotHookCounters(t)
	if err := pipeline.broadcaster.broadcastDirty(context.Background(), map[uuid.UUID]struct{}{deviceBID: {}}, false, false, false); err != nil {
		t.Fatalf("broadcastDirty live-client dirty B returned error: %v", err)
	}

	if hooks.fullCalls != 1 {
		t.Fatalf("full state snapshot calls = %d, want 1", hooks.fullCalls)
	}
	if hooks.narrowCalls != 0 {
		t.Fatalf("narrow state snapshot calls = %d, want 0", hooks.narrowCalls)
	}
	types := broadcastMessageTypes(t, drainBroadcastCh(hub))
	if len(types) != 1 || types[0] != ws.MessageTypeSnapshot {
		t.Fatalf("expected stale-base dirty device to broadcast full snapshot, got %v", types)
	}

	pipeline.runtime.mu.RLock()
	snapshot := ws.CloneSnapshot(pipeline.runtime.lastSnapshot)
	pipeline.runtime.mu.RUnlock()
	deviceA, ok := snapshot.Devices[deviceAID.String()]
	if !ok {
		t.Fatalf("expected runtime snapshot to include device A %s", deviceAID)
	}
	if deviceA.CPUPercent == nil || *deviceA.CPUPercent != cpuA {
		t.Fatalf("device A CPUPercent = %#v, want %v", deviceA.CPUPercent, cpuA)
	}
	deviceB, ok := snapshot.Devices[deviceBID.String()]
	if !ok {
		t.Fatalf("expected runtime snapshot to include device B %s", deviceBID)
	}
	if deviceB.CPUPercent == nil || *deviceB.CPUPercent != cpuB {
		t.Fatalf("device B CPUPercent = %#v, want %v", deviceB.CPUPercent, cpuB)
	}
}

func TestPipelineOrchestratorGetOrBuildOverviewSnapshotConcurrentRebuildsShareSingleBuildAndVersion(t *testing.T) {
	pipeline, _, _, _, _ := newNoClientBroadcastTestPipeline(t)

	pipeline.runtime.mu.Lock()
	pipeline.runtime.prevHashes = nil
	pipeline.runtime.overviewVersion = 10
	pipeline.runtime.mu.Unlock()

	previousSnapshotAll := snapshotAllPipelineState
	previousSnapshotFor := snapshotPipelineStateFor
	const callers = 8
	buildEntered := make(chan struct{}, callers)
	releaseBuild := make(chan struct{})
	var buildMu sync.Mutex
	buildCalls := 0
	snapshotAllPipelineState = func(store *state.Store) map[uuid.UUID]state.DeviceState {
		buildMu.Lock()
		buildCalls++
		buildMu.Unlock()
		buildEntered <- struct{}{}
		<-releaseBuild
		return store.Snapshot()
	}
	snapshotPipelineStateFor = func(store *state.Store, ids []uuid.UUID) map[uuid.UUID]state.DeviceState {
		t.Fatalf("GetOrBuildOverviewSnapshot called narrow state snapshot hook with ids=%v", ids)
		return nil
	}
	t.Cleanup(func() {
		snapshotAllPipelineState = previousSnapshotAll
		snapshotPipelineStateFor = previousSnapshotFor
	})

	start := make(chan struct{})
	var ready sync.WaitGroup
	var done sync.WaitGroup
	versions := make([]uint64, callers)
	ready.Add(callers)
	done.Add(callers)
	for i := 0; i < callers; i++ {
		go func(index int) {
			defer done.Done()
			ready.Done()
			<-start
			_, version := pipeline.GetOrBuildOverviewSnapshot()
			versions[index] = version
		}(i)
	}
	ready.Wait()
	close(start)

	waitForBuildEntrants(t, buildEntered, 1, time.Second)
	_ = waitForBuildEntrantsOrTimeout(buildEntered, 1, 250*time.Millisecond)
	close(releaseBuild)

	doneCh := make(chan struct{})
	go func() {
		done.Wait()
		close(doneCh)
	}()
	select {
	case <-doneCh:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for concurrent GetOrBuildOverviewSnapshot calls")
	}

	buildMu.Lock()
	gotBuildCalls := buildCalls
	buildMu.Unlock()
	if gotBuildCalls != 1 {
		t.Fatalf("full state snapshot calls = %d, want 1", gotBuildCalls)
	}
	for i, version := range versions {
		if version != versions[0] {
			t.Fatalf("versions[%d] = %d, want shared version %d: all=%v", i, version, versions[0], versions)
		}
	}
	if versions[0] != 11 {
		t.Fatalf("shared version = %d, want 11", versions[0])
	}
}

func TestGetOrBuildOverviewStateReturnsClonedAtomicState(t *testing.T) {
	pipeline, _, _, _, deviceID := newNoClientBroadcastTestPipeline(t)

	first := pipeline.GetOrBuildOverviewState()
	if first.StreamID == "" {
		t.Fatal("overview stream ID is empty")
	}
	if _, err := uuid.Parse(first.StreamID); err != nil {
		t.Fatalf("overview stream ID %q is not a UUID: %v", first.StreamID, err)
	}
	if first.Snapshot == nil {
		t.Fatal("overview snapshot is nil")
	}
	if _, ok := first.Snapshot.Devices[deviceID.String()]; !ok {
		t.Fatalf("overview snapshot does not contain device %s", deviceID)
	}

	pipeline.overviewBuildMu.Lock()
	pipeline.runtime.mu.RLock()
	storedSnapshot := pipeline.runtime.lastSnapshot
	storedVersion := pipeline.runtime.overviewVersion
	storedStreamID := pipeline.runtime.overviewStreamID
	pipeline.runtime.mu.RUnlock()
	pipeline.overviewBuildMu.Unlock()
	if first.Snapshot == storedSnapshot {
		t.Fatal("GetOrBuildOverviewState leaked the stored snapshot")
	}
	if first.Version != storedVersion || first.StreamID != storedStreamID {
		t.Fatalf(
			"overview state tuple = (%d, %q), want (%d, %q)",
			first.Version,
			first.StreamID,
			storedVersion,
			storedStreamID,
		)
	}

	delete(first.Snapshot.Devices, deviceID.String())
	second := pipeline.GetOrBuildOverviewState()
	if second.Version != first.Version || second.StreamID != first.StreamID {
		t.Fatalf(
			"unchanged overview state tuple = (%d, %q), want (%d, %q)",
			second.Version,
			second.StreamID,
			first.Version,
			first.StreamID,
		)
	}
	if _, ok := second.Snapshot.Devices[deviceID.String()]; !ok {
		t.Fatalf("mutating returned state removed stored device %s", deviceID)
	}
}

func TestGetOrBuildOverviewStateConcurrentGettersObserveOneTuple(t *testing.T) {
	pipeline, _, _, _, _ := newNoClientBroadcastTestPipeline(t)

	pipeline.runtime.mu.Lock()
	pipeline.runtime.lastSnapshot = ws.EmptySnapshot()
	pipeline.runtime.prevHashes = computeSnapshotHashes(pipeline.runtime.lastSnapshot)
	pipeline.runtime.overviewVersion = 1
	pipeline.runtime.overviewStreamID = "stream-1"
	pipeline.runtime.mu.Unlock()

	pipeline.overviewBuildMu.Lock()
	pipeline.runtime.mu.Lock()
	pipeline.runtime.overviewVersion = 2
	pipeline.runtime.mu.Unlock()

	const callers = 8
	results := make(chan ws.RuntimeOverviewState, callers)
	start := make(chan struct{})
	var ready sync.WaitGroup
	ready.Add(callers)
	for range callers {
		go func() {
			ready.Done()
			<-start
			results <- pipeline.GetOrBuildOverviewState()
		}()
	}
	ready.Wait()
	close(start)

	select {
	case state := <-results:
		pipeline.overviewBuildMu.Unlock()
		t.Fatalf("getter returned while overview tuple was incomplete: %#v", state)
	case <-time.After(50 * time.Millisecond):
	}

	pipeline.runtime.mu.Lock()
	pipeline.runtime.overviewStreamID = "stream-2"
	pipeline.runtime.mu.Unlock()
	pipeline.overviewBuildMu.Unlock()

	for range callers {
		select {
		case state := <-results:
			if state.Version != 2 || state.StreamID != "stream-2" {
				t.Fatalf("concurrent overview state tuple = (%d, %q), want (2, %q)", state.Version, state.StreamID, "stream-2")
			}
		case <-time.After(time.Second):
			t.Fatal("timed out waiting for concurrent overview state getter")
		}
	}
}

func TestOverviewMailboxRecoverySelectsCurrentReplayAndLegacySnapshot(t *testing.T) {
	hub := ws.NewHub()
	go hub.Run()
	pipeline := NewPipelineOrchestrator(
		newPipelineTestScheduler(),
		nil,
		nil,
		hub,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
	)
	configureRuntimeRecoveryState(t, pipeline, "runtime-stream-1", 92)

	v2Conn, v2Client, releaseV2 := attachRuntimeRecoveryClient(
		t,
		hub,
		pipeline.GetOverviewSnapshot,
		ws.RuntimeStreamProtocolVersion,
		ws.RuntimeCursor{StreamID: "runtime-stream-1", Version: 92, Known: true},
		false,
	)
	defer releaseV2()
	legacyConn, legacyClient, releaseLegacy := attachRuntimeRecoveryClient(
		t,
		hub,
		pipeline.GetOverviewSnapshot,
		0,
		ws.RuntimeCursor{StreamID: "runtime-stream-1", Version: 92, Known: true},
		false,
	)
	defer releaseLegacy()

	if got := v2Client.RuntimeProtocol(); got != ws.RuntimeStreamProtocolVersion {
		t.Fatalf("protocol-v2 client capability = %d, want %d", got, ws.RuntimeStreamProtocolVersion)
	}
	if got := v2Client.AckedRuntimeCursor(); got != (ws.RuntimeCursor{StreamID: "runtime-stream-1", Version: 92, Known: true}) {
		t.Fatalf("protocol-v2 ACK cursor = %#v, want runtime-stream-1/92", got)
	}

	pipeline.SyncOverviewClient(v2Client, ws.RuntimeSyncRequest{
		Cursor: ws.RuntimeCursor{StreamID: "runtime-stream-1", Version: 92, Known: true},
		Reason: ws.ResyncReasonClientResync,
	})
	current := readRuntimeRecoveryMessages(t, v2Conn, 1)
	assertRuntimeRecoveryMessageTypes(t, current, ws.MessageTypeReady)
	var currentReady ws.ReadyPayload
	decodeRuntimeRecoveryPayload(t, current[0], &currentReady)
	if currentReady.SyncMode != string(ws.OverviewSyncModeCurrent) || currentReady.RuntimeVersion != 92 {
		t.Fatalf("current ready payload = %#v, want current/92", currentReady)
	}

	advanceRuntimeRecoveryState(t, pipeline, 92, 93, &ws.RuntimeDeltaPayload{
		Devices: map[string]map[string]any{"dev-1": {"primary_health": "degraded"}},
		Links:   map[string]map[string]any{},
	})
	advanceRuntimeRecoveryState(t, pipeline, 93, 94, &ws.RuntimeDeltaPayload{
		Devices: map[string]map[string]any{"dev-1": {"primary_health": "up_fresh"}},
		Links:   map[string]map[string]any{},
	})
	assertRuntimeRecoveryMessageTypes(
		t,
		readRuntimeRecoveryMessages(t, v2Conn, 2),
		ws.MessageTypeRuntimeDelta,
		ws.MessageTypeRuntimeDelta,
	)
	assertRuntimeRecoveryMessageTypes(
		t,
		readRuntimeRecoveryMessages(t, legacyConn, 2),
		ws.MessageTypeRuntimeDelta,
		ws.MessageTypeRuntimeDelta,
	)

	pipeline.SyncOverviewClient(v2Client, ws.RuntimeSyncRequest{
		Cursor: ws.RuntimeCursor{StreamID: "runtime-stream-1", Version: 92, Known: true},
		Reason: ws.ResyncReasonClientResync,
	})
	replay := readRuntimeRecoveryMessages(t, v2Conn, 3)
	assertRuntimeRecoveryMessageTypes(t, replay, ws.MessageTypeResyncRequired, ws.MessageTypeRuntimeReplay, ws.MessageTypeReady)
	var replayState ws.RuntimeReplayMessagePayload
	decodeRuntimeRecoveryPayload(t, replay[1], &replayState)
	if replayState.FromVersion != 92 || replayState.Version != 94 || replayState.RuntimeStreamID != "runtime-stream-1" {
		t.Fatalf("replay payload = %#v, want runtime-stream-1 92->94", replayState)
	}

	pipeline.SyncOverviewClient(legacyClient, ws.RuntimeSyncRequest{
		Cursor: ws.RuntimeCursor{StreamID: "runtime-stream-1", Version: 92, Known: true},
		Reason: ws.ResyncReasonClientResync,
	})
	legacy := readRuntimeRecoveryMessages(t, legacyConn, 3)
	assertRuntimeRecoveryMessageTypes(t, legacy, ws.MessageTypeResyncRequired, ws.MessageTypeSnapshot, ws.MessageTypeReady)
	for _, message := range legacy {
		if message.Type == ws.MessageTypeRuntimeReplay {
			t.Fatal("legacy client received runtime_replay")
		}
	}

	pipeline.SyncOverviewClient(v2Client, ws.RuntimeSyncRequest{
		Cursor: ws.RuntimeCursor{StreamID: "obsolete-stream", Version: 94, Known: true},
		Reason: ws.ResyncReasonClientResync,
	})
	wrongStream := readRuntimeRecoveryMessages(t, v2Conn, 3)
	assertRuntimeRecoveryMessageTypes(t, wrongStream, ws.MessageTypeResyncRequired, ws.MessageTypeSnapshot, ws.MessageTypeReady)
}

func TestOverviewMailboxRecoveryFallsBackWhenSameStreamClientCursorDiffersFromRequest(t *testing.T) {
	hub := ws.NewHub()
	go hub.Run()
	pipeline := NewPipelineOrchestrator(
		newPipelineTestScheduler(),
		nil,
		nil,
		hub,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
	)
	configureRuntimeRecoveryState(t, pipeline, "runtime-stream-1", 92)

	conn, client, release := attachRuntimeRecoveryClient(
		t,
		hub,
		pipeline.GetOverviewSnapshot,
		ws.RuntimeStreamProtocolVersion,
		ws.RuntimeCursor{StreamID: "runtime-stream-1", Version: 92, Known: true},
		false,
	)
	defer release()

	advanceRuntimeRecoveryState(t, pipeline, 92, 93, &ws.RuntimeDeltaPayload{
		Devices: map[string]map[string]any{"dev-1": {"primary_health": "degraded"}},
		Links:   map[string]map[string]any{},
	})
	advanceRuntimeRecoveryState(t, pipeline, 93, 94, &ws.RuntimeDeltaPayload{
		Devices: map[string]map[string]any{"dev-1": {"primary_health": "up_fresh"}},
		Links:   map[string]map[string]any{},
	})
	assertRuntimeRecoveryMessageTypes(
		t,
		readRuntimeRecoveryMessages(t, conn, 2),
		ws.MessageTypeRuntimeDelta,
		ws.MessageTypeRuntimeDelta,
	)
	if err := conn.WriteJSON(map[string]any{
		"type": ws.MessageTypeHello,
		"payload": map[string]any{
			"runtime_protocol":  ws.RuntimeStreamProtocolVersion,
			"runtime_stream_id": "runtime-stream-1",
			"runtime_version":   93,
		},
	}); err != nil {
		t.Fatalf("advance runtime recovery client cursor: %v", err)
	}
	deadline := time.Now().Add(time.Second)
	for client.AckedRuntimeCursor().Version != 93 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if got := client.AckedRuntimeCursor(); got.StreamID != "runtime-stream-1" || got.Version != 93 {
		t.Fatalf("advanced runtime cursor = %#v, want runtime-stream-1/93", got)
	}

	pipeline.SyncOverviewClient(client, ws.RuntimeSyncRequest{
		Cursor: ws.RuntimeCursor{StreamID: "runtime-stream-1", Version: 92, Known: true},
		Reason: ws.ResyncReasonClientResync,
	})
	messages := readRuntimeRecoveryMessages(t, conn, 3)
	assertRuntimeRecoveryMessageTypes(t, messages, ws.MessageTypeResyncRequired, ws.MessageTypeSnapshot, ws.MessageTypeReady)
}

func TestOverviewMailboxRecoveryCurrentRequestFallsBackWhenLiveClientCursorIsBehind(t *testing.T) {
	hub := ws.NewHub()
	go hub.Run()
	pipeline := NewPipelineOrchestrator(
		newPipelineTestScheduler(),
		nil,
		nil,
		hub,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
	)
	configureRuntimeRecoveryState(t, pipeline, "runtime-stream-1", 93)

	conn, client, release := attachRuntimeRecoveryClient(
		t,
		hub,
		pipeline.GetOverviewSnapshot,
		ws.RuntimeStreamProtocolVersion,
		ws.RuntimeCursor{StreamID: "runtime-stream-1", Version: 93, Known: true},
		false,
	)
	defer release()
	advanceRuntimeRecoveryState(t, pipeline, 93, 94, &ws.RuntimeDeltaPayload{
		Devices: map[string]map[string]any{"dev-1": {"primary_health": "up_fresh"}},
		Links:   map[string]map[string]any{},
	})
	assertRuntimeRecoveryMessageTypes(
		t,
		readRuntimeRecoveryMessages(t, conn, 1),
		ws.MessageTypeRuntimeDelta,
	)
	if got := client.AckedRuntimeCursor(); got != (ws.RuntimeCursor{StreamID: "runtime-stream-1", Version: 93, Known: true}) {
		t.Fatalf("live client cursor = %#v, want runtime-stream-1/93", got)
	}

	pipeline.SyncOverviewClient(client, ws.RuntimeSyncRequest{
		Cursor: ws.RuntimeCursor{StreamID: "runtime-stream-1", Version: 94, Known: true},
		Reason: ws.ResyncReasonClientResync,
	})
	first := readRuntimeRecoveryMessages(t, conn, 1)
	if first[0].Type == ws.MessageTypeReady {
		t.Fatal("current request emitted ready-only target 94 while live client ACK remained at 93")
	}
	recovery := append(first, readRuntimeRecoveryMessages(t, conn, 2)...)
	assertRuntimeRecoveryMessageTypes(t, recovery, ws.MessageTypeResyncRequired, ws.MessageTypeSnapshot, ws.MessageTypeReady)
}

func TestRuntimeRecoveryMetricsCompleteReplayOnlyAtValidTargetAck(t *testing.T) {
	registry := observability.ResetDefaultForTest()
	hub := ws.NewHub()
	go hub.Run()
	pipeline := NewPipelineOrchestrator(
		newPipelineTestScheduler(), nil, nil, hub, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil,
	)
	now := time.Date(2026, time.July, 14, 12, 0, 0, 0, time.UTC)
	pipeline.runtime.now = func() time.Time { return now }
	configureRuntimeRecoveryState(t, pipeline, "runtime-stream-1", 92)

	conn, client, release := attachRuntimeRecoveryClient(
		t, hub, pipeline.GetOverviewSnapshot, ws.RuntimeStreamProtocolVersion,
		ws.RuntimeCursor{StreamID: "runtime-stream-1", Version: 92, Known: true}, false,
	)
	defer release()
	advanceRuntimeRecoveryState(t, pipeline, 92, 93, &ws.RuntimeDeltaPayload{
		Devices: map[string]map[string]any{"dev-1": {"primary_health": "degraded"}},
		Links:   map[string]map[string]any{},
	})
	advanceRuntimeRecoveryState(t, pipeline, 93, 94, &ws.RuntimeDeltaPayload{
		Devices: map[string]map[string]any{"dev-1": {"primary_health": "up_fresh"}},
		Links:   map[string]map[string]any{},
	})
	readRuntimeRecoveryMessages(t, conn, 2)

	pipeline.SyncOverviewClient(client, ws.RuntimeSyncRequest{
		Cursor: ws.RuntimeCursor{StreamID: "runtime-stream-1", Version: 92, Known: true},
		Reason: ws.ResyncReasonClientResync,
	})
	assertRuntimeRecoveryMessageTypes(
		t,
		readRuntimeRecoveryMessages(t, conn, 3),
		ws.MessageTypeResyncRequired,
		ws.MessageTypeRuntimeReplay,
		ws.MessageTypeReady,
	)

	body := string(registry.MarshalPrometheus())
	assertPipelineMetric(t, body, `theia_ws_runtime_recovery_total{mode="replay",outcome="scheduled",reason="client_gap"} 1`)
	assertPipelineMetric(t, body, `theia_ws_runtime_replay_versions_count 1`)
	assertPipelineMetric(t, body, `theia_ws_runtime_replay_versions_sum 2`)
	if strings.Contains(body, `mode="replay",outcome="completed"`) {
		t.Fatalf("replay completed before target ACK\n%s", body)
	}

	now = now.Add(time.Second)
	pipeline.ObserveRuntimeAck(client, ws.RuntimeCursor{StreamID: "wrong-stream", Version: 94, Known: true})
	pipeline.ObserveRuntimeAck(client, ws.RuntimeCursor{StreamID: "runtime-stream-1", Version: 95, Known: true})
	pipeline.ObserveRuntimeAck(client, ws.RuntimeCursor{StreamID: "runtime-stream-1", Version: 93, Known: true})
	body = string(registry.MarshalPrometheus())
	assertPipelineMetric(t, body, `theia_ws_runtime_ack_lag_versions_count 1`)
	assertPipelineMetric(t, body, `theia_ws_runtime_ack_lag_versions_sum 1`)
	if strings.Contains(body, `mode="replay",outcome="completed"`) {
		t.Fatalf("replay completed below target ACK\n%s", body)
	}

	pipeline.ObserveRuntimeAck(client, ws.RuntimeCursor{StreamID: "runtime-stream-1", Version: 94, Known: true})
	body = string(registry.MarshalPrometheus())
	assertPipelineMetric(t, body, `theia_ws_runtime_ack_lag_versions_count 2`)
	assertPipelineMetric(t, body, `theia_ws_runtime_ack_lag_versions_sum 1`)
	assertPipelineMetric(t, body, `theia_ws_runtime_recovery_total{mode="replay",outcome="completed",reason="client_gap"} 1`)
	assertPipelineMetric(t, body, `theia_ws_runtime_recovery_duration_seconds_sum{mode="replay",outcome="completed"} 1`)
	if got := len(pipeline.runtimeRecoveryAttempts); got != 0 {
		t.Fatalf("runtime recovery attempts after completion = %d, want 0", got)
	}
}

func TestRuntimeAckInvalidCursorDiagnosticsStayBounded(t *testing.T) {
	previousWriter := log.Writer()
	previousFlags := log.Flags()
	previousPrefix := log.Prefix()
	var output bytes.Buffer
	log.SetOutput(&output)
	logging.Configure("debug")
	t.Cleanup(func() {
		logging.Configure("info")
		log.SetOutput(previousWriter)
		log.SetFlags(previousFlags)
		log.SetPrefix(previousPrefix)
	})

	pipeline := NewPipelineOrchestrator(
		newPipelineTestScheduler(), nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil,
	)
	configureRuntimeRecoveryState(t, pipeline, "runtime-stream-secret", 94)
	client := &ws.Client{}
	pipeline.ObserveRuntimeAck(client, ws.RuntimeCursor{
		StreamID: "wrong-stream-secret",
		Version:  94,
		Known:    true,
	})
	pipeline.ObserveRuntimeAck(client, ws.RuntimeCursor{
		StreamID: "runtime-stream-secret",
		Version:  999,
		Known:    true,
	})

	logs := output.String()
	for _, reason := range []string{"stream_mismatch", "beyond_current"} {
		if !strings.Contains(logs, "runtime ACK ignored reason="+reason) {
			t.Fatalf("runtime ACK diagnostics missing bounded reason %q\n%s", reason, logs)
		}
	}
	for _, forbidden := range []string{"runtime-stream-secret", "wrong-stream-secret", "999"} {
		if strings.Contains(logs, forbidden) {
			t.Fatalf("runtime ACK diagnostics leaked cursor identity %q\n%s", forbidden, logs)
		}
	}
}

func TestRuntimeRecoveryMetricsTrackCurrentAndSnapshotSelections(t *testing.T) {
	tests := []struct {
		name       string
		request    ws.RuntimeSyncRequest
		wantMode   string
		wantReason string
	}{
		{
			name: "current",
			request: ws.RuntimeSyncRequest{
				Cursor: ws.RuntimeCursor{StreamID: "runtime-stream-1", Version: 92, Known: true},
				Reason: ws.ResyncReasonClientResync,
			},
			wantMode:   "current",
			wantReason: ws.ResyncReasonClientResync,
		},
		{
			name: "snapshot on connect",
			request: ws.RuntimeSyncRequest{
				Reason: ws.ResyncReasonClientResync,
			},
			wantMode:   "snapshot",
			wantReason: "connect",
		},
		{
			name: "snapshot on stream mismatch",
			request: ws.RuntimeSyncRequest{
				Cursor: ws.RuntimeCursor{StreamID: "obsolete-stream", Version: 92, Known: true},
				Reason: ws.ResyncReasonClientResync,
			},
			wantMode:   "snapshot",
			wantReason: "stream_mismatch",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			registry := observability.ResetDefaultForTest()
			hub := ws.NewHub()
			go hub.Run()
			pipeline := NewPipelineOrchestrator(
				newPipelineTestScheduler(), nil, nil, hub, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil,
			)
			configureRuntimeRecoveryState(t, pipeline, "runtime-stream-1", 92)
			_, client, release := attachRuntimeRecoveryClient(
				t, hub, pipeline.GetOverviewSnapshot, ws.RuntimeStreamProtocolVersion,
				ws.RuntimeCursor{StreamID: "runtime-stream-1", Version: 92, Known: true}, false,
			)
			defer release()

			pipeline.SyncOverviewClient(client, tt.request)

			body := string(registry.MarshalPrometheus())
			assertPipelineMetric(
				t,
				body,
				`theia_ws_runtime_recovery_total{mode="`+tt.wantMode+`",outcome="scheduled",reason="`+tt.wantReason+`"} 1`,
			)
		})
	}
}

func TestRuntimeRecoveryMetricsRecordFailedSnapshotInstallation(t *testing.T) {
	registry := observability.ResetDefaultForTest()
	hub := ws.NewHub()
	go hub.Run()
	pipeline := NewPipelineOrchestrator(
		newPipelineTestScheduler(), nil, nil, hub, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil,
	)
	configureRuntimeRecoveryState(t, pipeline, "runtime-stream-1", 92)
	_, client, release := attachRuntimeRecoveryClient(
		t, hub, pipeline.GetOverviewSnapshot, ws.RuntimeStreamProtocolVersion,
		ws.RuntimeCursor{StreamID: "runtime-stream-1", Version: 92, Known: true}, false,
	)
	defer release()

	notJSON := math.NaN()
	snapshot := ws.EmptySnapshot()
	snapshot.Devices["dev-1"] = ws.DeviceRuntimeDTO{DeviceID: "dev-1", CPUPercent: &notJSON}
	pipeline.overviewBuildMu.Lock()
	pipeline.runtime.mu.Lock()
	pipeline.runtime.lastSnapshot = snapshot
	pipeline.runtime.prevHashes = computeSnapshotHashes(snapshot)
	pipeline.runtime.overviewVersion = 93
	pipeline.runtime.mu.Unlock()
	pipeline.overviewBuildMu.Unlock()

	pipeline.SyncOverviewClient(client, ws.RuntimeSyncRequest{
		Cursor: ws.RuntimeCursor{StreamID: "obsolete-stream", Version: 92, Known: true},
		Reason: ws.ResyncReasonClientResync,
	})

	body := string(registry.MarshalPrometheus())
	assertPipelineMetric(t, body, `theia_ws_runtime_recovery_total{mode="snapshot",outcome="scheduled",reason="stream_mismatch"} 1`)
	assertPipelineMetric(t, body, `theia_ws_runtime_recovery_total{mode="snapshot",outcome="failed",reason="stream_mismatch"} 1`)
	assertPipelineMetric(t, body, `theia_ws_runtime_recovery_duration_seconds_count{mode="snapshot",outcome="failed"} 1`)
	if got := len(pipeline.runtimeRecoveryAttempts); got != 0 {
		t.Fatalf("runtime recovery attempts after install failure = %d, want 0", got)
	}
}

func TestRuntimeRecoveryTrackingPrunesExpiredAttemptBeforeLateAck(t *testing.T) {
	registry := observability.ResetDefaultForTest()
	hub := ws.NewHub()
	go hub.Run()
	pipeline := NewPipelineOrchestrator(
		newPipelineTestScheduler(), nil, nil, hub, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil,
	)
	now := time.Date(2026, time.July, 14, 12, 0, 0, 0, time.UTC)
	pipeline.runtime.now = func() time.Time { return now }
	configureRuntimeRecoveryState(t, pipeline, "runtime-stream-1", 92)
	_, client, release := attachRuntimeRecoveryClient(
		t, hub, pipeline.GetOverviewSnapshot, ws.RuntimeStreamProtocolVersion,
		ws.RuntimeCursor{StreamID: "runtime-stream-1", Version: 92, Known: true}, false,
	)
	defer release()

	pipeline.SyncOverviewClient(client, ws.RuntimeSyncRequest{
		Cursor: ws.RuntimeCursor{StreamID: "runtime-stream-1", Version: 92, Known: true},
		Reason: ws.ResyncReasonClientResync,
	})
	now = now.Add(pipeline.runtimeRecoveryTTL + time.Second)
	pipeline.ObserveRuntimeAck(client, ws.RuntimeCursor{
		StreamID: "runtime-stream-1",
		Version:  92,
		Known:    true,
	})

	body := string(registry.MarshalPrometheus())
	assertPipelineMetric(t, body, `theia_ws_runtime_recovery_total{mode="current",outcome="failed",reason="client_resync_scheduled"} 1`)
	if strings.Contains(body, `mode="current",outcome="completed"`) {
		t.Fatalf("expired recovery completed from a late ACK\n%s", body)
	}
	if got := len(pipeline.runtimeRecoveryAttempts); got != 0 {
		t.Fatalf("runtime recovery attempts after TTL pruning = %d, want 0", got)
	}
}

func TestRuntimeRecoveryTrackingExpiresWithoutLaterClientActivity(t *testing.T) {
	registry := observability.ResetDefaultForTest()
	hub := ws.NewHub()
	go hub.Run()
	pipeline := NewPipelineOrchestrator(
		newPipelineTestScheduler(), nil, nil, hub, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil,
	)
	pipeline.runtimeRecoveryTTL = 20 * time.Millisecond
	configureRuntimeRecoveryState(t, pipeline, "runtime-stream-1", 92)
	_, client, release := attachRuntimeRecoveryClient(
		t, hub, pipeline.GetOverviewSnapshot, ws.RuntimeStreamProtocolVersion,
		ws.RuntimeCursor{StreamID: "runtime-stream-1", Version: 92, Known: true}, false,
	)
	defer release()

	pipeline.SyncOverviewClient(client, ws.RuntimeSyncRequest{
		Cursor: ws.RuntimeCursor{StreamID: "runtime-stream-1", Version: 92, Known: true},
		Reason: ws.ResyncReasonClientResync,
	})
	deadline := time.Now().Add(time.Second)
	for {
		pipeline.overviewBuildMu.Lock()
		remaining := len(pipeline.runtimeRecoveryAttempts)
		pipeline.overviewBuildMu.Unlock()
		if remaining == 0 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("runtime recovery attempt did not expire without client activity: %d remain", remaining)
		}
		time.Sleep(time.Millisecond)
	}

	body := string(registry.MarshalPrometheus())
	assertPipelineMetric(t, body, `theia_ws_runtime_recovery_total{mode="current",outcome="failed",reason="client_resync_scheduled"} 1`)
	if strings.Contains(body, `mode="current",outcome="completed"`) {
		t.Fatalf("autonomously expired recovery completed\n%s", body)
	}
}

func TestRuntimeRecoveryTimerIgnoresStaleGenerationAfterReplacement(t *testing.T) {
	registry := observability.ResetDefaultForTest()
	pipeline := NewPipelineOrchestrator(
		newPipelineTestScheduler(), nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil,
	)
	configureRuntimeRecoveryState(t, pipeline, "runtime-stream-1", 42)
	client := &ws.Client{}
	batch := ws.OverviewSyncBatch{
		Mode:            ws.OverviewSyncModeCurrent,
		RuntimeStreamID: "runtime-stream-1",
		TargetVersion:   42,
	}

	pipeline.overviewBuildMu.Lock()
	pipeline.recordRuntimeRecoveryScheduledLocked(client, batch, ws.ResyncReasonClientResync, pipeline.clockNow())
	pipeline.armRuntimeRecoveryTimerLocked(pipeline.clockNow())
	staleGeneration := pipeline.runtimeRecoveryTimerGen
	pipeline.recordRuntimeRecoveryScheduledLocked(client, batch, ws.ResyncReasonClientResync, pipeline.clockNow())
	pipeline.armRuntimeRecoveryTimerLocked(pipeline.clockNow())
	currentGeneration := pipeline.runtimeRecoveryTimerGen
	pipeline.overviewBuildMu.Unlock()
	if staleGeneration == currentGeneration {
		t.Fatalf("replacement timer generation = %d, want newer than %d", currentGeneration, staleGeneration)
	}

	pipeline.expireRuntimeRecoveryAttempts(staleGeneration)
	pipeline.overviewBuildMu.Lock()
	attempt, ok := pipeline.runtimeRecoveryAttempts[client]
	pipeline.overviewBuildMu.Unlock()
	if !ok || attempt.targetVersion != 42 {
		t.Fatalf("stale timer removed replacement attempt: %#v, present=%t", attempt, ok)
	}

	pipeline.ObserveRuntimeAck(client, ws.RuntimeCursor{
		StreamID: "runtime-stream-1",
		Version:  42,
		Known:    true,
	})
	pipeline.overviewBuildMu.Lock()
	remaining := len(pipeline.runtimeRecoveryAttempts)
	timer := pipeline.runtimeRecoveryTimer
	pipeline.overviewBuildMu.Unlock()
	if remaining != 0 || timer != nil {
		t.Fatalf("completed replacement left attempts=%d timer=%v", remaining, timer)
	}
	body := string(registry.MarshalPrometheus())
	assertPipelineMetric(t, body, `theia_ws_runtime_recovery_total{mode="current",outcome="failed",reason="client_resync_scheduled"} 1`)
	assertPipelineMetric(t, body, `theia_ws_runtime_recovery_total{mode="current",outcome="completed",reason="client_resync_scheduled"} 1`)
}

func TestRuntimeRecoveryTrackingCapDoesNotEvictOnExistingClientReplacement(t *testing.T) {
	registry := observability.ResetDefaultForTest()
	pipeline := NewPipelineOrchestrator(
		newPipelineTestScheduler(), nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil,
	)
	now := time.Date(2026, time.July, 14, 12, 0, 0, 0, time.UTC)
	pipeline.runtime.now = func() time.Time { return now }
	clients := make([]*ws.Client, runtimeRecoveryAttemptLimit)
	for index := range clients {
		clients[index] = &ws.Client{}
		pipeline.runtimeRecoveryAttempts[clients[index]] = runtimeRecoveryAttempt{
			mode:          "current",
			reason:        ws.ResyncReasonClientResync,
			streamID:      "runtime-stream-1",
			targetVersion: 41,
			startedAt:     now.Add(time.Duration(index) * time.Nanosecond),
		}
	}

	pipeline.overviewBuildMu.Lock()
	pipeline.recordRuntimeRecoveryScheduledLocked(
		clients[len(clients)-1],
		ws.OverviewSyncBatch{
			Mode:            ws.OverviewSyncModeCurrent,
			RuntimeStreamID: "runtime-stream-1",
			TargetVersion:   42,
		},
		ws.ResyncReasonClientResync,
		now.Add(runtimeRecoveryAttemptLimit*time.Nanosecond),
	)
	pipeline.overviewBuildMu.Unlock()

	if got := len(pipeline.runtimeRecoveryAttempts); got != runtimeRecoveryAttemptLimit {
		t.Fatalf("runtime recovery attempts after replacement = %d, want cap %d", got, runtimeRecoveryAttemptLimit)
	}
	newClient := &ws.Client{}
	pipeline.overviewBuildMu.Lock()
	pipeline.recordRuntimeRecoveryScheduledLocked(
		newClient,
		ws.OverviewSyncBatch{
			Mode:            ws.OverviewSyncModeCurrent,
			RuntimeStreamID: "runtime-stream-1",
			TargetVersion:   43,
		},
		ws.ResyncReasonClientResync,
		now.Add(time.Second),
	)
	pipeline.overviewBuildMu.Unlock()
	if got := len(pipeline.runtimeRecoveryAttempts); got != runtimeRecoveryAttemptLimit {
		t.Fatalf("runtime recovery attempts after capped insert = %d, want %d", got, runtimeRecoveryAttemptLimit)
	}
	if _, ok := pipeline.runtimeRecoveryAttempts[clients[0]]; ok {
		t.Fatal("oldest runtime recovery attempt survived capped insert")
	}
	if _, ok := pipeline.runtimeRecoveryAttempts[newClient]; !ok {
		t.Fatal("new runtime recovery attempt missing after capped insert")
	}
	body := string(registry.MarshalPrometheus())
	assertPipelineMetric(t, body, `theia_ws_runtime_recovery_total{mode="current",outcome="failed",reason="client_resync_scheduled"} 2`)
}

func TestRuntimeRecoveryHTTPFallbackMetricsTrackGETHEADAndFailure(t *testing.T) {
	registry := observability.ResetDefaultForTest()
	handler := theiaapi.NewRuntimeOverviewHandler(func() ws.RuntimeOverviewState {
		return ws.RuntimeOverviewState{
			Snapshot: ws.EmptySnapshot(),
			StreamID: "runtime-stream-1",
			Version:  42,
		}
	})

	for _, method := range []string{http.MethodGet, http.MethodHead} {
		request := httptest.NewRequest(method, "/api/v1/runtime/overview", nil)
		response := httptest.NewRecorder()
		handler.Handle(response, request)
		if response.Code != http.StatusOK {
			t.Fatalf("%s status = %d, want 200", method, response.Code)
		}
	}

	unavailable := theiaapi.NewRuntimeOverviewHandler(nil)
	unavailableResponse := httptest.NewRecorder()
	unavailable.Handle(
		unavailableResponse,
		httptest.NewRequest(http.MethodGet, "/api/v1/runtime/overview", nil),
	)
	if unavailableResponse.Code != http.StatusServiceUnavailable {
		t.Fatalf("unavailable GET status = %d, want 503", unavailableResponse.Code)
	}

	unsupportedResponse := httptest.NewRecorder()
	handler.Handle(
		unsupportedResponse,
		httptest.NewRequest(http.MethodPost, "/api/v1/runtime/overview", nil),
	)
	if unsupportedResponse.Code != http.StatusMethodNotAllowed {
		t.Fatalf("POST status = %d, want 405", unsupportedResponse.Code)
	}
	encodingFailure := &runtimeRecoveryFailingResponseWriter{header: make(http.Header)}
	handler.Handle(
		encodingFailure,
		httptest.NewRequest(http.MethodGet, "/api/v1/runtime/overview", nil),
	)

	body := string(registry.MarshalPrometheus())
	assertPipelineMetric(t, body, `theia_ws_runtime_recovery_total{mode="http_fallback",outcome="scheduled",reason="timeout"} 4`)
	assertPipelineMetric(t, body, `theia_ws_runtime_recovery_total{mode="http_fallback",outcome="completed",reason="timeout"} 2`)
	assertPipelineMetric(t, body, `theia_ws_runtime_recovery_total{mode="http_fallback",outcome="failed",reason="timeout"} 2`)
	assertPipelineMetric(t, body, `theia_ws_runtime_recovery_duration_seconds_count{mode="http_fallback",outcome="completed"} 2`)
	assertPipelineMetric(t, body, `theia_ws_runtime_recovery_duration_seconds_count{mode="http_fallback",outcome="failed"} 2`)
}

func TestRuntimeRecoveryMetricsTrackBulkSnapshotInstallAndFailure(t *testing.T) {
	registry := observability.ResetDefaultForTest()
	hub := ws.NewHub()
	go hub.Run()
	pipeline := NewPipelineOrchestrator(
		newPipelineTestScheduler(), nil, nil, hub, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil,
	)
	configureRuntimeRecoveryState(t, pipeline, "runtime-stream-1", 92)
	conn, client, release := attachRuntimeRecoveryClient(
		t, hub, pipeline.GetOverviewSnapshot, ws.RuntimeStreamProtocolVersion,
		ws.RuntimeCursor{StreamID: "runtime-stream-1", Version: 92, Known: true}, false,
	)
	defer release()

	state := pipeline.GetOrBuildOverviewState()
	pipeline.overviewBuildMu.Lock()
	pipeline.replaceOverviewClientsWithSnapshotLocked(state, ws.ResyncReasonStateChangesDrop)
	pipeline.overviewBuildMu.Unlock()
	assertRuntimeRecoveryMessageTypes(
		t,
		readRuntimeRecoveryMessages(t, conn, 3),
		ws.MessageTypeResyncRequired,
		ws.MessageTypeSnapshot,
		ws.MessageTypeReady,
	)
	body := string(registry.MarshalPrometheus())
	assertPipelineMetric(t, body, `theia_ws_runtime_recovery_total{mode="snapshot",outcome="scheduled",reason="state_changes_dropped"} 1`)
	pipeline.ObserveRuntimeAck(client, ws.RuntimeCursor{
		StreamID: "runtime-stream-1",
		Version:  92,
		Known:    true,
	})
	body = string(registry.MarshalPrometheus())
	assertPipelineMetric(t, body, `theia_ws_runtime_recovery_total{mode="snapshot",outcome="completed",reason="state_changes_dropped"} 1`)
	pipeline.SyncOverviewClient(client, ws.RuntimeSyncRequest{
		Cursor: ws.RuntimeCursor{StreamID: "runtime-stream-1", Version: 92, Known: true},
		Reason: ws.ResyncReasonClientResync,
	})
	assertRuntimeRecoveryMessageTypes(t, readRuntimeRecoveryMessages(t, conn, 1), ws.MessageTypeReady)

	notJSON := math.NaN()
	failedSnapshot := ws.EmptySnapshot()
	failedSnapshot.Devices["dev-1"] = ws.DeviceRuntimeDTO{DeviceID: "dev-1", CPUPercent: &notJSON}
	pipeline.overviewBuildMu.Lock()
	pipeline.replaceOverviewClientsWithSnapshotLocked(ws.RuntimeOverviewState{
		Snapshot: failedSnapshot,
		StreamID: "runtime-stream-1",
		Version:  93,
	}, ws.ResyncReasonHubBufferFull)
	pipeline.overviewBuildMu.Unlock()

	body = string(registry.MarshalPrometheus())
	assertPipelineMetric(t, body, `theia_ws_runtime_recovery_total{mode="snapshot",outcome="scheduled",reason="hub_buffer_full"} 1`)
	assertPipelineMetric(t, body, `theia_ws_runtime_recovery_total{mode="snapshot",outcome="failed",reason="hub_buffer_full"} 1`)
	assertPipelineMetric(t, body, `theia_ws_runtime_recovery_total{mode="current",outcome="failed",reason="client_resync_scheduled"} 1`)
	pipeline.overviewBuildMu.Lock()
	remainingAttempts := len(pipeline.runtimeRecoveryAttempts)
	pipeline.overviewBuildMu.Unlock()
	if got := remainingAttempts; got != 0 {
		t.Fatalf("runtime recovery attempts after failed bulk install = %d, want 0", got)
	}
}

type runtimeRecoveryFailingResponseWriter struct {
	header http.Header
}

func (w *runtimeRecoveryFailingResponseWriter) Header() http.Header {
	return w.header
}

func (*runtimeRecoveryFailingResponseWriter) Write([]byte) (int, error) {
	return 0, errors.New("response write failed")
}

func (*runtimeRecoveryFailingResponseWriter) WriteHeader(int) {}

func assertPipelineMetric(t *testing.T, body, needle string) {
	t.Helper()
	if !strings.Contains(body, needle) {
		t.Fatalf("metrics output missing %q\n%s", needle, body)
	}
}

type runtimeRecoveryWireMessage struct {
	Type    string          `json:"type"`
	Payload json.RawMessage `json:"payload"`
}

func configureRuntimeRecoveryState(t *testing.T, pipeline *PipelineOrchestrator, streamID string, version uint64) {
	t.Helper()
	snapshot := ws.EmptySnapshot()
	snapshot.Devices["dev-1"] = ws.DeviceRuntimeDTO{DeviceID: "dev-1", PrimaryHealth: "up_fresh"}

	pipeline.overviewBuildMu.Lock()
	pipeline.runtime.mu.Lock()
	pipeline.runtime.lastSnapshot = snapshot
	pipeline.runtime.prevHashes = computeSnapshotHashes(snapshot)
	pipeline.runtime.overviewVersion = version
	pipeline.runtime.overviewStreamID = streamID
	pipeline.runtime.mu.Unlock()
	pipeline.runtime.overviewJournal.Reset()
	pipeline.overviewBuildMu.Unlock()
}

func advanceRuntimeRecoveryState(t *testing.T, pipeline *PipelineOrchestrator, baseVersion, version uint64, delta *ws.RuntimeDeltaPayload) {
	t.Helper()
	pipeline.overviewBuildMu.Lock()
	pipeline.runtime.mu.Lock()
	actualVersion := pipeline.runtime.overviewVersion
	if actualVersion != baseVersion {
		pipeline.runtime.mu.Unlock()
		pipeline.overviewBuildMu.Unlock()
		t.Fatalf("runtime recovery base = %d, want %d", actualVersion, baseVersion)
	}
	streamID := pipeline.runtime.overviewStreamID
	pipeline.runtime.overviewVersion = version
	pipeline.runtime.mu.Unlock()
	pipeline.runtime.overviewJournal.Append(baseVersion, version, delta)
	overflowed := pipeline.hub.BroadcastOverviewStreamDelta(delta, baseVersion, version, streamID)
	pipeline.overviewBuildMu.Unlock()
	if len(overflowed) != 0 {
		t.Fatalf("runtime recovery delta %d->%d overflowed %d clients", baseVersion, version, len(overflowed))
	}
}

func attachRuntimeRecoveryClient(
	t *testing.T,
	hub *ws.Hub,
	snapshotFunc func() (*ws.SnapshotPayload, uint64),
	runtimeProtocol int,
	cursor ws.RuntimeCursor,
	holdWritePump bool,
) (*websocket.Conn, *ws.Client, func()) {
	t.Helper()

	probeDeviceID := uuid.New()
	gate := make(chan struct{})
	gateEntered := make(chan struct{})
	var releaseOnce sync.Once
	release := func() {
		releaseOnce.Do(func() {
			close(gate)
		})
	}
	if !holdWritePump {
		release()
	}

	handler := ws.NewHandler(
		hub,
		snapshotFunc,
		func() ws.AlertMessagePayload { return ws.AlertMessagePayload{Alerts: []ws.AlertDTO{}} },
		func() ws.PrometheusStatusPayload {
			close(gateEntered)
			<-gate
			return ws.PrometheusStatusPayload{}
		},
	)
	server := httptest.NewServer(handler)
	t.Cleanup(func() {
		release()
		server.Close()
	})

	conn, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(server.URL, "http"), nil)
	if err != nil {
		t.Fatalf("dial runtime recovery client: %v", err)
	}
	t.Cleanup(func() {
		_ = conn.Close()
	})
	if err := conn.WriteJSON(map[string]any{
		"type": ws.MessageTypeHello,
		"payload": map[string]any{
			"runtime_protocol":  runtimeProtocol,
			"runtime_stream_id": cursor.StreamID,
			"runtime_version":   cursor.Version,
		},
	}); err != nil {
		t.Fatalf("send runtime recovery hello: %v", err)
	}
	if err := conn.WriteJSON(map[string]any{
		"type": ws.MessageTypeSubscribeDetail,
		"payload": map[string]any{
			"device_id": probeDeviceID.String(),
		},
	}); err != nil {
		t.Fatalf("send runtime recovery probe subscription: %v", err)
	}

	bootstrap := readRuntimeRecoveryMessages(t, conn, 2)
	assertRuntimeRecoveryMessageTypes(t, bootstrap, ws.MessageTypeReady, ws.MessageTypeAlert)
	select {
	case <-gateEntered:
	case <-time.After(time.Second):
		t.Fatal("runtime recovery handler did not reach write-pump gate")
	}

	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		clients := hub.DetailSubscribers(probeDeviceID)
		if len(clients) == 1 {
			return conn, clients[0], release
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("runtime recovery client was not registered")
	return nil, nil, release
}

func readRuntimeRecoveryMessages(t *testing.T, conn *websocket.Conn, count int) []runtimeRecoveryWireMessage {
	t.Helper()
	if err := conn.SetReadDeadline(time.Now().Add(time.Second)); err != nil {
		t.Fatalf("set runtime recovery read deadline: %v", err)
	}
	defer conn.SetReadDeadline(time.Time{})

	messages := make([]runtimeRecoveryWireMessage, 0, count)
	for index := 0; index < count; index++ {
		var message runtimeRecoveryWireMessage
		if err := conn.ReadJSON(&message); err != nil {
			t.Fatalf("read runtime recovery message %d/%d: %v", index+1, count, err)
		}
		messages = append(messages, message)
	}
	return messages
}

func decodeRuntimeRecoveryPayload(t *testing.T, message runtimeRecoveryWireMessage, target any) {
	t.Helper()
	if err := json.Unmarshal(message.Payload, target); err != nil {
		t.Fatalf("decode %s payload: %v", message.Type, err)
	}
}

func assertRuntimeRecoveryMessageTypes(t *testing.T, messages []runtimeRecoveryWireMessage, want ...string) {
	t.Helper()
	if len(messages) != len(want) {
		t.Fatalf("runtime recovery message count = %d, want %d", len(messages), len(want))
	}
	for index := range want {
		if messages[index].Type != want[index] {
			t.Fatalf("runtime recovery message %d type = %q, want %q", index, messages[index].Type, want[index])
		}
	}
}

func TestPipelineOrchestratorBroadcastDirty_AlertResolutionWithoutRuntimeBaseFallsBackToFullSnapshotAndAlert(t *testing.T) {
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
	if len(types) != 2 || types[0] != ws.MessageTypeSnapshot || types[1] != ws.MessageTypeAlert {
		t.Fatalf("expected alert resolution without runtime base to broadcast snapshot then alert, got %v", types)
	}

	var snapshotMessage wsVersionedSnapshotMessage
	if err := json.Unmarshal(messages[0], &snapshotMessage); err != nil {
		t.Fatalf("decode alert resolution snapshot: %v", err)
	}
	if snapshotMessage.Payload.Snapshot == nil {
		t.Fatal("expected alert resolution fallback snapshot payload")
	}
	deviceRuntime, ok := snapshotMessage.Payload.Snapshot.Devices[deviceID.String()]
	if !ok {
		t.Fatalf("expected alert resolution snapshot for device %s", deviceID)
	}
	if deviceRuntime.AlertStatus != string(domain.AlertStatusNormal) {
		t.Fatalf("alert status = %q, want %q", deviceRuntime.AlertStatus, domain.AlertStatusNormal)
	}
	if deviceRuntime.FiringAlertCount != 0 {
		t.Fatalf("firing alert count = %d, want 0", deviceRuntime.FiringAlertCount)
	}
	if fullSnapshotCalls != 1 {
		t.Fatalf("full state snapshot calls = %d, want 1", fullSnapshotCalls)
	}
	if narrowSnapshotCalls != 0 {
		t.Fatalf("narrow state snapshot calls = %d, want 0", narrowSnapshotCalls)
	}
	if len(requestedIDs) != 0 {
		t.Fatalf("narrow requested IDs = %v, want none", requestedIDs)
	}
}

func TestPipelineOrchestratorBroadcastDirty_AlertOnlyWithStaleBaseUsesFullSnapshot(t *testing.T) {
	pipeline, hub, _, _, deviceID := newNoClientBroadcastTestPipeline(t)

	pipeline.broadcaster.broadcastOnce(context.Background())
	pipeline.runtime.setAlerts(map[uuid.UUID][]domain.AlertState{
		deviceID: {{
			DeviceID:  deviceID,
			Severity:  "critical",
			AlertName: "DeviceDown",
			State:     "firing",
			Summary:   "device down",
		}},
	})
	if err := pipeline.broadcaster.broadcastDirty(context.Background(), map[uuid.UUID]struct{}{deviceID: {}}, false, false, false); err != nil {
		t.Fatalf("broadcastDirty no-client dirty returned error: %v", err)
	}
	pipeline.runtime.mu.RLock()
	stale := pipeline.runtime.prevHashes == nil
	pipeline.runtime.mu.RUnlock()
	if !stale {
		t.Fatal("expected no-client dirty work to clear runtime hash base")
	}

	attachOverviewBroadcastTestClient(t, hub)
	drainBroadcastCh(hub)

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

	if err := pipeline.broadcaster.broadcastDirty(context.Background(), nil, true, false, false); err != nil {
		t.Fatalf("broadcastDirty alert-only stale base returned error: %v", err)
	}

	if fullSnapshotCalls != 1 {
		t.Fatalf("full state snapshot calls = %d, want 1", fullSnapshotCalls)
	}
	if narrowSnapshotCalls != 0 {
		t.Fatalf("narrow state snapshot calls = %d, want 0", narrowSnapshotCalls)
	}
	types := broadcastMessageTypes(t, drainBroadcastCh(hub))
	if len(types) != 2 || types[0] != ws.MessageTypeSnapshot || types[1] != ws.MessageTypeAlert {
		t.Fatalf("expected alert-only stale base to broadcast snapshot then alert, got %v", types)
	}
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
	attachOverviewBroadcastTestClient(t, hub)

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

type pipelineSnapshotHookCounters struct {
	fullCalls   int
	narrowCalls int
}

func installPipelineSnapshotHookCounters(t *testing.T) *pipelineSnapshotHookCounters {
	t.Helper()

	previousSnapshotAll := snapshotAllPipelineState
	previousSnapshotFor := snapshotPipelineStateFor
	counters := &pipelineSnapshotHookCounters{}
	snapshotAllPipelineState = func(store *state.Store) map[uuid.UUID]state.DeviceState {
		counters.fullCalls++
		return store.Snapshot()
	}
	snapshotPipelineStateFor = func(store *state.Store, ids []uuid.UUID) map[uuid.UUID]state.DeviceState {
		counters.narrowCalls++
		return store.SnapshotFor(ids)
	}
	t.Cleanup(func() {
		snapshotAllPipelineState = previousSnapshotAll
		snapshotPipelineStateFor = previousSnapshotFor
	})
	return counters
}

func waitForBuildEntrants(t *testing.T, buildEntered <-chan struct{}, want int, timeout time.Duration) {
	t.Helper()

	for i := 0; i < want; i++ {
		select {
		case <-buildEntered:
		case <-time.After(timeout):
			t.Fatalf("timed out waiting for full snapshot build entrant %d/%d", i+1, want)
		}
	}
}

func waitForBuildEntrantsOrTimeout(buildEntered <-chan struct{}, want int, timeout time.Duration) int {
	got := 0
	timer := time.NewTimer(timeout)
	defer timer.Stop()

	for got < want {
		select {
		case <-buildEntered:
			got++
		case <-timer.C:
			return got
		}
	}
	return got
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
