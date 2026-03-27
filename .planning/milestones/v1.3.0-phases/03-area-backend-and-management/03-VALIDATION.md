---
phase: 3
slug: area-backend-and-management
status: draft
nyquist_compliant: true
wave_0_complete: true
created: 2026-03-26
---

# Phase 3 — Validation Strategy

> Per-phase validation contract for feedback sampling during execution.

---

## Test Infrastructure

| Property | Value |
|----------|-------|
| **Framework (Go)** | Go standard `testing` package |
| **Framework (Frontend)** | Vitest 4.1 with jsdom + @testing-library/react 16.3 |
| **Config file (Frontend)** | `frontend/vitest.config.ts` |
| **Quick run command (Go)** | `go test ./internal/api/ -run TestArea -count=1` |
| **Quick run command (Frontend)** | `cd frontend && npx vitest run src/components/AreaManager.test.tsx` |
| **Full suite command (Go)** | `go test ./internal/...` |
| **Full suite command (Frontend)** | `cd frontend && npx vitest run` |
| **Estimated runtime** | ~15 seconds (Go) + ~10 seconds (Frontend) |

---

## Sampling Rate

- **After every task commit:** Run `go test ./internal/api/ -run TestArea -count=1 && go test ./internal/repository/sqlite/ -run TestArea -count=1`
- **After every plan wave:** Run `go test ./internal/... && cd frontend && npx vitest run`
- **Before `/gsd:verify-work`:** Full suite must be green
- **Max feedback latency:** 25 seconds

---

## Per-Task Verification Map

| Task ID | Plan | Wave | Requirement | Test Type | Automated Command | File Exists | Status |
|---------|------|------|-------------|-----------|-------------------|-------------|--------|
| 03-01-01 | 01 | 1 | AREA-01, AREA-03 | integration | `go test ./internal/repository/sqlite/ -run TestAreaRepo -count=1` | Yes (in plan) | pending |
| 03-01-02 | 01 | 1 | AREA-04 | integration | `go test ./internal/repository/sqlite/ -run "TestAreaRepo\|TestDeviceRepo" -count=1` | Yes (in plan) | pending |
| 03-01-03 | 01 | 1 | AREA-02 | unit | `go test ./internal/api/ -run "TestArea\|TestDeviceHandlerUpdate_AreaID" -count=1` | Yes (in plan) | pending |
| 03-02-01 | 02 | 2 | AREA-05 | component | `cd frontend && npx vitest run src/components/AreaManager.test.tsx` | Yes (in plan) | pending |
| 03-02-02 | 02 | 2 | AREA-06 | component | `cd frontend && npx vitest run src/components/DeviceConfigPanel.test.tsx` | Exists (extend) | pending |

*Status: pending · green · red · flaky*

---

## Wave 0 Requirements

- [x] `internal/repository/sqlite/area_repo_test.go` — created in Plan 03-01 Task 1 (covers AREA-01, AREA-03)
- [x] `internal/api/area_handler_test.go` — created in Plan 03-01 Task 3 (covers AREA-02)
- [x] Extend `internal/api/device_handler_test.go` — extended in Plan 03-01 Task 3 (covers AREA-04)
- [x] `frontend/src/components/AreaManager.test.tsx` — created in Plan 03-02 Task 1 (covers AREA-05)
- [x] Extend `frontend/src/components/DeviceConfigPanel.test.tsx` — extended in Plan 03-02 Task 2 (covers AREA-06)

---

## Manual-Only Verifications

| Behavior | Requirement | Why Manual | Test Instructions |
|----------|-------------|------------|-------------------|
| Color swatch renders correct hex in UI | AREA-05 | Visual rendering fidelity | Open Settings > Areas, verify swatch dot matches selected palette color |
| Area dropdown shows swatch dots | AREA-06 | Visual rendering fidelity | Open device config, verify area dropdown shows colored dots next to area names |

---

## Validation Sign-Off

- [x] All tasks have `<automated>` verify with behavioral test commands
- [x] Sampling continuity: no 3 consecutive tasks without automated verify
- [x] Wave 0 covers all MISSING references
- [x] No watch-mode flags
- [x] Feedback latency < 25s
- [x] `nyquist_compliant: true` set in frontmatter

**Approval:** ready
