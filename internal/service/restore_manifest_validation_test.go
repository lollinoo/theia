package service

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lollinoo/theia/internal/crypto"
)

func TestReadRestoreManifest(t *testing.T) {
	t.Run("missing manifest", func(t *testing.T) {
		_, err := readRestoreManifest(t.TempDir())
		if err == nil {
			t.Fatal("readRestoreManifest() error = nil, want missing manifest error")
		}
		if got := err.Error(); got != "archive missing manifest.json" {
			t.Fatalf("readRestoreManifest() error = %q, want archive missing manifest.json", got)
		}
	})

	t.Run("malformed manifest", func(t *testing.T) {
		tempDir := t.TempDir()
		writeRestoreManifestFile(t, tempDir, "{")

		_, err := readRestoreManifest(tempDir)
		if err == nil {
			t.Fatal("readRestoreManifest() error = nil, want parse error")
		}
		if !strings.Contains(err.Error(), "parsing manifest.json") {
			t.Fatalf("readRestoreManifest() error = %q, want parsing manifest.json", err.Error())
		}
	})

	t.Run("valid manifest", func(t *testing.T) {
		tempDir := t.TempDir()
		writeRestoreManifestFile(t, tempDir, `{"version":1,"db_entry_name":"database.dump","migration_version":7}`)

		manifest, err := readRestoreManifest(tempDir)
		if err != nil {
			t.Fatalf("readRestoreManifest() error = %v", err)
		}
		if manifest.Version != 1 || manifest.DBEntryName != postgresArchiveDBEntry || manifest.MigrationVersion != 7 {
			t.Fatalf("readRestoreManifest() = %#v, want parsed manifest", manifest)
		}
	})
}

func TestRestoreManifestDatabaseEntryName(t *testing.T) {
	tests := []struct {
		name    string
		entry   string
		want    string
		wantErr string
	}{
		{name: "defaults postgres dump", want: postgresArchiveDBEntry},
		{name: "accepts postgres dump", entry: postgresArchiveDBEntry, want: postgresArchiveDBEntry},
		{name: "rejects legacy sqlite with guidance", entry: legacySQLiteArchiveDBEntry, wantErr: "legacy SQLite instance backup archives"},
		{name: "rejects unsupported entry", entry: "other.db", wantErr: `unsupported database entry "other.db" in manifest`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := manifestDatabaseEntryName(backupManifest{DBEntryName: tt.entry})
			if tt.wantErr != "" {
				if err == nil {
					t.Fatalf("manifestDatabaseEntryName() error = nil, want %q", tt.wantErr)
				}
				if !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("manifestDatabaseEntryName() error = %q, want %q", err.Error(), tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("manifestDatabaseEntryName() error = %v", err)
			}
			if got != tt.want {
				t.Fatalf("manifestDatabaseEntryName() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestValidateRestoreManifestEncryptionKey(t *testing.T) {
	key := []byte("restore-manifest-key")
	manifest := backupManifest{EncryptionKeyHash: computeEncryptionKeyHash(key)}

	if err := validateRestoreManifestEncryptionKey(manifest, key); err != nil {
		t.Fatalf("validateRestoreManifestEncryptionKey() error = %v", err)
	}

	err := validateRestoreManifestEncryptionKey(manifest, []byte("different-key"))
	if err == nil {
		t.Fatal("validateRestoreManifestEncryptionKey() error = nil, want mismatch")
	}
	if got := err.Error(); got != "encryption key mismatch: backup was created with a different THEIA_ENCRYPTION_KEY" {
		t.Fatalf("validateRestoreManifestEncryptionKey() error = %q, want stable mismatch error", got)
	}

	keyring := mustRestoreTestKeyring(t)
	newManifest := backupManifest{Encryption: &backupManifestEncryption{
		Version:        1,
		ActiveKeyID:    "kid-b",
		RequiredKeyIDs: []string{"kid-a", "kid-b"},
	}}
	if err := validateRestoreManifestEncryptionKey(newManifest, keyring); err != nil {
		t.Fatalf("validateRestoreManifestEncryptionKey() keyring error = %v", err)
	}

	missingKeyManifest := backupManifest{Encryption: &backupManifestEncryption{
		Version:        1,
		ActiveKeyID:    "kid-b",
		RequiredKeyIDs: []string{"kid-a", "kid-missing"},
	}}
	err = validateRestoreManifestEncryptionKey(missingKeyManifest, keyring)
	if err == nil {
		t.Fatal("validateRestoreManifestEncryptionKey() missing key error = nil")
	}
	if got, want := err.Error(), "archive requires encryption key id kid-missing, but it is not configured"; got != want {
		t.Fatalf("validateRestoreManifestEncryptionKey() error = %q, want %q", got, want)
	}
}

func mustRestoreTestKeyring(t *testing.T) *crypto.Keyring {
	t.Helper()
	keyring, err := crypto.NewKeyring("kid-b", map[string]string{
		"kid-a": "old restore secret",
		"kid-b": "new restore secret",
	})
	if err != nil {
		t.Fatalf("NewKeyring failed: %v", err)
	}
	return keyring
}

func TestValidateRestoreManifestMigrationCompatibility(t *testing.T) {
	tests := []struct {
		name           string
		manifest       backupManifest
		currentVersion int
		wantNeeds      bool
		wantErr        string
	}{
		{
			name:           "current version",
			manifest:       backupManifest{MigrationVersion: 7},
			currentVersion: 7,
		},
		{
			name:           "older archive needs migration",
			manifest:       backupManifest{MigrationVersion: 5},
			currentVersion: 7,
			wantNeeds:      true,
		},
		{
			name:           "newer archive fails safely",
			manifest:       backupManifest{MigrationVersion: 9},
			currentVersion: 7,
			wantErr:        "archive has newer migration version (9) than current (7); upgrade Theia first",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := validateRestoreManifestMigrationCompatibility(tt.manifest, tt.currentVersion)
			if tt.wantErr != "" {
				if err == nil {
					t.Fatalf("validateRestoreManifestMigrationCompatibility() error = nil, want %q", tt.wantErr)
				}
				if gotErr := err.Error(); gotErr != tt.wantErr {
					t.Fatalf("validateRestoreManifestMigrationCompatibility() error = %q, want %q", gotErr, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("validateRestoreManifestMigrationCompatibility() error = %v", err)
			}
			if got != tt.wantNeeds {
				t.Fatalf("validateRestoreManifestMigrationCompatibility() = %v, want %v", got, tt.wantNeeds)
			}
		})
	}
}

func writeRestoreManifestFile(t *testing.T, tempDir string, body string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(tempDir, "manifest.json"), []byte(body), 0o600); err != nil {
		t.Fatalf("writing manifest: %v", err)
	}
}
