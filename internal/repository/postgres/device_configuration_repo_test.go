package postgres

import (
	"testing"

	"github.com/google/uuid"
	"github.com/lollinoo/theia/internal/domain"
)

func TestDeviceConfigurationPreservesConcurrentFieldsAndRelations(t *testing.T) {
	db := setupTestDB(t)
	repo := NewDeviceRepo(db, testKeyring, nil)
	device := &domain.Device{ID: uuid.New(), IP: "192.0.2.1", Hostname: "before", Status: domain.DeviceStatusDown,
		MetricsSource: domain.MetricsSourceSNMP, Tags: map[string]string{},
		SNMPCredentials: domain.SNMPCredentials{Version: domain.SNMPVersionV2c, V2c: &domain.SNMPv2cCredentials{Community: "secret"}},
		Interfaces:      []domain.Interface{{IfIndex: 1, IfName: "eth0"}},
	}
	if err := repo.Create(device); err != nil {
		t.Fatal(err)
	}
	before, err := repo.GetByID(device.ID)
	if err != nil {
		t.Fatal(err)
	}
	var secretBefore string
	if err := db.QueryRow("SELECT snmp_credentials_json::text FROM devices WHERE id=$1", device.ID).Scan(&secretBefore); err != nil {
		t.Fatal(err)
	}
	// A stale PATCH view must never write a later probe's status or unrelated edits.
	patch, err := repo.GetDeviceForUpdate(device.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(patch.Interfaces) != 0 || patch.SNMPCredentials.V2c != nil {
		t.Fatal("PATCH projection loaded interfaces or secrets")
	}
	if _, err := db.Exec("UPDATE devices SET status='up', vendor='concurrent' WHERE id=$1", device.ID); err != nil {
		t.Fatal(err)
	}
	patch.Hostname = "after"
	if err := repo.UpdateConfiguration(patch, domain.DeviceConfigurationFields{Hostname: true}); err != nil {
		t.Fatal(err)
	}
	after, err := repo.GetByID(device.ID)
	if err != nil {
		t.Fatal(err)
	}
	var secretAfter string
	if err := db.QueryRow("SELECT snmp_credentials_json::text FROM devices WHERE id=$1", device.ID).Scan(&secretAfter); err != nil {
		t.Fatal(err)
	}
	if after.Hostname != "after" || after.Status != domain.DeviceStatusUp || after.Vendor != "concurrent" || secretBefore != secretAfter {
		t.Fatalf("overwrote unrelated fields: %+v", after)
	}
	if after.Interfaces[0].ID != before.Interfaces[0].ID || after.Addresses[0].ID != before.Addresses[0].ID {
		t.Fatal("replaced unchanged relationships")
	}
	patch.Notes = nil
	patch.PollIntervalOverride = nil
	if err := repo.UpdateConfiguration(patch, domain.DeviceConfigurationFields{Notes: true, PollIntervalOverride: true}); err != nil {
		t.Fatal(err)
	}
	if err := repo.UpdateConfiguration(&domain.Device{ID: uuid.New()}, domain.DeviceConfigurationFields{Hostname: true}); err == nil {
		t.Fatal("missing device update succeeded")
	}
}

func TestDeviceConfigurationNormalizationUsesCurrentVirtualAddress(t *testing.T) {
	db := setupTestDB(t)
	repo := NewDeviceRepo(db, testKeyring, nil)
	device := &domain.Device{ID: uuid.New(), DeviceType: domain.DeviceTypeVirtual}
	if err := repo.Create(device); err != nil {
		t.Fatal(err)
	}
	// The PATCH started against a legacy placeholder while another request
	// assigned an IP and the probe recorded a live status.
	patch, err := repo.GetDeviceForUpdate(device.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec("UPDATE devices SET ip='192.0.2.4',status='up' WHERE id=$1", device.ID); err != nil {
		t.Fatal(err)
	}
	patch.Status = domain.DeviceStatusUnknown
	patch.Hostname = "renamed"
	if err := repo.UpdateConfiguration(patch, domain.DeviceConfigurationFields{Hostname: true, Status: true}); err != nil {
		t.Fatal(err)
	}
	var status string
	if err := db.QueryRow("SELECT status FROM devices WHERE id=$1", device.ID).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != "up" {
		t.Fatalf("stale normalization overwrote live status: %s", status)
	}
}
