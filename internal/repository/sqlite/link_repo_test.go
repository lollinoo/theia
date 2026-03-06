package sqlite

import (
	"testing"

	"github.com/azmin/mikrotik-theia/internal/domain"
	"github.com/google/uuid"
	_ "github.com/mattn/go-sqlite3"
)

func createTestDevicePair(t *testing.T, repo *DeviceRepo) (uuid.UUID, uuid.UUID) {
	t.Helper()
	d1 := &domain.Device{
		IP:       "10.1.0.1",
		Hostname: "device-A",
		SNMPCredentials: domain.SNMPCredentials{
			Version: domain.SNMPVersionV2c,
			V2c:     &domain.SNMPv2cCredentials{Community: "public"},
		},
		DeviceType: domain.DeviceTypeRouter,
		Status:     domain.DeviceStatusUp,
		Managed:    true,
	}
	d2 := &domain.Device{
		IP:       "10.1.0.2",
		Hostname: "device-B",
		SNMPCredentials: domain.SNMPCredentials{
			Version: domain.SNMPVersionV2c,
			V2c:     &domain.SNMPv2cCredentials{Community: "public"},
		},
		DeviceType: domain.DeviceTypeSwitch,
		Status:     domain.DeviceStatusUp,
		Managed:    true,
	}

	if err := repo.Create(d1); err != nil {
		t.Fatalf("Create device A: %v", err)
	}
	if err := repo.Create(d2); err != nil {
		t.Fatalf("Create device B: %v", err)
	}

	return d1.ID, d2.ID
}

func TestLinkRepo_CreateAndGetByDeviceID(t *testing.T) {
	db := setupTestDB(t)
	deviceRepo := NewDeviceRepo(db)
	linkRepo := NewLinkRepo(db)

	d1ID, d2ID := createTestDevicePair(t, deviceRepo)

	link := &domain.Link{
		SourceDeviceID:    d1ID,
		SourceIfName:      "ether1",
		TargetDeviceID:    d2ID,
		TargetIfName:      "ether1",
		DiscoveryProtocol: domain.DiscoveryProtocolLLDP,
	}

	if err := linkRepo.Create(link); err != nil {
		t.Fatalf("Create link: %v", err)
	}

	if link.ID == uuid.Nil {
		t.Fatal("Expected link ID to be assigned")
	}

	// Should appear when querying by source device
	links, err := linkRepo.GetByDeviceID(d1ID)
	if err != nil {
		t.Fatalf("GetByDeviceID(source): %v", err)
	}
	if len(links) != 1 {
		t.Fatalf("Expected 1 link for source device, got %d", len(links))
	}
	if links[0].SourceIfName != "ether1" {
		t.Errorf("SourceIfName = %q, want %q", links[0].SourceIfName, "ether1")
	}
	if links[0].DiscoveryProtocol != domain.DiscoveryProtocolLLDP {
		t.Errorf("DiscoveryProtocol = %q, want %q", links[0].DiscoveryProtocol, domain.DiscoveryProtocolLLDP)
	}

	// Should also appear when querying by target device
	links2, err := linkRepo.GetByDeviceID(d2ID)
	if err != nil {
		t.Fatalf("GetByDeviceID(target): %v", err)
	}
	if len(links2) != 1 {
		t.Fatalf("Expected 1 link for target device, got %d", len(links2))
	}
}

func TestLinkRepo_Upsert_InsertNew(t *testing.T) {
	db := setupTestDB(t)
	deviceRepo := NewDeviceRepo(db)
	linkRepo := NewLinkRepo(db)

	d1ID, d2ID := createTestDevicePair(t, deviceRepo)

	link := &domain.Link{
		SourceDeviceID:    d1ID,
		SourceIfName:      "ether2",
		TargetDeviceID:    d2ID,
		TargetIfName:      "ether2",
		DiscoveryProtocol: domain.DiscoveryProtocolCDP,
	}

	if err := linkRepo.Upsert(link); err != nil {
		t.Fatalf("Upsert (insert): %v", err)
	}

	if link.ID == uuid.Nil {
		t.Fatal("Expected link ID to be assigned after Upsert")
	}

	all, err := linkRepo.GetAll()
	if err != nil {
		t.Fatalf("GetAll: %v", err)
	}
	if len(all) != 1 {
		t.Fatalf("Expected 1 link, got %d", len(all))
	}
	if all[0].DiscoveryProtocol != domain.DiscoveryProtocolCDP {
		t.Errorf("DiscoveryProtocol = %q, want %q", all[0].DiscoveryProtocol, domain.DiscoveryProtocolCDP)
	}
}

func TestLinkRepo_Upsert_UpdateExisting(t *testing.T) {
	db := setupTestDB(t)
	deviceRepo := NewDeviceRepo(db)
	linkRepo := NewLinkRepo(db)

	d1ID, d2ID := createTestDevicePair(t, deviceRepo)

	// First upsert (insert)
	link1 := &domain.Link{
		SourceDeviceID:    d1ID,
		SourceIfName:      "ether3",
		TargetDeviceID:    d2ID,
		TargetIfName:      "ether3",
		DiscoveryProtocol: domain.DiscoveryProtocolLLDP,
	}
	if err := linkRepo.Upsert(link1); err != nil {
		t.Fatalf("Upsert (insert): %v", err)
	}
	originalID := link1.ID

	// Second upsert (update) — same interface pair, different protocol
	link2 := &domain.Link{
		SourceDeviceID:    d1ID,
		SourceIfName:      "ether3",
		TargetDeviceID:    d2ID,
		TargetIfName:      "ether3",
		DiscoveryProtocol: domain.DiscoveryProtocolCDP,
	}
	if err := linkRepo.Upsert(link2); err != nil {
		t.Fatalf("Upsert (update): %v", err)
	}

	// Should still have only 1 link
	all, err := linkRepo.GetAll()
	if err != nil {
		t.Fatalf("GetAll: %v", err)
	}
	if len(all) != 1 {
		t.Fatalf("Expected 1 link after upsert, got %d", len(all))
	}

	// ID should be preserved from the first insert
	if all[0].ID != originalID {
		t.Errorf("Link ID changed after upsert: got %s, want %s", all[0].ID, originalID)
	}
	// Protocol should be updated
	if all[0].DiscoveryProtocol != domain.DiscoveryProtocolCDP {
		t.Errorf("DiscoveryProtocol = %q, want %q", all[0].DiscoveryProtocol, domain.DiscoveryProtocolCDP)
	}
}

func TestLinkRepo_Delete(t *testing.T) {
	db := setupTestDB(t)
	deviceRepo := NewDeviceRepo(db)
	linkRepo := NewLinkRepo(db)

	d1ID, d2ID := createTestDevicePair(t, deviceRepo)

	link := &domain.Link{
		SourceDeviceID:    d1ID,
		SourceIfName:      "ether1",
		TargetDeviceID:    d2ID,
		TargetIfName:      "ether1",
		DiscoveryProtocol: domain.DiscoveryProtocolLLDP,
	}
	if err := linkRepo.Create(link); err != nil {
		t.Fatalf("Create: %v", err)
	}

	if err := linkRepo.Delete(link.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	all, err := linkRepo.GetAll()
	if err != nil {
		t.Fatalf("GetAll: %v", err)
	}
	if len(all) != 0 {
		t.Errorf("Expected 0 links after delete, got %d", len(all))
	}
}

func TestLinkRepo_GetAll(t *testing.T) {
	db := setupTestDB(t)
	deviceRepo := NewDeviceRepo(db)
	linkRepo := NewLinkRepo(db)

	d1ID, d2ID := createTestDevicePair(t, deviceRepo)

	links := []*domain.Link{
		{SourceDeviceID: d1ID, SourceIfName: "ether1", TargetDeviceID: d2ID, TargetIfName: "ether1", DiscoveryProtocol: domain.DiscoveryProtocolLLDP},
		{SourceDeviceID: d1ID, SourceIfName: "ether2", TargetDeviceID: d2ID, TargetIfName: "ether2", DiscoveryProtocol: domain.DiscoveryProtocolCDP},
	}

	for i, l := range links {
		if err := linkRepo.Create(l); err != nil {
			t.Fatalf("Create link %d: %v", i, err)
		}
	}

	all, err := linkRepo.GetAll()
	if err != nil {
		t.Fatalf("GetAll: %v", err)
	}
	if len(all) != 2 {
		t.Errorf("GetAll returned %d links, want 2", len(all))
	}
}

func TestSettingsRepo_GetSetGetAll(t *testing.T) {
	db := setupTestDB(t)
	repo := NewSettingsRepo(db)

	// Defaults should be seeded by migrations
	all, err := repo.GetAll()
	if err != nil {
		t.Fatalf("GetAll: %v", err)
	}
	if len(all) == 0 {
		t.Fatal("Expected default settings to be seeded, got 0")
	}

	// Check a default value
	val, err := repo.Get(domain.SettingPollingInterval)
	if err != nil {
		t.Fatalf("Get polling_interval: %v", err)
	}
	if val != "60" {
		t.Errorf("Default polling_interval = %q, want %q", val, "60")
	}

	// Set a custom value
	if err := repo.Set(domain.SettingPrometheusURL, "http://prometheus:9090"); err != nil {
		t.Fatalf("Set prometheus_url: %v", err)
	}

	val2, err := repo.Get(domain.SettingPrometheusURL)
	if err != nil {
		t.Fatalf("Get prometheus_url: %v", err)
	}
	if val2 != "http://prometheus:9090" {
		t.Errorf("prometheus_url = %q, want %q", val2, "http://prometheus:9090")
	}

	// Set a new key
	if err := repo.Set("custom_key", "custom_value"); err != nil {
		t.Fatalf("Set custom_key: %v", err)
	}

	val3, err := repo.Get("custom_key")
	if err != nil {
		t.Fatalf("Get custom_key: %v", err)
	}
	if val3 != "custom_value" {
		t.Errorf("custom_key = %q, want %q", val3, "custom_value")
	}

	// Error for non-existent key
	_, err = repo.Get("nonexistent")
	if err == nil {
		t.Error("Expected error for nonexistent key")
	}
}
