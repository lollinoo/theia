package config

import "testing"

func TestLoad_EnvironmentOverridesDatabaseFields(t *testing.T) {
	t.Setenv("THEIA_DB_DRIVER", "postgres")
	t.Setenv("THEIA_DB_DSN", "postgres://theia:theia@127.0.0.1:5432/theia?sslmode=disable")
	t.Setenv("THEIA_DATA_DIR", "/tmp/theia-data")

	cfg, err := Load("/nonexistent-config.yaml")
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	if cfg.DBDriver != "postgres" {
		t.Fatalf("DBDriver = %q, want postgres", cfg.DBDriver)
	}
	if cfg.DBDSN == "" {
		t.Fatal("DBDSN should be populated from env")
	}
	if cfg.DataDir != "/tmp/theia-data" {
		t.Fatalf("DataDir = %q, want /tmp/theia-data", cfg.DataDir)
	}
}
