# Phase 2: Component Restyling - Context

**Gathered:** 2026-03-25
**Status:** Ready for planning

<domain>
## Phase Boundary

Restyle all 25+ existing frontend components to match the Neon Topography design language. Every component must be visually correct in both dark and light themes. This phase does NOT add new features, views, or capabilities — it transforms the visual presentation of existing UI using the token system established in Phase 1.

</domain>

<decisions>
## Implementation Decisions

### Icon Strategy
- **D-01:** Use Material Symbols (self-hosted variable font, woff2 subset) for all icons across the application
- **D-02:** Icon style is Material Symbols Rounded — softer corners match panel radius (12px) and pill geometry
- **D-03:** Replace all Phase 1 inline Heroicons SVGs (sun/moon theme toggle) with Material Symbols equivalents for system-wide consistency
- **D-04:** Self-host the font — no CDN dependency. Subset to only the icons needed (~15-20 icons, ~100KB woff2)

### Overlay Surfaces
- **D-05:** Glassmorphism (backdrop-blur + translucent bg) is dark-mode only for overlay surfaces (context menu, search overlay), consistent with Phase 1 decision D-07
- **D-06:** Light-mode overlays use tinted solid surfaces: `rgba(255,255,255,0.85)` background, no `backdrop-filter`, subtle border `rgba(0,0,0,0.06)`
- **D-07:** Dark-mode glassmorphism for context menu and search overlay uses medium opacity range (0.06-0.10) — more substance than the area background spec (0.02) for readability over charcoal

### Bloom/Glow Performance
- **D-08:** Canvas node glow uses CSS box-shadow only — no `backdrop-filter`, no pseudo-element radial gradients. Must maintain 60fps at 100+ nodes
- **D-09:** Glow intensity scales with device status severity: critical states (down, warning) get larger spread and higher opacity shadows; healthy 'up' gets subtle glow. Draws operator's eye to problems
- **D-10:** Off-canvas elements (panel status indicators, future area cards) — Claude's discretion on whether to use richer bloom (radial-blur) where element count is low

### DeviceCard Restructuring
- **D-11:** Remove the 3px colored top border accent. Replace with a Glow Node (rounded-full element with status-colored box-shadow bloom) in the card header for status indication
- **D-12:** Internal section separation uses surface color tiers (no-line rule): header on `surface`, body on `bg`, metrics area on `surface-high`. No border separators
- **D-13:** Remove the 6 decorative bottom port dots entirely. ReactFlow handles on hover provide connection points already
- **D-14:** Vendor badge switches from colored tertiary pill to muted JetBrains Mono monospace tag with subtle `surface-high` background — less attention-grabbing, more terminal feel
- **D-15:** DeviceCard hover accent — Claude's discretion on whether to use primary green glow or status-matched glow

### Claude's Discretion
- Exact glow shadow spread/opacity values per status level (within the box-shadow-only constraint)
- DeviceCard hover accent color strategy (primary green vs status-matched)
- Whether off-canvas bloom uses radial-blur or stays box-shadow-only
- Component restyling order and wave grouping for parallel execution
- Exact Material Symbols icon names for each context menu action
- Transition timing and easing for theme-switch animations on restyled components
- How to handle the `metricColor()` function — whether to keep threshold-based coloring or simplify

</decisions>

<canonical_refs>
## Canonical References

**Downstream agents MUST read these before planning or implementing.**

### Design System
- `.planning/DESIGN.md` — Neon Topography design system specification: colors, typography, elevation, glassmorphism, component rules, do's and don'ts
- `.planning/examples_mocks/node_context_menu/dark/code.html` — Context menu dark theme HTML mock (target visual)
- `.planning/examples_mocks/node_context_menu/light/code.html` — Context menu light theme HTML mock
- `.planning/examples_mocks/node_context_menu/dark/screen.png` — Context menu dark screenshot
- `.planning/examples_mocks/node_context_menu/light/screen.png` — Context menu light screenshot
- `.planning/examples_mocks/ospf_area_hub/dark/screen.png` — Area Hub dark screenshot (shows target card style)
- `.planning/examples_mocks/ospf_area_hub/light/screen.png` — Area Hub light screenshot

### Token System (Phase 1 output)
- `frontend/src/index.css` — CSS token definitions: `--nt-*` primitives, `@theme inline` semantic mappings, ReactFlow CSS variable overrides, dark/light theme blocks
- `frontend/src/components/ThemeProvider.tsx` — Theme context, `data-theme` attribute management, localStorage persistence

### Requirements
- `.planning/REQUIREMENTS.md` — COMP-01 through COMP-12, THEME-05 define Phase 2 acceptance criteria
- `.planning/ROADMAP.md` — Phase 2 success criteria and dependency chain

### Phase 1 Context
- `.planning/phases/01-design-token-foundation-and-theme-infrastructure/01-CONTEXT.md` — Phase 1 decisions (D-01 through D-11) that constrain Phase 2 work

</canonical_refs>

<code_context>
## Existing Code Insights

### Reusable Assets
- `frontend/src/index.css` — Complete token system with dark/light variants, semantic Tailwind mappings, glassmorphism variables
- `frontend/src/components/ThemeProvider.tsx` — Theme context with `useTheme()` hook for accessing current theme
- `frontend/src/components/StatusDot.tsx` — Existing status indicator component (will be enhanced with glow)
- `frontend/src/components/icons/DeviceIcon.tsx` — Device type icon component (may need Material Symbols integration)
- `frontend/src/types/metrics.ts` — `metricColor()` function for threshold-based metric coloring

### Established Patterns
- All styling via Tailwind utility classes — no CSS modules or styled-components
- All color values already use CSS variable tokens from Phase 1 (`bg-surface`, `text-on-bg`, etc.)
- Components use `memo()` with custom comparators for React performance (DeviceCard pattern)
- `font-mono` maps to JetBrains Mono, `font-sans`/`font-display` maps to Outfit via `@theme inline`

### Integration Points
- `frontend/src/components/ContextMenu.tsx` — Primary target for glassmorphism + Material Symbols icons
- `frontend/src/components/DeviceCard.tsx` — Primary target for glow node + surface tier restructuring
- `frontend/src/components/NavBar.tsx` — Theme toggle icons need Material Symbols replacement
- `frontend/src/components/SearchOverlay.tsx` — Overlay surface treatment (glassmorphism dark / solid light)
- 26 component files total in `frontend/src/components/` requiring visual audit

</code_context>

<specifics>
## Specific Ideas

- DeviceCard should feel like a "status panel" per DESIGN.md — the glow node is the live heartbeat indicator
- Vendor badge should feel like a terminal tag — small, monospace, unobtrusive
- Context menu should reference the HTML mocks in `.planning/examples_mocks/node_context_menu/` for exact visual target
- Severity-scaled glow means a network operator scanning the canvas immediately sees red-glowing problem devices
- The no-line rule is absolute — no 1px borders for layout sectioning anywhere. Surface color shifts and spacing only

</specifics>

<deferred>
## Deferred Ideas

- Canvas.tsx decomposition (750 lines) — should happen before Phase 4 adds area filtering, not in Phase 2
- NavigationPill component — Phase 4 scope, theme toggle gets absorbed there
- Area-specific accent coloring on DeviceCards — Phase 4 scope (requires area assignment from Phase 3)

</deferred>

---

*Phase: 02-component-restyling*
*Context gathered: 2026-03-25*
