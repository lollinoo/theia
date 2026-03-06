package sqlite

import (
	"database/sql"
	"testing"

	"github.com/azmin/mikrotik-theia/internal/domain"
	"github.com/google/uuid"
	_ "github.com/mattn/go-sqlite3"
)

func setupTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite3", ":memory:?_foreign_keys=on")
	if err != nil {
		t.Fatalf("opening test db: %v", err)
	}
	if err := RunMigrations(db); err != nil {
		t.Fatalf("running migrations: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func TestDeviceRepo_CreateAndGetByID(t *testing.T) {
	db := setupTestDB(t)
	repo := NewDeviceRepo(db)

	device := &domain.Device{
		Hostname: "core-router-01",
		IP:       "192.168.1.1",
		SNMPCredentials: domain.SNMPCredentials{
			Version: domain.SNMPVersionV2c,
			V2c:     &domain.SNMPv2cCredentials{Community: "public"},
		},
		DeviceType: domain.DeviceTypeRouter,
		Status:     domain.DeviceStatusUp,
		SysName:    "core-router-01.local",
		SysDescr:   "RouterOS v7.14",
		Managed:    true,
		Tags:       map[string]string{"site": "datacenter-1", "role": "core"},
		Interfaces: []domain.Interface{
			{IfIndex: 1, IfName: "ether1", IfDescr: "Ethernet 1", Speed: 1000000000, AdminStatus: "up", OperStatus: "up"},
			{IfIndex: 2, IfName: "ether2", IfDescr: "Ethernet 2", Speed: 1000000000, AdminStatus: "up", OperStatus: "down"},
		},
	}

	if err := repo.Create(device); err != nil {
		t.Fatalf("Create: %v", err)
	}

	if device.ID == uuid.Nil {
		t.Fatal("Expected UUID to be assigned after Create")
	}

	got, err := repo.GetByID(device.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}

	if got.Hostname != "core-router-01" {
		t.Errorf("Hostname = %q, want %q", got.Hostname, "core-router-01")
	}
	if got.IP != "192.168.1.1" {
		t.Errorf("IP = %q, want %q", got.IP, "192.168.1.1")
	}
	if got.DeviceType != domain.DeviceTypeRouter {
		t.Errorf("DeviceType = %q, want %q", got.DeviceType, domain.DeviceTypeRouter)
	}
	if got.Status != domain.DeviceStatusUp {
		t.Errorf("Status = %q, want %q", got.Status, domain.DeviceStatusUp)
	}
	if got.SysName != "core-router-01.local" {
		t.Errorf("SysName = %q, want %q", got.SysName, "core-router-01.local")
	}
	if !got.Managed {
		t.Error("Expected Managed to be true")
	}
	if got.Tags["site"] != "datacenter-1" {
		t.Errorf("Tags[site] = %q, want %q", got.Tags["site"], "datacenter-1")
	}
	if got.Tags["role"] != "core" {
		t.Errorf("Tags[role] = %q, want %q", got.Tags["role"], "core")
	}
	if len(got.Interfaces) != 2 {
		t.Fatalf("Interfaces count = %d, want 2", len(got.Interfaces))
	}
	if got.Interfaces[0].IfName != "ether1" {
		t.Errorf("Interface[0].IfName = %q, want %q", got.Interfaces[0].IfName, "ether1")
	}
	if got.Interfaces[1].OperStatus != "down" {
		t.Errorf("Interface[1].OperStatus = %q, want %q", got.Interfaces[1].OperStatus, "down")
	}
}

func TestDeviceRepo_SNMPv2cRoundTrip(t *testing.T) {
	db := setupTestDB(t)
	repo := NewDeviceRepo(db)

	device := &domain.Device{
		IP:       "10.0.0.1",
		Hostname: "test-v2c",
		SNMPCredentials: domain.SNMPCredentials{
			Version: domain.SNMPVersionV2c,
			V2c:     &domain.SNMPv2cCredentials{Community: "secret_community"},
		},
		DeviceType: domain.DeviceTypeSwitch,
		Status:     domain.DeviceStatusProbing,
		Managed:    true,
	}

	if err := repo.Create(device); err != nil {
		t.Fatalf("Create: %v", err)
	}

	got, err := repo.GetByID(device.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}

	if got.SNMPCredentials.Version != domain.SNMPVersionV2c {
		t.Errorf("SNMP Version = %q, want %q", got.SNMPCredentials.Version, domain.SNMPVersionV2c)
	}
	if got.SNMPCredentials.V2c == nil {
		t.Fatal("Expected V2c credentials to be non-nil")
	}
	if got.SNMPCredentials.V2c.Community != "secret_community" {
		t.Errorf("V2c.Community = %q, want %q", got.SNMPCredentials.V2c.Community, "secret_community")
	}
}

func TestDeviceRepo_SNMPv3RoundTrip(t *testing.T) {
	db := setupTestDB(t)
	repo := NewDeviceRepo(db)

	device := &domain.Device{
		IP:       "10.0.0.2",
		Hostname: "test-v3",
		SNMPCredentials: domain.SNMPCredentials{
			Version: domain.SNMPVersionV3,
			V3: &domain.SNMPv3Credentials{
				Username:      "admin",
				AuthProtocol:  "SHA",
				AuthPassword:  "authpass123",
				PrivProtocol:  "AES",
				PrivPassword:  "privpass456",
				SecurityLevel: "authPriv",
			},
		},
		DeviceType: domain.DeviceTypeRouter,
		Status:     domain.DeviceStatusUp,
		Managed:    true,
	}

	if err := repo.Create(device); err != nil {
		t.Fatalf("Create: %v", err)
	}

	got, err := repo.GetByID(device.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}

	if got.SNMPCredentials.Version != domain.SNMPVersionV3 {
		t.Errorf("SNMP Version = %q, want %q", got.SNMPCredentials.Version, domain.SNMPVersionV3)
	}
	if got.SNMPCredentials.V3 == nil {
		t.Fatal("Expected V3 credentials to be non-nil")
	}
	v3 := got.SNMPCredentials.V3
	if v3.Username != "admin" {
		t.Errorf("V3.Username = %q, want %q", v3.Username, "admin")
	}
	if v3.AuthProtocol != "SHA" {
		t.Errorf("V3.AuthProtocol = %q, want %q", v3.AuthProtocol, "SHA")
	}
	if v3.AuthPassword != "authpass123" {
		t.Errorf("V3.AuthPassword = %q, want %q", v3.AuthPassword, "authpass123")
	}
	if v3.PrivProtocol != "AES" {
		t.Errorf("V3.PrivProtocol = %q, want %q", v3.PrivProtocol, "AES")
	}
	if v3.PrivPassword != "privpass456" {
		t.Errorf("V3.PrivPassword = %q, want %q", v3.PrivPassword, "privpass456")
	}
	if v3.SecurityLevel != "authPriv" {
		t.Errorf("V3.SecurityLevel = %q, want %q", v3.SecurityLevel, "authPriv")
	}
}

func TestDeviceRepo_Update(t *testing.T) {
	db := setupTestDB(t)
	repo := NewDeviceRepo(db)

	device := &domain.Device{
		IP:       "10.0.0.3",
		Hostname: "old-hostname",
		SNMPCredentials: domain.SNMPCredentials{
			Version: domain.SNMPVersionV2c,
			V2c:     &domain.SNMPv2cCredentials{Community: "public"},
		},
		DeviceType: domain.DeviceTypeUnknown,
		Status:     domain.DeviceStatusProbing,
		Managed:    true,
	}

	if err := repo.Create(device); err != nil {
		t.Fatalf("Create: %v", err)
	}

	device.Hostname = "new-hostname"
	device.DeviceType = domain.DeviceTypeSwitch
	device.Status = domain.DeviceStatusUp
	device.Tags = map[string]string{"updated": "true"}

	if err := repo.Update(device); err != nil {
		t.Fatalf("Update: %v", err)
	}

	got, err := repo.GetByID(device.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}

	if got.Hostname != "new-hostname" {
		t.Errorf("Hostname = %q, want %q", got.Hostname, "new-hostname")
	}
	if got.DeviceType != domain.DeviceTypeSwitch {
		t.Errorf("DeviceType = %q, want %q", got.DeviceType, domain.DeviceTypeSwitch)
	}
	if got.Status != domain.DeviceStatusUp {
		t.Errorf("Status = %q, want %q", got.Status, domain.DeviceStatusUp)
	}
	if got.Tags["updated"] != "true" {
		t.Errorf("Tags[updated] = %q, want %q", got.Tags["updated"], "true")
	}
}

func TestDeviceRepo_Delete(t *testing.T) {
	db := setupTestDB(t)
	repo := NewDeviceRepo(db)

	device := &domain.Device{
		IP:       "10.0.0.4",
		Hostname: "to-delete",
		SNMPCredentials: domain.SNMPCredentials{
			Version: domain.SNMPVersionV2c,
			V2c:     &domain.SNMPv2cCredentials{Community: "public"},
		},
		DeviceType: domain.DeviceTypeAP,
		Status:     domain.DeviceStatusUp,
		Managed:    true,
	}

	if err := repo.Create(device); err != nil {
		t.Fatalf("Create: %v", err)
	}

	if err := repo.Delete(device.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	_, err := repo.GetByID(device.ID)
	if err == nil {
		t.Error("Expected error after deleting device, got nil")
	}
}

func TestDeviceRepo_GetAll(t *testing.T) {
	db := setupTestDB(t)
	repo := NewDeviceRepo(db)

	for i, ip := range []string{"10.0.0.10", "10.0.0.11", "10.0.0.12"} {
		device := &domain.Device{
			IP:       ip,
			Hostname: "device-" + string(rune('A'+i)),
			SNMPCredentials: domain.SNMPCredentials{
				Version: domain.SNMPVersionV2c,
				V2c:     &domain.SNMPv2cCredentials{Community: "public"},
			},
			DeviceType: domain.DeviceTypeRouter,
			Status:     domain.DeviceStatusUp,
			Managed:    true,
		}
		if err := repo.Create(device); err != nil {
			t.Fatalf("Create device %d: %v", i, err)
		}
	}

	devices, err := repo.GetAll()
	if err != nil {
		t.Fatalf("GetAll: %v", err)
	}

	if len(devices) != 3 {
		t.Errorf("GetAll returned %d devices, want 3", len(devices))
	}
}

func TestDeviceRepo_GetByIP(t *testing.T) {
	db := setupTestDB(t)
	repo := NewDeviceRepo(db)

	device := &domain.Device{
		IP:       "192.168.50.1",
		Hostname: "by-ip-test",
		SNMPCredentials: domain.SNMPCredentials{
			Version: domain.SNMPVersionV2c,
			V2c:     &domain.SNMPv2cCredentials{Community: "public"},
		},
		DeviceType: domain.DeviceTypeRouter,
		Status:     domain.DeviceStatusUp,
		Managed:    true,
	}

	if err := repo.Create(device); err != nil {
		t.Fatalf("Create: %v", err)
	}

	got, err := repo.GetByIP("192.168.50.1")
	if err != nil {
		t.Fatalf("GetByIP: %v", err)
	}
	if got == nil {
		t.Fatal("Expected device, got nil")
	}
	if got.Hostname != "by-ip-test" {
		t.Errorf("Hostname = %q, want %q", got.Hostname, "by-ip-test")
	}

	// Non-existent IP should return nil
	notFound, err := repo.GetByIP("10.99.99.99")
	if err != nil {
		t.Fatalf("GetByIP for non-existent: %v", err)
	}
	if notFound != nil {
		t.Error("Expected nil for non-existent IP")
	}
}

func TestDeviceRepo_TagsRoundTrip(t *testing.T) {
	db := setupTestDB(t)
	repo := NewDeviceRepo(db)

	tags := map[string]string{
		"site":     "datacenter-2",
		"role":     "access",
		"floor":    "3",
		"building": "HQ",
	}

	device := &domain.Device{
		IP:       "10.0.0.20",
		Hostname: "tags-test",
		SNMPCredentials: domain.SNMPCredentials{
			Version: domain.SNMPVersionV2c,
			V2c:     &domain.SNMPv2cCredentials{Community: "public"},
		},
		DeviceType: domain.DeviceTypeSwitch,
		Status:     domain.DeviceStatusUp,
		Managed:    true,
		Tags:       tags,
	}

	if err := repo.Create(device); err != nil {
		t.Fatalf("Create: %v", err)
	}

	got, err := repo.GetByID(device.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}

	if len(got.Tags) != 4 {
		t.Fatalf("Tags count = %d, want 4", len(got.Tags))
	}
	for k, v := range tags {
		if got.Tags[k] != v {
			t.Errorf("Tags[%q] = %q, want %q", k, got.Tags[k], v)
		}
	}
}
