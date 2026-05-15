package service

import (
	"bytes"
	"database/sql"
	"errors"
	"log"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/lollinoo/theia/internal/domain"
	"github.com/lollinoo/theia/internal/observability"
	sqliterepo "github.com/lollinoo/theia/internal/repository/sqlite"
	"github.com/lollinoo/theia/internal/snmp"
	"github.com/lollinoo/theia/internal/topology"
	_ "github.com/mattn/go-sqlite3"
)

func captureLogs(t *testing.T, fn func()) string {
	t.Helper()

	var buffer bytes.Buffer
	previousWriter := log.Writer()
	previousFlags := log.Flags()
	log.SetOutput(&buffer)
	log.SetFlags(0)
	defer log.SetOutput(previousWriter)
	defer log.SetFlags(previousFlags)

	fn()
	return buffer.String()
}

func newStaticPersistenceService(topologyNotify chan struct{}) (*DeviceService, *mockDeviceRepo, *mockLinkRepo) {
	deviceRepo := newMockDeviceRepo()
	linkRepo := newMockLinkRepo()
	settingsRepo := newMockSettingsRepo()

	discoverFn := func(target string, creds domain.SNMPCredentials, _ domain.TopologyDiscoveryMode) (*snmp.DiscoveryResult, error) {
		return nil, nil
	}

	return NewDeviceService(deviceRepo, linkRepo, settingsRepo, discoverFn, topologyNotify), deviceRepo, linkRepo
}

func TestApplyStaticDiscoveryUpdatesPersistedDeviceMetadataAndInterfaces(t *testing.T) {
	topologyNotify := make(chan struct{}, 1)
	svc, deviceRepo, _ := newStaticPersistenceService(topologyNotify)

	device := &domain.Device{
		ID:         uuid.New(),
		Hostname:   "edge-1",
		IP:         "192.0.2.10",
		Status:     domain.DeviceStatusProbing,
		Vendor:     "default",
		DeviceType: domain.DeviceTypeUnknown,
		PollClass:  domain.PollClassStandard,
		Interfaces: []domain.Interface{
			{IfIndex: 1, IfName: "ether1", IfDescr: "old-uplink", Speed: 100_000_000},
		},
	}
	if err := deviceRepo.Create(device); err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	persisted, err := svc.ApplyStaticDiscovery(device.ID, StaticDiscoveryInput{
		SysName:       "edge-sw-1",
		SysDescr:      "MikroTik CRS",
		SysObjectID:   ".1.3.6.1.4.1.14988.1",
		HardwareModel: "CRS326-24G-2S+",
		OSVersion:     "7.22.1",
		Vendor:        "mikrotik",
		DeviceType:    domain.DeviceTypeSwitch,
		Interfaces: []domain.Interface{
			{IfIndex: 1, IfName: "ether1", IfDescr: "uplink", Speed: 1_000_000_000},
			{IfIndex: 2, IfName: "ether2", IfDescr: "server", Speed: 1_000_000_000},
		},
	})
	if err != nil {
		t.Fatalf("ApplyStaticDiscovery failed: %v", err)
	}

	if !persisted.TopologyChanged {
		t.Fatal("expected TopologyChanged when interfaces change")
	}
	if persisted.LinksCreated != 0 {
		t.Fatalf("expected no links created, got %d", persisted.LinksCreated)
	}

	updated, err := deviceRepo.GetByID(device.ID)
	if err != nil {
		t.Fatalf("GetByID failed: %v", err)
	}
	if updated.Status != domain.DeviceStatusProbing {
		t.Fatalf("expected status probing to remain caller-owned, got %s", updated.Status)
	}
	if updated.SysName != "edge-sw-1" {
		t.Fatalf("expected SysName edge-sw-1, got %q", updated.SysName)
	}
	if updated.SysDescr != "MikroTik CRS" {
		t.Fatalf("expected SysDescr updated, got %q", updated.SysDescr)
	}
	if updated.SysObjectID != ".1.3.6.1.4.1.14988.1" {
		t.Fatalf("expected SysObjectID updated, got %q", updated.SysObjectID)
	}
	if updated.HardwareModel != "CRS326-24G-2S+" {
		t.Fatalf("expected HardwareModel updated, got %q", updated.HardwareModel)
	}
	if updated.OSVersion != "7.22.1" {
		t.Fatalf("expected OSVersion updated, got %q", updated.OSVersion)
	}
	if updated.Vendor != "mikrotik" {
		t.Fatalf("expected Vendor mikrotik, got %q", updated.Vendor)
	}
	if updated.DeviceType != domain.DeviceTypeSwitch {
		t.Fatalf("expected DeviceType switch, got %s", updated.DeviceType)
	}
	if updated.PollClass != domain.PollClassCore {
		t.Fatalf("expected PollClass core, got %s", updated.PollClass)
	}
	if len(updated.Interfaces) != 2 {
		t.Fatalf("expected 2 interfaces, got %d", len(updated.Interfaces))
	}
	if updated.Interfaces[0].IfDescr != "uplink" {
		t.Fatalf("expected first interface description updated, got %q", updated.Interfaces[0].IfDescr)
	}

	select {
	case <-topologyNotify:
		t.Fatal("ApplyStaticDiscovery should not write topology notifications")
	default:
	}
}

func TestApplyStaticDiscoveryCreatesLinksAndReturnsTopologyChanged(t *testing.T) {
	topologyNotify := make(chan struct{}, 1)
	svc, deviceRepo, linkRepo := newStaticPersistenceService(topologyNotify)

	device := &domain.Device{
		ID:         uuid.New(),
		Hostname:   "switch1",
		IP:         "192.0.2.11",
		SysName:    "switch1",
		Status:     domain.DeviceStatusUp,
		Vendor:     "mikrotik",
		DeviceType: domain.DeviceTypeSwitch,
		PollClass:  domain.PollClassCore,
		Interfaces: []domain.Interface{
			{IfIndex: 1, IfName: "ether1", IfDescr: "uplink", Speed: 1_000_000_000},
		},
	}
	if err := deviceRepo.Create(device); err != nil {
		t.Fatalf("Create device failed: %v", err)
	}

	remote := &domain.Device{
		ID:       uuid.New(),
		Hostname: "switch2",
		IP:       "192.0.2.12",
		SysName:  "switch2",
		Status:   domain.DeviceStatusUp,
	}
	if err := deviceRepo.Create(remote); err != nil {
		t.Fatalf("Create remote device failed: %v", err)
	}

	persisted, err := svc.ApplyStaticDiscovery(device.ID, StaticDiscoveryInput{
		SysName:    "switch1",
		DeviceType: domain.DeviceTypeSwitch,
		Interfaces: []domain.Interface{
			{IfIndex: 1, IfName: "ether1", IfDescr: "uplink", Speed: 1_000_000_000},
		},
		Neighbors: []snmp.NeighborInfo{
			{
				RemoteSysName:   "switch2",
				RemotePortID:    "ether2",
				LocalIfIndex:    1,
				LocalIfName:     "ether1",
				Protocol:        domain.DiscoveryProtocolLLDP,
				RemoteChassisID: "aa:bb:cc:dd:ee:ff",
			},
			{
				RemoteSysName:   "switch2",
				RemotePortID:    "ether2",
				LocalIfIndex:    1,
				LocalIfName:     "",
				Protocol:        domain.DiscoveryProtocolLLDP,
				RemoteChassisID: "aa:bb:cc:dd:ee:ff",
			},
		},
	})
	if err != nil {
		t.Fatalf("ApplyStaticDiscovery failed: %v", err)
	}

	if !persisted.TopologyChanged {
		t.Fatal("expected TopologyChanged when a new link is created")
	}
	if persisted.LinksCreated != 1 {
		t.Fatalf("expected 1 link created, got %d", persisted.LinksCreated)
	}

	links, err := linkRepo.GetAll()
	if err != nil {
		t.Fatalf("GetAll links failed: %v", err)
	}
	if len(links) != 1 {
		t.Fatalf("expected 1 persisted link, got %d", len(links))
	}
	if links[0].SourceIfName != "ether1" {
		t.Fatalf("expected physical interface link, got %q", links[0].SourceIfName)
	}
	if links[0].TargetIfName != "ether2" {
		t.Fatalf("expected target interface ether2, got %q", links[0].TargetIfName)
	}
	if links[0].DiscoveryProtocol != domain.DiscoveryProtocolLLDP {
		t.Fatalf("expected LLDP discovery protocol, got %s", links[0].DiscoveryProtocol)
	}

	select {
	case <-topologyNotify:
		t.Fatal("ApplyStaticDiscovery should not write topology notifications")
	default:
	}
}

func TestApplyStaticDiscoveryMarksTopologyChangedWhenExistingLinkIsEnriched(t *testing.T) {
	svc, deviceRepo, linkRepo := newStaticPersistenceService(nil)

	device := &domain.Device{
		ID:         uuid.New(),
		Hostname:   "switch1",
		IP:         "192.0.2.21",
		SysName:    "switch1",
		Status:     domain.DeviceStatusUp,
		Vendor:     "mikrotik",
		DeviceType: domain.DeviceTypeSwitch,
		PollClass:  domain.PollClassCore,
	}
	if err := deviceRepo.Create(device); err != nil {
		t.Fatalf("Create device failed: %v", err)
	}

	remote := &domain.Device{
		ID:       uuid.New(),
		Hostname: "switch2",
		IP:       "192.0.2.22",
		SysName:  "switch2",
		Status:   domain.DeviceStatusUp,
	}
	if err := deviceRepo.Create(remote); err != nil {
		t.Fatalf("Create remote device failed: %v", err)
	}

	if err := linkRepo.Create(&domain.Link{
		SourceDeviceID:    device.ID,
		SourceIfName:      "",
		TargetDeviceID:    remote.ID,
		TargetIfName:      "ether2",
		DiscoveryProtocol: domain.DiscoveryProtocolLLDP,
	}); err != nil {
		t.Fatalf("Create incomplete link failed: %v", err)
	}

	persisted, err := svc.ApplyStaticDiscovery(device.ID, StaticDiscoveryInput{
		SysName:    "switch1",
		DeviceType: domain.DeviceTypeSwitch,
		Neighbors: []snmp.NeighborInfo{
			{
				RemoteSysName:   "switch2",
				RemotePortID:    "ether2",
				LocalIfName:     "ether1",
				Protocol:        domain.DiscoveryProtocolLLDP,
				RemoteChassisID: "aa:bb:cc:dd:ee:ff",
			},
		},
	})
	if err != nil {
		t.Fatalf("ApplyStaticDiscovery failed: %v", err)
	}

	if !persisted.TopologyChanged {
		t.Fatal("expected TopologyChanged when an existing link is enriched")
	}
	if persisted.LinksCreated != 0 {
		t.Fatalf("expected no new links created, got %d", persisted.LinksCreated)
	}
}

func TestApplyStaticDiscovery_ReverseEnrichmentCompletesPeerBootstrapState(t *testing.T) {
	db, err := sql.Open("sqlite3", ":memory:?_foreign_keys=on")
	if err != nil {
		t.Fatalf("opening sqlite db: %v", err)
	}
	defer db.Close()
	if err := sqliterepo.RunMigrations(db); err != nil {
		t.Fatalf("RunMigrations failed: %v", err)
	}

	deviceRepo := sqliterepo.NewDeviceRepo(db, nil, nil)
	linkRepo := sqliterepo.NewLinkRepo(db, nil)
	settingsRepo := newMockSettingsRepo()
	svc := NewDeviceService(
		deviceRepo,
		linkRepo,
		settingsRepo,
		func(target string, creds domain.SNMPCredentials, _ domain.TopologyDiscoveryMode) (*snmp.DiscoveryResult, error) {
			return nil, nil
		},
		nil,
	)

	local := &domain.Device{
		ID:                          uuid.New(),
		Hostname:                    "switch1",
		IP:                          "192.0.2.31",
		SysName:                     "switch1",
		Status:                      domain.DeviceStatusUp,
		Managed:                     true,
		DeviceType:                  domain.DeviceTypeSwitch,
		MetricsSource:               domain.MetricsSourceSNMP,
		TopologyDiscoveryMode:       domain.TopologyDiscoveryModeBootstrapOnce,
		TopologyBootstrapState:      domain.TopologyBootstrapStateFollowupScheduled,
		LastTopologyDiscoveryResult: "ports_pending",
	}
	remote := &domain.Device{
		ID:                    uuid.New(),
		Hostname:              "switch2",
		IP:                    "192.0.2.32",
		SysName:               "switch2",
		Status:                domain.DeviceStatusUp,
		Managed:               true,
		DeviceType:            domain.DeviceTypeSwitch,
		MetricsSource:         domain.MetricsSourceSNMP,
		TopologyDiscoveryMode: domain.TopologyDiscoveryModeBootstrapOnce,
	}
	if err := deviceRepo.Create(local); err != nil {
		t.Fatalf("Create local failed: %v", err)
	}
	if err := deviceRepo.Create(remote); err != nil {
		t.Fatalf("Create remote failed: %v", err)
	}

	if err := linkRepo.Create(&domain.Link{
		SourceDeviceID:    local.ID,
		SourceIfName:      "",
		TargetDeviceID:    remote.ID,
		TargetIfName:      "ether10",
		DiscoveryProtocol: domain.DiscoveryProtocolLLDP,
	}); err != nil {
		t.Fatalf("Create incomplete link failed: %v", err)
	}

	persisted, err := svc.ApplyStaticDiscovery(remote.ID, StaticDiscoveryInput{
		SysName:    remote.SysName,
		DeviceType: domain.DeviceTypeSwitch,
		Neighbors: []snmp.NeighborInfo{
			{
				RemoteSysName: local.SysName,
				LocalIfName:   "",
				RemotePortID:  "ether6-Link_Ufficio",
				Protocol:      domain.DiscoveryProtocolLLDP,
			},
		},
	})
	if err != nil {
		t.Fatalf("ApplyStaticDiscovery failed: %v", err)
	}
	if !persisted.TopologyChanged {
		t.Fatal("expected reverse enrichment to report topology change")
	}

	updatedLocal, err := deviceRepo.GetByID(local.ID)
	if err != nil {
		t.Fatalf("GetByID local failed: %v", err)
	}
	if updatedLocal.TopologyBootstrapState != domain.TopologyBootstrapStateCompleted {
		t.Fatalf("expected local bootstrap state completed, got %s", updatedLocal.TopologyBootstrapState)
	}
	if updatedLocal.LastTopologyDiscoveryResult != "neighbors_found" {
		t.Fatalf("expected local last_topology_discovery_result neighbors_found, got %q", updatedLocal.LastTopologyDiscoveryResult)
	}

	links, err := linkRepo.GetAll()
	if err != nil {
		t.Fatalf("GetAll links failed: %v", err)
	}
	if len(links) != 1 {
		t.Fatalf("expected 1 link, got %d", len(links))
	}
	if links[0].SourceIfName != "ether6-Link_Ufficio" {
		t.Fatalf("expected source interface to be backfilled, got %q", links[0].SourceIfName)
	}
	if links[0].TargetIfName != "ether10" {
		t.Fatalf("expected target interface ether10, got %q", links[0].TargetIfName)
	}
}

func TestApplyStaticDiscoveryRespectsPollIntervalOverride(t *testing.T) {
	svc, deviceRepo, _ := newStaticPersistenceService(nil)

	device := &domain.Device{
		ID:                   uuid.New(),
		Hostname:             "router1",
		IP:                   "192.0.2.13",
		Status:               domain.DeviceStatusProbing,
		DeviceType:           domain.DeviceTypeUnknown,
		PollClass:            domain.PollClassStandard,
		PollIntervalOverride: intPtr(15),
	}
	if err := deviceRepo.Create(device); err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	persisted, err := svc.ApplyStaticDiscovery(device.ID, StaticDiscoveryInput{
		SysName:    "router1",
		DeviceType: domain.DeviceTypeRouter,
	})
	if err != nil {
		t.Fatalf("ApplyStaticDiscovery failed: %v", err)
	}

	if persisted.TopologyChanged {
		t.Fatal("expected TopologyChanged false without interface or link changes")
	}
	if persisted.LinksCreated != 0 {
		t.Fatalf("expected 0 links created, got %d", persisted.LinksCreated)
	}

	updated, err := deviceRepo.GetByID(device.ID)
	if err != nil {
		t.Fatalf("GetByID failed: %v", err)
	}
	if updated.DeviceType != domain.DeviceTypeRouter {
		t.Fatalf("expected DeviceType router, got %s", updated.DeviceType)
	}
	if updated.PollClass != domain.PollClassStandard {
		t.Fatalf("expected PollClass standard to stay caller override-owned, got %s", updated.PollClass)
	}
	if updated.PollIntervalOverride == nil || *updated.PollIntervalOverride != 15 {
		t.Fatalf("expected PollIntervalOverride 15, got %v", updated.PollIntervalOverride)
	}
	if updated.Status != domain.DeviceStatusProbing {
		t.Fatalf("expected helper not to change status, got %s", updated.Status)
	}
}

func TestProbeDeviceUsesApplyStaticDiscoveryAndSignalsTopologyNotify(t *testing.T) {
	topologyNotify := make(chan struct{}, 2)
	deviceRepo := newMockDeviceRepo()
	linkRepo := newMockLinkRepo()
	settingsRepo := newMockSettingsRepo()

	probeResult := &snmp.DiscoveryResult{
		SysName:       "probe-switch",
		SysDescr:      "MikroTik CRS",
		SysObjectID:   ".1.3.6.1.4.1.14988.1",
		HardwareModel: "CRS326-24G-2S+",
		Vendor:        "mikrotik",
		DeviceType:    domain.DeviceTypeSwitch,
		Interfaces: []domain.Interface{
			{IfIndex: 1, IfName: "ether1", IfDescr: "uplink", Speed: 1_000_000_000},
		},
	}

	discoverFn := func(target string, creds domain.SNMPCredentials, _ domain.TopologyDiscoveryMode) (*snmp.DiscoveryResult, error) {
		return probeResult, nil
	}

	svc := NewDeviceService(deviceRepo, linkRepo, settingsRepo, discoverFn, topologyNotify)

	helperDevice := &domain.Device{
		ID:         uuid.New(),
		Hostname:   "helper-switch",
		IP:         "192.0.2.20",
		Status:     domain.DeviceStatusProbing,
		Vendor:     "default",
		DeviceType: domain.DeviceTypeUnknown,
		PollClass:  domain.PollClassStandard,
		Interfaces: []domain.Interface{
			{IfIndex: 1, IfName: "ether10", IfDescr: "old-distribution", Speed: 100_000_000},
		},
	}
	if err := deviceRepo.Create(helperDevice); err != nil {
		t.Fatalf("Create helper device failed: %v", err)
	}

	probeTarget := &domain.Device{
		ID:         uuid.New(),
		Hostname:   "probe-switch",
		IP:         "192.0.2.21",
		Status:     domain.DeviceStatusProbing,
		Vendor:     "default",
		DeviceType: domain.DeviceTypeUnknown,
		PollClass:  domain.PollClassStandard,
		Interfaces: []domain.Interface{
			{IfIndex: 1, IfName: "ether1", IfDescr: "old-uplink", Speed: 100_000_000},
		},
		MetricsSource: domain.MetricsSourceSNMP,
		SNMPCredentials: domain.SNMPCredentials{
			Version: domain.SNMPVersionV2c,
			V2c:     &domain.SNMPv2cCredentials{Community: "public"},
		},
	}
	if err := deviceRepo.Create(probeTarget); err != nil {
		t.Fatalf("Create probe target failed: %v", err)
	}

	helperPersisted, err := svc.ApplyStaticDiscovery(helperDevice.ID, StaticDiscoveryInput{
		SysName:       "helper-switch",
		SysDescr:      "MikroTik CRS",
		SysObjectID:   ".1.3.6.1.4.1.14988.1",
		HardwareModel: "CRS326-24G-2S+",
		Vendor:        "mikrotik",
		DeviceType:    domain.DeviceTypeSwitch,
		Interfaces: []domain.Interface{
			{IfIndex: 1, IfName: "ether10", IfDescr: "distribution", Speed: 1_000_000_000},
		},
	})
	if err != nil {
		t.Fatalf("ApplyStaticDiscovery failed: %v", err)
	}
	if !helperPersisted.TopologyChanged {
		t.Fatal("expected helper persistence to report topology change")
	}

	select {
	case <-topologyNotify:
		t.Fatal("ApplyStaticDiscovery should not notify callers directly")
	default:
	}

	svc.probeDevice(probeTarget)

	updated, err := deviceRepo.GetByID(probeTarget.ID)
	if err != nil {
		t.Fatalf("GetByID failed: %v", err)
	}
	if updated.Status != domain.DeviceStatusUp {
		t.Fatalf("expected probe target status up, got %s", updated.Status)
	}
	if updated.SysName != "probe-switch" {
		t.Fatalf("expected probe target SysName persisted, got %q", updated.SysName)
	}
	if updated.Vendor != "mikrotik" {
		t.Fatalf("expected probe target Vendor persisted, got %q", updated.Vendor)
	}
	if len(updated.Interfaces) != 1 || updated.Interfaces[0].IfDescr != "uplink" {
		t.Fatalf("expected discovery interfaces persisted via shared helper, got %+v", updated.Interfaces)
	}

	select {
	case <-topologyNotify:
	default:
		t.Fatal("expected probeDevice to signal topology changes after shared persistence")
	}
}

func TestApplyStaticDiscoveryDoesNotLogNoopAutolinkTwice(t *testing.T) {
	observability.ResetDefaultForTest()

	svc, deviceRepo, _ := newStaticPersistenceService(nil)
	device := &domain.Device{
		ID:       uuid.New(),
		Hostname: "switch1",
		IP:       "192.0.2.31",
		SysName:  "switch1",
		Status:   domain.DeviceStatusUp,
	}
	remote := &domain.Device{
		ID:       uuid.New(),
		Hostname: "switch2",
		IP:       "192.0.2.32",
		SysName:  "switch2",
		Status:   domain.DeviceStatusUp,
	}
	if err := deviceRepo.Create(device); err != nil {
		t.Fatalf("Create device failed: %v", err)
	}
	if err := deviceRepo.Create(remote); err != nil {
		t.Fatalf("Create remote failed: %v", err)
	}

	input := StaticDiscoveryInput{
		SysName: "switch1",
		Neighbors: []snmp.NeighborInfo{{
			RemoteSysName: "switch2",
			RemotePortID:  "ether9",
			LocalIfName:   "ether3",
			Protocol:      domain.DiscoveryProtocolLLDP,
		}},
	}

	firstLogs := captureLogs(t, func() {
		if _, err := svc.ApplyStaticDiscovery(device.ID, input); err != nil {
			t.Fatalf("first ApplyStaticDiscovery failed: %v", err)
		}
	})
	secondLogs := captureLogs(t, func() {
		if _, err := svc.ApplyStaticDiscovery(device.ID, input); err != nil {
			t.Fatalf("second ApplyStaticDiscovery failed: %v", err)
		}
	})

	if !strings.Contains(firstLogs, "Auto-linked switch1:ether3 <-> switch2:ether9 via lldp (created)") {
		t.Fatalf("expected create log, got %q", firstLogs)
	}
	if strings.Contains(secondLogs, "Auto-linked") {
		t.Fatalf("expected noop rediscovery to avoid info log, got %q", secondLogs)
	}
}

func TestApplyStaticDiscoveryAggregatesUnknownNeighborLogsAndMetrics(t *testing.T) {
	registry := observability.ResetDefaultForTest()

	svc, deviceRepo, _ := newStaticPersistenceService(nil)
	device := &domain.Device{
		ID:       uuid.New(),
		Hostname: "switch1",
		IP:       "192.0.2.41",
		SysName:  "switch1",
		Status:   domain.DeviceStatusUp,
	}
	if err := deviceRepo.Create(device); err != nil {
		t.Fatalf("Create device failed: %v", err)
	}

	logs := captureLogs(t, func() {
		if _, err := svc.ApplyStaticDiscovery(device.ID, StaticDiscoveryInput{
			SysName: "switch1",
			Neighbors: []snmp.NeighborInfo{
				{RemoteSysName: "missing-a", RemotePortID: "ether1", LocalIfName: "ether7", Protocol: domain.DiscoveryProtocolLLDP},
				{RemoteSysName: "missing-a", RemotePortID: "ether1", LocalIfName: "ether7", Protocol: domain.DiscoveryProtocolLLDP},
				{RemoteSysName: "missing-b", RemotePortID: "ether2", LocalIfName: "ether8", Protocol: domain.DiscoveryProtocolLLDP},
				{RemoteChassisID: " 0011.2233.4455 ", RemotePortID: "ether3", LocalIfName: "ether9", Protocol: domain.DiscoveryProtocolLLDP},
			},
		}); err != nil {
			t.Fatalf("ApplyStaticDiscovery failed: %v", err)
		}
	})

	if strings.Contains(logs, "Skipping neighbor") {
		t.Fatalf("expected aggregated unknown-neighbor log, got %q", logs)
	}
	if !strings.Contains(logs, "Static discovery for switch1 observed off-map neighbors [lldp=4]") {
		t.Fatalf("expected aggregated summary log, got %q", logs)
	}
	if !strings.Contains(logs, "missing-a(lldp)x2") || !strings.Contains(logs, "missing-b(lldp)x1") {
		t.Fatalf("expected summarized neighbor names, got %q", logs)
	}
	if !strings.Contains(logs, "0011.2233.4455(lldp)x1") {
		t.Fatalf("expected summarized chassis identity, got %q", logs)
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/metrics", nil)
	registry.ServeHTTP(rec, req)
	body := rec.Body.String()
	if !strings.Contains(body, `theia_discovery_neighbors{device_id="`+device.ID.String()+`",protocol="lldp"} 4`) {
		t.Fatalf("expected discovery-neighbor gauge in metrics output, got %s", body)
	}
	if !strings.Contains(body, `theia_unknown_neighbors_total{device_id="`+device.ID.String()+`",protocol="lldp"} 4`) {
		t.Fatalf("expected unknown-neighbor counter in metrics output, got %s", body)
	}
}

func TestApplyStaticDiscoveryEmitsObservabilityMetrics(t *testing.T) {
	registry := observability.ResetDefaultForTest()

	svc, deviceRepo, _ := newStaticPersistenceService(nil)
	device := &domain.Device{
		ID:       uuid.New(),
		Hostname: "switch1",
		IP:       "192.0.2.51",
		SysName:  "switch1",
		Status:   domain.DeviceStatusUp,
	}
	remote := &domain.Device{
		ID:       uuid.New(),
		Hostname: "switch2",
		IP:       "192.0.2.52",
		SysName:  "switch2",
		Status:   domain.DeviceStatusUp,
	}
	if err := deviceRepo.Create(device); err != nil {
		t.Fatalf("Create device failed: %v", err)
	}
	if err := deviceRepo.Create(remote); err != nil {
		t.Fatalf("Create remote failed: %v", err)
	}

	if _, err := svc.ApplyStaticDiscovery(device.ID, StaticDiscoveryInput{
		SysName: "switch1",
		Neighbors: []snmp.NeighborInfo{{
			RemoteSysName: "switch2",
			RemotePortID:  "ether2",
			LocalIfName:   "ether1",
			Protocol:      domain.DiscoveryProtocolLLDP,
		}},
	}); err != nil {
		t.Fatalf("ApplyStaticDiscovery failed: %v", err)
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/metrics", nil)
	registry.ServeHTTP(rec, req)
	body := rec.Body.String()
	if !strings.Contains(body, `theia_discovery_neighbors{device_id="`+device.ID.String()+`",protocol="lldp"} 1`) {
		t.Fatalf("expected neighbor gauge in metrics output, got %s", body)
	}
	if !strings.Contains(body, `theia_link_upserts_total{protocol="lldp",result="created"} 1`) {
		t.Fatalf("expected link upsert counter in metrics output, got %s", body)
	}
	if !strings.Contains(body, `theia_topology_materialization_seconds_count{result="success"} 1`) {
		t.Fatalf("expected topology materialization metric in output, got %s", body)
	}
}

func TestApplyStaticDiscoveryWithObservationStore_PersistsObservationsAndUnresolvedNeighbors(t *testing.T) {
	db, err := sql.Open("sqlite3", ":memory:?_foreign_keys=on")
	if err != nil {
		t.Fatalf("opening sqlite db: %v", err)
	}
	defer db.Close()
	if err := sqliterepo.RunMigrations(db); err != nil {
		t.Fatalf("RunMigrations failed: %v", err)
	}

	deviceRepo := sqliterepo.NewDeviceRepo(db, nil, nil)
	linkRepo := sqliterepo.NewLinkRepo(db, nil)
	observationRepo := sqliterepo.NewTopologyObservationRepo(db)
	settingsRepo := newMockSettingsRepo()
	svc := NewDeviceService(
		deviceRepo,
		linkRepo,
		settingsRepo,
		func(target string, creds domain.SNMPCredentials, _ domain.TopologyDiscoveryMode) (*snmp.DiscoveryResult, error) {
			return nil, nil
		},
		nil,
		WithTopologyObservationStore(observationRepo),
	)

	local := &domain.Device{ID: uuid.New(), Hostname: "local", IP: "192.0.2.51", SysName: "local", Managed: true, Status: domain.DeviceStatusUp}
	remote := &domain.Device{ID: uuid.New(), Hostname: "remote", IP: "192.0.2.52", SysName: "remote", Managed: true, Status: domain.DeviceStatusUp}
	if err := deviceRepo.Create(local); err != nil {
		t.Fatalf("Create local failed: %v", err)
	}
	if err := deviceRepo.Create(remote); err != nil {
		t.Fatalf("Create remote failed: %v", err)
	}

	persisted, err := svc.ApplyStaticDiscovery(local.ID, StaticDiscoveryInput{
		SysName: "local",
		Neighbors: []snmp.NeighborInfo{
			{RemoteSysName: "remote", RemotePortID: "ether2", LocalIfName: "ether1", Protocol: domain.DiscoveryProtocolLLDP},
			{RemoteSysName: "missing-edge", RemotePortID: "ether9", LocalIfName: "ether8", Protocol: domain.DiscoveryProtocolLLDP},
		},
	})
	if err != nil {
		t.Fatalf("ApplyStaticDiscovery failed: %v", err)
	}
	if !persisted.TopologyChanged {
		t.Fatal("expected topology change through observation materialization")
	}
	if persisted.LinksCreated != 1 {
		t.Fatalf("LinksCreated = %d, want 1", persisted.LinksCreated)
	}

	observations, err := observationRepo.ListObservationsForDevices([]uuid.UUID{local.ID, remote.ID})
	if err != nil {
		t.Fatalf("ListObservationsForDevices failed: %v", err)
	}
	if len(observations) != 2 {
		t.Fatalf("expected 2 persisted observations, got %d", len(observations))
	}

	unresolved, err := observationRepo.GetUnresolvedNeighborsByDeviceID(local.ID)
	if err != nil {
		t.Fatalf("GetUnresolvedNeighborsByDeviceID failed: %v", err)
	}
	if len(unresolved) != 1 {
		t.Fatalf("expected 1 unresolved neighbor, got %d", len(unresolved))
	}
	if unresolved[0].RemoteIdentity != "missing-edge" {
		t.Fatalf("RemoteIdentity = %q, want missing-edge", unresolved[0].RemoteIdentity)
	}

	links, err := linkRepo.GetAll()
	if err != nil {
		t.Fatalf("GetAll links failed: %v", err)
	}
	if len(links) != 1 {
		t.Fatalf("expected 1 canonical link, got %d", len(links))
	}
	if links[0].SourceIfName != "ether1" || links[0].TargetIfName != "ether2" {
		t.Fatalf("unexpected link after materialization: %+v", links[0])
	}
}

func TestApplyStaticDiscoveryWithObservationStore_PersistsChassisOnlyUnresolvedNeighbor(t *testing.T) {
	registry := observability.ResetDefaultForTest()

	db, err := sql.Open("sqlite3", ":memory:?_foreign_keys=on")
	if err != nil {
		t.Fatalf("opening sqlite db: %v", err)
	}
	defer db.Close()
	if err := sqliterepo.RunMigrations(db); err != nil {
		t.Fatalf("RunMigrations failed: %v", err)
	}

	deviceRepo := sqliterepo.NewDeviceRepo(db, nil, nil)
	linkRepo := sqliterepo.NewLinkRepo(db, nil)
	observationRepo := sqliterepo.NewTopologyObservationRepo(db)
	settingsRepo := newMockSettingsRepo()
	svc := NewDeviceService(
		deviceRepo,
		linkRepo,
		settingsRepo,
		func(target string, creds domain.SNMPCredentials, _ domain.TopologyDiscoveryMode) (*snmp.DiscoveryResult, error) {
			return nil, nil
		},
		nil,
		WithTopologyObservationStore(observationRepo),
	)

	local := &domain.Device{ID: uuid.New(), Hostname: "local", IP: "192.0.2.61", SysName: "local", Managed: true, Status: domain.DeviceStatusUp}
	if err := deviceRepo.Create(local); err != nil {
		t.Fatalf("Create local failed: %v", err)
	}

	var persisted StaticPersistenceResult
	logs := captureLogs(t, func() {
		var applyErr error
		persisted, applyErr = svc.ApplyStaticDiscovery(local.ID, StaticDiscoveryInput{
			SysName: "local",
			Neighbors: []snmp.NeighborInfo{{
				RemoteChassisID: " 0011.2233.4455 ",
				RemotePortID:    "ether2",
				LocalIfName:     "ether1",
				Protocol:        domain.DiscoveryProtocolLLDP,
			}},
		})
		if applyErr != nil {
			t.Fatalf("ApplyStaticDiscovery failed: %v", applyErr)
		}
	})

	if persisted.TopologyChanged {
		t.Fatal("expected no topology change without a resolvable remote device")
	}
	if persisted.LinksCreated != 0 {
		t.Fatalf("LinksCreated = %d, want 0", persisted.LinksCreated)
	}

	observations, err := observationRepo.ListObservationsForDevices([]uuid.UUID{local.ID})
	if err != nil {
		t.Fatalf("ListObservationsForDevices failed: %v", err)
	}
	if len(observations) != 1 {
		t.Fatalf("expected 1 persisted observation, got %d", len(observations))
	}
	if observations[0].RemoteIdentity != "0011.2233.4455" {
		t.Fatalf("observation RemoteIdentity = %q, want 0011.2233.4455", observations[0].RemoteIdentity)
	}
	if observations[0].RemoteDeviceID != uuid.Nil {
		t.Fatalf("RemoteDeviceID = %s, want nil", observations[0].RemoteDeviceID)
	}

	unresolved, err := observationRepo.GetUnresolvedNeighborsByDeviceID(local.ID)
	if err != nil {
		t.Fatalf("GetUnresolvedNeighborsByDeviceID failed: %v", err)
	}
	if len(unresolved) != 1 {
		t.Fatalf("expected 1 unresolved neighbor, got %d", len(unresolved))
	}
	if unresolved[0].RemoteIdentity != "0011.2233.4455" {
		t.Fatalf("RemoteIdentity = %q, want 0011.2233.4455", unresolved[0].RemoteIdentity)
	}

	links, err := linkRepo.GetAll()
	if err != nil {
		t.Fatalf("GetAll links failed: %v", err)
	}
	if len(links) != 0 {
		t.Fatalf("expected no canonical links, got %d", len(links))
	}

	if !strings.Contains(logs, "Static discovery for local observed off-map neighbors [lldp=1]") {
		t.Fatalf("expected aggregated summary log, got %q", logs)
	}
	if !strings.Contains(logs, "0011.2233.4455(lldp)x1") {
		t.Fatalf("expected chassis identity in summary log, got %q", logs)
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/metrics", nil)
	registry.ServeHTTP(rec, req)
	body := rec.Body.String()
	if !strings.Contains(body, `theia_discovery_neighbors{device_id="`+local.ID.String()+`",protocol="lldp"} 1`) {
		t.Fatalf("expected chassis-only neighbor in discovery count, got %s", body)
	}
	if !strings.Contains(body, `theia_unknown_neighbors_total{device_id="`+local.ID.String()+`",protocol="lldp"} 1`) {
		t.Fatalf("expected chassis-only neighbor in unknown count, got %s", body)
	}
}

func TestApplyStaticDiscoveryWithObservationStore_PrunesStaleLLDPObservationAndAutoLink(t *testing.T) {
	svc, deviceRepo, linkRepo, observationRepo := newObservationStoreStaticPersistenceService(t)

	local := &domain.Device{ID: uuid.New(), Hostname: "local", IP: "192.0.2.71", SysName: "local", Managed: true, Status: domain.DeviceStatusUp}
	remote := &domain.Device{ID: uuid.New(), Hostname: "remote", IP: "192.0.2.72", SysName: "remote", Managed: true, Status: domain.DeviceStatusUp}
	for _, device := range []*domain.Device{local, remote} {
		if err := deviceRepo.Create(device); err != nil {
			t.Fatalf("Create %s failed: %v", device.Hostname, err)
		}
	}

	first, err := svc.ApplyStaticDiscovery(local.ID, StaticDiscoveryInput{
		SysName:                    "local",
		NeighborDiscoveryProtocols: []domain.DiscoveryProtocol{domain.DiscoveryProtocolLLDP},
		Neighbors: []snmp.NeighborInfo{{
			RemoteSysName: "remote",
			RemotePortID:  "ether2",
			LocalIfName:   "ether1",
			Protocol:      domain.DiscoveryProtocolLLDP,
		}},
	})
	if err != nil {
		t.Fatalf("first ApplyStaticDiscovery failed: %v", err)
	}
	if !first.TopologyChanged || first.LinksCreated != 1 {
		t.Fatalf("first persistence result = %+v, want topology change with one created link", first)
	}

	second, err := svc.ApplyStaticDiscovery(local.ID, StaticDiscoveryInput{
		SysName:                    "local",
		NeighborDiscoveryProtocols: []domain.DiscoveryProtocol{domain.DiscoveryProtocolLLDP},
	})
	if err != nil {
		t.Fatalf("second ApplyStaticDiscovery failed: %v", err)
	}
	if !second.TopologyChanged {
		t.Fatal("expected stale auto-link pruning to report topology changed")
	}

	observations, err := observationRepo.ListObservationsForDevices([]uuid.UUID{local.ID, remote.ID})
	if err != nil {
		t.Fatalf("ListObservationsForDevices failed: %v", err)
	}
	if len(observations) != 0 {
		t.Fatalf("expected stale LLDP observations to be pruned, got %d", len(observations))
	}

	links, err := linkRepo.GetAll()
	if err != nil {
		t.Fatalf("GetAll links failed: %v", err)
	}
	if len(links) != 0 {
		t.Fatalf("expected stale LLDP auto-link to be deleted, got %d", len(links))
	}
}

func TestApplyStaticDiscoveryWithObservationStore_PartialPortSnapshotKeepsEnrichedAutoLink(t *testing.T) {
	svc, deviceRepo, linkRepo, _ := newObservationStoreStaticPersistenceService(t)

	local := &domain.Device{ID: uuid.New(), Hostname: "local", IP: "192.0.2.73", SysName: "local", Managed: true, Status: domain.DeviceStatusUp}
	remote := &domain.Device{ID: uuid.New(), Hostname: "remote", IP: "192.0.2.74", SysName: "remote", Managed: true, Status: domain.DeviceStatusUp}
	for _, device := range []*domain.Device{local, remote} {
		if err := deviceRepo.Create(device); err != nil {
			t.Fatalf("Create %s failed: %v", device.Hostname, err)
		}
	}

	first, err := svc.ApplyStaticDiscovery(local.ID, StaticDiscoveryInput{
		SysName:                    "local",
		NeighborDiscoveryProtocols: []domain.DiscoveryProtocol{domain.DiscoveryProtocolLLDP},
		Neighbors: []snmp.NeighborInfo{{
			RemoteSysName: "remote",
			RemotePortID:  "ether2",
			LocalIfName:   "ether1",
			Protocol:      domain.DiscoveryProtocolLLDP,
		}},
	})
	if err != nil {
		t.Fatalf("first ApplyStaticDiscovery failed: %v", err)
	}
	if !first.TopologyChanged || first.LinksCreated != 1 {
		t.Fatalf("first persistence result = %+v, want topology change with one created link", first)
	}

	if _, err := svc.ApplyStaticDiscovery(local.ID, StaticDiscoveryInput{
		SysName:                    "local",
		NeighborDiscoveryProtocols: []domain.DiscoveryProtocol{domain.DiscoveryProtocolLLDP},
		Neighbors: []snmp.NeighborInfo{{
			RemoteSysName: "remote",
			LocalIfName:   "ether1",
			Protocol:      domain.DiscoveryProtocolLLDP,
		}},
	}); err != nil {
		t.Fatalf("second ApplyStaticDiscovery failed: %v", err)
	}

	links, err := linkRepo.GetAll()
	if err != nil {
		t.Fatalf("GetAll links failed: %v", err)
	}
	if len(links) != 1 {
		t.Fatalf("expected partial snapshot to keep one auto-link, got %d links: %+v", len(links), links)
	}
	if links[0].SourceDeviceID != local.ID || links[0].TargetDeviceID != remote.ID {
		t.Fatalf("expected local to remote link to remain, got %+v", links[0])
	}
	if links[0].SourceIfName != "ether1" || links[0].TargetIfName != "ether2" {
		t.Fatalf("expected enriched ports ether1/ether2 to remain, got %+v", links[0])
	}
}

func TestApplyStaticDiscoveryWithObservationStore_UnresolvedChassisSnapshotKeepsExistingAutoLink(t *testing.T) {
	svc, deviceRepo, linkRepo, _ := newObservationStoreStaticPersistenceService(t)

	local := &domain.Device{ID: uuid.New(), Hostname: "local", IP: "192.0.2.75", SysName: "local", Managed: true, Status: domain.DeviceStatusUp}
	remote := &domain.Device{ID: uuid.New(), Hostname: "remote", IP: "192.0.2.76", SysName: "remote", Managed: true, Status: domain.DeviceStatusUp}
	for _, device := range []*domain.Device{local, remote} {
		if err := deviceRepo.Create(device); err != nil {
			t.Fatalf("Create %s failed: %v", device.Hostname, err)
		}
	}

	first, err := svc.ApplyStaticDiscovery(local.ID, StaticDiscoveryInput{
		SysName:                    "local",
		NeighborDiscoveryProtocols: []domain.DiscoveryProtocol{domain.DiscoveryProtocolLLDP},
		Neighbors: []snmp.NeighborInfo{{
			RemoteSysName:   "remote",
			RemoteChassisID: "aa:bb:cc:dd:ee:ff",
			RemotePortID:    "ether2",
			LocalIfName:     "ether1",
			Protocol:        domain.DiscoveryProtocolLLDP,
		}},
	})
	if err != nil {
		t.Fatalf("first ApplyStaticDiscovery failed: %v", err)
	}
	if !first.TopologyChanged || first.LinksCreated != 1 {
		t.Fatalf("first persistence result = %+v, want topology change with one created link", first)
	}

	if _, err := svc.ApplyStaticDiscovery(local.ID, StaticDiscoveryInput{
		SysName:                    "local",
		NeighborDiscoveryProtocols: []domain.DiscoveryProtocol{domain.DiscoveryProtocolLLDP},
		Neighbors: []snmp.NeighborInfo{{
			RemoteChassisID: "aa:bb:cc:dd:ee:ff",
			LocalIfName:     "ether1",
			Protocol:        domain.DiscoveryProtocolLLDP,
		}},
	}); err != nil {
		t.Fatalf("second ApplyStaticDiscovery failed: %v", err)
	}

	links, err := linkRepo.GetAll()
	if err != nil {
		t.Fatalf("GetAll links failed: %v", err)
	}
	if len(links) != 1 {
		t.Fatalf("expected unresolved snapshot to keep one auto-link, got %d links: %+v", len(links), links)
	}
	if links[0].SourceDeviceID != local.ID || links[0].TargetDeviceID != remote.ID {
		t.Fatalf("expected local to remote link to remain, got %+v", links[0])
	}
	if links[0].SourceIfName != "ether1" || links[0].TargetIfName != "ether2" {
		t.Fatalf("expected full ports ether1/ether2 to remain, got %+v", links[0])
	}
}

func TestApplyStaticDiscoveryWithObservationStore_SkipsPruneWhenLLDPDiscoveryFailure(t *testing.T) {
	svc, deviceRepo, linkRepo, observationRepo := newObservationStoreStaticPersistenceService(t)

	local := &domain.Device{ID: uuid.New(), Hostname: "local", IP: "192.0.2.81", SysName: "local", Managed: true, Status: domain.DeviceStatusUp}
	remote := &domain.Device{ID: uuid.New(), Hostname: "remote", IP: "192.0.2.82", SysName: "remote", Managed: true, Status: domain.DeviceStatusUp}
	for _, device := range []*domain.Device{local, remote} {
		if err := deviceRepo.Create(device); err != nil {
			t.Fatalf("Create %s failed: %v", device.Hostname, err)
		}
	}

	if _, err := svc.ApplyStaticDiscovery(local.ID, StaticDiscoveryInput{
		SysName:                    "local",
		NeighborDiscoveryProtocols: []domain.DiscoveryProtocol{domain.DiscoveryProtocolLLDP},
		Neighbors: []snmp.NeighborInfo{{
			RemoteSysName: "remote",
			RemotePortID:  "ether2",
			LocalIfName:   "ether1",
			Protocol:      domain.DiscoveryProtocolLLDP,
		}},
	}); err != nil {
		t.Fatalf("first ApplyStaticDiscovery failed: %v", err)
	}

	second, err := svc.ApplyStaticDiscovery(local.ID, StaticDiscoveryInput{
		SysName:                    "local",
		NeighborDiscoveryProtocols: []domain.DiscoveryProtocol{domain.DiscoveryProtocolLLDP},
		NeighborDiscoveryFailures: []snmp.NeighborDiscoveryFailure{{
			Protocol: domain.DiscoveryProtocolLLDP,
			OID:      snmp.OidLLDPLocPortId,
			Critical: false,
			Error:    "walk failed",
		}},
	})
	if err != nil {
		t.Fatalf("second ApplyStaticDiscovery failed: %v", err)
	}
	if second.TopologyChanged {
		t.Fatalf("expected failed LLDP discovery to skip pruning without topology change, got %+v", second)
	}

	observations, err := observationRepo.ListObservationsForDevices([]uuid.UUID{local.ID, remote.ID})
	if err != nil {
		t.Fatalf("ListObservationsForDevices failed: %v", err)
	}
	if len(observations) != 1 {
		t.Fatalf("expected stale LLDP observation to remain after failure, got %d", len(observations))
	}

	links, err := linkRepo.GetAll()
	if err != nil {
		t.Fatalf("GetAll links failed: %v", err)
	}
	if len(links) != 1 || links[0].DiscoveryProtocol != domain.DiscoveryProtocolLLDP {
		t.Fatalf("expected stale LLDP auto-link to remain after failure, got %+v", links)
	}
}

func TestApplyStaticDiscoveryWithObservationStore_PruneDoesNotDeleteManualLinks(t *testing.T) {
	svc, deviceRepo, linkRepo, _ := newObservationStoreStaticPersistenceService(t)

	local := &domain.Device{ID: uuid.New(), Hostname: "local", IP: "192.0.2.91", SysName: "local", Managed: true, Status: domain.DeviceStatusUp}
	remote := &domain.Device{ID: uuid.New(), Hostname: "remote", IP: "192.0.2.92", SysName: "remote", Managed: true, Status: domain.DeviceStatusUp}
	for _, device := range []*domain.Device{local, remote} {
		if err := deviceRepo.Create(device); err != nil {
			t.Fatalf("Create %s failed: %v", device.Hostname, err)
		}
	}

	if _, err := svc.ApplyStaticDiscovery(local.ID, StaticDiscoveryInput{
		SysName:                    "local",
		NeighborDiscoveryProtocols: []domain.DiscoveryProtocol{domain.DiscoveryProtocolLLDP},
		Neighbors: []snmp.NeighborInfo{{
			RemoteSysName: "remote",
			RemotePortID:  "ether2",
			LocalIfName:   "ether1",
			Protocol:      domain.DiscoveryProtocolLLDP,
		}},
	}); err != nil {
		t.Fatalf("first ApplyStaticDiscovery failed: %v", err)
	}

	manualLink := &domain.Link{
		SourceDeviceID:    local.ID,
		SourceIfName:      "ether9",
		TargetDeviceID:    remote.ID,
		TargetIfName:      "ether10",
		DiscoveryProtocol: domain.DiscoveryProtocolManual,
	}
	if err := linkRepo.Create(manualLink); err != nil {
		t.Fatalf("Create manual link failed: %v", err)
	}

	if _, err := svc.ApplyStaticDiscovery(local.ID, StaticDiscoveryInput{
		SysName:                    "local",
		NeighborDiscoveryProtocols: []domain.DiscoveryProtocol{domain.DiscoveryProtocolLLDP},
	}); err != nil {
		t.Fatalf("second ApplyStaticDiscovery failed: %v", err)
	}

	links, err := linkRepo.GetAll()
	if err != nil {
		t.Fatalf("GetAll links failed: %v", err)
	}
	if len(links) != 1 {
		t.Fatalf("expected only manual link to remain, got %d links: %+v", len(links), links)
	}
	if links[0].ID != manualLink.ID || links[0].DiscoveryProtocol != domain.DiscoveryProtocolManual {
		t.Fatalf("expected manual link to remain after LLDP pruning, got %+v", links[0])
	}
}

func TestApplyStaticDiscoveryWithObservationStore_ReturnsErrorBeforePruneWhenNeighborLookupFails(t *testing.T) {
	svc, deviceRepo, linkRepo, observationRepo := newObservationStoreStaticPersistenceService(t)

	local := &domain.Device{ID: uuid.New(), Hostname: "local", IP: "192.0.2.101", SysName: "local", Managed: true, Status: domain.DeviceStatusUp}
	remote := &domain.Device{ID: uuid.New(), Hostname: "remote", IP: "192.0.2.102", SysName: "remote", Managed: true, Status: domain.DeviceStatusUp}
	for _, device := range []*domain.Device{local, remote} {
		if err := deviceRepo.Create(device); err != nil {
			t.Fatalf("Create %s failed: %v", device.Hostname, err)
		}
	}

	if _, err := svc.ApplyStaticDiscovery(local.ID, StaticDiscoveryInput{
		SysName:                    "local",
		NeighborDiscoveryProtocols: []domain.DiscoveryProtocol{domain.DiscoveryProtocolLLDP},
		Neighbors: []snmp.NeighborInfo{{
			RemoteSysName: "remote",
			RemotePortID:  "ether2",
			LocalIfName:   "ether1",
			Protocol:      domain.DiscoveryProtocolLLDP,
		}},
	}); err != nil {
		t.Fatalf("first ApplyStaticDiscovery failed: %v", err)
	}

	svc.deviceRepo = lookupErrorDeviceRepo{
		DeviceRepository: deviceRepo,
		err:              errors.New("sysname lookup unavailable"),
	}

	_, err := svc.ApplyStaticDiscovery(local.ID, StaticDiscoveryInput{
		SysName:                    "local",
		NeighborDiscoveryProtocols: []domain.DiscoveryProtocol{domain.DiscoveryProtocolLLDP},
		Neighbors: []snmp.NeighborInfo{{
			RemoteSysName: "remote",
			RemotePortID:  "ether2",
			LocalIfName:   "ether1",
			Protocol:      domain.DiscoveryProtocolLLDP,
		}},
	})
	if err == nil {
		t.Fatal("expected lookup error to stop reconciliation before pruning")
	}
	if !strings.Contains(err.Error(), "looking up neighbor remote") {
		t.Fatalf("error = %q, want neighbor lookup context", err)
	}

	observations, err := observationRepo.ListObservationsForDevices([]uuid.UUID{local.ID, remote.ID})
	if err != nil {
		t.Fatalf("ListObservationsForDevices failed: %v", err)
	}
	if len(observations) != 1 {
		t.Fatalf("expected existing observation to remain after lookup error, got %d", len(observations))
	}

	links, err := linkRepo.GetAll()
	if err != nil {
		t.Fatalf("GetAll links failed: %v", err)
	}
	if len(links) != 1 {
		t.Fatalf("expected existing auto-link to remain after lookup error, got %d", len(links))
	}
}

func TestApplyStaticDiscoveryWithObservationStore_PruneKeepsLinkSupportedByReverseObservation(t *testing.T) {
	svc, deviceRepo, linkRepo, observationRepo := newObservationStoreStaticPersistenceService(t)

	local := &domain.Device{ID: uuid.New(), Hostname: "local", IP: "192.0.2.111", SysName: "local", Managed: true, Status: domain.DeviceStatusUp}
	remote := &domain.Device{ID: uuid.New(), Hostname: "remote", IP: "192.0.2.112", SysName: "remote", Managed: true, Status: domain.DeviceStatusUp}
	for _, device := range []*domain.Device{local, remote} {
		if err := deviceRepo.Create(device); err != nil {
			t.Fatalf("Create %s failed: %v", device.Hostname, err)
		}
	}

	link := &domain.Link{
		SourceDeviceID:    local.ID,
		SourceIfName:      "ether1",
		TargetDeviceID:    remote.ID,
		TargetIfName:      "ether2",
		DiscoveryProtocol: domain.DiscoveryProtocolLLDP,
	}
	if err := linkRepo.Create(link); err != nil {
		t.Fatalf("Create link failed: %v", err)
	}

	now := time.Date(2026, 5, 15, 12, 30, 0, 0, time.UTC)
	if err := observationRepo.UpsertObservation(&topology.Observation{
		LocalDeviceID:   remote.ID,
		RemoteIdentity:  topology.NormalizeRemoteIdentity(local.SysName),
		RemoteDeviceID:  local.ID,
		LocalPort:       "ether2",
		RemotePort:      "ether1",
		Protocol:        domain.DiscoveryProtocolLLDP,
		FirstObservedAt: now,
		LastObservedAt:  now,
	}); err != nil {
		t.Fatalf("UpsertObservation failed: %v", err)
	}

	if _, err := svc.ApplyStaticDiscovery(local.ID, StaticDiscoveryInput{
		SysName:                    "local",
		NeighborDiscoveryProtocols: []domain.DiscoveryProtocol{domain.DiscoveryProtocolLLDP},
	}); err != nil {
		t.Fatalf("ApplyStaticDiscovery failed: %v", err)
	}

	links, err := linkRepo.GetAll()
	if err != nil {
		t.Fatalf("GetAll links failed: %v", err)
	}
	if len(links) != 1 {
		t.Fatalf("expected reverse observation to protect existing link, got %d links: %+v", len(links), links)
	}
	if links[0].ID != link.ID {
		t.Fatalf("link ID = %s, want existing link %s", links[0].ID, link.ID)
	}
}

type lookupErrorDeviceRepo struct {
	domain.DeviceRepository
	err error
}

func (r lookupErrorDeviceRepo) GetBySysName(string) (*domain.Device, error) {
	return nil, r.err
}

func newObservationStoreStaticPersistenceService(
	t *testing.T,
) (*DeviceService, *sqliterepo.DeviceRepo, *sqliterepo.LinkRepo, *sqliterepo.TopologyObservationRepo) {
	t.Helper()

	db, err := sql.Open("sqlite3", ":memory:?_foreign_keys=on")
	if err != nil {
		t.Fatalf("opening sqlite db: %v", err)
	}
	t.Cleanup(func() {
		_ = db.Close()
	})
	if err := sqliterepo.RunMigrations(db); err != nil {
		t.Fatalf("RunMigrations failed: %v", err)
	}

	deviceRepo := sqliterepo.NewDeviceRepo(db, nil, nil)
	linkRepo := sqliterepo.NewLinkRepo(db, nil)
	observationRepo := sqliterepo.NewTopologyObservationRepo(db)
	settingsRepo := newMockSettingsRepo()
	svc := NewDeviceService(
		deviceRepo,
		linkRepo,
		settingsRepo,
		func(target string, creds domain.SNMPCredentials, _ domain.TopologyDiscoveryMode) (*snmp.DiscoveryResult, error) {
			return nil, nil
		},
		nil,
		WithTopologyObservationStore(observationRepo),
	)
	return svc, deviceRepo, linkRepo, observationRepo
}
