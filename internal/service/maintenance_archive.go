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
)

// writeDatabaseArchive does not depend on the source backup/credential tables.
// This lets the first legacy transition protect an older schema before migrating
// it. Verification still restores and migrates a real isolated PostgreSQL cluster.
func (m *Maintenance) writeDatabaseArchive(ctx context.Context, db *sql.DB, state *instance.State, id uuid.UUID, path, recipient string) (*domain.InstanceBackup, error) {
	if err := instance.PrivateDirectory(filepath.Dir(path)); err != nil {
		return nil, err
	}
	work, err := os.MkdirTemp(filepath.Dir(path), ".archive-*")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(work)
	dump := filepath.Join(work, postgresArchiveDBEntry)
	if err := runPostgresDump(ctx, m.DBDSN, dump); err != nil {
		return nil, err
	}
	var exists bool
	if err := db.QueryRowContext(ctx, "SELECT to_regclass('public.schema_migrations') IS NOT NULL").Scan(&exists); err != nil {
		return nil, err
	}
	version := 0
	if exists {
		if err := db.QueryRowContext(ctx, "SELECT version FROM schema_migrations").Scan(&version); err != nil {
			return nil, err
		}
	}
	keys, err := state.Keyring()
	if err != nil {
		return nil, err
	}
	svc := NewInstanceBackupService(db, nil, nil, m.BackupDir, m.DeviceBackupDir, m.KnownHostsPath, m.DataDir, m.DBDSN, keys)
	svc.SetManagedState(m.StatePath)
	info, err := os.Stat(dump)
	if err != nil {
		return nil, err
	}
	files, count, known, total, entries, err := svc.collectArchiveSourceFiles(ctx, svc.BackupArchiveLimits(), info.Size())
	if err != nil {
		return nil, err
	}
	hash, err := computeFileHashContext(ctx, dump)
	if err != nil {
		return nil, err
	}
	private, err := json.Marshal(state)
	if err != nil {
		return nil, err
	}
	artifact := databaseBackupArtifact{tempPath: dump, archiveEntryName: postgresArchiveDBEntry, migrationVersion: version}
	plan, err := buildInstanceBackupArchiveManifestPlan(instanceBackupArchiveManifestInput{privateStateBytes: int64(len(private)), dbArtifact: artifact, backupCreatedAt: time.Now().UTC(), dbSHA256: hash, backupFileCount: count, totalSourceBytes: total, archiveFileEntries: entries, encryptionKeyring: keys, requiredKeyIDs: keys.KeyIDs(), limits: svc.BackupArchiveLimits()})
	if err != nil {
		return nil, err
	}
	_, verification, err := svc.createArchiveWithProtection(ctx, path, artifact, files, known, plan.manifestJSON, &plan.manifest, id, private, recipient)
	if err != nil {
		return nil, err
	}
	if err := svc.verifyProtectedBackup(ctx, path, verification); err != nil {
		return nil, err
	}
	if err := validateRestoreArchiveFile(path, m.restoreLimits()); err != nil {
		return nil, err
	}
	archiveHash, err := computeFileHashContext(ctx, path)
	if err != nil {
		return nil, err
	}
	archiveInfo, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	return &domain.InstanceBackup{ID: id, FileName: filepath.Base(path), FilePath: path, SizeBytes: archiveInfo.Size(), SHA256: archiveHash, MigrationVersion: version, Status: domain.InstanceBackupStatusSuccess, CreatedAt: time.Now().UTC(), Trigger: domain.InstanceBackupTriggerManual}, nil
}

// extractSafetySnapshot authenticates the operation's immutable bytes and uses
// only its separate online identity, never the administrator's recovery file.
func (m *Maintenance) extractSafetySnapshot(ctx context.Context, op *MaintenanceOperation, prefix string) (string, *instance.State, error) {
	dir := m.operationDir(op)
	snapshot := filepath.Join(dir, "snapshot.age")
	hash, err := computeFileHashContext(ctx, snapshot)
	if err != nil {
		return "", nil, err
	}
	if op.SnapshotSHA256 == "" || hash != op.SnapshotSHA256 {
		return "", nil, fmt.Errorf("safety snapshot does not match this operation's verified bytes")
	}
	identities, err := instance.ReadRecoveryFile(filepath.Join(dir, "snapshot.key"))
	if err != nil {
		return "", nil, err
	}
	staging, err := os.MkdirTemp(dir, prefix)
	if err != nil {
		return "", nil, err
	}
	state, err := ExtractProtectedInstanceBackup(ctx, snapshot, staging, m.restoreLimits(), identities...)
	if err != nil {
		os.RemoveAll(staging)
		return "", nil, err
	}
	return staging, state, nil
}
