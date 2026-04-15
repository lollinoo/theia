---
phase: 40-collectors
plan: 02
subsystem: infra
tags: [snmp, collectors, worker, throughput, testing]
requires:
  - phase: 40-01
    provides: typed collector results plus InterfaceCounterSnapshot speed metadata
provides:
  - pure counter-rate helper with explicit CounterBaseline state
  - MetricsCollector SNMP link path delegated to collector.ComputeCounterRates
  - reset, gap, overspeed, warm-up, and recovery coverage for SNMP link rates
affects:
  - 40-03-PLAN
  - 42-pipeline-orchestrator
tech-stack:
  added: []
  patterns:
    - stateless counter-rate helper with explicit baseline maps
    - worker-owned runtime baselines delegating all rate decisions to collector helpers
key-files:
  created:
    - internal/collector/rates.go
    - internal/collector/rates_test.go
  modified:
    - internal/worker/metrics_collector.go
    - internal/worker/metrics_collector_test.go
key-decisions:
  - "MetricsCollector keeps the runtime baseline map, but all counter validation and rate math live in the pure collector helper."
  - "Discarded counter intervals stay absent from link DTOs; throughput is never clamped to zero or back-filled from stale samples."
patterns-established:
  - "SNMP counter polls are converted to collector.InterfaceCounterSnapshot values before any rate calculation."
  - "Invalid intervals set NeedsWarmup and require one clean baseline sample before user-visible rates resume."
requirements-completed: [PIPE-02, PIPE-04]
duration: 7 min
completed: 2026-04-12
---

# Phase 40 Plan 02: Collectors Summary

**Stateless counter-rate helper with reset, gap, and overspeed discards wired into the runtime SNMP link path**

## Performance

- **Duration:** 7 min
- **Started:** 2026-04-12T14:02:12Z
- **Completed:** 2026-04-12T14:09:19Z
- **Tasks:** 2
- **Files modified:** 4

## Accomplishments
- Added `internal/collector/rates.go` with a pure `ComputeCounterRates()` helper and explicit `CounterBaseline` state that never mutates the caller's baseline map in place.
- Replaced inline unsigned subtraction in `MetricsCollector.buildSnapshot()` with helper delegation using discovered interface speed metadata and the existing polling interval.
- Added unit and integration coverage for first-sample warm-up, reset discard, gap discard, overspeed discard, and the recovery sample after warm-up.

## Task Commits

Each task was committed atomically via TDD:

1. **Task 1 RED: counter-rate helper tests** - `348b289` (test)
2. **Task 1 GREEN: pure counter-rate helper** - `c3e978b` (feat)
3. **Task 2 RED: MetricsCollector discard-path tests** - `7a3bf9b` (test)
4. **Task 2 GREEN: MetricsCollector helper delegation** - `34ef250` (feat)

## Files Created/Modified
- `internal/collector/rates.go` - Pure counter-rate helper with explicit baseline cloning, reset/gap/overspeed discard handling, and warm-up state.
- `internal/collector/rates_test.go` - Unit coverage for first sample, reset, gap, overspeed, and happy-path calculations.
- `internal/worker/metrics_collector.go` - Runtime SNMP link path now converts raw counters into collector snapshots and delegates all rate decisions to `collector.ComputeCounterRates`.
- `internal/worker/metrics_collector_test.go` - Integration coverage proving invalid intervals stay absent from snapshot DTOs until a warm-up sample restores rates.

## Verification
- `PATH=/usr/local/go/bin:$PATH rtk go test ./internal/collector -count=1` - passed
- `PATH=/usr/local/go/bin:$PATH rtk go test ./internal/worker -count=1` - passed
- `PATH=/usr/local/go/bin:$PATH rtk go build ./...` - passed
- Task acceptance grep/file checks for `CounterBaseline`, `ComputeCounterRates`, `collector.ComputeCounterRates`, and removal of the old inline subtraction logic - passed

## Decisions Made
- `MetricsCollector` remains the owner of per-device runtime baselines in Phase 40, but the state is now the explicit `collector.CounterBaseline` type instead of worker-local inline math.
- Interface speeds are resolved before calling the helper so sanity-bound enforcement stays in the stateless collector seam rather than leaking inventory lookups into the helper itself.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 2 - Missing Critical] Discarded non-increasing timestamps as invalid intervals**
- **Found during:** Task 1 (Create a pure counter-rate helper with explicit baseline state)
- **Issue:** Untrusted sample timestamps could produce a zero or negative interval, which would otherwise allow divide-by-zero or stale rate output.
- **Fix:** Treated `!collectedAt.After(prev.SampledAt)` as an invalid interval, discarded throughput for that sample, and forced a fresh warm-up baseline.
- **Files modified:** `internal/collector/rates.go`
- **Verification:** `PATH=/usr/local/go/bin:$PATH rtk go test ./internal/collector -count=1`; `PATH=/usr/local/go/bin:$PATH rtk go test ./internal/worker -count=1`; `PATH=/usr/local/go/bin:$PATH rtk go build ./...`
- **Committed in:** `c3e978b`

---

**Total deviations:** 1 auto-fixed (Rule 2: 1)
**Impact on plan:** Closed an additional invalid-interval correctness edge case without widening the public contract or task scope.

## Issues Encountered
- The new worker recovery assertions needed a short sleep before the post-warm-up sample so `time.Now()` advanced far enough for deterministic positive-rate calculations in test runs.
- `go` is not on the default shell `PATH` in this environment. Verification commands were run with `PATH=/usr/local/go/bin:$PATH`.

## User Setup Required

None - no external service configuration required.

## Next Phase Readiness
- Phase 42 can reuse `collector.ComputeCounterRates()` as the seam between collection and state application without carrying over worker-local arithmetic.
- The current `MetricsCollector` behavior now matches the Phase 40 discard policy while preserving the existing DISC-02 link-walk guard and `attachLinkMetrics()` pipeline.

## Self-Check: PASSED
- Found summary file: `.planning/phases/40-collectors/40-02-SUMMARY.md`
- Found created file: `internal/collector/rates.go`
- Found task commits: `348b289`, `c3e978b`, `7a3bf9b`, `34ef250`

---
*Phase: 40-collectors*
*Completed: 2026-04-12*
