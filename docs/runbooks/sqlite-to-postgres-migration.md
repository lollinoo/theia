# SQLite To PostgreSQL Migration Runbook

## Goal
Move an existing SQLite-backed Theia instance onto PostgreSQL with a repeatable, auditable path.

## Preconditions
1. Source SQLite database is healthy.
2. Target PostgreSQL database is reachable.
3. Application version on the migration host matches the target deployment version.
4. You have a filesystem backup of the SQLite database file before starting.

## Migration Command
Use the built-in migrator:

```bash
go run ./cmd/theia-db-migrate \
  -config config.yaml \
  -source-sqlite /path/to/theia.db \
  -target-dsn 'postgres://theia:change-me@127.0.0.1:5432/theia?sslmode=disable'
```

Useful flags:
- `-truncate-target` to wipe the target tables before import.
- `-batch-size` to tune insert batch size during copy.

## Cutover Sequence
1. Stop the backend using the source SQLite database.
2. Take a final copy of the SQLite file.
3. Run `theia-db-migrate`.
4. Start the backend against PostgreSQL.
5. Run plan validation:
   `go run ./cmd/theia-db-check -driver postgres -dsn "$THEIA_DB_DSN"`
6. Confirm `/api/v1/health` reports `"db_dialect":"postgres"`.

## Verification Checklist
1. Device count matches pre-migration.
2. Link count matches pre-migration.
3. `topology_observations` and `unresolved_neighbors` row counts are plausible.
4. UI loads devices, links, and live data without rebuild loops.
5. Background polling resumes and LLDP enrichment still converges.

## Rollback
1. Stop the PostgreSQL-backed backend.
2. Restore the backed-up SQLite file if needed.
3. Re-point the backend to SQLite only if the application version is still compatible with that SQLite schema.
4. Treat the PostgreSQL database as failed cutover state until investigated; do not resume dual-write experiments.
