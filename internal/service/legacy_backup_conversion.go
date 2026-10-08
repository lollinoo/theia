package service

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/google/uuid"
	"github.com/lollinoo/theia/internal/domain"
	"github.com/lollinoo/theia/internal/instance"
	"github.com/lollinoo/theia/internal/repository/postgres"
)

// ConvertLegacyArchive preserves the original PostgreSQL archive, checks its
// original keys and contents, and creates a new verified age recovery point.
// It does not connect to or modify the live instance database.
func (m *Maintenance) ConvertLegacyArchive(ctx context.Context, input, output string) (*domain.InstanceBackup, error) {
	state, err := (instance.Store{Path: m.StatePath}).Load()
	if err != nil {
		return nil, err
	}
	if state.RecoveryRecipient == "" {
		return nil, fmt.Errorf("verify an operator recovery file before converting archives")
	}
	if _, err := os.Lstat(output); !os.IsNotExist(err) {
		return nil, fmt.Errorf("converted archive destination already exists or cannot be inspected")
	}
	if err := validateRestoreArchiveFile(input, m.restoreLimits()); err != nil {
		return nil, err
	}
	staging, err := os.MkdirTemp("", "theia-legacy-convert-*")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(staging)
	if err := extractArchiveContext(ctx, input, staging, m.restoreLimits()); err != nil {
		return nil, err
	}
	manifest, err := readRestoreManifest(staging)
	if err != nil {
		return nil, err
	}
	if manifest.Version != 1 {
		return nil, fmt.Errorf("legacy conversion requires a version 1 PostgreSQL archive")
	}
	entry, err := manifestDatabaseEntryName(manifest)
	if err != nil {
		return nil, err
	}
	keys, err := state.Keyring()
	if err != nil {
		return nil, err
	}
	if err := validateRestoreManifestEncryptionKey(manifest, keys); err != nil {
		return nil, err
	}
	dump := filepath.Join(staging, entry)
	hash, err := computeFileHashContext(ctx, dump)
	if err != nil {
		return nil, err
	}
	if hash != manifest.DBSHA256 {
		return nil, fmt.Errorf("legacy database checksum mismatch")
	}
	if err := VerifyIsolatedPostgresDump(ctx, dump, keys); err != nil {
		return nil, err
	}
	svc := NewInstanceBackupService(nil, nil, nil, m.BackupDir, filepath.Join(staging, "backups"), filepath.Join(staging, "known_hosts"), staging, "", keys)
	svc.SetManagedState(m.StatePath)
	info, err := os.Stat(dump)
	if err != nil {
		return nil, err
	}
	files, count, known, total, entries, err := svc.collectArchiveSourceFiles(ctx, svc.BackupArchiveLimits(), info.Size())
	if err != nil {
		return nil, err
	}
	if count != manifest.BackupFileCount {
		return nil, fmt.Errorf("legacy archive has an incomplete device backup set")
	}
	private, err := json.Marshal(state)
	if err != nil {
		return nil, err
	}
	artifact := databaseBackupArtifact{tempPath: dump, archiveEntryName: postgresArchiveDBEntry, migrationVersion: manifest.MigrationVersion}
	created := time.Now().UTC()
	plan, err := buildInstanceBackupArchiveManifestPlan(instanceBackupArchiveManifestInput{privateStateBytes: int64(len(private)), dbArtifact: artifact, backupCreatedAt: created, dbSHA256: hash, backupFileCount: count, totalSourceBytes: total, archiveFileEntries: entries, encryptionKeyring: keys, requiredKeyIDs: keys.KeyIDs(), limits: svc.BackupArchiveLimits()})
	if err != nil {
		return nil, err
	}
	backup := &domain.InstanceBackup{ID: uuid.New(), FileName: filepath.Base(output), FilePath: output, MigrationVersion: manifest.MigrationVersion, CreatedAt: created, Trigger: domain.InstanceBackupTriggerManual}
	_, verification, err := svc.createArchiveWithProtection(ctx, output, artifact, files, known, plan.manifestJSON, &plan.manifest, backup.ID, private)
	if err != nil {
		return nil, err
	}
	if err := svc.verifyProtectedBackup(ctx, output, verification); err != nil {
		return nil, err
	}
	backup.SHA256, err = computeFileHashContext(ctx, output)
	if err != nil {
		return nil, err
	}
	info, err = os.Stat(output)
	if err != nil {
		return nil, err
	}
	backup.SizeBytes = info.Size()
	if err := writeBackupVerificationReceipt(backup); err != nil {
		return nil, err
	}
	backup.Status = domain.InstanceBackupStatusSuccess
	return backup, nil
}

func (m *Maintenance) registerLegacyPreventiveBackup(db *sql.DB, op *MaintenanceOperation) error {
	id, err := uuid.Parse(op.PreventiveBackupID)
	if err != nil {
		return err
	}
	path := filepath.Join(m.BackupDir, id.String(), op.LegacyPreventiveFileName)
	data, err := os.ReadFile(path + ".verified.json")
	if err != nil {
		return err
	}
	var receipt backupVerificationReceipt
	if err := json.Unmarshal(data, &receipt); err != nil {
		return err
	}
	return postgres.NewInstanceBackupRepo(db).Create(&domain.InstanceBackup{ID: id, FileName: op.LegacyPreventiveFileName, FilePath: path, SizeBytes: receipt.Size, SHA256: receipt.SHA256, MigrationVersion: receipt.MigrationVersion, Status: domain.InstanceBackupStatusSuccess, CreatedAt: op.CreatedAt, Trigger: domain.InstanceBackupTriggerManual})
}

func (m *Maintenance) createLegacyPreventiveBackup(ctx context.Context, dbState *instance.State, dbArchive func(string, uuid.UUID) (*domain.InstanceBackup, error)) (*domain.InstanceBackup, error) {
	if dbState.RecoveryRecipient == "" {
		return nil, fmt.Errorf("initial legacy migration requires a verified operator recovery recipient")
	}
	id := uuid.New()
	path := filepath.Join(m.BackupDir, id.String(), fmt.Sprintf("theia-transition-%s.tar.gz.age", time.Now().UTC().Format("20060102-150405")))
	backup, err := dbArchive(path, id)
	if err != nil {
		return nil, err
	}
	if err := writeBackupVerificationReceipt(backup); err != nil {
		return nil, err
	}
	if dbState.BackupDestination != nil {
		destination, err := instance.NewS3Destination(*dbState.BackupDestination)
		if err != nil {
			return nil, err
		}
		if err := destination.PutVerified(ctx, id.String()+"/"+backup.FileName, path, backup.SHA256); err != nil {
			return nil, fmt.Errorf("verified local transition archive retained; required external copy is incomplete: %w", err)
		}
	}
	return backup, nil
}
