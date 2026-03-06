# MikroTik Theia

## What This Is

A modern, interactive network topology visualizer that provides a bird's-eye view of network infrastructure. Built as a web application (React + Go) that integrates with an existing Prometheus/Grafana monitoring stack to display real-time statistics for routers, switches, and their interconnections on a drag-and-drop canvas. Think of it as a modern replacement for MikroTik's The Dude — starting with network devices, with a vision to expand to full infrastructure mapping.

## Core Value

Network operators can see their entire topology at a glance with live stats on every device and link, and drill into Grafana for deep dives — all from a single interactive map.

## Requirements

### Validated

(None yet — ship to validate)

### Active

- [ ] Interactive canvas with free-form drag positioning of network devices
- [ ] Auto-layout algorithm that initially positions nodes based on topology, with manual adjustment
- [ ] Rich device cards showing hostname, IP, hardware specs, and status indicator
- [ ] Real-time metrics on device cards (CPU, memory, uptime, temperature) pulled from Prometheus
- [ ] Link visualization between devices with bandwidth labels and live throughput stats
- [ ] Per-interface statistics (TX/RX, errors, drops) accessible from device cards
- [ ] Routing information display (BGP sessions, OSPF neighbors, route counts)
- [ ] Multi-vendor support via SNMP as common denominator
- [ ] Direct device access (SNMP/API) for topology and configuration information
- [ ] Prometheus as primary data source for all metrics (PromQL queries)
- [ ] Configurable polling intervals (per-device and global)
- [ ] Visual alerts on map (color changes, status icons) when devices or links go down
- [ ] Click-through links from devices/metrics to corresponding Grafana dashboards
- [ ] Manual device addition by IP/hostname
- [ ] Device type icons (Router, Switch, AP) with visual differentiation
- [ ] Dark theme UI matching the reference design
- [ ] Support for 100+ devices on a single map

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

## Current Milestone: v1.0 Network Topology Visualizer

**Goal:** Deliver a functional network topology map with real-time Prometheus metrics, multi-vendor SNMP support, and Grafana integration.

**Target features:**
- Interactive canvas with drag-and-drop device positioning and auto-layout
- Real-time device metrics (CPU, memory, uptime, temperature) from Prometheus
- Link visualization with bandwidth labels and live throughput
- Multi-vendor support via SNMP
- Visual alerts for device/link status changes
- Click-through to Grafana dashboards
- Dark theme UI matching reference design

## Key Decisions

| Decision | Rationale | Outcome |
|----------|-----------|---------|
| React + Go stack | Performance-oriented, good ecosystem for both network tools and interactive UIs | — Pending |
| Prometheus as primary data source | Leverage existing monitoring infrastructure, avoid duplicate collection | — Pending |
| Complement Grafana (not replace) | Grafana excels at deep-dive dashboards; this provides topology context | — Pending |
| Multi-vendor via SNMP | Common denominator across vendors; direct API integration can be added per-vendor | — Pending |
| Manual device add only (v1) | Simpler MVP; auto-discovery adds complexity and security considerations | — Pending |
| Skip containers for v1 | Focus on network topology first; container/service mapping is a v2 feature | — Pending |

---
*Last updated: 2026-03-05 after milestone v1.0 start*
