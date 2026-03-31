---
phase: 08-virtual-device-backend
plan: 02
subsystem: api, worker
tags: [virtual-device, device-handler, link-handler, poller, validation]

# Dependency graph
requires:
  - "08-01"
provides:
  - Virtual device creation via POST /api/v1/devices with device_type=virtual
  - Virtual-side if_name relaxation in link creation
  - Both-virtual link rejection
  - Poller virtual device skip
affects: [frontend-api-client, metrics-collector]

# Tech tracking
tech-stack:
  added: []
  patterns:
    - "Virtual device validation branch in HandleCreate"
    - "Virtual-aware if_name relaxation in link handler"
    - "Defense-in-depth virtual skip in poller (in addition to probeDevice guard)"

key-files:
  created: []
  modified:
    - internal/api/device_handler.go
    - internal/api/device_handler_test.go
    - internal/api/link_handler.go
    - internal/api/link_handler_test.go
    - internal/worker/poller.go
    - internal/worker/poller_test.go

key-decisions:
  - "Virtual device creation uses early-return branch before regular IP validation"
  - "Both-virtual link rejection checked before if_name validation for clear error messages"
  - "Poller skip is defense-in-depth with probeDevice guard from Plan 01"

patterns-established:
  - "Virtual branch pattern: check deviceType early, return before regular validation"
  - "Device type fetching in link handler: get both devices before field validation"

requirements-completed: [VIRT-05]

# Metrics
duration: 6min
completed: 2026-03-31
---

# Phase 8 Plan 2: Virtual Device API Handlers and Poller Skip Summary

**Virtual device creation via API with tag validation, virtual-side link if_name relaxation, and poller exclusion with 37 passing tests across api and worker packages**

## Performance

- **Duration:** 6 min
- **Started:** 2026-03-31T19:52:09Z
- **Completed:** 2026-03-31T19:58:24Z
- **Tasks:** 3
- **Files modified:** 6

## Accomplishments
- Added DeviceType field to createDeviceRequest and virtual validation branch in HandleCreate (display_name, virtual_subtype, optional IP)
- Updated HandleCreate and HandleBatchAdd to call AddDevice with 12-arg signature including deviceType parameter
- Updated link handler HandleCreate to fetch both devices before if_name validation, allowing empty if_name for virtual side while rejecting both-virtual links
- Added virtual device skip in poller pollAllDevices loop as defense-in-depth with probeDevice guard
- Added 7 new device handler tests, 4 new link handler tests, and 1 new poller test -- all passing

## Task Commits

Each task was committed atomically:

1. **Task 1: Device handler virtual validation, batch signature fix, and handler tests** - `e0ef434` (feat)
2. **Task 2: Link handler virtual validation and link handler tests** - `55a032a` (feat)
3. **Task 3: Poller virtual device skip and poller test** - `c5d0689` (feat)

## Files Created/Modified
- `internal/api/device_handler.go` - Added DeviceType field to request struct, virtual validation branch in HandleCreate, 12-arg AddDevice calls in both HandleCreate and HandleBatchAdd
- `internal/api/device_handler_test.go` - Added 7 virtual device tests (happy path, with IP, missing display_name, invalid subtype, missing subtype, no tags, regular IP regression)
- `internal/api/link_handler.go` - Replaced unconditional if_name checks with virtual-aware validation (fetch devices, check DeviceType, reject both-virtual, relax if_name for virtual side)
- `internal/api/link_handler_test.go` - Added seedVirtualDevice helper and 4 virtual link tests (source virtual, target virtual, both virtual rejected, both physical still requires if_names)
- `internal/worker/poller.go` - Added DeviceTypeVirtual skip in pollAllDevices loop after Managed check
- `internal/worker/poller_test.go` - Added TestPollAllDevices_SkipsVirtualDevices with real service, mock repos, and discoverFn counter

## Decisions Made
- Virtual device creation uses an early-return branch that exits before SNMP parsing, SSH profile lookup, and regular IP validation
- Link handler fetches both devices upfront (replacing the old separate existence check) to get DeviceType for virtual-aware validation
- Poller virtual skip is defense-in-depth alongside the probeDevice virtual guard from Plan 01

## Deviations from Plan

None -- plan executed exactly as written.

## Known Stubs

None -- all code paths are fully wired.

## Issues Encountered

None.

## User Setup Required

None -- no external service configuration required.

## Next Phase Readiness
- Backend virtual device story is complete (domain, service, API, poller)
- Frontend can now POST virtual devices and create links with virtual sides
- MetricsCollector integration (virtual device IPs in probe_success query) is separate scope

## Self-Check: PASSED

All 6 modified files verified on disk. All 3 task commits (e0ef434, 55a032a, c5d0689) found in git log.

---
*Phase: 08-virtual-device-backend*
*Completed: 2026-03-31*
