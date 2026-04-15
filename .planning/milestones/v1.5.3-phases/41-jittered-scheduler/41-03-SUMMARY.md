---
phase: 41-jittered-scheduler
plan: 03
subsystem: infra
tags: [scheduler, snmp, concurrency, lifecycle, go]
requires:
  - phase: 41-jittered-scheduler
    provides: Heap-backed scheduler state, deterministic timing helpers, and managed-device refresh reconciliation from Plans 01-02
provides:
  - Runnable scheduler lifecycle via Start, Stop, Tasks, and Complete
  - Global SNMP concurrency limiting from snmp_worker_pool_size with volatility-priority dispatch
  - Coalesced backlog handling with completion-time reinsertion and one immediate rerun for overlapped work
affects:
  - 41-jittered-scheduler
  - 42-pipeline-orchestrator-cutover
  - internal/scheduler
tech-stack:
  added: []
  patterns:
    - Single-goroutine scheduler loop with one timer, one ticker, one heap, and three ready queues
    - Completion-driven reinsertion with explicit overlap coalescing instead of replaying every missed interval
key-files:
  created:
    - .planning/phases/41-jittered-scheduler/41-03-SUMMARY.md
  modified:
    - internal/scheduler/scheduler.go
    - internal/scheduler/scheduler_test.go
key-decisions:
  - Reused domain.SettingSNMPWorkerPoolSize as the single global scheduler concurrency cap, with fallback 5 on missing or invalid settings.
  - Mirrored the reusable Start/Stop worker lifecycle already used in internal/state so the scheduler can be restarted safely after Stop().
  - Treated only strictly overlapped completions as coalesced reruns; completions exactly on the next due boundary reinsert from FinishedAt normally.
patterns-established:
  - Scheduler dispatch drains volatility-specific ready queues in performance, operational, then static order under one shared in-flight cap.
  - A task that runs past its next due boundary queues exactly one immediate rerun, then returns to normal jittered cadence from the later completion point.
requirements-completed: [POLL-01, POLL-02, POLL-04]
duration: 6m37s
completed: 2026-04-12
---

# Phase 41 Plan 03: Jittered Scheduler Summary

**Runnable heap-backed scheduler with reusable worker lifecycle, shared SNMP concurrency cap, priority dispatch, and bounded backlog coalescing**

## Performance

- **Duration:** 6m37s
- **Started:** 2026-04-12T18:57:19+00:00
- **Completed:** 2026-04-12T19:03:56+00:00
- **Tasks:** 2
- **Files modified:** 3

## Accomplishments
- Added the scheduler runtime loop with `Start(ctx)`, `Stop()`, `Tasks()`, and `Complete()` on top of the existing heap-backed state from Plan 02.
- Enforced one global in-flight SNMP limit from `snmp_worker_pool_size` with fallback `5`, while dispatching ready work in `performance`, `operational`, then `static` order.
- Encoded coalesced backlog semantics so due work never fans out unboundedly and completion-based reinsertion always advances from the completion timestamp.

## Verification Results
- `go test ./internal/scheduler -run 'TestSchedulerDispatchesPriorityOrder|TestSchedulerMaxInFlight|TestSchedulerStartStop' -count=1 -v` passed after Task 1.
- `go test ./internal/scheduler -run 'TestSchedulerCoalesces|TestSchedulerComplete' -count=1 -v` passed after Task 2.
- `go test ./internal/scheduler -count=1` passed after both tasks completed.
- `go build ./...` passed after both tasks completed.
- `internal/scheduler/scheduler.go` now contains `Start`, `Stop`, `Complete`, `handleCompletion`, `time.NewTimer`, and `SettingSNMPWorkerPoolSize`, with no `time.After(` usage in `internal/scheduler`.

## Task Commits

Each task was committed atomically:

1. **Task 1: Add runtime loop, lifecycle, priority dispatch, and concurrency limiting** - `516b3d3` (test), `99034b5` (feat)
2. **Task 2: Encode coalesced backlog semantics and completion-based reinsertion** - `a08e96b` (test), `3200df6` (feat)

## Files Created/Modified
- `internal/scheduler/scheduler.go` - Runtime lifecycle, timer loop, ready-queue dispatch, completion handling, and global concurrency-cap logic.
- `internal/scheduler/scheduler_test.go` - Coverage for priority dispatch, worker-cap parsing, reusable lifecycle, pending reruns, completion reinsertion, disabled drops, and overlapped-run coalescing.
- `.planning/phases/41-jittered-scheduler/41-03-SUMMARY.md` - Plan-local execution summary; shared GSD state artifacts were intentionally left untouched for the orchestrator.

## Decisions Made
- Reused the existing `snmp_worker_pool_size` setting rather than adding scheduler-local configuration, keeping Phase 41 aligned with the current worker-pool contract.
- Kept the runtime loop single-threaded inside the scheduler package: one goroutine owns heap, ready queues, completions, and in-flight bookkeeping.
- Left `STATE.md` and `ROADMAP.md` unchanged per executor constraints because `.planning/ROADMAP.md` already had unrelated uncommitted changes.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 1 - Bug] Disabled stale queued items during refresh reconciliation**
- **Found during:** Task 1 (Add runtime loop, lifecycle, priority dispatch, and concurrency limiting)
- **Issue:** Once ready queues existed, removing an unseen queued item from `s.items` immediately could leave a stale pointer in the ready queue and allow an unmanaged task to dispatch anyway.
- **Fix:** Treated queued unseen items like unseen in-flight items by marking them `disabled` and letting the runtime loop drop them safely on the next ready/completion path.
- **Files modified:** `internal/scheduler/scheduler.go`
- **Verification:** `go test ./internal/scheduler -count=1`, `go build ./...`
- **Committed in:** `99034b5`

**2. [Rule 2 - Missing Critical] Coalesced long-running overlap into one immediate rerun**
- **Found during:** Task 2 (Encode coalesced backlog semantics and completion-based reinsertion)
- **Issue:** A task that ran longer than its interval would never set `pending` because active work is not kept in the heap while running, so the coalesced rerun could be skipped entirely.
- **Fix:** On completion, if `FinishedAt` is strictly after `item.dueAt + item.interval`, the scheduler now queues exactly one immediate rerun at `FinishedAt` before returning to normal jittered cadence.
- **Files modified:** `internal/scheduler/scheduler.go`
- **Verification:** `go test ./internal/scheduler -run 'TestSchedulerCoalesces|TestSchedulerComplete' -count=1 -v`, `go test ./internal/scheduler -count=1`, `go build ./...`
- **Committed in:** `3200df6`

---

**Total deviations:** 2 auto-fixed (1 bug, 1 missing critical functionality)
**Impact on plan:** Both auto-fixes were required for the scheduler to enforce the plan's own bounded-backlog and stale-task correctness guarantees. Scope stayed inside the owned scheduler files.

## Issues Encountered
- The literal task split left a correctness gap: overlap coalescing was not actually observable for long-running tasks until completion handling compared `FinishedAt` against the next due boundary. The extra Task 2 test exposed and closed that gap.

## User Setup Required

None - no external service configuration required.

## Next Phase Readiness
- Phase 42 can now consume `Scheduler.Tasks()` and report `Completion` events back through `Scheduler.Complete()` without redesigning task ownership or backlog behavior.
- The scheduler already enforces one shared SNMP cap and reusable worker lifecycle, so the next phase can focus on wiring collectors and state updates rather than rebuilding timing semantics.

## Self-Check: PASSED
- Found `.planning/phases/41-jittered-scheduler/41-03-SUMMARY.md` on disk.
- Verified task commits `516b3d3`, `99034b5`, `a08e96b`, and `3200df6` exist in `git log --oneline --all`.
- Confirmed `go test ./internal/scheduler -count=1` and `go build ./...` passed after the final implementation.

---
*Phase: 41-jittered-scheduler*
*Completed: 2026-04-12*
