package service

import (
	"encoding/base64"
	"encoding/json"
	"testing"
	"time"

	"github.com/lollinoo/theia/internal/crypto"
)

func TestBuildInstanceBackupArchiveManifestPlanSetsStableMetadataAndFinalSize(t *testing.T) {
	createdAt := time.Date(2026, 6, 4, 12, 30, 0, 0, time.FixedZone("CEST", 2*60*60))

	plan, err := buildInstanceBackupArchiveManifestPlan(instanceBackupArchiveManifestInput{
		appVersion:         "v-test",
		gitCommit:          "abc123",
		dbArtifact:         databaseBackupArtifact{archiveEntryName: postgresArchiveDBEntry, migrationVersion: 42},
		backupCreatedAt:    createdAt,
		dbSHA256:           "db-hash",
		backupFileCount:    3,
		totalSourceBytes:   20,
		archiveFileEntries: 4,
		encryptionKeyring:  mustManifestTestKeyring(t),
		requiredKeyIDs:     []string{"kid-a", "legacy"},
		limits: BackupArchiveLimits{
			MaxTotalBytes:  1 << 20,
			MaxEntryBytes:  1 << 20,
			MaxFileEntries: 10,
			MaxDuration:    time.Minute,
		},
	})

	if err != nil {
		t.Fatalf("buildInstanceBackupArchiveManifestPlan: %v", err)
	}
	if plan.totalArchiveBytes != plan.manifest.TotalSizeBytes {
		t.Fatalf("totalArchiveBytes = %d, manifest.TotalSizeBytes = %d", plan.totalArchiveBytes, plan.manifest.TotalSizeBytes)
	}
	if plan.totalArchiveBytes != int64(20+len(plan.manifestJSON)) {
		t.Fatalf("totalArchiveBytes = %d, want source bytes plus final manifest JSON %d", plan.totalArchiveBytes, 20+len(plan.manifestJSON))
	}
	var manifest backupManifest
	if err := json.Unmarshal(plan.manifestJSON, &manifest); err != nil {
		t.Fatalf("unmarshal manifestJSON: %v", err)
	}
	if manifest.AppVersion != "v-test" || manifest.GitCommit != "abc123" {
		t.Fatalf("manifest version metadata = %q/%q, want v-test/abc123", manifest.AppVersion, manifest.GitCommit)
	}
	if manifest.DBEntryName != postgresArchiveDBEntry || manifest.MigrationVersion != 42 {
		t.Fatalf("manifest db metadata = %q/%d, want %q/42", manifest.DBEntryName, manifest.MigrationVersion, postgresArchiveDBEntry)
	}
	if manifest.CreatedAt != createdAt.UTC().Format(time.RFC3339) {
		t.Fatalf("manifest.CreatedAt = %q, want UTC RFC3339", manifest.CreatedAt)
	}
	if manifest.DBSHA256 != "db-hash" || manifest.BackupFileCount != 3 {
		t.Fatalf("manifest hash/count = %q/%d, want db-hash/3", manifest.DBSHA256, manifest.BackupFileCount)
	}
	if manifest.Encryption == nil {
		t.Fatal("manifest.Encryption = nil, want encryption metadata")
	}
	if manifest.Encryption.Version != 1 {
		t.Fatalf("manifest.Encryption.Version = %d, want 1", manifest.Encryption.Version)
	}
	if manifest.Encryption.ActiveKeyID != "kid-b" {
		t.Fatalf("manifest.Encryption.ActiveKeyID = %q, want kid-b", manifest.Encryption.ActiveKeyID)
	}
	if got, want := manifest.Encryption.RequiredKeyIDs, []string{"kid-a", "kid-b", "legacy"}; !equalStringSlices(got, want) {
		t.Fatalf("manifest.Encryption.RequiredKeyIDs = %#v, want %#v", got, want)
	}
	if manifest.EncryptionKeyHash != "" {
		t.Fatalf("manifest.EncryptionKeyHash = %q, want empty for keyring manifest", manifest.EncryptionKeyHash)
	}
}

func TestRequiredCredentialKeyIDsFromValuesIncludesActiveOldAndLegacyCiphertext(t *testing.T) {
	keyring := mustManifestTestKeyring(t)
	activeEnvelope, err := keyring.EncryptString("active-secret")
	if err != nil {
		t.Fatalf("EncryptString active failed: %v", err)
	}
	oldKeyring, err := crypto.NewKeyring("kid-a", map[string]string{
		"kid-a": "old manifest secret",
		"kid-b": "new manifest secret",
	})
	if err != nil {
		t.Fatalf("NewKeyring old failed: %v", err)
	}
	oldEnvelope, err := oldKeyring.EncryptString("old-secret")
	if err != nil {
		t.Fatalf("EncryptString old failed: %v", err)
	}
	legacyRaw, err := crypto.Encrypt([]byte("legacy-secret"), crypto.DeriveKey("legacy manifest secret"))
	if err != nil {
		t.Fatalf("legacy Encrypt failed: %v", err)
	}

	got, err := requiredCredentialKeyIDsFromValues([]string{
		activeEnvelope,
		oldEnvelope,
		base64.StdEncoding.EncodeToString(legacyRaw),
		"plain-secret",
		"",
	})
	if err != nil {
		t.Fatalf("requiredCredentialKeyIDsFromValues() error = %v", err)
	}
	if want := []string{"kid-a", "kid-b", "legacy"}; !equalStringSlices(got, want) {
		t.Fatalf("requiredCredentialKeyIDsFromValues() = %#v, want %#v", got, want)
	}
}

func mustManifestTestKeyring(t *testing.T) *crypto.Keyring {
	t.Helper()
	keyring, err := crypto.NewKeyring("kid-b", map[string]string{
		"kid-a": "old manifest secret",
		"kid-b": "new manifest secret",
	})
	if err != nil {
		t.Fatalf("NewKeyring failed: %v", err)
	}
	return keyring
}

func equalStringSlices(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
