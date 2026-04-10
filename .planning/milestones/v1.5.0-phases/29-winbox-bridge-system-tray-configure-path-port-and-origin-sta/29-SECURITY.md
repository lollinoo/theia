---
phase: 29
slug: winbox-bridge-system-tray-configure-path-port-and-origin-sta
status: verified
threats_open: 0
asvs_level: L1
created: 2026-04-09
---

# Phase 29 — Security

> Per-phase security contract: threat register, accepted risks, and audit trail.

---

## Trust Boundaries

| Boundary | Description | Data Crossing |
|----------|-------------|---------------|
| Config file → bridge process | Config file is user-writable; malicious values could cause unexpected behavior | port, WinBox path, origin URL (non-secret) |
| Tray menu → ServerManager | Menu clicks trigger Start/Stop — concurrent clicks could race | control signals only |
| OS editor launch | openFileInEditor passes path to exec.Command | local filesystem path |
| CI runner → binary artifact | Build artifacts uploaded as GitHub Release assets | compiled binaries |
| macOS CGO compiler → binary | CGO enables C code compilation | system headers |

---

## Threat Register

| Threat ID | Category | Component | Disposition | Mitigation | Status |
|-----------|----------|-----------|-------------|------------|--------|
| T-29-01 | Tampering | config.go / server.go | mitigate | `config.go:83` `os.MkdirAll(..., 0o700)`; `config.go:90` `os.WriteFile(..., 0o600)`; `server.go` port range guard `< 1 \|\| > 65535` returns error | closed |
| T-29-02 | Denial of Service | server.go | mitigate | `server.go:16` `sync.Mutex`; `server.go:28-29` Start() no-op guard if already running; `server.go:55-56` Stop() no-op guard | closed |
| T-29-03 | Information Disclosure | config.go | accept | Config contains WinBox path and origin URL only — no credentials stored | closed |
| T-29-04 | Spoofing | server.go (securityCheck) | mitigate | `server.go:32` `expectedHost := fmt.Sprintf("localhost:%d", cfg.ListenPort)` — dynamic host header validation; no hardcoded port | closed |
| T-29-05 | Tampering | tray.go (config reload) | accept | Config owned by same user running bridge; local user access grants full control; port/origin validated in Start() | closed |
| T-29-06 | Denial of Service | tray.go (rapid click) | mitigate | `tray.go:24,28` initial Disable(); `tray.go:49,54` updateState() toggles Enable/Disable after every action | closed |
| T-29-07 | Elevation of Privilege | tray.go (openFileInEditor) | accept | Opens bridge's own config path via `os.UserConfigDir()` — not attacker-controlled; OS-standard file openers used | closed |
| T-29-08 | Spoofing | icon.go (tray icon) | accept | General OS-level concern; bridge is a local-only tool — not mitigatable at application level | closed |
| T-29-09 | Tampering | CI pipeline (.github/workflows/ci.yml) | mitigate | `ci.yml:266` `checkout@v4`, `ci.yml:268` `setup-go@v5`, `ci.yml:282` `action-gh-release@v2`; `ci.yml:286` `GITHUB_TOKEN` scoped to repo | closed |
| T-29-10 | Information Disclosure | binary artifacts | mitigate | All 6 CI matrix `ldflags` entries contain `-s -w`; `Makefile:165` `-ldflags="-s -w ${ldextra}"` strips symbol table and debug info | closed |
| T-29-11 | Elevation of Privilege | Windows -H=windowsgui | accept | Standard GUI/tray app pattern on Windows; same user context — no privilege change | closed |
| T-29-12 | Tampering | macOS CGO supply chain | accept | GitHub-managed macos-latest runner with Apple-provided Xcode/clang; equivalent risk to any CGO build | closed |

*Status: open · closed*
*Disposition: mitigate (implementation required) · accept (documented risk) · transfer (third-party)*

---

## Accepted Risks Log

| Risk ID | Threat Ref | Rationale | Accepted By | Date |
|---------|------------|-----------|-------------|------|
| AR-29-01 | T-29-03 | Config file contains only WinBox path and origin URL — no credentials or secrets at risk | Lollinoo | 2026-04-09 |
| AR-29-02 | T-29-05 | Config file is owned by the same OS user running the bridge; local user access already grants full system control | Lollinoo | 2026-04-09 |
| AR-29-03 | T-29-07 | openFileInEditor uses bridge's own config path from os.UserConfigDir() — not user-supplied input; path is not attacker-controlled | Lollinoo | 2026-04-09 |
| AR-29-04 | T-29-08 | Tray icon spoofing is an OS-level concern not addressable at application level; bridge is local-only with no network exposure | Lollinoo | 2026-04-09 |
| AR-29-05 | T-29-11 | -H=windowsgui is standard practice for GUI/tray apps on Windows; removes console window with no privilege change | Lollinoo | 2026-04-09 |
| AR-29-06 | T-29-12 | macOS CGO uses Apple-provided toolchain on GitHub-managed runners; risk equivalent to any standard CGO build | Lollinoo | 2026-04-09 |

---

## Security Audit Trail

| Audit Date | Threats Total | Closed | Open | Run By |
|------------|---------------|--------|------|--------|
| 2026-04-09 | 12 | 11 | 1 | gsd-security-auditor |
| 2026-04-09 | 12 | 12 | 0 | gsd-security-auditor (after T-29-01 port validation fix) |

---

## Sign-Off

- [x] All threats have a disposition (mitigate / accept / transfer)
- [x] Accepted risks documented in Accepted Risks Log
- [x] `threats_open: 0` confirmed
- [x] `status: verified` set in frontmatter

**Approval:** verified 2026-04-09
