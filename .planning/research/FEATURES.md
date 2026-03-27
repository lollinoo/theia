# Feature Landscape: v1.3.0 Frontend Redesign

**Domain:** Network Topology Visualization -- Design System, Theming, and Area-Based Navigation
**Researched:** 2026-03-25
**Scope:** Neon Topography design system implementation, dark/light theming, OSPF area views, context menu redesign, status glow effects
**Overall confidence:** HIGH

## Context

This research updates the feature landscape specifically for the v1.3.0 milestone: a full frontend redesign. The existing product (v1.2.0) already ships a functional topology canvas with device cards, real-time metrics, link visualization, search, alerts panel, context menus, and a dark-only theme using IBM Plex Sans + a purple-accented color scheme. The v1.3.0 milestone replaces the visual layer entirely with the Neon Topography design system while adding area-based navigation and dual-theme support.

The existing codebase uses:
- Tailwind CSS with hardcoded color tokens in `tailwind.config.js` (no CSS variables)
- `darkMode: 'class'` configured but only dark mode implemented
- IBM Plex Sans as the UI font, monospace for metric values
- A purple (`#7b2ff7`) + cyan (`#00d4ff`) accent scheme
- react-flow for the canvas engine

The redesign replaces this with:
- Outfit + JetBrains Mono dual-font strategy
- Green (`#00E676`) primary glow + blue/purple area accents
- Charcoal (`#161618`) dark base, light (`#F8F9FA`) light base
- CSS variable-driven theming for dark/light switching
- Glassmorphism, bloom effects, and the "no-line rule"

---

## Table Stakes

Features users expect from a design system redesign of this nature. Missing any of these makes the redesign feel incomplete or broken.

### Design System Foundation

| Feature | Why Expected | Complexity | Notes |
|---------|--------------|------------|-------|
| CSS variable-driven color tokens | Every modern design system uses CSS custom properties for theming. Hardcoded Tailwind colors (current approach) cannot support theme switching without full class swaps. | MEDIUM | Define semantic tokens (`--color-surface`, `--color-text-primary`, `--color-status-up`) mapped to primitive values per theme. The existing `tailwind.config.js` has 11 hardcoded colors that all need to become variable-driven. |
| Consistent component styling across all existing views | Users will notice if the Settings panel, Alerts panel, Search overlay, or Dashboard table uses old styling while the canvas uses new styling. Partial redesigns feel worse than no redesign. | HIGH | There are 25+ component files in the codebase. Every one must be updated: DeviceCard, ContextMenu, SidePanel, Toolbar, NavBar, AlertsPanel, SearchOverlay, SettingsPanel, AddDevicePanel, DeviceConfigPanel, LinkDetailsPanel, InterfaceStatsPanel, LinkCreatePanel, ZoomControls, ShortcutHelp, SNMPProfileManager, SSHProfileManager, Dashboard, DeviceTable, DeviceRow, BackupPanel, BulkBackupPanel, ConfigViewer, VendorSettingsPanel, SSHCredentialForm. |
| Outfit + JetBrains Mono font integration | The design system specifies a dual-font strategy. Outfit replaces IBM Plex Sans for UI text. JetBrains Mono replaces the generic monospace for technical readouts. Using the wrong fonts undermines the entire aesthetic. | LOW | Google Fonts import. Existing code already uses `font-mono` utility class for metric values; the font-family just changes. Main risk is font-loading flash (FOUT). |
| Surface hierarchy (background > surface > elevated) | The "no-line rule" means panels differentiate through color tiers, not borders. Current code uses `bg-bg-canvas`, `bg-bg-surface`, `bg-bg-elevated` which maps well but the values change. | LOW | Currently: canvas `#2d2d3d`, surface `#363647`, elevated `#3f3f53`. New dark: background `#161618`, surface `#222225`, elevated needs definition. Light: background `#F8F9FA`, surface `#FFFFFF`, elevated `#F1F5F9` or similar. |
| Functional color semantics (status-up, status-down, warning, critical) | Status colors must be readable and distinct in both themes. Current code uses `status-up: #00c853`, `status-down: #ff1744` which aligns well with the Neon Topography palette but needs theme-aware adjustments for light mode. | LOW | Dark: green `#00E676`, red `#FF1744`, yellow `#FFEA00`. Light mode needs darkened variants for contrast on white backgrounds (mock shows `#00C853` for green, `#D50000` for red). |

### Dark/Light Theme Switching

| Feature | Why Expected | Complexity | Notes |
|---------|--------------|------------|-------|
| Toggle between dark and light themes | Every modern web app with a design system supports theme switching. The mocks explicitly show both dark and light variants. Users working in bright environments need light mode; NOC operators at night need dark mode. | MEDIUM | Add a theme toggle to the NavBar or Settings. Use `class` strategy (existing `darkMode: 'class'` config). Apply class to `<html>` element. Both mocks use `class="dark"` / `class="light"` on `<html>`. |
| Theme persistence across sessions | Users will be annoyed if the theme resets on every page load. | LOW | Store preference in `localStorage`. Apply before React hydration to prevent flash of wrong theme (FOWT). A small inline script in `index.html` reads localStorage and sets the class before any CSS renders. |
| System preference detection | Users who set their OS to dark mode expect the app to follow unless overridden. | LOW | `window.matchMedia('(prefers-color-scheme: dark)')`. Use as default when no explicit preference is stored. |
| No flash of wrong theme (FOWT) | A visible flash from light to dark (or vice versa) on page load is jarring and feels buggy. | LOW | Blocking inline script in `<head>` that reads `localStorage` and sets the `dark`/`light` class before any rendering occurs. Must execute before Tailwind/CSS is parsed. |
| All components readable in both themes | If a single panel or modal is unreadable in light mode, users lose trust in the entire theme. This is the most labor-intensive part of theming. | HIGH | Every component must be tested in both themes. The mocks show different shadow strategies (dark: heavy `rgba(0,0,0,0.5)` shadows; light: subtle `rgba(0,0,0,0.05)` shadows), different border treatments, different hover states. The context menu mock shows dark uses `hover:bg-border-subtle` while light uses `hover:bg-white/60`. This per-component adaptation is where most effort goes. |

### Redesigned Device Cards

| Feature | Why Expected | Complexity | Notes |
|---------|--------------|------------|-------|
| Glow-based status indicators replacing the current StatusDot | The design system specifies "Glow Nodes" -- 3x3 rounded-full elements with matching color shadows (e.g., `shadow-[0_0_10px_#00E676]`). Current StatusDot is a simple 10px circle with a glow shadow. Needs to adopt the Neon Topography glow intensity and colors. | LOW | Current StatusDot already has `shadow-[0_0_14px_rgba(0,200,83,0.55)]` for "up" state. Adjustments: update colors to match palette exactly (`#00E676` not `#00c853`), and ensure glow is theme-appropriate (stronger in dark, subtler in light). |
| Card restyling to Neon Topography aesthetics | The current DeviceCard has a purple top-border header (`border-accent-purple`) and body split. The new design should use the charcoal surface with green-glow status, Outfit for labels, JetBrains Mono for values, and the "no-line rule" for internal sectioning. | MEDIUM | The entire DeviceCard component layout changes. The current purple-header/dark-body split is replaced by a unified surface card with glow-based accents. Metric values already use `font-mono`; labels switch from generic sans to Outfit. Ring/highlight logic stays but colors change from cyan/purple to green/area-accent. |
| Hover interactions matching design system | The design system specifies: hovering over a panel triggers an `outline` color transition to the category's signature accent. The OSPF area hub mock demonstrates `hover:border-[#00E676]` on area cards with bloom opacity transitions. | LOW | Add `group` class for hover state management (mocks already use this pattern). Transition border color to area-specific accent on hover. Increase bloom opacity from 10% to 20% on hover. |

### Redesigned Context Menu

| Feature | Why Expected | Complexity | Notes |
|---------|--------------|------------|-------|
| Context menu restyled with Material Symbols icons | The mock shows each menu item with a Material Symbols Outlined icon (terminal, network_ping, content_copy, power_settings_new). Current ContextMenu is text-only with no icons. | MEDIUM | Add Material Symbols Outlined font import. Update ContextMenuItem interface to include an `icon` field (string for the icon name). Render `<span class="material-symbols-outlined">` before each label. The mock uses 18px icon size with muted color, transitioning to text color on hover. |
| Separator support between menu item groups | The mock shows a 1px horizontal divider between normal actions and destructive actions (Reboot Device). Current ContextMenu renders items as a flat list with no grouping. | LOW | Add a `'separator'` type to the menu items array, or a `group` property. Render a `<div class="h-[1px] w-full bg-border-subtle my-1">` between groups. |
| Danger/destructive action styling | The mock shows "Reboot Device" in critical red with a red icon, and a different hover background (`hover:bg-[#331c1e]` in dark, `hover:bg-red-50` in light). Current code has a `variant: 'danger'` property but only changes text color. | LOW | Update danger variant to also change icon color and hover background. Dark: `hover:bg-[#331c1e]`. Light: `hover:bg-red-50`. |
| Glassmorphism surface treatment | Both context menu mocks show `backdrop-blur` and semi-transparent backgrounds. Dark: `bg-surface border border-border-subtle`. Light: `bg-surface` where surface is `rgba(255,255,255,0.7)`. | LOW | Current ContextMenu already has `backdrop-blur-xl` and `bg-bg-surface/95`. Adjust to match exact mock values and ensure the blur works in both themes. |

### OSPF Area Hub View

| Feature | Why Expected | Complexity | Notes |
|---------|--------------|------------|-------|
| Area Hub landing page with aggregate stats | The mock shows a dedicated page with global uptime, total routes, aggregate health as top-level stat cards, followed by per-area breakdown cards. This is the primary new view for v1.3.0. | HIGH | New route/view in the app. Currently the app has two views: 'canvas' and 'dashboard'. This adds 'area-hub' (or replaces the nav structure). Requires backend API for area aggregate data. |
| Per-area cards with health, routes, active links | Each area card shows a glow node (colored by area), area name, description, and key metrics in JetBrains Mono. Area cards are the navigation entry point to per-area topology views. | MEDIUM | Data model: Area { id, name, description, color/accent, devices[] }. Card component renders health status, device/route counts, active link counts. Color-coded by area accent (primary green for backbone, blue for area-1, purple for area-2). |
| Area-specific accent colors and bloom effects | Each area has its own signature color. Area 0 (Backbone): primary green `#00E676`. Area 1: blue `#2979FF`. Area 2: purple `#E040FB`. Each card has a 80px-radius radial blur in its accent color. | LOW | CSS `filter: blur(80px)` on a positioned pseudo-element or child div. The mocks show this as an `absolute` positioned `w-32 h-32` div with `rounded-full` and `opacity-10` (increasing to `opacity-20` on hover). Performance note: blur(80px) is GPU-accelerated but should be limited to visible cards only. |
| Warning state on area cards | Area 2 in the mock shows a warning state: yellow border, yellow glow node, "Warning" health text. Areas inherit the worst status of their member devices. | LOW | Compute area health from member device statuses. If any device is down: critical. If any degraded: warning. All up: optimal. Change border color and glow node color accordingly. |
| Atmospheric watermark | The mock shows "GLOBAL TOPOLOGY" in 60px Outfit font at bottom-left, opacity 20% (dark) / 8% (light), pointer-events-none. Changes contextually by selected area. | LOW | Fixed-position text element. `z-0` so it sits behind content. Text content changes when area filter changes (e.g., "AREA 0 - BACKBONE" when viewing Area 0). |

### Floating Navigation Pill

| Feature | Why Expected | Complexity | Notes |
|---------|--------------|------------|-------|
| Fixed floating pill-shaped tab bar for area switching | The design system positions this as the highest-elevation element. The mock shows it centered at `top-8`, with `rounded-pill` (24px radius), heavy shadow (`0 24px 48px rgba(0,0,0,0.5)` dark / `0 20px 40px -10px rgba(0,0,0,0.1)` light). Tabs: Global, Area 0, Area 1, Area 2, etc. | MEDIUM | This replaces or supplements the current NavBar. The current NavBar is a fixed `h-10` bar at `top-0` with "THEIA" branding and Topology/Devices tabs. The pill either replaces this entirely or sits below it. Based on the mock, the pill is the primary navigation; the current NavBar style would need to be rethought. |
| Active state with glow text-shadow | The mock shows the active tab with `text-shadow: 0 0 8px rgba(245,245,247,0.3)` in dark mode, or a filled background (`bg-text text-white`) in light mode. Inactive tabs show muted text that transitions to the area's accent color on hover. | LOW | CSS `text-shadow` for dark mode active state. Solid background fill for light mode active state. The implementation differs per theme -- this is a good case for CSS variables controlling the active indicator style. |
| Dynamic pill content based on user's areas | The pill should show only the areas the user has created. An empty install shows just "Global". Adding areas adds tabs. | LOW | Render pill tabs from the areas data model. Always include "Global" as the first tab. Each user-created area gets a tab. Max 5-6 tabs before the pill becomes too wide (design constraint from tab bar research: avoid more than 5 items). |

### Area Management in Settings

| Feature | Why Expected | Complexity | Notes |
|---------|--------------|------------|-------|
| Area CRUD (create, read, update, delete) | Users need to define their areas manually (PROJECT.md confirms SNMP-derived areas are out of scope). The Settings panel already has sections for SNMP profiles and SSH profiles; area management follows the same pattern. | MEDIUM | New settings section: "Areas". List existing areas with name, description, accent color, device count. Add/edit form with name, description, color picker (limited to palette: green, blue, purple, or custom). Delete with confirmation. Backend: new `/api/v1/areas` REST endpoints. DB: new `areas` table. |
| Device-to-area assignment | Devices must be assignable to areas. A device belongs to zero or one area (unassigned devices appear in Global view but not in any area view). | MEDIUM | UI: in DeviceConfigPanel (the existing device edit panel), add an "Area" dropdown. Or in the area management section, provide a multi-select to add/remove devices. Backend: `area_id` foreign key on devices table (nullable). |
| Area-filtered topology views | Clicking an area in the pill or hub should filter the canvas to show only devices in that area, with their inter-device links. Global shows everything. | MEDIUM | Filter the react-flow nodes and edges by area membership. When area filter is active, hide devices not in that area. Links where both endpoints are in the area stay visible; links crossing area boundaries could be shown as stubs or hidden. |

---

## Differentiators

Features that go beyond table stakes and create competitive advantage. These are the design choices that make Neon Topography feel like a premium experience rather than a generic dashboard skin.

| Feature | Value Proposition | Complexity | Notes |
|---------|-------------------|------------|-------|
| Bloom/radial-blur effects behind status-critical elements | No open-source NMS uses luminosity-based depth. The 80px radial blur behind area cards and status indicators creates a "deep space" atmosphere that distinguishes this from every competitor (The Dude, LibreNMS, Zabbix all use flat card layouts). | LOW | GPU-accelerated CSS `filter: blur()`. Keep to max 3-5 bloom elements visible at once to avoid performance issues on lower-end machines. The mocks limit bloom to area cards and the pill, not to every element. |
| "No-line rule" -- depth through luminosity, not borders | The design system explicitly rejects `1px solid` borders for layout regions. Instead, surface color tiers create hierarchy. This is the editorial-magazine aesthetic that makes the tool feel high-end. | LOW | Already partially implemented: the mocks use `border-border` only on interactive elements (cards, pill) where click targets need definition. Large layout regions (page sections, the main content area) have no visible borders. Replace existing `border-b border-border-subtle` dividers in NavBar, SidePanel headers, and Settings sections with spacing or color shifts. |
| Monospace delta tags for metric changes | The mock shows `+12%` and `-0.2%` as styled tags with 10% opacity background of their semantic color (green for positive, red for negative). This turns raw numbers into visually scannable trend indicators. | LOW | Small `<span>` with `font-mono text-[13px]` + `bg-primary/10 px-2 py-1 rounded` for positive, `bg-critical/10` for negative. Requires the backend to compute deltas (current value vs previous snapshot). If delta computation is not available yet, defer the dynamic part but build the UI component. |
| Smooth theme transitions (not instant swap) | Most apps that support theming do a hard swap (flash to new theme). A 200-300ms CSS transition on `background-color`, `color`, and `border-color` makes the switch feel intentional and polished. | LOW | Add `transition: background-color 200ms, color 200ms, border-color 200ms` to `body` and key layout elements. Caveat: transitioning `box-shadow` and `filter` can be expensive; only transition `background-color` and `color` on the body. |
| Canvas-integrated area zones (colored region backgrounds) | On the topology canvas, devices belonging to the same area could be visually grouped with a subtle colored background region (like a "zone" or "swim lane"). The glassmorphism spec mentions `rgba(255,255,255,0.02)` for area backgrounds. | HIGH | react-flow supports group nodes (type: 'group') with child nodes inside them. Alternatively, render background rectangles behind device clusters. This is a significant canvas feature that may need its own research spike. If too complex, defer to v1.4 and rely on the area-filtered views instead. |
| Animated link throughput (pulse along edge) | Animating a subtle pulse along link edges in the direction of dominant traffic flow would be a visual differentiator no competitor has. | HIGH | SVG stroke-dasharray animation or a moving gradient along the edge path. Performance concern: animating 50+ edges simultaneously could be expensive. If implemented, only animate visible edges and only when zoom level shows enough detail. Recommend deferring to v1.4. |

---

## Anti-Features

Features to deliberately NOT build in v1.3.0. These are tempting additions that would derail the redesign scope.

| Anti-Feature | Why Tempting | Why Avoid | What to Do Instead |
|--------------|-------------|-----------|-------------------|
| Custom user-defined theme colors | "Let users pick their own accent colors" seems like a natural extension of theming | The Neon Topography design system is opinionated by design. User-customizable colors would break the curated palette, make bloom effects look wrong, and create untested color combinations that harm readability. | Ship exactly two themes (dark and light) with the curated Neon Topography palette. Both are already mocked and tested. |
| Per-device custom colors or icons | "Let users assign custom colors to specific devices" | Creates visual noise that undermines the design system's information hierarchy. If every device is a different color, the status glow system (green=up, red=down) loses its meaning. | Use area accent colors for grouping. Individual devices are differentiated by type icon and hostname, not color. |
| Animated canvas background (particles, grid lines, moving patterns) | "The deep-space aesthetic would look amazing with floating particles" | Constant background animation competes with the actual data (device status, link utilization) for attention. It also burns GPU resources that should be reserved for react-flow rendering at 100+ nodes. | Use the static atmospheric watermark and subtle radial gradients as in the mocks. The light mode mock shows a subtle dot-grid pattern (`background-size: 24px 24px`) which is static and lightweight. |
| Drag-to-assign devices to areas on the canvas | "Drag a device onto an area zone to assign it" | Conflates canvas position (spatial/physical layout) with logical grouping (OSPF areas). A device can be physically close to devices in another area. This interaction model creates confusion. | Assign devices to areas via the Settings panel or DeviceConfigPanel dropdown. Canvas position remains independent of area membership. |
| Real-time OSPF/routing data from SNMP for areas | "Automatically populate areas from OSPF area assignments on routers" | PROJECT.md explicitly states: "SNMP exporters don't expose OSPF area OIDs; manual grouping is simpler and more flexible." Attempting this would require custom SNMP OID polling that the existing snmp_exporter setup does not support. | Manual area creation and device assignment. The "OSPF" naming is organizational/conceptual, not derived from live protocol data. |
| Third theme or "auto" mode that blends dark/light based on time of day | "Automatically switch to dark mode at night" | Adds complexity for minimal value. OS-level auto dark mode already handles this. A time-based blend (twilight theme) would require designing a third complete color set. | Support system preference detection (`prefers-color-scheme`). When set to "system", follow OS setting. User override takes precedence. |
| Inline area editing on the Area Hub page | "Click an area card to edit its name/description right there" | Mixes navigation (clicking an area card should filter to that area) with editing (clicking should open edit mode). Conflicting click targets create confusing UX. | Area cards in the hub are navigation targets (click to view area). Editing happens in Settings > Areas, following the same pattern as SNMP/SSH profile management. |

---

## Feature Dependencies

```
[CSS Variable Token System]
    |-- required by --> [Dark/Light Theme Switching]
    |-- required by --> [All Component Restyling]
    |-- required by --> [Bloom/Glow Effects (theme-aware opacity)]

[Dark/Light Theme Switching]
    |-- requires --> [CSS Variable Token System]
    |-- requires --> [FOWT Prevention Script]
    |-- requires --> [Theme Toggle UI in NavBar/Settings]
    |-- requires --> [localStorage Persistence]

[Font Integration (Outfit + JetBrains Mono)]
    |-- required by --> [All Component Restyling]
    |-- independent of --> [Theme Switching] (fonts don't change per theme)

[Floating Navigation Pill]
    |-- requires --> [Area Data Model (backend)]
    |-- required by --> [Area-Filtered Topology Views]
    |-- required by --> [OSPF Area Hub View]

[Area Data Model (backend: areas table, CRUD API)]
    |-- required by --> [Floating Navigation Pill]
    |-- required by --> [OSPF Area Hub View]
    |-- required by --> [Device-to-Area Assignment]
    |-- required by --> [Area-Filtered Topology Views]
    |-- required by --> [Area Management in Settings]

[OSPF Area Hub View]
    |-- requires --> [Area Data Model]
    |-- requires --> [Floating Navigation Pill]
    |-- requires --> [Restyled Area Cards]
    |-- requires --> [Atmospheric Watermark Component]
    |-- enhances --> [Area-Filtered Topology Views]

[Device-to-Area Assignment]
    |-- requires --> [Area Data Model]
    |-- enhances --> [DeviceConfigPanel (area dropdown)]
    |-- required by --> [Area-Filtered Topology Views]

[Area-Filtered Topology Views]
    |-- requires --> [Device-to-Area Assignment]
    |-- requires --> [Floating Navigation Pill (trigger)]
    |-- modifies --> [Canvas (filter nodes/edges by area)]

[Redesigned Device Cards]
    |-- requires --> [CSS Variable Token System]
    |-- requires --> [Font Integration]
    |-- independent of --> [Area Features]

[Redesigned Context Menu]
    |-- requires --> [CSS Variable Token System]
    |-- requires --> [Material Symbols Font]
    |-- independent of --> [Area Features]

[All Component Restyling]
    |-- requires --> [CSS Variable Token System]
    |-- requires --> [Font Integration]
    |-- includes --> [NavBar, SidePanel, Toolbar, SettingsPanel,
                      AlertsPanel, SearchOverlay, AddDevicePanel,
                      DeviceConfigPanel, LinkDetailsPanel,
                      InterfaceStatsPanel, LinkCreatePanel,
                      ZoomControls, ShortcutHelp, Dashboard,
                      DeviceTable, DeviceRow, BackupPanel,
                      BulkBackupPanel, ConfigViewer,
                      VendorSettingsPanel, SSHCredentialForm,
                      SNMPProfileManager, SSHProfileManager,
                      ReconnectBanner, LinkEdge]
```

### Critical Path

The dependency chain that gates everything else:

1. **CSS Variable Token System** -- nothing can be restyled without this
2. **Font Integration** -- quick win, unblocks component work
3. **Component Restyling** (can be parallelized across components) + **Area Data Model** (backend, can happen in parallel with frontend token work)
4. **Theme Switching** (once tokens are in place, switching is mechanical)
5. **Area Features** (hub view, pill, filtered views -- once backend and tokens are ready)

---

## MVP Recommendation for v1.3.0

### Must Ship (the redesign is incomplete without these)

1. **CSS variable token system** with full dark/light palette -- the foundation everything else depends on
2. **All 25+ components restyled** to Neon Topography -- partial restyling is worse than no restyling
3. **Dark/light theme toggle** with persistence and FOWT prevention -- core user-facing feature
4. **Redesigned DeviceCard** with glow status indicators -- the most visible component
5. **Redesigned ContextMenu** with icons, separators, and glassmorphism -- frequently used interaction
6. **Floating Navigation Pill** -- the signature navigation element of the new design
7. **OSPF Area Hub** with aggregate stats and per-area cards -- the primary new view
8. **Area CRUD in Settings** -- users need to create areas for the hub to be useful
9. **Device-to-area assignment** -- areas are meaningless without device membership
10. **Atmospheric watermark** -- low effort, high visual impact

### Defer to v1.4 (valuable but not essential for the redesign launch)

- **Area-filtered topology views** on the canvas -- the hub view provides area navigation; canvas filtering is an enhancement. Reason: requires react-flow node/edge filtering logic that is orthogonal to the visual redesign work.
- **Canvas-integrated area zones** (colored region backgrounds) -- complex react-flow grouping feature. The hub view provides area context without modifying the canvas.
- **Animated link throughput** -- performance risk, no mock exists for this.
- **Monospace delta tags** (metric change indicators) -- requires backend delta computation that does not exist yet.
- **Smooth theme transitions** -- nice polish, but the core theme swap must work first.

---

## Feature Prioritization Matrix

| Feature | User Value | Effort | Priority | Phase |
|---------|------------|--------|----------|-------|
| CSS variable token system | HIGH | MEDIUM | P0 | Foundation |
| Font integration (Outfit + JetBrains Mono) | HIGH | LOW | P0 | Foundation |
| FOWT prevention script | MEDIUM | LOW | P0 | Foundation |
| Theme toggle UI + localStorage persistence | HIGH | LOW | P1 | Theming |
| System preference detection | MEDIUM | LOW | P1 | Theming |
| Redesigned DeviceCard with glow indicators | HIGH | MEDIUM | P1 | Components |
| Redesigned ContextMenu with icons/separators | MEDIUM | MEDIUM | P1 | Components |
| Redesigned NavBar / navigation structure | HIGH | MEDIUM | P1 | Components |
| All remaining component restyling (23+ components) | HIGH | HIGH | P1 | Components |
| LinkEdge restyling (colors, hover effects) | MEDIUM | LOW | P1 | Components |
| Area data model (backend: table, CRUD API) | HIGH | MEDIUM | P1 | Backend |
| Area CRUD in Settings | HIGH | MEDIUM | P1 | Areas |
| Device-to-area assignment | HIGH | LOW | P1 | Areas |
| Floating Navigation Pill | HIGH | MEDIUM | P1 | Areas |
| OSPF Area Hub view | HIGH | HIGH | P1 | Areas |
| Atmospheric watermark | LOW | LOW | P1 | Areas |
| Area-filtered topology views on canvas | MEDIUM | HIGH | P2 | Deferred |
| Canvas area zones (colored backgrounds) | LOW | HIGH | P2 | Deferred |
| Monospace delta tags | LOW | MEDIUM | P2 | Deferred |
| Smooth theme transitions | LOW | LOW | P2 | Deferred |
| Animated link throughput | LOW | HIGH | P3 | Future |

**Priority key:**
- P0: Foundation -- must be done first, everything depends on it
- P1: Core delivery -- ships in v1.3.0
- P2: Enhancement -- deferred to v1.4
- P3: Future -- speculative, needs its own research

---

## Competitive Context

How the Neon Topography redesign positions Theia against competitors on visual/UX quality:

| Aspect | The Dude | LibreNMS | Zabbix Maps | PRTG | Theia v1.3.0 |
|--------|----------|----------|-------------|------|---------------|
| Dark theme | No | No | No | Partial | Yes (primary) |
| Light theme | N/A | Yes (only) | Yes (only) | Yes (only) | Yes (dual) |
| Design system | None | Bootstrap | Custom, dated | Custom, enterprise | Neon Topography (editorial) |
| Glow/bloom effects | No | No | No | No | Yes |
| Area-based navigation | Sub-maps | No | Linked maps | Multi-layer maps | Area Hub + Pill Nav |
| Glassmorphism | No | No | No | No | Yes |
| Context menu with icons | No | Basic | Basic | Yes | Yes (Material Symbols) |
| Status glow indicators | No | Colored dots | Trigger icons | Colored dots | Glow nodes with shadows |
| Font strategy | System | System | System | System | Dual (Outfit + JetBrains Mono) |

The redesign positions Theia as the only open-source network topology tool with a cohesive, modern design system. This is a genuine differentiator in a space where every competitor looks like enterprise software from 2015.

---

## Sources

- [Neon Topography Design System](/home/azmin/projects/theia/.planning/DESIGN.md) -- primary design reference (HIGH confidence)
- [OSPF Area Hub dark mock](/home/azmin/projects/theia/.planning/examples_mocks/ospf_area_hub/dark/code.html) -- implementation reference (HIGH confidence)
- [OSPF Area Hub light mock](/home/azmin/projects/theia/.planning/examples_mocks/ospf_area_hub/light/code.html) -- implementation reference (HIGH confidence)
- [Node Context Menu dark mock](/home/azmin/projects/theia/.planning/examples_mocks/node_context_menu/dark/code.html) -- implementation reference (HIGH confidence)
- [Node Context Menu light mock](/home/azmin/projects/theia/.planning/examples_mocks/node_context_menu/light/code.html) -- implementation reference (HIGH confidence)
- [Dashboard Design Principles 2026](https://www.designrush.com/agency/ui-ux-design/dashboard/trends/dashboard-design-principles) -- industry patterns (MEDIUM confidence)
- [Dark Glassmorphism UI Trend 2026](https://medium.com/@developer_89726/dark-glassmorphism-the-aesthetic-that-will-define-ui-in-2026-93aa4153088f) -- glassmorphism best practices (MEDIUM confidence)
- [Glassmorphism Implementation Guide](https://playground.halfaccessible.com/blog/glassmorphism-design-trend-implementation-guide) -- performance and accessibility (MEDIUM confidence)
- [React Flow Sub-Flows Documentation](https://reactflow.dev/learn/layouting/sub-flows) -- node grouping patterns (HIGH confidence)
- [React Flow Dynamic Grouping Example](https://reactflow.dev/examples/nodes/dynamic-grouping) -- selection grouping (HIGH confidence)
- [CSS Variables for Design Tokens 2025](https://www.frontendtools.tech/blog/css-variables-guide-design-tokens-theming-2025) -- theming architecture (MEDIUM confidence)
- [Dark Mode in React with Tailwind](https://medium.com/@roman_fedyskyi/dark-mode-in-react-a-scalable-theme-system-with-tailwind-d14e9c1afd1a) -- implementation patterns (MEDIUM confidence)
- [Pill Navigation Design Patterns](https://tegan.io/trends-pill-shaped-navigation/) -- floating pill UX (MEDIUM confidence)
- [Tab Bar Best Practices (Apple HIG)](https://developer.apple.com/design/human-interface-guidelines/tab-bars) -- tab count limits (HIGH confidence)
- [NN/g Glassmorphism Best Practices](https://www.nngroup.com/articles/glassmorphism/) -- accessibility considerations (HIGH confidence)
- [Network Visualization Key Features 2025](https://www.selector.ai/learning-center/network-visualization-tools-key-features-and-top-6-tools/) -- competitor feature sets (MEDIUM confidence)

---
*Feature research for: v1.3.0 Frontend Redesign -- Neon Topography Design System*
*Researched: 2026-03-25*
*Updates previous research from: 2026-03-05 (v1.0 scope)*
