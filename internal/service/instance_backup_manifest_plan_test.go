package service

import (
	"encoding/json"
	"testing"
	"time"
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
		encryptionKey:      []byte("1234567890abcdef"),
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
	if manifest.EncryptionKeyHash != computeEncryptionKeyHash([]byte("1234567890abcdef")) {
		t.Fatalf("manifest.EncryptionKeyHash = %q, want key hash", manifest.EncryptionKeyHash)
	}
}
