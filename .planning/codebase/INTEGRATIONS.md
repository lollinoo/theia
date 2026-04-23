# External Integrations

**Analysis Date:** 2026-04-23

## APIs & External Services

**Network Device Management:**
- SNMP agents - Primary device discovery, topology observation, performance metrics, operational state, and interface counters.
  - SDK/Client: `github.com/gosnmp/gosnmp` in `internal/snmp/client.go`, collectors in `internal/collector/`, and pipeline wiring in `cmd/theia/runtime_bootstrap.go`.
  - Auth: SNMP v2c community and SNMPv3 credentials are stored in encrypted database profiles via `internal/repository/sqlite/snmp_crypto.go`; runtime timeout/retry settings use `snmp_timeout_seconds` and `snmp_retries` from `internal/domain/settings.go`.
- SSH/SFTP devices - Device config backup, command execution, reachability tests, and backup file download.
  - SDK/Client: `golang.org/x/crypto/ssh` and `github.com/pkg/sftp` in `internal/ssh/client.go`; backup orchestration in `internal/service/backup_service.go`.
  - Auth: credential profiles stored encrypted with `THEIA_ENCRYPTION_KEY`; host key verification uses known-hosts store initialized in `cmd/theia/runtime_bootstrap.go`.
- MikroTik WinBox local bridge - Browser-initiated local bridge launches WinBox with an encrypted credential token.
  - SDK/Client: local HTTP bridge implemented in `cmd/winbox-bridge/main.go`; backend token endpoint in `internal/api/bridge_handler.go`; frontend flow in `frontend/src/hooks/useWinboxFlow.ts`.
  - Auth: per-bridge `bridge_secret` setting plus bridge-local `config.json`; token encryption uses AES-256-GCM in `internal/api/bridge_handler.go` and `cmd/winbox-bridge/main.go`.

**Metrics & Observability:**
- Prometheus HTTP API - Optional enrichment, health status, alerts, hostnames, probe reachability, and metric queries.
  - SDK/Client: custom HTTP client `internal/metrics/prometheus.go`; collector wrapper `internal/collector/prometheus.go`; runtime monitor `internal/worker/pipeline_prometheus_monitor.go`.
  - Auth: no Prometheus auth mechanism is implemented; base URL comes from database setting `prometheus_url` in `internal/domain/settings.go` and `internal/api/prometheus_handler.go`.
- Prometheus scrape endpoint - Theia exposes its own runtime metrics at `/metrics`.
  - SDK/Client: custom Prometheus text exposition in `internal/observability/registry.go`, mounted in `cmd/theia/runtime_bootstrap.go`.
  - Auth: no auth guard detected on `/metrics`.
- Grafana - Optional external dashboard links opened from the frontend.
  - SDK/Client: no SDK; URLs are settings-driven in `frontend/src/components/Canvas.tsx`, `frontend/src/components/SettingsPanel.tsx`, and `frontend/src/components/DeviceConfigPanel.tsx`.
  - Auth: handled externally by Grafana/browser session; Theia stores only `grafana_url` and per-device `grafana_dashboard_url:<device_id>` settings.

**Container/Deployment Services:**
- GitHub Container Registry (GHCR) - Publishes backend/frontend images and pulls staged/production images.
  - SDK/Client: GitHub Actions Docker actions in `.github/workflows/ci.yml`; compose image references in `docker-compose.staging.yml` and `docker-compose.prod.yml`.
  - Auth: GitHub Actions uses `GHCR_PAT` secret in `.github/workflows/ci.yml`; local deploy docs reference Docker login in `.env.prod.example` and `.env.staging.example`.
- Watchtower - Staging auto-update integration polls GHCR images.
  - SDK/Client: `ghcr.io/nicholas-fedor/watchtower:latest` service in `docker-compose.staging.yml`.
  - Auth: Docker config mount path controlled by `DOCKER_CONFIG_PATH` in `.env.staging.example`.

## Data Storage

**Databases:**
- PostgreSQL 17 - Standard development, staging, and production database.
  - Connection: `THEIA_DB_DSN` / `db_dsn` configured in `internal/config/config.go`, `config.example.yaml`, `.env.prod.example`, and `.env.staging.example`.
  - Client: Go `database/sql` with `github.com/jackc/pgx/v5/stdlib`; opened in `internal/repository/sqlite/db_tuning.go` and registered in `cmd/theia/main.go`.
- SQLite - Demo/lab/small-install fallback only.
  - Connection: `THEIA_DB_PATH` / `db_path`; DSN tuning uses WAL, foreign keys, busy timeout, and immediate tx lock in `internal/repository/sqlite/db_tuning.go`.
  - Client: Go `database/sql` with `github.com/mattn/go-sqlite3`; production startup requires explicit `THEIA_ALLOW_SQLITE_SMALL_INSTALL=true` for SQLite in `cmd/theia/runtime_bootstrap.go`.

**File Storage:**
- Local filesystem data directory - Application data, known_hosts, device backup files, and instance backup archives live under `THEIA_DATA_DIR` / `data_dir` resolved in `cmd/theia/runtime_paths.go` and prepared in `cmd/theia/runtime_bootstrap.go`.
- Bridge binaries - Optional local directory `THEIA_BRIDGE_BINARIES_DIR` / `bridge_binaries_dir` exposes prebuilt bridge downloads via `internal/api/bridge_handler.go`.
- WinBox bridge config - Desktop bridge stores `config.json` under the OS user config directory in `cmd/winbox-bridge/config.go`.

**Caching:**
- In-process device/link cache - `internal/cache` is created in `cmd/theia/runtime_bootstrap.go` and invalidated by repository writes.
- No external cache service detected; Redis/Memcached are not present in `go.mod`, `frontend/package.json`, or compose files.

## Authentication & Identity

**Auth Provider:**
- No application user authentication provider detected.
  - Implementation: API routes in `internal/api/router.go` are mounted without login/session/JWT/OAuth middleware; CORS, request logging, max body size, and JSON content-type middleware are present.
- Device credentials are profile-based, not user identity-based.
  - Implementation: encrypted SNMP and SSH/WinBox credential profiles in `internal/repository/sqlite/snmp_crypto.go`, `internal/api/credential_profile_handler.go`, and `internal/api/device_credential_profile_handler.go`.
- Bridge launch authorization is secret/token based.
  - Implementation: backend encrypts one-time launch payloads with request-supplied bridge secret in `internal/api/bridge_handler.go`; bridge validates `Origin` and `Host` in `cmd/winbox-bridge/main.go`.

## Monitoring & Observability

**Error Tracking:**
- None detected; no Sentry, Datadog, Honeycomb, OpenTelemetry collector/exporter, or similar service dependency appears in `go.mod` or `frontend/package.json`.

**Logs:**
- Backend uses Go standard `log` throughout `cmd/theia/runtime_bootstrap.go`, `internal/ws/`, and workers; `THEIA_LOG_LEVEL` is loaded but logging framework remains standard library.
- API request logging middleware is applied in `internal/api/router.go` via `RequestLogger` from `internal/api/middleware.go`.
- WinBox bridge writes logs to stderr and, in tray/debug modes, a temp log file via `cmd/winbox-bridge/main.go`.
- Metrics are exposed in Prometheus text format by `internal/observability/registry.go` at `/metrics`.

## CI/CD & Deployment

**Hosting:**
- Containerized self-hosted deployment using Docker Compose.
- Backend production container is built from `Dockerfile`; frontend production container is built from `Dockerfile.frontend` and serves through nginx.
- Staging pulls GHCR images and runs backend, PostgreSQL, frontend, and Watchtower in `docker-compose.staging.yml`.
- Production compose exists at `docker-compose.prod.yml`; environment template is `.env.prod.example`.

**CI Pipeline:**
- GitHub Actions in `.github/workflows/ci.yml`.
- Required quality jobs: `backend-fast`, `frontend-fast`, `realtime-stress`, `collector-contract`, and `browser-e2e`.
- Image publishing jobs: branch images for non-tag pushes and release images for `v*` tags in `.github/workflows/ci.yml`.
- Bridge release assets are cross-compiled for windows/linux/darwin amd64/arm64 in `.github/workflows/ci.yml`.

## Environment Configuration

**Required env vars:**
- `THEIA_ENCRYPTION_KEY` - Required by `internal/crypto/encrypt.go`; required in staging/prod examples `.env.staging.example` and `.env.prod.example`.
- `THEIA_DB_DRIVER` - Database driver (`postgres` standard, `sqlite` exception) in `internal/config/config.go`.
- `THEIA_DB_DSN` - Required when `THEIA_DB_DRIVER=postgres` in `cmd/theia/runtime_bootstrap.go`.
- `THEIA_DB_PATH` - SQLite database path for explicit small-install/lab mode in `config.example.yaml` and `internal/config/config.go`.
- `THEIA_ALLOW_SQLITE_SMALL_INSTALL` - Required opt-in for SQLite in `cmd/theia/runtime_bootstrap.go`.
- `THEIA_DATA_DIR` - Data, backup, and known-hosts base directory in `internal/config/config.go` and runtime path resolution.
- `THEIA_LISTEN_ADDR` - Backend bind address in `internal/config/config.go`.
- `THEIA_LOG_LEVEL` / `LOG_LEVEL` - Backend log level setting in `internal/config/config.go` and env templates.
- `THEIA_BRIDGE_BINARIES_DIR` - Optional bridge binary download directory in `internal/config/config.go` and `internal/api/bridge_handler.go`.
- `VITE_API_URL` - Frontend dev API proxy target in `frontend/vite.config.ts`.
- `BACKEND_PORT` / `FRONTEND_PORT` - Container proxy/listen ports in `Dockerfile.frontend`, `frontend/nginx.conf.template`, `.env.prod.example`, and `.env.staging.example`.
- `POSTGRES_DB`, `POSTGRES_USER`, `POSTGRES_PASSWORD`, `POSTGRES_BIND_ADDR`, `POSTGRES_HOST_PORT` - Bundled PostgreSQL settings in env templates and compose files.
- `IMAGE_TAG`, `DOCKER_CONFIG_PATH` - Staging image tag and Watchtower auth config path in `.env.staging.example` and `docker-compose.staging.yml`.

**Secrets location:**
- Real `.env.prod` and `.env.staging` files are expected local-only deployment secret files; examples are `.env.prod.example` and `.env.staging.example`.
- `THEIA_ENCRYPTION_KEY` must be supplied through environment/deployment secret management, not source code.
- GitHub Actions uses repository secrets `GHCR_PAT` and `GITHUB_TOKEN` in `.github/workflows/ci.yml`.
- Bridge secret is generated and stored locally by the bridge in OS config path from `cmd/winbox-bridge/config.go`; the same value is copied into Theia runtime setting `bridge_secret` via the UI in `frontend/src/components/SettingsPanel.tsx`.
- SNMP, SSH, and WinBox profile secrets are encrypted at rest using `THEIA_ENCRYPTION_KEY` in `internal/crypto/encrypt.go`, `internal/repository/sqlite/snmp_crypto.go`, and `internal/service/backup_service.go`.

## Webhooks & Callbacks

**Incoming:**
- REST API under `/api/v1/*` in `internal/api/router.go`.
- WebSocket endpoint `/api/v1/ws` in `internal/api/router.go`, `internal/ws/handler.go`, and frontend `frontend/src/hooks/useWebSocket.ts`.
- Prometheus scrape endpoint `/metrics` in `cmd/theia/runtime_bootstrap.go` and `internal/observability/registry.go`.
- WinBox local bridge endpoint `POST /launch` and public `GET /health` on localhost bridge port in `cmd/winbox-bridge/main.go`.
- Instance backup restore upload endpoint `/api/v1/instance-backups/restore` in `internal/api/router.go`.
- No third-party webhook receiver endpoints detected.

**Outgoing:**
- Prometheus HTTP API queries from `internal/metrics/prometheus.go` to configured `prometheus_url`.
- SNMP UDP requests to managed devices from `internal/snmp/client.go`.
- SSH/SFTP connections to managed devices from `internal/ssh/client.go` and `internal/service/backup_service.go`.
- Frontend browser opens configured Grafana URLs with `window.open` in `frontend/src/components/Canvas.tsx`.
- Frontend browser sends `POST /launch` to local WinBox bridge at `http://localhost:{bridge_port}` in `frontend/src/hooks/useWinboxFlow.ts`.
- Docker/CI pushes and pulls GHCR images via `.github/workflows/ci.yml`, `docker-compose.staging.yml`, and `docker-compose.prod.yml`.

---

*Integration audit: 2026-04-23*
