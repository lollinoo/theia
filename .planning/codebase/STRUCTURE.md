# Codebase Structure

**Analysis Date:** 2026-04-19

## Directory Layout

```text
[project-root]/
├── cmd/                           # Go binary entry points
│   ├── theia/                     # Main backend server
│   ├── theia-db-check/            # PostgreSQL validation CLI
│   ├── theia-db-migrate/          # SQLite -> PostgreSQL migration CLI
│   ├── theia-scale-lab/           # Synthetic scale test runner
│   └── winbox-bridge/             # Local WinBox launcher companion service
├── internal/                      # Backend application packages
│   ├── api/                       # HTTP router, handlers, middleware
│   ├── cache/                     # In-memory device/link inventory cache
│   ├── collector/                 # SNMP and Prometheus collectors
│   ├── config/                    # Bootstrap config loading
│   ├── domain/                    # Entities and repository interfaces
│   ├── repository/sqlite/         # DB implementations and migrations
│   ├── scheduler/                 # Poll task scheduler
│   ├── service/                   # Business logic and orchestration
│   ├── state/                     # Volatile runtime state store
│   ├── vendor/                    # Embedded vendor definitions and registry
│   ├── worker/                    # Background orchestration and snapshot building
│   └── ws/                        # WebSocket hub and message contracts
├── frontend/                      # React/Vite frontend app
│   ├── public/                    # Static assets
│   └── src/                       # Frontend source code
├── docker/                        # Container and lab support assets
├── scripts/                       # Repository scripts
├── vendors/                       # External binary/vendor assets
├── config.yaml                    # Local bootstrap config
├── config.example.yaml            # Example bootstrap config
└── go.mod                         # Backend module definition
```

## Directory Purposes

**`cmd/`:**
- Purpose: Keep each executable's `main` package isolated.
- Contains: One directory per binary.
- Key files: `cmd/theia/main.go`, `cmd/winbox-bridge/main.go`, `cmd/theia-db-migrate/main.go`

**`internal/api/`:**
- Purpose: Hold all HTTP boundary code.
- Contains: Route registration, per-resource handlers, middleware, API tests.
- Key files: `internal/api/router.go`, `internal/api/middleware.go`, `internal/api/device_handler.go`

**`internal/domain/`:**
- Purpose: Define the shared backend model before implementation details.
- Contains: Entity structs, enums, repository interfaces, change-event types.
- Key files: `internal/domain/device.go`, `internal/domain/link.go`, `internal/domain/change_event.go`

**`internal/service/`:**
- Purpose: Implement business workflows that span repos or protocols.
- Contains: Device orchestration, backup workflows, static persistence logic.
- Key files: `internal/service/device_service.go`, `internal/service/backup_service.go`, `internal/service/instance_backup_service.go`

**`internal/repository/sqlite/`:**
- Purpose: Keep database code and schema evolution together.
- Contains: Repository implementations, SQL dialect helpers, migrations, PostgreSQL planning helpers.
- Key files: `internal/repository/sqlite/device_repo.go`, `internal/repository/sqlite/sql_store.go`, `internal/repository/sqlite/migrations.go`

**`internal/cache/`:**
- Purpose: Materialize device/link inventory for fast repeated reads.
- Contains: `DeviceLinkCache` and incremental change application.
- Key files: `internal/cache/cache.go`

**`internal/scheduler/`:**
- Purpose: Decide when each device poll task should run.
- Contains: Scheduler core, heap, jitter, task types.
- Key files: `internal/scheduler/scheduler.go`, `internal/scheduler/heap.go`, `internal/scheduler/types.go`

**`internal/collector/`:**
- Purpose: Fetch runtime telemetry from SNMP and Prometheus.
- Contains: Performance, operational, static, and Prometheus collectors.
- Key files: `internal/collector/performance.go`, `internal/collector/operational.go`, `internal/collector/prometheus.go`

**`internal/state/`:**
- Purpose: Store volatile runtime status independent of the DB-backed cache.
- Contains: In-memory device state, health rules, staleness tracking.
- Key files: `internal/state/store.go`, `internal/state/health.go`

**`internal/worker/`:**
- Purpose: Run long-lived background orchestration.
- Contains: Pipeline orchestrator, snapshot builder, backup schedulers, legacy poller.
- Key files: `internal/worker/pipeline.go`, `internal/worker/snapshot_builder.go`, `internal/worker/device_backup_scheduler.go`

**`internal/ws/`:**
- Purpose: Serve and distribute real-time messages.
- Contains: Hub, handler, message types, tests.
- Key files: `internal/ws/hub.go`, `internal/ws/handler.go`, `internal/ws/messages.go`

**`internal/vendor/`:**
- Purpose: Own vendor-specific device metadata and SNMP config.
- Contains: Embedded YAML data, registry loading, schema helpers.
- Key files: `internal/vendor/embedded.go`, `internal/vendor/registry.go`, `internal/vendor/data/*.yaml`

**`frontend/src/`:**
- Purpose: Hold the browser application.
- Contains: components, hooks, context, API clients, type parsers, utilities.
- Key files: `frontend/src/main.tsx`, `frontend/src/App.tsx`, `frontend/src/api/client.ts`

**`frontend/src/components/`:**
- Purpose: Keep UI feature code close to rendered components.
- Contains: top-level screens, panels, cards, canvas helpers, colocated tests.
- Key files: `frontend/src/components/Canvas.tsx`, `frontend/src/components/Dashboard.tsx`, `frontend/src/components/canvas/useCanvasData.ts`

## Key File Locations

**Entry Points:**
- `cmd/theia/main.go`: Main backend server bootstrap and dependency graph
- `frontend/src/main.tsx`: Frontend mount point
- `frontend/src/App.tsx`: Frontend root composition
- `cmd/winbox-bridge/main.go`: Separate bridge executable

**Configuration:**
- `internal/config/config.go`: YAML/env bootstrap config loader
- `config.yaml`: Local runtime bootstrap config
- `config.example.yaml`: Config example for setup
- `frontend/vite.config.ts`: Frontend dev/build configuration
- `frontend/tsconfig.json`: Frontend TypeScript base config

**Core Logic:**
- `internal/api/router.go`: API route map
- `internal/service/device_service.go`: Device orchestration
- `internal/cache/cache.go`: Inventory cache
- `internal/scheduler/scheduler.go`: Poll scheduling
- `internal/worker/pipeline.go`: Runtime orchestration
- `internal/worker/snapshot_builder.go`: Snapshot materialization
- `internal/ws/hub.go`: WebSocket broadcast fan-out
- `frontend/src/components/Canvas.tsx`: Topology UI shell
- `frontend/src/components/canvas/useCanvasData.ts`: Frontend topology composition logic

**Testing:**
- `internal/**/*_test.go`: Backend tests live beside implementation
- `frontend/src/**/*.test.tsx`: Frontend component tests live beside implementation
- `frontend/src/**/*.test.ts`: Frontend hook, type, and util tests live beside implementation
- `frontend/src/components/__tests__/`: Extra audit/smoke tests for UI rules

## Naming Conventions

**Files:**
- Go backend files use lowercase snake_case names: `internal/service/device_service.go`, `internal/api/device_handler.go`
- React component files use PascalCase: `frontend/src/components/Canvas.tsx`, `frontend/src/components/Dashboard.tsx`
- Hooks use `use*` camelCase filenames: `frontend/src/hooks/useWebSocket.ts`, `frontend/src/components/canvas/useCanvasData.ts`
- Utility/type files use lower camelCase or lowercase domain names: `frontend/src/utils/bridgeRequests.ts`, `frontend/src/types/metrics.ts`
- Tests stay next to source with `.test.*` or `_test.go`: `frontend/src/api/client.test.ts`, `internal/api/router_test.go` is not present; handler-specific tests use files like `internal/api/device_handler_test.go`

**Directories:**
- Backend package directories are lowercase and package-oriented: `internal/service`, `internal/state`, `internal/ws`
- Frontend feature directories are lowercase nouns or feature names: `frontend/src/components/canvas`, `frontend/src/components/dashboard`, `frontend/src/hooks`

## Where to Add New Code

**New Backend API feature:**
- Primary code: add domain contracts in `internal/domain/`, implementation in `internal/service/` and/or `internal/repository/sqlite/`, HTTP surface in `internal/api/`
- Wiring: register dependencies and routes from `cmd/theia/main.go` and `internal/api/router.go`
- Tests: place `*_test.go` beside each new backend file

**New Runtime polling or live-update feature:**
- Scheduler/task behavior: `internal/scheduler/`
- Poll collection logic: `internal/collector/`
- Runtime state shape: `internal/state/`
- Snapshot/WebSocket delivery: `internal/worker/` and `internal/ws/`

**New Frontend component/module:**
- Implementation: `frontend/src/components/` for UI, `frontend/src/components/canvas/` for topology-specific helpers, `frontend/src/components/dashboard/` for dashboard subviews
- Tests: colocate as `*.test.tsx` or `*.test.ts` next to the new file

**Utilities:**
- Shared backend helpers: the owning backend package under `internal/` rather than a global helpers folder
- Shared frontend helpers: `frontend/src/utils/`, typed browser parsers in `frontend/src/types/`, reusable hooks in `frontend/src/hooks/`

## Special Directories

**`internal/repository/sqlite/migrations/`:**
- Purpose: SQLite schema migrations
- Generated: No
- Committed: Yes

**`internal/repository/sqlite/postgres_migrations/`:**
- Purpose: PostgreSQL schema migrations for production/reference deployments
- Generated: No
- Committed: Yes

**`internal/vendor/data/`:**
- Purpose: Embedded vendor YAML definitions loaded by `internal/vendor/embedded.go`
- Generated: No
- Committed: Yes

**`internal/scalelab/testdata/`:**
- Purpose: Scale-lab fixtures and validation inputs
- Generated: No
- Committed: Yes

**`frontend/public/`:**
- Purpose: Static frontend assets served by Vite/build output
- Generated: No
- Committed: Yes

**`.planning/codebase/`:**
- Purpose: Generated codebase reference documents for GSD workflows
- Generated: Yes
- Committed: Yes

---

*Structure analysis: 2026-04-19*
