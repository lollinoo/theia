---
phase: 09-virtual-node-rendering
plan: 01
subsystem: ui
tags: [reactflow, material-symbols, woff2, virtual-devices, typescript]

# Dependency graph
requires:
  - phase: 08-virtual-device-backend
    provides: Virtual device type in domain model, API creation with subtype tags
provides:
  - Virtual device type in frontend DeviceType union and parseDeviceType
  - Virtual card rendering branch in DeviceCard.tsx with compact centered layout
  - nodeBuilder virtual detection and isVirtual/subtype flag propagation
  - MaterialIcon glyph tests for language, cloud, dns icon names
  - Font subset regenerated with 24 icons (added language, cloud, dns)
  - Font subset build script for reproducible font generation
affects: [09-02-virtual-link-edge-labels, 10-virtual-node-forms]

# Tech tracking
tech-stack:
  added: [pyftsubset font subsetting script]
  patterns: [virtual card early-return branch pattern, subtypeIconMap lookup]

key-files:
  created:
    - frontend/scripts/subset-material-icons.sh
  modified:
    - frontend/src/types/api.ts
    - frontend/src/components/DeviceCard.tsx
    - frontend/src/components/DeviceCard.test.tsx
    - frontend/src/components/MaterialIcon.test.tsx
    - frontend/src/components/canvas/nodeBuilder.ts
    - frontend/public/fonts/material-symbols-rounded-subset.woff2
    - frontend/src/index.css

key-decisions:
  - "Virtual card rendering uses early-return branch in DeviceCardInner (same pattern as ghost nodes)"
  - "Font subset regenerated via pyftsubset from Google Fonts source (4.2KB output)"
  - "Metrics set to null for virtual devices in nodeBuilder (no SNMP metrics)"

patterns-established:
  - "subtypeIconMap lookup pattern: Record<string, string> mapping subtype to Material Symbol name"
  - "isVirtual flag propagation: nodeBuilder detects device_type=virtual, passes isVirtual+subtype to DeviceNodeData"

requirements-completed: [VIRT-06, VIRT-07, VIRT-08, VIRT-09]

# Metrics
duration: 4min
completed: 2026-04-01
---

# Phase 9 Plan 1: Virtual Node Rendering Summary

**Compact virtual device cards with subtype Material Symbol icons, dashed borders, IP-conditional width variants (200px/160px), and regenerated 24-icon font subset**

## Performance

- **Duration:** 4 min
- **Started:** 2026-04-01T19:43:08Z
- **Completed:** 2026-04-01T19:47:30Z
- **Tasks:** 2
- **Files modified:** 8

## Accomplishments
- Extended frontend type system with 'virtual' DeviceType and parseDeviceType handling
- Built virtual card rendering branch with dashed border, centered vertical layout, and subtype icon
- IP-bearing virtual nodes render at 200px with StatusDot and IP body section; no-IP at 160px with icon and label only
- Regenerated Material Symbols font subset with language, cloud, and dns glyphs (21 -> 24 icons)
- Added 6 virtual card tests and 3 MaterialIcon glyph verification tests (32 total tests pass)

## Task Commits

Each task was committed atomically:

1. **Task 1: Extend type system and DeviceNodeData for virtual devices** - `d374de6` (feat)
2. **Task 2: Virtual card rendering branch, font subset, icon glyph tests, and DeviceCard tests** - `8a25567` (feat)

## Files Created/Modified
- `frontend/src/types/api.ts` - Added 'virtual' to DeviceType union and parseDeviceType switch
- `frontend/src/components/DeviceCard.tsx` - Added isVirtual/subtype to DeviceNodeData, subtypeIconMap, virtual card branch, memo comparator updates
- `frontend/src/components/canvas/nodeBuilder.ts` - Virtual device detection, isVirtual/subtype flag propagation, metrics null for virtuals
- `frontend/src/components/DeviceCard.test.tsx` - 6 new virtual card tests (VIRT-06, VIRT-07, VIRT-08, D-01/D-02, D-11, D-12)
- `frontend/src/components/MaterialIcon.test.tsx` - 3 new glyph verification tests for language, cloud, dns (VIRT-09)
- `frontend/public/fonts/material-symbols-rounded-subset.woff2` - Regenerated with 24 icons (added language U+E894, cloud U+E2BD, dns U+E875)
- `frontend/scripts/subset-material-icons.sh` - Reproducible font subset build script using pyftsubset
- `frontend/src/index.css` - Updated comment from 21 to 24 icons

## Decisions Made
- Virtual card uses early-return branch in DeviceCardInner, following same pattern as ghost node branch
- Font subset regenerated with pyftsubset from full Google Fonts source (4.2KB output vs 25.7KB previous)
- Metrics explicitly set to null for virtual devices in nodeBuilder since virtual devices have no SNMP metrics
- Virtual card wrapper uses identical glow/status class logic as physical cards per D-03 and D-11

## Deviations from Plan

None - plan executed exactly as written.

## Known Stubs

None - all virtual card rendering is fully wired with real data from DeviceNodeData.

## Issues Encountered
None

## User Setup Required
None - no external service configuration required.

## Next Phase Readiness
- Virtual nodes render correctly on canvas with full interactivity
- Ready for Plan 02 (virtual link edge label adaptation)
- Ready for Phase 10 (virtual node forms and context menu)

---
*Phase: 09-virtual-node-rendering*
*Completed: 2026-04-01*
