package config

// This file exercises config behavior so refactors preserve the documented contract.

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
	legacyAuthField := "Operator" + "Token"
	if _, ok := cfgType.FieldByName(legacyAuthField); ok {
		t.Fatalf("Config still exposes %s; backend auth must use password sessions", legacyAuthField)
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

func TestLoad_DefaultsArchiveLimitsToConservativeValues(t *testing.T) {
	cfg, err := Load("/nonexistent-config.yaml")
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	if got, want := cfg.RestoreArchiveLimits.MaxCompressedBytes, int64(256<<20); got != want {
		t.Fatalf("RestoreArchiveLimits.MaxCompressedBytes = %d, want %d", got, want)
	}
	if got, want := cfg.RestoreArchiveLimits.MaxTotalBytes, int64(1<<30); got != want {
		t.Fatalf("RestoreArchiveLimits.MaxTotalBytes = %d, want %d", got, want)
	}
	if got, want := cfg.RestoreArchiveLimits.MaxEntryBytes, int64(512<<20); got != want {
		t.Fatalf("RestoreArchiveLimits.MaxEntryBytes = %d, want %d", got, want)
	}
	if got, want := cfg.RestoreArchiveLimits.MaxFileEntries, 25000; got != want {
		t.Fatalf("RestoreArchiveLimits.MaxFileEntries = %d, want %d", got, want)
	}
	if got, want := cfg.InstanceBackupArchiveLimits.MaxTotalBytes, int64(2<<30); got != want {
		t.Fatalf("InstanceBackupArchiveLimits.MaxTotalBytes = %d, want %d", got, want)
	}
	if got, want := cfg.InstanceBackupArchiveLimits.MaxEntryBytes, int64(1<<30); got != want {
		t.Fatalf("InstanceBackupArchiveLimits.MaxEntryBytes = %d, want %d", got, want)
	}
	if got, want := cfg.InstanceBackupArchiveLimits.MaxFileEntries, 50000; got != want {
		t.Fatalf("InstanceBackupArchiveLimits.MaxFileEntries = %d, want %d", got, want)
	}
	if got, want := cfg.InstanceBackupArchiveLimits.MaxDurationSeconds, 1800; got != want {
		t.Fatalf("InstanceBackupArchiveLimits.MaxDurationSeconds = %d, want %d", got, want)
	}
	if got, want := cfg.BulkBackupLimits.MaxDevices, 100; got != want {
		t.Fatalf("BulkBackupLimits.MaxDevices = %d, want %d", got, want)
	}
	if got, want := cfg.BulkBackupLimits.MaxQueuedJobs, 100; got != want {
		t.Fatalf("BulkBackupLimits.MaxQueuedJobs = %d, want %d", got, want)
	}
	if got, want := cfg.BulkDownloadLimits.MaxDevices, 100; got != want {
		t.Fatalf("BulkDownloadLimits.MaxDevices = %d, want %d", got, want)
	}
	if got, want := cfg.BulkDownloadLimits.MaxFiles, 500; got != want {
		t.Fatalf("BulkDownloadLimits.MaxFiles = %d, want %d", got, want)
	}
	if got, want := cfg.BulkDownloadLimits.MaxBytes, int64(512<<20); got != want {
		t.Fatalf("BulkDownloadLimits.MaxBytes = %d, want %d", got, want)
	}
	if got, want := cfg.BulkDownloadLimits.MaxConcurrentPerActor, 1; got != want {
		t.Fatalf("BulkDownloadLimits.MaxConcurrentPerActor = %d, want %d", got, want)
	}
	if got, want := cfg.BulkDownloadLimits.MaxConcurrentGlobal, 4; got != want {
		t.Fatalf("BulkDownloadLimits.MaxConcurrentGlobal = %d, want %d", got, want)
	}
}

func TestLoad_EnvironmentOverridesDatabaseFields(t *testing.T) {
	t.Setenv("THEIA_DB_DSN", "postgres://theia:theia@127.0.0.1:5432/theia?sslmode=disable")
	t.Setenv("THEIA_DATA_DIR", "/tmp/theia-data")
	t.Setenv("THEIA_DEPLOYMENT_ENV", "production")
	t.Setenv("THEIA_SESSION_SECRET", "0123456789abcdef0123456789abcdef")
	t.Setenv("THEIA_SESSION_TTL_MINUTES", "90")
	t.Setenv("THEIA_PASSWORD_RESET_TTL_MINUTES", "45")
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
	if cfg.SessionSecret == "" {
		t.Fatal("SessionSecret should be populated from env")
	}
	if cfg.SessionTTLMinutes != 90 {
		t.Fatalf("SessionTTLMinutes = %d, want 90", cfg.SessionTTLMinutes)
	}
	if cfg.PasswordResetTTLMinutes != 45 {
		t.Fatalf("PasswordResetTTLMinutes = %d, want 45", cfg.PasswordResetTTLMinutes)
	}
	if cfg.MetricsToken == "" {
		t.Fatal("MetricsToken should be populated from env")
	}
	if got, want := cfg.AllowedOrigins, []string{"https://theia.example.com", "http://localhost:3000"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("AllowedOrigins = %#v, want %#v", got, want)
	}
}

func TestLoad_ArchiveLimitOverridesFromYAMLAndEnvironment(t *testing.T) {
	t.Setenv("THEIA_RESTORE_MAX_TOTAL_BYTES", "2222")
	t.Setenv("THEIA_INSTANCE_BACKUP_MAX_DURATION_SECONDS", "88")
	t.Setenv("THEIA_BULK_BACKUP_MAX_DEVICES", "12")
	t.Setenv("THEIA_BULK_DOWNLOAD_MAX_BYTES", "9999")
	t.Setenv("THEIA_BULK_DOWNLOAD_MAX_CONCURRENT_GLOBAL", "6")

	path := filepath.Join(t.TempDir(), "config.yaml")
	contents := `
restore_archive_limits:
  max_compressed_bytes: 1111
  max_total_bytes: 2000
  max_entry_bytes: 3333
  max_file_entries: 44
instance_backup_archive_limits:
  max_total_bytes: 5555
  max_entry_bytes: 6666
  max_file_entries: 77
  max_duration_seconds: 99
bulk_backup_limits:
  max_devices: 11
  max_queued_jobs: 22
bulk_download_limits:
  max_devices: 33
  max_files: 44
  max_bytes: 8888
  max_concurrent_per_actor: 5
  max_concurrent_global: 7
`
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	if got, want := cfg.RestoreArchiveLimits.MaxCompressedBytes, int64(1111); got != want {
		t.Fatalf("RestoreArchiveLimits.MaxCompressedBytes = %d, want %d", got, want)
	}
	if got, want := cfg.RestoreArchiveLimits.MaxTotalBytes, int64(2222); got != want {
		t.Fatalf("RestoreArchiveLimits.MaxTotalBytes = %d, want env override %d", got, want)
	}
	if got, want := cfg.RestoreArchiveLimits.MaxEntryBytes, int64(3333); got != want {
		t.Fatalf("RestoreArchiveLimits.MaxEntryBytes = %d, want %d", got, want)
	}
	if got, want := cfg.RestoreArchiveLimits.MaxFileEntries, 44; got != want {
		t.Fatalf("RestoreArchiveLimits.MaxFileEntries = %d, want %d", got, want)
	}
	if got, want := cfg.InstanceBackupArchiveLimits.MaxTotalBytes, int64(5555); got != want {
		t.Fatalf("InstanceBackupArchiveLimits.MaxTotalBytes = %d, want %d", got, want)
	}
	if got, want := cfg.InstanceBackupArchiveLimits.MaxEntryBytes, int64(6666); got != want {
		t.Fatalf("InstanceBackupArchiveLimits.MaxEntryBytes = %d, want %d", got, want)
	}
	if got, want := cfg.InstanceBackupArchiveLimits.MaxFileEntries, 77; got != want {
		t.Fatalf("InstanceBackupArchiveLimits.MaxFileEntries = %d, want %d", got, want)
	}
	if got, want := cfg.InstanceBackupArchiveLimits.MaxDurationSeconds, 88; got != want {
		t.Fatalf("InstanceBackupArchiveLimits.MaxDurationSeconds = %d, want env override %d", got, want)
	}
	if got, want := cfg.BulkBackupLimits.MaxDevices, 12; got != want {
		t.Fatalf("BulkBackupLimits.MaxDevices = %d, want env override %d", got, want)
	}
	if got, want := cfg.BulkBackupLimits.MaxQueuedJobs, 22; got != want {
		t.Fatalf("BulkBackupLimits.MaxQueuedJobs = %d, want %d", got, want)
	}
	if got, want := cfg.BulkDownloadLimits.MaxDevices, 33; got != want {
		t.Fatalf("BulkDownloadLimits.MaxDevices = %d, want %d", got, want)
	}
	if got, want := cfg.BulkDownloadLimits.MaxFiles, 44; got != want {
		t.Fatalf("BulkDownloadLimits.MaxFiles = %d, want %d", got, want)
	}
	if got, want := cfg.BulkDownloadLimits.MaxBytes, int64(9999); got != want {
		t.Fatalf("BulkDownloadLimits.MaxBytes = %d, want env override %d", got, want)
	}
	if got, want := cfg.BulkDownloadLimits.MaxConcurrentPerActor, 5; got != want {
		t.Fatalf("BulkDownloadLimits.MaxConcurrentPerActor = %d, want %d", got, want)
	}
	if got, want := cfg.BulkDownloadLimits.MaxConcurrentGlobal, 6; got != want {
		t.Fatalf("BulkDownloadLimits.MaxConcurrentGlobal = %d, want env override %d", got, want)
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
			contents: "listen_addr: \":9090\"\ndb_dsn: postgres://user:pass@db:5432/theia?sslmode=disable\ndata_dir: ./custom-data\nbridge_binaries_dir: ./bridges\ndeployment_env: staging\nsession_secret: yaml-session-secret\nsession_ttl_minutes: 120\npassword_reset_ttl_minutes: 60\nmetrics_token: yaml-metrics\nallowed_origins:\n  - https://theia.example.com\n",
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
				if cfg.SessionSecret != "yaml-session-secret" {
					t.Fatalf("SessionSecret = %q, want yaml-session-secret", cfg.SessionSecret)
				}
				if cfg.SessionTTLMinutes != 120 {
					t.Fatalf("SessionTTLMinutes = %d, want 120", cfg.SessionTTLMinutes)
				}
				if cfg.PasswordResetTTLMinutes != 60 {
					t.Fatalf("PasswordResetTTLMinutes = %d, want 60", cfg.PasswordResetTTLMinutes)
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

func TestLoad_RejectsInvalidArchiveLimitOverrides(t *testing.T) {
	tests := []struct {
		name  string
		key   string
		value string
	}{
		{name: "restore compressed zero", key: "THEIA_RESTORE_MAX_COMPRESSED_BYTES", value: "0"},
		{name: "restore total negative", key: "THEIA_RESTORE_MAX_TOTAL_BYTES", value: "-1"},
		{name: "restore entry non integer", key: "THEIA_RESTORE_MAX_ENTRY_BYTES", value: "not-an-integer"},
		{name: "restore entries zero", key: "THEIA_RESTORE_MAX_FILE_ENTRIES", value: "0"},
		{name: "backup total zero", key: "THEIA_INSTANCE_BACKUP_MAX_TOTAL_BYTES", value: "0"},
		{name: "backup entry negative", key: "THEIA_INSTANCE_BACKUP_MAX_ENTRY_BYTES", value: "-1"},
		{name: "backup entries non integer", key: "THEIA_INSTANCE_BACKUP_MAX_FILE_ENTRIES", value: "not-an-integer"},
		{name: "backup duration zero", key: "THEIA_INSTANCE_BACKUP_MAX_DURATION_SECONDS", value: "0"},
		{name: "backup duration overflows time duration", key: "THEIA_INSTANCE_BACKUP_MAX_DURATION_SECONDS", value: "9223372037"},
		{name: "bulk backup devices zero", key: "THEIA_BULK_BACKUP_MAX_DEVICES", value: "0"},
		{name: "bulk backup queued negative", key: "THEIA_BULK_BACKUP_MAX_QUEUED_JOBS", value: "-1"},
		{name: "bulk download devices non integer", key: "THEIA_BULK_DOWNLOAD_MAX_DEVICES", value: "not-an-integer"},
		{name: "bulk download files zero", key: "THEIA_BULK_DOWNLOAD_MAX_FILES", value: "0"},
		{name: "bulk download bytes negative", key: "THEIA_BULK_DOWNLOAD_MAX_BYTES", value: "-1"},
		{name: "bulk download actor concurrency zero", key: "THEIA_BULK_DOWNLOAD_MAX_CONCURRENT_PER_ACTOR", value: "0"},
		{name: "bulk download global concurrency negative", key: "THEIA_BULK_DOWNLOAD_MAX_CONCURRENT_GLOBAL", value: "-1"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv(tt.key, tt.value)
			if _, err := Load("/nonexistent-config.yaml"); err == nil {
				t.Fatalf("Load() error = nil, want invalid %s rejection", tt.key)
			}
		})
	}
}

func TestLoad_RejectsInvalidArchiveLimitYAML(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	tests := []struct {
		name     string
		contents string
		want     string
	}{
		{
			name:     "restore total",
			contents: "restore_archive_limits:\n  max_total_bytes: 0\n",
			want:     "restore_archive_limits.max_total_bytes",
		},
		{
			name:     "bulk backup devices",
			contents: "bulk_backup_limits:\n  max_devices: 0\n",
			want:     "bulk_backup_limits.max_devices",
		},
		{
			name:     "bulk download bytes",
			contents: "bulk_download_limits:\n  max_bytes: -1\n",
			want:     "bulk_download_limits.max_bytes",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := os.WriteFile(path, []byte(tt.contents), 0o644); err != nil {
				t.Fatalf("WriteFile failed: %v", err)
			}

			_, err := Load(path)
			if err == nil {
				t.Fatal("Load() error = nil, want invalid YAML limit rejection")
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("Load() error = %q, want %s", err.Error(), tt.want)
			}
		})
	}
}

func TestLoad_RejectsArchiveLimitYAMLDurationOverflow(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	contents := "instance_backup_archive_limits:\n  max_duration_seconds: 9223372037\n"
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}

	_, err := Load(path)
	if err == nil {
		t.Fatal("Load() error = nil, want duration overflow rejection")
	}
	if !strings.Contains(err.Error(), "instance_backup_archive_limits.max_duration_seconds") {
		t.Fatalf("Load() error = %q, want instance_backup_archive_limits.max_duration_seconds", err.Error())
	}
}

func TestLoad_RejectsInvalidAuthTTLOverrides(t *testing.T) {
	tests := []struct {
		name string
		key  string
	}{
		{name: "session ttl", key: "THEIA_SESSION_TTL_MINUTES"},
		{name: "password reset ttl", key: "THEIA_PASSWORD_RESET_TTL_MINUTES"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv(tt.key, "not-an-integer")
			if _, err := Load("/nonexistent-config.yaml"); err == nil {
				t.Fatalf("Load() error = nil, want invalid %s rejection", tt.key)
			}
		})
	}
}
