package service

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"filippo.io/age"
	"github.com/google/uuid"
	"github.com/lollinoo/theia/internal/instance"
	"github.com/lollinoo/theia/internal/repository/postgres"
)

func TestMaintenanceLegacyConversionIntegration(t *testing.T) {
	if os.Getenv("THEIA_MAINTENANCE_INTEGRATION") != "1" {
		t.Skip("run make maintenance-test")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	if err := withIsolatedPostgres(ctx, func(dsn string) error {
		db, err := postgres.OpenPrimaryDB(dsn)
		if err != nil {
			return err
		}
		defer db.Close()
		state, err := instance.Generate(time.Now())
		if err != nil {
			return err
		}
		identity, _ := age.GenerateX25519Identity()
		state.RecoveryRecipient = identity.Recipient().String()
		keys, _ := state.Keyring()
		if err := postgres.RunMigrations(db, keys); err != nil {
			return err
		}
		encrypted, err := keys.EncryptString("legacy-preserved-password")
		if err != nil {
			return err
		}
		if _, err := db.ExecContext(ctx, "INSERT INTO credential_profiles(id,name,encrypted_secret,created_at,updated_at) VALUES($1,'legacy-conversion',$2,now(),now())", uuid.NewString(), encrypted); err != nil {
			return err
		}
		root := t.TempDir()
		m := &Maintenance{StatePath: filepath.Join(root, "control", "state.json"), DataDir: filepath.Join(root, "data"), BackupDir: filepath.Join(root, "archives"), DeviceBackupDir: filepath.Join(root, "devices"), KnownHostsPath: filepath.Join(root, "known_hosts"), DBDSN: dsn}
		if err := (instance.Store{Path: m.StatePath}).Create(state); err != nil {
			return err
		}
		legacy := NewInstanceBackupService(db, postgres.NewInstanceBackupRepo(db), postgres.NewSettingsRepo(db), m.BackupDir, m.DeviceBackupDir, m.KnownHostsPath, m.DataDir, dsn, keys)
		original, err := legacy.Create(ctx)
		if err != nil {
			return err
		}
		before, err := computeFileHashContext(ctx, original.FilePath)
		if err != nil {
			return err
		}
		converted := filepath.Join(root, "converted.age")
		if _, err := m.ConvertLegacyArchive(ctx, original.FilePath, converted); err != nil {
			return err
		}
		staging := filepath.Join(root, "extracted")
		if err := os.Mkdir(staging, 0700); err != nil {
			return err
		}
		restored, err := ExtractProtectedInstanceBackup(ctx, converted, staging, m.restoreLimits(), identity)
		if err != nil {
			return err
		}
		if restored.CredentialKeys[state.ActiveKeyID].Secret != state.CredentialKeys[state.ActiveKeyID].Secret {
			return fmt.Errorf("legacy secret was replaced")
		}
		after, _ := computeFileHashContext(ctx, original.FilePath)
		if before != after {
			return fmt.Errorf("original legacy archive changed")
		}
		if _, err := m.ConvertLegacyArchive(ctx, original.FilePath, converted); err == nil {
			return fmt.Errorf("converted archive was overwritten")
		}
		// Recreate the precise pre-29 schema. Its first transition must use a raw
		// preventive archive and retain a record once the new schema is verified.
		if _, err := db.ExecContext(ctx, `DROP TRIGGER retain_deleted_backup_file ON backup_files; DROP FUNCTION retain_deleted_backup_file(); DROP INDEX idx_backup_files_file_path; DROP TABLE backup_file_deletions; UPDATE schema_migrations SET version=28`); err != nil {
			return err
		}
		m.AfterApply = func() error { return fmt.Errorf("interruption after older schema was migrated") }
		if err := m.Migrate(ctx, false); err == nil {
			return fmt.Errorf("injected migration fault ignored")
		}
		db.Close()
		db, err = postgres.OpenPrimaryDB(dsn)
		if err != nil {
			return err
		}
		defer db.Close()
		var version int
		if err := db.QueryRowContext(ctx, "SELECT version FROM schema_migrations").Scan(&version); err != nil {
			return err
		}
		if version != 28 {
			return fmt.Errorf("older original schema not restored")
		}
		m.AfterApply = nil
		if err := m.Migrate(ctx, false); err != nil {
			return err
		}
		op, err := m.Status()
		if err != nil {
			return err
		}
		if op.LegacyPreventiveFileName == "" {
			return fmt.Errorf("legacy preventive archive missing")
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}
