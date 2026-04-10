---
phase: 29
slug: winbox-bridge-system-tray-configure-path-port-and-origin-sta
status: draft
nyquist_compliant: false
wave_0_complete: false
created: 2026-04-08
---

# Phase 29 — Validation Strategy

> Per-phase validation contract for feedback sampling during execution.

---

## Test Infrastructure

| Property | Value |
|----------|-------|
| **Framework** | go test |
| **Config file** | `winbox-bridge/go.mod` (or project root `go.mod` if bridge is a sub-package) |
| **Quick run command** | `go test ./cmd/winbox-bridge/...` |
| **Full suite command** | `go test ./cmd/winbox-bridge/... -v` |
| **Estimated runtime** | ~5 seconds |

---

## Sampling Rate

- **After every task commit:** Run `go test ./cmd/winbox-bridge/...`
- **After every plan wave:** Run `go test ./cmd/winbox-bridge/... -v`
- **Before `/gsd-verify-work`:** Full suite must be green
- **Max feedback latency:** 10 seconds

---

## Per-Task Verification Map

| Task ID | Plan | Wave | Requirement | Threat Ref | Secure Behavior | Test Type | Automated Command | File Exists | Status |
|---------|------|------|-------------|------------|-----------------|-----------|-------------------|-------------|--------|
| 29-01-01 | 01 | 1 | CONFIG | T-29-01 | Config file only readable by owner | unit | `go test ./cmd/winbox-bridge/... -run TestConfig` | ❌ W0 | ⬜ pending |
| 29-01-02 | 01 | 1 | SERVER | — | N/A | unit | `go test ./cmd/winbox-bridge/... -run TestServerManager` | ❌ W0 | ⬜ pending |
| 29-02-01 | 02 | 2 | TRAY | — | N/A | manual | Verify tray icon appears on each platform | N/A | ⬜ pending |

*Status: ⬜ pending · ✅ green · ❌ red · ⚠️ flaky*

---

## Wave 0 Requirements

- [ ] `cmd/winbox-bridge/config_test.go` — stubs for config read/write
- [ ] `cmd/winbox-bridge/server_test.go` — stubs for ServerManager start/stop lifecycle

*Existing `go test` infrastructure covers all phase requirements.*

---

## Manual-Only Verifications

| Behavior | Requirement | Why Manual | Test Instructions |
|----------|-------------|------------|-------------------|
| System tray icon appears and menus render | TRAY | Requires display/OS integration | Run binary on each platform, verify icon in tray |
| Start/Stop server toggles without binary restart | SERVER-LIFECYCLE | Requires manual click | Click Start, verify HTTP responds; click Stop, verify HTTP closed |
| Config dialog/file opens and saves values | CONFIG-UX | Requires user interaction | Edit path/port/origin, save, restart server, verify new values applied |

---

## Validation Sign-Off

- [ ] All tasks have `<automated>` verify or Wave 0 dependencies
- [ ] Sampling continuity: no 3 consecutive tasks without automated verify
- [ ] Wave 0 covers all MISSING references
- [ ] No watch-mode flags
- [ ] Feedback latency < 10s
- [ ] `nyquist_compliant: true` set in frontmatter

**Approval:** pending
