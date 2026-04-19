# Architecture

**Analysis Date:** 2026-04-19

## Pattern Overview

**Overall:** Layered monolith with a Go backend, React frontend, and an event-driven runtime telemetry pipeline.

**Key Characteristics:**
- Bootstrap and dependency wiring happen centrally in `cmd/theia/main.go`.
- HTTP CRUD flows are separated into `internal/api`, `internal/service`, `internal/domain`, and `internal/repository/sqlite`.
- Live monitoring is handled by a dedicated in-memory pipeline across `internal/scheduler`, `internal/collector`, `internal/state`, `internal/worker`, and `internal/ws`.

## Layers

**Application entry points:**
- Purpose: Start binaries, load configuration, wire dependencies, and own process lifecycle.
- Location: `cmd/theia/main.go`, `cmd/theia-db-check/main.go`, `cmd/theia-db-migrate/main.go`, `cmd/theia-scale-lab/main.go`, `cmd/winbox-bridge/main.go`
- Contains: bootstrap config loading, DB startup, migrations, worker startup, HTTP server startup, CLI flags
- Depends on: `internal/config`, `internal/repository/sqlite`, `internal/service`, `internal/worker`, `internal/ws`, `internal/api`
- Used by: OS process startup and operational tooling

**HTTP/API layer:**
- Purpose: Expose `/api/v1/*` routes and translate HTTP requests into repository/service calls.
- Location: `internal/api/router.go`, `internal/api/*_handler.go`, `internal/api/middleware.go`
- Contains: route registration, request decoding, response encoding, middleware, download/upload exceptions
- Depends on: `internal/service`, `internal/domain`, `internal/repository/sqlite`, `internal/ws`
- Used by: `cmd/theia/main.go`

**Service/domain layer:**
- Purpose: Hold application rules around devices, backups, topology persistence, and restore workflows.
- Location: `internal/service/*.go`, `internal/domain/*.go`
- Contains: orchestration logic, business rules, interfaces, entity types, enums, change-event contracts
- Depends on: domain repository interfaces plus protocol helpers such as `internal/snmp`, `internal/ssh`, `internal/topology`
- Used by: `internal/api`, `internal/worker`, `cmd/theia/main.go`

**Persistence layer:**
- Purpose: Persist configuration and inventory state in SQLite/PostgreSQL-compatible repositories.
- Location: `internal/repository/sqlite/*.go`, `internal/repository/sqlite/migrations/*`, `internal/repository/sqlite/postgres_migrations/*`
- Contains: repo implementations, DB dialect abstraction, migrations, data migration helpers, tuning
- Depends on: `database/sql`, `internal/domain`, encryption helpers, SQL dialect wrapper in `internal/repository/sqlite/sql_store.go`
- Used by: `cmd/theia/main.go`, `internal/service`, `internal/api`

**Cached inventory layer:**
- Purpose: Keep device and link inventory resident in memory and apply repo change events incrementally.
- Location: `internal/cache/cache.go`
- Contains: `DeviceLinkCache`, lookup indexes, full-reload fallback, incremental repair handling
- Depends on: `internal/domain` repositories and change event channels from `internal/repository/sqlite/device_repo.go` and `internal/repository/sqlite/link_repo.go`
- Used by: `internal/scheduler`, `internal/worker`, legacy polling code in `internal/worker/poller.go`

**Telemetry pipeline layer:**
- Purpose: Schedule polling, collect runtime metrics, materialize snapshots, and publish real-time updates.
- Location: `internal/scheduler/scheduler.go`, `internal/collector/*.go`, `internal/state/store.go`, `internal/worker/pipeline.go`, `internal/worker/snapshot_builder.go`, `internal/ws/*.go`
- Contains: task scheduling, poll execution, volatile runtime state, snapshot building, WebSocket broadcasting
- Depends on: `internal/cache`, `internal/service`, `internal/domain`, `internal/metrics`, `internal/observability`
- Used by: `cmd/theia/main.go` and frontend clients connected through `/api/v1/ws`

**Frontend presentation layer:**
- Purpose: Render topology, dashboard, and settings UI against REST and WebSocket data.
- Location: `frontend/src/App.tsx`, `frontend/src/components/**/*.tsx`, `frontend/src/api/client.ts`, `frontend/src/hooks/useWebSocket.ts`
- Contains: view switching, canvas rendering, dashboard panels, API client wrappers, browser-side state composition
- Depends on: backend endpoints in `internal/api/router.go`
- Used by: browser entry point `frontend/src/main.tsx`

## Data Flow

**Bootstrap and runtime wiring:**

1. `cmd/theia/main.go` loads YAML/env config via `internal/config/config.go`.
2. `cmd/theia/main.go` opens the primary DB through `internal/repository/sqlite`, runs migrations, and seeds vendor config from `internal/vendor/embedded.go`.
3. `cmd/theia/main.go` constructs repositories, `internal/cache.DeviceLinkCache`, services, `internal/scheduler.Scheduler`, `internal/worker.PipelineOrchestrator`, `internal/ws.Hub`, and `internal/api.NewRouter`.
4. `cmd/theia/main.go` starts the HTTP server and exposes `/metrics` plus `/api/v1/*`.

**REST CRUD and settings flow:**

1. The frontend calls `fetch()` wrappers in `frontend/src/api/client.ts`.
2. `internal/api/router.go` dispatches to a handler in `internal/api/*_handler.go`.
3. The handler calls either a service such as `internal/service/device_service.go` or a repository directly such as `internal/repository/sqlite/position_repo.go`.
4. Repository writes publish change events from files like `internal/repository/sqlite/device_repo.go`, which feed `internal/cache/cache.go` and the runtime pipeline.

**Live telemetry and WebSocket flow:**

1. `internal/scheduler/scheduler.go` reads devices from `internal/cache/cache.go` and emits poll tasks by volatility class.
2. `internal/worker/pipeline.go` workers execute `internal/collector` collectors and apply results into `internal/state/store.go`.
3. `internal/worker/snapshot_builder.go` combines cached inventory plus runtime state into `internal/ws.SnapshotPayload`.
4. `internal/ws/hub.go` broadcasts snapshots and deltas through `internal/ws/handler.go` at `/api/v1/ws`, and `frontend/src/hooks/useWebSocket.ts` merges them into browser state.

**Frontend topology composition flow:**

1. `frontend/src/App.tsx` mounts all major views and passes the shared WebSocket snapshot down.
2. `frontend/src/components/canvas/useCanvasData.ts` combines REST inventory from `frontend/src/api/client.ts` with runtime snapshot data from `frontend/src/hooks/useWebSocket.ts`.
3. `frontend/src/components/Canvas.tsx` turns that composed state into nodes, edges, overlays, and side panels.

**State Management:**
- Use `internal/state/store.go` for volatile runtime metrics, health, reachability, and staleness.
- Use `internal/cache/cache.go` for DB-backed inventory state and link/device lookup acceleration.
- Use repository tables under `internal/repository/sqlite/*.go` for durable configuration and topology state.
- Use local React state in `frontend/src/App.tsx`, `frontend/src/components/Canvas.tsx`, and hooks such as `frontend/src/hooks/useWebSocket.ts` for UI state.

## Key Abstractions

**Domain entities and repository interfaces:**
- Purpose: Define the stable application model and storage contracts.
- Examples: `internal/domain/device.go`, `internal/domain/link.go`, `internal/domain/settings.go`, `internal/domain/change_event.go`
- Pattern: Keep backend contracts in `internal/domain` and implement them in `internal/repository/sqlite`.

**DeviceService:**
- Purpose: Orchestrate device lifecycle, SNMP discovery, topology discovery mode handling, and async re-probes.
- Examples: `internal/service/device_service.go`
- Pattern: Put business logic in services instead of handlers; inject repository interfaces plus protocol functions.

**DeviceLinkCache:**
- Purpose: Separate durable inventory reads from runtime polling by maintaining an in-memory inventory cache.
- Examples: `internal/cache/cache.go`
- Pattern: Use incremental repo change events first, then fall back to full reload when repair is required.

**PipelineOrchestrator:**
- Purpose: Coordinate scheduler tasks, collectors, snapshot refresh, and WebSocket broadcasts.
- Examples: `internal/worker/pipeline.go`, `internal/worker/snapshot_builder.go`
- Pattern: Centralize runtime monitoring orchestration in one worker layer instead of scattering polling across handlers.

**WebSocket hub and snapshot DTOs:**
- Purpose: Decouple runtime state production from connection fan-out.
- Examples: `internal/ws/hub.go`, `internal/ws/handler.go`, `frontend/src/types/metrics.ts`
- Pattern: Send an initial full snapshot, then merge sparse deltas on the client.

**Vendor registry:**
- Purpose: Provide vendor-specific detection and SNMP schema behavior from embedded or DB-backed config.
- Examples: `internal/vendor/embedded.go`, `internal/vendor/registry.go`, `internal/repository/sqlite/vendor_config_repo.go`
- Pattern: Seed from YAML/embedded files, then build the active registry from database records.

## Entry Points

**Primary backend server:**
- Location: `cmd/theia/main.go`
- Triggers: Main application process startup
- Responsibilities: Load config, open DB, run migrations, seed vendor registry, construct repos/services/pipeline, start HTTP and background workers

**Primary frontend app:**
- Location: `frontend/src/main.tsx`
- Triggers: Browser page load
- Responsibilities: Mount `frontend/src/App.tsx` into `#root`

**Frontend root composition:**
- Location: `frontend/src/App.tsx`
- Triggers: React app render
- Responsibilities: Own active view selection, area selection, shared WebSocket state, and view-to-view data handoff

**Database migration utility:**
- Location: `cmd/theia-db-migrate/main.go`
- Triggers: Manual CLI execution
- Responsibilities: Copy SQLite data into PostgreSQL using `internal/repository/sqlite/data_migrator.go`

**Database validation utility:**
- Location: `cmd/theia-db-check/main.go`
- Triggers: Manual CLI execution
- Responsibilities: Validate PostgreSQL production readiness and migration state

**Scale simulation utility:**
- Location: `cmd/theia-scale-lab/main.go`
- Triggers: Manual CLI execution
- Responsibilities: Run synthetic scale scenarios from `internal/scalelab`

**WinBox bridge companion service:**
- Location: `cmd/winbox-bridge/main.go`
- Triggers: Separate desktop/local process startup
- Responsibilities: Validate origin/host, decrypt launch tokens, and launch WinBox locally

## Error Handling

**Strategy:** Fail fast during process bootstrap, return structured HTTP errors at the API edge, and log background/runtime failures without crashing the server.

**Patterns:**
- Use `log.Fatalf` in `cmd/theia/main.go` and CLI entry points for unrecoverable startup errors.
- Use `writeError(...)` and method guards in `internal/api/router.go` and handler files for request-level failures.
- Wrap repo/service failures with context using `fmt.Errorf(...: %w)` in files such as `internal/service/device_service.go` and `internal/repository/sqlite/device_repo.go`.
- Guard middleware-sensitive routes in `internal/api/router.go` by bypassing JSON/body-size wrappers for WebSocket, downloads, and multipart restore endpoints.

## Cross-Cutting Concerns

**Logging:** `log.Printf` is the shared logging mechanism across `cmd/theia/main.go`, `internal/api/middleware.go`, `internal/service/*.go`, `internal/worker/*.go`, and `internal/ws/*.go`.

**Validation:** Request and payload validation live near boundaries in `internal/api/*_handler.go`, `frontend/src/api/client.ts`, and parsers in `frontend/src/types/metrics.ts`.

**Authentication:** The main API in `internal/api/router.go` does not apply authentication middleware. The separate bridge process in `cmd/winbox-bridge/main.go` enforces custom Origin and Host checks plus encrypted launch tokens.

---

*Architecture analysis: 2026-04-19*
