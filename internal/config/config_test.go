package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoad_DefaultsToPostgres(t *testing.T) {
	cfg, err := Load("/nonexistent-config.yaml")
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	if cfg.DBDriver != "postgres" {
		t.Fatalf("DBDriver = %q, want postgres", cfg.DBDriver)
	}
	if cfg.DBPath != "./data/theia.db" {
		t.Fatalf("DBPath = %q, want ./data/theia.db", cfg.DBPath)
	}
}

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

func TestLoad_FileHandling(t *testing.T) {
	tests := []struct {
		name        string
		contents    string
		env         map[string]string
		assert      func(t *testing.T, cfg *Config, err error)
	}{
		{
			name: "loads values from yaml file",
			contents: "db_driver: sqlite\nlisten_addr: \":9090\"\ndb_path: ./custom.db\ndata_dir: ./custom-data\nbridge_binaries_dir: ./bridges\n",
			assert: func(t *testing.T, cfg *Config, err error) {
				t.Helper()
				if err != nil {
					t.Fatalf("Load failed: %v", err)
				}
				if cfg.DBDriver != "sqlite" {
					t.Fatalf("DBDriver = %q, want sqlite", cfg.DBDriver)
				}
				if cfg.ListenAddr != ":9090" {
					t.Fatalf("ListenAddr = %q, want :9090", cfg.ListenAddr)
				}
				if cfg.DBPath != "./custom.db" {
					t.Fatalf("DBPath = %q, want ./custom.db", cfg.DBPath)
				}
				if cfg.DataDir != "./custom-data" {
					t.Fatalf("DataDir = %q, want ./custom-data", cfg.DataDir)
				}
				if cfg.BridgeBinariesDir != "./bridges" {
					t.Fatalf("BridgeBinariesDir = %q, want ./bridges", cfg.BridgeBinariesDir)
				}
			},
		},
		{
			name:     "returns parse error for invalid yaml",
			contents: "db_driver: [",
			assert: func(t *testing.T, cfg *Config, err error) {
				t.Helper()
				if err == nil {
					t.Fatal("Load error = nil, want parse error")
				}
				if !strings.Contains(err.Error(), "parsing config file") {
					t.Fatalf("Load error = %q, want parsing config file", err)
				}
			},
		},
		{
			name:     "environment overrides yaml values",
			contents: "db_driver: sqlite\ndata_dir: ./from-file\n",
			env: map[string]string{
				"THEIA_DB_DRIVER": "postgres",
				"THEIA_DATA_DIR":  "./from-env",
			},
			assert: func(t *testing.T, cfg *Config, err error) {
				t.Helper()
				if err != nil {
					t.Fatalf("Load failed: %v", err)
				}
				if cfg.DBDriver != "postgres" {
					t.Fatalf("DBDriver = %q, want postgres", cfg.DBDriver)
				}
				if cfg.DataDir != "./from-env" {
					t.Fatalf("DataDir = %q, want ./from-env", cfg.DataDir)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for key, value := range tt.env {
				t.Setenv(key, value)
			}

			path := filepath.Join(t.TempDir(), "config.yaml")
			if err := os.WriteFile(path, []byte(tt.contents), 0o644); err != nil {
				t.Fatalf("WriteFile failed: %v", err)
			}

			cfg, err := Load(path)
			tt.assert(t, cfg, err)
		})
	}
}
