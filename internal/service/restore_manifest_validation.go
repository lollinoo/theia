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

// RestoreEncryptionKeyError indicates that an instance backup archive requires one or more
// encryption keys that are missing or mismatched in the active runtime keyring.
type RestoreEncryptionKeyError struct {
	MissingKeyIDs []string `json:"missing_key_ids"`
	Message       string   `json:"message"`
}

func (e *RestoreEncryptionKeyError) Error() string {
	if e == nil {
		return ""
	}
	return e.Message
}

// validateRestoreManifestEncryptionKey ensures the configured keyring can decrypt the archive.
func validateRestoreManifestEncryptionKey(manifest backupManifest, encryptionKey any) error {
	if manifest.Encryption != nil {
		keyring, ok := encryptionKey.(*crypto.Keyring)
		if !ok || keyring == nil {
			return &RestoreEncryptionKeyError{
				MissingKeyIDs: append([]string(nil), manifest.Encryption.RequiredKeyIDs...),
				Message:       "archive requires encryption key metadata, but no encryption keyring is configured",
			}
		}
		missing := missingRestoreManifestKeyIDs(manifest, encryptionKey)
		for _, keyID := range manifest.Encryption.RequiredKeyIDs {
			if !keyring.HasKey(keyID) {
				return &RestoreEncryptionKeyError{
					MissingKeyIDs: missing,
					Message:       fmt.Sprintf("archive requires encryption key id %s, but it is not configured", keyID),
				}
			}
			if expectedHash, ok := manifest.Encryption.KeyHashes[keyID]; ok && expectedHash != "" {
				actualHash, hasActual := keyring.KeyHashForID(keyID)
				if !hasActual || actualHash != expectedHash {
					return &RestoreEncryptionKeyError{
						MissingKeyIDs: missing,
						Message:       fmt.Sprintf("encryption key mismatch for key id %s: provided secret does not match backup", keyID),
					}
				}
			}
		}
		return nil
	}

	currentKeyHash, ok := encryptionKeyHashForConfiguredKey(encryptionKey, manifest.EncryptionKeyHash)
	if !ok || manifest.EncryptionKeyHash != currentKeyHash {
		return &RestoreEncryptionKeyError{
			MissingKeyIDs: []string{crypto.LegacyKeyID},
			Message:       "encryption key mismatch: backup was created with a different THEIA_ENCRYPTION_KEY",
		}
	}
	return nil
}

// missingRestoreManifestKeyIDs returns key IDs required by the backup manifest that are missing or mismatched in encryptionKey.
func missingRestoreManifestKeyIDs(manifest backupManifest, encryptionKey any) []string {
	if manifest.Encryption != nil {
		keyring, _ := encryptionKey.(*crypto.Keyring)
		var missing []string
		for _, keyID := range manifest.Encryption.RequiredKeyIDs {
			if keyring == nil || !keyring.HasKey(keyID) {
				missing = append(missing, keyID)
				continue
			}
			if expectedHash, ok := manifest.Encryption.KeyHashes[keyID]; ok && expectedHash != "" {
				if actualHash, hasActual := keyring.KeyHashForID(keyID); !hasActual || actualHash != expectedHash {
					missing = append(missing, keyID)
				}
			}
		}
		return missing
	}
	currentKeyHash, ok := encryptionKeyHashForConfiguredKey(encryptionKey, manifest.EncryptionKeyHash)
	if !ok || manifest.EncryptionKeyHash != currentKeyHash {
		return []string{crypto.LegacyKeyID}
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
