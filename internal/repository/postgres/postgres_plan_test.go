package postgres

// This file exercises postgres plan behavior so refactors preserve the documented contract.

import (
	"fmt"
	"reflect"
	"regexp"
	"strings"
	"testing"
)

func TestDefaultPostgresPlanChecks_CoverScaleCriticalLookups(t *testing.T) {
	checks := DefaultPostgresPlanChecks()
	registeredChecks := repositoryPlanChecks()
	if len(checks) != len(registeredChecks) {
		t.Fatalf("len(checks) = %d, want %d", len(checks), len(registeredChecks))
	}

	wantIndexes := map[string]string{
		"device-by-sysname":              "idx_devices_sys_name_lookup",
		"link-pair-lookup":               "idx_links_target_device_created_at",
		"observation-ingest-lookup":      "idx_topology_observations_remote_identity_protocol",
		"unresolved-neighbors-by-device": "idx_unresolved_neighbors_local_device_id",
	}

	for i, check := range checks {
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

		registered := registeredChecks[i]
		if check.Name != registered.Name {
			t.Fatalf("check %d name = %q, want %q", i, check.Name, registered.Name)
		}
		if check.Query != registered.Query {
			t.Fatalf("check %s postgres query differs from registry", check.Name)
		}
		if check.ExpectedIndex != registered.ExpectedIndex {
			t.Fatalf("check %s expected index differs from registry", check.Name)
		}
		if !reflect.DeepEqual(check.Args, registered.Args) {
			t.Fatalf("check %s args = %#v, want %#v", check.Name, check.Args, registered.Args)
		}
	}
}

func TestRepositoryPlanChecks_MatchProductionLookupShapes(t *testing.T) {
	checks := repositoryPlanChecksByName(t)

	device := checks["device-by-sysname"]
	if !strings.Contains(device.Query, "sys_name_lookup = $1 AND sys_name_lookup != ''") {
		t.Fatalf("postgres device sysname check does not match runtime partial-index predicate: %s", device.Query)
	}

	if _, ok := checks["unresolved-neighbor-resolution-lookup"]; ok {
		t.Fatal("synthetic unresolved neighbor resolution lookup should not be registered")
	}
	unresolved := checks["unresolved-neighbors-by-device"]
	if !strings.Contains(unresolved.Query, "WHERE local_device_id = $1 AND resolved_at IS NULL") {
		t.Fatalf("postgres unresolved neighbor check does not match active list lookup predicate: %s", unresolved.Query)
	}
	if !strings.Contains(unresolved.Query, "ORDER BY protocol, remote_identity") {
		t.Fatalf("postgres unresolved neighbor check does not match active list lookup ordering: %s", unresolved.Query)
	}
}

func TestRepositoryPlanChecks_DefinePostgresSQL(t *testing.T) {
	checks := repositoryPlanChecks()
	if len(checks) == 0 {
		t.Fatal("no repository plan checks registered")
	}

	postgresPlaceholder := regexp.MustCompile(`\$\d+`)

	for _, check := range checks {
		t.Run(check.Name, func(t *testing.T) {
			if check.Query == "" {
				t.Fatal("postgres query is empty")
			}
			if check.ExpectedIndex == "" {
				t.Fatal("postgres expected index is empty")
			}
			if len(check.Args) == 0 {
				t.Fatal("registered check has no args")
			}
			if strings.Contains(check.Query, "?") {
				t.Fatalf("postgres query must not use ? placeholders: %s", check.Query)
			}
			postgresPlaceholders := postgresPlaceholder.FindAllString(check.Query, -1)
			if len(postgresPlaceholders) == 0 {
				t.Fatalf("postgres query with args must use $n placeholders: %s", check.Query)
			}
			if got, want := len(postgresPlaceholders), len(check.Args); got != want {
				t.Fatalf("postgres placeholder count = %d, want %d", got, want)
			}
			for i, placeholder := range postgresPlaceholders {
				if want := fmt.Sprintf("$%d", i+1); placeholder != want {
					t.Fatalf("postgres placeholder %d = %s, want %s in query: %s", i, placeholder, want, check.Query)
				}
			}
		})
	}
}

func repositoryPlanChecksByName(t *testing.T) map[string]repositoryPlanCheck {
	t.Helper()

	checksByName := make(map[string]repositoryPlanCheck)
	for _, check := range repositoryPlanChecks() {
		if _, exists := checksByName[check.Name]; exists {
			t.Fatalf("duplicate repository plan check %q", check.Name)
		}
		checksByName[check.Name] = check
	}
	return checksByName
}
