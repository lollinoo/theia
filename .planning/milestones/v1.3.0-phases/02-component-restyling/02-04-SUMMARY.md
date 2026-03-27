---
phase: 02-component-restyling
plan: 04
subsystem: ui
tags: [material-symbols, no-line-rule, glow-effects, surface-tiers, toolbar, sidepanel, zoom-controls, alerts-panel]

# Dependency graph
requires:
  - phase: 02-component-restyling
    plan: 01
    provides: "MaterialIcon component and self-hosted Material Symbols Rounded subset woff2"
provides:
  - "Toolbar with 6 Material Symbols icons, no border separators, dark-only backdrop-blur"
  - "SidePanel with surface tier header, shadow-panel depth, MaterialIcon close button"
  - "ZoomControls with Material Symbols zoom_in/zoom_out/fit_screen, no border separators"
  - "AlertsPanel with glow status dots (firing red, resolved green) and borderless surface tier cards"
  - "ShortcutHelp with no-line rule, surface tier kbd elements, shadow-panel modal"
  - "ReconnectBanner with warm warning glow and transition-colors"
affects: [02-06]

# Tech tracking
tech-stack:
  added: []
  patterns: [glow status dots via box-shadow with var(--nt-glow-shadow-opacity), surface tier card separation replacing border-outline]

key-files:
  created: []
  modified:
    - frontend/src/components/Toolbar.tsx
    - frontend/src/components/SidePanel.tsx
    - frontend/src/components/ZoomControls.tsx
    - frontend/src/components/AlertsPanel.tsx
    - frontend/src/components/ShortcutHelp.tsx
    - frontend/src/components/ReconnectBanner.tsx

key-decisions:
  - "AlertsPanel empty state icon replaced with MaterialIcon check_circle for consistency"
  - "ShortcutHelp close button SVG replaced with MaterialIcon for system-wide icon consistency"
  - "Prometheus status cards retain semantic border-red/border-yellow (not layout separators, functional status borders)"

patterns-established:
  - "Glow status dots: firing uses shadow-[0_0_10px_rgba(255,23,68,var(--nt-glow-shadow-opacity))] with animate-pulse + motion-reduce:animate-none"
  - "Resolved status dots: shadow-[0_0_6px_rgba(0,230,118,var(--nt-glow-shadow-opacity))] static glow"
  - "Alert card surface tiers: firing bg-elevated shadow-panel, resolved bg-surface-high opacity-60"
  - "Dark-only backdrop-blur: dark:backdrop-blur-xl on floating controls (Toolbar, ZoomControls)"

requirements-completed: [COMP-04, COMP-06, COMP-12]

# Metrics
duration: 4min
completed: 2026-03-25
---

# Phase 02 Plan 04: Navigation Controls, AlertsPanel, and Utility Components Summary

**Toolbar, SidePanel, ZoomControls restyled with Material Symbols icons and no-line rule; AlertsPanel with glow status dots and surface tier cards; ShortcutHelp and ReconnectBanner visually aligned**

## Performance

- **Duration:** 4 min
- **Started:** 2026-03-25T21:57:41Z
- **Completed:** 2026-03-25T22:01:50Z
- **Tasks:** 4
- **Files modified:** 6

## Accomplishments
- Replaced all inline SVGs in Toolbar (6 icons), SidePanel (close), ShortcutHelp (close), and AlertsPanel (check_circle) with MaterialIcon components
- Replaced ZoomControls text buttons (+/-/Fit) with MaterialIcon zoom_in/zoom_out/fit_screen
- Removed all layout border separators (border-b, border-l, border border-outline) from 6 components per COMP-12 no-line rule
- Added glow effects to AlertsPanel firing (red) and resolved (green) status dots with theme-aware opacity variable
- Applied dark-only backdrop-blur (dark:backdrop-blur-xl) to Toolbar and ZoomControls
- Added transition-colors duration-200 to all 6 components for smooth theme switching
- Added motion-reduce:animate-none to all animate-pulse elements for accessibility

## Task Commits

Each task was committed atomically:

1. **Task 1: Restyle Toolbar with Material Symbols icons and no-line rule** - `034ba12` (feat)
2. **Task 2a: Restyle SidePanel and ZoomControls** - `0d4cf84` (feat)
3. **Task 2b: Restyle ShortcutHelp and ReconnectBanner** - `4d0240b` (feat)
4. **Task 3: Restyle AlertsPanel with glow status dots and surface tier cards** - `3d3aaf8` (feat)

## Files Created/Modified
- `frontend/src/components/Toolbar.tsx` - All 6 inline SVGs replaced with MaterialIcon, border separators removed, dark-only backdrop-blur
- `frontend/src/components/SidePanel.tsx` - border-l and border-b removed, shadow-panel depth, bg-surface-high header, MaterialIcon close
- `frontend/src/components/ZoomControls.tsx` - Text buttons replaced with MaterialIcon, borders removed, dark-only backdrop-blur
- `frontend/src/components/AlertsPanel.tsx` - Glow status dots, borderless surface tier cards, MaterialIcon empty state, motion-reduce
- `frontend/src/components/ShortcutHelp.tsx` - No-line rule on rows and kbd, shadow-panel modal, bg-surface-high kbd, MaterialIcon close
- `frontend/src/components/ReconnectBanner.tsx` - Warm warning glow shadow, transition-colors

## Decisions Made
- AlertsPanel empty state SVG icon replaced with MaterialIcon check_circle (size=32) for system-wide icon consistency
- ShortcutHelp close button SVG also replaced with MaterialIcon close for consistency
- Prometheus status section cards (red/yellow border indicators) retained as-is -- these are semantic status borders, not layout separators under the no-line rule
- Warning severity badge retains bg-yellow-400/15 text-yellow-400 (non-token color) as it matches the existing pattern; full token migration for these can happen in a later pass

## Deviations from Plan

None - plan executed exactly as written.

## Issues Encountered
- Worktree was on an older commit; fast-forward merge to milestone branch was required to get MaterialIcon component from Plan 02-01

## User Setup Required
None - no external service configuration required.

## Next Phase Readiness
- All 6 navigation/utility components are restyled and ready for Phase 2 verification
- MaterialIcon is now used across Toolbar, SidePanel, ZoomControls, ShortcutHelp, AlertsPanel
- AlertsPanel glow pattern established for reuse in DeviceCard status indicators

## Known Stubs
None - all functionality is fully wired.

## Self-Check: PASSED

All files verified present. All commit hashes verified in git log.

---
*Phase: 02-component-restyling*
*Completed: 2026-03-25*
