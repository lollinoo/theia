package service

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/lollinoo/theia/internal/instance"
	"github.com/lollinoo/theia/internal/repository/postgres"
)

func TestMaintenanceEmptyDatabaseIntegration(t *testing.T) {
	if os.Getenv("THEIA_MAINTENANCE_INTEGRATION") != "1" {
		t.Skip("run make maintenance-test for isolated PostgreSQL integration")
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	if err := withIsolatedPostgres(ctx, func(dsn string) error {
		root := t.TempDir()
		state, _ := instance.Generate(time.Now())
		store := instance.Store{Path: filepath.Join(root, "control", "state.json")}
		if err := store.Create(state); err != nil {
			return err
		}
		m := &Maintenance{StatePath: store.Path, DataDir: filepath.Join(root, "data"), DeviceBackupDir: filepath.Join(root, "data", "backups"), BackupDir: filepath.Join(root, "data", "archives"), KnownHostsPath: filepath.Join(root, "data", "known_hosts"), DBDSN: dsn, ReleaseTag: "v1.8.0"}
		if err := m.Migrate(ctx, false); err != nil {
			return err
		}
		db, err := postgres.OpenPrimaryDB(dsn)
		if err != nil {
			return err
		}
		defer db.Close()
		if _, err := db.ExecContext(ctx, "DROP SCHEMA public CASCADE; CREATE SCHEMA public"); err != nil {
			return err
		}
		for range 2 {
			if err := m.Migrate(ctx, false); err == nil || !strings.Contains(err.Error(), "unexpectedly empty") {
				return fmt.Errorf("unexpected empty database was silently initialized: %v", err)
			}
		}
		var exists bool
		if err := db.QueryRowContext(ctx, "SELECT to_regclass('public.schema_migrations') IS NOT NULL").Scan(&exists); err != nil {
			return err
		}
		if exists {
			return fmt.Errorf("lost database was replaced with an empty schema")
		}
		persisted, err := store.Load()
		if err != nil {
			return err
		}
		if persisted.ActiveKeyID != state.ActiveKeyID {
			return fmt.Errorf("lost database triggered replacement keys")
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}
