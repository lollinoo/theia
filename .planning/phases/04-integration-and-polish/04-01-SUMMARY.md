---
phase: 04-integration-and-polish
plan: 01
subsystem: ui
tags: [react, typescript, context-menu, keyboard-shortcuts, side-panel, toolbar]

# Dependency graph
requires:
  - phase: 03-live-metrics
    provides: Canvas.tsx with WebSocket metrics, DeviceCard, LinkEdge components to extend
provides:
  - ContextMenu: reusable dark-themed context menu with viewport-aware positioning
  - SidePanel: 320px slide-out right panel with smooth transition
  - ShortcutHelp: keyboard shortcut reference overlay
  - useKeyboardShortcuts: centralized keydown hook with Ctrl/Cmd and input-focus awareness
  - Toolbar: top-right floating icon buttons with tooltip+shortcut hints
  - Device and link right-click context menus wired into Canvas
  - Full keyboard shortcut system (Ctrl+K, Ctrl+N, Ctrl+,, +/-/0, ?, Esc)
affects:
  - 04-02 (Grafana links use device context menu items)
  - 04-03 (Settings and Add Device use SidePanel)

# Tech tracking
tech-stack:
  added: []
  patterns:
    - Context menu rendered as fixed-position overlay with viewport-bounds clamping
    - Side panel driven by panelContent state (type+data), filled in by later plans
    - useKeyboardShortcuts accepts a stable Record<string, ShortcutHandler> map
    - Device onContextMenu passed through node data (not props) for React Flow compatibility

key-files:
  created:
    - frontend/src/components/ContextMenu.tsx
    - frontend/src/components/SidePanel.tsx
    - frontend/src/components/ShortcutHelp.tsx
    - frontend/src/components/Toolbar.tsx
    - frontend/src/hooks/useKeyboardShortcuts.ts
  modified:
    - frontend/src/components/Canvas.tsx
    - frontend/src/components/DeviceCard.tsx
    - frontend/src/components/LinkEdge.tsx
    - frontend/src/components/SearchOverlay.tsx

key-decisions:
  - "Device onContextMenu callback passed via node.data (not component props) to satisfy React Flow's NodeProps contract"
  - "ContextMenu does viewport-bounds adjustment via useEffect after first render (measure then reposition)"
  - "Escape key priority order: context menu > side panel > search overlay > shortcut help"
  - "SidePanel uses CSS translate-x transform for slide animation rather than conditional render to avoid layout jumps"

patterns-established:
  - "Panel type pattern: panelContent: {type: string, data?: any} | null drives SidePanel content; later plans fill specific types"
  - "Keyboard shortcuts defined as useMemo-stable record in Canvas, passed to useKeyboardShortcuts hook"

requirements-completed: [UX-03, UX-04]

# Metrics
duration: 1min
completed: 2026-03-08
---

# Phase 04 Plan 01: UI Infrastructure Summary

**Dark-themed ContextMenu, slide-out SidePanel, floating Toolbar, and useKeyboardShortcuts hook wired into Canvas with device and link right-click menus and full shortcut system**

## Performance

- **Duration:** ~1 min (files pre-authored; verification + commit time)
- **Started:** 2026-03-08T17:26:11Z
- **Completed:** 2026-03-08T17:27:20Z
- **Tasks:** 2
- **Files modified:** 9

## Accomplishments
- Created five new files (ContextMenu, SidePanel, ShortcutHelp, Toolbar, useKeyboardShortcuts) providing all shared UI infrastructure for Phase 4
- Wired device and link right-click context menus into Canvas, replacing the old inline edge menu JSX with the generic ContextMenu component
- Registered complete keyboard shortcut system (Ctrl+K search, Ctrl+N add, Ctrl+, settings, +/-/0 zoom, ? help, Esc close chain)
- TypeScript compilation and production build both pass with zero errors

## Task Commits

Each task was committed atomically:

1. **Task 1: Create ContextMenu, SidePanel, ShortcutHelp and useKeyboardShortcuts** - `bed0e63` (feat)
2. **Task 2: Create Toolbar and wire everything into Canvas** - `493aee4` (feat)

## Files Created/Modified
- `frontend/src/components/ContextMenu.tsx` - Generic dark-themed fixed-position menu with viewport clamping and click-outside/Escape close
- `frontend/src/components/SidePanel.tsx` - Right slide-out 320px panel with CSS translate transition, z-20
- `frontend/src/components/ShortcutHelp.tsx` - Modal overlay listing all shortcuts, platform-aware modifier key display
- `frontend/src/components/Toolbar.tsx` - Vertical icon button stack at top-right with tooltip labels
- `frontend/src/hooks/useKeyboardShortcuts.ts` - Single keydown listener, ignores input focus, Ctrl/Cmd aware
- `frontend/src/components/Canvas.tsx` - Integrated all five components; device+edge context menus; shortcut registration; SidePanel placeholder content
- `frontend/src/components/DeviceCard.tsx` - Added onContextMenu to DeviceNodeData, wired to root div with preventDefault
- `frontend/src/components/LinkEdge.tsx` - Minor: existing onContextMenu already correct, no functional changes
- `frontend/src/components/SearchOverlay.tsx` - Added autoFocus to search input on mount

## Decisions Made
- Device `onContextMenu` passed through `node.data` (not component props) because React Flow's `NodeProps` doesn't support arbitrary prop pass-through; node data is the correct extension point
- `ContextMenu` repositions after initial render using a two-pass approach (render offscreen at `opacity-0`, measure, then snap to clamped position) to handle dynamic menu heights
- Escape key closes in priority order (context menu first, then side panel, then search, then shortcut help) to avoid swallowing Escape from nested overlays
- `SidePanel` uses `translate-x-full` / `translate-x-0` CSS transform (always mounted) so the exit animation plays; conditional render would immediately remove the element

## Deviations from Plan

None - plan executed exactly as written. All files were pre-authored and verified to compile and build cleanly.

## Issues Encountered
None - TypeScript compilation and `npm run build` passed on first attempt.

## User Setup Required
None - no external service configuration required.

## Next Phase Readiness
- ContextMenu, SidePanel, Toolbar, and useKeyboardShortcuts are ready for Phase 4 plans 02 and 03
- Plan 02 (Grafana links): wire "Open in Grafana" context menu items with real URLs
- Plan 03 (Settings/Add Device): replace SidePanel placeholder content for `settings` and `addDevice` types
- All existing Canvas behavior (drag, zoom, WebSocket metrics, search, minimap) preserved with no regressions

---
*Phase: 04-integration-and-polish*
*Completed: 2026-03-08*
