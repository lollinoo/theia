package scalelab

import (
	"encoding/json"
	"os"
	"testing"
)

func TestBuiltinProfiles_Cover1005001000(t *testing.T) {
	for _, name := range []string{"100", "500", "1000"} {
		profile, err := BuiltinProfile(name)
		if err != nil {
			t.Fatalf("BuiltinProfile(%q): %v", name, err)
		}
		if profile.DeviceCount <= 0 {
			t.Fatalf("profile %s has invalid device count %d", name, profile.DeviceCount)
		}
		if profile.PerformanceInterval <= 0 || profile.OperationalInterval <= 0 || profile.StaticInterval <= 0 {
			t.Fatalf("profile %s has invalid intervals: %#v", name, profile)
		}
	}
}

func TestGenerateSyntheticFixture_ExpandsBurstAndUnresolvedNeighbors(t *testing.T) {
	profile, err := BuiltinProfile("100")
	if err != nil {
		t.Fatalf("BuiltinProfile: %v", err)
	}
	scenario, err := BuiltinScenario("burst-unresolved-neighbors", profile)
	if err != nil {
		t.Fatalf("BuiltinScenario: %v", err)
	}

	fixture := GenerateSyntheticFixture(profile, scenario)
	if len(fixture.Observations) <= profile.DeviceCount {
		t.Fatalf("expected fixture observations > device count, got %d", len(fixture.Observations))
	}

	unresolved := 0
	for _, observation := range fixture.Observations {
		if observation.RemoteDeviceID == "" {
			unresolved++
		}
	}
	if unresolved != scenario.BurstUnresolvedNeighbors {
		t.Fatalf("unresolved observations = %d, want %d", unresolved, scenario.BurstUnresolvedNeighbors)
	}
}

func TestRun_ReplaySecondPassProducesNoops(t *testing.T) {
	profile, err := BuiltinProfile("100")
	if err != nil {
		t.Fatalf("BuiltinProfile: %v", err)
	}
	scenario, err := BuiltinScenario("baseline", profile)
	if err != nil {
		t.Fatalf("BuiltinScenario: %v", err)
	}
	scenario.ReplayPasses = 2

	report, err := Run(profile, scenario, GenerateSyntheticFixture(profile, scenario))
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	if report.Replay.LinkEvents["created"] == 0 {
		t.Fatal("expected created link events in replay report")
	}
	if report.Replay.LinkEvents["noop"] == 0 {
		t.Fatal("expected noop link events on repeated replay pass")
	}
	if report.Workload.PerformanceTasksPerMinute <= report.Workload.StaticTasksPerMinute {
		t.Fatalf("expected performance task rate to exceed static rate, got %#v", report.Workload)
	}
}

func TestSampleFixture_TracksUnresolvedNeighbors(t *testing.T) {
	raw, err := os.ReadFile("testdata/lldp-sample.json")
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}

	var fixture ReplayFixture
	if err := json.Unmarshal(raw, &fixture); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}

	_, unresolved, _, err := toObservations(fixture)
	if err != nil {
		t.Fatalf("toObservations: %v", err)
	}
	if unresolved != 1 {
		t.Fatalf("unresolved = %d, want 1", unresolved)
	}
}
