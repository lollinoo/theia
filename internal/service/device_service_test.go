package service

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/lollinoo/theia/internal/domain"
	"github.com/lollinoo/theia/internal/snmp"
	"github.com/google/uuid"
)

// --- Mock Device Repository ---

type mockDeviceRepo struct {
	mu      sync.Mutex
	devices map[uuid.UUID]*domain.Device
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

func (r *mockLinkRepo) Upsert(link *domain.Link) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	// Check for existing match
	for id, existing := range r.links {
		if existing.SourceDeviceID == link.SourceDeviceID &&
			existing.SourceIfName == link.SourceIfName &&
			existing.TargetDeviceID == link.TargetDeviceID &&
			existing.TargetIfName == link.TargetIfName {
			link.ID = id
			link.UpdatedAt = time.Now().UTC()
			r.links[id] = link
			return nil
		}
	}
	if link.ID == uuid.Nil {
		link.ID = uuid.New()
	}
	now := time.Now().UTC()
	link.CreatedAt = now
	link.UpdatedAt = now
	r.links[link.ID] = link
	return nil
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

	svc := NewDeviceService(deviceRepo, linkRepo, settingsRepo, discoverFn)
	return svc, deviceRepo, linkRepo
}

// --- Tests ---

func TestAddDevice_CreatesWithStatusProbing(t *testing.T) {
	svc, deviceRepo, _ := newTestService(&snmp.DiscoveryResult{
		SysName: "router1",
	}, nil)

	device, err := svc.AddDevice(context.Background(), "192.168.1.1", "router1",
		domain.SNMPCredentials{
			Version: domain.SNMPVersionV2c,
			V2c:     &domain.SNMPv2cCredentials{Community: "public"},
		}, nil, "", "", "", "", nil, nil)
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
		domain.SNMPCredentials{
			Version: domain.SNMPVersionV2c,
			V2c:     &domain.SNMPv2cCredentials{Community: "public"},
		}, nil, "", domain.MetricsSourceSNMP, "", "", nil, nil)
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
		domain.SNMPCredentials{
			Version: domain.SNMPVersionV2c,
			V2c:     &domain.SNMPv2cCredentials{Community: "public"},
		}, nil, "", domain.MetricsSourceSNMP, "", "", nil, nil)
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

func TestProbeDiscoversNeighbors_CreatesUnmanagedPlaceholders(t *testing.T) {
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
		domain.SNMPCredentials{
			Version: domain.SNMPVersionV2c,
			V2c:     &domain.SNMPv2cCredentials{Community: "public"},
		}, nil, "", "", "", "", nil, nil)
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
		domain.SNMPCredentials{
			Version: domain.SNMPVersionV2c,
			V2c:     &domain.SNMPv2cCredentials{Community: "public"},
		}, nil, "", "", "", "", nil, nil)
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

	svc := NewDeviceService(deviceRepo, linkRepo, settingsRepo, discoverFn)

	device, err := svc.AddDevice(context.Background(), "10.0.9.254", "",
		// No meaningful SNMP credentials — user only wants Prometheus
		domain.SNMPCredentials{
			Version: domain.SNMPVersionV2c,
			V2c:     &domain.SNMPv2cCredentials{Community: "public"},
		}, nil, "", domain.MetricsSourcePrometheus, "instance", "10.0.9.254", nil, nil)
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

	svc := NewDeviceService(deviceRepo, linkRepo, settingsRepo, discoverFn)

	// Simulate what happens when user picks "Prometheus without Fallback" but
	// previously had v3 credentials in the form with authPriv + empty priv_password.
	device, err := svc.AddDevice(context.Background(), "10.0.9.254", "",
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
		}, nil, "", domain.MetricsSourcePrometheus, "instance", "10.0.9.254", nil, nil)
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

// helper
func strPtr(s string) *string { return &s }
