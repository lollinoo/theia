package service

// This file defines instance backup manifest plan backup and restore service behavior, including filesystem safety and cleanup expectations.

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"sort"
	"time"

	"github.com/lollinoo/theia/internal/crypto"
)

// backupManifest describes the contents and metadata of an instance backup archive.
type backupManifest struct {
	Version           int                       `json:"version"`
	AppVersion        string                    `json:"app_version"`
	GitCommit         string                    `json:"git_commit"`
	DBEntryName       string                    `json:"db_entry_name,omitempty"`
	MigrationVersion  int                       `json:"migration_version"`
	CreatedAt         string                    `json:"created_at"`
	DBSHA256          string                    `json:"db_sha256"`
	BackupFileCount   int                       `json:"backup_file_count"`
	TotalSizeBytes    int64                     `json:"total_size_bytes"`
	EncryptionKeyHash string                    `json:"encryption_key_hash"`
	Encryption        *backupManifestEncryption `json:"encryption,omitempty"`
}

type backupManifestEncryption struct {
	Version         int      `json:"version"`
	ActiveKeyID     string   `json:"active_key_id"`
	RequiredKeyIDs  []string `json:"required_key_ids"`
	LegacyKeyHashes []string `json:"legacy_key_hashes,omitempty"`
}

const (
	legacySQLiteArchiveDBEntry = "theia.db"
	postgresArchiveDBEntry     = "database.dump"
)

type instanceBackupArchiveManifestInput struct {
	appVersion         string
	gitCommit          string
	dbArtifact         databaseBackupArtifact
	backupCreatedAt    time.Time
	dbSHA256           string
	backupFileCount    int
	totalSourceBytes   int64
	archiveFileEntries int
	encryptionKey      []byte
	encryptionKeyring  *crypto.Keyring
	requiredKeyIDs     []string
	limits             BackupArchiveLimits
}

type instanceBackupArchiveManifestPlan struct {
	manifest          backupManifest
	manifestJSON      []byte
	totalArchiveBytes int64
}

// buildInstanceBackupArchiveManifestPlan builds manifest JSON and final archive byte totals.
func buildInstanceBackupArchiveManifestPlan(input instanceBackupArchiveManifestInput) (instanceBackupArchiveManifestPlan, error) {
	var plan instanceBackupArchiveManifestPlan
	manifest := backupManifest{
		Version:          1,
		AppVersion:       input.appVersion,
		GitCommit:        input.gitCommit,
		DBEntryName:      input.dbArtifact.archiveEntryName,
		MigrationVersion: input.dbArtifact.migrationVersion,
		CreatedAt:        input.backupCreatedAt.UTC().Format(time.RFC3339),
		DBSHA256:         input.dbSHA256,
		BackupFileCount:  input.backupFileCount,
		TotalSizeBytes:   0, // will be updated after archiving
	}
	if input.encryptionKeyring != nil {
		manifest.Encryption = &backupManifestEncryption{
			Version:        1,
			ActiveKeyID:    input.encryptionKeyring.ActiveKeyID(),
			RequiredKeyIDs: requiredBackupManifestEncryptionKeyIDs(input.encryptionKeyring.ActiveKeyID(), input.requiredKeyIDs),
		}
	} else {
		manifest.EncryptionKeyHash = computeEncryptionKeyHash(input.encryptionKey)
	}

	manifestJSON, err := json.MarshalIndent(&manifest, "", "  ")
	if err != nil {
		return plan, fmt.Errorf("marshaling manifest: %w", err)
	}
	estimatedManifestTotal, err := checkedArchiveByteTotal(
		input.totalSourceBytes,
		int64(len(manifestJSON)),
		input.limits.MaxTotalBytes,
	)
	if err != nil {
		return plan, err
	}
	manifest.TotalSizeBytes = estimatedManifestTotal
	manifestJSON, err = json.MarshalIndent(&manifest, "", "  ")
	if err != nil {
		return plan, fmt.Errorf("marshaling manifest: %w", err)
	}
	totalArchiveBytes, err := checkedArchiveByteTotal(input.totalSourceBytes, int64(len(manifestJSON)), input.limits.MaxTotalBytes)
	if err != nil {
		return plan, err
	}
	manifest.TotalSizeBytes = totalArchiveBytes
	if err := checkBackupArchiveTotals(totalArchiveBytes, input.archiveFileEntries+1, input.limits); err != nil {
		return plan, err
	}

	plan.manifest = manifest
	plan.manifestJSON = manifestJSON
	plan.totalArchiveBytes = totalArchiveBytes
	return plan, nil
}

func requiredBackupManifestEncryptionKeyIDs(activeKeyID string, discovered []string) []string {
	seen := make(map[string]struct{}, len(discovered)+1)
	if activeKeyID != "" {
		seen[activeKeyID] = struct{}{}
	}
	for _, keyID := range discovered {
		if keyID == "" {
			continue
		}
		seen[keyID] = struct{}{}
	}
	ids := make([]string, 0, len(seen))
	for keyID := range seen {
		ids = append(ids, keyID)
	}
	sort.Strings(ids)
	return ids
}

func requiredCredentialKeyIDsFromValues(values []string) ([]string, error) {
	seen := make(map[string]struct{})
	for _, value := range values {
		if value == "" {
			continue
		}
		if crypto.IsEnvelope(value) {
			keyID, err := crypto.EnvelopeKeyID(value)
			if err != nil {
				return nil, err
			}
			seen[keyID] = struct{}{}
			continue
		}
		if looksLikeLegacyCredentialCiphertext(value) {
			seen[crypto.LegacyKeyID] = struct{}{}
		}
	}
	ids := make([]string, 0, len(seen))
	for keyID := range seen {
		ids = append(ids, keyID)
	}
	sort.Strings(ids)
	return ids, nil
}

func looksLikeLegacyCredentialCiphertext(value string) bool {
	raw, err := base64.StdEncoding.DecodeString(value)
	if err != nil {
		return false
	}
	return len(raw) >= 12+16
}
