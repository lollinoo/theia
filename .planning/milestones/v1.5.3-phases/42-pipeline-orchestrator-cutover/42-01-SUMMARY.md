---
phase: 42-pipeline-orchestrator-cutover
plan: 01
subsystem: backend
tags: [go, state-engine, collectors, snmp, websocket]
requires:
  - phase: 38-state-engine
    provides: "Thread-safe device state storage, change emission, and health evaluation"
  - phase: 40-collectors
    provides: "Typed performance and operational collector results with store adapters"
provides:
  - "Tier-aware store merge rules keyed by domain.VolatilityClass"
  - "Live link-metric storage and cloning in the state engine"
  - "Collector adapters that stamp store updates with explicit volatility classes"
affects: [pipeline-orchestrator, websocket-overview, phase-42-cutover]
tech-stack:
  added: []
  patterns: [tier-aware store merges, defensive slice cloning at runtime boundaries]
key-files:
  created: []
  modified:
    - internal/state/store.go
    - internal/state/store_test.go
    - internal/collector/results.go
    - internal/collector/results_test.go
key-decisions:
  - "Operational updates now own reachability, consecutive-failure tracking, and uptime while preserving last-known performance and link data."
  - "Performance updates merge CPU, memory, temperature, and non-empty link metrics without clearing last-known values on failed polls."
  - "No additional store fields were needed beyond VolatilityClass and LinkMetrics; compatibility for unstamped callers is handled by a temporary legacy update path."
patterns-established:
  - "Store.Update() routes explicit performance, operational, and static updates through dedicated helper functions."
  - "Link metrics are cloned on ingress and egress so collector/broadcaster code cannot alias mutable runtime slices."
requirements-completed: [PIPE-03]
duration: 6 min
completed: 2026-04-13
---

# Phase 42 Plan 01: Tier-Aware Store Contract Summary

**Tier-aware state-store merges with last-known performance/link preservation and collector volatility-stamped updates**

## Performance

- **Duration:** 6 min
- **Started:** 2026-04-13T07:33:56Z
- **Completed:** 2026-04-13T07:40:15Z
- **Tasks:** 2
- **Files modified:** 4

## Accomplishments
- Added a volatility-aware store contract so performance, operational, and static polls no longer clobber each other during the Phase 42 cutover.
- Preserved last-known overview metrics and link throughput across operational polls and failed performance polls, with deep-copy protection for link slices.
- Stamped collector `ToStoreUpdate()` adapters with explicit volatility classes so future pipeline wiring can call one store API safely.

## Task Commits

Each task was committed atomically:

1. **Task 1: Add tier-aware state-store merge rules for performance, operational, and static updates** - `9d7a000` (test), `1a9e32c` (feat)
2. **Task 2: Stamp collector store updates with volatility class and preserve the tier contract in tests** - `1d25b48` (test), `bd84b65` (feat)

## Merge Rules Implemented

- **Performance success:** merges CPU, memory, temperature, and `CollectedAt` from the incoming metrics payload; preserves existing uptime unless the new payload includes `UptimeSecs`; replaces stored `LinkMetrics` only when the incoming slice is non-empty; recomputes health from the merged metric set.
- **Performance failure:** updates poll timing metadata only; keeps prior device metrics, severities, health, and link metrics intact so transient misses do not blank overview state.
- **Operational success/failure:** owns reachability and consecutive-failure transitions; updates only `UptimeSecs` from the incoming metrics payload; leaves CPU, memory, temperature, and link metrics untouched.
- **Static updates:** rely on `LastPolledAt` / `ExpectedInterval` / `Stale` bookkeeping in `Store.Update()` and do not overwrite live overview metrics or stored link throughput.

## Tests Added

- `TestStoreUpdate_PerformanceThenOperationalPreservesPerformanceMetrics`
- `TestStoreUpdate_FailedPerformancePollKeepsLastKnownMetricsAndLinks`
- `TestStoreSnapshot_ClonesLinkMetrics`
- `TestStoreUpdate_LinkMetricDiffTriggersChange`
- `TestPerformanceResultToStoreUpdateSetsPerformanceVolatility`
- `TestOperationalResultToStoreUpdateSetsOperationalVolatility`

These tests prove failed performance polls preserve last-known device metrics and link metrics, operational polls only update uptime/reachability, snapshot readers cannot mutate stored link slices, and collector adapters stamp the expected volatility tier.

## Files Created/Modified

- `internal/state/store.go` - Added `VolatilityClass`/`LinkMetrics`, tier-aware merge helpers, legacy fallback compatibility, and link-metric cloning/equality helpers.
- `internal/state/store_test.go` - Added cross-tier regression coverage for performance preservation, failed-performance retention, snapshot cloning, and link-metric diff emission.
- `internal/collector/results.go` - Stamped performance and operational store updates with explicit volatility classes.
- `internal/collector/results_test.go` - Added adapter tests that assert the volatility marker in addition to the existing payload checks.

## Additional Store Fields

None beyond `VolatilityClass` and `LinkMetrics`. The cutover semantics fit inside the existing state-store shape once merge ownership moved into helper functions.

## Decisions Made

- Kept `CollectedAt` tied to performance samples so operational uptime updates do not make last-known performance data look fresher than it is.
- Cloned link metrics on both write and read boundaries; the plan only required snapshot/get-device cloning, but cloning on ingress closes the same aliasing risk at the collector-to-store boundary.
- Preserved a legacy empty-volatility path so existing pre-cutover callers and Phase 38 tests still behave correctly until later Phase 42 plans wire explicit tiers everywhere.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 3 - Blocking] Preserved a legacy fallback for unstamped state updates**
- **Found during:** Task 1 (Add tier-aware state-store merge rules for performance, operational, and static updates)
- **Issue:** Existing store callers and tests still construct `StateUpdate{...}` without `VolatilityClass`, which would have broken pre-cutover behavior before Task 2 and later Phase 42 plans finish the migration.
- **Fix:** Added `applyLegacyUpdate()` so empty-volatility updates retain the original Phase 38 semantics, while explicit `performance`, `operational`, and `static` updates use the new tier-aware helpers.
- **Files modified:** `internal/state/store.go`
- **Verification:** `go test ./internal/state -count=1`
- **Committed in:** `1a9e32c`

---

**Total deviations:** 1 auto-fixed (1 blocking)
**Impact on plan:** The compatibility path avoids breaking the current runtime/test surface while still landing the new tier contract exactly where Phase 42 needs it.

## Issues Encountered

None

## User Setup Required

None - no external service configuration required.

## Next Phase Readiness

- Ready for the next Phase 42 plans to wire the pipeline orchestrator into the tier-aware store contract.
- Collector outputs now carry the volatility metadata the runtime needs, and the state store preserves overview metrics/link throughput until staleness logic says otherwise.

## Self-Check: PASSED

- Summary file exists at `.planning/phases/42-pipeline-orchestrator-cutover/42-01-SUMMARY.md`.
- Verified task commits `9d7a000`, `1a9e32c`, `1d25b48`, and `bd84b65` exist in `git log --oneline --all`.

---
*Phase: 42-pipeline-orchestrator-cutover*
*Completed: 2026-04-13*
