package sqlite

import (
	"database/sql"
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
		"device-by-sysname":                     "idx_devices_sys_name_lookup",
		"link-pair-lookup":                      "idx_links_pair_lookup",
		"observation-ingest-lookup":             "idx_topology_observations_ingest_lookup",
		"unresolved-neighbor-resolution-lookup": "idx_unresolved_neighbors_active_lookup",
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
		if check.Query != registered.PostgresQuery {
			t.Fatalf("check %s postgres query differs from registry", check.Name)
		}
		if check.ExpectedIndex != registered.PostgresExpectedIndex {
			t.Fatalf("check %s expected index differs from registry", check.Name)
		}
		if !reflect.DeepEqual(check.Args, registered.Args) {
			t.Fatalf("check %s args = %#v, want %#v", check.Name, check.Args, registered.Args)
		}
	}
}

func TestRepositoryPlanChecks_DefineExplicitDialectSQL(t *testing.T) {
	checks := repositoryPlanChecks()
	if len(checks) == 0 {
		t.Fatal("no repository plan checks registered")
	}

	postgresPlaceholder := regexp.MustCompile(`\$\d+`)

	for _, check := range checks {
		t.Run(check.Name, func(t *testing.T) {
			if check.SQLiteQuery == "" {
				t.Fatal("sqlite query is empty")
			}
			if check.PostgresQuery == "" {
				t.Fatal("postgres query is empty")
			}
			if check.SQLiteExpectedIndex == "" {
				t.Fatal("sqlite expected index is empty")
			}
			if check.PostgresExpectedIndex == "" {
				t.Fatal("postgres expected index is empty")
			}
			if len(check.Args) == 0 {
				t.Fatal("registered check has no args")
			}
			if !strings.Contains(check.SQLiteQuery, "?") {
				t.Fatalf("sqlite query must use ? placeholders: %s", check.SQLiteQuery)
			}
			if strings.Contains(strings.ToUpper(check.SQLiteQuery), "INDEXED BY") {
				t.Fatalf("sqlite query must not force index selection: %s", check.SQLiteQuery)
			}
			if strings.Contains(check.PostgresQuery, "?") {
				t.Fatalf("postgres query must not use ? placeholders: %s", check.PostgresQuery)
			}
			postgresPlaceholders := postgresPlaceholder.FindAllString(check.PostgresQuery, -1)
			if len(postgresPlaceholders) == 0 {
				t.Fatalf("postgres query with args must use $n placeholders: %s", check.PostgresQuery)
			}
			if got, want := strings.Count(check.SQLiteQuery, "?"), len(check.Args); got != want {
				t.Fatalf("sqlite placeholder count = %d, want %d", got, want)
			}
			if got, want := len(postgresPlaceholders), len(check.Args); got != want {
				t.Fatalf("postgres placeholder count = %d, want %d", got, want)
			}
			for i, placeholder := range postgresPlaceholders {
				if want := fmt.Sprintf("$%d", i+1); placeholder != want {
					t.Fatalf("postgres placeholder %d = %s, want %s in query: %s", i, placeholder, want, check.PostgresQuery)
				}
			}
		})
	}
}

func TestRepositoryPlanChecks_SQLiteQueryPlansUseExpectedIndexes(t *testing.T) {
	db := setupTestDB(t)

	for _, check := range repositoryPlanChecks() {
		t.Run(check.Name, func(t *testing.T) {
			plan := explainSQLiteQueryPlan(t, db, check.SQLiteQuery, check.Args)
			if !strings.Contains(plan, check.SQLiteExpectedIndex) {
				t.Fatalf("sqlite plan did not use %s:\n%s", check.SQLiteExpectedIndex, plan)
			}
		})
	}
}

func explainSQLiteQueryPlan(t *testing.T, db *sql.DB, query string, args []any) string {
	t.Helper()

	rows, err := db.Query("EXPLAIN QUERY PLAN "+query, args...)
	if err != nil {
		t.Fatalf("running sqlite explain: %v", err)
	}
	defer rows.Close()

	var lines []string
	for rows.Next() {
		var id, parent, notUsed int
		var detail string
		if err := rows.Scan(&id, &parent, &notUsed, &detail); err != nil {
			t.Fatalf("scanning sqlite explain output: %v", err)
		}
		lines = append(lines, detail)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("reading sqlite explain output: %v", err)
	}
	return strings.Join(lines, "\n")
}
