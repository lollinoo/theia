# Architecture

**Analysis Date:** 2026-04-23

## Pattern Overview

**Overall:** Modular Go backend with repository/service/API layers, event-driven polling runtime, and a Vite React topology UI.

**Key Characteristics:**
- Backend entrypoint `cmd/theia/main.go` delegates runtime composition to `cmd/theia/runtime_bootstrap.go`; keep dependency wiring centralized there.
- Domain models and repository interfaces live in `internal/domain/`; concrete persistence lives in `internal/repository/sqlite/` with SQLite and PostgreSQL dialect support.
- HTTP is standard `net/http` through `internal/api/router.go`; handlers wrap services/repositories and return JSON API-style resources.
- Runtime telemetry is asynchronous: `internal/scheduler/`, `internal/collector/`, `internal/state/`, `internal/worker/`, and `internal/ws/` cooperate through channels and snapshots.
- Frontend state is React hook/component driven in `frontend/src/`; REST calls are centralized in `frontend/src/api/client.ts`, WebSocket runtime data in `frontend/src/hooks/useWebSocket.ts`.

## Layers

**Command / Runtime Composition:**
- Purpose: Parse flags, load config, open DB, run migrations, construct repositories/services/workers, and own HTTP lifecycle.
- Location: `cmd/theia/`
- Contains: `cmd/theia/main.go`, `cmd/theia/runtime_bootstrap.go`, runtime path helpers, vendor registry bootstrap.
- Depends on: `internal/config/`, `internal/repository/sqlite/`, `internal/service/`, `internal/worker/`, `internal/api/`, `internal/ws/`.
- Used by: Main application binary `cmd/theia/main.go` and runtime tests in `cmd/theia/*_test.go`.

**HTTP API:**
- Purpose: Route `/api/v1/*`, translate HTTP requests to service/repository calls, apply middleware, and expose WebSocket/metrics entrypoints.
- Location: `internal/api/`
- Contains: Resource handlers such as `internal/api/device_handler.go`, `internal/api/link_handler.go`, `internal/api/backup_handler.go`, `internal/api/area_handler.go`, plus middleware in `internal/api/middleware.go`.
- Depends on: `internal/domain/` interfaces, `internal/service/` services, `internal/repository/sqlite/` repos where concrete repo features are needed, `internal/vendor/`, `internal/ws/`.
- Used by: `cmd/theia/runtime_bootstrap.go` via `api.NewRouter(...)`.

**Domain Contracts:**
- Purpose: Own core business types, enums, normalization helpers, and repository interfaces.
- Location: `internal/domain/`
- Contains: Devices in `internal/domain/device.go`, links in `internal/domain/link.go`, settings in `internal/domain/settings.go`, metrics in `internal/domain/metrics.go`, credentials in `internal/domain/credential_profile.go`.
- Depends on: Standard library plus `github.com/google/uuid`.
- Used by: All backend layers; new business concepts should define DTO-independent core types here before handlers or repositories consume them.

**Persistence:**
- Purpose: Store durable config/topology/backup data and emit cache/runtime invalidation signals.
- Location: `internal/repository/sqlite/`
- Contains: Repositories such as `internal/repository/sqlite/device_repo.go`, `internal/repository/sqlite/link_repo.go`, `internal/repository/sqlite/settings_repo.go`, migrations in `internal/repository/sqlite/migrations/`, PostgreSQL migrations in `internal/repository/sqlite/postgres_migrations/`.
- Depends on: `database/sql`, `internal/domain/`, dialect wrapper `internal/repository/sqlite/sql_store.go`, encryption helpers used by credential repositories.
- Used by: `cmd/theia/runtime_bootstrap.go`, `internal/service/`, and selected API handlers.

**Service / Business Orchestration:**
- Purpose: Coordinate multi-repository mutations, SNMP discovery persistence, backup/restore workflows, and device management rules.
- Location: `internal/service/`
- Contains: `internal/service/device_service.go`, `internal/service/device_mutation_service.go`, `internal/service/device_discovery_coordinator.go`, `internal/service/backup_service.go`, `internal/service/instance_backup_service.go`, `internal/service/restore_coordinator.go`.
- Depends on: `internal/domain/` repositories, `internal/snmp/`, `internal/topology/`, `internal/vendor/`, `internal/ssh/`.
- Used by: `internal/api/`, `internal/worker/`, and runtime bootstrap.

**Polling Runtime:**
- Purpose: Schedule device poll tasks, collect metrics/static data, update in-memory state, persist topology changes, and broadcast snapshots.
- Location: `internal/scheduler/`, `internal/collector/`, `internal/state/`, `internal/worker/`, `internal/cache/`.
- Contains: Task scheduler `internal/scheduler/scheduler.go`, SNMP/Prometheus collectors in `internal/collector/`, runtime state store `internal/state/store.go`, orchestrator `internal/worker/pipeline.go`, cache `internal/cache/device_link_cache.go`.
- Depends on: `internal/domain/`, `internal/service/`, `internal/ws/`, `internal/metrics/`, `internal/pollingbudget/`.
- Used by: `cmd/theia/runtime_bootstrap.go`, WebSocket snapshots, health endpoint.

**Realtime Delivery:**
- Purpose: Maintain WebSocket clients, detail subscriptions, full snapshots, deltas, alerts, and backpressure/resync behavior.
- Location: `internal/ws/`
- Contains: Hub/client lifecycle in `internal/ws/hub.go`, HTTP upgrade handler in `internal/ws/handler.go`, message contracts in `internal/ws/messages.go`.
- Depends on: `github.com/gorilla/websocket`, `internal/observability/`.
- Used by: `internal/worker/pipeline.go` for broadcasts and `internal/api/router.go` for `/api/v1/ws`.

**Frontend Application:**
- Purpose: Render area hub, topology canvas, dashboard, settings panels, and consume backend REST/WebSocket APIs.
- Location: `frontend/src/`
- Contains: Entrypoint `frontend/src/main.tsx`, root composition `frontend/src/App.tsx`, API client `frontend/src/api/client.ts`, WebSocket hook `frontend/src/hooks/useWebSocket.ts`, components in `frontend/src/components/`.
- Depends on: React, `@xyflow/react`, Vite proxy configuration in `frontend/vite.config.ts`.
- Used by: Browser UI served through the frontend container/build.

**WinBox Bridge:**
- Purpose: Local companion service for launching MikroTik WinBox with encrypted credentials from Theia.
- Location: `cmd/winbox-bridge/`
- Contains: Local HTTP server lifecycle `cmd/winbox-bridge/server.go`, launch token decryption and WinBox discovery in `cmd/winbox-bridge/main.go`, platform-specific launch files.
- Depends on: `fyne.io/systray`, local OS process launch, AES-GCM token secret.
- Used by: Backend bridge endpoints in `internal/api/bridge_handler.go` and frontend WinBox flow hooks in `frontend/src/hooks/useWinboxFlow.ts`.

## Data Flow

**Device Create / Discovery Flow:**

1. UI calls `createDevice()` in `frontend/src/api/client.ts`, posting to `/api/v1/devices` registered in `internal/api/router.go`.
2. `internal/api/device_handler.go` validates request data and calls `service.DeviceService.AddDevice()` in `internal/service/device_service.go`.
3. `internal/service/device_mutation_service.go` persists the device via `domain.DeviceRepository`, implemented by `internal/repository/sqlite/device_repo.go`.
4. Non-virtual devices trigger asynchronous SNMP discovery through `internal/service/device_discovery_coordinator.go` and `internal/snmp/`.
5. Repository change channels invalidate `internal/cache/DeviceLinkCache` and notify `internal/worker/PipelineOrchestrator` to rebuild/broadcast topology snapshots.

**Polling / Metrics Flow:**

1. `internal/scheduler/scheduler.go` reads devices through `internal/cache/device_link_cache.go` and emits `scheduler.PollTask` values.
2. `internal/worker/pipeline_task_runner.go` receives tasks from `internal/worker/pipeline.go` and invokes collectors in `internal/collector/`.
3. `internal/collector/performance.go`, `internal/collector/operational.go`, and `internal/collector/static.go` poll SNMP using vendor OID definitions from `internal/vendor/`.
4. `internal/collector/prometheus.go` enriches data when Prometheus is configured through `internal/metrics/`.
5. `internal/state/store.go` stores volatile metrics/reachability/freshness and emits changed device IDs.
6. `internal/worker/pipeline_snapshot_broadcaster.go` builds snapshots/deltas and broadcasts via `internal/ws/hub.go`.

**Frontend Realtime Flow:**

1. `frontend/src/App.tsx` opens `/api/v1/ws` with `useWebSocket()` from `frontend/src/hooks/useWebSocket.ts`.
2. `internal/ws/handler.go` upgrades the connection and immediately sends snapshot, alert, and Prometheus status payloads.
3. `frontend/src/types/metrics.ts` parses snapshot/delta/alert messages; `useWebSocket()` merges deltas and dispatches resync/topology events.
4. `frontend/src/components/Canvas.tsx` combines persistent topology from REST via `useCanvasData()` with runtime snapshot data.
5. `frontend/src/components/canvas/topologyComposer.ts` builds React Flow nodes/edges from API devices, links, saved positions, runtime metrics, and alerts.

**Backup / Restore Flow:**

1. API routes in `internal/api/backup_handler.go` and `internal/api/instance_backup_handler.go` receive backup requests from `frontend/src/api/client.ts`.
2. `internal/service/backup_service.go` connects through `internal/ssh/` and writes device backup files under runtime backup paths from `cmd/theia/runtime_paths.go`.
3. `internal/service/instance_backup_service.go` creates full instance backups using DB, device backup directory, known hosts file, and encryption key.
4. Schedulers in `internal/worker/backup_scheduler.go` and `internal/worker/device_backup_scheduler.go` run recurring backup jobs.
5. Startup restore coordination happens before DB migrations in `cmd/theia/runtime_bootstrap.go` through `internal/service/restore_coordinator.go`.

**State Management:**
- Durable state: PostgreSQL or SQLite through `internal/repository/sqlite/` and migrations in `internal/repository/sqlite/migrations/` plus `internal/repository/sqlite/postgres_migrations/`.
- Runtime state: in-memory `internal/state/store.go` for volatile metrics/reachability/freshness.
- DB-backed read cache: `internal/cache/device_link_cache.go` for devices/links/interfaces/credentials used by scheduler and pipeline.
- Frontend state: React `useState`/hooks in `frontend/src/App.tsx`, `frontend/src/components/Canvas.tsx`, and `frontend/src/components/canvas/useCanvasData.ts`.

## Key Abstractions

**Repository Interfaces:**
- Purpose: Separate domain persistence contracts from SQL implementations.
- Examples: `internal/domain/device.go`, `internal/domain/link.go`, `internal/domain/settings.go`, `internal/domain/position.go`.
- Pattern: Define interface in `internal/domain/`; implement with `*Repo` structs in `internal/repository/sqlite/`; inject implementations in `cmd/theia/runtime_bootstrap.go`.

**Service Objects:**
- Purpose: Encapsulate business workflows that touch multiple repositories or external systems.
- Examples: `internal/service/device_service.go`, `internal/service/backup_service.go`, `internal/service/instance_backup_service.go`.
- Pattern: Constructor injection with small function/interface seams such as `service.DiscoverFunc` for testability.

**PipelineOrchestrator:**
- Purpose: Own polling lifecycle, worker goroutines, runtime state, snapshot broadcasting, and health status.
- Examples: `internal/worker/pipeline.go`, `internal/worker/pipeline_task_runner.go`, `internal/worker/pipeline_snapshot_broadcaster.go`.
- Pattern: Long-running component with `Start(context.Context)`, `Stop()`, typed channels, coalesced broadcasts, and WebSocket resync fallbacks.

**DeviceLinkCache:**
- Purpose: Provide DB-backed, invalidation-aware topology inventory to scheduler and pipeline without querying repositories on every poll.
- Examples: `internal/cache/device_link_cache.go`, constructed in `cmd/theia/runtime_bootstrap.go`.
- Pattern: Repository-backed cache invalidated by non-blocking repo change channels.

**WebSocket Message Contracts:**
- Purpose: Provide versioned realtime envelopes for snapshots, deltas, alerts, Prometheus status, topology changes, and resync requests.
- Examples: Backend `internal/ws/messages.go`, frontend parser `frontend/src/types/metrics.ts`, client merger `frontend/src/hooks/useWebSocket.ts`.
- Pattern: Server emits full snapshot first, then versioned deltas; client requests/handles resync when versions diverge.

**Frontend API Parsers:**
- Purpose: Normalize backend JSON API-style responses into UI types and fail early on invalid payloads.
- Examples: `frontend/src/types/api.ts`, `frontend/src/api/client.ts`.
- Pattern: Keep fetch functions in `frontend/src/api/client.ts`; keep payload validation/defaulting in `frontend/src/types/api.ts`.

**Canvas Topology Composition:**
- Purpose: Transform devices, links, positions, runtime metrics, alerts, and area filters into React Flow nodes and edges.
- Examples: `frontend/src/components/Canvas.tsx`, `frontend/src/components/canvas/topologyComposer.ts`, `frontend/src/components/canvas/nodeBuilder.ts`, `frontend/src/components/canvas/edgeBuilder.ts`.
- Pattern: Keep pure topology transforms in `frontend/src/components/canvas/`; keep React event wiring in `frontend/src/components/Canvas.tsx`.

## Entry Points

**Backend Server:**
- Location: `cmd/theia/main.go`
- Triggers: `go run ./cmd/theia`, Docker backend service, built `theia` binary.
- Responsibilities: Resolve config path, run `runtimeBootstrap.Run()`, and terminate through `log.Fatal` on startup/runtime errors.

**Runtime Bootstrap:**
- Location: `cmd/theia/runtime_bootstrap.go`
- Triggers: Called by `cmd/theia/main.go`.
- Responsibilities: Load config, enforce database policy, prepare runtime directories, apply pending restores, migrate DB, seed/load vendor registry, wire repositories/services/workers/API, start HTTP server, and handle SIGINT/SIGTERM shutdown.

**HTTP API Router:**
- Location: `internal/api/router.go`
- Triggers: Requests routed by `http.Server` in `cmd/theia/runtime_bootstrap.go`.
- Responsibilities: Register `/api/v1/devices`, `/api/v1/links`, `/api/v1/positions`, `/api/v1/settings`, `/api/v1/snmp-profiles`, `/api/v1/areas`, `/api/v1/credential-profiles`, `/api/v1/backups`, `/api/v1/vendors`, `/api/v1/instance-backups`, `/api/v1/bridge`, `/api/v1/health`, `/api/v1/prometheus/health`, and `/api/v1/ws`.

**WebSocket Endpoint:**
- Location: `internal/ws/handler.go`
- Triggers: Browser connects to `/api/v1/ws` through `frontend/src/hooks/useWebSocket.ts`.
- Responsibilities: Upgrade HTTP, register client, send initial snapshots, process detail subscribe/unsubscribe control messages, and manage ping/pong lifecycle.

**Metrics Endpoint:**
- Location: `internal/observability/` via `observability.Handler()` in `cmd/theia/runtime_bootstrap.go`
- Triggers: HTTP GET `/metrics`.
- Responsibilities: Expose process/runtime/pipeline/WebSocket metrics outside the JSON API middleware chain.

**Frontend App:**
- Location: `frontend/src/main.tsx`
- Triggers: Browser loads Vite/compiled frontend.
- Responsibilities: Mount `frontend/src/App.tsx` under React StrictMode and load global styles from `frontend/src/index.css`.

**Vite Dev Proxy:**
- Location: `frontend/vite.config.ts`
- Triggers: `npm --prefix frontend run dev`.
- Responsibilities: Proxy `/api` and `/api/v1/ws` to backend target from `VITE_API_URL` or `http://backend:8080`.

**WinBox Bridge Binary:**
- Location: `cmd/winbox-bridge/main.go`
- Triggers: Local bridge executable and systray controls.
- Responsibilities: Run local-only bridge server, decrypt launch tokens, discover WinBox binary, and launch WinBox processes.

**Database Utilities:**
- Location: `cmd/theia-db-migrate/main.go`, `cmd/theia-db-check/main.go`, `cmd/theia-scale-lab/main.go`
- Triggers: Make targets or direct `go run ./cmd/...` commands.
- Responsibilities: Migrate SQLite data to PostgreSQL, inspect DB state, and generate scale-lab data.

## Error Handling

**Strategy:** Return wrapped Go errors across backend layers, convert API failures to JSON error responses at handlers/middleware, and keep long-running goroutines alive by logging recoverable poll/broadcast failures.

**Patterns:**
- Wrap startup and service errors with context using `fmt.Errorf("...: %w", err)` in `cmd/theia/runtime_bootstrap.go`, `internal/service/`, and repositories.
- Handlers write status-coded JSON errors through helpers in `internal/api/` such as `writeError(...)`.
- Runtime loops log recoverable failures with `log.Printf` in `internal/scheduler/scheduler.go`, `internal/ws/hub.go`, and `internal/worker/`.
- WebSocket backpressure uses resync markers and fallback snapshots in `internal/ws/hub.go` instead of blocking producers.
- Frontend API errors are normalized by `frontend/src/api/client.ts` into `ValidationError`, `ServerError`, or contextual `Error` instances.

## Cross-Cutting Concerns

**Logging:** Standard library `log` is used in `cmd/theia/`, `internal/api/middleware.go`, `internal/scheduler/`, `internal/ws/`, and service code. Keep runtime logs contextual and avoid logging credentials.

**Validation:** Backend handlers validate HTTP method/path/body in `internal/api/*_handler.go`; domain enum normalization lives in `internal/domain/`; frontend response parsing/defaulting lives in `frontend/src/types/api.ts` and `frontend/src/types/metrics.ts`.

**Authentication:** User authentication is not detected. Security controls are data/transport scoped: SNMP/SSH credentials are encrypted by repositories/services, bridge launch tokens use AES-GCM in `cmd/winbox-bridge/main.go`, and CORS/private-network handling lives in `internal/api/middleware.go` and `cmd/winbox-bridge/main.go`.

---

*Architecture analysis: 2026-04-23*
