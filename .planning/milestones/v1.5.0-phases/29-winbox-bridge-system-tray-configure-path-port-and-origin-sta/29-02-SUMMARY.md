---
phase: 29-winbox-bridge-system-tray-configure-path-port-and-origin-sta
plan: 02
subsystem: infra
tags: [go, systray, fyne, tray-icon, winbox-bridge, desktop-integration, cgo]

# Dependency graph
requires:
  - phase: 29-01
    provides: ServerManager (Start/Stop/Running/Port), Config struct (loadConfig/saveConfig/configFilePath), main.go with --no-tray flag and headless path

provides:
  - System tray icon for WinBox bridge binary using fyne.io/systray v1.12.0
  - setupTray() function wiring Start/Stop/Status/Open Config/Quit menu items to ServerManager
  - Embedded tray icon (22x22 blue PNG) via go:embed in icon.go
  - Config reload from disk on each Start click (user edits take effect immediately on next start)
  - Cross-platform file editor opener (xdg-open/open/cmd /c start)
  - systray.Run() in main() satisfying macOS Cocoa main-thread requirement

affects:
  - 29-03 (CI build strategy — fyne.io/systray requires CGO on macOS, split build jobs needed)

# Tech tracking
tech-stack:
  added:
    - fyne.io/systray v1.12.0 (system tray icon and menu, CGO-free on Windows/Linux, CGO required on macOS)
    - github.com/godbus/dbus/v5 v5.1.0 (indirect — Linux DBus backend for fyne.io/systray)
  patterns:
    - systray.Run() must block main() — all goroutines spawned inside onReady callback (macOS Cocoa requirement)
    - config reloaded from disk on each Start click — no restart required to pick up user config edits
    - menu item enabled/disabled state mirrors ServerManager.Running() — prevents invalid Start-while-running or Stop-while-stopped clicks (T-29-06 mitigation)

key-files:
  created:
    - cmd/winbox-bridge/tray.go
    - cmd/winbox-bridge/icon.go
    - cmd/winbox-bridge/icon.png
  modified:
    - cmd/winbox-bridge/main.go
    - go.mod
    - go.sum

key-decisions:
  - "setupTray auto-starts the server before systray.Run() in main() — bridge is immediately usable on launch without a manual Start click"
  - "Config reloaded from disk on every Start menu click — users can edit config.json and click Start without restarting the binary"
  - "ensureConfigFileExists() creates config.json before opening in editor — first-run UX is seamless (no 'file not found' in editor)"

patterns-established:
  - "Tray state pattern: updateState() called after every Start/Stop action to keep menu enabled/disabled in sync with ServerManager.Running()"
  - "openFileInEditor uses OS-appropriate opener: xdg-open (Linux), open (macOS), cmd /c start (Windows)"

requirements-completed:
  - TRAY-01
  - TRAY-02

# Metrics
duration: 2min
completed: 2026-04-08
---

# Phase 29 Plan 02: System Tray Integration Summary

**fyne.io/systray tray icon with Start/Stop/Status/Open Config/Quit menu — wired to ServerManager, config reloaded on each Start click, systray.Run() blocks main()**

## Performance

- **Duration:** ~2 min
- **Started:** 2026-04-08T20:57:10Z
- **Completed:** 2026-04-08T20:58:56Z
- **Tasks:** 1 of 2 (stopped at checkpoint:human-verify)
- **Files modified:** 6

## Accomplishments

- Added fyne.io/systray v1.12.0 to go.mod (direct dependency, godbus/dbus/v5 as indirect)
- Created tray.go: setupTray() builds menu with Start Server, Stop Server, Status (display-only), Open Config File, Quit; goroutine event loop handles clicks; config reloaded from disk on Start; openFileInEditor uses OS-appropriate opener
- Created icon.go + icon.png: 22x22 blue (#4A90D9) PNG embedded via go:embed
- Updated main.go: replaced headless placeholder with systray.Run() blocking main(); auto-starts server before tray setup; --no-tray headless path unchanged

## Task Commits

1. **Task 1: Add fyne.io/systray dependency, create icon.go + icon.png, create tray.go, wire into main.go** - `31e7b11` (feat)

## Files Created/Modified

- `cmd/winbox-bridge/tray.go` — setupTray(), ensureConfigFileExists(), openFileInEditor()
- `cmd/winbox-bridge/icon.go` — embedded PNG icon via go:embed
- `cmd/winbox-bridge/icon.png` — 22x22 blue PNG for system tray
- `cmd/winbox-bridge/main.go` — systray.Run() wired in, fyne.io/systray imported
- `go.mod` — fyne.io/systray v1.12.0 added as direct dependency
- `go.sum` — checksum entries for systray + godbus/dbus/v5

## Decisions Made

- Server auto-starts before `systray.Run()` in `main()` — bridge is immediately usable on launch without a manual "Start" click in the tray menu
- Config is reloaded from disk on every "Start Server" menu click — users can edit `config.json` and immediately apply changes by clicking Stop then Start without restarting the binary
- `ensureConfigFileExists()` creates the config file before opening in the editor — guarantees a valid JSON file exists for the user to edit even on first run

## Deviations from Plan

None — plan executed exactly as written. Minor addition: ran `go mod tidy` after `go get` to promote `fyne.io/systray` from `// indirect` to direct (since it is directly imported in package main).

## Issues Encountered

None. `go get fyne.io/systray@v1.12.0` resolved cleanly, pulling `godbus/dbus/v5` as an indirect dep. Build succeeded on first attempt. All existing tests pass without modification.

## Known Stubs

None — tray integration is fully wired. Menu items connect to real ServerManager methods.

## User Setup Required

None — no external service configuration required.

## Checkpoint Status

Stopped at **Task 2: Verify system tray integration on desktop** (checkpoint:human-verify).

The binary is built at `/tmp/winbox-bridge-test` for immediate testing. See checkpoint message for exact verification steps.

## Next Phase Readiness

- After human verification approves: plan 29-02 is complete, ready for 29-03 (CI build strategy update for macOS CGO)
- The macOS build in CI will require a new `macos-latest` runner job since fyne.io/systray uses CGO on macOS (Objective-C/Cocoa)

---
*Phase: 29-winbox-bridge-system-tray-configure-path-port-and-origin-sta*
*Completed: 2026-04-08 (partial — stopped at checkpoint)*
