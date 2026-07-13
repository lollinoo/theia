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
