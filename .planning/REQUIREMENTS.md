# Requirements: MikroTik Theia

**Defined:** 2026-03-05
**Core Value:** Network operators can see their entire topology at a glance with live stats on every device and link, and drill into Grafana for deep dives

## v1 Requirements

Requirements for initial release. Each maps to roadmap phases.

### Canvas

- [ ] **CANV-01**: User can pan and zoom the topology canvas freely
- [ ] **CANV-02**: User can drag devices to reposition them on the canvas
- [ ] **CANV-03**: Device positions persist across browser sessions
- [ ] **CANV-04**: Auto-layout algorithm initially positions nodes based on topology connections
- [ ] **CANV-05**: User can manually override auto-layout positions per device
- [ ] **CANV-06**: User can upload a background image (floor plan/network diagram) to the canvas

### Devices

- [ ] **DEV-01**: User can add a device by IP/hostname with SNMP credentials
- [ ] **DEV-02**: Device cards display hostname, IP, and hardware model
- [ ] **DEV-03**: Device cards show a type icon (Router, Switch, AP) with visual differentiation
- [ ] **DEV-04**: Device cards show a status indicator (up/down/degraded)
- [ ] **DEV-05**: User can edit device properties after creation
- [ ] **DEV-06**: User can remove a device from the topology

### Links

- [ ] **LINK-01**: Links between devices are visualized as lines on the canvas
- [ ] **LINK-02**: Links display bandwidth capacity labels
- [ ] **LINK-03**: Links show live throughput (TX/RX) from Prometheus metrics
- [ ] **LINK-04**: Links are color-coded by utilization level
- [ ] **LINK-05**: User can click a link to see per-interface statistics (TX/RX, errors, drops)

### Metrics

- [ ] **METR-01**: Device cards display live CPU utilization from Prometheus
- [ ] **METR-02**: Device cards display live memory utilization from Prometheus
- [ ] **METR-03**: Device cards display device uptime from Prometheus
- [ ] **METR-04**: Device cards display temperature from Prometheus (where available)
- [ ] **METR-05**: Metrics update in real-time via WebSocket push
- [ ] **METR-06**: User can configure global polling interval
- [ ] **METR-07**: User can configure per-device polling interval override

### Alerts

- [ ] **ALRT-01**: Devices visually change (color/icon) when they go down
- [ ] **ALRT-02**: Links visually change when they go down or degrade
- [ ] **ALRT-03**: Alert states reflect Prometheus alerting rules

### Integration

- [ ] **INTG-01**: All metrics are sourced from an existing Prometheus instance via PromQL
- [ ] **INTG-02**: User can click a device to open its Grafana dashboard in a new tab
- [ ] **INTG-03**: User can click a metric to open the relevant Grafana panel
- [ ] **INTG-04**: SNMP is used for topology discovery (LLDP/CDP neighbors, interfaces)
- [ ] **INTG-05**: Multi-vendor support — works with any device exposing standard SNMP MIBs

### Routing

- [ ] **ROUT-01**: User can view BGP session status and neighbor details per device
- [ ] **ROUT-02**: User can view OSPF neighbor status per device
- [ ] **ROUT-03**: User can view route count summaries per device

### UX

- [ ] **UX-01**: Dark theme UI matching the reference design
- [ ] **UX-02**: User can search for devices by hostname or IP and canvas focuses on result
- [ ] **UX-03**: Keyboard shortcuts for common actions (search, add device, zoom)
- [ ] **UX-04**: Canvas supports 100+ devices without performance degradation

## v2 Requirements

Deferred to future release. Tracked but not in current roadmap.

### Discovery

- **DISC-01**: Auto-discovery of devices via subnet scanning
- **DISC-02**: Automatic topology population from LLDP/CDP data without manual add

### Advanced Visualization

- **ADVZ-01**: Link aggregation (LAG/LACP) visualization
- **ADVZ-02**: Sub-maps for hierarchical topology views
- **ADVZ-03**: Device grouping/tagging with visual clusters

### Multi-User

- **USER-01**: Multiple users can view the topology simultaneously
- **USER-02**: Role-based access control (viewer, editor, admin)

## Out of Scope

| Feature | Reason |
|---------|--------|
| Container/service nesting in device cards | Separate domain — deferred to v2 |
| Server, NAS, PC, SBC, UPS device types | Routers/switches/APs first |
| Push notifications / alerting | Visual status only — alerting stays in Prometheus/Alertmanager |
| Replacing Grafana | Complements Grafana, not replaces it |
| Mobile app | Web-first for v1 |
| Embedded Grafana panels (iframes) | Fragile, link out instead |
| Component palette sidebar | Simplified add flow for v1 |
| Configuration management | Separate domain entirely |
| Built-in alerting engine | Duplicates Alertmanager |

## Traceability

Which phases cover which requirements. Updated during roadmap creation.

| Requirement | Phase | Status |
|-------------|-------|--------|
| CANV-01 | Phase 2 | Pending |
| CANV-02 | Phase 2 | Pending |
| CANV-03 | Phase 2 | Pending |
| CANV-04 | Phase 2 | Pending |
| CANV-05 | Phase 2 | Pending |
| CANV-06 | Phase 2 | Pending |
| DEV-01 | Phase 1 | Pending |
| DEV-02 | Phase 1 | Pending |
| DEV-03 | Phase 2 | Pending |
| DEV-04 | Phase 2 | Pending |
| DEV-05 | Phase 1 | Pending |
| DEV-06 | Phase 1 | Pending |
| LINK-01 | Phase 2 | Pending |
| LINK-02 | Phase 2 | Pending |
| LINK-03 | Phase 3 | Pending |
| LINK-04 | Phase 3 | Pending |
| LINK-05 | Phase 4 | Pending |
| METR-01 | Phase 3 | Pending |
| METR-02 | Phase 3 | Pending |
| METR-03 | Phase 3 | Pending |
| METR-04 | Phase 3 | Pending |
| METR-05 | Phase 3 | Pending |
| METR-06 | Phase 4 | Pending |
| METR-07 | Phase 4 | Pending |
| ALRT-01 | Phase 3 | Pending |
| ALRT-02 | Phase 3 | Pending |
| ALRT-03 | Phase 3 | Pending |
| INTG-01 | Phase 3 | Pending |
| INTG-02 | Phase 4 | Pending |
| INTG-03 | Phase 4 | Pending |
| INTG-04 | Phase 1 | Pending |
| INTG-05 | Phase 1 | Pending |
| ROUT-01 | Phase 5 | Pending |
| ROUT-02 | Phase 5 | Pending |
| ROUT-03 | Phase 5 | Pending |
| UX-01 | Phase 2 | Pending |
| UX-02 | Phase 2 | Pending |
| UX-03 | Phase 4 | Pending |
| UX-04 | Phase 2 | Pending |

**Coverage:**
- v1 requirements: 39 total
- Mapped to phases: 39
- Unmapped: 0

---
*Requirements defined: 2026-03-05*
*Last updated: 2026-03-05 after roadmap creation*
