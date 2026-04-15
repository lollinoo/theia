---
phase: 41-jittered-scheduler
plan: 01
subsystem: infra
tags: [scheduler, snmp, jitter, fnv, go]
requires:
  - phase: 39-domain-types-db-migration
    provides: PollClass, VolatilityClass, and locked cadence constants for scheduler interval resolution
  - phase: 40-collectors
    provides: Collector volatility vocabulary aligned with scheduler PollTask contracts
provides:
  - Scheduler task identity via TaskKey, PollTask, and Completion
  - EffectiveInterval and VolatilityPriority helpers for volatility-tiered scheduling
  - Deterministic FNV-based initial offsets and bounded next-fire jitter proven by distribution tests
affects:
  - 41-jittered-scheduler
  - 42-pipeline-orchestrator-cutover
  - internal/scheduler
tech-stack:
  added: []
  patterns:
    - Pure scheduler timing helpers verified with TDD
    - Deterministic device scheduling spread encoded in small, package-local helpers
key-files:
  created:
    - internal/scheduler/types.go
    - internal/scheduler/types_test.go
    - internal/scheduler/jitter.go
    - internal/scheduler/jitter_test.go
    - .planning/phases/41-jittered-scheduler/41-01-SUMMARY.md
  modified: []
key-decisions:
  - Performance cadence alone honors device.PollIntervalOverride; operational and static stay on shared system intervals.
  - Task identity is the pair of device UUID and volatility class so later backlog coalescing has a stable dedupe key.
  - Deterministic FNV offsets are mapped with hash modulo interval to preserve the required spread test and avoid float precision collapse.
patterns-established:
  - TaskKey is the canonical scheduler identity for one device and one volatility tier.
  - Scheduler timing helpers stay pure and side-effect free so later queue logic can reuse them without per-device goroutines.
requirements-completed: [POLL-02, POLL-04]
duration: 5m13s
completed: 2026-04-12
---

# Phase 41 Plan 01: Jittered Scheduler Summary

**Scheduler task contracts plus deterministic FNV-derived poll spreading and bounded next-fire jitter for volatility-tiered polling**

## Performance

- **Duration:** 5m13s
- **Started:** 2026-04-12T18:37:07+00:00
- **Completed:** 2026-04-12T18:42:20Z
- **Tasks:** 2
- **Files modified:** 5

## Accomplishments
- Added the scheduler package contract layer with `TaskKey`, `PollTask`, `Completion`, `EffectiveInterval`, and `VolatilityPriority`.
- Locked the phase interval policy in one place: only performance tasks use `poll_interval_override`; operational stays at `60s` and static stays at `300s`.
- Added deterministic first-fire offsets and bounded next-fire jitter helpers with tests covering determinism, bounds, and anti-burst distribution.

## Verification Results
- **Determinism:** `TestInitialOffset_DeterministicAndBounded` proves the same UUID and interval always produce the same initial offset, and non-positive intervals return `0`.
- **Bounds:** `TestInitialOffset_DeterministicAndBounded` keeps offsets in `[0, interval)`, and `TestJitteredNext_BoundedToTenPercent` keeps subsequent fires within `90%-110%` of the base interval while `nil` RNG falls back to `lastFire.Add(interval)`.
- **Distribution:** `TestInitialOffset_DistributionAcrossBuckets` spreads 256 deterministic UUIDs across 8 buckets with every bucket between 16 and 48 samples.
- **Override scope:** `TestEffectiveInterval_PerformanceUsesOverride`, `TestEffectiveInterval_OperationalIgnoresOverride`, and `TestEffectiveInterval_StaticIgnoresOverride` confirm only performance scheduling honors `PollIntervalOverride`.
- **Plan verification:** `go test ./internal/scheduler -count=1` and `go build ./...` both passed after implementation.

## Task Commits

Each task was committed atomically:

1. **Task 1: Create scheduler task contracts, interval helpers, and volatility priority ordering** - `f9921ac` (test), `7e220c2` (feat)
2. **Task 2: Implement deterministic initial offsets and bounded next-fire jitter** - `4433bc9` (test), `8f51ee5` (feat)

## Files Created/Modified
- `internal/scheduler/types.go` - Scheduler task identity types plus interval-resolution and priority helpers.
- `internal/scheduler/types_test.go` - TDD coverage for performance override handling and volatility priority ordering.
- `internal/scheduler/jitter.go` - Deterministic initial offset and bounded next-fire jitter primitives.
- `internal/scheduler/jitter_test.go` - Determinism, bounds, RNG, and burst-distribution tests for scheduler timing helpers.
- `.planning/phases/41-jittered-scheduler/41-01-SUMMARY.md` - Execution summary for this plan only; shared GSD state artifacts were intentionally left untouched.

## Decisions Made
- Used `TaskKey{DeviceID, VolatilityClass}` as the exact dedupe identity the later coalescing scheduler can reuse directly.
- Kept `initialOffset` and `jitteredNext` unexported and package-pure so future runtime logic can depend on them without exposing premature API surface.
- Left `.planning/STATE.md` and `.planning/ROADMAP.md` unchanged per executor constraints; only plan-owned files and this summary were modified.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 1 - Bug] Replaced float-scaled offset mapping with modulo-based mapping**
- **Found during:** Task 2 (Implement deterministic initial offsets and bounded next-fire jitter)
- **Issue:** The plan's float64-based offset mapping clustered all 256 deterministic UUID smoke-test samples into one bucket, failing the required anti-burst distribution guarantee and threat mitigation `T-41-01`.
- **Fix:** Kept the required `fnv.New64a()` hash input but mapped the hash with `Sum64() % interval` so deterministic offsets stay inside `[0, interval)` while distributing the required test UUIDs across the full window.
- **Files modified:** `internal/scheduler/jitter.go`
- **Verification:** `go test ./internal/scheduler -run 'TestInitialOffset|TestJitteredNext' -count=1 -v`, `go test ./internal/scheduler -count=1`, and `go build ./...`
- **Committed in:** `8f51ee5`

---

**Total deviations:** 1 auto-fixed (1 bug)
**Impact on plan:** The deviation was necessary for correctness because the literal float64 formula could not satisfy the plan's own distribution requirement. Scope stayed inside the owned scheduler helper.

## Issues Encountered
- The original float-scaled offset mapping produced a deterministic but unusably clustered result for the plan's 256-UUID distribution test. The modulo mapping resolved that without widening the scheduler surface or adding dependencies.

## User Setup Required

None - no external service configuration required.

## Next Phase Readiness
- Later scheduler plans can reuse `TaskKey`, `EffectiveInterval`, `VolatilityPriority`, `initialOffset`, and `jitteredNext` without reopening cadence policy.
- The runtime scheduler still needs queueing, concurrency limiting, and device inventory refresh logic in later Phase 41 plans; those plans can build on pure, tested timing primitives now.

## Self-Check: PASSED
- Found `.planning/phases/41-jittered-scheduler/41-01-SUMMARY.md` and all four scheduler files on disk.
- Verified task commits `f9921ac`, `7e220c2`, `4433bc9`, and `8f51ee5` exist in `git log --oneline --all`.
- Confirmed `go test ./internal/scheduler -count=1` and `go build ./...` passed after the final implementation.
