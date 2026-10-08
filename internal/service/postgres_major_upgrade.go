package service

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/google/uuid"
	"github.com/lollinoo/theia/internal/instance"
	"github.com/lollinoo/theia/internal/repository/postgres"
)

func postgresServerMajor(ctx context.Context, db *sql.DB) (int, error) {
	var version int
	if err := db.QueryRowContext(ctx, "SELECT current_setting('server_version_num')::int").Scan(&version); err != nil {
		return 0, fmt.Errorf("PostgreSQL version preflight failed")
	}
	return version / 10000, nil
}

// PreparePostgresUpgrade protects PostgreSQL 17 before the deployment adapter
// starts a separate, empty PostgreSQL 18 volume. The source volume is preserved.
func (m *Maintenance) PreparePostgresUpgrade(ctx context.Context, target int) error {
	if err := m.validatePaths(); err != nil {
		return err
	}
	if target != 18 {
		return fmt.Errorf("the supported PostgreSQL transition is 17 to 18")
	}
	release, err := instance.AcquireLease(m.StatePath)
	if err != nil {
		return err
	}
	defer release()
	if err := m.RequireReady(); err != nil {
		return err
	}
	state, err := (instance.Store{Path: m.StatePath}).Load()
	if err != nil {
		return err
	}
	db, err := m.openDB()
	if err != nil {
		return err
	}
	defer db.Close()
	major, err := postgresServerMajor(ctx, db)
	if err != nil {
		return err
	}
	if major != 17 {
		return fmt.Errorf("PostgreSQL upgrade requires a verified version 17 source")
	}
	if err := postgres.RequireCurrentSchema(ctx, db); err != nil {
		return err
	}
	if err := VerifyInstanceIdentity(ctx, db, state.InstanceID); err != nil {
		return err
	}
	previous, err := m.Status()
	if err != nil {
		return err
	}
	if previous == nil {
		return fmt.Errorf("source instance must complete managed migration before its PostgreSQL major upgrade")
	}
	op := &MaintenanceOperation{ID: uuid.NewString(), Action: "postgres_major", Attempt: 1, CreatedAt: time.Now().UTC(), OriginalReleaseTag: previous.VerifiedReleaseTag, SourcePostgresMajor: major, TargetPostgresMajor: target}
	if err := m.persist(op, "preparing"); err != nil {
		return err
	}
	if err := m.prepareSafetySnapshot(ctx, db, state, op, previous); err != nil {
		op.Error = "PostgreSQL upgrade preparation failed; original volume was not changed"
		op.VerifiedInstanceID, op.VerifiedActiveKeyID, op.VerifiedReleaseTag = previous.VerifiedInstanceID, previous.VerifiedActiveKeyID, previous.VerifiedReleaseTag
		_ = m.persist(op, "rolled_back")
		return err
	}
	return m.persist(op, "awaiting_database_cutover")
}

func (m *Maintenance) postgresUpgradeOperation() (*MaintenanceOperation, error) {
	op, err := m.Status()
	if err != nil {
		return nil, err
	}
	if op == nil || op.Action != "postgres_major" || op.SourcePostgresMajor != 17 || op.TargetPostgresMajor != 18 || op.WritesReopened {
		return nil, fmt.Errorf("no PostgreSQL major upgrade is eligible for recovery")
	}
	return op, nil
}

// FinishPostgresUpgrade imports the exact verified source into a fresh version 18
// database. A partial or wrong target is refused; the adapter retains version 17
// and aborts the cutover rather than treating partial target data as a new source.
func (m *Maintenance) FinishPostgresUpgrade(ctx context.Context) (result error) {
	if err := m.validatePaths(); err != nil {
		return err
	}
	release, err := instance.AcquireLease(m.StatePath)
	if err != nil {
		return err
	}
	defer release()
	op, err := m.postgresUpgradeOperation()
	if err != nil {
		return err
	}
	if op.Phase != "awaiting_database_cutover" {
		return fmt.Errorf("cutover was interrupted; restart the original PostgreSQL 17 volume and abort this operation")
	}
	defer func() {
		if result != nil {
			op.Error = "version 18 verification failed; original version 17 volume remains preserved"
			_ = m.persist(op, "operator_required")
		}
	}()
	db, err := m.openDB()
	if err != nil {
		return err
	}
	major, err := postgresServerMajor(ctx, db)
	if err != nil {
		db.Close()
		return err
	}
	var tables int
	if err := db.QueryRowContext(ctx, "SELECT count(*) FROM pg_tables WHERE schemaname='public'").Scan(&tables); err != nil {
		db.Close()
		return err
	}
	db.Close()
	if major != 18 || tables != 0 {
		return fmt.Errorf("PostgreSQL 18 target must be a separate empty database")
	}
	staging, state, err := m.extractSafetySnapshot(ctx, op, ".major-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(staging)
	if err := m.persist(op, "database_importing"); err != nil {
		return err
	}
	if err := runPostgresRestore(ctx, m.DBDSN, filepath.Join(staging, postgresArchiveDBEntry)); err != nil {
		return err
	}
	if err := (instance.Store{Path: m.StatePath}).Update(func(current *instance.State) error {
		*current = *state
		_, err := current.RotateCredentials(time.Now(), false)
		return err
	}); err != nil {
		return err
	}
	current, err := (instance.Store{Path: m.StatePath}).Load()
	if err != nil {
		return err
	}
	keys, err := current.Keyring()
	if err != nil {
		return err
	}
	db, err = m.openDB()
	if err != nil {
		return err
	}
	defer db.Close()
	if err := postgres.RunMigrations(db, keys); err != nil {
		return err
	}
	if err := m.persist(op, "database_verifying"); err != nil {
		return err
	}
	if m.AfterApply != nil {
		if err := m.AfterApply(); err != nil {
			return err
		}
	}
	if err := postgres.RequireCurrentSchema(ctx, db); err != nil {
		return err
	}
	if err := VerifyStoredCredentials(ctx, db, keys, true); err != nil {
		return err
	}
	if err := VerifyInstanceIdentity(ctx, db, current.InstanceID); err != nil {
		return err
	}
	op.VerifiedInstanceID, op.VerifiedActiveKeyID, op.VerifiedReleaseTag = current.InstanceID, current.ActiveKeyID, m.ReleaseTag
	return m.persist(op, "completed")
}

// AbortPostgresUpgrade verifies the original version 17 volume and restores its
// matching files and secret state. It never overwrites or imports into that volume.
func (m *Maintenance) AbortPostgresUpgrade(ctx context.Context) (result error) {
	if err := m.validatePaths(); err != nil {
		return err
	}
	release, err := instance.AcquireLease(m.StatePath)
	if err != nil {
		return err
	}
	defer release()
	op, err := m.postgresUpgradeOperation()
	if err != nil {
		return err
	}
	if op.Phase == "rolled_back" {
		return nil
	}
	if op.Attempt >= 3 {
		return fmt.Errorf("PostgreSQL upgrade recovery retry limit reached")
	}
	op.Attempt++
	defer func() {
		if result != nil {
			op.Error = "original version 17 recovery could not be verified; instance remains in maintenance"
			_ = m.persist(op, "operator_required")
		}
	}()
	db, err := m.openDB()
	if err != nil {
		return err
	}
	defer db.Close()
	major, err := postgresServerMajor(ctx, db)
	if err != nil {
		return err
	}
	if major != 17 {
		return fmt.Errorf("restart the original PostgreSQL 17 volume before aborting the cutover")
	}
	if op.SnapshotSHA256 == "" {
		if op.Phase != "preparing" && op.Phase != "snapshot_preparing" {
			return fmt.Errorf("missing verified snapshot for an ambiguous cutover")
		}
		state, err := (instance.Store{Path: m.StatePath}).Load()
		if err != nil {
			return err
		}
		keys, err := state.Keyring()
		if err != nil {
			return err
		}
		if err := postgres.RequireCurrentSchema(ctx, db); err != nil {
			return err
		}
		if err := VerifyStoredCredentials(ctx, db, keys, true); err != nil {
			return err
		}
		if err := VerifyInstanceIdentity(ctx, db, state.InstanceID); err != nil {
			return err
		}
		op.VerifiedInstanceID, op.VerifiedActiveKeyID, op.VerifiedReleaseTag = state.InstanceID, state.ActiveKeyID, op.OriginalReleaseTag
		op.Error = "preparation interrupted before cutover; original PostgreSQL 17 instance verified"
		return m.persist(op, "rolled_back")
	}
	staging, state, err := m.extractSafetySnapshot(ctx, op, ".major-abort-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(staging)
	keys, err := state.Keyring()
	if err != nil {
		return err
	}
	if err := postgres.RequireCurrentSchema(ctx, db); err != nil {
		return err
	}
	if err := VerifyStoredCredentials(ctx, db, keys, true); err != nil {
		return err
	}
	if err := VerifyInstanceIdentity(ctx, db, state.InstanceID); err != nil {
		return err
	}
	if err := m.persist(op, "restoring_original_configuration"); err != nil {
		return err
	}
	if err := m.activateArtifacts(staging); err != nil {
		return err
	}
	if err := (instance.Store{Path: m.StatePath}).Update(func(current *instance.State) error { *current = *state; return nil }); err != nil {
		return err
	}
	op.VerifiedInstanceID, op.VerifiedActiveKeyID, op.VerifiedReleaseTag = state.InstanceID, state.ActiveKeyID, op.OriginalReleaseTag
	op.Error = "original PostgreSQL 17 volume, files and instance secrets verified; failed replacement remains separate"
	return m.persist(op, "rolled_back")
}
