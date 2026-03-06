---
phase: 01-foundation
plan: 02
subsystem: snmp
tags: [gosnmp, snmp, discovery, lldp, cdp, device-detection]

# Dependency graph
requires:
  - phase: 01-foundation-01
    provides: "Domain model with Device, Interface, SNMPCredentials, DeviceType types"
provides:
  - "SNMP client abstraction (v2c + v3) via gosnmp"
  - "Device discovery: sysInfo, interfaces, LLDP/CDP neighbors"
  - "Device type auto-detection for MikroTik, Cisco, Ubiquiti"
affects: [01-foundation-03, 02-api]

# Tech tracking
tech-stack:
  added: [gosnmp v1.43.2]
  patterns: [ClientInterface for testability, table-driven tests with mock PDU data, OID column matching]

key-files:
  created:
    - internal/snmp/client.go
    - internal/snmp/discovery.go
    - internal/snmp/detector.go
    - internal/snmp/client_test.go
    - internal/snmp/discovery_test.go
    - internal/snmp/detector_test.go
  modified:
    - go.mod
    - go.sum
    - .gitignore

key-decisions:
  - "ClientInterface abstraction for mock-based testing without real SNMP devices"
  - "matchOIDColumn helper to prevent ambiguous OID prefix matching between column 1 and column 15"

patterns-established:
  - "ClientInterface pattern: define interface for SNMP operations to enable mock-based testing"
  - "OID column matching: always append dot separator when matching OID prefixes"

requirements-completed: [INTG-04, INTG-05]

# Metrics
duration: 2min
completed: 2026-03-06
---

# Phase 1 Plan 02: SNMP Discovery Summary

**SNMP client (v2c/v3), device type detector (MikroTik/Cisco/Ubiquiti), and discovery service (sysInfo + interfaces + LLDP/CDP neighbors) using gosnmp with mock-based tests**

## Performance

- **Duration:** 2 min
- **Started:** 2026-03-06T14:14:27Z
- **Completed:** 2026-03-06T14:16:59Z
- **Tasks:** 1
- **Files modified:** 9

## Accomplishments
- SNMP client wraps gosnmp with full v2c and v3 support (auth protocols: MD5/SHA/SHA256/SHA512, priv: DES/AES/AES256)
- Device discovery extracts system info, full interface table (ifTable + ifXTable merge), and both LLDP and CDP neighbors
- Device type auto-detection covers MikroTik (RouterOS/SwOS), Cisco (ISR/ASR/C9000/Catalyst/C2960/C3750), Ubiquiti (EdgeRouter/US-/UniFi Switch/UAP/U6) with sysDescr keyword fallback
- 21 tests covering all parsing logic with mock SNMP PDU responses

## Task Commits

Each task was committed atomically:

1. **Task 1: SNMP client, device type detector, and discovery service** - `6c50d9e` (feat)

## Files Created/Modified
- `internal/snmp/client.go` - SNMP client wrapping gosnmp for v2c and v3
- `internal/snmp/discovery.go` - Device discovery: sysInfo, interfaces (ifTable+ifXTable), LLDP/CDP neighbors
- `internal/snmp/detector.go` - Device type auto-detection from sysObjectID and sysDescr patterns
- `internal/snmp/client_test.go` - Client config tests for v2c, v3, and invalid inputs
- `internal/snmp/discovery_test.go` - Discovery, LLDP parsing, CDP parsing, and error handling tests
- `internal/snmp/detector_test.go` - 17 table-driven test cases for device type detection
- `go.mod` / `go.sum` - Added gosnmp dependency
- `.gitignore` - Added build output exclusion

## Decisions Made
- Used ClientInterface abstraction to enable mock-based testing without real SNMP devices
- Added matchOIDColumn helper to prevent ambiguous OID prefix matching (e.g., column `.1.1` matching `.1.15`)

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 1 - Bug] Fixed OID column prefix matching ambiguity**
- **Found during:** Task 1 (discovery test failures)
- **Issue:** `strings.HasPrefix` on OID columns caused false matches - column 1 OID (`.1.3.6.1.2.1.31.1.1.1.1`) incorrectly matched column 15 OID (`.1.3.6.1.2.1.31.1.1.1.15`) because "15" starts with "1"
- **Fix:** Added `matchOIDColumn()` helper that appends a dot separator before prefix comparison
- **Files modified:** internal/snmp/discovery.go
- **Verification:** All discovery tests pass with correct ifName and ifHighSpeed parsing
- **Committed in:** 6c50d9e

**2. [Rule 3 - Blocking] Added build binary to .gitignore**
- **Found during:** Task 1 (staging files for commit)
- **Issue:** `go build` output binary `theia` appeared as untracked file
- **Fix:** Added `theia` to `.gitignore`
- **Files modified:** .gitignore
- **Committed in:** 6c50d9e

---

**Total deviations:** 2 auto-fixed (1 bug, 1 blocking)
**Impact on plan:** Both fixes necessary for correctness and clean commits. No scope creep.

## Issues Encountered
None beyond the auto-fixed deviations above.

## User Setup Required
None - no external service configuration required.

## Next Phase Readiness
- SNMP package ready for consumption by REST API (Plan 03)
- ClientInterface enables easy integration testing
- Discovery result maps directly to domain model types

---
*Phase: 01-foundation*
*Completed: 2026-03-06*
