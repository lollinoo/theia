package sqlite

import (
	"testing"

	"github.com/google/uuid"
	"github.com/lollinoo/theia/internal/domain"
)

func TestDeviceRepoGetBySysName_NormalizedLookup(t *testing.T) {
	db := newTestDB(t)
	repo := NewDeviceRepo(db, testKey, nil)

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

func TestDeviceRepoGetBySysName_EmptyOrUnknownLookup(t *testing.T) {
	db := newTestDB(t)
	repo := NewDeviceRepo(db, testKey, nil)

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
	repo := NewDeviceRepo(db, testKey, nil)

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
	repo := NewDeviceRepo(db, testKey, nil)

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
	repo := NewDeviceRepo(db, testKey, nil)

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

// ---------------------------------------------------------------------------
// TestDeviceRepo_PollClassEmptyDefaultsToStandard (Phase 39 Plan 03)
// ---------------------------------------------------------------------------
// Verifies that an empty PollClass is normalized to PollClassStandard by createOnce.
func TestDeviceRepo_PollClassEmptyDefaultsToStandard(t *testing.T) {
	db := newTestDB(t)
	repo := NewDeviceRepo(db, testKey, nil)

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
	repo := NewDeviceRepo(db, testKey, nil)

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
