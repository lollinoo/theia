# Pitfalls Research

**Domain:** Network Topology Visualization with Real-Time Monitoring
**Researched:** 2026-03-05
**Confidence:** HIGH (multiple sources, well-documented problem domains)

## Critical Pitfalls

### Pitfall 1: GoSNMP Connection-Per-Device Architecture Neglected

**What goes wrong:**
Developers create a single GoSNMP client and attempt to poll multiple devices concurrently through it. GoSNMP connections are **not safe for parallel requests** on the same connection -- simultaneous reads on the socket cause responses to be received by the wrong goroutine, returning corrupted data. At 100+ devices, this manifests as intermittent wrong metric values, phantom interface flaps, and corrupted counter data that poisons graphs.

**Why it happens:**
GoSNMP's API looks like a simple client you can reuse. The documentation does not prominently warn about the single-connection concurrency limitation. Developers assume "Go is concurrent, the library must be too."

**How to avoid:**
Create one GoSNMP connection per device. Use a worker pool pattern with a bounded number of goroutines (e.g., 20-30 concurrent polls). Each goroutine gets its own GoSNMP client instance, polls its assigned device, and returns results through a channel. Reuse connections across poll cycles but never share them between concurrent polls.

```go
// WRONG: shared client
client := gosnmp.GoSNMP{Target: "..."}
for _, device := range devices {
    go poll(client, device) // corrupted results
}

// RIGHT: per-device client pool
pool := make(chan *gosnmp.GoSNMP, 30)
for _, device := range devices {
    go func(d Device) {
        client := <- pool // or create new
        client.Target = d.IP
        result := client.Get(oids)
        pool <- client
    }(device)
}
```

**Warning signs:**
- Intermittent wrong values for metrics that are correct when polled individually
- SNMP response timeout rates that increase with device count
- Metric values that "swap" between devices

**Phase to address:**
Phase 1 (Backend/SNMP foundation) -- this is architectural and cannot be retrofitted easily.

---

### Pitfall 2: Canvas Rendering Technology Lock-In with SVG

**What goes wrong:**
Developers start with SVG for the topology canvas because it is easier to develop, debug, and style (DOM elements, CSS, click handlers). At 100+ nodes with 200+ links, each with real-time metric labels updating every few seconds, SVG performance collapses. The browser spends all its time on DOM reconciliation and layout recalculation. FPS drops below 15, drag operations become laggy, and the UI feels broken.

**Why it happens:**
SVG works beautifully for 20-30 nodes. The performance cliff at 100+ elements is not gradual -- it hits suddenly. By the time you notice, substantial code is built around SVG assumptions (CSS styling, DOM event handlers, React component-per-node patterns).

**How to avoid:**
Use HTML5 Canvas 2D (not SVG, not WebGL) from day one. For 100-500 nodes, Canvas 2D delivers consistent 60 FPS without the complexity overhead of WebGL. Use a library like React Konva or Pixi.js that provides a React-friendly declarative API over Canvas. Reserve SVG only for static overlay elements like toolbar icons. Implement hit-testing manually for click/hover interactions on canvas elements.

Key benchmarks from research:
- SVG degrades below 30 FPS at ~3,000-5,000 elements (but with real-time text updates, the threshold drops to ~200-500 elements)
- Canvas 2D maintains 60 FPS for moderately large scenes (hundreds to low thousands of elements)
- WebGL is overkill for 100-500 nodes and adds GPU compatibility complexity

**Warning signs:**
- Prototype feels smooth with 10 test devices but janky with 50+
- React DevTools shows excessive re-renders on metric updates
- Drag-and-drop has visible lag even without metric updates running

**Phase to address:**
Phase 1 (Frontend foundation) -- rendering technology choice is the single hardest thing to change later.

---

### Pitfall 3: Prometheus Query Explosion Per Dashboard Load

**What goes wrong:**
The frontend fires individual PromQL queries per device per metric on every update cycle. With 100 devices and 5 metrics each (CPU, memory, uptime, temperature, interface throughput), that is 500+ HTTP requests to Prometheus every 15 seconds. Prometheus query engine bogs down, response times spike, and the browser's concurrent connection limit (6 per origin) creates a request queue that means some metrics are always stale by the time they render.

**Why it happens:**
The natural mental model is "for each device card, fetch its metrics." This maps cleanly to React component architecture (each DeviceCard fetches its own data). It works in dev with 5 devices but fails catastrophically at scale.

**How to avoid:**
Batch all metric queries server-side. The Go backend should execute a small number of aggregated PromQL queries that fetch metrics for ALL devices at once, then fan out results to the frontend via a single WebSocket message. Use queries like `node_cpu_seconds_total{instance=~"device1|device2|..."}` or better yet, use Prometheus recording rules to pre-aggregate topology dashboard metrics into dedicated series.

Target architecture:
- Backend polls Prometheus with 5-10 batched queries per cycle (one per metric type, matching all devices)
- Backend assembles per-device metric snapshots
- Backend pushes full state snapshot to frontend via WebSocket
- Frontend never queries Prometheus directly

**Warning signs:**
- Prometheus `/api/v1/query` endpoint showing high latency (>500ms)
- Browser network tab showing 100+ pending requests to backend
- Metrics on device cards update at visibly different times (staggered refresh)

**Phase to address:**
Phase 2 (Prometheus integration) -- must be designed as batch-first from the start.

---

### Pitfall 4: SNMP ifIndex Instability Across Reboots

**What goes wrong:**
The system uses SNMP ifIndex values as stable interface identifiers to correlate topology links and display per-interface stats. After a device reboot, firmware upgrade, or interface reconfiguration, ifIndex values change on some vendors. Links point to wrong interfaces, throughput stats show incorrect values, and the topology map displays phantom connections.

**Why it happens:**
ifIndex is guaranteed unique at any point in time but NOT guaranteed persistent across reboots on all vendors. MikroTik and some Cisco platforms re-assign ifIndex values. The `snmp ifindex persist` setting exists on Cisco IOS but is not enabled by default and does not exist on all platforms. Ubiquiti devices have their own ifIndex behavior.

**How to avoid:**
Never use ifIndex as a persistent identifier. Instead, use ifName or ifDescr as the primary interface identifier and store a mapping of `(device_ip, ifName) -> current_ifIndex` that gets refreshed on each poll cycle. When correlating interface data, always look up the current ifIndex for a named interface rather than caching old ifIndex values.

Additionally, implement interface discovery as a separate, less-frequent operation (every 5-10 minutes) from metric polling (every 15-60 seconds). This way, ifIndex remapping is detected and corrected without affecting metric continuity.

**Warning signs:**
- After a device reboot, link stats show zero or wildly different values
- Interface names in the UI don't match what the device reports
- Topology links appear between wrong port pairs

**Phase to address:**
Phase 1 (SNMP data model design) -- the interface identity model must be correct from day one.

---

### Pitfall 5: WebSocket Reconnection Causing State Desynchronization

**What goes wrong:**
After a network interruption, the WebSocket reconnects but the frontend state is stale. The client shows the last-known state from before the disconnect, which may be minutes old. Devices that went down during the disconnect still show "up." Metrics display frozen values. Users trust the stale data because there is no visible indicator that the view was interrupted.

**Why it happens:**
Most WebSocket implementations reconnect with exponential backoff (correct) but then only receive new delta updates going forward. If the server sends incremental updates ("device X metric changed to Y"), the client misses all changes during the disconnect window. Without a full state resync mechanism, the frontend accumulates drift.

**How to avoid:**
Implement a two-phase reconnection protocol:
1. On reconnect, server sends a full state snapshot (all devices, all current metrics, all link states)
2. After snapshot is applied, server resumes incremental updates
3. Frontend displays a visible "reconnecting..." banner during disconnect
4. Frontend displays "resyncing..." during snapshot application
5. Include a sequence number in each message; if the client detects a gap, it requests a full resync

Also implement server-side heartbeat pings (every 15-30 seconds). If the client misses 2-3 heartbeats, proactively trigger reconnection rather than waiting for the TCP timeout (which can take minutes).

**Warning signs:**
- Metrics freeze but no error is shown to the user
- Device status indicators show "up" for devices that have been down for minutes
- Users report "the map lied to me" after incidents

**Phase to address:**
Phase 2 (WebSocket real-time pipeline) -- reconnection logic must be part of the initial WebSocket implementation, not bolted on later.

---

### Pitfall 6: Force-Directed Layout Thrashing on Real-Time Updates

**What goes wrong:**
The auto-layout algorithm runs on every topology change (device added, link state change, new device discovered). When metrics update and a device status changes, the layout recalculates, causing all nodes to shift position. Users who have mentally mapped the topology are disoriented. Worse, if the layout runs continuously, nodes jitter and never settle, making the map unusable.

**Why it happens:**
Force-directed algorithms (Fruchterman-Reingold, d3-force) are iterative and non-deterministic. Different initial positions produce different final layouts. Running the algorithm on each update restarts convergence, and with real-time data causing frequent state changes, the layout never reaches equilibrium.

**How to avoid:**
Separate layout from live updates completely:
1. Auto-layout runs ONLY on explicit user action ("Auto-arrange" button) or on initial topology load
2. All node positions are persisted (saved to database) after any manual drag or auto-layout
3. Real-time metric/status updates NEVER trigger re-layout -- they only update colors, labels, and badges on nodes/links in their current positions
4. When new devices are added, place them at a default position (e.g., edge of canvas) without disturbing existing positions
5. Use a layout algorithm combination: Kamada-Kawai for initial rough placement, then Fruchterman-Reingold for local refinement

**Warning signs:**
- Nodes visibly move when metrics update
- Users complain they "can't find" devices because positions changed
- Layout takes >1 second to converge with 100+ nodes

**Phase to address:**
Phase 1 (Canvas/layout foundation) -- layout behavior is core UX and must be intentionally designed from the start.

---

## Technical Debt Patterns

| Shortcut | Immediate Benefit | Long-term Cost | When Acceptable |
|----------|-------------------|----------------|-----------------|
| Frontend directly queries Prometheus | Simpler architecture, fewer moving parts | 500+ requests per update at 100 devices, poor UX | Never for production; OK for a 5-device prototype |
| Storing topology as flat JSON file | No database dependency for MVP | No concurrent access safety, no migration path, no query capability | Only for initial prototype (< 1 week) |
| Using SNMPv2c instead of SNMPv3 | Simpler config, no auth/encryption overhead | Community strings sent in plaintext, security risk | Acceptable for internal-only deployments with network segmentation |
| Polling all OIDs in a single SNMP GET | Fewer SNMP requests per device | PDU size exceeds device limits on some vendors, causing silent failures | Never -- use GETBULK for tables, targeted GETs for scalars |
| Single WebSocket endpoint for all data | Simpler server implementation | Cannot independently control update rates for different data types (metrics vs topology vs alerts) | OK for MVP; split into channels/topics before scaling |

## Integration Gotchas

| Integration | Common Mistake | Correct Approach |
|-------------|----------------|------------------|
| Prometheus | Querying `rate()` over too-short windows for SNMP counter metrics (15s window on 60s scrape interval) | Use `rate()` window at least 4x the scrape interval (e.g., `rate(metric[5m])` for 60s scrape). Shorter windows produce gaps and spikes |
| Prometheus | Using `instance` label directly as device identifier | `instance` includes the port (e.g., `192.168.1.1:9116`). Strip port or use a relabel to add a clean `device` label |
| SNMP-Exporter | Assuming snmp-exporter returns data in the same label format as node-exporter | snmp-exporter uses OID-derived metric names and the `instance` is the target parameter, not the exporter's address. Requires careful relabeling |
| Grafana (link-out) | Hardcoding Grafana dashboard UIDs in topology links | Dashboard UIDs change on reimport. Use Grafana's `/d/SLUG/name` URL pattern with stable slugs, or look up UIDs via Grafana API at startup |
| GoSNMP | Using `Get()` for interface table walks | `Get()` retrieves single OIDs. Use `BulkWalkAll()` for table traversal (ifTable, ifXTable). `Get()` on a table OID returns nothing useful |

## Performance Traps

| Trap | Symptoms | Prevention | When It Breaks |
|------|----------|------------|----------------|
| React re-renders entire canvas on any metric update | FPS drops, visible jank when dragging nodes | Use `React.memo` aggressively, separate metric state from position state, update canvas via imperative draw calls not React reconciliation | >30 nodes with 5-second update interval |
| Unbounded WebSocket message size | Memory spikes, parse lag on client | Implement delta updates after initial full sync; compress with gzip; cap message size at 64KB | >100 devices with per-interface metrics |
| SNMP GETBULK with max-repetitions too high | Device agent crashes or returns truncated response | Set max-repetitions to 10-25 (not the default 50+). Some MikroTik devices choke on large GETBULK requests | Varies by vendor; MikroTik and older Cisco are sensitive |
| Prometheus query_range for dashboard metrics | Queries scan hours of historical data when you only need current values | Use instant queries (`/api/v1/query`) for dashboard, reserve `query_range` for trend graphs | >50 concurrent metric queries |
| Canvas redraw on every frame regardless of changes | CPU at 100% even when map is idle | Implement dirty-flag rendering: only redraw when state actually changes (metric update, user interaction, animation) | Always; this is a design flaw, not a scale issue |

## Security Mistakes

| Mistake | Risk | Prevention |
|---------|------|------------|
| Exposing SNMP community strings in frontend config or API responses | Attacker gains read (or write) access to all network devices | Store community strings server-side only. Frontend never sees SNMP credentials. Use environment variables or a secrets manager |
| Proxying Prometheus queries without sanitization | PromQL injection -- attacker crafts queries that DoS Prometheus or extract metrics from unrelated systems | Backend constructs all PromQL queries using parameterized templates. Never pass user input directly into PromQL strings |
| WebSocket endpoint without authentication | Anyone with network access can receive real-time network topology and metrics | Authenticate WebSocket upgrade requests with the same session/token used for HTTP. Validate on every connection, not just initial page load |
| SNMP write access enabled on devices | Topology tool accidentally (or maliciously via compromise) reconfigures network devices | Use SNMP read-only community strings. Never configure the poller with write access. Enforce at device ACL level |

## UX Pitfalls

| Pitfall | User Impact | Better Approach |
|---------|-------------|-----------------|
| Showing raw SNMP counter values instead of computed rates | Users see meaningless large numbers (e.g., "ifInOctets: 847293847293") | Always display rates (bits/sec, packets/sec) computed server-side. Never expose raw SNMP counters |
| No visual feedback during SNMP timeout | Device card shows stale "OK" status while device is unreachable | Show explicit "polling..." state, then "unreachable" after timeout. Timestamp the last successful poll visibly |
| Auto-layout on every map open | Users lose their carefully arranged positions | Persist positions. Only auto-layout on first view or explicit user request |
| Cramming all metrics onto the device card | Cards become unreadable at zoom levels needed for 100+ nodes | Show 2-3 key metrics on the card (status, CPU, hostname). Put details in a slide-out panel on click |
| Link lines overlapping in dense topologies | Cannot distinguish which link is which | Use curved/offset parallel links between same device pairs. Add hover highlight that dims other links |

## "Looks Done But Isn't" Checklist

- [ ] **SNMP Polling:** Often missing timeout handling per-device -- verify that one slow device does not block the entire poll cycle
- [ ] **WebSocket:** Often missing reconnection logic -- verify behavior after 30+ second network interruption
- [ ] **Prometheus Queries:** Often missing error handling for failed queries -- verify UI behavior when Prometheus is temporarily unreachable
- [ ] **Canvas Interactions:** Often missing zoom-to-fit and minimap -- verify usability when topology exceeds viewport at default zoom
- [ ] **Device Status:** Often missing "unknown/never-polled" state -- verify new devices don't show green/healthy before first successful poll
- [ ] **Link Stats:** Often missing counter wrap handling -- verify 32-bit counter rollover on high-bandwidth links doesn't show as negative throughput
- [ ] **Layout Persistence:** Often missing per-user position storage -- verify that one user's layout changes don't overwrite another's
- [ ] **Multi-vendor:** Often missing vendor-specific OID fallbacks -- verify that a device returning `noSuchObject` for a MikroTik-specific OID doesn't crash the poller

## Recovery Strategies

| Pitfall | Recovery Cost | Recovery Steps |
|---------|---------------|----------------|
| SVG-based canvas at 100+ nodes | HIGH | Full rewrite of rendering layer. If using React component-per-node, every node/link component needs rewriting. Budget 2-4 weeks |
| Frontend-direct Prometheus queries | MEDIUM | Add batch query endpoint to backend, refactor frontend data layer. Can be done incrementally per metric type. Budget 1-2 weeks |
| GoSNMP shared connection | MEDIUM | Refactor to connection-per-device pool. Polling logic stays same, only connection management changes. Budget 3-5 days |
| ifIndex as persistent ID | HIGH | Requires schema migration, re-mapping all existing link definitions, potential data loss for historical correlations. Budget 1-2 weeks |
| No WebSocket resync mechanism | MEDIUM | Add full-state snapshot endpoint, sequence numbering. Mostly server-side work. Budget 3-5 days |
| Layout thrashing from live updates | LOW | Separate layout trigger from data updates. Mostly configuration/flag changes. Budget 1-2 days |

## Pitfall-to-Phase Mapping

| Pitfall | Prevention Phase | Verification |
|---------|------------------|--------------|
| GoSNMP connection architecture | Phase 1: Backend foundation | Load test with 100 simulated SNMP targets, verify no data corruption |
| Canvas rendering technology | Phase 1: Frontend foundation | Render 150 nodes with 5-second metric updates, verify >30 FPS |
| ifIndex instability | Phase 1: Data model design | Simulate device reboot (change ifIndex values), verify link integrity maintained |
| Prometheus query explosion | Phase 2: Prometheus integration | Monitor Prometheus query count per dashboard load, verify <15 queries total |
| WebSocket reconnection | Phase 2: Real-time pipeline | Kill WebSocket mid-stream, verify full state recovery within 5 seconds of reconnect |
| Layout thrashing | Phase 1: Canvas/UX foundation | Change device status during map view, verify no node position changes |
| SNMP vendor quirks | Phase 3: Multi-vendor hardening | Poll at least one device per vendor (MikroTik, Cisco, Ubiquiti), verify consistent data |
| Prometheus cardinality | Phase 2: Prometheus integration | Check `prometheus_tsdb_head_series` after adding 100 devices, verify series count is bounded |
| SNMP community string exposure | Phase 1: API design | Audit all API responses for credential leakage before any deployment |
| Counter wrap handling | Phase 2: Metric computation | Simulate 32-bit counter wrap on a high-bandwidth interface, verify correct rate computation |

## Sources

- [GoSNMP multithreading issue #64](https://github.com/gosnmp/gosnmp/issues/64) - Confirms per-connection concurrency limitation (HIGH confidence)
- [GoSNMP OOM issue #401](https://github.com/gosnmp/gosnmp/issues/401) - Bad SNMP client can cause infinite loop/OOM (HIGH confidence)
- [SVG vs Canvas vs WebGL benchmarks 2025](https://www.svggenie.com/blog/svg-vs-canvas-vs-webgl-performance-2025) - Performance thresholds for rendering technologies (MEDIUM confidence)
- [SVG vs Canvas vs WebGL for Diagram Viewers](https://medium.com/@codetip.top/svg-vs-canvas-vs-webgl-for-diagram-viewers-tradeoffs-bottlenecks-and-how-to-measure-8cedbd3b7499) - Detailed tradeoff analysis (MEDIUM confidence)
- [Prometheus cardinality management](https://last9.io/blog/how-to-manage-high-cardinality-metrics-in-prometheus/) - Cardinality explosion causes and prevention (HIGH confidence)
- [Prometheus recording rules](https://prometheus.io/docs/prometheus/latest/configuration/recording_rules/) - Official docs on pre-aggregation (HIGH confidence)
- [Prometheus recording rules optimization](https://omarghader.github.io/prometheus-recording-rules-reduce-load-speed-queries/) - Practical recording rule patterns (MEDIUM confidence)
- [SNMP polling limits](https://news.networktigers.com/network-knowhow/snmp-and-the-limits-of-polling-based-network-monitoring/) - SNMP architectural limitations at scale (MEDIUM confidence)
- [Cisco SNMP polling delay](https://www.cisco.com/c/en/us/support/docs/interfaces-modules/catalyst-3850-fan-module/221136-snmp-polling-delay.html) - Vendor-specific SNMP agent limitations (HIGH confidence)
- [IBM SNMP timeout guidance](https://www.ibm.com/support/pages/increase-snmp-timeout-value-may-resolve-snmp-device-polling-timeout-rtm) - Timeout tuning for large responses (MEDIUM confidence)
- [SNMP ifIndex persistence](https://community.logicmonitor.com/discussions/product-discussions/network-interface-id-persistence-or-how-i-learned-to-stop-worrying-and-love-snmp/16060) - ifIndex instability across vendors (MEDIUM confidence)
- [Force-directed layout algorithms](https://cs.brown.edu/people/rtamassi/gdhandbook/chapters/force-directed.pdf) - Local minima and convergence issues (HIGH confidence)
- [Force-directed graph drawing](https://en.wikipedia.org/wiki/Force-directed_graph_drawing) - Algorithm limitations and initial position sensitivity (HIGH confidence)
- [WebSocket React best practices](https://ably.com/blog/websockets-react-tutorial) - Connection lifecycle management (MEDIUM confidence)
- [WebSocket reconnection patterns](https://maybe.works/blogs/react-websocket) - Reconnection and message queuing (MEDIUM confidence)
- [Prometheus slow query diagnosis](https://drdroid.io/stack-diagnosis/prometheus-slow-query-performance) - Query performance troubleshooting (MEDIUM confidence)
- [MikroTik SNMP documentation](https://help.mikrotik.com/docs/spaces/ROS/pages/8978519/SNMP) - MikroTik-specific SNMP behavior (HIGH confidence)
- [LibreNMS polling rate discussion](https://github.com/librenms/librenms/issues/1379) - Real-world polling interval constraints (MEDIUM confidence)

---
*Pitfalls research for: Network Topology Visualization (MikroTik Theia)*
*Researched: 2026-03-05*
