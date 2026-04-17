package sqlite

import "testing"

func TestDefaultPostgresPlanChecks_CoverScaleCriticalLookups(t *testing.T) {
	checks := DefaultPostgresPlanChecks()
	if len(checks) != 4 {
		t.Fatalf("len(checks) = %d, want 4", len(checks))
	}

	wantIndexes := map[string]string{
		"device-by-sysname":                     "idx_devices_sys_name_lookup",
		"link-pair-lookup":                      "idx_links_pair_lookup",
		"observation-ingest-lookup":             "idx_topology_observations_ingest_lookup",
		"unresolved-neighbor-resolution-lookup": "idx_unresolved_neighbors_active_lookup",
	}

	for _, check := range checks {
		expected, ok := wantIndexes[check.Name]
		if !ok {
			t.Fatalf("unexpected plan check %q", check.Name)
		}
		if check.ExpectedIndex != expected {
			t.Fatalf("check %s expected index %q, got %q", check.Name, expected, check.ExpectedIndex)
		}
		if check.Query == "" {
			t.Fatalf("check %s has empty query", check.Name)
		}
		if len(check.Args) == 0 {
			t.Fatalf("check %s has no placeholder args", check.Name)
		}
	}
}
