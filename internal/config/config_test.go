package config

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestConfigSchemaIsPostgresOnly(t *testing.T) {
	cfgType := reflect.TypeOf(Config{})
	for _, fieldName := range []string{"DB" + "Driver", "DB" + "Path"} {
		if _, ok := cfgType.FieldByName(fieldName); ok {
			t.Fatalf("Config still exposes %s; runtime database selection must stay removed", fieldName)
		}
	}
}

func TestLoad_DefaultsToPostgresDSNConfiguration(t *testing.T) {
	cfg, err := Load("/nonexistent-config.yaml")
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	if cfg.DBDSN != "" {
		t.Fatalf("DBDSN = %q, want empty default", cfg.DBDSN)
	}
	if cfg.DataDir != "./data" {
		t.Fatalf("DataDir = %q, want ./data", cfg.DataDir)
	}
}

func TestLoad_EnvironmentOverridesDatabaseFields(t *testing.T) {
	t.Setenv("THEIA_DB_DSN", "postgres://theia:theia@127.0.0.1:5432/theia?sslmode=disable")
	t.Setenv("THEIA_DATA_DIR", "/tmp/theia-data")
	t.Setenv("THEIA_DEPLOYMENT_ENV", "production")
	t.Setenv("THEIA_OPERATOR_TOKEN", "0123456789abcdef0123456789abcdef")
	t.Setenv("THEIA_METRICS_TOKEN", "abcdef0123456789abcdef0123456789")
	t.Setenv("THEIA_ALLOWED_ORIGINS", "https://theia.example.com,http://localhost:3000")

	cfg, err := Load("/nonexistent-config.yaml")
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	if cfg.DBDSN == "" {
		t.Fatal("DBDSN should be populated from env")
	}
	if cfg.DataDir != "/tmp/theia-data" {
		t.Fatalf("DataDir = %q, want /tmp/theia-data", cfg.DataDir)
	}
	if cfg.DeploymentEnv != "production" {
		t.Fatalf("DeploymentEnv = %q, want production", cfg.DeploymentEnv)
	}
	if cfg.OperatorToken == "" {
		t.Fatal("OperatorToken should be populated from env")
	}
	if cfg.MetricsToken == "" {
		t.Fatal("MetricsToken should be populated from env")
	}
	if got, want := cfg.AllowedOrigins, []string{"https://theia.example.com", "http://localhost:3000"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("AllowedOrigins = %#v, want %#v", got, want)
	}
}

func TestLoad_FileHandling(t *testing.T) {
	tests := []struct {
		name     string
		contents string
		env      map[string]string
		assert   func(t *testing.T, cfg *Config, err error)
	}{
		{
			name:     "loads values from yaml file",
			contents: "listen_addr: \":9090\"\ndb_dsn: postgres://user:pass@db:5432/theia?sslmode=disable\ndata_dir: ./custom-data\nbridge_binaries_dir: ./bridges\ndeployment_env: staging\noperator_token: yaml-token\nmetrics_token: yaml-metrics\nallowed_origins:\n  - https://theia.example.com\n",
			assert: func(t *testing.T, cfg *Config, err error) {
				t.Helper()
				if err != nil {
					t.Fatalf("Load failed: %v", err)
				}
				if cfg.ListenAddr != ":9090" {
					t.Fatalf("ListenAddr = %q, want :9090", cfg.ListenAddr)
				}
				if cfg.DBDSN != "postgres://user:pass@db:5432/theia?sslmode=disable" {
					t.Fatalf("DBDSN = %q, want yaml dsn", cfg.DBDSN)
				}
				if cfg.DataDir != "./custom-data" {
					t.Fatalf("DataDir = %q, want ./custom-data", cfg.DataDir)
				}
				if cfg.BridgeBinariesDir != "./bridges" {
					t.Fatalf("BridgeBinariesDir = %q, want ./bridges", cfg.BridgeBinariesDir)
				}
				if cfg.DeploymentEnv != "staging" {
					t.Fatalf("DeploymentEnv = %q, want staging", cfg.DeploymentEnv)
				}
				if cfg.OperatorToken != "yaml-token" {
					t.Fatalf("OperatorToken = %q, want yaml-token", cfg.OperatorToken)
				}
				if cfg.MetricsToken != "yaml-metrics" {
					t.Fatalf("MetricsToken = %q, want yaml-metrics", cfg.MetricsToken)
				}
				if got, want := cfg.AllowedOrigins, []string{"https://theia.example.com"}; !reflect.DeepEqual(got, want) {
					t.Fatalf("AllowedOrigins = %#v, want %#v", got, want)
				}
			},
		},
		{
			name:     "returns parse error for invalid yaml",
			contents: "db_dsn: [",
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
			contents: "db_dsn: postgres://file:pass@db:5432/theia?sslmode=disable\ndata_dir: ./from-file\ndeployment_env: staging\n",
			env: map[string]string{
				"THEIA_DB_DSN":         "postgres://env:pass@db:5432/theia?sslmode=disable",
				"THEIA_DATA_DIR":       "./from-env",
				"THEIA_DEPLOYMENT_ENV": "production",
			},
			assert: func(t *testing.T, cfg *Config, err error) {
				t.Helper()
				if err != nil {
					t.Fatalf("Load failed: %v", err)
				}
				if cfg.DBDSN != "postgres://env:pass@db:5432/theia?sslmode=disable" {
					t.Fatalf("DBDSN = %q, want env dsn", cfg.DBDSN)
				}
				if cfg.DataDir != "./from-env" {
					t.Fatalf("DataDir = %q, want ./from-env", cfg.DataDir)
				}
				if cfg.DeploymentEnv != "production" {
					t.Fatalf("DeploymentEnv = %q, want production", cfg.DeploymentEnv)
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
