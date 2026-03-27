# Pitfalls Research

**Domain:** Frontend Redesign -- Design System, Theming, and Navigation for Existing React + ReactFlow App
**Researched:** 2026-03-25
**Confidence:** HIGH (codebase-specific analysis combined with documented community patterns)

## Critical Pitfalls

### Pitfall 1: Glow/Shadow Performance Death on the ReactFlow Canvas at 100+ Nodes

**What goes wrong:**
The Neon Topography design system calls for glow status indicators (`box-shadow` with colored blur), bloom effects (large-radius radial blurs behind content blocks), and glassmorphism (`backdrop-filter: blur()`). The existing DeviceCard already uses 9 shadow/animation/transition declarations per node. At 100+ nodes on a ReactFlow canvas, every node drag or pan/zoom triggers a repaint of all visible nodes. Adding heavier `box-shadow` blur values (the spec calls for `0 24px 48px` ambient shadows) and `backdrop-filter` to each node will cause visible jank -- `box-shadow` blur cost scales roughly quadratically with blur radius, and `backdrop-filter` triggers a separate GPU blur calculation per element.

**Why it happens:**
The design system was spec'd for static page layouts (Area Hub cards, nav pills) where there are 5-10 glassmorphic elements at most. Nobody tested how those same effects perform when 100+ nodes are dragged across a canvas simultaneously. ReactFlow's own docs explicitly warn: "Complex CSS styles, particularly those involving animations, shadows, or gradients, can significantly impact performance" at high node counts.

**How to avoid:**
1. Create two visual tiers: "canvas mode" and "detail mode." Canvas nodes get simplified shadows (single-layer, small blur radius like `0 4px 12px`) and NO `backdrop-filter`. Full Neon Topography styling (bloom, glassmorphism, heavy shadows) only applies in the Area Hub view and panels where element count is bounded.
2. Use the pseudo-element opacity trick for glow animations: place the glow shadow on a `::after` element with `opacity: 0`, then animate only `opacity` on status change. Opacity is a compositor-friendly property (GPU-accelerated, no repaint).
3. Set `onlyRenderVisibleElements={true}` on ReactFlow to avoid rendering off-screen nodes.
4. Benchmark with 100+ device nodes on a mid-range device before merging any visual phase.

**Warning signs:**
- FPS drops below 30 during canvas pan/zoom with devtools Performance panel open
- "Recalculate Style" or "Paint" entries dominating the flame chart
- Visible stutter when dragging a node while many others are visible

**Phase to address:**
Phase 1 (Design Token Foundation) -- define the two-tier visual strategy upfront. Phase 3 (Canvas Restyle) -- enforce simplified shadows on canvas nodes and benchmark.

---

### Pitfall 2: Hardcoded Hex Colors Scattered Across 29 Files Block Theme Switching

**What goes wrong:**
The current codebase has `bg-[#1a1a24]`, `bg-[#12121a]`, `bg-[#8899a6]` and similar hardcoded hex values in Tailwind arbitrary value syntax throughout DeviceCard and other components (4 occurrences in DeviceCard alone, 80+ shadow/transition declarations with hex colors across 29 files). Tailwind's `darkMode: 'class'` strategy requires every color to be expressed through theme tokens that can flip with the `dark:` variant or CSS custom properties. Any hardcoded hex value will remain static when the user switches between dark and light themes, creating a patchwork of themed and un-themed elements.

**Why it happens:**
During rapid v1 development, hardcoded hex values are faster to write than defining a Tailwind theme extension for every one-off color. The codebase had no light mode requirement, so there was no incentive to abstract colors. Now these values are load-bearing in 29 component files.

**How to avoid:**
1. Before touching any component, audit and extract every hardcoded color into the Tailwind theme config as semantic tokens (e.g., `surface-header`, `surface-body`, `text-muted`). Map each to a CSS custom property.
2. Define the full Neon Topography palette as CSS custom properties on `:root` (dark) and `.light` / `[data-theme="light"]` (light), then reference them in `tailwind.config`.
3. Use a script or regex search (`bg-\[#`, `text-\[#`, `border-\[#`, `shadow-\[.*#`) to find and replace all arbitrary hex values before starting component restyling.
4. Lint rule: add a Tailwind ESLint or Biome rule to disallow arbitrary color values in new code.

**Warning signs:**
- Switching themes leaves some elements unchanged (white text on white background, dark panel on light canvas)
- Searching for `[#` in component files still returns matches after the migration phase

**Phase to address:**
Phase 1 (Design Token Foundation) -- this must be the very first work. Every subsequent phase depends on colors being tokenized.

---

### Pitfall 3: Flash of Wrong Theme (FOWT) on Page Load

**What goes wrong:**
The user selects light mode, refreshes the page, and sees the dark theme flash for 200-500ms before React hydrates and applies the correct theme class. The current `index.html` has `<html lang="en" class="dark">` hardcoded, and no script runs before the first paint. With Vite's client-side rendering, React mounts after the CSS is parsed, meaning the browser renders the `dark`-themed styles, then JavaScript reads localStorage and flips to `light`, causing a visible flash.

**Why it happens:**
Theme preference is stored in JavaScript-land (localStorage or React state) but CSS renders before JavaScript executes. Without a blocking inline script in `<head>`, the browser will always render with whatever class is in the static HTML first.

**How to avoid:**
Add a tiny inline `<script>` in `index.html` `<head>`, before any stylesheet, that reads the theme preference from localStorage and sets the class on `<html>`:
```html
<script>
  (function() {
    var t = localStorage.getItem('theia-theme');
    if (t === 'light') document.documentElement.classList.replace('dark', 'light');
  })();
</script>
```
This executes synchronously before first paint, so the browser never renders the wrong theme. Keep the script minimal (no imports, no async) to avoid blocking.

**Warning signs:**
- Visible color flicker on page refresh when in non-default theme
- Users report "flash of dark mode" when using light mode

**Phase to address:**
Phase 2 (Theme Infrastructure) -- implement alongside the theme toggle.

---

### Pitfall 4: Font Loading Causes Layout Shift (CLS) with Dual-Font Strategy

**What goes wrong:**
The design system requires two non-system fonts: Outfit (display/headlines) and JetBrains Mono (technical readouts). The current codebase uses IBM Plex Sans as the system fallback loaded via OS font stack -- no web fonts are loaded at all. Introducing two web fonts means the browser initially renders with system fonts, then reflows when the web fonts arrive. Since Outfit and JetBrains Mono have significantly different metrics than system fallback fonts, every DeviceCard metric readout, every headline, and every nav element shifts position. On the canvas with 100+ nodes, this can cause a massive layout storm as every node's dimensions change.

**Why it happens:**
Developers add `@font-face` declarations and assume `font-display: swap` is sufficient. It prevents invisible text (FOIT) but explicitly allows visible layout shift (FOUT). With two fonts at multiple weights, the reflow can happen twice.

**How to avoid:**
1. Self-host both fonts as WOFF2 (eliminate third-party DNS lookup and connection time). Place them in `/public/fonts/`.
2. Preload the most critical weights in `<head>`: `<link rel="preload" href="/fonts/outfit-600.woff2" as="font" type="font/woff2" crossorigin>` and the same for JetBrains Mono Regular.
3. Use `font-display: swap` on the `@font-face` declarations.
4. Define `size-adjust`, `ascent-override`, `descent-override`, and `line-height-override` on the fallback font in `@font-face` to match the web font metrics as closely as possible, minimizing the layout shift when the swap happens.
5. Keep font file count to the minimum needed: Outfit 400, 500, 600 and JetBrains Mono 400, 500 (5 files total, approximately 50-70KB each in WOFF2).

**Warning signs:**
- CLS (Cumulative Layout Shift) score above 0.1 in Lighthouse
- Visible "jump" of text elements on initial page load
- Canvas nodes resizing on first load as fonts arrive

**Phase to address:**
Phase 1 (Design Token Foundation) -- font loading infrastructure must be set up before any component uses the new fonts.

---

### Pitfall 5: Big-Bang Component Rewrite Breaks Working Features

**What goes wrong:**
The team rewrites all 32 component files simultaneously to match the new design system. Mid-way through, existing features break -- WebSocket snapshot merging in Canvas.tsx stops working because a refactor changed the node data structure, the context menu positioning is off because the new NavBar has different height, the custom memo comparator in DeviceCard no longer checks the right fields. The app is broken for days while both the redesign and the regressions are debugged.

**Why it happens:**
Redesign feels like "just CSS changes" so developers try to do it all at once. But this codebase has complex runtime behavior (WebSocket state merging, ReactFlow node/edge lifecycle, position persistence, auto-layout) tightly coupled to the component tree. The CONCERNS.md already notes "No E2E frontend tests" -- there is no safety net to catch regressions.

**How to avoid:**
1. Adopt a strangler-fig approach: restyle one component at a time, keep both old and new styles functional via the token system.
2. Order component migration by dependency: tokens first, then leaf components (StatusDot, DeviceIcon), then compound components (DeviceCard, LinkEdge), then container components (Canvas, Dashboard), then navigation (NavBar, Area Hub).
3. Never change component behavior and visual style in the same commit. If a component needs both a logic change (e.g., area-aware filtering) and a visual change (Neon Topography styling), do them in separate PRs.
4. Before starting the redesign phases, add at minimum snapshot tests or visual regression tests for the 5 critical components: DeviceCard, Canvas, LinkEdge, ContextMenu, NavBar.

**Warning signs:**
- More than 5 files changed in a single PR for "visual updates"
- WebSocket-driven data stops rendering after a component restyle
- Position persistence (usePositions hook) breaks silently

**Phase to address:**
All phases -- but Phase 1 must establish the migration order and testing baseline.

---

### Pitfall 6: Tailwind Color Token Naming Collision with Neon Topography Palette

**What goes wrong:**
The existing Tailwind config defines semantic color tokens like `accent: '#00d4ff'`, `bg-canvas: '#2d2d3d'`, `text-primary: '#e1e8ed'`. The Neon Topography spec defines a completely different palette: primary glow is `#00E676` (green, not cyan), background is `#161618` (deeper charcoal, not `#2d2d3d`), and text-primary is `#F5F5F7`. If both old and new tokens coexist during migration, components using `text-primary` will render differently depending on whether they have been migrated. If old tokens are overwritten immediately, every un-migrated component breaks visually.

**Why it happens:**
The existing token names (`accent`, `text-primary`, `bg-canvas`) are reasonable names that the new design system also wants to use, but with different values. There is no clean namespace separation.

**How to avoid:**
1. Introduce the new palette under a `neon-` prefix initially: `neon-primary`, `neon-surface`, `neon-on-surface`. This lets old and new tokens coexist.
2. Alternatively, switch immediately to CSS custom property references: define `--color-primary`, `--color-surface`, etc. in CSS, then update the Tailwind config to reference `var(--color-primary)`. Both old and new components use the same utility classes, but the values come from CSS variables that can be swapped.
3. Option 2 is strongly preferred because it also enables theme switching (dark/light values swap at the CSS variable level).
4. Once all components are migrated, remove the old static hex values from `tailwind.config`.

**Warning signs:**
- Components look "half old, half new" during migration
- `accent` class produces different colors in different components
- Confusing PR diffs where token values change globally

**Phase to address:**
Phase 1 (Design Token Foundation) -- resolve the naming strategy before any component migration begins.

---

### Pitfall 7: Area Hub View Unmounts Canvas WebSocket State

**What goes wrong:**
The new navigation adds an Area Hub view alongside the existing Canvas and Dashboard views. The current App.tsx keeps both Canvas and Dashboard mounted and toggles visibility with CSS (`'hidden'` class). If the Area Hub is implemented the same way (always mounted), it means three heavy views are mounted simultaneously -- wasteful. If it is implemented with conditional rendering (unmount when not visible), navigating away from Canvas tears down the WebSocket connection, loses the in-memory device metrics snapshot, and forces a full re-fetch and WebSocket reconnect when returning. Users will see a blank canvas for 1-2 seconds every time they navigate back.

**Why it happens:**
The existing "keep both mounted" pattern in App.tsx was a deliberate choice to preserve Canvas state. Adding a third view makes developers question whether to continue this pattern, and neither option is obvious.

**How to avoid:**
1. Lift WebSocket connection and snapshot state out of the Canvas component into a top-level provider (or Zustand store) that persists regardless of which view is active. The WebSocket connection stays alive, and metrics keep streaming, even when the user is on the Area Hub.
2. Continue the CSS visibility toggle pattern for Canvas (it holds ReactFlow state that is expensive to reconstruct), but the Area Hub and Dashboard can be lazily rendered since they are lighter to mount.
3. This is also the right time to move from the current in-component `useWebSocket` + `useState` pattern to a shared store, which ReactFlow's own docs recommend for performance.

**Warning signs:**
- WebSocket reconnect banner flashing when switching views
- Canvas showing stale or empty data after navigating back from Area Hub
- Metrics latency spike (2-3 second gap) visible when switching back to Canvas

**Phase to address:**
Phase 2 (Theme Infrastructure / State Architecture) -- must be solved before the Area Hub view is built.

---

### Pitfall 8: Glassmorphism Looks Broken on Light Theme

**What goes wrong:**
The design system specifies glassmorphism using `rgba(255, 255, 255, 0.02)` for "Area Backgrounds" with subtle glow bleed-through. This is calibrated for the `#161618` dark background. On a light theme background (e.g., `#F5F5F7`), `rgba(255, 255, 255, 0.02)` is completely invisible -- white-on-near-white transparency produces zero visual contrast. The bloom effects (radial blurs of accent colors) are designed to "pop" against dark backgrounds; on light backgrounds they wash out or create muddy, desaturated blobs.

**Why it happens:**
The Neon Topography spec was created for dark mode. Light mode was added as a requirement ("ship both themes in v1.3.0") but the spec does not include light-mode-specific glassmorphism or bloom parameters.

**How to avoid:**
1. Define separate glassmorphism parameters per theme. Dark: `rgba(255, 255, 255, 0.02)` background, light glow blurs. Light: `rgba(0, 0, 0, 0.03)` background, darker/saturated bloom colors, possibly stronger border definition.
2. The "no-line rule" may need a light-mode exception -- glassmorphic panels on light backgrounds often require a subtle border (`1px solid rgba(0,0,0,0.06)`) for visual separation that the dark mode achieves through luminosity alone.
3. Create a dedicated light-mode design review checkpoint before shipping. Do not assume dark-mode specs translate.
4. Consider making light mode a "clean" variant (no glassmorphism, solid surfaces with shadows) rather than forcing translucent effects that look wrong.

**Warning signs:**
- Glassmorphic panels become invisible on light backgrounds
- Bloom effects look like dirty smudges on light surfaces
- Users report "nothing changed" when switching to light mode because transparent overlays are invisible

**Phase to address:**
Phase 2 (Theme Infrastructure) -- light mode design specifications must be finalized before component styling begins in Phase 3.

---

## Technical Debt Patterns

Shortcuts that seem reasonable but create long-term problems.

| Shortcut | Immediate Benefit | Long-term Cost | When Acceptable |
|----------|-------------------|----------------|-----------------|
| Using `dark:` prefix on every utility instead of CSS variables | Faster to write, no config changes | Every color has two classes; adding a third theme (e.g., OLED dark) requires touching every component; bundle size bloat from doubled classes | Never for this project -- CSS variables are required for the dual-theme design system |
| Keeping `font-mono` as system monospace instead of loading JetBrains Mono | Zero font loading overhead | Inconsistent typography across OS; metrics readouts look different on macOS vs Windows vs Linux | Only acceptable in initial scaffolding phase; must be replaced before visual polish |
| Inlining glow colors in Tailwind arbitrary values (`shadow-[0_0_14px_rgba(0,200,83,0.55)]`) | Quick visual feedback | Cannot be themed (hardcoded RGBA); each glow requires a unique arbitrary value string; no design token control | Never -- define glow shadows as CSS custom properties from the start |
| Skipping visual regression tests | Ship visual changes faster | The "no E2E tests" concern from CONCERNS.md compounds; redesign regressions caught only by manual QA | Only for the first 1-2 leaf component migrations; must be in place by Phase 3 |

## Integration Gotchas

Common mistakes when connecting new design system features with existing infrastructure.

| Integration | Common Mistake | Correct Approach |
|-------------|----------------|------------------|
| ReactFlow + CSS Custom Properties | Defining theme colors in Tailwind only; ReactFlow's internal styles (minimap, controls, background grid) don't pick them up | Define colors as CSS custom properties first; use them in both Tailwind config AND override ReactFlow's CSS variables (e.g., `--xy-node-border-radius`, `--xy-background-color`) |
| WebSocket Metrics + Area Filtering | Filtering devices client-side in the Area Hub but the WebSocket still broadcasts metrics for ALL devices, wasting bandwidth | Keep the WebSocket broadcasting all metrics (it is already efficient for 100 devices); filter in the view layer only, not the data layer |
| Theme Toggle + LocalStorage + OS Preference | Reading only localStorage, ignoring `prefers-color-scheme`; or reading only OS preference, ignoring user override | Check localStorage first (user explicit choice overrides system), fall back to `prefers-color-scheme` media query, default to dark |
| Outfit Font + ReactFlow Node Labels | Loading Outfit for node labels causes layout instability because ReactFlow caches node dimensions on mount; when the font loads and text reflows, the cached dimensions are stale | Either preload Outfit before ReactFlow mounts, or use a monospace fallback for node labels so dimension changes are minimal |

## Performance Traps

Patterns that work at small scale but fail as usage grows.

| Trap | Symptoms | Prevention | When It Breaks |
|------|----------|------------|----------------|
| `backdrop-filter: blur()` on every DeviceCard node | Canvas pan/zoom drops below 20 FPS | Reserve `backdrop-filter` for max 5-10 overlay/panel elements; use solid `bg-opacity` on canvas nodes | 30+ nodes visible in viewport simultaneously |
| Animating `box-shadow` directly on status change | Stutter during metric updates when many devices change status simultaneously | Animate opacity of a pseudo-element that has the shadow pre-rendered; or use CSS `transition` only on `box-shadow` color (not blur radius) | 50+ simultaneous status transitions (e.g., after network event) |
| Re-rendering all canvas nodes on theme switch | 1-2 second freeze while every node re-renders with new colors | Use CSS custom properties for all colors so theme switch is a single class change on `<html>`, not a React state update that triggers reconciliation | 100+ nodes |
| Loading 10+ font files (multiple weights of two families) | 300KB+ of font data blocking first meaningful paint | Limit to 5 WOFF2 files; preload the 2 most critical; use `font-display: swap` | Always, but especially on 3G/slow connections |
| Neon bloom background effects using CSS `radial-gradient` with large spread | Composite layer promotion for each gradient; GPU memory pressure | Use a single full-viewport canvas or SVG for atmospheric effects rather than per-element CSS gradients | 10+ elements with bloom effects visible simultaneously |

## UX Pitfalls

Common user experience mistakes in this domain.

| Pitfall | User Impact | Better Approach |
|---------|-------------|-----------------|
| Theme toggle buried in Settings | Users who want light mode cannot find it; they assume the app is dark-only | Place theme toggle in the nav pill or as a quick-access icon in the navigation bar; Settings can have the full preference (auto/light/dark) |
| Area Hub replaces the Canvas as default view | Power users who live on the Canvas view have an extra click on every session | Remember last active view in localStorage; default to Canvas for new users (it is the core product) |
| Glow effects on every status dot simultaneously | When many devices are "up", the canvas becomes a green glow soup with no visual hierarchy | Reserve glow for status changes (transitions in/out) and critical states (down, degraded); "up" gets a simple solid dot without glow |
| Atmospheric watermark text too prominent | Large "BACKBONE" or "AREA 1" text competes with actual data for visual attention | Use very low opacity (2-5%), very large size (60px+), and ensure it is behind all interactive elements; test with actual topology data overlaid |
| No transition animation between views | Switching between Canvas, Area Hub, and Dashboard feels like a jarring page jump | Add a subtle cross-fade or slide transition (150-200ms); keep it short to avoid feeling sluggish |

## "Looks Done But Isn't" Checklist

Things that appear complete but are missing critical pieces.

- [ ] **Theme switching:** Often missing OS-preference sync -- verify that `prefers-color-scheme` media query listener updates theme when user changes OS setting while app is open
- [ ] **Font loading:** Often missing fallback metrics -- verify that the system font fallback has `size-adjust` to prevent layout shift, not just `font-display: swap`
- [ ] **Color tokens:** Often missing opacity variants -- verify that theme tokens work with Tailwind opacity modifiers (e.g., `bg-surface/80` must resolve correctly from CSS variable)
- [ ] **Canvas restyle:** Often missing ReactFlow internal elements -- verify that the minimap, background dots/grid, selection box, and connection line all match the new theme
- [ ] **Glassmorphism:** Often missing cross-browser testing -- verify that `backdrop-filter` renders correctly in Firefox (historically delayed support) and Safari
- [ ] **Area Hub navigation:** Often missing deep-link / URL state -- verify that refreshing the page on Area Hub returns to Area Hub, not default Canvas view
- [ ] **Glow effects:** Often missing reduced-motion support -- verify that `prefers-reduced-motion` disables glow animations and pulse effects
- [ ] **Light mode:** Often missing form elements -- verify that inputs, selects, and checkboxes in Settings/AddDevice/SNMP panels are styled for light mode

## Recovery Strategies

When pitfalls occur despite prevention, how to recover.

| Pitfall | Recovery Cost | Recovery Steps |
|---------|---------------|----------------|
| Hardcoded hex colors blocking theme switch | MEDIUM | Run regex find-and-replace across all files; takes 2-4 hours for 29 files but is mechanical work |
| Canvas performance death from heavy CSS | LOW | Add a `canvas-node` Tailwind variant that strips heavy effects; can be done component-by-component without redesign |
| Font loading layout shift | LOW | Add `<link rel="preload">` tags and `size-adjust` to `@font-face`; can be fixed in a single commit |
| Big-bang rewrite broke Canvas state | HIGH | Must revert and re-approach incrementally; debugging interleaved visual + logic changes in Canvas.tsx (500+ lines) is extremely time-consuming |
| Flash of wrong theme | LOW | Add a 5-line inline script to `index.html`; immediate fix |
| Area Hub unmounting WebSocket | MEDIUM | Requires extracting WebSocket state to a provider/store; touching Canvas.tsx's core data flow carries regression risk |
| Glassmorphism invisible on light mode | MEDIUM | Requires design iteration for light-mode-specific parameters; not just a code fix but a design decision |
| Token naming collision during migration | LOW-MEDIUM | Rename with find-and-replace if caught early; becomes HIGH if components shipped with conflicting tokens |

## Pitfall-to-Phase Mapping

How roadmap phases should address these pitfalls.

| Pitfall | Prevention Phase | Verification |
|---------|------------------|--------------|
| Hardcoded hex colors (Pitfall 2) | Phase 1: Token Foundation | `grep -r "bg-\[#\|text-\[#\|border-\[#\|shadow-\[.*#" src/` returns zero component matches |
| Token naming collision (Pitfall 6) | Phase 1: Token Foundation | All color utilities resolve to CSS custom properties; no static hex in Tailwind config |
| Font loading CLS (Pitfall 4) | Phase 1: Token Foundation | Lighthouse CLS < 0.1; no visible text reflow on fresh load |
| Flash of wrong theme (Pitfall 3) | Phase 2: Theme Infra | Switch to light, hard refresh -- no dark flash visible |
| WebSocket state on navigation (Pitfall 7) | Phase 2: Theme Infra / State | Navigate Canvas -> Area Hub -> Canvas: metrics display immediately without reconnect |
| Light-mode glassmorphism (Pitfall 8) | Phase 2: Theme Infra | Visual QA of all glassmorphic elements on light background |
| Canvas glow performance (Pitfall 1) | Phase 3: Canvas Restyle | 100-node canvas maintains 60 FPS during pan/zoom (devtools Performance audit) |
| Big-bang rewrite (Pitfall 5) | All phases | No PR changes more than 3 component files; behavior and style changes are never in the same commit |

## Sources

- [React Flow Performance Documentation](https://reactflow.dev/learn/advanced-use/performance) -- official guidance on CSS complexity and node count
- [Glassmorphism Implementation Guide 2025](https://playground.halfaccessible.com/blog/glassmorphism-design-trend-implementation-guide) -- performance limits of backdrop-filter
- [Tailwind CSS Dark Mode Documentation](https://tailwindcss.com/docs/dark-mode) -- class strategy and CSS variable approach
- [Flash of Unstyled Dark Theme (FOUDT)](https://webcloud.se/blog/2020-04-06-flash-of-unstyled-dark-theme/) -- the inline-script prevention pattern
- [CSS Box Shadow Animation Performance](https://www.sitepoint.com/css-box-shadow-animation-performance/) -- pseudo-element opacity trick
- [Animating Box Shadow with Smooth Performance](https://tobiasahlin.com/blog/how-to-animate-box-shadow/) -- GPU-friendly shadow animation patterns
- [Dark Mode in React: A Scalable Theme System with Tailwind](https://medium.com/@roman_fedyskyi/dark-mode-in-react-a-scalable-theme-system-with-tailwind-d14e9c1afd1a) -- CSS variable + Tailwind integration
- [Tailwind CSS v4 Design Tokens](https://dev.to/wearethreebears/exploring-typesafe-design-tokens-in-tailwind-4-372d) -- @theme directive and CSS custom properties
- [How We Built Light Mode Without Tailwind's dark: Class](https://www.basedash.com/blog/how-we-built-light-mode-without-tailwind-s-dark-class) -- CSS variable approach vs class duplication
- [Font Loading Optimization Guide](https://onenine.com/ultimate-guide-to-font-loading-optimization/) -- WOFF2, preload, size-adjust strategies
- Theia codebase: `frontend/src/components/DeviceCard.tsx`, `frontend/tailwind.config.js`, `frontend/index.html`, `frontend/src/index.css`
- Theia `.planning/codebase/CONCERNS.md` -- existing known issues informing redesign risk

---
*Pitfalls research for: Theia v1.3.0 Frontend Redesign (Neon Topography)*
*Researched: 2026-03-25*
