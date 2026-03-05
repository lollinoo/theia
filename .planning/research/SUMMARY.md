# Project Research Summary

**Project:** MikroTik Theia
**Domain:** Real-time network topology visualization with monitoring integration
**Researched:** 2026-03-05
**Confidence:** HIGH

## Executive Summary

MikroTik Theia is a web-based network topology visualizer that renders live device status and metrics on an interactive canvas, pulling data from Prometheus and enriching it with direct SNMP topology discovery. The expert approach for this class of tool is a Go backend that handles all data acquisition (Prometheus queries, SNMP polling) and pushes pre-processed metric snapshots to a React frontend over WebSocket. The frontend renders device nodes as rich React components on a React Flow canvas, with Zustand stores sliced by concern (topology, metrics, UI) to prevent metric updates from causing full-canvas re-renders. This architecture is well-documented and the recommended stack choices have high confidence.

The recommended stack is React 19 + TypeScript + Vite on the frontend with React Flow 12 for the topology canvas, Zustand 5 for state, and TanStack Query 5 for REST data. The backend is Go with chi v5 (HTTP router), gosnmp (SNMP polling), coder/websocket (WebSocket server), Prometheus client_golang (PromQL queries), and SQLite via modernc.org (persistence). Every choice is deliberate: React Flow was chosen over Cytoscape.js because device cards need to be rich React components with live metrics, not canvas-drawn primitives. Zustand was chosen over Redux because its selector pattern enables per-device subscriptions critical for 100+ node performance. SQLite was chosen over PostgreSQL because this is a single-user operator tool, not a multi-tenant SaaS.

The primary risks are: (1) GoSNMP connection-per-device requirement -- sharing connections causes silent data corruption, (2) Prometheus query explosion if metrics are fetched per-device instead of batched, (3) React Flow performance degradation if custom nodes are not properly memoized and metric state is not isolated, and (4) SNMP ifIndex instability across device reboots breaking link integrity. All four are preventable with correct upfront architecture but expensive to fix retroactively.

## Key Findings

### Recommended Stack

The stack splits cleanly between a Go backend optimized for concurrent I/O (SNMP polling, Prometheus queries, WebSocket management) and a React frontend optimized for rendering 100+ interactive device nodes with live-updating metrics. See `.planning/research/STACK.md` for full rationale and alternatives considered.

**Core technologies:**
- **React Flow 12 (@xyflow/react):** Network topology canvas -- renders nodes as actual React components, making rich device cards with live metrics trivial. The critical decision over Cytoscape.js (canvas-based, poor HTML node support)
- **Zustand 5:** Client state -- selector pattern enables per-device subscriptions so only the changed device re-renders on metric updates
- **TanStack Query 5:** Server state -- handles REST data fetching, caching, and background refetch for non-real-time data (topology config, device list)
- **Go + chi v5:** Backend HTTP server -- goroutines make concurrent SNMP polling trivial; chi stays close to net/http stdlib
- **gosnmp:** SNMP client -- the only serious Go SNMP library, supports v1/v2c/v3
- **coder/websocket:** WebSocket server -- modern, context-aware, actively maintained
- **SQLite (modernc.org):** Persistence -- zero-config, embedded, pure Go (no CGO), perfect for small dataset
- **Prometheus client_golang:** Metrics source -- official Go client for PromQL queries via HTTP API

### Expected Features

The MVP is substantial but well-scoped. See `.planning/research/FEATURES.md` for full prioritization matrix and competitor analysis.

**Must have (table stakes):**
- Interactive canvas with pan/zoom/drag and persistent layout
- Device cards showing hostname, IP, type icon, status indicator, live metrics (CPU, memory, uptime)
- Link visualization with bandwidth labels (TX/RX) and color-coded utilization
- Prometheus integration as the sole metrics data source
- Grafana deep-links from devices/metrics to existing dashboards
- Visual alerts reflecting Prometheus alert states
- Search to find devices on the canvas
- Dark theme UI
- Manual device addition (the only way to populate in v1)

**Should have (differentiators):**
- Modern web-native UX (dark theme, smooth animations) -- genuinely better than anything in the open-source NMS space
- Prometheus-native architecture -- no other topology tool is built Prometheus-first
- Per-interface statistics panel
- Routing protocol visualization (BGP/OSPF session status)
- Background images / floor plans
- Keyboard shortcuts for power users
- Link aggregation visualization (LAG/LACP)

**Defer (v2+):**
- Auto-discovery / subnet scanning (security implications, complexity)
- Built-in alerting (duplicates Alertmanager)
- Embedded Grafana panels (fragile iframes)
- Configuration management (separate domain)
- Multi-user / RBAC
- Component palette sidebar

### Architecture Approach

The system follows a three-tier architecture: React frontend, Go backend, and external services (Prometheus, network devices, Grafana). The backend acts as the sole data aggregator -- the frontend never contacts Prometheus or SNMP devices directly. Real-time metrics flow through a WebSocket hub using a hub-and-spoke broadcast pattern. SNMP polling uses a fixed-size worker pool to prevent goroutine storms. The domain layer is pure Go types with no external dependencies, and all persistence goes through a repository interface over SQLite. See `.planning/research/ARCHITECTURE.md` for component diagrams and data flow.

**Major components:**
1. **Topology Canvas (React Flow)** -- interactive node-link graph with custom device node components
2. **Zustand Stores (3 sliced stores)** -- topology (nodes/edges), metrics (live values by device), UI (selection/panels)
3. **WebSocket Hub (Go)** -- connection registry, broadcasts batched metric updates to all connected clients
4. **SNMP Poller (Go)** -- worker pool polling devices for topology data (LLDP neighbors, interfaces)
5. **Prometheus Client (Go)** -- batched PromQL queries for device metrics, results pushed through Hub
6. **REST API (chi)** -- device CRUD, topology layout save/load, configuration
7. **SQLite Store** -- persists devices, positions, connections, settings
8. **Domain Layer** -- pure Go types (Device, Interface, Topology, Metrics) depended on by everything, depends on nothing

### Critical Pitfalls

See `.planning/research/PITFALLS.md` for full analysis with code examples and recovery costs.

1. **GoSNMP connection sharing** -- GoSNMP is NOT safe for concurrent requests on the same connection. Create one connection per device, use a worker pool. Recovery cost if missed: MEDIUM (3-5 days refactor)
2. **Prometheus query explosion** -- Fetching metrics per-device-per-metric creates 500+ requests at 100 devices. Batch all queries server-side with label matchers. Recovery cost: MEDIUM (1-2 weeks)
3. **React Flow re-render cascade** -- Without memoized nodes and sliced stores, every metric update re-renders all 100+ nodes. Use React.memo + Zustand selectors with shallow equality. Recovery cost: MEDIUM (requires state architecture rework)
4. **SNMP ifIndex instability** -- ifIndex values change after device reboots on MikroTik and some Cisco platforms. Use ifName as persistent identifier, refresh ifIndex mapping each poll cycle. Recovery cost: HIGH (schema migration + data loss)
5. **WebSocket reconnection desync** -- After network interruption, client shows stale data with no indication. Implement full-state resync on reconnect + visible reconnecting banner. Recovery cost: MEDIUM (3-5 days)
6. **Layout thrashing** -- Auto-layout running on metric updates causes nodes to shift constantly. Layout must ONLY run on explicit user action, never on data updates. Recovery cost: LOW (1-2 days)

## Implications for Roadmap

Based on research, the project has clear dependency chains that dictate build order. The architecture document identifies 5 phases; the feature dependencies confirm this ordering. Here is the recommended phase structure:

### Phase 1: Foundation (Backend Domain + Data Layer)
**Rationale:** Everything depends on the domain model and persistence layer. Device, Interface, and Topology types must be defined first. SQLite schema and repository pattern establish the data foundation.
**Delivers:** Go project scaffold, domain types, SQLite persistence, configuration loading, device CRUD REST API (testable with curl)
**Addresses:** Manual device addition, persistent layout (data layer)
**Avoids:** ifIndex instability (design ifName-based identity from day one), SNMP credential exposure (server-side only from start)

### Phase 2: Frontend Foundation (Canvas + Static Topology)
**Rationale:** With the REST API available, the frontend can render a static topology. Getting React Flow, custom nodes, and Zustand stores right before adding real-time data prevents the re-render cascade pitfall.
**Delivers:** React/Vite project, React Flow canvas with custom DeviceNode and LinkEdge components, Zustand stores (topology, metrics, UI), REST client, device add dialog, dark theme, persistent layout (UI layer)
**Addresses:** Canvas pan/zoom/drag, device cards, link visualization, dark theme, search
**Avoids:** Canvas rendering lock-in (React Flow chosen deliberately), layout thrashing (positions persisted, no auto-layout on data changes)

### Phase 3: Real-Time Pipeline (WebSocket + Prometheus + SNMP)
**Rationale:** This is the highest-risk phase -- it integrates three async systems (Prometheus polling, SNMP discovery, WebSocket broadcast). Must be built on the stable foundation from Phases 1-2.
**Delivers:** WebSocket hub with reconnection/resync, Prometheus batched queries, SNMP worker pool poller, live metric updates on device cards, bandwidth on links, device status indicators
**Addresses:** Real-time metrics overlay, bandwidth/throughput on links, device status indicators, link status color-coding, visual alerts, Prometheus integration
**Avoids:** GoSNMP connection sharing (per-device connections), Prometheus query explosion (batched queries), WebSocket desync (full-state resync on reconnect)

### Phase 4: Integration + Polish
**Rationale:** With live data flowing, add the features that connect Theia to the broader monitoring ecosystem and improve usability.
**Delivers:** Grafana deep-links, click-to-detail inspector panel, per-interface statistics, configurable polling intervals, visual alert styling (pulsing/color), zoom-level-aware detail rendering
**Addresses:** Grafana deep-links, click-to-detail, per-interface stats, configurable refresh, visual alerts polish
**Avoids:** Grafana UID hardcoding (use stable slugs), raw counter display (show computed rates)

### Phase 5: Advanced Features
**Rationale:** Features that add significant value but require the core to be solid first. Each is independently valuable.
**Delivers:** Routing protocol visualization (BGP/OSPF), background images, link aggregation display, keyboard shortcuts, device grouping/tagging, sub-maps
**Addresses:** All P2 features from feature prioritization matrix
**Avoids:** Scope creep into v2 territory (auto-discovery, config management)

### Phase Ordering Rationale

- **Domain-first:** The architecture research shows the domain layer depends on nothing while everything depends on it. Building it first enables parallel frontend/backend work afterward.
- **Static before real-time:** The pitfalls research shows that real-time data integration is where most projects fail (query explosion, re-render cascades, connection corruption). Getting the rendering layer correct with static data first means real-time integration only needs to plug into working infrastructure.
- **Backend data pipeline is the riskiest integration:** Prometheus batching, SNMP worker pools, and WebSocket broadcasting are the most complex subsystems. Grouping them in Phase 3 allows focused attention on the pitfalls that matter most.
- **Polish after plumbing:** Grafana links, inspector panels, and interface stats are all straightforward once data flows correctly. They should not block core functionality.

### Research Flags

Phases likely needing deeper research during planning:
- **Phase 3 (Real-Time Pipeline):** Complex multi-system integration. Needs research on exact PromQL query patterns for snmp_exporter metrics, WebSocket message format design, and SNMP LLDP/CDP neighbor discovery OIDs per vendor.
- **Phase 5 (BGP/OSPF visualization):** Sparse documentation on BGP/OSPF metric availability in Prometheus. Depends on which exporters the user runs. Needs research on bgp_exporter and OSPF MIB availability.

Phases with standard patterns (skip research-phase):
- **Phase 1 (Foundation):** Standard Go project structure, SQLite CRUD, chi router setup. Well-documented patterns.
- **Phase 2 (Frontend Foundation):** React Flow custom nodes, Zustand stores, Vite setup. Extensive official documentation and examples.
- **Phase 4 (Integration + Polish):** Grafana URL construction, detail panels, configurable settings. Straightforward implementation.

## Confidence Assessment

| Area | Confidence | Notes |
|------|------------|-------|
| Stack | HIGH | All technologies are mature, actively maintained, and well-documented. React Flow 12 is the clear winner for rich-node topology. Every alternative was evaluated with specific rationale for rejection. |
| Features | HIGH | Competitor analysis covers 6 tools (The Dude, LibreNMS, Zabbix, PRTG, SolarWinds NTM, NetBox). Table stakes are well-established in the NMS domain. MVP scope is clear. |
| Architecture | HIGH | Patterns (Hub-and-spoke WS, worker pool SNMP, sliced Zustand stores) are well-documented with production examples. Data flows are clear with no ambiguous integration points. |
| Pitfalls | HIGH | Multiple sources confirm each pitfall. GoSNMP concurrency issue is documented in GitHub issues. SVG/Canvas performance thresholds are benchmarked. Prometheus query patterns are from official docs. |

**Overall confidence:** HIGH

### Gaps to Address

- **SNMP exporter metric naming conventions:** The exact Prometheus metric names and labels produced by snmp_exporter for MikroTik devices need validation during Phase 3 planning. Different snmp.yml generator configs produce different metric names.
- **React Flow 12 performance at 150+ nodes with live updates:** Benchmarks exist for static nodes but not for the specific pattern of frequent selective metric updates via Zustand selectors. Should be validated with a prototype early in Phase 2.
- **LLDP/CDP neighbor discovery completeness:** Whether SNMP-based LLDP/CDP discovery provides enough data to auto-populate topology links (without manual link creation) is vendor-dependent. May need manual link creation as fallback.
- **coder/websocket vs gorilla/websocket in practice:** The architecture doc references gorilla/websocket in the diagram but the stack recommends coder/websocket. This is a minor inconsistency -- coder/websocket is the correct choice per stack research.
- **Single-binary deployment with embedded frontend:** The approach of embedding Vite build output into the Go binary (via `go:embed`) needs validation for the specific asset pipeline (Tailwind CSS, React Flow CSS).

## Sources

### Primary (HIGH confidence)
- [React Flow official docs](https://reactflow.dev/) -- custom nodes, performance optimization, v12 API
- [GoSNMP GitHub issues #64, #401](https://github.com/gosnmp/gosnmp) -- concurrency limitations, OOM risks
- [Prometheus HTTP API docs](https://prometheus.io/docs/prometheus/latest/querying/api/) -- query and query_range endpoints
- [Prometheus recording rules](https://prometheus.io/docs/prometheus/latest/configuration/recording_rules/) -- pre-aggregation patterns
- [go-chi/chi v5](https://github.com/go-chi/chi) -- stdlib-compatible router
- [MikroTik SNMP documentation](https://help.mikrotik.com/docs/spaces/ROS/pages/8978519/SNMP) -- MikroTik-specific SNMP behavior
- [Force-directed layout algorithms (Brown CS)](https://cs.brown.edu/people/rtamassi/gdhandbook/chapters/force-directed.pdf) -- convergence and stability

### Secondary (MEDIUM confidence)
- [SVG vs Canvas vs WebGL benchmarks 2025](https://www.svggenie.com/blog/svg-vs-canvas-vs-webgl-performance-2025) -- rendering performance thresholds
- [Zustand WebSocket integration patterns](https://github.com/pmndrs/zustand/discussions/1651) -- real-time store update patterns
- [Go WebSocket Hub pattern](https://dev.to/jones_charles_ad50858dbc0/go-websocket-programming-build-real-time-apps-with-ease-1o57) -- hub-and-spoke implementation
- [LibreNMS, Zabbix, PRTG official docs](various) -- competitor feature analysis
- [Network visualization key features 2025](https://www.selector.ai/) -- industry feature expectations

---
*Research completed: 2026-03-05*
*Ready for roadmap: yes*
