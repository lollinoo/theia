# Codebase Structure

**Analysis Date:** 2026-04-23

## Directory Layout

```
theia/
├── cmd/                    # Go binaries: backend, WinBox bridge, DB tools, scale lab
├── internal/               # Backend packages hidden from external import
├── frontend/               # Vite React/TypeScript frontend
├── vendors/                # Vendor/OID registry YAML embedded/loaded by backend
├── docker/                 # Docker support configs for Prometheus, SNMP sims, WISP lab
├── scripts/                # Validation, seeding, scale, and CI helper scripts
├── data/                   # Runtime local data directory
├── tmp/                    # Temporary/local lab artifacts
├── .github/                # GitHub workflows and repository automation
├── .githooks/              # Repo-managed Git hooks
├── .planning/              # Planning/codebase documentation
├── go.mod                  # Go module definition
├── frontend/package.json   # Frontend package/scripts/dependencies
├── Makefile                # Local development, test, release, and deployment targets
├── Dockerfile              # Backend image build
├── Dockerfile.frontend     # Frontend image build
├── docker-compose.yml      # Development/test compose stack
├── docker-compose.prod.yml # Production compose stack
├── docker-compose.staging.yml # Staging compose stack
└── config.yaml             # Local backend config file
```

## Directory Purposes

**`cmd/`:**
- Purpose: Application and utility binary entrypoints.
- Contains: `cmd/theia/`, `cmd/winbox-bridge/`, `cmd/theia-db-migrate/`, `cmd/theia-db-check/`, `cmd/theia-scale-lab/`.
- Key files: `cmd/theia/main.go`, `cmd/theia/runtime_bootstrap.go`, `cmd/winbox-bridge/main.go`, `cmd/winbox-bridge/server.go`, `cmd/theia-db-migrate/main.go`.

**`cmd/theia/`:**
- Purpose: Main backend executable and runtime composition root.
- Contains: Flag parsing, config selection, runtime path helpers, startup/shutdown, vendor registry bootstrap.
- Key files: `cmd/theia/main.go`, `cmd/theia/runtime_bootstrap.go`, `cmd/theia/runtime_paths.go`, `cmd/theia/runtime_fs.go`, `cmd/theia/vendor_registry_bootstrap.go`.

**`cmd/winbox-bridge/`:**
- Purpose: Local desktop companion bridge for WinBox launching.
- Contains: Systray app, local HTTP server, encrypted launch token handling, platform-specific launch files, icons/resources.
- Key files: `cmd/winbox-bridge/main.go`, `cmd/winbox-bridge/server.go`, `cmd/winbox-bridge/config.go`, `cmd/winbox-bridge/launch_windows.go`, `cmd/winbox-bridge/launch_other.go`.

**`internal/api/`:**
- Purpose: HTTP routing, resource handlers, middleware, and health/bridge/prometheus endpoints.
- Contains: One handler file per API resource plus `internal/api/router.go` and `internal/api/middleware.go`.
- Key files: `internal/api/router.go`, `internal/api/device_handler.go`, `internal/api/link_handler.go`, `internal/api/backup_handler.go`, `internal/api/instance_backup_handler.go`, `internal/api/bridge_handler.go`.

**`internal/domain/`:**
- Purpose: Backend domain entities, enums, normalization helpers, and repository interfaces.
- Contains: Core models for devices, links, positions, settings, areas, credentials, backups, vendors, metrics, and change events.
- Key files: `internal/domain/device.go`, `internal/domain/link.go`, `internal/domain/settings.go`, `internal/domain/metrics.go`, `internal/domain/credential_profile.go`, `internal/domain/change_event.go`.

**`internal/repository/sqlite/`:**
- Purpose: SQL persistence for SQLite and PostgreSQL despite package name.
- Contains: Repository implementations, SQL dialect wrapper, migrations, migration tests, SQLite-to-Postgres migrator.
- Key files: `internal/repository/sqlite/sql_store.go`, `internal/repository/sqlite/sql_dialect.go`, `internal/repository/sqlite/device_repo.go`, `internal/repository/sqlite/link_repo.go`, `internal/repository/sqlite/settings_repo.go`, `internal/repository/sqlite/migrations/000001_initial_schema.up.sql`, `internal/repository/sqlite/postgres_migrations/000001_initial_schema.up.sql`.

**`internal/service/`:**
- Purpose: Business workflows and orchestration above repositories.
- Contains: Device lifecycle/discovery, backup/restore, static persistence, external command helpers.
- Key files: `internal/service/device_service.go`, `internal/service/device_mutation_service.go`, `internal/service/device_discovery_coordinator.go`, `internal/service/backup_service.go`, `internal/service/instance_backup_service.go`, `internal/service/restore_coordinator.go`.

**`internal/collector/`:**
- Purpose: SNMP and Prometheus collection primitives for runtime polling.
- Contains: Performance, operational, static, Prometheus collectors and result/rate helpers.
- Key files: `internal/collector/performance.go`, `internal/collector/operational.go`, `internal/collector/static.go`, `internal/collector/prometheus.go`, `internal/collector/results.go`, `internal/collector/rates.go`.

**`internal/worker/`:**
- Purpose: Long-running orchestration workers for polling, snapshots, runtime state, and scheduled backups.
- Contains: Pipeline orchestrator, task runner, snapshot broadcaster, poller, metrics collector, backup schedulers.
- Key files: `internal/worker/pipeline.go`, `internal/worker/pipeline_task_runner.go`, `internal/worker/pipeline_snapshot_broadcaster.go`, `internal/worker/pipeline_runtime_state.go`, `internal/worker/backup_scheduler.go`, `internal/worker/device_backup_scheduler.go`.

**`internal/scheduler/`:**
- Purpose: Poll task scheduling and concurrency budgeting.
- Contains: Heap-backed scheduler, poll task definitions, interval calculation, completion handling.
- Key files: `internal/scheduler/scheduler.go`, `internal/scheduler/tasks.go`, `internal/scheduler/intervals.go`.

**`internal/state/`:**
- Purpose: In-memory volatile device state for metrics, reachability, freshness, and alert computation.
- Contains: Thread-safe state store, threshold/health evaluation, staleness ticking.
- Key files: `internal/state/store.go`, plus package tests in `internal/state/`.

**`internal/ws/`:**
- Purpose: WebSocket protocol, hub, client lifecycle, and message contracts.
- Contains: Hub/client code, upgrade handler, message constructors and DTOs.
- Key files: `internal/ws/hub.go`, `internal/ws/handler.go`, `internal/ws/messages.go`.

**`internal/cache/`:**
- Purpose: Cached DB-backed topology/device inventory for polling and snapshots.
- Contains: Device/link cache implementation and tests.
- Key files: `internal/cache/device_link_cache.go`.

**`internal/snmp/`:**
- Purpose: SNMP client wrapper, OID polling, discovery, and topology discovery policy.
- Contains: SNMP client setup, discovery result parsing, LLDP/CDP support, interface/performance polling.
- Key files: `internal/snmp/client.go`, `internal/snmp/discovery.go`, `internal/snmp/metrics.go`, `internal/snmp/topology.go`.

**`internal/ssh/`:**
- Purpose: SSH/SFTP access, known hosts handling, and backup transport support.
- Contains: Default SSH dialer, known hosts store, SFTP helpers.
- Key files: `internal/ssh/` Go files used by `internal/service/backup_service.go`.

**`internal/vendor/`:**
- Purpose: Vendor registry, OID resolution, and backup command support.
- Contains: Registry loading/normalization and vendor config parsing.
- Key files: `internal/vendor/` Go files used by `cmd/theia/vendor_registry_bootstrap.go` and collectors.

**`internal/metrics/` and `internal/observability/`:**
- Purpose: Prometheus client integration and internal runtime metrics export.
- Contains: Prometheus API client, observability registry/handler/counters.
- Key files: `internal/metrics/` Go files, `internal/observability/` Go files, `/metrics` registration in `cmd/theia/runtime_bootstrap.go`.

**`internal/config/`, `internal/crypto/`, `internal/pollingbudget/`, `internal/topology/`, `internal/scalelab/`, `internal/version/`:**
- Purpose: Focused backend support packages.
- Contains: YAML/env config in `internal/config/config.go`, encryption key handling in `internal/crypto/`, worker budget helpers in `internal/pollingbudget/`, topology observation abstractions in `internal/topology/`, synthetic lab tooling in `internal/scalelab/`, build version variables in `internal/version/`.
- Key files: `internal/config/config.go`, `internal/crypto/`, `internal/pollingbudget/`, `internal/topology/`, `internal/scalelab/`, `internal/version/version.go`.

**`frontend/src/`:**
- Purpose: React application source.
- Contains: Root app, components, hooks, API client, contexts, type parsers, utilities, test setup.
- Key files: `frontend/src/main.tsx`, `frontend/src/App.tsx`, `frontend/src/api/client.ts`, `frontend/src/hooks/useWebSocket.ts`, `frontend/src/components/Canvas.tsx`, `frontend/src/types/api.ts`, `frontend/src/types/metrics.ts`.

**`frontend/src/components/`:**
- Purpose: UI components and topology canvas implementation.
- Contains: Top-level panels/cards, dashboard, area hub, settings, alerts, React Flow nodes/edges, canvas-specific helper subdirectory.
- Key files: `frontend/src/components/Canvas.tsx`, `frontend/src/components/Dashboard.tsx`, `frontend/src/components/AreaHub.tsx`, `frontend/src/components/DeviceCard.tsx`, `frontend/src/components/LinkEdge.tsx`, `frontend/src/components/canvas/topologyComposer.ts`.

**`frontend/src/components/canvas/`:**
- Purpose: Pure canvas transforms, hooks, panel adapters, runtime adapters, and detail subscription helpers.
- Contains: Node/edge builders, topology composition, area filtering, canvas data/menu hooks, overlay/panel components.
- Key files: `frontend/src/components/canvas/useCanvasData.ts`, `frontend/src/components/canvas/topologyComposer.ts`, `frontend/src/components/canvas/nodeBuilder.ts`, `frontend/src/components/canvas/edgeBuilder.ts`, `frontend/src/components/canvas/runtimeAdapters.ts`, `frontend/src/components/canvas/detailSubscription.ts`.

**`frontend/src/hooks/`:**
- Purpose: Reusable React hooks for WebSocket, positions, layout, keyboard shortcuts, WinBox, bridge health, and freshness.
- Contains: Hook implementations and tests.
- Key files: `frontend/src/hooks/useWebSocket.ts`, `frontend/src/hooks/usePositions.ts`, `frontend/src/hooks/useAutoLayout.ts`, `frontend/src/hooks/useWinboxFlow.ts`, `frontend/src/hooks/useBridgeHealth.ts`.

**`frontend/src/api/` and `frontend/src/types/`:**
- Purpose: Frontend API boundary and payload contracts.
- Contains: Fetch functions, typed errors, REST parsers, WebSocket message parsers.
- Key files: `frontend/src/api/client.ts`, `frontend/src/api/errors.ts`, `frontend/src/types/api.ts`, `frontend/src/types/metrics.ts`.

**`docker/`:**
- Purpose: Supporting container configs for local/dev/prod observability and network labs.
- Contains: Prometheus configs in `docker/prometheus/`, SNMP simulator configs in `docker/snmp/`, WISP lab topology in `docker/wisp-lab/`.
- Key files: `docker/prometheus/prometheus.yml`, `docker/prometheus/prometheus.prod.yml`, `docker/prometheus/snmp.yml`, `docker/snmp/Dockerfile.snmpd`, `docker/wisp-lab/topology.json`.

**`scripts/`:**
- Purpose: Developer/CI helper scripts for validation, seeding, and local checks.
- Contains: Phase validation, coverage gates, seed scripts, WISP checks, collector contract runner.
- Key files: `scripts/check-go-cover.sh`, `scripts/run-collector-contract.sh`, `scripts/phase4-validate.sh`, `scripts/seed.sh`, `scripts/validate-commit-msg.sh`.

## Key File Locations

**Entry Points:**
- `cmd/theia/main.go`: Main backend binary flag/env config path selection.
- `cmd/theia/runtime_bootstrap.go`: Main backend dependency composition and lifecycle.
- `cmd/winbox-bridge/main.go`: Local WinBox bridge application.
- `cmd/theia-db-migrate/main.go`: SQLite-to-PostgreSQL migration utility.
- `cmd/theia-db-check/main.go`: Database check utility.
- `cmd/theia-scale-lab/main.go`: Scale lab utility.
- `frontend/src/main.tsx`: React browser entrypoint.
- `frontend/src/App.tsx`: Frontend root view composition.

**Configuration:**
- `internal/config/config.go`: Backend YAML/env configuration loader.
- `config.yaml`: Local backend configuration.
- `config.example.yaml`: Example backend configuration.
- `go.mod`: Go version and backend dependencies.
- `frontend/package.json`: Frontend scripts and dependencies.
- `frontend/vite.config.ts`: Vite plugins and `/api` proxy.
- `frontend/tsconfig.app.json`: Frontend TypeScript app config.
- `frontend/biome.json`: Frontend lint/format config.
- `docker-compose.yml`: Development/test services.
- `docker-compose.prod.yml`: Production service layout.
- `docker-compose.staging.yml`: Staging service layout.
- `.air.toml`: Go hot-reload configuration.

**Core Logic:**
- `internal/domain/device.go`: Device model, SNMP credentials, topology modes, and device repository contract.
- `internal/api/router.go`: API route registration and middleware chain.
- `internal/service/device_service.go`: Device orchestration facade.
- `internal/service/device_mutation_service.go`: Device create/update/delete workflow details.
- `internal/service/device_discovery_coordinator.go`: SNMP discovery persistence workflow.
- `internal/repository/sqlite/device_repo.go`: Device SQL persistence and change publication.
- `internal/repository/sqlite/sql_store.go`: SQL placeholder rebinding wrapper for PostgreSQL compatibility.
- `internal/scheduler/scheduler.go`: Poll task scheduler.
- `internal/worker/pipeline.go`: Polling runtime orchestrator.
- `internal/state/store.go`: Volatile metrics/reachability/freshness state.
- `internal/ws/hub.go`: WebSocket broadcast hub and backpressure behavior.
- `frontend/src/api/client.ts`: REST API functions.
- `frontend/src/hooks/useWebSocket.ts`: Realtime frontend state hook.
- `frontend/src/components/Canvas.tsx`: Main topology canvas container.
- `frontend/src/components/canvas/topologyComposer.ts`: Canvas node/edge composition.

**Testing:**
- `cmd/theia/*_test.go`: Backend runtime/bootstrap tests.
- `internal/api/*_test.go`: API handler and middleware tests.
- `internal/repository/sqlite/*_test.go`: Repository and migration tests.
- `internal/service/*_test.go`: Service behavior tests.
- `internal/worker/*_test.go`: Pipeline, scheduler-adjacent worker, snapshot, backup scheduler tests.
- `internal/ws/*_test.go`: WebSocket hub/message/handler tests.
- `frontend/src/**/*.test.ts`: Frontend unit tests for pure helpers/hooks.
- `frontend/src/**/*.test.tsx`: Frontend component tests.
- `frontend/e2e/`: Playwright end-to-end tests if present in the frontend tree.

## Naming Conventions

**Files:**
- Go packages use snake_case file names: `internal/service/device_service.go`, `internal/worker/pipeline_task_runner.go`.
- Go tests colocate with implementation using `_test.go`: `internal/api/device_handler_test.go`, `cmd/theia/runtime_bootstrap_test.go`.
- Backend API handlers use `{resource}_handler.go`: `internal/api/settings_handler.go`, `internal/api/snmp_profile_handler.go`.
- Backend repositories use `{resource}_repo.go`: `internal/repository/sqlite/device_repo.go`, `internal/repository/sqlite/area_repo.go`.
- SQL migrations use zero-padded version plus direction: `internal/repository/sqlite/migrations/000022_device_os_version.up.sql`.
- React components use PascalCase `.tsx`: `frontend/src/components/DeviceCard.tsx`, `frontend/src/components/SettingsPanel.tsx`.
- Frontend hooks use `use*.ts`: `frontend/src/hooks/useWebSocket.ts`, `frontend/src/components/canvas/useCanvasData.ts`.
- Frontend pure helpers use camelCase `.ts`: `frontend/src/components/canvas/topologyComposer.ts`, `frontend/src/components/linkSemantics.ts`.
- Frontend tests colocate with subject using `.test.ts` or `.test.tsx`: `frontend/src/components/Canvas.test.tsx`, `frontend/src/hooks/useWebSocket.test.ts`.

**Directories:**
- Backend packages are singular capability names under `internal/`: `internal/api/`, `internal/service/`, `internal/state/`, `internal/ws/`.
- Backend binaries live under `cmd/{binary-name}/`: `cmd/theia/`, `cmd/winbox-bridge/`.
- Frontend shared UI lives under `frontend/src/components/`; component-specific pure canvas code lives under `frontend/src/components/canvas/`.
- Frontend reusable hooks live under `frontend/src/hooks/`; API boundary lives under `frontend/src/api/`; shared TS contracts live under `frontend/src/types/`.
- Docker subdirectories group by service/domain: `docker/prometheus/`, `docker/snmp/`, `docker/wisp-lab/`.

## Where to Add New Code

**New Backend API Feature:**
- Domain types/interfaces: add to `internal/domain/{feature}.go` or extend existing `internal/domain/*.go`.
- Persistence: add repo implementation in `internal/repository/sqlite/{feature}_repo.go` and migration in `internal/repository/sqlite/migrations/`; add PostgreSQL migration in `internal/repository/sqlite/postgres_migrations/` when schema changes affect PostgreSQL.
- Business workflow: add service code in `internal/service/{feature}_service.go` when logic spans repositories or external systems.
- HTTP handler: add `internal/api/{feature}_handler.go` and register routes in `internal/api/router.go`.
- Runtime wiring: construct repos/services in `cmd/theia/runtime_bootstrap.go` and pass dependencies into `api.NewRouter(...)`.
- Tests: colocate tests as `internal/api/{feature}_handler_test.go`, `internal/service/{feature}_service_test.go`, and `internal/repository/sqlite/{feature}_repo_test.go`.

**New Polling / Runtime Capability:**
- Task semantics or cadence: `internal/scheduler/` and `internal/pollingbudget/`.
- Data collection: `internal/collector/{capability}.go` or `internal/snmp/` for raw SNMP logic.
- Volatile runtime state: `internal/state/store.go` or focused files in `internal/state/`.
- Pipeline orchestration: `internal/worker/pipeline_task_runner.go`, `internal/worker/pipeline_runtime_state.go`, or `internal/worker/pipeline_snapshot_broadcaster.go`.
- WebSocket contract: backend `internal/ws/messages.go`, frontend `frontend/src/types/metrics.ts`, and frontend merge handling in `frontend/src/hooks/useWebSocket.ts`.

**New Frontend View or Component:**
- Top-level view selection: update `frontend/src/App.tsx` and navigation in `frontend/src/components/NavigationPill.tsx`.
- Reusable component: add `frontend/src/components/{ComponentName}.tsx` with colocated `frontend/src/components/{ComponentName}.test.tsx`.
- Canvas-specific helper: add under `frontend/src/components/canvas/` and keep pure transforms separate from `frontend/src/components/Canvas.tsx`.
- API call: add fetch/mutation function in `frontend/src/api/client.ts` and parser/type in `frontend/src/types/api.ts`.
- Hook: add `frontend/src/hooks/use{Feature}.ts` with colocated tests.

**New Component/Module:**
- Backend module: create a focused package under `internal/{capability}/` only when it is not an API handler, domain type, repository, service, collector, or worker concern.
- Frontend module: create under `frontend/src/components/`, `frontend/src/hooks/`, `frontend/src/api/`, `frontend/src/types/`, or `frontend/src/utils/` based on responsibility.

**Utilities:**
- Backend shared helper with domain meaning: `internal/domain/` or the consuming package.
- Backend process/runtime helper: `cmd/theia/` if only bootstrap uses it; `internal/{capability}/` if reusable.
- Frontend pure utility: `frontend/src/utils/` for cross-component helpers, or colocate near one component in `frontend/src/components/canvas/`.
- Scripts/automation: `scripts/` and expose via `Makefile` when used by developers or CI.

## Special Directories

**`internal/repository/sqlite/migrations/`:**
- Purpose: SQLite schema migrations for primary application data.
- Generated: No
- Committed: Yes

**`internal/repository/sqlite/postgres_migrations/`:**
- Purpose: PostgreSQL schema migrations for production/reference DB path.
- Generated: No
- Committed: Yes

**`vendors/`:**
- Purpose: Vendor registry YAML/OID definitions consumed by bootstrap and collectors.
- Generated: No
- Committed: Yes

**`data/`:**
- Purpose: Local runtime data path for database, backups, known hosts, and app data as configured by `internal/config/config.go` and `cmd/theia/runtime_paths.go`.
- Generated: Yes
- Committed: Directory may exist; runtime contents should be treated as local data.

**`tmp/`:**
- Purpose: Temporary/lab artifacts including local Theia data under `tmp/theia`.
- Generated: Yes
- Committed: Local/generated contents should not be used as source-of-truth code.

**`docker/prometheus/`:**
- Purpose: Prometheus and SNMP exporter configs for dev/prod/WISP lab observability.
- Generated: No
- Committed: Yes

**`docker/snmp/`:**
- Purpose: SNMP simulator Dockerfiles and sample device configs.
- Generated: No
- Committed: Yes

**`docker/wisp-lab/`:**
- Purpose: Synthetic WISP/router lab topology and renderer.
- Generated: Mixed; source topology/scripts committed, container runtime artifacts generated.
- Committed: Yes for source files such as `docker/wisp-lab/topology.json` and `docker/wisp-lab/render_router_lab.py`.

**`frontend/src/components/__tests__/`:**
- Purpose: Frontend audit/smoke tests not tied one-to-one to a component file.
- Generated: No
- Committed: Yes

**`.planning/codebase/`:**
- Purpose: Codebase maps consumed by GSD planning/execution commands.
- Generated: Yes
- Committed: Planning docs may be committed when workflow requires them.

**`.githooks/`:**
- Purpose: Repository-managed hooks configured by `make install-hooks`.
- Generated: No
- Committed: Yes

---

*Structure analysis: 2026-04-23*
