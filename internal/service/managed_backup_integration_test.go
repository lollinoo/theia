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
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/lollinoo/theia/internal/domain"
	"github.com/lollinoo/theia/internal/instance"
	"github.com/lollinoo/theia/internal/repository/postgres"
)

// This test uses real PostgreSQL 18 binaries in the production backend image.
// No host database, live project volume, or external credentials are required.
func TestManagedBackupIntegration(t *testing.T) {
	if os.Getenv("THEIA_MAINTENANCE_INTEGRATION") != "1" {
		t.Skip("run make maintenance-test for isolated PostgreSQL integration")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
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
		identity, err := age.GenerateX25519Identity()
		if err != nil {
			return err
		}
		state.RecoveryRecipient = identity.Recipient().String()
		keys, err := state.Keyring()
		if err != nil {
			return err
		}
		if err := postgres.RunMigrations(db, keys); err != nil {
			return err
		}
		encrypted, err := keys.EncryptString("original-device-password")
		if err != nil {
			return err
		}
		if _, err := db.ExecContext(ctx, "INSERT INTO credential_profiles(id, name, encrypted_secret, created_at, updated_at) VALUES($1, $2, $3, now(), now())", uuid.NewString(), "recovery-test", encrypted); err != nil {
			return err
		}
		root := t.TempDir()
		deviceDir := filepath.Join(root, "devices")
		if err := os.Mkdir(deviceDir, 0700); err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(deviceDir, "device.cfg"), []byte("device-configuration"), 0600); err != nil {
			return err
		}
		store := instance.Store{Path: filepath.Join(root, "control", "secrets.json")}
		if err := store.Create(state); err != nil {
			return err
		}
		repo := postgres.NewInstanceBackupRepo(db)
		svc := NewInstanceBackupService(db, repo, postgres.NewSettingsRepo(db), filepath.Join(root, "archives"), deviceDir, filepath.Join(root, "known_hosts"), root, dsn, keys)
		svc.SetManagedState(store.Path)
		first, err := svc.Create(ctx)
		if err != nil {
			return err
		}
		if first.Status != domain.InstanceBackupStatusSuccess {
			t.Fatal("verified managed backup not successful")
		}
		if err := store.Update(func(s *instance.State) error {
			_, err := s.RotateCredentials(time.Now().Add(instance.CredentialRotationInterval), false)
			return err
		}); err != nil {
			return err
		}
		rotated, err := store.Load()
		if err != nil {
			return err
		}
		rotatedKeys, err := rotated.Keyring()
		if err != nil {
			return err
		}
		if err := postgres.RunMigrations(db, rotatedKeys); err != nil {
			return err
		}
		if err := VerifyStoredCredentials(ctx, db, rotatedKeys, true); err != nil {
			return err
		}
		// Recover an archive made before rotation into a replacement isolated host DB.
		staging := t.TempDir()
		recovered, err := ExtractProtectedInstanceBackup(ctx, first.FilePath, staging, DefaultRestoreArchiveLimits, identity)
		if err != nil {
			return err
		}
		recoveredKeys, err := recovered.Keyring()
		if err != nil {
			return err
		}
		if err := VerifyIsolatedPostgresDump(ctx, filepath.Join(staging, "database.dump"), recoveredKeys); err != nil {
			return err
		}
		recoveryFile := filepath.Join(t.TempDir(), "recovery.txt")
		if err := os.WriteFile(recoveryFile, []byte(identity.String()+"\n"), 0600); err != nil {
			return err
		}
		return withIsolatedPostgres(ctx, func(targetDSN string) error {
			targetRoot := t.TempDir()
			targetState, err := instance.Generate(time.Now())
			if err != nil {
				return err
			}
			targetState.DBDSN = targetDSN
			targetStore := instance.Store{Path: filepath.Join(targetRoot, "control", "state.json")}
			if err := targetStore.Create(targetState); err != nil {
				return err
			}
			maintenance := &Maintenance{StatePath: targetStore.Path, DataDir: filepath.Join(targetRoot, "data"), BackupDir: filepath.Join(targetRoot, "data", "archives"), DeviceBackupDir: filepath.Join(targetRoot, "data", "backups"), KnownHostsPath: filepath.Join(targetRoot, "data", "known_hosts"), DBDSN: targetDSN}
			if err := maintenance.Restore(ctx, first.FilePath, recoveryFile); err != nil {
				return err
			}
			targetKeys, err := targetStore.Load()
			if err != nil {
				return err
			}
			if targetKeys.ActiveKeyID != state.ActiveKeyID || targetKeys.DBDSN != targetDSN {
				return fmt.Errorf("restored keys or destination database connection were not preserved")
			}
			if err := maintenance.RequireVerifiedState(targetKeys.InstanceID, targetKeys.ActiveKeyID); err != nil {
				return err
			}
			got, err := os.ReadFile(filepath.Join(maintenance.DeviceBackupDir, "device.cfg"))
			if err != nil || string(got) != "device-configuration" {
				return fmt.Errorf("replacement-host restore lost device files")
			}
			return nil
		})
	}); err != nil {
		t.Fatal(err)
	}
}
