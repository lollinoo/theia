package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
)

type PostgresPlanCheck struct {
	Name          string
	Query         string
	Args          []any
	ExpectedIndex string
}

type repositoryPlanCheck struct {
	Name          string
	SQLiteQuery   string
	PostgresQuery string
	Args          []any
	ExpectedIndex string
}

var defaultRepositoryPlanCheckRegistry = []repositoryPlanCheck{
	{
		Name:          "device-by-sysname",
		SQLiteQuery:   `SELECT id FROM devices INDEXED BY idx_devices_sys_name_lookup WHERE sys_name_lookup = ? AND sys_name_lookup != '' ORDER BY updated_at DESC, created_at DESC LIMIT 1`,
		PostgresQuery: `SELECT id FROM devices WHERE sys_name_lookup = $1 ORDER BY updated_at DESC, created_at DESC LIMIT 1`,
		Args:          []any{"core-router"},
		ExpectedIndex: "idx_devices_sys_name_lookup",
	},
	{
		Name:          "link-pair-lookup",
		SQLiteQuery:   `SELECT id FROM links INDEXED BY idx_links_pair_lookup WHERE source_device_id = ? AND target_device_id = ? AND target_if_name = ? AND (source_if_name = ? OR source_if_name = '' OR ? = '')`,
		PostgresQuery: `SELECT id FROM links WHERE source_device_id = $1 AND target_device_id = $2 AND target_if_name = $3 AND (source_if_name = $4 OR source_if_name = '' OR $5 = '')`,
		Args:          []any{"device-a", "device-b", "ether10", "ether1", "ether1"},
		ExpectedIndex: "idx_links_pair_lookup",
	},
	{
		Name:          "observation-ingest-lookup",
		SQLiteQuery:   `SELECT id FROM topology_observations INDEXED BY idx_topology_observations_ingest_lookup WHERE local_device_id = ? AND remote_identity = ? AND local_port = ? AND remote_port = ? AND protocol = ?`,
		PostgresQuery: `SELECT id FROM topology_observations WHERE local_device_id = $1 AND remote_identity = $2 AND local_port = $3 AND remote_port = $4 AND protocol = $5`,
		Args:          []any{"device-a", "core-switch", "ether1", "ether10", "lldp"},
		ExpectedIndex: "idx_topology_observations_ingest_lookup",
	},
	{
		Name:          "unresolved-neighbor-resolution-lookup",
		SQLiteQuery:   `SELECT id FROM unresolved_neighbors INDEXED BY idx_unresolved_neighbors_active_lookup WHERE local_device_id = ? AND remote_identity = ? AND protocol = ? AND resolved_at IS NULL`,
		PostgresQuery: `SELECT id FROM unresolved_neighbors WHERE local_device_id = $1 AND remote_identity = $2 AND protocol = $3 AND resolved_at IS NULL`,
		Args:          []any{"device-a", "unknown-neighbor", "lldp"},
		ExpectedIndex: "idx_unresolved_neighbors_active_lookup",
	},
}

func repositoryPlanChecks() []repositoryPlanCheck {
	checks := make([]repositoryPlanCheck, len(defaultRepositoryPlanCheckRegistry))
	for i, check := range defaultRepositoryPlanCheckRegistry {
		checks[i] = check
		checks[i].Args = append([]any(nil), check.Args...)
	}
	return checks
}

func DefaultPostgresPlanChecks() []PostgresPlanCheck {
	registeredChecks := repositoryPlanChecks()
	checks := make([]PostgresPlanCheck, len(registeredChecks))
	for i, check := range registeredChecks {
		checks[i] = PostgresPlanCheck{
			Name:          check.Name,
			Query:         check.PostgresQuery,
			Args:          append([]any(nil), check.Args...),
			ExpectedIndex: check.ExpectedIndex,
		}
	}
	return checks
}

func ValidatePostgresPlanChecks(ctx context.Context, db *sql.DB, logf func(string, ...any)) error {
	if DetectDialect(db) != DialectPostgres {
		return fmt.Errorf("postgres plan validation requires a postgres database")
	}

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("beginning postgres plan validation tx: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	if _, err := tx.ExecContext(ctx, `SET LOCAL enable_seqscan = off`); err != nil {
		return fmt.Errorf("disabling seqscan for plan validation: %w", err)
	}

	for _, check := range DefaultPostgresPlanChecks() {
		plan, err := explainQueryPlan(ctx, tx, check.Query, check.Args)
		if err != nil {
			return fmt.Errorf("plan check %s: %w", check.Name, err)
		}
		if !strings.Contains(plan, check.ExpectedIndex) {
			return fmt.Errorf("plan check %s did not use %s: %s", check.Name, check.ExpectedIndex, plan)
		}
		if logf != nil {
			logf("postgres plan check ok: %s uses %s", check.Name, check.ExpectedIndex)
		}
	}

	return nil
}

func explainQueryPlan(ctx context.Context, tx *sql.Tx, query string, args []any) (string, error) {
	rows, err := tx.QueryContext(ctx, "EXPLAIN (COSTS OFF) "+query, args...)
	if err != nil {
		return "", fmt.Errorf("running explain: %w", err)
	}
	defer rows.Close()

	var lines []string
	for rows.Next() {
		var line string
		if err := rows.Scan(&line); err != nil {
			return "", fmt.Errorf("scanning explain output: %w", err)
		}
		lines = append(lines, line)
	}
	if err := rows.Err(); err != nil {
		return "", fmt.Errorf("reading explain output: %w", err)
	}
	return strings.Join(lines, "\n"), nil
}
