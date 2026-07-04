package main

// This file exercises runtime bootstrap behavior so refactors preserve the documented contract.

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/lollinoo/theia/internal/domain"
	"github.com/lollinoo/theia/internal/service"
	"gopkg.in/yaml.v3"
)

type stubRuntimeStopper struct {
	name  string
	stops *[]string
	mu    *sync.Mutex
}

func (s stubRuntimeStopper) Stop() {
	if s.mu != nil {
		s.mu.Lock()
		defer s.mu.Unlock()
	}
	*s.stops = append(*s.stops, s.name)
}

type blockingRuntimeStopper struct {
	started chan struct{}
	release <-chan struct{}
}

func (s blockingRuntimeStopper) Stop() {
	close(s.started)
	<-s.release
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

func TestCIWorkflowPublishesNonMasterBranchImages(t *testing.T) {
	repoRoot := filepath.Clean(filepath.Join("..", ".."))
	workflow := readGitHubWorkflow(t, filepath.Join(repoRoot, ".github", "workflows", "ci.yml"))

	job := requireWorkflowJob(t, workflow, "branch-images")
	for _, want := range []string{
		"always()",
		"github.event_name == 'push'",
		"github.ref != 'refs/heads/master'",
		"!contains(needs.*.result, 'failure')",
		"!contains(needs.*.result, 'cancelled')",
	} {
		if !strings.Contains(job.If, want) {
			t.Fatalf("branch-images if = %q, want fragment %q", job.If, want)
		}
	}
	for _, want := range []string{"changes", "backend-fast", "frontend-fast", "browser-e2e"} {
		if !stringSliceContains([]string(job.Needs), want) {
			t.Fatalf("branch-images needs = %#v, want %q", job.Needs, want)
		}
	}
	if job.Permissions["packages"] != "write" {
		t.Fatalf("branch-images packages permission = %q, want write", job.Permissions["packages"])
	}

	backendMeta := requireWorkflowStepByID(t, job, "meta-backend-branch")
	requireWorkflowDockerMetadataTags(t, backendMeta, "ghcr.io/lollinoo/theia-backend")
	backendBuild := requireWorkflowStepByName(t, job, "Build and push backend branch image")
	requireWorkflowBuildPushStep(t, backendBuild, ".", "./Dockerfile", "${{ steps.meta-backend-branch.outputs.tags }}")

	frontendMeta := requireWorkflowStepByID(t, job, "meta-frontend-branch")
	requireWorkflowDockerMetadataTags(t, frontendMeta, "ghcr.io/lollinoo/theia-frontend")
	frontendBuild := requireWorkflowStepByName(t, job, "Build and push frontend branch image")
	requireWorkflowBuildPushStep(t, frontendBuild, "./frontend", "./Dockerfile.frontend", "${{ steps.meta-frontend-branch.outputs.tags }}")
}

func TestCIImageJobsCheckoutRepositoryBeforeBuildPush(t *testing.T) {
	repoRoot := filepath.Clean(filepath.Join("..", ".."))
	workflow := readGitHubWorkflow(t, filepath.Join(repoRoot, ".github", "workflows", "ci.yml"))

	for _, jobName := range []string{"master-images", "branch-images", "release-images"} {
		t.Run(jobName, func(t *testing.T) {
			job := requireWorkflowJob(t, workflow, jobName)
			requireWorkflowCheckoutBeforeBuildPush(t, job)
		})
	}
}

func TestCIImageJobsUseGitHubTokenForGHCRLogin(t *testing.T) {
	repoRoot := filepath.Clean(filepath.Join("..", ".."))
	workflow := readGitHubWorkflow(t, filepath.Join(repoRoot, ".github", "workflows", "ci.yml"))

	for _, jobName := range []string{"master-images", "branch-images", "release-images"} {
		t.Run(jobName, func(t *testing.T) {
			job := requireWorkflowJob(t, workflow, jobName)
			login := requireWorkflowStepByUses(t, job, "docker/login-action@v4")
			if login.With["registry"] != "ghcr.io" {
				t.Fatalf("docker login registry = %q, want ghcr.io", login.With["registry"])
			}
			if login.With["password"] != "${{ secrets.GITHUB_TOKEN }}" {
				t.Fatalf("docker login password = %q, want GitHub token", login.With["password"])
			}
		})
	}
}

func TestDockerfileUsesPostgres17ClientTools(t *testing.T) {
	repoRoot := filepath.Clean(filepath.Join("..", ".."))
	content, err := os.ReadFile(filepath.Join(repoRoot, "Dockerfile"))
	if err != nil {
		t.Fatalf("read Dockerfile: %v", err)
	}
	dockerfile := string(content)

	if !strings.Contains(dockerfile, "FROM postgres:17-bookworm AS postgres-tools") {
		t.Fatalf("Dockerfile postgres-tools stage must use PostgreSQL 17 client tools")
	}
	for _, tool := range []string{"pg_dump", "pg_restore", "psql"} {
		want := "/usr/lib/postgresql/17/bin/" + tool
		if !strings.Contains(dockerfile, want) {
			t.Fatalf("Dockerfile must copy %s from PostgreSQL 17 tools", tool)
		}
	}
}

func TestPrometheusConfigsScrapeBackendMetrics(t *testing.T) {
	repoRoot := filepath.Clean(filepath.Join("..", ".."))
	tests := []struct {
		path     string
		wantAuth bool
	}{
		{path: "docker/prometheus/prometheus.yml"},
		{path: "docker/prometheus/prometheus.prod.yml", wantAuth: true},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			cfg := readPrometheusConfig(t, filepath.Join(repoRoot, tt.path))
			job := requirePrometheusScrapeJob(t, cfg, "theia-backend")
			if job.MetricsPath != "/metrics" {
				t.Fatalf("%s theia-backend metrics_path = %q, want /metrics", tt.path, job.MetricsPath)
			}
			if !prometheusScrapeJobHasTarget(job, "backend:8080") {
				t.Fatalf("%s theia-backend targets = %#v, want backend:8080", tt.path, job.StaticConfigs)
			}

			if !tt.wantAuth {
				if job.Authorization.Type != "" || job.Authorization.Credentials != "" || job.Authorization.CredentialsFile != "" {
					t.Fatalf("%s dev backend scrape must not require auth by default: %#v", tt.path, job.Authorization)
				}
				return
			}

			if job.Authorization.Type != "Bearer" {
				t.Fatalf("%s authorization.type = %q, want Bearer", tt.path, job.Authorization.Type)
			}
			if job.Authorization.Credentials != "" {
				t.Fatalf("%s must not embed a literal bearer token", tt.path)
			}
			if job.Authorization.CredentialsFile != "/run/secrets/theia_metrics_token" {
				t.Fatalf("%s authorization.credentials_file = %q, want /run/secrets/theia_metrics_token", tt.path, job.Authorization.CredentialsFile)
			}
			if !stringSliceContains(cfg.RuleFiles, "/etc/prometheus/alert_rules.yml") {
				t.Fatalf("%s rule_files = %#v, want absolute alert rules path for generated production config", tt.path, cfg.RuleFiles)
			}
		})
	}
}

func TestProductionMetricsStackUsesComposeNetworkAndMetricsSecret(t *testing.T) {
	repoRoot := filepath.Clean(filepath.Join("..", ".."))
	compose := readComposeConfig(t, filepath.Join(repoRoot, "docker-compose.prod.yml"))
	prometheus, ok := compose.Services["prometheus"]
	if !ok {
		t.Fatal("docker-compose.prod.yml missing prometheus service")
	}
	if prometheus.NetworkMode != "" {
		t.Fatalf("prometheus network_mode = %q, want Compose network so backend service DNS is reachable", prometheus.NetworkMode)
	}
	if !stringSliceContains(prometheus.Networks, "theia-net") {
		t.Fatalf("prometheus networks = %#v, want theia-net", prometheus.Networks)
	}
	if !stringSliceContains(prometheus.Ports, "${PROMETHEUS_BIND_ADDR:-127.0.0.1}:${PROMETHEUS_PORT:-9090}:9090") {
		t.Fatalf("prometheus ports = %#v, want loopback-bound PROMETHEUS_PORT mapping", prometheus.Ports)
	}
	if !stringSliceContains(prometheus.Secrets, "theia_metrics_token") {
		t.Fatalf("prometheus secrets = %#v, want theia_metrics_token", prometheus.Secrets)
	}
	if !reflect.DeepEqual(prometheus.Entrypoint, []string{"/bin/sh", "-c"}) {
		t.Fatalf("prometheus entrypoint = %#v, want shell templating entrypoint", prometheus.Entrypoint)
	}
	if len(prometheus.Command) != 1 {
		t.Fatalf("prometheus command = %#v, want single shell command", prometheus.Command)
	}
	for _, fragment := range []string{
		`sed "s/backend:8080/backend:${BACKEND_PORT:-8080}/g"`,
		"--config.file=/tmp/prometheus.yml",
		"exec /bin/prometheus",
	} {
		if !strings.Contains(prometheus.Command[0], fragment) {
			t.Fatalf("prometheus command = %q, want fragment %q", prometheus.Command[0], fragment)
		}
	}
	secret, ok := compose.Secrets["theia_metrics_token"]
	if !ok {
		t.Fatal("docker-compose.prod.yml missing theia_metrics_token secret")
	}
	if secret.Environment != "THEIA_METRICS_TOKEN" {
		t.Fatalf("theia_metrics_token secret environment = %q, want THEIA_METRICS_TOKEN", secret.Environment)
	}

	snmpExporter, ok := compose.Services["snmp-exporter"]
	if !ok {
		t.Fatal("docker-compose.prod.yml missing snmp-exporter service")
	}
	if snmpExporter.NetworkMode != "" {
		t.Fatalf("snmp-exporter network_mode = %q, want Compose network so Prometheus can scrape by service DNS", snmpExporter.NetworkMode)
	}
	if !stringSliceContains(snmpExporter.Networks, "theia-net") {
		t.Fatalf("snmp-exporter networks = %#v, want theia-net", snmpExporter.Networks)
	}
}

func TestRuntimeHTTPHandlerServesPprofWithMetricsToken(t *testing.T) {
	routerHit := false
	router := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		routerHit = true
		http.NotFound(w, r)
	})
	metrics := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("metrics-ok"))
	})
	handler := runtimeHTTPHandler(router, metrics, "0123456789abcdef0123456789abcdef")

	req := httptest.NewRequest(http.MethodGet, "/debug/pprof/goroutine?debug=1", nil)
	req.Header.Set("Authorization", "Bearer 0123456789abcdef0123456789abcdef")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("pprof status = %d, want %d; body: %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	if routerHit {
		t.Fatal("pprof request reached main API router")
	}
	if !strings.Contains(rec.Body.String(), "goroutine profile") {
		t.Fatalf("pprof body = %q, want goroutine profile", rec.Body.String())
	}
}

func TestRuntimeHTTPHandlerRejectsPprofWithoutMetricsToken(t *testing.T) {
	router := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("unauthorized pprof request reached main API router")
	})
	metrics := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("unauthorized pprof request reached metrics handler")
	})
	handler := runtimeHTTPHandler(router, metrics, "0123456789abcdef0123456789abcdef")

	req := httptest.NewRequest(http.MethodGet, "/debug/pprof/", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("pprof status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
	if got := rec.Header().Get("WWW-Authenticate"); got != `Bearer realm="theia-metrics"` {
		t.Fatalf("WWW-Authenticate = %q, want metrics bearer challenge", got)
	}
}

func TestRuntimeHTTPHandlerStillServesMetricsWithMetricsToken(t *testing.T) {
	router := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("metrics request reached main API router")
	})
	metrics := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("metrics-ok"))
	})
	handler := runtimeHTTPHandler(router, metrics, "0123456789abcdef0123456789abcdef")

	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	req.Header.Set("Authorization", "Bearer 0123456789abcdef0123456789abcdef")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("metrics status = %d, want %d", rec.Code, http.StatusOK)
	}
	if rec.Body.String() != "metrics-ok" {
		t.Fatalf("metrics body = %q, want metrics-ok", rec.Body.String())
	}
}

func TestPrometheusAlertRulesCoverRuntimePerformanceSignals(t *testing.T) {
	repoRoot := filepath.Clean(filepath.Join("..", ".."))
	rules := readPrometheusRules(t, filepath.Join(repoRoot, "docker/prometheus/alert_rules.yml"))
	tests := []struct {
		alert                string
		wantSeverity         string
		wantFragments        []string
		wantSummaryFragments []string
	}{
		{
			alert:        "BulkOperationRejections",
			wantSeverity: "warning",
			wantFragments: []string{
				"theia_bulk_operation_rejections_total",
				"increase(",
			},
		},
		{
			alert:        "BulkOperationSaturated",
			wantSeverity: "warning",
			wantFragments: []string{
				"theia_bulk_operation_in_flight",
				"theia_bulk_operation_concurrency_limit",
				`scope="global"`,
			},
		},
		{
			alert:        "WebSocketBackpressure",
			wantSeverity: "warning",
			wantFragments: []string{
				"theia_ws_backpressure_total",
				"increase(",
			},
		},
		{
			alert:        "WebSocketResyncRequired",
			wantSeverity: "warning",
			wantFragments: []string{
				"theia_ws_client_resync_required_total",
				"increase(",
			},
		},
		{
			alert:        "PollingEssentialOverloaded",
			wantSeverity: "warning",
			wantFragments: []string{
				"theia_polling_essential_overloaded",
				"== 1",
			},
		},
		{
			alert:        "PollingDeadlineMisses",
			wantSeverity: "warning",
			wantFragments: []string{
				"theia_polling_deadline_miss_total",
				"increase(",
			},
		},
		{
			alert:        "PollingFailuresHigh",
			wantSeverity: "warning",
			wantFragments: []string{
				"theia_poll_results_total",
				`outcome="failure"`,
			},
		},
		{
			alert:        "SchedulerQueueLagHigh",
			wantSeverity: "warning",
			wantFragments: []string{
				"theia_scheduler_queue_lag_seconds",
				"max by",
			},
		},
		{
			alert:        "SchedulerBackpressure",
			wantSeverity: "warning",
			wantFragments: []string{
				"theia_scheduler_backpressure_total",
				"increase(",
			},
		},
		{
			alert:        "SchedulerTaskDurationHigh",
			wantSeverity: "warning",
			wantFragments: []string{
				"theia_scheduler_task_duration_seconds_bucket",
				"histogram_quantile",
			},
		},
		{
			alert:        "SNMPBulkWalkErrors",
			wantSeverity: "warning",
			wantFragments: []string{
				"theia_snmp_collector_operations_total",
				`operation=~".*_walk"`,
				"collector, operation, result",
			},
			wantSummaryFragments: []string{
				"{{ $labels.operation }}",
			},
		},
		{
			alert:        "SNMPBulkWalkSlow",
			wantSeverity: "warning",
			wantFragments: []string{
				"theia_snmp_collector_operation_seconds_bucket",
				`operation=~".*_walk"`,
				"histogram_quantile",
				"collector, operation, le",
			},
			wantSummaryFragments: []string{
				"{{ $labels.operation }}",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.alert, func(t *testing.T) {
			rule := requirePrometheusAlertRule(t, rules, tt.alert)
			if rule.Expr == "" {
				t.Fatalf("%s expression is empty", tt.alert)
			}
			for _, fragment := range tt.wantFragments {
				if !strings.Contains(rule.Expr, fragment) {
					t.Fatalf("%s expression = %q, want fragment %q", tt.alert, rule.Expr, fragment)
				}
			}
			if got := rule.Labels["severity"]; got != tt.wantSeverity {
				t.Fatalf("%s severity = %q, want %q", tt.alert, got, tt.wantSeverity)
			}
			if rule.For == "" {
				t.Fatalf("%s must define a for duration", tt.alert)
			}
			if rule.Annotations["summary"] == "" {
				t.Fatalf("%s must define a summary annotation", tt.alert)
			}
			for _, fragment := range tt.wantSummaryFragments {
				if !strings.Contains(rule.Annotations["summary"], fragment) {
					t.Fatalf("%s summary = %q, want fragment %q", tt.alert, rule.Annotations["summary"], fragment)
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

type prometheusConfigFile struct {
	RuleFiles     []string                 `yaml:"rule_files"`
	ScrapeConfigs []prometheusScrapeConfig `yaml:"scrape_configs"`
}

type prometheusScrapeConfig struct {
	JobName        string                    `yaml:"job_name"`
	MetricsPath    string                    `yaml:"metrics_path"`
	StaticConfigs  []prometheusStaticConfig  `yaml:"static_configs"`
	Authorization  prometheusAuthorization   `yaml:"authorization"`
	RelabelConfigs []prometheusRelabelConfig `yaml:"relabel_configs"`
}

type prometheusStaticConfig struct {
	Targets []string `yaml:"targets"`
}

type prometheusAuthorization struct {
	Type            string `yaml:"type"`
	Credentials     string `yaml:"credentials"`
	CredentialsFile string `yaml:"credentials_file"`
}

type prometheusRelabelConfig struct {
	TargetLabel string `yaml:"target_label"`
	Replacement string `yaml:"replacement"`
}

type prometheusRuleFile struct {
	Groups []prometheusRuleGroup `yaml:"groups"`
}

type prometheusRuleGroup struct {
	Name  string                `yaml:"name"`
	Rules []prometheusAlertRule `yaml:"rules"`
}

type prometheusAlertRule struct {
	Alert       string            `yaml:"alert"`
	Expr        string            `yaml:"expr"`
	For         string            `yaml:"for"`
	Labels      map[string]string `yaml:"labels"`
	Annotations map[string]string `yaml:"annotations"`
}

type composeConfigFile struct {
	Services map[string]composeService `yaml:"services"`
	Secrets  map[string]composeSecret  `yaml:"secrets"`
}

type githubWorkflowFile struct {
	Jobs map[string]githubWorkflowJob `yaml:"jobs"`
}

type githubWorkflowJob struct {
	If          string               `yaml:"if"`
	Needs       githubWorkflowNeeds  `yaml:"needs"`
	Permissions map[string]string    `yaml:"permissions"`
	Steps       []githubWorkflowStep `yaml:"steps"`
}

type githubWorkflowNeeds []string

func (n *githubWorkflowNeeds) UnmarshalYAML(value *yaml.Node) error {
	switch value.Kind {
	case yaml.ScalarNode:
		if value.Value != "" {
			*n = []string{value.Value}
		}
	case yaml.SequenceNode:
		values := make([]string, 0, len(value.Content))
		for _, item := range value.Content {
			values = append(values, item.Value)
		}
		*n = values
	}
	return nil
}

type githubWorkflowStep struct {
	Name string            `yaml:"name"`
	ID   string            `yaml:"id"`
	Uses string            `yaml:"uses"`
	With map[string]string `yaml:"with"`
}

type composeService struct {
	NetworkMode string   `yaml:"network_mode"`
	Networks    []string `yaml:"networks"`
	Ports       []string `yaml:"ports"`
	Secrets     []string `yaml:"secrets"`
	Entrypoint  []string `yaml:"entrypoint"`
	Command     []string `yaml:"command"`
}

type composeSecret struct {
	Environment string `yaml:"environment"`
}

func readPrometheusConfig(t *testing.T, path string) prometheusConfigFile {
	t.Helper()
	var cfg prometheusConfigFile
	readYAMLFile(t, path, &cfg)
	return cfg
}

func readPrometheusRules(t *testing.T, path string) prometheusRuleFile {
	t.Helper()
	var rules prometheusRuleFile
	readYAMLFile(t, path, &rules)
	return rules
}

func readComposeConfig(t *testing.T, path string) composeConfigFile {
	t.Helper()
	var compose composeConfigFile
	readYAMLFile(t, path, &compose)
	return compose
}

func readGitHubWorkflow(t *testing.T, path string) githubWorkflowFile {
	t.Helper()
	var workflow githubWorkflowFile
	readYAMLFile(t, path, &workflow)
	return workflow
}

func readYAMLFile(t *testing.T, path string, target any) {
	t.Helper()
	contentBytes, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%s): %v", path, err)
	}
	if err := yaml.Unmarshal(contentBytes, target); err != nil {
		t.Fatalf("yaml.Unmarshal(%s): %v", path, err)
	}
}

func requirePrometheusScrapeJob(t *testing.T, cfg prometheusConfigFile, jobName string) prometheusScrapeConfig {
	t.Helper()
	for _, job := range cfg.ScrapeConfigs {
		if job.JobName == jobName {
			return job
		}
	}
	t.Fatalf("missing Prometheus scrape job %q", jobName)
	return prometheusScrapeConfig{}
}

func prometheusScrapeJobHasTarget(job prometheusScrapeConfig, target string) bool {
	for _, staticConfig := range job.StaticConfigs {
		if stringSliceContains(staticConfig.Targets, target) {
			return true
		}
	}
	return false
}

func requirePrometheusAlertRule(t *testing.T, rules prometheusRuleFile, alert string) prometheusAlertRule {
	t.Helper()
	for _, group := range rules.Groups {
		for _, rule := range group.Rules {
			if rule.Alert == alert {
				return rule
			}
		}
	}
	t.Fatalf("missing Prometheus alert %q", alert)
	return prometheusAlertRule{}
}

func requireWorkflowJob(t *testing.T, workflow githubWorkflowFile, name string) githubWorkflowJob {
	t.Helper()
	job, ok := workflow.Jobs[name]
	if !ok {
		t.Fatalf("missing workflow job %q", name)
	}
	return job
}

func requireWorkflowStepByID(t *testing.T, job githubWorkflowJob, id string) githubWorkflowStep {
	t.Helper()
	for _, step := range job.Steps {
		if step.ID == id {
			return step
		}
	}
	t.Fatalf("missing workflow step id %q", id)
	return githubWorkflowStep{}
}

func requireWorkflowStepByName(t *testing.T, job githubWorkflowJob, name string) githubWorkflowStep {
	t.Helper()
	for _, step := range job.Steps {
		if step.Name == name {
			return step
		}
	}
	t.Fatalf("missing workflow step %q", name)
	return githubWorkflowStep{}
}

func requireWorkflowStepByUses(t *testing.T, job githubWorkflowJob, uses string) githubWorkflowStep {
	t.Helper()
	for _, step := range job.Steps {
		if step.Uses == uses {
			return step
		}
	}
	t.Fatalf("missing workflow step uses %q", uses)
	return githubWorkflowStep{}
}

func requireWorkflowDockerMetadataTags(t *testing.T, step githubWorkflowStep, image string) {
	t.Helper()
	if step.Uses != "docker/metadata-action@v6" {
		t.Fatalf("%s uses = %q, want docker/metadata-action@v6", step.ID, step.Uses)
	}
	if step.With["images"] != image {
		t.Fatalf("%s images = %q, want %q", step.ID, step.With["images"], image)
	}
	tags := step.With["tags"]
	for _, want := range []string{"type=ref,event=branch", "type=sha"} {
		if !strings.Contains(tags, want) {
			t.Fatalf("%s tags = %q, want fragment %q", step.ID, tags, want)
		}
	}
}

func requireWorkflowBuildPushStep(t *testing.T, step githubWorkflowStep, contextPath, dockerfile, tags string) {
	t.Helper()
	if step.Uses != "docker/build-push-action@v7" {
		t.Fatalf("%s uses = %q, want docker/build-push-action@v7", step.Name, step.Uses)
	}
	want := map[string]string{
		"context": contextPath,
		"file":    dockerfile,
		"target":  "production",
		"push":    "true",
		"tags":    tags,
	}
	for key, value := range want {
		if step.With[key] != value {
			t.Fatalf("%s with.%s = %q, want %q", step.Name, key, step.With[key], value)
		}
	}
}

func requireWorkflowCheckoutBeforeBuildPush(t *testing.T, job githubWorkflowJob) {
	t.Helper()
	checkoutIndex := -1
	firstBuildIndex := -1
	for i, step := range job.Steps {
		switch step.Uses {
		case "actions/checkout@v7":
			if checkoutIndex == -1 {
				checkoutIndex = i
			}
		case "docker/build-push-action@v7":
			if firstBuildIndex == -1 {
				firstBuildIndex = i
			}
		}
	}
	if firstBuildIndex == -1 {
		t.Fatalf("missing docker/build-push-action@v7 step")
	}
	if checkoutIndex == -1 {
		t.Fatalf("missing actions/checkout@v7 step before docker/build-push-action@v7")
	}
	if checkoutIndex > firstBuildIndex {
		t.Fatalf("actions/checkout@v7 step index = %d, want before first docker/build-push-action@v7 step index %d", checkoutIndex, firstBuildIndex)
	}
}

func stringSliceContains(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
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

func TestConfigureBackupServiceBulkOperationLimitsUsesRuntimeConfig(t *testing.T) {
	cfg := &runtimeConfig{}
	cfg.BulkBackupLimits.MaxDevices = 11
	cfg.BulkBackupLimits.MaxQueuedJobs = 12
	cfg.BulkDownloadLimits.MaxDevices = 13
	cfg.BulkDownloadLimits.MaxFiles = 14
	cfg.BulkDownloadLimits.MaxBytes = 15
	cfg.BulkDownloadLimits.MaxConcurrentPerActor = 16
	cfg.BulkDownloadLimits.MaxConcurrentGlobal = 17

	svc := service.NewBackupService(nil, nil, nil, nil, nil, nil, nil, nil, "", nil)
	configureBackupServiceBulkOperationLimits(svc, cfg)

	limits := svc.BulkOperationLimits()
	if limits.BulkBackupMaxDevices != 11 ||
		limits.BulkBackupMaxQueuedJobs != 12 ||
		limits.BulkDownloadMaxDevices != 13 ||
		limits.BulkDownloadMaxFiles != 14 ||
		limits.BulkDownloadMaxBytes != 15 ||
		limits.BulkDownloadMaxConcurrentPerActor != 16 ||
		limits.BulkDownloadMaxConcurrentGlobal != 17 {
		t.Fatalf("bulk operation limits = %#v, want runtime config values", limits)
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
		{name: "pipeline", stopper: stubRuntimeStopper{name: "pipeline", stops: &order}},
		{name: "instance-backups", stopper: stubRuntimeStopper{name: "instance-backups", stops: &order}},
		{name: "device-backups", stopper: stubRuntimeStopper{name: "device-backups", stops: &order}},
	}

	bootstrap.stopRuntime(children)

	if got, want := fmt.Sprint(order), "[device-backups instance-backups pipeline]"; got != want {
		t.Fatalf("stop order = %s, want %s", got, want)
	}
}

func TestRuntimeBootstrapStopRuntimeContinuesAfterChildStopTimeout(t *testing.T) {
	originalTimeout := runtimeChildStopTimeout
	runtimeChildStopTimeout = 20 * time.Millisecond
	t.Cleanup(func() { runtimeChildStopTimeout = originalTimeout })

	bootstrap := &runtimeBootstrap{}
	var (
		mu      sync.Mutex
		order   []string
		started = make(chan struct{})
		release = make(chan struct{})
	)
	children := runtimeChildren{
		{name: "pipeline", stopper: stubRuntimeStopper{name: "pipeline", stops: &order, mu: &mu}},
		{name: "stuck-instance-backups", stopper: blockingRuntimeStopper{started: started, release: release}},
		{name: "device-backups", stopper: stubRuntimeStopper{name: "device-backups", stops: &order, mu: &mu}},
	}

	returned := make(chan struct{})
	go func() {
		bootstrap.stopRuntime(children)
		close(returned)
	}()

	select {
	case <-started:
	case <-time.After(250 * time.Millisecond):
		close(release)
		t.Fatal("blocking child Stop() was not called")
	}

	select {
	case <-returned:
	case <-time.After(250 * time.Millisecond):
		close(release)
		t.Fatal("stopRuntime did not return after child stop timeout")
	}
	close(release)

	mu.Lock()
	got := fmt.Sprint(order)
	mu.Unlock()
	if want := "[device-backups pipeline]"; got != want {
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
			name: "production missing active keyring id",
			cfg:  &runtimeConfig{DeploymentEnv: "production", DBDSN: "postgres://theia:strong-password@postgres:5432/theia?sslmode=disable"},
			env:  map[string]string{"THEIA_ENCRYPTION_KEYS": "kid-a=strong-encryption-key", "POSTGRES_PASSWORD": "strong-password"},
			want: []string{"THEIA_ENCRYPTION_KEY_ID", "required"},
		},
		{
			name: "production missing keyring keys",
			cfg:  &runtimeConfig{DeploymentEnv: "production", DBDSN: "postgres://theia:strong-password@postgres:5432/theia?sslmode=disable"},
			env:  map[string]string{"THEIA_ENCRYPTION_KEY_ID": "kid-a", "POSTGRES_PASSWORD": "strong-password"},
			want: []string{"THEIA_ENCRYPTION_KEYS", "required"},
		},
		{
			name: "production rejects keyring placeholder secret",
			cfg:  &runtimeConfig{DeploymentEnv: "production", DBDSN: "postgres://theia:strong-password@postgres:5432/theia?sslmode=disable"},
			env:  map[string]string{"THEIA_ENCRYPTION_KEY_ID": "kid-a", "THEIA_ENCRYPTION_KEYS": "kid-a=change-me", "POSTGRES_PASSWORD": "strong-password"},
			want: []string{"THEIA_ENCRYPTION_KEYS", "example"},
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
			t.Setenv("THEIA_ENCRYPTION_KEY", "")
			t.Setenv("THEIA_ENCRYPTION_KEY_ID", "")
			t.Setenv("THEIA_ENCRYPTION_KEYS", "")
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
		{
			name: "production accepts keyring secrets",
			cfg:  &runtimeConfig{DeploymentEnv: "production", DBDSN: "postgres://theia:strong-password@postgres:5432/theia?sslmode=disable", SessionSecret: "0123456789abcdef0123456789abcdef", MetricsToken: "abcdef0123456789abcdef0123456789"},
			env:  map[string]string{"THEIA_ENCRYPTION_KEY_ID": "kid-b", "THEIA_ENCRYPTION_KEYS": "kid-a=old-strong-encryption-key,kid-b=new-strong-encryption-key", "POSTGRES_PASSWORD": "another-strong-password"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("THEIA_ENCRYPTION_KEY", "")
			t.Setenv("THEIA_ENCRYPTION_KEY_ID", "")
			t.Setenv("THEIA_ENCRYPTION_KEYS", "")
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
