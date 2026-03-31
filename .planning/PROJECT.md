# MikroTik Theia

## What This Is

A modern, interactive network topology visualizer that provides a bird's-eye view of network infrastructure with the Neon Topography design system. Built as a web application (React + Go) that integrates with an existing Prometheus/Grafana monitoring stack to display real-time statistics for routers, switches, and their interconnections on a drag-and-drop canvas with OSPF area organization. Think of it as a modern replacement for MikroTik's The Dude — starting with network devices, with a vision to expand to full infrastructure mapping.

## Core Value

Network operators can see their entire topology at a glance with live stats on every device and link, and drill into Grafana for deep dives — all from a single interactive map.

## Current State

**Shipped:** v1.3.0 Frontend Redesign (2026-03-27)
**In Progress:** v1.3.7 Virtual/Representative Nodes — Phase 8 complete (backend), Phases 9-10 remaining (rendering, forms)

The frontend has been fully redesigned with the Neon Topography design system featuring:
- Dual dark/light theme support with CSS variable tokens, FOWT prevention, and localStorage persistence
- OSPF Area Hub view with floating navigation pill, area cards with bloom effects, and area-filtered topology
- Material Symbols icon system with custom woff2 subsets
- Redesigned devices page with custom filter dropdowns, expanded sortable columns, and icon actions
- Canvas decomposed from monolithic 1283-line file into 7 focused modules
- 193 frontend tests across 30 test files, 14.1k LOC TypeScript

**Tech stack:** React 18 + Tailwind CSS 4 + ReactFlow 12, Go 1.24 backend, SQLite, Prometheus integration

## Requirements

### Validated

- ✓ Interactive canvas with free-form drag positioning of network devices — v1.0
- ✓ Rich device cards showing hostname, IP, hardware specs, and status indicator — v1.0
- ✓ Real-time metrics on device cards (CPU, memory, uptime, temperature) pulled from Prometheus — v1.0
- ✓ Multi-vendor support via SNMP as common denominator — v1.0
- ✓ Direct device access (SNMP/API) for topology and configuration information — v1.0
- ✓ Prometheus as primary data source for all metrics (PromQL queries) — v1.0
- ✓ Manual device addition by IP/hostname — v1.0
- ✓ Device type icons (Router, Switch, AP) with visual differentiation — v1.0
- ✓ Dark theme UI — v1.0
- ✓ Visual alerts on map (color changes, status icons) when devices or links go down — v1.0
- ✓ Design token foundation and theme infrastructure — v1.3.0
- ✓ Neon Topography design system applied to all 25+ components — v1.3.0
- ✓ Dark and light theme support with seamless switching — v1.3.0
- ✓ OSPF Area Hub view with floating nav pill and area cards — v1.3.0
- ✓ Manual area CRUD in settings — v1.3.0
- ✓ Device-to-area assignment — v1.3.0
- ✓ Backend + DB support for areas — v1.3.0
- ✓ Redesigned device context menu matching Neon Topography — v1.3.0
- ✓ Device nodes with glow status indicators — v1.3.0
- ✓ Atmospheric watermark backgrounds — v1.3.0
- ✓ Area-filtered topology views — v1.3.0
- ✓ Redesigned devices page with custom dropdowns and icon actions — v1.3.0
- ✓ SidePanel and sub-panel form/metric restyling — v1.3.0
- ✓ Canvas theme compliance (no stale tokens or hardcoded hex) — v1.3.0

### Active

- Virtual device type ("virtual") in domain model with DeviceTypeVirtual constant — VIRT-01 (Phase 8 validated)
- Partial unique IP index allowing multiple virtual devices with empty IP — VIRT-02 (Phase 8 validated)
- Virtual device creation via API with subtype tags (internet/cloud/server/generic) — VIRT-03 (Phase 8 validated)
- Virtual device probe behavior: no-IP stays "unknown", with-IP gets status from MetricsCollector — VIRT-04 (Phase 8 validated)
- SNMP poller skips virtual devices entirely — VIRT-05 (Phase 8 validated)
- Virtual node compact card rendering with subtype icons — VIRT-06 through VIRT-09 (Phase 9)
- Virtual node forms and context menu adaptation — VIRT-10 through VIRT-16 (Phase 10)

### Out of Scope

- Container/service nesting inside device cards — deferred to v2
- Server, NAS, PC, SBC, UPS device types — routers/switches/APs first
- Auto-discovery/subnet scanning — manual add only for v1
- Push notifications or alerting — visual status only, alerting stays in Prometheus/Alertmanager
- Replacing Grafana — this complements it, not replaces it
- Mobile app — web-first
- Embedded Grafana panels — link out to Grafana instead
- Component palette sidebar (Types, Presets, Services tabs) — simplified add flow for v1
- Custom user-defined theme colors — design system is opinionated
- Animated canvas backgrounds — competes with data for attention
- Drag-to-assign devices to areas on canvas — conflates spatial position with logical grouping

## Context

- Existing monitoring stack: Prometheus, Grafana, SNMP-Exporter, Blackbox-Exporter
- Network is multi-vendor (MikroTik, Cisco, Ubiquiti, and others)
- Scale: 100+ routers in production
- Frontend: 14.1k LOC TypeScript, 193 tests, React 18 + Tailwind CSS 4 + ReactFlow 12
- Backend: Go 1.24, SQLite, SNMP polling, WebSocket metrics push
- The tool needs to work as both web and eventually desktop (Electron possible) — web-first for v1

## Known Tech Debt (from v1.3.0)

- AreaHub "Open Settings" CTA navigates to Canvas but does not open Settings panel (INT-01)
- App.tsx areas state not refreshed on navigation to Dashboard or after Settings area mutation (INT-02)
- DeviceConfigPanel uses raw neon area colors without adaptAreaColor in light theme (INT-03)

## Constraints

- **Data source**: Must integrate with existing Prometheus instance — no duplicate metric collection
- **Scale**: Must handle 100+ devices without performance degradation on the canvas
- **Tech stack**: React frontend, Go backend — chosen for performance and ecosystem fit
- **SNMP compatibility**: Must work with any device exposing standard SNMP MIBs
- **Real-time**: Configurable polling intervals, not just static snapshots

## Key Decisions

| Decision | Rationale | Outcome |
|----------|-----------|---------|
| React + Go stack | Performance-oriented, good ecosystem for both network tools and interactive UIs | ✓ Good |
| Prometheus as primary data source | Leverage existing monitoring infrastructure, avoid duplicate collection | ✓ Good |
| Complement Grafana (not replace) | Grafana excels at deep-dive dashboards; this provides topology context | ✓ Good |
| Multi-vendor via SNMP | Common denominator across vendors; direct API integration can be added per-vendor | ✓ Good |
| Manual device add only (v1) | Simpler MVP; auto-discovery adds complexity and security considerations | ✓ Good |
| Skip containers for v1 | Focus on network topology first; container/service mapping is a v2 feature | ✓ Good |
| Neon Topography design system | High-end editorial aesthetic, dual-font strategy, depth via luminosity | ✓ Good — shipped v1.3.0 |
| Manual areas (not SNMP-derived) | SNMP exporters don't expose OSPF area OIDs; manual grouping is simpler and more flexible | ✓ Good |
| No route count display | Too expensive to fetch at scale; show health and active links instead | ✓ Good |
| Both dark + light themes | Design system supports both; ship together in v1.3.0 | ✓ Good — shipped v1.3.0 |
| Tailwind v4 with @theme inline tokens | Native CSS variable integration, eliminates JS-side token mapping | ✓ Good — 34 semantic tokens |
| ReactFlow v12 with native colorMode | Built-in theme support, no custom CSS overrides needed | ✓ Good |
| Canvas decomposition (7 modules) | 1283-line monolith → 7 focused files, easier to maintain and test | ✓ Good — unlocked three-view architecture |
| Material Symbols woff2 subset | Custom subset keeps bundle at 29KB vs 4MB full icon font | ✓ Good — 21 icons |
| font-mono for all technical values | Consistent monospace rendering for metrics, timestamps, OIDs, code | ✓ Good |

## Evolution

This document evolves at phase transitions and milestone boundaries.

**After each phase transition** (via `/gsd:transition`):
1. Requirements invalidated? → Move to Out of Scope with reason
2. Requirements validated? → Move to Validated with phase reference
3. New requirements emerged? → Add to Active
4. Decisions to log? → Add to Key Decisions
5. "What This Is" still accurate? → Update if drifted

**After each milestone** (via `/gsd:complete-milestone`):
1. Full review of all sections
2. Core Value check — still the right priority?
3. Audit Out of Scope — reasons still valid?
4. Update Context with current state

---
*Last updated: 2026-03-31 after Phase 8 (Virtual Device Backend) completion*
