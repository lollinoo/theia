# Roadmap: MikroTik Theia

## Overview

MikroTik Theia goes from zero to a fully functional network topology visualizer in 5 phases. Phase 1 establishes the Go backend with domain model, persistence, and device management API. Phase 2 builds the React frontend with an interactive canvas, static device/link rendering, and dark theme. Phase 3 connects the real-time pipeline -- Prometheus metrics, SNMP polling, and WebSocket push -- making the map live. Phase 4 adds Grafana integration, per-interface drill-down, configurable polling, and keyboard shortcuts. Phase 5 delivers routing protocol visualization (BGP/OSPF). Each phase produces a verifiable, usable increment.

## Phases

**Phase Numbering:**
- Integer phases (0, 1, 2, 3...): Planned milestone work
- Decimal phases (2.1, 2.2): Urgent insertions (marked with INSERTED)

Decimal phases appear between their surrounding integers in numeric order.

- [x] **Phase 0: Docker Development Environment** - Docker environment for development, testing, and verification — prerequisite for all phases
- [ ] **Phase 1: Foundation** - Go backend with domain model, SQLite persistence, device CRUD API, and SNMP connectivity
- [ ] **Phase 2: Interactive Canvas** - React frontend with topology canvas, device/link rendering, dark theme, and layout persistence
- [ ] **Phase 3: Real-Time Pipeline** - Live metrics via Prometheus, WebSocket push, SNMP polling, and visual alerts
- [ ] **Phase 4: Integration and Polish** - Grafana deep-links, per-interface stats, configurable polling, keyboard shortcuts
- [ ] **Phase 5: Routing Protocols** - BGP session status, OSPF neighbors, and route count visualization

## Phase Details

### Phase 0: Docker Development Environment

**Goal:** All development, testing, and verification runs inside Docker containers with SNMP simulators, hot-reload, and a Makefile interface
**Requirements**: DOCKER-INFRA
**Depends on:** Nothing (prerequisite for all phases)
**Status:** Completed (2026-03-05)
**Plans:** 2 plans
**Success Criteria** (what must be TRUE):
  1. `make dev` starts the full stack (backend, frontend, 3 SNMP simulators) with a single command
  2. Go backend hot-reloads on source changes via Air inside the container
  3. Three SNMP simulators (MikroTik, Cisco, Ubiquiti) respond with realistic vendor-specific MIB data
  4. SNMP simulators have LLDP neighbor relationships forming Router <-> Switch <-> AP topology
  5. `make test` and `make test-integration` run tests inside containers
  6. `make seed` populates the 3 sim devices via the REST API

Plans:
- [x] 00-01-PLAN.md — Docker infrastructure: Dockerfile, docker-compose, Air hot-reload, frontend placeholder
- [x] 00-02-PLAN.md — SNMP simulator configs, Makefile, seed script, config.yaml

### Phase 1: Foundation
**Goal**: Operators can manage network devices through a REST API with persistent storage and SNMP connectivity validation
**Depends on**: Phase 0
**Requirements**: DEV-01, DEV-02, DEV-05, DEV-06, INTG-04, INTG-05
**Success Criteria** (what must be TRUE):
  1. User can add a device by IP/hostname with SNMP credentials and see it persisted across server restarts
  2. User can edit and delete existing devices via the API
  3. Backend successfully queries SNMP data (sysName, sysDescr, interfaces) from a real MikroTik or SNMP-capable device
  4. Backend discovers LLDP/CDP neighbors from a device and returns neighbor relationships
  5. API returns device data including hostname, IP, and hardware model parsed from SNMP
**Plans:** 3 plans

Plans:
- [ ] 01-01-PLAN.md — Go project scaffold, domain model, SQLite persistence, and config system
- [ ] 01-02-PLAN.md — SNMP client, device discovery, and device type auto-detection
- [ ] 01-03-PLAN.md — REST API, async device management, background poller, and main.go wiring

### Phase 2: Interactive Canvas
**Goal**: Operators can see their full network topology on an interactive dark-themed canvas with device cards, link lines, and persistent layout
**Depends on**: Phase 1
**Requirements**: CANV-01, CANV-02, CANV-03, CANV-04, CANV-05, CANV-06, DEV-03, DEV-04, LINK-01, LINK-02, UX-01, UX-02, UX-04
**Success Criteria** (what must be TRUE):
  1. User can pan, zoom, and drag devices freely on a dark-themed canvas that renders 100+ nodes without lag
  2. Device cards display hostname, IP, type icon (Router/Switch/AP), and a status indicator placeholder
  3. Links between devices appear as lines with bandwidth capacity labels
  4. Device positions persist across browser sessions (survive page reload)
  5. User can search for a device by hostname or IP and the canvas focuses on the result
**Plans**: TBD

Plans:
- [ ] 02-01: TBD
- [ ] 02-02: TBD

### Phase 3: Real-Time Pipeline
**Goal**: The topology map is alive -- device metrics update in real-time, links show live throughput, and alerts are visually reflected on the canvas
**Depends on**: Phase 2
**Requirements**: METR-01, METR-02, METR-03, METR-04, METR-05, LINK-03, LINK-04, INTG-01, ALRT-01, ALRT-02, ALRT-03
**Success Criteria** (what must be TRUE):
  1. Device cards display live CPU, memory, uptime, and temperature values that update without page refresh
  2. Links show real-time TX/RX throughput and are color-coded by utilization level
  3. When a device goes down, its card visually changes (color/icon) within one polling cycle
  4. When a link degrades or goes down, its visual appearance changes to reflect the state
  5. All metric data originates from the existing Prometheus instance via PromQL queries (no duplicate collection)
**Plans**: TBD

Plans:
- [ ] 03-01: TBD
- [ ] 03-02: TBD

### Phase 4: Integration and Polish
**Goal**: Operators can drill from the topology map into Grafana for deep dives, inspect per-interface statistics, and tune polling behavior
**Depends on**: Phase 3
**Requirements**: INTG-02, INTG-03, LINK-05, METR-06, METR-07, UX-03
**Success Criteria** (what must be TRUE):
  1. User can click a device card and open its corresponding Grafana dashboard in a new tab
  2. User can click a specific metric and open the relevant Grafana panel
  3. User can click a link to see per-interface statistics (TX/RX, errors, drops)
  4. User can configure global and per-device polling intervals, and changes take effect without restart
  5. Keyboard shortcuts work for common actions (search, add device, zoom)
**Plans**: TBD

Plans:
- [ ] 04-01: TBD
- [ ] 04-02: TBD

### Phase 5: Routing Protocols
**Goal**: Operators can view routing protocol status (BGP/OSPF) and route counts directly from device cards on the topology map
**Depends on**: Phase 3
**Requirements**: ROUT-01, ROUT-02, ROUT-03
**Success Criteria** (what must be TRUE):
  1. User can view BGP session status and neighbor details for a selected device
  2. User can view OSPF neighbor status for a selected device
  3. User can view route count summaries (total, BGP, OSPF) for a selected device
**Plans**: TBD

Plans:
- [ ] 05-01: TBD

## Progress

**Execution Order:**
Phases execute in numeric order: 0 -> 1 -> 2 -> 3 -> 4 -> 5

| Phase | Plans Complete | Status | Completed |
|-------|----------------|--------|-----------|
| 0. Docker Environment | 2/2 | Completed | 2026-03-05 |
| 1. Foundation | 0/3 | Planning complete | - |
| 2. Interactive Canvas | 0/0 | Not started | - |
| 3. Real-Time Pipeline | 0/0 | Not started | - |
| 4. Integration and Polish | 0/0 | Not started | - |
| 5. Routing Protocols | 0/0 | Not started | - |
