---
phase: 01-foundation
plan: 03
subsystem: api
tags: [rest-api, device-service, snmp-probing, background-poller, graceful-shutdown]

# Dependency graph
requires:
  - phase: 01-foundation-01
    provides: "Domain model, SQLite repositories, config system"
  - phase: 01-foundation-02
    provides: "SNMP client, device discovery, device type detection"
provides:
  - "DeviceService orchestrating SNMP discovery + repository persistence"
  - "REST API at /api/v1/ with device CRUD, batch add, links, settings, health"
  - "Background SNMP poller with configurable interval and worker pool"
  - "Fully wired main.go with graceful shutdown"
affects: [02-api, 02-frontend, 03-metrics]

# Tech tracking
tech-stack:
  added: []
  patterns: [DiscoverFunc abstraction for testability, async probe with WaitGroup, semaphore worker pool, JSON:API response format]

key-files:
  created:
    - internal/service/device_service.go
    - internal/service/device_service_test.go
    - internal/api/router.go
    - internal/api/device_handler.go
    - internal/api/settings_handler.go
    - internal/api/health_handler.go
    - internal/api/middleware.go
    - internal/worker/poller.go
  modified:
    - cmd/theia/main.go
    - .gitignore

key-decisions:
  - "DiscoverFunc abstraction instead of raw SNMP client interface for simpler mock testing"
  - "Re-fetch device from repo in probeDevice goroutine to avoid data races on shared pointer"
  - "JSON:API response format with type/id/attributes/relationships structure"

patterns-established:
  - "DiscoverFunc pattern: pass function instead of client interface for integration-level abstraction"
  - "Async probe: AddDevice returns immediately, goroutine probes and updates via repo"
  - "Semaphore worker pool: bounded goroutine concurrency for polling"

requirements-completed: [DEV-01, DEV-02, DEV-05, DEV-06, INTG-04, INTG-05]

# Metrics
duration: 9min
completed: 2026-03-06
---

# Phase 1 Plan 03: REST API and Service Wiring Summary

**Device service with async SNMP probing, full REST API at /api/v1/ with JSON:API format, background poller, and graceful shutdown wiring in main.go**

## Performance

- **Duration:** 9 min
- **Started:** 2026-03-06T14:18:54Z
- **Completed:** 2026-03-06T14:27:48Z
- **Tasks:** 2
- **Files modified:** 10

## Accomplishments
- DeviceService orchestrates async SNMP probing: AddDevice returns immediately with status "probing", background goroutine discovers sysInfo, interfaces, and LLDP/CDP neighbors
- Neighbor discovery creates unmanaged placeholder devices without duplicates, with links upserted between source and neighbor
- Full REST API with 11 endpoints: device CRUD, batch add, re-probe, links list, settings GET/PUT, health check
- Background poller re-probes all managed devices on configurable interval with bounded worker pool
- main.go fully wired: config, DB, migrations, repos, SNMP discover func, service, poller, router, graceful shutdown on SIGINT/SIGTERM
- 9 service tests passing with race detector enabled

## Task Commits

Each task was committed atomically:

1. **Task 1: Device service layer with async probing and neighbor handling** - `973a77d` (feat, TDD)
2. **Task 2: REST API handlers, background poller, and main.go wiring** - `16ce4e7` (feat)

## Files Created/Modified
- `internal/service/device_service.go` - Service layer orchestrating SNMP discovery + repository persistence
- `internal/service/device_service_test.go` - 9 tests covering add, probe, fail, neighbors, update, delete, get-all, re-probe
- `internal/api/router.go` - HTTP router with all /api/v1/ routes using net/http ServeMux
- `internal/api/device_handler.go` - Device CRUD handlers with JSON:API response format
- `internal/api/settings_handler.go` - Settings GET/PUT handlers
- `internal/api/health_handler.go` - Health endpoint reporting db and snmp_poller status
- `internal/api/middleware.go` - CORS, request logging, JSON content-type middleware
- `internal/worker/poller.go` - Background poller with semaphore-bounded worker pool
- `cmd/theia/main.go` - Fully wired entry point with graceful shutdown
- `.gitignore` - Fixed pattern to match only root binary, not cmd/theia directory

## Decisions Made
- Used DiscoverFunc abstraction (function type) instead of raw SNMP client interface -- simpler to mock, hides connect/close lifecycle
- Re-fetch device from repo inside probeDevice goroutine to avoid data race on the pointer returned to AddDevice caller
- JSON:API response format with nested interfaces as relationships
- Standard net/http ServeMux routing (no external framework needed at this scale)

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 1 - Bug] Fixed data race in async probeDevice**
- **Found during:** Task 1 (race detector flagged during tests)
- **Issue:** probeDevice goroutine mutated the same *domain.Device pointer that was returned to AddDevice caller, causing a data race
- **Fix:** probeDevice now re-fetches device from repository before mutating fields
- **Files modified:** internal/service/device_service.go
- **Verification:** All tests pass with -race flag
- **Committed in:** 973a77d

**2. [Rule 3 - Blocking] Fixed .gitignore pattern blocking cmd/theia directory**
- **Found during:** Task 2 (git add failed for cmd/theia/main.go)
- **Issue:** `.gitignore` had `theia` which matched the `cmd/theia` directory, preventing staging
- **Fix:** Changed to `/theia` to match only the root-level build binary
- **Files modified:** .gitignore
- **Verification:** git add succeeds for cmd/theia/main.go
- **Committed in:** 16ce4e7

---

**Total deviations:** 2 auto-fixed (1 bug, 1 blocking)
**Impact on plan:** Both fixes necessary for correctness and ability to commit. No scope creep.

## Issues Encountered
- Pre-existing `TestDeviceRepo_GetAll` failure in repository tests (missing interfaces table in test setup) -- out of scope, not caused by this plan's changes

## User Setup Required
None - no external service configuration required.

## Next Phase Readiness
- Phase 1 complete: domain model + SNMP discovery + REST API + service layer + persistence + background poller
- All components wired and running via `go run ./cmd/theia/`
- Ready for Phase 2: frontend integration consuming the REST API
- SNMP discovery works end-to-end with Docker SNMP simulators

---
*Phase: 01-foundation*
*Completed: 2026-03-06*
