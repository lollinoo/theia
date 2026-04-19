# Coding Conventions

**Analysis Date:** 2026-04-19

## Naming Patterns

**Files:**
- Use `PascalCase.tsx` for React components and component tests in `frontend/src/components/Toolbar.tsx`, `frontend/src/components/Toolbar.test.tsx`, and `frontend/src/components/Dashboard.tsx`.
- Use `camelCase.ts` for hooks, utilities, and plain modules in `frontend/src/hooks/useWebSocket.ts`, `frontend/src/utils/validation.ts`, and `frontend/src/components/deviceVisualState.ts`.
- Use `snake_case.go` for Go packages and `*_test.go` for tests in `internal/config/config.go`, `internal/service/device_service.go`, and `internal/service/device_service_test.go`.
- Keep test files adjacent to implementation files when possible, with special audit suites collected under `frontend/src/components/__tests__/`.

**Functions:**
- Use `camelCase` for TypeScript functions and hooks, with `use` prefixes for hooks such as `useWebSocket` in `frontend/src/hooks/useWebSocket.ts` and parser helpers like `parseDeviceType` in `frontend/src/types/api.ts`.
- Use `PascalCase` for exported Go functions and types, and lower camel-case for unexported helpers, as in `Load` / `Config` in `internal/config/config.go` and `defaults` in the same file.
- Name tests with `TestXxx` in Go and `describe`/`it` strings that mirror behavior in TypeScript, e.g. `TestRefreshDevices_SkipsUnmanagedDevices` in `internal/scheduler/scheduler_test.go` and `'handles prometheus_status message'` in `frontend/src/hooks/useWebSocket.test.ts`.

**Variables:**
- Use descriptive `camelCase` state and local variables in frontend code: `activeView`, `selectedAreaId`, and `detailDeviceId` in `frontend/src/App.tsx`.
- Use concise receiver names in Go methods, usually `s` or `r`, as in `func (s *DeviceService)` in `internal/service/device_service.go` and `func (r *mockDeviceRepo)` in `internal/service/device_service_test.go`.
- Prefer boolean names that read as predicates, such as `reconnecting` in `frontend/src/hooks/useWebSocket.ts`, `movedExisting` in `cmd/theia/main.go`, and `TopologyChanged` assertions in `internal/service/static_persistence_test.go`.

**Types:**
- Use `interface` and `type` aliases for frontend contracts in `frontend/src/api/client.ts` and `frontend/src/types/api.ts`.
- Use string-literal unions in TypeScript for enums such as `DeviceType`, `MetricsSource`, and `TopologyDiscoveryMode` in `frontend/src/types/api.ts`.
- Use typed string enums and structs in Go domain code, e.g. `DeviceType`, `DeviceStatus`, and `Device` in `internal/domain/device.go`.

## Code Style

**Formatting:**
- No ESLint, Prettier, or Biome config is detected at the repository root or in `frontend/`.
- Frontend formatting is mostly manual and close to Prettier-like defaults in `frontend/src/App.tsx`, `frontend/src/hooks/useWebSocket.ts`, and `frontend/src/contexts/ThemeContext.tsx`: semicolons, single quotes, trailing commas in multiline literals, and 2-space indentation.
- Frontend indentation is not fully uniform. `frontend/src/components/Toolbar.tsx` uses 4-space indentation while neighboring files such as `frontend/src/App.tsx` and `frontend/src/components/MaterialIcon.tsx` use 2-space indentation. Preserve the surrounding file style when editing.
- Go code follows `gofmt`-style formatting with tabs and grouped imports in `internal/config/config.go`, `internal/domain/device.go`, and `cmd/theia/main.go`.

**Linting:**
- TypeScript strictness is enforced through compiler settings in `frontend/tsconfig.test.json`: `strict`, `noUnusedLocals`, `noUnusedParameters`, and `noFallthroughCasesInSwitch` are enabled.
- The frontend build uses `tsc -b tsconfig.app.json && vite build` via `frontend/package.json`, so type errors block the production build.
- Backend verification relies on `go vet` and `go build` via `make verify` in `Makefile` rather than a dedicated linter config.

## Import Organization

**Order:**
1. Third-party packages first, such as React and library imports in `frontend/src/App.tsx` and standard-library packages in `internal/config/config.go`.
2. Internal relative imports second, such as `./components/Canvas` and `../types/api` in `frontend/src/App.tsx` and `frontend/src/api/client.ts`.
3. Type imports are either grouped inline with value imports or pulled in with `type`, as in `frontend/src/App.tsx`, `frontend/src/api/client.ts`, and `frontend/src/contexts/ThemeContext.tsx`.

**Path Aliases:**
- No path aliases are configured in `frontend/tsconfig.json` or `frontend/tsconfig.test.json`.
- Use relative imports like `../api/client` and `./MaterialIcon`, matching `frontend/src/components/BulkEditPanel.test.tsx` and `frontend/src/components/Toolbar.tsx`.

## Error Handling

**Patterns:**
- Wrap low-level fetch failures with user-facing context in the frontend. `frontend/src/api/client.ts` catches request failures and rethrows messages such as `Failed to fetch devices: ...`.
- Use typed frontend errors when status codes carry product meaning. `frontend/src/api/client.ts` throws `ValidationError` for `400`/`409` and `ServerError` for `500` responses.
- Treat some UI-side failures as non-fatal and swallow them intentionally when the screen can degrade gracefully, as in the area fetch `catch(() => {})` calls in `frontend/src/App.tsx`.
- Throw early on invalid payload shapes in parser modules such as `frontend/src/types/api.ts` (`invalid devices response`, `invalid interface payload`).
- In Go, return wrapped errors with `%w` for caller context, as in `internal/config/config.go` and `cmd/theia/main.go`.
- In Go tests, fail immediately with `t.Fatalf` for setup or invariant violations and use `t.Errorf` only when checking multiple related assertions, as in `internal/domain/poll_class_test.go`.

## Logging

**Framework:** `log` in Go, `console` in frontend

**Patterns:**
- Backend runtime logging uses the standard library `log` package in `cmd/theia/main.go` and `internal/service/device_service.go`.
- Logging focuses on operational events and warnings, such as restore workflow progress in `cmd/theia/main.go` and failed status updates in `internal/service/device_service.go`.
- Frontend logging is minimal and mostly reserved for unexpected parse/runtime failures, e.g. `console.error('Failed to parse WebSocket message', error)` in `frontend/src/hooks/useWebSocket.ts`.
- Tests that verify logging capture global log output with helpers like `captureLogs` in `internal/service/static_persistence_test.go`.

## Comments

**When to Comment:**
- Comment exported Go types and functions with doc comments, matching `Config`, `Load`, and `DeviceService` in `internal/config/config.go` and `internal/service/device_service.go`.
- Add inline comments for non-obvious branches, fallback behavior, or product rules, as seen in `frontend/src/api/client.ts`, `frontend/src/hooks/useWebSocket.ts`, and `cmd/theia/main.go`.
- Tests often annotate scenario intent with requirement IDs or behavior labels, such as `THEME-01` in `frontend/src/contexts/ThemeContext.test.tsx`, `COMP-04` in `frontend/src/components/Toolbar.test.tsx`, and `D-04`/`D-07` references in `internal/domain/poll_class_test.go`.

**JSDoc/TSDoc:**
- Lightweight JSDoc is used selectively for reusable UI primitives like `frontend/src/components/MaterialIcon.tsx`.
- Full TSDoc is not consistently used across the frontend; most guidance is inline or implicit from types.
- Go doc comments are more consistently applied to exported symbols than TSDoc is in TypeScript.

## Function Design

**Size:**
- Keep utility and parser helpers small and single-purpose, as in `readString`, `readNumber`, and `parseDeviceType` inside `frontend/src/types/api.ts`.
- Larger orchestrator-style functions are accepted for workflow-heavy paths, such as `useWebSocket` in `frontend/src/hooks/useWebSocket.ts`, `AddDevice` in `internal/service/device_service.go`, and restore helpers in `cmd/theia/main.go`. When editing these files, preserve the existing helper extraction style instead of inlining more logic.

**Parameters:**
- Prefer strongly typed object shapes for frontend payloads, as in `CreateDevicePayload` and `SNMPPayload` in `frontend/src/api/client.ts`.
- Use optional and nullable fields deliberately to represent API semantics, e.g. `notes?: string | null` and `poll_interval_override: number | null` in `frontend/src/types/api.ts`.
- In Go, dependency injection is primarily constructor- and function-based, using interfaces and function types such as `DiscoverFunc`, `SNMPPollFunc`, and `DeviceServiceOption` in `internal/service/device_service.go`.

**Return Values:**
- Return parsed, domain-shaped objects from frontend API helpers instead of raw JSON, as in `fetchDevices`, `createDevice`, and `fetchSettings` in `frontend/src/api/client.ts`.
- Use `null`/empty-object fallbacks only when the UI can tolerate degraded data, such as `fetchHealthVersion` and `fetchSettings` in `frontend/src/api/client.ts`.
- In Go, return `(value, error)` and keep mutation explicit through pointer receivers and repository updates, as in `internal/config/config.go` and `internal/service/device_service.go`.

## Module Design

**Exports:**
- Frontend modules mostly use named exports for reusable units, such as `ThemeProvider`, `useTheme`, `MaterialIcon`, and `useWebSocket` in `frontend/src/contexts/ThemeContext.tsx`, `frontend/src/components/MaterialIcon.tsx`, and `frontend/src/hooks/useWebSocket.ts`.
- Default exports are used sparingly for app-level entry files like `frontend/src/App.tsx`.
- Go packages export domain and service APIs directly from package files; there is no barrel-file pattern.

**Barrel Files:**
- Barrel files are not a dominant pattern. Imports usually target concrete files directly, such as `./components/Dashboard` from `frontend/src/App.tsx` and package-level imports like `github.com/lollinoo/theia/internal/service` from `cmd/theia/main.go`.

## Prescriptive Guidance

- Match the file-local formatting style before editing: use 2-space indentation in most frontend files, but preserve existing 4-space indentation in `frontend/src/components/Toolbar.tsx` unless you normalize the whole file.
- Keep frontend imports relative and grouped with external packages first; do not introduce path aliases without updating `frontend/tsconfig.json`.
- Use typed error classes from `frontend/src/api/errors.ts` when mapping backend validation or server failures in UI code.
- Prefer small parser/helper functions for shape validation in API-facing TypeScript modules, following `frontend/src/types/api.ts`.
- In Go, keep exported symbols documented and return wrapped errors with `%w`, following `internal/config/config.go` and `cmd/theia/main.go`.
- In tests, keep scenario names explicit and behavior-driven, following `frontend/src/hooks/useWebSocket.test.ts` and `internal/scheduler/scheduler_test.go`.

---

*Convention analysis: 2026-04-19*
