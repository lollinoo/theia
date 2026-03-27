---
phase: 03-area-backend-and-management
plan: 01
subsystem: api, database
tags: [go, sqlite, rest-api, area-crud, migration, foreign-key]

# Dependency graph
requires: []
provides:
  - "Area domain types (Area, AreaWithCount, AreaRepository interface)"
  - "SQLite migration 000007 with areas table and devices.area_id FK"
  - "Area CRUD REST API at /api/v1/areas"
  - "Device area_id assignment via existing PUT /api/v1/devices/{id}"
  - "Device responses include area_id when assigned"
affects: [03-area-backend-and-management/plan-02, 04-area-hub-frontend]

# Tech tracking
tech-stack:
  added: []
  patterns: [area-repository-pattern, double-pointer-uuid-assignment, on-delete-set-null-fk]

key-files:
  created:
    - internal/domain/area.go
    - internal/repository/sqlite/area_repo.go
    - internal/repository/sqlite/area_repo_test.go
    - internal/repository/sqlite/migrations/000007_areas.up.sql
    - internal/repository/sqlite/migrations/000007_areas.down.sql
    - internal/api/area_handler.go
    - internal/api/area_handler_test.go
  modified:
    - internal/domain/device.go
    - internal/repository/sqlite/device_repo.go
    - internal/service/device_service.go
    - internal/api/device_handler.go
    - internal/api/device_handler_test.go
    - internal/api/router.go
    - cmd/theia/main.go

key-decisions:
  - "Area repo follows same patterns as SNMP profile repo (no encryption needed, no onChange channel)"
  - "Device repo area_id changes included in Task 1 commit (Rule 3 deviation) since area repo tests depend on device repo handling area_id"

patterns-established:
  - "Area CRUD handler pattern: same as SNMPProfileHandler with mock repo tests"
  - "Double-pointer UUID pattern for area_id assignment in DeviceUpdate (nil=not set, *nil=unassign, **=set)"
  - "ON DELETE SET NULL FK pattern for area-device relationship"

requirements-completed: [AREA-01, AREA-02, AREA-03, AREA-04]

# Metrics
duration: 8min
completed: 2026-03-26
---

# Phase 3 Plan 1: Area Backend Summary

**Complete area CRUD REST API with SQLite persistence, device-to-area FK assignment, ON DELETE SET NULL cascade, and 14 passing behavioral tests**

## Performance

- **Duration:** 8 min
- **Started:** 2026-03-26T09:48:47Z
- **Completed:** 2026-03-26T09:56:23Z
- **Tasks:** 3
- **Files modified:** 14

## Accomplishments
- Area domain types, repository interface, and SQLite implementation with full CRUD
- Migration 000007 creates areas table with UNIQUE name index and adds area_id FK to devices with ON DELETE SET NULL
- Area REST API at /api/v1/areas with list (with device_count), create (with validation/defaults), get, update, delete
- Device update endpoint accepts area_id for assignment/unassignment via double-pointer pattern
- Device GET/LIST responses include area_id when assigned
- 14 new tests: 6 area repo tests + 7 area handler tests + 1 device area update test

## Task Commits

Each task was committed atomically:

1. **Task 1: Area domain, migration, repository, and repo tests** - `07f07dd` (feat)
2. **Task 2: Device service AreaID in DeviceUpdate struct** - `2ef2400` (feat)
3. **Task 3: Area API handler, router wiring, main.go integration, and handler tests** - `07129cc` (feat)

## Files Created/Modified
- `internal/domain/area.go` - Area struct, AreaWithCount, AreaRepository interface
- `internal/domain/device.go` - Added AreaID field to Device struct
- `internal/repository/sqlite/migrations/000007_areas.up.sql` - areas table, unique index, devices.area_id FK
- `internal/repository/sqlite/migrations/000007_areas.down.sql` - Rollback migration
- `internal/repository/sqlite/area_repo.go` - SQLite AreaRepo with CRUD and GetAllWithDeviceCount
- `internal/repository/sqlite/area_repo_test.go` - 6 integration tests using real SQLite
- `internal/repository/sqlite/device_repo.go` - Added area_id to all SELECTs, INSERT, UPDATE, and scan functions
- `internal/service/device_service.go` - Added AreaID double-pointer to DeviceUpdate
- `internal/api/area_handler.go` - HTTP handler for area CRUD with validation
- `internal/api/area_handler_test.go` - 7 handler tests with mock repo
- `internal/api/device_handler.go` - Added area_id to update request and device response
- `internal/api/device_handler_test.go` - Added device area_id update test
- `internal/api/router.go` - Added areaRepo parameter and /api/v1/areas routes
- `cmd/theia/main.go` - Wired AreaRepo into router

## Decisions Made
- Area repo follows same patterns as SNMP profile repo (no encryption needed for area data, no onChange channel for cache invalidation since areas change infrequently)
- Device repo area_id changes included in Task 1 commit since area repo tests create devices with area_id and would not compile without the device repo handling area_id column

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 3 - Blocking] Moved device_repo.go area_id changes from Task 2 to Task 1**
- **Found during:** Task 1 (area repo tests)
- **Issue:** Area repo tests (GetAllWithDeviceCount, DeleteSetsDeviceAreaIDToNull) create devices with AreaID set, which requires device_repo.go to handle the area_id column in INSERT/SELECT/UPDATE. Without this, tests would not compile.
- **Fix:** Included all device_repo.go area_id changes (4 SELECT queries, 2 scan functions, INSERT, UPDATE) in the Task 1 commit instead of Task 2
- **Files modified:** internal/repository/sqlite/device_repo.go
- **Verification:** All 6 area repo tests and all 10 existing device repo tests pass
- **Committed in:** 07f07dd (Task 1 commit)

---

**Total deviations:** 1 auto-fixed (1 blocking)
**Impact on plan:** Necessary reordering to satisfy compile-time dependency. Task 2 still added the service layer AreaID field as planned. No scope creep.

## Issues Encountered
None

## User Setup Required
None - no external service configuration required.

## Next Phase Readiness
- Area CRUD API fully functional, ready for Phase 3 Plan 02 (frontend area management UI)
- Area data layer ready for Phase 4 (Area Hub frontend with filtering)
- All 10 internal packages pass tests with zero regressions

## Self-Check: PASSED

All 7 created files verified present. All 3 task commit hashes verified in git log.

---
*Phase: 03-area-backend-and-management*
*Completed: 2026-03-26*
