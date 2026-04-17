package service

import (
	"context"
	"fmt"
	"hash/fnv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/lollinoo/theia/internal/domain"
	"github.com/lollinoo/theia/internal/scheduler"
	"github.com/lollinoo/theia/internal/snmp"
)

// --- Mock Device Repository ---

type mockDeviceRepo struct {
	mu         sync.Mutex
	devices    map[uuid.UUID]*domain.Device
	updateHook func(*domain.Device) error
}

func newMockDeviceRepo() *mockDeviceRepo {
	return &mockDeviceRepo{devices: make(map[uuid.UUID]*domain.Device)}
}

func (r *mockDeviceRepo) Create(device *domain.Device) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if device.ID == uuid.Nil {
		device.ID = uuid.New()
	}
	now := time.Now().UTC()
	device.CreatedAt = now
	device.UpdatedAt = now
	r.devices[device.ID] = device
	return nil
}

func (r *mockDeviceRepo) GetByID(id uuid.UUID) (*domain.Device, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	d, ok := r.devices[id]
	if !ok {
		return nil, fmt.Errorf("device not found: %s", id)
	}
	cp := *d
	return &cp, nil
}

func (r *mockDeviceRepo) GetByIP(ip string) (*domain.Device, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, d := range r.devices {
		if d.IP == ip {
			cp := *d
			return &cp, nil
		}
	}
	return nil, nil
}

func (r *mockDeviceRepo) GetAll() ([]domain.Device, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var result []domain.Device
	for _, d := range r.devices {
		result = append(result, *d)
	}
	return result, nil
}

func (r *mockDeviceRepo) Update(device *domain.Device) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.devices[device.ID]; !ok {
		return fmt.Errorf("device not found: %s", device.ID)
	}
	if r.updateHook != nil {
		if err := r.updateHook(device); err != nil {
			return err
		}
	}
	device.UpdatedAt = time.Now().UTC()
	r.devices[device.ID] = device
	return nil
}

func (r *mockDeviceRepo) Delete(id uuid.UUID) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.devices[id]; !ok {
		return fmt.Errorf("device not found: %s", id)
	}
	delete(r.devices, id)
	return nil
}

func (r *mockDeviceRepo) GetBySysName(sysName string) (*domain.Device, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, d := range r.devices {
		if d.SysName == sysName {
			cp := *d
			return &cp, nil
		}
	}
	return nil, nil
}

// --- Mock Link Repository ---

type mockLinkRepo struct {
	mu    sync.Mutex
	links map[uuid.UUID]*domain.Link
}

func newMockLinkRepo() *mockLinkRepo {
	return &mockLinkRepo{links: make(map[uuid.UUID]*domain.Link)}
}

func (r *mockLinkRepo) Create(link *domain.Link) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if link.ID == uuid.Nil {
		link.ID = uuid.New()
	}
	now := time.Now().UTC()
	link.CreatedAt = now
	link.UpdatedAt = now
	r.links[link.ID] = link
	return nil
}

func (r *mockLinkRepo) GetByID(id uuid.UUID) (*domain.Link, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	l, ok := r.links[id]
	if !ok {
		return nil, fmt.Errorf("link not found: %s", id)
	}
	cp := *l
	return &cp, nil
}

func (r *mockLinkRepo) GetByDeviceID(deviceID uuid.UUID) ([]domain.Link, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var result []domain.Link
	for _, l := range r.links {
		if l.SourceDeviceID == deviceID || l.TargetDeviceID == deviceID {
			result = append(result, *l)
		}
	}
	return result, nil
}

func (r *mockLinkRepo) GetAll() ([]domain.Link, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var result []domain.Link
	for _, l := range r.links {
		result = append(result, *l)
	}
	return result, nil
}

func (r *mockLinkRepo) Update(link *domain.Link) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.links[link.ID]; !ok {
		return fmt.Errorf("link not found: %s", link.ID)
	}
	link.UpdatedAt = time.Now().UTC()
	r.links[link.ID] = link
	return nil
}

func (r *mockLinkRepo) Delete(id uuid.UUID) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.links[id]; !ok {
		return fmt.Errorf("link not found: %s", id)
	}
	delete(r.links, id)
	return nil
}

func (r *mockLinkRepo) Upsert(link *domain.Link) (bool, error) {
	result, err := r.UpsertDetailed(link)
	return result.Created, err
}

func (r *mockLinkRepo) UpsertDetailed(link *domain.Link) (domain.LinkUpsertResult, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for id, existing := range r.links {
		if existing.SourceDeviceID == link.SourceDeviceID &&
			existing.TargetDeviceID == link.TargetDeviceID &&
			(existing.SourceIfName == link.SourceIfName || existing.SourceIfName == "" || link.SourceIfName == "") &&
			(existing.TargetIfName == link.TargetIfName || existing.TargetIfName == "" || link.TargetIfName == "") {
			updated := *existing
			if updated.SourceIfName == "" && link.SourceIfName != "" {
				updated.SourceIfName = link.SourceIfName
			}
			if updated.TargetIfName == "" && link.TargetIfName != "" {
				updated.TargetIfName = link.TargetIfName
			}
			updated.DiscoveryProtocol = link.DiscoveryProtocol
			updated.UpdatedAt = time.Now().UTC()
			r.links[id] = &updated
			*link = updated
			return domain.LinkUpsertResult{
				Created: false,
				Changed: updated.SourceIfName != existing.SourceIfName || updated.TargetIfName != existing.TargetIfName,
			}, nil
		}
	}
	if link.ID == uuid.Nil {
		link.ID = uuid.New()
	}
	now := time.Now().UTC()
	link.CreatedAt = now
	link.UpdatedAt = now
	r.links[link.ID] = link
	return domain.LinkUpsertResult{Created: true, Changed: true}, nil
}

// --- Mock Settings Repository ---

type mockSettingsRepo struct {
	settings map[string]string
}

func newMockSettingsRepo() *mockSettingsRepo {
	return &mockSettingsRepo{settings: domain.DefaultSettings()}
}

func (r *mockSettingsRepo) Get(key string) (string, error) {
	v, ok := r.settings[key]
	if !ok {
		return "", fmt.Errorf("setting not found: %s", key)
	}
	return v, nil
}

func (r *mockSettingsRepo) Set(key, value string) error {
	r.settings[key] = value
	return nil
}

func (r *mockSettingsRepo) GetAll() (map[string]string, error) {
	cp := make(map[string]string)
	for k, v := range r.settings {
		cp[k] = v
	}
	return cp, nil
}

// --- Helper to create a service with mock SNMP ---

func newTestService(snmpResult *snmp.DiscoveryResult, snmpErr error) (*DeviceService, *mockDeviceRepo, *mockLinkRepo) {
	deviceRepo := newMockDeviceRepo()
	linkRepo := newMockLinkRepo()
	settingsRepo := newMockSettingsRepo()

	discoverFn := func(target string, creds domain.SNMPCredentials) (*snmp.DiscoveryResult, error) {
		return snmpResult, snmpErr
	}

	svc := NewDeviceService(deviceRepo, linkRepo, settingsRepo, discoverFn, nil)
	return svc, deviceRepo, linkRepo
}

type fakePollRescheduler struct {
	calls []pollRescheduleCall
}

type pollRescheduleCall struct {
	device    domain.Device
	changedAt time.Time
}

func (f *fakePollRescheduler) ReduePerformanceTask(device domain.Device, changedAt time.Time) {
	f.calls = append(f.calls, pollRescheduleCall{
		device:    device,
		changedAt: changedAt,
	})
}

type schedulerDeviceSource struct {
	repo *mockDeviceRepo
}

func (s schedulerDeviceSource) GetDevices() ([]domain.Device, error) {
	return s.repo.GetAll()
}

// --- Tests ---

func TestAddDevice_CreatesWithStatusProbing(t *testing.T) {
	svc, deviceRepo, _ := newTestService(&snmp.DiscoveryResult{
		SysName: "router1",
	}, nil)

	device, err := svc.AddDevice(context.Background(), "192.168.1.1", "router1",
		domain.DeviceTypeUnknown,
		domain.SNMPCredentials{
			Version: domain.SNMPVersionV2c,
			V2c:     &domain.SNMPv2cCredentials{Community: "public"},
		}, nil, "", "", "", "", nil)
	if err != nil {
		t.Fatalf("AddDevice failed: %v", err)
	}
	if device.Status != domain.DeviceStatusProbing {
		t.Errorf("expected status probing, got %s", device.Status)
	}
	if device.Managed != true {
		t.Error("expected managed=true")
	}

	// Verify device was persisted
	stored, err := deviceRepo.GetByID(device.ID)
	if err != nil {
		t.Fatalf("device not persisted: %v", err)
	}
	if stored.IP != "192.168.1.1" {
		t.Errorf("expected IP 192.168.1.1, got %s", stored.IP)
	}
}

func TestAddDevice_DerivesPollClassFromDeviceType(t *testing.T) {
	snmpCalled := false
	deviceRepo := newMockDeviceRepo()
	linkRepo := newMockLinkRepo()
	settingsRepo := newMockSettingsRepo()

	discoverFn := func(target string, creds domain.SNMPCredentials) (*snmp.DiscoveryResult, error) {
		snmpCalled = true
		return nil, fmt.Errorf("should not be called")
	}

	svc := NewDeviceService(deviceRepo, linkRepo, settingsRepo, discoverFn, nil)

	device, err := svc.AddDevice(context.Background(), "10.0.9.254", "router1",
		domain.DeviceTypeRouter,
		domain.SNMPCredentials{
			Version: domain.SNMPVersionV2c,
			V2c:     &domain.SNMPv2cCredentials{Community: "public"},
		}, nil, "", domain.MetricsSourcePrometheus, "instance", "10.0.9.254", nil)
	if err != nil {
		t.Fatalf("AddDevice failed: %v", err)
	}

	if device.PollClass != domain.PollClassCore {
		t.Fatalf("expected returned device PollClass core, got %s", device.PollClass)
	}

	svc.WaitForProbes()

	if snmpCalled {
		t.Error("discoverFunc was called for a Prometheus device")
	}

	stored, err := deviceRepo.GetByID(device.ID)
	if err != nil {
		t.Fatalf("GetByID failed: %v", err)
	}
	if stored.PollClass != domain.PollClassCore {
		t.Errorf("expected stored device PollClass core, got %s", stored.PollClass)
	}
}

func TestProbeCompletes_DeviceStatusUp(t *testing.T) {
	result := &snmp.DiscoveryResult{
		SysName:       "core-router",
		SysDescr:      "RouterOS RB5009",
		SysObjectID:   ".1.3.6.1.4.1.14988",
		HardwareModel: "RB5009",
		DeviceType:    domain.DeviceTypeRouter,
		Interfaces: []domain.Interface{
			{IfIndex: 1, IfName: "ether1", Speed: 1000000000, AdminStatus: "up", OperStatus: "up"},
		},
	}

	svc, deviceRepo, _ := newTestService(result, nil)

	device, err := svc.AddDevice(context.Background(), "10.0.0.1", "core-router",
		domain.DeviceTypeUnknown,
		domain.SNMPCredentials{
			Version: domain.SNMPVersionV2c,
			V2c:     &domain.SNMPv2cCredentials{Community: "public"},
		}, nil, "", domain.MetricsSourceSNMP, "", "", nil)
	if err != nil {
		t.Fatalf("AddDevice failed: %v", err)
	}

	// Wait for async probe to complete
	svc.WaitForProbes()

	updated, err := deviceRepo.GetByID(device.ID)
	if err != nil {
		t.Fatalf("GetByID failed: %v", err)
	}
	if updated.Status != domain.DeviceStatusUp {
		t.Errorf("expected status up, got %s", updated.Status)
	}
	if updated.SysName != "core-router" {
		t.Errorf("expected sysName core-router, got %s", updated.SysName)
	}
	if len(updated.Interfaces) != 1 {
		t.Errorf("expected 1 interface, got %d", len(updated.Interfaces))
	}
}

func TestProbeFails_DeviceStatusDown(t *testing.T) {
	svc, deviceRepo, _ := newTestService(nil, fmt.Errorf("SNMP timeout"))

	device, err := svc.AddDevice(context.Background(), "10.0.0.2", "",
		domain.DeviceTypeUnknown,
		domain.SNMPCredentials{
			Version: domain.SNMPVersionV2c,
			V2c:     &domain.SNMPv2cCredentials{Community: "public"},
		}, nil, "", domain.MetricsSourceSNMP, "", "", nil)
	if err != nil {
		t.Fatalf("AddDevice failed: %v", err)
	}

	svc.WaitForProbes()

	updated, err := deviceRepo.GetByID(device.ID)
	if err != nil {
		t.Fatalf("GetByID failed: %v", err)
	}
	if updated.Status != domain.DeviceStatusDown {
		t.Errorf("expected status down, got %s", updated.Status)
	}
}

func TestProbeDevice_SchedulesDelayedLLDPReprobeForIncompleteNewLinks(t *testing.T) {
	deviceRepo := newMockDeviceRepo()
	linkRepo := newMockLinkRepo()
	settingsRepo := newMockSettingsRepo()

	remoteDevice := &domain.Device{
		ID:            uuid.New(),
		Hostname:      "distribution-01",
		IP:            "192.0.2.60",
		SysName:       "distribution-01",
		Status:        domain.DeviceStatusUp,
		Managed:       true,
		MetricsSource: domain.MetricsSourceSNMP,
	}
	if err := deviceRepo.Create(remoteDevice); err != nil {
		t.Fatalf("Create remote device failed: %v", err)
	}

	probeTarget := &domain.Device{
		ID:            uuid.New(),
		Hostname:      "edge-01",
		IP:            "192.0.2.50",
		Status:        domain.DeviceStatusProbing,
		Managed:       true,
		MetricsSource: domain.MetricsSourceSNMP,
		SNMPCredentials: domain.SNMPCredentials{
			Version: domain.SNMPVersionV2c,
			V2c:     &domain.SNMPv2cCredentials{Community: "public"},
		},
	}
	if err := deviceRepo.Create(probeTarget); err != nil {
		t.Fatalf("Create probe target failed: %v", err)
	}

	discoverFn := func(target string, creds domain.SNMPCredentials) (*snmp.DiscoveryResult, error) {
		return &snmp.DiscoveryResult{
			SysName:       "edge-01",
			SysDescr:      "RouterOS",
			SysObjectID:   ".1.3.6.1.4.1.14988.1",
			HardwareModel: "RB5009",
			Vendor:        "mikrotik",
			DeviceType:    domain.DeviceTypeRouter,
			Neighbors: []snmp.NeighborInfo{
				{
					RemoteSysName: remoteDevice.SysName,
					RemotePortID:  "ether8",
					LocalIfName:   "",
					Protocol:      domain.DiscoveryProtocolLLDP,
				},
			},
		}, nil
	}

	svc := NewDeviceService(deviceRepo, linkRepo, settingsRepo, discoverFn, nil)
	var scheduledDelays []time.Duration
	reprobeCalls := make(map[uuid.UUID]int)
	svc.scheduleFunc = func(delay time.Duration, fn func()) {
		scheduledDelays = append(scheduledDelays, delay)
		fn()
	}
	svc.delayedReprobe = func(ctx context.Context, id uuid.UUID) error {
		reprobeCalls[id]++
		return nil
	}

	svc.probeDevice(probeTarget)

	if len(scheduledDelays) != 2 {
		t.Fatalf("expected delayed reprobes for local device and peer, got %d schedules", len(scheduledDelays))
	}
	for _, delay := range scheduledDelays {
		if delay != svc.reprobeDelay {
			t.Fatalf("scheduled delay = %v, want %v", delay, svc.reprobeDelay)
		}
	}
	if reprobeCalls[probeTarget.ID] != 1 {
		t.Fatalf("expected one delayed reprobe for probe target, got %d", reprobeCalls[probeTarget.ID])
	}
	if reprobeCalls[remoteDevice.ID] != 1 {
		t.Fatalf("expected one delayed reprobe for remote device, got %d", reprobeCalls[remoteDevice.ID])
	}

	updated, err := deviceRepo.GetByID(probeTarget.ID)
	if err != nil {
		t.Fatalf("GetByID failed: %v", err)
	}
	if updated.Status != domain.DeviceStatusUp {
		t.Fatalf("expected probe target status up, got %s", updated.Status)
	}
}

func TestProbeDevice_DoesNotScheduleDelayedLLDPReprobeForOlderDevices(t *testing.T) {
	deviceRepo := newMockDeviceRepo()
	linkRepo := newMockLinkRepo()
	settingsRepo := newMockSettingsRepo()

	remoteDevice := &domain.Device{
		ID:            uuid.New(),
		Hostname:      "distribution-01",
		IP:            "192.0.2.61",
		SysName:       "distribution-01",
		Status:        domain.DeviceStatusUp,
		Managed:       true,
		MetricsSource: domain.MetricsSourceSNMP,
	}
	if err := deviceRepo.Create(remoteDevice); err != nil {
		t.Fatalf("Create remote device failed: %v", err)
	}

	probeTarget := &domain.Device{
		ID:            uuid.New(),
		Hostname:      "edge-01",
		IP:            "192.0.2.51",
		Status:        domain.DeviceStatusProbing,
		Managed:       true,
		MetricsSource: domain.MetricsSourceSNMP,
		SNMPCredentials: domain.SNMPCredentials{
			Version: domain.SNMPVersionV2c,
			V2c:     &domain.SNMPv2cCredentials{Community: "public"},
		},
	}
	if err := deviceRepo.Create(probeTarget); err != nil {
		t.Fatalf("Create probe target failed: %v", err)
	}

	stored, err := deviceRepo.GetByID(probeTarget.ID)
	if err != nil {
		t.Fatalf("GetByID failed: %v", err)
	}
	stored.CreatedAt = time.Now().Add(-(incompleteLinkReprobeWindow + time.Minute))
	if err := deviceRepo.Update(stored); err != nil {
		t.Fatalf("Update older probe target failed: %v", err)
	}

	discoverFn := func(target string, creds domain.SNMPCredentials) (*snmp.DiscoveryResult, error) {
		return &snmp.DiscoveryResult{
			SysName:       "edge-01",
			SysDescr:      "RouterOS",
			SysObjectID:   ".1.3.6.1.4.1.14988.1",
			HardwareModel: "RB5009",
			Vendor:        "mikrotik",
			DeviceType:    domain.DeviceTypeRouter,
			Neighbors: []snmp.NeighborInfo{
				{
					RemoteSysName: remoteDevice.SysName,
					RemotePortID:  "ether8",
					LocalIfName:   "",
					Protocol:      domain.DiscoveryProtocolLLDP,
				},
			},
		}, nil
	}

	svc := NewDeviceService(deviceRepo, linkRepo, settingsRepo, discoverFn, nil)
	scheduled := false
	svc.scheduleFunc = func(delay time.Duration, fn func()) {
		scheduled = true
	}
	svc.delayedReprobe = func(ctx context.Context, id uuid.UUID) error {
		t.Fatal("delayed reprobe should not run for older devices")
		return nil
	}

	svc.probeDevice(probeTarget)

	if scheduled {
		t.Fatal("expected no delayed LLDP reprobe to be scheduled for older devices")
	}
}

func TestProbeDiscoversNeighbors_SkipsUnmatchedNeighbors(t *testing.T) {
	result := &snmp.DiscoveryResult{
		SysName:    "switch1",
		DeviceType: domain.DeviceTypeSwitch,
		Interfaces: []domain.Interface{
			{IfIndex: 1, IfName: "ether1"},
		},
		Neighbors: []snmp.NeighborInfo{
			{
				RemoteChassisID: "aa:bb:cc:dd:ee:ff",
				RemotePortID:    "ether2",
				RemoteSysName:   "switch2",
				LocalIfIndex:    1,
				LocalIfName:     "ether1",
				Protocol:        domain.DiscoveryProtocolLLDP,
			},
		},
	}

	svc, deviceRepo, linkRepo := newTestService(result, nil)

	_, err := svc.AddDevice(context.Background(), "10.0.0.1", "switch1",
		domain.DeviceTypeUnknown,
		domain.SNMPCredentials{
			Version: domain.SNMPVersionV2c,
			V2c:     &domain.SNMPv2cCredentials{Community: "public"},
		}, nil, "", "", "", "", nil)
	if err != nil {
		t.Fatalf("AddDevice failed: %v", err)
	}

	svc.WaitForProbes()

	// The current implementation skips neighbors not already in the system
	// (no placeholder creation), so we expect only the 1 managed device.
	devices, err := deviceRepo.GetAll()
	if err != nil {
		t.Fatalf("GetAll failed: %v", err)
	}
	if len(devices) != 1 {
		t.Fatalf("expected 1 device (neighbor skipped), got %d", len(devices))
	}

	// No link created since neighbor device doesn't exist
	links, err := linkRepo.GetAll()
	if err != nil {
		t.Fatalf("GetAll links failed: %v", err)
	}
	if len(links) != 0 {
		t.Fatalf("expected 0 links (neighbor not in system), got %d", len(links))
	}
}

func TestProbeDiscoversNeighbor_ExistingIP_NoDuplicate(t *testing.T) {
	result := &snmp.DiscoveryResult{
		SysName:    "router1",
		DeviceType: domain.DeviceTypeRouter,
		Interfaces: []domain.Interface{
			{IfIndex: 1, IfName: "ether1"},
		},
		Neighbors: []snmp.NeighborInfo{
			{
				RemoteSysName: "router2",
				LocalIfIndex:  1,
				LocalIfName:   "ether1",
				Protocol:      domain.DiscoveryProtocolLLDP,
			},
		},
	}

	svc, deviceRepo, _ := newTestService(result, nil)

	// Pre-create a device that will appear as a neighbor by sysName
	existingDevice := &domain.Device{
		ID:       uuid.New(),
		Hostname: "router2",
		SysName:  "router2",
		IP:       "10.0.0.99",
		Managed:  true,
		Status:   domain.DeviceStatusUp,
	}
	deviceRepo.Create(existingDevice)

	_, err := svc.AddDevice(context.Background(), "10.0.0.1", "router1",
		domain.DeviceTypeUnknown,
		domain.SNMPCredentials{
			Version: domain.SNMPVersionV2c,
			V2c:     &domain.SNMPv2cCredentials{Community: "public"},
		}, nil, "", "", "", "", nil)
	if err != nil {
		t.Fatalf("AddDevice failed: %v", err)
	}

	svc.WaitForProbes()

	devices, err := deviceRepo.GetAll()
	if err != nil {
		t.Fatalf("GetAll failed: %v", err)
	}
	// Should be 2 (the pre-existing + the new one), NOT 3 (no duplicate for neighbor)
	if len(devices) != 2 {
		t.Errorf("expected 2 devices (no duplicate neighbor), got %d", len(devices))
	}
}

func TestProbeDiscoversNeighbors_PrefersLLDPOverCDPForSameLink(t *testing.T) {
	result := &snmp.DiscoveryResult{
		SysName:    "switch1",
		DeviceType: domain.DeviceTypeSwitch,
		Interfaces: []domain.Interface{{IfIndex: 1, IfName: "ether1"}},
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
				RemoteSysName: "switch2-cdp",
				RemotePortID:  "ether2",
				LocalIfIndex:  1,
				LocalIfName:   "ether1",
				Protocol:      domain.DiscoveryProtocolCDP,
			},
		},
	}

	svc, deviceRepo, linkRepo := newTestService(result, nil)

	remote := &domain.Device{
		ID:       uuid.New(),
		Hostname: "switch2",
		SysName:  "switch2",
		IP:       "10.0.0.2",
		Managed:  true,
		Status:   domain.DeviceStatusUp,
	}
	if err := deviceRepo.Create(remote); err != nil {
		t.Fatalf("failed to create remote device: %v", err)
	}

	_, err := svc.AddDevice(context.Background(), "10.0.0.1", "switch1",
		domain.DeviceTypeUnknown,
		domain.SNMPCredentials{
			Version: domain.SNMPVersionV2c,
			V2c:     &domain.SNMPv2cCredentials{Community: "public"},
		}, nil, "", domain.MetricsSourceSNMP, "", "", nil)
	if err != nil {
		t.Fatalf("AddDevice failed: %v", err)
	}

	svc.WaitForProbes()

	links, err := linkRepo.GetAll()
	if err != nil {
		t.Fatalf("GetAll links failed: %v", err)
	}
	if len(links) != 1 {
		t.Fatalf("expected 1 link from matched LLDP neighbor, got %d", len(links))
	}
	if links[0].DiscoveryProtocol != domain.DiscoveryProtocolLLDP {
		t.Fatalf("expected LLDP link to win, got %s", links[0].DiscoveryProtocol)
	}
}

func TestProbeDiscoversNeighbors_UsesCDPWhenLLDPFieldsAreMissing(t *testing.T) {
	result := &snmp.DiscoveryResult{
		SysName:    "switch1",
		DeviceType: domain.DeviceTypeSwitch,
		Interfaces: []domain.Interface{{IfIndex: 1, IfName: "ether1"}},
		Neighbors: []snmp.NeighborInfo{
			{
				RemoteSysName:   "",
				RemotePortID:    "",
				LocalIfIndex:    1,
				LocalIfName:     "ether1",
				Protocol:        domain.DiscoveryProtocolLLDP,
				RemoteChassisID: "aa:bb:cc:dd:ee:ff",
			},
			{
				RemoteSysName: "switch2",
				RemotePortID:  "ether2",
				LocalIfIndex:  1,
				LocalIfName:   "ether1",
				Protocol:      domain.DiscoveryProtocolCDP,
			},
		},
	}

	svc, deviceRepo, linkRepo := newTestService(result, nil)

	remote := &domain.Device{
		ID:       uuid.New(),
		Hostname: "switch2",
		SysName:  "switch2",
		IP:       "10.0.0.2",
		Managed:  true,
		Status:   domain.DeviceStatusUp,
	}
	if err := deviceRepo.Create(remote); err != nil {
		t.Fatalf("failed to create remote device: %v", err)
	}

	_, err := svc.AddDevice(context.Background(), "10.0.0.1", "switch1",
		domain.DeviceTypeUnknown,
		domain.SNMPCredentials{
			Version: domain.SNMPVersionV2c,
			V2c:     &domain.SNMPv2cCredentials{Community: "public"},
		}, nil, "", domain.MetricsSourceSNMP, "", "", nil)
	if err != nil {
		t.Fatalf("AddDevice failed: %v", err)
	}

	svc.WaitForProbes()

	links, err := linkRepo.GetAll()
	if err != nil {
		t.Fatalf("GetAll links failed: %v", err)
	}
	if len(links) != 1 {
		t.Fatalf("expected 1 link from CDP gap fill, got %d", len(links))
	}
	if links[0].DiscoveryProtocol != domain.DiscoveryProtocolCDP {
		t.Fatalf("expected CDP link when LLDP lacks matchable data, got %s", links[0].DiscoveryProtocol)
	}
	if links[0].TargetIfName != "ether2" {
		t.Fatalf("expected target interface ether2, got %q", links[0].TargetIfName)
	}
}

func TestProbeDiscoversNeighbors_PrefersPhysicalLinkOverVirtualVariant(t *testing.T) {
	result := &snmp.DiscoveryResult{
		SysName:    "switch1",
		DeviceType: domain.DeviceTypeSwitch,
		Interfaces: []domain.Interface{{IfIndex: 1, IfName: "ether2-verso-border-botte"}},
		Neighbors: []snmp.NeighborInfo{
			{
				RemoteSysName:   "switch2",
				RemotePortID:    "VLAN-99-MGMT-ETH6",
				LocalIfIndex:    1,
				LocalIfName:     "",
				Protocol:        domain.DiscoveryProtocolLLDP,
				RemoteChassisID: "aa:bb:cc:dd:ee:ff",
			},
			{
				RemoteSysName:   "switch2",
				RemotePortID:    "ether6-link_new_apparati",
				LocalIfIndex:    1,
				LocalIfName:     "ether2-verso-border-botte",
				Protocol:        domain.DiscoveryProtocolLLDP,
				RemoteChassisID: "aa:bb:cc:dd:ee:ff",
			},
			{
				RemoteSysName:   "switch2",
				RemotePortID:    "ether6-link_new_apparati",
				LocalIfIndex:    1,
				LocalIfName:     "",
				Protocol:        domain.DiscoveryProtocolLLDP,
				RemoteChassisID: "aa:bb:cc:dd:ee:ff",
			},
		},
	}

	svc, deviceRepo, linkRepo := newTestService(result, nil)

	remote := &domain.Device{
		ID:       uuid.New(),
		Hostname: "switch2",
		SysName:  "switch2",
		IP:       "10.0.0.2",
		Managed:  true,
		Status:   domain.DeviceStatusUp,
	}
	if err := deviceRepo.Create(remote); err != nil {
		t.Fatalf("failed to create remote device: %v", err)
	}

	_, err := svc.AddDevice(context.Background(), "10.0.0.1", "switch1",
		domain.DeviceTypeUnknown,
		domain.SNMPCredentials{
			Version: domain.SNMPVersionV2c,
			V2c:     &domain.SNMPv2cCredentials{Community: "public"},
		}, nil, "", domain.MetricsSourceSNMP, "", "", nil)
	if err != nil {
		t.Fatalf("AddDevice failed: %v", err)
	}

	svc.WaitForProbes()

	links, err := linkRepo.GetAll()
	if err != nil {
		t.Fatalf("GetAll links failed: %v", err)
	}
	if len(links) != 1 {
		t.Fatalf("expected 1 preferred link, got %d", len(links))
	}
	if links[0].SourceIfName != "ether2-verso-border-botte" {
		t.Fatalf("expected physical source interface to win, got %q", links[0].SourceIfName)
	}
	if links[0].TargetIfName != "ether6-link_new_apparati" {
		t.Fatalf("expected physical target interface to win, got %q", links[0].TargetIfName)
	}
}

func TestDedupePreferredDiscoveredNeighbors_PrefersPhysicalRemotePortWhenLocalInterfaceIsMissingOnAllVariants(t *testing.T) {
	neighbors := dedupePreferredDiscoveredNeighbors([]snmp.NeighborInfo{
		{
			RemoteSysName:   "PRE-M-PZ-GALLITELLO_DORSALE",
			RemotePortID:    "br_eoip_radius_vlan/eoip_gallitello_uff",
			LocalIfName:     "",
			Protocol:        domain.DiscoveryProtocolLLDP,
			RemoteChassisID: "aa:bb:cc:dd:ee:ff",
		},
		{
			RemoteSysName:   "PRE-M-PZ-GALLITELLO_DORSALE",
			RemotePortID:    "ether6-Link_Ufficio",
			LocalIfName:     "",
			Protocol:        domain.DiscoveryProtocolLLDP,
			RemoteChassisID: "aa:bb:cc:dd:ee:ff",
		},
	})

	if len(neighbors) != 1 {
		t.Fatalf("expected only the physical neighbor to remain, got %d", len(neighbors))
	}
	if neighbors[0].RemotePortID != "ether6-Link_Ufficio" {
		t.Fatalf("expected physical remote port to survive, got %q", neighbors[0].RemotePortID)
	}
}

func TestUpdateDevice_ChangesFieldsWithoutReprobing(t *testing.T) {
	svc, deviceRepo, _ := newTestService(&snmp.DiscoveryResult{}, nil)

	device := &domain.Device{
		ID:       uuid.New(),
		IP:       "10.0.0.1",
		Hostname: "old-name",
		Managed:  true,
		Status:   domain.DeviceStatusUp,
		Tags:     map[string]string{"site": "dc1"},
		SNMPCredentials: domain.SNMPCredentials{
			Version: domain.SNMPVersionV2c,
			V2c:     &domain.SNMPv2cCredentials{Community: "public"},
		},
	}
	deviceRepo.Create(device)

	err := svc.UpdateDevice(context.Background(), device.ID, DeviceUpdate{
		Hostname: strPtr("new-name"),
		Tags:     &map[string]string{"site": "dc2"},
	})
	if err != nil {
		t.Fatalf("UpdateDevice failed: %v", err)
	}

	updated, _ := deviceRepo.GetByID(device.ID)
	if updated.Hostname != "new-name" {
		t.Errorf("expected hostname new-name, got %s", updated.Hostname)
	}
	if updated.Tags["site"] != "dc2" {
		t.Errorf("expected tag site=dc2, got %s", updated.Tags["site"])
	}
}

func TestUpdateDevice_VirtualNoIPNormalizesStatusAndMetricsSource(t *testing.T) {
	svc, deviceRepo, _ := newTestService(&snmp.DiscoveryResult{}, nil)

	device := &domain.Device{
		ID:                   uuid.New(),
		IP:                   "10.0.0.99",
		Hostname:             "support-node",
		DeviceType:           domain.DeviceTypeVirtual,
		Managed:              true,
		Status:               domain.DeviceStatusDown,
		MetricsSource:        domain.MetricsSourcePrometheus,
		PrometheusLabelName:  "instance",
		PrometheusLabelValue: "10.0.0.99",
	}
	if err := deviceRepo.Create(device); err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	if err := svc.UpdateDevice(context.Background(), device.ID, DeviceUpdate{
		IP: strPtr(""),
	}); err != nil {
		t.Fatalf("UpdateDevice failed: %v", err)
	}

	updated, err := deviceRepo.GetByID(device.ID)
	if err != nil {
		t.Fatalf("GetByID failed: %v", err)
	}
	if updated.IP != "" {
		t.Fatalf("expected empty IP, got %q", updated.IP)
	}
	if updated.Status != domain.DeviceStatusUnknown {
		t.Fatalf("expected status unknown after removing IP from virtual device, got %s", updated.Status)
	}
	if updated.MetricsSource != domain.MetricsSourceNone {
		t.Fatalf("expected metrics source none after removing IP from virtual device, got %s", updated.MetricsSource)
	}
}

func TestUpdateDevice_PollIntervalOverrideTriState(t *testing.T) {
	svc, deviceRepo, _ := newTestService(&snmp.DiscoveryResult{}, nil)

	device := &domain.Device{
		ID:                   uuid.New(),
		IP:                   "10.0.0.1",
		Hostname:             "router1",
		Managed:              true,
		Status:               domain.DeviceStatusUp,
		PollClass:            domain.PollClassCore,
		PollIntervalOverride: intPtr(15),
		SNMPCredentials: domain.SNMPCredentials{
			Version: domain.SNMPVersionV2c,
			V2c:     &domain.SNMPv2cCredentials{Community: "public"},
		},
	}
	if err := deviceRepo.Create(device); err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	if err := svc.UpdateDevice(context.Background(), device.ID, DeviceUpdate{}); err != nil {
		t.Fatalf("UpdateDevice keep failed: %v", err)
	}

	unchanged, err := deviceRepo.GetByID(device.ID)
	if err != nil {
		t.Fatalf("GetByID failed after keep: %v", err)
	}
	if unchanged.PollIntervalOverride == nil || *unchanged.PollIntervalOverride != 15 {
		t.Fatalf("expected keep to preserve override=15, got %#v", unchanged.PollIntervalOverride)
	}

	var cleared *int
	if err := svc.UpdateDevice(context.Background(), device.ID, DeviceUpdate{
		PollIntervalOverride: &cleared,
	}); err != nil {
		t.Fatalf("UpdateDevice clear failed: %v", err)
	}

	clearedDevice, err := deviceRepo.GetByID(device.ID)
	if err != nil {
		t.Fatalf("GetByID failed after clear: %v", err)
	}
	if clearedDevice.PollIntervalOverride != nil {
		t.Fatalf("expected clear to remove override, got %d", *clearedDevice.PollIntervalOverride)
	}

	newOverride := intPtr(30)
	if err := svc.UpdateDevice(context.Background(), device.ID, DeviceUpdate{
		PollIntervalOverride: &newOverride,
	}); err != nil {
		t.Fatalf("UpdateDevice set failed: %v", err)
	}

	updated, err := deviceRepo.GetByID(device.ID)
	if err != nil {
		t.Fatalf("GetByID failed after set: %v", err)
	}
	if updated.PollIntervalOverride == nil || *updated.PollIntervalOverride != 30 {
		t.Fatalf("expected set to store override=30, got %#v", updated.PollIntervalOverride)
	}
}

func TestUpdateDevice_NotesTriState(t *testing.T) {
	svc, deviceRepo, _ := newTestService(&snmp.DiscoveryResult{}, nil)

	initialNotes := "Primary uplink near MDF"
	device := &domain.Device{
		ID:       uuid.New(),
		IP:       "10.0.0.11",
		Hostname: "router-notes",
		Notes:    &initialNotes,
		Managed:  true,
		Status:   domain.DeviceStatusUp,
		SNMPCredentials: domain.SNMPCredentials{
			Version: domain.SNMPVersionV2c,
			V2c:     &domain.SNMPv2cCredentials{Community: "public"},
		},
	}
	if err := deviceRepo.Create(device); err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	if err := svc.UpdateDevice(context.Background(), device.ID, DeviceUpdate{}); err != nil {
		t.Fatalf("UpdateDevice keep failed: %v", err)
	}
	unchanged, err := deviceRepo.GetByID(device.ID)
	if err != nil {
		t.Fatalf("GetByID failed after keep: %v", err)
	}
	if unchanged.Notes == nil || *unchanged.Notes != initialNotes {
		t.Fatalf("expected keep to preserve notes, got %#v", unchanged.Notes)
	}

	updatedNotes := strPtr("  Replace PSU during next visit  ")
	if err := svc.UpdateDevice(context.Background(), device.ID, DeviceUpdate{
		Notes: &updatedNotes,
	}); err != nil {
		t.Fatalf("UpdateDevice set failed: %v", err)
	}
	stored, err := deviceRepo.GetByID(device.ID)
	if err != nil {
		t.Fatalf("GetByID failed after set: %v", err)
	}
	if stored.Notes == nil || *stored.Notes != "Replace PSU during next visit" {
		t.Fatalf("expected trimmed notes after set, got %#v", stored.Notes)
	}

	blankNotes := strPtr("   ")
	if err := svc.UpdateDevice(context.Background(), device.ID, DeviceUpdate{
		Notes: &blankNotes,
	}); err != nil {
		t.Fatalf("UpdateDevice blank failed: %v", err)
	}
	cleared, err := deviceRepo.GetByID(device.ID)
	if err != nil {
		t.Fatalf("GetByID failed after blank clear: %v", err)
	}
	if cleared.Notes != nil {
		t.Fatalf("expected blank notes to clear field, got %#v", cleared.Notes)
	}

	var nilNotes *string
	if err := svc.UpdateDevice(context.Background(), device.ID, DeviceUpdate{
		Notes: &nilNotes,
	}); err != nil {
		t.Fatalf("UpdateDevice nil clear failed: %v", err)
	}
	clearedAgain, err := deviceRepo.GetByID(device.ID)
	if err != nil {
		t.Fatalf("GetByID failed after nil clear: %v", err)
	}
	if clearedAgain.Notes != nil {
		t.Fatalf("expected nil notes to remain cleared, got %#v", clearedAgain.Notes)
	}
}

func TestUpdateDevice_PollIntervalOverrideTriggersSchedulerRedueOnChange(t *testing.T) {
	t.Run("set from nil", func(t *testing.T) {
		svc, deviceRepo, _ := newTestService(&snmp.DiscoveryResult{}, nil)
		rescheduler := &fakePollRescheduler{}
		svc.SetPollRescheduler(rescheduler)

		device := &domain.Device{
			ID:        uuid.New(),
			IP:        "10.0.1.1",
			Hostname:  "router-nil",
			Managed:   true,
			Status:    domain.DeviceStatusUp,
			PollClass: domain.PollClassCore,
		}
		if err := deviceRepo.Create(device); err != nil {
			t.Fatalf("Create failed: %v", err)
		}

		override := intPtr(15)
		if err := svc.UpdateDevice(context.Background(), device.ID, DeviceUpdate{
			PollIntervalOverride: &override,
		}); err != nil {
			t.Fatalf("UpdateDevice failed: %v", err)
		}

		if got := len(rescheduler.calls); got != 1 {
			t.Fatalf("redue call count = %d, want 1", got)
		}
		if rescheduler.calls[0].device.PollIntervalOverride == nil || *rescheduler.calls[0].device.PollIntervalOverride != 15 {
			t.Fatalf("rescheduled override = %#v, want 15", rescheduler.calls[0].device.PollIntervalOverride)
		}
		if rescheduler.calls[0].changedAt.IsZero() || rescheduler.calls[0].changedAt.Location() != time.UTC {
			t.Fatalf("changedAt = %v, want non-zero UTC timestamp", rescheduler.calls[0].changedAt)
		}
	})

	t.Run("change existing value", func(t *testing.T) {
		svc, deviceRepo, _ := newTestService(&snmp.DiscoveryResult{}, nil)
		rescheduler := &fakePollRescheduler{}
		svc.SetPollRescheduler(rescheduler)

		device := &domain.Device{
			ID:                   uuid.New(),
			IP:                   "10.0.1.2",
			Hostname:             "router-old",
			Managed:              true,
			Status:               domain.DeviceStatusUp,
			PollClass:            domain.PollClassCore,
			PollIntervalOverride: intPtr(30),
		}
		if err := deviceRepo.Create(device); err != nil {
			t.Fatalf("Create failed: %v", err)
		}

		override := intPtr(20)
		if err := svc.UpdateDevice(context.Background(), device.ID, DeviceUpdate{
			PollIntervalOverride: &override,
		}); err != nil {
			t.Fatalf("UpdateDevice failed: %v", err)
		}

		if got := len(rescheduler.calls); got != 1 {
			t.Fatalf("redue call count = %d, want 1", got)
		}
		if rescheduler.calls[0].device.PollIntervalOverride == nil || *rescheduler.calls[0].device.PollIntervalOverride != 20 {
			t.Fatalf("rescheduled override = %#v, want 20", rescheduler.calls[0].device.PollIntervalOverride)
		}
	})

	t.Run("clear existing override", func(t *testing.T) {
		svc, deviceRepo, _ := newTestService(&snmp.DiscoveryResult{}, nil)
		rescheduler := &fakePollRescheduler{}
		svc.SetPollRescheduler(rescheduler)

		device := &domain.Device{
			ID:                   uuid.New(),
			IP:                   "10.0.1.3",
			Hostname:             "router-clear",
			Managed:              true,
			Status:               domain.DeviceStatusUp,
			PollClass:            domain.PollClassCore,
			PollIntervalOverride: intPtr(25),
		}
		if err := deviceRepo.Create(device); err != nil {
			t.Fatalf("Create failed: %v", err)
		}

		var cleared *int
		if err := svc.UpdateDevice(context.Background(), device.ID, DeviceUpdate{
			PollIntervalOverride: &cleared,
		}); err != nil {
			t.Fatalf("UpdateDevice failed: %v", err)
		}

		if got := len(rescheduler.calls); got != 1 {
			t.Fatalf("redue call count = %d, want 1", got)
		}
		if rescheduler.calls[0].device.PollIntervalOverride != nil {
			t.Fatalf("rescheduled override = %#v, want nil", rescheduler.calls[0].device.PollIntervalOverride)
		}
	})
}

func TestUpdateDevice_PollIntervalOverrideDoesNotTriggerSchedulerRedueWhenUnchanged(t *testing.T) {
	t.Run("omit preserves existing override without redue", func(t *testing.T) {
		svc, deviceRepo, _ := newTestService(&snmp.DiscoveryResult{}, nil)
		rescheduler := &fakePollRescheduler{}
		svc.SetPollRescheduler(rescheduler)

		device := &domain.Device{
			ID:                   uuid.New(),
			IP:                   "10.0.2.1",
			Hostname:             "router-keep",
			Managed:              true,
			Status:               domain.DeviceStatusUp,
			PollClass:            domain.PollClassCore,
			PollIntervalOverride: intPtr(15),
		}
		if err := deviceRepo.Create(device); err != nil {
			t.Fatalf("Create failed: %v", err)
		}

		if err := svc.UpdateDevice(context.Background(), device.ID, DeviceUpdate{}); err != nil {
			t.Fatalf("UpdateDevice failed: %v", err)
		}

		if got := len(rescheduler.calls); got != 0 {
			t.Fatalf("redue call count = %d, want 0", got)
		}
	})

	t.Run("same value override is a no-op", func(t *testing.T) {
		svc, deviceRepo, _ := newTestService(&snmp.DiscoveryResult{}, nil)
		rescheduler := &fakePollRescheduler{}
		svc.SetPollRescheduler(rescheduler)

		device := &domain.Device{
			ID:                   uuid.New(),
			IP:                   "10.0.2.2",
			Hostname:             "router-same",
			Managed:              true,
			Status:               domain.DeviceStatusUp,
			PollClass:            domain.PollClassCore,
			PollIntervalOverride: intPtr(30),
		}
		if err := deviceRepo.Create(device); err != nil {
			t.Fatalf("Create failed: %v", err)
		}

		override := intPtr(30)
		if err := svc.UpdateDevice(context.Background(), device.ID, DeviceUpdate{
			PollIntervalOverride: &override,
		}); err != nil {
			t.Fatalf("UpdateDevice failed: %v", err)
		}

		if got := len(rescheduler.calls); got != 0 {
			t.Fatalf("redue call count = %d, want 0", got)
		}
	})

	t.Run("unrelated update does not redue", func(t *testing.T) {
		svc, deviceRepo, _ := newTestService(&snmp.DiscoveryResult{}, nil)
		rescheduler := &fakePollRescheduler{}
		svc.SetPollRescheduler(rescheduler)

		device := &domain.Device{
			ID:        uuid.New(),
			IP:        "10.0.2.3",
			Hostname:  "router-name",
			Managed:   true,
			Status:    domain.DeviceStatusUp,
			PollClass: domain.PollClassCore,
		}
		if err := deviceRepo.Create(device); err != nil {
			t.Fatalf("Create failed: %v", err)
		}

		if err := svc.UpdateDevice(context.Background(), device.ID, DeviceUpdate{
			Hostname: strPtr("router-renamed"),
		}); err != nil {
			t.Fatalf("UpdateDevice failed: %v", err)
		}

		if got := len(rescheduler.calls); got != 0 {
			t.Fatalf("redue call count = %d, want 0", got)
		}
	})
}

func TestUpdateDevice_PollIntervalOverrideReduesNextPerformanceTask(t *testing.T) {
	deviceRepo := newMockDeviceRepo()
	linkRepo := newMockLinkRepo()
	settingsRepo := newMockSettingsRepo()
	svc := NewDeviceService(deviceRepo, linkRepo, settingsRepo, nil, nil)

	// Use a deterministic fixed UUID whose seeded offsets stay beyond the quiet window
	// plus a safety margin. That makes the first post-update task prove the override-
	// triggered path instead of an initial seed firing early.
	quietWindow := 150 * time.Millisecond
	seededOffsetFloor := quietWindow + 150*time.Millisecond
	deviceID := fixedSchedulerDeviceIDForQuietWindow(t, seededOffsetFloor)
	device := &domain.Device{
		ID:        deviceID,
		IP:        "10.0.3.1",
		Hostname:  "router-integration",
		Managed:   true,
		Status:    domain.DeviceStatusUp,
		PollClass: domain.PollClassCore,
	}
	if err := deviceRepo.Create(device); err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	sched := scheduler.NewScheduler(schedulerDeviceSource{repo: deviceRepo}, settingsRepo)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	sched.Start(ctx)
	defer sched.Stop()
	svc.SetPollRescheduler(sched)

	assertNoSchedulerTaskWithin(t, sched.Tasks(), quietWindow)

	beforeSave := time.Now().UTC()
	override := intPtr(12)
	if err := svc.UpdateDevice(context.Background(), device.ID, DeviceUpdate{
		PollIntervalOverride: &override,
	}); err != nil {
		t.Fatalf("UpdateDevice failed: %v", err)
	}
	afterSave := time.Now().UTC()

	task := waitForSchedulerTask(t, sched.Tasks(), time.Second)
	if task.Key != scheduler.NewTaskKey(deviceID, domain.VolatilityClassPerformance) {
		t.Fatalf("first post-update task key = %+v, want performance task for device %s", task.Key, deviceID)
	}
	if task.VolatilityClass != domain.VolatilityClassPerformance {
		t.Fatalf("first post-update task volatility = %q, want performance", task.VolatilityClass)
	}
	if task.ExpectedInterval != 12*time.Second {
		t.Fatalf("expected interval = %v, want 12s", task.ExpectedInterval)
	}
	if task.Device.PollIntervalOverride == nil || *task.Device.PollIntervalOverride != 12 {
		t.Fatalf("task override = %#v, want 12", task.Device.PollIntervalOverride)
	}
	if task.DueAt.Before(beforeSave) || task.DueAt.After(afterSave) {
		t.Fatalf("task dueAt = %v, want between save bounds [%v, %v]", task.DueAt, beforeSave, afterSave)
	}
}

func TestDeleteDevice_RemovesDeviceAndLinks(t *testing.T) {
	svc, deviceRepo, linkRepo := newTestService(&snmp.DiscoveryResult{}, nil)

	device := &domain.Device{
		ID:      uuid.New(),
		IP:      "10.0.0.1",
		Managed: true,
		Status:  domain.DeviceStatusUp,
		SNMPCredentials: domain.SNMPCredentials{
			Version: domain.SNMPVersionV2c,
			V2c:     &domain.SNMPv2cCredentials{Community: "public"},
		},
	}
	deviceRepo.Create(device)

	otherDevice := &domain.Device{
		ID:      uuid.New(),
		IP:      "10.0.0.2",
		Managed: true,
		Status:  domain.DeviceStatusUp,
	}
	deviceRepo.Create(otherDevice)

	link := &domain.Link{
		ID:                uuid.New(),
		SourceDeviceID:    device.ID,
		SourceIfName:      "ether1",
		TargetDeviceID:    otherDevice.ID,
		TargetIfName:      "ether2",
		DiscoveryProtocol: domain.DiscoveryProtocolLLDP,
	}
	linkRepo.Create(link)

	err := svc.DeleteDevice(context.Background(), device.ID)
	if err != nil {
		t.Fatalf("DeleteDevice failed: %v", err)
	}

	// Device should be gone
	_, err = deviceRepo.GetByID(device.ID)
	if err == nil {
		t.Error("expected device to be deleted")
	}

	// Links involving device should be gone
	links, _ := linkRepo.GetByDeviceID(device.ID)
	if len(links) != 0 {
		t.Errorf("expected 0 links for deleted device, got %d", len(links))
	}
}

func TestGetAllDevices_ReturnsAllWithInterfaces(t *testing.T) {
	svc, deviceRepo, _ := newTestService(&snmp.DiscoveryResult{}, nil)

	d1 := &domain.Device{
		ID: uuid.New(), IP: "10.0.0.1", Managed: true, Status: domain.DeviceStatusUp,
		Interfaces: []domain.Interface{
			{IfIndex: 1, IfName: "ether1"},
		},
	}
	d2 := &domain.Device{
		ID: uuid.New(), IP: "10.0.0.2", Managed: true, Status: domain.DeviceStatusUp,
	}
	deviceRepo.Create(d1)
	deviceRepo.Create(d2)

	devices, err := svc.GetAllDevices(context.Background())
	if err != nil {
		t.Fatalf("GetAllDevices failed: %v", err)
	}
	if len(devices) != 2 {
		t.Errorf("expected 2 devices, got %d", len(devices))
	}
}

func TestProbeDevice_ReprobeUpdatesFields(t *testing.T) {
	result := &snmp.DiscoveryResult{
		SysName:       "router1-updated",
		SysDescr:      "RouterOS RB5009",
		SysObjectID:   ".1.3.6.1.4.1.14988",
		HardwareModel: "RB5009",
		DeviceType:    domain.DeviceTypeRouter,
		Interfaces: []domain.Interface{
			{IfIndex: 1, IfName: "ether1"},
			{IfIndex: 2, IfName: "ether2"},
		},
	}

	svc, deviceRepo, _ := newTestService(result, nil)

	device := &domain.Device{
		ID:       uuid.New(),
		IP:       "10.0.0.1",
		Hostname: "router1",
		SysName:  "router1-old",
		Managed:  true,
		Status:   domain.DeviceStatusUp,
		SNMPCredentials: domain.SNMPCredentials{
			Version: domain.SNMPVersionV2c,
			V2c:     &domain.SNMPv2cCredentials{Community: "public"},
		},
		Interfaces: []domain.Interface{
			{IfIndex: 1, IfName: "ether1"},
		},
	}
	deviceRepo.Create(device)

	err := svc.ProbeDevice(context.Background(), device.ID)
	if err != nil {
		t.Fatalf("ProbeDevice failed: %v", err)
	}

	svc.WaitForProbes()

	updated, _ := deviceRepo.GetByID(device.ID)
	if updated.SysName != "router1-updated" {
		t.Errorf("expected sysName router1-updated, got %s", updated.SysName)
	}
	if len(updated.Interfaces) != 2 {
		t.Errorf("expected 2 interfaces, got %d", len(updated.Interfaces))
	}
}

// TestPrometheusDevice_SkipsSNMPProbe verifies that adding a device with
// MetricsSourcePrometheus does NOT call the gosnmp discovery function and
// immediately sets status to "up" without requiring SNMP credentials.
func TestPrometheusDevice_SkipsSNMPProbe(t *testing.T) {
	snmpCalled := false
	deviceRepo := newMockDeviceRepo()
	linkRepo := newMockLinkRepo()
	settingsRepo := newMockSettingsRepo()

	discoverFn := func(target string, creds domain.SNMPCredentials) (*snmp.DiscoveryResult, error) {
		snmpCalled = true
		return nil, fmt.Errorf("should not be called")
	}

	svc := NewDeviceService(deviceRepo, linkRepo, settingsRepo, discoverFn, nil)

	device, err := svc.AddDevice(context.Background(), "10.0.9.254", "",
		domain.DeviceTypeUnknown,
		// No meaningful SNMP credentials — user only wants Prometheus
		domain.SNMPCredentials{
			Version: domain.SNMPVersionV2c,
			V2c:     &domain.SNMPv2cCredentials{Community: "public"},
		}, nil, "", domain.MetricsSourcePrometheus, "instance", "10.0.9.254", nil)
	if err != nil {
		t.Fatalf("AddDevice failed: %v", err)
	}

	svc.WaitForProbes()

	if snmpCalled {
		t.Error("discoverFunc (gosnmp) was called for a Prometheus-sourced device — it should have been skipped")
	}

	updated, err := deviceRepo.GetByID(device.ID)
	if err != nil {
		t.Fatalf("GetByID failed: %v", err)
	}
	if updated.Status != domain.DeviceStatusUp {
		t.Errorf("expected status up for Prometheus device, got %s", updated.Status)
	}
}

// TestPrometheusDevice_SNMPv3WithPrivProtocol verifies the specific bug scenario:
// a device with MetricsSourcePrometheus and SNMPv3 authPriv credentials (including
// priv_protocol but no priv_password) does not cause a gosnmp connection error.
func TestPrometheusDevice_SNMPv3WithPrivProtocol(t *testing.T) {
	snmpCalled := false
	deviceRepo := newMockDeviceRepo()
	linkRepo := newMockLinkRepo()
	settingsRepo := newMockSettingsRepo()

	discoverFn := func(target string, creds domain.SNMPCredentials) (*snmp.DiscoveryResult, error) {
		snmpCalled = true
		return nil, fmt.Errorf("securityParameters.PrivacyPassphrase is required when a privacy protocol is specified")
	}

	svc := NewDeviceService(deviceRepo, linkRepo, settingsRepo, discoverFn, nil)

	// Simulate what happens when user picks "Prometheus without Fallback" but
	// previously had v3 credentials in the form with authPriv + empty priv_password.
	device, err := svc.AddDevice(context.Background(), "10.0.9.254", "",
		domain.DeviceTypeUnknown,
		domain.SNMPCredentials{
			Version: domain.SNMPVersionV3,
			V3: &domain.SNMPv3Credentials{
				Username:      "monitorUser",
				AuthProtocol:  "SHA",
				AuthPassword:  "authpass123",
				PrivProtocol:  "AES",
				PrivPassword:  "", // empty — bug trigger
				SecurityLevel: "authPriv",
			},
		}, nil, "", domain.MetricsSourcePrometheus, "instance", "10.0.9.254", nil)
	if err != nil {
		t.Fatalf("AddDevice failed: %v", err)
	}

	svc.WaitForProbes()

	if snmpCalled {
		t.Error("discoverFunc (gosnmp) was called — should have been skipped for MetricsSourcePrometheus")
	}

	updated, err := deviceRepo.GetByID(device.ID)
	if err != nil {
		t.Fatalf("GetByID failed: %v", err)
	}
	if updated.Status != domain.DeviceStatusUp {
		t.Errorf("expected status up, got %s", updated.Status)
	}
}

func TestProbeDevice_PrometheusReclassifiesPollClassWithoutOverride(t *testing.T) {
	snmpCalled := false
	deviceRepo := newMockDeviceRepo()
	linkRepo := newMockLinkRepo()
	settingsRepo := newMockSettingsRepo()

	discoverFn := func(target string, creds domain.SNMPCredentials) (*snmp.DiscoveryResult, error) {
		snmpCalled = true
		return nil, fmt.Errorf("should not be called")
	}

	svc := NewDeviceService(deviceRepo, linkRepo, settingsRepo, discoverFn, nil)

	device := &domain.Device{
		ID:            uuid.New(),
		IP:            "192.0.2.10",
		Hostname:      "router-prometheus",
		DeviceType:    domain.DeviceTypeRouter,
		PollClass:     "",
		Managed:       true,
		Status:        domain.DeviceStatusProbing,
		MetricsSource: domain.MetricsSourcePrometheus,
		SNMPCredentials: domain.SNMPCredentials{
			Version: domain.SNMPVersionV2c,
			V2c:     &domain.SNMPv2cCredentials{Community: "public"},
		},
	}
	if err := deviceRepo.Create(device); err != nil {
		t.Fatalf("failed to create device: %v", err)
	}

	svc.probeWg.Add(1)
	go func() {
		defer svc.probeWg.Done()
		svc.probeDevice(device)
	}()
	svc.WaitForProbes()

	if snmpCalled {
		t.Error("discoverFunc was called for a Prometheus device")
	}

	updated, err := deviceRepo.GetByID(device.ID)
	if err != nil {
		t.Fatalf("GetByID failed: %v", err)
	}
	if updated.Status != domain.DeviceStatusUp {
		t.Errorf("expected status up, got %s", updated.Status)
	}
	if updated.PollClass != domain.PollClassCore {
		t.Errorf("expected poll_class core for Prometheus router, got %s", updated.PollClass)
	}
	if updated.PollIntervalOverride != nil {
		t.Errorf("expected PollIntervalOverride nil, got %v", updated.PollIntervalOverride)
	}
}

// TestAddDevice_VirtualNoIP verifies that adding a virtual device with no IP
// creates a device with DeviceType=virtual, Status=unknown, and does NOT call
// the SNMP discovery function.
func TestAddDevice_VirtualNoIP(t *testing.T) {
	snmpCalled := false
	deviceRepo := newMockDeviceRepo()
	linkRepo := newMockLinkRepo()
	settingsRepo := newMockSettingsRepo()

	discoverFn := func(target string, creds domain.SNMPCredentials) (*snmp.DiscoveryResult, error) {
		snmpCalled = true
		return nil, fmt.Errorf("should not be called")
	}

	svc := NewDeviceService(deviceRepo, linkRepo, settingsRepo, discoverFn, nil)

	device, err := svc.AddDevice(context.Background(), "", "",
		domain.DeviceTypeVirtual,
		domain.SNMPCredentials{}, nil, "", "", "", "", nil)
	if err != nil {
		t.Fatalf("AddDevice failed: %v", err)
	}

	svc.WaitForProbes()

	if device.DeviceType != domain.DeviceTypeVirtual {
		t.Errorf("expected device_type virtual, got %s", device.DeviceType)
	}
	if device.Status != domain.DeviceStatusUnknown {
		t.Errorf("expected status unknown, got %s", device.Status)
	}
	if device.IP != "" {
		t.Errorf("expected empty IP, got %s", device.IP)
	}
	if snmpCalled {
		t.Error("discoverFunc was called for virtual device — should have been skipped")
	}
}

// TestAddDevice_VirtualWithIP verifies that a virtual device with an IP address
// is created with Status=unknown (MetricsCollector will resolve via probe_success)
// and does NOT call the SNMP discovery function.
func TestAddDevice_VirtualWithIP(t *testing.T) {
	snmpCalled := false
	deviceRepo := newMockDeviceRepo()
	linkRepo := newMockLinkRepo()
	settingsRepo := newMockSettingsRepo()

	discoverFn := func(target string, creds domain.SNMPCredentials) (*snmp.DiscoveryResult, error) {
		snmpCalled = true
		return nil, fmt.Errorf("should not be called")
	}

	svc := NewDeviceService(deviceRepo, linkRepo, settingsRepo, discoverFn, nil)

	device, err := svc.AddDevice(context.Background(), "10.0.0.99", "",
		domain.DeviceTypeVirtual,
		domain.SNMPCredentials{}, nil, "", "", "", "", nil)
	if err != nil {
		t.Fatalf("AddDevice failed: %v", err)
	}

	svc.WaitForProbes()

	if device.DeviceType != domain.DeviceTypeVirtual {
		t.Errorf("expected device_type virtual, got %s", device.DeviceType)
	}
	if device.Status != domain.DeviceStatusUnknown {
		t.Errorf("expected status unknown, got %s", device.Status)
	}
	if device.IP != "10.0.0.99" {
		t.Errorf("expected IP 10.0.0.99, got %s", device.IP)
	}
	if snmpCalled {
		t.Error("discoverFunc was called for virtual device — should have been skipped")
	}
}

func TestGetAllDevices_NormalizesLegacyVirtualNoIPState(t *testing.T) {
	svc, deviceRepo, _ := newTestService(&snmp.DiscoveryResult{}, nil)

	legacyVirtual := &domain.Device{
		ID:                   uuid.New(),
		Hostname:             "support-node",
		IP:                   "",
		DeviceType:           domain.DeviceTypeVirtual,
		Managed:              true,
		Status:               domain.DeviceStatusUp,
		MetricsSource:        domain.MetricsSourcePrometheus,
		PrometheusLabelName:  "instance",
		PrometheusLabelValue: "10.0.0.99",
	}
	if err := deviceRepo.Create(legacyVirtual); err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	devices, err := svc.GetAllDevices(context.Background())
	if err != nil {
		t.Fatalf("GetAllDevices failed: %v", err)
	}
	if len(devices) != 1 {
		t.Fatalf("expected 1 device, got %d", len(devices))
	}
	if devices[0].Status != domain.DeviceStatusUnknown {
		t.Fatalf("expected returned status unknown for legacy no-IP virtual node, got %s", devices[0].Status)
	}
	if devices[0].MetricsSource != domain.MetricsSourceNone {
		t.Fatalf("expected returned metrics source none for legacy no-IP virtual node, got %s", devices[0].MetricsSource)
	}

	stored, err := deviceRepo.GetByID(legacyVirtual.ID)
	if err != nil {
		t.Fatalf("GetByID failed: %v", err)
	}
	if stored.Status != domain.DeviceStatusUp {
		t.Fatalf("expected repo state to remain unchanged during read normalization, got %s", stored.Status)
	}
	if stored.MetricsSource != domain.MetricsSourcePrometheus {
		t.Fatalf("expected repo metrics source to remain unchanged during read normalization, got %s", stored.MetricsSource)
	}
}

// TestAddDevice_RegularStillRequiresProbe verifies that non-virtual devices
// still go through the normal SNMP probe flow.
func TestAddDevice_RegularStillRequiresProbe(t *testing.T) {
	snmpCalled := false
	deviceRepo := newMockDeviceRepo()
	linkRepo := newMockLinkRepo()
	settingsRepo := newMockSettingsRepo()

	discoverFn := func(target string, creds domain.SNMPCredentials) (*snmp.DiscoveryResult, error) {
		snmpCalled = true
		return &snmp.DiscoveryResult{SysName: "test-router"}, nil
	}

	svc := NewDeviceService(deviceRepo, linkRepo, settingsRepo, discoverFn, nil)

	device, err := svc.AddDevice(context.Background(), "10.0.0.1", "",
		domain.DeviceTypeUnknown,
		domain.SNMPCredentials{
			Version: domain.SNMPVersionV2c,
			V2c:     &domain.SNMPv2cCredentials{Community: "public"},
		}, nil, "", domain.MetricsSourceSNMP, "", "", nil)
	if err != nil {
		t.Fatalf("AddDevice failed: %v", err)
	}

	if device.Status != domain.DeviceStatusProbing {
		t.Errorf("expected status probing, got %s", device.Status)
	}

	svc.WaitForProbes()

	if !snmpCalled {
		t.Error("discoverFunc was NOT called — regular devices should trigger SNMP probe")
	}
}

type syncBuffer struct {
	mu sync.Mutex
	b  []byte
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.b = append(b.b, p...)
	return len(p), nil
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return string(b.b)
}

func (b *syncBuffer) Contains(substr string) bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	return strings.Contains(string(b.b), substr)
}

// helper
func strPtr(s string) *string { return &s }

func intPtr(v int) *int { return &v }

func fixedSchedulerDeviceIDForQuietWindow(t *testing.T, minimumOffset time.Duration) uuid.UUID {
	t.Helper()

	candidates := []uuid.UUID{
		uuid.MustParse("51000000-0000-0000-0000-000000000001"),
		uuid.MustParse("52000000-0000-0000-0000-000000000002"),
		uuid.MustParse("53000000-0000-0000-0000-000000000003"),
		uuid.MustParse("54000000-0000-0000-0000-000000000004"),
	}

	for _, candidate := range candidates {
		performanceOffset := schedulerInitialOffset(candidate, domain.PollClassCore.Interval())
		operationalOffset := schedulerInitialOffset(candidate, domain.OperationalClassInterval)
		staticOffset := schedulerInitialOffset(candidate, domain.StaticClassInterval)
		if performanceOffset > minimumOffset && operationalOffset > minimumOffset && staticOffset > minimumOffset {
			return candidate
		}
	}

	t.Fatalf("no fixed UUID had seeded offsets beyond quiet window floor %v", minimumOffset)
	return uuid.Nil
}

func schedulerInitialOffset(deviceID uuid.UUID, interval time.Duration) time.Duration {
	if interval <= 0 {
		return 0
	}

	hasher := fnv.New64a()
	_, _ = hasher.Write(deviceID[:])
	return time.Duration(hasher.Sum64() % uint64(interval))
}

func assertNoSchedulerTaskWithin(t *testing.T, tasks <-chan scheduler.PollTask, quietWindow time.Duration) {
	t.Helper()

	select {
	case task := <-tasks:
		t.Fatalf("scheduler emitted task %+v inside quiet window %v", task, quietWindow)
	case <-time.After(quietWindow):
	}
}

func waitForSchedulerTask(t *testing.T, tasks <-chan scheduler.PollTask, timeout time.Duration) scheduler.PollTask {
	t.Helper()

	select {
	case task := <-tasks:
		return task
	case <-time.After(timeout):
		t.Fatalf("timed out waiting for scheduler task after override save within %v", timeout)
		return scheduler.PollTask{}
	}
}

// TestProbeDevice_ReclassifyOnTypeChange verifies that when SNMP probe detects a
// device_type change (unknown -> router), poll_class is auto-recomputed to core
// via domain.ClassifyPollClass. PollIntervalOverride must remain nil (untouched).
func TestProbeDevice_ReclassifyOnTypeChange(t *testing.T) {
	result := &snmp.DiscoveryResult{
		SysName:    "router-reclassify",
		SysDescr:   "RouterOS",
		DeviceType: domain.DeviceTypeRouter,
	}

	svc, deviceRepo, _ := newTestService(result, nil)

	device := &domain.Device{
		ID:            uuid.New(),
		IP:            "192.0.2.1",
		Hostname:      "router-reclassify",
		DeviceType:    domain.DeviceTypeUnknown,
		PollClass:     domain.PollClassStandard,
		Managed:       true,
		Status:        domain.DeviceStatusProbing,
		MetricsSource: domain.MetricsSourceSNMP,
		SNMPCredentials: domain.SNMPCredentials{
			Version: domain.SNMPVersionV2c,
			V2c:     &domain.SNMPv2cCredentials{Community: "public"},
		},
	}
	if err := deviceRepo.Create(device); err != nil {
		t.Fatalf("failed to create device: %v", err)
	}

	svc.probeWg.Add(1)
	go func() {
		defer svc.probeWg.Done()
		svc.probeDevice(device)
	}()
	svc.WaitForProbes()

	updated, err := deviceRepo.GetByID(device.ID)
	if err != nil {
		t.Fatalf("GetByID failed: %v", err)
	}
	if updated.DeviceType != domain.DeviceTypeRouter {
		t.Errorf("expected device_type router, got %s", updated.DeviceType)
	}
	if updated.PollClass != domain.PollClassCore {
		t.Errorf("expected poll_class core (router->core per D-04), got %s", updated.PollClass)
	}
	if updated.PollIntervalOverride != nil {
		t.Errorf("expected PollIntervalOverride nil, got %v", updated.PollIntervalOverride)
	}
}

// TestProbeDevice_RespectsPollIntervalOverride verifies that when a device has a
// manual PollIntervalOverride set, the auto-reclassify hook does NOT overwrite
// poll_class even when device_type changes. DeviceType still propagates (SNMP wins).
func TestProbeDevice_RespectsPollIntervalOverride(t *testing.T) {
	result := &snmp.DiscoveryResult{
		SysName:    "router-override",
		DeviceType: domain.DeviceTypeRouter,
	}

	svc, deviceRepo, _ := newTestService(result, nil)

	device := &domain.Device{
		ID:                   uuid.New(),
		IP:                   "192.0.2.2",
		Hostname:             "router-override",
		DeviceType:           domain.DeviceTypeUnknown,
		PollClass:            domain.PollClassStandard,
		PollIntervalOverride: intPtr(15),
		Managed:              true,
		Status:               domain.DeviceStatusProbing,
		MetricsSource:        domain.MetricsSourceSNMP,
		SNMPCredentials: domain.SNMPCredentials{
			Version: domain.SNMPVersionV2c,
			V2c:     &domain.SNMPv2cCredentials{Community: "public"},
		},
	}
	if err := deviceRepo.Create(device); err != nil {
		t.Fatalf("failed to create device: %v", err)
	}

	svc.probeWg.Add(1)
	go func() {
		defer svc.probeWg.Done()
		svc.probeDevice(device)
	}()
	svc.WaitForProbes()

	updated, err := deviceRepo.GetByID(device.ID)
	if err != nil {
		t.Fatalf("GetByID failed: %v", err)
	}
	// DeviceType MUST propagate from discovery (SNMP result wins)
	if updated.DeviceType != domain.DeviceTypeRouter {
		t.Errorf("expected device_type router (SNMP result propagates), got %s", updated.DeviceType)
	}
	// PollClass MUST NOT change when override is set (manual control wins)
	if updated.PollClass != domain.PollClassStandard {
		t.Errorf("expected poll_class standard (override set, must not stomp), got %s", updated.PollClass)
	}
	// Override value MUST be preserved
	if updated.PollIntervalOverride == nil || *updated.PollIntervalOverride != 15 {
		t.Errorf("expected PollIntervalOverride=15, got %v", updated.PollIntervalOverride)
	}
}

// TestProbeDevice_NoTypeChangeStillSyncsPollClassWhenEmpty verifies that even when
// device_type does NOT change (router -> router), an empty PollClass on a legacy row
// is healed to the correct class on first probe (unconditional recompute path).
func TestProbeDevice_NoTypeChangeStillSyncsPollClassWhenEmpty(t *testing.T) {
	result := &snmp.DiscoveryResult{
		SysName:    "router-legacy",
		DeviceType: domain.DeviceTypeRouter,
	}

	svc, deviceRepo, _ := newTestService(result, nil)

	device := &domain.Device{
		ID:            uuid.New(),
		IP:            "192.0.2.3",
		Hostname:      "router-legacy",
		DeviceType:    domain.DeviceTypeRouter,
		PollClass:     "", // legacy row: empty PollClass
		Managed:       true,
		Status:        domain.DeviceStatusUp,
		MetricsSource: domain.MetricsSourceSNMP,
		SNMPCredentials: domain.SNMPCredentials{
			Version: domain.SNMPVersionV2c,
			V2c:     &domain.SNMPv2cCredentials{Community: "public"},
		},
	}
	if err := deviceRepo.Create(device); err != nil {
		t.Fatalf("failed to create device: %v", err)
	}

	svc.probeWg.Add(1)
	go func() {
		defer svc.probeWg.Done()
		svc.probeDevice(device)
	}()
	svc.WaitForProbes()

	updated, err := deviceRepo.GetByID(device.ID)
	if err != nil {
		t.Fatalf("GetByID failed: %v", err)
	}
	if updated.PollClass != domain.PollClassCore {
		t.Errorf("expected legacy empty PollClass healed to core (router->core), got %s", updated.PollClass)
	}
	if updated.PollIntervalOverride != nil {
		t.Errorf("expected PollIntervalOverride nil, got %v", updated.PollIntervalOverride)
	}
}

func TestProbeDevice_StaticDiscoveryPersistenceFailureStillMarksUp(t *testing.T) {
	result := &snmp.DiscoveryResult{
		SysName:    "router-persist-fail",
		DeviceType: domain.DeviceTypeRouter,
	}

	svc, deviceRepo, _ := newTestService(result, nil)
	deviceRepo.updateHook = func(device *domain.Device) error {
		if device.SysName == "router-persist-fail" {
			return fmt.Errorf("simulated static persistence failure")
		}
		return nil
	}

	device := &domain.Device{
		ID:            uuid.New(),
		IP:            "192.0.2.44",
		Hostname:      "router-persist-fail",
		DeviceType:    domain.DeviceTypeUnknown,
		PollClass:     domain.PollClassStandard,
		Managed:       true,
		Status:        domain.DeviceStatusProbing,
		MetricsSource: domain.MetricsSourceSNMP,
		SNMPCredentials: domain.SNMPCredentials{
			Version: domain.SNMPVersionV2c,
			V2c:     &domain.SNMPv2cCredentials{Community: "public"},
		},
	}
	if err := deviceRepo.Create(device); err != nil {
		t.Fatalf("failed to create device: %v", err)
	}

	svc.probeWg.Add(1)
	go func() {
		defer svc.probeWg.Done()
		svc.probeDevice(device)
	}()
	svc.WaitForProbes()

	updated, err := deviceRepo.GetByID(device.ID)
	if err != nil {
		t.Fatalf("GetByID failed: %v", err)
	}
	if updated.Status != domain.DeviceStatusUp {
		t.Fatalf("status = %s, want up after successful discovery", updated.Status)
	}
	if updated.SysName != "" {
		t.Fatalf("sysName = %q, want empty because static discovery persistence failed", updated.SysName)
	}
}
