package postgres

// This file exercises device repo behavior so refactors preserve the documented contract.

import (
	"database/sql"
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/lollinoo/theia/internal/domain"
)

func TestDeviceRepoGetBySysName_NormalizedLookup(t *testing.T) {
	db := newTestDB(t)
	repo := NewDeviceRepo(db, testKeyring, nil)

	first := &domain.Device{
		ID:       uuid.New(),
		Hostname: "dist-sw-01",
		IP:       "10.0.0.1",
		SysName:  "Dist-SW-01",
		Managed:  true,
		Status:   domain.DeviceStatusUp,
		Tags:     map[string]string{},
		SNMPCredentials: domain.SNMPCredentials{
			Version: domain.SNMPVersionV2c,
			V2c:     &domain.SNMPv2cCredentials{Community: "public"},
		},
	}
	if err := repo.Create(first); err != nil {
		t.Fatalf("Create first device failed: %v", err)
	}

	second := &domain.Device{
		ID:       uuid.New(),
		Hostname: "core-sw-02",
		IP:       "10.0.0.2",
		SysName:  "core-sw-02.example.net.",
		Managed:  true,
		Status:   domain.DeviceStatusUp,
		Tags:     map[string]string{},
		SNMPCredentials: domain.SNMPCredentials{
			Version: domain.SNMPVersionV2c,
			V2c:     &domain.SNMPv2cCredentials{Community: "public"},
		},
	}
	if err := repo.Create(second); err != nil {
		t.Fatalf("Create second device failed: %v", err)
	}

	tests := []struct {
		name       string
		lookup     string
		expectedID uuid.UUID
	}{
		{
			name:       "matches case and whitespace",
			lookup:     "  dist-sw-01  ",
			expectedID: first.ID,
		},
		{
			name:       "matches fqdn against short stored sysName",
			lookup:     "dist-sw-01.example.net.",
			expectedID: first.ID,
		},
		{
			name:       "matches short name against stored fqdn",
			lookup:     "CORE-SW-02",
			expectedID: second.ID,
		},
		{
			name:       "matches stored fqdn with different case",
			lookup:     "core-sw-02.EXAMPLE.NET.",
			expectedID: second.ID,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			device, err := repo.GetBySysName(tc.lookup)
			if err != nil {
				t.Fatalf("GetBySysName failed: %v", err)
			}
			if device == nil {
				t.Fatalf("expected device for lookup %q", tc.lookup)
			}
			if device.ID != tc.expectedID {
				t.Fatalf("expected device %s, got %s", tc.expectedID, device.ID)
			}
		})
	}
}

func TestDeviceRepoFindPhysicalVirtualIPConflictUsesScopedNormalizedLookup(t *testing.T) {
	db := newTestDB(t)
	repo := NewDeviceRepo(db, testKeyring, nil)

	virtual := &domain.Device{
		ID:         uuid.New(),
		Hostname:   "virtual-edge",
		IP:         " Example.Host ",
		DeviceType: domain.DeviceTypeVirtual,
		Managed:    true,
		Status:     domain.DeviceStatusUnknown,
		Tags:       map[string]string{},
	}
	if err := repo.Create(virtual); err != nil {
		t.Fatalf("Create virtual device failed: %v", err)
	}
	physical := &domain.Device{
		ID:         uuid.New(),
		Hostname:   "physical-edge",
		IP:         "10.0.0.10",
		DeviceType: domain.DeviceTypeRouter,
		Managed:    true,
		Status:     domain.DeviceStatusUp,
		Tags:       map[string]string{},
		SNMPCredentials: domain.SNMPCredentials{
			Version: domain.SNMPVersionV2c,
			V2c:     &domain.SNMPv2cCredentials{Community: "public"},
		},
	}
	if err := repo.Create(physical); err != nil {
		t.Fatalf("Create physical device failed: %v", err)
	}

	conflict, err := repo.FindPhysicalVirtualIPConflict("example.host", domain.DeviceTypeRouter, uuid.Nil)
	if err != nil {
		t.Fatalf("FindPhysicalVirtualIPConflict failed: %v", err)
	}
	if conflict == nil {
		t.Fatal("expected physical candidate to conflict with virtual device")
	}
	if conflict.ID != virtual.ID {
		t.Fatalf("conflict ID = %s, want %s", conflict.ID, virtual.ID)
	}
	if conflict.DeviceType != domain.DeviceTypeVirtual {
		t.Fatalf("conflict device type = %s, want virtual", conflict.DeviceType)
	}

	conflict, err = repo.FindPhysicalVirtualIPConflict("example.host", domain.DeviceTypeVirtual, uuid.Nil)
	if err != nil {
		t.Fatalf("FindPhysicalVirtualIPConflict same type failed: %v", err)
	}
	if conflict != nil {
		t.Fatalf("same-type virtual candidate returned conflict: %+v", conflict)
	}

	conflict, err = repo.FindPhysicalVirtualIPConflict("example.host", domain.DeviceTypeRouter, virtual.ID)
	if err != nil {
		t.Fatalf("FindPhysicalVirtualIPConflict excluded device failed: %v", err)
	}
	if conflict != nil {
		t.Fatalf("excluded device returned conflict: %+v", conflict)
	}
}

func TestDeviceRepoDatabaseRejectsPhysicalVirtualDuplicateIP(t *testing.T) {
	db := newTestDB(t)
	repo := NewDeviceRepo(db, testKeyring, nil)

	physical := &domain.Device{
		ID:         uuid.New(),
		Hostname:   "physical-edge",
		IP:         "10.0.0.99",
		DeviceType: domain.DeviceTypeRouter,
		Managed:    true,
		Status:     domain.DeviceStatusUp,
		Tags:       map[string]string{},
		SNMPCredentials: domain.SNMPCredentials{
			Version: domain.SNMPVersionV2c,
			V2c:     &domain.SNMPv2cCredentials{Community: "public"},
		},
	}
	if err := repo.Create(physical); err != nil {
		t.Fatalf("Create physical device failed: %v", err)
	}

	virtual := &domain.Device{
		ID:         uuid.New(),
		Hostname:   "virtual-edge",
		IP:         " 10.0.0.99 ",
		DeviceType: domain.DeviceTypeVirtual,
		Managed:    true,
		Status:     domain.DeviceStatusUnknown,
		Tags:       map[string]string{},
	}
	err := repo.Create(virtual)
	if err == nil {
		t.Fatal("expected database to reject physical/virtual duplicate IP")
	}
	if !isDeviceIPInvariantError(err) {
		t.Fatalf("Create duplicate error = %v, want device IP invariant error", err)
	}
}

func TestDeviceRepoCreateAllowsDuplicateVirtualAddress(t *testing.T) {
	db := newTestDB(t)
	repo := NewDeviceRepo(db, testKeyring, nil)
	first := &domain.Device{
		ID:         uuid.New(),
		Hostname:   "virtual-duplicate-first",
		IP:         "virtual-duplicate.example.net",
		DeviceType: domain.DeviceTypeVirtual,
		Status:     domain.DeviceStatusUnknown,
		Tags:       map[string]string{},
	}
	second := &domain.Device{
		ID:         uuid.New(),
		Hostname:   "virtual-duplicate-second",
		IP:         " VIRTUAL-DUPLICATE.EXAMPLE.NET ",
		DeviceType: domain.DeviceTypeVirtual,
		Status:     domain.DeviceStatusUnknown,
		Tags:       map[string]string{},
	}

	if err := repo.Create(first); err != nil {
		t.Fatalf("Create first virtual device failed: %v", err)
	}
	if err := repo.Create(second); err != nil {
		t.Fatalf("Create second virtual device failed: %v", err)
	}
	if got := importTestCount(t, db,
		`SELECT COUNT(*) FROM device_addresses WHERE normalized_address = $1`,
		"virtual-duplicate.example.net"); got != 2 {
		t.Fatalf("manual virtual address count = %d, want 2", got)
	}
}

func TestDeviceRepoCreatePublishes(t *testing.T) {
	db := newTestDB(t)
	onChange := make(chan struct{}, 2)
	repo := NewDeviceRepo(db, testKeyring, onChange)
	changes := repo.SubscribeDeviceChanges(2)
	device := &domain.Device{
		ID:         uuid.New(),
		Hostname:   "create-publish",
		IP:         "10.0.0.200",
		DeviceType: domain.DeviceTypeRouter,
		Managed:    true,
		Status:     domain.DeviceStatusUp,
		Tags:       map[string]string{},
		SNMPCredentials: domain.SNMPCredentials{
			Version: domain.SNMPVersionV2c,
			V2c:     &domain.SNMPv2cCredentials{Community: "public"},
		},
	}

	if err := repo.Create(device); err != nil {
		t.Fatalf("Create failed: %v", err)
	}
	select {
	case <-onChange:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for Create cache invalidation")
	}
	select {
	case <-onChange:
		t.Fatal("Create emitted more than one cache invalidation")
	default:
	}
	select {
	case event := <-changes:
		if event.Kind != domain.ChangeKindCreated || event.DeviceID != device.ID {
			t.Fatalf("Create event = %#v, want created event for %s", event, device.ID)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for Create device event")
	}
	select {
	case event := <-changes:
		t.Fatalf("Create emitted extra event %#v", event)
	default:
	}
}

func TestDeviceRepoGetByIDsForTopologySkipsSNMPDecryption(t *testing.T) {
	db := newTestDB(t)
	repo := NewDeviceRepo(db, testKeyring, nil)

	device := &domain.Device{
		ID:       uuid.New(),
		Hostname: "router-topology",
		IP:       "10.0.0.10",
		Managed:  true,
		Status:   domain.DeviceStatusUp,
		Tags:     map[string]string{},
		SNMPCredentials: domain.SNMPCredentials{
			Version: domain.SNMPVersionV2c,
			V2c:     &domain.SNMPv2cCredentials{Community: "public"},
		},
	}
	if err := repo.Create(device); err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	_, err := db.Exec(
		`UPDATE devices SET snmp_credentials_json = ? WHERE id = ?`,
		`{"version":"2c","v2c":{"community":"plaintext-community"}}`,
		device.ID.String(),
	)
	if err != nil {
		t.Fatalf("corrupting stored credentials failed: %v", err)
	}

	if _, err := repo.GetByIDs([]uuid.UUID{device.ID}); err == nil {
		t.Fatal("GetByIDs should fail when strict credential decryption sees plaintext")
	}

	devices, err := repo.GetByIDsForTopology([]uuid.UUID{device.ID})
	if err != nil {
		t.Fatalf("GetByIDsForTopology failed: %v", err)
	}
	if len(devices) != 1 {
		t.Fatalf("device count = %d, want 1", len(devices))
	}
	if devices[0].SNMPCredentials != (domain.SNMPCredentials{}) {
		t.Fatalf("SNMP credentials = %+v, want empty topology projection", devices[0].SNMPCredentials)
	}

	var stored sql.NullString
	if err := db.QueryRow(`SELECT snmp_credentials_json FROM devices WHERE id = ?`, device.ID.String()).Scan(&stored); err != nil {
		t.Fatalf("reading stored credentials failed: %v", err)
	}
	if !stored.Valid || stored.String == "" {
		t.Fatal("stored credentials unexpectedly empty")
	}
}

func TestDeviceRepoUpdateStaticDiscoverySkipsSNMPCredentials(t *testing.T) {
	db := newTestDB(t)
	onChange := make(chan struct{}, 2)
	repo := NewDeviceRepo(db, testKeyring, onChange)

	pollingEnabled := false
	notes := "preserve notes"
	pollOverride := 45
	device := &domain.Device{
		ID:                          uuid.New(),
		Hostname:                    "router-static",
		IP:                          "10.0.0.10",
		DeviceType:                  domain.DeviceTypeRouter,
		Status:                      domain.DeviceStatusUp,
		Managed:                     true,
		Tags:                        map[string]string{"role": "edge"},
		MetricsSource:               domain.MetricsSourcePrometheusSNMPFallback,
		PrometheusLabelName:         "instance",
		PrometheusLabelValue:        "router-static",
		PollClass:                   domain.PollClassStandard,
		PollIntervalOverride:        &pollOverride,
		PollingEnabled:              &pollingEnabled,
		Notes:                       &notes,
		ProbePorts:                  []int{161, 9161},
		TopologyDiscoveryMode:       domain.TopologyDiscoveryModeLLDP,
		TopologyBootstrapState:      domain.TopologyBootstrapStatePending,
		LastTopologyDiscoveryResult: "ports_pending",
		SysName:                     "router-old",
		SysDescr:                    "old descr",
		SysObjectID:                 ".1.3.6.1.4.1.9.1.1",
		HardwareModel:               "old model",
		OSVersion:                   "1.0",
		Vendor:                      "cisco",
		Interfaces: []domain.Interface{
			{IfIndex: 1, IfName: "ge-0/0/0", IfDescr: "old uplink", Speed: 1_000_000_000, AdminStatus: "up", OperStatus: "up"},
		},
		SNMPCredentials: domain.SNMPCredentials{
			Version: domain.SNMPVersionV2c,
			V2c:     &domain.SNMPv2cCredentials{Community: "public"},
		},
	}
	if err := repo.Create(device); err != nil {
		t.Fatalf("Create failed: %v", err)
	}
	select {
	case <-onChange:
	default:
	}
	changes := repo.SubscribeDeviceChanges(1)

	rawCredentials := `{"version":"2c","v2c":{"community":"plaintext-community"}}`
	if _, err := db.Exec(
		`UPDATE devices SET snmp_credentials_json = ? WHERE id = ?`,
		rawCredentials,
		device.ID.String(),
	); err != nil {
		t.Fatalf("corrupting stored credentials failed: %v", err)
	}

	if _, err := repo.GetByIDs([]uuid.UUID{device.ID}); err == nil {
		t.Fatal("GetByIDs should fail when strict credential decryption sees plaintext")
	}

	devices, err := repo.GetByIDsForTopology([]uuid.UUID{device.ID})
	if err != nil {
		t.Fatalf("GetByIDsForTopology failed: %v", err)
	}
	if len(devices) != 1 {
		t.Fatalf("device count = %d, want 1", len(devices))
	}

	staticDevice := devices[0]
	staticDevice.Hostname = "router-static-new"
	staticDevice.SysName = "router-new"
	staticDevice.SysDescr = "new descr"
	staticDevice.SysObjectID = ".1.3.6.1.4.1.14988.1"
	staticDevice.HardwareModel = "new model"
	staticDevice.OSVersion = "2.0"
	staticDevice.Vendor = "mikrotik"
	staticDevice.DeviceType = domain.DeviceTypeSwitch
	staticDevice.PollClass = domain.PollClassCore
	staticDevice.Interfaces = []domain.Interface{
		{IfIndex: 7, IfName: "ether7", IfDescr: "new uplink", Speed: 10_000_000_000, AdminStatus: "up", OperStatus: "down"},
	}
	if err := repo.UpdateStaticDiscovery(&staticDevice); err != nil {
		t.Fatalf("UpdateStaticDiscovery failed: %v", err)
	}
	select {
	case <-onChange:
	case <-time.After(time.Second):
		t.Fatal("expected static discovery update to notify repository change listeners")
	}
	select {
	case event := <-changes:
		if event.Kind != domain.ChangeKindUpdated || event.DeviceID != device.ID {
			t.Fatalf("device change event = %#v, want updated event for %s", event, device.ID)
		}
	case <-time.After(time.Second):
		t.Fatal("expected static discovery update to publish device change event")
	}

	var storedCreds, storedProbePorts, storedMetricsSource, storedTags, storedTopologyMode, storedTopologyResult string
	var storedPollingEnabled int
	var storedNotes sql.NullString
	if err := db.QueryRow(
		`SELECT snmp_credentials_json, probe_ports, metrics_source, polling_enabled, notes, tags_json,
			topology_discovery_mode, last_topology_discovery_result
		FROM devices WHERE id = ?`,
		device.ID.String(),
	).Scan(&storedCreds, &storedProbePorts, &storedMetricsSource, &storedPollingEnabled, &storedNotes, &storedTags, &storedTopologyMode, &storedTopologyResult); err != nil {
		t.Fatalf("reading raw device row failed: %v", err)
	}
	if storedCreds != rawCredentials {
		t.Fatalf("snmp_credentials_json changed to %q, want unchanged corrupted blob", storedCreds)
	}
	if storedProbePorts != domain.FormatProbePortsCSV(device.ProbePorts) {
		t.Fatalf("probe_ports = %q, want unchanged", storedProbePorts)
	}
	if storedMetricsSource != string(domain.MetricsSourcePrometheusSNMPFallback) {
		t.Fatalf("metrics_source = %q, want unchanged", storedMetricsSource)
	}
	if storedPollingEnabled != 0 {
		t.Fatalf("polling_enabled = %d, want unchanged disabled value", storedPollingEnabled)
	}
	if !storedNotes.Valid || storedNotes.String != notes {
		t.Fatalf("notes = %#v, want unchanged %q", storedNotes, notes)
	}
	if !strings.Contains(storedTags, `"role":"edge"`) {
		t.Fatalf("tags_json = %q, want unchanged role tag", storedTags)
	}
	if storedTopologyMode != string(domain.TopologyDiscoveryModeLLDP) {
		t.Fatalf("topology_discovery_mode = %q, want unchanged", storedTopologyMode)
	}
	if storedTopologyResult != "ports_pending" {
		t.Fatalf("last_topology_discovery_result = %q, want unchanged", storedTopologyResult)
	}

	updated, err := repo.GetByIDsForTopology([]uuid.UUID{device.ID})
	if err != nil {
		t.Fatalf("GetByIDsForTopology after update failed: %v", err)
	}
	if len(updated) != 1 {
		t.Fatalf("updated device count = %d, want 1", len(updated))
	}
	got := updated[0]
	if got.SNMPCredentials != (domain.SNMPCredentials{}) {
		t.Fatalf("SNMP credentials = %+v, want empty topology projection", got.SNMPCredentials)
	}
	if got.Hostname != "router-static-new" || got.SysName != "router-new" || got.DeviceType != domain.DeviceTypeSwitch || got.PollClass != domain.PollClassCore {
		t.Fatalf("static fields not updated: hostname=%q sysName=%q deviceType=%q pollClass=%q", got.Hostname, got.SysName, got.DeviceType, got.PollClass)
	}
	if len(got.Interfaces) != 1 || got.Interfaces[0].IfIndex != 7 || got.Interfaces[0].IfName != "ether7" {
		t.Fatalf("interfaces = %#v, want replaced static interface", got.Interfaces)
	}
}

func TestDeviceRepoGetByIDsLoadsOnlyRequestedDevicesWithInterfacesAndAreas(t *testing.T) {
	db := newTestDB(t)
	repo := NewDeviceRepo(db, testKeyring, nil)
	areaRepo := NewAreaRepo(db)

	area := &domain.Area{
		ID:    uuid.New(),
		Name:  "Backbone",
		Color: "#00AEEF",
	}
	if err := areaRepo.Create(area); err != nil {
		t.Fatalf("Create area failed: %v", err)
	}

	first := &domain.Device{
		ID:         uuid.New(),
		Hostname:   "router-a",
		IP:         "10.80.0.1",
		Managed:    true,
		Status:     domain.DeviceStatusUp,
		Tags:       map[string]string{},
		DeviceType: domain.DeviceTypeRouter,
		SNMPCredentials: domain.SNMPCredentials{
			Version: domain.SNMPVersionV2c,
			V2c:     &domain.SNMPv2cCredentials{Community: "public"},
		},
		Interfaces: []domain.Interface{{IfIndex: 1, IfName: "ether1", IfDescr: "uplink", Speed: 1000000000}},
		AreaIDs:    []uuid.UUID{area.ID},
	}
	second := &domain.Device{
		ID:         uuid.New(),
		Hostname:   "router-b",
		IP:         "10.80.0.2",
		Managed:    true,
		Status:     domain.DeviceStatusUp,
		Tags:       map[string]string{},
		DeviceType: domain.DeviceTypeRouter,
		SNMPCredentials: domain.SNMPCredentials{
			Version: domain.SNMPVersionV2c,
			V2c:     &domain.SNMPv2cCredentials{Community: "public"},
		},
	}
	third := &domain.Device{
		ID:         uuid.New(),
		Hostname:   "router-c",
		IP:         "10.80.0.3",
		Managed:    true,
		Status:     domain.DeviceStatusUp,
		Tags:       map[string]string{},
		DeviceType: domain.DeviceTypeRouter,
		SNMPCredentials: domain.SNMPCredentials{
			Version: domain.SNMPVersionV2c,
			V2c:     &domain.SNMPv2cCredentials{Community: "public"},
		},
	}
	for _, device := range []*domain.Device{first, second, third} {
		if err := repo.Create(device); err != nil {
			t.Fatalf("Create device %s failed: %v", device.Hostname, err)
		}
	}

	devices, err := repo.GetByIDs([]uuid.UUID{second.ID, first.ID, uuid.New()})
	if err != nil {
		t.Fatalf("GetByIDs failed: %v", err)
	}
	if len(devices) != 2 {
		t.Fatalf("GetByIDs returned %d devices, want 2: %#v", len(devices), devices)
	}
	byID := make(map[uuid.UUID]domain.Device, len(devices))
	for _, device := range devices {
		byID[device.ID] = device
	}
	if _, ok := byID[third.ID]; ok {
		t.Fatalf("GetByIDs returned unrequested device %s", third.ID)
	}
	gotFirst := byID[first.ID]
	if len(gotFirst.Interfaces) != 1 || gotFirst.Interfaces[0].IfName != "ether1" {
		t.Fatalf("first interfaces = %#v, want ether1", gotFirst.Interfaces)
	}
	if len(gotFirst.AreaIDs) != 1 || gotFirst.AreaIDs[0] != area.ID {
		t.Fatalf("first area IDs = %#v, want %s", gotFirst.AreaIDs, area.ID)
	}

	empty, err := repo.GetByIDs(nil)
	if err != nil {
		t.Fatalf("GetByIDs(nil) failed: %v", err)
	}
	if len(empty) != 0 {
		t.Fatalf("GetByIDs(nil) = %#v, want empty", empty)
	}
}

func TestDeviceRepoCreateHydratesLegacyPrimaryAddress(t *testing.T) {
	db := newTestDB(t)
	repo := NewDeviceRepo(db, testKeyring, nil)

	device := &domain.Device{
		ID:         uuid.New(),
		Hostname:   "router-addresses",
		IP:         " 10.90.0.1 ",
		Managed:    true,
		Status:     domain.DeviceStatusUp,
		Tags:       map[string]string{},
		DeviceType: domain.DeviceTypeRouter,
		SNMPCredentials: domain.SNMPCredentials{
			Version: domain.SNMPVersionV2c,
			V2c:     &domain.SNMPv2cCredentials{Community: "public"},
		},
	}
	if err := repo.Create(device); err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	got, err := repo.GetByID(device.ID)
	if err != nil {
		t.Fatalf("GetByID failed: %v", err)
	}
	if got.IP != "10.90.0.1" {
		t.Fatalf("IP = %q, want trimmed legacy primary", got.IP)
	}
	assertDeviceAddresses(t, got.Addresses, []addressExpectation{
		{address: "10.90.0.1", role: domain.DeviceAddressRolePrimary, isPrimary: true, priority: 0},
	})
}

func TestDeviceRepoCreateUpdateAndLookupDeviceAddresses(t *testing.T) {
	db := newTestDB(t)
	repo := NewDeviceRepo(db, testKeyring, nil)

	device := &domain.Device{
		ID:         uuid.New(),
		Hostname:   "router-multi-address",
		IP:         "10.91.0.1",
		Managed:    true,
		Status:     domain.DeviceStatusUp,
		Tags:       map[string]string{},
		DeviceType: domain.DeviceTypeRouter,
		SNMPCredentials: domain.SNMPCredentials{
			Version: domain.SNMPVersionV2c,
			V2c:     &domain.SNMPv2cCredentials{Community: "public"},
		},
		Addresses: []domain.DeviceAddress{
			{Address: "10.91.0.1", Label: "mgmt", Role: domain.DeviceAddressRolePrimary, IsPrimary: true},
			{Address: "198.51.100.91", Label: "backup link", Role: domain.DeviceAddressRoleBackup, Priority: 10},
		},
	}
	if err := repo.Create(device); err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	byAddress, err := repo.GetByAddress(" 198.51.100.91 ")
	if err != nil {
		t.Fatalf("GetByAddress failed: %v", err)
	}
	if byAddress == nil || byAddress.ID != device.ID {
		t.Fatalf("GetByAddress returned %#v, want device %s", byAddress, device.ID)
	}
	assertDeviceAddresses(t, byAddress.Addresses, []addressExpectation{
		{address: "10.91.0.1", label: "mgmt", role: domain.DeviceAddressRolePrimary, isPrimary: true, priority: 0},
		{address: "198.51.100.91", label: "backup link", role: domain.DeviceAddressRoleBackup, priority: 10},
	})

	byAddress.IP = "10.91.0.2"
	byAddress.Addresses = []domain.DeviceAddress{
		{Address: "10.91.0.2", Label: "new mgmt", Role: domain.DeviceAddressRolePrimary, IsPrimary: true},
		{Address: "monitor.example.test", Label: "monitor", Role: domain.DeviceAddressRoleMonitoring, Priority: 20},
	}
	if err := repo.Update(byAddress); err != nil {
		t.Fatalf("Update failed: %v", err)
	}

	updated, err := repo.GetByID(device.ID)
	if err != nil {
		t.Fatalf("GetByID after update failed: %v", err)
	}
	if updated.IP != "10.91.0.2" {
		t.Fatalf("updated IP = %q, want new primary", updated.IP)
	}
	assertDeviceAddresses(t, updated.Addresses, []addressExpectation{
		{address: "10.91.0.2", label: "new mgmt", role: domain.DeviceAddressRolePrimary, isPrimary: true, priority: 0},
		{address: "monitor.example.test", label: "monitor", role: domain.DeviceAddressRoleMonitoring, priority: 20},
	})
	if stale, err := repo.GetByAddress("198.51.100.91"); err != nil {
		t.Fatalf("GetByAddress stale failed: %v", err)
	} else if stale != nil {
		t.Fatalf("old backup address still resolved to device: %#v", stale)
	}
}

func TestDeviceRepoCreateAndGetPersistsProbePorts(t *testing.T) {
	db := newTestDB(t)
	repo := NewDeviceRepo(db, testKeyring, nil)

	device := &domain.Device{
		ID:         uuid.New(),
		Hostname:   "router-probe-ports",
		IP:         "10.94.0.1",
		ProbePorts: []int{22, 8291},
		Managed:    true,
		Status:     domain.DeviceStatusUp,
		Tags:       map[string]string{},
		DeviceType: domain.DeviceTypeRouter,
		SNMPCredentials: domain.SNMPCredentials{
			Version: domain.SNMPVersionV2c,
			V2c:     &domain.SNMPv2cCredentials{Community: "public"},
		},
		Addresses: []domain.DeviceAddress{
			{Address: "10.94.0.1", Label: "mgmt", Role: domain.DeviceAddressRolePrimary, IsPrimary: true},
			{Address: "198.51.100.94", Label: "backup oob", Role: domain.DeviceAddressRoleBackup, Priority: 10, ProbePorts: []int{2222}},
		},
	}
	if err := repo.Create(device); err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	got, err := repo.GetByID(device.ID)
	if err != nil {
		t.Fatalf("GetByID failed: %v", err)
	}
	if !slices.Equal(got.ProbePorts, []int{22, 8291}) {
		t.Fatalf("ProbePorts = %#v, want %#v", got.ProbePorts, []int{22, 8291})
	}

	var backup *domain.DeviceAddress
	for i := range got.Addresses {
		if got.Addresses[i].Address == "198.51.100.94" && got.Addresses[i].Role == domain.DeviceAddressRoleBackup {
			backup = &got.Addresses[i]
			break
		}
	}
	if backup == nil {
		t.Fatalf("backup address not loaded: %#v", got.Addresses)
	}
	if !slices.Equal(backup.ProbePorts, []int{2222}) {
		t.Fatalf("backup ProbePorts = %#v, want %#v", backup.ProbePorts, []int{2222})
	}
}

func TestDeviceRepoBatchReadsHydrateAddresses(t *testing.T) {
	db := newTestDB(t)
	repo := NewDeviceRepo(db, testKeyring, nil)

	first := &domain.Device{
		ID:         uuid.New(),
		Hostname:   "router-address-a",
		IP:         "10.92.0.1",
		Managed:    true,
		Status:     domain.DeviceStatusUp,
		Tags:       map[string]string{},
		DeviceType: domain.DeviceTypeRouter,
		SNMPCredentials: domain.SNMPCredentials{
			Version: domain.SNMPVersionV2c,
			V2c:     &domain.SNMPv2cCredentials{Community: "public"},
		},
		Addresses: []domain.DeviceAddress{
			{Address: "10.92.0.1", Role: domain.DeviceAddressRolePrimary, IsPrimary: true},
			{Address: "198.51.100.92", Role: domain.DeviceAddressRoleBackup, Priority: 5},
		},
	}
	second := &domain.Device{
		ID:         uuid.New(),
		Hostname:   "router-address-b",
		IP:         "10.92.0.2",
		Managed:    true,
		Status:     domain.DeviceStatusUp,
		Tags:       map[string]string{},
		DeviceType: domain.DeviceTypeRouter,
		SNMPCredentials: domain.SNMPCredentials{
			Version: domain.SNMPVersionV2c,
			V2c:     &domain.SNMPv2cCredentials{Community: "public"},
		},
	}
	for _, device := range []*domain.Device{first, second} {
		if err := repo.Create(device); err != nil {
			t.Fatalf("Create %s failed: %v", device.Hostname, err)
		}
	}

	all, err := repo.GetAll()
	if err != nil {
		t.Fatalf("GetAll failed: %v", err)
	}
	assertDeviceFromListHasAddress(t, all, first.ID, "198.51.100.92")

	selected, err := repo.GetByIDs([]uuid.UUID{first.ID})
	if err != nil {
		t.Fatalf("GetByIDs failed: %v", err)
	}
	assertDeviceFromListHasAddress(t, selected, first.ID, "198.51.100.92")

	topology, err := repo.GetByIDsForTopology([]uuid.UUID{first.ID})
	if err != nil {
		t.Fatalf("GetByIDsForTopology failed: %v", err)
	}
	assertDeviceFromListHasAddress(t, topology, first.ID, "198.51.100.92")

	orphans, err := repo.GetOrphans()
	if err != nil {
		t.Fatalf("GetOrphans failed: %v", err)
	}
	assertDeviceFromListHasAddress(t, orphans, first.ID, "198.51.100.92")
}

func TestDeviceRepoRejectsDuplicateAddressForSameDevice(t *testing.T) {
	db := newTestDB(t)
	repo := NewDeviceRepo(db, testKeyring, nil)

	device := &domain.Device{
		ID:         uuid.New(),
		Hostname:   "router-duplicate-address",
		IP:         "10.93.0.1",
		Managed:    true,
		Status:     domain.DeviceStatusUp,
		Tags:       map[string]string{},
		DeviceType: domain.DeviceTypeRouter,
		SNMPCredentials: domain.SNMPCredentials{
			Version: domain.SNMPVersionV2c,
			V2c:     &domain.SNMPv2cCredentials{Community: "public"},
		},
		Addresses: []domain.DeviceAddress{
			{Address: "10.93.0.1", Role: domain.DeviceAddressRolePrimary, IsPrimary: true},
			{Address: " 10.93.0.1 ", Role: domain.DeviceAddressRoleBackup},
		},
	}
	if err := repo.Create(device); err != nil {
		t.Fatalf("Create should normalize duplicate addresses instead of failing: %v", err)
	}

	err := repo.ReplaceDeviceAddresses(device.ID, []domain.DeviceAddress{
		{Address: "10.93.0.1", Role: domain.DeviceAddressRolePrimary, IsPrimary: true},
		{Address: "10.93.0.1", Role: domain.DeviceAddressRoleBackup},
	})
	if err == nil {
		t.Fatal("expected duplicate address replacement to fail")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "duplicate") &&
		!strings.Contains(strings.ToLower(err.Error()), "unique") {
		t.Fatalf("duplicate replacement error = %v, want unique/duplicate failure", err)
	}
}

func TestDeviceRepoGetOrphansReturnsDevicesWithoutCanvasMapMembership(t *testing.T) {
	db := newTestDB(t)
	repo := NewDeviceRepo(db, testKeyring, nil)
	areaRepo := NewAreaRepo(db)
	mapRepo := NewCanvasMapRepo(db)

	area := &domain.Area{
		ID:    uuid.New(),
		Name:  "Staging",
		Color: "#2979FF",
	}
	if err := areaRepo.Create(area); err != nil {
		t.Fatalf("Create area failed: %v", err)
	}

	mapped := &domain.Device{
		ID:         uuid.New(),
		Hostname:   "mapped-router",
		IP:         "10.90.0.1",
		Managed:    true,
		Status:     domain.DeviceStatusUp,
		Tags:       map[string]string{},
		DeviceType: domain.DeviceTypeRouter,
		SNMPCredentials: domain.SNMPCredentials{
			Version: domain.SNMPVersionV2c,
			V2c:     &domain.SNMPv2cCredentials{Community: "public"},
		},
	}
	orphan := &domain.Device{
		ID:         uuid.New(),
		Hostname:   "orphan-switch",
		IP:         "10.90.0.2",
		Managed:    true,
		Status:     domain.DeviceStatusUnknown,
		Tags:       map[string]string{},
		DeviceType: domain.DeviceTypeSwitch,
		SNMPCredentials: domain.SNMPCredentials{
			Version: domain.SNMPVersionV2c,
			V2c:     &domain.SNMPv2cCredentials{Community: "public"},
		},
		Interfaces: []domain.Interface{{IfIndex: 7, IfName: "ether7", IfDescr: "orphan port", Speed: 1000000000}},
		AreaIDs:    []uuid.UUID{area.ID},
	}
	if err := repo.Create(mapped); err != nil {
		t.Fatalf("Create mapped device failed: %v", err)
	}
	if err := repo.Create(orphan); err != nil {
		t.Fatalf("Create orphan device failed: %v", err)
	}

	canvasMap, err := mapRepo.Create(domain.CanvasMapCreate{
		Name:   "Operations",
		Filter: domain.CanvasMapFilter{DeviceIDs: []uuid.UUID{}},
	})
	if err != nil {
		t.Fatalf("Create canvas map failed: %v", err)
	}
	if err := mapRepo.ReplaceMembership(canvasMap.ID, domain.CanvasMapMembership{
		Devices: []domain.CanvasMapDeviceMembership{{
			DeviceID: mapped.ID,
			Role:     domain.CanvasMapDeviceRoleBase,
		}},
	}); err != nil {
		t.Fatalf("ReplaceMembership failed: %v", err)
	}

	devices, err := repo.GetOrphans()
	if err != nil {
		t.Fatalf("GetOrphans failed: %v", err)
	}
	if len(devices) != 1 {
		t.Fatalf("expected 1 orphan device, got %d", len(devices))
	}
	if devices[0].ID != orphan.ID {
		t.Fatalf("expected orphan device %s, got %s", orphan.ID, devices[0].ID)
	}
	if len(devices[0].Interfaces) != 1 || devices[0].Interfaces[0].IfName != "ether7" {
		t.Fatalf("expected orphan interfaces to be loaded, got %#v", devices[0].Interfaces)
	}
	if len(devices[0].AreaIDs) != 1 || devices[0].AreaIDs[0] != area.ID {
		t.Fatalf("expected orphan area IDs to be loaded, got %#v", devices[0].AreaIDs)
	}
}

func TestDeviceRepoGetBySysName_EmptyOrUnknownLookup(t *testing.T) {
	db := newTestDB(t)
	repo := NewDeviceRepo(db, testKeyring, nil)

	device := &domain.Device{
		ID:       uuid.New(),
		Hostname: "edge-sw-01",
		IP:       "10.0.0.3",
		SysName:  "edge-sw-01",
		Managed:  true,
		Status:   domain.DeviceStatusUp,
		Tags:     map[string]string{},
		SNMPCredentials: domain.SNMPCredentials{
			Version: domain.SNMPVersionV2c,
			V2c:     &domain.SNMPv2cCredentials{Community: "public"},
		},
	}
	if err := repo.Create(device); err != nil {
		t.Fatalf("Create device failed: %v", err)
	}

	for _, lookup := range []string{"", "   ", ".", "unknown-host.example.net"} {
		result, err := repo.GetBySysName(lookup)
		if err != nil {
			t.Fatalf("GetBySysName(%q) failed: %v", lookup, err)
		}
		if lookup == "unknown-host.example.net" && result != nil {
			t.Fatalf("expected nil for unknown lookup %q, got %s", lookup, result.ID)
		}
		if lookup != "unknown-host.example.net" && result != nil {
			t.Fatalf("expected nil for blank-equivalent lookup %q, got %s", lookup, result.ID)
		}
	}
}

func TestDeviceRepoGetBySysName_UpdateRefreshesLookupIndex(t *testing.T) {
	db := newTestDB(t)
	repo := NewDeviceRepo(db, testKeyring, nil)

	device := &domain.Device{
		ID:       uuid.New(),
		Hostname: "agg-01",
		IP:       "10.0.0.4",
		SysName:  "agg-01.old.example.net",
		Managed:  true,
		Status:   domain.DeviceStatusUp,
		Tags:     map[string]string{},
		SNMPCredentials: domain.SNMPCredentials{
			Version: domain.SNMPVersionV2c,
			V2c:     &domain.SNMPv2cCredentials{Community: "public"},
		},
	}
	if err := repo.Create(device); err != nil {
		t.Fatalf("Create device failed: %v", err)
	}

	device.SysName = "agg-02.new.example.net."
	if err := repo.Update(device); err != nil {
		t.Fatalf("Update device failed: %v", err)
	}

	oldLookup, err := repo.GetBySysName("agg-01.old.example.net")
	if err != nil {
		t.Fatalf("GetBySysName(old) failed: %v", err)
	}
	if oldLookup != nil {
		t.Fatalf("expected old lookup to be empty, got %s", oldLookup.ID)
	}

	newLookup, err := repo.GetBySysName("AGG-02.NEW.EXAMPLE.NET")
	if err != nil {
		t.Fatalf("GetBySysName(new) failed: %v", err)
	}
	if newLookup == nil {
		t.Fatal("expected updated device to be found by new lookup")
	}
	if newLookup.ID != device.ID {
		t.Fatalf("expected device %s, got %s", device.ID, newLookup.ID)
	}
}

// intPtr is a local helper to create a *int from a literal int value.
func intPtr(i int) *int { return &i }

// ---------------------------------------------------------------------------
// TestDeviceRepo_PollClassRoundTrip (Phase 39 Plan 03)
// ---------------------------------------------------------------------------
// Verifies that PollClass=PollClassCore round-trips through Create → GetByID.
func TestDeviceRepo_PollClassRoundTrip(t *testing.T) {
	db := newTestDB(t)
	repo := NewDeviceRepo(db, testKeyring, nil)

	device := &domain.Device{
		ID:        uuid.New(),
		Hostname:  "core-router-01",
		IP:        "10.1.0.1",
		PollClass: domain.PollClassCore,
		Status:    domain.DeviceStatusUnknown,
		Tags:      map[string]string{},
		SNMPCredentials: domain.SNMPCredentials{
			Version: domain.SNMPVersionV2c,
			V2c:     &domain.SNMPv2cCredentials{Community: "public"},
		},
	}

	if err := repo.Create(device); err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	got, err := repo.GetByID(device.ID)
	if err != nil {
		t.Fatalf("GetByID failed: %v", err)
	}

	if got.PollClass != domain.PollClassCore {
		t.Errorf("PollClass: got %q, want %q", got.PollClass, domain.PollClassCore)
	}
	if got.PollIntervalOverride != nil {
		t.Errorf("PollIntervalOverride: got %v, want nil", got.PollIntervalOverride)
	}
}

// ---------------------------------------------------------------------------
// TestDeviceRepo_PollIntervalOverrideRoundTrip (Phase 39 Plan 03)
// ---------------------------------------------------------------------------
// Verifies that a non-nil PollIntervalOverride persists and can be cleared.
func TestDeviceRepo_PollIntervalOverrideRoundTrip(t *testing.T) {
	db := newTestDB(t)
	repo := NewDeviceRepo(db, testKeyring, nil)

	device := &domain.Device{
		ID:                   uuid.New(),
		Hostname:             "standard-ap-01",
		IP:                   "10.1.0.2",
		PollClass:            domain.PollClassStandard,
		PollIntervalOverride: intPtr(15),
		Status:               domain.DeviceStatusUnknown,
		Tags:                 map[string]string{},
		SNMPCredentials: domain.SNMPCredentials{
			Version: domain.SNMPVersionV2c,
			V2c:     &domain.SNMPv2cCredentials{Community: "public"},
		},
	}

	if err := repo.Create(device); err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	got, err := repo.GetByID(device.ID)
	if err != nil {
		t.Fatalf("GetByID after Create failed: %v", err)
	}
	if got.PollIntervalOverride == nil {
		t.Fatal("PollIntervalOverride: got nil, want non-nil")
	}
	if *got.PollIntervalOverride != 15 {
		t.Errorf("PollIntervalOverride: got %d, want 15", *got.PollIntervalOverride)
	}

	// Clear the override via Update.
	got.PollIntervalOverride = nil
	if err := repo.Update(got); err != nil {
		t.Fatalf("Update failed: %v", err)
	}

	updated, err := repo.GetByID(device.ID)
	if err != nil {
		t.Fatalf("GetByID after Update failed: %v", err)
	}
	if updated.PollIntervalOverride != nil {
		t.Errorf("PollIntervalOverride after clear: got %v, want nil", updated.PollIntervalOverride)
	}
}

func TestDeviceRepo_PollingEnabledDefaultsTrueAndRoundTripsFalse(t *testing.T) {
	db := newTestDB(t)
	repo := NewDeviceRepo(db, testKeyring, nil)

	device := &domain.Device{
		ID:        uuid.New(),
		Hostname:  "polling-router",
		IP:        "10.1.0.5",
		PollClass: domain.PollClassCore,
		Status:    domain.DeviceStatusUnknown,
		Managed:   true,
		Tags:      map[string]string{},
		SNMPCredentials: domain.SNMPCredentials{
			Version: domain.SNMPVersionV2c,
			V2c:     &domain.SNMPv2cCredentials{Community: "public"},
		},
	}

	if err := repo.Create(device); err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	got, err := repo.GetByID(device.ID)
	if err != nil {
		t.Fatalf("GetByID after Create failed: %v", err)
	}
	if !domain.DevicePollingEnabled(*got) {
		t.Fatalf("PollingEnabled after Create = false, want true")
	}

	disabled := false
	got.PollingEnabled = &disabled
	if err := repo.Update(got); err != nil {
		t.Fatalf("Update failed: %v", err)
	}

	updated, err := repo.GetByID(device.ID)
	if err != nil {
		t.Fatalf("GetByID after Update failed: %v", err)
	}
	if domain.DevicePollingEnabled(*updated) {
		t.Fatalf("PollingEnabled after Update = true, want false")
	}

	all, err := repo.GetAll()
	if err != nil {
		t.Fatalf("GetAll failed: %v", err)
	}
	for _, candidate := range all {
		if candidate.ID == device.ID {
			if domain.DevicePollingEnabled(candidate) {
				t.Fatalf("PollingEnabled from GetAll = true, want false")
			}
			return
		}
	}
	t.Fatalf("device %s missing from GetAll", device.ID)
}

// ---------------------------------------------------------------------------
// TestDeviceRepo_PollClassEmptyDefaultsToStandard (Phase 39 Plan 03)
// ---------------------------------------------------------------------------
// Verifies that an empty PollClass is normalized to PollClassStandard by createOnce.
func TestDeviceRepo_PollClassEmptyDefaultsToStandard(t *testing.T) {
	db := newTestDB(t)
	repo := NewDeviceRepo(db, testKeyring, nil)

	device := &domain.Device{
		ID:        uuid.New(),
		Hostname:  "virtual-node-01",
		IP:        "10.1.0.3",
		PollClass: "", // zero value — should default to PollClassStandard
		Status:    domain.DeviceStatusUnknown,
		Tags:      map[string]string{},
		SNMPCredentials: domain.SNMPCredentials{
			Version: domain.SNMPVersionV2c,
			V2c:     &domain.SNMPv2cCredentials{Community: "public"},
		},
	}

	if err := repo.Create(device); err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	got, err := repo.GetByID(device.ID)
	if err != nil {
		t.Fatalf("GetByID failed: %v", err)
	}
	if got.PollClass != domain.PollClassStandard {
		t.Errorf("PollClass: got %q, want %q (empty should default to standard)", got.PollClass, domain.PollClassStandard)
	}
}

func TestDeviceRepo_NotesRoundTrip(t *testing.T) {
	db := newTestDB(t)
	repo := NewDeviceRepo(db, testKeyring, nil)

	notes := "Installed in rack A3"
	device := &domain.Device{
		ID:       uuid.New(),
		Hostname: "notes-router",
		IP:       "10.1.0.4",
		Notes:    &notes,
		Status:   domain.DeviceStatusUnknown,
		Tags:     map[string]string{},
		SNMPCredentials: domain.SNMPCredentials{
			Version: domain.SNMPVersionV2c,
			V2c:     &domain.SNMPv2cCredentials{Community: "public"},
		},
	}

	if err := repo.Create(device); err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	got, err := repo.GetByID(device.ID)
	if err != nil {
		t.Fatalf("GetByID failed: %v", err)
	}
	if got.Notes == nil || *got.Notes != notes {
		t.Fatalf("Notes after Create: got %#v, want %q", got.Notes, notes)
	}

	got.Notes = nil
	if err := repo.Update(got); err != nil {
		t.Fatalf("Update failed: %v", err)
	}

	updated, err := repo.GetByID(device.ID)
	if err != nil {
		t.Fatalf("GetByID after Update failed: %v", err)
	}
	if updated.Notes != nil {
		t.Fatalf("Notes after clear: got %#v, want nil", updated.Notes)
	}
}

type addressExpectation struct {
	address   string
	label     string
	role      domain.DeviceAddressRole
	isPrimary bool
	priority  int
}

func assertDeviceAddresses(t *testing.T, got []domain.DeviceAddress, want []addressExpectation) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("addresses len = %d, want %d: %#v", len(got), len(want), got)
	}
	for i := range want {
		if got[i].Address != want[i].address {
			t.Fatalf("address[%d].Address = %q, want %q", i, got[i].Address, want[i].address)
		}
		if got[i].Label != want[i].label {
			t.Fatalf("address[%d].Label = %q, want %q", i, got[i].Label, want[i].label)
		}
		if got[i].Role != want[i].role {
			t.Fatalf("address[%d].Role = %q, want %q", i, got[i].Role, want[i].role)
		}
		if got[i].IsPrimary != want[i].isPrimary {
			t.Fatalf("address[%d].IsPrimary = %v, want %v", i, got[i].IsPrimary, want[i].isPrimary)
		}
		if got[i].Priority != want[i].priority {
			t.Fatalf("address[%d].Priority = %d, want %d", i, got[i].Priority, want[i].priority)
		}
		if got[i].ID == uuid.Nil {
			t.Fatalf("address[%d].ID is nil", i)
		}
		if got[i].DeviceID == uuid.Nil {
			t.Fatalf("address[%d].DeviceID is nil", i)
		}
	}
}

func assertDeviceFromListHasAddress(t *testing.T, devices []domain.Device, deviceID uuid.UUID, address string) {
	t.Helper()
	for _, device := range devices {
		if device.ID != deviceID {
			continue
		}
		for _, candidate := range device.Addresses {
			if candidate.Address == address {
				return
			}
		}
		t.Fatalf("device %s addresses = %#v, want address %q", deviceID, device.Addresses, address)
	}
	t.Fatalf("device %s not found in list %#v", deviceID, devices)
}

func isDeviceIPInvariantError(err error) bool {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return pgErr.Code == "23505" || pgErr.Code == "23P01"
	}
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "idx_devices_ip") ||
		strings.Contains(message, "devices_ip_physical_virtual_excl") ||
		strings.Contains(message, "duplicate key") ||
		strings.Contains(message, "exclusion constraint")
}
