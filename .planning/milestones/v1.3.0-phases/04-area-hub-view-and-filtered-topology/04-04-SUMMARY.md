---
phase: 04-area-hub-view-and-filtered-topology
plan: 04
subsystem: ui
tags: [react, area-filtering, ghost-nodes, canvas, reactflow, navigation]

# Dependency graph
requires:
  - phase: 04-area-hub-view-and-filtered-topology
    plan: 01
    provides: Canvas decomposition, three-view architecture, selectedAreaId prop
  - phase: 04-area-hub-view-and-filtered-topology
    plan: 02
    provides: NavigationPill with area selection buttons
  - phase: 04-area-hub-view-and-filtered-topology
    plan: 03
    provides: AreaHub view, areas state in App.tsx, onLinksChange
provides:
  - useAreaFilteredTopology hook for filtering devices/links by area
  - Ghost node rendering variant (isGhost, onGhostClick) in DeviceCard
  - Canvas area filtering wired via selectedAreaId prop
  - fitView re-centering on area switch
  - Ghost node click navigates to target device's area
affects: []

# Tech tracking
tech-stack:
  added: []
  patterns: [useAreaFilteredTopology hook for derived filtering, ghost node variant via isGhost prop]

key-files:
  created:
    - frontend/src/components/canvas/useAreaFilteredTopology.ts
    - frontend/src/components/canvas/useAreaFilteredTopology.test.ts
  modified:
    - frontend/src/components/Canvas.tsx
    - frontend/src/components/DeviceCard.tsx
    - frontend/src/components/NavigationPill.tsx
    - frontend/src/components/AreaHub.tsx
    - frontend/src/App.tsx

key-decisions:
  - "Ghost nodes positioned 200px offset from connected real node per RESEARCH.md recommendation"
  - "Ghost nodes are non-draggable (draggable: false) to distinguish from real nodes"
  - "handleAreaSelect in App.tsx always routes to canvas view (hub only via onViewChange)"
  - "NavigationPill Global button shows unfiltered canvas, Hub icon shows Area Hub"

patterns-established:
  - "Area filtering pattern: derive filtered sets from full device/link arrays using useMemo"
  - "Ghost node pattern: isGhost flag + onGhostClick callback + dashed border + reduced opacity"

requirements-completed: [AREA-09, AREA-10, AREA-11]

# Metrics
duration: 15min
completed: 2026-03-26
---

# Phase 04 Plan 04: Area-Filtered Topology Summary

**Area-filtered canvas with useAreaFilteredTopology hook, ghost node rendering, NavigationPill routing fixes, and visual verification**

## Performance

- **Duration:** 15 min (including bug fixes during verification)
- **Started:** 2026-03-26T12:30:00Z
- **Completed:** 2026-03-26T13:05:00Z
- **Tasks:** 3 (2 code + 1 visual verification)
- **Files modified:** 7

## Accomplishments
- Created useAreaFilteredTopology hook that filters devices/links by selectedAreaId and identifies ghost devices for cross-area links
- Added isGhost and onGhostClick props to DeviceCard for ghost node variant (dashed border, reduced opacity, non-draggable)
- Wired area filtering into Canvas.tsx with displayNodes/displayEdges derived from full topology
- Ghost nodes positioned near connected real nodes with 200px offset
- Clicking ghost nodes navigates to target device's area via onAreaSelect
- fitView re-centers on filtered subset when selectedAreaId changes
- Fixed 3 post-merge bugs during visual verification:
  1. `reactflow` imports → `@xyflow/react` (9 occurrences across 8 files)
  2. Missing ThemeProvider wrapper in App.tsx (lost during restructuring)
  3. NavigationPill routing (Global → canvas, Hub icon → hub), design improvements, AreaHub theme transition consistency
- 108 tests passing, TypeScript clean

## Task Commits

Each task was committed atomically:

1. **Task 1: useAreaFilteredTopology hook + ghost DeviceCard variant** - `a7718ae` (feat)
2. **Task 2: Wire area filtering into Canvas.tsx** - `e3794f3` (feat)
3. **Bug fixes during verification:**
   - `d7210ac` — fix: replace reactflow imports with @xyflow/react
   - `7f0cdfa` — fix: restore ThemeProvider wrapper in App.tsx
   - `9eadcaa` — fix: NavigationPill design/routing, AreaHub theme transitions

## Files Created/Modified
- `frontend/src/components/canvas/useAreaFilteredTopology.ts` - Hook filtering devices/links by area, deriving ghost devices
- `frontend/src/components/canvas/useAreaFilteredTopology.test.ts` - Unit tests for filtering and ghost device identification
- `frontend/src/components/Canvas.tsx` - displayNodes/displayEdges memo, ghost node creation, fitView on area change
- `frontend/src/components/DeviceCard.tsx` - isGhost + onGhostClick props, ghost variant styling
- `frontend/src/components/NavigationPill.tsx` - Routing fix (Global → canvas), larger icons, dividers, no scroll
- `frontend/src/components/AreaHub.tsx` - Added transition-colors to stat cards for consistent theme animation
- `frontend/src/App.tsx` - handleAreaSelect always routes to canvas, ThemeProvider restored

## Decisions Made
- Ghost nodes use 200px horizontal offset from their connected real node
- handleAreaSelect decoupled from view routing — always goes to canvas, hub only via explicit onViewChange
- NavigationPill Global button means "show all devices on canvas" (not Hub view)
- Stat cards and area cards now have matching transition-colors duration-200 for consistent theme switching

## Deviations from Plan
- Three bug fixes required during visual verification (reactflow imports, ThemeProvider, NavigationPill routing)
- NavigationPill redesigned beyond original plan scope to fix UX issues (larger touch targets, visual dividers, removed scroll)

## Issues Encountered
- reactflow → @xyflow/react import mismatch: Wave 1 agent used old package name despite Phase 1 migration
- ThemeProvider wrapper lost during App.tsx restructuring in 04-01
- NavigationPill "Global" button incorrectly routed to Hub view instead of full canvas

## User Setup Required
None

## Known Stubs
None — all data wired to live sources.

## Self-Check: PASSED

All created/modified files verified present. All task and fix commits verified in git log. 108 tests passing.

---
*Phase: 04-area-hub-view-and-filtered-topology*
*Completed: 2026-03-26*
