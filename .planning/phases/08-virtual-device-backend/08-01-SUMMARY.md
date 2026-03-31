---
phase: 08-virtual-device-backend
plan: 01
subsystem: domain, service, database
tags: [virtual-device, device-type, migration, sqlite, snmp-skip]

# Dependency graph
requires: []
provides:
  - DeviceTypeVirtual domain constant
  - Migration 000009 partial unique index on devices(ip) WHERE ip != ''
  - AddDevice accepts deviceType parameter (12 args)
  - probeDevice virtual device guard (skip SNMP)
  - Service-layer tests for virtual device creation
affects: [08-02, api-layer, poller, metrics-collector]

# Tech tracking
tech-stack:
  added: []
  patterns:
    - "Virtual device probe skip in AddDevice and probeDevice"
    - "Partial unique index for empty-IP coexistence"
    - "deviceType parameter threading through service layer"

key-files:
  created:
    - internal/repository/sqlite/migrations/000009_partial_unique_ip.up.sql
    - internal/repository/sqlite/migrations/000009_partial_unique_ip.down.sql
  modified:
    - internal/domain/device.go
    - internal/service/device_service.go
    - internal/service/device_service_test.go

key-decisions:
  - "Removed IP-required validation from service layer; handler validates conditionally per Plan 02"
  - "Virtual devices start with status unknown; MetricsCollector resolves via probe_success for IP-bearing virtuals"

patterns-established:
  - "DeviceTypeVirtual skip pattern: check deviceType before launching probe"
  - "initialStatus variable pattern: compute status before struct construction"

requirements-completed: [VIRT-01, VIRT-02, VIRT-03, VIRT-04]

# Metrics
duration: 3min
completed: 2026-03-31
---

# Phase 8 Plan 1: Virtual Device Backend Foundation Summary

**DeviceTypeVirtual constant, partial-unique-IP migration, and AddDevice/probeDevice virtual skip with 14 passing service tests**

## Performance

- **Duration:** 3 min
- **Started:** 2026-03-31T19:44:58Z
- **Completed:** 2026-03-31T19:48:30Z
- **Tasks:** 2
- **Files modified:** 5

## Accomplishments
- Added DeviceTypeVirtual constant to domain package, enabling virtual device type throughout the system
- Created migration 000009 that converts devices(ip) unique index to partial unique index WHERE ip != '', allowing multiple virtual devices with empty IP
- Updated AddDevice to accept deviceType parameter, skip SNMP probe for virtual devices, and set initial status to "unknown"
- Added probeDevice virtual guard that short-circuits before any SNMP or Prometheus logic
- All 14 service tests pass including 3 new virtual device tests

## Task Commits

Each task was committed atomically:

1. **Task 1: Domain constant and database migration** - `ae80f2e` (feat)
2. **Task 2: Service layer AddDevice signature change, virtual probe skip, and tests** - `8b47068` (feat)

## Files Created/Modified
- `internal/domain/device.go` - Added DeviceTypeVirtual constant to DeviceType enum
- `internal/repository/sqlite/migrations/000009_partial_unique_ip.up.sql` - Partial unique index allowing empty IP coexistence
- `internal/repository/sqlite/migrations/000009_partial_unique_ip.down.sql` - Rollback to full unique index
- `internal/service/device_service.go` - AddDevice with deviceType param, virtual skip, probeDevice guard
- `internal/service/device_service_test.go` - Updated all calls to 12-arg signature, added 3 new virtual device tests

## Decisions Made
- Removed the `if ip == ""` validation from the service layer AddDevice method. The handler will now validate IP conditionally based on device type (Plan 02 responsibility).
- Virtual devices with IP start as "unknown" rather than "up" to let MetricsCollector resolve status via probe_success on the next cycle (per D-05/D-07).

## Deviations from Plan

None - plan executed exactly as written.

## Known Stubs

None - all code paths are fully wired.

## Issues Encountered

None.

## User Setup Required

None - no external service configuration required.

## Next Phase Readiness
- API callers (device_handler.go HandleCreate and HandleBatchAdd) need updating to pass deviceType parameter - Plan 02 scope
- API and worker tests will temporarily fail to compile until Plan 02 updates those callers
- Domain constant and service contract ready for downstream consumption

## Self-Check: PASSED

All 5 created/modified files verified on disk. Both task commits (ae80f2e, 8b47068) found in git log.

---
*Phase: 08-virtual-device-backend*
*Completed: 2026-03-31*
