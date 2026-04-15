---
phase: 42-pipeline-orchestrator-cutover
plan: 02
subsystem: api
tags: [go, snmp, topology, pipeline]
requires:
  - phase: 40-collectors
    provides: Static collector inventory and topology result shape reused by the persistence seam
provides:
  - Shared static discovery persistence helper for metadata, interfaces, and links
  - Legacy probe path reuse of ApplyStaticDiscovery with caller-owned topology notifications
  - Regression coverage for interface-driven topology change reporting
affects: [42-pipeline-orchestrator-cutover, internal/worker/pipeline.go, topology broadcast]
tech-stack:
  added: []
  patterns: [service-owned static discovery persistence, caller-owned topology notify gating]
key-files:
  created: [internal/service/static_persistence.go, internal/service/static_persistence_test.go]
  modified: [internal/service/device_service.go]
key-decisions:
  - "Static discovery persistence now lives in DeviceService.ApplyStaticDiscovery so legacy probing and the future orchestrator share one topology-write seam."
  - "ApplyStaticDiscovery reports TopologyChanged but never writes TopologyNotify; probeDevice remains the caller-owned notification point."
patterns-established:
  - "Static collector output is converted into StaticDiscoveryInput before persistence."
  - "TopologyChanged becomes true for interface-set mutations (count/name/descr/speed) as well as newly created links."
requirements-completed: [PIPE-03]
duration: 8m53s
completed: 2026-04-13
---

# Phase 42 Plan 02: Static Discovery Persistence Summary

**Shared static discovery persistence seam with caller-owned topology notifications for legacy probe and orchestrator reuse**

## Performance

- **Duration:** 8m53s
- **Started:** 2026-04-13T07:46:27Z
- **Completed:** 2026-04-13T07:55:20Z
- **Tasks:** 2
- **Files modified:** 3

## Accomplishments

- Added `DeviceService.ApplyStaticDiscovery` to persist discovered metadata, interfaces, and deduped neighbors from a service-owned input contract.
- Moved `probeDevice()` off its duplicated topology persistence path and onto the shared helper while keeping the non-blocking `TopologyNotify` send in the caller.
- Added regression coverage proving interface-only topology mutations now surface as `TopologyChanged` and that the helper itself never emits notify signals.

## Task Commits

Each task was committed atomically:

1. **Task 1: Extract reusable static discovery persistence helper** - `25854dc` (test), `6c5c618` (feat)
2. **Task 2: Rewrite probeDevice to use the shared helper and keep caller-owned topology notifications** - `2cfb3db` (test), `baddaf7` (feat)

## Files Created/Modified

- `internal/service/static_persistence.go` - shared persistence seam for static collector output
- `internal/service/static_persistence_test.go` - helper and probe-path regression coverage
- `internal/service/device_service.go` - legacy probe path refactored to use `ApplyStaticDiscovery`

## Decisions Made

- Shared topology writes behind `ApplyStaticDiscovery` instead of duplicating the legacy `probeDevice` metadata/interface/link logic in the upcoming orchestrator path.
- Kept topology notification ordering caller-owned: the helper only reports `TopologyChanged`, and `probeDevice` still performs the non-blocking send after persistence succeeds.
- Treated interface-set changes as topology mutations when interface count, name, description, or speed differ from the persisted snapshot.

## Deviations from Plan

None - plan executed exactly as written.

## Issues Encountered

None.

## User Setup Required

None - no external service configuration required.

## Next Phase Readiness

- `internal/worker/pipeline.go` can now persist static collector results through `ApplyStaticDiscovery` without depending on the legacy poller path.
- Topology broadcast ordering remains aligned with D-04: persistence can report change, but the caller still owns when `topology_changed` is signaled.

## Self-Check: PASSED

- Found `internal/service/static_persistence.go`
- Found `internal/service/static_persistence_test.go`
- Found `.planning/phases/42-pipeline-orchestrator-cutover/42-02-SUMMARY.md`
- Verified commits `25854dc`, `6c5c618`, `2cfb3db`, and `baddaf7` in `git log --oneline --all`
