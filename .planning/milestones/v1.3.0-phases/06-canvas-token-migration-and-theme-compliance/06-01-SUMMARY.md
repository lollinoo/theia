---
phase: 06-canvas-token-migration-and-theme-compliance
plan: 01
subsystem: ui
tags: [tailwind, css-tokens, theme, canvas]

requires:
  - phase: 01-design-token-foundation-and-theme-infrastructure
    provides: "@theme inline token definitions in index.css"
provides:
  - "Canvas token audit test scanning 6 files for stale tokens and hardcoded hex"
  - "4 smaller canvas files migrated to valid Tailwind v4 theme tokens"
  - "statusColor() returns CSS variable references instead of hardcoded hex"
affects: [06-02]

tech-stack:
  added: []
  patterns: ["CSS variable references for ReactFlow MiniMap node colors"]

key-files:
  created:
    - frontend/src/components/__tests__/canvas-token-audit.test.ts
  modified:
    - frontend/src/components/canvas/canvasHelpers.ts
    - frontend/src/App.tsx
    - frontend/src/components/ReconnectBanner.tsx
    - frontend/src/components/canvas/CanvasPanels.tsx

key-decisions:
  - "statusColor() returns var(--color-status-*) CSS variables for MiniMap theme adaptivity"
  - "ReconnectBanner uses semantic warning tokens instead of fixed yellow palette"

patterns-established:
  - "CSS variable references for inline style values that need theme adaptivity"

requirements-completed: [FOUND-06, THEME-05, COMP-12]

duration: 12min
completed: 2026-03-27
---

# Phase 06 Plan 01: Canvas Token Audit + Small File Migration Summary

**Canvas token audit test created; 4 files migrated from stale/hardcoded values to valid Tailwind v4 theme tokens**

## Performance

- **Duration:** 12 min
- **Started:** 2026-03-27T08:40:00Z
- **Completed:** 2026-03-27T08:52:00Z
- **Tasks:** 2
- **Files modified:** 5

## Accomplishments
- Created canvas-token-audit.test.ts scanning 6 target files for stale tokens and hardcoded hex
- Migrated statusColor() from hardcoded hex (#00c853, etc.) to CSS variable references (var(--color-status-up), etc.)
- Replaced stale bg-bg-canvas/text-text-primary with valid bg-bg/text-on-bg in App.tsx
- Converted ReconnectBanner from fixed yellow-* palette to semantic warning tokens
- Updated CanvasPanels fallback text from text-text-secondary to text-on-bg-secondary

## Task Commits

Each task was committed atomically:

1. **Task 1: Create canvas-token-audit.test.ts** - `c383f6f` (test)
2. **Task 2: Migrate 4 canvas files to valid tokens** - `1280083` (feat)

## Files Created/Modified
- `frontend/src/components/__tests__/canvas-token-audit.test.ts` - Source-scan test for stale tokens and hardcoded hex across 6 canvas files
- `frontend/src/components/canvas/canvasHelpers.ts` - statusColor() returns CSS variable references
- `frontend/src/App.tsx` - Root container uses bg-bg text-on-bg
- `frontend/src/components/ReconnectBanner.tsx` - Semantic warning tokens replace yellow palette
- `frontend/src/components/canvas/CanvasPanels.tsx` - Valid on-bg-secondary token

## Decisions Made
- Used `var(--color-status-*)` CSS variable references in statusColor() since MiniMap nodeColor accepts any CSS color string — this makes MiniMap dots theme-adaptive without needing Tailwind classes

## Deviations from Plan
None - plan executed exactly as written

## Issues Encountered
None

## User Setup Required
None - no external service configuration required.

## Next Phase Readiness
- Canvas token audit test now validates all 6 files — remaining violations are in Canvas.tsx and CanvasOverlays.tsx (Plan 02 targets)
- statusColor() CSS variable pattern established for Plan 02's MiniMap background migration

---
*Phase: 06-canvas-token-migration-and-theme-compliance*
*Completed: 2026-03-27*
