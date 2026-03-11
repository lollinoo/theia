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
| Frontend | http://localhost:5173 | React + Vite dev server |
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

Open http://localhost:5173 to see the topology.

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

**Frontend** — Vite's HMR is active on port 5173. The dev server proxies `/api/*` and `/api/v1/ws` to the backend at `http://backend:8080`, so the frontend container talks to the backend over the internal Docker network.

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

Production uses a compiled Go binary in a minimal Debian image. There is no hot-reload, no SNMP simulators, and no Prometheus stack included — those are expected to be provided externally or configured separately.

### 1. Build the production image

```bash
make build
# Produces: theia:latest
```

Or with a custom tag:

```bash
docker build --target production -t theia:1.0.0 -f Dockerfile .
```

### 2. Prepare storage

The backend uses a single SQLite database file. Create a persistent directory:

```bash
mkdir -p /opt/theia/data
```

### 3. Run the backend

```bash
docker run -d \
  --name theia-backend \
  --restart unless-stopped \
  -p 8080:8080 \
  -v /opt/theia/data:/data \
  -e THEIA_DB_PATH=/data/theia.db \
  -e THEIA_LISTEN_ADDR=:8080 \
  -e THEIA_LOG_LEVEL=info \
  theia:latest
```

### 4. Build and serve the frontend

The frontend is a static React SPA built with Vite. In production, point `VITE_API_URL` at your backend host and serve the output with any static file server (nginx, Caddy, etc.).

```bash
cd frontend

# Set the backend URL (must be reachable from the browser)
export VITE_API_URL=http://your-backend-host:8080

npm ci
npm run build
# Output: frontend/dist/
```

#### Serve with nginx (example)

```nginx
server {
    listen 80;
    server_name theia.example.com;

    root /var/www/theia;
    index index.html;

    # SPA fallback
    location / {
        try_files $uri $uri/ /index.html;
    }

    # Proxy API + WebSocket to backend
    location /api/ {
        proxy_pass http://backend-host:8080;
        proxy_http_version 1.1;
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection "upgrade";
        proxy_set_header Host $host;
    }
}
```

### 5. Prometheus integration (optional)

If you have an existing Prometheus instance, point it at the SNMP exporter and configure the backend's Prometheus URL through the UI (Settings panel → Prometheus URL).

The alert rules file is at `docker/prometheus/alert_rules.yml`. Import it into your Prometheus setup.

### 6. docker-compose for production (minimal)

```yaml
# docker-compose.prod.yml
services:
  backend:
    image: theia:latest
    restart: unless-stopped
    ports:
      - "8080:8080"
    volumes:
      - theia-data:/data
    environment:
      - THEIA_DB_PATH=/data/theia.db
      - THEIA_LISTEN_ADDR=:8080
      - THEIA_LOG_LEVEL=info

volumes:
  theia-data:
```

```bash
docker compose -f docker-compose.prod.yml up -d
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
