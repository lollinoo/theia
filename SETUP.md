# Theia — Setup Guide

Network topology visualizer with SNMP monitoring, real-time metrics, and link management.

---

## Table of Contents

- [Prerequisites](#prerequisites)
- [Development Environment](#development-environment)
- [Production Environment](#production-environment)
- [Credential Encryption Keyring](#credential-encryption-keyring)
- [Configuration Reference](#configuration-reference)
- [API Quick Reference](#api-quick-reference)
- [Resumable Runtime Stream Operations](#resumable-runtime-stream-operations)
- [Troubleshooting](#troubleshooting)

---

## Prerequisites

| Tool | Version | Purpose |
|------|---------|---------|
| Docker | 24+ | All services run in containers |
| Docker Compose | v2 (plugin) | Orchestrates the stack |
| GNU Make | 4+ recommended | Convenience commands |
| PowerShell | 5.1+ on Windows | Windows-native Make recipes |
| `curl` | any | Seed script / API testing |

No Go or Node.js installation is required — the build happens inside Docker.

Windows is supported with native GNU Make plus PowerShell and Docker Desktop. Git Bash and WSL are not required for Makefile targets. On macOS and Linux, the Makefile keeps using the existing POSIX shell scripts.

Docker Desktop networking differs from native Linux networking. The standard development compose stack and the WISP lab publish explicit host ports and avoid `network_mode: host`. WISP seed commands auto-detect the Docker backend container and use the lab management network on Windows/macOS.

---

## Development Environment

The dev stack runs the application locally with hot-reload for both backend and frontend, plus PostgreSQL and the optional Prometheus/SNMP exporter metrics path.

### Stack

| Service | URL | Description |
|---------|-----|-------------|
| Backend | http://localhost:8080 | Go API with Air hot-reload |
| Frontend | http://localhost:3000 | React + Vite dev server |
| PostgreSQL | `127.0.0.1:5432` | Bundled development database |
| Prometheus | http://localhost:9090 | Metrics and alerting |
| SNMP exporter | http://localhost:9116 | Prometheus SNMP scrape adapter |

### 1. Clone and start

```bash
git clone <repo-url>
cd theia
make dev
```

This builds all images and starts the full stack in the background. First build takes 2-4 minutes to compile Go and download npm packages.

Open http://localhost:3000 and sign in as `administrator` with password `theia`. The first login requires changing that password before normal API and UI workflows continue.

`config.yaml` is a local-only file and is ignored by git because it can contain a database DSN or other deployment-specific values. The dev stack works through environment variables without it. If you need a local config file, start from the template:

```bash
cp config.example.yaml config.yaml
```

### 2. Verify everything is up

```bash
docker compose ps
# All services should show "healthy" or "running"

curl -s http://localhost:8080/api/v1/health
# {"status":"ok"}
```

### 3. Add devices

```bash
make seed
```

This calls the REST API to register sample SNMP devices. Those addresses must be reachable from the backend container; the standard `docker-compose.yml` no longer starts local SNMP simulator containers.

For a self-contained lab topology, start the WISP lab and seed it instead:

```bash
make wisp-lab
make wisp-seed-all
```

After seeding reachable devices, the backend probes them immediately via SNMP and the canvas will populate within ~10 seconds.

Open http://localhost:3000 to see the topology.

### 4. Useful dev commands

```bash
make logs           # Tail backend logs
make stop           # Stop all containers
make clean          # Stop + delete volumes (resets the database)
make test           # Run unit tests
make test-integration  # Run integration-tagged tests
```

### 4.1 Database

PostgreSQL is the standard database backend for Theia in development, staging, and production. The normal `make dev` flow starts the backend against the bundled PostgreSQL service and publishes PostgreSQL on `127.0.0.1:5432` for host tools.

Instance backup / restore is supported on PostgreSQL deployments when compatible PostgreSQL client tools are available on `PATH`. PostgreSQL backup jobs require `pg_dump` 18.x; restore validation, staging, and apply require `pg_restore` 18.x; non-dry-run restore apply also requires `pg_dump` 18.x so Theia can take a pre-restore live database backup before changing the database, and `psql` 18.x so Theia can reset the target schema before loading the staged dump.

### 4.2 PostgreSQL client tools

The bundled development, staging, and production compose stacks use `postgres:18-bookworm`. Official backend images bundle PostgreSQL 18 client tools from the same image; custom host or custom image deployments must place compatible tools on `PATH`.

| Tool | Requirement | Supported version |
|------|-------------|-------------------|
| `pg_dump` | Required for PostgreSQL instance backup and pre-restore live DB backup | 18.x |
| `pg_restore` | Required for PostgreSQL restore validation and apply | 18.x |
| `psql` | Required for PostgreSQL restore apply schema cleanup | 18.x |

Missing or incompatible PostgreSQL client tools fail the backup or restore job at startup with actionable diagnostics. Error output redacts connection strings and passwords.

PostgreSQL 18 Docker images store cluster data in a major-version-specific directory below `/var/lib/postgresql`; the bundled compose stacks therefore mount their named database volumes at `/var/lib/postgresql`. PostgreSQL 17 and earlier used `/var/lib/postgresql/data`.

Existing PostgreSQL 17 volumes are not upgraded by changing the image tag or mount target. Before deploying this change over a persistent PostgreSQL 17 environment, stop application writes, take and verify a database backup, then migrate with `pg_dump`/`pg_restore` into a fresh PostgreSQL 18 volume or run `pg_upgrade` with both major versions available. Keep the original PostgreSQL 17 volume until the PostgreSQL 18 restore and application checks have completed successfully.

The compose stacks use distinct volume names so an existing PostgreSQL 17 cluster is never mounted into PostgreSQL 18 automatically:

| Stack | PostgreSQL 17 volume | PostgreSQL 18 volume |
|-------|----------------------|----------------------|
| Development | `theia-postgres-data` | `theia-postgres18-data` |
| Staging | `theia-staging-postgres-data` | `theia-staging-postgres18-data` |
| Production | `theia-prod-postgres-data` | `theia-prod-postgres18-data` |

#### Migrating bundled production PostgreSQL 17 to 18

The following dump/restore procedure preserves the original PostgreSQL 17 volume. Run every command in the same shell from the repository root with a completed `.env.prod`. Do not run `make prod-clean`, `docker compose down -v`, or `docker volume rm` during the migration.

Load the production environment and derive `THEIA_DB_DSN` when it is intentionally left empty in `.env.prod`, matching the behavior of the production Make targets:

```bash
set -e
set -a
. ./.env.prod
set +a

if [ -z "$THEIA_DB_DSN" ]; then
  : "${POSTGRES_PASSWORD:?POSTGRES_PASSWORD must be set}"
  THEIA_DB_DSN="$(printf '%s://%s:%s@postgres:5432/%s?sslmode=disable' \
    postgres "${POSTGRES_USER:-theia}" "$POSTGRES_PASSWORD" "${POSTGRES_DB:-theia}")"
  export THEIA_DB_DSN
fi
```

First stop the production stack without removing volumes, then start PostgreSQL 17 temporarily against the legacy volume:

```bash
docker compose -f docker-compose.prod.yml --env-file .env.prod down

docker run --detach --rm \
  --name theia-postgres17-migrate \
  --env-file .env.prod \
  --volume theia-prod-postgres-data:/var/lib/postgresql/data \
  postgres:17-bookworm

postgres17_ready=false
for attempt in $(seq 1 30); do
  if docker exec theia-postgres17-migrate \
    sh -c 'pg_isready -U "$POSTGRES_USER" -d "$POSTGRES_DB"'; then
    postgres17_ready=true
    break
  fi

  if [ "$(docker inspect --format '{{.State.Running}}' \
    theia-postgres17-migrate 2>/dev/null || true)" != "true" ]; then
    break
  fi
  sleep 2
done

if [ "$postgres17_ready" != "true" ]; then
  docker logs theia-postgres17-migrate || true
  docker stop theia-postgres17-migrate || true
  exit 1
fi
```

Create a custom-format dump on the host and verify that PostgreSQL can read its archive catalog before stopping the temporary PostgreSQL 17 container. This catalog check catches an unreadable archive structure; the full payload is validated by the atomic restore into PostgreSQL 18 in the next step:

```bash
mkdir -p backups
docker exec theia-postgres17-migrate \
  sh -c 'pg_dump -U "$POSTGRES_USER" -d "$POSTGRES_DB" --format=custom' \
  > backups/theia-postgres17.dump

test -s backups/theia-postgres17.dump
docker exec -i theia-postgres17-migrate pg_restore --list \
  < backups/theia-postgres17.dump > /dev/null

docker stop theia-postgres17-migrate
```

Start only PostgreSQL 18. Compose creates and mounts the new `theia-prod-postgres18-data` volume, leaving `theia-prod-postgres-data` unchanged. Restore the archive into the freshly initialized database in a single transaction. `pg_restore` must exit successfully before starting any other service; otherwise the transaction is rolled back and `set -e` stops the procedure:

```bash
docker compose -f docker-compose.prod.yml --env-file .env.prod \
  up -d --wait postgres

docker compose -f docker-compose.prod.yml --env-file .env.prod \
  exec -T postgres sh -c \
  'pg_restore -U "$POSTGRES_USER" -d "$POSTGRES_DB" --clean --if-exists --no-owner --no-privileges --single-transaction --exit-on-error' \
  < backups/theia-postgres17.dump
```

Confirm the server version and database identity, then start the remaining services and validate application data, authentication, settings, devices, and backups:

```bash
docker compose -f docker-compose.prod.yml --env-file .env.prod \
  exec -T postgres sh -c \
  'psql -U "$POSTGRES_USER" -d "$POSTGRES_DB" -c "SELECT current_database(), current_user, version();"'

docker compose -f docker-compose.prod.yml --env-file .env.prod up -d
```

Retain both `backups/theia-postgres17.dump` and the `theia-prod-postgres-data` volume until the PostgreSQL 18 deployment has passed application checks and a new PostgreSQL 18 backup has been restore-validated.

### 5. How hot-reload works

**Backend** — Air watches `internal/` and `cmd/`. On any `.go` file change, it recompiles and restarts the server automatically. The `./tmp/` directory holds the compiled binary; do not commit it.

**Frontend** — Vite's HMR is active on port 3000. The dev server proxies `/api/*` and `/api/v1/ws` to the backend at `http://backend:8080`, so the frontend container talks to the backend over the internal Docker network.

### 6. Adding a real device (optional)

If you have a real SNMP-capable device on the same network:

```bash
curl -X POST http://localhost:8080/api/v1/devices \
  -H "Content-Type: application/json" \
  -d '{
    "ip": "192.168.1.1",
    "hostname": "my-router",
    "snmp": {
      "version": "2c",
      "community": "public"
    },
    "tags": {"vendor": "mikrotik", "role": "gateway"}
  }'
```

---

## WISP Lab

If you want a self-contained simulated topology, this repo also includes a dedicated 10-router MikroTik-flavoured WISP lab with active OSPF and SNMP/LLDP discovery data for Theia.

### What it gives you

- 10 simulated MikroTik routers
- Real OSPF adjacencies via FRRouting inside the lab containers
- Static SNMP + LLDP data so Theia can discover interfaces and links
- A separate Prometheus instance on `http://localhost:9091`
- A Docker management network for SNMP targets at `172.31.250.21` through `172.31.250.42`

### Topology shape

- `wisp-core-01`, `wisp-core-02`: backbone core
- `wisp-pop-north-01`, `wisp-pop-south-01`: POP aggregation / ABR
- `wisp-ix-edge-01`: internet edge that originates default route
- `wisp-dc-agg-01`: datacenter aggregation
- `wisp-tower-{north,south}-{01,02}`: access tower routers

The lab uses OSPF area `0.0.0.0` for the backbone, `0.0.0.10` for the north tower domain, and `0.0.0.20` for the south tower domain.

### Start the lab

```bash
make wisp-lab
```

This starts only the lab containers plus a dedicated Prometheus at `http://localhost:9091`. It does not replace the normal Theia dev stack.
The lab now includes an internal transit router used only for eBGP with `wisp-ix-edge-01`, while the 10 MikroTik WISP routers remain the seeded topology shown in Theia.
If the Docker backend container is already running, `make wisp-lab` also connects it to the WISP management network so SNMP probes work from Docker Desktop.

If Theia is not already running, start it separately:

```bash
make dev
```

### Seed the 10 routers into Theia

```bash
make wisp-seed
```

The seed script defaults to `WISP_SEED_TARGET_MODE=auto`. With the Docker backend container running, it registers the routers at `172.31.250.21` through `172.31.250.30` and connects the backend to the lab network if needed. If no backend container is running, it falls back to host loopback targets `127.0.10.21` through `127.0.10.30`.
Seed scripts prompt for Theia credentials and authenticate with the same cookie session flow as the browser. When run interactively against a fresh dev database, they can complete the first-login password change before seeding.

### Seed the radio access layer

```bash
make wisp-radio-seed
```

This adds 4 sector APs and 8 subscriber CPE nodes on the same target range: `172.31.250.31` through `172.31.250.42` for Docker-backed Theia, or `127.0.10.31` through `127.0.10.42` in host-loopback mode. The AP sector interfaces simulate PtMP by advertising multiple LLDP neighbors on the same wireless interface.

Override the auto-detection only when needed:

```bash
make wisp-seed-all WISP_SEED_TARGET_MODE=docker
make wisp-seed-all WISP_SEED_TARGET_MODE=host
```

For a fresh environment you can seed everything in one pass:

```bash
make wisp-seed-all
```

### Verify OSPF

```bash
make wisp-ospf
```

This executes `show ip ospf neighbor` on each simulated router and prints the adjacency table.

### Verify BGP and default-route propagation

```bash
make wisp-bgp
```

This checks the eBGP session between `wisp-ix-edge-01` and the internal transit router, then shows the default route on the edge and on representative OSPF routers to confirm propagation.

### Stop the lab

```bash
make wisp-lab-down
```

---

## Production Environment

The production stack uses compiled images — no hot-reload, no source mounts, no SNMP simulators. The frontend is built into a static bundle served by nginx, which also proxies `/api` to the backend.

### Stack

| Service | URL | Description |
|---------|-----|-------------|
| Frontend | http://localhost:80 | nginx serving React SPA + API proxy |
| Backend | internal-only | Compiled Go binary behind the frontend proxy |
| PostgreSQL | `127.0.0.1:5432` | Standard bundled production database |
| Prometheus | http://localhost:9090 | Metrics (optional, `--profile metrics`) |
| SNMP exporter | http://localhost:9116 | Scrape adapter (optional, `--profile metrics`) |

### 1. Configure environment

`.env.prod.example` is a placeholder template only. Copy it to `.env.prod`, fill the required values locally, and keep `.env.prod` plus any production `config.yaml` override local and untracked because they can contain deployment secrets.

Production startup runs strict secret validation because `THEIA_DEPLOYMENT_ENV=production` is set. The backend rejects missing or example secret values before opening the database.

Production pulls the stable release image channel by default:

```text
IMAGE_TAG=latest
```

The CI workflow publishes the `master` image channel from the `master` branch for internal validation. For public production deployments, prefer `latest` for the newest stable GitHub Release or a pinned version tag such as `v1.6.0`. For every non-`master` branch push, CI publishes branch and `sha-<shortsha>` image tags for both backend and frontend. Branch image tags are generated from the branch name and sanitized by Docker metadata-action for Docker tag compatibility.

Publishing a stable GitHub Release builds backend and frontend images for the release tag, for example `v1.6.0`, and also updates the `latest` image tag.

Required operator inputs for the standard bundled PostgreSQL stack:

- `THEIA_ENCRYPTION_KEY_ID`
- `THEIA_ENCRYPTION_KEYS`
- `THEIA_SESSION_SECRET`
- `THEIA_METRICS_TOKEN`
- `THEIA_DB_DSN`
- `POSTGRES_PASSWORD` for the bundled `postgres` service

If you restore or start against data that was previously protected by the
legacy `THEIA_ENCRYPTION_KEY`, keep that old secret configured as key id
`legacy` until startup has rewrapped credentials with the active key and you
have created and restore-validated a fresh backup. See
[Credential Encryption Keyring](#credential-encryption-keyring) for the full
restore and rotation procedure:

```text
THEIA_ENCRYPTION_KEY_ID=kid-prod-2026-06
THEIA_ENCRYPTION_KEYS=kid-prod-2026-06=<new-secret>,legacy=<old-THEIA_ENCRYPTION_KEY>
```

Alternatively, while the new keyring variables are set, `THEIA_ENCRYPTION_KEY`
is still accepted as a compatibility fallback and is loaded as key id `legacy`.

For bundled PostgreSQL, use this DSN shape:

```text
postgres://<postgres-user>:<postgres-password>@postgres:5432/<postgres-db>?sslmode=disable
```

Unix Makefile targets derive `THEIA_DB_DSN` from `POSTGRES_USER`, `POSTGRES_PASSWORD`, and `POSTGRES_DB` when `THEIA_DB_DSN` is blank and the bundled PostgreSQL service is used. Direct `docker compose` commands require `THEIA_DB_DSN` to be set explicitly. If the password contains URL-reserved characters or you use an external PostgreSQL service, set `THEIA_DB_DSN` explicitly with a URL-encoded password.

```bash
cp .env.prod.example .env.prod
# Fill required operator-provided values before first start
```

### 2. Start the stack

```bash
make prod
```

`make prod` starts the standard production stack on PostgreSQL using the bundled `postgres` service from `docker-compose.prod.yml`. If you need an external PostgreSQL service, use a custom compose override and provide `THEIA_DB_DSN`.

To build the production images from the checked-out source tree instead of pulling GHCR images:

```bash
make prod-build
```

This uses `docker-compose.prod-build.yml` as an override and preserves the same production runtime shape.

Or with the metrics stack (Prometheus + SNMP exporter):

```bash
make prod-metrics
```

Open `http://localhost` in the browser. Use `http://localhost/api/v1/...` for API requests through the frontend proxy. Sign in as `administrator` with password `theia` on first start, then change the password when prompted.

### 3. Add your network devices

Via the UI Settings panel, or directly via the API:

```bash
curl -X POST http://localhost/api/v1/devices \
  -b cookies.txt \
  -H "X-CSRF-Token: <theia_csrf-cookie-value>" \
  -H "Content-Type: application/json" \
  -d '{
    "ip": "192.168.1.1",
    "hostname": "core-router",
    "snmp": {"version": "2c", "community": "public"},
    "tags": {"vendor": "mikrotik", "role": "gateway", "site": "hq"}
  }'
```

### 4. Configure SNMP scrape targets (metrics profile only)

Edit `docker/prometheus/prometheus.prod.yml` and add your device IPs:

```yaml
static_configs:
  - targets:
      - 192.168.1.1   # core-router
      - 192.168.1.2   # dist-switch
```

Then in the Theia UI Settings panel, set the Prometheus URL to a host-reachable Prometheus address for the Docker host/network, for example `http://<docker-host-address>:9090`.

Restart Prometheus to pick up the new targets:

```bash
docker compose -f docker-compose.prod.yml --env-file .env.prod --profile metrics restart prometheus
```

### 5. Production commands

```bash
make prod           # Start backend + frontend
make prod-build     # Build backend + frontend locally, then start production
make prod-metrics   # Start with Prometheus + SNMP exporter
make prod-down      # Stop all production containers
make prod-logs      # Follow backend logs
make prod-clean     # Stop + delete volumes (resets database)
```

---

## Staging Environment

The staging stack pulls pre-built images from GHCR and uses different default ports so it can run next to production.

### 1. Configure environment

`.env.staging.example` is a placeholder template only. Copy it to `.env.staging`, fill the required values locally, and keep `.env.staging` plus any staging `config.yaml` override local and untracked because they can contain deployment secrets.

Staging startup runs strict secret validation because `THEIA_DEPLOYMENT_ENV=staging` is set. The backend rejects missing or example secret values before opening the database.

Staging pulls the `master` image channel by default. To deploy a branch build, set `IMAGE_TAG` in `.env.staging` to the sanitized branch tag published by CI. Branch tags are generated by Docker metadata-action from the branch name, for example `feature/prometheus-fix` becomes `feature-prometheus-fix`.

Required operator inputs for the standard bundled PostgreSQL stack:

- `THEIA_ENCRYPTION_KEY_ID`
- `THEIA_ENCRYPTION_KEYS`
- `THEIA_SESSION_SECRET`
- `THEIA_METRICS_TOKEN`
- `THEIA_DB_DSN`
- `POSTGRES_PASSWORD` for the bundled `postgres` service

If you restore or start against data that was previously protected by the
legacy `THEIA_ENCRYPTION_KEY`, keep that old secret configured as key id
`legacy` until startup has rewrapped credentials with the active key and you
have created and restore-validated a fresh backup. See
[Credential Encryption Keyring](#credential-encryption-keyring) for the full
restore and rotation procedure:

```text
THEIA_ENCRYPTION_KEY_ID=kid-staging-2026-06
THEIA_ENCRYPTION_KEYS=kid-staging-2026-06=<new-secret>,legacy=<old-THEIA_ENCRYPTION_KEY>
```

Alternatively, while the new keyring variables are set, `THEIA_ENCRYPTION_KEY`
is still accepted as a compatibility fallback and is loaded as key id `legacy`.

For bundled PostgreSQL, use this DSN shape:

```text
postgres://<postgres-user>:<postgres-password>@postgres:5432/<postgres-db>?sslmode=disable
```

Unix Makefile targets derive `THEIA_DB_DSN` from `POSTGRES_USER`, `POSTGRES_PASSWORD`, and `POSTGRES_DB` when `THEIA_DB_DSN` is blank and the bundled PostgreSQL service is used. Direct `docker compose` commands require `THEIA_DB_DSN` to be set explicitly. If the password contains URL-reserved characters or you use an external PostgreSQL service, set `THEIA_DB_DSN` explicitly with a URL-encoded password.

```bash
cp .env.staging.example .env.staging
# Fill required operator-provided values before first start
```

### 2. Start the stack

```bash
make staging
```

`make staging` starts the standard staging stack on PostgreSQL using the bundled `postgres` service from `docker-compose.staging.yml`. If you need an external PostgreSQL service, use a custom compose override and provide `THEIA_DB_DSN`.

Example branch deployment:

```bash
IMAGE_TAG=feature-runtime-channel docker compose -f docker-compose.staging.yml --env-file .env.staging up -d
```

Default staging ports:

- Frontend: `http://localhost:3001`
- Backend: internal-only in the shipped staging compose stack unless you publish it separately
- PostgreSQL: `127.0.0.1:5433` for the bundled staging database

### 3. Staging commands

```bash
make staging           # Start staging stack
make staging-down      # Stop all staging containers
make staging-logs      # Follow staging backend logs
```

---

## Credential Encryption Keyring

The backend encrypts stored credential secrets with a versioned keyring. The
active key is used for all new writes, and non-active keys remain available so
startup migrations can decrypt and rewrap older values.

Encrypted data includes device SNMP credentials, SNMP profile credentials, and
credential profile secrets stored in PostgreSQL. These values are normalized
during backend startup after SQL migrations complete.

### Variables

Use the keyring variables for production and staging:

```env
THEIA_ENCRYPTION_KEY_ID=<active-key-id>
THEIA_ENCRYPTION_KEYS=<key-id>=<secret>[,<key-id>=<secret>...]
```

Example:

```env
THEIA_ENCRYPTION_KEY_ID=kid-staging-2026-06
THEIA_ENCRYPTION_KEYS=kid-staging-2026-06=<secret>
```

`THEIA_ENCRYPTION_KEY_ID` must match one entry in `THEIA_ENCRYPTION_KEYS`.
The active key id is written into every new encryption envelope. Keep key ids
stable and descriptive, for example `kid-prod-2026-06` or
`kid-staging-2026-09`.

### Legacy Key Compatibility

Deployments that previously used only `THEIA_ENCRYPTION_KEY` may have stored
data encrypted under key id `legacy`. During migration or restore, provide the
old secret as a keyring entry named `legacy`:

```env
THEIA_ENCRYPTION_KEY_ID=kid-staging-2026-06
THEIA_ENCRYPTION_KEYS=kid-staging-2026-06=<new-secret>,legacy=<old-THEIA_ENCRYPTION_KEY>
```

As a compatibility fallback, if the new keyring variables are set and
`THEIA_ENCRYPTION_KEY` is also present, the backend loads that value as key id
`legacy` unless `THEIA_ENCRYPTION_KEYS` already contains `legacy=...`.

If startup fails with an error like:

```text
archive or ciphertext requires encryption key id "legacy", but it is not configured
```

the database or restored backup still contains at least one encrypted value that
requires the legacy secret. Add `legacy=<old-secret>` to
`THEIA_ENCRYPTION_KEYS`, or provide the old value through `THEIA_ENCRYPTION_KEY`,
then restart the backend.

### First Migration From Legacy

Use this sequence when moving an existing deployment from `THEIA_ENCRYPTION_KEY`
to the keyring variables:

1. Generate a new high-entropy secret for the active key.
2. Configure the new key and the old legacy key together:

```env
THEIA_ENCRYPTION_KEY_ID=kid-prod-2026-06
THEIA_ENCRYPTION_KEYS=kid-prod-2026-06=<new-secret>,legacy=<old-THEIA_ENCRYPTION_KEY>
```

3. Start the backend and confirm startup migrations complete without encryption
   errors.
4. Create a fresh instance backup after the successful startup.
5. Restore-test that backup with both keys still configured.
6. Restore-test or restart with only the new key:

```env
THEIA_ENCRYPTION_KEY_ID=kid-prod-2026-06
THEIA_ENCRYPTION_KEYS=kid-prod-2026-06=<new-secret>
```

If the backend starts without a `requires encryption key id "legacy"` error, the
stack no longer needs `legacy` in `.env`. Keep the old secret in your secret
manager until all backups created before the migration have expired or are no
longer part of your recovery plan.

### Rotating A Keyring Secret

To rotate from one keyring secret to another, keep the old and new keys
configured during the migration window and make the new key active.

Current state:

```env
THEIA_ENCRYPTION_KEY_ID=kid-staging-2026-06
THEIA_ENCRYPTION_KEYS=kid-staging-2026-06=<current-secret>
```

Rotated state:

```env
THEIA_ENCRYPTION_KEY_ID=kid-staging-2026-09
THEIA_ENCRYPTION_KEYS=kid-staging-2026-06=<current-secret>,kid-staging-2026-09=<new-secret>
```

Then:

1. Restart the backend.
2. Confirm startup migrations complete without encryption errors.
3. Create a fresh backup after the successful startup.
4. Restore-test or restart once with both keys configured.
5. Restore-test or restart with only the new active key:

```env
THEIA_ENCRYPTION_KEY_ID=kid-staging-2026-09
THEIA_ENCRYPTION_KEYS=kid-staging-2026-09=<new-secret>
```

If the backend starts without a `requires encryption key id
"kid-staging-2026-06"` error, the old key can be removed from the stack `.env`.
Keep the old secret in a secret manager until backups created before the
rotation have expired.

### Operational Rules

- Do not change the secret for an existing key id. Create a new key id for every
  rotation.
- Do not remove an old key before the backend has started successfully with the
  old and new keys together.
- Do not delete old secrets from your secret manager while backups that require
  them are still retained.
- Never commit real `THEIA_ENCRYPTION_KEYS`, `THEIA_ENCRYPTION_KEY`, database
  passwords, session secrets, or metrics tokens.

---

## Configuration Reference

### Backend

Configuration is loaded from local `config.yaml` when present. The tracked `config.example.yaml` is a placeholder template; copy it to `config.yaml` only when you need local file-based overrides. All keys can be overridden with environment variables.

| config.yaml key | Environment variable | Default | Description |
|-----------------|---------------------|---------|-------------|
| `deployment_env` | `THEIA_DEPLOYMENT_ENV` | `development` | Must be `development`, `staging`, or `production`; staging and production enforce required secret validation |
| none | `THEIA_ENCRYPTION_KEY_ID` | none | Active credential encryption key id; required with `THEIA_ENCRYPTION_KEYS` in production and staging |
| none | `THEIA_ENCRYPTION_KEYS` | none | Comma-separated credential encryption keyring entries in `<key-id>=<secret>` format |
| none | `THEIA_ENCRYPTION_KEY` | none | Legacy credential encryption fallback; when keyring variables are set, this value is loaded as key id `legacy` |
| `listen_addr` | `THEIA_LISTEN_ADDR` | `:8080` | HTTP server bind address |
| `db_dsn` | `THEIA_DB_DSN` | none | PostgreSQL DSN; Unix Makefile production and staging targets derive it for bundled PostgreSQL when blank, while direct Compose, external database, and non-compose deployments must provide it through local config, local env, or a secret manager |
| `data_dir` | `THEIA_DATA_DIR` | `./data` | Local app data directory for known_hosts and backup files |
| `bridge_binaries_dir` | `THEIA_BRIDGE_BINARIES_DIR` | `` | Directory containing pre-built bridge binaries; compose defaults to `/data/bridge_binaries` for staging and production |
| `session_secret` | `THEIA_SESSION_SECRET` | none | Secret used to protect first-party password sessions; Required whenever the backend initializes first-party password auth |
| `metrics_token` | `THEIA_METRICS_TOKEN` | none | Bearer token for `/metrics`; required for staging and production runtime startup |
| `allowed_origins` | `THEIA_ALLOWED_ORIGINS` | none | Optional comma-separated exact browser origins for direct backend REST/WebSocket access; same-host proxy requests are allowed |
| `log_level` | `THEIA_LOG_LEVEL` | `info` | Log verbosity: `debug`, `info`, `warn`, `error` |

Runtime settings (poll interval, Prometheus URL, Grafana URL) are stored in the database and configurable via the Settings panel in the UI — no restart needed.

All protected `/api/v1` routes use the first-party password session cookie `theia_session`. Mutating requests must also send the `X-CSRF-Token` header with the value from the readable `theia_csrf` cookie. `/metrics` uses `THEIA_METRICS_TOKEN`.

### User Settings and Bridge Connector

Every authenticated user can open **User Settings** from the account menu. The account section allows safe self-service profile updates; privileged user fields such as roles, status, and admin flags remain server-controlled.

Bridge Connector authentication is per-user. The legacy global `bridge_secret` runtime setting is deprecated and ignored by bridge authentication. Existing deployments should have each user generate a personal Bridge Secret from **User Settings -> Bridge Connector** and paste it into their local connector config.

The recommended setup flow is wizard-first:

1. Generate a personal Bridge Secret from **User Settings -> Bridge Connector**.
2. Download and start the Bridge Connector for your platform.
3. Click **Configure Local Connector** in User Settings, or use the tray menu item **Setup Connector...**.
4. In the local setup wizard, select the WinBox executable, paste the Bridge Secret, confirm the Theia URLs and bridge port, and optionally click **Install / Repair Connector** before enabling start-at-login.
5. Save the wizard and restart the connector when the wizard reports that a restart is required.

The setup wizard runs from the local connector at `http://localhost:<bridge-port>/setup`. Setup endpoints are limited to loopback requests and do not return the saved Bridge Secret. When start-at-login is enabled, the wizard first installs or repairs a copy of the connector in a stable per-user location and points the OS autostart entry to that installed copy. If the original downloaded executable is later deleted, autostart continues to use the installed copy; if that installed copy is missing, the wizard reports autostart as needing repair.

Stable connector install paths:

- Windows: `%LOCALAPPDATA%\Theia\WinBoxBridge\winbox-bridge.exe`
- macOS: `~/Library/Application Support/Theia/WinBoxBridge/winbox-bridge`
- Linux: `${XDG_DATA_HOME:-~/.local/share}/theia/winbox-bridge/winbox-bridge`

The connector still stores its local runtime settings in `config.json`, which remains available from the tray menu as an advanced fallback.

Advanced connector config shape:

```json
{
  "winbox_path": "",
  "listen_port": 1337,
  "theia_origin": "http://localhost:3000",
  "theia_base_url": "http://localhost:3000",
  "bridge_secret": "<paste-secret-shown-once>",
  "log_level": "info"
}
```

The Bridge Secret is shown only once after generation or rotation. If a user loses it, rotate the secret and update the connector through the setup wizard; the previous secret stops working immediately. Connector downloads are served only to authenticated users and require `THEIA_BRIDGE_BINARIES_DIR` to point at a directory containing files named like `winbox-bridge-linux-amd64` or `winbox-bridge-windows-amd64.exe`.

CI builds Bridge Connector executables for Linux, Windows, and macOS on pull requests, branch pushes that touch bridge code or build scripts, and manual promotion runs. Download the workflow artifacts and place the six `winbox-bridge-*` files in the backend container path configured by `THEIA_BRIDGE_BINARIES_DIR`, or mount a host directory there in a compose override.

For browser access through a LAN IP or alternate hostname, add the exact browser origin to `THEIA_ALLOWED_ORIGINS` before logging in. Example:

```bash
THEIA_ALLOWED_ORIGINS=http://localhost:3000,http://192.168.1.30:3000
```

Same-host frontend proxy requests do not need this setting, but direct cross-origin REST/WebSocket requests do.

Bridge migration notes:

- Run the new database migration before users configure connectors.
- Do not copy the old global `bridge_secret` to users; each user must generate a unique personal secret.
- Old connector configs that only relied on the global secret must be updated with the local setup wizard or manually with `theia_base_url`, `theia_origin`, and the personal `bridge_secret`.
- Use HTTPS for non-local deployments because the connector sends the Bridge Secret to Theia during launch resolution.

### Frontend (build-time)

| Variable | Default | Description |
|----------|---------|-------------|
| `VITE_API_URL` | `http://backend:8080` | Backend base URL — used by Vite proxy in dev; baked into bundle for production |

---

## API Quick Reference

Base path:

- Development: `http://localhost:8080/api/v1`
- Production: `http://localhost/api/v1`
- Staging: `http://localhost:3001/api/v1`

Programmatic API clients should first `POST /auth/login` with `{"identifier":"<username>","password":"<password>"}` and store the returned `theia_session` and `theia_csrf` cookies. Send the cookies on API requests; also send `X-CSRF-Token: <theia_csrf>` on POST, PUT, PATCH, and DELETE requests. One-time admin reset tokens are redeemed through public `POST /auth/password/reset` with `{"token":"<token>","new_password":"<new password>"}`.

| Method | Path | Description |
|--------|------|-------------|
| POST | `/auth/login` | Start a password session |
| GET | `/auth/me` | Get the current password session |
| POST | `/auth/password/change` | Change password for the current session |
| POST | `/auth/password/reset` | Complete a one-time password reset token |
| GET | `/health` | Health check |
| GET | `/devices` | List all devices |
| POST | `/devices` | Add a device |
| GET | `/devices/:id` | Get device by ID |
| PUT | `/devices/:id` | Update device |
| DELETE | `/devices/:id` | Delete device |
| POST | `/devices/:id/probe` | Force SNMP probe now |
| GET | `/devices/:id/interfaces` | List SNMP-detected interfaces |
| GET | `/links` | List all topology links |
| POST | `/links` | Create a manual link |
| GET | `/links/:id` | Get link by ID |
| PUT | `/links/:id` | Update link port assignments |
| DELETE | `/links/:id` | Delete a link |
| GET | `/settings` | Get runtime settings |
| PUT | `/settings` | Update runtime settings |
| GET | `/runtime/overview` | Get an uncached runtime-only recovery snapshot |
| GET | `/ws` | WebSocket: live metrics stream |

### Device payload example

Legacy single-address clients can continue to send only `ip`. The backend treats `ip` as the primary address and returns it for compatibility:

```json
{
  "ip": "192.168.1.1",
  "hostname": "core-router",
  "snmp": {
    "version": "2c",
    "community": "public"
  },
  "tags": {
    "vendor": "mikrotik",
    "role": "gateway",
    "site": "hq",
    "display_name": "Core Router"
  }
}
```

Multi-address clients can include `addresses`. When both `ip` and `addresses` are provided, `ip` remains authoritative and is normalized as the primary address:

```json
{
  "ip": "192.168.1.1",
  "hostname": "core-router",
  "addresses": [
    {
      "address": "192.168.1.1",
      "role": "primary",
      "is_primary": true,
      "priority": 0
    },
    {
      "address": "192.0.2.10",
      "label": "OOB",
      "role": "backup",
      "priority": 10
    }
  ],
  "snmp": {
    "version": "2c",
    "community": "public"
  }
}
```

Device responses include both the legacy `ip` field and address metadata:

```json
{
  "data": {
    "id": "<uuid>",
    "attributes": {
      "hostname": "core-router",
      "ip": "192.168.1.1",
      "addresses": [
        {
          "id": "<uuid>",
          "device_id": "<uuid>",
          "address": "192.168.1.1",
          "label": "",
          "role": "primary",
          "is_primary": true,
          "priority": 0,
          "created_at": "2026-06-09T00:00:00Z",
          "updated_at": "2026-06-09T00:00:00Z"
        }
      ]
    }
  }
}
```

Backup jobs select targets by address role: `backup` first, then `management`, then the primary `ip`.

### Link payload example

```json
{
  "source_device_id": "<uuid>",
  "source_if_name": "ether1",
  "target_device_id": "<uuid>",
  "target_if_name": "GigabitEthernet1/0/1"
}
```

---

## Resumable Runtime Stream Operations

The protocol v2 runtime stream keeps canvas runtime state current without repeatedly reloading structural topology. A v2 client declares `runtime_protocol=2` and identifies its last applied cursor with `runtime_stream_id` and `runtime_version`. The stream ID identifies one backend runtime lineage; the version advances monotonically within that lineage. On reconnect, the client reports its cursor and the backend selects the smallest safe response:

| Mode | When it is selected | Result |
|------|---------------------|--------|
| `current` | The client cursor exactly matches the backend cursor | A `ready` barrier confirms that no runtime payload is needed |
| `replay` | The stream ID matches and the bounded in-process journal still covers the missing versions | A compact `runtime_replay` advances the client to the current cursor |
| `snapshot` | The cursor is missing, belongs to another lineage, is outside the journal, or queued deltas must be replaced | An atomic runtime snapshot replaces the stale runtime state |
| `http_fallback` | WebSocket recovery does not complete before the client timeout | The client fetches the compact runtime-only HTTP snapshot |

Runtime deltas are contiguous: their `base_version` must match the client's applied `runtime_version`. After applying `ready`, replay, snapshot, or live delta state, a v2 client sends `runtime_ack` for the applied stream ID and version. The backend accepts only monotonic ACKs within the cursor range it has offered to that connection; an arbitrary future ACK cannot advance server-side client state. Messages without a stream ID retain legacy snapshot behavior and do not establish a v2 ACK ceiling.

Legacy clients remain supported by the v2 backend, never receive `runtime_replay`, and recover through a snapshot. Snapshot and `ready` payloads may include `runtime_stream_id` and runtime version fields; legacy clients safely ignore or tolerate those fields, so compatibility does not depend on the payload being streamless. The v2 frontend also recognizes truly streamless backend messages and safely falls back to legacy snapshot behavior during a mixed-version deployment.

### Production rollout and rollback

Roll out in this order:

1. Deploy the backend to every replica and wait for health checks to pass. Confirm `/metrics` exposes the four `theia_ws_runtime_*` metric families below and that an authenticated `GET /api/v1/runtime/overview` succeeds.
2. Deploy the frontend only after all backend replicas support protocol v2. Existing frontend sessions remain compatible while the backend rollout completes.
3. Reconnect a small client cohort, verify `current` or `replay` completions, and then expand the frontend rollout.

Rollback in the reverse order:

1. Roll back the frontend first so new reconnects use the legacy client flow while the v2 backend is still available.
2. After the old frontend is serving all users, roll back the backend replicas.

A backend restart, rollback, or reconnect to a replica with another lineage can make the saved stream ID unknown. The expected response is a runtime snapshot, not an unsafe replay. This can increase snapshot traffic, but protocol-v2 snapshot recovery remains runtime-only: it does not require a structural topology reload and does not emit the structural "Live Topology resynced" notice.

### Compact HTTP fallback

`GET /api/v1/runtime/overview` is the authenticated fallback for a timed-out WebSocket recovery and requires topology-read permission. It returns an uncached (`Cache-Control: no-store`) atomic tuple containing `schema_version`, `runtime_stream_id`, `runtime_version`, `runtime_identity`, and `runtime_snapshot`. It intentionally omits structural topology so recovery is smaller than reloading the canvas endpoint. `HEAD` is also supported for checking endpoint availability without a response body.

Every authenticated GET or HEAD that reaches this handler records an `http_fallback` `scheduled` event followed by `completed` or `failed`, including manual checks and health probes. Do not poll this endpoint as a generic health check. Run validation probes before recording soak baselines, or account for their metric increments in dashboards and acceptance queries.

### Metrics and PromQL

The backend exports these metric families:

| Metric | Meaning and labels |
|--------|--------------------|
| `theia_ws_runtime_recovery_total` | Recovery lifecycle counter labeled by `mode`, `reason`, and `outcome` |
| `theia_ws_runtime_recovery_duration_seconds` | Terminal recovery duration histogram labeled by `mode` and `outcome` |
| `theia_ws_runtime_ack_lag_versions` | Histogram of the validated client ACK distance behind the current runtime version |
| `theia_ws_runtime_replay_versions` | Histogram of the version span selected for installed replay recoveries |

Protocol-v2 recovery completion is recorded when the validated target ACK arrives. Legacy clients cannot send `runtime_ack`, so their snapshot recovery completes when the replacement batch is successfully installed in the client mailbox. Repeated protocol-v2 requests for the same pending mode, reason, stream, and target remain one logical recovery attempt.

Bounded recovery labels are:

- Modes: `current`, `replay`, `snapshot`, `http_fallback`
- Outcomes: `scheduled`, `completed`, `failed`
- Reasons: `client_resync_scheduled`, `client_missing_runtime_snapshot`, `state_changes_dropped`, `hub_buffer_full`, `connect`, `client_gap`, `stream_mismatch`, `timeout`

Scheduled recovery rate by target, mode, and reason:

```promql
sum by (job, instance, mode, reason) (
  rate(theia_ws_runtime_recovery_total{outcome="scheduled"}[5m])
)
```

Recovery failures during the last five minutes:

```promql
sum by (job, instance, mode, reason) (
  increase(theia_ws_runtime_recovery_total{outcome="failed"}[5m])
)
```

Completed recovery duration p95:

```promql
histogram_quantile(
  0.95,
  sum by (job, instance, mode, le) (
    rate(theia_ws_runtime_recovery_duration_seconds_bucket{outcome="completed"}[5m])
  )
)
```

ACK lag p95 in versions:

```promql
histogram_quantile(
  0.95,
  sum by (job, instance, le) (
    rate(theia_ws_runtime_ack_lag_versions_bucket[5m])
  )
)
```

Installed replay span p95 in versions:

```promql
histogram_quantile(
  0.95,
  sum by (job, instance, le) (
    rate(theia_ws_runtime_replay_versions_bucket[5m])
  )
)
```

The related rules in `docker/prometheus/alert_rules.yml` mean:

- `RuntimeRecoveryFailures`: at least one failed recovery occurred in the rolling five-minute window, grouped by target, mode, and reason, and that condition persisted for ten minutes. Inspect the failure reason before increasing capacity; snapshot, replay, and HTTP failures have different causes.
- `RuntimeAckLagHigh`: the target's five-minute ACK lag p95 stayed above 32 versions for ten minutes. This usually indicates a slow client, event-loop stalls, network delay, or sustained producer/backpressure pressure.
- `WebSocketBackpressure`: more than 25 WebSocket backpressure events occurred in five minutes for a target/scope/reason and the condition persisted for 60 seconds. It is a useful corroborating signal for `hub_buffer_full` snapshot recoveries.

### Thirty-minute staging soak

Use production-like replica and proxy settings. Deployments restart process-local metrics, so establish the Prometheus counter baselines only after the backend rollout and any manual `/api/v1/runtime/overview` validation probes:

1. Deploy every backend replica, wait for healthy targets, run the manual validation probes, and deploy the frontend second.
2. After the frontend is healthy, record the Prometheus counter baselines and start the 30-minute soak window.
3. Keep multiple canvases open while normal polling produces runtime changes.
4. Exercise foreground/background tab transitions and several short network disconnects so clients reconnect with saved cursors.
5. If the production load balancer is multi-replica, include both sticky reconnects and at least one deliberate cross-replica reconnect. The latter should complete through a snapshot.
6. Watch the PromQL queries above together with `theia_ws_backpressure_total`, `theia_state_changes_dropped_total`, and `theia_ws_connected_clients`.

Accept the rollout when all of these signals hold for the soak window:

- `failed` recovery counters do not increase, and scheduled/completed counts converge after allowing for recoveries still in flight at the end of the query window.
- Neither `RuntimeRecoveryFailures` nor `RuntimeAckLagHigh` fires; ACK lag p95 remains below the alert threshold of 32 versions.
- Reconnects on the same lineage normally use `current` or `replay`. Snapshot recovery is limited to expected deployment or restart, journal-gap, overflow or dropped-change recovery, cross-replica routing, and full snapshot or topology rebuilds that rotate the lineage even on the same replica.
- Excluding documented manual/probe calls, `http_fallback` is absent or isolated and completes successfully rather than recurring continuously.
- WebSocket client count returns to its baseline after reconnects, with no sustained backpressure or dropped state-change growth.
- The canvas keeps its structural topology and applies fresh runtime values. Protocol-v2 current, replay, snapshot, and HTTP fallback recoveries do not add a structural canvas-bootstrap request or emit the structural "Live Topology resynced" notice.

### Horizontal replicas

The replay journal and runtime lineage are in-process state. A reconnect to another backend replica therefore presents a different `runtime_stream_id`; the receiving replica safely selects snapshot recovery instead of attempting a replay from an unrelated journal. Sticky WebSocket routing improves replay hit rate and reduces snapshot bandwidth, but correctness does not depend on affinity. No shared journal, synchronized lineage, or deployment flag is required.

### Repeated "Live Topology resynced"

The literal `Live Topology resynced` text belongs to an older frontend asset or session; the current frontend reports structural recovery as `Topology refreshed`, `Topology refreshed after reconnect`, or `Topology refreshed after backend resync`. Protocol-v2 runtime recovery is a separate path: `current`, replay, runtime snapshot, and `/api/v1/runtime/overview` fallback update runtime state without reloading structural topology and do not emit any of these notices. Diagnose the notice first as a structural recovery, then use runtime metrics to determine whether a separate runtime issue happened at the same time:

1. In browser network tools, correlate the notice timestamp with reconnects and requests to the saved-map canvas bootstrap or other structural topology endpoint. Repeated structural requests explain the notice; a v2 runtime snapshot or runtime-overview request alone does not.
2. Verify whether the connection advertises protocol 2 and the last `runtime_stream_id`/`runtime_version`. A legacy connection or a reconnect that enters the legacy structural fallback is the relevant notice path.
3. Break down `theia_ws_runtime_recovery_total` by `mode`, `reason`, and `outcome`. Completed runtime-only recovery without a matching structural request is not the source of the notice. Repeated `stream_mismatch` can still explain runtime snapshots after restarts, cross-replica routing, or full rebuilds that rotate the lineage on the same replica, but those snapshots do not themselves emit the structural notice.
4. Check `RuntimeRecoveryFailures`, recovery duration p95, and `theia_ws_runtime_ack_lag_versions` for a concurrent runtime problem. High ACK lag with increasing `theia_ws_backpressure_total` or `theia_ws_overview_mailbox_clear_total` indicates a slow runtime consumer, not proof of structural topology recovery.
5. If WebSocket runtime recovery times out, make one authenticated `GET /api/v1/runtime/overview`. A `503` or `theia_ws_runtime_recovery_total{mode="http_fallback",outcome="failed"}` growth identifies the compact runtime boundary; remember that the diagnostic request itself increments the `http_fallback` scheduled and terminal counters.
6. On multiple replicas, compare the replica serving the old connection with the one serving the reconnect. Sticky routing improves runtime replay efficiency, while the separate structural notice still needs evidence from topology requests or the legacy recovery path.

Investigate repeated notices even when runtime recovery metrics are healthy. Conversely, treat a lineage-change runtime snapshot as expected unless it fails or repeats excessively; do not attribute the structural notice to that snapshot without a matching topology recovery event.

---

## Troubleshooting

### Backend doesn't start

```bash
docker compose logs backend
```

Common causes:
- Port 8080 already in use: change the host port in `docker-compose.yml` (`"8081:8080"`)
- Database permission error: ensure `/data` in the container, or your configured `THEIA_DATA_DIR`, is writable by the container user

### SNMP probes fail / devices stay "down"

```bash
# Check SNMP reachability from inside the backend container
docker exec theia-backend sh -c "apt-get install -y snmp -q && snmpget -v2c -c public <device-ip> 1.3.6.1.2.1.1.1.0"
```

- For WISP lab devices on Docker Desktop, prefer `WISP_SEED_TARGET_MODE=docker` so Theia stores the `172.31.250.x` management targets reachable from the backend container
- Verify the device IP is reachable from the Docker network
- Confirm the SNMP community string matches the device configuration
- For SNMPv3 devices, use `version: "3"` in the device payload with `username`, `auth_protocol`, `auth_passphrase`, `priv_protocol`, `priv_passphrase`

### Frontend shows blank canvas after seeding devices

- Open browser devtools (F12) → Console for errors
- Check that `/api/v1/devices` returns devices: `curl http://localhost:8080/api/v1/devices`
- The topology canvas requires at least one device with status `up`; wait ~15 seconds for the first probe cycle to complete

### "3 links showing instead of 1"

This can occur if LLDP and CDP both report the same physical link from opposite directions before the deduplication fix. Clean up via the UI (click each duplicate link → Delete Link) or directly:

```bash
# List links and IDs
curl -s http://localhost:8080/api/v1/links | python3 -m json.tool

# Delete a specific link
curl -X DELETE http://localhost:8080/api/v1/links/<uuid>
```

### Prometheus / metrics not showing

1. Check Prometheus is running: http://localhost:9090/targets — all targets should be `UP`
2. In the Theia UI, open Settings and confirm the Prometheus URL is set to a host-reachable Prometheus address for the Docker host/network, for example `http://<docker-host-address>:9090`
3. Metrics appear only after the first successful scrape cycle (~15–30 seconds after startup)

### Reset everything

```bash
make clean   # Stops containers and deletes the theia-data volume
make dev     # Fresh start
make seed    # Re-add sample devices if they are reachable from the backend container
```
