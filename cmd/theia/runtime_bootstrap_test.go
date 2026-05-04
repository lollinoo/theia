package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/lollinoo/theia/internal/domain"
	"github.com/lollinoo/theia/internal/repository/sqlite"
)

type stubRuntimeStopper struct {
	name  string
	stops *[]string
}

func (s stubRuntimeStopper) Stop() {
	*s.stops = append(*s.stops, s.name)
}

type stubRuntimeServer struct {
	listenErr error
}

func (s stubRuntimeServer) ListenAndServe() error {
	return s.listenErr
}

func (stubRuntimeServer) Shutdown(context.Context) error {
	return nil
}

type runtimeDebugSettingsRepo struct {
	values map[string]string
}

func (r runtimeDebugSettingsRepo) Get(key string) (string, error) {
	value, ok := r.values[key]
	if !ok {
		return "", errors.New("missing setting")
	}
	return value, nil
}

func (r runtimeDebugSettingsRepo) Set(key, value string) error {
	if r.values == nil {
		r.values = map[string]string{}
	}
	r.values[key] = value
	return nil
}

func (r runtimeDebugSettingsRepo) GetAll() (map[string]string, error) {
	out := make(map[string]string, len(r.values))
	for key, value := range r.values {
		out[key] = value
	}
	return out, nil
}

func TestRuntimeDebugSettingsSummaryIncludesEffectivePollingConfig(t *testing.T) {
	cfg := &runtimeConfig{
		DBDriver:   "postgres",
		ListenAddr: ":8080",
		LogLevel:   "debug",
	}
	repo := runtimeDebugSettingsRepo{values: map[string]string{
		domain.SettingPollingInterval:            "30",
		domain.SettingSNMPWorkerPoolPerformance:  "32",
		domain.SettingSNMPWorkerPoolOperational:  "16",
		domain.SettingSNMPWorkerPoolStatic:       "6",
		domain.SettingPollingMaxWorkersPerDevice: "2",
		domain.SettingSNMPTimeout:                "8",
		domain.SettingSNMPRetries:                "1",
		domain.SettingPollingWebSocketCoalesceMS: "250",
		domain.SettingPrometheusURL:              "http://prometheus.internal:9090",
	}}

	summary := runtimeDebugSettingsSummary(cfg, repo)

	for _, want := range []string{
		"log_level=debug",
		"db_driver=postgres",
		"listen=:8080",
		"polling_interval_seconds=30",
		"pool_performance=32",
		"pool_operational=16",
		"pool_static=6",
		"polling_max_workers_per_device=2",
		"snmp_timeout_seconds=8",
		"snmp_retries=1",
		"websocket_coalesce_ms=250",
		"prometheus_configured=true",
	} {
		if !strings.Contains(summary, want) {
			t.Fatalf("summary missing %s: %q", want, summary)
		}
	}
	if strings.Contains(summary, "prometheus.internal") {
		t.Fatalf("summary leaked Prometheus URL: %q", summary)
	}
}

func TestRuntimeBootstrapRunWrapsLoadConfigError(t *testing.T) {
	bootstrap := &runtimeBootstrap{}
	original := loadRuntimeConfig
	loadRuntimeConfig = func(path string) (*runtimeConfig, error) {
		return nil, errors.New("boom")
	}
	t.Cleanup(func() { loadRuntimeConfig = original })

	err := bootstrap.Run("/tmp/theia.yaml")
	if err == nil {
		t.Fatal("Run() error = nil, want wrapped load config error")
	}
	if got, want := err.Error(), "load config: boom"; got != want {
		t.Fatalf("Run() error = %q, want %q", got, want)
	}
}

func TestRuntimeBootstrapStopRuntimeStopsChildrenInReverseOrder(t *testing.T) {
	bootstrap := &runtimeBootstrap{}
	var order []string
	children := runtimeChildren{
		stubRuntimeStopper{name: "pipeline", stops: &order},
		stubRuntimeStopper{name: "instance-backups", stops: &order},
		stubRuntimeStopper{name: "device-backups", stops: &order},
	}

	bootstrap.stopRuntime(children)

	if got, want := fmt.Sprint(order), "[device-backups instance-backups pipeline]"; got != want {
		t.Fatalf("stop order = %s, want %s", got, want)
	}
}

func TestRuntimeBootstrapServeTreatsServerClosedAsSuccess(t *testing.T) {
	bootstrap := &runtimeBootstrap{}

	if err := bootstrap.serve(stubRuntimeServer{listenErr: http.ErrServerClosed}); err != nil {
		t.Fatalf("serve() error = %v, want nil", err)
	}
	if err := bootstrap.serve(stubRuntimeServer{}); err != nil {
		t.Fatalf("serve() unexpected error = %v", err)
	}

	boom := errors.New("boom")
	if err := bootstrap.serve(stubRuntimeServer{listenErr: boom}); !reflect.DeepEqual(err, boom) {
		t.Fatalf("serve() error = %v, want %v", err, boom)
	}
}

func TestRuntimeBootstrapRunHardensBackupDirForPostgres(t *testing.T) {
	bootstrap := &runtimeBootstrap{}
	runtimeDir := t.TempDir()
	backupTarget := filepath.Join(runtimeDir, "backup-target")
	if err := os.Mkdir(backupTarget, 0o700); err != nil {
		t.Fatalf("Mkdir(backupTarget): %v", err)
	}
	backupLink := filepath.Join(runtimeDir, "backup-link")
	if err := os.Symlink(backupTarget, backupLink); err != nil {
		t.Fatalf("Symlink(backupLink): %v", err)
	}

	originalLoadRuntimeConfig := loadRuntimeConfig
	loadRuntimeConfig = func(path string) (*runtimeConfig, error) {
		return &runtimeConfig{
			DBDriver:   "postgres",
			DBDSN:      "postgres://user:pass@127.0.0.1:1/theia?sslmode=disable",
			DBPath:     filepath.Join(runtimeDir, "theia.db"),
			DataDir:    runtimeDir,
			ListenAddr: ":0",
			LogLevel:   "info",
		}, nil
	}
	t.Cleanup(func() { loadRuntimeConfig = originalLoadRuntimeConfig })

	originalBackupDir, hadBackupDir := os.LookupEnv("THEIA_BACKUP_DIR")
	if err := os.Setenv("THEIA_BACKUP_DIR", backupLink); err != nil {
		t.Fatalf("Setenv(THEIA_BACKUP_DIR): %v", err)
	}
	t.Cleanup(func() {
		if hadBackupDir {
			_ = os.Setenv("THEIA_BACKUP_DIR", originalBackupDir)
		} else {
			_ = os.Unsetenv("THEIA_BACKUP_DIR")
		}
	})

	err := bootstrap.Run("/tmp/theia.yaml")
	if err == nil {
		t.Fatal("Run() error = nil, want backup directory hardening error")
	}
	if got, want := err.Error(), fmt.Sprintf("prepare backup directory %s: ensure private dir: path is a symlink", backupLink); got != want {
		t.Fatalf("Run() error = %q, want %q", got, want)
	}
}

func TestRuntimeBootstrapRunTightensExistingKnownHostsFile(t *testing.T) {
	bootstrap := &runtimeBootstrap{}
	runtimeDir := t.TempDir()
	knownHostsPath := filepath.Join(runtimeDir, "known_hosts")
	if err := os.WriteFile(knownHostsPath, []byte("127.0.0.1 ssh-ed25519 AAAA\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(known_hosts): %v", err)
	}

	originalLoadRuntimeConfig := loadRuntimeConfig
	loadRuntimeConfig = func(path string) (*runtimeConfig, error) {
		return &runtimeConfig{
			DBDriver:   "postgres",
			DBDSN:      "postgres://user:pass@127.0.0.1:1/theia?sslmode=disable",
			DBPath:     filepath.Join(runtimeDir, "theia.db"),
			DataDir:    runtimeDir,
			ListenAddr: ":0",
			LogLevel:   "info",
		}, nil
	}
	t.Cleanup(func() { loadRuntimeConfig = originalLoadRuntimeConfig })

	err := bootstrap.Run("/tmp/theia.yaml")
	if err == nil {
		t.Fatal("Run() error = nil, want database connect error")
	}
	if !strings.Contains(err.Error(), "connect to database") {
		t.Fatalf("Run() error = %q, want connect wrapper", err.Error())
	}
	if !strings.Contains(err.Error(), "THEIA_DB_DSN") {
		t.Fatalf("Run() error = %q, want postgres recovery hint", err.Error())
	}

	info, statErr := os.Stat(knownHostsPath)
	if statErr != nil {
		t.Fatalf("Stat(known_hosts): %v", statErr)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Fatalf("known_hosts mode = %04o, want %04o", got, 0o600)
	}
}

func TestRuntimeBootstrapRunRejectsSQLiteWithoutExplicitOptIn(t *testing.T) {
	bootstrap := &runtimeBootstrap{}
	runtimeDir := t.TempDir()

	originalLoadRuntimeConfig := loadRuntimeConfig
	loadRuntimeConfig = func(path string) (*runtimeConfig, error) {
		return &runtimeConfig{
			DBDriver:   "sqlite",
			DBPath:     filepath.Join(runtimeDir, "theia.db"),
			DataDir:    runtimeDir,
			ListenAddr: ":0",
			LogLevel:   "info",
		}, nil
	}
	t.Cleanup(func() { loadRuntimeConfig = originalLoadRuntimeConfig })

	t.Setenv("THEIA_ALLOW_SQLITE_SMALL_INSTALL", "")

	err := bootstrap.Run("/tmp/theia.yaml")
	if err == nil {
		t.Fatal("Run() error = nil, want sqlite policy rejection")
	}
	if got := err.Error(); !strings.Contains(got, "sqlite is only supported for demo, lab, or small-install deployments") {
		t.Fatalf("Run() error = %q, want sqlite policy guidance", got)
	}
	if got := err.Error(); !strings.Contains(got, "THEIA_ALLOW_SQLITE_SMALL_INSTALL=true") {
		t.Fatalf("Run() error = %q, want sqlite opt-in hint", got)
	}
}

func TestRuntimeBootstrapRunRejectsMissingPostgresDSNWithGuidance(t *testing.T) {
	bootstrap := &runtimeBootstrap{}
	runtimeDir := t.TempDir()

	originalLoadRuntimeConfig := loadRuntimeConfig
	loadRuntimeConfig = func(path string) (*runtimeConfig, error) {
		return &runtimeConfig{
			DBDriver:   "postgres",
			DBPath:     filepath.Join(runtimeDir, "theia.db"),
			DataDir:    runtimeDir,
			ListenAddr: ":0",
			LogLevel:   "info",
		}, nil
	}
	t.Cleanup(func() { loadRuntimeConfig = originalLoadRuntimeConfig })

	err := bootstrap.Run("/tmp/theia.yaml")
	if err == nil {
		t.Fatal("Run() error = nil, want postgres DSN error")
	}
	if got := err.Error(); !strings.Contains(got, "postgres is the default database driver and requires db_dsn") {
		t.Fatalf("Run() error = %q, want missing DSN guidance", got)
	}
	if got := err.Error(); !strings.Contains(got, "make migrate-postgres") {
		t.Fatalf("Run() error = %q, want migration hint", got)
	}
}

func TestRuntimeBootstrapRunTreatsBlankDriverAsPostgresDefault(t *testing.T) {
	bootstrap := &runtimeBootstrap{}
	runtimeDir := t.TempDir()

	originalLoadRuntimeConfig := loadRuntimeConfig
	loadRuntimeConfig = func(path string) (*runtimeConfig, error) {
		return &runtimeConfig{
			DBDriver:   "   ",
			DBPath:     filepath.Join(runtimeDir, "theia.db"),
			DataDir:    runtimeDir,
			ListenAddr: ":0",
			LogLevel:   "info",
		}, nil
	}
	t.Cleanup(func() { loadRuntimeConfig = originalLoadRuntimeConfig })

	err := bootstrap.Run("/tmp/theia.yaml")
	if err == nil {
		t.Fatal("Run() error = nil, want postgres DSN error for blank driver")
	}
	if got := err.Error(); !strings.Contains(got, "postgres is the default database driver and requires db_dsn") {
		t.Fatalf("Run() error = %q, want missing DSN guidance", got)
	}
	if got := err.Error(); !strings.Contains(got, "THEIA_DB_DSN") {
		t.Fatalf("Run() error = %q, want postgres recovery hint", got)
	}
}

func TestValidateDeploymentSecretPolicyRejectsUnsafeProductionAndStagingSecrets(t *testing.T) {
	tests := []struct {
		name string
		cfg  *runtimeConfig
		env  map[string]string
		want []string
	}{
		{
			name: "production missing encryption key",
			cfg:  &runtimeConfig{DeploymentEnv: "production", DBDriver: "postgres", DBDSN: "postgres://theia:strong-password@postgres:5432/theia?sslmode=disable"},
			env:  map[string]string{"THEIA_ENCRYPTION_KEY": "", "POSTGRES_PASSWORD": "strong-password"},
			want: []string{"THEIA_ENCRYPTION_KEY", "required"},
		},
		{
			name: "staging rejects example encryption key",
			cfg:  &runtimeConfig{DeploymentEnv: "staging", DBDriver: "postgres", DBDSN: "postgres://theia:strong-password@postgres:5432/theia?sslmode=disable"},
			env:  map[string]string{"THEIA_ENCRYPTION_KEY": "change-me", "POSTGRES_PASSWORD": "strong-password"},
			want: []string{"THEIA_ENCRYPTION_KEY", "example"},
		},
		{
			name: "staging missing encryption key",
			cfg:  &runtimeConfig{DeploymentEnv: "staging", DBDriver: "postgres", DBDSN: "postgres://theia:strong-password@postgres:5432/theia?sslmode=disable"},
			env:  map[string]string{"THEIA_ENCRYPTION_KEY": "", "POSTGRES_PASSWORD": "strong-password"},
			want: []string{"THEIA_ENCRYPTION_KEY", "required"},
		},
		{
			name: "production rejects example db dsn password",
			cfg:  &runtimeConfig{DeploymentEnv: "production", DBDriver: "postgres", DBDSN: "postgres://theia:change-me@postgres:5432/theia?sslmode=disable"},
			env:  map[string]string{"THEIA_ENCRYPTION_KEY": "strong-encryption-key", "POSTGRES_PASSWORD": "strong-password"},
			want: []string{"THEIA_DB_DSN", "example"},
		},
		{
			name: "production rejects example db dsn password after postgres driver normalization",
			cfg:  &runtimeConfig{DeploymentEnv: "production", DBDriver: " postgresql ", DBDSN: "postgres://theia:change-me@postgres:5432/theia?sslmode=disable"},
			env:  map[string]string{"THEIA_ENCRYPTION_KEY": "strong-encryption-key", "POSTGRES_PASSWORD": "strong-password"},
			want: []string{"THEIA_DB_DSN", "example"},
		},
		{
			name: "production rejects example db dsn keyword password",
			cfg:  &runtimeConfig{DeploymentEnv: "production", DBDriver: "postgres", DBDSN: "host=postgres user=theia password='change-me' dbname=theia sslmode=disable"},
			env:  map[string]string{"THEIA_ENCRYPTION_KEY": "strong-encryption-key", "POSTGRES_PASSWORD": "strong-password"},
			want: []string{"THEIA_DB_DSN", "example"},
		},
		{
			name: "production rejects example db dsn url query password",
			cfg:  &runtimeConfig{DeploymentEnv: "production", DBDriver: "postgres", DBDSN: "postgres://theia@postgres:5432/theia?password=change-me&sslmode=disable"},
			env:  map[string]string{"THEIA_ENCRYPTION_KEY": "strong-encryption-key", "POSTGRES_PASSWORD": "strong-password"},
			want: []string{"THEIA_DB_DSN", "example"},
		},
		{
			name: "staging rejects example db dsn password",
			cfg:  &runtimeConfig{DeploymentEnv: "staging", DBDriver: "postgres", DBDSN: "host=postgres user=theia password=change-me dbname=theia sslmode=disable"},
			env:  map[string]string{"THEIA_ENCRYPTION_KEY": "strong-encryption-key", "POSTGRES_PASSWORD": "strong-password"},
			want: []string{"THEIA_DB_DSN", "example"},
		},
		{
			name: "production rejects postgres password env placeholder",
			cfg:  &runtimeConfig{DeploymentEnv: "production", DBDriver: "postgres", DBDSN: "postgres://theia:strong-password@postgres:5432/theia?sslmode=disable"},
			env:  map[string]string{"THEIA_ENCRYPTION_KEY": "strong-encryption-key", "POSTGRES_PASSWORD": "change-me"},
			want: []string{"POSTGRES_PASSWORD", "example"},
		},
		{
			name: "staging rejects postgres password env placeholder",
			cfg:  &runtimeConfig{DeploymentEnv: "staging", DBDriver: "postgres", DBDSN: "postgres://theia:strong-password@postgres:5432/theia?sslmode=disable"},
			env:  map[string]string{"THEIA_ENCRYPTION_KEY": "strong-encryption-key", "POSTGRES_PASSWORD": "change-me"},
			want: []string{"POSTGRES_PASSWORD", "example"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for key, value := range tt.env {
				t.Setenv(key, value)
			}
			err := validateDeploymentSecretPolicy(tt.cfg)
			if err == nil {
				t.Fatal("validateDeploymentSecretPolicy() error = nil, want rejection")
			}
			for _, want := range tt.want {
				if !strings.Contains(err.Error(), want) {
					t.Fatalf("error = %q, want %q", err.Error(), want)
				}
			}
		})
	}
}

func TestValidateDeploymentSecretPolicyAllowsDevelopmentBlankAndSafeProductionSecrets(t *testing.T) {
	tests := []struct {
		name string
		cfg  *runtimeConfig
		env  map[string]string
	}{
		{
			name: "blank deployment env does not enforce secret policy",
			cfg:  &runtimeConfig{DeploymentEnv: "", DBDriver: "postgres", DBDSN: "postgres://theia:change-me@postgres:5432/theia?sslmode=disable"},
			env:  map[string]string{"THEIA_ENCRYPTION_KEY": "", "POSTGRES_PASSWORD": "change-me"},
		},
		{
			name: "development deployment env does not enforce secret policy",
			cfg:  &runtimeConfig{DeploymentEnv: "development", DBDriver: "postgres", DBDSN: "postgres://theia:change-me@postgres:5432/theia?sslmode=disable"},
			env:  map[string]string{"THEIA_ENCRYPTION_KEY": "change-me", "POSTGRES_PASSWORD": "change-me"},
		},
		{
			name: "production accepts non-placeholder secrets",
			cfg:  &runtimeConfig{DeploymentEnv: "production", DBDriver: "postgres", DBDSN: "postgres://theia:strong-password@postgres:5432/theia?sslmode=disable"},
			env:  map[string]string{"THEIA_ENCRYPTION_KEY": "strong-encryption-key", "POSTGRES_PASSWORD": "another-strong-password"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for key, value := range tt.env {
				t.Setenv(key, value)
			}
			if err := validateDeploymentSecretPolicy(tt.cfg); err != nil {
				t.Fatalf("validateDeploymentSecretPolicy() error = %v, want nil", err)
			}
		})
	}
}

func TestRuntimeBootstrapRunWrapsPostgresConnectFailureWithGuidance(t *testing.T) {
	bootstrap := &runtimeBootstrap{}
	runtimeDir := t.TempDir()

	originalLoadRuntimeConfig := loadRuntimeConfig
	loadRuntimeConfig = func(path string) (*runtimeConfig, error) {
		return &runtimeConfig{
			DBDriver:   "postgres",
			DBDSN:      "postgres://user:pass@127.0.0.1:1/theia?sslmode=disable",
			DBPath:     filepath.Join(runtimeDir, "theia.db"),
			DataDir:    runtimeDir,
			ListenAddr: ":0",
			LogLevel:   "info",
		}, nil
	}
	t.Cleanup(func() { loadRuntimeConfig = originalLoadRuntimeConfig })

	err := bootstrap.Run("/tmp/theia.yaml")
	if err == nil {
		t.Fatal("Run() error = nil, want postgres connection error")
	}
	if got := err.Error(); !strings.Contains(got, "connect to database") {
		t.Fatalf("Run() error = %q, want connect wrapper", got)
	}
	if got := err.Error(); !strings.Contains(got, "set THEIA_DB_DSN") {
		t.Fatalf("Run() error = %q, want postgres recovery hint", got)
	}
}

func TestRuntimeBootstrapRunWrapsPostgresOpenFailureWithGuidance(t *testing.T) {
	bootstrap := &runtimeBootstrap{}
	runtimeDir := t.TempDir()

	originalLoadRuntimeConfig := loadRuntimeConfig
	loadRuntimeConfig = func(path string) (*runtimeConfig, error) {
		return &runtimeConfig{
			DBDriver:   "postgres",
			DBDSN:      "postgres://user:%zz@127.0.0.1:5432/theia?sslmode=disable",
			DBPath:     filepath.Join(runtimeDir, "theia.db"),
			DataDir:    runtimeDir,
			ListenAddr: ":0",
			LogLevel:   "info",
		}, nil
	}
	t.Cleanup(func() { loadRuntimeConfig = originalLoadRuntimeConfig })

	originalOpenPrimaryDB := openPrimaryRuntimeDB
	openPrimaryRuntimeDB = func(driver, path, dsn string) (*sql.DB, sqlite.Dialect, error) {
		return nil, "", errors.New("boom")
	}
	t.Cleanup(func() { openPrimaryRuntimeDB = originalOpenPrimaryDB })

	err := bootstrap.Run("/tmp/theia.yaml")
	if err == nil {
		t.Fatal("Run() error = nil, want postgres open error")
	}
	if got := err.Error(); !strings.Contains(got, "open database") {
		t.Fatalf("Run() error = %q, want open wrapper", got)
	}
	if got := err.Error(); !strings.Contains(got, "set THEIA_DB_DSN") {
		t.Fatalf("Run() error = %q, want postgres recovery hint", got)
	}
}
