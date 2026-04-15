---
phase: 40-collectors
plan: 03
subsystem: infra
tags: [snmp, collectors, operational-state, static-discovery, vendor-registry]
requires:
  - phase: 40-collectors
    provides: shared collector result contract, SNMP client constructor type, and performance collector pattern from Plan 01
  - phase: 39-domain-types-db-migration
    provides: tiered operational OID resolution through vendor.Registry
provides:
  - reusable snmp.PollOperationalStatus helper for sysUpTime and ifOperStatus polling
  - OperationalCollector typed wrapper over vendor OID resolution and operational SNMP polling
  - StaticCollector typed wrapper over snmp.DiscoverDevice without service or repository coupling
affects:
  - 40-04-PLAN
  - 41-scheduler
  - 42-pipeline-orchestrator
tech-stack:
  added: []
  patterns:
    - reusable operational SNMP helper preserving nil-per-field partial results
    - typed collector wrappers sharing one SNMP client lifecycle per poll
key-files:
  created:
    - internal/collector/operational.go
    - internal/collector/operational_test.go
    - internal/collector/static.go
    - internal/collector/static_test.go
  modified:
    - internal/snmp/discovery.go
    - internal/snmp/discovery_test.go
key-decisions:
  - "OperationalCollector resolves OIDs only through Registry.ResolveOperationalOIDs and normalizes blank vendor names to default."
  - "StaticCollector remains a pure wrapper around snmp.DiscoverDevice(); static YAML migration stays deferred in this plan."
patterns-established:
  - "Operational polling marks Reachable=true whenever transport and query execution succeed, even if uptime or individual statuses are absent."
  - "Static discovery returns typed inventory and topology results without DeviceService, DeviceRepository, or DB-write collaborators."
requirements-completed: [PIPE-02]
duration: 6 min
completed: 2026-04-12
---

# Phase 40 Plan 03: Collectors Summary

**Operational uptime/status polling plus typed operational and static collectors that wrap existing SNMP helpers without adding scheduler or DB coupling**

## Performance

- **Duration:** 6 min
- **Started:** 2026-04-12T14:16:10Z
- **Completed:** 2026-04-12T14:21:49Z
- **Tasks:** 2
- **Files modified:** 6

## Accomplishments
- Added `snmp.PollOperationalStatus` as a focused helper for uptime and interface operational status, with fallback OIDs, timetick-to-seconds conversion, and partial-result semantics.
- Added `OperationalCollector` and `StaticCollector` as stateless typed wrappers using one SNMP client lifecycle per poll and the shared collector result contract from Plan 01.
- Added unit coverage for operational success, fallback OIDs, partial results, query failures, static discovery wrapping, and the no-service/no-repository boundary on `StaticCollector`.

## Task Commits

Each task was committed atomically via TDD:

1. **Task 1 RED: operational helper tests** - `650b813` (test)
2. **Task 1 GREEN: operational SNMP helper** - `df70a13` (feat)
3. **Task 2 RED: operational/static collector tests** - `422b5b8` (test)
4. **Task 2 GREEN: operational/static collectors** - `25567eb` (feat)

## Files Created/Modified
- `internal/snmp/discovery.go` - Added `PollOperationalStatus` for sysUpTime and ifOperStatus polling with fallback OIDs and partial-result handling.
- `internal/snmp/discovery_test.go` - Added focused tests for success, fallback OIDs, missing fields, and query-error behavior.
- `internal/collector/operational.go` - Added `OperationalCollector` wrapper around vendor OID resolution and `snmp.PollOperationalStatus`.
- `internal/collector/operational_test.go` - Added tests for happy path, partial results, query failures, and `OperationalResult` interface satisfaction.
- `internal/collector/static.go` - Added `StaticCollector` wrapper around `snmp.DiscoverDevice` with typed result copying and no DB/service collaborators.
- `internal/collector/static_test.go` - Added tests for discovery success, discovery failure, `StaticResult` interface satisfaction, and explicit no-collaborator structure checks.

## Verification
- `PATH=/usr/local/go/bin:$PATH rtk go test ./internal/snmp -count=1` - passed
- `PATH=/usr/local/go/bin:$PATH rtk go test ./internal/collector -count=1` - passed
- `PATH=/usr/local/go/bin:$PATH rtk go build ./...` - passed
- `grep -q 'snmp.PollOperationalStatus' internal/collector/operational.go` and `grep -q 'snmp.DiscoverDevice' internal/collector/static.go` - passed
- `! grep -q 'DeviceService' internal/collector/static.go` and `! grep -q 'DeviceRepository' internal/collector/static.go` - passed

## Decisions Made
- Operational collection stays narrow: the helper only polls `sysUpTime` and `ifOperStatus`, and the collector resolves overrides exclusively through `Registry.ResolveOperationalOIDs`.
- Static collection reuses `snmp.DiscoverDevice` directly. Static YAML migration for ifTable/ifXTable/LLDP/CDP remains deferred exactly as Phase 40 context D-04 and D-05 require.
- `StaticCollector` has no `DeviceService`, `DeviceRepository`, or DB-write collaborator in its struct or call path, preserving the plan’s pure-wrapper boundary.

## Deviations from Plan

None - plan executed exactly as written.

## Issues Encountered
- `go` was not on the default shell `PATH` in this environment. Verification commands were run as `PATH=/usr/local/go/bin:$PATH rtk go ...` while keeping the required `rtk` wrapper.

## User Setup Required

None - no external service configuration required.

## Next Phase Readiness
- Phase 40 now has dedicated collectors for all three volatility classes: performance from Plan 01, plus operational and static from this plan.
- Phase 41 scheduler work can consume typed operational and static collectors without adding direct SNMP GET/WALK logic at the scheduler layer.
- Static YAML migration remains deferred; future phases should keep using `snmp.DiscoverDevice` until that migration has its own plan.

## Self-Check: PASSED
- Found summary file: `.planning/phases/40-collectors/40-03-SUMMARY.md`
- Found key files: `internal/snmp/discovery.go`, `internal/snmp/discovery_test.go`, `internal/collector/operational.go`, `internal/collector/operational_test.go`, `internal/collector/static.go`, `internal/collector/static_test.go`
- Found task commits: `650b813`, `df70a13`, `422b5b8`, `25567eb`

---
*Phase: 40-collectors*
*Completed: 2026-04-12*
