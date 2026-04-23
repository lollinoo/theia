# Coding Conventions

**Analysis Date:** 2026-04-23

## Naming Patterns

**Files:**
- Go uses package-oriented snake_case filenames under `cmd/` and `internal/`: `internal/service/device_service.go`, `internal/repository/sqlite/device_repo.go`, `internal/ws/hub_broadcast_ch.go`.
- Go tests live beside implementation with `_test.go`: `internal/api/device_handler_test.go`, `internal/repository/sqlite/device_repo_test.go`.
- Frontend components use PascalCase `.tsx`: `frontend/src/components/DeviceCard.tsx`, `frontend/src/components/dashboard/BulkBackupPanel.tsx`.
- Frontend hooks use `useX.ts`: `frontend/src/hooks/useWebSocket.ts`, `frontend/src/hooks/useBridgeHealth.ts`.
- Frontend utilities and model helpers use camelCase filenames: `frontend/src/utils/freshness.ts`, `frontend/src/components/canvas/topologyComposer.ts`, `frontend/src/components/forms/deviceFormModels.ts`.
- Frontend tests are co-located as `.test.ts` / `.test.tsx`, with a few audit tests in `frontend/src/components/__tests__/`.

**Functions:**
- Go exported constructors and handlers use PascalCase with domain prefixes: `NewDeviceHandler` in `internal/api/device_handler.go`, `NewDeviceService` in `internal/service/device_service.go`, `NormalizeTopologyDiscoveryMode` in `internal/domain/device.go`.
- Go unexported helpers use lower camelCase and are kept near call sites: `writeError`, `decodeJSON`, and `isValidIPOrHostname` in `internal/api/device_handler.go`.
- React components use PascalCase for exported components and lower camelCase for local helpers: `DeviceCardInner`, `displayName`, `formatPercent`, and `buildReadouts` in `frontend/src/components/DeviceCard.tsx`.
- React hooks start with `use`: `useWebSocket` in `frontend/src/hooks/useWebSocket.ts`, `usePositions` in `frontend/src/hooks/usePositions.ts`.
- Parser/normalizer functions use verb prefixes: `parseDevicesResponse` in `frontend/src/types/api.ts`, `parseWSMessage` and `mergeSnapshotDelta` in `frontend/src/types/metrics.ts`.

**Variables:**
- Go variables use lower camelCase; constants use PascalCase when exported and lower camelCase when package-private: `DeviceTypeRouter` in `internal/domain/device.go`, `incompleteLinkReprobeDelay` in `internal/service/device_service.go`.
- Go request/response structs are unexported when handler-local: `createDeviceRequest`, `snmpCredsRequest`, `jsonAPIResource` in `internal/api/device_handler.go`.
- TypeScript values use camelCase; constants use camelCase unless representing static records: `deviceTypeLabels`, `subtypeLabels`, `macAddressPattern` in `frontend/src/components/DeviceCard.tsx`.
- API payload fields mirror backend snake_case at boundaries: `CreateDevicePayload.metrics_source` in `frontend/src/api/client.ts`, `Device.device_type` in `frontend/src/types/api.ts`.

**Types:**
- Go domain enums are custom string types with typed constants: `DeviceType`, `DeviceStatus`, `MetricsSource`, `TopologyDiscoveryMode` in `internal/domain/device.go`.
- Go interfaces define repository seams in domain packages: `DeviceRepository` in `internal/domain/device.go`.
- TypeScript interfaces describe component props and API DTOs: `DeviceNodeData` in `frontend/src/components/DeviceCard.tsx`, `CreateDevicePayload` in `frontend/src/api/client.ts`.
- TypeScript union types represent narrow UI/control states: `DetailControlType` in `frontend/src/hooks/useWebSocket.ts`, `Readout['tone']` in `frontend/src/components/DeviceCard.tsx`.

## Code Style

**Formatting:**
- Go follows `gofmt`/standard Go formatting. Imports are grouped stdlib first, blank line, third-party/local imports as in `internal/api/device_handler.go`.
- Frontend uses Biome configured in `frontend/biome.json`.
- Use 2-space indentation for frontend code, single quotes, semicolons, trailing commas, and 100-character line width per `frontend/biome.json`.
- Use TypeScript strict mode with no unused locals/parameters and no fallthrough in `frontend/tsconfig.app.json` and `frontend/tsconfig.test.json`.

**Linting:**
- Frontend linting is `npm --prefix frontend run lint` / `npm --prefix frontend run check`, backed by `frontend/biome.json`.
- Biome recommended rules are enabled, with selected relaxations: `a11y.noLabelWithoutControl`, `complexity.noForEach`, `correctness.useExhaustiveDependencies`, `style.useImportType`, and `style.noNonNullAssertion` are disabled in `frontend/biome.json`.
- Backend gate uses `go vet ./...` in `Makefile` target `backend-fast`.
- CI runs `make backend-fast`, `make frontend-fast`, `make realtime-stress`, `make collector-contract`, and `make browser-e2e` in `.github/workflows/ci.yml`.

## Import Organization

**Order:**
1. Standard library imports in Go (`encoding/json`, `net/http`, `testing`) and package imports in TypeScript (`react`, `@testing-library/react`, `@xyflow/react`).
2. Third-party packages (`github.com/google/uuid`, `@vitejs/plugin-react`, `vitest`).
3. Local project imports (`github.com/lollinoo/theia/internal/domain`, `../types/api`, `./DeviceCard`).

**Path Aliases:**
- No TypeScript path aliases are configured in `frontend/tsconfig.app.json`; use relative imports such as `../types/api` and `./errors` in `frontend/src/api/client.ts`.
- Go imports use the module path `github.com/lollinoo/theia` from `go.mod`.

## Error Handling

**Patterns:**
- Backend HTTP handlers validate early and return immediately through `writeError` in `internal/api/device_handler.go`.
- Backend request parsing uses `decodeJSON(w, r, &req)` returning `false` after writing the response; follow this pattern in handlers under `internal/api/`.
- Backend internal errors should be logged server-side with correlation IDs by `writeError` in `internal/api/device_handler.go`; user-facing responses stay generic for 5xx.
- Backend services return errors to callers and use `log.Printf` only for asynchronous/non-fatal background failures: `markDeviceStatus` in `internal/service/device_service.go`, WebSocket hub logging in `internal/ws/hub.go`.
- Frontend API wrappers catch unknown errors and rethrow contextual `Error` messages: `fetchDevices`, `fetchLinks`, and `fetchSettings` in `frontend/src/api/client.ts`.
- Frontend mutation APIs convert 400/409 responses to `ValidationError` and 500 responses to `ServerError` with optional correlation IDs in `frontend/src/api/client.ts` and `frontend/src/api/errors.ts`.
- UI components handle `ServerError` and `ValidationError` explicitly before generic errors: `frontend/src/components/BulkBackupPanel.tsx`, `frontend/src/components/SNMPProfileManager.tsx`, `frontend/src/components/LinkCreatePanel.tsx`.

## Logging

**Framework:** Go standard `log`; browser `console` only for exceptional diagnostics.

**Patterns:**
- Use `log.Printf` in backend long-running workers and infrastructure paths: `internal/worker/metrics_collector.go`, `internal/worker/backup_scheduler.go`, `internal/ws/hub.go`.
- Request logging is centralized in `RequestLogger` in `internal/api/middleware.go`; avoid duplicate per-handler access logs.
- Frontend logs parse/network failures with `console.error` in targeted places such as `frontend/src/hooks/useWebSocket.ts`, `frontend/src/hooks/usePositions.ts`, and dashboard fetch panels.
- Tests may use `console.error` to print audit violations before failing, as in `frontend/src/components/__tests__/canvas-token-audit.test.ts` and `frontend/src/components/__tests__/no-line-audit.test.ts`.

## Comments

**When to Comment:**
- Use Go doc comments for exported types and functions: `DeviceHandler`, `NewDeviceHandler`, `DeviceService`, and `NewDeviceService` in `internal/api/device_handler.go` and `internal/service/device_service.go`.
- Use comments to explain protocol/domain decisions, not mechanical code. Examples: virtual device validation notes in `internal/api/device_handler.go` and WebSocket reconnect notes in `frontend/src/hooks/useWebSocket.ts`.
- Keep phase/audit comments only when they encode enforcement context, as in `frontend/src/components/__tests__/canvas-token-audit.test.ts`.

**JSDoc/TSDoc:**
- TSDoc is light and reserved for exported error classes or audit rationale: `frontend/src/api/errors.ts`.
- Prefer self-documenting TypeScript interfaces and types over extensive comments in component code: `frontend/src/components/DeviceCard.tsx`.

## Function Design

**Size:**
- Prefer small pure helpers for formatting, normalization, and state derivation: `formatPercent`, `freshnessMeta`, and `readoutToneClass` in `frontend/src/components/DeviceCard.tsx`; `NormalizeTopologyDiscoveryMode` in `internal/domain/device.go`.
- Handler functions may be longer when performing endpoint validation and response shaping; keep validation as guard clauses and move reusable logic to helpers in the same package.
- For complex backend services, use option functions and coordinator/helper services instead of expanding constructors: `DeviceServiceOption` and `WithTopologyObservationStore` in `internal/service/device_service.go`.

**Parameters:**
- Go service methods pass `context.Context` first for operations that touch repositories or external work: `AddDevice` in `internal/service/device_service.go`.
- Use option functions for optional backend dependencies: `DeviceServiceOption` in `internal/service/device_service.go`.
- TypeScript functions use typed payload interfaces for API mutations: `CreateDevicePayload` and `SNMPPayload` in `frontend/src/api/client.ts`.
- React components accept one props object and derive internal display models through local helpers: `DeviceCardInner` in `frontend/src/components/DeviceCard.tsx`.

**Return Values:**
- Go returns `(value, error)` and checks errors immediately; tests use `t.Fatalf` on unexpected errors as in `internal/repository/sqlite/device_repo_test.go`.
- Backend HTTP helpers return booleans when they already wrote a response: `decodeJSON` in `internal/api/device_handler.go`.
- Frontend async API methods return parsed DTOs and throw typed errors on failures: `createDevice`, `updateDevice`, and `fetchDevices` in `frontend/src/api/client.ts`.
- React hooks return explicit result interfaces: `UseWebSocketResult` in `frontend/src/hooks/useWebSocket.ts`.

## Module Design

**Exports:**
- Backend packages expose constructors and interfaces; keep implementation structs unexported when package-local: `pollRescheduler` in `internal/service/device_service.go`.
- Frontend component modules usually default export the component and named-export supporting types: `DeviceCard` / `DeviceNodeData` in `frontend/src/components/DeviceCard.tsx`.
- Frontend API modules named-export all client functions and error classes: `frontend/src/api/client.ts`.
- Parser modules named-export parse functions and DTO types: `frontend/src/types/api.ts`, `frontend/src/types/metrics.ts`.

**Barrel Files:**
- No broad frontend barrel file is used; import directly from concrete modules such as `frontend/src/types/api.ts` and `frontend/src/api/client.ts`.
- Go package boundaries are directories; add new backend code under the appropriate package in `internal/` and import via `github.com/lollinoo/theia/internal/...`.

---

*Convention analysis: 2026-04-23*
