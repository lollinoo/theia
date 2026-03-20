package sqlite

import (
	"strings"
	"testing"

	"github.com/lollinoo/theia/internal/domain"
)

func TestSNMPProfileRepoEncryptionRoundTrip(t *testing.T) {
	db := setupTestDB(t)
	repo := NewSNMPProfileRepo(db, testKey)

	profile := &domain.SNMPProfile{
		Name:        "test-profile-v3",
		Description: "Test SNMP v3 profile",
		Credentials: domain.SNMPCredentials{
			Version: domain.SNMPVersionV3,
			V3: &domain.SNMPv3Credentials{
				Username:      "admin",
				AuthProtocol:  "SHA",
				AuthPassword:  "profile-auth-secret",
				PrivProtocol:  "AES",
				PrivPassword:  "profile-priv-secret",
				SecurityLevel: "authPriv",
			},
		},
	}

	if err := repo.Create(profile); err != nil {
		t.Fatalf("Create: %v", err)
	}

	// Read back via repo (should decrypt transparently)
	got, err := repo.GetByID(profile.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}

	if got.Credentials.V3 == nil {
		t.Fatal("Expected V3 credentials to be non-nil")
	}
	if got.Credentials.V3.AuthPassword != "profile-auth-secret" {
		t.Errorf("AuthPassword = %q, want %q", got.Credentials.V3.AuthPassword, "profile-auth-secret")
	}
	if got.Credentials.V3.PrivPassword != "profile-priv-secret" {
		t.Errorf("PrivPassword = %q, want %q", got.Credentials.V3.PrivPassword, "profile-priv-secret")
	}
	// Non-sensitive fields should be unchanged
	if got.Credentials.V3.Username != "admin" {
		t.Errorf("Username = %q, want %q", got.Credentials.V3.Username, "admin")
	}

	// Verify raw DB value does NOT contain plaintext
	var rawJSON string
	err = db.QueryRow("SELECT credentials_json FROM snmp_profiles WHERE id = ?", profile.ID.String()).Scan(&rawJSON)
	if err != nil {
		t.Fatalf("querying raw JSON: %v", err)
	}
	if strings.Contains(rawJSON, "profile-auth-secret") {
		t.Error("Raw DB JSON contains plaintext 'profile-auth-secret' -- encryption failed")
	}
	if strings.Contains(rawJSON, "profile-priv-secret") {
		t.Error("Raw DB JSON contains plaintext 'profile-priv-secret' -- encryption failed")
	}
}

func TestSNMPProfileRepoV2cEncryptionRoundTrip(t *testing.T) {
	db := setupTestDB(t)
	repo := NewSNMPProfileRepo(db, testKey)

	profile := &domain.SNMPProfile{
		Name:        "test-profile-v2c",
		Description: "Test SNMP v2c profile",
		Credentials: domain.SNMPCredentials{
			Version: domain.SNMPVersionV2c,
			V2c:     &domain.SNMPv2cCredentials{Community: "profile-community"},
		},
	}

	if err := repo.Create(profile); err != nil {
		t.Fatalf("Create: %v", err)
	}

	got, err := repo.GetByID(profile.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}

	if got.Credentials.V2c == nil {
		t.Fatal("Expected V2c credentials to be non-nil")
	}
	if got.Credentials.V2c.Community != "profile-community" {
		t.Errorf("Community = %q, want %q", got.Credentials.V2c.Community, "profile-community")
	}

	// Verify raw DB value does NOT contain plaintext
	var rawJSON string
	err = db.QueryRow("SELECT credentials_json FROM snmp_profiles WHERE id = ?", profile.ID.String()).Scan(&rawJSON)
	if err != nil {
		t.Fatalf("querying raw JSON: %v", err)
	}
	if strings.Contains(rawJSON, "profile-community") {
		t.Error("Raw DB JSON contains plaintext 'profile-community' -- encryption failed")
	}
}

func TestSNMPProfileRepoGetAll(t *testing.T) {
	db := setupTestDB(t)
	repo := NewSNMPProfileRepo(db, testKey)

	// Create two profiles
	for _, name := range []string{"profile-A", "profile-B"} {
		p := &domain.SNMPProfile{
			Name:        name,
			Description: "Test",
			Credentials: domain.SNMPCredentials{
				Version: domain.SNMPVersionV2c,
				V2c:     &domain.SNMPv2cCredentials{Community: name + "-secret"},
			},
		}
		if err := repo.Create(p); err != nil {
			t.Fatalf("Create %s: %v", name, err)
		}
	}

	profiles, err := repo.GetAll()
	if err != nil {
		t.Fatalf("GetAll: %v", err)
	}
	if len(profiles) != 2 {
		t.Fatalf("GetAll returned %d profiles, want 2", len(profiles))
	}

	// Verify decrypted values (profiles sorted by name ASC)
	for _, p := range profiles {
		expected := p.Name + "-secret"
		if p.Credentials.V2c == nil || p.Credentials.V2c.Community != expected {
			t.Errorf("Profile %s: Community = %q, want %q",
				p.Name,
				p.Credentials.V2c.Community,
				expected)
		}
	}
}
