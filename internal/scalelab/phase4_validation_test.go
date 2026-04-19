package scalelab

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestPhase4BuiltinProfile300_MatchesValidationContract(t *testing.T) {
	profile, err := BuiltinProfile("300")
	if err != nil {
		t.Fatalf("BuiltinProfile(300): %v", err)
	}

	if profile.Name != "300" {
		t.Fatalf("profile name = %q, want %q", profile.Name, "300")
	}
	if profile.DeviceCount != 300 {
		t.Fatalf("device count = %d, want 300", profile.DeviceCount)
	}
	if profile.PerformanceInterval != 30*time.Second {
		t.Fatalf("performance interval = %s, want %s", profile.PerformanceInterval, 30*time.Second)
	}
	if profile.OperationalInterval != 60*time.Second {
		t.Fatalf("operational interval = %s, want %s", profile.OperationalInterval, 60*time.Second)
	}
	if profile.StaticInterval != 5*time.Minute {
		t.Fatalf("static interval = %s, want %s", profile.StaticInterval, 5*time.Minute)
	}
	if profile.DefaultReplayPasses != 2 {
		t.Fatalf("default replay passes = %d, want 2", profile.DefaultReplayPasses)
	}
	if profile.DefaultBurstAdds != 15 {
		t.Fatalf("default burst adds = %d, want 15", profile.DefaultBurstAdds)
	}
	if profile.DefaultUnresolvedAdd != 60 {
		t.Fatalf("default unresolved add = %d, want 60", profile.DefaultUnresolvedAdd)
	}
}

func TestPhase4WISPHybridFixture_BaselineReportMatchesValidationContract(t *testing.T) {
	raw, err := os.ReadFile("testdata/wisp-hybrid.json")
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}

	var fixture ReplayFixture
	if err := json.Unmarshal(raw, &fixture); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}

	profile, err := BuiltinProfile("300")
	if err != nil {
		t.Fatalf("BuiltinProfile(300): %v", err)
	}
	scenario, err := BuiltinScenario("baseline", profile)
	if err != nil {
		t.Fatalf("BuiltinScenario(baseline): %v", err)
	}

	report, err := Run(profile, scenario, fixture)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	if report.Replay.FixtureName != "wisp-hybrid" {
		t.Fatalf("fixture name = %q, want %q", report.Replay.FixtureName, "wisp-hybrid")
	}
	if report.Replay.ObservationCount != 18 {
		t.Fatalf("observation count = %d, want 18", report.Replay.ObservationCount)
	}
	if report.Replay.ResolvedCount != 16 {
		t.Fatalf("resolved count = %d, want 16", report.Replay.ResolvedCount)
	}
	if report.Replay.UnresolvedCount != 2 {
		t.Fatalf("unresolved count = %d, want 2", report.Replay.UnresolvedCount)
	}
	if report.Replay.LinkEvents["created"] != 8 {
		t.Fatalf("created link events = %d, want 8", report.Replay.LinkEvents["created"])
	}
	if report.Replay.LinkEvents["noop"] != 24 {
		t.Fatalf("noop link events = %d, want 24", report.Replay.LinkEvents["noop"])
	}
}

func TestPhase4ValidateScript_SyntheticWritesRequiredEvidenceSurfaces(t *testing.T) {
	for _, tool := range []string{"bash", "curl"} {
		if _, err := exec.LookPath(tool); err != nil {
			t.Skipf("%s not available: %v", tool, err)
		}
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/metrics" {
			http.NotFound(w, r)
			return
		}

		w.Header().Set("Content-Type", "text/plain; version=0.0.4")
		_, _ = w.Write([]byte(strings.TrimSpace(`
# HELP theia_refresh_snapshot_build_seconds Snapshot build duration.
# TYPE theia_refresh_snapshot_build_seconds histogram
theia_refresh_snapshot_build_seconds_bucket{mode="dirty",result="success",le="1"} 1
theia_refresh_snapshot_build_seconds_sum{mode="dirty",result="success"} 0.1
theia_refresh_snapshot_build_seconds_count{mode="dirty",result="success"} 1
# HELP theia_refresh_topology_reload_total Topology reload count by reason.
# TYPE theia_refresh_topology_reload_total counter
theia_refresh_topology_reload_total{reason="startup"} 1
# HELP theia_state_changes_dropped_total Dropped state-store change batches.
# TYPE theia_state_changes_dropped_total counter
theia_state_changes_dropped_total 0
		`)))
	}))
	defer server.Close()

	root := phase4RepoRoot(t)
	outputDir := t.TempDir()

	cmd := exec.Command("bash", "scripts/phase4-validate.sh", "synthetic", server.URL, outputDir)
	cmd.Dir = root
	cmd.Env = os.Environ()
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("phase4-validate.sh failed: %v\n%s", err, output)
	}

	for _, name := range []string{
		"scale-300-baseline.json",
		"scale-300-burst-adds.json",
		"metrics.prom",
		"README.md",
	} {
		if _, err := os.Stat(filepath.Join(outputDir, name)); err != nil {
			t.Fatalf("expected %s to exist: %v", name, err)
		}
	}

	metrics, err := os.ReadFile(filepath.Join(outputDir, "metrics.prom"))
	if err != nil {
		t.Fatalf("ReadFile(metrics.prom): %v", err)
	}
	for _, metric := range []string{
		"theia_refresh_snapshot_build_seconds",
		"theia_refresh_topology_reload_total",
		"theia_state_changes_dropped_total",
	} {
		if !strings.Contains(string(metrics), metric) {
			t.Fatalf("metrics.prom missing %s", metric)
		}
	}

	readme, err := os.ReadFile(filepath.Join(outputDir, "README.md"))
	if err != nil {
		t.Fatalf("ReadFile(README.md): %v", err)
	}
	for _, expected := range []string{
		"theia_refresh_snapshot_build_seconds",
		"theia_refresh_topology_reload_total",
		"theia_state_changes_dropped_total",
		"window.__THEIA_CANVAS_METRICS__",
	} {
		if !strings.Contains(string(readme), expected) {
			t.Fatalf("README.md missing %s", expected)
		}
	}
}

func phase4RepoRoot(t *testing.T) string {
	t.Helper()

	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd: %v", err)
	}
	root := filepath.Clean(filepath.Join(wd, "..", ".."))
	if _, err := os.Stat(filepath.Join(root, "go.mod")); err != nil {
		t.Fatalf("repo root missing go.mod: %v", err)
	}
	return root
}
