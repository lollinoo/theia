# MikroTik Theia

## What This Is

A modern, interactive network topology visualizer that provides a bird's-eye view of network infrastructure. Built as a web application (React + Go) that integrates with an existing Prometheus/Grafana monitoring stack to display real-time statistics for routers, switches, and their interconnections on a drag-and-drop canvas. Think of it as a modern replacement for MikroTik's The Dude — starting with network devices, with a vision to expand to full infrastructure mapping.

## Core Value

Network operators can see their entire topology at a glance with live stats on every device and link, and drill into Grafana for deep dives — all from a single interactive map.

## Requirements

### Validated

- [x] Manual device lifecycle via REST API: add, edit, and remove devices by IP/hostname with SNMP credentials
- [x] Multi-vendor SNMP topology discovery for devices, interfaces, and LLDP/CDP neighbor relationships
- [x] Interactive dark-themed topology canvas with pan, zoom, drag, minimap, search, and persistent layout
- [x] Auto-layout with manual position override and saved node positions
- [x] Device cards showing hostname, IP, hardware model, type icon, and down/degraded status treatment
- [x] Link visualization with bandwidth labels
- [x] Prometheus as the primary data source for live topology metrics
- [x] Real-time device metrics on the canvas via WebSocket (CPU, memory, uptime, temperature where available)
- [x] Live link throughput and utilization coloring on the canvas
- [x] Reconnect handling and stale-metric clearing for the live topology view

### Active

- [ ] Background image upload for the topology canvas
- [ ] Per-interface statistics (TX/RX, errors, drops) accessible from links or link drill-down
- [ ] Prometheus alert-rule-backed visual alert coverage for device and link failures
- [ ] Grafana dashboard and panel deep-links from devices and metrics
- [ ] Configurable polling intervals (global and per-device)
- [ ] Keyboard shortcuts for common actions (search, add device, zoom)
- [ ] Verified performance hardening for 100+ devices on a single map
- [ ] Routing information display (BGP sessions, OSPF neighbors, route counts)
- [ ] Broader multi-vendor validation beyond the current dev fixtures and seeded devices
- [ ] Vendor-specific API extensions beyond SNMP where they improve topology or routing fidelity

### Out of Scope

- Container/service nesting inside device cards — deferred to v2
- Server, NAS, PC, SBC, UPS device types — routers/switches/APs first
- Auto-discovery/subnet scanning — manual add only for v1
- Push notifications or alerting — visual status only, alerting stays in Prometheus/Alertmanager
- Replacing Grafana — this complements it, not replaces it
- Mobile app — web-first
- Embedded Grafana panels — link out to Grafana instead
- Component palette sidebar (Types, Presets, Services tabs) — simplified add flow for v1

## Context

- Existing monitoring stack: Prometheus, Grafana, SNMP-Exporter, Blackbox-Exporter
- Network is multi-vendor (MikroTik, Cisco, Ubiquiti, and others)
- Scale: 100+ routers in production
- Reference UI: dark-themed canvas with device cards, dashed link lines, bandwidth labels, status dots (see frontend_example.jpg)
- The tool needs to work as both web and eventually desktop (Electron possible) — web-first for v1

## Constraints

- **Data source**: Must integrate with existing Prometheus instance — no duplicate metric collection
- **Scale**: Must handle 100+ devices without performance degradation on the canvas
- **Tech stack**: React frontend, Go backend — chosen for performance and ecosystem fit
- **SNMP compatibility**: Must work with any device exposing standard SNMP MIBs
- **Real-time**: Configurable polling intervals, not just static snapshots

## Current Milestone: Phase 4 Integration And Polish

**Goal:** Build on the completed live topology map with Grafana deep-links, per-interface drill-down, configurable polling, and workflow polish.

**Delivered foundation:**
- Interactive dark-themed canvas with drag-and-drop positioning and auto-layout
- Real-time device metrics from Prometheus via WebSocket
- Link visualization with bandwidth labels and live throughput
- Multi-vendor SNMP-backed topology discovery and persisted layout

**Remaining target features:**
- Grafana dashboard and panel deep-links
- Per-interface statistics and link drill-down
- Configurable polling controls
- Background image upload and remaining canvas polish
- Prometheus alert-rule-backed device/link failure visuals
- Performance validation and hardening for 100+ devices
- Keyboard shortcuts and remaining UI polish

## Key Decisions

| Decision | Rationale | Outcome |
|----------|-----------|---------|
| React + Go stack | Performance-oriented, good ecosystem for both network tools and interactive UIs | Adopted and implemented |
| Prometheus as primary data source | Leverage existing monitoring infrastructure, avoid duplicate collection | Adopted and implemented |
| Complement Grafana (not replace) | Grafana excels at deep-dive dashboards; this provides topology context | Adopted; deep-links scheduled for Phase 4 |
| Multi-vendor via SNMP | Common denominator across vendors; direct API integration can be added per-vendor | Adopted and implemented for core topology and metrics |
| Manual device add only (v1) | Simpler MVP; auto-discovery adds complexity and security considerations | Adopted and implemented |
| Skip containers for v1 | Focus on network topology first; container/service mapping is a v2 feature | Adopted and implemented |

---
*Last updated: 2026-03-07 after Phase 3 completion review*
