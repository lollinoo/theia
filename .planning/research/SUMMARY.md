# Project Research Summary

**Project:** MikroTik Theia — v1.3.0 Frontend Redesign (Neon Topography)
**Domain:** Network Topology Visualizer — Design System, Theming, and Area-Based Navigation
**Researched:** 2026-03-25
**Confidence:** HIGH

## Executive Summary

Theia v1.3.0 is a frontend design system overhaul layered on top of a working React + ReactFlow network topology visualizer. The research consensus is clear: this is fundamentally a CSS architecture problem, not a component rewrite. The entire redesign pivots on establishing a CSS custom property token system first — every other deliverable (dark/light theming, Neon Topography aesthetics, OSPF area views) is downstream of that foundation. The stack changes are narrow and well-understood: Tailwind CSS v3 to v4 (via automated codemod), `reactflow` to `@xyflow/react` v12, and adding two self-hosted variable fonts (Outfit + JetBrains Mono). No new state management libraries, routing libraries, or UI component libraries are needed.

The recommended architecture wraps the existing canvas and data pipeline in two new providers — `ThemeProvider` for CSS class toggling and `AreaProvider` for area CRUD state and device-to-area filtering. The WebSocket metrics stream and ReactFlow canvas remain structurally unchanged; the redesign adds a CSS variable layer that all components inherit automatically, and an area filter layer that sits above the data. The new Area Hub view and floating NavigationPill are net-new components that coexist with the existing topology canvas rather than replacing it.

The primary risk is scope creep disguised as visual work: 29 component files contain hardcoded hex values that will silently break theme switching if not extracted before styling begins, and a big-bang component rewrite (as opposed to incremental migration) has a documented failure mode given the absence of E2E tests. The secondary risk is canvas performance — the Neon Topography glow and glassmorphism effects are calibrated for static page layouts, not 100+ ReactFlow nodes. Both risks are preventable through disciplined phase ordering: token foundation first, then incremental component migration, then new views.

---

## Key Findings

### Recommended Stack

The stack delta for v1.3.0 is minimal by design. Tailwind CSS v4's `@theme` directive is the keystone: it generates CSS custom properties and utility classes from a single declaration, making the Neon Topography token system work simultaneously as Tailwind classes (`bg-surface`) and raw CSS variables (`var(--color-surface)`). This dual access is required for React Flow node styling and eliminates the need for a parallel token system outside of Tailwind. The PostCSS pipeline is replaced by the `@tailwindcss/vite` plugin, and `tailwind.config.js` is deleted in favor of CSS-native `@theme` declarations.

React Flow v12 (`@xyflow/react`) adds a `colorMode` prop and `--xy-*` CSS variable namespace that aligns naturally with the design token system. The codebase is small enough (~30 components) to absorb the mechanical import renames. The main migration risk is the `node.measured` API change affecting `useAutoLayout.ts`. Both fonts are self-hosted via Fontsource variable font packages — critical for air-gapped deployments where Google Fonts CDN is unavailable.

**Core technologies (delta from current):**
- `tailwindcss@4.2.2` + `@tailwindcss/vite@4.2.2` — replace PostCSS pipeline; `@theme` directive unifies design tokens and utility classes, the backbone of the entire token system
- `@xyflow/react@12.10.1` — package rename from `reactflow`; adds `colorMode` prop and `--xy-*` CSS variable theming that maps to Neon Topography tokens
- `@fontsource-variable/outfit@5.2.8` + `@fontsource-variable/jetbrains-mono@5.2.8` — self-hosted variable fonts; single file per family covers all weights; no CDN dependency
- CSS Custom Properties (native) — entire color system expressed as `:root`/`[data-theme]` overrides; zero JS required for theme switching
- React Context (built-in) — `ThemeProvider` (~40 lines) and `AreaProvider`; no external theme or state libraries needed

**What does not change:** React 18.3, TypeScript 5.7, Vite 7.0, d3-force 3.0, Go 1.24 backend, SQLite, Vitest. The React 19 upgrade is explicitly deferred as an independent effort.

### Expected Features

The v1.3.0 redesign has three categories of deliverables. The design system layer (CSS tokens, fonts, glassmorphism, glow effects) is a prerequisite for everything else. The component restyling layer applies the new design to all 25+ existing components. The area navigation layer (Area Hub, NavigationPill, area CRUD, device assignment) is the primary new product capability.

**Must have (table stakes for v1.3.0):**
- CSS variable token system with full dark and light Neon Topography palettes — nothing themes without this
- All 25+ existing components restyled (DeviceCard, ContextMenu, NavBar, SidePanel, Toolbar, Dashboard, all settings panels) — partial restyling is worse than no restyling
- Dark/light theme toggle with localStorage persistence and FOWT prevention — core user-facing feature
- Redesigned DeviceCard with glow status indicators (Outfit labels, JetBrains Mono values, `box-shadow` glow on status dot)
- Redesigned ContextMenu with glassmorphism, Material Symbols icons, separators, and danger-state styling
- Floating NavigationPill replacing the current NavBar — the signature navigation element of the new design
- OSPF Area Hub view with aggregate stats bar and per-area cards with bloom effects
- Area CRUD in Settings (create/edit/delete areas, assign devices) — areas are meaningless without this
- Atmospheric watermark component (contextual large-opacity background text)

**Should have (differentiators):**
- Bloom/radial-blur effects behind area cards and status-critical elements — no open-source NMS competitor has this
- "No-line rule" applied throughout — depth through luminosity, not borders
- Area-filtered topology views on the canvas (filter nodes/edges by selected area)
- Smooth theme transitions (200ms CSS transition on color properties)

**Defer to v1.4:**
- Canvas-integrated area zone backgrounds (colored region groupings) — complex react-flow grouping, orthogonal to visual redesign
- Animated link throughput (SVG pulse animation) — performance risk, no mock exists
- Monospace delta tags for metric changes — requires backend delta computation not yet built

### Architecture Approach

The architecture introduces two new provider layers (`ThemeProvider`, `AreaProvider`) and three new view-level components (`NavigationPill`, `AreaHubView`, `AreaCard`) while leaving the existing ReactFlow canvas, WebSocket pipeline, and REST client structurally unchanged. Theme switching is CSS-only — `ThemeProvider` sets a `data-theme` attribute on `<html>` which triggers CSS variable cascades; no React reconciliation happens on theme change, which is critical for performance at 100+ canvas nodes. Area data flows from `AreaProvider` (fetched once from `/api/v1/areas`) as a `filteredDeviceIds: Set<string>` that consumers use to filter their own data — the WebSocket snapshot is never duplicated per area.

**Major components:**
1. `ThemeProvider` — reads localStorage, sets `data-theme` on `<html>`, provides `{ theme, setTheme, resolvedTheme }` context; CSS cascade handles all visual changes with zero re-renders
2. `AreaProvider` — fetches area list, maintains `deviceAreaMap: Map<string, string>`, exposes `filteredDeviceIds: Set<string> | null` (null = Global); holds area CRUD methods
3. `NavigationPill` — replaces NavBar entirely; floating pill with view tabs (Areas/Topology/Devices) and area tabs (Global/Area 0/Area 1...); drives both view state and area selection
4. `AreaHubView` — grid of `AreaCard` components plus `AggregateStatsBar`; reads from AreaProvider and WebSocket snapshot for aggregate health computation
5. `TopologyView` (refactored Canvas.tsx) — receives `filteredDeviceIds` from AreaProvider; Canvas.tsx decomposition into `useTopologyData`, `useSnapshotApplication`, `useCanvasInteractions` hooks strongly recommended before adding area filtering
6. Backend area domain — new `areas` table, `device_area_assignments` (nullable FK), CRUD REST API at `/api/v1/areas`; can be built in parallel with frontend token work

**Key technical decisions:**
- View routing stays as `activeView` state (not React Router); extended from 2 to 3 views (`areas | canvas | dashboard`)
- No Zustand for this milestone; React Context is sufficient for low-frequency area and theme state
- WebSocket connection must be lifted to a top-level provider before the Area Hub is built
- Self-host fonts via Fontsource npm packages — not Google Fonts CDN
- NavigationPill replaces NavBar entirely; the two must not coexist

### Critical Pitfalls

1. **Hardcoded hex colors in 29 files block theme switching** — Audit and replace every `bg-[#...]`, `text-[#...]`, `border-[#...]`, `shadow-[.*#...]` arbitrary Tailwind value before touching any component. This is Phase 1 work; everything else depends on it. DeviceCard.tsx alone has 4 occurrences.

2. **Canvas glow/glassmorphism performance death at 100+ nodes** — The design system was spec'd for static page layouts (5-10 elements). Apply full Neon Topography effects (heavy shadows, bloom, `backdrop-filter`) only in Area Hub, panels, and NavigationPill. Canvas nodes get a simplified two-tier treatment: small-radius single-layer `box-shadow`, no `backdrop-filter`. Benchmark with 100+ nodes at 60 FPS before merging any canvas restyle.

3. **Flash of wrong theme (FOWT) on page load** — Add a 5-line inline `<script>` in `index.html` `<head>` that reads localStorage and sets `data-theme` before any CSS renders. Without this, users in light mode see a dark flash on every page refresh.

4. **WebSocket state unmounts when Area Hub is added** — Lift the WebSocket connection and snapshot state to a top-level provider before building the Area Hub. The current Canvas-level WebSocket will disconnect when the user navigates away, causing a 1-2 second reconnect delay on return.

5. **Big-bang component rewrite breaks working features** — No E2E tests exist (confirmed in CONCERNS.md). Restyle one component at a time, leaf-to-container order. Never change component behavior and visual styling in the same commit.

6. **Glassmorphism invisible on light theme** — The `rgba(255,255,255,0.02)` spec is calibrated for dark backgrounds. On `#F5F5F7` it renders as invisible. Define separate glassmorphism parameters per theme before styling any glassmorphic component.

---

## Implications for Roadmap

Based on the dependency chain identified across all four research files, the work organizes into five phases with a clear critical path: Phase 1 -> Phase 2 + Phase 3 (parallel) -> Phase 4. Phase 5 is optional for v1.3.0 but strongly recommended.

### Phase 1: Design Token Foundation

**Rationale:** Every other piece of work depends on the CSS token system existing. Components cannot be restyled until tokens are defined. Theme switching cannot be tested until tokens are switchable. This phase also resolves the two hardest pitfalls (hardcoded hex audit, token naming collision) — failing to do this first causes every downstream phase to inherit the problem.

**Delivers:** Complete CSS custom property system for dark and light Neon Topography palettes; Tailwind v4 `@theme` configured; `@tailwindcss/vite` replacing PostCSS; Outfit + JetBrains Mono self-hosted and preloaded; `ThemeProvider` context with localStorage persistence; FOWT prevention script in `index.html`; React Flow v12 (`@xyflow/react`) migration with import renames complete; all 29 files audited for hardcoded hex values replaced with semantic tokens.

**Addresses:** CSS variable token system (P0), font integration (P0), FOWT prevention (P0), system preference detection (P1)

**Avoids:** Pitfall 2 (hardcoded hex), Pitfall 3 (FOWT), Pitfall 4 (font CLS), Pitfall 6 (token naming collision)

**Research flag:** Standard patterns — Tailwind v4 upgrade guide is authoritative, codemod handles ~90%. React Flow v12 migration guide covers all breaking changes. Run `npx @tailwindcss/upgrade`, review diff, fix `useAutoLayout.ts` for `node.measured` API change.

---

### Phase 2: Theme Infrastructure and State Architecture

**Rationale:** Before any component is restyled, the theme switching mechanism must be verified end-to-end (dark/light swap, persistence, OS preference sync), the WebSocket state lift must happen, and the light-mode glassmorphism design must be resolved. Doing the WebSocket lift here — before the Area Hub is built in Phase 4 — avoids a high-recovery-cost pitfall later.

**Delivers:** Verified dark/light theme toggle with OS preference detection; light-mode glassmorphism parameters defined (separate from dark-mode spec); WebSocket connection and snapshot state lifted to a top-level provider; Canvas visibility-toggle pattern confirmed for 3-view navigation; visual regression baseline tests for 5 critical components (DeviceCard, Canvas, LinkEdge, ContextMenu, NavBar).

**Addresses:** Theme toggle UI (P1), system preference detection (P1), WebSocket state architecture (prerequisite for Area Hub)

**Avoids:** Pitfall 3 (FOWT), Pitfall 7 (WebSocket unmount on navigation), Pitfall 8 (glassmorphism invisible on light)

**Research flag:** Light-mode glassmorphism parameters need a design decision, not a research spike. The question is whether light mode uses translucency (different parameters) or clean solid surfaces. This must be decided before Phase 3 component styling begins.

---

### Phase 3: Component Restyling

**Rationale:** With tokens in place and theme switching verified, component restyling is systematic incremental work. Dependency order is leaf-to-container: status indicators and icons first, then atomic components, then compound components, then containers, then navigation. Each component is restyled independently with no behavior changes in the same commit.

**Delivers:** All 25+ existing components restyled to Neon Topography aesthetics; DeviceNode with glow status indicators using `box-shadow` opacity pseudo-element trick (GPU-friendly); ContextMenu with Material Symbols icons, glassmorphism, separators, and danger styling; NavigationPill replacing NavBar; atmospheric watermark CSS class; two-tier canvas node visual strategy (simplified for canvas, full for panels).

**Addresses:** All P1 component restyling features; redesigned DeviceCard, ContextMenu, NavBar

**Avoids:** Pitfall 1 (canvas glow performance — simplified shadows on canvas nodes), Pitfall 5 (big-bang rewrite — one component per PR), Pitfall 8 (light mode glassmorphism resolved in Phase 2)

**Research flag:** Standard patterns. Component targets are fully specified in the mock files at `.planning/examples_mocks/`. Material Symbols Outlined integration is a straightforward font import. No research spike needed.

---

### Phase 4: Area Backend and Provider

**Rationale:** Can run in parallel with Phase 3 since it is backend and context work with no visual dependencies. The backend must exist before the Area Hub UI (Phase 5) can be built. This phase closes the loop between the NavigationPill (wired to real area data) and the Settings panel (area management UI).

**Delivers:** Go backend: `areas` table, device-area assignment (nullable FK or join table), CRUD REST API (`GET/POST/PUT/DELETE /api/v1/areas`); frontend: `AreaProvider` context with `deviceAreaMap` and `filteredDeviceIds` Set; area management UI in SettingsPanel (create/edit/delete areas, assign devices via DeviceConfigPanel dropdown); NavigationPill wired to real area data; App.tsx view routing extended to three views (`areas | canvas | dashboard`).

**Addresses:** Area data model (P1), area CRUD in Settings (P1), device-to-area assignment (P1)

**Avoids:** Anti-feature: drag-to-assign on canvas (areas assigned in Settings, not by dragging)

**Research flag:** Backend area CRUD follows the same pattern as existing SNMP and SSH profile management in the Go codebase. Established internal pattern — no research spike needed.

---

### Phase 5: Area Hub View and Filtered Topology

**Rationale:** Depends on Phase 3 (styled components) and Phase 4 (area data). This is the primary new product surface. The atmospheric watermark and area-specific bloom effects complete the Neon Topography visual language. Canvas.tsx decomposition (extracting focused hooks) should happen here if not done during Phase 3.

**Delivers:** `AreaHubView` with `AggregateStatsBar` and grid of `AreaCard` components; area-specific bloom effects (radial blur pseudo-elements, max 3-5 per viewport); atmospheric watermark updating contextually by selected area; area-filtered `TopologyView` (canvas shows only devices in selected area when area is active); area-filtered `DashboardView`; Canvas.tsx decomposition into `useTopologyData`, `useSnapshotApplication`, `useCanvasInteractions` hooks.

**Addresses:** OSPF Area Hub view (P1), atmospheric watermark (P1), area-filtered topology views (P2 promoted — infrastructure exists from Phase 4)

**Avoids:** Pitfall 1 (bloom limited to bounded element count in Area Hub), Pitfall 7 (WebSocket state already lifted in Phase 2)

**Research flag:** Area Hub aggregate stat computation (deriving network uptime, health from WebSocket snapshot filtered by area) needs a brief design spike: decide whether aggregation happens in `AreaProvider`, a custom hook, or the component. Low complexity, pure frontend logic over existing data.

---

### Phase Ordering Rationale

- Phases 1 and 2 are strict serial prerequisites: no component can be reliably styled until tokens exist (Phase 1), and no navigation flow is safe until WebSocket state is lifted and light-mode design is resolved (Phase 2).
- Phases 3 and 4 are parallel tracks: frontend component restyling and backend area domain + provider wiring have no dependencies on each other and can be worked simultaneously.
- Phase 5 requires both Phase 3 (styled AreaCard, AggregateStatsBar) and Phase 4 (real area data from AreaProvider) to be complete before meaningful implementation or testing.
- Canvas.tsx decomposition is recommended before Phase 5 adds area filtering to a 750-line mega-component with 15+ useState calls.

### Research Flags

**Phases needing design decisions before implementation:**
- **Phase 2:** Light-mode glassmorphism parameters — not a research spike but a design review checkpoint. Produce explicit CSS values for `--color-glassmorphism` and bloom `opacity` per theme before writing component styles.
- **Phase 5:** Area Hub aggregate stat computation — brief design spike to decide where aggregation logic lives. Low complexity but must be decided before building.

**Phases with standard, well-documented patterns (skip research-phase):**
- **Phase 1:** Tailwind v4 upgrade has an official codemod and authoritative migration guide. React Flow v12 migration guide covers all breaking changes. Fontsource installation is straightforward.
- **Phase 3:** All component targets are fully specified in mock files at `.planning/examples_mocks/`. No API design decisions needed.
- **Phase 4:** Backend area CRUD follows the existing SNMP/SSH profile pattern in the Go codebase.

---

## Confidence Assessment

| Area | Confidence | Notes |
|------|------------|-------|
| Stack | HIGH | All package versions verified on npm. Tailwind v4 and React Flow v12 have official migration guides. One tension resolved: ARCHITECTURE.md recommended staying on RF v11, STACK.md recommends upgrading to v12 (accounts for Tailwind v4 integration). STACK.md recommendation is correct. |
| Features | HIGH | Feature list is grounded in detailed mocks at `.planning/examples_mocks/` and DESIGN.md spec. Table stakes vs. differentiators vs. deferred features are clearly separated with rationale. The "defer to v1.4" list is specific and justified. |
| Architecture | HIGH | Architecture patterns are validated against the existing codebase (DeviceCard.tsx, Canvas.tsx, tailwind.config.js analyzed directly). Component boundaries are concrete with TypeScript interface sketches. Data flow is explicit for all three pipelines (theme, area, metrics). |
| Pitfalls | HIGH | Pitfalls are codebase-specific rather than generic: hardcoded hex count is 4 in DeviceCard alone, 29 files total (confirmed by direct file inspection). Performance pitfalls reference ReactFlow's own documentation. Recovery costs are estimated per pitfall. |

**Overall confidence:** HIGH

### Gaps to Address

- **Light-mode glassmorphism design:** No research file resolves what `rgba(255,255,255,0.02)` becomes on a light background. This must be a design decision at the start of Phase 2. Produce explicit CSS values before Phase 3 begins.

- **Canvas.tsx decomposition timing:** ARCHITECTURE.md recommends this as "Phase 5 optional" but acknowledges it becomes unwieldy when Phase 5 adds area filtering to a 750-line file. The roadmap should make this decision explicit: mandatory before Phase 5, or a parallel track during Phase 4.

- **React Flow v12 `node.measured` impact on `useAutoLayout.ts`:** Both STACK.md and ARCHITECTURE.md identify this as the highest-risk part of the v12 migration, but neither provides the specific code change. Resolve as the first verification step in Phase 1: read `useAutoLayout.ts`, identify all direct dimension accesses, confirm mapping to `node.measured.width`/`node.measured.height`.

- **Material Symbols icon font hosting:** The context menu redesign requires Material Symbols Outlined. The air-gapped deployment concern raised for text fonts applies equally here. Decide on self-hosted WOFF2 vs. CDN import before Phase 3 context menu work begins.

---

## Sources

### Primary (HIGH confidence)
- `/home/azmin/projects/theia/.planning/DESIGN.md` — Neon Topography design system specification; primary source for color values, typography, and visual rules
- `/home/azmin/projects/theia/.planning/examples_mocks/` — OSPF Area Hub (dark + light) and Node Context Menu (dark + light) HTML mocks; ground truth for component implementation
- [Tailwind CSS v4 upgrade guide](https://tailwindcss.com/docs/upgrade-guide) — migration steps, codemod, breaking changes
- [Tailwind CSS v4 `@theme` directive docs](https://tailwindcss.com/docs/theme) — token namespaces, CSS variable generation
- [React Flow v12 migration guide](https://reactflow.dev/learn/troubleshooting/migrate-to-v12) — breaking changes, `node.measured`, package rename
- [React Flow theming docs](https://reactflow.dev/learn/customization/theming) — CSS variable overrides, `colorMode` prop
- [React Flow performance documentation](https://reactflow.dev/learn/advanced-use/performance) — CSS complexity limits at high node counts
- [Apple HIG: Tab Bars](https://developer.apple.com/design/human-interface-guidelines/tab-bars) — max 5 items in pill navigation

### Secondary (MEDIUM confidence)
- [Tailwind CSS v4 theming patterns (Medium)](https://medium.com/@sir.raminyavari/theming-in-tailwind-css-v4-support-multiple-color-schemes-and-dark-mode-ba97aead5c14) — semantic token pattern with `@theme` + CSS variables
- [Dark Mode in React with Tailwind (Medium)](https://medium.com/@roman_fedyskyu/dark-mode-in-react-a-scalable-theme-system-with-tailwind-d14e9c1afd1a) — CSS variable + class strategy integration
- [Glassmorphism Implementation Guide](https://playground.halfaccessible.com/blog/glassmorphism-design-trend-implementation-guide) — performance limits of `backdrop-filter`
- [Flash of Unstyled Dark Theme prevention](https://webcloud.se/blog/2020-04-06-flash-of-unstyled-dark-theme/) — inline-script pattern for FOWT
- [CSS Box Shadow Animation Performance](https://tobiasahlin.com/blog/how-to-animate-box-shadow/) — pseudo-element opacity trick for GPU-friendly glow animation
- [Font Loading Optimization Guide](https://onenine.com/ultimate-guide-to-font-loading-optimization/) — WOFF2, preload, `size-adjust` for CLS prevention
- [Fontsource: Outfit variable](https://fontsource.org/fonts/outfit/install) + [JetBrains Mono variable](https://fontsource.org/fonts/jetbrains-mono/install)
- Theia codebase: `frontend/src/components/DeviceCard.tsx`, `frontend/tailwind.config.js`, `frontend/index.html`, `frontend/src/index.css`, `.planning/codebase/CONCERNS.md`

### Tertiary (LOW confidence)
- [Network Visualization Key Features 2025](https://www.selector.ai/learning-center/network-visualization-tools-key-features-and-top-6-tools/) — competitor feature sets for positioning context
- [Dark Glassmorphism UI Trend 2026 (Medium)](https://medium.com/@developer_89726/dark-glassmorphism-the-aesthetic-that-will-define-ui-in-2026-93aa4153088f) — glassmorphism best practices; trend piece, not technical reference

---
*Research completed: 2026-03-25*
*Ready for roadmap: yes*
