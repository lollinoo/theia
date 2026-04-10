---
phase: 31-dynamic-bridge-port
plan: 01
subsystem: ui
tags: [settings, winbox, bridge, typescript, react, go]

# Dependency graph
requires:
  - phase: 29-winbox-bridge-tray
    provides: WinBox bridge binary with configurable ListenPort in config.json
  - phase: 25-winbox-frontend-integration
    provides: useBridgeHealth hook, Canvas/Dashboard bridge integration, SettingsPanel bridge_secret field
provides:
  - SettingBridgePort constant and DefaultSettings entry in domain layer
  - bridge_port in settings allowlist with integer validation (rejects non-integer with 400)
  - useBridgeHealth(bridgePort: string) - dynamic health URL from port parameter
  - Canvas.tsx and Dashboard.tsx read bridge_port from settings and pass to hook and launch URL
  - SettingsPanel.tsx numeric input for bridge_port with debounced save and inline validation
affects: [winbox-bridge, canvas, dashboard, settings]

# Tech tracking
tech-stack:
  added: []
  patterns:
    - "Dynamic port wiring: settings value read at mount, passed to hook and fetch URL as template literal"
    - "TDD: failing tests committed before implementation for all 3 tasks"

key-files:
  created: []
  modified:
    - internal/domain/settings.go
    - internal/api/settings_handler.go
    - internal/api/settings_handler_test.go
    - frontend/src/hooks/useBridgeHealth.ts
    - frontend/src/hooks/useBridgeHealth.test.ts
    - frontend/src/components/Canvas.tsx
    - frontend/src/components/Dashboard.tsx
    - frontend/src/components/SettingsPanel.tsx

key-decisions:
  - "Port range validation (1-65535) handled on frontend only; backend uses strconv.Atoi (any integer) per plan — keeps backend minimal"
  - "bridgePort added to useBridgeHealth useEffect dependency array so health check restarts automatically if port changes at runtime"
  - "SettingsPanel bridge_port input placed immediately after bridge_secret field as a sibling div — consistent pattern"

patterns-established:
  - "Bridge config pattern: read bridge_port from fetchSettings at component mount, default '1337', pass as state to hook and fetch URL"
  - "Numeric settings validation: numericSettings map in settings_handler.go gates all integer-type setting keys"

requirements-completed: [BRIDGE-05, WINBOX-01, WINBOX-02, TRAY-04]

# Metrics
duration: 5min
completed: 2026-04-10
---

# Phase 31 Plan 01: Dynamic Bridge Port Summary

**Settings-driven bridge port replaces every hardcoded localhost:1337 — operators configure non-default ListenPort via SettingsPanel and all health checks and WinBox launches use it automatically**

## Performance

- **Duration:** 5 min
- **Started:** 2026-04-10T07:28:30Z
- **Completed:** 2026-04-10T07:33:00Z
- **Tasks:** 3
- **Files modified:** 8

## Accomplishments

- Added `SettingBridgePort = "bridge_port"` constant with default `"1337"` to domain layer; PUT validates as integer (rejects "abc" with 400)
- Changed `useBridgeHealth(bridgePort: string)` signature — removes hardcoded `BRIDGE_HEALTH_URL`, builds URL dynamically from port param; health check restarts on port change
- Canvas.tsx and Dashboard.tsx both read `bridge_port` from settings on mount and pass it to `useBridgeHealth` and the `/launch` fetch URL — zero hardcoded `localhost:1337` remain
- SettingsPanel.tsx renders a numeric bridge_port input (min=1, max=65535) with 500ms debounced save, SavedIndicator, and inline range/type validation error

## Task Commits

Each task was committed atomically:

1. **Task 1: Add bridge_port setting constant and backend validation** - `959e8bb` (feat)
2. **Task 2: Update useBridgeHealth hook signature and Canvas/Dashboard port wiring** - `5946b45` (feat)
3. **Task 3: Add bridge_port input field to SettingsPanel** - `3b987f7` (feat)

## Files Created/Modified

- `internal/domain/settings.go` - Added `SettingBridgePort` constant and `"1337"` default in `DefaultSettings()`
- `internal/api/settings_handler.go` - Added `SettingBridgePort` to `validSettingKeys` and `numericSettings`
- `internal/api/settings_handler_test.go` - 3 new tests: valid integer 200, invalid string 400, DefaultSettings default value
- `frontend/src/hooks/useBridgeHealth.ts` - Signature changed to `(bridgePort: string)`, removed `BRIDGE_HEALTH_URL` constant, port in useEffect deps
- `frontend/src/hooks/useBridgeHealth.test.ts` - All calls updated to pass `'1337'`; new test verifies URL uses provided port (`'9000'`)
- `frontend/src/components/Canvas.tsx` - Added `bridgePort` state, reads `bridge_port` from settings, passes to hook and launch URL
- `frontend/src/components/Dashboard.tsx` - Same pattern as Canvas.tsx
- `frontend/src/components/SettingsPanel.tsx` - Added `bridgePort` state, refs, `handleBridgePortChange`, and numeric input JSX after bridge_secret field

## Decisions Made

- Port range validation (1-65535) is frontend-only; backend `numericSettings` check uses `strconv.Atoi` (accepts any integer). This follows the plan's explicit note to keep backend validation minimal.
- `bridgePort` added to `useBridgeHealth`'s `useEffect` dependency array so a settings change at runtime automatically restarts the health polling loop on the new port.

## Deviations from Plan

None - plan executed exactly as written.

## Issues Encountered

None.

## User Setup Required

None - no external service configuration required. The bridge_port setting defaults to `1337` matching the bridge binary's default `ListenPort`, so existing installations work without any configuration change.

## Next Phase Readiness

- Dynamic bridge port is fully wired end-to-end: backend → API → frontend settings → health check + launch URL
- Operators running the bridge on a non-default port can now configure it via Settings without source code changes
- No blockers — v1.5.0 WinBox Integration milestone is complete

---
*Phase: 31-dynamic-bridge-port*
*Completed: 2026-04-10*
