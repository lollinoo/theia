---
phase: 45-polling-cadence-gap-closure
plan: 01
subsystem: infra
tags: [scheduler, polling, device-service, bootstrap, go]
requires:
  - phase: 41-jittered-scheduler
    provides: Scheduler heap/ready/in-flight ownership, pull-based inventory refresh, and targeted poll task dispatch
  - phase: 44-frontend-integration
    provides: Device API/UI persistence for poll_interval_override updates
provides:
  - Targeted scheduler performance-task re-due path for override changes
  - DeviceService hook that notifies the live scheduler only after persisted override changes
  - Bootstrap helper that wires Scheduler into DeviceService before runtime startup
affects:
  - 45-polling-cadence-gap-closure
  - 46-detail-delta-gap-closure
  - internal/scheduler
  - internal/service
  - cmd/theia
tech-stack:
  added: []
  patterns:
    - Scheduler command queue for targeted runtime state changes that preserves single-goroutine ownership
    - Service-to-scheduler bridge triggered only after persistence and only for real poll override changes
key-files:
  created:
    - .planning/phases/45-polling-cadence-gap-closure/45-01-SUMMARY.md
  modified:
    - internal/scheduler/scheduler.go
    - internal/scheduler/scheduler_test.go
    - internal/service/device_service.go
    - internal/service/device_service_test.go
    - cmd/theia/main.go
    - cmd/theia/main_test.go
key-decisions:
  - Targeted performance re-due requests are funneled through a buffered scheduler command channel so heap and ready state remain owned by the scheduler goroutine.
  - DeviceService compares previous and persisted override values after repository update; omitted, unrelated, and same-value edits do not schedule extra work.
  - Production bootstrap uses a dedicated wirePollRescheduler helper so tests can prove the live scheduler is attached before the API path can save overrides.
patterns-established:
  - Override-driven runtime nudges stay narrow: only TaskKey(deviceID, performance) is updated and the periodic pull-based inventory refresh model remains intact for all other scheduler state.
  - Service-layer hooks into background runtimes use a tiny private interface plus explicit bootstrap wiring instead of importing a broad worker dependency surface.
requirements-completed: [POLL-02, POLL-06]
duration: 10m13s
completed: 2026-04-13
---

# Phase 45 Plan 01: Polling Cadence Gap Closure Summary

**Immediate performance-task re-due for persisted poll overrides, with DeviceService-to-scheduler bridging and production bootstrap wiring**

## Performance

- **Duration:** 10m13s
- **Started:** 2026-04-13T20:23:07Z
- **Completed:** 2026-04-13T20:33:20Z
- **Tasks:** 2
- **Files modified:** 6

## Accomplishments

- Added `Scheduler.ReduePerformanceTask(...)` with explicit heap, ready-queue, in-flight, and missing-device handling that only touches the affected device's performance task.
- Added a `DeviceService` rescheduler seam so persisted `poll_interval_override` changes immediately notify the live scheduler while unchanged or unrelated updates stay no-op.
- Added regression coverage proving the next emitted performance task carries the new override cadence and that `cmd/theia` bootstrap really attaches the scheduler to `DeviceService`.

## Verification Results

- `go test ./internal/scheduler -run 'TestScheduler(ReduePerformanceTask|Complete_RequeuesImmediatePendingRerun)' -count=1 -v` passed after Task 1 and again in final plan verification.
- `go test ./internal/service ./cmd/theia -run 'Test(UpdateDevice_PollIntervalOverride(TriState|TriggersSchedulerRedueOnChange|DoesNotTriggerSchedulerRedueWhenUnchanged|ReduesNextPerformanceTask)|WirePollRescheduler_AttachesSchedulerToDeviceService)' -count=1 -v` passed after Task 2 and again in final plan verification.

## Task Commits

Each task was committed atomically:

1. **Task 1: Add a scheduler-side targeted performance re-due command for override changes** - `9c6c5c7` (test), `fb2d670` (feat)
2. **Task 2: Trigger the targeted scheduler re-due only when a persisted override actually changes, then prove both cadence behavior and bootstrap wiring** - `4c1e736` (test), `77d46d2` (feat)

## Files Created/Modified

- `internal/scheduler/scheduler.go` - Adds the buffered targeted re-due command path, per-state item updates, and request draining on reset.
- `internal/scheduler/scheduler_test.go` - Covers heap, queued, in-flight, missing-managed, and unmanaged redue scenarios.
- `internal/service/device_service.go` - Adds the private poll rescheduler interface, setter, and persisted-change-only trigger logic in `UpdateDevice`.
- `internal/service/device_service_test.go` - Covers change/no-change trigger behavior and the real scheduler integration with the quiet-window/seeded-offset anti-seeding proof.
- `cmd/theia/main.go` - Extracts and uses `wirePollRescheduler(...)` so production startup attaches the live scheduler before pipeline/router startup.
- `cmd/theia/main_test.go` - Verifies bootstrap wiring through reflection on the private `pollRescheduler` field without adding a production getter.

## Decisions Made

- The targeted reschedule path stays inside the scheduler event loop instead of mutating heap or ready state directly from service goroutines.
- `UpdateDevice(...)` triggers the scheduler only after repository persistence succeeds and only when `poll_interval_override` materially changed.
- Bootstrap proof is done through a dedicated helper plus reflection in tests, preserving encapsulation on `DeviceService`.

## Deviations from Plan

None - plan executed exactly as written.

## Issues Encountered

None.

## User Setup Required

None - no external service configuration required.

## Next Phase Readiness

- Phase 45 Plan 02 can now rely on immediate override-driven performance task re-due behavior in the live runtime instead of waiting for periodic inventory refresh.
- The scheduler remains pull-based overall, so the remaining freshness/cadence ownership work can stay focused on state and websocket semantics rather than runtime rescheduling.

## Self-Check: PASSED

- Found `.planning/phases/45-polling-cadence-gap-closure/45-01-SUMMARY.md` on disk.
- Verified task commits `9c6c5c7`, `fb2d670`, `4c1e736`, and `77d46d2` exist in `git log --oneline --all`.

---
*Phase: 45-polling-cadence-gap-closure*
*Completed: 2026-04-13*
