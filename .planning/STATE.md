# Project State

## Project Reference

See: .planning/PROJECT.md (updated 2026-03-05)

**Core value:** Network operators can see their entire topology at a glance with live stats on every device and link, and drill into Grafana for deep dives
**Current focus:** Phase 3 implementation complete; awaiting explicit human approval for Plan 04

## Current Position

Phase: 3 of 5 (Real-Time Pipeline)
Plan: 4 of 4 in current phase
Status: Phase 3 In Review
Last activity: 2026-03-07 — Phase 3, Plan 04 implemented and automated verification passed
Progress: [██████████] 100% (Phase 0) -> [██████████] 100% (Phase 1) -> [██████████] 100% (Phase 2) -> [██████████] 100% (Phase 3)

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
- Trend: Phase 3 real-time pipeline is implemented end to end and waiting on explicit human sign-off

*Updated after each plan completion*

## Accumulated Context

### Roadmap Evolution

- Docker environment promoted from Phase 1.1 (inserted) to Phase 0 (prerequisite for all phases)

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

### Pending Todos

- Human approval for `03-04-PLAN.md`: user browser confirmation / explicit `approved`
- Decide whether to begin Phase 4 after human approval

### Blockers/Concerns

- Dev simulators do not currently expose ENTITY-SENSOR temperature series, so temperature will remain nil / `N/A` unless a device reports it.
- Prometheus alert transport is wired, but the dev Prometheus config still has no alerting rules, so alert snapshots are empty for now.
- The plan’s blocking human verification step is still open because automated checks passed but the user has not yet explicitly approved the phase.

## Session Continuity

Last session: 2026-03-07
Stopped at: Phase 3 Plan 04 implemented, proxy/runtime verified, awaiting human approval
Resume file: .planning/phases/03-real-time-pipeline/03-04-SUMMARY.md
