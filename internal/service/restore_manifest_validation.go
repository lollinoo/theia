package service

// This file defines restore manifest validation backup and restore service behavior, including filesystem safety and cleanup expectations.

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/lollinoo/theia/internal/crypto"
)

// readRestoreManifest loads the archive manifest from the extracted restore directory.
func readRestoreManifest(tempDir string) (backupManifest, error) {
	manifestPath := filepath.Join(tempDir, "manifest.json")
	manifestData, err := os.ReadFile(manifestPath)
	if err != nil {
		return backupManifest{}, fmt.Errorf("archive missing manifest.json")
	}

	var manifest backupManifest
	if err := json.Unmarshal(manifestData, &manifest); err != nil {
		return backupManifest{}, fmt.Errorf("parsing manifest.json: %w", err)
	}
	return manifest, nil
}

// manifestDatabaseEntryName selects the PostgreSQL dump entry and rejects legacy SQLite entries.
func manifestDatabaseEntryName(manifest backupManifest) (string, error) {
	if entry := strings.TrimSpace(manifest.DBEntryName); entry != "" {
		if entry == postgresArchiveDBEntry {
			return entry, nil
		}
		if entry == legacySQLiteArchiveDBEntry {
			return "", legacySQLiteRestoreArchiveError()
		}
		return "", fmt.Errorf("unsupported database entry %q in manifest", entry)
	}
	return postgresArchiveDBEntry, nil
}

// validateRestoreManifestEncryptionKey ensures the configured keyring can decrypt the archive.
func validateRestoreManifestEncryptionKey(manifest backupManifest, encryptionKey any) error {
	if manifest.Encryption != nil {
		keyring, ok := encryptionKey.(*crypto.Keyring)
		if !ok || keyring == nil {
			return fmt.Errorf("archive requires encryption key metadata, but no encryption keyring is configured")
		}
		for _, keyID := range manifest.Encryption.RequiredKeyIDs {
			if !keyring.HasKey(keyID) {
				return fmt.Errorf("archive requires encryption key id %s, but it is not configured", keyID)
			}
		}
		return nil
	}

	currentKeyHash, ok := encryptionKeyHashForConfiguredKey(encryptionKey, manifest.EncryptionKeyHash)
	if !ok {
		return fmt.Errorf("encryption key mismatch: backup was created with a different THEIA_ENCRYPTION_KEY")
	}
	if manifest.EncryptionKeyHash != currentKeyHash {
		return fmt.Errorf("encryption key mismatch: backup was created with a different THEIA_ENCRYPTION_KEY")
	}
	return nil
}

func encryptionKeyHashForConfiguredKey(encryptionKey any, requiredHash string) (string, bool) {
	switch key := encryptionKey.(type) {
	case []byte:
		return computeEncryptionKeyHash(key), true
	case *crypto.Keyring:
		for _, hash := range key.LegacyKeyHashes() {
			if hash == requiredHash {
				return hash, true
			}
		}
		return "", false
	default:
		return "", false
	}
}

// validateRestoreManifestMigrationCompatibility prevents restoring archives from newer schemas.
func validateRestoreManifestMigrationCompatibility(manifest backupManifest, currentVersion int) (bool, error) {
	if manifest.MigrationVersion > currentVersion {
		return false, fmt.Errorf("archive has newer migration version (%d) than current (%d); upgrade Theia first",
			manifest.MigrationVersion, currentVersion)
	}
	return manifest.MigrationVersion < currentVersion, nil
}
