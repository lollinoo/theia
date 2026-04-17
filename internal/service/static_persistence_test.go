package service

import (
	"testing"

	"github.com/google/uuid"
	"github.com/lollinoo/theia/internal/domain"
	"github.com/lollinoo/theia/internal/snmp"
)

func newStaticPersistenceService(topologyNotify chan struct{}) (*DeviceService, *mockDeviceRepo, *mockLinkRepo) {
	deviceRepo := newMockDeviceRepo()
	linkRepo := newMockLinkRepo()
	settingsRepo := newMockSettingsRepo()

	discoverFn := func(target string, creds domain.SNMPCredentials) (*snmp.DiscoveryResult, error) {
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

	discoverFn := func(target string, creds domain.SNMPCredentials) (*snmp.DiscoveryResult, error) {
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
