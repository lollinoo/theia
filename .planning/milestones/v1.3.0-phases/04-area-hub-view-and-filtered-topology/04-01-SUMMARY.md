---
phase: 04-area-hub-view-and-filtered-topology
plan: 01
subsystem: ui
tags: [react, reactflow, canvas, decomposition, websocket, material-symbols]

# Dependency graph
requires:
  - phase: 02-component-restyling
    provides: Material Symbols font infrastructure, MaterialIcon component
  - phase: 03-area-backend-and-management
    provides: Area CRUD API, device area_id support
provides:
  - 7 focused canvas modules extracted from Canvas.tsx (canvasHelpers, edgeBuilder, nodeBuilder, useCanvasData, useCanvasMenus, CanvasPanels, CanvasOverlays)
  - Canvas.tsx slimmed to 204-line orchestrator
  - Three-view architecture (hub/canvas/dashboard) in App.tsx
  - WebSocket lifted from Canvas to App level (shared across views)
  - selectedAreaId state in App.tsx for area filtering
  - Updated Material Symbols font subset with hub and devices icons (21 total)
affects: [04-02-navigation-pill, 04-03-area-hub-view, 04-04-canvas-area-filtering]

# Tech tracking
tech-stack:
  added: []
  patterns: [canvas module decomposition, hook extraction, prop-driven WebSocket]

key-files:
  created:
    - frontend/src/components/canvas/canvasHelpers.ts
    - frontend/src/components/canvas/edgeBuilder.ts
    - frontend/src/components/canvas/nodeBuilder.ts
    - frontend/src/components/canvas/useCanvasData.ts
    - frontend/src/components/canvas/useCanvasMenus.ts
    - frontend/src/components/canvas/CanvasPanels.tsx
    - frontend/src/components/canvas/CanvasOverlays.tsx
  modified:
    - frontend/src/components/Canvas.tsx
    - frontend/src/App.tsx
    - frontend/src/components/NavBar.tsx
    - frontend/public/fonts/material-symbols-rounded-subset.woff2

key-decisions:
  - "Canvas decomposition into 7 modules with pure function extraction first, then hooks/components"
  - "WebSocket lifted to App.tsx for cross-view metric sharing"
  - "selectedAreaId state placed in App.tsx for NavigationPill and Canvas to consume"
  - "NavBar updated with hub tab inline (will be replaced by NavigationPill in Plan 02)"

patterns-established:
  - "Canvas module pattern: canvas/ subdirectory with focused files (helpers, builders, hooks, components)"
  - "WebSocket as app-level prop pattern: useWebSocket at App level, snapshot/reconnecting/prometheusStatus passed as props"

requirements-completed: [AREA-11]

# Metrics
duration: 10min
completed: 2026-03-26
---

# Phase 04 Plan 01: Canvas Decomposition and View Architecture Summary

**Canvas.tsx decomposed from 1283 to 204 lines across 7 focused modules, with three-view App.tsx architecture and WebSocket lift for cross-view metric sharing**

## Performance

- **Duration:** 10 min
- **Started:** 2026-03-26T12:05:45Z
- **Completed:** 2026-03-26T12:16:06Z
- **Tasks:** 3
- **Files modified:** 11

## Accomplishments
- Extracted pure utility functions, edge builder, and node builder into separate canvas modules
- Created useCanvasData hook encapsulating devices, links, snapshot merge, stale timer, and settings fetch
- Created useCanvasMenus hook managing menu state, keyboard shortcuts, and panel title resolution
- Extracted CanvasPanels and CanvasOverlays as focused React components
- Slimmed Canvas.tsx from 1283 to 204 lines as a pure orchestrator
- Lifted useWebSocket from Canvas to App.tsx for cross-view metric sharing
- Extended App.tsx with three-view architecture (hub/canvas/dashboard)
- Added selectedAreaId state for area filtering support
- Updated Material Symbols font subset with hub and devices icons (21 total)

## Task Commits

Each task was committed atomically:

1. **Task 1: Extract pure functions and builder modules** - `d1c4419` (feat)
2. **Task 2: Extract hooks, panels, overlays; slim Canvas to orchestrator** - `10c3e28` (feat)
3. **Task 3: Rewire App.tsx with three-view architecture and font subset** - `fdb1a32` (feat)

## Files Created/Modified
- `frontend/src/components/canvas/canvasHelpers.ts` - Pure utility functions (buildPositionPayload, inferSpeedLabel, compactThroughput, etc.)
- `frontend/src/components/canvas/edgeBuilder.ts` - Edge construction logic (buildEdgeData, getHandleSide, buildTopologyEdges, alertStatusForLink)
- `frontend/src/components/canvas/nodeBuilder.ts` - Node construction from devices, positions, and snapshot data
- `frontend/src/components/canvas/useCanvasData.ts` - Core data hook with devices, links, snapshot merge, stale timer
- `frontend/src/components/canvas/useCanvasMenus.ts` - Menu state, keyboard shortcuts, panel title resolution
- `frontend/src/components/canvas/CanvasPanels.tsx` - SidePanel children rendering by panel type
- `frontend/src/components/canvas/CanvasOverlays.tsx` - Edit mode banner, reconnect banner, prometheus alerts
- `frontend/src/components/Canvas.tsx` - Slimmed to 204-line orchestrator (from 1283)
- `frontend/src/App.tsx` - Three-view architecture with WebSocket lift and selectedAreaId
- `frontend/src/components/NavBar.tsx` - Updated ActiveView type with 'hub' tab
- `frontend/public/fonts/material-symbols-rounded-subset.woff2` - Updated with hub and devices icons

## Decisions Made
- Canvas decomposition follows pure-functions-first pattern: extract stateless helpers and builders before hooks and components
- WebSocket lifted to App.tsx level so Hub view (Plan 03) can access live device metrics without a separate connection
- selectedAreaId state placed in App.tsx (not Canvas) so NavigationPill (Plan 02) can set it
- NavBar temporarily updated with 'hub' tab; it will be replaced by NavigationPill in Plan 02
- snapshotRef pattern preserved in useCanvasData to maintain the first-load race condition fix documented in MEMORY.md
- devicesLengthRef pattern used in useCanvasData to avoid stale closure over devices.length in loadTopology callback

## Deviations from Plan

None - plan executed exactly as written.

## Issues Encountered
None

## User Setup Required
None - no external service configuration required.

## Known Stubs
- Hub view placeholder in App.tsx: "Area Hub (coming soon)" text -- intentional stub, will be replaced by AreaHub component in Plan 03

## Next Phase Readiness
- Canvas decomposition complete -- all Phase 4 plans can now add area filtering without touching a 1283-line file
- useCanvasData hook ready to accept area filter parameter
- App.tsx three-view architecture ready for NavigationPill (Plan 02) and AreaHub (Plan 03)
- selectedAreaId state wired and ready for consumption
- Material Symbols font subset includes hub and devices icons for NavigationPill

## Self-Check: PASSED

All 10 created/modified files verified present. All 3 task commits verified in git log.

---
*Phase: 04-area-hub-view-and-filtered-topology*
*Completed: 2026-03-26*
