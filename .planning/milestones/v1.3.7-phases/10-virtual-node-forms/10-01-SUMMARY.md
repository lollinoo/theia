---
phase: 10-virtual-node-forms
plan: 01
subsystem: ui
tags: [react, forms, virtual-device, segmented-control, material-icons]

# Dependency graph
requires:
  - phase: 08-virtual-device-backend
    provides: Backend virtual device creation with device_type validation and SNMP skip
  - phase: 09-virtual-node-rendering
    provides: Virtual DeviceCard rendering, subtype icon mapping, Material Symbols subset
provides:
  - Dual-mode AddDevicePanel with Physical Device / Virtual Node segmented toggle
  - Extended CreateDevicePayload with optional device_type, ip, and snmp fields
  - Virtual form with Display Name, 2x2 subtype icon cards, optional IP, and shared area multi-select
  - Virtual submit path sending device_type 'virtual' with tags payload
affects: [10-virtual-node-forms]

# Tech tracking
tech-stack:
  added: []
  patterns:
    - "DeviceMode union type with segmented control toggle for form mode switching"
    - "handleModeSwitch resets all form fields to defaults on mode change"
    - "Shared area multi-select rendered outside conditional branch for both modes"
    - "Virtual submit omits snmp field entirely (backend handles validation)"

key-files:
  created: []
  modified:
    - frontend/src/api/client.ts
    - frontend/src/components/AddDevicePanel.tsx
    - frontend/src/components/AddDevicePanel.test.tsx

key-decisions:
  - "Made ip and snmp optional in CreateDevicePayload to support virtual devices without SNMP"
  - "Virtual submit omits snmp field rather than sending placeholder (cleaner, backend handles it)"
  - "Area multi-select shared between both modes by extracting it outside the conditional branch"
  - "SSH Profile section hidden for virtual mode (virtual nodes don't use SSH backups)"

patterns-established:
  - "DeviceMode segmented control pattern for toggling between physical and virtual forms"

requirements-completed: [VIRT-10, VIRT-11]

# Metrics
duration: 5min
completed: 2026-04-01
---

# Phase 10 Plan 01: AddDevicePanel Virtual Mode Summary

**Dual-mode AddDevicePanel with Physical/Virtual segmented toggle, 2x2 subtype icon cards, and virtual payload construction**

## Performance

- **Duration:** 5 min
- **Started:** 2026-04-01T20:47:29Z
- **Completed:** 2026-04-01T20:52:58Z
- **Tasks:** 2
- **Files modified:** 3

## Accomplishments
- Added Physical Device / Virtual Node segmented control toggle at top of AddDevicePanel
- Virtual mode shows Display Name (required), 2x2 subtype icon cards (Internet, Cloud, Server, Generic), optional IP, and shared area multi-select
- Extended CreateDevicePayload with optional device_type, ip, and snmp fields
- Virtual submit sends device_type: 'virtual' with display_name and virtual_subtype in tags, omitting snmp
- 7 new tests covering segmented control, virtual form fields, mode reset, and payload verification
- All 216 tests pass across 31 test files with zero regressions

## Task Commits

Each task was committed atomically:

1. **Task 1: Extend CreateDevicePayload and add virtual mode to AddDevicePanel** - `4ff1806` (feat)
2. **Task 2: Add tests for segmented control and virtual form behavior** - `d814b2b` (test)

## Files Created/Modified
- `frontend/src/api/client.ts` - Extended CreateDevicePayload with optional device_type, ip, snmp fields
- `frontend/src/components/AddDevicePanel.tsx` - Added segmented toggle, virtual form fields, subtype icon cards, virtual submit path
- `frontend/src/components/AddDevicePanel.test.tsx` - Added 7 tests for virtual mode behavior

## Decisions Made
- Made ip and snmp optional in CreateDevicePayload to support virtual devices that don't use SNMP (backward-compatible with existing call sites)
- Virtual submit omits snmp field entirely rather than sending a placeholder -- backend skips SNMP validation for virtual types (Phase 8 D-08)
- Area multi-select is shared between both modes by rendering it outside the conditional branch, avoiding code duplication
- SSH Profile section hidden for virtual mode since virtual nodes don't support SSH config backups

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 1 - Bug] Fixed setSshProfileId casing in handleModeSwitch**
- **Found during:** Task 2 (test execution)
- **Issue:** handleModeSwitch used `setSshProfileId('')` but the actual state setter is `setSSHProfileId` (capital SSH)
- **Fix:** Changed `setSshProfileId` to `setSSHProfileId`
- **Files modified:** frontend/src/components/AddDevicePanel.tsx
- **Verification:** All tests pass, tsc --noEmit clean
- **Committed in:** d814b2b (Task 2 commit)

---

**Total deviations:** 1 auto-fixed (1 bug)
**Impact on plan:** Auto-fix was a typo in the plan's suggested code. No scope creep.

## Issues Encountered
None

## User Setup Required
None - no external service configuration required.

## Next Phase Readiness
- AddDevicePanel now supports creating virtual nodes with full form UX
- Ready for Plan 10-02 (LinkCreatePanel virtual adaptation and context menu filtering)

## Self-Check: PASSED

All files verified present, all commit hashes found in git history.

---
*Phase: 10-virtual-node-forms*
*Completed: 2026-04-01*
