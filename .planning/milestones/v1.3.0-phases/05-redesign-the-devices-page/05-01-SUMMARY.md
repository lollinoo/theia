---
phase: 05-redesign-the-devices-page
plan: 01
subsystem: ui
tags: [react, material-symbols, font-subset, dropdown, tailwind, typescript]

# Dependency graph
requires:
  - phase: 04-area-hub-view-and-filtered-topology
    provides: Material Symbols font infrastructure (21-icon subset), WebSocket lifted to App.tsx, areas state in App.tsx
provides:
  - Updated woff2 font subset with backup, history, description, expand_more icons (21 total)
  - Reusable FilterSelect dropdown component with outside-click-to-close and active indicator
  - Dashboard receives areas and snapshot props from App.tsx for Plan 02 consumption
affects: [05-02-restyle-filter-bar-and-table, 05-03-restyle-sidepanel-and-forms]

# Tech tracking
tech-stack:
  added: []
  patterns: [custom dropdown with useRef + mousedown listener, optional color dot in options]

key-files:
  created:
    - frontend/src/components/dashboard/FilterSelect.tsx
    - frontend/src/components/dashboard/FilterSelect.test.tsx
  modified:
    - frontend/public/fonts/material-symbols-rounded-subset.woff2
    - frontend/src/index.css
    - frontend/src/App.tsx
    - frontend/src/components/Dashboard.tsx

key-decisions:
  - "Font subset regenerated with pyftsubset using --unicodes for PUA codepoints + lowercase letters with rlig feature"
  - "FilterSelect uses mousedown listener for outside-click-to-close (consistent with existing codebase patterns)"
  - "Dashboard areas and snapshot props are optional to avoid breaking existing tests"

patterns-established:
  - "FilterSelect pattern: controlled dropdown with bg-primary/15 active indicator, z-20 stacking, color dot support"

requirements-completed: [COMP-07]

# Metrics
duration: 12min
completed: 2026-03-26
---

# Phase 05 Plan 01: Infrastructure for Devices Page Redesign Summary

**Material Symbols font expanded with 4 action icons, FilterSelect dropdown component with TDD tests, and areas/snapshot props threaded from App to Dashboard**

## Performance

- **Duration:** 12 min
- **Started:** 2026-03-26T21:46:52Z
- **Completed:** 2026-03-26T21:59:30Z
- **Tasks:** 2
- **Files modified:** 6

## Accomplishments
- Expanded Material Symbols Rounded font subset from 17 to 21 icons (added backup, history, description, expand_more)
- Created FilterSelect component with 8 test cases covering render, interaction, outside-click-to-close, active indicator, and color dot support
- Threaded areas and snapshot props from App.tsx to Dashboard for Plan 02 to use for area filter and uptime column

## Task Commits

Each task was committed atomically:

1. **Task 1: Expand Material Symbols font subset** - `907d835` (feat)
2. **Task 2 RED: Failing tests for FilterSelect** - `f841333` (test)
3. **Task 2 GREEN: FilterSelect component + prop threading** - `6bde495` (feat)

## Files Created/Modified
- `frontend/public/fonts/material-symbols-rounded-subset.woff2` - Updated woff2 with 21 icons (29KB)
- `frontend/src/index.css` - Updated icon count comment from 19 to 21
- `frontend/src/components/dashboard/FilterSelect.tsx` - Reusable custom select dropdown component
- `frontend/src/components/dashboard/FilterSelect.test.tsx` - 8 test cases for FilterSelect
- `frontend/src/App.tsx` - Pass areas and snapshot props to Dashboard
- `frontend/src/components/Dashboard.tsx` - Accept optional areas and snapshot props

## Decisions Made
- Font subset uses pyftsubset with explicit Unicode codepoints (--unicodes) and lowercase letters for rlig ligature matching, avoiding the layout closure that would pull in all 4000+ icon glyphs
- FilterSelect props are optional in Dashboard to avoid breaking existing test infrastructure
- TDD approach: failing tests committed separately, then implementation

## Deviations from Plan

None - plan executed exactly as written.

## Issues Encountered
- Font subsetting required careful handling: Material Symbols uses rlig (not liga/clig) for ligatures, and layout closure with a-z letters includes ALL icon glyphs (4MB). Solved by using --unicodes for specific PUA codepoints with --no-layout-closure to keep only relevant ligatures (29KB).
- Test 2 initially failed due to duplicate "All" text (trigger label + dropdown option). Fixed by using getAllByText with length assertion.

## User Setup Required

None - no external service configuration required.

## Next Phase Readiness
- Font subset ready for Plan 02 action icon buttons (backup, history, description)
- FilterSelect ready for Plan 02 to replace native select elements in the filter bar
- Dashboard now receives areas (for area filter + column) and snapshot (for uptime column)
- All 119 tests pass

---
*Phase: 05-redesign-the-devices-page*
*Completed: 2026-03-26*
