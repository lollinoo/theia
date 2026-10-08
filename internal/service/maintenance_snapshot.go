package service

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"filippo.io/age"
	"github.com/google/uuid"
	"github.com/lollinoo/theia/internal/domain"
	"github.com/lollinoo/theia/internal/instance"
	"github.com/lollinoo/theia/internal/repository/postgres"
)

func (m *Maintenance) prepareSafetySnapshot(ctx context.Context, db *sql.DB, state *instance.State, op *MaintenanceOperation, previous *MaintenanceOperation) error {
	var schemaExists bool
	if err := db.QueryRowContext(ctx, "SELECT to_regclass('public.schema_migrations') IS NOT NULL").Scan(&schemaExists); err != nil {
		return fmt.Errorf("preflight database connection failed")
	}
	if !schemaExists {
		var tables int
		if err := db.QueryRowContext(ctx, "SELECT count(*) FROM pg_tables WHERE schemaname = 'public'").Scan(&tables); err != nil {
			return err
		}
		if tables != 0 {
			return fmt.Errorf("database has existing tables without Theia migration metadata; import its compatible original deployment first")
		}
		if op.Action == "migrate" && previous != nil && previous.VerifiedInstanceID != "" && !(previous.Phase == "rolled_back" && previous.SourceWasEmpty) {
			return fmt.Errorf("a previously verified instance database is unexpectedly empty; restore a verified instance backup instead of initializing a replacement schema")
		}
		op.SourceWasEmpty = true
	} else {
		var sourceVersion int
		var dirty bool
		if err := db.QueryRowContext(ctx, "SELECT version,dirty FROM schema_migrations").Scan(&sourceVersion, &dirty); err != nil {
			return err
		}
		if dirty || sourceVersion > postgres.SupportedSchemaVersion() {
			return fmt.Errorf("source migration metadata is dirty or newer than this release")
		}
		if sourceVersion < postgres.SupportedSchemaVersion() {
			backup, err := m.createLegacyPreventiveBackup(ctx, state, func(path string, id uuid.UUID) (*domain.InstanceBackup, error) {
				return m.writeDatabaseArchive(ctx, db, state, id, path, state.RecoveryRecipient)
			})
			if err != nil {
				return err
			}
			op.PreventiveBackupID, op.LegacyPreventiveFileName = backup.ID.String(), backup.FileName
		} else {
			s, err := m.backupService(db, state)
			if err != nil {
				return err
			}
			s.FailStaleRunning()
			backup, err := s.Create(ctx)
			if err != nil {
				return fmt.Errorf("preventive backup is incomplete; maintenance was not started: %w", err)
			}
			op.PreventiveBackupID = backup.ID.String()
		}
	}
	if err := m.persist(op, "snapshot_preparing"); err != nil {
		return err
	}
	dir := m.operationDir(op)
	identity, err := age.GenerateX25519Identity()
	if err != nil {
		return err
	}
	// This online key remains outside the encrypted snapshot and outside replaced
	// application state. It is specific to this operation and never reused.
	if err := instance.WritePrivateFile(filepath.Join(dir, "snapshot.key"), []byte(identity.String()+"\n")); err != nil {
		return err
	}
	work, err := os.MkdirTemp(dir, ".prepare-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(work)
	dump := filepath.Join(work, postgresArchiveDBEntry)
	if err := runPostgresDump(ctx, m.DBDSN, dump); err != nil {
		return err
	}
	keys, err := state.Keyring()
	if err != nil {
		return err
	}
	svc := NewInstanceBackupService(db, nil, nil, m.BackupDir, m.DeviceBackupDir, m.KnownHostsPath, m.DataDir, m.DBDSN, keys)
	svc.SetManagedState(m.StatePath)
	info, err := os.Stat(dump)
	if err != nil {
		return err
	}
	files, count, knownHosts, total, entries, err := svc.collectArchiveSourceFiles(ctx, svc.BackupArchiveLimits(), info.Size())
	if err != nil {
		return err
	}
	version := 0
	if schemaExists {
		if err := db.QueryRowContext(ctx, "SELECT version FROM schema_migrations").Scan(&version); err != nil {
			return err
		}
	}
	hash, err := computeFileHashContext(ctx, dump)
	if err != nil {
		return err
	}
	stateJSON, err := json.Marshal(state)
	if err != nil {
		return err
	}
	artifact := databaseBackupArtifact{tempPath: dump, archiveEntryName: postgresArchiveDBEntry, migrationVersion: version}
	plan, err := buildInstanceBackupArchiveManifestPlan(instanceBackupArchiveManifestInput{privateStateBytes: int64(len(stateJSON)), dbArtifact: artifact, backupCreatedAt: time.Now().UTC(), dbSHA256: hash, backupFileCount: count, totalSourceBytes: total, archiveFileEntries: entries, encryptionKeyring: keys, requiredKeyIDs: keys.KeyIDs(), limits: svc.BackupArchiveLimits()})
	if err != nil {
		return err
	}
	snapshot := filepath.Join(dir, "snapshot.age")
	_, verification, err := svc.createArchiveWithProtection(ctx, snapshot, artifact, files, knownHosts, plan.manifestJSON, &plan.manifest, uuid.New(), stateJSON, identity.Recipient().String())
	if err != nil {
		return err
	}
	if err := svc.verifyProtectedBackup(ctx, snapshot, verification); err != nil {
		return err
	}
	if err := validateRestoreArchiveFile(snapshot, m.restoreLimits()); err != nil {
		return err
	}
	op.SnapshotSHA256, err = computeFileHashContext(ctx, snapshot)
	if err != nil {
		return err
	}
	return m.persist(op, "snapshot_verified")
}

func (m *Maintenance) rollback(ctx context.Context, op *MaintenanceOperation) (result error) {
	defer func() {
		if result != nil {
			op.Error = "rollback could not complete; check maintenance logs and resume this operation"
			_ = m.persist(op, "operator_required")
		}
	}()
	if op.Attempt > 3 {
		return fmt.Errorf("maintenance retry limit reached")
	}
	if err := m.persist(op, "rolling_back"); err != nil {
		return err
	}
	dir := m.operationDir(op)
	snapshot := filepath.Join(dir, "snapshot.age")
	hash, err := computeFileHashContext(ctx, snapshot)
	if err != nil {
		return err
	}
	if op.SnapshotSHA256 == "" || hash != op.SnapshotSHA256 {
		return fmt.Errorf("safety snapshot does not match this operation's verified bytes")
	}
	identities, err := instance.ReadRecoveryFile(filepath.Join(dir, "snapshot.key"))
	if err != nil {
		return err
	}
	staging, err := os.MkdirTemp(dir, ".rollback-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(staging)
	state, err := ExtractProtectedInstanceBackup(ctx, snapshot, staging, m.restoreLimits(), identities...)
	if err != nil {
		return err
	}
	if op.Action == "rotate_operational" {
		var connected *sql.DB
		for _, dsn := range []string{m.DBDSN, state.DBDSN} {
			candidate := *m
			candidate.DBDSN = dsn
			db, openErr := candidate.openDB()
			if openErr != nil {
				continue
			}
			probe, cancel := context.WithTimeout(ctx, 5*time.Second)
			pingErr := db.PingContext(probe)
			cancel()
			if pingErr == nil {
				connected = db
				break
			}
			db.Close()
		}
		if connected == nil {
			return fmt.Errorf("neither protected database password can resume this operation")
		}
		err := setDatabaseRolePassword(ctx, connected, state.DatabasePassword)
		connected.Close()
		if err != nil {
			return err
		}
		m.DBDSN = state.DBDSN
	}
	if err := runPostgresRestore(ctx, m.DBDSN, filepath.Join(staging, postgresArchiveDBEntry)); err != nil {
		return err
	}
	if err := m.activateArtifacts(staging); err != nil {
		return err
	}
	if err := (instance.Store{Path: m.StatePath}).Update(func(current *instance.State) error { *current = *state; return nil }); err != nil {
		return err
	}
	if !op.SourceWasEmpty {
		db, err := m.openDB()
		if err != nil {
			return err
		}
		defer db.Close()
		keys, err := state.Keyring()
		if err != nil {
			return err
		}
		// Verify decryption without upgrading the schema again during rollback.
		manifest, err := readRestoreManifest(staging)
		if err != nil {
			return err
		}
		if manifest.MigrationVersion < postgres.SupportedSchemaVersion() {
			var version int
			var dirty bool
			if err := db.QueryRowContext(ctx, "SELECT version,dirty FROM schema_migrations").Scan(&version, &dirty); err != nil {
				return err
			}
			if dirty || version != manifest.MigrationVersion {
				return fmt.Errorf("original schema was not restored")
			}
			if err := VerifyIsolatedPostgresDump(ctx, filepath.Join(staging, postgresArchiveDBEntry), keys); err != nil {
				return err
			}
		} else if err := VerifyStoredCredentials(ctx, db, keys, false); err != nil {
			return err
		}
	}
	op.Error = "original database, files and instance secrets restored; application writes may reopen on the original release"
	op.VerifiedInstanceID, op.VerifiedActiveKeyID = state.InstanceID, state.ActiveKeyID
	op.VerifiedReleaseTag = op.OriginalReleaseTag
	return m.persist(op, "rolled_back")
}

func (m *Maintenance) activateArtifacts(staging string) error {
	certificates := filepath.Join(staging, "certificates")
	if _, err := os.Stat(certificates); os.IsNotExist(err) {
		if err := os.Mkdir(certificates, 0700); err != nil {
			return err
		}
	} else if err != nil {
		return err
	}
	if err := replaceDirForRestore(certificates, filepath.Join(m.DataDir, "certificates")); err != nil {
		return err
	}
	backups := filepath.Join(staging, "backups")
	if _, err := os.Stat(backups); os.IsNotExist(err) {
		if err := os.Mkdir(backups, 0700); err != nil {
			return err
		}
	} else if err != nil {
		return err
	}
	if err := replaceDirForRestore(backups, m.DeviceBackupDir); err != nil {
		return err
	}
	known := filepath.Join(staging, "known_hosts")
	if _, err := os.Stat(known); os.IsNotExist(err) {
		return os.RemoveAll(m.KnownHostsPath)
	} else if err != nil {
		return err
	}
	return replaceFileForRestore(known, m.KnownHostsPath)
}

// Resume rolls an interrupted mutation back to its immutable verified snapshot.
// It never captures a second source snapshot after live state may have changed.
func (m *Maintenance) Resume(ctx context.Context) error {
	release, err := instance.AcquireLease(m.StatePath)
	if err != nil {
		return err
	}
	defer release()
	op, err := m.Status()
	if err != nil {
		return err
	}
	if op == nil || terminalMaintenancePhase(op.Phase) {
		return nil
	}
	if op.Action == "postgres_major" {
		return fmt.Errorf("restart the original PostgreSQL 17 volume and use postgres-abort for this cutover")
	}
	if op.Phase == "preparing" || op.Phase == "snapshot_preparing" {
		op.Error = "operation interrupted before live changes; start a new operation"
		return m.persist(op, "rolled_back")
	}
	if op.Attempt >= 3 {
		return fmt.Errorf("maintenance retry limit reached; inspect the operation before manual recovery")
	}
	op.Attempt++
	return m.rollback(ctx, op)
}

// Restore validates and decrypts with an operator-held identity before taking the
// instance lease or modifying live state. Identity material is never journaled.
func (m *Maintenance) Restore(ctx context.Context, archive, recoveryFile string) error {
	identities, err := instance.ReadRecoveryFile(recoveryFile)
	if err != nil {
		return err
	}
	staging, err := os.MkdirTemp("", "theia-maintenance-restore-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(staging)
	recovered, err := ExtractProtectedInstanceBackup(ctx, archive, staging, m.restoreLimits(), identities...)
	if err != nil {
		return err
	}
	manifest, err := readRestoreManifest(staging)
	if err != nil {
		return err
	}
	if manifest.MigrationVersion > postgres.SupportedSchemaVersion() {
		return fmt.Errorf("backup requires a newer Theia release")
	}
	keys, err := recovered.Keyring()
	if err != nil {
		return err
	}
	if err := VerifyIsolatedPostgresDump(ctx, filepath.Join(staging, postgresArchiveDBEntry), keys); err != nil {
		return err
	}
	return m.mutate(ctx, "restore", func(ctx context.Context, db *sql.DB, prior *instance.State) error {
		if err := runPostgresRestore(ctx, m.DBDSN, filepath.Join(staging, postgresArchiveDBEntry)); err != nil {
			return err
		}
		if err := m.activateArtifacts(staging); err != nil {
			return err
		}
		// Destination connectivity remains local; archive credentials protect the
		// recovered data without requiring manual key-history edits on the new host.
		recovered.DBDSN, recovered.DatabasePassword = prior.DBDSN, prior.DatabasePassword
		if err := (instance.Store{Path: m.StatePath}).Update(func(current *instance.State) error { *current = *recovered; return nil }); err != nil {
			return err
		}
		// Restoring terminates existing database connections. Do not reuse a pool
		// that participated in preflight after the public schema was replaced.
		_ = db.Close()
		restoredDB, err := m.openDB()
		if err != nil {
			return err
		}
		defer restoredDB.Close()
		if err := postgres.RunMigrations(restoredDB, keys); err != nil {
			return err
		}
		return postgres.NewAuthRepo(restoredDB).RevokeAllSessions(ctx, time.Now().UTC())
	})
}
