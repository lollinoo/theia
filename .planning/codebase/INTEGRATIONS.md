# External Integrations

**Analysis Date:** 2026-04-19

## APIs & External Services

**Network management:**
- SNMP devices - topology discovery, device metadata, and live operational polling
  - SDK/Client: `github.com/gosnmp/gosnmp` in `go.mod`, `internal/snmp/client.go`, `internal/snmp/discovery.go`
  - Auth: device credentials stored through SNMP profiles and assignments exposed by `internal/api/router.go`

**Metrics & observability:**
- Prometheus HTTP API - live device enrichment and health checks
  - SDK/Client: custom HTTP client in `internal/metrics/prometheus.go`
  - Auth: no auth mechanism detected; base URL is stored as `prometheus_url` in `internal/domain/settings.go`
- Prometheus scrape stack + SNMP exporter - optional dev/prod metrics collection for real or simulated devices
  - SDK/Client: Docker services in `docker-compose.yml`, `docker-compose.prod.yml`, `docker/prometheus/prometheus.yml`, `docker/prometheus/prometheus.prod.yml`
  - Auth: SNMP exporter auth profile names are configured in `docker/prometheus/snmp.yml` references from `docker/prometheus/prometheus.prod.yml`

**Device access & backup:**
- SSH endpoints on managed devices - command execution and reachability checks for backups
  - SDK/Client: `golang.org/x/crypto/ssh` in `internal/ssh/client.go`, `internal/service/backup_service.go`
  - Auth: credential profiles managed through `/api/v1/credential-profiles` in `internal/api/router.go`
- SFTP endpoints on managed devices - download backup artifacts
  - SDK/Client: `github.com/pkg/sftp` in `internal/ssh/client.go`, `internal/service/backup_service.go`
  - Auth: same device credential profile flow as SSH
- WinBox desktop bridge - launches WinBox locally using backend-issued encrypted tokens
  - SDK/Client: bridge binary in `cmd/winbox-bridge/main.go`, backend bridge endpoints in `internal/api/bridge_handler.go`
  - Auth: per-request `bridge_secret` token encryption via `bridge_secret` setting in `internal/domain/settings.go`

**Container registry & updates:**
- GitHub Container Registry (GHCR) - source of production/staging images
  - SDK/Client: image references in `docker-compose.prod.yml`, `docker-compose.staging.yml`
  - Auth: `GHCR_PAT` in `.github/workflows/ci.yml` and example env file comments in `.env.prod.example`, `.env.staging.example`
- Watchtower - staging auto-update agent
  - SDK/Client: `ghcr.io/nicholas-fedor/watchtower:latest` in `docker-compose.staging.yml`
  - Auth: Docker config mount via `DOCKER_CONFIG_PATH` in `docker-compose.staging.yml`

## Data Storage

**Databases:**
- SQLite
  - Connection: `THEIA_DB_PATH` / `db_path` in `internal/config/config.go`, `config.example.yaml`
  - Client: `github.com/mattn/go-sqlite3` through repository code in `internal/repository/sqlite/db_tuning.go`
- PostgreSQL
  - Connection: `THEIA_DB_DSN` / `db_dsn` in `internal/config/config.go`, `config.example.yaml`, `docker-compose.prod.yml`
  - Client: `github.com/jackc/pgx/v5/stdlib` through startup and migration commands in `cmd/theia/main.go`, `cmd/theia-db-migrate/main.go`

**File Storage:**
- Local filesystem for app data, device backups, instance backups, and known hosts in `cmd/theia/main.go`, `internal/service/instance_backup_service.go`, `internal/ssh/known_hosts.go`

**Caching:**
- In-process cache only via `internal/cache` and runtime state in `cmd/theia/main.go`

## Authentication & Identity

**Auth Provider:**
- None detected for user-facing API auth
  - Implementation: API routes in `internal/api/router.go` are exposed behind permissive CORS middleware in `internal/api/middleware.go`

## Monitoring & Observability

**Error Tracking:**
- None detected

**Logs:**
- Standard library request and process logging via `log.Printf` in `internal/api/middleware.go`, `cmd/theia/main.go`, `cmd/winbox-bridge/main.go`
- Prometheus-format internal metrics are exposed on `/metrics` by `internal/observability/registry.go` and wired in `cmd/theia/main.go`

## CI/CD & Deployment

**Hosting:**
- Docker Compose deployments using GHCR images for backend/frontend in `docker-compose.prod.yml`, `docker-compose.staging.yml`
- nginx serves the SPA and proxies API traffic in `Dockerfile.frontend`, `frontend/nginx.conf.template`

**CI Pipeline:**
- GitHub Actions runs backend/frontend tests, builds release and branch images, and publishes bridge binaries in `.github/workflows/ci.yml`
- Commit message validation runs in `.github/workflows/commit-messages.yml`

## Environment Configuration

**Required env vars:**
- `THEIA_ENCRYPTION_KEY` for backend crypto operations in `internal/crypto/encrypt.go`, `cmd/theia/main.go`
- `THEIA_DB_DRIVER`, `THEIA_DB_PATH`, `THEIA_DB_DSN`, `THEIA_DATA_DIR`, `THEIA_LISTEN_ADDR`, `THEIA_LOG_LEVEL` in `internal/config/config.go`
- `VITE_API_URL` for frontend dev proxy in `frontend/vite.config.ts`
- `THEIA_VERSION`, `BACKEND_PORT`, `FRONTEND_PORT`, `POSTGRES_DB`, `POSTGRES_USER`, `POSTGRES_PASSWORD` in `docker-compose.prod.yml`, `docker-compose.staging.yml`
- `GHCR_PAT` for CI and first-time GHCR login in `.github/workflows/ci.yml`, `.env.prod.example`, `.env.staging.example`

**Secrets location:**
- Deployment secrets are expected in local env files such as `.env.prod` and `.env.staging` referenced by `Makefile`, `docker-compose.prod.yml`, and `docker-compose.staging.yml`
- CI secrets are stored in GitHub Actions secrets, referenced as `secrets.GHCR_PAT` in `.github/workflows/ci.yml`
- WinBox bridge local secret is stored in the platform config file described by `cmd/winbox-bridge/config.go`

## Webhooks & Callbacks

**Incoming:**
- None detected; no webhook receiver routes are registered in `internal/api/router.go`

**Outgoing:**
- None detected; outbound integrations are polling/request based in `internal/metrics/prometheus.go`, `internal/snmp/client.go`, and `internal/ssh/client.go`

---

*Integration audit: 2026-04-19*
