# Phase 1: Foundation - Context

**Gathered:** 2026-03-05
**Status:** Ready for planning

<domain>
## Phase Boundary

Go backend delivering device management through a REST API with persistent SQLite storage and SNMP connectivity. Covers device CRUD (add, edit, delete), SNMP probing (sysName, sysDescr, interfaces, LLDP/CDP neighbors), and topology link persistence. Does NOT include the frontend, real-time metrics, Prometheus integration, or canvas rendering.

</domain>

<decisions>
## Implementation Decisions

### API Design
- JSON:API response format with type, id, attributes, relationships
- HTTP status codes for errors (400, 404, 500) with `{"error": "message", "details": ...}` body
- Versioned endpoints under `/api/v1/`
- No pagination or filtering for v1 — simple list-all endpoints
- Async device creation: POST /devices returns immediately with status "probing", SNMP probe runs in background
- Fully async batch add: POST /devices/batch returns batch ID, each device probes in background, poll batch status endpoint
- Health endpoint at /api/v1/health reporting component status (db, snmp_poller, prometheus reachability)
- No authentication for v1 — single-user tool on trusted network
- WebSocket is a separate concern — REST does not reference WS endpoints

### Device Data Model
- UUID as primary key for devices (IP and hostname are attributes)
- SNMP credentials stored as plaintext in SQLite (acceptable for v1 single-user threat model)
- Device type (Router, Switch, AP) auto-detected from SNMP sysObjectID/sysDescr with manual user override
- Topology links stored in DB (source device, source interface, target device, target interface), refreshed from SNMP but not lost when SNMP is down
- Full interface table per device (name, ifIndex, speed, admin/oper status) — not just linked interfaces
- Canvas positions (x, y) NOT included in Phase 1 schema — Phase 2 adds them
- Key-value tags per device for user-defined grouping (e.g., site: "datacenter-1", role: "core")
- Bootstrap config via YAML file (listen address, DB path), runtime settings (Prometheus URL, polling interval, Grafana URL) stored in SQLite settings table and editable via API

### SNMP Interaction
- Support SNMPv2c (community string) and SNMPv3 (username/password + encryption)
- Full discovery on initial device add: sysName, sysDescr, sysObjectID, all interfaces (ifTable), LLDP/CDP neighbors
- Timeout handling: retry 2-3 times with increasing timeout, then mark device as "down" in DB, keep retrying on schedule
- Background scheduled polling (runs continuously on configurable interval)
- Unknown LLDP/CDP neighbors shown as placeholder "discovered but unmanaged" devices — user can promote to managed by adding credentials
- Configurable SNMP worker pool size via settings
- ifName as persistent interface identifier (stable across reboots), ifIndex refreshed each poll cycle
- Direct neighbor discovery only — no multi-hop topology building

### Project Structure
- Standard Go layout: cmd/, internal/, pkg/
- YAML config file with environment variable overrides (Docker-friendly)
- Frontend and backend as separate services (no go:embed)
- SQLite database path configurable, defaulting to ./data/theia.db

### Claude's Discretion
- Exact YAML config file structure and key naming
- SQLite schema migration strategy (embed migrations or auto-create)
- Exact retry timing/backoff for SNMP timeouts
- SNMP OID selection for device type auto-detection
- Internal package organization within internal/
- Batch operation concurrency limits
- Health endpoint response structure details

</decisions>

<specifics>
## Specific Ideas

- Docker deployment is a priority — config must work well with docker-compose (env var overrides, volume mounts for DB)
- Placeholder devices for unknown neighbors should be visually distinct when they reach the canvas (Phase 2 concern, but data model must support the "unmanaged" state)
- The settings table pattern (file for bootstrap, DB for runtime) means the app can start with just a config file and Prometheus/Grafana URLs can be configured through the eventual UI

</specifics>

<code_context>
## Existing Code Insights

### Reusable Assets
- None — greenfield project, no existing code

### Established Patterns
- None — this phase establishes all patterns for the project

### Integration Points
- External SNMP devices (network routers/switches/APs)
- Future Prometheus instance (URL stored in settings, not used in Phase 1)
- Future Grafana instance (URL stored in settings, not used in Phase 1)
- Future React frontend consumes the REST API (Phase 2)

</code_context>

<deferred>
## Deferred Ideas

None — discussion stayed within phase scope

</deferred>

---

*Phase: 01-foundation*
*Context gathered: 2026-03-05*
