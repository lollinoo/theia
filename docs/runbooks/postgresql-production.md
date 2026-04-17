# PostgreSQL Production Runbook

## Goal
Run Theia on PostgreSQL for staging and production deployments. SQLite remains the local dev and very-small-install path only.

## Default Deployment Path
1. Copy `.env.prod.example` to `.env.prod`.
2. Set `THEIA_ENCRYPTION_KEY`.
3. Set `POSTGRES_PASSWORD` to a non-default value.
4. Start the stack:
   `docker compose -f docker-compose.prod.yml --env-file .env.prod up -d`

The production compose file now defaults the backend to:
- `THEIA_DB_DRIVER=postgres`
- `THEIA_DB_DSN=postgres://theia:${POSTGRES_PASSWORD}@postgres:5432/${POSTGRES_DB}?sslmode=disable`

## External PostgreSQL
If PostgreSQL is managed outside Docker:
1. Override `THEIA_DB_DSN` with the external DSN.
2. Keep `THEIA_DB_DRIVER=postgres`.
3. Ensure the database is reachable from the backend container.
4. Run plan validation after boot:
   `go run ./cmd/theia-db-check -driver postgres -dsn "$THEIA_DB_DSN"`

## Validation
After startup:
1. Check health:
   `curl -sf http://localhost:8080/api/v1/health`
2. Confirm the health payload includes `"db_dialect":"postgres"`.
3. Run PostgreSQL plan validation:
   `go run ./cmd/theia-db-check -driver postgres -dsn "$THEIA_DB_DSN"`

The validator checks the scale-critical queries against the expected indexes:
- `idx_devices_sys_name_lookup`
- `idx_links_pair_lookup`
- `idx_topology_observations_ingest_lookup`
- `idx_unresolved_neighbors_active_lookup`

## Rollback
1. Stop writes to the old backend.
2. Keep the PostgreSQL snapshot or backup untouched.
3. Point the backend back to the prior DSN or prior SQLite file only if a validated rollback path exists.
4. Restore the last known-good application image and database snapshot together.

Do not mix a rolled-back application with a newer database state unless rollback has been exercised in staging.
