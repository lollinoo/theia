---
phase: 01-design-token-foundation-and-theme-infrastructure
plan: 01
subsystem: ui
tags: [tailwind-v4, reactflow-v12, css-tokens, fontsource, fowt-prevention, design-tokens, theme-switching]

# Dependency graph
requires:
  - phase: none
    provides: first phase, no dependencies
provides:
  - Tailwind v4 with @tailwindcss/vite plugin and @theme inline token system
  - Complete dark/light Neon Topography CSS custom property palettes
  - ReactFlow v12 (@xyflow/react) installed with CSS variable overrides
  - Outfit and JetBrains Mono variable fonts via Fontsource
  - FOWT prevention inline script in index.html
  - data-theme attribute-based theme switching infrastructure
affects: [01-02, 01-03, phase-2-component-restyling]

# Tech tracking
tech-stack:
  added: [tailwindcss@4.2.2, "@tailwindcss/vite@4.2.2", "@xyflow/react@12.10.1", "@fontsource-variable/outfit@5.2.8", "@fontsource-variable/jetbrains-mono@5.2.8"]
  removed: [reactflow@11.11.4, autoprefixer@10.4.20, postcss.config.js, tailwind.config.js]
  patterns: [CSS custom properties for theming, "@theme inline" for Tailwind v4 token generation, data-theme attribute selector for dark/light mode]

key-files:
  created: []
  modified: [frontend/src/index.css, frontend/index.html, frontend/vite.config.ts, frontend/src/main.tsx, frontend/package.json]
  deleted: [frontend/postcss.config.js, frontend/tailwind.config.js]

key-decisions:
  - "Used @theme inline (not plain @theme) for all tokens referencing CSS variables -- required for runtime theme switching"
  - "Removed postcss.config.js entirely -- @tailwindcss/vite handles all CSS processing"
  - "ReactFlow styles imported inside @layer base in index.css instead of main.tsx -- keeps specificity below @theme tokens"

patterns-established:
  - "CSS token naming: --nt-* for primitive theme variables, --color-* for semantic Tailwind tokens"
  - "Theme switching via data-theme attribute on html element (not class-based)"
  - "FOWT prevention via synchronous inline script in head reading localStorage('theia-theme')"
  - "Font stack: Outfit Variable for display/sans, JetBrains Mono Variable for mono"

requirements-completed: [FOUND-01, FOUND-02, FOUND-03, FOUND-04, FOUND-05, THEME-04]

# Metrics
duration: 4min
completed: 2026-03-25
---

# Phase 1 Plan 1: Library Upgrades and CSS Token Foundation Summary

**Tailwind v4 + @theme inline token system with dark/light Neon Topography palettes, ReactFlow v12, Fontsource fonts, and FOWT prevention script**

## Performance

- **Duration:** 4 min
- **Started:** 2026-03-25T13:35:53Z
- **Completed:** 2026-03-25T13:39:49Z
- **Tasks:** 2
- **Files modified:** 7 (including package-lock.json)

## Accomplishments
- Upgraded from Tailwind CSS v3 to v4 with @tailwindcss/vite plugin replacing PostCSS pipeline
- Installed ReactFlow v12 (@xyflow/react) replacing the deprecated reactflow package
- Built complete CSS token system with 30+ design tokens for surfaces, text, borders, glass, status, accents, shadows, and radii
- Defined dark (#161618 base) and light (#F5F5F7 base) Neon Topography palettes as CSS custom properties
- Added FOWT prevention inline script that reads localStorage and OS preference before first paint
- Installed and configured Outfit and JetBrains Mono self-hosted variable fonts

## Task Commits

Each task was committed atomically:

1. **Task 1: Upgrade packages and reconfigure build pipeline** - `9d046c9` (feat)
2. **Task 2: Write complete CSS token system and FOWT prevention script** - `da6afc6` (feat)

## Files Created/Modified
- `frontend/package.json` - Updated dependencies: added @xyflow/react, @tailwindcss/vite, tailwindcss v4, Fontsource fonts; removed reactflow, autoprefixer
- `frontend/package-lock.json` - Lockfile updated for dependency changes
- `frontend/vite.config.ts` - Added @tailwindcss/vite plugin to Vite build pipeline
- `frontend/src/index.css` - Complete rewrite: @theme inline token system, dark/light palettes, font imports, ReactFlow CSS overrides
- `frontend/index.html` - Replaced class="dark" with data-theme="dark", added FOWT prevention inline script
- `frontend/src/main.tsx` - Removed reactflow/dist/style.css import (moved to index.css)
- `frontend/postcss.config.js` - Deleted (replaced by @tailwindcss/vite)
- `frontend/tailwind.config.js` - Deleted (config moved to CSS @theme inline)

## Decisions Made
- Used `@theme inline` (not plain `@theme`) for all tokens referencing CSS variables -- this is required for runtime theme switching because plain `@theme` would bake in the variable values at build time
- Imported ReactFlow v12 styles inside `@layer base` in index.css rather than in main.tsx -- keeps specificity below `@theme` tokens and centralizes all CSS
- Kept `postcss` in devDependencies (Vite uses it internally) but deleted the config file since @tailwindcss/vite handles processing
- Used `@custom-variant dark` with `data-theme` attribute selector instead of Tailwind's default class-based dark mode

## Deviations from Plan

None - plan executed exactly as written.

## Issues Encountered
- `npx vite build` fails because other source files (App.tsx, Canvas.tsx, DeviceCard.tsx, LinkEdge.tsx) still import from the old `reactflow` package. This is expected and will be resolved in Plan 01-02 (ReactFlow v12 import migration). The CSS pipeline itself compiles successfully as verified via `@tailwindcss/cli`.

## User Setup Required

None - no external service configuration required.

## Next Phase Readiness
- CSS token system is in place -- Plan 01-02 can proceed with ThemeProvider context and ReactFlow v12 import migration
- Plan 01-03 can then replace hardcoded hex values with the new semantic token classes (bg-bg, bg-surface, text-on-bg, etc.)
- Full vite build will work once Plan 01-02 migrates reactflow imports to @xyflow/react

## Self-Check: PASSED

All files verified present, deleted files confirmed removed, both commit hashes found in git log.

---
*Phase: 01-design-token-foundation-and-theme-infrastructure*
*Completed: 2026-03-25*
