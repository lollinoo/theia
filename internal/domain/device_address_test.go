package domain

import (
	"reflect"
	"testing"

	"github.com/google/uuid"
)

func TestNormalizeDeviceAddressesSynthesizesPrimaryFromLegacyIP(t *testing.T) {
	deviceID := uuid.New()
	device := &Device{
		ID:       deviceID,
		IP:       " 192.0.2.10 ",
		Hostname: "router-1",
	}

	NormalizeDeviceAddresses(device)

	if device.IP != "192.0.2.10" {
		t.Fatalf("Device.IP = %q, want trimmed primary", device.IP)
	}
	if len(device.Addresses) != 1 {
		t.Fatalf("addresses len = %d, want 1", len(device.Addresses))
	}
	address := device.Addresses[0]
	if address.DeviceID != deviceID {
		t.Fatalf("address DeviceID = %s, want %s", address.DeviceID, deviceID)
	}
	if address.Address != "192.0.2.10" {
		t.Fatalf("address = %q, want legacy IP", address.Address)
	}
	if address.Role != DeviceAddressRolePrimary {
		t.Fatalf("role = %q, want primary", address.Role)
	}
	if !address.IsPrimary {
		t.Fatal("expected synthesized address to be primary")
	}
	if address.Priority != 0 {
		t.Fatalf("priority = %d, want 0", address.Priority)
	}
}

func TestNormalizeDeviceAddressesTrimsRolesAndDeduplicates(t *testing.T) {
	device := &Device{
		ID: uuid.New(),
		IP: "192.0.2.10",
		Addresses: []DeviceAddress{
			{Address: " 192.0.2.10 ", Label: " mgmt ", Role: "management", IsPrimary: true, Priority: 7},
			{Address: "192.0.2.10", Label: "duplicate", Role: "backup", Priority: 3},
			{Address: " BACKUP.EXAMPLE.TEST ", Label: " Backup Link ", Role: "backup", Priority: 4},
			{Address: "backup.example.test", Label: "duplicate backup", Role: "bogus", Priority: 9},
			{Address: "   ", Role: "monitoring"},
		},
	}

	NormalizeDeviceAddresses(device)

	if len(device.Addresses) != 2 {
		t.Fatalf("addresses len = %d, want 2: %#v", len(device.Addresses), device.Addresses)
	}
	if device.Addresses[0].Address != "192.0.2.10" {
		t.Fatalf("primary address = %q, want 192.0.2.10", device.Addresses[0].Address)
	}
	if device.Addresses[0].Label != "mgmt" {
		t.Fatalf("primary label = %q, want trimmed label", device.Addresses[0].Label)
	}
	if device.Addresses[0].Role != DeviceAddressRolePrimary {
		t.Fatalf("primary role = %q, want primary", device.Addresses[0].Role)
	}
	if !device.Addresses[0].IsPrimary || device.Addresses[0].Priority != 0 {
		t.Fatalf("primary flags = (%v,%d), want (true,0)", device.Addresses[0].IsPrimary, device.Addresses[0].Priority)
	}
	if device.Addresses[1].Address != "BACKUP.EXAMPLE.TEST" {
		t.Fatalf("backup address = %q, want trimmed original casing", device.Addresses[1].Address)
	}
	if device.Addresses[1].Role != DeviceAddressRoleBackup {
		t.Fatalf("backup role = %q, want backup", device.Addresses[1].Role)
	}
}

func TestNormalizeDeviceAddressesDerivesLegacyIPFromPrimaryOrFirst(t *testing.T) {
	t.Run("explicit primary", func(t *testing.T) {
		device := &Device{
			ID: uuid.New(),
			Addresses: []DeviceAddress{
				{Address: "198.51.100.10", Role: DeviceAddressRoleBackup},
				{Address: "192.0.2.10", Role: DeviceAddressRoleManagement, IsPrimary: true},
			},
		}

		NormalizeDeviceAddresses(device)

		if device.IP != "192.0.2.10" {
			t.Fatalf("Device.IP = %q, want explicit primary", device.IP)
		}
		if got := PrimaryAddress(*device); got != "192.0.2.10" {
			t.Fatalf("PrimaryAddress = %q, want explicit primary", got)
		}
	})

	t.Run("first address", func(t *testing.T) {
		device := &Device{
			ID: uuid.New(),
			Addresses: []DeviceAddress{
				{Address: "198.51.100.10", Role: DeviceAddressRoleBackup},
				{Address: "192.0.2.10", Role: DeviceAddressRoleManagement},
			},
		}

		NormalizeDeviceAddresses(device)

		if device.IP != "198.51.100.10" {
			t.Fatalf("Device.IP = %q, want first address", device.IP)
		}
		if !device.Addresses[0].IsPrimary || device.Addresses[0].Role != DeviceAddressRolePrimary {
			t.Fatalf("first address not normalized as primary: %#v", device.Addresses[0])
		}
	})
}

func TestNormalizeDeviceAddressesLegacyIPWinsWhenBothProvided(t *testing.T) {
	device := &Device{
		ID: uuid.New(),
		IP: "192.0.2.10",
		Addresses: []DeviceAddress{
			{Address: "198.51.100.10", Role: DeviceAddressRoleBackup, IsPrimary: true},
			{Address: "203.0.113.10", Role: DeviceAddressRoleManagement, IsPrimary: true},
		},
	}

	NormalizeDeviceAddresses(device)

	if got := PrimaryAddress(*device); got != "192.0.2.10" {
		t.Fatalf("PrimaryAddress = %q, want legacy IP", got)
	}
	if device.Addresses[0].Address != "192.0.2.10" {
		t.Fatalf("first normalized address = %q, want legacy IP primary", device.Addresses[0].Address)
	}
	if len(device.Addresses) != 3 {
		t.Fatalf("addresses len = %d, want legacy primary plus two secondary addresses", len(device.Addresses))
	}
}

func TestAddressForRoleAndBackupAddressFallbacks(t *testing.T) {
	device := Device{
		IP: "192.0.2.10",
		Addresses: []DeviceAddress{
			{Address: "192.0.2.10", Role: DeviceAddressRolePrimary, IsPrimary: true},
			{Address: "192.0.2.11", Role: DeviceAddressRoleManagement},
			{Address: "198.51.100.10", Role: DeviceAddressRoleBackup},
		},
	}

	if got := AddressForRole(device, DeviceAddressRoleBackup); got != "198.51.100.10" {
		t.Fatalf("AddressForRole(backup) = %q, want backup address", got)
	}
	if got := BackupAddress(device); got != "198.51.100.10" {
		t.Fatalf("BackupAddress = %q, want backup address", got)
	}

	device.Addresses = device.Addresses[:2]
	if got := BackupAddress(device); got != "192.0.2.11" {
		t.Fatalf("BackupAddress without backup = %q, want management fallback", got)
	}

	device.Addresses = device.Addresses[:1]
	if got := BackupAddress(device); got != "192.0.2.10" {
		t.Fatalf("BackupAddress without role addresses = %q, want primary fallback", got)
	}
}

func TestDeviceAddressValuesAndEquality(t *testing.T) {
	device := Device{
		IP: "192.0.2.10",
		Addresses: []DeviceAddress{
			{Address: "192.0.2.10", Role: DeviceAddressRolePrimary, IsPrimary: true},
			{Address: "198.51.100.10", Role: DeviceAddressRoleBackup, Label: "backup", Priority: 20},
		},
	}

	if got, want := DeviceAddressValues(device), []string{"192.0.2.10", "198.51.100.10"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("DeviceAddressValues = %#v, want %#v", got, want)
	}

	first := []DeviceAddress{
		{Address: " 192.0.2.10 ", Role: DeviceAddressRolePrimary, IsPrimary: true},
		{Address: "backup.example.test", Role: DeviceAddressRoleBackup, Label: "Backup", Priority: 10},
	}
	second := []DeviceAddress{
		{Address: "192.0.2.10", Role: DeviceAddressRolePrimary, IsPrimary: true},
		{Address: "BACKUP.EXAMPLE.TEST", Role: DeviceAddressRoleBackup, Label: "Backup", Priority: 10},
	}
	if !DeviceAddressesEqual(first, second) {
		t.Fatalf("DeviceAddressesEqual returned false for normalized-equivalent addresses")
	}
}
