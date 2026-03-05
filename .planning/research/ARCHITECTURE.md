# Architecture Research

**Domain:** Real-time network topology visualization (React + Go)
**Researched:** 2026-03-05
**Confidence:** HIGH

## Standard Architecture

### System Overview

```
┌─────────────────────────────────────────────────────────────────────┐
│                        FRONTEND (React)                             │
│  ┌──────────────┐  ┌──────────────┐  ┌──────────────┐              │
│  │  Topology     │  │  Device      │  │  Sidebar /   │              │
│  │  Canvas       │  │  Cards       │  │  Inspector   │              │
│  │  (React Flow) │  │  (Custom     │  │  Panel       │              │
│  │              │  │   Nodes)     │  │              │              │
│  └──────┬───────┘  └──────┬───────┘  └──────┬───────┘              │
│         │                 │                 │                       │
│  ┌──────┴─────────────────┴─────────────────┴───────┐              │
│  │              Zustand State Store                  │              │
│  │  (topology, metrics, selection, layout)           │              │
│  └──────────────────────┬────────────────────────────┘              │
│                         │                                          │
│  ┌──────────────────────┴────────────────────────────┐              │
│  │           WebSocket Client + REST Client           │              │
│  └──────────────────────┬────────────────────────────┘              │
├─────────────────────────┼───────────────────────────────────────────┤
│                         │ WS: metrics stream                       │
│                         │ HTTP: CRUD, topology, config              │
├─────────────────────────┼───────────────────────────────────────────┤
│                     BACKEND (Go)                                    │
│  ┌──────────────┐  ┌──────────────┐  ┌──────────────┐              │
│  │  REST API     │  │  WebSocket   │  │  SNMP        │              │
│  │  Server       │  │  Hub         │  │  Poller      │              │
│  │  (chi router) │  │  (gorilla/ws)│  │  (gosnmp)    │              │
│  └──────┬───────┘  └──────┬───────┘  └──────┬───────┘              │
│         │                 │                 │                       │
│  ┌──────┴─────────────────┴─────────────────┴───────┐              │
│  │              Core Domain Layer                    │              │
│  │  (device registry, topology graph, metric cache)  │              │
│  └──────┬───────────────────────────────┬────────────┘              │
│         │                               │                          │
│  ┌──────┴──────────┐           ┌────────┴─────────┐                │
│  │  SQLite Store    │           │  Prometheus       │                │
│  │  (device config, │           │  Client           │                │
│  │   positions,     │           │  (PromQL queries) │                │
│  │   layout)        │           │                   │                │
│  └─────────────────┘           └────────┬─────────┘                │
├─────────────────────────────────────────┼───────────────────────────┤
│                    EXTERNAL             │                           │
│  ┌──────────────┐  ┌──────────────┐  ┌──┴───────────┐              │
│  │  Network      │  │  Grafana     │  │  Prometheus  │              │
│  │  Devices      │  │  (link-out)  │  │  Server      │              │
│  │  (SNMP)       │  │              │  │              │              │
│  └──────────────┘  └──────────────┘  └──────────────┘              │
└─────────────────────────────────────────────────────────────────────┘
```

### Component Responsibilities

| Component | Responsibility | Typical Implementation |
|-----------|----------------|------------------------|
| **Topology Canvas** | Renders interactive node-link graph with pan/zoom/drag | React Flow (xyflow) with custom nodes |
| **Device Cards** | Custom React Flow nodes showing hostname, IP, status, live metrics | React.memo-wrapped custom node components |
| **Sidebar/Inspector** | Detailed device info, interface stats, routing info on selection | Standard React panel, reads from Zustand selection state |
| **Zustand Store** | All client-side state: nodes, edges, metrics, selection, UI | Zustand with sliced stores (topology, metrics, ui) |
| **WebSocket Client** | Receives real-time metric updates, connection status | Native WebSocket or reconnecting-websocket library |
| **REST Client** | CRUD for devices, topology layout save/load, configuration | fetch/axios with typed API client |
| **REST API Server** | HTTP endpoints for device CRUD, topology, configuration | go-chi/chi v5 router with middleware stack |
| **WebSocket Hub** | Broadcasts metric updates to all connected frontends | gorilla/websocket with hub pattern (connection registry) |
| **SNMP Poller** | Polls network devices for topology data (neighbors, interfaces) | gosnmp with worker pool goroutines |
| **Core Domain** | Device registry, topology graph model, metric cache | Pure Go domain types, no framework dependency |
| **SQLite Store** | Persists device configs, canvas positions, user preferences | mattn/go-sqlite3 or glebarez/go-sqlite |
| **Prometheus Client** | Queries Prometheus HTTP API for device metrics via PromQL | prometheus/client_golang api/v1 package |

## Recommended Project Structure

```
mikrotik-theia/
├── cmd/
│   └── theia/
│       └── main.go              # Entry point, wires everything together
├── internal/
│   ├── api/
│   │   ├── router.go            # chi router setup, middleware
│   │   ├── device_handler.go    # Device CRUD endpoints
│   │   ├── topology_handler.go  # Topology/layout endpoints
│   │   └── ws_handler.go        # WebSocket upgrade handler
│   ├── ws/
│   │   ├── hub.go               # Connection registry, broadcast
│   │   └── client.go            # Per-connection read/write goroutines
│   ├── poller/
│   │   ├── manager.go           # Manages poll schedules, worker pool
│   │   ├── worker.go            # Individual device poll logic
│   │   └── snmp.go              # SNMP get/walk wrappers (gosnmp)
│   ├── prom/
│   │   ├── client.go            # Prometheus HTTP API client
│   │   └── queries.go           # PromQL query templates for metrics
│   ├── domain/
│   │   ├── device.go            # Device model
│   │   ├── topology.go          # Topology graph model (nodes + edges)
│   │   ├── metrics.go           # Metric types and cache
│   │   └── interface.go         # Network interface model
│   ├── store/
│   │   ├── sqlite.go            # SQLite connection, migrations
│   │   ├── device_repo.go       # Device persistence
│   │   └── layout_repo.go       # Canvas layout persistence
│   └── config/
│       └── config.go            # App configuration (env/file)
├── web/                          # React frontend (Vite)
│   ├── package.json
│   ├── vite.config.ts
│   ├── tsconfig.json
│   ├── index.html
│   └── src/
│       ├── main.tsx
│       ├── App.tsx
│       ├── api/
│       │   ├── client.ts        # REST API client (typed)
│       │   └── ws.ts            # WebSocket connection manager
│       ├── stores/
│       │   ├── topologyStore.ts  # Nodes, edges, layout state
│       │   ├── metricsStore.ts   # Live metric values by device
│       │   └── uiStore.ts       # Selection, sidebar, theme
│       ├── components/
│       │   ├── Canvas/
│       │   │   ├── TopologyCanvas.tsx   # React Flow wrapper
│       │   │   ├── DeviceNode.tsx       # Custom node (memoized)
│       │   │   ├── LinkEdge.tsx         # Custom edge with throughput
│       │   │   └── CanvasControls.tsx   # Zoom, fit, layout buttons
│       │   ├── Sidebar/
│       │   │   ├── DeviceInspector.tsx  # Selected device details
│       │   │   ├── InterfaceList.tsx    # Interface stats table
│       │   │   └── RoutingInfo.tsx      # BGP/OSPF display
│       │   ├── Toolbar/
│       │   │   ├── AddDeviceDialog.tsx  # Manual device add form
│       │   │   └── SettingsPanel.tsx    # Global settings
│       │   └── common/
│       │       ├── StatusIndicator.tsx  # Green/yellow/red dot
│       │       └── MetricBadge.tsx      # CPU/mem/temp display
│       ├── hooks/
│       │   ├── useWebSocket.ts         # WS connection hook
│       │   ├── useDeviceMetrics.ts     # Subscribe to device metrics
│       │   └── useAutoLayout.ts        # Dagre layout trigger
│       ├── types/
│       │   └── index.ts               # Shared TypeScript types
│       └── utils/
│           ├── formatters.ts          # Bandwidth, uptime formatting
│           └── grafanaLinks.ts        # Grafana URL construction
├── Makefile                     # Build targets for both Go + React
├── Dockerfile                   # Multi-stage: build React, embed in Go
├── go.mod
├── go.sum
└── .planning/
```

### Structure Rationale

- **`cmd/theia/`:** Single binary entry point. Go convention for executable packages. Wires dependencies together (dependency injection via constructors, no DI framework).
- **`internal/`:** Go's built-in encapsulation. Nothing in `internal/` is importable by external packages. Keeps API surface clean.
- **`internal/api/`:** HTTP layer only. Handlers parse requests, call domain/services, return JSON. No business logic here.
- **`internal/ws/`:** Separated from REST because WebSocket lifecycle (long-lived connections, broadcast) is fundamentally different from request/response.
- **`internal/poller/`:** Isolated polling subsystem. Runs independently on its own goroutine pool. Communicates results via channels to the hub for broadcast.
- **`internal/prom/`:** Prometheus integration isolated behind a clean interface. Easy to mock in tests, easy to swap if metrics source changes.
- **`internal/domain/`:** Pure Go types with no external dependencies. The topology graph model, device model, and metric types live here. Everything else depends on domain; domain depends on nothing.
- **`internal/store/`:** Repository pattern over SQLite. Accepts and returns domain types. Database is an implementation detail.
- **`web/`:** Complete React app with its own package.json. Developed independently with `npm run dev` (Vite dev server with proxy to Go backend). Built output is embedded into the Go binary for single-binary deployment.
- **Zustand stores split by concern:** `topologyStore` owns React Flow nodes/edges. `metricsStore` owns live metric values (updated via WebSocket). `uiStore` owns selection, sidebar visibility, theme. This prevents metric updates from triggering topology re-renders.

## Architectural Patterns

### Pattern 1: Hub-and-Spoke WebSocket Broadcasting

**What:** A central Hub goroutine manages a registry of connected WebSocket clients. When new metric data arrives (from Prometheus queries or SNMP polls), the Hub broadcasts to all registered clients. Each client has dedicated read and write goroutines.

**When to use:** Always, for this system. Multiple browser tabs or operators may view the topology simultaneously.

**Trade-offs:** Simple to implement and reason about. Single Hub is not a bottleneck at this scale (tens of clients, not thousands). If needed later, can shard by topology/room.

**Example:**
```go
type Hub struct {
    clients    map[*Client]bool
    broadcast  chan []byte
    register   chan *Client
    unregister chan *Client
    mu         sync.RWMutex
}

func (h *Hub) Run() {
    for {
        select {
        case client := <-h.register:
            h.clients[client] = true
        case client := <-h.unregister:
            delete(h.clients, client)
            close(client.send)
        case message := <-h.broadcast:
            for client := range h.clients {
                select {
                case client.send <- message:
                default:
                    // Client buffer full, disconnect slow client
                    close(client.send)
                    delete(h.clients, client)
                }
            }
        }
    }
}
```

### Pattern 2: Worker Pool SNMP Poller

**What:** A fixed-size goroutine pool processes SNMP poll jobs from a channel. A scheduler goroutine enqueues poll jobs based on per-device intervals. Workers execute SNMP operations and push results to a results channel consumed by the domain layer.

**When to use:** For polling 100+ devices without spawning unbounded goroutines. SNMP is I/O-bound (network timeout-limited), so a pool of 20-50 workers handles 100+ devices comfortably.

**Trade-offs:** Fixed pool size prevents resource exhaustion. Trade-off is that with very aggressive polling intervals on many devices, jobs queue up. Mitigated by monitoring queue depth and adjusting pool size or intervals.

**Example:**
```go
type PollManager struct {
    devices   []Device
    jobCh     chan PollJob
    resultCh  chan PollResult
    workers   int
    ticker    map[string]*time.Ticker // per-device tickers
}

func (pm *PollManager) Start(ctx context.Context) {
    // Spawn worker pool
    for i := 0; i < pm.workers; i++ {
        go pm.worker(ctx)
    }
    // Schedule jobs based on device poll intervals
    for _, dev := range pm.devices {
        go pm.scheduleDevice(ctx, dev)
    }
}

func (pm *PollManager) worker(ctx context.Context) {
    for {
        select {
        case <-ctx.Done():
            return
        case job := <-pm.jobCh:
            result := pm.pollDevice(job)
            pm.resultCh <- result
        }
    }
}
```

### Pattern 3: Sliced Zustand Stores with Selective Subscriptions

**What:** Separate Zustand stores for topology (nodes/edges), metrics (live values), and UI (selection/panels). Components subscribe to only the slice they need. Metric updates (frequent, every few seconds) never cause topology or UI re-renders.

**When to use:** Always for real-time dashboards. The alternative (single store) causes cascading re-renders when metrics update at high frequency.

**Trade-offs:** Slightly more boilerplate (3 stores instead of 1). Trivial cost for significant performance gain. Cross-store reads are simple (import both stores where needed).

**Example:**
```typescript
// metricsStore.ts - updated by WebSocket, subscribed by DeviceNode
import { create } from 'zustand';

interface MetricsState {
  deviceMetrics: Record<string, DeviceMetrics>;
  updateMetrics: (deviceId: string, metrics: DeviceMetrics) => void;
}

export const useMetricsStore = create<MetricsState>((set) => ({
  deviceMetrics: {},
  updateMetrics: (deviceId, metrics) =>
    set((state) => ({
      deviceMetrics: { ...state.deviceMetrics, [deviceId]: metrics },
    })),
}));

// DeviceNode.tsx - only re-renders when THIS device's metrics change
const DeviceNode = React.memo(({ id, data }: NodeProps) => {
  const metrics = useMetricsStore(
    (state) => state.deviceMetrics[id],
    shallow
  );
  // render device card with metrics
});
```

## Data Flow

### Metric Data Flow (Real-Time)

```
Prometheus Server
    │ (HTTP API, PromQL queries every N seconds)
    v
Go Prometheus Client (internal/prom/)
    │ (query results: CPU, memory, throughput per device)
    v
Domain Layer (internal/domain/)
    │ (transforms to MetricUpdate messages, updates cache)
    v
WebSocket Hub (internal/ws/)
    │ (JSON broadcast to all connected clients)
    v
WebSocket Client (web/src/api/ws.ts)
    │ (parses message, dispatches to store)
    v
Zustand metricsStore
    │ (selective subscription via deviceId)
    v
DeviceNode Component (re-renders only changed devices)
```

### Topology Discovery Flow (On-Demand / Periodic)

```
Network Device (SNMP agent)
    │ (SNMP GetBulk: sysDescr, ifTable, lldpRemTable, ospfNbrTable)
    v
SNMP Poller Worker (internal/poller/)
    │ (parsed into domain types: interfaces, neighbors, routes)
    v
Domain Layer (internal/domain/)
    │ (updates device registry, rebuilds topology graph edges)
    v
SQLite Store (internal/store/)
    │ (persists device info, interface data)
    v
REST API / WebSocket
    │ (full topology on REST GET, incremental updates on WS)
    v
Frontend topologyStore
    │ (React Flow nodes and edges updated)
    v
TopologyCanvas (re-renders graph)
```

### User Interaction Flow

```
User clicks device on canvas
    │
    v
React Flow onNodeClick → uiStore.setSelectedDevice(id)
    │
    v
DeviceInspector subscribes to uiStore.selectedDevice
    │ (reads device data from topologyStore)
    │ (reads metrics from metricsStore)
    v
Sidebar renders full device details
    │
    v
User clicks "Open in Grafana" → window.open(grafanaUrl)
```

### Key Data Flows

1. **Metrics pipeline:** Prometheus -> Go backend (periodic PromQL queries) -> WebSocket broadcast -> Zustand metricsStore -> DeviceNode components. This is the hot path, running every 5-30 seconds per device. The Go backend queries Prometheus (not the other way around), keeping the frontend thin.

2. **Topology pipeline:** SNMP devices -> Go SNMP poller (on device add or periodic refresh) -> domain layer -> SQLite (persist) + WebSocket (notify) -> frontend topologyStore -> React Flow canvas. This runs less frequently (minutes, not seconds) and results in structural graph changes.

3. **Configuration pipeline:** Frontend form -> REST API -> Go handler -> SQLite store -> acknowledgment. Used for adding devices, saving positions, changing poll intervals. Standard request/response, no WebSocket involvement.

### State Management

```
┌─────────────────────────────────────────────────┐
│                  Zustand Stores                  │
│                                                 │
│  topologyStore          metricsStore   uiStore   │
│  ┌─────────────┐  ┌──────────────┐  ┌────────┐ │
│  │ nodes[]     │  │ deviceMetrics│  │selected│ │
│  │ edges[]     │  │ {id: {cpu,   │  │sidebar │ │
│  │ onNodesChg  │  │  mem, temp,  │  │theme   │ │
│  │ onEdgesChg  │  │  uptime}}    │  │addModal│ │
│  └──────┬──────┘  └──────┬───────┘  └───┬────┘ │
│         │                │              │       │
│  Canvas components  DeviceNode     Sidebar/UI   │
│  (structure changes) (metric updates) (clicks)  │
└─────────────────────────────────────────────────┘

Update sources:
  topologyStore ← REST API (initial load, device add/remove)
                ← WebSocket (topology change events)
                ← User drag (position updates, saved to backend)
  metricsStore  ← WebSocket only (continuous metric stream)
  uiStore       ← User interaction only (clicks, panel toggles)
```

## Scaling Considerations

| Scale | Architecture Adjustments |
|-------|--------------------------|
| 1-50 devices | Single Go binary, single SQLite file, one Prometheus query batch. Poll all devices in one pass. WebSocket broadcasts full metric snapshots. No optimization needed. |
| 50-200 devices | Batch PromQL queries (query per metric type, not per device). SNMP worker pool of 20-30. WebSocket sends delta updates (only changed metrics). Memoize all React Flow custom nodes. |
| 200-500 devices | Shard SNMP polling by device groups. Consider viewport-based rendering in React Flow (only render visible nodes). Add metric aggregation/downsampling for overview mode. Consider moving from SQLite to PostgreSQL if concurrent write pressure increases. |
| 500+ devices | Multiple topology maps/views (filter by site/region). Backend metric cache with TTL to reduce Prometheus query load. Consider server-side topology rendering for very large graphs. This is beyond v1 scope. |

### Scaling Priorities

1. **First bottleneck: SNMP polling throughput.** SNMP is slow (100-500ms per device roundtrip). With 100 devices at 30-second intervals, a pool of 10 workers handles this easily (100 * 0.5s / 10 workers = 5 seconds per cycle). Monitor: poll cycle completion time vs configured interval.

2. **Second bottleneck: React Flow rendering with many nodes.** At 100+ custom nodes with live-updating metrics, un-memoized components will cause frame drops. Fix: `React.memo` on all custom nodes, shallow equality on Zustand selectors, avoid complex CSS (shadows, gradients) on nodes.

3. **Third bottleneck: Prometheus query volume.** Querying per-device metrics individually does not scale. Fix: Use PromQL label matchers to batch queries (e.g., `node_cpu_seconds_total{instance=~"device1|device2|..."}`) and parse results server-side.

## Anti-Patterns

### Anti-Pattern 1: Frontend Queries Prometheus Directly

**What people do:** Have the React app call the Prometheus HTTP API directly from the browser.

**Why it's wrong:** Exposes Prometheus to the internet/users. PromQL queries are complex and device-specific -- the frontend should not need to know PromQL. CORS issues. No caching layer. Every browser tab independently hammers Prometheus.

**Do this instead:** Backend queries Prometheus on a schedule, caches results, broadcasts via WebSocket. Frontend receives pre-processed, display-ready metric objects.

### Anti-Pattern 2: One WebSocket Message Per Metric Per Device

**What people do:** Send individual WebSocket messages for each metric of each device (e.g., one message for device1.cpu, another for device1.memory).

**Why it's wrong:** At 100 devices with 5 metrics each, that is 500 messages per poll cycle. Message framing overhead dominates. Frontend processes 500 state updates instead of 1 batched update.

**Do this instead:** Batch all metric updates into a single JSON message per broadcast cycle: `{ "type": "metrics_update", "devices": { "dev1": {...}, "dev2": {...} } }`. Frontend applies the entire batch in one `setState` call.

### Anti-Pattern 3: Storing React Flow State in the Backend

**What people do:** Persist every node drag event to the backend in real-time, or store React Flow's internal state format in the database.

**Why it's wrong:** Massive write amplification during drag operations. Couples backend schema to React Flow's internal types. Database becomes bottleneck for UI interactions.

**Do this instead:** Store positions only on explicit "save layout" action (or debounced auto-save after drag ends). Store as simple `{deviceId, x, y}` records, not React Flow node objects. Reconstruct React Flow nodes from domain data + saved positions on load.

### Anti-Pattern 4: Unbounded Goroutines for SNMP Polling

**What people do:** Spawn a new goroutine per device per poll cycle without limits.

**Why it's wrong:** With 100+ devices, you get goroutine storms. SNMP connections consume file descriptors. If devices are unreachable, goroutines block on timeouts and pile up.

**Do this instead:** Fixed-size worker pool with a job channel. Workers reuse SNMP connections where possible. Context-based timeouts prevent goroutine leaks. Monitor active goroutine count.

## Integration Points

### External Services

| Service | Integration Pattern | Notes |
|---------|---------------------|-------|
| Prometheus | Go HTTP client querying `/api/v1/query` and `/api/v1/query_range` | Use `prometheus/client_golang/api/v1` package. Batch queries with label matchers. Query interval should match or be slower than scrape interval. |
| Grafana | URL construction only (link-out) | No API integration needed. Construct dashboard URLs with device-specific variables: `/d/dashboard-id?var-instance=device_ip`. Store dashboard URL templates per device type. |
| Network Devices | SNMP v2c/v3 via gosnmp | Each device needs: IP, community string (v2c) or credentials (v3), poll interval. Use GetBulk for interface tables, Walk for neighbor discovery. Timeout: 5s per request, 2 retries. |
| SNMP Exporter | Prometheus scrapes it; Theia reads from Prometheus | Theia does NOT replace snmp_exporter. snmp_exporter handles metric collection for Prometheus. Theia uses SNMP directly only for topology discovery (LLDP/CDP neighbors, interface details) that snmp_exporter does not expose. |

### Internal Boundaries

| Boundary | Communication | Notes |
|----------|---------------|-------|
| REST API <-> Domain | Direct function calls | Handlers call domain service methods. Domain returns typed results. Handler serializes to JSON. |
| SNMP Poller <-> Domain | Go channels (`chan PollResult`) | Poller is async. Results flow through a channel. Domain layer processes results and updates device registry. |
| Domain <-> WebSocket Hub | Go channel (`hub.broadcast <- message`) | Domain pushes serialized metric updates to the hub's broadcast channel. Hub fans out to clients. |
| Domain <-> SQLite Store | Repository interface | Domain defines `DeviceRepository` interface. Store implements it. Dependency points inward (store depends on domain, not reverse). |
| Domain <-> Prometheus Client | Direct function calls behind interface | `MetricsSource` interface with `QueryDeviceMetrics(ctx, deviceIDs)` method. Prometheus client implements it. Testable with mock. |
| Frontend REST <-> Backend API | HTTP JSON (OpenAPI-shaped) | Standard REST: `GET /api/devices`, `POST /api/devices`, `PUT /api/devices/:id/position`, `GET /api/topology`. |
| Frontend WS <-> Backend Hub | JSON over WebSocket | Message types: `metrics_update`, `topology_change`, `device_status`. Frontend dispatches to appropriate Zustand store based on type. |

## Build Order (Dependency Chain)

The components have clear dependency relationships that dictate build order:

```
Phase 1: Foundation
  domain/ (types, models) ← everything depends on this
  store/ (SQLite, device persistence) ← API needs this
  config/ ← everything reads config

Phase 2: Backend Services
  api/ (REST handlers) ← needs domain + store
  prom/ (Prometheus client) ← needs domain types
  poller/ (SNMP) ← needs domain + store
  ws/ (WebSocket hub) ← needs domain

Phase 3: Frontend Foundation
  Zustand stores ← mirrors domain types
  REST client ← needs backend API running
  Canvas with static nodes ← needs topology data

Phase 4: Real-Time Integration
  WebSocket client ← needs ws hub
  Metrics pipeline (Prom -> Hub -> Store -> Node) ← needs all layers
  SNMP poller integration ← needs poller + domain

Phase 5: Polish
  Auto-layout (Dagre) ← needs topology working
  Grafana link-out ← needs device metadata
  Visual alerts ← needs metrics + status logic
  Save/load layout ← needs positions API
```

This ordering ensures each phase produces a testable, demonstrable increment. Phase 1 and 2 can be tested with curl. Phase 3 shows a visible UI. Phase 4 brings it to life with real data. Phase 5 is polish that builds on a working system.

## Sources

- [gosnmp/gosnmp - Go SNMP library](https://github.com/gosnmp/gosnmp) - HIGH confidence
- [React Flow (xyflow) - Node-based UI library](https://reactflow.dev/) - HIGH confidence
- [React Flow Performance Guide](https://reactflow.dev/learn/advanced-use/performance) - HIGH confidence
- [go-chi/chi - Lightweight Go HTTP router](https://github.com/go-chi/chi) - HIGH confidence
- [gorilla/websocket - Go WebSocket library](https://github.com/gorilla/mux) - HIGH confidence
- [Zustand - React state management](https://github.com/pmndrs/zustand) - HIGH confidence
- [Prometheus HTTP API documentation](https://prometheus.io/docs/prometheus/latest/querying/api/) - HIGH confidence
- [prometheus/client_golang API package](https://pkg.go.dev/github.com/prometheus/client_golang/api/prometheus/v1) - HIGH confidence
- [mattn/go-sqlite3](https://github.com/mattn/go-sqlite3) - HIGH confidence
- [High-Concurrency IoT Data Collection with Golang SNMP Poller](https://www.ksolves.com/case-studies/big-data/high-concurrency-iot-data-collection) - MEDIUM confidence
- [Real-Time Systems: Go + React App](https://www.freecodecamp.org/news/real-time-systems-for-web-developers-from-theory-to-a-live-go-react-app/) - MEDIUM confidence
- [Go WebSocket Hub pattern](https://dev.to/jones_charles_ad50858dbc0/go-websocket-programming-build-real-time-apps-with-ease-1o57) - MEDIUM confidence
- [Zustand WebSocket integration discussion](https://github.com/pmndrs/zustand/discussions/1651) - MEDIUM confidence
- [Monorepo structure for React + Go backend](https://dev.to/ynwd/how-to-setup-golang-backend-and-react-frontend-in-a-monorepo-3api) - MEDIUM confidence

---
*Architecture research for: MikroTik Theia - Network Topology Visualizer*
*Researched: 2026-03-05*
