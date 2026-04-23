# Technology Stack

**Analysis Date:** 2026-04-23

## Languages

**Primary:**
- Go 1.24.0 - Backend API, SNMP/SSH collectors, scheduling pipeline, database migrations, CLI utilities, and WinBox bridge in `go.mod`, `cmd/theia/main.go`, `cmd/theia/runtime_bootstrap.go`, `cmd/winbox-bridge/main.go`, and `internal/`.
- TypeScript 5.7.2 - React frontend source and tests in `frontend/package.json`, `frontend/tsconfig.app.json`, and `frontend/src/`.

**Secondary:**
- YAML - Runtime/bootstrap config, vendor registry, Prometheus config, Docker Compose config in `config.example.yaml`, `vendors/default.yaml`, `vendors/mikrotik.yaml`, `docker/prometheus/prometheus.yml`, and `docker-compose.yml`.
- SQL - Embedded migrations and repository queries in `internal/repository/sqlite/migrations/` and `internal/repository/sqlite/`.
- Shell/Make - Local dev, CI gates, and release helpers in `Makefile` and `scripts/`.

## Runtime

**Environment:**
- Go runtime 1.24 with CGO enabled for SQLite support; backend Docker stages use `golang:1.24-bookworm` and production uses `debian:bookworm-slim` in `Dockerfile`.
- Node.js 22 for frontend development/build; Docker uses `node:22-alpine` in `Dockerfile.frontend`, and CI uses `actions/setup-node@v4` with `node-version: 22` in `.github/workflows/ci.yml`.
- Browser runtime for the React SPA served by Vite dev server or nginx production image from `Dockerfile.frontend` and `frontend/nginx.conf.template`.

**Package Manager:**
- Go modules - `go.mod` and `go.sum` present at repository root.
- npm - `frontend/package-lock.json` present; use `npm ci` for reproducible frontend installs.
- Lockfile: present for both Go (`go.sum`) and frontend npm (`frontend/package-lock.json`).

## Frameworks

**Core:**
- Standard Go `net/http` - Backend REST API and middleware; routes are registered in `internal/api/router.go` without a web framework.
- React 18.3.1 - SPA UI in `frontend/src/`; dependencies declared in `frontend/package.json`.
- Vite 7.0.6 - Frontend dev server/build and API/WebSocket proxy in `frontend/vite.config.ts`.
- Tailwind CSS 4.2.2 via `@tailwindcss/vite` - UI styling pipeline configured in `frontend/package.json` and `frontend/vite.config.ts`.
- nginx 1.27-alpine - Production frontend static hosting and `/api/` proxy in `Dockerfile.frontend` and `frontend/nginx.conf.template`.

**Testing:**
- Go built-in test runner - Backend and bridge tests use `go test ./...`; PR gates are in `Makefile` and `.github/workflows/ci.yml`.
- Vitest 4.1.0 with jsdom - Frontend unit/component tests configured in `frontend/vitest.config.ts`.
- Playwright 1.54.2 - Browser E2E tests run with `npm --prefix frontend run e2e` from `frontend/package.json` and CI `browser-e2e` job in `.github/workflows/ci.yml`.
- Testing Library - React tests use `@testing-library/react`, `@testing-library/user-event`, and `@testing-library/jest-dom` from `frontend/package.json`.

**Build/Dev:**
- Air v1.61.5 - Backend hot reload in `Dockerfile` dev stage and `.air.toml`.
- Docker Compose - Dev/test/staging/prod orchestration in `docker-compose.yml`, `docker-compose.staging.yml`, and `docker-compose.prod.yml`.
- Biome 1.9.4 - Frontend formatting/lint/check commands in `frontend/package.json`.
- TypeScript project build - `tsc -b tsconfig.app.json` in `frontend/package.json`.
- GitHub Actions - CI, GHCR image publishing, and bridge release asset builds in `.github/workflows/ci.yml`.

## Key Dependencies

**Critical:**
- `github.com/gosnmp/gosnmp` v1.43.2 - Direct SNMP v2c/v3 polling, discovery, and metric collection in `internal/snmp/client.go` and `cmd/theia/main.go`.
- `github.com/jackc/pgx/v5` v5.5.4 - PostgreSQL driver registered in `cmd/theia/main.go` and opened via `internal/repository/sqlite/db_tuning.go`.
- `github.com/mattn/go-sqlite3` v1.14.22 - SQLite small-install/lab backend opened via `internal/repository/sqlite/db_tuning.go`; CGO is required.
- `github.com/gorilla/websocket` v1.5.3 - Live snapshot, alert, and Prometheus status WebSocket transport in `internal/ws/handler.go` and `internal/ws/hub.go`.
- `golang.org/x/crypto` v0.45.0 - SSH client auth and AES-related crypto support in `internal/ssh/client.go` and `internal/crypto/encrypt.go`.
- `github.com/pkg/sftp` v1.13.10 - Device backup file downloads over SFTP in `internal/ssh/client.go`.
- `gopkg.in/yaml.v3` v3.0.1 - Bootstrap config and vendor registry loading in `internal/config/config.go` and `vendors/*.yaml`.
- `fyne.io/systray` v1.12.0 - Desktop tray integration for `cmd/winbox-bridge/main.go`.
- `@xyflow/react` 12.10.1 - Topology/canvas graph UI in `frontend/src/components/canvas/`.
- `d3-force` 3.0.0 - Force layout support for topology visualization in frontend code under `frontend/src/`.

**Infrastructure:**
- PostgreSQL 17 - Standard development, staging, and production database path; configured in `Dockerfile`, `docker-compose.yml`, `docker-compose.staging.yml`, and `.env.prod.example`.
- Prometheus - Optional metrics/enrichment service queried by backend and configured for local dev in `docker/prometheus/prometheus.yml`.
- Prometheus SNMP exporter - Dev metrics stack uses `prom/snmp-exporter` with `docker/prometheus/snmp.yml`.
- GHCR - Backend and frontend images are published to `ghcr.io/lollinoo/theia-backend` and `ghcr.io/lollinoo/theia-frontend` in `.github/workflows/ci.yml`.
- Watchtower - Staging auto-update service in `docker-compose.staging.yml`.

## Configuration

**Environment:**
- Bootstrap config loads YAML first, then env overrides in `internal/config/config.go`; default config path is `config.yaml`, overridable with `THEIA_CONFIG` or `-config` in `cmd/theia/main.go`.
- Required backend secret: `THEIA_ENCRYPTION_KEY` is loaded by `internal/crypto/encrypt.go` and used for AES-GCM encryption of SNMP credentials, SSH/WinBox profile secrets, and backups.
- Database env vars: `THEIA_DB_DRIVER`, `THEIA_DB_DSN`, `THEIA_DB_PATH`, `THEIA_ALLOW_SQLITE_SMALL_INSTALL`, `THEIA_DATA_DIR` from `internal/config/config.go`, `config.example.yaml`, `.env.prod.example`, and `.env.staging.example`.
- Runtime settings live in the database via `internal/domain/settings.go`; key settings include `prometheus_url`, `grafana_url`, `polling_interval_seconds`, SNMP worker/timeout settings, backup intervals, `bridge_secret`, and `bridge_port`.
- Frontend dev API target uses `VITE_API_URL` in `frontend/vite.config.ts`; production nginx proxies to backend using `BACKEND_PORT` in `frontend/nginx.conf.template`.
- `.env.prod.example` and `.env.staging.example` document deployment env var names; do not commit real `.env.prod` or `.env.staging` files.

**Build:**
- Backend Docker build injects `VERSION`, `GIT_COMMIT`, and `BUILD_DATE` into `internal/version` via `Dockerfile` ldflags.
- Frontend Docker build injects `VERSION` into Vite as `__APP_VERSION__` via `Dockerfile.frontend` and `frontend/vite.config.ts`.
- CI quality gates are `make backend-fast`, `make frontend-fast`, `make realtime-stress`, `make collector-contract`, and `make browser-e2e` in `Makefile` and `.github/workflows/ci.yml`.
- Release builds publish Docker images and cross-compiled bridge binaries for windows/linux/darwin amd64/arm64 in `.github/workflows/ci.yml`.

## Platform Requirements

**Development:**
- Docker + Docker Compose for the full dev/test stack in `docker-compose.yml`.
- Go 1.24 with CGO and C compiler for backend builds that include `github.com/mattn/go-sqlite3`; Docker dev image installs `gcc` and `libc6-dev` in `Dockerfile`.
- Node.js 22 and npm for frontend development in `frontend/package.json` and `.github/workflows/ci.yml`.
- PostgreSQL is the normal local backend (`make dev`); SQLite requires explicit `THEIA_ALLOW_SQLITE_SMALL_INSTALL=true` in `cmd/theia/runtime_bootstrap.go`.

**Production:**
- Backend runs as a Debian-based container from `Dockerfile` with `pg_dump`, `pg_restore`, and `psql` copied from PostgreSQL 17 tooling for backup/restore.
- Frontend runs as nginx static/proxy container from `Dockerfile.frontend`.
- PostgreSQL is the production reference database; `cmd/theia/runtime_bootstrap.go` rejects PostgreSQL startup without `THEIA_DB_DSN`.
- Deployment images are expected from GHCR and configured by `.env.prod.example`, `docker-compose.prod.yml`, and `.github/workflows/ci.yml`.

---

*Stack analysis: 2026-04-23*
