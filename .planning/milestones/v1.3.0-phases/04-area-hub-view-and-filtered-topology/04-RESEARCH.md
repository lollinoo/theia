# Phase 4: Area Hub View and Filtered Topology - Research

**Researched:** 2026-03-26
**Domain:** React frontend -- new views, navigation architecture, canvas decomposition, area-filtered topology
**Confidence:** HIGH

## Summary

Phase 4 is a frontend-only phase that introduces three major features: (1) an Area Hub view showing aggregate network stats and per-area cards, (2) a floating NavigationPill that replaces the NavBar as the sole navigation element, and (3) area-filtered topology canvas with ghost nodes for cross-area links. A prerequisite Canvas.tsx decomposition must happen first, as the file is 1294 lines and adding area filtering without splitting it would make it unmanageable.

The existing codebase provides strong foundations: `fetchAreas()` API client, `Area` interface with `device_count`, `areaColorMap` pattern in Canvas.tsx, `DeviceCard` with `areaColor` prop, and the ThemeProvider/useTheme hook. The view switching pattern in App.tsx (`useState<ActiveView>` with CSS `hidden` class) is simple to extend for a third view. No new npm packages are needed -- all functionality can be built with React, @xyflow/react v12, and the existing Tailwind token system.

**Primary recommendation:** Decompose Canvas.tsx first (extracting utility functions, data builders, and panel rendering), then build NavigationPill and Hub in parallel, and finally add area filtering with ghost nodes to the decomposed canvas.

<user_constraints>
## User Constraints (from CONTEXT.md)

### Locked Decisions
- **D-01:** The floating navigation pill replaces the NavBar entirely -- NavBar component is removed. The pill is the sole navigation element in the app
- **D-02:** Single context-aware pill design. On Hub/Topology views: shows THEIA branding, Hub icon, area buttons (Global + each area), Devices icon, and theme toggle. On Devices view: simplified pill with Hub icon, "Devices" label, and theme toggle
- **D-03:** THEIA branding and version live inside the pill at the left edge, before the Hub icon
- **D-04:** Area buttons overflow via horizontal scroll inside the pill with a max-width constraint. Subtle fade edges hint at more areas when scrollable
- **D-05:** "Global" in the pill navigates to the Area Hub page (aggregate stats + area card grid). Clicking an area button navigates to the filtered topology canvas for that area
- **D-06:** The Area Hub is a new view type in App.tsx alongside 'canvas' and 'dashboard'. Three views: Hub, Topology (canvas), Devices (dashboard)
- **D-07:** Canvas.tsx must be decomposed before adding area filtering. The file is 1294 lines -- split into smaller modules as the first task of this phase
- **D-08:** Instant view swap between Hub and filtered canvas -- no animation. The pill's active area highlight provides orientation
- **D-09:** Hub is the default landing when "Global" is selected in the pill
- **D-10:** Hub header shows four aggregate stats: Network Uptime (longest common uptime from device metrics), Aggregate Health (% devices up), Total Device Count, Active Link Count. No route count (explicitly excluded in PROJECT.md)
- **D-11:** Aggregate Health = (devices with status 'up') / (total devices) * 100. Thresholds: >= 95% "Optimal" (green), >= 80% "Degraded" (yellow), < 80% "Critical" (red)
- **D-12:** Per-area cards show: area name, description, accent color glow dot, health status (same formula scoped to area), device count, active link count
- **D-13:** Selecting an area in the nav pill filters the topology canvas to show only that area's devices and links
- **D-14:** Unassigned devices (no area) only appear in the Global/full topology view. Hidden when any specific area is active
- **D-15:** Cross-area links shown as stubs with ghost nodes -- remote device appears as a small muted node (no stats, just hostname). Makes external connections visible without clutter
- **D-16:** Clicking a ghost node navigates to that device's area (switches the pill to the ghost device's area, showing that area's filtered topology)
- **D-17:** Atmospheric watermark at bottom-left, small text (~1.5rem / text-2xl), Outfit font, very low opacity (~0.10-0.15 dark, ~0.05-0.08 light), fixed position, pointer-events-none
- **D-18:** Watermark text updates contextually: "GLOBAL TOPOLOGY" on Hub, area name (uppercase) on filtered canvas
- **D-19:** Simple fade transition (150ms) when watermark text changes between areas
- **D-20:** Area cards use radial blur bloom (per Phase 2 D-10 allowing richer effects for off-canvas elements). Large radial blur circle in area accent color at top-right, ~0.10 opacity default, ~0.20 on hover. Light mode: subtle, no blur
- **D-21:** Area card border: default standard surface border, hover transitions to area accent color (200ms). Bloom intensifies on hover
- **D-22:** Pill follows established overlay surface pattern: glassmorphism in dark mode (translucent bg + backdrop-blur-16px + subtle border), solid tinted surface in light mode (no backdrop-blur). Consistent with ContextMenu/SearchOverlay
- **D-23:** Each area button in the pill has a small color dot (6-8px) in the area's accent color before the text label. Active area has a glowing dot
- **D-24:** Hub with no areas: show aggregate stats (global data) plus a CTA card -- "No areas yet. Create your first area in Settings" with a link to Settings > Areas. Nav pill shows only "Global"
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

### Deferred Ideas (OUT OF SCOPE)
- Animated link throughput visualization (CANVAS-01) -- future requirement, not in v1.3.0
- Canvas-integrated area zones with colored region backgrounds (POLISH-03) -- future visual polish
- Smooth CSS transitions on theme switch (POLISH-01) -- future polish
- Drag-to-assign devices to areas on canvas -- explicitly out of scope (conflates position with grouping)
- Bulk device assignment -- nice-to-have from Phase 3 deferred items
- Area detail page (intermediate view between Hub and canvas) -- considered but rejected; direct Hub-to-canvas is cleaner
</user_constraints>

<phase_requirements>
## Phase Requirements

| ID | Description | Research Support |
|----|-------------|------------------|
| AREA-07 | OSPF Area Hub view displays aggregate stats (network uptime, aggregate health) and per-area cards | D-10/D-11 define aggregate stats calculation; HTML mocks in `.planning/examples_mocks/ospf_area_hub/` provide visual target; existing `fetchAreas()` and `useWebSocket` provide data sources |
| AREA-08 | Per-area cards show area name, description, health status, device count, and active link count with area-specific accent colors | D-12/D-20/D-21 define card content and bloom effects; `Area` interface already has `device_count` and `color`; link counts derived from filtering `topologyLinks` by area membership |
| AREA-09 | Floating navigation pill allows switching between Global view and individual area views | D-01 through D-05 define pill architecture; replaces NavBar; version info from `fetchHealthVersion()`; areas from `fetchAreas()`; theme toggle from `useTheme()` |
| AREA-10 | Atmospheric watermark displays contextual text that changes per selected area | D-17/D-18/D-19 define watermark styling and behavior; pure CSS/React component with fixed positioning |
| AREA-11 | Area-filtered topology canvas shows only devices and links belonging to the selected area when an area is active in the nav pill | D-13/D-14/D-15/D-16 define filtering logic and ghost nodes; Canvas.tsx decomposition (D-07) is prerequisite |
</phase_requirements>

## Standard Stack

### Core
| Library | Version | Purpose | Why Standard |
|---------|---------|---------|--------------|
| React | 18.3.1 | Component framework | Already in use; all views are React components |
| @xyflow/react | 12.10.1 | Topology canvas | Already in use; provides node types for ghost nodes |
| Tailwind CSS | 4.2.2 | Styling with design tokens | Already in use; all `--nt-*` tokens available |
| Vitest | 4.1.0 | Test runner | Already in use; jsdom environment configured |
| @testing-library/react | 16.3 | Component test utilities | Already in use; established test patterns |

### Supporting
| Library | Version | Purpose | When to Use |
|---------|---------|---------|-------------|
| Material Symbols Rounded | self-hosted subset | Navigation icons | Hub icon, Devices icon in pill (may need font subset update) |

### Alternatives Considered
| Instead of | Could Use | Tradeoff |
|------------|-----------|----------|
| CSS `hidden` class for view switching | React Router | Unnecessary complexity; existing pattern works, all views stay mounted for instant switching |
| Custom horizontal scroll | react-horizontal-scrolling-menu | Over-engineered; native CSS `overflow-x: auto` with fade masks is sufficient |
| New state management lib | React Context or lifting state | Not needed; App.tsx can manage `selectedAreaId` and pass it down |

**Installation:**
No new packages required. All functionality is achievable with the existing stack.

## Architecture Patterns

### Recommended Canvas.tsx Decomposition

The 1294-line Canvas.tsx should be split into focused modules. Analysis of the file reveals these natural boundaries:

```
frontend/src/
├── components/
│   ├── Canvas.tsx                    # Slim orchestrator (~200 lines)
│   ├── canvas/
│   │   ├── canvasHelpers.ts          # Pure functions: buildPositionPayload, inferSpeedLabel,
│   │   │                             # compactThroughput, normalizeInterfaceName, buildThroughputLabel,
│   │   │                             # findLinkMetrics, statusColor, viewportSize (~120 lines)
│   │   ├── edgeBuilder.ts            # buildEdgeData, getHandleSide, buildTopologyEdges,
│   │   │                             # alertStatusForLink (~130 lines)
│   │   ├── nodeBuilder.ts            # Node construction from devices + snapshot merge (~60 lines)
│   │   ├── useCanvasData.ts          # Custom hook: loadTopology, devices/links/areas state,
│   │   │                             # snapshot application, stale data timer (~300 lines)
│   │   ├── useCanvasMenus.ts         # Custom hook: deviceMenu, edgeMenu, panelContent state,
│   │   │                             # context menu items/rendering (~100 lines)
│   │   ├── CanvasPanels.tsx          # Panel rendering IIFE block extraction (SidePanel children) (~150 lines)
│   │   └── CanvasOverlays.tsx        # Edit mode banner, reconnect banner, prometheus alerts (~80 lines)
│   ├── NavigationPill.tsx            # New: floating nav pill
│   ├── AreaHub.tsx                   # New: hub view with aggregate stats + area cards
│   ├── AreaCard.tsx                  # New: individual area card component
│   ├── Watermark.tsx                 # New: atmospheric watermark
│   └── NavBar.tsx                    # DELETED (replaced by NavigationPill)
```

**Rationale for split boundaries:**

1. **canvasHelpers.ts** -- Pure functions with zero React dependencies. Lines 55-100 and 226-273 of current Canvas.tsx. Easy to extract, easy to test.

2. **edgeBuilder.ts** -- All edge/link construction logic. Lines 102-266. Depends only on types, not React state. Already self-contained.

3. **nodeBuilder.ts** -- Node construction from device + position + snapshot data. Currently inlined in `loadTopology`. Extract as `buildTopologyNodes()` function.

4. **useCanvasData.ts** -- The core data hook. Encapsulates: devices/links/areas state, loadTopology function, snapshot effect, stale timer, position saving. Returns `{ nodes, edges, devices, areas, links, loading, error, loadTopology, ... }`. This is the largest extraction (~300 lines) but has a clean boundary.

5. **useCanvasMenus.ts** -- Menu state and handlers. Currently lines 288-398 (device/edge menu state, panel content, shortcuts). Returns menu state and handler callbacks.

6. **CanvasPanels.tsx** -- The massive JSX block (lines 1024-1136) that renders SidePanel children based on panelContent type. Extract as `<CanvasPanels panelContent={...} ... />`.

7. **CanvasOverlays.tsx** -- Edit mode banner, reconnect banner, Prometheus alerts (lines 1140-1191). Extract as `<CanvasOverlays ... />`.

After extraction, Canvas.tsx becomes a ~200-line orchestrator that composes hooks and renders the ReactFlow instance with its overlays.

### View Architecture (App.tsx)

```typescript
// Extended ActiveView type
type ActiveView = 'hub' | 'canvas' | 'dashboard';

// New navigation state
interface NavigationState {
  view: ActiveView;
  selectedAreaId: string | null;  // null = "Global" (Hub shows, canvas shows all)
}
```

**Key design decisions for App.tsx:**
- Add `selectedAreaId` state alongside `activeView` in App.tsx
- When `selectedAreaId` is null and view is 'hub' -- show Hub
- When `selectedAreaId` is set -- view switches to 'canvas' automatically, canvas filters to that area
- NavigationPill receives: `activeView`, `selectedAreaId`, `areas[]`, `onViewChange`, `onAreaSelect`
- Canvas receives: `selectedAreaId` prop for filtering
- Hub receives: `devices`, `areas`, `links`, `snapshot` for aggregate stats

### Ghost Node Pattern (for cross-area links)

```typescript
// Ghost node is a standard DeviceCard variant with minimal data
interface DeviceNodeData {
  device: Device;
  pinned: boolean;
  highlighted?: boolean;
  editMode?: boolean;
  metrics?: DeviceMetricsDTO | null;
  alertStatus?: AlertStatus;
  areaColor?: string;
  onContextMenu?: (event: React.MouseEvent, deviceId: string) => void;
  isGhost?: boolean;              // NEW: marks node as ghost
  onGhostClick?: (deviceId: string) => void;  // NEW: navigate to ghost's area
}
```

**Ghost node filtering algorithm:**
1. Start with all devices that belong to `selectedAreaId`
2. Find all links where at least one end is in the area
3. For links where one end is in the area and the other is not, create a ghost node for the external device
4. Ghost nodes: small size, muted opacity (~0.4), no metrics, show only hostname, dashed or dotted border
5. Ghost node click -> look up device's `area_id`, call `onAreaSelect(areaId)`

### Area Hub Data Flow

The Hub needs data from two sources:
1. **Static data:** Areas (from `fetchAreas()`) and Devices (from `fetchDevices()`) -- already available in App.tsx via Canvas
2. **Live data:** Device statuses and metrics (from WebSocket snapshot) -- need to share `useWebSocket` between views

**Recommendation:** Lift `useWebSocket` from Canvas.tsx to App.tsx during decomposition. The WebSocket connection should be app-level, not canvas-level. This enables the Hub to display live health data without maintaining a separate connection.

```
App.tsx
├── useWebSocket('/api/v1/ws')  -- single connection, shared
├── NavigationPill              -- reads areas, activeView
├── Hub                         -- reads devices, areas, links, snapshot
├── Canvas                      -- reads selectedAreaId, snapshot (passed down)
├── Dashboard                   -- reads devices
└── Watermark                   -- reads selectedAreaId, areas
```

### NavigationPill Layout

```
+--[THEIA v1.3.0]--[Hub icon]--[Global | Area1 | Area2 | ...]--[Devices icon]--[theme toggle]--+
```

- Fixed position: `fixed top-4 left-1/2 -translate-x-1/2 z-30`
- Glassmorphism dark / solid tinted light (per D-22): `border-glass-border bg-glass-bg dark:backdrop-blur-[16px]`
- Pill geometry: `rounded-full` (radius-pill: 9999px)
- Area overflow section: `overflow-x-auto` with CSS mask for fade edges
- Each area button: small color dot (6px `rounded-full`, inline `backgroundColor`) + text label
- Active state: text-on-bg + glowing dot (box-shadow with area color)
- Inactive: text-on-bg-secondary + static dot

### Watermark Component

```typescript
function Watermark({ text }: { text: string }) {
  return (
    <div className="fixed bottom-6 left-6 z-0 pointer-events-none select-none">
      <span
        className="font-sans font-semibold text-2xl tracking-tight
                   text-on-bg-muted opacity-[0.12] dark:opacity-[0.12]
                   transition-opacity duration-150"
        style={{ opacity: undefined }}  // Use CSS tokens
      >
        {text}
      </span>
    </div>
  );
}
```

Per D-17: ~1.5rem (text-2xl in Tailwind = 1.5rem), Outfit font (already `font-sans` in the token system), very low opacity (0.10-0.15 dark, 0.05-0.08 light). The opacity difference between themes can be handled with `dark:opacity-[0.12] opacity-[0.06]`.

Per D-19: Simple fade transition on text change. Use a CSS `transition-opacity` with a brief flash to transparent on change, or use React key to trigger re-mount with CSS animation.

### Anti-Patterns to Avoid
- **Creating a separate WebSocket connection for Hub view:** Share the existing connection from App.tsx. Two connections would double server load and cause inconsistent data.
- **Deep prop drilling for selectedAreaId:** Use direct props through App.tsx -> Canvas. Only two levels deep, no need for Context.
- **Filtering devices/links inside Canvas render:** Compute filtered lists in a `useMemo` with `selectedAreaId` as dependency, not on every render.
- **Making NavBar and NavigationPill coexist:** NavBar is removed entirely (D-01). No transition period.
- **Animating view transitions:** D-08 explicitly says instant view swap, no animation.

## Don't Hand-Roll

| Problem | Don't Build | Use Instead | Why |
|---------|-------------|-------------|-----|
| Horizontal scroll with fade edges | Custom scroll event listener + gradient overlay | CSS `overflow-x: auto` + `mask-image: linear-gradient(...)` | Pure CSS, no JS needed, performant |
| Health percentage formatting | Custom string builder | `Math.round(percentage)` + template literal | Simple arithmetic, not worth abstracting |
| View routing | Custom history/URL management | React state (`useState<ActiveView>`) | No URL routing needed per existing pattern |
| Area color to rgba conversion | Regex hex parser | CSS `color-mix()` or opacity utilities | Tailwind arbitrary values with `/opacity` syntax |

**Key insight:** This phase adds UI views and filtering logic. No complex data transformations, no new API calls, no state management libraries needed. The primary complexity is in the Canvas decomposition (refactoring existing code safely) and ghost node topology filtering (graph algorithm).

## Common Pitfalls

### Pitfall 1: Canvas Decomposition Breaking Refs and Closures
**What goes wrong:** Extracting `loadTopology` into a custom hook breaks closure references to `snapshotRef`, `reactFlow`, `devices`, or `topologyLinks` because these refs/states are no longer in the same scope.
**Why it happens:** `loadTopology` reads from both state and refs, and writes to both. Moving it into a hook requires carefully passing all dependencies.
**How to avoid:** The custom hook `useCanvasData` must own ALL the state it reads/writes: `devices`, `areas`, `topologyLinks`, `nodes`, `edges`, `snapshotRef`, `lastSnapshotTimeRef`. Pass in `reactFlow` as an argument. Return everything the orchestrator Canvas.tsx needs.
**Warning signs:** TypeScript errors about missing properties, stale closure data (metrics not updating after snapshot), `reactFlow` methods throwing "not inside ReactFlowProvider".

### Pitfall 2: Ghost Node Positioning
**What goes wrong:** Ghost nodes appear at (0,0) or overlap with real nodes because they have no saved positions.
**Why it happens:** Ghost nodes are synthetic -- they don't exist in the position store and the force layout doesn't know about them.
**How to avoid:** Position ghost nodes relative to the link they represent. Place them on the edge of the visible area, offset from the real node they connect to. Use the existing `getHandleSide` logic to determine direction, then offset by a fixed distance (e.g., 200px) from the connected real node.
**Warning signs:** Nodes stacked at origin, overlapping nodes, ghost nodes appearing inside the cluster of real nodes.

### Pitfall 3: WebSocket Hook Lifting Breaks Snapshot Timing
**What goes wrong:** After lifting `useWebSocket` to App.tsx, the first-load snapshot race condition (previously fixed with `snapshotRef.current` merge in `loadTopology`) reappears because snapshot and loadTopology are now in different components.
**Why it happens:** The snapshot arrives via WebSocket before `loadTopology` resolves. Previously both lived in the same component so the fix was a ref check.
**How to avoid:** Pass `snapshot` as a prop to Canvas. In `useCanvasData`, accept `snapshot` as a parameter and continue the `snapshotRef` merge pattern. The hook stores the latest snapshot ref internally and merges on loadTopology completion.
**Warning signs:** First page load shows no metrics on device cards until the second snapshot arrives (the exact bug fixed in prior work, documented in MEMORY.md).

### Pitfall 4: Material Symbols Font Subset Missing New Icons
**What goes wrong:** New icons for the pill (e.g., `hub`, `devices`, `lan`) render as blank squares because they're not in the self-hosted font subset.
**Why it happens:** Phase 2 downloaded a subset with exactly 19 icons. New icons for Phase 4 are not included.
**How to avoid:** Re-download the subset with additional icon names appended. Use the Google Fonts API `icon_names` parameter pattern from Phase 2.
**Warning signs:** Blank squares or missing glyphs in the navigation pill.

### Pitfall 5: Infinite Re-render When Filtering Changes
**What goes wrong:** Changing `selectedAreaId` triggers a cascade: filter devices -> rebuild nodes -> trigger edges rebuild -> trigger snapshot reapplication -> trigger another rebuild.
**Why it happens:** Dependencies are not properly memoized; filtering logic runs inside effects that depend on derived state.
**How to avoid:** Use `useMemo` for filtered device/link lists with `[devices, links, selectedAreaId]` dependencies. Don't use `useEffect` for derived data. The filtering is a pure computation, not a side effect.
**Warning signs:** Browser becomes sluggish when switching areas, React DevTools shows rapid re-renders, console warnings about maximum update depth.

### Pitfall 6: Area Link Count Mismatch
**What goes wrong:** Hub area cards show incorrect "active link count" because links cross areas (a link connects device A in area 1 to device B in area 2 -- which area does it belong to?).
**Why it happens:** Ambiguous ownership of cross-area links.
**How to avoid:** Count a link as belonging to an area if EITHER endpoint is in that area. This means a cross-area link is counted in both areas. This matches network operator expectations (they want to see all connections touching their area). Document this counting rule in the component.
**Warning signs:** Link counts don't add up to the global total (expected behavior with cross-area links).

## Code Examples

### Filtering Devices and Links by Area

```typescript
// Source: Derived from existing Canvas.tsx areaColorMap pattern (line 417)
function useAreaFilteredTopology(
  devices: Device[],
  links: Link[],
  selectedAreaId: string | null,
) {
  return useMemo(() => {
    // No filter = show everything (Global view)
    if (!selectedAreaId) {
      return { filteredDevices: devices, filteredLinks: links, ghostDevices: [] };
    }

    // Devices in the selected area
    const areaDeviceIds = new Set(
      devices
        .filter((d) => d.area_id === selectedAreaId)
        .map((d) => d.id),
    );
    const filteredDevices = devices.filter((d) => areaDeviceIds.has(d.id));

    // Links where at least one endpoint is in the area
    const filteredLinks = links.filter(
      (l) => areaDeviceIds.has(l.source_device_id) || areaDeviceIds.has(l.target_device_id),
    );

    // Ghost devices: remote endpoints of cross-area links
    const ghostDeviceIds = new Set<string>();
    for (const link of filteredLinks) {
      if (!areaDeviceIds.has(link.source_device_id)) {
        ghostDeviceIds.add(link.source_device_id);
      }
      if (!areaDeviceIds.has(link.target_device_id)) {
        ghostDeviceIds.add(link.target_device_id);
      }
    }
    const ghostDevices = devices.filter((d) => ghostDeviceIds.has(d.id));

    return { filteredDevices, filteredLinks, ghostDevices };
  }, [devices, links, selectedAreaId]);
}
```

### Aggregate Health Calculation

```typescript
// Source: D-11 from CONTEXT.md
function computeAggregateHealth(
  devices: Device[],
  statuses: Record<string, string>,
): { percentage: number; label: string; color: string } {
  if (devices.length === 0) {
    return { percentage: 100, label: 'N/A', color: 'text-on-bg-secondary' };
  }

  const upCount = devices.filter((d) => {
    const liveStatus = statuses[d.id] ?? d.status;
    return liveStatus === 'up';
  }).length;

  const percentage = (upCount / devices.length) * 100;

  if (percentage >= 95) {
    return { percentage, label: 'Optimal', color: 'text-status-up' };
  }
  if (percentage >= 80) {
    return { percentage, label: 'Degraded', color: 'text-warning' };
  }
  return { percentage, label: 'Critical', color: 'text-status-down' };
}
```

### NavigationPill Horizontal Scroll with Fade Mask

```typescript
// Source: Established CSS pattern for scrollable containers with gradient fade
// CSS mask creates transparent-to-opaque edges that hint at overflow
<div
  className="flex items-center gap-1 overflow-x-auto max-w-[400px] scrollbar-hide"
  style={{
    maskImage: 'linear-gradient(to right, transparent, black 16px, black calc(100% - 16px), transparent)',
    WebkitMaskImage: 'linear-gradient(to right, transparent, black 16px, black calc(100% - 16px), transparent)',
  }}
>
  {areas.map((area) => (
    <AreaButton key={area.id} area={area} active={selectedAreaId === area.id} />
  ))}
</div>
```

### Network Uptime (Longest Common Uptime)

```typescript
// Source: Discretion item -- recommended approach
// "Network Uptime" = minimum uptime across all UP devices (longest common uptime)
// This represents how long the network has been continuously operational
function computeNetworkUptime(
  devices: Device[],
  metrics: Record<string, DeviceMetricsDTO>,
  statuses: Record<string, string>,
): number | null {
  const uptimes = devices
    .filter((d) => (statuses[d.id] ?? d.status) === 'up')
    .map((d) => metrics[d.id]?.uptime_secs)
    .filter((u): u is number => u !== null && u !== undefined);

  if (uptimes.length === 0) return null;
  return Math.min(...uptimes);
}
```

### Ghost Node Visual (DeviceCard variant)

```typescript
// Source: Discretion item -- recommended design
// Ghost nodes are 60% the size of regular DeviceCards, muted styling
// In DeviceCard, check data.isGhost and render a simplified card:
if (data.isGhost) {
  return (
    <div
      className="w-[120px] rounded-xl border border-dashed border-outline-subtle
                 bg-surface/40 px-3 py-2 text-center cursor-pointer
                 hover:border-outline hover:bg-surface/60 transition-colors"
      onClick={() => data.onGhostClick?.(data.device.id)}
    >
      <p className="text-xs text-on-bg-muted truncate">
        {data.device.sys_name || data.device.ip}
      </p>
    </div>
  );
}
```

## Icon Subset Update

The current Material Symbols subset (19 icons) needs additional icons for Phase 4. Recommended additions:

| Icon Name | Usage | Priority |
|-----------|-------|----------|
| `hub` | Hub view icon in NavigationPill | Required |
| `devices` | Devices view icon in NavigationPill | Required |

**Updated download URL:**
```
https://fonts.googleapis.com/css2?family=Material+Symbols+Rounded:opsz,wght,FILL,GRAD@20..48,100..700,0..1,-50..200&icon_names=add,check_circle,close,content_copy,dark_mode,delete,devices,edit,fit_screen,hub,light_mode,link,monitoring,network_ping,notifications,power_settings_new,search,settings,terminal,zoom_in,zoom_out&display=block
```

This adds `hub` and `devices` to the subset (21 total, still well under 30KB).

## State of the Art

| Old Approach | Current Approach | When Changed | Impact |
|--------------|------------------|--------------|--------|
| NavBar as top bar | NavigationPill as floating pill | Phase 4 | NavBar.tsx deleted, NavigationPill.tsx created |
| Two views (canvas/dashboard) | Three views (hub/canvas/dashboard) | Phase 4 | ActiveView type extended, App.tsx routing updated |
| Canvas.tsx monolith (1294 lines) | Decomposed into canvas/ modules | Phase 4 | Better maintainability, easier area filtering |
| WebSocket in Canvas only | WebSocket lifted to App.tsx | Phase 4 | Hub can show live health data |

**Deprecated/outdated:**
- `NavBar.tsx`: Removed entirely, replaced by `NavigationPill.tsx`
- `ActiveView = 'canvas' | 'dashboard'`: Extended to `'hub' | 'canvas' | 'dashboard'`

## Open Questions

1. **Icon name `devices` availability**
   - What we know: The Material Symbols Rounded set has a `devices` icon (device hub icon)
   - What's unclear: Whether `devices` or `devices_other` or `important_devices` is the best match visually
   - Recommendation: Download subset with `devices` first; if visual doesn't fit, try `important_devices` or `dns` as alternatives. The `hub` icon is confirmed to exist in Material Symbols.

2. **Pill z-index interaction with ReactFlow controls**
   - What we know: NavBar currently uses `z-30`, Toolbar uses `z-10`, ReactFlow MiniMap and ZoomControls have their own stacking
   - What's unclear: Whether the pill at `z-30` will conflict with SidePanel or modal overlays
   - Recommendation: Keep `z-30` for pill (same as current NavBar), overlays (SearchOverlay, ContextMenu) already use `z-40+`

3. **Canvas fitView behavior when switching areas**
   - What we know: Current fitView runs on initial load and device count changes
   - What's unclear: Should fitView trigger when switching between areas (to re-center on area's devices)?
   - Recommendation: Yes -- call `fitView` when `selectedAreaId` changes, with the existing `padding: 0.18, duration: 280` parameters. This re-centers the viewport on the filtered subset.

## Validation Architecture

### Test Framework
| Property | Value |
|----------|-------|
| Framework | Vitest 4.1.0 + @testing-library/react 16.3 |
| Config file | `frontend/vitest.config.ts` |
| Quick run command | `cd frontend && npx vitest run` |
| Full suite command | `cd frontend && npx vitest run` |

### Phase Requirements -> Test Map
| Req ID | Behavior | Test Type | Automated Command | File Exists? |
|--------|----------|-----------|-------------------|-------------|
| AREA-07 | Hub displays aggregate stats and per-area cards | unit | `cd frontend && npx vitest run src/components/AreaHub.test.tsx -x` | Wave 0 |
| AREA-08 | Area cards show name, health, device count, link count with accent colors | unit | `cd frontend && npx vitest run src/components/AreaCard.test.tsx -x` | Wave 0 |
| AREA-09 | NavigationPill switches between Global and area views | unit | `cd frontend && npx vitest run src/components/NavigationPill.test.tsx -x` | Wave 0 |
| AREA-10 | Watermark displays contextual text | unit | `cd frontend && npx vitest run src/components/Watermark.test.tsx -x` | Wave 0 |
| AREA-11 | Area-filtered canvas shows only area devices + ghost nodes | unit | `cd frontend && npx vitest run src/components/canvas/useAreaFilteredTopology.test.ts -x` | Wave 0 |

### Sampling Rate
- **Per task commit:** `cd frontend && npx vitest run`
- **Per wave merge:** `cd frontend && npx vitest run`
- **Phase gate:** Full suite green before `/gsd:verify-work`

### Wave 0 Gaps
- [ ] `frontend/src/components/AreaHub.test.tsx` -- covers AREA-07 (aggregate stats rendering)
- [ ] `frontend/src/components/AreaCard.test.tsx` -- covers AREA-08 (card content and accent colors)
- [ ] `frontend/src/components/NavigationPill.test.tsx` -- covers AREA-09 (view switching, area selection)
- [ ] `frontend/src/components/Watermark.test.tsx` -- covers AREA-10 (contextual text)
- [ ] `frontend/src/components/canvas/useAreaFilteredTopology.test.ts` -- covers AREA-11 (filtering logic with ghost nodes)

Existing tests (79 passing) must remain green throughout -- especially `DeviceCard.test.tsx` (will need update for `isGhost` prop) and `AreaManager.test.tsx` (may reference NavBar indirectly).

## Project Constraints (from CLAUDE.md)

- **GSD Workflow:** Must use GSD commands for all file-changing operations
- **Component naming:** PascalCase.tsx for components (NavigationPill.tsx, AreaHub.tsx, AreaCard.tsx, Watermark.tsx)
- **Hook naming:** camelCase.ts with `use` prefix (useCanvasData.ts, useCanvasMenus.ts, useAreaFilteredTopology.ts)
- **Helper files:** camelCase.ts (canvasHelpers.ts, edgeBuilder.ts, nodeBuilder.ts)
- **Test files:** co-located with `.test.tsx` suffix
- **Styling:** Tailwind utility classes only, no separate CSS files for component styles
- **Imports:** relative paths only, no `@/` aliases
- **Module exports:** Named exports for hooks/utilities, default export for primary React components
- **Type imports:** Use `import type { ... }` syntax
- **Error handling:** Try/catch with context messages, safe defaults for malformed data
- **No console.log in production:** No logging in component code
- **Existing patterns:** `memo()` with custom comparators for canvas nodes, glassmorphism pattern for overlays

## Sources

### Primary (HIGH confidence)
- Direct codebase inspection of Canvas.tsx (1294 lines), App.tsx, NavBar.tsx, DeviceCard.tsx, ThemeContext.tsx, api/client.ts, types/api.ts, types/metrics.ts
- `.planning/phases/04-area-hub-view-and-filtered-topology/04-CONTEXT.md` -- all 25 locked decisions
- `.planning/DESIGN.md` -- Neon Topography design system spec
- `.planning/examples_mocks/ospf_area_hub/dark/code.html` and `light/code.html` -- visual targets
- `.planning/REQUIREMENTS.md` -- AREA-07 through AREA-11 definitions
- `.planning/STATE.md` -- project history and accumulated decisions

### Secondary (MEDIUM confidence)
- Phase 2 research and summaries for Material Symbols subset pattern, glassmorphism pattern
- @xyflow/react v12 API (verified installed version 12.10.1 in node_modules)

### Tertiary (LOW confidence)
- None -- all findings verified against codebase

## Metadata

**Confidence breakdown:**
- Standard stack: HIGH -- no new packages, fully existing stack
- Architecture: HIGH -- decomposition strategy derived from direct Canvas.tsx analysis, view pattern derived from existing App.tsx
- Pitfalls: HIGH -- identified from actual code patterns and documented prior bugs (MEMORY.md snapshot race condition)
- Canvas decomposition: HIGH -- every module boundary identified by analyzing line-by-line dependencies in the actual 1294-line file
- Ghost node design: MEDIUM -- the filtering algorithm is straightforward but positioning strategy needs validation during implementation

**Research date:** 2026-03-26
**Valid until:** 2026-04-25 (stable frontend stack, no external API changes expected)
