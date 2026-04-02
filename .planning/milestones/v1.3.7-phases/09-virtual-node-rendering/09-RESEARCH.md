# Phase 9: Virtual Node Rendering - Research

**Researched:** 2026-03-31
**Domain:** React/ReactFlow canvas rendering, Material Symbols font subsetting, TypeScript type extensions
**Confidence:** HIGH

## Summary

Phase 9 extends the existing ReactFlow canvas to render virtual device nodes as compact cards with subtype-specific Material Symbol icons, status indicators for IP-bearing nodes, and adapted link edge labels. The backend already supports virtual devices (Phase 8 complete) -- this phase is purely frontend rendering.

The work spans four concern areas: (1) extending the TypeScript type system to include `'virtual'` as a DeviceType, (2) adding a virtual card rendering branch inside DeviceCard.tsx with two variants (IP-bearing 200px and no-IP 160px), (3) adapting edgeBuilder.ts to detect virtual links and suppress speed mismatch indicators while using single-side metrics, and (4) regenerating the Material Symbols woff2 font subset to include `language`, `cloud`, and `dns` glyphs.

**Primary recommendation:** Implement as a branch within DeviceCard.tsx (not a separate component) following the existing `isGhost` branch pattern. Extend `DeviceNodeData` with `isVirtual` and `subtype` fields. Add virtual link detection in `buildEdgeData()` to suppress mismatch and use single-side bandwidth.

<user_constraints>
## User Constraints (from CONTEXT.md)

### Locked Decisions
- **D-01:** Virtual node cards use dashed borders and muted background to visually distinguish them from physical device cards. Interactive and fully opaque (unlike ghost nodes which are semi-transparent and non-interactive).
- **D-02:** Same dashed border style as ghost nodes, but virtual nodes are fully opaque. The opacity difference (virtual = opaque, ghost = semi-transparent) is sufficient to distinguish them.
- **D-03:** Virtual nodes show status-based glow effects identical to physical devices -- green (up), red (down), gray (unknown). Both IP-bearing and no-IP virtual nodes get glow treatment.
- **D-04:** Virtual node cards use a centered vertical layout: subtype Material Symbol icon centered on top, display_name text below it. This differs from physical cards which use horizontal [VendorIcon] hostname [StatusDot] layout.
- **D-05:** Subtype icon size is 22-24px (medium). Larger than the default 18px Material Symbols but not dominant.
- **D-06:** Display name uses `font-mono` (same as physical card hostnames). Truncated with ellipsis if too long.
- **D-07:** For IP-bearing virtual nodes (200px card): StatusDot appears next to the display name, IP address line below in a body section.
- **D-08:** For no-IP virtual nodes (160px card): Icon and label only, no body section.
- **D-09:** Throughput labels on links to virtual nodes show rates only (up 1.2Mbps down 3.4Mbps) without interface name. The user knows which device has the real interface.
- **D-10:** Bandwidth label shows the single real interface speed (e.g., "1Gbps") with no mismatch indicator (!). Only one side has negotiated speed data.
- **D-11:** Virtual nodes assigned to an area receive the same area gradient background as physical devices. Reinforces area grouping visually.
- **D-12:** Internet -> `language`, Cloud -> `cloud`, Server -> `dns`, Generic -> `hub` (Material Symbols icon names). The `hub` icon is already in the subset; `language`, `cloud`, and `dns` must be added.

### Claude's Discretion
- Exact CSS values for dashed border pattern, muted background color, and opacity levels
- How to regenerate the Material Symbols woff2 subset with new glyphs (build tooling)
- Card component implementation -- whether to branch within DeviceCard.tsx or extract a VirtualDeviceCard component
- Edge builder logic for detecting virtual links and adapting label computation
- ReactFlow node type registration for virtual devices

### Deferred Ideas (OUT OF SCOPE)
None -- discussion stayed within phase scope.
</user_constraints>

<phase_requirements>
## Phase Requirements

| ID | Description | Research Support |
|----|-------------|------------------|
| VIRT-06 | Virtual node renders as compact card with subtype-specific Material Symbol icon and display name | DeviceCard.tsx branch pattern (isGhost precedent), MaterialIcon component with `size` prop, subtype-to-icon mapping |
| VIRT-07 | Virtual node with IP shows StatusDot and IP address line (200px card) | StatusDot component already handles all statuses including 'unknown', card width set via inline style |
| VIRT-08 | Virtual node without IP shows icon and label only (160px card, no body) | IP-presence check via `device.ip` string truthiness, card width controlled by `w-[Npx]` Tailwind class |
| VIRT-09 | Material Symbols font subset expanded with language, cloud, and dns glyphs | pyftsubset available, build script pattern found in worktree, current font confirmed missing these 3 glyphs |
| VIRT-14 | Link to virtual node displays real interface tx/rx throughput on edge label | findLinkMetrics uses source_device_id + source_if_name -- works when real device is source; buildThroughputLabel format already matches D-09 |
| VIRT-15 | Link bandwidth label shows real interface speed only (no mismatch indicator) | buildEdgeData computes speedMismatch -- add virtual device detection to force speedMismatch=false and use single-side speed |
</phase_requirements>

## Standard Stack

### Core
| Library | Version | Purpose | Why Standard |
|---------|---------|---------|--------------|
| React | 18.3 | Component rendering | Already in use |
| @xyflow/react (ReactFlow) | 11.11 | Canvas node/edge rendering | Already in use |
| TypeScript | 5.7 | Type system for virtual device types | Already in use |
| Tailwind CSS | 3.4 (v4 token system) | Styling virtual card variants | Already in use |
| Material Symbols Rounded | Self-hosted woff2 subset | Subtype icons (language, cloud, dns, hub) | Already in use |

### Supporting
| Library | Version | Purpose | When to Use |
|---------|---------|---------|-------------|
| fonttools (pyftsubset) | 4.62.1 | Regenerate woff2 font subset | One-time build step for VIRT-09 |
| Vitest | 4.1.0 | Test runner for component tests | Testing virtual card rendering |
| @testing-library/react | 16.3 | Component test utilities | Testing virtual card rendering |

No new npm dependencies required. All rendering uses existing libraries.

## Architecture Patterns

### Recommended Implementation Structure

```
frontend/src/
  components/
    DeviceCard.tsx          # Add isVirtual branch (like isGhost)
    MaterialIcon.tsx        # No changes needed
    StatusDot.tsx           # No changes needed
    LinkEdge.tsx            # No changes needed (data-driven)
    canvas/
      nodeBuilder.ts        # Detect virtual devices, set width/flags
      edgeBuilder.ts        # Detect virtual links, suppress mismatch
      canvasHelpers.ts      # No changes needed
  types/
    api.ts                  # Extend DeviceType union, update parseDeviceType
  public/
    fonts/
      material-symbols-rounded-subset.woff2  # Regenerated with 3 new glyphs
  scripts/
    subset-material-icons.sh  # New build script for font subsetting
```

### Pattern 1: Virtual Card Branch in DeviceCard.tsx

**What:** Add an `isVirtual` early-return branch in `DeviceCardInner`, similar to the existing `isGhost` branch at lines 63-87.

**When to use:** When `data.isVirtual` is true.

**Example:**
```typescript
// Source: Existing pattern in DeviceCard.tsx lines 62-87
function DeviceCardInner({ data, selected }: NodeProps<DeviceNode>) {
  // Ghost node branch (existing)
  if (data.isGhost) {
    return (/* ghost card JSX */);
  }

  // Virtual node branch (new)
  if (data.isVirtual) {
    const hasIP = !!data.device.ip;
    const cardWidth = hasIP ? 200 : 160;
    const subtypeIcon = subtypeIconMap[data.subtype ?? ''] ?? 'hub';
    const label = data.device.tags?.display_name || data.device.sys_name || data.device.ip;
    const statusForDot = /* same logic as physical cards */;

    return (
      /* wrapper div with dashed border, glow, area colors */
      /* centered vertical layout: icon -> name [+ StatusDot] -> [IP body] */
    );
  }

  // Physical device card (existing, unchanged)
  // ...
}
```

**Why this pattern (not a separate component):** The DeviceCard is registered as the `'device'` node type in ReactFlow. Creating a separate VirtualDeviceCard would require registering a new node type and modifying nodeBuilder to use a different `type` value. The branch pattern is simpler, keeps one node type, and follows the established `isGhost` precedent. The memo comparator already exists and just needs extension.

### Pattern 2: Subtype Icon Mapping

**What:** A constant map from virtual subtype tag value to Material Symbol icon name.

**Example:**
```typescript
// Map from tags.virtual_subtype value to Material Symbol icon name
const subtypeIconMap: Record<string, string> = {
  internet: 'language',
  cloud: 'cloud',
  server: 'dns',
  generic: 'hub',
};
```

### Pattern 3: Virtual Link Detection in Edge Builder

**What:** Detect when a link connects to a virtual device and adapt bandwidth/mismatch computation.

**When to use:** In `buildEdgeData()` when computing `bandwidthLabel` and `speedMismatch`.

**Example:**
```typescript
// Source: edgeBuilder.ts buildEdgeData()
export function buildEdgeData(
  link: Link,
  devicesByID: Map<string, Device>,
  existingData?: LinkEdgeData,
  onContextMenu?: ...,
): LinkEdgeData {
  const sourceDevice = devicesByID.get(link.source_device_id);
  const targetDevice = devicesByID.get(link.target_device_id);

  // Detect virtual link: one side is virtual
  const sourceIsVirtual = sourceDevice?.device_type === 'virtual';
  const targetIsVirtual = targetDevice?.device_type === 'virtual';
  const isVirtualLink = sourceIsVirtual || targetIsVirtual;

  if (isVirtualLink) {
    // Use only the real device's interface speed (D-10)
    const realDevice = sourceIsVirtual ? targetDevice : sourceDevice;
    const realIfName = sourceIsVirtual ? link.target_if_name : link.source_if_name;
    const realInterface = realDevice?.interfaces.find(i => i.if_name === realIfName);
    const speed = realInterface?.speed && realInterface.speed > 0 ? realInterface.speed : 0;

    return {
      link,
      bandwidthLabel: speed > 0 ? formatBandwidth(speed) : undefined,
      speedMismatch: false,  // Never show mismatch for virtual links (D-10)
      // ... rest of data
    };
  }

  // Existing physical-physical logic unchanged
  // ...
}
```

### Pattern 4: DeviceNodeData Extension

**What:** Add `isVirtual` and `subtype` fields to the DeviceNodeData interface.

**Example:**
```typescript
export interface DeviceNodeData {
  device: Device;
  pinned: boolean;
  highlighted?: boolean;
  editMode?: boolean;
  metrics?: DeviceMetricsDTO | null;
  alertStatus?: AlertStatus;
  areaColors?: string[];
  onContextMenu?: (event: React.MouseEvent, deviceId: string) => void;
  isGhost?: boolean;
  onGhostClick?: (deviceId: string) => void;
  isVirtual?: boolean;    // NEW: virtual device rendering branch
  subtype?: string;       // NEW: Material Symbol icon name key
  [key: string]: unknown;
}
```

### Pattern 5: nodeBuilder.ts Virtual Detection

**What:** In `buildTopologyNodes()`, detect virtual devices and set the `isVirtual` flag and `subtype` in node data.

**Example:**
```typescript
// In buildTopologyNodes(), after building the node data object:
const isVirtual = device.device_type === 'virtual';

return {
  id: device.id,
  type: 'device',  // Same node type -- branch handled inside DeviceCard
  position: { x: position.x, y: position.y },
  data: {
    device: deviceData,
    pinned: saved?.pinned ?? false,
    highlighted: false,
    editMode,
    onContextMenu: openDeviceMenu,
    metrics: isVirtual ? null : nodeMetrics,  // Virtual devices have no device metrics
    alertStatus: pendingSnapshot
      ? alertStatusForDevice(device.id, pendingSnapshot.alerts)
      : undefined,
    isVirtual,
    subtype: isVirtual ? (deviceData.tags?.virtual_subtype ?? 'generic') : undefined,
  },
};
```

### Anti-Patterns to Avoid

- **Registering a new ReactFlow node type for virtual devices:** Adds unnecessary complexity when the branch pattern works. All devices flow through the same `'device'` type; the card component branches internally.
- **Duplicating the wrapper/glow/area-color logic:** The virtual card must use the same wrapper pattern as physical cards (same glow classes, area colors, selection ring). Extract if needed but do not duplicate.
- **Checking `device_type === 'virtual'` in LinkEdge.tsx:** The edge component is data-driven. Virtual link adaptation happens in `buildEdgeData()` which produces the data the edge consumes. LinkEdge.tsx itself should not need changes.
- **Hardcoding the throughput label format for virtual links:** The existing `buildThroughputLabel` function in canvasHelpers.ts already produces `TX: {rate} / RX: {rate}` format. This format satisfies D-09. Do not create a separate format function.

## Don't Hand-Roll

| Problem | Don't Build | Use Instead | Why |
|---------|-------------|-------------|-----|
| Font subsetting | Custom glyph extraction | `pyftsubset` from fonttools | Complex binary format; pyftsubset handles WOFF2, ligatures, variable font axes correctly |
| Icon rendering | Raw SVG for virtual icons | `MaterialIcon` component + woff2 font | Already self-hosted, tested, theme-aware; consistent with all other icons in the app |
| Status glow effects | New glow CSS for virtual | Existing StatusDot + wrapper glow pattern from DeviceCard.tsx | D-03 explicitly requires identical glow treatment |
| Throughput formatting | Custom label builder | Existing `buildThroughputLabel` / `compactThroughput` | Already handles null values, unit formatting, TX/RX labels |

**Key insight:** This phase is almost entirely about wiring existing components together in a new configuration. The rendering primitives (StatusDot, MaterialIcon, glow wrapper, edge labels) all exist. The work is integration and branching logic, not new primitives.

## Common Pitfalls

### Pitfall 1: Memo Comparator Not Updated for Virtual Fields
**What goes wrong:** Virtual node rerenders get suppressed because the DeviceCard memo comparator does not check `isVirtual` or `subtype` fields.
**Why it happens:** The custom `memo()` comparator at line 281 explicitly lists every field to compare. New fields default to "always equal" (referentially stable) and may not cause issues initially, but subtype changes or isVirtual toggles would be silently swallowed.
**How to avoid:** Add `pd.isVirtual === nd.isVirtual && pd.subtype === nd.subtype` to the memo comparator.
**Warning signs:** Virtual card doesn't update when device subtype changes via API.

### Pitfall 2: parseDeviceType Drops 'virtual' to 'unknown'
**What goes wrong:** The frontend type guard `parseDeviceType()` in `api.ts` does not include `'virtual'` in its switch statement, so backend responses with `device_type: "virtual"` are silently mapped to `'unknown'`.
**Why it happens:** The function was written before Phase 8 added the virtual type to the backend.
**How to avoid:** Add `case 'virtual': return value;` to `parseDeviceType()` and extend the `DeviceType` union.
**Warning signs:** All virtual devices render as "unknown" type on the canvas.

### Pitfall 3: findLinkMetrics Fails for Virtual-Source Links
**What goes wrong:** `findLinkMetrics()` in canvasHelpers.ts looks up metrics using `link.source_device_id` and `link.source_if_name`. If the virtual device is the source, there are no link metrics keyed by the virtual device ID (it has no interfaces being polled).
**Why it happens:** Link metrics are keyed by the device ID that has the physical interface. The WebSocket snapshot sends link metrics under the real device's ID.
**How to avoid:** When the source device is virtual, look up metrics using the target device ID and target interface name instead. Add a fallback in `findLinkMetrics` or in the edge builder where metrics are resolved.
**Warning signs:** Virtual links never show throughput labels despite the real device having active interface metrics.

### Pitfall 4: Font Subset Regeneration Breaks Existing Icons
**What goes wrong:** Running pyftsubset with an incomplete codepoint list removes existing glyphs that other components depend on.
**Why it happens:** The codepoint list must be comprehensive -- all 21+ existing glyphs plus the 3 new ones.
**How to avoid:** Use the complete codepoint list from the worktree's `subset-material-icons.sh` script which already includes all current glyphs plus the 3 new ones (language, cloud, dns).
**Warning signs:** Icons in Toolbar, ContextMenu, SidePanel, etc. render as blank squares after font regeneration.

### Pitfall 5: Virtual Card Wrapper Glow Missing for No-IP Nodes
**What goes wrong:** No-IP virtual nodes with status 'unknown' render without any glow, but D-03 says "Both IP-bearing and no-IP virtual nodes get glow treatment."
**Why it happens:** The wrapper glow is driven by `device.status`. For 'unknown' status, the existing glow logic shows no glow (it only glows for down/degraded/highlighted/selected). The StatusDot for 'unknown' shows a gray dot with subtle shadow.
**How to avoid:** This is actually correct behavior -- D-03 says virtual nodes get the "same" glow treatment as physical devices. Unknown status on physical devices also has no wrapper glow, only the gray StatusDot. The no-IP variant simply won't show a StatusDot. Document this explicitly.
**Warning signs:** None -- this is expected behavior.

### Pitfall 6: MiniMap nodeColor Doesn't Handle Virtual Devices
**What goes wrong:** The MiniMap's `nodeColor` callback in Canvas.tsx checks for `isGhost` but not `isVirtual`. Virtual nodes may get unexpected minimap colors.
**Why it happens:** The minimap uses device status to determine color. Virtual devices with status 'unknown' fall through to the default `statusColor()` which returns gray. This is acceptable but should be verified.
**How to avoid:** Verify the minimap rendering. If virtual devices with 'unknown' status show gray in minimap, this is correct. No code change likely needed, but should be tested.
**Warning signs:** Virtual nodes invisible or wrong color on minimap.

## Code Examples

### Virtual Card Rendering (IP-Bearing Variant)

```typescript
// Source: UI-SPEC Variant A, DeviceCard.tsx ghost pattern
// 200px card with header (icon + name + StatusDot) and body (IP)
<div
  className={`w-[${hasIP ? 200 : 160}px] rounded-[12px] border border-dashed border-outline-subtle
              bg-surface/80 text-center overflow-visible`}
  onContextMenu={(e) => {
    if (data.onContextMenu) {
      e.preventDefault();
      e.stopPropagation();
      data.onContextMenu(e, data.device.id);
    }
  }}
>
  {/* 4 connection handles (same as physical card) */}
  <Handle id="top" type="source" position={Position.Top} ... />
  <Handle id="right" type="source" position={Position.Right} ... />
  <Handle id="bottom" type="source" position={Position.Bottom} ... />
  <Handle id="left" type="source" position={Position.Left} ... />

  {/* HEADER SECTION */}
  <div className="flex flex-col items-center px-3 py-2">
    <MaterialIcon name={subtypeIcon} size={24} className="text-on-bg-secondary" />
    <div className="mt-1 flex items-center gap-1.5">
      <span className="font-mono text-[13px] font-semibold text-on-bg truncate max-w-full">
        {label}
      </span>
      {hasIP && <StatusDot status={statusForDot} />}
    </div>
  </div>

  {/* BODY SECTION (IP-bearing only) */}
  {hasIP && (
    <div className="rounded-b-[12px] bg-bg px-3 py-2">
      <div className="flex items-center justify-between">
        <span className="text-[11px] font-bold text-on-bg-secondary/70">IP:</span>
        <span className="font-mono text-[14px] font-bold text-on-bg">{data.device.ip}</span>
      </div>
    </div>
  )}
</div>
```

### DeviceType Union Extension

```typescript
// Source: frontend/src/types/api.ts line 1
// Before:
export type DeviceType = 'router' | 'switch' | 'ap' | 'unknown';

// After:
export type DeviceType = 'router' | 'switch' | 'ap' | 'virtual' | 'unknown';

// Also update parseDeviceType():
function parseDeviceType(value: unknown): DeviceType {
  switch (value) {
    case 'router':
    case 'switch':
    case 'ap':
    case 'virtual':   // NEW
      return value;
    default:
      return 'unknown';
  }
}
```

### Font Subset Build Script

```bash
#!/usr/bin/env bash
# Source: .claude/worktrees/agent-a8b66dae/frontend/scripts/subset-material-icons.sh
# Already includes language (U+E894), cloud (U+E2BD), dns (U+E875)
set -euo pipefail

UNICODES="U+E145,U+E250,U+E2BD,U+E326,U+E518,U+E51C,U+E5CD,U+E5CF,U+E7F5,U+E864,U+E873,U+E875,U+E894,U+E8B3,U+E8B6,U+E8B8,U+E8FF,U+E900,U+E92E,U+E9F4,U+EA10,U+EB8E,U+F097,U+F0BE,U+0020,U+005F,0061-007A"

# Download full font, subset with pyftsubset
pyftsubset "$FULL_FONT" \
  --output-file="$FONT_DIR/material-symbols-rounded-subset.woff2" \
  --flavor=woff2 \
  --unicodes="$UNICODES" \
  --layout-features='liga,clig' \
  --no-hinting \
  --desubroutinize
```

## State of the Art

| Old Approach | Current Approach | When Changed | Impact |
|--------------|------------------|--------------|--------|
| DeviceType has 4 values | DeviceType has 5 values (+ virtual) | Phase 8 backend | Frontend must update type union and parser |
| Font subset: 21 icons | Font subset: 24 icons (+ language, cloud, dns) | This phase | woff2 file ~1-2KB larger |
| All edges compute mismatch | Virtual edges skip mismatch | This phase | buildEdgeData gets virtual branch |
| DeviceCard has 2 branches | DeviceCard has 3 branches | This phase | isGhost, isVirtual, physical |

**Deprecated/outdated:**
- None for this phase. All existing patterns remain valid.

## Open Questions

1. **Throughput label direction for virtual links**
   - What we know: D-09 says "rates only" without interface name. The existing `buildThroughputLabel` produces `TX: {rate} / RX: {rate}`. This format does not include interface names.
   - What's unclear: Whether the TX/RX direction labels (from the real device's perspective) need to be shown differently when the virtual device is the source vs target of the link.
   - Recommendation: Use the existing `buildThroughputLabel` as-is. The format already matches D-09. The TX/RX values from the real interface's perspective are meaningful regardless of which end is virtual.

2. **DeviceCard width: Tailwind class vs inline style**
   - What we know: Physical cards use `w-[260px]` Tailwind class (line 144). Ghost cards use `w-[120px]` (line 68). The width needs to be dynamic for virtual cards (200px vs 160px based on IP).
   - What's unclear: Whether to use conditional Tailwind classes (`w-[200px]` / `w-[160px]`) or a dynamic inline `style={{ width }}`.
   - Recommendation: Use conditional Tailwind classes matching the existing pattern. The width is known at render time from `hasIP`.

## Environment Availability

| Dependency | Required By | Available | Version | Fallback |
|------------|------------|-----------|---------|----------|
| Node.js | Frontend build | Yes | 24.13.1 | -- |
| fonttools (pyftsubset) | VIRT-09 font subsetting | Yes | 4.62.1 | Google Fonts API URL with text parameter |
| Vitest | Test running | Yes | 4.1.0 | -- |

**Missing dependencies with no fallback:** None.

**Missing dependencies with fallback:** None -- all tools available.

## Validation Architecture

### Test Framework
| Property | Value |
|----------|-------|
| Framework | Vitest 4.1.0 + @testing-library/react 16.3 |
| Config file | `frontend/vitest.config.ts` |
| Quick run command | `cd frontend && npx vitest run --reporter=verbose` |
| Full suite command | `cd frontend && npx vitest run` |

### Phase Requirements -> Test Map
| Req ID | Behavior | Test Type | Automated Command | File Exists? |
|--------|----------|-----------|-------------------|-------------|
| VIRT-06 | Virtual node renders compact card with subtype icon and display name | unit | `cd frontend && npx vitest run src/components/DeviceCard.test.tsx -x` | Exists (needs virtual test cases) |
| VIRT-07 | Virtual node with IP shows StatusDot and IP line (200px card) | unit | `cd frontend && npx vitest run src/components/DeviceCard.test.tsx -x` | Exists (needs virtual test cases) |
| VIRT-08 | Virtual node without IP shows icon and label only (160px card) | unit | `cd frontend && npx vitest run src/components/DeviceCard.test.tsx -x` | Exists (needs virtual test cases) |
| VIRT-09 | Material Symbols font subset includes language, cloud, dns | unit | `cd frontend && npx vitest run src/components/MaterialIcon.test.tsx -x` | Exists (needs glyph verification test) |
| VIRT-14 | Link to virtual node displays real interface tx/rx throughput | unit | `cd frontend && npx vitest run src/components/LinkEdge.test.tsx -x` | Exists (needs virtual edge test cases) |
| VIRT-15 | Link bandwidth label shows real interface speed only (no mismatch) | unit | `cd frontend && npx vitest run src/components/canvas/edgeBuilder.test.ts -x` | Does NOT exist (Wave 0) |

### Sampling Rate
- **Per task commit:** `cd frontend && npx vitest run --reporter=verbose`
- **Per wave merge:** `cd frontend && npx vitest run`
- **Phase gate:** Full suite green before `/gsd:verify-work`

### Wave 0 Gaps
- [ ] `frontend/src/components/canvas/edgeBuilder.test.ts` -- covers VIRT-14, VIRT-15 (buildEdgeData virtual link tests)
- [ ] Add virtual device test cases to existing `DeviceCard.test.tsx` -- covers VIRT-06, VIRT-07, VIRT-08
- [ ] Add font glyph verification to `MaterialIcon.test.tsx` or new test -- covers VIRT-09

## Project Constraints (from CLAUDE.md)

- **TypeScript strict mode** -- all new code must pass strict type checking
- **Tailwind CSS for all styling** -- no separate CSS files for virtual card styles
- **Single quotes for strings** -- project convention
- **Relative imports only** -- no `@/` aliases
- **font-mono for monospaced text** -- JetBrains Mono Variable
- **Component props as explicit interfaces** -- extend DeviceNodeData properly
- **Named exports for hooks/utilities, default export for primary components** -- DeviceCard uses default export
- **Test files co-located** -- DeviceCard.test.tsx next to DeviceCard.tsx
- **Docker-containerized dev environment** -- font subset rebuild may need to run outside container if pyftsubset is host-only
- **GSD Workflow Enforcement** -- all edits through GSD commands

## Sources

### Primary (HIGH confidence)
- `frontend/src/components/DeviceCard.tsx` -- current card rendering, ghost pattern, memo comparator
- `frontend/src/components/LinkEdge.tsx` -- edge label rendering, bandwidth/throughput display
- `frontend/src/components/canvas/edgeBuilder.ts` -- buildEdgeData bandwidth/mismatch computation
- `frontend/src/components/canvas/nodeBuilder.ts` -- buildTopologyNodes node construction
- `frontend/src/components/canvas/canvasHelpers.ts` -- buildThroughputLabel, findLinkMetrics
- `frontend/src/types/api.ts` -- DeviceType union, parseDeviceType guard
- `frontend/src/components/MaterialIcon.tsx` -- icon rendering component
- `frontend/src/components/StatusDot.tsx` -- status indicator with glow
- `frontend/src/index.css` -- Material Symbols @font-face declaration
- `.planning/phases/09-virtual-node-rendering/09-CONTEXT.md` -- all locked decisions
- `.planning/phases/09-virtual-node-rendering/09-UI-SPEC.md` -- visual contract and CSS specs
- `.planning/phases/08-virtual-device-backend/08-CONTEXT.md` -- backend data model decisions
- `internal/domain/device.go` -- DeviceTypeVirtual constant confirmed

### Secondary (MEDIUM confidence)
- `.claude/worktrees/agent-a8b66dae/frontend/scripts/subset-material-icons.sh` -- font subset build script (in worktree, not in git, but pattern is verified)
- Font file verification via fonttools Python library -- confirmed 3 glyphs missing from current subset

### Tertiary (LOW confidence)
- None.

## Metadata

**Confidence breakdown:**
- Standard stack: HIGH -- no new dependencies, all existing libraries
- Architecture: HIGH -- patterns directly visible in codebase, branch pattern well-established
- Pitfalls: HIGH -- each pitfall identified from specific code paths with line numbers
- Font subsetting: HIGH -- pyftsubset available, codepoint list verified, build script pattern exists

**Research date:** 2026-03-31
**Valid until:** 2026-04-30 (stable -- no breaking changes expected in existing patterns)
