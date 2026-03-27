# Architecture Patterns

**Domain:** Frontend redesign of a network topology visualizer -- design system, theming, and area-based navigation
**Researched:** 2026-03-25

## Recommended Architecture

The redesign introduces three architectural layers on top of the existing frontend: a **design token system** driven by CSS custom properties, a **theme provider** that swaps token values for dark/light modes, and an **area-aware navigation layer** that filters topology data without duplicating it. The existing react-flow canvas, WebSocket data pipeline, and REST client remain intact -- the redesign wraps them, it does not replace them.

### High-Level Component Tree (Target)

```
<ThemeProvider>                          -- CSS variable injection + class toggle
  <AreaProvider>                         -- area CRUD state, selected area, device-area map
    <App>
      <NavigationPill />                 -- floating area switcher (replaces NavBar)
      <AreaHubView />                    -- area cards, aggregate stats, watermark
        <AggregateStatsBar />
        <AreaCard /> x N
      <ReactFlowProvider>
        <TopologyView />                 -- existing Canvas, filtered by selected area
          <DeviceNode />                 -- restyled DeviceCard
          <LinkEdge />                   -- existing, receives theme tokens
          <Toolbar />
          <ContextMenu />               -- restyled per mockup
          <SidePanel />
          <ZoomControls />
        </TopologyView>
      </ReactFlowProvider>
      <DashboardView />                 -- existing Dashboard, filtered by area
    </App>
  </AreaProvider>
</ThemeProvider>
```

### Component Boundaries

| Component | Responsibility | Communicates With | New/Modified |
|-----------|---------------|-------------------|--------------|
| **ThemeProvider** | Reads preference from localStorage, applies `dark`/`light` class to `<html>`, injects CSS variable sets | All components (via CSS inheritance) | **New** |
| **AreaProvider** | Holds area list, selected area ID, device-to-area assignments; fetches from `/api/v1/areas` | NavigationPill, AreaHubView, TopologyView, DashboardView | **New** |
| **NavigationPill** | Floating pill with area tabs (Global, Area 0, Area 1...); fires area selection and view switching | AreaProvider (via context), App (view state) | **New** (replaces NavBar) |
| **AreaHubView** | Grid of AreaCards with aggregate stats; shown when areas view active | AreaProvider, WebSocket snapshot data | **New** |
| **AreaCard** | Single area summary: health badge, device count, active links, glow status node | AreaHubView (props) | **New** |
| **AggregateStatsBar** | Network Uptime, Total Devices, Aggregate Health panels | WebSocket snapshot, AreaProvider filter | **New** |
| **TopologyView** | Existing Canvas component, refactored to accept filtered device/link lists | AreaProvider (filtered data), WebSocket snapshot | **Modified** (was Canvas.tsx) |
| **DeviceNode** | Restyled device card with glow status indicators, Outfit + JetBrains Mono typography | TopologyView (via react-flow nodeTypes) | **Modified** (was DeviceCard.tsx) |
| **ContextMenu** | Restyled with glassmorphism, icon-labeled items per mockup | TopologyView (positioned via event coordinates) | **Modified** |
| **DashboardView** | Existing Dashboard, receives area-filtered device list | AreaProvider filter | **Modified** (was Dashboard.tsx) |
| **SettingsPanel** | Extended with area management (CRUD areas, assign devices) | AreaProvider, REST client | **Modified** |

### Data Flow

**1. Theme Data Flow (CSS-only, no React re-renders)**

```
User toggles theme
  -> ThemeProvider sets document.documentElement.classList to 'dark' or 'light'
  -> ThemeProvider writes preference to localStorage
  -> CSS custom properties cascade to all elements
  -> No component re-renders needed (CSS handles color swap)
```

**2. Area Data Flow**

```
App mounts
  -> AreaProvider fetches GET /api/v1/areas (list of areas + device assignments)
  -> AreaProvider stores in React context:
       { areas, selectedAreaId, deviceAreaMap, filteredDeviceIds }

User clicks area in NavigationPill
  -> AreaProvider.setSelectedArea(areaId)
  -> TopologyView reads context, filters devices/links to those in selected area
  -> AreaHubView shows aggregate stats for selected area (or all areas if "Global")
  -> DashboardView filters device list by area

User creates/edits/deletes area in SettingsPanel
  -> REST call to POST/PUT/DELETE /api/v1/areas
  -> AreaProvider refetches or optimistically updates context
  -> All consumers re-render with updated area list
```

**3. Real-time Metrics Data Flow (unchanged, but area-filtered)**

```
WebSocket snapshot arrives (existing flow)
  -> useWebSocket hook updates snapshot state
  -> TopologyView applies metrics to nodes/edges (existing logic in Canvas.tsx)
  -> AreaHubView computes aggregate stats from snapshot
     by filtering device IDs per area

Key: snapshot data is NOT duplicated per area. A single snapshot is filtered
at the component level using the deviceAreaMap from AreaProvider.
```

**4. Device Node Rendering (react-flow integration)**

```
react-flow renders nodes via nodeTypes registry
  -> 'device' type maps to DeviceNode component
  -> DeviceNode receives device data + metrics via NodeProps<DeviceNodeData>
  -> DeviceNode uses CSS variables for all colors (theme-aware automatically)
  -> Glow effects use CSS box-shadow with variable-driven colors
  -> Status dot color derived from CSS variables (--color-status-up, etc.)
```

## Patterns to Follow

### Pattern 1: CSS Custom Property Design Tokens

**What:** Define all Neon Topography colors, typography, spacing, and shadows as CSS custom properties on `:root` (light) and `.dark` (dark), referenced by Tailwind via `theme.extend.colors`.

**When:** Every visual property that changes between themes.

**Why:** CSS custom properties cascade without React re-renders. Tailwind v3's `darkMode: 'class'` strategy already supports this. The existing codebase already uses Tailwind's `extend.colors` -- the change is moving from static hex values to CSS variable references.

**Example:**

```css
/* index.css - Design tokens */
@layer base {
  :root {
    /* Light theme */
    --color-bg-canvas: #F5F5F7;
    --color-bg-surface: #FFFFFF;
    --color-bg-surface-container: #F0F0F2;
    --color-bg-surface-container-high: #E8E8EA;
    --color-text-primary: #1A1A1E;
    --color-text-secondary: #6B6B73;
    --color-text-muted: #8A8A93;
    --color-primary: #00E676;
    --color-area-1: #2979FF;
    --color-area-2: #E040FB;
    --color-warning: #FFEA00;
    --color-critical: #FF1744;
    --color-border: #E0E0E4;
    --color-outline: #D0D0D4;
    --color-status-up: #00c853;
    --color-status-down: #FF1744;
    --color-status-probing: #ffc107;
    --color-status-unknown: #657786;
    --color-glassmorphism: rgba(255, 255, 255, 0.85);

    --shadow-panel: 0 24px 48px rgba(0, 0, 0, 0.08);
    --shadow-pill: 0 24px 48px rgba(0, 0, 0, 0.15);
    --shadow-canvas: 0 24px 60px rgba(0, 0, 0, 0.12);

    --font-ui: 'Outfit', sans-serif;
    --font-mono: 'JetBrains Mono', monospace;
  }

  .dark {
    --color-bg-canvas: #161618;
    --color-bg-surface: #222225;
    --color-bg-surface-container: #2A2A2D;
    --color-bg-surface-container-high: #333338;
    --color-text-primary: #F5F5F7;
    --color-text-secondary: #8A8A93;
    --color-text-muted: #6B6B73;
    --color-primary: #00E676;
    --color-area-1: #2979FF;
    --color-area-2: #E040FB;
    --color-warning: #FFEA00;
    --color-critical: #FF1744;
    --color-border: #333338;
    --color-outline: #444448;
    --color-status-up: #00c853;
    --color-status-down: #FF1744;
    --color-status-probing: #ffc107;
    --color-status-unknown: #657786;
    --color-glassmorphism: rgba(255, 255, 255, 0.02);

    --shadow-panel: 0 24px 48px rgba(0, 0, 0, 0.2);
    --shadow-pill: 0 24px 48px rgba(0, 0, 0, 0.5);
    --shadow-canvas: 0 24px 60px rgba(0, 0, 0, 0.28);
  }
}
```

```js
// tailwind.config.js - Points to CSS variables
export default {
  darkMode: 'class',
  theme: {
    extend: {
      colors: {
        'bg-canvas': 'var(--color-bg-canvas)',
        'bg-surface': 'var(--color-bg-surface)',
        'bg-surface-container': 'var(--color-bg-surface-container)',
        'bg-surface-container-high': 'var(--color-bg-surface-container-high)',
        'text-primary': 'var(--color-text-primary)',
        'text-secondary': 'var(--color-text-secondary)',
        'text-muted': 'var(--color-text-muted)',
        primary: 'var(--color-primary)',
        'area-1': 'var(--color-area-1)',
        'area-2': 'var(--color-area-2)',
        warning: 'var(--color-warning)',
        critical: 'var(--color-critical)',
        border: 'var(--color-border)',
        outline: 'var(--color-outline)',
        'status-up': 'var(--color-status-up)',
        'status-down': 'var(--color-status-down)',
        'status-probing': 'var(--color-status-probing)',
        'status-unknown': 'var(--color-status-unknown)',
      },
      fontFamily: {
        ui: ['var(--font-ui)', 'sans-serif'],
        mono: ['var(--font-mono)', 'monospace'],
      },
      boxShadow: {
        panel: 'var(--shadow-panel)',
        pill: 'var(--shadow-pill)',
        canvas: 'var(--shadow-canvas)',
      },
    },
  },
};
```

**Migration path:** Every existing Tailwind color reference (`bg-bg-canvas`, `text-text-primary`, etc.) keeps the same class name but now resolves through CSS variables instead of static hex values. Existing component JSX does not change.

### Pattern 2: ThemeProvider as Thin Class Toggle

**What:** A minimal React context that manages the dark/light preference and applies a CSS class to the document root. No theme object passed through React context -- all styling goes through CSS variables.

**When:** Always. This is the theme switching mechanism.

**Why:** Avoids the "fat context" anti-pattern where changing a theme value re-renders the entire tree. CSS variable changes are handled by the browser's style engine, not React's reconciler. For 100+ device nodes on a react-flow canvas, this is a performance requirement.

**Example:**

```tsx
// contexts/ThemeContext.tsx
type Theme = 'dark' | 'light' | 'system';

interface ThemeContextValue {
  theme: Theme;
  resolvedTheme: 'dark' | 'light';
  setTheme: (theme: Theme) => void;
}

export function ThemeProvider({ children }: { children: ReactNode }) {
  const [theme, setTheme] = useState<Theme>(() => {
    return (localStorage.getItem('theia-theme') as Theme) || 'dark';
  });

  const resolvedTheme = useMemo(() => {
    if (theme === 'system') {
      return window.matchMedia('(prefers-color-scheme: dark)').matches
        ? 'dark' : 'light';
    }
    return theme;
  }, [theme]);

  useEffect(() => {
    const root = document.documentElement;
    root.classList.remove('dark', 'light');
    root.classList.add(resolvedTheme);
    localStorage.setItem('theia-theme', theme);
  }, [resolvedTheme, theme]);

  return (
    <ThemeContext.Provider value={{ theme, resolvedTheme, setTheme }}>
      {children}
    </ThemeContext.Provider>
  );
}
```

### Pattern 3: AreaProvider with Derived Filtering

**What:** A React context that holds area data and exposes a `filteredDeviceIds` Set derived from `selectedAreaId` + `deviceAreaMap`. Consumer components use this Set to filter their own data -- they do not receive pre-filtered arrays.

**When:** Any component that needs to show area-specific data.

**Why:** Avoids duplicating device/link arrays per area. The WebSocket snapshot and REST data stay in their existing locations (Canvas state, useWebSocket hook). Filtering is a cheap `Set.has()` check at the consumer level.

**Example:**

```tsx
// contexts/AreaContext.tsx
interface Area {
  id: string;
  name: string;
  description: string;
  color?: string;
}

interface AreaContextValue {
  areas: Area[];
  selectedAreaId: string | null;     // null = "Global" (all areas)
  setSelectedAreaId: (id: string | null) => void;
  deviceAreaMap: Map<string, string>; // deviceId -> areaId
  filteredDeviceIds: Set<string> | null; // null = show all (Global)
  createArea: (area: Omit<Area, 'id'>) => Promise<void>;
  updateArea: (area: Area) => Promise<void>;
  deleteArea: (id: string) => Promise<void>;
  assignDevice: (deviceId: string, areaId: string) => Promise<void>;
  unassignDevice: (deviceId: string) => Promise<void>;
}
```

### Pattern 4: View Router via State (Not React Router)

**What:** Extend the existing `activeView` state in App.tsx from two views (`canvas` | `dashboard`) to three (`areas` | `canvas` | `dashboard`). The NavigationPill controls both the view and the area selection.

**When:** User navigates between Area Hub, Topology, and Devices views.

**Why:** The app currently uses a simple state-based view toggle (no React Router). Adding a client-side router for three views is over-engineering. The NavigationPill already provides the navigation UX. Keep the pattern consistent.

**Example flow:**
- User on Area Hub (Global) -> clicks "Area 0" in pill -> Area Hub filters to Area 0
- User on Area Hub (Area 0) -> clicks area card -> switches to Topology view filtered to Area 0
- User in Topology -> clicks "Areas" tab in pill -> back to Area Hub
- User clicks "Devices" -> Dashboard view (area filter persists)

### Pattern 5: React-Flow Theme Integration Without v12 Upgrade

**What:** Stay on `reactflow@^11.11.4` for this milestone. Override react-flow's default styles using CSS class overrides (`.react-flow__node`, `.react-flow__edge`, `.react-flow__background`, `.react-flow__minimap`) with CSS variable references.

**When:** For all react-flow visual customization in this milestone.

**Why:** Upgrading to `@xyflow/react` v12 would be a significant refactor: new package name (`reactflow` -> `@xyflow/react`), renamed APIs (`parentNode` -> `parentId`, `onEdgeUpdate` -> `onReconnect`, `xPos`/`yPos` -> `positionAbsoluteX`/`positionAbsoluteY`), immutable update requirements, and new node dimension handling (`node.measured.width`). This is a separate migration that should not be bundled with a design system overhaul. v11's CSS classes are stable and fully overridable.

**How react-flow inherits the theme:**
```css
/* react-flow overrides in index.css */
.react-flow__background {
  background-color: var(--color-bg-canvas);
}

.react-flow__minimap {
  background-color: var(--color-bg-surface);
}

.react-flow__controls button {
  background-color: var(--color-bg-surface);
  color: var(--color-text-primary);
  border-color: var(--color-border);
}

.react-flow__attribution {
  display: none; /* or style to match */
}
```

Custom nodes (DeviceNode) and custom edges (LinkEdge) already use Tailwind classes -- they inherit the theme automatically once the Tailwind config points to CSS variables.

### Pattern 6: Glow Effects via CSS, Not Canvas/SVG Filters

**What:** Implement the Neon Topography "glow node" status indicators and bloom effects using CSS `box-shadow` with multiple spread values, not SVG filters or canvas rendering.

**When:** Device status indicators, active area card highlights, NavigationPill active state.

**Why:** CSS box-shadow is GPU-composited and performant. SVG filters (feGaussianBlur) are expensive to re-render on 100+ nodes. The mockups already use this approach.

**Example:**

```css
/* Status glow: 3x3 dot with matching shadow */
.glow-node-up {
  background-color: var(--color-status-up);
  box-shadow: 0 0 8px var(--color-status-up),
              0 0 16px rgba(0, 200, 83, 0.3);
}

.glow-node-down {
  background-color: var(--color-status-down);
  box-shadow: 0 0 8px var(--color-status-down),
              0 0 16px rgba(255, 23, 68, 0.3);
  animation: pulse 2s ease-in-out infinite;
}

/* Bloom effect behind area cards */
.bloom-primary {
  background: radial-gradient(
    ellipse 200px 200px at center,
    rgba(0, 230, 118, 0.08),
    transparent
  );
}
```

## Anti-Patterns to Avoid

### Anti-Pattern 1: Theme Object in React Context

**What:** Passing a full theme object (`{ colors: { primary: '#00E676', ... }, fonts: { ... } }`) through React context and reading it in every component.

**Why bad:** Every theme change triggers re-render of the entire component tree. With 100+ react-flow nodes, this causes visible frame drops. It also creates tight coupling between components and the theme shape.

**Instead:** CSS custom properties on `:root`/`.dark` + Tailwind utility classes. Components never import or read theme values -- they use `bg-primary`, `text-text-primary`, etc. and the CSS cascade handles the rest.

### Anti-Pattern 2: Duplicating Data Per Area

**What:** Maintaining separate device arrays, link arrays, or snapshot data per area.

**Why bad:** The WebSocket pushes a single snapshot with all devices. Splitting it into per-area copies creates synchronization problems (stale data in inactive areas) and doubles memory usage.

**Instead:** Single source of truth for all data. Areas are a UI filter layer using `Set.has(deviceId)`.

### Anti-Pattern 3: react-flow v12 Upgrade Bundled With Redesign

**What:** Upgrading from `reactflow@11` to `@xyflow/react@12` as part of the design system work.

**Why bad:** The v12 migration has its own breaking changes (package rename, API renames, immutability requirements, node dimension changes). Combining it with a visual redesign creates two failure modes in one milestone -- if the design looks wrong, is it a theme bug or a v12 API mismatch?

**Instead:** Complete the design system on v11. Plan a separate v12 migration milestone afterwards, which will be simpler because the CSS variable infrastructure will already be in place.

### Anti-Pattern 4: Canvas State Mega-Component

**What:** The current Canvas.tsx is ~750 lines with 15+ useState calls, multiple useEffect chains, and all business logic inline. Adding area filtering here would push it past maintainable limits.

**Why bad:** Difficult to test, impossible to reuse logic, hard to reason about data flow.

**Instead:** Extract into focused modules during or before the redesign:
- `useTopologyData` hook: device/link fetching + position merging
- `useSnapshotApplication` hook: applying WebSocket snapshots to nodes/edges
- `useCanvasInteractions` hook: context menus, edit mode, keyboard shortcuts
- Area filtering stays in AreaProvider, consumed via context

### Anti-Pattern 5: Inline Hardcoded Colors

**What:** Using hex values directly in component JSX (e.g., `bg-[#1a1a24]`, `text-[#8899a6]`, `border-bg-canvas`).

**Why bad:** The current codebase has several hardcoded hex values in DeviceCard.tsx (`bg-[#1a1a24]`, `bg-[#12121a]`), Canvas.tsx, and StatusDot.tsx that bypass the Tailwind theme. These will not respond to theme changes.

**Instead:** Every color must go through the token system. Audit all `bg-[#...]`, `text-[#...]`, `border-[#...]` arbitrary Tailwind classes and replace with semantic token references. Discovered hardcoded values so far:
- `DeviceCard.tsx` line 133: `bg-[#1a1a24]` (header background)
- `DeviceCard.tsx` line 149: `bg-[#12121a]` (body background)
- `DeviceCard.tsx` line 24: `!bg-[#8899a6]` (handle color)
- `index.css` line 18: `background-color: #2d2d3d` (body)
- Various `shadow-[0_0_28px_rgba(...)]` in DeviceCard.tsx

## Scalability Considerations

| Concern | At 50 devices | At 100+ devices | At 500+ devices |
|---------|--------------|-----------------|-----------------|
| **Theme switching** | Instant (CSS variables) | Instant (CSS variables) | Instant (CSS variables) |
| **Area filtering** | Set.has() trivial | Set.has() trivial | Set.has() trivial |
| **Snapshot application** | ~50 node updates | ~100 node maps, use startTransition (already done) | Virtualize or batch; may need react-flow viewport culling |
| **Area Hub aggregation** | Iterate snapshot once | Iterate snapshot once per area card | Memoize per-area aggregates with useMemo keyed on snapshot + area |
| **Glow/bloom CSS effects** | No impact | Watch for composite layer count | Reduce effects at zoom-out levels via CSS `will-change` or conditional classes |

## Suggested Build Order

The architecture has clear dependency chains that dictate build order:

```
Phase 1: Design Token Foundation
  |-- CSS custom properties in index.css (dark + light token sets)
  |-- tailwind.config.js updated to use CSS variable references
  |-- ThemeProvider context (class toggle + localStorage persistence)
  |-- Theme toggle control in settings or nav
  |-- Google Fonts loaded: Outfit + JetBrains Mono
  |-- Audit and replace all hardcoded hex values in existing components
  |
  Dependency: Nothing else can be themed until tokens exist.
  Risk: Low. Mechanical find-and-replace. Existing Tailwind class names stay the same.

Phase 2: Component Restyling
  |-- DeviceCard -> DeviceNode (Neon Topography: Outfit/JetBrains Mono, glow indicators,
  |   surface hierarchy, no-line rule)
  |-- ContextMenu restyled (glassmorphism, icon-labeled items per mockup)
  |-- NavigationPill (replaces NavBar, floating pill geometry, shadow-pill)
  |-- SidePanel, Toolbar, ZoomControls restyled to Neon Topography
  |-- Atmospheric watermark CSS class
  |
  Dependency: Requires Phase 1 tokens. Can be done component-by-component.
  Risk: Medium. DeviceNode restyle must preserve react-flow Handle positions and
  the custom memo equality check.

Phase 3: Area Backend + Provider
  |-- Backend: Area domain model, DB migration, CRUD API endpoints
  |   (area table, device_area_assignments table)
  |-- AreaProvider context (fetch areas, device-area map, selection state)
  |-- Area management UI in SettingsPanel (create/edit/delete areas, assign devices)
  |-- NavigationPill wired to AreaProvider for area tab rendering
  |
  Dependency: Backend must exist before frontend can fetch areas.
  Can run in parallel with Phase 2 (different people/tracks).
  Risk: Medium. New domain entity end-to-end (Go + SQL + React).

Phase 4: Area Hub View + Filtered Topology
  |-- AreaHubView with AreaCards grid and AggregateStatsBar
  |-- Aggregate stat computation from WebSocket snapshot + area filter
  |-- Atmospheric watermark per area (area name as large background text)
  |-- Area-filtered TopologyView (Canvas filters nodes/edges by area)
  |-- Area-filtered DashboardView
  |-- View routing in App.tsx extended to three views (areas | canvas | dashboard)
  |
  Dependency: Requires Phase 3 (areas exist) AND Phase 2 (components styled).
  Risk: Medium-High. New view with live data aggregation.

Phase 5 (Recommended, Not Blocking): Canvas Decomposition
  |-- Extract useTopologyData, useSnapshotApplication, useCanvasInteractions hooks
  |-- Canvas.tsx becomes a thin orchestrator (~150 lines)
  |
  Dependency: Can happen during any phase but yields most value before Phase 4.
  Risk: Low-medium. Pure refactor, no behavior change.
```

**Critical path:** Phase 1 -> Phase 2 + Phase 3 (parallel) -> Phase 4

**Phase 5 is optional for v1.3.0** but strongly recommended if Canvas.tsx area filtering proves unwieldy.

## Key Technical Decisions

### Stay on reactflow v11, Do Not Upgrade to v12

**Confidence:** HIGH (verified via official migration docs at reactflow.dev/learn/troubleshooting/migrate-to-v12)

React Flow v12 (`@xyflow/react`) introduces native `colorMode` prop and `--xy-*` CSS variables for theming. However, it also renames the package and breaks multiple APIs. The current `reactflow@^11.11.4` CSS class overrides (`.react-flow__*`) are sufficient for theming, and custom nodes (DeviceCard) use Tailwind which will inherit CSS variables automatically. Upgrade to v12 in a separate future milestone.

### CSS Variables Over Tailwind dark: Variant

**Confidence:** HIGH

Tailwind's `dark:` variant would require adding `dark:` prefixes to every color class in every component (e.g., `bg-surface dark:bg-surface-dark`). With 20+ components and hundreds of color references, this is error-prone and noisy. Instead, using CSS variables means each Tailwind class (e.g., `bg-bg-surface`) resolves to the correct color based on the `.dark`/`.light` class on `<html>`. Zero component JSX changes needed for theme switching.

### No Zustand -- Keep React Context + useState

**Confidence:** MEDIUM

The current codebase uses zero Zustand -- all state is React useState/useRef inside Canvas.tsx and local component state. Adding Zustand for area/theme state while keeping Canvas on useState creates an inconsistent split. Use React Context for ThemeProvider and AreaProvider. Area state is low-frequency (user manages areas occasionally, not every frame). If Canvas.tsx state management becomes a bottleneck during the redesign, evaluate Zustand in a separate refactoring effort.

### Self-Host Fonts, Not Google Fonts CDN

**Confidence:** MEDIUM

The mockups reference Outfit and JetBrains Mono from Google Fonts. For a network monitoring tool deployed on internal networks that may lack internet access, bundle the WOFF2 files in `frontend/public/fonts/` and declare `@font-face` rules in `index.css`. This eliminates the external CDN dependency.

### NavigationPill Replaces NavBar, Does Not Coexist

**Confidence:** HIGH (based on mockup analysis)

The OSPF Area Hub mockups show a floating pill at the top center as the sole navigation element. The existing NavBar (fixed top bar with "Topology" and "Devices" tabs) should be replaced entirely. The pill handles both view switching (Areas/Topology/Devices) and area selection (Global/Area 0/Area 1/...). Having both would be confusing and waste vertical space.

## Sources

- [Tailwind CSS v3 Dark Mode docs](https://v3.tailwindcss.com/docs/dark-mode) -- HIGH confidence
- [React Flow Theming](https://reactflow.dev/learn/customization/theming) -- HIGH confidence
- [React Flow v12 Migration Guide](https://reactflow.dev/learn/troubleshooting/migrate-to-v12) -- HIGH confidence
- [React Flow Custom Nodes](https://reactflow.dev/learn/customization/custom-nodes) -- HIGH confidence
- [React Flow Dark Mode Example](https://reactflow.dev/examples/styling/dark-mode) -- HIGH confidence
- [Building a Themed Design System with Tailwind and CSS Variables](https://medium.com/@andriy.vl/building-a-themed-design-system-with-tailwind-and-css-variables-for-react-and-next-js-apps-2df0ff783440) -- MEDIUM confidence
- [Dark Mode in React: A Scalable Theme System with Tailwind](https://medium.com/@roman_fedyskyi/dark-mode-in-react-a-scalable-theme-system-with-tailwind-d14e9c1afd1a) -- MEDIUM confidence
- [Zustand Slice Pattern](https://github.com/pmndrs/zustand/blob/main/docs/learn/guides/slices-pattern.md) -- HIGH confidence (considered and decided against for this milestone)
- [Creating Custom Themes with Tailwind CSS](https://blog.logrocket.com/creating-custom-themes-tailwind-css/) -- MEDIUM confidence

---

*Architecture analysis: 2026-03-25*
*Domain: Frontend redesign -- design system, theming, and area-based navigation for Theia network topology visualizer*
