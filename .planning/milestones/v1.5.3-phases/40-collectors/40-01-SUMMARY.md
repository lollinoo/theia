---
phase: 40-collectors
plan: 01
subsystem: infra
tags: [snmp, collectors, pipeline, state-engine, vendor-registry]
requires:
  - phase: 38-state-engine
    provides: current state.StateUpdate shape and store adapter target
  - phase: 39-domain-types-db-migration
    provides: domain.VolatilityClass constants and vendor performance OID resolution
provides:
  - shared collector result interface with typed performance, operational, and static results
  - reusable SNMP client contract for future collector plans
  - SNMP-primary PerformanceCollector with partial-result semantics and raw counter snapshots
affects:
  - 40-02-PLAN
  - 40-03-PLAN
  - 40-04-PLAN
  - 41-scheduler
  - 42-pipeline-orchestrator
tech-stack:
  added: []
  patterns:
    - stateless collector results with optional current-store adapters
    - SNMP-primary performance polling with zero-value enrichment contract
key-files:
  created:
    - internal/collector/results.go
    - internal/collector/results_test.go
    - internal/collector/performance.go
    - internal/collector/performance_test.go
  modified: []
key-decisions:
  - "PerformanceCollector remains SNMP-authoritative; PrometheusEnrichment stays zero-value in this plan."
  - "Interface speed metadata is copied by exact IfName first, then IfDescr, and defaults to 0 when discovery data is missing."
patterns-established:
  - "Collector result types satisfy a shared GetDeviceID/GetVolatilityClass/GetCollectedAt contract."
  - "Performance polling returns raw counters plus discovered speed metadata without retaining collector state."
requirements-completed: [PIPE-01, PIPE-02]
duration: 7 min
completed: 2026-04-12
---

# Phase 40 Plan 01: Collectors Summary

**Typed collector contracts plus an SNMP-primary performance collector that returns best-effort metrics, raw counters, and future-facing enrichment placeholders**

## Performance

- **Duration:** 7 min
- **Started:** 2026-04-12T13:50:49Z
- **Completed:** 2026-04-12T13:57:26Z
- **Tasks:** 2
- **Files modified:** 4

## Accomplishments
- Added `internal/collector` with a shared `StateUpdate` contract, reusable SNMP client function type, and typed performance/operational/static result structs.
- Added current-store adapters only where the Phase 38 store can already consume the data: performance and operational results.
- Implemented `PerformanceCollector` as an SNMP-primary poller that resolves vendor OIDs, preserves nil-per-field partial results, and returns raw counter snapshots with discovered speed metadata.

## Task Commits

Each task was committed atomically via TDD:

1. **Task 1 RED: collector result contract tests** - `8524c71` (test)
2. **Task 1 GREEN: collector result contract and adapters** - `56992c9` (feat)
3. **Task 2 RED: performance collector tests** - `cdc4e2d` (test)
4. **Task 2 GREEN: SNMP-primary performance collector** - `d0938ac` (feat)

## Files Created/Modified
- `internal/collector/results.go` - Shared collector contract, typed result structs, SNMP client contract, counter snapshot type, and store adapters.
- `internal/collector/results_test.go` - TDD coverage for interface satisfaction, SNMP client constructor signature, volatility classes, and store adapter behavior.
- `internal/collector/performance.go` - Stateless `PerformanceCollector` that resolves vendor OIDs, polls SNMP metrics/counters, and stamps typed results.
- `internal/collector/performance_test.go` - TDD coverage for happy path, partial results, connect failure, and zero-value enrichment behavior.

## Verification
- `PATH=/usr/local/go/bin:$PATH rtk go test ./internal/collector -count=1` - passed
- `PATH=/usr/local/go/bin:$PATH rtk go build ./...` - passed
- `grep -q 'type StateUpdate interface' internal/collector/results.go` and grep checks for `PerformanceCollector`, `snmp.PollDeviceMetrics`, and `snmp.PollInterfaceCounters` - passed

## Decisions Made
- SNMP remained authoritative in this plan. `PerformanceCollector` only resolves performance OIDs and calls `snmp.PollDeviceMetrics` plus `snmp.PollInterfaceCounters`; it does not query Prometheus.
- `PrometheusEnrichment` was carried forward as a zero-value contract only, preserving the later-plan write boundary.
- Counter snapshots include interface speed metadata from discovered device interfaces so downstream rate code can enforce the 110% sanity bound without hidden collector state.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 2 - Missing Critical] Guarded collector wiring failures instead of panicking**
- **Found during:** Task 2 (Implement an SNMP-primary PerformanceCollector)
- **Issue:** A nil collector, nil registry, nil SNMP factory, or pre-canceled context would have crashed or performed wasted work instead of returning a typed error result.
- **Fix:** Added defensive checks in `PerformanceCollector.Poll()` so invalid wiring and canceled contexts set `result.Err` and return without fabricating metrics or counters.
- **Files modified:** `internal/collector/performance.go`
- **Verification:** `PATH=/usr/local/go/bin:$PATH rtk go test ./internal/collector -count=1`; `PATH=/usr/local/go/bin:$PATH rtk go build ./...`
- **Committed in:** `d0938ac`

---

**Total deviations:** 1 auto-fixed (Rule 2: 1)
**Impact on plan:** Improved correctness at the collector seam without widening scope or changing the plan's public contract.

## Issues Encountered
- `go` was not on the default shell `PATH` in this execution environment. Verification commands were run with `PATH=/usr/local/go/bin:$PATH` while still using the required `rtk` wrapper.
- The initial RED test assumed a deterministic SNMP counter order. The finalized test asserts counter speed metadata by interface name instead, which matches the underlying helper behavior.

## User Setup Required

None - no external service configuration required.

## Next Phase Readiness
- The shared collector package now exists as the Phase 40 write root for later collector plans.
- Phase 40 follow-up plans can reuse the common SNMP client contract and result interface without overlapping these write sets.
- The Phase 38 store remains unchanged; compatibility is handled strictly through adapters in `internal/collector/results.go`.

## Self-Check: PASSED
- Found summary file: `.planning/phases/40-collectors/40-01-SUMMARY.md`
- Found task commits: `8524c71`, `56992c9`, `cdc4e2d`, `d0938ac`

---
*Phase: 40-collectors*
*Completed: 2026-04-12*
