---
phase: 29-winbox-bridge-system-tray-configure-path-port-and-origin-sta
plan: 01
subsystem: infra
tags: [go, winbox-bridge, config, server-lifecycle, tdd]

# Dependency graph
requires:
  - phase: 26-winbox-bridge
    provides: "Original main.go with securityCheck, buildMux, handleLaunch handlers"
provides:
  - "Config struct with JSON persistence to os.UserConfigDir()/winbox-bridge/config.json"
  - "loadConfigFrom/saveConfigTo testable helpers + loadConfig/saveConfig public API"
  - "ServerManager with mutex-protected Start/Stop/Running/Port lifecycle methods"
  - "Dynamic host header validation using configured port (not hardcoded 1337)"
  - "--no-tray flag for headless server operation on Linux servers without display"
  - "parsePort() helper for extracting port from address strings"
affects: [29-02-tray-setup, 29-03-ci-build]

# Tech tracking
tech-stack:
  added: []
  patterns:
    - "TDD: failing tests committed first, then implementation to GREEN"
    - "Testable path helpers: loadConfigFrom/saveConfigTo accept explicit path; public API delegates to configFilePath()"
    - "ServerManager: captures srv local var in goroutine (not m.server field) to prevent nil dereference race"
    - "CLI flag overrides: flags set to non-empty override config file values for backward compatibility"

key-files:
  created:
    - cmd/winbox-bridge/config.go
    - cmd/winbox-bridge/config_test.go
    - cmd/winbox-bridge/server.go
    - cmd/winbox-bridge/server_test.go
  modified:
    - cmd/winbox-bridge/main.go
    - cmd/winbox-bridge/main_test.go

key-decisions:
  - "Config uses loadConfigFrom/saveConfigTo (path-accepting helpers) for testability — public loadConfig/saveConfig delegate via configFilePath()"
  - "securityCheck takes expectedHost string param — removes hardcoded 'localhost:1337', dynamic port config enabled"
  - "ServerManager.Start goroutine captures local srv var not m.server — prevents nil dereference if Stop() races"
  - "--no-tray and default path both call runHeadless() until Plan 02 adds systray.Run()"

patterns-established:
  - "Pattern: Path-parametric helpers — unexported *From/*To variants for test isolation without env-var manipulation"
  - "Pattern: ServerManager local capture — goroutines in Start() capture the srv pointer, not the struct field"

requirements-completed: [TRAY-01, TRAY-03, TRAY-04, TRAY-05, TRAY-06]

# Metrics
duration: 4min
completed: 2026-04-08
---

# Phase 29 Plan 01: Config + ServerManager — Summary

**Config JSON persistence with os.UserConfigDir(), mutex-protected ServerManager start/stop lifecycle, and dynamic host-header validation replacing hardcoded port 1337**

## Performance

- **Duration:** ~4 min
- **Started:** 2026-04-08T20:50:45Z
- **Completed:** 2026-04-08T20:54:25Z
- **Tasks:** 2
- **Files modified:** 6 (4 created, 2 modified)

## Accomplishments

- Config struct with JSON persistence to platform-correct config dir; 0o600 file / 0o700 dir permissions (T-29-01 mitigated)
- ServerManager with mutex-protected Start/Stop/Running/Port; Stop→Start re-bind works; concurrent race prevented by local goroutine capture
- securityCheck parameterized on expectedHost — dynamic port validation, no hardcoded `"localhost:1337"` in main.go
- --no-tray headless flag enables servers without a display (Open Question 4 from RESEARCH.md resolved)
- 38 tests pass: 10 config + 20 existing HTTP handler + 8 ServerManager lifecycle

## Task Commits

Each task was committed atomically:

1. **Task 1: Config struct with JSON persistence** - `7477552` (feat)
2. **Task 2: ServerManager + --no-tray + securityCheck refactor** - `f0e3496` (feat)

_Note: TDD tasks use RED→GREEN flow: tests written first (compilation failure), then implementation to pass._

## Files Created/Modified

- `cmd/winbox-bridge/config.go` — Config struct, DefaultConfig, configFilePath, loadConfigFrom/saveConfigTo, loadConfig/saveConfig
- `cmd/winbox-bridge/config_test.go` — 10 tests: defaults, round-trip, missing file, corrupt JSON, permissions, JSON field names
- `cmd/winbox-bridge/server.go` — ServerManager with Start/Stop/Running/Port; mutex-protected; 5s shutdown timeout
- `cmd/winbox-bridge/server_test.go` — 8 tests: lifecycle, no-op guards, Stop→Start re-bind, /health over real TCP, host validation
- `cmd/winbox-bridge/main.go` — securityCheck updated (expectedHost param), main() refactored with config loading + flag overrides + --no-tray
- `cmd/winbox-bridge/main_test.go` — buildHandler updated to 3-arg form; all call sites pass "localhost:1337"

## Decisions Made

- Config uses `loadConfigFrom(path)`/`saveConfigTo(cfg, path)` path-accepting helpers so tests use `t.TempDir()` without env-var manipulation. Public `loadConfig()`/`saveConfig()` delegate to them.
- `securityCheck` receives `expectedHost string` parameter — removes hardcoded `"localhost:1337"`, enables dynamic port configuration as required by T-29-04.
- `--no-tray` and default (no flag) both call `runHeadless()` until Plan 02 replaces it with `systray.Run()` — binary is functional now.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 1 - Bug] Fixed nil pointer dereference race in ServerManager.Start goroutine**
- **Found during:** Task 2 (ServerManager lifecycle tests — TestServerManager_StopRunningFalse)
- **Issue:** The goroutine in `Start()` referenced `m.server.ListenAndServe()` — if `Stop()` raced to set `m.server = nil` before the goroutine ran, this caused a nil dereference panic (`SIGSEGV` in `net/http.(*Server).shuttingDown`)
- **Fix:** Captured `srv := &http.Server{...}` as a local variable before the goroutine; goroutine uses `srv.ListenAndServe()` instead of `m.server.ListenAndServe()`
- **Files modified:** `cmd/winbox-bridge/server.go`
- **Verification:** All 8 ServerManager tests including Stop→Start pass without panic
- **Committed in:** `f0e3496` (Task 2 commit)

---

**Total deviations:** 1 auto-fixed (Rule 1 — race condition bug)
**Impact on plan:** Fix was essential for correctness; the goroutine closure race is a standard Go pitfall (see RESEARCH.md Pitfall 3). No scope creep.

## Issues Encountered

- `go` binary not in default `$PATH` in shell environment — used explicit `/usr/local/go/bin/go` for all commands.

## User Setup Required

None — no external service configuration required.

## Next Phase Readiness

- Config struct, loadConfig/saveConfig, ServerManager, and --no-tray are all in place for Plan 02 (systray integration)
- Plan 02 needs to: add `fyne.io/systray` dependency, create `tray.go` with `setupTray()`, and replace the `runHeadless()` call in `main()` with `systray.Run()`
- No blockers

---
*Phase: 29-winbox-bridge-system-tray-configure-path-port-and-origin-sta*
*Completed: 2026-04-08*
