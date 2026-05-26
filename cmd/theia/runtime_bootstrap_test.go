package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/lollinoo/theia/internal/domain"
	"github.com/lollinoo/theia/internal/service"
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
		"log_level=debug", "listen=:8080",
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

func TestProductionStagingConfigSurfacesDoNotShipSecretDefaults(t *testing.T) {
	repoRoot := filepath.Clean(filepath.Join("..", ".."))
	surfaces := []struct {
		path                 string
		deploymentEnv        string
		requireBlankEnvValue bool
		requirePostgresEnv   bool
		rejectConcreteDSN    bool
	}{
		{path: ".env.prod.example", deploymentEnv: "production", requireBlankEnvValue: true, rejectConcreteDSN: true},
		{path: ".env.staging.example", deploymentEnv: "staging", requireBlankEnvValue: true, rejectConcreteDSN: true},
		{path: "docker-compose.prod.yml", deploymentEnv: "production", requirePostgresEnv: true},
		{path: "docker-compose.staging.yml", deploymentEnv: "staging", requirePostgresEnv: true},
		{path: "Makefile"},
		{path: "SETUP.md", rejectConcreteDSN: true},
		{path: "config.example.yaml", rejectConcreteDSN: true},
		{path: "cmd/theia/runtime_bootstrap.go", rejectConcreteDSN: true},
	}
	unsafeFragments := []struct {
		name  string
		value string
	}{
		{name: "overrideable deployment environment", value: "THEIA_DEPLOYMENT_ENV=${THEIA_DEPLOYMENT_ENV:-"},
		{name: "placeholder PostgreSQL password", value: "POSTGRES_PASSWORD=change-me"},
		{name: "concrete PostgreSQL DSN example", value: "THEIA_DB_DSN=postgres://"},
		{name: "placeholder PostgreSQL DSN password", value: "THEIA_DB_DSN=postgres://theia:change-me@"},
		{name: "PostgreSQL password fallback", value: "POSTGRES_PASSWORD:-"},
		{name: "PostgreSQL DSN fallback", value: "THEIA_DB_DSN:-postgres://"},
	}
	concreteDSNFragments := []struct {
		name  string
		value string
	}{
		{name: "concrete local PostgreSQL DSN example", value: "postgres://theia:theia@"},
		{name: "concrete yaml PostgreSQL DSN example", value: "db_dsn: \"postgres://"},
	}

	if isTracked, err := gitPathIsTracked(repoRoot, "config.yaml"); err != nil {
		t.Fatalf("check tracked config.yaml: %v", err)
	} else if isTracked {
		t.Error("config.yaml must not be tracked because local config files may contain secrets")
	}
	gitignoreBytes, err := os.ReadFile(filepath.Join(repoRoot, ".gitignore"))
	if err != nil {
		t.Fatalf("ReadFile(.gitignore): %v", err)
	}
	if !gitignoreHasPattern(string(gitignoreBytes), "config.yaml") {
		t.Error(".gitignore must ignore config.yaml")
	}

	for _, surface := range surfaces {
		t.Run(surface.path, func(t *testing.T) {
			contentBytes, err := os.ReadFile(filepath.Join(repoRoot, surface.path))
			if err != nil {
				t.Fatalf("ReadFile(%s): %v", surface.path, err)
			}
			content := string(contentBytes)

			if surface.deploymentEnv != "" && !surfaceContainsDeploymentEnv(content, surface.deploymentEnv) {
				t.Errorf("%s must set THEIA_DEPLOYMENT_ENV for %s validation", surface.path, surface.deploymentEnv)
			}
			for _, unsafe := range unsafeFragments {
				if strings.Contains(content, unsafe.value) {
					t.Errorf("%s contains unsafe %s fragment %q", surface.path, unsafe.name, unsafe.value)
				}
			}
			if surface.rejectConcreteDSN {
				for _, unsafe := range concreteDSNFragments {
					if strings.Contains(content, unsafe.value) {
						t.Errorf("%s contains unsafe %s fragment %q", surface.path, unsafe.name, unsafe.value)
					}
				}
			}
			if surface.requireBlankEnvValue {
				for _, key := range []string{"THEIA_DB_DSN", "POSTGRES_PASSWORD", "THEIA_SESSION_SECRET", "THEIA_METRICS_TOKEN"} {
					value, ok := envExampleAssignment(content, key)
					if !ok {
						t.Errorf("%s must include %s=", surface.path, key)
						continue
					}
					if value != "" {
						t.Errorf("%s must leave %s blank in the tracked example", surface.path, key)
					}
				}
			}
			if surface.requirePostgresEnv {
				const postgresPasswordRequirement = "- POSTGRES_PASSWORD=${POSTGRES_PASSWORD:?POSTGRES_PASSWORD must be set}"
				if got := countActiveLines(content, postgresPasswordRequirement); got < 2 {
					t.Errorf("%s must pass POSTGRES_PASSWORD to both backend and postgres service; found %d active entries", surface.path, got)
				}
				const sessionSecretRequirement = "- THEIA_SESSION_SECRET=${THEIA_SESSION_SECRET:?THEIA_SESSION_SECRET must be set}"
				if got := countActiveLines(content, sessionSecretRequirement); got != 1 {
					t.Errorf("%s must pass required THEIA_SESSION_SECRET to backend; found %d active entries", surface.path, got)
				}
				const metricsTokenRequirement = "- THEIA_METRICS_TOKEN=${THEIA_METRICS_TOKEN:?THEIA_METRICS_TOKEN must be set}"
				if got := countActiveLines(content, metricsTokenRequirement); got != 1 {
					t.Errorf("%s must pass required THEIA_METRICS_TOKEN to backend; found %d active entries", surface.path, got)
				}
			}
		})
	}
}

func TestSessionSecretDocumentationMatchesRuntimeRequirement(t *testing.T) {
	repoRoot := filepath.Clean(filepath.Join("..", ".."))
	docs := []string{"config.example.yaml", "SETUP.md"}
	stalePhrases := []string{
		"Required for staging and production",
		"required for staging/production runtime startup",
	}
	wantPhrase := "Required whenever the backend initializes first-party password auth"

	for _, doc := range docs {
		t.Run(doc, func(t *testing.T) {
			contentBytes, err := os.ReadFile(filepath.Join(repoRoot, doc))
			if err != nil {
				t.Fatalf("ReadFile(%s): %v", doc, err)
			}
			content := string(contentBytes)
			for _, stale := range stalePhrases {
				if strings.Contains(content, stale) {
					t.Fatalf("%s contains stale session_secret requirement wording %q", doc, stale)
				}
			}
			if !strings.Contains(content, wantPhrase) {
				t.Fatalf("%s must document session_secret as a runtime auth requirement", doc)
			}
		})
	}
}

func TestSetupRequiredOperatorInputsIncludeMetricsToken(t *testing.T) {
	repoRoot := filepath.Clean(filepath.Join("..", ".."))
	contentBytes, err := os.ReadFile(filepath.Join(repoRoot, "SETUP.md"))
	if err != nil {
		t.Fatalf("ReadFile(SETUP.md): %v", err)
	}
	content := string(contentBytes)
	requiredKeys := []string{
		"THEIA_ENCRYPTION_KEY",
		"THEIA_SESSION_SECRET",
		"THEIA_METRICS_TOKEN",
		"THEIA_DB_DSN",
		"POSTGRES_PASSWORD",
	}

	for _, heading := range []string{"## Production Environment", "## Staging Environment"} {
		t.Run(heading, func(t *testing.T) {
			block := markdownBlockBetween(content, heading, "For bundled PostgreSQL")
			for _, key := range requiredKeys {
				if !strings.Contains(block, "- `"+key) {
					t.Errorf("SETUP.md %s required operator inputs missing %s", heading, key)
				}
			}
		})
	}
}

func markdownBlockBetween(content, start, end string) string {
	startIndex := strings.Index(content, start)
	if startIndex < 0 {
		return ""
	}
	block := content[startIndex:]
	endIndex := strings.Index(block, end)
	if endIndex < 0 {
		return block
	}
	return block[:endIndex]
}

func gitPathIsTracked(repoRoot, path string) (bool, error) {
	cmd := exec.Command("git", "ls-files", "--error-unmatch", path)
	cmd.Dir = repoRoot
	err := cmd.Run()
	if err == nil {
		return true, nil
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return false, nil
	}
	return false, err
}

func gitignoreHasPattern(content, pattern string) bool {
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		if line == pattern {
			return true
		}
	}
	return false
}

func countActiveLines(content, want string) int {
	count := 0
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "#") {
			continue
		}
		if line == want {
			count++
		}
	}
	return count
}

func surfaceContainsDeploymentEnv(content, deploymentEnv string) bool {
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "#") {
			continue
		}
		if line == "THEIA_DEPLOYMENT_ENV="+deploymentEnv || line == "- THEIA_DEPLOYMENT_ENV="+deploymentEnv {
			return true
		}
	}
	return false
}

func envExampleAssignment(content, key string) (string, bool) {
	prefix := key + "="
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, prefix) {
			return strings.TrimSpace(strings.TrimPrefix(line, prefix)), true
		}
	}
	return "", false
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

func TestConfigureInstanceBackupArchiveLimitsUsesRuntimeConfig(t *testing.T) {
	cfg := &runtimeConfig{}
	cfg.RestoreArchiveLimits.MaxCompressedBytes = 101
	cfg.RestoreArchiveLimits.MaxTotalBytes = 202
	cfg.RestoreArchiveLimits.MaxEntryBytes = 303
	cfg.RestoreArchiveLimits.MaxFileEntries = 4
	cfg.InstanceBackupArchiveLimits.MaxTotalBytes = 505
	cfg.InstanceBackupArchiveLimits.MaxEntryBytes = 606
	cfg.InstanceBackupArchiveLimits.MaxFileEntries = 7
	cfg.InstanceBackupArchiveLimits.MaxDurationSeconds = 8

	svc := service.NewInstanceBackupService(nil, nil, nil, "", "", "", "", "", nil)
	configureInstanceBackupArchiveLimits(svc, cfg)

	restoreLimits := svc.RestoreArchiveLimits()
	if restoreLimits.MaxCompressedBytes != 101 ||
		restoreLimits.MaxTotalBytes != 202 ||
		restoreLimits.MaxEntryBytes != 303 ||
		restoreLimits.MaxFileEntries != 4 {
		t.Fatalf("restore limits = %#v, want runtime config values", restoreLimits)
	}
	backupLimits := svc.BackupArchiveLimits()
	if backupLimits.MaxTotalBytes != 505 ||
		backupLimits.MaxEntryBytes != 606 ||
		backupLimits.MaxFileEntries != 7 ||
		backupLimits.MaxDuration != 8*time.Second {
		t.Fatalf("backup limits = %#v, want runtime config values", backupLimits)
	}
}

func TestRuntimeBootstrapRunRejectsUnsafeProductionSecretsBeforeOpeningDatabase(t *testing.T) {
	bootstrap := &runtimeBootstrap{}
	runtimeDir := t.TempDir()

	originalLoadRuntimeConfig := loadRuntimeConfig
	loadRuntimeConfig = func(path string) (*runtimeConfig, error) {
		return &runtimeConfig{
			DeploymentEnv: "production",
			DBDSN:         "postgres://theia:strong-password@postgres:5432/theia?sslmode=disable",
			DataDir:       runtimeDir,
			ListenAddr:    ":0",
			LogLevel:      "info",
		}, nil
	}
	t.Cleanup(func() { loadRuntimeConfig = originalLoadRuntimeConfig })

	openCalled := false
	originalOpenPrimaryDB := openPrimaryRuntimeDB
	openPrimaryRuntimeDB = func(dsn string) (*sql.DB, error) {
		openCalled = true
		return nil, errors.New("open should not be called")
	}
	t.Cleanup(func() { openPrimaryRuntimeDB = originalOpenPrimaryDB })

	t.Setenv("THEIA_ENCRYPTION_KEY", "change-me")
	t.Setenv("POSTGRES_PASSWORD", "strong-password")

	err := bootstrap.Run("/tmp/theia.yaml")
	if err == nil {
		t.Fatal("Run() error = nil, want unsafe secret rejection")
	}
	if got := err.Error(); !strings.Contains(got, "THEIA_ENCRYPTION_KEY") || !strings.Contains(got, "example") {
		t.Fatalf("Run() error = %q, want encryption key example rejection", got)
	}
	if openCalled {
		t.Fatal("openPrimaryRuntimeDB was called before unsafe secret rejection")
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
	createRuntimeTestSymlink(t, backupTarget, backupLink)

	originalLoadRuntimeConfig := loadRuntimeConfig
	loadRuntimeConfig = func(path string) (*runtimeConfig, error) {
		return &runtimeConfig{
			DBDSN:      "postgres://user:pass@127.0.0.1:1/theia?sslmode=disable",
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
			DBDSN:      "postgres://user:pass@127.0.0.1:1/theia?sslmode=disable",
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
	assertRuntimePathMode(t, "known_hosts", info.Mode().Perm(), 0o600)
}

func TestRuntimeBootstrapRunRejectsMissingPostgresDSNWithGuidance(t *testing.T) {
	bootstrap := &runtimeBootstrap{}
	runtimeDir := t.TempDir()

	originalLoadRuntimeConfig := loadRuntimeConfig
	loadRuntimeConfig = func(path string) (*runtimeConfig, error) {
		return &runtimeConfig{
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
	if got := err.Error(); !strings.Contains(got, "postgres is the required database and needs db_dsn") {
		t.Fatalf("Run() error = %q, want missing DSN guidance", got)
	}
}

func TestRuntimeBootstrapRunTreatsBlankDriverAsPostgresDefault(t *testing.T) {
	bootstrap := &runtimeBootstrap{}
	runtimeDir := t.TempDir()

	originalLoadRuntimeConfig := loadRuntimeConfig
	loadRuntimeConfig = func(path string) (*runtimeConfig, error) {
		return &runtimeConfig{
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
	if got := err.Error(); !strings.Contains(got, "postgres is the required database and needs db_dsn") {
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
			cfg:  &runtimeConfig{DeploymentEnv: "production", DBDSN: "postgres://theia:strong-password@postgres:5432/theia?sslmode=disable"},
			env:  map[string]string{"THEIA_ENCRYPTION_KEY": "", "POSTGRES_PASSWORD": "strong-password"},
			want: []string{"THEIA_ENCRYPTION_KEY", "required"},
		},
		{
			name: "production rejects example encryption key",
			cfg:  &runtimeConfig{DeploymentEnv: "production", DBDSN: "postgres://theia:strong-password@postgres:5432/theia?sslmode=disable"},
			env:  map[string]string{"THEIA_ENCRYPTION_KEY": "change-me", "POSTGRES_PASSWORD": "strong-password"},
			want: []string{"THEIA_ENCRYPTION_KEY", "example"},
		},
		{
			name: "staging rejects example encryption key",
			cfg:  &runtimeConfig{DeploymentEnv: "staging", DBDSN: "postgres://theia:strong-password@postgres:5432/theia?sslmode=disable"},
			env:  map[string]string{"THEIA_ENCRYPTION_KEY": "change-me", "POSTGRES_PASSWORD": "strong-password"},
			want: []string{"THEIA_ENCRYPTION_KEY", "example"},
		},
		{
			name: "staging missing encryption key",
			cfg:  &runtimeConfig{DeploymentEnv: "staging", DBDSN: "postgres://theia:strong-password@postgres:5432/theia?sslmode=disable"},
			env:  map[string]string{"THEIA_ENCRYPTION_KEY": "", "POSTGRES_PASSWORD": "strong-password"},
			want: []string{"THEIA_ENCRYPTION_KEY", "required"},
		},
		{
			name: "production rejects example db dsn password",
			cfg:  &runtimeConfig{DeploymentEnv: "production", DBDSN: "postgres://theia:change-me@postgres:5432/theia?sslmode=disable"},
			env:  map[string]string{"THEIA_ENCRYPTION_KEY": "strong-encryption-key", "POSTGRES_PASSWORD": "strong-password"},
			want: []string{"THEIA_DB_DSN", "example"},
		},
		{
			name: "production rejects example db dsn password after postgres driver normalization",
			cfg:  &runtimeConfig{DeploymentEnv: "production", DBDSN: "postgres://theia:change-me@postgres:5432/theia?sslmode=disable"},
			env:  map[string]string{"THEIA_ENCRYPTION_KEY": "strong-encryption-key", "POSTGRES_PASSWORD": "strong-password"},
			want: []string{"THEIA_DB_DSN", "example"},
		},
		{
			name: "production rejects example db dsn keyword password",
			cfg:  &runtimeConfig{DeploymentEnv: "production", DBDSN: "host=postgres user=theia password='change-me' dbname=theia sslmode=disable"},
			env:  map[string]string{"THEIA_ENCRYPTION_KEY": "strong-encryption-key", "POSTGRES_PASSWORD": "strong-password"},
			want: []string{"THEIA_DB_DSN", "example"},
		},
		{
			name: "production rejects duplicate keyword dsn placeholder password",
			cfg:  &runtimeConfig{DeploymentEnv: "production", DBDSN: "host=postgres user=theia password=strong-password password=change-me dbname=theia sslmode=disable"},
			env:  map[string]string{"THEIA_ENCRYPTION_KEY": "strong-encryption-key", "POSTGRES_PASSWORD": "strong-password"},
			want: []string{"THEIA_DB_DSN", "example"},
		},
		{
			name: "production rejects example db dsn url query password",
			cfg:  &runtimeConfig{DeploymentEnv: "production", DBDSN: "postgres://theia@postgres:5432/theia?password=change-me&sslmode=disable"},
			env:  map[string]string{"THEIA_ENCRYPTION_KEY": "strong-encryption-key", "POSTGRES_PASSWORD": "strong-password"},
			want: []string{"THEIA_DB_DSN", "example"},
		},
		{
			name: "staging rejects example db dsn password",
			cfg:  &runtimeConfig{DeploymentEnv: "staging", DBDSN: "host=postgres user=theia password=change-me dbname=theia sslmode=disable"},
			env:  map[string]string{"THEIA_ENCRYPTION_KEY": "strong-encryption-key", "POSTGRES_PASSWORD": "strong-password"},
			want: []string{"THEIA_DB_DSN", "example"},
		},
		{
			name: "production rejects postgres password env placeholder",
			cfg:  &runtimeConfig{DeploymentEnv: "production", DBDSN: "postgres://theia:strong-password@postgres:5432/theia?sslmode=disable"},
			env:  map[string]string{"THEIA_ENCRYPTION_KEY": "strong-encryption-key", "POSTGRES_PASSWORD": "change-me"},
			want: []string{"POSTGRES_PASSWORD", "example"},
		},
		{
			name: "staging rejects postgres password env placeholder",
			cfg:  &runtimeConfig{DeploymentEnv: "staging", DBDSN: "postgres://theia:strong-password@postgres:5432/theia?sslmode=disable"},
			env:  map[string]string{"THEIA_ENCRYPTION_KEY": "strong-encryption-key", "POSTGRES_PASSWORD": "change-me"},
			want: []string{"POSTGRES_PASSWORD", "example"},
		},
		{
			name: "production requires session secret",
			cfg:  &runtimeConfig{DeploymentEnv: "production", DBDSN: "postgres://theia:strong-password@postgres:5432/theia?sslmode=disable"},
			env:  map[string]string{"THEIA_ENCRYPTION_KEY": "strong-encryption-key", "POSTGRES_PASSWORD": "strong-password"},
			want: []string{"THEIA_SESSION_SECRET", "required"},
		},
		{
			name: "production rejects weak session secret",
			cfg:  &runtimeConfig{DeploymentEnv: "production", DBDSN: "postgres://theia:strong-password@postgres:5432/theia?sslmode=disable", SessionSecret: "short-secret"},
			env:  map[string]string{"THEIA_ENCRYPTION_KEY": "strong-encryption-key", "POSTGRES_PASSWORD": "strong-password"},
			want: []string{"THEIA_SESSION_SECRET", "weak"},
		},
		{
			name: "production requires metrics token",
			cfg:  &runtimeConfig{DeploymentEnv: "production", DBDSN: "postgres://theia:strong-password@postgres:5432/theia?sslmode=disable", SessionSecret: "0123456789abcdef0123456789abcdef"},
			env:  map[string]string{"THEIA_ENCRYPTION_KEY": "strong-encryption-key", "POSTGRES_PASSWORD": "strong-password"},
			want: []string{"THEIA_METRICS_TOKEN", "required"},
		},
		{
			name: "staging rejects weak metrics token",
			cfg:  &runtimeConfig{DeploymentEnv: "staging", DBDSN: "postgres://theia:strong-password@postgres:5432/theia?sslmode=disable", SessionSecret: "0123456789abcdef0123456789abcdef", MetricsToken: "short-token"},
			env:  map[string]string{"THEIA_ENCRYPTION_KEY": "strong-encryption-key", "POSTGRES_PASSWORD": "strong-password"},
			want: []string{"THEIA_METRICS_TOKEN", "weak"},
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
			cfg:  &runtimeConfig{DeploymentEnv: "", DBDSN: "postgres://theia:change-me@postgres:5432/theia?sslmode=disable"},
			env:  map[string]string{"THEIA_ENCRYPTION_KEY": "", "POSTGRES_PASSWORD": "change-me"},
		},
		{
			name: "development deployment env does not enforce secret policy",
			cfg:  &runtimeConfig{DeploymentEnv: "development", DBDSN: "postgres://theia:change-me@postgres:5432/theia?sslmode=disable"},
			env:  map[string]string{"THEIA_ENCRYPTION_KEY": "change-me", "POSTGRES_PASSWORD": "change-me"},
		},
		{
			name: "production accepts non-placeholder secrets",
			cfg:  &runtimeConfig{DeploymentEnv: "production", DBDSN: "postgres://theia:strong-password@postgres:5432/theia?sslmode=disable", SessionSecret: "0123456789abcdef0123456789abcdef", MetricsToken: "abcdef0123456789abcdef0123456789"},
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
			DBDSN:      "postgres://user:pass@127.0.0.1:1/theia?sslmode=disable",
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
			DBDSN:      "postgres://user:%zz@127.0.0.1:5432/theia?sslmode=disable",
			DataDir:    runtimeDir,
			ListenAddr: ":0",
			LogLevel:   "info",
		}, nil
	}
	t.Cleanup(func() { loadRuntimeConfig = originalLoadRuntimeConfig })

	originalOpenPrimaryDB := openPrimaryRuntimeDB
	openPrimaryRuntimeDB = func(dsn string) (*sql.DB, error) {
		return nil, errors.New("boom")
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
