# Phase 29: WinBox Bridge System Tray — Research

**Researched:** 2026-04-08
**Domain:** Go system tray, HTTP server lifecycle management, cross-platform config storage
**Confidence:** HIGH

---

## Summary

Phase 29 adds a system tray icon to the WinBox bridge binary (`cmd/winbox-bridge/main.go`) so users can configure the bridge (WinBox path, listen port, allowed origin) and start/stop the HTTP server without restarting the process.

The dominant concern discovered in research is **CGO**. The current bridge is `CGO_ENABLED=0` and cross-compiled from a single Linux runner in CI for all 6 targets. Adding a system tray library breaks this:

- `fyne.io/systray` v1.12.0 (the canonical library) requires CGO only on **macOS** (Objective-C/Cocoa). Windows and Linux implementations are CGO-free (Windows uses `golang.org/x/sys/windows`; Linux uses `godbus/dbus/v5` via pure Go DBus).
- This means a **split build strategy is mandatory**: Windows and Linux continue to cross-compile with `CGO_ENABLED=0` from Linux; macOS binaries must be natively compiled on `macos-latest` GitHub Actions runners where Xcode/clang is pre-installed.

The config storage solution is straightforward: a small JSON file in `os.UserConfigDir()` (maps to `%APPDATA%\winbox-bridge\config.json` on Windows, `~/.config/winbox-bridge/config.json` on Linux, `~/Library/Application Support/winbox-bridge/config.json` on macOS) with no external dependencies — just `encoding/json` + `os`.

The server start/stop lifecycle uses `http.Server.Shutdown()` (introduced Go 1.8) on a goroutine-managed `*http.Server` pointer with a mutex, replacing the current single-start pattern. When the user changes config and starts the server, a new `*http.Server` is constructed, and the old one is gracefully shut down first.

**Primary recommendation:** Use `fyne.io/systray` v1.12.0. Restructure the bridge into: a `Config` struct with JSON persistence, a `ServerManager` struct that owns start/stop, and a `setupTray()` function that wires menu items to server lifecycle. CI gains two additional native jobs (macOS amd64 + arm64). Windows/Linux CI jobs drop `CGO_ENABLED: 0` and set it to `0` explicitly per-job (unchanged since Windows/Linux are CGO-free).

---

## Project Constraints (from CLAUDE.md)

- **Go 1.24** — CGO is available; `os.UserConfigDir()` has been available since Go 1.13
- **CGO constraint for main theia binary**: requires CGO (sqlite3), but `cmd/winbox-bridge/` is currently `CGO_ENABLED=0`
- **No external web framework**: bridge uses stdlib `net/http`; this does not change
- **Naming conventions**: Go files `snake_case.go`, exported types `PascalCase`, constructor functions `New*`
- **Error handling**: `(value, error)` pairs, log errors and continue in background goroutines
- **Logging**: stdlib `log` package only — no structured logging
- **Comments**: GoDoc on all exported types and functions
- **Module**: `github.com/lollinoo/theia` in `go.mod`
- **Build**: `bridge-build-all` Makefile target and `build-bridge` CI job must be updated

---

## Standard Stack

### Core

| Library | Version | Purpose | Why Standard |
|---------|---------|---------|--------------|
| `fyne.io/systray` | v1.12.0 | System tray icon + menu | Cross-platform, actively maintained, Windows/Linux CGO-free, only standard library for this in Go ecosystem |
| `github.com/godbus/dbus/v5` | v5.1.0 (indirect, via systray) | Linux DBus (auto-pulled by systray) | Required by fyne.io/systray on Linux; pure Go |
| `golang.org/x/sys` | v0.40.0 (already in go.mod) | Windows API access (via systray) | Already a dependency; no version bump needed |
| `encoding/json` (stdlib) | Go 1.24 | Config file serialization | Zero deps, sufficient for a flat config struct |
| `os` stdlib | Go 1.24 | `os.UserConfigDir()` for config path, file I/O | Standard cross-platform config dir lookup |

[VERIFIED: pkg.go.dev/fyne.io/systray — version v1.12.0, December 23, 2025]
[VERIFIED: GitHub fyne-io/systray go.mod — godbus/dbus v5.1.0, golang.org/x/sys v0.15.0]
[VERIFIED: project go.mod — golang.org/x/sys v0.40.0 already present]

### Supporting

| Library | Version | Purpose | When to Use |
|---------|---------|---------|-------------|
| `golang.org/x/sys/windows` | already in go.mod (indirect) | Windows API for systray Windows impl | Auto-used by fyne.io/systray on Windows |

### Alternatives Considered

| Instead of | Could Use | Tradeoff |
|------------|-----------|----------|
| `fyne.io/systray` | `getlantern/systray` | getlantern requires CGO on Linux too (GTK3 + libayatana-appindicator3); fyne.io/systray uses pure Go DBus on Linux — better for this project's constraints |
| `fyne.io/systray` | `energye/systray` | Less maintained, similar CGO profile |
| `fyne.io/systray` | Roll native per-platform code | Massive scope, brittle |
| JSON config file | SQLite settings | Overkill; bridge has no SQLite dependency and must remain standalone |
| JSON config file | TOML/YAML | Would need a parsing dependency (bridge is standalone, no gopkg.in/yaml.v3 in the bridge cmd) |
| `os.UserConfigDir()` | Hardcoded `~/.winbox-bridge/` | Less correct; UserConfigDir is platform-appropriate |

**Installation (adds to go.mod):**
```bash
go get fyne.io/systray@v1.12.0
```

Note: `golang.org/x/sys` and `golang.org/x/sys/windows` are already in go.sum. `godbus/dbus/v5` will be added as an indirect dependency.

**Version verification:** [VERIFIED: npm registry equivalent — pkg.go.dev shows v1.12.0 published Dec 23, 2025]

---

## Architecture Patterns

### Recommended File Structure Change

```
cmd/winbox-bridge/
├── main.go            # main() — wires everything, starts systray.Run()
├── main_test.go       # existing tests (HTTP handler tests unchanged)
├── config.go          # Config struct, loadConfig(), saveConfig(), configFilePath()
├── config_test.go     # config round-trip tests
├── server.go          # ServerManager struct — start/stop lifecycle
├── server_test.go     # ServerManager lifecycle tests
└── tray.go            # setupTray() — builds menu, wires channels to ServerManager
```

The existing `main.go` is split because it already exceeds a comfortable line count for adding tray + config code in one file.

### Pattern 1: Config Struct with JSON File

**What:** Flat struct serialized to/from JSON in `os.UserConfigDir()`
**When to use:** Always — this is the single source of truth for persistent config

```go
// Source: stdlib encoding/json + os.UserConfigDir() [VERIFIED: pkg.go.dev/os]
type Config struct {
    WinBoxPath  string `json:"winbox_path"`
    ListenPort  int    `json:"listen_port"`
    TheiaOrigin string `json:"theia_origin"`
}

// DefaultConfig returns the config matching current CLI flag defaults.
func DefaultConfig() Config {
    return Config{
        WinBoxPath:  "",               // empty = auto-discover
        ListenPort:  1337,
        TheiaOrigin: "http://localhost:3000",
    }
}

func configFilePath() (string, error) {
    dir, err := os.UserConfigDir()
    if err != nil {
        return "", fmt.Errorf("config dir: %w", err)
    }
    return filepath.Join(dir, "winbox-bridge", "config.json"), nil
}
```

`os.UserConfigDir()` platform paths [VERIFIED: pkg.go.dev/os and golang/go#29960]:
- Windows: `%APPDATA%\winbox-bridge\config.json`
- Linux: `~/.config/winbox-bridge/config.json`
- macOS: `~/Library/Application Support/winbox-bridge/config.json`

### Pattern 2: ServerManager — Start/Stop Lifecycle

**What:** A struct owning a `*http.Server` pointer and a mutex. `Start()` creates a new server + listener goroutine; `Stop()` calls `server.Shutdown()`.
**When to use:** Whenever a menu item click needs to start or stop the HTTP server

```go
// Source: net/http stdlib [VERIFIED: pkg.go.dev/net/http]
type ServerManager struct {
    mu     sync.Mutex
    server *http.Server
}

// Start creates and runs a new HTTP server with the given config.
// It is a no-op if the server is already running.
func (m *ServerManager) Start(cfg Config, winboxPath string) error {
    m.mu.Lock()
    defer m.mu.Unlock()
    if m.server != nil {
        return nil // already running
    }
    mux := buildMux(winboxPath)
    handler := securityCheck(cfg.TheiaOrigin, mux)
    m.server = &http.Server{
        Addr:    fmt.Sprintf(":%d", cfg.ListenPort),
        Handler: handler,
    }
    go func() {
        if err := m.server.ListenAndServe(); err != http.ErrServerClosed {
            log.Printf("winbox-bridge: server error: %v", err)
        }
    }()
    return nil
}

// Stop gracefully shuts down the server with a 5-second timeout.
func (m *ServerManager) Stop() error {
    m.mu.Lock()
    defer m.mu.Unlock()
    if m.server == nil {
        return nil
    }
    ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
    defer cancel()
    err := m.server.Shutdown(ctx)
    m.server = nil
    return err
}

// Running reports whether the server is currently started.
func (m *ServerManager) Running() bool {
    m.mu.Lock()
    defer m.mu.Unlock()
    return m.server != nil
}
```

### Pattern 3: Systray Setup — Menu Wired to ServerManager

**What:** `systray.Run(onReady, onExit)` blocks the main goroutine (required by the systray event loop). All other logic runs from goroutines spawned inside `onReady`.

```go
// Source: fyne.io/systray v1.12.0 [VERIFIED: pkg.go.dev/fyne.io/systray]
func setupTray(mgr *ServerManager, cfg Config) {
    systray.SetIcon(iconBytes)  // embed .ico for Windows, .png for others
    systray.SetTooltip("WinBox Bridge")

    mStart  := systray.AddMenuItem("Start Server", "Start the WinBox bridge HTTP server")
    mStop   := systray.AddMenuItem("Stop Server", "Stop the WinBox bridge HTTP server")
    mStatus := systray.AddMenuItem("Status: Stopped", "Current server status")
    mStatus.Disable()
    systray.AddSeparator()
    mQuit := systray.AddMenuItem("Quit", "Exit WinBox Bridge")

    // Initial state
    updateMenuState(mgr, mStart, mStop, mStatus)

    go func() {
        for {
            select {
            case <-mStart.ClickedCh:
                // re-discover winbox path on each start (config may have changed)
                winboxPath := discoverWinBox(cfg.WinBoxPath)
                if err := mgr.Start(cfg, winboxPath); err != nil {
                    log.Printf("winbox-bridge: start error: %v", err)
                }
                updateMenuState(mgr, mStart, mStop, mStatus)
            case <-mStop.ClickedCh:
                if err := mgr.Stop(); err != nil {
                    log.Printf("winbox-bridge: stop error: %v", err)
                }
                updateMenuState(mgr, mStart, mStop, mStatus)
            case <-mQuit.ClickedCh:
                mgr.Stop()
                systray.Quit()
                return
            }
        }
    }()
}
```

**Key constraint:** `systray.Run()` MUST be called from `main()`, and it MUST block. All other goroutines start inside `onReady`. [VERIFIED: fyne.io/systray docs]

### Pattern 4: Config Edit — "Open Config File" Menu Item

**What:** A menu item that opens the config file in the OS default editor. Uses `exec.Command` with OS-appropriate opener.
**When to use:** Simple config editing without a native dialog (no CGO dialog dependency)

```go
// platform-appropriate file open
func openConfigFile(path string) error {
    var cmd *exec.Cmd
    switch runtime.GOOS {
    case "windows":
        cmd = exec.Command("cmd", "/c", "start", path) //nolint:gosec
    case "darwin":
        cmd = exec.Command("open", path)
    default:
        cmd = exec.Command("xdg-open", path)
    }
    return cmd.Start()
}
```

This avoids any native dialog library dependency. The config file is opened in whatever the user's default `.json` editor is. On changes, the user must restart the server via the tray menu.

### Anti-Patterns to Avoid

- **`systray.Run()` not on main goroutine:** The systray event loop must own the main thread on macOS (Cocoa requirement). Spawning it in a goroutine will cause crashes on macOS. [VERIFIED: fyne.io/systray README]
- **No `windowsgui` ldflags on Windows:** Without `-H=windowsgui`, a console window opens alongside the tray app on Windows. [VERIFIED: fyne.io/systray docs]
- **Config mutation without server restart:** If the user edits `listen_port` or `theia_origin`, the running server is NOT automatically updated. Reload requires Stop + Start.
- **Calling `http.Server.ListenAndServe` on the main goroutine:** It blocks. Always run in a separate goroutine.
- **Ignoring `http.ErrServerClosed`:** After `Shutdown()` is called, `ListenAndServe` returns `http.ErrServerClosed`; treat this as success, not an error.
- **No mutex on ServerManager:** `Start()` and `Stop()` can be called concurrently from the event goroutine. A mutex is required. [ASSUMED] — concurrent tray click races are theoretically possible.

---

## Don't Hand-Roll

| Problem | Don't Build | Use Instead | Why |
|---------|-------------|-------------|-----|
| Cross-platform tray icon | Custom Win32 API calls, NSStatusBar bindings, DBus StatusNotifierItem | `fyne.io/systray` | Each platform has 200-500 lines of platform-specific code; icon format handling, menu update races, event loop integration — all solved |
| Config dir paths | Hardcoded `~/.winbox-bridge` | `os.UserConfigDir()` | Platform-correct (follows XDG on Linux, %APPDATA% on Windows, Library/AS on macOS) |
| Browser opening | `exec.Command("xdg-open", ...)` per-platform | `github.com/pkg/browser` | Handles edge cases (WSL, Snap, GNOME, KDE) — BUT only needed if a "Open Config in Browser" approach is chosen over file-editing. For this phase, file-editing is simpler. |
| HTTP server lifecycle | Ad-hoc process kill/restart | `http.Server.Shutdown()` | Handles in-flight requests, drains connections gracefully |

**Key insight:** System tray integration has significant platform divergence. The 82.8% Go / 15.4% Objective-C split in fyne.io/systray shows exactly how much native code is needed — don't replicate that.

---

## CGO and Build Strategy — Critical Finding

This is the most important architectural constraint for this phase.

### Per-Platform CGO Requirements for fyne.io/systray

| Platform | CGO Required | Why | Verified |
|----------|-------------|-----|---------|
| Windows | No | `systray_windows.go` uses `golang.org/x/sys/windows` (pure Go) | [VERIFIED: GitHub fyne-io/systray blob/master/systray_windows.go — no `import "C"`] |
| Linux | No | `systray_unix.go` uses `godbus/dbus/v5` (pure Go) | [VERIFIED: GitHub fyne-io/systray blob/master/systray_unix.go — no `import "C"`] |
| macOS | **YES** | `systray_darwin.go` + `systray_darwin.m` use Objective-C/Cocoa | [VERIFIED: GitHub fyne-io/systray blob/master/systray_darwin.go — `import "C"`, `#cgo darwin CFLAGS: -x objective-c -fobjc-arc`] |

### Required CI Changes

The current `build-bridge` CI job runs on `ubuntu-latest` with `CGO_ENABLED: 0` for all 6 targets. This must split into:

| Target | Runner | CGO | Status |
|--------|--------|-----|--------|
| windows/amd64 | ubuntu-latest | 0 | Unchanged |
| windows/arm64 | ubuntu-latest | 0 | Unchanged |
| linux/amd64 | ubuntu-latest | 0 | Unchanged |
| linux/arm64 | ubuntu-latest | 0 | Unchanged |
| darwin/amd64 | **macos-latest** | **1** | NEW runner |
| darwin/arm64 | **macos-latest** | **1** | NEW runner |

`macos-latest` GitHub Actions runners have Xcode command-line tools (clang) pre-installed. No additional setup step required for CGO on macOS. [VERIFIED: GitHub Actions docs — Xcode CLT pre-installed; CITED: community/discussions#46166]

### macOS Build Flags

macOS systray requires an application bundle or explicit compile flag:
```bash
GOOS=darwin GOARCH=amd64 CGO_ENABLED=1 go build -ldflags="-s -w" ./cmd/winbox-bridge/
```

macOS does NOT require `-H=windowsgui` (that is Windows-only). macOS tray apps do not need a formal `.app` bundle for CLI-invoked binaries when distributed as standalone. [ASSUMED — need user confirmation if notarization is required]

### Windows Build Flag

Windows tray binary must suppress the console window:
```bash
GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build -ldflags="-s -w -H=windowsgui" ./cmd/winbox-bridge/
```

Without `-H=windowsgui`, a blank console window opens every time the user launches the bridge. [VERIFIED: fyne.io/systray docs]

### Makefile Update Required

`bridge-build-all` Makefile target currently uses a loop with `CGO_ENABLED=0` for all targets. After this phase:
- Windows and Linux: `CGO_ENABLED=0`, Windows gets `-H=windowsgui` ldflags
- macOS: Requires native Mac to build locally (cannot cross-compile from Linux with CGO=1 for macOS without a full macOS SDK). For local development, maintainer builds macOS binary on their Mac. CI handles it.

---

## Common Pitfalls

### Pitfall 1: `systray.Run()` Not on Main Thread
**What goes wrong:** macOS Cocoa requires the event loop on the main OS thread. Running `systray.Run()` in a goroutine causes a crash or silent failure on macOS.
**Why it happens:** Go's goroutine scheduler doesn't guarantee thread identity. `runtime.LockOSThread()` would be needed, but systray handles this internally when called from main.
**How to avoid:** Call `systray.Run(onReady, onExit)` as the last statement in `main()`. All HTTP server startup logic goes inside `onReady`.
**Warning signs:** App works on Linux/Windows dev machines but crashes silently on macOS.

### Pitfall 2: Config Not Reloaded After File Edit
**What goes wrong:** User edits the JSON config file, but the running server still uses the old port/origin. Requests go to wrong port or get blocked.
**Why it happens:** The config is loaded at startup and cached in memory. File changes are not watched.
**How to avoid:** Require explicit "Reload" or "Restart Server" action from tray menu to apply new config. Display current config values in tooltip or disabled menu items.
**Warning signs:** Bridge works but WinBox can't connect after config change.

### Pitfall 3: `http.ErrServerClosed` Treated as Fatal Error
**What goes wrong:** After `Stop()` is called, `ListenAndServe()` returns `http.ErrServerClosed`. If this error is logged as fatal or checked without the `!= http.ErrServerClosed` guard, the program panics/exits.
**Why it happens:** `http.ErrServerClosed` is a sentinel, not a real error.
**How to avoid:** `if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed { log.Printf(...) }`.
**Warning signs:** Bridge quits immediately after clicking "Stop Server".

### Pitfall 4: Missing `-H=windowsgui` on Windows
**What goes wrong:** A console terminal window opens behind the system tray icon on Windows. Users see a blank black window on startup.
**Why it happens:** Go programs default to the console subsystem on Windows.
**How to avoid:** Add `-H=windowsgui` to ldflags for Windows builds only.
**Warning signs:** During testing on Windows, a console window appears.

### Pitfall 5: Port Already In Use on Re-Start
**What goes wrong:** After `Stop()` → `Start()`, `ListenAndServe` returns "address already in use".
**Why it happens:** TCP sockets in TIME_WAIT state briefly hold the port after `Shutdown()`.
**How to avoid:** Use `net.Listen` with `SO_REUSEADDR` or add a short delay; alternatively, document this as expected and surface the error in the tray tooltip. The simpler fix is `server.SetKeepAlivesEnabled(false)` before `Shutdown()`. [ASSUMED — TIME_WAIT behavior is standard TCP; exact mitigation approach needs confirmation]
**Warning signs:** Second "Start Server" click immediately shows error.

### Pitfall 6: Config File Not Created on First Run
**What goes wrong:** `loadConfig()` fails because the file doesn't exist yet, returning an error instead of defaults.
**Why it happens:** First-time user has no config file.
**How to avoid:** `loadConfig()` must treat `os.IsNotExist(err)` as "return DefaultConfig(), nil", not an error.
**Warning signs:** Bridge fails to start on a clean system.

### Pitfall 7: `arm64` macOS Binary Cross-Compiled from `amd64`
**What goes wrong:** On macOS, `CGO_ENABLED=1 GOARCH=arm64 GOOS=darwin go build` from an `amd64` mac runner fails because the C compiler produces amd64 object files by default.
**Why it happens:** Apple's clang on x86 Mac can produce arm64 code with `-arch arm64`, but Go's CGO doesn't automatically set this.
**How to avoid:** Use `macos-latest` runner (which is ARM since mid-2024) or use a matrix with the correct runner architecture. GitHub's `macos-latest` has been ARM (M-series) since 2024. For `darwin/amd64`, the macOS runner should still work because Apple clang supports cross-arch. [CITED: github.com/actions/runner-images — macos-latest is now macos-14 ARM]
**Warning signs:** CGO compilation succeeds but binary crashes at runtime on macOS.

---

## Code Examples

### Config Load/Save (No External Deps)

```go
// Source: stdlib encoding/json, os [VERIFIED]
func loadConfig() (Config, error) {
    path, err := configFilePath()
    if err != nil {
        return DefaultConfig(), nil // degrade gracefully
    }
    data, err := os.ReadFile(path)
    if err != nil {
        if os.IsNotExist(err) {
            return DefaultConfig(), nil // first run
        }
        return DefaultConfig(), fmt.Errorf("read config: %w", err)
    }
    var cfg Config
    if err := json.Unmarshal(data, &cfg); err != nil {
        return DefaultConfig(), fmt.Errorf("parse config: %w", err)
    }
    return cfg, nil
}

func saveConfig(cfg Config) error {
    path, err := configFilePath()
    if err != nil {
        return err
    }
    if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
        return fmt.Errorf("create config dir: %w", err)
    }
    data, err := json.MarshalIndent(cfg, "", "  ")
    if err != nil {
        return err
    }
    return os.WriteFile(path, data, 0o600)
}
```

### Main Entrypoint After Restructure

```go
// Source: fyne.io/systray v1.12.0 Run API [VERIFIED: pkg.go.dev/fyne.io/systray]
func main() {
    // Parse CLI flags (still supported for headless/scripted use)
    // ...
    
    cfg, err := loadConfig()
    if err != nil {
        log.Printf("winbox-bridge: config load error: %v (using defaults)", err)
    }
    // CLI flags override config file (backward compat)
    applyFlagOverrides(&cfg, ...)

    mgr := &ServerManager{}

    // Auto-start if was previously running (or based on config flag)
    if cfg.AutoStart {
        winboxPath := discoverWinBox(cfg.WinBoxPath)
        if err := mgr.Start(cfg, winboxPath); err != nil {
            log.Printf("winbox-bridge: auto-start failed: %v", err)
        }
    }

    // systray.Run MUST be called from main(); it blocks until systray.Quit()
    systray.Run(
        func() { setupTray(mgr, cfg) },
        func() { mgr.Stop() },
    )
}
```

### Minimal Tray Icon (Embedded PNG)

```go
// icon can be embedded at build time
//go:embed icon.png
var iconBytes []byte

func onReady() {
    systray.SetIcon(iconBytes)
    systray.SetTooltip("WinBox Bridge")
    // ...
}
```

Windows requires `.ico` format for best rendering; other platforms accept `.png`. [VERIFIED: fyne.io/systray SetIcon docs — "ico for Windows and ico/jpg/png for other platforms"]. A build-tag approach can embed different icon files per platform.

---

## State of the Art

| Old Approach | Current Approach | When Changed | Impact |
|--------------|------------------|--------------|--------|
| `getlantern/systray` (GTK3 on Linux, CGO everywhere) | `fyne.io/systray` (DBus on Linux, CGO-free on Win/Linux) | Forked ~2021, stable | Linux builds no longer need GTK3 headers |
| Manual `POSIX signal` for shutdown | `http.Server.Shutdown()` | Go 1.8 (2017) | Graceful in-flight request completion |
| XDG config dirs via third-party lib | `os.UserConfigDir()` | Go 1.13 (2019) | No external dependency needed |
| macOS tray via CGO always required | Still CGO (no pure Go Cocoa equivalent) | Unchanged | macOS CI requires native runner |

**Note on `getlantern/systray` vs `fyne.io/systray`:** `fyne.io/systray` is a fork that removes the GTK dependency on Linux (uses DBus instead). For this project, `fyne.io/systray` is strictly better because Linux builds remain `CGO_ENABLED=0`. `getlantern/systray` v1.2.2 would require GTK3 + libayatana-appindicator3 headers on Linux CI.

---

## Assumptions Log

| # | Claim | Section | Risk if Wrong |
|---|-------|---------|---------------|
| A1 | macOS binaries don't require notarization for this use case (user downloads and runs manually) | CGO Build Strategy | If notarization required, CI needs Apple Developer certificate setup — significant additional work |
| A2 | `macos-latest` GitHub Actions runner has Xcode CLT pre-installed and CGO_ENABLED=1 works without extra steps | CGO Build Strategy / CI | If clang not present, a `- run: xcode-select --install` step may be needed |
| A3 | TIME_WAIT on port re-bind is mitigated by `SetKeepAlivesEnabled(false)` before Shutdown | Pitfall 5 | May need `SO_REUSEADDR` via custom listener instead |
| A4 | "Open Config File" via OS default editor is acceptable UX for configuration (no native dialog) | Architecture | If users want in-tray text input, a native dialog library (sqweek/dialog) or embedded webview would be needed — more complex |
| A5 | `AutoStart` flag in config is optional / out of scope; server does NOT auto-start on bridge launch | Architecture | If auto-start is wanted, config needs an `auto_start` bool and startup logic |

---

## Open Questions (RESOLVED)

1. **macOS notarization** — RESOLVED: Accepted per recommendation. Gatekeeper warning is acceptable for this user base (network operators, trusted internal tool). Document in release notes. Notarization is out of scope for Phase 29.

2. **`darwin/arm64` runner in CI** — RESOLVED: Plan 29-03 uses `macos-latest` (ARM, macOS 14) for both `darwin/amd64` and `darwin/arm64`. Apple clang on ARM runners supports cross-arch compilation via `GOARCH=amd64` without additional flags. Risk accepted: if cross-arch CGO fails in CI for `darwin/amd64`, the fallback is to add `macos-13` (Intel) for that target. This is a known execution risk, not a planning blocker.

3. **Config edit UX: file editor vs. tray submenu** — RESOLVED: "Open Config File" (opens JSON in OS default editor) is the Phase 29 approach. Accepted per recommendation. Full config dialog via embedded webview is deferred to future work (BRIDGE-F01).

4. **`godbus/dbus/v5` availability on headless Linux servers** — RESOLVED: `--no-tray` flag implemented in Plan 29-01 Task 2. Headless servers use `./winbox-bridge --no-tray` to run without a display. D-Bus unavailability is bypassed entirely.

---

## Environment Availability

| Dependency | Required By | Available | Version | Fallback |
|------------|------------|-----------|---------|----------|
| `fyne.io/systray` | Tray icon | Not yet in go.mod | v1.12.0 | — |
| `godbus/dbus/v5` | Linux tray (auto-pulled) | Not yet in go.mod | v5.1.0 | — |
| `golang.org/x/sys` | Windows tray (already present) | ✓ | v0.40.0 | — |
| macOS Xcode CLT | macOS CGO build in CI | ✓ (macos-latest runner) | pre-installed | — |
| Linux GTK3 headers | NOT required (using DBus) | N/A | N/A | — |

**Missing dependencies with no fallback:**
- `fyne.io/systray` — must be added to go.mod

**Missing dependencies with fallback:**
- None — DBus on headless Linux is a runtime concern (Open Question 4), not a build concern

---

## Validation Architecture

### Test Framework

| Property | Value |
|----------|-------|
| Framework | Go standard `testing` package |
| Config file | none (tests run with `go test ./cmd/winbox-bridge/`) |
| Quick run command | `CGO_ENABLED=0 go test ./cmd/winbox-bridge/ -count=1` |
| Full suite command | `CGO_ENABLED=0 go test ./... -count=1` |

Note: `CGO_ENABLED=0` works for tests because the tray code is not exercised in unit tests (systray.Run requires a display). Tests use the same handler/mux pattern as existing tests.

### Phase Requirements → Test Map

| Req ID | Behavior | Test Type | Automated Command | File Exists? |
|--------|----------|-----------|-------------------|-------------|
| CONFIG-01 | `loadConfig()` returns defaults when file absent | unit | `go test ./cmd/winbox-bridge/ -run TestLoadConfig_DefaultsWhenMissing` | ❌ Wave 0 |
| CONFIG-02 | `saveConfig()` + `loadConfig()` round-trip preserves all fields | unit | `go test ./cmd/winbox-bridge/ -run TestConfig_RoundTrip` | ❌ Wave 0 |
| CONFIG-03 | `configFilePath()` returns platform-correct path | unit | `go test ./cmd/winbox-bridge/ -run TestConfigFilePath` | ❌ Wave 0 |
| SERVER-01 | `ServerManager.Start()` starts server (port responds) | integration | `go test ./cmd/winbox-bridge/ -run TestServerManager_Start` | ❌ Wave 0 |
| SERVER-02 | `ServerManager.Stop()` shuts down gracefully | integration | `go test ./cmd/winbox-bridge/ -run TestServerManager_Stop` | ❌ Wave 0 |
| SERVER-03 | `ServerManager.Running()` reflects state correctly | unit | `go test ./cmd/winbox-bridge/ -run TestServerManager_Running` | ❌ Wave 0 |
| SECURITY-01 | Origin + Host validation unchanged after refactor | unit | `go test ./cmd/winbox-bridge/ -run TestOriginValidation` | ✅ (existing) |

### Sampling Rate

- **Per task commit:** `CGO_ENABLED=0 go test ./cmd/winbox-bridge/ -count=1`
- **Per wave merge:** `CGO_ENABLED=0 go test ./... -count=1`
- **Phase gate:** Full suite green before `/gsd-verify-work`

### Wave 0 Gaps

- [ ] `cmd/winbox-bridge/config_test.go` — covers CONFIG-01, CONFIG-02, CONFIG-03
- [ ] `cmd/winbox-bridge/server_test.go` — covers SERVER-01, SERVER-02, SERVER-03

*(Existing `main_test.go` HTTP handler tests remain unchanged and already cover SECURITY-01)*

---

## Security Domain

### Applicable ASVS Categories

| ASVS Category | Applies | Standard Control |
|---------------|---------|-----------------|
| V2 Authentication | no | Bridge has no user auth |
| V3 Session Management | no | Stateless HTTP |
| V4 Access Control | yes | Origin + Host header validation — **already implemented in `securityCheck`** |
| V5 Input Validation | yes | Config values: port range (1-65535), origin URL format, path existence check |
| V6 Cryptography | no | Config is not encrypted (no credentials stored in bridge config) |

### Known Threat Patterns for This Stack

| Pattern | STRIDE | Standard Mitigation |
|---------|--------|---------------------|
| DNS rebinding via Host header spoof | Spoofing | Already mitigated: `securityCheck` validates `Host: localhost:1337` — unchanged by this phase |
| Arbitrary config file path traversal | Tampering | `configFilePath()` uses `os.UserConfigDir()` — no user-controlled path component |
| Port hijacking (another process on same port) | Denial of Service | Not mitigated; documented limitation (localhost-only bridge assumption) |
| Config file world-readable (leaks origin) | Info disclosure | `os.WriteFile(path, data, 0o600)` — user-only read/write permissions |

---

## Sources

### Primary (HIGH confidence)
- `pkg.go.dev/fyne.io/systray` — API reference, version v1.12.0, CGO requirement statement
- `github.com/fyne-io/systray` blob/master/systray_windows.go — confirmed no CGO on Windows
- `github.com/fyne-io/systray` blob/master/systray_unix.go — confirmed no CGO on Linux (DBus only)
- `github.com/fyne-io/systray` blob/master/systray_darwin.go — confirmed CGO + Objective-C required on macOS
- `raw.githubusercontent.com/fyne-io/systray/master/go.mod` — godbus/dbus v5.1.0, golang.org/x/sys v0.15.0
- `pkg.go.dev/os#UserConfigDir` — platform-specific config dir paths
- `pkg.go.dev/net/http#Server.Shutdown` — graceful shutdown API
- `github.com/lollinoo/theia/go.mod` — existing dependencies (golang.org/x/sys v0.40.0 already present)
- `github.com/lollinoo/theia/.github/workflows/ci.yml` — current bridge CI uses CGO_ENABLED: 0 from ubuntu-latest

### Secondary (MEDIUM confidence)
- `github.com/orgs/community/discussions/46166` — GitHub Actions CGO multi-platform build discussion confirming macos-latest works with CGO_ENABLED=1
- `fyne-io/systray` README — Linux uses DBus, not GTK3

### Tertiary (LOW confidence)
- macOS-latest = ARM/M-series since mid-2024 (actions/runner-images — not directly verified in this session)

---

## Metadata

**Confidence breakdown:**
- Standard stack: HIGH — verified against pkg.go.dev and GitHub source
- CGO split strategy: HIGH — verified by reading actual source files of fyne.io/systray per platform
- Architecture: HIGH — follows existing bridge patterns
- CI changes: HIGH for Windows/Linux; MEDIUM for macOS (Open Question 2 about runner arch)
- Pitfalls: HIGH for systray main-thread and ErrServerClosed; MEDIUM for TIME_WAIT, headless Linux

**Research date:** 2026-04-08
**Valid until:** 2026-10-08 (fyne.io/systray is stable; CI runner changes could invalidate CI section sooner)
