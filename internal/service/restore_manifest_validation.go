package service

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
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

// validateRestoreManifestEncryptionKey ensures the archive was created with the current key.
func validateRestoreManifestEncryptionKey(manifest backupManifest, encryptionKey []byte) error {
	currentKeyHash := computeEncryptionKeyHash(encryptionKey)
	if manifest.EncryptionKeyHash != currentKeyHash {
		return fmt.Errorf("encryption key mismatch: backup was created with a different THEIA_ENCRYPTION_KEY")
	}
	return nil
}

// validateRestoreManifestMigrationCompatibility prevents restoring archives from newer schemas.
func validateRestoreManifestMigrationCompatibility(manifest backupManifest, currentVersion int) (bool, error) {
	if manifest.MigrationVersion > currentVersion {
		return false, fmt.Errorf("archive has newer migration version (%d) than current (%d); upgrade Theia first",
			manifest.MigrationVersion, currentVersion)
	}
	return manifest.MigrationVersion < currentVersion, nil
}
