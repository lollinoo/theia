package service

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/lollinoo/theia/internal/crypto"
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
			RequiredKeyIDs: input.encryptionKeyring.KeyIDs(),
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
