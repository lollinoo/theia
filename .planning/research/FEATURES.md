# Feature Research

**Domain:** Network Topology Visualization with Real-Time Monitoring
**Researched:** 2026-03-05
**Confidence:** HIGH

## Feature Landscape

### Table Stakes (Users Expect These)

Features users assume exist. Missing these = product feels incomplete.

#### Canvas / Map Features

| Feature | Why Expected | Complexity | Notes |
|---------|--------------|------------|-------|
| Drag-and-drop node positioning | Every topology tool from The Dude to PRTG has draggable nodes. Users expect to arrange their network spatially | MEDIUM | Core canvas interaction; positions must persist to backend |
| Pan and zoom | Standard canvas interaction; impossible to work with 100+ devices without it | LOW | Infinite canvas with smooth zoom (mouse wheel + pinch). Must handle zoom levels gracefully (hide detail at far zoom, show detail on close zoom) |
| Device status indicators (up/down/warning) | The Dude, LibreNMS, Zabbix, PRTG all show device health at a glance via color-coded dots or borders | LOW | Green=up, red=down, yellow=degraded, grey=unknown. Pull from Prometheus ICMP/SNMP up metrics |
| Link visualization between devices | Every competitor draws lines/edges between connected devices. Links are the core of topology | MEDIUM | Lines connecting device nodes, representing physical or logical connections |
| Link status indicators | LibreNMS uses color-coded links (green/yellow/orange/red/purple by utilization %). PRTG changes link color by load. This is standard | MEDIUM | Color-coded by utilization threshold: green (0-50%), yellow (50-75%), orange (75-90%), red (90%+) |
| Device cards with basic info | The Dude shows hostname, IP, device type. Every NMS shows at minimum hostname and IP on the map | LOW | Hostname, management IP, device type icon, status dot at minimum |
| Device type icons | The Dude uses SVG icons; PRTG and SolarWinds differentiate routers, switches, APs visually. Users orient by shape | LOW | Router, Switch, AP icons at minimum. Use distinct silhouettes, not just color differences |
| Real-time metric overlay on devices | PRTG shows current metrics on map nodes. The Dude shows service status. Users expect live data, not static diagrams | HIGH | CPU, memory, uptime from Prometheus. This is the main integration point and core value prop |
| Bandwidth/throughput on links | The Dude shows per-link throughput. LibreNMS shows TX/RX on links. PRTG shows bandwidth on link lines. This is table stakes for any monitoring-integrated topology tool | HIGH | TX/RX bps labels on link lines, pulled from Prometheus interface metrics via snmp_exporter |
| Manual device addition | Required for v1 per PROJECT.md. The Dude supports manual add alongside auto-discovery. For a tool without auto-discovery, this is the only way to populate the map | LOW | Add by IP/hostname, specify device type, assign to map position |
| Click-to-detail / drill-down | Every tool lets you click a device for more info. The Dude opens device details. PRTG navigates to sensor details. LibreNMS opens device page | MEDIUM | Click device to expand card or open detail panel with full metrics, interface list, routing info |
| Persistent layout (save positions) | LibreNMS custom maps and NetBox topology views both persist node coordinates. Users will not re-arrange 100 devices every session | MEDIUM | Save X/Y positions per device to database. Load on page open |
| Visual alerts on map | The Dude changes device color and shows notifications on map. Zabbix maps show trigger status on map elements. PRTG changes node colors by alarm state | MEDIUM | When a device or link is in alarm state (from Prometheus alerting rules), reflect visually on map with color change, pulsing, or icon overlay |

#### Monitoring Integration Features

| Feature | Why Expected | Complexity | Notes |
|---------|--------------|------------|-------|
| Prometheus as data source | Core constraint from PROJECT.md. The tool exists to visualize data already in Prometheus | HIGH | PromQL queries for device metrics, interface stats, status. Must handle query construction and result mapping |
| Grafana deep-link integration | Core value prop from PROJECT.md. Users already have Grafana dashboards; this tool provides topology context that links into them | LOW | Construct Grafana URLs with device-specific variables (hostname, interface). Configurable base URL |
| Per-interface statistics | The Dude shows per-port stats. LibreNMS shows per-interface bandwidth. PRTG monitors per-sensor. Network operators need interface-level visibility | HIGH | TX/RX bytes, errors, drops, speed per interface. Pulled from Prometheus snmp_exporter metrics |
| Configurable polling/refresh intervals | The Dude supports per-device polling intervals. PRTG supports per-sensor intervals. Users need control over update frequency vs load | LOW | Global default + per-device override. Controls how often frontend refreshes metrics from backend |
| Multi-vendor SNMP support | The Dude, LibreNMS, Observium, PRTG all support multi-vendor via SNMP. Network operators have mixed fleets | MEDIUM | Standard MIB support (IF-MIB, HOST-RESOURCES-MIB, etc.) via snmp_exporter already in stack |

### Differentiators (Competitive Advantage)

Features that set the product apart. Not expected in every tool, but create real value.

| Feature | Value Proposition | Complexity | Notes |
|---------|-------------------|------------|-------|
| Modern web-native UX | The Dude is a Windows desktop app (or Wine). LibreNMS/Zabbix maps feel dated. PRTG is better but still enterprise-clunky. A modern React canvas with smooth animations, dark theme, and responsive design is genuinely better than anything in the open-source NMS space | MEDIUM | Dark theme, smooth animations, keyboard shortcuts, responsive. This is the primary differentiator over The Dude |
| Prometheus-native architecture | No other open-source topology tool is built Prometheus-first. LibreNMS, Zabbix, Observium all have their own collectors. Building on existing Prometheus means zero duplicate data collection | HIGH | Instead of reimplementing SNMP polling, query Prometheus for metrics that snmp_exporter already collects. Unique architectural advantage |
| Routing protocol visualization (BGP/OSPF) | Most topology tools show physical/L2 topology. Showing BGP session status, OSPF neighbor state, and route counts on the map is rare outside enterprise tools like Cisco Crosswork | HIGH | BGP session state (established/idle), OSPF neighbor status, route table counts. Data from Prometheus if bgp_exporter or similar is running |
| Topology as Grafana companion | No existing tool is purpose-built as a "topology layer for Grafana." LibreNMS and Zabbix are self-contained NMS platforms. Positioning as the spatial/topological complement to Grafana's time-series dashboards is a unique niche | LOW | Marketing/positioning, but also informs UX: every metric should be one click from a Grafana panel |
| Sub-map / hierarchical views | The Dude supports sub-maps (click into a site to see its devices). PRTG has multi-layer maps. Most open-source tools lack this. At 100+ devices, a single flat map becomes unusable | HIGH | Group devices by site/location/function. Click a site group to zoom into its sub-map. Critical for scale |
| Background images / floor plans | The Dude supports custom SVG backgrounds. PRTG supports data center floor plans. LibreNMS custom maps support PNG/JPG backgrounds. Useful for mapping devices to physical locations | LOW | Upload PNG/JPG as canvas background. Position devices on top of rack diagrams or office floor plans |
| Keyboard shortcuts and power-user UX | Network operators live in terminal. Keyboard-driven interaction (search, navigate, quick-add) is valued but rare in topology tools | MEDIUM | / for search, arrow keys for navigation, Enter for detail, Esc to close. Vim-style optional |
| Link aggregation visualization | Network operators use LAG/LACP between switches. Showing aggregated links as a single thick line (with combined throughput) rather than N separate links reduces visual clutter | MEDIUM | Detect and group interfaces in a bond/LAG. Show single link with aggregate stats. Dashed vs solid per LibreNMS convention |
| Search and filter | With 100+ devices, users need to find a specific device quickly. The Dude has search. PRTG has filtering. LibreNMS has device search but not on the map | LOW | Search bar that highlights/navigates to matching device on canvas. Filter by device type, status, name pattern |

### Anti-Features (Commonly Requested, Often Problematic)

Features that seem good but create problems. Explicitly NOT building these.

| Feature | Why Requested | Why Problematic | Alternative |
|---------|---------------|-----------------|-------------|
| Auto-discovery / subnet scanning | The Dude and PRTG do it. Users think it saves time | Massive security implications (scanning production networks), complex to implement correctly (SNMP community strings, credential management, false positives), and PROJECT.md explicitly defers this | Manual device addition for v1. Auto-discovery is a v2 feature once the core visualization is proven |
| Built-in alerting / notifications | The Dude sends alerts. PRTG sends notifications. Users want "one tool for everything" | Duplicates Prometheus Alertmanager which already handles alerting well. Building another alerting pipeline creates confusion about "which alert is authoritative" | Visual status on map reflects Prometheus alert states. Clicking an alerted device links to Alertmanager or Grafana alert panel |
| Embedded Grafana panels | Users want to see Grafana graphs inline on the topology map | iframe embedding is fragile, slow, creates auth complexity, and clutters the topology view. PROJECT.md explicitly excludes this | Deep-link to Grafana dashboards. One-click opens the right Grafana panel in a new tab |
| SNMP polling / metric collection | "Why not poll SNMP directly instead of going through Prometheus?" | Duplicates existing snmp_exporter infrastructure. Creates two sources of truth for the same metrics. Adds operational burden of managing SNMP credentials in two places | Query Prometheus for all metrics. The backend never touches SNMP directly |
| Configuration management | The Dude can configure RouterOS devices. Users want to "manage from the map" | Configuration management is a separate domain (Ansible, Oxidized, RANCID). Mixing monitoring and config management in one tool creates dangerous accidental-change scenarios | Link out to device management tools. Show config status (last backup date) but never modify configs |
| Mobile app | "I want to check my network from my phone" | Mobile topology visualization at 100+ nodes is a terrible UX. Small screens cannot show meaningful topology. Development cost is high for low value | Responsive web design that works acceptably on tablets. For phone-size screens, show a device list with status rather than the full canvas |
| Full NMS replacement | "Can it replace LibreNMS/Zabbix entirely?" | Scope creep that would take years and produce a mediocre NMS. The value is in being a focused topology layer | Stay complementary. Topology + live stats + deep-link to Grafana/Alertmanager. Let specialized tools handle alerting, historical trending, capacity planning |
| Component palette sidebar | Tabbed sidebar with Types, Presets, Services tabs as in reference design | Adds UI complexity for v1. Users need to add devices, not browse a component catalog. PROJECT.md defers this | Simple "Add Device" dialog/modal for v1. Palette sidebar is a v2 UX enhancement |

## Feature Dependencies

```
[Canvas Engine (pan/zoom/drag)]
    |-- requires --> [Persistent Layout (save positions)]
    |-- requires --> [Device Cards (render nodes)]
                        |-- requires --> [Manual Device Addition (populate data)]
                        |-- enhances --> [Real-Time Metrics Overlay]
                                            |-- requires --> [Prometheus Integration (data source)]
                                            |-- enhances --> [Visual Alerts on Map]
                        |-- enhances --> [Click-to-Detail Panel]
                                            |-- enhances --> [Per-Interface Statistics]
                                            |-- enhances --> [Grafana Deep-Links]
                                            |-- enhances --> [Routing Protocol Viz (BGP/OSPF)]

[Link Visualization]
    |-- requires --> [Canvas Engine]
    |-- requires --> [Device Cards (endpoints)]
    |-- enhances --> [Bandwidth/Throughput Labels]
                        |-- requires --> [Prometheus Integration]
    |-- enhances --> [Link Status Indicators (color-coded)]
                        |-- requires --> [Prometheus Integration]
    |-- enhances --> [Link Aggregation Visualization]

[Search & Filter]
    |-- requires --> [Device Cards (searchable data)]

[Sub-Maps / Hierarchical Views]
    |-- requires --> [Canvas Engine]
    |-- requires --> [Persistent Layout]
    |-- requires --> [Device Grouping concept]

[Background Images]
    |-- requires --> [Canvas Engine]
```

### Dependency Notes

- **Canvas Engine is foundational:** Everything visual depends on it. Must be built first and built well. Performance at 100+ nodes is a hard requirement.
- **Prometheus Integration is the data backbone:** All real-time features (metrics, alerts, link stats) depend on being able to query Prometheus efficiently.
- **Device Cards require devices in the system:** Manual device addition must work before cards can be rendered.
- **Sub-Maps require grouping:** Cannot build hierarchical views without a device-grouping/location concept in the data model.
- **Link visualization requires both endpoints:** Links can only be drawn once devices exist on the canvas.

## MVP Definition

### Launch With (v1)

Minimum viable product -- what's needed to validate the concept.

- [ ] **Canvas engine with pan/zoom/drag** -- without this, nothing works
- [ ] **Manual device addition** (IP, hostname, type) -- only way to populate the map in v1
- [ ] **Device cards** (hostname, IP, type icon, status dot) -- the core visual element
- [ ] **Device status indicators** (up/down/warning) -- first thing operators look for
- [ ] **Link visualization** between devices -- topology without links is just a list
- [ ] **Link status indicators** (color by utilization) -- makes links meaningful
- [ ] **Bandwidth labels on links** (TX/RX) -- the data that justifies the topology view
- [ ] **Real-time metrics on device cards** (CPU, memory, uptime) -- core value prop
- [ ] **Prometheus integration** (PromQL queries) -- the data source for everything
- [ ] **Grafana deep-links** from devices/metrics -- connects topology context to existing dashboards
- [ ] **Persistent layout** (save/load positions) -- users won't re-arrange 100 devices daily
- [ ] **Visual alerts** (color change on alarm) -- must reflect Prometheus alert state
- [ ] **Dark theme UI** -- matches reference design, operator preference
- [ ] **Search** (find device on map) -- essential at 100+ devices

### Add After Validation (v1.x)

Features to add once core is working.

- [ ] **Per-interface statistics panel** -- trigger: users want interface-level detail beyond aggregate card stats
- [ ] **Routing protocol visualization** (BGP/OSPF) -- trigger: users with BGP/OSPF want session status on map
- [ ] **Background images** -- trigger: users want to overlay on floor plans or rack diagrams
- [ ] **Link aggregation visualization** -- trigger: users with LAG/LACP want aggregated link display
- [ ] **Keyboard shortcuts** -- trigger: power users requesting faster navigation
- [ ] **Sub-maps / hierarchical views** -- trigger: map becomes crowded at scale, users request site-based grouping
- [ ] **Device grouping / tagging** -- trigger: needed for sub-maps and filtering by site/role
- [ ] **Configurable metric display** -- trigger: users want to choose which metrics show on cards

### Future Consideration (v2+)

Features to defer until product-market fit is established.

- [ ] **Auto-discovery** -- why defer: security implications, complex implementation, manual add works for v1
- [ ] **Container/service nesting** -- why defer: different domain (infrastructure vs network), PROJECT.md defers to v2
- [ ] **Additional device types** (servers, NAS, UPS) -- why defer: routers/switches/APs first per PROJECT.md
- [ ] **Geographic/world map view** -- why defer: nice to have for multi-site but not core topology value
- [ ] **Topology diff / change history** -- why defer: requires versioning infrastructure, v2 maturity feature
- [ ] **API for external integrations** -- why defer: need stable internal model first
- [ ] **Multi-user / role-based access** -- why defer: single-user or team-shared is fine for v1
- [ ] **Component palette sidebar** -- why defer: PROJECT.md defers, simple add dialog suffices

## Feature Prioritization Matrix

| Feature | User Value | Implementation Cost | Priority |
|---------|------------|---------------------|----------|
| Canvas engine (pan/zoom/drag) | HIGH | HIGH | P1 |
| Manual device addition | HIGH | LOW | P1 |
| Device cards (hostname, IP, icon, status) | HIGH | MEDIUM | P1 |
| Device status indicators (up/down) | HIGH | LOW | P1 |
| Link visualization | HIGH | MEDIUM | P1 |
| Prometheus integration | HIGH | HIGH | P1 |
| Real-time metrics on cards | HIGH | HIGH | P1 |
| Bandwidth on links | HIGH | MEDIUM | P1 |
| Link status (color-coded) | HIGH | LOW | P1 |
| Grafana deep-links | HIGH | LOW | P1 |
| Persistent layout | HIGH | MEDIUM | P1 |
| Visual alerts on map | HIGH | MEDIUM | P1 |
| Dark theme | MEDIUM | LOW | P1 |
| Search / find device | HIGH | LOW | P1 |
| Per-interface statistics | HIGH | MEDIUM | P2 |
| Routing protocol viz (BGP/OSPF) | MEDIUM | HIGH | P2 |
| Background images | MEDIUM | LOW | P2 |
| Link aggregation display | MEDIUM | MEDIUM | P2 |
| Keyboard shortcuts | MEDIUM | LOW | P2 |
| Sub-maps / hierarchy | HIGH | HIGH | P2 |
| Device grouping / tagging | MEDIUM | MEDIUM | P2 |
| Configurable metric display | MEDIUM | MEDIUM | P2 |
| Auto-discovery | HIGH | HIGH | P3 |
| Container/service nesting | MEDIUM | HIGH | P3 |
| Additional device types | MEDIUM | LOW | P3 |
| Geographic map view | LOW | MEDIUM | P3 |
| Multi-user / RBAC | LOW | HIGH | P3 |

**Priority key:**
- P1: Must have for launch
- P2: Should have, add when possible
- P3: Nice to have, future consideration

## Competitor Feature Analysis

| Feature | The Dude | LibreNMS | Zabbix Maps | PRTG | SolarWinds NTM | Our Approach |
|---------|----------|----------|-------------|------|-----------------|--------------|
| Auto-discovery | Yes (subnet scan) | Yes (SNMP/ARP) | Manual only | Yes (SNMP/WMI) | Yes (multi-protocol) | Manual only v1, auto v2 |
| Drag-drop canvas | Yes | Custom Maps only | Yes | Yes (map editor) | Yes | Yes, core feature |
| Device status on map | Yes (color/icon) | Yes (color) | Yes (triggers) | Yes (color) | Static export only | Yes, from Prometheus |
| Link bandwidth display | Yes | Yes (color-coded %) | No (manual labels) | Yes (color + value) | No (static) | Yes, from Prometheus |
| Real-time metrics | Yes | Yes | Yes (via triggers) | Yes | No (mapping only) | Yes, from Prometheus |
| Custom backgrounds | Yes (SVG) | Yes (PNG/JPG) | Yes | Yes (floor plans) | No | Yes (PNG/JPG) |
| Sub-maps / layers | Yes | No | Yes (linked maps) | Yes (multi-layer) | No | v1.x feature |
| Grafana integration | No | Plugin available | Plugin available | No | No | Core feature (deep-links) |
| Prometheus-native | No | No | No | No | No | Yes, unique differentiator |
| Web-based | Partial (web viewer) | Yes | Yes | Yes | No (desktop) | Yes, web-first |
| Dark theme | No | No | No | No | No | Yes, operator preference |
| BGP/OSPF visualization | No | Partial | No | No | No | v1.x feature |
| Multi-vendor SNMP | Yes | Yes | Yes | Yes | Yes | Yes, via snmp_exporter |
| Free / open source | Free (closed) | Yes (GPL) | Yes (GPL) | Freemium | Paid | Open source |

## Sources

- [MikroTik The Dude Downloads](https://mikrotik.com/thedude) -- official feature list (HIGH confidence)
- [The Dude features overview](https://ded9.com/introducing-dude-in-mikrotik/) -- comprehensive feature description (MEDIUM confidence)
- [LibreNMS Custom Map Documentation](https://docs.librenms.org/Extensions/Custom-Map/) -- official docs (HIGH confidence)
- [LibreNMS Network Map Documentation](https://docs.librenms.org/Extensions/Network-Map/) -- official docs (HIGH confidence)
- [Zabbix Network Maps Documentation](https://www.zabbix.com/documentation/current/en/manual/config/visualization/maps) -- official docs (HIGH confidence)
- [Zabbix Roadmap](https://www.zabbix.com/roadmap) -- topology as top-voted feature request (HIGH confidence)
- [PRTG Network Mapping](https://www.paessler.com/monitoring/network/network-mapping-tool) -- official feature page (HIGH confidence)
- [SolarWinds Network Topology Mapper](https://www.solarwinds.com/network-topology-mapper) -- official feature page (HIGH confidence)
- [NetBox Topology Views Plugin](https://github.com/netbox-community/netbox-topology-views) -- community plugin (MEDIUM confidence)
- [Grafana Network Map Panel](https://grafana.com/grafana/plugins/esnet-networkmap-panel/) -- official plugin (HIGH confidence)
- [Network Visualization Key Features 2025](https://www.selector.ai/learning-center/network-visualization-tools-key-features-and-top-6-tools-in-2025/) -- industry overview (MEDIUM confidence)

---
*Feature research for: Network Topology Visualization with Real-Time Monitoring*
*Researched: 2026-03-05*
