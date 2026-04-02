# Phase 9: Virtual Node Rendering - Context

**Gathered:** 2026-03-31
**Status:** Ready for planning

<domain>
## Phase Boundary

Render virtual device nodes on the ReactFlow canvas as compact cards with subtype-specific Material Symbol icons, status indicators for IP-bearing nodes, and adapted link edge labels showing real-interface-only metrics. Expand the Material Symbols font subset with the required glyphs.

Requirements: VIRT-06, VIRT-07, VIRT-08, VIRT-09, VIRT-14, VIRT-15.

</domain>

<decisions>
## Implementation Decisions

### Card Visual Identity
- **D-01:** Virtual node cards use dashed borders and muted background to visually distinguish them from physical device cards. Interactive and fully opaque (unlike ghost nodes which are semi-transparent and non-interactive).
- **D-02:** Same dashed border style as ghost nodes, but virtual nodes are fully opaque. The opacity difference (virtual = opaque, ghost = semi-transparent) is sufficient to distinguish them.
- **D-03:** Virtual nodes show status-based glow effects identical to physical devices — green (up), red (down), gray (unknown). Both IP-bearing and no-IP virtual nodes get glow treatment.

### Card Header Layout
- **D-04:** Virtual node cards use a centered vertical layout: subtype Material Symbol icon centered on top, display_name text below it. This differs from physical cards which use horizontal [VendorIcon] hostname [StatusDot] layout.
- **D-05:** Subtype icon size is 22-24px (medium). Larger than the default 18px Material Symbols but not dominant.
- **D-06:** Display name uses `font-mono` (same as physical card hostnames). Truncated with ellipsis if too long.
- **D-07:** For IP-bearing virtual nodes (200px card): StatusDot appears next to the display name, IP address line below in a body section.
- **D-08:** For no-IP virtual nodes (160px card): Icon and label only, no body section.

### Link Metric Labels
- **D-09:** Throughput labels on links to virtual nodes show rates only (↑ 1.2Mbps ↓ 3.4Mbps) without interface name. The user knows which device has the real interface.
- **D-10:** Bandwidth label shows the single real interface speed (e.g., "1Gbps") with no mismatch indicator (!). Only one side has negotiated speed data.

### Area Color and Glow
- **D-11:** Virtual nodes assigned to an area receive the same area gradient background as physical devices. Reinforces area grouping visually.

### Subtype Icon Mapping (from Phase 8)
- **D-12:** Internet → `language`, Cloud → `cloud`, Server → `dns`, Generic → `hub` (Material Symbols icon names). The `hub` icon is already in the subset; `language`, `cloud`, and `dns` must be added.

### Claude's Discretion
- Exact CSS values for dashed border pattern, muted background color, and opacity levels
- How to regenerate the Material Symbols woff2 subset with new glyphs (build tooling)
- Card component implementation — whether to branch within DeviceCard.tsx or extract a VirtualDeviceCard component
- Edge builder logic for detecting virtual links and adapting label computation
- ReactFlow node type registration for virtual devices

</decisions>

<canonical_refs>
## Canonical References

**Downstream agents MUST read these before planning or implementing.**

### Frontend Components
- `frontend/src/components/DeviceCard.tsx` — Current device card rendering, ghost node pattern (lines 62-87), DeviceNodeData interface (lines 13-25)
- `frontend/src/components/LinkEdge.tsx` — Link edge rendering, bandwidth/throughput labels (lines 191-230), mismatch indicator
- `frontend/src/components/StatusDot.tsx` — Status indicator with glow effects, StatusDotStatus type
- `frontend/src/components/MaterialIcon.tsx` — Material Symbols wrapper component
- `frontend/src/components/icons/VendorIcon.tsx` — Vendor icon mapping (physical devices)
- `frontend/src/components/icons/DeviceIcon.tsx` — DeviceType to SVG icon mapping

### Canvas Infrastructure
- `frontend/src/components/Canvas.tsx` — Canvas orchestration, ghost node creation (lines 150-188), area color injection
- `frontend/src/components/canvas/nodeBuilder.ts` — buildTopologyNodes() creates ReactFlow nodes from Device array
- `frontend/src/components/canvas/edgeBuilder.ts` — buildEdgeData() computes bandwidth/mismatch, buildTopologyEdges() builds edges

### Type System
- `frontend/src/types/api.ts` — Device interface (lines 35-54), DeviceType (line 1: currently missing 'virtual'), DeviceStatus
- `frontend/src/types/metrics.ts` — DeviceMetricsDTO, LinkMetricsDTO, AlertStatus

### Font Assets
- `frontend/src/index.css` — Material Symbols @font-face and CSS class (lines 197-222)
- `frontend/public/fonts/material-symbols-rounded-subset.woff2` — Current 26KB subset font

### Phase 8 Context (Backend decisions)
- `.planning/phases/08-virtual-device-backend/08-CONTEXT.md` — Virtual device data model decisions (D-01 through D-13)

</canonical_refs>

<code_context>
## Existing Code Insights

### Reusable Assets
- `DeviceCard.tsx` ghost node pattern (lines 62-87): Dashed border, muted colors, minimal content — directly reusable for virtual node visual identity
- `StatusDot.tsx`: Already handles 'unknown' status with gray glow — works for no-IP virtual nodes
- `MaterialIcon.tsx`: Ready to render subtype icons at custom sizes via `size` prop
- `buildEdgeData()` in edgeBuilder.ts: Already computes speedMismatch — add virtual link detection to suppress it

### Established Patterns
- DeviceType as union type in api.ts — extend with `'virtual'` literal
- Card width controlled by inline style in DeviceCard.tsx (currently 260px for regular, 120px for ghost)
- Area gradient background applied via `style` prop based on `areaColors` array
- Ghost node detection via `isGhost` flag in DeviceNodeData — similar flag pattern for virtual nodes

### Integration Points
- `nodeBuilder.ts` buildTopologyNodes(): Detect virtual devices, set card width and pass virtual flag
- `edgeBuilder.ts` buildEdgeData(): Detect when one device is virtual, suppress mismatch, use single-side metrics
- `DeviceCard.tsx`: Add virtual device rendering branch (dashed border, centered layout, subtype icon)
- `Canvas.tsx`: No changes expected — virtual devices flow through existing device array

</code_context>

<specifics>
## Specific Ideas

- Virtual cards should feel like "placeholders on the map" — present but visually secondary to physical infrastructure
- The centered icon layout gives virtual nodes a distinct silhouette on the canvas, making them instantly recognizable at any zoom level
- Ghost nodes and virtual nodes both use dashed borders but serve different purposes: ghost = cross-area reference (non-interactive), virtual = user-defined concept (fully interactive)

</specifics>

<deferred>
## Deferred Ideas

None — discussion stayed within phase scope.

</deferred>

---

*Phase: 09-virtual-node-rendering*
*Context gathered: 2026-03-31*
