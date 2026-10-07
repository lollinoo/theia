package postgres

import (
	"github.com/google/uuid"
	"github.com/lollinoo/theia/internal/domain"
	"testing"
)

func TestDeviceStatusPreservesConfigurationAndRelations(t *testing.T) {
	db := setupTestDB(t)
	repo := NewDeviceRepo(db, testKeyring, nil)
	device := &domain.Device{ID: uuid.New(), IP: "192.0.2.1", Hostname: "edited", Status: domain.DeviceStatusDown,
		MetricsSource: domain.MetricsSourceSNMP, Tags: map[string]string{"owner": "changed"},
		SNMPCredentials: domain.SNMPCredentials{Version: domain.SNMPVersionV2c, V2c: &domain.SNMPv2cCredentials{Community: "new-secret"}},
		Interfaces:      []domain.Interface{{IfIndex: 1, IfName: "eth0"}},
	}
	if err := repo.Create(device); err != nil {
		t.Fatal(err)
	}
	var encryptedBefore string
	if err := db.QueryRow("SELECT snmp_credentials_json::text FROM devices WHERE id=$1", device.ID).Scan(&encryptedBefore); err != nil {
		t.Fatal(err)
	}
	before, err := repo.GetByID(device.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err := repo.UpdateStatus(device.ID, domain.DeviceStatusUp); err != nil {
		t.Fatal(err)
	}
	after, err := repo.GetByID(device.ID)
	if err != nil {
		t.Fatal(err)
	}
	var encryptedAfter string
	if err := db.QueryRow("SELECT snmp_credentials_json::text FROM devices WHERE id=$1", device.ID).Scan(&encryptedAfter); err != nil {
		t.Fatal(err)
	}
	if after.Status != domain.DeviceStatusUp || after.Hostname != before.Hostname || encryptedAfter != encryptedBefore {
		t.Fatalf("status update changed configuration: %+v", after)
	}
	if len(after.Interfaces) != 1 || after.Interfaces[0].ID != before.Interfaces[0].ID || after.Addresses[0].ID != before.Addresses[0].ID {
		t.Fatal("status update replaced relationships")
	}
	if err := repo.UpdateStatus(uuid.New(), domain.DeviceStatusUp); err == nil {
		t.Fatal("missing device update succeeded")
	}
}
