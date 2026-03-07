# Phase 3: Real-Time Pipeline - Context

**Gathered:** 2026-03-07
**Status:** Ready for planning

<domain>
## Phase Boundary

The topology map becomes live -- device cards display real-time metrics (CPU, memory, uptime, temperature) from Prometheus, links show live throughput with utilization-based color coding, and alert states are visually reflected on device cards and links. All metric data originates from an existing Prometheus instance via PromQL queries. Grafana deep-links, per-interface drill-down, and configurable polling are Phase 4.

</domain>

<decisions>
## Implementation Decisions

### Metrics on device cards
- Always-visible stats row in the card body showing CPU, MEM, TEMP, and UP
- Color-coded thresholds for metric values: green (<60%), yellow (60-85%), red (>85%)
- When no data available, show dashes: CPU --% | MEM --% | TEMP -- | UP --
- Temperature shows "N/A" for devices that don't report it
- Uptime uses human-readable short format: "14d 3h" or "2h 15m"
- Only Prometheus metrics in the stats row -- no interface counts or other static data

### Link throughput visualization
- Show both capacity label AND live TX/RX throughput on each link (two labels)
- 3-tier link color coding by utilization: green (<50%), yellow (50-80%), red (>80%)
- Utilization color based on max(TX, RX) -- single color per link, whichever direction is higher
- Links with no throughput data from Prometheus keep the current static gray (#4a4a5e)

### Alert visual treatment
- Device down: red pulsing border glow + dimmed card body content (similar to existing highlight ring but red)
- Device degraded: yellow/amber border glow -- warning state between normal and down
- Degraded state determined by Prometheus alerting rules (not app-defined thresholds)
- Link down: link turns red. Link degraded: handled by utilization color coding (yellow/red thresholds)
- Alert messages include severity levels (warning, critical) mapping to amber/red visual states

### Prometheus data flow
- Backend proxies all PromQL queries -- frontend never talks to Prometheus directly
- WebSocket for real-time push from backend to frontend
- Typed message channels: separate message types for `metrics`, `alert`, `link_metrics`
- Full state snapshot sent on WebSocket connect, then periodic typed updates
- Full state snapshots each push cycle (not deltas)
- Push frequency matches SNMP polling interval (configurable, default 60s)
- Standard prometheus-snmp-exporter metric names (snmp_*, ifHCInOctets, ifHCOutOctets, hrProcessorLoad, hrStorageUsed, etc.)
- Auto-reconnect with exponential backoff on WebSocket disconnection, with subtle "reconnecting..." indicator

### Metric staleness
- After timeout (2x polling interval), metric values revert to "--" dashes
- Stale metrics and device down are independent states -- a device can be "up" (SNMP reachable) with stale metrics (Prometheus issue)

### Prometheus Docker setup
- Add Prometheus + snmp_exporter as separate Docker services in docker-compose for dev
- Prometheus URL configurable for production use
- Separate snmp_exporter container (standard prometheus/snmp-exporter image)

### Claude's Discretion
- Whether to include mini progress bars or just numeric percentages for CPU/MEM
- Prometheus scrape target discovery method for dev environment (static or file-based)
- Exact stale timeout multiplier
- WebSocket reconnection backoff parameters
- Progress bar / gauge visual design details
- Loading skeleton design during initial WebSocket connection

</decisions>

<specifics>
## Specific Ideas

- Device cards should show metrics at a glance without hover/click -- the whole point is a live NOC-style view
- Link labels: capacity on top, live throughput below (two-line edge label)
- Degraded state uses the same ring/glow pattern as the current highlight ring but with amber color
- Down state uses the same ring/glow pattern but with red color and pulse animation

</specifics>

<code_context>
## Existing Code Insights

### Reusable Assets
- `StatusDot` component (`frontend/src/components/StatusDot.tsx`): Already handles up/down/probing/unknown with color mapping and glow shadows. Can be extended for degraded state.
- `DeviceCard` (`frontend/src/components/DeviceCard.tsx`): 260px wide card with header + body sections. Stats row goes in the body section below the IP line.
- `LinkEdge` (`frontend/src/components/LinkEdge.tsx`): Already has `bandwidthLabel` in EdgeLabelRenderer. Can add second label for live throughput and dynamic stroke color.
- `formatBandwidth()` in LinkEdge: Existing utility for formatting bps values -- reusable for throughput display.
- Background `Poller` (`internal/worker/poller.go`): Already runs on configurable interval with semaphore worker pool. Prometheus polling can follow the same pattern.

### Established Patterns
- **API client**: `frontend/src/api/client.ts` uses fetch + manual JSON parsing with type-safe parsers. WebSocket client will be a new pattern.
- **State management**: Canvas uses React `useState`/`useNodesState` from ReactFlow. Metrics state will need to merge into node data.
- **Dark theme**: Tailwind CSS with custom color tokens (bg-bg-canvas, text-text-primary, etc.). Metric colors should use existing status-* tokens where possible.
- **Domain types**: `internal/domain/device.go` has Device, Interface, DeviceStatus. May need new domain types for metrics and alerts.
- **Settings**: `internal/domain/settings.go` + settings repo already supports configurable polling interval.

### Integration Points
- `Canvas.tsx` `loadTopology()`: Currently fetches devices + links + positions. Will need to integrate WebSocket-driven metric updates into node/edge data.
- `DeviceNodeData` interface: Currently `{ device, pinned, highlighted }`. Needs metrics fields added.
- `LinkEdgeData` interface: Currently `{ link, bandwidthLabel, manual, parallelIndex }`. Needs throughput and utilization fields.
- `docker-compose.yml`: Add prometheus and snmp_exporter services alongside existing backend, frontend, and simulator containers.
- `cmd/theia/main.go`: WebSocket handler registration and Prometheus client initialization.

</code_context>

<deferred>
## Deferred Ideas

None -- discussion stayed within phase scope

</deferred>

---

*Phase: 03-real-time-pipeline*
*Context gathered: 2026-03-07*
