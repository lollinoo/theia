# Technology Stack

**Analysis Date:** 2026-04-19

## Languages

**Primary:**
- Go 1.24.0 - backend API, workers, collectors, desktop bridge, and CLI tools in `go.mod`, `cmd/theia/main.go`, `cmd/winbox-bridge/main.go`
- TypeScript - React frontend SPA and tests in `frontend/package.json`, `frontend/src/main.tsx`, `frontend/src/api/client.ts`

**Secondary:**
- YAML - bootstrap config, vendor registry, Docker Compose, Prometheus, and CI config in `config.example.yaml`, `vendors/mikrotik.yaml`, `docker-compose.yml`, `.github/workflows/ci.yml`
- SQL - embedded SQLite and PostgreSQL migrations in `internal/repository/sqlite/migrations/` and `internal/repository/sqlite/postgres_migrations/`

## Runtime

**Environment:**
- Go 1.24 backend runtime and build target in `go.mod`, `Dockerfile`
- Node.js 22 for frontend dev/build in `Dockerfile.frontend`, `.github/workflows/ci.yml`
- nginx 1.27 serves the production SPA in `Dockerfile.frontend`

**Package Manager:**
- Go modules via `go.mod` and `go.sum`
- npm via `frontend/package.json`
- Lockfile: present in `frontend/package-lock.json`; backend dependency lock state is captured in `go.sum`

## Frameworks

**Core:**
- Standard `net/http` router/server for the API in `internal/api/router.go`, `cmd/theia/main.go`
- React 18 SPA in `frontend/package.json`, `frontend/src/App.tsx`, `frontend/src/main.tsx`
- Vite 7 for frontend dev server and bundling in `frontend/package.json`, `frontend/vite.config.ts`

**Testing:**
- Go `testing` package for backend tests throughout `cmd/` and `internal/`
- Vitest 4 with jsdom and Testing Library in `frontend/package.json`, `frontend/vitest.config.ts`

**Build/Dev:**
- Docker multi-stage builds for backend and frontend in `Dockerfile`, `Dockerfile.frontend`
- Docker Compose for dev, test, prod, staging, and WISP lab orchestration in `docker-compose.yml`, `docker-compose.prod.yml`, `docker-compose.staging.yml`, `docker-compose.wisp-lab.yml`
- Air hot reload for backend dev in `.air.toml`, `Dockerfile`
- Tailwind CSS v4 via Vite plugin in `frontend/package.json`, `frontend/vite.config.ts`

## Key Dependencies

**Critical:**
- `github.com/gosnmp/gosnmp` v1.43.2 - SNMP polling and discovery client in `go.mod`, `internal/snmp/client.go`
- `github.com/gorilla/websocket` v1.5.3 - live snapshot and status streaming in `go.mod`, `internal/ws/handler.go`, `internal/ws/hub.go`
- `github.com/mattn/go-sqlite3` v1.14.22 - default embedded database driver in `go.mod`, `internal/repository/sqlite/db_tuning.go`
- `github.com/jackc/pgx/v5` v5.5.4 - PostgreSQL driver for production/staging in `go.mod`, `cmd/theia/main.go`, `cmd/theia-db-migrate/main.go`
- `github.com/pkg/sftp` v1.13.10 - remote config backup file transfer in `go.mod`, `internal/ssh/client.go`, `internal/service/backup_service.go`
- `react` / `react-dom` 18.3.1 - frontend rendering in `frontend/package.json`

**Infrastructure:**
- `github.com/golang-migrate/migrate/v4` v4.19.1 - DB migrations in `go.mod`, `internal/repository/sqlite/migrations.go`
- `gopkg.in/yaml.v3` v3.0.1 - bootstrap and vendor config parsing in `go.mod`, `internal/config/config.go`, `internal/vendor/registry.go`
- `fyne.io/systray` v1.12.0 - desktop WinBox bridge tray app in `go.mod`, `cmd/winbox-bridge/main.go`
- `@vitejs/plugin-react` 5.0.1 and `@tailwindcss/vite` 4.2.2 - frontend toolchain in `frontend/package.json`, `frontend/vite.config.ts`
- `@xyflow/react` 12.10.1 and `d3-force` 3.0.0 - topology graph UI in `frontend/package.json`

## Configuration

**Environment:**
- Bootstrap config comes from `config.yaml`/`config.example.yaml` plus env overrides in `internal/config/config.go`
- Backend runtime env vars include `THEIA_DB_DRIVER`, `THEIA_LISTEN_ADDR`, `THEIA_DB_PATH`, `THEIA_DB_DSN`, `THEIA_DATA_DIR`, `THEIA_LOG_LEVEL`, and `THEIA_BRIDGE_BINARIES_DIR` in `internal/config/config.go`
- Additional backend env vars are consumed directly in `cmd/theia/main.go`: `THEIA_CONFIG`, `THEIA_VENDORS_DIR`, `THEIA_BACKUP_DIR`, `THEIA_INSTANCE_BACKUP_DIR`, and required `THEIA_ENCRYPTION_KEY`
- Frontend dev proxy uses `VITE_API_URL` in `frontend/vite.config.ts`; production nginx proxy uses `BACKEND_PORT` in `Dockerfile.frontend`, `docker-compose.prod.yml`
- Example environment files exist at `.env.prod.example` and `.env.staging.example`; `.env.prod` and `.env.staging` are expected but not read

**Build:**
- Backend build metadata is injected with `VERSION`, `GIT_COMMIT`, and `BUILD_DATE` in `Dockerfile`, `.air.toml`, `.github/workflows/ci.yml`
- Frontend build metadata is injected with `VERSION` in `Dockerfile.frontend`, `frontend/vite.config.ts`
- CI builds and pushes container images from `.github/workflows/ci.yml`

## Platform Requirements

**Development:**
- Docker and Docker Compose are the primary local workflow in `Makefile`, `docker-compose.yml`
- Local Go 1.24 and CGO-compatible toolchain are required only for direct host builds in `go.mod`, `Dockerfile`
- Local Node.js 22 + npm are required for direct frontend work outside containers in `Dockerfile.frontend`, `.github/workflows/ci.yml`

**Production:**
- Container deployment targets GHCR-hosted backend and frontend images in `docker-compose.prod.yml`, `.github/workflows/ci.yml`
- Production reference database is PostgreSQL 17 in `docker-compose.prod.yml`, `config.example.yaml`
- Frontend is served as static assets behind nginx in `Dockerfile.frontend`, `docker-compose.prod.yml`

---

*Stack analysis: 2026-04-19*
