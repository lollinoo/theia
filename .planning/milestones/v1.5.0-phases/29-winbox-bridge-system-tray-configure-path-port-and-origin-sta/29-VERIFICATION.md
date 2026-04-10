---
phase: 29-winbox-bridge-system-tray-configure-path-port-and-origin-sta
verified: 2026-04-09T13:38:00Z
status: human_needed
score: 5/6 must-haves verified (1 requires human)
human_verification:
  - test: "Launch ./winbox-bridge without --no-tray and verify tray icon appears"
    expected: "System tray icon appears in notification area; menu shows Status (Running on :1337), Start Server (disabled), Stop Server (enabled), Open Config File, Quit"
    why_human: "systray.Run() blocks the process; cannot be tested programmatically without a display"
  - test: "Click Stop Server, then Start Server; verify status reflects port change after config edit"
    expected: "Status changes between 'Stopped' and 'Running on :PORT'; after editing port in config.json and clicking Stop then Start, port updates in status"
    why_human: "Interactive tray menu behavior requires a desktop environment"
  - test: "Click Open Config File"
    expected: "config.json opens in the OS default text editor"
    why_human: "xdg-open/open/notepad.exe launch requires a running desktop session"
  - test: "Click Quit"
    expected: "Bridge process exits cleanly; server stops"
    why_human: "Process lifecycle requires interactive tray interaction"
  - test: "Run ./winbox-bridge --no-tray and send SIGINT"
    expected: "Server starts, responds to GET /health, exits cleanly on Ctrl+C"
    why_human: "Signal-based shutdown requires an interactive terminal or manual kill"
---

# Phase 29: WinBox Bridge System Tray Verification Report

**Phase Goal:** The WinBox bridge binary shows a system tray icon with menu items to start/stop the HTTP server, configure WinBox path, listen port, and allowed origin, and persist settings to a JSON config file — all without restarting the process
**Verified:** 2026-04-09T13:38:00Z
**Status:** human_needed
**Re-verification:** No — initial verification

## Goal Achievement

### Observable Truths

| #  | Truth                                                                                             | Status     | Evidence                                                                                                     |
|----|---------------------------------------------------------------------------------------------------|------------|--------------------------------------------------------------------------------------------------------------|
| 1  | Bridge settings persist in JSON config at OS-appropriate config dir                               | VERIFIED   | `configFilePath()` uses `os.UserConfigDir()/winbox-bridge/config.json`; `loadConfig`/`saveConfig` tested (14 config tests pass) |
| 2  | HTTP server can be started and stopped from tray without restarting the bridge process             | VERIFIED   | `ServerManager` with `Start`/`Stop`/`Running`/`Port` — 8 lifecycle tests pass including Stop→Start re-bind   |
| 3  | Host header validation uses configured port dynamically (not hardcoded 1337)                      | VERIFIED   | `securityCheck` takes `expectedHost string` param; `fmt.Sprintf("localhost:%d", cfg.ListenPort)` in `ServerManager.Start`; grep confirms no `"localhost:1337"` in security logic |
| 4  | `--no-tray` flag enables headless operation (start server, exit on SIGINT/SIGTERM)                | VERIFIED   | `noTray := flag.Bool("no-tray", false, ...)` in `main.go`; `runHeadless()` called when set; `signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)` present |
| 5  | Windows binaries suppress console window via `-H=windowsgui` ldflags                             | VERIFIED   | Makefile `ldextra="-H=windowsgui"` for Windows; CI `ldflags: "-s -w -H=windowsgui"` for windows/amd64 and windows/arm64 |
| 6  | macOS binaries build with CGO_ENABLED=1 on native macOS CI runners                               | VERIFIED   | CI matrix: darwin entries use `runs-on: macos-latest`, `cgo: "1"` (2 entries confirmed); `softprops/action-gh-release@v2` upload maintained for all 6 targets |
| 7  | System tray icon appears when bridge launched without --no-tray (TRAY-01/02 user-visible UX)      | ? UNCERTAIN | `setupTray()` implemented with full menu wiring; `systray.Run()` blocks main() (not in goroutine — verified); icon embedded via `//go:embed icon.png`/`icon.ico`; requires human verification on desktop |

**Score:** 6/7 truths verified programmatically; 1 requires human verification (visual/interactive tray UX)

### Required Artifacts

| Artifact                                     | Expected                                         | Status     | Details                                                                             |
|----------------------------------------------|--------------------------------------------------|------------|-------------------------------------------------------------------------------------|
| `cmd/winbox-bridge/config.go`                | Config struct, DefaultConfig, load/save funcs    | VERIFIED   | 111 lines; all required funcs present; 0o700 dir / 0o600 file perms; path-parametric helpers |
| `cmd/winbox-bridge/config_test.go`           | Config round-trip, defaults, missing file tests  | VERIFIED   | 14 Test functions covering defaults, round-trip, corrupt JSON, permissions, field names, log_level backward compat, ensureBridgeSecret |
| `cmd/winbox-bridge/server.go`                | ServerManager with Start/Stop/Running/Port       | VERIFIED   | 82 lines; mutex-protected; local `srv` capture prevents nil dereference race        |
| `cmd/winbox-bridge/server_test.go`           | ServerManager lifecycle tests                    | VERIFIED   | 8 Test functions: Start→Running, Stop→!Running, no-ops, Stop→Start re-bind, /health, host check |
| `cmd/winbox-bridge/main.go`                  | Refactored main with config loading, --no-tray   | VERIFIED   | `loadConfig()` called; flag overrides applied; `systray.Run(...)` blocks main(); `--no-tray` headless path |
| `cmd/winbox-bridge/tray.go`                  | setupTray wiring menu to ServerManager           | VERIFIED   | `setupTray(mgr, initialCfg, activeLogFile)`; Start/Stop/Status/Open Config/Quit menu items; config reload on Start click |
| `cmd/winbox-bridge/icon_other.go`            | Embedded tray icon bytes via go:embed (non-Win)  | VERIFIED   | `//go:embed icon.png` with `var iconBytes []byte`                                    |
| `cmd/winbox-bridge/icon_windows.go`          | Embedded tray icon bytes via go:embed (Windows)  | VERIFIED   | `//go:embed icon.ico` with `var iconBytes []byte`                                    |
| `cmd/winbox-bridge/icon.png`                 | PNG icon file (non-zero size)                    | VERIFIED   | 124 bytes, 22x22 blue PNG                                                           |
| `cmd/winbox-bridge/icon.ico`                 | ICO icon file for Windows                        | VERIFIED   | 5,258 bytes                                                                         |
| `go.mod`                                     | fyne.io/systray v1.12.0 dependency               | VERIFIED   | `fyne.io/systray v1.12.0` present as direct dependency                               |
| `Makefile`                                   | bridge-build-all with Windows ldflags, macOS note | VERIFIED  | `BRIDGE_TARGETS_NOCGO` (no darwin); `-H=windowsgui` for Windows; NOTE echo for macOS |
| `.github/workflows/ci.yml`                   | Split build-bridge: ubuntu for Win/Linux, macos-latest for macOS | VERIFIED | 6 matrix entries; darwin uses `macos-latest` + `cgo: "1"`; windows uses `-H=windowsgui` |

### Key Link Verification

| From                        | To                          | Via                                              | Status   | Details                                                              |
|-----------------------------|-----------------------------|-------------------------------------------------|----------|----------------------------------------------------------------------|
| `main.go`                   | `config.go`                 | `loadConfig()` call in `main()`                  | WIRED    | Line 365: `cfg, err := loadConfig()`                                  |
| `main.go`                   | `server.go`                 | `mgr.Start(cfg)` call in `main()`                | WIRED    | Lines 423, 445: `mgr.Start(cfg)`                                      |
| `main.go`                   | `tray.go`                   | `systray.Run(...)` in `main()` (not goroutine)   | WIRED    | Lines 456-459: `systray.Run(func() { setupTray(mgr, cfg, ...) }, ...)` |
| `tray.go`                   | `server.go`                 | `mgr.Start(cfg)` and `mgr.Stop()` from menu handlers | WIRED | Lines 82, 87, 109 in tray.go                                         |
| `tray.go`                   | `config.go`                 | `loadConfig()` on Start click; `configFilePath()` for Open Config | WIRED | Lines 76, 91 in tray.go                                |
| `server.go`                 | `main.go`                   | `buildMux(...)` called inside `ServerManager.Start()` | WIRED | `buildMux(winboxPath, cfg.TheiaOrigin, expectedHost, cfg.BridgeSecret)` line 33 |
| `.github/workflows/ci.yml`  | `cmd/winbox-bridge/`        | `go build ./cmd/winbox-bridge/` in build steps  | WIRED    | Line 279 in ci.yml                                                    |
| `Makefile`                  | `cmd/winbox-bridge/`        | `BRIDGE_SRC := ./cmd/winbox-bridge/`             | WIRED    | `go build ... $(BRIDGE_SRC)` in bridge-build-all target               |

### Data-Flow Trace (Level 4)

Not applicable — this phase produces a standalone binary (no web components rendering dynamic data from an API/store). The data flow is config-file → Config struct → ServerManager/tray menu, which is fully verified through unit tests.

### Behavioral Spot-Checks

| Behavior                                              | Command                                                                | Result            | Status  |
|-------------------------------------------------------|------------------------------------------------------------------------|-------------------|---------|
| Binary compiles (Linux, CGO=0)                        | `CGO_ENABLED=0 go build ./cmd/winbox-bridge/`                         | 8.2 MB binary     | PASS    |
| Linux binary builds for amd64 (CGO=0)                 | `CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build ...`                  | 8.2 MB at /tmp    | PASS    |
| All tests pass                                        | `go test ./cmd/winbox-bridge/... -count=1`                             | `ok` in 0.062s    | PASS    |
| Config round-trip (14 config tests)                   | `go test ./cmd/winbox-bridge/... -run TestConfig`                      | All PASS          | PASS    |
| ServerManager lifecycle (8 tests)                     | `go test ./cmd/winbox-bridge/... -run TestServerManager`               | All PASS          | PASS    |
| No hardcoded `"localhost:1337"` in security logic     | `grep "localhost:1337" main.go server.go tray.go`                      | Only in comment   | PASS    |
| systray.Run not in goroutine                          | `grep -B5 "systray.Run(" main.go \| grep -c "go func"`                 | 0                 | PASS    |
| Tray icon appears on desktop launch                   | Requires running `./winbox-bridge` on a desktop                        | —                 | SKIP (human) |

### Requirements Coverage

| Requirement | Source Plan | Description                                                                            | Status      | Evidence                                                                                   |
|-------------|-------------|----------------------------------------------------------------------------------------|-------------|--------------------------------------------------------------------------------------------|
| TRAY-01     | 29-02, 29-03 | Bridge binary shows a system tray icon when launched on a desktop system              | HUMAN NEEDED | `setupTray()` + `systray.Run()` wired; icon embedded; needs desktop verification           |
| TRAY-02     | 29-01, 29-02 | User can start/stop bridge HTTP server from system tray without restarting binary     | HUMAN NEEDED | `ServerManager.Start/Stop` wired to menu items; 8 lifecycle tests pass; needs tray UX verification |
| TRAY-03     | 29-01        | User can configure WinBox executable path via bridge config file, accessible from tray | SATISFIED  | `Config.WinBoxPath` field; `Open Config File` menu item writes config.json for user to edit |
| TRAY-04     | 29-01        | User can configure bridge listening port via bridge config file, accessible from tray  | SATISFIED  | `Config.ListenPort` field; JSON-persisted; config file editable via tray menu              |
| TRAY-05     | 29-01        | User can configure allowed Theia origin via bridge config file, accessible from tray  | SATISFIED  | `Config.TheiaOrigin` field; JSON-persisted; config file editable via tray menu             |
| TRAY-06     | 29-01        | Bridge supports `--no-tray` headless mode for servers without a display                | SATISFIED  | `flag.Bool("no-tray", ...)` + `runHeadless()` with `signal.Notify(SIGINT/SIGTERM)`         |

Note: TRAY-01 and TRAY-02 are marked HUMAN NEEDED because while the implementation is complete and compiles, the observable behavior (tray icon appearing, menu responding to clicks) can only be confirmed on a live desktop environment.

### Anti-Patterns Found

No anti-patterns found. Scan of `config.go`, `server.go`, `tray.go`, and `main.go` returned zero matches for TODO/FIXME/PLACEHOLDER/stub patterns. No empty return values that flow to rendering. No `console.log`-only handlers.

### Human Verification Required

#### 1. System Tray Icon Appears

**Test:** Build the binary (`go build -o winbox-bridge ./cmd/winbox-bridge/`) and run it: `./winbox-bridge`
**Expected:** A system tray icon (22x22 blue square on Linux/macOS, custom ICO on Windows) appears in the notification area within 2 seconds. Right-clicking shows: "Status: Running on :1337", "Start Server" (disabled), "Stop Server" (enabled), separator, "Open Config File", separator, "Quit"
**Why human:** `systray.Run()` blocks the main goroutine and requires a display server (X11/Wayland/Cocoa/Win32); cannot be automated without a graphical environment

#### 2. Start/Stop Toggle and Status Update

**Test:** With the tray running, click "Stop Server". Then click "Start Server".
**Expected:** After Stop: status shows "Stopped", Stop is disabled, Start is enabled. After Start: status shows "Running on :1337", Start is disabled, Stop is enabled. Bridge responds to `curl http://localhost:1337/health` only when running.
**Why human:** Interactive tray menu click events require a desktop session

#### 3. Config Port Change Takes Effect

**Test:** Click "Open Config File", edit `listen_port` to `1338`, save. Click "Stop Server", then "Start Server".
**Expected:** Status changes to "Running on :1338". `curl http://localhost:1338/health` returns `{"ok":true}`. `:1337` no longer responds.
**Why human:** End-to-end config reload + server restart flow requires manual interaction

#### 4. Open Config File Opens Editor

**Test:** Click "Open Config File" from tray menu.
**Expected:** config.json opens in the OS default text editor (xdg-open on Linux, Notepad on Windows, `open` on macOS)
**Why human:** Requires a running desktop with default application associations

#### 5. Quit Exits Cleanly

**Test:** Click "Quit" from tray menu.
**Expected:** Bridge process exits; `pgrep winbox-bridge` returns empty; no zombie processes
**Why human:** Requires interactive tray interaction

#### 6. Headless Mode (--no-tray)

**Test:** Run `./winbox-bridge --no-tray` in a terminal. Then send SIGINT (Ctrl+C).
**Expected:** Server starts on :1337. `curl -H "Origin: http://localhost:3000" -H "Host: localhost:1337" http://localhost:1337/health` returns `{"ok":true}`. Ctrl+C logs "received signal interrupt, shutting down..." and exits with code 0.
**Why human:** Terminal interaction required to send SIGINT and observe shutdown log output

---

### Gaps Summary

No gaps blocking goal achievement. All programmatically verifiable aspects of the phase are confirmed:

- Config JSON persistence with correct paths, permissions, defaults, and round-trip behavior
- ServerManager lifecycle (Start/Stop/Running/Port) with mutex protection and race-free goroutine capture
- Dynamic host header validation (no hardcoded port)
- `--no-tray` headless flag wired to signal-based shutdown
- System tray integration code is complete and compiles (fyne.io/systray v1.12.0, icon embedded, menu wired)
- Windows `-H=windowsgui` ldflags in both Makefile and CI
- macOS CGO split in CI (macos-latest runner, cgo: "1" for darwin targets)
- All 6 CI matrix targets present with correct runner and flag configuration
- 51 tests total pass (14 config + 8 server + 29 main/handler)

The only outstanding items are interactive desktop verifications of the tray UX (TRAY-01, TRAY-02), which cannot be automated without a display server.

---

_Verified: 2026-04-09T13:38:00Z_
_Verifier: Claude (gsd-verifier)_
