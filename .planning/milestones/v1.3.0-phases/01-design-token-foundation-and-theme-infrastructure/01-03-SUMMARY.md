---
phase: 01-design-token-foundation-and-theme-infrastructure
plan: 03
subsystem: ui
tags: [css-variables, design-tokens, tailwind-v4, theme-switching, hex-to-token, dark-mode, light-mode]

# Dependency graph
requires:
  - phase: 01-01
    provides: CSS token system with @theme inline, dark/light palettes, status/surface/text/outline tokens
  - phase: 01-02
    provides: ThemeProvider context, ReactFlow v12 imports, colorMode integration, NavBar toggle
provides:
  - Zero hardcoded hex colors in any production source file
  - All component files using semantic CSS variable tokens for colors
  - All Tailwind class names migrated to new design token naming convention
  - SVG icons using var(--nt-bg) for contrast elements
  - utilizationColor() and statusColor() functions returning CSS variable references
affects: [phase-2-component-restyling]

# Tech tracking
tech-stack:
  added: []
  patterns: [CSS variable references in inline styles via var(--color-*) and var(--nt-*), semantic Tailwind token classes replacing arbitrary hex values]

key-files:
  created: []
  modified:
    - frontend/src/components/Canvas.tsx
    - frontend/src/components/LinkEdge.tsx
    - frontend/src/components/DeviceCard.tsx
    - frontend/src/components/icons/DeviceIcon.tsx
    - frontend/src/types/metrics.ts
    - frontend/src/components/InterfaceStatsPanel.tsx
    - frontend/src/components/LinkDetailsPanel.tsx
    - frontend/src/components/LinkCreatePanel.tsx
    - frontend/src/components/AddDevicePanel.tsx
    - frontend/src/components/AlertsPanel.tsx
    - frontend/src/components/ContextMenu.tsx
    - frontend/src/components/Dashboard.tsx
    - frontend/src/components/DeviceConfigPanel.tsx
    - frontend/src/components/SNMPProfileManager.tsx
    - frontend/src/components/SSHProfileManager.tsx
    - frontend/src/components/SearchOverlay.tsx
    - frontend/src/components/SettingsPanel.tsx
    - frontend/src/components/ShortcutHelp.tsx
    - frontend/src/components/SidePanel.tsx
    - frontend/src/components/Toolbar.tsx
    - frontend/src/components/ZoomControls.tsx
    - frontend/src/components/dashboard/BackupHistoryTable.tsx
    - frontend/src/components/dashboard/BackupPanel.tsx
    - frontend/src/components/dashboard/BulkBackupPanel.tsx
    - frontend/src/components/dashboard/ConfigViewer.tsx
    - frontend/src/components/dashboard/DeviceRow.tsx
    - frontend/src/components/dashboard/DeviceTable.tsx
    - frontend/src/components/dashboard/SSHCredentialForm.tsx
    - frontend/src/components/dashboard/VendorSettingsPanel.tsx

key-decisions:
  - "Used var(--nt-bg) instead of currentColor for SVG icon contrast elements -- currentColor would make them invisible against the parent fill"
  - "Mapped accent to primary, accent-purple to tertiary in the new token system"
  - "Extended Tailwind class migration beyond the 8 planned files to all 26 component files for consistency"

patterns-established:
  - "Status colors always via var(--color-status-*) tokens, never hardcoded hex"
  - "Surface/outline/text colors via var(--nt-*) or var(--color-*) CSS variable references in inline styles"
  - "Tailwind token class naming: bg-bg, bg-surface, bg-elevated, text-on-bg, text-on-bg-secondary, border-outline, text-primary, bg-primary, text-tertiary"

requirements-completed: [FOUND-06]

# Metrics
duration: 7min
completed: 2026-03-25
---

# Phase 1 Plan 3: Hardcoded Hex-to-Token Migration Summary

**All 45+ hardcoded hex color values replaced with CSS variable token references across 32 source files, with full Tailwind class name migration to new design token system**

## Performance

- **Duration:** 7 min
- **Started:** 2026-03-25T18:47:31Z
- **Completed:** 2026-03-25T18:55:19Z
- **Tasks:** 3
- **Files modified:** 32

## Accomplishments
- Replaced all hardcoded hex color values (#ff1744, #ffc107, #00c853, #657786, #4a4a5e, #363647, #3f3f53, #8899a6, #2d2d3d, #1a1a24, #12121a, #666, #7a7a9e) with CSS variable references
- Migrated utilizationColor() and statusColor() functions to return var(--color-status-*) tokens instead of hex strings
- Updated SVG icon fills from hardcoded #2d2d3d to var(--nt-bg) for theme-responsive contrast
- Migrated all old Tailwind class names across 26 component files (bg-bg-canvas -> bg-bg, text-text-primary -> text-on-bg, border-border-subtle -> border-outline, text-accent -> text-primary, etc.)
- Vite build succeeds and all 55 tests pass

## Task Commits

Each task was committed atomically:

1. **Task 1: Replace hardcoded hex values in Canvas.tsx and LinkEdge.tsx** - `d8b09d2` (feat)
2. **Task 2: Replace hardcoded hex values in DeviceCard, DeviceIcon, metrics.ts, and remaining panels** - `020aff9` (feat)
3. **Task 3: Update remaining Tailwind class names across all component files and verify build** - `0328239` (refactor)

## Files Created/Modified
- `frontend/src/components/Canvas.tsx` - statusColor() returns var(--color-status-*); MiniMap/Background/connectionLine use token references
- `frontend/src/components/LinkEdge.tsx` - All 15 strokeColor/throughputColor hex values replaced with CSS variable references
- `frontend/src/components/DeviceCard.tsx` - bg-[#1a1a24] -> bg-surface, bg-[#12121a] -> bg-bg, !bg-[#8899a6] -> !bg-on-bg-secondary, old class names migrated
- `frontend/src/components/icons/DeviceIcon.tsx` - 7 SVG fill/stroke #2d2d3d -> var(--nt-bg), text-accent -> text-tertiary
- `frontend/src/types/metrics.ts` - utilizationColor() returns var(--color-status-*) instead of hex
- `frontend/src/components/InterfaceStatsPanel.tsx` - Fallback color #657786 -> var(--color-status-unknown)
- `frontend/src/components/LinkDetailsPanel.tsx` - color: '#666' -> var(--nt-on-bg-muted)
- `frontend/src/components/LinkCreatePanel.tsx` - color: '#666' -> var(--nt-on-bg-muted)
- 24 additional component files - Old Tailwind class names (bg-bg-canvas, text-text-primary, border-border-subtle, text-accent, bg-accent, etc.) migrated to new token names

## Decisions Made
- Used `var(--nt-bg)` instead of `currentColor` for SVG icon contrast elements (router LED circles, switch port rectangles) -- using currentColor would make them the same color as the body fill, making them invisible
- Mapped `accent` -> `primary` and `accent-purple` -> `tertiary` following the token system's color naming hierarchy
- Extended the Tailwind class migration beyond the 8 files specified in the plan to all 26 component files that contained old class names -- this was necessary for consistency and to avoid a mix of old and new naming

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 2 - Missing Critical] Extended Tailwind class migration to all 26 component files**
- **Found during:** Task 3 (Tailwind class name sweep)
- **Issue:** Plan only specified Canvas.tsx, DeviceCard.tsx, and LinkEdge.tsx in the Task 3 files tag, but the action text said "sweep ALL component files." Grep revealed 26 files with old class names.
- **Fix:** Applied consistent sed replacements across all 26 files in frontend/src/
- **Files modified:** All 26 files listed in git diff
- **Verification:** grep confirms zero old class names remain; build and tests pass
- **Committed in:** 0328239 (Task 3 commit)

---

**Total deviations:** 1 auto-fixed (1 missing critical)
**Impact on plan:** Migration scope was larger than frontmatter suggested but aligned with action text. No scope creep.

## Issues Encountered
None -- all replacements were straightforward string substitutions. Build and tests passed on first attempt.

## User Setup Required

None - no external service configuration required.

## Next Phase Readiness
- Phase 01 (design-token-foundation-and-theme-infrastructure) is now complete across all 3 plans
- All CSS tokens defined (Plan 01), ThemeProvider + ReactFlow wired (Plan 02), hex-to-token migration done (Plan 03)
- FOUND-06 requirement fully satisfied: zero hardcoded hex colors remain in production source files
- Phase 2 (component restyling) can proceed with confidence that all colors respond to theme switching
- Status indicators, link colors, device cards, SVG icons, and panel elements all adapt to dark/light themes

## Known Stubs
None -- all token references resolve against the CSS variable system defined in index.css.

## Self-Check: PASSED

All 8 key modified files verified present. All 3 task commit hashes found in git log. SUMMARY.md created. Vite build succeeds. All 55 tests pass. Zero hardcoded hex colors remain in production source files.

---
*Phase: 01-design-token-foundation-and-theme-infrastructure*
*Completed: 2026-03-25*
