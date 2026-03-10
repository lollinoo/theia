# Project State

## Project Reference

See: .planning/PROJECT.md (updated 2026-03-05)

**Core value:** Network operators can see their entire topology at a glance with live stats on every device and link, and drill into Grafana for deep dives
**Current focus:** Phase 04.1 in progress — manually setup links using detected ports from SNMP

## Current Position

Phase: 04.1 of 5 (Manually Setup Links Using Detected Ports from SNMP)
Plan: 1 of 1 in current phase
Status: Completed
Last activity: 2026-03-10 — Phase 04.1, Plan 01 complete — Link CRUD API, interfaces endpoint, and LLDP/CDP auto-link creation
Progress: [██████████] 100% (Phase 0) -> [██████████] 100% (Phase 1) -> [██████████] 100% (Phase 2) -> [██████████] 100% (Phase 3) -> [██████████] 100% (Phase 4) -> [██████████] 100% (Phase 04.1)

## Performance Metrics

**Velocity:**
- Total plans completed: 13
- Average duration: not rigorously tracked
- Total execution time: several hours across phases

**By Phase:**

| Phase | Plans | Total | Avg/Plan |
|-------|-------|-------|----------|
| 0 | 2 | 2 | 1 |
| 1 | 3 | 3 | 4min |
| 2 | 4 | 4 | 9min |
| 3 | 4 | 4 | n/a |

**Recent Trend:**
- Last 5 plans: P2-4, P3-1, P3-2, P3-3, P3-4
- Trend: Phase 3 is complete and the project is ready to define Phase 4 work

*Updated after each plan completion*

## Accumulated Context

### Roadmap Evolution

- Docker environment promoted from Phase 1.1 (inserted) to Phase 0 (prerequisite for all phases)
- Phase 04.1 inserted after Phase 04: Manually setup the link using detected ports from SNMP (URGENT)

### Decisions

Decisions are logged in PROJECT.md Key Decisions table.
Recent decisions affecting current work:

- [Roadmap]: 5-phase structure following domain-first, static-before-realtime ordering per research recommendations
- [Roadmap]: Phase 5 (Routing Protocols) depends on Phase 3 (not Phase 4), enabling parallel work with Phase 4
- [Phase 1]: JSON serialization for SNMP credentials in SQLite
- [Phase 1]: ClientInterface abstraction for mock-based SNMP testing
- [Phase 1]: matchOIDColumn helper to prevent ambiguous OID prefix matching
- [Phase 1]: DiscoverFunc abstraction for simpler SNMP mock testing than raw client interface
- [Phase 1]: Re-fetch device from repo in async probe to avoid data races on shared pointer
- [Phase 1]: JSON:API response format with type/id/attributes/relationships
- [Phase 2]: Vite + React + Tailwind frontend running in Docker with API proxy to backend
- [Phase 2]: React Flow chosen for the topology canvas with custom node/edge components
- [Phase 2]: Device positions persisted in SQLite via `/api/v1/positions`
- [Phase 2]: Force-directed auto-layout implemented with `d3-force`
- [Phase 2]: Search overlay focuses devices by hostname/IP and zoom controls are fixed overlay actions
- [Phase 3]: Dev stack now includes Prometheus + snmp_exporter scraping simulator targets through relabeled Prometheus `/snmp` requests
- [Phase 3]: Prometheus HTTP API is queried through a lightweight `PromClient` that manually parses JSON responses
- [Phase 3]: Link utilization prefers `ifHighSpeed` when available and falls back to `ifSpeed`
- [Phase 3]: `/api/v1/ws` must bypass the standard JSON/logger middleware chain because WebSocket upgrade requires an unhijacked `ResponseWriter`
- [Phase 3]: Metrics collector caches and broadcasts full `snapshot` payloads, and also serves the cached snapshot immediately on WebSocket connect
- [Phase 3]: Frontend metric parsing and formatting are centralized in `frontend/src/types/metrics.ts` so Canvas can merge snapshot data without duplicating display logic
- [Phase 3]: Device cards use an always-visible numeric CPU/MEM/TEMP/UP stats row with red/amber alert glow states taking priority over selection/highlight styling
- [Phase 3]: Link edges support a second live-throughput label and utilization-driven stroke colors while preserving existing manual-edge context menu behavior
- [Phase 3]: Canvas overlays WebSocket snapshot metrics onto existing React Flow nodes/edges without re-fetching topology, and stale metrics are cleared after a 2x polling-interval timeout
- [Phase 3]: The dev frontend’s live WebSocket path is proxied through the Vite dev server at `/api/v1/ws`; the runtime config file actually loaded by the container is `frontend/vite.config.js`
- [Phase 3]: The frontend dev container only bind-mounts `src` and `index.html`, so Vite config changes require a frontend image rebuild before runtime verification
- [Phase 4]: Device onContextMenu callback is passed through node.data (not component props) because React Flow's NodeProps doesn't support arbitrary prop pass-through
- [Phase 4]: ContextMenu repositions after initial render (render offscreen, measure, clamp) to handle dynamic menu heights
- [Phase 4]: Escape key priority order for Canvas: context menu > side panel > search overlay > shortcut help
- [Phase 4]: SidePanel uses translate-x CSS transform (always mounted) so exit animation plays; conditional render would skip it
- [Phase 4]: panelContent state pattern (type + optional data) drives SidePanel; later plans fill specific panel types
- [Phase 4]: LinkDown alert severity is warning (not critical) — a link down is less severe than a device becoming entirely unreachable
- [Phase 4]: Link alert status uses best-effort interface name matching via alert summary string (Prometheus labels vary by exporter config)
- [Phase 4]: Background image rendered as z-index 0 positioned div with 0.15 opacity behind React Flow so topology nodes/links remain readable
- [Phase 4]: SNMP API payload uses nested snmp: { version, community } object matching backend JSON:API design; flat snmp_community/snmp_version fields were wrong
- [Phase 4]: Device display_name stored in tags map (not a top-level field) consistent with backend domain.Device.Tags design
- [Phase 4]: Background image feature removed per user request — base64 approach was problematic; server-side upload would be needed if re-added
- [Phase 4]: Grafana deep-link opens per-device configured URL if set (grafana_dashboard_url:<id> setting), then global URL; hostname-slug URL generation removed
- [Phase 4]: Ctrl+N keyboard shortcut changed to plain A — Ctrl+N is reserved by browsers for new window
- [Phase 04.1]: Only source_if_name and target_if_name editable via PUT link; changing devices requires delete + recreate
- [Phase 04.1]: Interface filter excludes names starting with "lo", "Null", or empty; up-first then alpha sort
- [Phase 04.1]: in_use annotation derived from GetByDeviceID result set (no dedicated DB column)
- [Phase 04.1]: GetBySysName added to DeviceRepository for O(1) LLDP neighbor resolution
- [Phase 04.1]: LLDP auto-link skips neighbors where RemoteSysName not found; both devices must exist

### Pending Todos

- Phase 5 (Routing Protocols) planning and execution
- Frontend link management UI (Phase 04.1 plan 02 or later)

### Blockers/Concerns

- Dev simulators do not currently expose ENTITY-SENSOR temperature series, so temperature will remain nil / `N/A` unless a device reports it.

## Session Continuity

Last session: 2026-03-10
Stopped at: Phase 04.1, Plan 01 complete — backend Link CRUD API, interfaces endpoint, and LLDP auto-link creation
Resume file: .planning/phases/04.1-manually-setup-the-link-using-detected-ports-from-snmp/ (next plan in phase)
