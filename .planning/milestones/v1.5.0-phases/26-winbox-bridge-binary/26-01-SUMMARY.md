---
phase: 26-winbox-bridge-binary
plan: 01
subsystem: bridge
tags: [go, http, security, winbox, cors, origin-validation, host-validation, cgo-free]

# Dependency graph
requires:
  - phase: 25-frontend-credential-profile-manager-winbox-actions
    provides: frontend useBridgeHealth.ts and Canvas.tsx launch contract already implemented
  - phase: 24-backend-api-bridge-download
    provides: bridge_handler.go binary naming convention (winbox-bridge-{os}-{arch}[.exe])
provides:
  - Standalone winbox-bridge Go binary (cmd/winbox-bridge/main.go)
  - Origin + Host header security middleware
  - GET /health endpoint (200 {"ok":true})
  - POST /launch endpoint ({ip, username, password} -> spawns WinBox detached)
  - discoverWinBox() with PATH search and platform defaults
  - 20 unit tests covering all security + endpoint behaviours
affects:
  - 26-02 (Makefile cross-compile + CI release job — depends on this binary source)

# Tech tracking
tech-stack:
  added: []
  patterns:
    - "Injectable startProcess var for testability without real OS process"
    - "securityCheck middleware wraps all routes for Origin+Host validation"
    - "buildMux() extracted for test handler construction"

key-files:
  created:
    - cmd/winbox-bridge/main.go
    - cmd/winbox-bridge/main_test.go
  modified:
    - .gitignore

key-decisions:
  - "CORS preflight handled in securityCheck middleware (not a separate handler) — keeps validation and CORS headers co-located"
  - "startProcess injectable var (not interface) — simplest testability pattern, matches Go stdlib testing conventions"
  - "discoverWinBoxFromPATH() extracted as separate testable function"
  - "fmt import removed — not needed in implementation; no string formatting beyond log.Printf"
  - "Added /winbox-bridge, /winbox-bridge.exe, bridge_binaries/ to .gitignore"

patterns-established:
  - "Security-first middleware: validate Origin AND Host before ANY handler runs"
  - "writeJSON/writeError helpers follow internal/api convention (JSON error body)"
  - "buildMux() factory enables test handler construction without starting a real server"

requirements-completed:
  - BRIDGE-03
  - BRIDGE-04

# Metrics
duration: 2min
completed: 2026-04-08
---

# Phase 26 Plan 01: WinBox Bridge Binary Summary

**Standalone CGO-free Go HTTP server on localhost:1337 with Origin+Host validation, /health and /launch endpoints, WinBox auto-discovery, and 20 unit tests.**

## Performance

- **Duration:** 2 min
- **Started:** 2026-04-08T12:00:21Z
- **Completed:** 2026-04-08T12:03:20Z
- **Tasks:** 1
- **Files modified:** 3 (2 created, 1 modified)

## Accomplishments

- Implemented `cmd/winbox-bridge/main.go` — a stdlib-only Go binary (CGO_ENABLED=0) that satisfies the existing frontend contract from Phases 24/25
- Security middleware validates Origin header (exact match against `--theia-origin`, default `http://localhost:3000`) and Host header (must be `localhost:1337`) on ALL routes — returns 403 on mismatch; mitigates DNS rebinding (T-26-02), cross-origin spoofing (T-26-01)
- `launchRequest` struct has exactly 3 fields (IP, Username, Password) — no executable/command field exists anywhere in the request path, making arbitrary process execution structurally impossible (T-26-06, T-26-08)
- Created `cmd/winbox-bridge/main_test.go` with 20 tests covering all security, endpoint, discovery, and CORS preflight behaviours

## Task Commits

Each task was committed atomically:

1. **Task 1: Implement winbox-bridge binary** - `10a6c81` (feat)

## Files Created/Modified

- `/home/azmin/projects/theia/cmd/winbox-bridge/main.go` — WinBox bridge HTTP server binary (stdlib only, ~270 lines)
- `/home/azmin/projects/theia/cmd/winbox-bridge/main_test.go` — 20 unit tests (~240 lines)
- `/home/azmin/projects/theia/.gitignore` — Added /winbox-bridge, /winbox-bridge.exe, bridge_binaries/

## Decisions Made

- **CORS in securityCheck middleware**: Preflight OPTIONS handling co-located with Origin/Host validation rather than a separate handler — keeps security checks and CORS headers together, reduces code paths.
- **`startProcess` injectable var**: Package-level `var startProcess = defaultStartProcess` pattern chosen over interface injection — idiomatic Go for single-function testability, matches stdlib testing conventions.
- **`discoverWinBoxFromPATH()` as separate function**: Extracted from `discoverWinBox()` to enable direct unit testing of PATH search logic without touching flag state.
- **`buildMux()` factory**: Extracted mux construction from `main()` so tests can build the full handler chain with arbitrary `winboxPath` values.

## Deviations from Plan

None - plan executed exactly as written.

## Issues Encountered

None.

## User Setup Required

None - no external service configuration required. The binary compiles locally and runs standalone.

## Next Phase Readiness

- `cmd/winbox-bridge/main.go` is the source file ready for cross-compilation in Plan 26-02 (Makefile + CI pipeline)
- Binary naming convention already implemented in `internal/api/bridge_handler.go` (Phase 24); this binary matches: `winbox-bridge-{os}-{arch}[.exe]`
- All security mitigations from the threat model (T-26-01 through T-26-08) are implemented and tested

---
*Phase: 26-winbox-bridge-binary*
*Completed: 2026-04-08*
