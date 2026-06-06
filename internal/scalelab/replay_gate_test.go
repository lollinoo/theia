package scalelab

// This file exercises replay gate behavior so refactors preserve the documented contract.

import "testing"

func TestBurstReplayFixtureKeepsDeterministicLinkCountsAcrossPasses(t *testing.T) {
	profile, err := BuiltinProfile("300")
	if err != nil {
		t.Fatalf("BuiltinProfile(300): %v", err)
	}
	scenario, err := BuiltinScenario("burst-adds", profile)
	if err != nil {
		t.Fatalf("BuiltinScenario(burst-adds): %v", err)
	}

	fixture := GenerateSyntheticFixture(profile, scenario)
	first, err := Run(profile, scenario, fixture)
	if err != nil {
		t.Fatalf("first Run: %v", err)
	}
	second, err := Run(profile, scenario, fixture)
	if err != nil {
		t.Fatalf("second Run: %v", err)
	}

	assertReplayCountsMatch(t, first.Replay, second.Replay)
}

func assertReplayCountsMatch(t *testing.T, first ReplayReport, second ReplayReport) {
	t.Helper()

	if first.ObservationCount != second.ObservationCount {
		t.Fatalf("observation count changed across passes: %d vs %d", first.ObservationCount, second.ObservationCount)
	}
	if first.ResolvedCount != second.ResolvedCount {
		t.Fatalf("resolved count changed across passes: %d vs %d", first.ResolvedCount, second.ResolvedCount)
	}
	if first.UnresolvedCount != second.UnresolvedCount {
		t.Fatalf("unresolved count changed across passes: %d vs %d", first.UnresolvedCount, second.UnresolvedCount)
	}
	if first.SelfNeighborCount != second.SelfNeighborCount {
		t.Fatalf("self-neighbor count changed across passes: %d vs %d", first.SelfNeighborCount, second.SelfNeighborCount)
	}
	if len(first.LinkEvents) != len(second.LinkEvents) {
		t.Fatalf("link event kinds changed across passes: %v vs %v", first.LinkEvents, second.LinkEvents)
	}
	for kind, want := range first.LinkEvents {
		if got := second.LinkEvents[kind]; got != want {
			t.Fatalf("link event count for %q = %d, want %d", kind, got, want)
		}
	}
}
