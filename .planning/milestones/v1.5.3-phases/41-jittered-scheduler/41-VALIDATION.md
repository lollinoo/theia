---
phase: 41
slug: jittered-scheduler
status: verified
nyquist_compliant: true
wave_0_complete: true
created: 2026-04-15
---

# Phase 41 -- Validation Strategy

> Per-phase validation contract for feedback sampling during execution.

---

## Test Infrastructure

| Property | Value |
|----------|-------|
| **Framework** | Go standard `testing` package |
| **Config file** | None -- repo Go test conventions |
| **Quick run command** | `rtk bash -lc 'go test ./internal/scheduler -run "TestInitialOffset_DistributionAcrossBuckets|TestRefreshDevices_SeedsManagedDeviceAcrossAllThreeVolatilityClasses|TestSchedulerDispatchesPriorityOrder|TestSchedulerComplete_CoalescesElapsedIntervalsToSingleImmediateRerun|TestSchedulerMaxInFlight_DefaultAndConfigured" -count=1 -v'` |
| **Full suite command** | `rtk go test ./internal/scheduler -count=1` |
| **Estimated runtime** | ~10 seconds |

---

## Sampling Rate

- **After every task commit:** Run the exact scheduler test command captured in that task's plan.
- **After every plan wave:** Re-run the accepted targeted scheduler regression command from `41-VERIFICATION.md`.
- **Before `/gsd-verify-work`:** `rtk go test ./internal/scheduler -count=1` must remain green on the current codebase.
- **Max feedback latency:** 30 seconds

---

## Per-Task Verification Map

| Task ID | Plan | Wave | Requirement | Threat Ref | Secure Behavior | Test Type | Automated Command | Key Artifact | Status |
|---------|------|------|-------------|------------|-----------------|-----------|-------------------|--------------|--------|
| 41-01-T1 | 01 | 1 | POLL-02, POLL-04 | -- | Scheduler contracts reuse shared poll/volatility enums and define deterministic priority ordering | unit | `cd /home/azmin/projects/theia && go test ./internal/scheduler -run 'TestEffectiveInterval|TestVolatilityPriority' -count=1 -v` | `internal/scheduler/types.go` | ✅ green |
| 41-01-T2 | 01 | 1 | POLL-02, POLL-04 | -- | Initial offsets and next-fire jitter stay deterministic and bounded inside the interval window | unit | `cd /home/azmin/projects/theia && go test ./internal/scheduler -run 'TestInitialOffset|TestJitteredNext' -count=1 -v` | `internal/scheduler/jitter.go` | ✅ green |
| 41-02-T1 | 02 | 2 | POLL-01, POLL-02, POLL-04 | -- | Heap ordering remains stable across due time, volatility priority, and device identity | unit | `cd /home/azmin/projects/theia && go test ./internal/scheduler -run 'TestTaskHeap' -count=1 -v` | `internal/scheduler/heap.go` | ✅ green |
| 41-02-T2 | 02 | 2 | POLL-01, POLL-02, POLL-04 | -- | Refresh reconciliation seeds managed devices across static, operational, and performance cadences without duplicate keys | unit | `cd /home/azmin/projects/theia && go test ./internal/scheduler -run 'TestRefreshDevices' -count=1 -v` | `internal/scheduler/scheduler.go` | ✅ green |
| 41-03-T1 | 03 | 3 | POLL-01, POLL-02, POLL-04 | -- | Runtime loop, lifecycle, dispatch priority, and concurrency limiting hold under targeted regression tests | runtime | `cd /home/azmin/projects/theia && go test ./internal/scheduler -run 'TestSchedulerDispatchesPriorityOrder|TestSchedulerMaxInFlight|TestSchedulerStartStop' -count=1 -v` | `internal/scheduler/scheduler.go` | ✅ green |
| 41-03-T2 | 03 | 3 | POLL-01, POLL-02, POLL-04 | -- | Coalesced backlog semantics and completion-based reinsertion avoid duplicate reruns and burst rebuilds | runtime | `cd /home/azmin/projects/theia && go test ./internal/scheduler -run 'TestSchedulerCoalesces|TestSchedulerComplete' -count=1 -v` | `internal/scheduler/scheduler.go` | ✅ green |

*Status: ⬜ pending · ✅ green · ❌ red · ⚠️ flaky*

---

## Wave 0 Requirements

- [x] `internal/scheduler/types_test.go` covers effective interval resolution, override semantics, and volatility ordering.
- [x] `internal/scheduler/jitter_test.go` covers bounded jitter, deterministic offsets, and `TestInitialOffset_DistributionAcrossBuckets`.
- [x] `internal/scheduler/scheduler_test.go` covers refresh seeding, lifecycle, concurrency limits, dispatch priority, backlog coalescing, and reinsertion behavior.
- [x] The accepted `41-VERIFICATION.md` package regression reruns the scheduler package directly on current HEAD.
- [x] No new framework install was required -- the existing Go scheduler test harness was sufficient.

---

## Manual-Only Verifications

| Behavior | Requirement | Why Manual | Test Instructions |
|----------|-------------|------------|-------------------|
| -- | -- | All phase behaviors have automated verification. | -- |

*All phase behaviors have automated verification.*

---

## Validation Sign-Off

- [x] All tasks have `<automated>` verify or Wave 0 dependencies.
- [x] Sampling continuity: no 3 consecutive tasks without automated verify.
- [x] Wave 0 covers the shipped scheduler package tests and targeted distribution evidence.
- [x] No watch-mode flags were used in accepted evidence.
- [x] Feedback latency remains under 30 seconds for the scheduler package.
- [x] `nyquist_compliant: true` is set in frontmatter.

**Approval:** verified 2026-04-15
