# Technology Stack

**Project:** MikroTik Theia
**Researched:** 2026-03-05

## Recommended Stack

### Frontend Core

| Technology | Version | Purpose | Why | Confidence |
|------------|---------|---------|-----|------------|
| React | 19.x | UI framework | Already decided. React 19 is stable with concurrent rendering improvements | HIGH |
| TypeScript | 5.x | Type safety | Non-negotiable for a project with complex state (device models, metrics, topology graphs). Catches entire classes of bugs at compile time | HIGH |
| Vite | 6.x | Build tooling | Fast HMR, native ESM, simpler config than webpack. Industry standard for new React projects in 2025-2026 | HIGH |

### Graph Visualization

| Technology | Version | Purpose | Why | Confidence |
|------------|---------|---------|-----|------------|
| @xyflow/react (React Flow) | 12.x | Network topology canvas | **The critical decision.** React Flow renders nodes as actual DOM elements (React components), making rich device cards with live metrics, status indicators, and interactive elements trivial to build. Cytoscape.js is canvas-based and requires hacks for HTML content in nodes. At 100-200 nodes, React Flow performs well with built-in virtualization (`onlyRenderVisibleElements`). Memoized custom nodes maintain ~60 FPS at 100 nodes. | HIGH |

**Why NOT Cytoscape.js:** Cytoscape.js excels at raw graph performance (1000+ nodes) but renders to Canvas, not DOM. The project requires rich device cards showing hostname, IP, CPU/memory bars, status dots, temperature -- all as interactive React components. Cytoscape forces you to draw these as canvas primitives or use the `cytoscape.js-layers` plugin to overlay HTML, which is fragile and fights the library's design. React Flow was built for exactly this use case: node-based UIs with rich React component nodes.

**Why NOT D3.js:** D3 is a low-level visualization toolkit, not a graph UI framework. Building drag-and-drop, pan/zoom, edge routing, node selection, and connection handles from scratch in D3 would take months. React Flow provides all of this out of the box.

**Why NOT Reagraph:** WebGL-based (good for 3D/large graphs), but the project needs 2D topology with rich HTML nodes. Reagraph's node customization is limited compared to React Flow.

### State Management

| Technology | Version | Purpose | Why | Confidence |
|------------|---------|---------|-----|------------|
| Zustand | 5.x | Client state (UI state, selections, canvas viewport) | Lightweight, hook-based, no boilerplate. Perfect for real-time data where you need fine-grained subscriptions to avoid re-rendering 100+ nodes when one device updates. Zustand's selector pattern (`useStore(state => state.devices[id])`) means each device card only re-renders when its own data changes | HIGH |
| TanStack Query | 5.x | Server state (REST API calls, initial data loading) | Handles caching, refetching, background updates for REST endpoints. Use for topology config, device list, non-real-time data. Do NOT use for WebSocket streaming data -- that goes through Zustand | HIGH |

**Architecture:** WebSocket messages update Zustand store directly. Each device card subscribes to its own slice. TanStack Query handles initial page loads and config mutations. This separation prevents the "everything re-renders on every WebSocket message" problem.

### Real-Time Communication

| Technology | Version | Purpose | Why | Confidence |
|------------|---------|---------|-----|------------|
| Native WebSocket (browser) | - | Frontend WebSocket client | No library needed. The browser WebSocket API is sufficient. Wrap it in a custom hook with reconnection logic. Libraries like socket.io add unnecessary abstraction and protocol overhead for this use case | HIGH |

### UI & Styling

| Technology | Version | Purpose | Why | Confidence |
|------------|---------|---------|-----|------------|
| Tailwind CSS | 4.x | Styling | Utility-first CSS. Fast iteration on dark theme UI. No context switching between CSS files and components. Class-based approach works well with React Flow custom nodes | HIGH |
| Radix UI | latest | Accessible primitives (dropdowns, dialogs, tooltips) | Unstyled, composable, accessible. Pair with Tailwind for the dark theme. Avoids the visual weight of Material UI while providing solid interaction patterns | MEDIUM |
| Lucide React | latest | Icons (device types, status indicators) | Lightweight, tree-shakeable icon set. Good coverage of network-related icons. MIT licensed | MEDIUM |

### Backend Core

| Technology | Version | Purpose | Why | Confidence |
|------------|---------|---------|-----|------------|
| Go | 1.23+ | Backend language | Already decided. Excellent for concurrent SNMP polling, WebSocket management, and Prometheus API calls. goroutines make polling 100+ devices trivially concurrent | HIGH |
| chi | v5.2.x | HTTP router | Lightweight, idiomatic, fully net/http compatible. No framework lock-in. Supports Go 1.22 enhanced routing patterns. Composable middleware. Chi stays close to stdlib unlike Gin/Fiber which have their own abstractions | HIGH |

**Why NOT Gin:** Gin is popular but opinionated -- it wraps net/http with its own context type, making it harder to use stdlib-compatible middleware. Chi uses standard `http.Handler` and `http.HandlerFunc` throughout. For a project that needs WebSocket upgrades and custom middleware, staying close to stdlib is better.

**Why NOT Fiber:** Built on fasthttp (not net/http), which means incompatibility with the Go stdlib ecosystem. WebSocket libraries, Prometheus client, and other middleware expect net/http.

### Backend Libraries

| Technology | Version | Purpose | Why | Confidence |
|------------|---------|---------|-----|------------|
| gosnmp/gosnmp | v1.38.x | SNMP client (Get, GetBulk, Walk) | The only serious Go SNMP library. Community-maintained, supports SNMPv1/v2c/v3, IPv4/IPv6. Used by Prometheus snmp_exporter itself | HIGH |
| prometheus/client_golang | v1.x (api/prometheus/v1) | Query Prometheus via HTTP API | Official Prometheus Go client. Use the `api/prometheus/v1` package for PromQL queries (Query, QueryRange). Do NOT use the instrumentation package -- this tool queries Prometheus, it doesn't expose metrics | HIGH |
| coder/websocket | v1.x (nhooyr) | WebSocket server | Modern, minimal, idiomatic Go WebSocket library. Uses context.Context natively, supports concurrent writes, lighter API than gorilla/websocket. Now maintained by Coder (the company behind code-server) | MEDIUM |

**Why NOT gorilla/websocket:** gorilla/websocket works but has a larger, more complex API with multiple ways to do things. coder/websocket (formerly nhooyr.io/websocket) is more idiomatic, supports context cancellation natively, and is actively maintained by Coder. For new projects in 2025+, it is the better choice.

### Database

| Technology | Version | Purpose | Why | Confidence |
|------------|---------|---------|-----|------------|
| SQLite | via modernc.org/sqlite | Topology persistence (device positions, connections, config) | This is a single-user tool for network operators, not a multi-tenant SaaS. SQLite is perfect: zero config, single file, embedded in the Go binary, no external database server to manage. The topology data is small (devices, positions, connections, settings) -- well within SQLite's sweet spot | HIGH |

**Why modernc.org/sqlite over mattn/go-sqlite3:** Pure Go (no CGO), which simplifies cross-compilation and deployment. The performance difference is negligible for this use case (small dataset, low write frequency). Avoiding CGO means simpler Docker builds and no C compiler dependency.

**Why NOT PostgreSQL:** Massive overkill for persisting ~100 device records and their canvas positions. Adds deployment complexity (separate database server). The data model is simple key-value/relational -- no need for advanced query features, full-text search, or high-concurrency writes.

**Why NOT embedded key-value stores (BoltDB, BadgerDB):** The topology data has relational structure (devices have interfaces, interfaces connect to other interfaces, devices belong to groups). SQL is the natural query language for this. Key-value stores would require manual indexing and join logic.

### Supporting Go Libraries

| Library | Version | Purpose | When to Use | Confidence |
|---------|---------|---------|-------------|------------|
| slog (stdlib) | Go 1.21+ | Structured logging | All logging. Use stdlib slog, not zerolog/zap. Structured logging is in stdlib now | HIGH |
| golang.org/x/sync/errgroup | latest | Concurrent SNMP polling | Managing goroutine pools for device polling. Provides error propagation and context cancellation for groups of goroutines | HIGH |
| encoding/json (stdlib) | - | JSON serialization | API responses, WebSocket messages. Use stdlib, no need for jsoniter or easyjson at this scale | HIGH |

## Alternatives Considered

| Category | Recommended | Alternative | Why Not |
|----------|-------------|-------------|---------|
| Graph visualization | React Flow 12 | Cytoscape.js | Canvas rendering makes rich device cards difficult; requires HTML overlay hacks |
| Graph visualization | React Flow 12 | D3.js | Too low-level; would rebuild pan/zoom, drag-drop, edge routing from scratch |
| Graph visualization | React Flow 12 | Reagraph | WebGL/3D focused; limited HTML node customization |
| Graph visualization | React Flow 12 | vis-network | Older library, less active maintenance, canvas-based |
| State management | Zustand 5 | Redux Toolkit | Too much boilerplate for real-time updates; Zustand's selector pattern is simpler for per-device subscriptions |
| State management | Zustand 5 | Jotai | Atomic model is good but Zustand's single-store approach is simpler for the WebSocket update pattern |
| HTTP router | chi v5 | Gin | Custom context type fights stdlib compatibility |
| HTTP router | chi v5 | Fiber | Built on fasthttp, incompatible with net/http ecosystem |
| WebSocket | coder/websocket | gorilla/websocket | Larger API, no native context support, less idiomatic |
| Database | SQLite (modernc) | PostgreSQL | Deployment complexity for a simple data model |
| Database | SQLite (modernc) | mattn/go-sqlite3 | CGO dependency complicates builds |
| SNMP | gosnmp | net-snmp (CGO) | Pure Go preferred; gosnmp is feature-complete |
| CSS | Tailwind 4 | CSS Modules | Slower iteration; Tailwind utility classes pair well with component-per-node pattern |
| CSS | Tailwind 4 | styled-components | Runtime CSS-in-JS is being abandoned by the ecosystem; Tailwind is zero-runtime |

## Installation

```bash
# Frontend
npm create vite@latest frontend -- --template react-ts
cd frontend
npm install @xyflow/react zustand @tanstack/react-query
npm install tailwindcss @tailwindcss/vite
npm install @radix-ui/react-dialog @radix-ui/react-dropdown-menu @radix-ui/react-tooltip
npm install lucide-react

# Dev dependencies
npm install -D typescript @types/react @types/react-dom

# Backend (Go modules)
go mod init github.com/azmin/mikrotik-theia
go get github.com/go-chi/chi/v5
go get github.com/gosnmp/gosnmp
go get github.com/prometheus/client_golang/api/prometheus/v1
go get nhooyr.io/websocket
go get modernc.org/sqlite
```

## Version Pinning Notes

- **@xyflow/react**: Pin to `^12.x`. This is the rebranded package (was `reactflow`). Do NOT install the old `reactflow` package.
- **Zustand**: Pin to `^5.x`. Version 5 has breaking changes from v4 (better concurrent rendering support).
- **chi**: Use `github.com/go-chi/chi/v5` import path. v5 is the current major version.
- **coder/websocket**: Import as `nhooyr.io/websocket` (the package path hasn't changed despite the maintainer transfer to Coder).

## Sources

- [React Flow official site and docs](https://reactflow.dev/) - Custom nodes, performance optimization, v12 features
- [React Flow performance guide](https://reactflow.dev/learn/advanced-use/performance) - Virtualization, memoization benchmarks
- [xyflow/xyflow GitHub](https://github.com/xyflow/xyflow) - v12 release, Spring 2025 updates
- [Cytoscape.js](https://js.cytoscape.org) - Canvas rendering limitations for HTML nodes
- [Cytoscape.js WebGL Renderer Preview (Jan 2025)](https://blog.js.cytoscape.org/2025/01/13/webgl-preview/) - New WebGL renderer still canvas-based
- [cytoscape.js-layers plugin](https://github.com/sgratzl/cytoscape.js-layers) - HTML overlay workaround
- [gosnmp/gosnmp GitHub](https://github.com/gosnmp/gosnmp) - Community-maintained SNMP library
- [prometheus/client_golang](https://github.com/prometheus/client_golang) - Official Prometheus Go client
- [Prometheus API client docs](https://pkg.go.dev/github.com/prometheus/client_golang/api/prometheus/v1) - PromQL query API
- [coder/websocket GitHub](https://github.com/coder/websocket) - Maintained by Coder, formerly nhooyr.io/websocket
- [go-chi/chi GitHub](https://github.com/go-chi/chi) - v5.2.x, stdlib compatible router
- [modernc.org/sqlite](https://pkg.go.dev/modernc.org/sqlite) - Pure Go SQLite driver
- [Zustand GitHub](https://github.com/pmndrs/zustand) - v5.0.11, 20M+ weekly downloads
- [React state management in 2025](https://www.developerway.com/posts/react-state-management-2025) - Zustand as standard choice
- [Go web frameworks comparison 2025](https://blog.logrocket.com/top-go-frameworks-2025/) - Chi vs Gin vs Fiber analysis
