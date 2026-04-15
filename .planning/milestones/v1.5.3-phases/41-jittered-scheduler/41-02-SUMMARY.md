---
phase: 41-jittered-scheduler
plan: 02
subsystem: infra
tags: [scheduler, heap, refresh, go]
requires:
  - phase: 41-jittered-scheduler
    provides: TaskKey identity, volatility priorities, effective intervals, and deterministic initial offsets from Plan 01
provides:
  - Heap-backed scheduler state keyed by device UUID and volatility class
  - Pull-based device refresh reconciliation that seeds managed devices only
  - In-place metadata updates that preserve existing due times across refreshes
affects:
  - 41-jittered-scheduler
  - 42-pipeline-orchestrator-cutover
  - internal/scheduler
tech-stack:
  added: []
  patterns:
    - Single min-heap plus item map for deduplicated scheduler state
    - Periodic pull refresh reconciles managed inventory without CRUD push hooks
key-files:
  created:
    - internal/scheduler/heap.go
    - internal/scheduler/heap_test.go
    - internal/scheduler/scheduler.go
    - internal/scheduler/scheduler_test.go
    - .planning/phases/41-jittered-scheduler/41-02-SUMMARY.md
  modified: []
key-decisions:
  - Existing scheduled keys preserve their current due time while refresh updates device snapshots, poll class metadata, and expected intervals in place.
  - Refresh seeds exactly three scheduler entries per managed device: performance, operational, and static.
  - Missing or unmanaged in-flight work is marked disabled rather than deleted so completion handling in Plan 03 can avoid reinserting stale tasks.
patterns-established:
  - Scheduler state is anchored by a `map[TaskKey]*heapItem` plus a single `taskHeap`, preventing duplicate heap growth across refresh cycles.
  - Pull-based reconciliation uses a `seen` set of managed keys and removes any non-seen idle items immediately.
requirements-completed: [POLL-01, POLL-02, POLL-04]
duration: 5m13s
completed: 2026-04-12
---

# Phase 41 Plan 02: Jittered Scheduler Summary

**Heap-backed scheduler state with managed-device refresh reconciliation and deterministic first-fire seeding across performance, operational, and static work**

## Performance

- **Duration:** 5m13s
- **Started:** 2026-04-12T18:47:43+00:00
- **Completed:** 2026-04-12T18:52:56Z
- **Tasks:** 2
- **Files modified:** 5

## Accomplishments
- Added the scheduler heap item model and strict min-heap ordering by due time, volatility priority, and device UUID.
- Added pull-based scheduler state with default refresh/task buffers, per-device/per-volatility item storage, and deterministic first due times.
- Proved managed-only seeding, interval separation, stale-key removal, disabled in-flight reconciliation, and metadata refresh without due-time drift.

## Verification Results
- `go test ./internal/scheduler -run 'TestTaskHeap' -count=1 -v` passed after the heap implementation landed.
- `go test ./internal/scheduler -run 'TestRefreshDevices' -count=1 -v` passed after scheduler refresh reconciliation landed.
- `go test ./internal/scheduler -count=1` passed after both tasks completed.
- `go build ./...` passed after both tasks completed.

## Task Commits

Each task was committed atomically:

1. **Task 1: Add heap item state and deterministic min-heap ordering** - `03f107d` (test), `0c017fa` (feat)
2. **Task 2: Build scheduler refresh reconciliation and first-fire seeding** - `d6bc864` (test), `b7c603b` (feat)

## Files Created/Modified
- `internal/scheduler/heap.go` - Heap item runtime state and `container/heap` interface implementation for scheduler ordering.
- `internal/scheduler/heap_test.go` - TDD coverage for ordering, `heap.Fix`, and disabled-item removal behavior.
- `internal/scheduler/scheduler.go` - Scheduler constructor defaults, `DeviceSource`, `Tasks()`, and pull-based refresh reconciliation.
- `internal/scheduler/scheduler_test.go` - TDD coverage for managed-only seeding, interval independence, stale-key reconciliation, and in-place metadata refresh.
- `.planning/phases/41-jittered-scheduler/41-02-SUMMARY.md` - Execution summary for this plan only; shared GSD state artifacts were intentionally left untouched.

## Decisions Made
- Kept `defaultInventoryRefreshInterval` hardcoded at `30s` per the locked phase decision instead of introducing settings-driven refresh control in this plan.
- Updated existing items in place during refresh rather than re-pushing them, which directly mitigates duplicate heap growth and preserves due-time determinism.
- Seeded new work from `now.Add(initialOffset(device.ID, interval))` for every volatility class so first-fire spreading stays consistent with Plan 01 timing helpers.

## Deviations from Plan

None - plan executed exactly as written.

## Issues Encountered
- Go's `container/heap.Init` does not populate custom item indices for a prebuilt slice, so the heap tests initialize indices before calling `heap.Init` to match the stdlib contract and keep `heap.Fix` / `heap.Remove` assertions meaningful.

## User Setup Required

None - no external service configuration required.

## Next Phase Readiness
- Plan 03 can build the runtime dispatch loop on top of the existing `items` map, `taskHeap`, task channel, and completion channel without revisiting refresh semantics.
- Disabled in-flight stale tasks now have an explicit representation, so completion handling can safely suppress reinsertion for removed or unmanaged devices.

## Self-Check: PASSED
- Found `.planning/phases/41-jittered-scheduler/41-02-SUMMARY.md` on disk.
- Verified task commits `03f107d`, `0c017fa`, `d6bc864`, and `b7c603b` exist in `git log --oneline --all`.
- Confirmed `go test ./internal/scheduler -count=1` and `go build ./...` passed after the final implementation.
