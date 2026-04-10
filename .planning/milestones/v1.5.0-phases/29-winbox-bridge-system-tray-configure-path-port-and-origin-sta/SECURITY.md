# SECURITY.md — Phase 29: WinBox Bridge System Tray

**Phase:** 29 — winbox-bridge-system-tray-configure-path-port-and-origin-sta  
**ASVS Level:** L1  
**Audit Date:** 2026-04-09  
**block_on:** open  

---

## Threat Verification Summary

**Closed:** 8/9 | **Open:** 1/9

---

## Mitigated Threats

| Threat ID | Category | Disposition | Evidence |
|-----------|----------|-------------|----------|
| T-29-01 (partial) | Tampering | mitigate | `config.go:83` `os.MkdirAll(..., 0o700)`; `config.go:90` `os.WriteFile(..., 0o600)` — file/directory permissions CLOSED. Port range validation OPEN (see Open Threats). |
| T-29-02 | Denial of Service | mitigate | `server.go:16` `sync.Mutex` field; `server.go:26-29` Start() no-op if `m.server != nil`; `server.go:53-56` Stop() no-op if `m.server == nil`. All Start/Stop/Running/Port methods lock `m.mu` — CLOSED. |
| T-29-04 | Spoofing | mitigate | `server.go:32` `expectedHost := fmt.Sprintf("localhost:%d", cfg.ListenPort)` passed to `buildMux`; `main.go:191` `securityCheck` signature `(allowedOrigin, expectedHost string, next)` — no hardcoded `"localhost:1337"` in security check path — CLOSED. |
| T-29-06 | Denial of Service | mitigate | `tray.go:24` `mStatus.Disable()`; `tray.go:28` `mStop.Disable()` on init; `tray.go:49,54` `mStart.Disable()`/`mStop.Enable()` and `mStart.Enable()`/`mStop.Disable()` called inside `updateState()` after every Start/Stop action — CLOSED. |
| T-29-09 | Tampering | mitigate | `ci.yml:266` `actions/checkout@v4`; `ci.yml:268` `actions/setup-go@v5`; `ci.yml:282` `softprops/action-gh-release@v2`; `ci.yml:286` `GITHUB_TOKEN: ${{ secrets.GITHUB_TOKEN }}` (auto-rotated, repo-scoped). Build from checked-out source only — CLOSED. |
| T-29-10 | Information Disclosure | mitigate | All 6 CI matrix entries (`ci.yml:233,239,245,251,257,263`) contain `ldflags: "-s -w"`. `Makefile:165` uses `-ldflags="-s -w ${ldextra}"` for Windows/Linux local builds. Symbol table and debug info stripped from all binaries — CLOSED. |

---

## Open Threats

| Threat ID | Category | Mitigation Expected | Files Searched | Gap |
|-----------|----------|---------------------|----------------|-----|
| T-29-01 (port validation) | Tampering | Port range 1–65535 validation in `ServerManager.Start()` | `cmd/winbox-bridge/server.go`, `cmd/winbox-bridge/config.go` | No port range check found anywhere in the winbox-bridge package. A config file with `listen_port: 0` or `listen_port: 99999` will attempt to bind without rejection. |

**Action required:** Implement port range validation in `ServerManager.Start()` before `ListenAndServe`, e.g.:

```go
if cfg.ListenPort < 1 || cfg.ListenPort > 65535 {
    return fmt.Errorf("invalid port %d: must be 1-65535", cfg.ListenPort)
}
```

---

## Accepted Risks Log

The following threats were reviewed and accepted. They require no code mitigation. This log satisfies the `accept` disposition for each entry.

| Threat ID | Category | Component | Rationale | Accepted By |
|-----------|----------|-----------|-----------|-------------|
| T-29-03 | Information Disclosure | config.go | Config file contains only WinBox executable path, listen port, Theia origin URL, and log level. No credentials, tokens, or secrets are stored in the config file. `BridgeSecret` is stored in config but is a randomly generated shared key — its disclosure only affects the single-host local bridge, not external systems. Risk accepted. | Phase 29 threat model |
| T-29-05 | Tampering | tray.go (config reload) | Config file is owned by the same OS user running the bridge process. Any attacker with write access to the config file already has full local user access, which grants control over the process itself. Accepting tampered config values is equivalent risk to the attacker directly killing/relaunching the process. Risk accepted. | Phase 29 threat model |
| T-29-07 | Elevation of Privilege | tray.go (openFileInEditor) | `openFileInEditor` receives a path from `configFilePath()`, which derives its value solely from `os.UserConfigDir()` — not from any user-controlled input or network data. The exec.Command uses OS-standard file openers (`notepad.exe`, `open`, `xdg-open`) with a single non-shell-expanded argument. No privilege escalation path identified. Risk accepted. | Phase 29 threat model |
| T-29-08 | Spoofing | icon.go (tray icon) | Tray icon spoofing (another app impersonating the bridge icon) is an OS-level concern not mitigatable at the application layer. The bridge is a local-only tool with no sensitive data exposed via its tray icon. Risk accepted. | Phase 29 threat model |
| T-29-11 | Elevation of Privilege | Windows -H=windowsgui | The `-H=windowsgui` linker flag instructs the Windows PE loader to use the GUI subsystem, suppressing the console window. This is the standard pattern for tray/GUI applications on Windows. It does not change the process's privilege level, token, or access rights. Risk accepted. | Phase 29 threat model |
| T-29-12 | Tampering | macOS CGO supply chain | The `macos-latest` GitHub Actions runner provides Apple-supplied Xcode/clang. CGO is required for `fyne.io/systray` on macOS (Objective-C Cocoa binding). The risk from the C toolchain supply chain is equivalent to any other CGO build and is mitigated by GitHub's management of runner images. Risk accepted. | Phase 29 threat model |

---

## Unregistered Threat Flags

No unregistered threat flags were raised in any SUMMARY.md `## Threat Flags` section for plans 29-01, 29-02, or 29-03. The 29-03 "Threat Surface Scan" section confirmed build-tooling-only changes with no new network endpoints, auth paths, file access patterns, or schema changes.

---

## Notes

- T-29-01 is split: the **file/directory permissions** sub-item is CLOSED; the **port range validation** sub-item is OPEN.
- `block_on: open` is in effect. The open T-29-01 port validation gap must be resolved before this phase is considered fully secured.
- Port range validation is a low-complexity fix (2–3 lines in `server.go:Start()`). It does not require architecture changes.
