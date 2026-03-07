# Phase 2: Interactive Canvas - Context

**Gathered:** 2026-03-06
**Status:** Ready for planning

<domain>
## Phase Boundary

Operators can see their full network topology on an interactive dark-themed canvas with device cards, link lines, and persistent layout. Users can pan, zoom, drag devices, search by hostname/IP, and have positions persist across sessions. This is the first frontend phase — no frontend code exists yet.

**Deferred from this phase:** Background image upload (CANV-06) — implement in a future phase.

</domain>

<decisions>
## Implementation Decisions

### Canvas & Rendering
- React Flow for the graph/canvas library — built-in pan/zoom/drag, custom node rendering, edge routing
- Embedded SPA via go:embed — React app built and embedded in the Go binary for single-binary deployment
- Tailwind CSS for styling — utility-first, dark theme via class strategy
- Charcoal dark theme — background around #2d2d3d range, like Linear/Figma dark. Softer, easier on eyes for long NOC sessions

### Device Card Design
- Compact cards: type icon + hostname + IP + status dot
- Details available on hover or click in later phases — Phase 2 keeps cards minimal
- Filled geometric device type icons — bold silhouettes visible at small zoom levels (router, switch, AP, unknown)
- Status indicator: colored dot — green=up, red=down, yellow=probing, gray=unknown
- Click only selects the card (visual highlight) — no detail panel or popover in Phase 2

### Layout & Positioning
- Force-directed auto-layout algorithm for initial device positioning — connected devices cluster together naturally
- Device positions persisted in backend SQLite (new table + API endpoint) — syncs across browsers
- New/discovered devices auto-appear on canvas with auto-layout positioning — user can drag to reposition after

### Search & Navigation
- Top bar overlay with search bar (left) + zoom controls (right) — always visible above canvas
- As-you-type dropdown showing matching devices (hostname or IP) — like browser autocomplete
- Smooth pan + zoom animation to center on matched device with temporary highlight/pulse
- Zoom controls: zoom in, zoom out, fit-all buttons

### Claude's Discretion
- Exact charcoal color palette and accent colors
- React Flow configuration details (minimap, controls positioning)
- Loading/error states for API data fetching
- Debounce timing for search-as-you-type
- Force-directed layout library choice (dagre, elkjs, or d3-force)
- SQLite schema for position storage
- Build tooling setup (Vite configuration, TypeScript config)

</decisions>

<specifics>
## Specific Ideas

- Theme reference: Linear/Figma dark mode — charcoal, not pitch black
- Icon style: modernized Cisco-style network device silhouettes, filled geometric shapes
- Search behavior: similar to VS Code's top-bar search — always accessible, dropdown results

</specifics>

<code_context>
## Existing Code Insights

### Reusable Assets
- Domain models in `internal/domain/device.go`: Device struct with hostname, IP, device_type (router/switch/ap/unknown), status (up/down/probing/unknown), interfaces with speed data
- Domain models in `internal/domain/link.go`: Link struct with source/target device+interface pairs, discovery protocol
- REST API already serves JSON:API format at `/api/v1/devices` (list/CRUD) and `/api/v1/links` (list)
- Interface speed field (`Speed int64` in bits/sec) available for bandwidth capacity labels on links

### Established Patterns
- Go backend uses standard `net/http` (no framework) with middleware chain: CORS -> Logger -> JSON Content-Type
- JSON:API response format with `type`, `id`, `attributes`, `relationships` structure
- SQLite for persistence via `internal/repository/sqlite/` — migration pattern established in `migrations.go`
- Settings repo pattern exists (`internal/repository/sqlite/settings_repo.go`) — could inform position storage

### Integration Points
- New frontend must be served by the Go binary alongside existing `/api/v1/` routes
- Position persistence needs new SQLite table + migration + repository + API handler
- Frontend fetches device/link data from existing REST endpoints
- CORS middleware already configured for API access

</code_context>

<deferred>
## Deferred Ideas

- Background image upload (CANV-06) — deferred from this phase to keep scope focused on core canvas
- Device detail panel/popover on click — Phase 4 (Grafana integration)
- Keyboard shortcuts — Phase 4 (UX-03)

</deferred>

---

*Phase: 02-interactive-canvas*
*Context gathered: 2026-03-06*
