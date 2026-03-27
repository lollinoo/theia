# Phase 1: Design Token Foundation and Theme Infrastructure - Context

**Gathered:** 2026-03-25
**Status:** Ready for planning

<domain>
## Phase Boundary

Deliver the complete CSS token system, library upgrades (Tailwind v4, ReactFlow v12), self-hosted fonts (Outfit + JetBrains Mono), and working dark/light theme switching with persistence and FOWT prevention. This is the foundation that every subsequent phase depends on. No component restyling beyond what's needed to verify tokens work.

</domain>

<decisions>
## Implementation Decisions

### Theme Toggle Location
- **D-01:** Theme toggle lives in the current NavBar, right side, as a sun/moon icon swap button
- **D-02:** Icon style is a single icon that swaps between sun (light mode active) and moon (dark mode active) — not a pill slider
- **D-03:** This toggle location is temporary — it gets absorbed into the NavigationPill in Phase 2

### Light Theme Palette
- **D-04:** Light theme base background is cool gray `#F5F5F7` (Apple-style, not pure white)
- **D-05:** Light theme surface hierarchy: Background `#F5F5F7` > Surface `#EDEDF0` > Elevated `#FFFFFF` > Text `#1A1A1C`
- **D-06:** Primary green accent `#00E676` stays the same hue in light theme — glow intensity reduced (shadow opacity ~0.25 instead of ~0.5, bloom opacity ~0.06 instead of ~0.15)
- **D-07:** Glassmorphism is a dark-mode-only signature. Light theme uses tinted solid surfaces: `rgba(255,255,255,0.85)` background, no `backdrop-filter`, subtle border `rgba(0,0,0,0.06)`

### Status Colors
- **D-08:** Status colors are theme-invariant — same hex values in both dark and light themes. Glow intensity varies by theme, hue does not
- **D-09:** Status 'up' color aligns with primary glow: `#00E676` (not the old `#00c853`)
- **D-10:** Full status palette: Up `#00E676`, Down `#FF1744`, Probing `#FFEA00`, Unknown `#9E9E9E`

### Token Palette Scope
- **D-11:** Area-specific accent colors (Secondary `#2979FF` blue, Tertiary `#E040FB` purple) are included in the Phase 1 token system even though areas aren't built until Phase 3/4. Complete token system from day one.

### Claude's Discretion
- Token naming conventions (semantic names that bridge DESIGN.md vocabulary and Tailwind conventions)
- Number of surface tiers beyond the 3 defined for light theme (dark theme may need more granularity per DESIGN.md)
- Exact glow/bloom CSS values — guidelines are set (reduced intensity in light mode), Claude tunes specific values
- ReactFlow v12 migration approach and `useAutoLayout.ts` adaptation for `node.measured` API change
- Tailwind v4 migration strategy (codemod-first vs manual)
- FOWT prevention inline script implementation

</decisions>

<canonical_refs>
## Canonical References

**Downstream agents MUST read these before planning or implementing.**

### Design System
- `.planning/DESIGN.md` — Neon Topography design system specification: colors, typography, elevation, glassmorphism, component rules, do's and don'ts
- `.planning/examples_mocks/ospf_area_hub/` — OSPF Area Hub HTML mocks (dark + light) showing target visual language
- `.planning/examples_mocks/node_context_menu/` — Node Context Menu HTML mocks (dark + light)

### Requirements
- `.planning/REQUIREMENTS.md` — FOUND-01 through FOUND-06, THEME-01 through THEME-04 define Phase 1 acceptance criteria
- `.planning/ROADMAP.md` — Phase 1 success criteria and dependency chain

### Research
- `.planning/research/SUMMARY.md` — Consolidated research findings, recommended stack delta, architecture approach, and critical pitfalls
- `.planning/research/STACK.md` — Detailed package versions, migration guides, Tailwind v4 and ReactFlow v12 specifics
- `.planning/research/ARCHITECTURE.md` — ThemeProvider design, token architecture, CSS variable cascade strategy
- `.planning/research/PITFALLS.md` — Hardcoded hex audit scope (40 occurrences in 6 files), FOWT prevention, font CLS

### Current Frontend
- `frontend/tailwind.config.js` — Current Tailwind v3 config with custom colors (to be replaced by CSS `@theme`)
- `frontend/src/index.css` — Current base styles with hardcoded colors and IBM Plex Sans (to be overhauled)
- `frontend/package.json` — Current dependency versions (reactflow 11.11, tailwindcss 3.4)

</canonical_refs>

<code_context>
## Existing Code Insights

### Reusable Assets
- `frontend/tailwind.config.js` — Already has `darkMode: 'class'` configured and a semantic color naming pattern (`bg-canvas`, `bg-surface`, `bg-elevated`, `status-up`, etc.) that can inform token naming
- `frontend/src/components/NavBar.tsx` — Existing NavBar where the theme toggle will be added
- `frontend/src/App.tsx` — Clean entry point already using Tailwind semantic classes (`bg-bg-canvas`, `text-text-primary`)

### Established Patterns
- All styling via Tailwind utility classes — no separate CSS modules or styled-components
- No `@/` path aliases — all imports use relative paths
- PostCSS 8 + autoprefixer pipeline (will be replaced by `@tailwindcss/vite` plugin)

### Integration Points
- `frontend/index.html` — FOWT prevention inline script goes here in `<head>`
- `frontend/src/index.css` — Token definitions via `@theme` directive replace current `@layer base` block
- 6 component files with 40 hardcoded hex values: `Canvas.tsx`, `LinkEdge.tsx`, `DeviceCard.tsx`, `DeviceIcon.tsx`, `InterfaceStatsPanel.tsx`, `types/metrics.ts`

</code_context>

<specifics>
## Specific Ideas

- Light theme character should feel "Apple-like" — clean, professional, cool-toned
- Glassmorphism is the dark theme's signature visual; light theme gets solid tinted surfaces instead
- Green glow is the system's identity — it stays in both themes, just softer in light mode
- Status colors are universal — a network operator should recognize "green = up" instantly regardless of theme

</specifics>

<deferred>
## Deferred Ideas

None — discussion stayed within phase scope

</deferred>

---

*Phase: 01-design-token-foundation-and-theme-infrastructure*
*Context gathered: 2026-03-25*
