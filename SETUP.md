# MikroTik Theia — Setup Guide

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
| Prometheus | http://localhost:9090 | Metrics and alerting |
| SNMP exporter | http://localhost:9116 | Prometheus SNMP scrape adapter |
| SNMP Router sim | `172.28.10.10:161` | MikroTik simulator (UDP) |
| SNMP Switch sim | `172.28.10.11:161` | Cisco simulator (UDP) |
| SNMP AP sim | `172.28.10.12:161` | Ubiquiti simulator (UDP) |

### 1. Clone and start

```bash
git clone <repo-url>
cd mikrotik-theia
make dev
```

This builds all images and starts the full stack in the background. First build takes 2–4 minutes to compile Go and download npm packages.

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
make snmpwalk-router   # snmpwalk 172.28.10.10
make snmpwalk-switch   # snmpwalk 172.28.10.11
make snmpwalk-ap       # snmpwalk 172.28.10.12
```

### 5. How hot-reload works

**Backend** — Air watches `internal/` and `cmd/`. On any `.go` file change, it recompiles and restarts the server automatically. The `./tmp/` directory holds the compiled binary; do not commit it.

**Frontend** — Vite's HMR is active on port 3000. The dev server proxies `/api/*` and `/api/v1/ws` to the backend at `http://backend:8080`, so the frontend container talks to the backend over the internal Docker network.

### 6. Adding a real device (optional)

If you have a real MikroTik router or other SNMP-capable device on the same network:

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

## Production Environment

The production stack uses compiled images — no hot-reload, no source mounts, no SNMP simulators. The frontend is built into a static bundle served by nginx, which also proxies `/api` to the backend.

### Stack

| Service | URL | Description |
|---------|-----|-------------|
| Frontend | http://localhost:80 | nginx serving React SPA + API proxy |
| Backend | http://localhost:8080 | Compiled Go binary |
| Prometheus | http://localhost:9090 | Metrics (optional, `--profile metrics`) |
| SNMP exporter | http://localhost:9116 | Scrape adapter (optional, `--profile metrics`) |

### 1. Configure environment (optional)

```bash
cp .env.prod.example .env.prod
# Edit ports or log level if needed
```

### 2. Start the stack

```bash
make prod
```

Or with the metrics stack (Prometheus + SNMP exporter):

```bash
make prod-metrics
```

Open http://localhost in the browser.

### 3. Add your network devices

Via the UI Settings panel, or directly via the API:

```bash
curl -X POST http://localhost:8080/api/v1/devices \
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

Then in the Theia UI Settings panel, set the Prometheus URL to `http://theia-prometheus:9090`.

Restart Prometheus to pick up the new targets:

```bash
docker compose -f docker-compose.prod.yml --profile metrics restart prometheus
```

### 5. Production commands

```bash
make prod           # Start backend + frontend
make prod-metrics   # Start with Prometheus + SNMP exporter
make prod-down      # Stop all production containers
make prod-build     # Build images without starting
make prod-logs      # Follow backend logs
make prod-clean     # Stop + delete volumes (resets database)
```

---

## Configuration Reference

### Backend

Configuration is loaded from `config.yaml` (or `config.example.yaml` as a template). All keys can be overridden with environment variables.

| config.yaml key | Environment variable | Default | Description |
|-----------------|---------------------|---------|-------------|
| `listen_addr` | `THEIA_LISTEN_ADDR` | `:8080` | HTTP server bind address |
| `db_path` | `THEIA_DB_PATH` | `./data/theia.db` | SQLite database file path |
| `log_level` | `THEIA_LOG_LEVEL` | `info` | Log verbosity: `debug`, `info`, `warn`, `error` |

Runtime settings (poll interval, Prometheus URL, Grafana URL) are stored in the database and configurable via the Settings panel in the UI — no restart needed.

### Frontend (build-time)

| Variable | Default | Description |
|----------|---------|-------------|
| `VITE_API_URL` | `http://backend:8080` | Backend base URL — used by Vite proxy in dev; baked into bundle for production |

---

## API Quick Reference

Base path: `http://localhost:8080/api/v1`

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
- Database permission error: ensure `/opt/theia/data` is writable by the container user

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
2. In the Theia UI, open Settings and confirm the Prometheus URL is set to `http://theia-prometheus:9090` (internal Docker hostname) or `http://localhost:9090` if running Prometheus externally
3. Metrics appear only after the first successful scrape cycle (~15–30 seconds after startup)

### Reset everything

```bash
make clean   # Stops containers and deletes the theia-data volume
make dev     # Fresh start
make seed    # Re-add simulator devices
```
