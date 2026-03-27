---
phase: 04-area-hub-view-and-filtered-topology
plan: 03
subsystem: ui
tags: [react, area-hub, aggregate-stats, area-cards, bloom-effects, health-calculation]

# Dependency graph
requires:
  - phase: 04-area-hub-view-and-filtered-topology
    plan: 01
    provides: Canvas decomposition, three-view App.tsx architecture, WebSocket lift, selectedAreaId state
  - phase: 03-area-backend-and-management
    provides: Area CRUD API, fetchAreas client, Area interface with color/device_count
provides:
  - AreaCard component with bloom effect, glow dot, accent color hover, health/device/link stats
  - AreaHub view with four aggregate stat cards and per-area card grid
  - Health computation helper (Optimal/Degraded/Critical thresholds)
  - Network Uptime calculation (minimum uptime across UP devices)
  - Empty state CTA for zero-area Hub view
  - onLinksChange prop on Canvas for lifting links to App.tsx
  - App.tsx wired with AreaHub, areas fetch, link state
affects: [04-04-canvas-area-filtering]

# Tech tracking
tech-stack:
  added: []
  patterns: [computeHealth helper for reusable health thresholds, onLinksChange lift pattern matching onDevicesChange]

key-files:
  created:
    - frontend/src/components/AreaCard.tsx
    - frontend/src/components/AreaCard.test.tsx
    - frontend/src/components/AreaHub.tsx
    - frontend/src/components/AreaHub.test.tsx
  modified:
    - frontend/src/App.tsx
    - frontend/src/components/Canvas.tsx
    - frontend/src/components/canvas/useCanvasData.ts

key-decisions:
  - "computeHealth helper extracted for reuse between aggregate and per-area health calculation"
  - "Link count per area counts links where EITHER endpoint is in the area (cross-area links counted in both)"
  - "Areas fetched at App level and re-fetched on hub view activation to pick up settings changes"
  - "onLinksChange prop on Canvas mirrors onDevicesChange pattern for lifting links to App.tsx"

patterns-established:
  - "Health thresholds pattern: >=95% Optimal (text-status-up), >=80% Degraded (text-warning), <80% Critical (text-status-down)"
  - "Area card bloom pattern: absolute positioned blur circle with area accent color, opacity transition on hover"

requirements-completed: [AREA-07, AREA-08]

# Metrics
duration: 5min
completed: 2026-03-26
---

# Phase 04 Plan 03: Area Hub View and Area Cards Summary

**AreaHub view with four aggregate stat cards (uptime/health/devices/links), per-area cards with accent color bloom effects, and health computation wired into App.tsx**

## Performance

- **Duration:** 5 min
- **Started:** 2026-03-26T12:20:08Z
- **Completed:** 2026-03-26T12:25:08Z
- **Tasks:** 2
- **Files modified:** 7

## Accomplishments
- Built AreaCard component with accent color bloom circle, glow dot, hover border transition, and health/device/link metric rows
- Built AreaHub view with four aggregate stat cards: Network Uptime (min uptime as days/hours/mins), Aggregate Health (percentage with threshold labels), Total Devices, Active Links
- Implemented computeHealth helper with Optimal/Degraded/Critical thresholds reused for aggregate and per-area stats
- Added empty state CTA card ("No areas yet") when no areas exist, linking to settings
- Wired AreaHub into App.tsx with areas fetch, link state lift via onLinksChange prop, and area selection navigation
- 10 new tests (5 AreaCard + 5 AreaHub), all 89 tests passing

## Task Commits

Each task was committed atomically:

1. **Task 1: AreaCard component with bloom effects and tests** - `c63be64` (feat)
2. **Task 2: AreaHub view with aggregate stats, area grid, empty state, and App.tsx wiring** - `9a521a5` (feat)

## Files Created/Modified
- `frontend/src/components/AreaCard.tsx` - Individual area card with bloom effect, glow dot, accent color hover, metric rows
- `frontend/src/components/AreaCard.test.tsx` - 5 unit tests for AreaCard rendering, health labels, counts, dot color, click
- `frontend/src/components/AreaHub.tsx` - Hub view with aggregate stats header (4 cards) and area card grid with health computation
- `frontend/src/components/AreaHub.test.tsx` - 5 unit tests for heading/subtitle, stat labels, empty state, area cards, health computation
- `frontend/src/App.tsx` - Added AreaHub import/render, areas fetch, canvasLinks state, handleAreaSelect, handleCanvasLinksChange
- `frontend/src/components/Canvas.tsx` - Added onLinksChange prop to CanvasProps interface and pass-through to useCanvasData
- `frontend/src/components/canvas/useCanvasData.ts` - Added onLinksChange to params and propagation effect for topologyLinks

## Decisions Made
- computeHealth helper extracted as a standalone function for reuse between aggregate and per-area health, matching D-11 thresholds
- Link counting per area uses EITHER-endpoint rule (a cross-area link is counted in both areas) per RESEARCH.md pitfall 6
- Areas fetched at App level and re-fetched when switching to hub view to pick up changes made in settings
- onLinksChange prop on Canvas follows the same pattern as the existing onDevicesChange for lifting state to App.tsx

## Deviations from Plan

None - plan executed exactly as written.

## Issues Encountered
None

## User Setup Required
None - no external service configuration required.

## Known Stubs
None - all data is wired to live sources (devices from Canvas, areas from API, links from Canvas, snapshot from WebSocket).

## Next Phase Readiness
- AreaHub view complete and wired into App.tsx three-view architecture
- Area cards render with accent colors, health computation, and click-to-navigate
- onAreaSelect handler navigates to canvas view with selectedAreaId set
- Ready for Plan 04 (canvas area filtering with ghost nodes) which consumes selectedAreaId

## Self-Check: PASSED

All 7 created/modified files verified present. Both task commits verified in git log.

---
*Phase: 04-area-hub-view-and-filtered-topology*
*Completed: 2026-03-26*
