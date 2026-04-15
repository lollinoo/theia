---
phase: 38
slug: state-engine
status: approved
nyquist_compliant: true
wave_0_complete: true
created: 2026-04-11
approved: 2026-04-12
---

# Phase 38 — Validation Strategy

> Per-phase validation contract for feedback sampling during execution.

---

## Test Infrastructure

| Property | Value |
|----------|-------|
| **Framework** | Go standard `testing` package (Go 1.24) |
| **Config file** | None — Go test conventions |
| **Quick run command** | `go test -race ./internal/state/...` |
| **Full suite command** | `go test -race ./internal/state/ -v -count=1` |
| **Estimated runtime** | ~5 seconds |

---

## Sampling Rate

- **After every task commit:** Run `go test -race ./internal/state/...`
- **After every plan wave:** Run `go test -race ./internal/state/ -v -count=1`
- **Before `/gsd-verify-work`:** Full suite must be green under `-race`
- **Max feedback latency:** 10 seconds

---

## Per-Task Verification Map

> Wave 0 test stubs are folded into Plan 01 Task 2 (`health_test.go`) and Plan 02 Task 2 (`store_test.go`) rather than a pre-stage — each test file is created in the same task that asserts on it.

| Task ID | Plan | Wave | Requirement | Threat Ref | Secure Behavior | Test Type | Automated Command | File Created In | Status |
|---------|------|------|-------------|------------|-----------------|-----------|-------------------|-----------------|--------|
| 38-02-T2 | 02 | 2 | STATE-01 | T-38-01 | No data race on shared state map under concurrent Update/Snapshot | unit + race | `go test -race ./internal/state/ -run TestStore` | Plan 02 Task 2 | ⬜ pending |
| 38-01-T2 | 01 | 1 | STATE-02 | — | Health enum = worst-of per-metric severity | unit + race | `go test -race ./internal/state/ -run TestHealth` | Plan 01 Task 2 | ⬜ pending |
| 38-01-T2 | 01 | 1 | STATE-03 | — | Hysteresis prevents flapping at threshold boundaries | unit + race | `go test -race ./internal/state/ -run TestHysteresis` | Plan 01 Task 2 | ⬜ pending |
| 38-02-T2 | 02 | 2 | STATE-04 | — | Single failure=soft down, 3 consecutive=hard down, 1 success recovers | unit + race | `go test -race ./internal/state/ -run TestReachability` | Plan 02 Task 2 | ⬜ pending |
| 38-02-T2 | 02 | 2 | STATE-05 | T-38-03 | Unchanged devices emit nothing; changed devices emit on Changes channel (non-blocking send) | unit + race | `go test -race ./internal/state/ -run TestChanges` | Plan 02 Task 2 | ⬜ pending |

*Status: ⬜ pending · ✅ green · ❌ red · ⚠️ flaky*

---

## Wave 0 Requirements

Wave 0 is absorbed into the implementation plans — no separate stub stage. The four new files are created as part of the plans that own them:

- [x] `internal/state/store.go` — created in Plan 01 Task 1 (types + NewStore) and extended in Plan 02 Task 1 (Update/Snapshot/Changes/Start/Stop methods)
- [x] `internal/state/health.go` — created in Plan 01 Task 1 (thresholds, evaluateMetricSeverity, evaluateHealth, aggregateHealth)
- [x] `internal/state/health_test.go` — created in Plan 01 Task 2 (covers STATE-02, STATE-03)
- [x] `internal/state/store_test.go` — created in Plan 02 Task 2 (covers STATE-01, STATE-04, STATE-05)
- [x] No new framework install — Go stdlib `testing` already used project-wide; `-race` flag enforced by acceptance criteria

---

## Manual-Only Verifications

| Behavior | Requirement | Why Manual | Test Instructions |
|----------|-------------|------------|-------------------|
| — | — | All phase behaviors have automated verification | — |

*All phase behaviors have automated verification.*

---

## Validation Sign-Off

- [x] All tasks have `<automated>` verify or Wave 0 dependencies — Plan 01 T1 (`go build`/`go vet`) + T2 (`go test -race -run 'TestHealth|TestHysteresis'`); Plan 02 T1 (`go build`/`go vet`) + T2 (`go test -race ./internal/state/...`)
- [x] Sampling continuity: no 3 consecutive tasks without automated verify — all 4 tasks across both plans have automated verify
- [x] Wave 0 covers all MISSING references — Wave 0 absorbed into Plan 01/02 Task 2 (test files created in same task that asserts on them)
- [x] No watch-mode flags — all commands use `go test` without `-watch`
- [x] Feedback latency < 10s — unit tests under `go test -race` on ~4 files, empirically sub-second
- [x] `nyquist_compliant: true` set in frontmatter

**Approval:** approved 2026-04-12
