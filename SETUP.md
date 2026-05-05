# Theia — Setup Guide

Network topology visualizer with SNMP monitoring, real-time metrics, and link management.

---

## Table of Contents

- [Prerequisites](#prerequisites)
- [Development Environment](#development-environment)
- [Production Environment](#production-environment)
- [Configuration Reference](#configuration-reference)
- [API Quick Reference](#api-quick-reference)
- [Troubleshooting](#troubleshooting)

---

## Prerequisites

| Tool | Version | Purpose |
|------|---------|---------|
| Docker | 24+ | All services run in containers |
| Docker Compose | v2 (plugin) | Orchestrates the stack |
| `make` | any | Convenience commands |
| `curl` | any | Seed script / API testing |

No Go or Node.js installation is required — the build happens inside Docker.

---

## Development Environment

The dev stack runs everything locally with hot-reload for both backend and frontend, plus three SNMP device simulators (Router / Switch / AP) so you can develop without real network hardware.

### Stack

| Service | URL | Description |
|---------|-----|-------------|
| Backend | http://localhost:8080 | Go API with Air hot-reload |
| Frontend | http://localhost:3000 | React + Vite dev server |
| PostgreSQL | `127.0.0.1:5432` | Bundled development database |
| Prometheus | http://localhost:9090 | Metrics and alerting |
| SNMP exporter | http://localhost:9116 | Prometheus SNMP scrape adapter |
| SNMP Router sim | `127.0.10.10:161` | Router simulator (UDP) |
| SNMP Switch sim | `127.0.10.11:161` | Cisco simulator (UDP) |
| SNMP AP sim | `127.0.10.12:161` | Ubiquiti simulator (UDP) |

### 1. Clone and start

```bash
git clone <repo-url>
cd theia
make install-hooks
make dev
```

This builds all images and starts the full stack in the background. First build takes 2–4 minutes to compile Go and download npm packages.

`config.yaml` is a local-only file and is ignored by git because it can contain a database DSN or other deployment-specific values. The dev stack works through environment variables without it. If you need a local config file, start from the template:

```bash
cp config.example.yaml config.yaml
```

`make install-hooks` is a one-time setup step per clone. It enables the repo-managed Git hooks so local commits must follow the conventional commits format, for example `feat(api): add device backup endpoint`. GitHub Actions validates the same rule again in CI.

### 2. Verify everything is up

```bash
docker compose ps
# All services should show "healthy" or "running"

curl -s http://localhost:8080/api/v1/health
# {"status":"ok"}
```

### 3. Seed the simulator devices

```bash
make seed
```

This calls the REST API to register the three SNMP simulators. After seeding, the backend probes them immediately via SNMP and the canvas will populate within ~10 seconds.

Open http://localhost:3000 to see the topology.

### 4. Useful dev commands

```bash
make logs           # Tail backend logs
make stop           # Stop all containers
make clean          # Stop + delete volumes (resets the database)
make test           # Run unit tests
make test-integration  # Run integration tests against SNMP sims

# Debug SNMP simulators directly
make snmpwalk-router   # snmpwalk 127.0.10.10
make snmpwalk-switch   # snmpwalk 127.0.10.11
make snmpwalk-ap       # snmpwalk 127.0.10.12
```

### 4.1 Database default and small-install exception

PostgreSQL is the standard database backend for Theia in development, staging, and production. The normal `make dev` flow starts the backend against the local PostgreSQL service on `127.0.0.1:5432`.

SQLite is supported only for demo, lab, or very small installs, and only with explicit opt-in:

```bash
THEIA_DB_DRIVER=sqlite \
THEIA_ALLOW_SQLITE_SMALL_INSTALL=true \
make dev
```

Use SQLite only when the installation stays within all of these limits:

- up to 50 managed devices
- one active Theia process
- one active administrative operator at a time
- no expectation of overlapping intensive polling, topology churn, scheduled backup activity, and configuration-write bursts

Instance backup / restore is supported on PostgreSQL deployments when compatible PostgreSQL client tools are available on `PATH`. PostgreSQL backup jobs require `pg_dump` 17.x; restore validation, staging, and apply require `pg_restore` 17.x; non-dry-run restore apply also requires `pg_dump` 17.x so Theia can take a pre-restore live database backup before changing the database.

### 4.2 PostgreSQL client tools

The bundled development, staging, and production compose stacks use PostgreSQL 17. Official backend images bundle these PostgreSQL 17 client tools from `postgres:17-bookworm`; custom host or custom image deployments must place compatible tools on `PATH`.

| Tool | Requirement | Supported version |
|------|-------------|-------------------|
| `pg_dump` | Required for PostgreSQL instance backup and pre-restore live DB backup | 17.x |
| `pg_restore` | Required for PostgreSQL restore validation and apply | 17.x |
| `psql` | Bundled in official images for operator diagnostics/admin use; not required by backup/restore | 17.x |

Missing or incompatible PostgreSQL client tools fail the backup or restore job at startup with actionable diagnostics. Error output redacts connection strings and passwords.

### 4.3 Migrate an existing SQLite dataset to PostgreSQL

Once an installation outgrows the small-install envelope, migration to PostgreSQL is the expected path. Stop the backend first so the SQLite source stays stable during the copy, then import the database into PostgreSQL:

```bash
make postgres-up
export THEIA_DB_DSN='<postgresql-target-dsn>'
make migrate-postgres
```

The import command runs the PostgreSQL schema migrations first, then copies all application tables from the SQLite file configured in `config.yaml` (or from `MIGRATE_SOURCE` if you override it).

You can also run the migrator directly if you have a local Go toolchain installed:

```bash
go run ./cmd/theia-db-migrate \
  -config config.yaml \
  -source-sqlite ./data/theia.db \
  -target-dsn '<postgresql-target-dsn>' \
  -truncate-target
```

Notes:

- `-truncate-target` makes the import deterministic by clearing the target tables before copying.
- Device backup archives and `known_hosts` remain file-based in `data_dir`; move that directory separately if you are migrating the whole deployment to another host.
- Instance backup metadata rows are copied. PostgreSQL-backed instance backup and restore APIs are available after migration when the target deployment has compatible PostgreSQL client tools on `PATH`.

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

If you want a denser topology than the default 3-device dev stack, this repo also includes a dedicated 10-router MikroTik-flavoured WISP lab with active OSPF and SNMP/LLDP discovery data for Theia.

### What it gives you

- 10 simulated MikroTik routers
- Real OSPF adjacencies via FRRouting inside the lab containers
- Static SNMP + LLDP data so Theia can discover interfaces and links
- A separate Prometheus instance on `http://localhost:9091`

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

If Theia is not already running, start it separately:

```bash
make dev
```

### Seed the 10 routers into Theia

```bash
make wisp-seed
```

The seed script registers the routers at `127.0.10.21` through `127.0.10.30`. After probing completes, Theia should populate the WISP topology automatically from LLDP.

### Seed the radio access layer

```bash
make wisp-radio-seed
```

This adds 4 sector APs and 8 subscriber CPE nodes on `127.0.10.31` through `127.0.10.42`. The AP sector interfaces simulate PtMP by advertising multiple LLDP neighbors on the same wireless interface.

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

Required operator inputs for the standard bundled PostgreSQL stack:

- `THEIA_ENCRYPTION_KEY`
- `THEIA_DB_DSN`
- `POSTGRES_PASSWORD` for the bundled `postgres` service

For bundled PostgreSQL, use this DSN shape:

```text
postgres://<postgres-user>:<postgres-password>@postgres:5432/<postgres-db>?sslmode=disable
```

The DSN user, password, and database placeholders must match `POSTGRES_USER`, `POSTGRES_PASSWORD`, and `POSTGRES_DB`.

```bash
cp .env.prod.example .env.prod
# Fill required operator-provided values before first start
```

### 2. Start the stack

```bash
make prod
```

`make prod` starts the standard production stack on PostgreSQL using the bundled `postgres` service from `docker-compose.prod.yml`.

The shipped `make prod` and `make prod-metrics` targets hard-depend on the bundled `postgres` service in `docker-compose.prod.yml`. If you need an external PostgreSQL service or the SQLite small-install exception path, use a custom compose override or edit instead of only setting `THEIA_DB_DSN` or `THEIA_DB_DRIVER`.

SQLite is only for explicit demo, lab, or very small production installs. To use that exception path, first remove or override the bundled `postgres` dependency in your production compose setup, then set:

```bash
# THEIA_DB_DRIVER=sqlite
# THEIA_ALLOW_SQLITE_SMALL_INSTALL=true
make prod
```

Or with the metrics stack (Prometheus + SNMP exporter):

```bash
make prod-metrics
```

Open `http://localhost` in the browser. Use `http://localhost/api/v1/...` for API requests through the frontend proxy.

### 3. Add your network devices

Via the UI Settings panel, or directly via the API:

```bash
curl -X POST http://localhost/api/v1/devices \
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
make prod-metrics   # Start with Prometheus + SNMP exporter
make prod-down      # Stop all production containers
make prod-logs      # Follow backend logs
make prod-clean     # Stop + delete volumes (resets database)
```

---

## Staging Environment

The staging stack pulls pre-built images from GHCR and keeps them updated with Watchtower. It uses different default ports so it can run next to production.

### 1. Configure environment

`.env.staging.example` is a placeholder template only. Copy it to `.env.staging`, fill the required values locally, and keep `.env.staging` plus any staging `config.yaml` override local and untracked because they can contain deployment secrets.

Staging startup runs strict secret validation because `THEIA_DEPLOYMENT_ENV=staging` is set. The backend rejects missing or example secret values before opening the database.

Required operator inputs for the standard bundled PostgreSQL stack:

- `THEIA_ENCRYPTION_KEY`
- `THEIA_DB_DSN`
- `POSTGRES_PASSWORD` for the bundled `postgres` service

For bundled PostgreSQL, use this DSN shape:

```text
postgres://<postgres-user>:<postgres-password>@postgres:5432/<postgres-db>?sslmode=disable
```

The DSN user, password, and database placeholders must match `POSTGRES_USER`, `POSTGRES_PASSWORD`, and `POSTGRES_DB`.

```bash
cp .env.staging.example .env.staging
# Fill required operator-provided values before first start
```

### 2. Start the stack

```bash
make staging
```

`make staging` starts the standard staging stack on PostgreSQL using the bundled `postgres` service from `docker-compose.staging.yml`.

The shipped `make staging` target hard-depends on the bundled `postgres` service in `docker-compose.staging.yml`. If you need an external PostgreSQL service or the SQLite small-install exception path, use a custom compose override or edit instead of only setting `THEIA_DB_DSN` or `THEIA_DB_DRIVER`.

SQLite is only for explicit demo, lab, or very small staging installs. To use that exception path, first remove or override the bundled `postgres` dependency in your staging compose setup, then set:

```bash
# THEIA_DB_DRIVER=sqlite
# THEIA_ALLOW_SQLITE_SMALL_INSTALL=true
make staging
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

## Configuration Reference

### Backend

Configuration is loaded from local `config.yaml` when present. The tracked `config.example.yaml` is a placeholder template; copy it to `config.yaml` only when you need local file-based overrides. All keys can be overridden with environment variables.

| config.yaml key | Environment variable | Default | Description |
|-----------------|---------------------|---------|-------------|
| `db_driver` | `THEIA_DB_DRIVER` | `postgres` | Primary database driver: `postgres` by default; `sqlite` only with explicit small-install opt-in |
| `deployment_env` | `THEIA_DEPLOYMENT_ENV` | none | Set to `production` or `staging` for deployed environments so startup enforces required secret validation |
| `listen_addr` | `THEIA_LISTEN_ADDR` | `:8080` | HTTP server bind address |
| `db_path` | `THEIA_DB_PATH` | `./data/theia.db` | SQLite database file path |
| `db_dsn` | `THEIA_DB_DSN` | none | PostgreSQL DSN for the standard postgres path; `config.Load()` does not inject one, so operators must provide it explicitly through local config, local env, or a secret manager |
| `data_dir` | `THEIA_DATA_DIR` | `./data` | Local app data directory for known_hosts and backup files |
| `bridge_binaries_dir` | `THEIA_BRIDGE_BINARIES_DIR` | `` | Optional directory containing pre-built bridge binaries; leave empty to disable bridge downloads |
| `log_level` | `THEIA_LOG_LEVEL` | `info` | Log verbosity: `debug`, `info`, `warn`, `error` |

Runtime settings (poll interval, Prometheus URL, Grafana URL) are stored in the database and configurable via the Settings panel in the UI — no restart needed.

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

| Method | Path | Description |
|--------|------|-------------|
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

- Verify the device IP is reachable from the Docker network
- Confirm the SNMP community string matches the device configuration
- For SNMPv3 devices, use `version: "3"` in the device payload with `username`, `auth_protocol`, `auth_passphrase`, `priv_protocol`, `priv_passphrase`

### Frontend shows blank canvas after seed

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
make seed    # Re-add simulator devices
```
