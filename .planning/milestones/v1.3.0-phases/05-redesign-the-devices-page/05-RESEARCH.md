# Phase 5: Redesign the Devices Page - Research

**Researched:** 2026-03-26
**Domain:** Frontend UI redesign (React, Tailwind CSS, Neon Topography design system)
**Confidence:** HIGH

## Summary

Phase 5 transforms the existing Devices page (Dashboard view) from its current utilitarian HTML table into a polished, Neon Topography-styled table with enhanced filters, icon-based actions, and proper empty/loading states. It also restyles the SidePanel and its sub-panels. This is a pure frontend restyling phase -- no new backend endpoints, no new views, no new features. The scope covers three open requirements: COMP-07 (Dashboard/DeviceTable/DeviceRow), COMP-08 (form panel restyling), and COMP-09 (metric panels restyling).

The existing codebase is well-structured for this work. Dashboard.tsx orchestrates filter state and panel switching, DeviceTable.tsx handles sorting, and DeviceRow.tsx renders individual rows. All three are clean, focused components that can be restyled in place. The main integration gap is that Dashboard currently does not receive area data -- it only gets `devices: Device[]` from App.tsx, but devices already carry `area_id?: string`. Area data (for the area filter dropdown and color dots) needs to be threaded from App.tsx down to Dashboard.

**Primary recommendation:** Structure this as 3 plans: (1) Restyle the filter bar and table structure (Dashboard + DeviceTable + DeviceRow) with area data threading, empty states, loading skeleton, and sticky header; (2) Replace text-label action buttons with Material Symbol icon buttons and add the custom select dropdowns; (3) Restyle SidePanel chrome and sub-panel forms/content for COMP-08 and COMP-09.

<user_constraints>
## User Constraints (from CONTEXT.md)

### Locked Decisions
- **D-01:** Enhanced table layout -- keep tabular format, restyle with Neon Topography tokens. Familiar to network operators, scannable at 100+ devices
- **D-02:** Columns: Name (hostname/display_name), IP Address, Status (glow dot), Area (with accent color dot), Model, Vendor icon, Uptime, OS version -- all sortable
- **D-03:** Row separation via alternating surface tiers (even rows surface-high/30, odd rows bg) -- no-line rule, no borders
- **D-04:** Sticky table header -- column headers stay visible while scrolling through long device lists
- **D-05:** Hover state is subtle highlight only (elevated/50 background) -- no layout shift, no inline expansion
- **D-06:** Row click behavior -- Claude's Discretion (navigate to device on canvas, or keep rows passive)
- **D-07:** Render all rows -- no pagination or virtualization. ~100-200 devices is well within browser DOM limits
- **D-08:** Per-device actions presented as Material Symbols icon buttons in a compact row (SSH, Backup, History, Config) with tooltips on hover
- **D-09:** Global actions (Backup All, Vendor Settings) stay in the filter bar area as styled buttons
- **D-10:** Custom styled select elements replacing native dropdowns -- surface tiers, no borders, matching Neon Topography form inputs
- **D-11:** Add area filter dropdown with area name + accent color dot -- table-level area filtering complements the topology-level area filtering
- **D-12:** Search input stays inline in the filter bar -- restyled with surface-high bg, no border, Material Symbols search icon, placeholder text
- **D-13:** Device count shown as styled badge with JetBrains Mono numerals (e.g., surface-high pill showing "12 / 45 devices")
- **D-14:** Active filter indicator -- when filter is not on "All", the dropdown gets a primary color accent (dot, underline, or tinted bg)
- **D-15:** No devices: CTA card with Material Symbols device icon, "No devices yet" message, hint to add devices via canvas. Matches Hub empty state pattern (Phase 4 D-24)
- **D-16:** Loading: Skeleton rows with pulsing surface-high blocks showing table structure before data loads
- **D-17:** No filter matches: "No devices match your filters" with a "Clear filters" link/button for quick recovery
- **D-18:** Restyle SidePanel header, close button, and internal spacing to match Neon Topography tokens. Material Symbols icon for close button
- **D-19:** Panel behavior remains overlay (slides in on top of table from right) -- no push layout
- **D-20:** Horizontal scroll on narrow viewports -- all columns preserved, sticky first column (hostname). Mobile is out of scope per PROJECT.md but horizontal scroll handles it gracefully
- **D-21:** No row selection checkboxes -- "Backup All" stays as global action. Bulk area assignment deferred from Phase 3 remains deferred

### Claude's Discretion
- Row click behavior (navigate to canvas vs passive rows)
- Exact Material Symbols icon names for each action button
- Custom select dropdown implementation details (pure CSS vs small JS for open/close)
- Skeleton row count and animation timing
- Active filter indicator style (dot, underline, or tinted background)
- Table column width distribution and min-widths
- Vendor icon placement (inline in name column vs separate column)
- SidePanel sub-panel restyling scope (how deeply to restyle SSH, Backup, Config forms)

### Deferred Ideas (OUT OF SCOPE)
- Bulk row selection with checkboxes -- no current need beyond "Backup All" which is already a global action
- Bulk area assignment -- explicitly deferred from Phase 3, remains deferred
- Column visibility customization (show/hide columns) -- adds complexity without clear demand
- Saved filter presets -- nice-to-have for power users, could be its own small phase
- Device detail page (dedicated full-page view per device) -- currently handled by SidePanel, could be future enhancement
</user_constraints>

<phase_requirements>
## Phase Requirements

| ID | Description | Research Support |
|----|-------------|------------------|
| COMP-07 | Dashboard, DeviceTable, and DeviceRow restyled with new typography and color tokens | Core of this phase -- table layout, filter bar, row styling, empty/loading states all address this |
| COMP-08 | AddDevicePanel, DeviceConfigPanel, and LinkCreatePanel restyled with updated form styling | SidePanel chrome + sub-panel form input standardization covers this |
| COMP-09 | LinkDetailsPanel and InterfaceStatsPanel restyled with JetBrains Mono for metric values | SidePanel sub-panel metric readout restyling covers this |
</phase_requirements>

## Standard Stack

### Core
| Library | Version | Purpose | Why Standard |
|---------|---------|---------|--------------|
| React | 18.3 | Component framework | Already in use, no changes needed |
| Tailwind CSS | 4.x | Utility-first styling via `@theme inline` | Already in use with `--nt-*` token system |
| TypeScript | 5.7 | Type safety | Already in use with strict mode |

### Supporting
| Library | Version | Purpose | When to Use |
|---------|---------|---------|-------------|
| Material Symbols Rounded | self-hosted subset | Icon font for action buttons, search icon, close icon | All icon instances in this phase |
| `@fontsource-variable/outfit` | installed | Display/UI font | Headers, labels, filter text |
| `@fontsource-variable/jetbrains-mono` | installed | Technical readout font | IP addresses, models, uptime, OS version, device count badge |

### Alternatives Considered
| Instead of | Could Use | Tradeoff |
|------------|-----------|----------|
| Custom select dropdown (JS) | Radix UI Select or Headless UI | Adding a new dependency for 2-3 dropdowns is excessive; a minimal JS toggle with CSS styling matches the rest of the codebase |
| Virtualized list | Render all rows | D-07 explicitly rules out virtualization; ~200 rows is trivially handled by DOM |

## Architecture Patterns

### Recommended Project Structure
```
frontend/src/
  components/
    Dashboard.tsx              # Restyled - orchestrator with area data, filter bar, empty/loading states
    SidePanel.tsx              # Restyled - chrome (header, close button, spacing)
    StatusDot.tsx              # Reused as-is for table status column
    MaterialIcon.tsx           # Reused as-is for action icons
    icons/VendorIcon.tsx       # Reused as-is for vendor column
    dashboard/
      DeviceTable.tsx          # Restyled - expanded columns, sticky header, skeleton loading
      DeviceRow.tsx            # Restyled - icon actions, area dot, typography, new columns
      SSHCredentialForm.tsx    # Form inputs restyled to match Neon Topography
      BackupPanel.tsx          # Form/content restyled
      BackupHistoryTable.tsx   # Table restyled with tokens
      ConfigViewer.tsx         # Content restyled
      BulkBackupPanel.tsx      # Content restyled
      VendorSettingsPanel.tsx  # Content restyled
```

### Pattern 1: Area Data Threading
**What:** Pass `areas: Area[]` from App.tsx through to Dashboard, which needs it for the area filter dropdown and for resolving area names/colors from device `area_id`.
**When to use:** Whenever Dashboard needs to display or filter by area.
**Example:**
```typescript
// App.tsx -- add areas prop to Dashboard
<Dashboard devices={canvasDevices} areas={areas} />

// Dashboard.tsx -- new prop
interface DashboardProps {
  devices: Device[];
  areas: Area[];
}

// Area lookup helper
const areaMap = useMemo(() => {
  const map = new Map<string, Area>();
  for (const a of areas) map.set(a.id, a);
  return map;
}, [areas]);
```

### Pattern 2: Custom Select Dropdown
**What:** A lightweight JS-controlled dropdown that replaces native `<select>` with styled options matching Neon Topography surfaces.
**When to use:** For status filter, type filter, and area filter dropdowns (D-10).
**Example:**
```typescript
// Minimal controlled dropdown - use state for open/close, render options in a positioned div
function FilterSelect({ value, onChange, options, label }: FilterSelectProps) {
  const [open, setOpen] = useState(false);
  const ref = useRef<HTMLDivElement>(null);

  // Close on outside click
  useEffect(() => {
    if (!open) return;
    const handler = (e: MouseEvent) => {
      if (ref.current && !ref.current.contains(e.target as Node)) setOpen(false);
    };
    document.addEventListener('mousedown', handler);
    return () => document.removeEventListener('mousedown', handler);
  }, [open]);

  return (
    <div ref={ref} className="relative">
      <button onClick={() => setOpen(!open)} className="...styled trigger...">
        {label}: {options.find(o => o.value === value)?.label}
      </button>
      {open && (
        <div className="absolute top-full mt-1 bg-surface-high rounded-panel shadow-panel z-50 min-w-[160px]">
          {options.map(opt => (
            <button
              key={opt.value}
              onClick={() => { onChange(opt.value); setOpen(false); }}
              className="w-full text-left px-3 py-2 text-sm hover:bg-elevated/50 transition-colors ..."
            >
              {opt.label}
            </button>
          ))}
        </div>
      )}
    </div>
  );
}
```

### Pattern 3: Skeleton Loading Rows
**What:** Placeholder rows with animated pulsing blocks that mirror the real table column layout.
**When to use:** When `devices.length === 0` and we haven't determined if it's "truly empty" vs "still loading" (D-16).
**Example:**
```typescript
function SkeletonRow() {
  return (
    <tr>
      <td className="px-3 py-2.5"><div className="h-4 w-32 bg-surface-high rounded animate-pulse" /></td>
      <td className="px-3 py-2.5"><div className="h-4 w-24 bg-surface-high rounded animate-pulse" /></td>
      <td className="px-3 py-2.5"><div className="h-4 w-12 bg-surface-high rounded animate-pulse" /></td>
      {/* ... more columns ... */}
    </tr>
  );
}
```

### Pattern 4: Sticky Table Header with CSS
**What:** Use `position: sticky` on `<thead>` or `<th>` elements to keep column headers visible during scroll (D-04).
**When to use:** On the DeviceTable when the device list exceeds viewport height.
**Example:**
```typescript
// thead with sticky positioning
<thead className="sticky top-0 z-10 bg-bg">
  <tr className="text-left text-on-bg-secondary">
    {columns.map(col => (
      <th key={col.key} className="px-3 py-2 text-[12px] font-normal uppercase tracking-[0.16em] ...">
        {col.label}
      </th>
    ))}
  </tr>
</thead>
```
**Important:** The `bg-bg` (or appropriate surface color) must be set on the sticky header so it doesn't show content bleeding through behind it. The scroll container should be the `flex-1 overflow-auto` div wrapping the table in Dashboard.tsx.

### Pattern 5: StatusDot Reuse
**What:** Import and use the existing `StatusDot` component from `../StatusDot` in DeviceRow for status column.
**When to use:** Status column rendering. Already uses the same glow pattern as DeviceCard.
**Example:**
```typescript
import { StatusDot } from '../StatusDot';

// In the status column
<td className="px-3 py-2.5">
  <div className="flex items-center gap-1.5">
    <StatusDot status={device.status} />
    <span className="text-on-bg-secondary capitalize">{device.status}</span>
  </div>
</td>
```

### Anti-Patterns to Avoid
- **Using `border-b` on table rows:** Violates the no-line rule (COMP-12, Phase 2 D-12). Use alternating surface tiers instead.
- **Native `<select>` elements:** D-10 explicitly requires custom styled selects. Native dropdowns cannot be styled to match Neon Topography.
- **Hardcoded hex colors:** All colors must use CSS variable tokens (`bg-surface-high`, `text-on-bg`, etc.) -- never hardcode hex values.
- **Pagination or virtualization:** D-07 explicitly rules this out for ~100-200 devices.
- **Adding `backdrop-filter` for table effects:** Glassmorphism is reserved for overlay surfaces (context menu, search overlay, nav pill). Table elements are standard surfaces.

## Don't Hand-Roll

| Problem | Don't Build | Use Instead | Why |
|---------|-------------|-------------|-----|
| Status indicator dots | Custom styled spans | `StatusDot` component | Already built with correct glow severity scaling and theme-adaptive colors |
| Icon rendering | Inline SVGs or custom icon components | `MaterialIcon` component | Already wired to self-hosted Material Symbols Rounded subset font |
| Vendor icons | Text labels or emoji | `VendorIcon` component | Already has MikroTik, Cisco, Ubiquiti, and Generic SVG icons |
| Area color adaptation | Raw hex colors | `adaptAreaColor()` from ThemeContext | Handles dark-to-light color mapping for area accent colors |
| Area data fetching | Custom fetch in Dashboard | Receive `areas` prop from App.tsx | App.tsx already fetches and manages area state |

## Common Pitfalls

### Pitfall 1: Font Subset Missing Icons
**What goes wrong:** New Material Symbols icon names used for action buttons (SSH, Backup, History, Config) may not be in the current 19-icon woff2 subset, rendering as blank rectangles.
**Why it happens:** The font is a pre-built subset containing only the icons used by the existing codebase: `edit`, `search`, `add`, `link`, `notifications`, `settings`, `close`, `hub`, `check_circle`, `zoom_in`, `zoom_out`, `fit_screen`, `devices`, `terminal`, `delete`.
**How to avoid:** Before choosing icon names, verify they exist in the subset. If new icons are needed (e.g., `backup`, `history`, `key`, `description`), the subset woff2 must be regenerated to include them. The subset was previously expanded in Phase 4 (plan 04-01) -- the same process applies.
**Warning signs:** Icons render as empty space or a square fallback glyph.

### Pitfall 2: Sticky Header Background Bleed-Through
**What goes wrong:** When using `position: sticky` on `<thead>`, scrolled table rows show through the header because the header has a transparent or semi-transparent background.
**Why it happens:** Sticky positioning removes the element from normal scroll flow but keeps it visually in place -- without an opaque background, content scrolls visibly behind it.
**How to avoid:** Set an opaque `bg-bg` (the root background color) on the sticky `<thead>`. Do NOT use `bg-surface/50` or any alpha-transparent value.
**Warning signs:** Text from table rows visually overlapping with header text during scroll.

### Pitfall 3: Dashboard Not Receiving Areas
**What goes wrong:** Area filter dropdown and area column render nothing because Dashboard doesn't have access to area data.
**Why it happens:** Currently `App.tsx` passes only `devices={canvasDevices}` to Dashboard. The `areas` state exists in App.tsx but is not threaded down.
**How to avoid:** Add `areas` prop to `DashboardProps` and pass it from App.tsx: `<Dashboard devices={canvasDevices} areas={areas} />`.
**Warning signs:** Area filter dropdown shows no options; area column shows raw UUIDs instead of names/colors.

### Pitfall 4: Empty vs Loading State Ambiguity
**What goes wrong:** "No devices yet" CTA card shows when devices are still loading, creating a confusing flash of incorrect state.
**Why it happens:** Dashboard receives `devices: Device[]` from App.tsx. On initial load, this is `[]` before the first WebSocket snapshot arrives. The component cannot distinguish "no devices exist" from "devices haven't arrived yet."
**How to avoid:** Use a separate loading indicator. Options: (a) add a `loading` prop from App.tsx, (b) treat `devices.length === 0` as "loading" for the first 2-3 seconds then switch to empty state, (c) check if the WebSocket has delivered at least one snapshot. The current code already uses `devices.length === 0` as the loading condition -- this works because the app receives an initial snapshot very quickly via WebSocket, and once that arrives, `canvasDevices` is populated. The skeleton loading state (D-16) should show for the `devices.length === 0` case.
**Warning signs:** Users see "No devices yet" CTA for a split second before devices appear.

### Pitfall 5: Custom Select Dropdown Z-Index Conflicts
**What goes wrong:** Custom dropdown options render behind the SidePanel or NavigationPill.
**Why it happens:** The filter bar is in the normal document flow while SidePanel is `z-40` and NavigationPill is `z-30`.
**How to avoid:** Set dropdown menu z-index to `z-20` (below nav pill and side panel but above table content). The dropdown will naturally close when SidePanel opens because user interaction shifts.
**Warning signs:** Dropdown appears clipped or hidden behind other UI elements.

### Pitfall 6: Expanded Column Set Breaking Sort Logic
**What goes wrong:** Adding new sortable columns (Area, Vendor, Uptime, OS Version) without updating the `SortKey` type and sort comparison logic causes TypeScript errors or runtime failures.
**Why it happens:** Current `SortKey` is a union of `'hostname' | 'ip' | 'status' | 'hardware_model'`. New columns need new sort keys but some fields (area, uptime, OS version) require custom comparison logic (area needs name-based sort from lookup, uptime needs numeric sort).
**How to avoid:** Expand `SortKey` union, add column-specific sort comparators for non-string fields.
**Warning signs:** TypeScript compile errors on new column sort keys, or areas sorting by UUID instead of name.

## Code Examples

### Verified: Alternating Row Surfaces (D-03)
```typescript
// Already in DeviceRow.tsx -- this pattern is correct and should be kept
<tr className="[&:nth-child(even)]:bg-surface-high/30 hover:bg-elevated/50 transition-colors duration-150">
```

### Verified: Status Glow Pattern (from DeviceRow.tsx)
```typescript
// Current inline status colors -- should be replaced with StatusDot component import
const statusColors: Record<string, string> = {
  up: 'bg-status-up shadow-[0_0_8px_rgba(0,230,118,var(--nt-glow-shadow-opacity))]',
  down: 'bg-status-down shadow-[0_0_16px_rgba(255,23,68,var(--nt-glow-shadow-opacity))] animate-pulse',
  probing: 'bg-status-probing shadow-[0_0_12px_rgba(255,234,0,var(--nt-glow-shadow-opacity))] animate-pulse',
  unknown: 'bg-status-unknown shadow-[0_0_6px_rgba(158,158,158,var(--nt-glow-shadow-opacity))]',
};
// Replace with: import { StatusDot } from '../StatusDot';
// StatusDot already has this exact severity scaling built in
```

### Verified: Hub Empty State Pattern (from AreaHub.tsx)
```typescript
// D-15 should match this CTA card pattern
<div className="bg-surface border border-dashed border-outline rounded-xl p-6 flex flex-col items-center justify-center text-center min-h-[180px] transition-colors duration-200">
  <p className="text-on-bg font-semibold text-lg">No areas yet</p>
  <p className="text-on-bg-secondary text-sm mt-1">
    Create your first area in Settings
  </p>
  <button
    type="button"
    className="text-primary hover:text-primary/80 text-sm font-medium mt-3 transition-colors"
    onClick={onOpenSettings}
  >
    Open Settings
  </button>
</div>
```

### Verified: Area Color Dot Pattern (from NavigationPill.tsx)
```typescript
// D-11 area filter dropdown and table area column should use this pattern
import { adaptAreaColor } from '../contexts/ThemeContext';

<span
  className="w-2 h-2 rounded-full flex-shrink-0"
  style={{
    backgroundColor: adaptAreaColor(area.color, resolvedTheme),
    boxShadow: isActive ? `0 0 8px ${adaptAreaColor(area.color, resolvedTheme)}` : undefined,
  }}
/>
```

### Verified: Form Input Classes (from SSHCredentialForm.tsx)
```typescript
// Existing standardized input classes used across dashboard sub-panels
const inputClass =
  'w-full rounded-md border border-outline-subtle bg-elevated px-3 py-2 text-sm text-on-bg placeholder-on-bg-muted outline-none focus:border-primary focus:ring-1 focus:ring-primary/30';
const selectClass =
  'w-full rounded-md border border-outline-subtle bg-elevated px-3 py-2 text-sm text-on-bg outline-none focus:border-primary focus:ring-1 focus:ring-primary/30';
// These already use token-based classes and are consistent. Sub-panel restyling (COMP-08)
// may need minor tweaks but the pattern is sound.
```

### Verified: Device Display Name Logic (from DeviceRow.tsx)
```typescript
// Existing logic for determining display name -- should be consistent with DeviceCard
const displayName = device.tags?.display_name || device.sys_name || device.hostname || device.ip;
```

## State of the Art

| Old Approach | Current Approach | When Changed | Impact |
|--------------|------------------|--------------|--------|
| Native `<select>` dropdowns | Custom styled dropdowns | Phase 5 (this phase) | Consistent Neon Topography look for all form controls |
| Text-label action buttons | Material Symbols icon buttons with tooltips | Phase 5 (this phase) | Compact, professional action bar per row |
| Inline status dot styles | Imported StatusDot component | Phase 2 | Consistent glow severity across canvas and table |
| No area data in Dashboard | Areas threaded from App.tsx | Phase 5 (this phase) | Area filter dropdown and area column become possible |

## Open Questions

1. **Which Material Symbols icons are needed for action buttons?**
   - What we know: Current actions are SSH, Backup, Backup History, Config. Existing subset includes `settings`, `terminal`, `search`, `edit`, `link`, `hub`, `devices`, `close`, `delete`, `add`.
   - What's unclear: Whether icons like `key` (SSH), `backup`/`cloud_download` (Backup), `history` (History), `description`/`code` (Config) are in the subset.
   - Recommendation: Check the woff2 subset at build time. If icons are missing, regenerate the subset as was done in Phase 4 plan 04-01. Likely candidate icon names: `key` for SSH, `backup` for Backup, `history` for History, `description` for Config.

2. **How to distinguish "loading" from "truly empty"?**
   - What we know: Dashboard receives `devices: Device[]` from App.tsx. On mount, `canvasDevices` is `[]` until the first WebSocket snapshot. The current code shows "Loading devices..." for `devices.length === 0`.
   - What's unclear: Whether there is a guaranteed signal that "loading is complete."
   - Recommendation: Keep the current pattern -- show skeleton rows for `devices.length === 0` (loading state). Once devices arrive (even if zero real devices exist), the snapshot will contain `devices: []` with a populated message, and `canvasDevices` will have been set at least once. If needed, a `hasReceivedSnapshot` flag can be added to App.tsx. For now, the skeleton-then-empty transition is acceptable.

3. **Uptime and OS Version data availability**
   - What we know: D-02 specifies Uptime and OS version columns. The `Device` interface has `sys_descr` (which often contains OS info) but no dedicated `uptime` or `os_version` field. Uptime comes from WebSocket metric snapshots, not from the device REST response.
   - What's unclear: Whether to add uptime data to Dashboard (requires snapshot prop) or show only REST-available fields.
   - Recommendation: Uptime and OS version can be derived from existing data. `sys_descr` often contains OS version (e.g., "RouterOS 7.14.3"). For uptime, Dashboard would need access to the WebSocket snapshot (currently not passed to it). The planner should decide: either thread `snapshot` to Dashboard or defer live-metric columns to a future enhancement and show only REST-available data (Name, IP, Status, Area, Model, Vendor). The safest approach is to show columns that are available from the Device object and defer live-metric columns.

## Validation Architecture

### Test Framework
| Property | Value |
|----------|-------|
| Framework | Vitest 4.1 + @testing-library/react 16.3 |
| Config file | `frontend/vitest.config.ts` |
| Quick run command | `cd frontend && npx vitest run --reporter=verbose` |
| Full suite command | `cd frontend && npx vitest run` |

### Phase Requirements to Test Map
| Req ID | Behavior | Test Type | Automated Command | File Exists? |
|--------|----------|-----------|-------------------|-------------|
| COMP-07 | Dashboard renders with Neon Topography tokens, no hardcoded colors | unit | `cd frontend && npx vitest run src/components/Dashboard.test.tsx -x` | Yes (needs update) |
| COMP-07 | DeviceTable renders expanded columns with sorting | unit | `cd frontend && npx vitest run src/components/dashboard/DeviceTable.test.tsx -x` | No -- Wave 0 |
| COMP-07 | DeviceRow renders icon actions and area dot | unit | `cd frontend && npx vitest run src/components/dashboard/DeviceRow.test.tsx -x` | No -- Wave 0 |
| COMP-07 | Empty state CTA card renders when no devices | unit | `cd frontend && npx vitest run src/components/Dashboard.test.tsx -x` | Yes (needs update for new empty state) |
| COMP-07 | No-line rule -- no border-b classes on table elements | unit | `cd frontend && npx vitest run src/components/Dashboard.test.tsx -x` | Yes (existing test) |
| COMP-08 | SidePanel renders with restyled header and close icon | unit | `cd frontend && npx vitest run src/components/SidePanel.test.tsx -x` | No -- Wave 0 |
| COMP-09 | Sub-panel metric values use font-mono class | unit | `cd frontend && npx vitest run src/components/dashboard/BackupHistoryTable.test.tsx -x` | No -- Wave 0 |

### Sampling Rate
- **Per task commit:** `cd frontend && npx vitest run --reporter=verbose`
- **Per wave merge:** `cd frontend && npx vitest run`
- **Phase gate:** Full suite green before `/gsd:verify-work`

### Wave 0 Gaps
- [ ] `frontend/src/components/dashboard/DeviceTable.test.tsx` -- covers COMP-07 table structure
- [ ] `frontend/src/components/dashboard/DeviceRow.test.tsx` -- covers COMP-07 row rendering
- [ ] No new framework install needed -- Vitest + @testing-library/react already configured

## Sources

### Primary (HIGH confidence)
- Direct codebase inspection of all files listed in canonical references
- `frontend/src/index.css` -- CSS token definitions verified
- `frontend/src/components/Dashboard.tsx` -- current component structure verified
- `frontend/src/components/dashboard/DeviceTable.tsx` -- current sort logic verified
- `frontend/src/components/dashboard/DeviceRow.tsx` -- current row rendering verified
- `frontend/src/components/SidePanel.tsx` -- current panel chrome verified
- `frontend/src/components/StatusDot.tsx` -- reusable status dot component verified
- `frontend/src/components/MaterialIcon.tsx` -- icon component verified
- `frontend/src/components/icons/VendorIcon.tsx` -- vendor icon component verified
- `frontend/src/types/api.ts` -- Device interface with `area_id` field verified
- `frontend/src/App.tsx` -- area data flow and view switching verified
- `frontend/src/components/AreaHub.tsx` -- empty state CTA pattern verified
- `frontend/src/components/NavigationPill.tsx` -- area color dot pattern verified
- `frontend/src/contexts/ThemeContext.tsx` -- `adaptAreaColor` function verified

### Secondary (MEDIUM confidence)
- `.planning/DESIGN.md` -- Neon Topography design system specification
- `.planning/phases/02-component-restyling/02-CONTEXT.md` -- Phase 2 decisions on icons, glassmorphism, no-line rule
- `.planning/phases/04-area-hub-view-and-filtered-topology/04-CONTEXT.md` -- Phase 4 decisions on NavigationPill, area colors

## Metadata

**Confidence breakdown:**
- Standard stack: HIGH -- all libraries already installed and in use; no new dependencies
- Architecture: HIGH -- all files inspected directly, patterns verified from existing codebase
- Pitfalls: HIGH -- identified from direct code analysis of current component structure and data flow

**Research date:** 2026-03-26
**Valid until:** 2026-04-26 (stable -- no external dependencies changing)
