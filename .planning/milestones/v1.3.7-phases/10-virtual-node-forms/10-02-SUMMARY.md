---
phase: 10-virtual-node-forms
plan: 02
subsystem: ui
tags: [react, forms, virtual-device, context-menu, link-creation]

# Dependency graph
requires:
  - phase: 10-virtual-node-forms
    plan: 01
    provides: AddDevicePanel virtual mode toggle and payload extension
  - phase: 08-virtual-device-backend
    provides: Backend virtual device creation, both-virtual link rejection, empty if_name for virtual side
  - phase: 09-virtual-node-rendering
    provides: Virtual DeviceCard rendering, subtype icon mapping
provides:
  - LinkCreatePanel virtual-aware interface hiding and validation
  - Both-virtual inline error with disabled Create button
  - Virtual-side empty if_name in createLink payload
  - Canvas context menu filtering for virtual nodes (Grafana + Configure only)
  - SearchableDeviceSelect handles virtual devices with empty IP
affects: []

# Tech tracking
tech-stack:
  added: []
  patterns:
    - "Virtual detection via device_type === 'virtual' check on looked-up device"
    - "Effective if_name pattern: sourceIsVirtual ? '' : sourceIfName for payload construction"
    - "Stable id-based context menu filtering using Set.has() instead of fragile label matching"

key-files:
  created:
    - frontend/src/components/LinkCreatePanel.test.tsx
    - frontend/src/components/Canvas.test.tsx
  modified:
    - frontend/src/components/LinkCreatePanel.tsx
    - frontend/src/components/Canvas.tsx

key-decisions:
  - "Used stable id field on context menu items for filtering instead of label string matching (robust against label text changes)"
  - "Virtual detection done per-render via device_type check rather than prop threading (simpler, no API changes needed)"
  - "Canvas context menu filtering tests use isolated logic replication (Approach 2) since full Canvas rendering is impractical in tests"

patterns-established:
  - "id-based context menu item filtering pattern for device-type-specific menus"

requirements-completed: [VIRT-12, VIRT-13, VIRT-16]

# Metrics
duration: 4min
completed: 2026-04-01
---

# Phase 10 Plan 02: LinkCreatePanel Virtual Adaptation and Context Menu Filtering Summary

**Virtual-aware link creation with interface hiding, both-virtual validation, and id-based Canvas context menu filtering for virtual nodes**

## Performance

- **Duration:** 4 min
- **Started:** 2026-04-01T20:56:38Z
- **Completed:** 2026-04-01T21:00:45Z
- **Tasks:** 2
- **Files modified:** 4

## Accomplishments
- LinkCreatePanel hides interface selector for virtual devices, shows "(virtual node -- no interface)" label
- Both-virtual validation: inline error message and disabled Create button when both devices are virtual
- Virtual side sends empty if_name in createLink payload (backend accepts this per Phase 8)
- deviceLabel and SearchableDeviceSelect handle virtual devices with empty IP (show display_name)
- Canvas context menu filters to 2 items (Grafana + Configure) for virtual nodes using stable id-based filtering
- 8 new tests: 5 for LinkCreatePanel virtual behavior, 3 for Canvas context menu filtering (VIRT-16)
- All 224 tests pass across 33 test files with zero regressions

## Task Commits

Each task was committed atomically:

1. **Task 1: Adapt LinkCreatePanel for virtual devices and filter Canvas context menu** - `7afc954` (feat)
2. **Task 2: Add LinkCreatePanel tests and context menu filtering tests for virtual behavior** - `c3f47d0` (test)

## Files Created/Modified
- `frontend/src/components/LinkCreatePanel.tsx` - Virtual detection flags, conditional interface hiding, both-virtual validation, effective if_name logic, deviceLabel empty IP handling, SearchableDeviceSelect virtual display
- `frontend/src/components/Canvas.tsx` - Context menu items with stable id field, virtual node filtering (Grafana + Configure only), ContextMenuItem type import
- `frontend/src/components/LinkCreatePanel.test.tsx` - 5 tests for virtual interface hiding, physical interface showing, both-virtual error, both-virtual button disable, virtual device label
- `frontend/src/components/Canvas.test.tsx` - 3 tests for VIRT-16 context menu filtering (virtual 2 items, physical 4 items, switch 4 items)

## Decisions Made
- Used stable `id` field on context menu items for filtering instead of fragile label string matching (e.g., "Open in Grafana" vs "Open in Grafana (not configured)" would break label-based filtering)
- Virtual detection done per-render via `device_type === 'virtual'` check on looked-up device -- simpler than prop threading, no API changes needed
- Canvas context menu filtering tests use isolated logic replication (Approach 2 from plan) since full Canvas rendering requires mocking ReactFlow, WebSocket, and many sub-components

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 2 - Missing Critical] Fixed SearchableDeviceSelect display for virtual devices without IP**
- **Found during:** Task 1
- **Issue:** SearchableDeviceSelect always renders `d.ip` in a font-mono span. For virtual devices with empty IP, this renders an empty span followed by "-- display_name", which looks broken
- **Fix:** Added conditional rendering in both the selected device display and dropdown list items: if `d.ip` is empty, show display_name directly without the IP prefix and dash separator
- **Files modified:** frontend/src/components/LinkCreatePanel.tsx
- **Verification:** Test "shows display name for virtual device without IP in dropdown" passes
- **Committed in:** 7afc954 (Task 1 commit)

---

**Total deviations:** 1 auto-fixed (1 missing critical)
**Impact on plan:** Auto-fix was necessary for correct virtual device display in the dropdown. No scope creep.

## Issues Encountered
- Worktree was behind the feature branch (missing phases 8, 9, 10-01 changes). Resolved by fast-forward merging `gsd/v1.3.7-virtual-representative-nodes` into the worktree.
- Frontend `node_modules` not present in worktree. Resolved by running `npm install`.

## User Setup Required
None - no external service configuration required.

## Next Phase Readiness
- All LinkCreatePanel and Canvas context menu virtual adaptations are complete
- Phase 10 (Virtual Node Forms) is fully implemented: AddDevicePanel virtual mode (10-01), LinkCreatePanel + context menu (10-02)
- Ready for phase-level verification

## Self-Check: PASSED

All files verified present, all commit hashes found in git history.

---
*Phase: 10-virtual-node-forms*
*Completed: 2026-04-01*
