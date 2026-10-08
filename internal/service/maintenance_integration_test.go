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

func TestMaintenanceIntegration(t *testing.T) {
	if os.Getenv("THEIA_MAINTENANCE_INTEGRATION") != "1" {
		t.Skip("run make maintenance-test for isolated PostgreSQL integration")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	if err := withIsolatedPostgres(ctx, func(dsn string) error {
		root := t.TempDir()
		state, err := instance.Generate(time.Now())
		if err != nil {
			return err
		}
		identity, _ := age.GenerateX25519Identity()
		state.RecoveryRecipient = identity.Recipient().String()
		store := instance.Store{Path: filepath.Join(root, "control", "secrets.json")}
		if err := store.Create(state); err != nil {
			return err
		}
		m := &Maintenance{StatePath: store.Path, DataDir: filepath.Join(root, "data"), BackupDir: filepath.Join(root, "archives"), DeviceBackupDir: filepath.Join(root, "devices"), KnownHostsPath: filepath.Join(root, "known_hosts"), DBDSN: dsn}
		if err := m.Migrate(ctx, false); err != nil {
			return err
		}
		initialized, err := m.Status()
		if err != nil {
			return err
		}
		if err := m.RequireVerifiedState(state.InstanceID, state.ActiveKeyID); err != nil {
			return err
		}
		if err := m.MarkWritesReopened(state.InstanceID, state.ActiveKeyID); err != nil {
			return err
		}
		if err := m.RollbackBeforeReopen(ctx); err == nil {
			return fmt.Errorf("rollback was allowed after application writes reopened")
		}
		if err := m.Migrate(ctx, false); err != nil {
			return err
		}
		unchanged, err := m.Status()
		if err != nil {
			return err
		}
		if unchanged.ID != initialized.ID {
			return fmt.Errorf("unchanged restart created another destructive maintenance operation")
		}
		db, err := postgres.OpenPrimaryDB(dsn)
		if err != nil {
			return err
		}
		defer db.Close()
		if err := os.MkdirAll(m.DeviceBackupDir, 0700); err != nil {
			return err
		}
		deviceFile := filepath.Join(m.DeviceBackupDir, "device.cfg")
		if err := os.WriteFile(deviceFile, []byte("original-device-config"), 0600); err != nil {
			return err
		}
		keys, _ := state.Keyring()
		ciphertext, err := keys.EncryptString("original-password")
		if err != nil {
			return err
		}
		if _, err := db.ExecContext(ctx, "INSERT INTO credential_profiles(id, name, encrypted_secret, created_at, updated_at) VALUES($1,$2,$3,now(),now())", uuid.NewString(), "snapshot-test", ciphertext); err != nil {
			return err
		}
		m.AfterApply = func() error {
			if _, err := db.ExecContext(ctx, "DELETE FROM credential_profiles"); err != nil {
				return err
			}
			if err := os.WriteFile(deviceFile, []byte("partially-replaced-config"), 0600); err != nil {
				return err
			}
			return fmt.Errorf("injected failure after mutation")
		}
		if err := m.Migrate(ctx, true); err == nil {
			t.Fatal("injected failure was ignored")
		}
		_ = db.Close()
		db, err = postgres.OpenPrimaryDB(dsn)
		if err != nil {
			return err
		}
		defer db.Close()
		first, err := m.Status()
		if err != nil {
			return err
		}
		if first.Phase != "rolled_back" {
			t.Fatalf("automatic rollback phase: %s", first.Phase)
		}
		assertOriginal := func(expected string) error {
			var restored string
			if err := db.QueryRowContext(ctx, "SELECT encrypted_secret FROM credential_profiles WHERE name = 'snapshot-test'").Scan(&restored); err != nil {
				return err
			}
			if restored != ciphertext {
				return fmt.Errorf("original database credential ciphertext was not restored")
			}
			got, err := os.ReadFile(deviceFile)
			if err != nil || string(got) != expected {
				return fmt.Errorf("device files were not rolled back")
			}
			restarted, err := store.Load()
			if err != nil {
				return err
			}
			if restarted.ActiveKeyID != state.ActiveKeyID {
				return fmt.Errorf("original credential key was not restored")
			}
			return nil
		}
		if err := assertOriginal("original-device-config"); err != nil {
			return err
		}
		// A later operation has a distinct immutable source, including intervening
		// file changes; it must never reuse the first operation's safety snapshot.
		if err := os.WriteFile(deviceFile, []byte("new-pre-operation-config"), 0600); err != nil {
			return err
		}
		second := &MaintenanceOperation{ID: uuid.NewString(), Action: "restore", Attempt: 1, CreatedAt: time.Now().UTC()}
		if err := m.persist(second, "preparing"); err != nil {
			return err
		}
		if err := m.prepareSafetySnapshot(ctx, db, state, second, nil); err != nil {
			return err
		}
		if err := m.persist(second, "applying"); err != nil {
			return err
		}
		if _, err := db.ExecContext(ctx, "DELETE FROM credential_profiles"); err != nil {
			return err
		}
		if err := os.WriteFile(deviceFile, []byte("crash-mid-restore"), 0600); err != nil {
			return err
		}
		if err := m.RequireReady(); err == nil {
			t.Fatal("HTTP allowed before recovering an interrupted operation")
		}
		if err := m.Resume(ctx); err != nil {
			return err
		}
		_ = db.Close()
		db, err = postgres.OpenPrimaryDB(dsn)
		if err != nil {
			return err
		}
		defer db.Close()
		if err := assertOriginal("new-pre-operation-config"); err != nil {
			return err
		}
		if first.ID == second.ID {
			t.Fatal("operations shared an identifier")
		}
		hash, err := computeFileHashContext(ctx, filepath.Join(m.operationDir(first), "snapshot.age"))
		if err != nil {
			return err
		}
		if hash != first.SnapshotSHA256 {
			t.Fatal("earlier verified safety snapshot was overwritten")
		}
		if err := m.RequireReady(); err != nil {
			return err
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}
