# Phase 5: Redesign the Devices Page - Context

**Gathered:** 2026-03-26
**Status:** Ready for planning

<domain>
## Phase Boundary

Redesign the Devices page (Dashboard view) from its current utilitarian HTML table into a polished, Neon Topography-styled table view with enhanced filters, icon-based actions, and proper empty/loading states. Also restyle the SidePanel and its sub-panels to match. This phase does NOT add new views or capabilities — it transforms the visual presentation and UX of the existing Devices page.

</domain>

<decisions>
## Implementation Decisions

### Page Layout
- **D-01:** Enhanced table layout — keep tabular format, restyle with Neon Topography tokens. Familiar to network operators, scannable at 100+ devices
- **D-02:** Columns: Name (hostname/display_name), IP Address, Status (glow dot), Area (with accent color dot), Model, Vendor icon, Uptime, OS version — all sortable
- **D-03:** Row separation via alternating surface tiers (even rows surface-high/30, odd rows bg) — no-line rule, no borders
- **D-04:** Sticky table header — column headers stay visible while scrolling through long device lists
- **D-05:** Hover state is subtle highlight only (elevated/50 background) — no layout shift, no inline expansion
- **D-06:** Row click behavior — Claude's Discretion (navigate to device on canvas, or keep rows passive)
- **D-07:** Render all rows — no pagination or virtualization. ~100-200 devices is well within browser DOM limits

### Device Actions
- **D-08:** Per-device actions presented as Material Symbols icon buttons in a compact row (SSH, Backup, History, Config) with tooltips on hover
- **D-09:** Global actions (Backup All, Vendor Settings) stay in the filter bar area as styled buttons

### Filter & Search Bar
- **D-10:** Custom styled select elements replacing native dropdowns — surface tiers, no borders, matching Neon Topography form inputs
- **D-11:** Add area filter dropdown with area name + accent color dot — table-level area filtering complements the topology-level area filtering
- **D-12:** Search input stays inline in the filter bar — restyled with surface-high bg, no border, Material Symbols search icon, placeholder text
- **D-13:** Device count shown as styled badge with JetBrains Mono numerals (e.g., surface-high pill showing "12 / 45 devices")
- **D-14:** Active filter indicator — when filter is not on "All", the dropdown gets a primary color accent (dot, underline, or tinted bg)

### Empty & Loading States
- **D-15:** No devices: CTA card with Material Symbols device icon, "No devices yet" message, hint to add devices via canvas. Matches Hub empty state pattern (Phase 4 D-24)
- **D-16:** Loading: Skeleton rows with pulsing surface-high blocks showing table structure before data loads
- **D-17:** No filter matches: "No devices match your filters" with a "Clear filters" link/button for quick recovery

### Side Panel Integration
- **D-18:** Restyle SidePanel header, close button, and internal spacing to match Neon Topography tokens. Material Symbols icon for close button
- **D-19:** Panel behavior remains overlay (slides in on top of table from right) — no push layout

### Responsive Behavior
- **D-20:** Horizontal scroll on narrow viewports — all columns preserved, sticky first column (hostname). Mobile is out of scope per PROJECT.md but horizontal scroll handles it gracefully

### Bulk Operations
- **D-21:** No row selection checkboxes — "Backup All" stays as global action. Bulk area assignment deferred from Phase 3 remains deferred

### Claude's Discretion
- Row click behavior (navigate to canvas vs passive rows)
- Exact Material Symbols icon names for each action button
- Custom select dropdown implementation details (pure CSS vs small JS for open/close)
- Skeleton row count and animation timing
- Active filter indicator style (dot, underline, or tinted background)
- Table column width distribution and min-widths
- Vendor icon placement (inline in name column vs separate column)
- SidePanel sub-panel restyling scope (how deeply to restyle SSH, Backup, Config forms)

</decisions>

<canonical_refs>
## Canonical References

**Downstream agents MUST read these before planning or implementing.**

### Design System
- `.planning/DESIGN.md` — Neon Topography design system specification: colors, typography, elevation, glassmorphism, component rules
- `.planning/examples_mocks/ospf_area_hub/dark/screen.png` — Area Hub dark screenshot (reference for card/surface styling)
- `.planning/examples_mocks/ospf_area_hub/light/screen.png` — Area Hub light screenshot

### Token System
- `frontend/src/index.css` — CSS token definitions: `--nt-*` primitives, `@theme inline` semantic mappings, dark/light theme blocks

### Requirements
- `.planning/REQUIREMENTS.md` — COMP-07 (Dashboard/DeviceTable/DeviceRow restyling), COMP-08 (form panel restyling), COMP-09 (metric panels restyling)
- `.planning/ROADMAP.md` — Phase 5 entry

### Prior Phase Context
- `.planning/phases/02-component-restyling/02-CONTEXT.md` — D-01/D-04: Material Symbols Rounded; D-05/D-06: Glassmorphism dark-only; D-12: No-line rule absolute; D-14: Vendor badge muted JetBrains Mono
- `.planning/phases/04-area-hub-view-and-filtered-topology/04-CONTEXT.md` — D-01: NavigationPill replaces NavBar; D-06: Three views (Hub, Topology, Devices); D-24: Hub empty state CTA card pattern

### Existing Components
- `frontend/src/components/Dashboard.tsx` — Current devices page orchestrator
- `frontend/src/components/dashboard/DeviceTable.tsx` — Current table with sortable columns
- `frontend/src/components/dashboard/DeviceRow.tsx` — Current table row with action buttons
- `frontend/src/components/SidePanel.tsx` — Slide-in overlay panel
- `frontend/src/components/MaterialIcon.tsx` — Material Symbols icon component
- `frontend/src/components/icons/VendorIcon.tsx` — Vendor-specific SVG icons

</canonical_refs>

<code_context>
## Existing Code Insights

### Reusable Assets
- `MaterialIcon` component — Ready for action button icons, search icon, close icon, empty state icon
- `VendorIcon` component — Vendor-specific SVG icons (MikroTik, Generic) already built
- `StatusDot` component — Status glow dots with severity scaling
- `SidePanel` component — Slide-in overlay, needs restyling but structure is sound
- `fetchAreas()` in `api/client.ts` — Area data for the area filter dropdown
- `Area` interface in `types/api.ts` — Area type with color property
- CSS token system with dark/light variants — all `--nt-*` tokens available

### Established Patterns
- Tailwind utility classes with CSS variable tokens for all styling
- `font-mono` maps to JetBrains Mono, `font-sans`/`font-display` maps to Outfit
- Surface tier hierarchy: `bg` → `surface` → `surface-high` → `elevated`
- Status glow using box-shadow with `--nt-glow-shadow-opacity` variable
- Overlay surface pattern: glassmorphism dark / solid tinted light

### Integration Points
- `Dashboard.tsx` receives `devices: Device[]` from App.tsx — same data source as Canvas
- `App.tsx` view switching: `ActiveView` type includes 'dashboard', views mounted with CSS `hidden` class
- `NavigationPill.tsx` — Devices icon button triggers `onViewChange('dashboard')`
- Area data flows from `fetchAreas()` — already used in Canvas, needs to be added to Dashboard

</code_context>

<specifics>
## Specific Ideas

- The table should feel like a professional network management console — clean, data-dense, JetBrains Mono for all technical values (IPs, models, versions, uptime)
- Status glow dots in the table should match the canvas DeviceCard glow dots — same severity scaling, same colors
- Area color dots in the table and area filter dropdown should match the NavigationPill color dots — consistent color language across all views
- The CTA empty state should guide new users toward adding devices, not leave them stranded on an empty page
- Skeleton loading should show the actual table structure (columns, rough row heights) so the transition to real data feels seamless

</specifics>

<deferred>
## Deferred Ideas

- Bulk row selection with checkboxes — no current need beyond "Backup All" which is already a global action
- Bulk area assignment — explicitly deferred from Phase 3, remains deferred
- Column visibility customization (show/hide columns) — adds complexity without clear demand
- Saved filter presets — nice-to-have for power users, could be its own small phase
- Device detail page (dedicated full-page view per device) — currently handled by SidePanel, could be future enhancement

</deferred>

---

*Phase: 05-redesign-the-devices-page*
*Context gathered: 2026-03-26*
