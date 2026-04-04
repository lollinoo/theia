<!-- GSD:project-start source:PROJECT.md -->
## Project

**MikroTik Theia**

A modern, interactive network topology visualizer that provides a bird's-eye view of network infrastructure. Built as a web application (React + Go) that integrates with an existing Prometheus/Grafana monitoring stack to display real-time statistics for routers, switches, and their interconnections on a drag-and-drop canvas. Think of it as a modern replacement for MikroTik's The Dude — starting with network devices, with a vision to expand to full infrastructure mapping.

**Core Value:** Network operators can see their entire topology at a glance with live stats on every device and link, and drill into Grafana for deep dives — all from a single interactive map.

### Constraints

- **Data source**: Must integrate with existing Prometheus instance — no duplicate metric collection
- **Scale**: Must handle 100+ devices without performance degradation on the canvas
- **Tech stack**: React frontend, Go backend — chosen for performance and ecosystem fit
- **SNMP compatibility**: Must work with any device exposing standard SNMP MIBs
- **Real-time**: Configurable polling intervals, not just static snapshots
<!-- GSD:project-end -->

<!-- GSD:stack-start source:codebase/STACK.md -->
## Technology Stack

## Languages
- Go 1.24 - Backend API server, SNMP polling, SSH/SFTP, all business logic
- TypeScript 5.7 - Frontend React application (strict mode, ESNext target)
- SQL - SQLite schema migrations (`internal/repository/sqlite/migrations/`)
- YAML - Vendor profiles (`internal/vendor/data/*.yaml`), Prometheus config, app config
- CSS - Tailwind utility classes via `frontend/src/index.css`
## Runtime
- Go runtime 1.24 (backend) - requires CGO enabled for `mattn/go-sqlite3`
- Node.js 22 (frontend build) - Alpine-based Docker image
- Go: built-in Go modules (`go.mod` / `go.sum`)
- Frontend: npm with lockfile (`frontend/package-lock.json` - present)
## Frameworks
- React 18.3 - Frontend SPA (`frontend/src/`)
- `net/http` (stdlib) - Backend HTTP server with no external web framework; router hand-built in `internal/api/router.go`
- Tailwind CSS 3.4 - Utility-first styling, custom design tokens defined in `frontend/tailwind.config.js`
- ReactFlow 11.11 - Network topology canvas (`frontend/src/components/`)
- d3-force 3.0 - Force-directed graph layout (`frontend/src/`)
- Vitest 4.1 - Frontend test runner (`frontend/vitest.config.ts`)
- `@testing-library/react` 16.3 - Frontend component tests
- Go standard `testing` package - Backend unit and integration tests
- Vite 7.0 - Frontend bundler + dev server with HMR (`frontend/vite.config.ts`)
- Air v1.61.5 - Go hot-reload for development (installed in dev Docker image)
- PostCSS 8 / autoprefixer - CSS pipeline (`frontend/postcss.config.js`)
- nginx 1.27 (Alpine) - Serves compiled React SPA and proxies `/api/` + `/api/v1/ws` to backend (`frontend/nginx.conf.template`)
## Key Dependencies
- `github.com/mattn/go-sqlite3` v1.14.22 - SQLite driver; requires CGO (glibc, not Alpine)
- `github.com/gosnmp/gosnmp` v1.43.2 - SNMP v2c/v3 client for device discovery and metrics polling
- `github.com/gorilla/websocket` v1.5.3 - WebSocket hub for live metrics push to frontend (`internal/ws/`)
- `github.com/pkg/sftp` v1.13.10 - SFTP file download for device config backups
- `golang.org/x/crypto` v0.45.0 - SSH client (`golang.org/x/crypto/ssh`) and AES-256-GCM encryption (`internal/crypto/`)
- `github.com/golang-migrate/migrate/v4` v4.19.1 - SQL migration management (`internal/repository/sqlite/migrations.go`)
- `github.com/google/uuid` v1.6.0 - UUID generation for entity IDs
- `gopkg.in/yaml.v3` v3.0.1 - Parsing `config.yaml` and vendor profile YAMLs
- `reactflow` 11.11 - Network graph canvas (core frontend visualization)
## Configuration
- File: `config.yaml` (or path from `--config` flag / `THEIA_CONFIG` env var)
- Fields: `listen_addr`, `db_path`, `log_level`
- Env var overrides: `THEIA_LISTEN_ADDR`, `THEIA_DB_PATH`, `THEIA_LOG_LEVEL`
- Parsed in `internal/config/config.go`
- Settings table in SQLite, managed via `/api/v1/settings`
- Keys: `prometheus_url`, `grafana_url`, `polling_interval_seconds`, `snmp_worker_pool_size`, `snmp_timeout_seconds`, `snmp_retries`, `timezone`
- Defined in `internal/domain/settings.go`
- `THEIA_ENCRYPTION_KEY` - Required env var; used to derive AES-256 key via SHA-256 (`internal/crypto/encrypt.go`)
- Credentials at rest (SNMP community strings, SSH passwords/keys) encrypted in SQLite using AES-256-GCM
- `Dockerfile` - Multi-stage: `dev` (Air hot-reload), `builder` (CGO binary), `production` (debian:bookworm-slim)
- `Dockerfile.frontend` - Multi-stage: `dev` (Vite), `builder` (npm build), `production` (nginx)
- `docker-compose.yml` - Dev stack with SNMP simulators and SSH mock
- `docker-compose.prod.yml` - Production stack; optional `metrics` profile adds Prometheus + SNMP exporter
- Build args embedded in binary: `VERSION`, `GIT_COMMIT`, `BUILD_DATE` via `-ldflags` (`internal/version/`)
## Platform Requirements
- Docker + Docker Compose (all dev tooling containerized)
- CGO requires glibc (Debian-based images, not Alpine)
- Ports: backend `:8080`, frontend `:3000`, Prometheus `:9090`, SNMP exporter `:9116`
- Container runtime (Docker Compose or equivalent)
- `THEIA_ENCRYPTION_KEY` env var must be set before first start
- Persistent volume for SQLite DB (default `/data/theia.db`)
- Optional: Prometheus + SNMP exporter for live metrics (enable with `--profile metrics`)
<!-- GSD:stack-end -->

<!-- GSD:conventions-start source:CONVENTIONS.md -->
## Conventions

## Naming Patterns
- Go: `snake_case.go`, test files co-located as `snake_case_test.go` (e.g., `device_handler.go` / `device_handler_test.go`)
- TypeScript/TSX: `PascalCase.tsx` for components (e.g., `DeviceCard.tsx`), `camelCase.ts` for hooks/utilities (e.g., `useWebSocket.ts`, `client.ts`)
- Test files: same name as source with `.test.ts` / `.test.tsx` suffix, co-located (e.g., `DeviceCard.test.tsx` next to `DeviceCard.tsx`)
- Exported: `PascalCase` — e.g., `NewDeviceHandler`, `HandleCreate`, `WriteError`
- Unexported: `camelCase` — e.g., `extractIDFromPath`, `parseSNMPCreds`, `decodeJSON`
- Constructor functions prefixed with `New` — e.g., `NewDeviceService`, `NewDeviceHandler`
- Handler methods prefixed with `Handle` — e.g., `HandleCreate`, `HandleList`, `HandleDelete`
- All: `camelCase` — e.g., `fetchDevices`, `createDevice`, `parseDevicesResponse`
- React components: `PascalCase` — e.g., `DeviceCard`, `DeviceCardInner`
- Custom hooks: `use` prefix — e.g., `useWebSocket`, `usePositions`, `useAutoLayout`
- Internal component helpers: plain `camelCase` — e.g., `displayName`, `secondaryText`, `vendorLabel`
- Go: `camelCase` for local vars, `PascalCase` for exported struct fields
- TypeScript: `camelCase` throughout; screaming snake case avoided
- React component state: `camelCase` nouns — e.g., `connected`, `snapshot`, `prometheusStatus`
- Go: `PascalCase` structs/interfaces — e.g., `DeviceHandler`, `DeviceUpdate`, `DiscoverFunc`
- Go domain types: typed strings for enumerations — e.g., `type DeviceStatus string`, `type MetricsSource string`
- TypeScript interfaces: `PascalCase` — e.g., `Device`, `DeviceNodeData`, `CreateDevicePayload`
- TypeScript union types: lowercase literals — e.g., `type DeviceStatus = 'up' | 'down' | 'probing' | 'unknown'`
- Named with type prefix — e.g., `DeviceTypeRouter`, `DeviceStatusUp`, `SNMPVersionV2c`, `MetricsSourcePrometheus`
## Code Style
- Standard `gofmt` formatting (implied by Go conventions; no custom config detected)
- Import grouping: stdlib first, then third-party, then internal — enforced by blank lines between groups
- Example from `device_handler.go`:
- No `.prettierrc` or `eslint.config.*` detected; Vite + TypeScript compiler enforce basic syntax
- Tailwind CSS used for all styling — no separate CSS files for component styles
- Single quotes for string literals; trailing commas in multi-line objects/arrays
- No ESLint config file detected in `frontend/`
- TypeScript strict mode implied by `tsconfig.app.json`; code uses explicit type guards throughout
## Import Organization (TypeScript)
- No `@/` or other aliases configured; all imports use relative paths (e.g., `'../types/api'`, `'./DeviceCard'`)
## Error Handling
- Functions return `(value, error)` — callers always check `err != nil` immediately
- HTTP handlers use the `writeError(w, statusCode, message)` helper for all error responses, which returns a `{"error": "..."}` JSON body
- Error messages include context — e.g., `fmt.Errorf("device not found: %s", id)`
- `strings.Contains(err.Error(), "not found")` pattern used in handlers to differentiate 404 vs 500
- Custom error types for specific validation errors — e.g., `invalidFieldError` struct with `Error()` method
- `errors.As` used for detecting typed errors (e.g., `http.MaxBytesError`) — see `decodeJSON` in `internal/api/device_handler.go`
- Async functions use try/catch; errors re-thrown with context: `throw new Error(\`Failed to fetch devices: ${message}\`)`
- Error message extraction pattern: `const message = error instanceof Error ? error.message : 'unknown error'`
- Network responses validated with type guards (`isRecord`, `readString`, etc.) before use — see `frontend/src/types/api.ts`
- API client returns safe defaults (empty arrays, fallback strings) for malformed responses rather than throwing
## Logging
- Uses standard library `log` package only — `log.Printf("%s %s %d %s", ...)` in middleware
- Location: `internal/api/middleware.go` RequestLogger
- No structured logging framework (no `slog`, `zap`, or `logrus`)
- No logging framework; no `console.log` calls in production component code detected
## Comments
- All exported types, functions, and methods have doc comments (GoDoc style)
- Comment directly above the declaration: `// DeviceHandler provides HTTP handlers for device CRUD operations.`
- Inline section separators used to group related code blocks: `// --- Request types ---`, `// --- Helpers ---`
- Non-obvious logic explained inline: `// double pointer: nil=not set, *nil=unassign, **=set`
- TSDoc-style comments not consistently used; inline comments explain non-obvious logic
- JSX sections use comment labels: `{/* HEADER SECTION */}`, `{/* BODY SECTION */}`
- Mock purpose stated in test files: `// Mock sub-components that have their own complex dependencies`
## Function Design
- HTTP handlers are single-responsibility: parse request → validate → call service → write response
- Service methods are concise; heavy logic delegated to SNMP/domain packages
- Helper functions extracted for reuse: `writeError`, `decodeJSON`, `extractIDFromPath`, `parseSNMPCreds`
- Component props defined as explicit interfaces — e.g., `interface DeviceNodeData { ... }`
- Optional props use `?` — e.g., `highlighted?: boolean`, `editMode?: boolean`
- Factory/override pattern for test data: `function mockDevice(overrides: Partial<Device> = {}): Device`
- Go: `(result, error)` pairs; never return error and non-nil result simultaneously
- TypeScript: explicit return types on exported functions; async functions always `Promise<T>`
## Module Design (TypeScript)
- Named exports for all components, hooks, and utility functions — e.g., `export function useWebSocket`, `export interface DeviceNodeData`
- Default export only for primary React components — e.g., `export default DeviceCard`
- Type-only imports use `import type` syntax — e.g., `import type { Device } from '../types/api'`
- Not used; no `index.ts` re-export files found; all imports reference specific module paths
## Module Design (Go)
- Each package has a single focused responsibility: `api`, `service`, `domain`, `snmp`, `cache`, `crypto`, `vendor`
- Domain interfaces defined in `internal/domain/` package — e.g., `DeviceRepository`, `LinkRepository`
- Concrete implementations in `internal/repository/sqlite/` and `internal/service/`
- Services receive dependencies via constructor functions: `NewDeviceService(deviceRepo, linkRepo, settingsRepo, discoverFunc)`
- Function types used for injectable behavior: `type DiscoverFunc func(...) (...)` in `internal/service/device_service.go`
<!-- GSD:conventions-end -->

<!-- GSD:architecture-start source:ARCHITECTURE.md -->
## Architecture

## Pattern Overview
- `domain` package defines interfaces and types (no dependencies on other internal packages)
- `repository/sqlite` implements domain interfaces (persistence layer)
- `service` orchestrates business logic using domain interfaces
- `api` handles HTTP transport and delegates to services/repos
- `worker` runs background goroutines (polling, metrics collection)
- Domain interfaces (`DeviceRepository`, `LinkRepository`, etc.) decouple business logic from persistence
- Constructor-injection used throughout; all dependencies wired in `cmd/theia/main.go`
- No external HTTP framework; uses standard `net/http` with a hand-rolled mux
- Frontend is a separate Vite/React SPA served independently; communicates via REST + WebSocket
## Layers
- Purpose: Core types, enums, and repository interfaces — the shared language of the system
- Location: `internal/domain/`
- Contains: `Device`, `Link`, `BackupJob`, `SNMPProfile`, `SSHProfile`, `DeviceMetrics`, `LinkMetrics`, `AlertState`, setting keys, and all repository interfaces
- Depends on: `github.com/google/uuid`, standard library only
- Used by: Every other internal package
- Purpose: SQLite-backed implementations of all domain repository interfaces
- Location: `internal/repository/sqlite/`
- Contains: `DeviceRepo`, `LinkRepo`, `PositionRepo`, `SettingsRepo`, `SNMPProfileRepo`, `SSHProfileRepo`, `BackupJobRepo`, `BackupFileRepo`, `VendorConfigRepo`
- Depends on: `domain`, `crypto`, `database/sql`, `mattn/go-sqlite3`
- Used by: `main.go` (wires repos into services/API)
- Note: `device_repo.go` and `snmp_profile_repo.go` encrypt sensitive fields at rest using AES-GCM via `internal/crypto`
- Purpose: Business logic orchestration; does not know about HTTP
- Location: `internal/service/`
- Contains: `DeviceService` (device CRUD + async SNMP probe), `BackupService` (SSH-based config backup)
- Depends on: `domain`, `snmp`, `ssh`, `vendor`, `crypto`
- Used by: `api` (handlers), `worker` (poller)
- Purpose: HTTP transport layer — request parsing, response formatting, middleware
- Location: `internal/api/`
- Contains: One handler per resource (`device_handler.go`, `link_handler.go`, etc.), `router.go`, `middleware.go`
- Depends on: `service`, `domain`, `vendor`, `worker`, `ws`
- Used by: `main.go` via `api.NewRouter()`
- Response format: JSON; device responses use a JSON:API-like envelope (`{"data": ...}`)
- Purpose: Background goroutines for periodic SNMP probing and metrics collection
- Location: `internal/worker/`
- Contains: `Poller` (re-probes all managed devices on a configurable interval), `MetricsCollector` (queries Prometheus + SNMP fallback, builds snapshots, broadcasts via WebSocket)
- Depends on: `service`, `domain`, `metrics`, `vendor`, `ws`, `cache`
- Used by: `main.go`
- Purpose: Lazy-loaded, invalidation-driven in-memory cache for devices and links
- Location: `internal/cache/cache.go`
- Contains: `DeviceLinkCache` — shared between `Poller` and `MetricsCollector` to avoid duplicate DB reads per polling cycle
- Invalidation: `DeviceRepo` and `LinkRepo` signal a `chan struct{}` after writes; cache drains the channel on next read
- Purpose: Real-time push of metric snapshots to all connected browser clients
- Location: `internal/ws/`
- Contains: `Hub` (manages clients, serializes broadcasts), `Handler` (upgrades HTTP → WS, sends initial snapshot on connect), `messages.go` (DTOs and snapshot serialization)
- Pattern: Server-push only; clients do not send data (read pump only reads to detect disconnects)
- Purpose: SNMP client abstraction and device discovery
- Location: `internal/snmp/`
- Contains: `client.go` (wraps `gosnmp`), `discovery.go` (walks OIDs, collects interfaces via LLDP/CDP), `detector.go` (matches sysObjectID/sysDescr to vendor registry)
- Purpose: SSH client for config backups; known-hosts management
- Location: `internal/ssh/`
- Contains: `client.go`, `known_hosts.go`, `reachable.go`
- Purpose: Vendor configuration registry — defines per-vendor SNMP OIDs, PromQL templates, detection patterns, backup commands
- Location: `internal/vendor/`
- Contains: `Registry` (thread-safe in-memory store), `schema.go` (types), `embedded.go` (embeds `data/*.yaml` into binary)
- Built-in definitions: `internal/vendor/data/default.yaml`, `internal/vendor/data/mikrotik.yaml`
- Overridable: Set `THEIA_VENDORS_DIR` to load from disk; DB records take precedence at runtime
- Purpose: AES-GCM encryption for sensitive fields (SNMP credentials, SSH passwords) stored in SQLite
- Location: `internal/crypto/`
- Key source: `THEIA_ENCRYPTION_KEY` environment variable
- Purpose: Prometheus HTTP API client
- Location: `internal/metrics/`
- Contains: `PromClient` — queries device metrics, link metrics, interface info, alerts, probe status, and hostnames from a Prometheus endpoint
- Purpose: React SPA — network topology canvas + tabular dashboard
- Location: `frontend/src/`
- Contains: React components, `api/client.ts` (REST calls via `fetch`), `hooks/useWebSocket.ts` (WS with exponential backoff reconnect), `types/api.ts` + `types/metrics.ts` (TypeScript types)
## Data Flow
## Key Abstractions
- Purpose: Decouple service/worker from SQLite implementation; enable test mocks
- Examples: `domain.DeviceRepository`, `domain.LinkRepository`, `domain.SettingsRepository`, `domain.SNMPProfileRepository`, `domain.BackupJobRepository`
- Pattern: Interface defined in `domain`, implemented in `internal/repository/sqlite/`, injected at construction time
- Purpose: Function type for SNMP discovery; allows test injection without real network
- Pattern: `type DiscoverFunc func(target string, creds domain.SNMPCredentials) (*snmp.DiscoveryResult, error)`
- Wired in `main.go` as `newSNMPDiscoverFunc(settingsRepo, vendorRegistry)`
- Purpose: Function type for per-device SNMP metrics polling; enables testing without network
- Pattern: `type SNMPPollFunc func(target string, creds domain.SNMPCredentials, vendorName string) (domain.DeviceMetrics, error)`
- Wired in `main.go` as `newSNMPMetricsPollFunc(settingsRepo, vendorRegistry)`
- Purpose: Central store for all vendor definitions; resolves vendor-specific SNMP OIDs, PromQL templates, detection rules
- Pattern: `registry.ResolveSNMPConfig(vendorName)` and `registry.ResolvePrometheusMetrics(vendorName)` return vendor-specific config or fall back to `default`
- Seeded from embedded YAML (`internal/vendor/data/`), then synced to SQLite `vendor_configs` table; DB records take priority at runtime
- Purpose: Shared read-through cache invalidated by write signals from repos
- Pattern: Single invalidation channel (`chan struct{}`) passed to both `DeviceRepo` and `LinkRepo`; cache `drainInvalidations()` checks the channel before returning data
## Entry Points
- Location: `cmd/theia/main.go`
- Triggers: Process start, `--config` flag or `THEIA_CONFIG` env var
- Responsibilities: Load config, open DB, run migrations, wire all dependencies (repos → services → workers → API), start background goroutines, serve HTTP
- Location: `internal/api/router.go`
- Triggers: All HTTP requests
- Responsibilities: Route dispatch, middleware application (CORS → RequestLogger → MaxBodySize → JSONContentType), WebSocket bypass
- Location: `internal/ws/handler.go`, registered at `/api/v1/ws`
- Triggers: WS upgrade request from browser
- Responsibilities: Upgrade connection, send cached snapshot immediately, register client with hub, start read/write pumps
- Location: `frontend/src/main.tsx`
- Triggers: Browser load
- Responsibilities: Mount `<App>` into `#root`
- Location: `frontend/src/App.tsx`
- Responsibilities: Toggle between `Canvas` (ReactFlow topology view) and `Dashboard` (tabular view) via `NavBar`
## Error Handling
- Services return `fmt.Errorf("context: %w", err)` for wrapping
- API handlers call `writeError(w, statusCode, message)` (defined in `internal/api/`) to write `{"error": "..."}` JSON
- Background workers (`Poller`, `MetricsCollector`) log errors and continue (non-fatal)
- SNMP probe failures set `device.Status = DeviceStatusDown` rather than aborting
- WebSocket: client read errors trigger unregister (hub removes client silently)
## Cross-Cutting Concerns
<!-- GSD:architecture-end -->

<!-- GSD:workflow-start source:GSD defaults -->
## GSD Workflow Enforcement

Before using Edit, Write, or other file-changing tools, start work through a GSD command so planning artifacts and execution context stay in sync.

Use these entry points:
- `/gsd:quick` for small fixes, doc updates, and ad-hoc tasks
- `/gsd:debug` for investigation and bug fixing
- `/gsd:execute-phase` for planned phase work

Do not make direct repo edits outside a GSD workflow unless the user explicitly asks to bypass it.
<!-- GSD:workflow-end -->



<!-- GSD:profile-start -->
## Developer Profile

> Profile not yet configured. Run `/gsd:profile-user` to generate your developer profile.
> This section is managed by `generate-claude-profile` -- do not edit manually.
<!-- GSD:profile-end -->
