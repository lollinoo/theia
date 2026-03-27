# Phase 4: Area Hub View and Filtered Topology - Context

**Gathered:** 2026-03-26
**Status:** Ready for planning

<domain>
## Phase Boundary

Deliver the OSPF Area Hub view (aggregate stats + per-area cards), a floating navigation pill that replaces the current NavBar entirely, an atmospheric watermark, and area-filtered topology canvas with ghost nodes for cross-area links. Canvas.tsx decomposition is a prerequisite task within this phase. This phase consumes the area backend and management UI from Phase 3 and the restyled components from Phase 2.

</domain>

<decisions>
## Implementation Decisions

### Navigation Architecture
- **D-01:** The floating navigation pill replaces the NavBar entirely — NavBar component is removed. The pill is the sole navigation element in the app
- **D-02:** Single context-aware pill design. On Hub/Topology views: shows THEIA branding, Hub icon, area buttons (Global + each area), Devices icon, and theme toggle. On Devices view: simplified pill with Hub icon, "Devices" label, and theme toggle
- **D-03:** THEIA branding and version live inside the pill at the left edge, before the Hub icon
- **D-04:** Area buttons overflow via horizontal scroll inside the pill with a max-width constraint. Subtle fade edges hint at more areas when scrollable
- **D-05:** "Global" in the pill navigates to the Area Hub page (aggregate stats + area card grid). Clicking an area button navigates to the filtered topology canvas for that area

### View Architecture
- **D-06:** The Area Hub is a new view type in App.tsx alongside 'canvas' and 'dashboard'. Three views: Hub, Topology (canvas), Devices (dashboard)
- **D-07:** Canvas.tsx must be decomposed before adding area filtering. The file is 1294 lines — split into smaller modules as the first task of this phase
- **D-08:** Instant view swap between Hub and filtered canvas — no animation. The pill's active area highlight provides orientation
- **D-09:** Hub is the default landing when "Global" is selected in the pill

### Hub Content
- **D-10:** Hub header shows four aggregate stats: Network Uptime (longest common uptime from device metrics), Aggregate Health (% devices up), Total Device Count, Active Link Count. No route count (explicitly excluded in PROJECT.md)
- **D-11:** Aggregate Health = (devices with status 'up') / (total devices) * 100. Thresholds: >= 95% "Optimal" (green), >= 80% "Degraded" (yellow), < 80% "Critical" (red)
- **D-12:** Per-area cards show: area name, description, accent color glow dot, health status (same formula scoped to area), device count, active link count

### Area-Filtered Canvas
- **D-13:** Selecting an area in the nav pill filters the topology canvas to show only that area's devices and links
- **D-14:** Unassigned devices (no area) only appear in the Global/full topology view. Hidden when any specific area is active
- **D-15:** Cross-area links shown as stubs with ghost nodes — remote device appears as a small muted node (no stats, just hostname). Makes external connections visible without clutter
- **D-16:** Clicking a ghost node navigates to that device's area (switches the pill to the ghost device's area, showing that area's filtered topology)

### Watermark
- **D-17:** Atmospheric watermark at bottom-left, small text (~1.5rem / text-2xl), Outfit font, very low opacity (~0.10-0.15 dark, ~0.05-0.08 light), fixed position, pointer-events-none
- **D-18:** Watermark text updates contextually: "GLOBAL TOPOLOGY" on Hub, area name (uppercase) on filtered canvas
- **D-19:** Simple fade transition (150ms) when watermark text changes between areas

### Area Card Effects
- **D-20:** Area cards use radial blur bloom (per Phase 2 D-10 allowing richer effects for off-canvas elements). Large radial blur circle in area accent color at top-right, ~0.10 opacity default, ~0.20 on hover. Light mode: subtle, no blur
- **D-21:** Area card border: default standard surface border, hover transitions to area accent color (200ms). Bloom intensifies on hover

### Pill Visual Styling
- **D-22:** Pill follows established overlay surface pattern: glassmorphism in dark mode (translucent bg + backdrop-blur-16px + subtle border), solid tinted surface in light mode (no backdrop-blur). Consistent with ContextMenu/SearchOverlay
- **D-23:** Each area button in the pill has a small color dot (6-8px) in the area's accent color before the text label. Active area has a glowing dot

### Empty States
- **D-24:** Hub with no areas: show aggregate stats (global data) plus a CTA card — "No areas yet. Create your first area in Settings" with a link to Settings > Areas. Nav pill shows only "Global"
- **D-25:** Filtered canvas with devices but no links: just show devices as normal nodes, no special empty state message

### Claude's Discretion
- Canvas.tsx decomposition strategy (how to split the 1294-line file into modules)
- Exact pill layout measurements, padding, border-radius
- Hub page layout (grid columns, responsive breakpoints)
- Ghost node visual design (size, opacity, border style)
- Material Symbols icon choices for Hub icon, Devices icon in the pill
- Area card grid layout and responsive behavior
- Health calculation for "Network Uptime" aggregate stat (which device metric to use)
- Pill horizontal scroll implementation details (fade gradient vs arrow indicators)

</decisions>

<canonical_refs>
## Canonical References

**Downstream agents MUST read these before planning or implementing.**

### Design System
- `.planning/DESIGN.md` — Neon Topography design system specification: colors, typography, elevation, glassmorphism, bloom effects, no-line rule
- `.planning/examples_mocks/ospf_area_hub/dark/code.html` — Area Hub dark theme HTML mock (target visual for Hub page and area cards)
- `.planning/examples_mocks/ospf_area_hub/dark/screen.png` — Area Hub dark screenshot
- `.planning/examples_mocks/ospf_area_hub/light/code.html` — Area Hub light theme HTML mock
- `.planning/examples_mocks/ospf_area_hub/light/screen.png` — Area Hub light screenshot

### Requirements
- `.planning/REQUIREMENTS.md` — AREA-07 through AREA-11 define Phase 4 acceptance criteria
- `.planning/ROADMAP.md` — Phase 4 success criteria, dependency chain (depends on Phase 2 + Phase 3)

### Prior Phase Context
- `.planning/phases/01-design-token-foundation-and-theme-infrastructure/01-CONTEXT.md` — D-03: Theme toggle absorbed into NavigationPill; D-04/D-05: Light theme palette; D-06: Green glow reduced in light; D-07: Glassmorphism dark-only; D-11: Area accent tokens in system
- `.planning/phases/02-component-restyling/02-CONTEXT.md` — D-01/D-04: Material Symbols Rounded; D-05/D-06: Glassmorphism dark-only overlays; D-08: Canvas node glow box-shadow only for 60fps; D-10: Off-canvas can use richer bloom; D-12: No-line rule absolute
- `.planning/phases/03-area-backend-and-management/03-CONTEXT.md` — D-01: 7 curated accent swatches; D-02: Colors not unique; D-04: Hex stored in DB; D-15: Alphabetical sort; D-16: Unique names enforced

### Key Frontend Files (decomposition targets)
- `frontend/src/components/Canvas.tsx` — 1294 lines, must be decomposed. Already imports fetchAreas, has areas state and areaColorMap
- `frontend/src/App.tsx` — View switching logic (currently canvas/dashboard, needs Hub view added)
- `frontend/src/components/NavBar.tsx` — Will be replaced by NavigationPill component
- `frontend/src/components/DeviceCard.tsx` — Already has areaColor prop for accent stripe (Phase 3 work)

### Backend (area data source)
- `internal/api/area_handler.go` — Area CRUD API handler (GET/POST/PUT/DELETE /api/v1/areas)
- `frontend/src/api/client.ts` — fetchAreas() already implemented
- `frontend/src/types/api.ts` — Area interface and Device.area_id already defined

</canonical_refs>

<code_context>
## Existing Code Insights

### Reusable Assets
- `fetchAreas()` in `api/client.ts` — Already implemented and used by Canvas.tsx
- `Area` interface in `types/api.ts` — Already defined with id, name, description, color
- `areaColorMap` pattern in Canvas.tsx — Already builds Map<areaId, color> for device node coloring
- `DeviceCard.tsx` `areaColor` prop — Already renders area accent stripe on device cards
- `MaterialIcon` component — Ready for Hub/Devices icons in the pill
- `ThemeProvider` / `useTheme()` hook — Theme toggle logic to be moved into NavigationPill
- Glassmorphism pattern: `border-glass-border bg-glass-bg dark:backdrop-blur-[16px] transition-colors` — Established in Phase 2 overlays

### Established Patterns
- View switching in App.tsx: `useState<ActiveView>` with CSS `hidden` class for inactive views (both views stay mounted)
- All styling via Tailwind utility classes with `--nt-*` CSS variable tokens
- `memo()` with custom comparators for React performance on canvas nodes
- Overlay surface pattern: glassmorphism dark / solid tinted light (ContextMenu, SearchOverlay)

### Integration Points
- `App.tsx` — Add 'hub' to ActiveView type, add Hub component, wire NavigationPill as NavBar replacement
- `Canvas.tsx` — Decompose into modules, add area filter prop, implement ghost node rendering
- `NavBar.tsx` — Remove and replace with `NavigationPill.tsx`
- `DeviceCard.tsx` — Ghost node variant needed (muted, no stats, hostname only)

</code_context>

<specifics>
## Specific Ideas

- The pill is the signature navigation element — it's always visible, always oriented. No secondary navigation bars
- THEIA branding stays visible as part of the pill, keeping identity present without a dedicated bar
- Ghost nodes make cross-area topology readable — network operators need to see where traffic exits their area
- Clicking ghost nodes to navigate areas follows the network graph naturally — operators think in terms of device connections
- Watermark should be quiet, like a page label — not a bold design statement. ~1.5rem is enough
- Area cards should feel alive — the bloom effect gives them ambient energy, the hover border accent invites interaction
- The Hub is the "home base" for area navigation — operators land here to get the big picture before drilling into areas

</specifics>

<deferred>
## Deferred Ideas

- Animated link throughput visualization (CANVAS-01) — future requirement, not in v1.3.0
- Canvas-integrated area zones with colored region backgrounds (POLISH-03) — future visual polish
- Smooth CSS transitions on theme switch (POLISH-01) — future polish
- Drag-to-assign devices to areas on canvas — explicitly out of scope (conflates position with grouping)
- Bulk device assignment — nice-to-have from Phase 3 deferred items
- Area detail page (intermediate view between Hub and canvas) — considered but rejected; direct Hub-to-canvas is cleaner

</deferred>

---

*Phase: 04-area-hub-view-and-filtered-topology*
*Context gathered: 2026-03-26*
