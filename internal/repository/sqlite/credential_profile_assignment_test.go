package sqlite

import (
	"database/sql"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/lollinoo/theia/internal/domain"
	_ "github.com/mattn/go-sqlite3"
)

// setupCredentialProfileAssignmentTest creates an in-memory SQLite DB, runs migrations,
// and returns a CredentialProfileRepo ready for assignment/WinBox tests.
func setupCredentialProfileAssignmentTest(t *testing.T) (*CredentialProfileRepo, *sql.DB) {
	t.Helper()

	db, err := sql.Open("sqlite3", ":memory:?_foreign_keys=on")
	if err != nil {
		t.Fatalf("opening test db: %v", err)
	}
	if err := RunMigrations(db); err != nil {
		t.Fatalf("running migrations: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	return NewCredentialProfileRepo(db), db
}

// insertTestDevice inserts a device row directly for FK satisfaction.
func insertTestDevice(t *testing.T, db *sql.DB) uuid.UUID {
	t.Helper()
	deviceID := uuid.New()
	_, err := db.Exec(
		`INSERT INTO devices (id, hostname, ip, snmp_credentials_json, device_type, status, managed, tags_json, metrics_source, prometheus_label_name, prometheus_label_value, vendor, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, datetime('now'), datetime('now'))`,
		deviceID.String(), "test-device", "10.0.0.1",
		`{"version":"2c","v2c":{"community":"public"}}`,
		"router", "up", 1, "{}", "prometheus", "instance", "10.0.0.1", "default",
	)
	if err != nil {
		t.Fatalf("inserting test device: %v", err)
	}
	return deviceID
}

// insertTestProfile creates a credential profile and returns its ID.
func insertTestProfile(t *testing.T, repo *CredentialProfileRepo, name string) uuid.UUID {
	t.Helper()
	profile := &domain.CredentialProfile{
		Name:       name,
		Username:   "admin",
		Port:       22,
		AuthMethod: domain.SSHAuthPassword,
		Role:       "Admin",
	}
	if err := repo.Create(profile); err != nil {
		t.Fatalf("creating profile %q: %v", name, err)
	}
	return profile.ID
}

// --- Test 1: ListAssignedProfiles returns empty slice for device with no assignments ---

func TestCredentialProfileListAssignedProfiles_Empty(t *testing.T) {
	repo, db := setupCredentialProfileAssignmentTest(t)
	deviceID := insertTestDevice(t, db)

	rows, err := repo.ListAssignedProfiles(deviceID)
	if err != nil {
		t.Fatalf("ListAssignedProfiles: %v", err)
	}
	if len(rows) != 0 {
		t.Fatalf("expected 0 assigned profiles, got %d", len(rows))
	}
}

// --- Test 2: AssignProfile inserts row; ListAssignedProfiles returns it with is_winbox=false ---

func TestCredentialProfileAssignProfile_HappyPath(t *testing.T) {
	repo, db := setupCredentialProfileAssignmentTest(t)
	deviceID := insertTestDevice(t, db)
	profileID := insertTestProfile(t, repo, "assign-test")

	if err := repo.AssignProfile(deviceID, profileID); err != nil {
		t.Fatalf("AssignProfile: %v", err)
	}

	rows, err := repo.ListAssignedProfiles(deviceID)
	if err != nil {
		t.Fatalf("ListAssignedProfiles after assign: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected 1 assigned profile, got %d", len(rows))
	}
	if rows[0].ProfileID != profileID {
		t.Errorf("expected profile ID %s, got %s", profileID, rows[0].ProfileID)
	}
	if rows[0].IsWinbox {
		t.Error("expected is_winbox=false after initial assign, got true")
	}
	if rows[0].CreatedAt.IsZero() {
		t.Error("expected non-zero created_at")
	}
}

// --- Test 3: AssignProfile returns error for duplicate (device_id, profile_id) ---

func TestCredentialProfileAssignProfile_Duplicate(t *testing.T) {
	repo, db := setupCredentialProfileAssignmentTest(t)
	deviceID := insertTestDevice(t, db)
	profileID := insertTestProfile(t, repo, "dup-assign-test")

	if err := repo.AssignProfile(deviceID, profileID); err != nil {
		t.Fatalf("first AssignProfile: %v", err)
	}
	err := repo.AssignProfile(deviceID, profileID)
	if err == nil {
		t.Fatal("expected error on duplicate assign, got nil")
	}
}

// --- Test 4: UnassignProfile removes the row; returns error if not assigned ---

func TestCredentialProfileUnassignProfile_HappyPath(t *testing.T) {
	repo, db := setupCredentialProfileAssignmentTest(t)
	deviceID := insertTestDevice(t, db)
	profileID := insertTestProfile(t, repo, "unassign-test")

	if err := repo.AssignProfile(deviceID, profileID); err != nil {
		t.Fatalf("AssignProfile: %v", err)
	}
	if err := repo.UnassignProfile(deviceID, profileID); err != nil {
		t.Fatalf("UnassignProfile: %v", err)
	}

	rows, err := repo.ListAssignedProfiles(deviceID)
	if err != nil {
		t.Fatalf("ListAssignedProfiles after unassign: %v", err)
	}
	if len(rows) != 0 {
		t.Fatalf("expected 0 profiles after unassign, got %d", len(rows))
	}
}

func TestCredentialProfileUnassignProfile_NotAssigned(t *testing.T) {
	repo, db := setupCredentialProfileAssignmentTest(t)
	deviceID := insertTestDevice(t, db)
	profileID := insertTestProfile(t, repo, "unassign-notfound-test")

	err := repo.UnassignProfile(deviceID, profileID)
	if err == nil {
		t.Fatal("expected error when unassigning profile not assigned, got nil")
	}
}

// --- Test 5: SetWinboxProfile sets is_winbox=1 for target, 0 for others; returns error if profile not assigned ---

func TestCredentialProfileSetWinboxProfile_HappyPath(t *testing.T) {
	repo, db := setupCredentialProfileAssignmentTest(t)
	deviceID := insertTestDevice(t, db)
	profileID1 := insertTestProfile(t, repo, "winbox-profile-1")
	profileID2 := insertTestProfile(t, repo, "winbox-profile-2")

	if err := repo.AssignProfile(deviceID, profileID1); err != nil {
		t.Fatalf("AssignProfile 1: %v", err)
	}
	if err := repo.AssignProfile(deviceID, profileID2); err != nil {
		t.Fatalf("AssignProfile 2: %v", err)
	}

	// Set profile1 as winbox
	if err := repo.SetWinboxProfile(deviceID, profileID1); err != nil {
		t.Fatalf("SetWinboxProfile: %v", err)
	}

	rows, err := repo.ListAssignedProfiles(deviceID)
	if err != nil {
		t.Fatalf("ListAssignedProfiles: %v", err)
	}

	for _, row := range rows {
		if row.ProfileID == profileID1 && !row.IsWinbox {
			t.Errorf("expected profile1 is_winbox=true, got false")
		}
		if row.ProfileID == profileID2 && row.IsWinbox {
			t.Errorf("expected profile2 is_winbox=false, got true")
		}
	}

	// Switch winbox to profile2 — profile1 should become false
	if err := repo.SetWinboxProfile(deviceID, profileID2); err != nil {
		t.Fatalf("SetWinboxProfile switch: %v", err)
	}

	rows, err = repo.ListAssignedProfiles(deviceID)
	if err != nil {
		t.Fatalf("ListAssignedProfiles after switch: %v", err)
	}
	for _, row := range rows {
		if row.ProfileID == profileID1 && row.IsWinbox {
			t.Errorf("expected profile1 is_winbox=false after switch, got true")
		}
		if row.ProfileID == profileID2 && !row.IsWinbox {
			t.Errorf("expected profile2 is_winbox=true after switch, got false")
		}
	}
}

func TestCredentialProfileSetWinboxProfile_NotAssigned(t *testing.T) {
	repo, db := setupCredentialProfileAssignmentTest(t)
	deviceID := insertTestDevice(t, db)
	profileID := insertTestProfile(t, repo, "winbox-notassigned-test")

	// Profile not assigned to device — should return error
	err := repo.SetWinboxProfile(deviceID, profileID)
	if err == nil {
		t.Fatal("expected error when setting winbox for unassigned profile, got nil")
	}
}

// --- Test 6: ClearWinboxProfile sets all is_winbox=0 for the device (idempotent) ---

func TestCredentialProfileClearWinboxProfile_HappyPath(t *testing.T) {
	repo, db := setupCredentialProfileAssignmentTest(t)
	deviceID := insertTestDevice(t, db)
	profileID := insertTestProfile(t, repo, "clear-winbox-test")

	if err := repo.AssignProfile(deviceID, profileID); err != nil {
		t.Fatalf("AssignProfile: %v", err)
	}
	if err := repo.SetWinboxProfile(deviceID, profileID); err != nil {
		t.Fatalf("SetWinboxProfile: %v", err)
	}
	if err := repo.ClearWinboxProfile(deviceID); err != nil {
		t.Fatalf("ClearWinboxProfile: %v", err)
	}

	rows, err := repo.ListAssignedProfiles(deviceID)
	if err != nil {
		t.Fatalf("ListAssignedProfiles: %v", err)
	}
	for _, row := range rows {
		if row.IsWinbox {
			t.Errorf("expected all is_winbox=false after clear, but profile %s is still true", row.ProfileID)
		}
	}
}

func TestCredentialProfileClearWinboxProfile_Idempotent(t *testing.T) {
	repo, db := setupCredentialProfileAssignmentTest(t)
	deviceID := insertTestDevice(t, db)

	// Calling ClearWinboxProfile on device with no assignments should not error
	if err := repo.ClearWinboxProfile(deviceID); err != nil {
		t.Fatalf("ClearWinboxProfile on empty device: %v", err)
	}
}

// --- Test 7: GetWinboxAssignment returns profile data + encrypted_secret for winbox profile; error if none ---

func TestCredentialProfileGetWinboxAssignment_HappyPath(t *testing.T) {
	repo, db := setupCredentialProfileAssignmentTest(t)
	deviceID := insertTestDevice(t, db)
	profileID := insertTestProfile(t, repo, "get-winbox-test")

	if err := repo.AssignProfile(deviceID, profileID); err != nil {
		t.Fatalf("AssignProfile: %v", err)
	}
	if err := repo.SetWinboxProfile(deviceID, profileID); err != nil {
		t.Fatalf("SetWinboxProfile: %v", err)
	}

	assignment, err := repo.GetWinboxAssignment(deviceID)
	if err != nil {
		t.Fatalf("GetWinboxAssignment: %v", err)
	}
	if assignment == nil {
		t.Fatal("expected non-nil WinboxAssignmentRow")
	}
	if assignment.ProfileID != profileID {
		t.Errorf("expected profile ID %s, got %s", profileID, assignment.ProfileID)
	}
	// Username comes from the profile we created ("admin")
	if assignment.Username != "admin" {
		t.Errorf("expected username 'admin', got %q", assignment.Username)
	}
	// EncryptedSecret starts empty (we didn't set one in insertTestProfile)
	_ = assignment.EncryptedSecret
}

func TestCredentialProfileGetWinboxAssignment_NoneDesignated(t *testing.T) {
	repo, db := setupCredentialProfileAssignmentTest(t)
	deviceID := insertTestDevice(t, db)

	_, err := repo.GetWinboxAssignment(deviceID)
	if err == nil {
		t.Fatal("expected error when no winbox profile designated, got nil")
	}
}

// --- Test 8: IsInUse returns true when profile exists in device_credential_profiles; false when not ---

func TestCredentialProfileIsInUse_True(t *testing.T) {
	repo, db := setupCredentialProfileAssignmentTest(t)
	deviceID := insertTestDevice(t, db)
	profileID := insertTestProfile(t, repo, "in-use-check")

	if err := repo.AssignProfile(deviceID, profileID); err != nil {
		t.Fatalf("AssignProfile: %v", err)
	}

	inUse, err := repo.IsInUse(profileID)
	if err != nil {
		t.Fatalf("IsInUse: %v", err)
	}
	if !inUse {
		t.Error("expected IsInUse=true, got false")
	}
}

func TestCredentialProfileIsInUse_False(t *testing.T) {
	repo, _ := setupCredentialProfileAssignmentTest(t)
	profileID := uuid.New() // not assigned anywhere

	inUse, err := repo.IsInUse(profileID)
	if err != nil {
		t.Fatalf("IsInUse: %v", err)
	}
	if inUse {
		t.Error("expected IsInUse=false for unassigned profile, got true")
	}
}

// --- CRED-02: Multiple profiles per device ---

// TestCredentialProfileAssignProfile_MultipleProfiles assigns 2 different profiles to
// the same device and verifies that ListAssignedProfiles returns both entries.
func TestCredentialProfileAssignProfile_MultipleProfiles(t *testing.T) {
	repo, db := setupCredentialProfileAssignmentTest(t)
	deviceID := insertTestDevice(t, db)
	profileID1 := insertTestProfile(t, repo, "multi-profile-1")
	profileID2 := insertTestProfile(t, repo, "multi-profile-2")

	if err := repo.AssignProfile(deviceID, profileID1); err != nil {
		t.Fatalf("AssignProfile 1: %v", err)
	}
	if err := repo.AssignProfile(deviceID, profileID2); err != nil {
		t.Fatalf("AssignProfile 2: %v", err)
	}

	rows, err := repo.ListAssignedProfiles(deviceID)
	if err != nil {
		t.Fatalf("ListAssignedProfiles: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("expected 2 assigned profiles, got %d", len(rows))
	}

	// Verify both profile IDs are present
	seen := make(map[string]bool)
	for _, row := range rows {
		seen[row.ProfileID.String()] = true
	}
	if !seen[profileID1.String()] {
		t.Errorf("expected profileID1 (%s) in results, not found", profileID1)
	}
	if !seen[profileID2.String()] {
		t.Errorf("expected profileID2 (%s) in results, not found", profileID2)
	}
}

// --- Timestamp test: verify created_at is a valid recent time ---

func TestCredentialProfileAssignProfile_CreatedAtIsSet(t *testing.T) {
	repo, db := setupCredentialProfileAssignmentTest(t)
	deviceID := insertTestDevice(t, db)
	profileID := insertTestProfile(t, repo, "created-at-test")

	before := time.Now().UTC().Add(-time.Second)
	if err := repo.AssignProfile(deviceID, profileID); err != nil {
		t.Fatalf("AssignProfile: %v", err)
	}
	after := time.Now().UTC().Add(time.Second)

	rows, err := repo.ListAssignedProfiles(deviceID)
	if err != nil {
		t.Fatalf("ListAssignedProfiles: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(rows))
	}
	if rows[0].CreatedAt.Before(before) || rows[0].CreatedAt.After(after) {
		t.Errorf("created_at %v is outside expected range [%v, %v]", rows[0].CreatedAt, before, after)
	}
}
