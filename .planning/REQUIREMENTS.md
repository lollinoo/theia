# Requirements: MikroTik Theia

**Defined:** 2026-03-05
**Core Value:** Network operators can see their entire topology at a glance with live stats on every device and link, and drill into Grafana for deep dives

## v1 Requirements

Requirements for initial release. Each maps to roadmap phases.

### Canvas

- [x] **CANV-01**: User can pan and zoom the topology canvas freely
- [x] **CANV-02**: User can drag devices to reposition them on the canvas
- [x] **CANV-03**: Device positions persist across browser sessions
- [x] **CANV-04**: Auto-layout algorithm initially positions nodes based on topology connections
- [x] **CANV-05**: User can manually override auto-layout positions per device
- [ ] **CANV-06**: User can upload a background image (floor plan/network diagram) to the canvas

### Devices

- [x] **DEV-01**: User can add a device by IP/hostname with SNMP credentials
- [x] **DEV-02**: Device cards display hostname, IP, and hardware model
- [x] **DEV-03**: Device cards show a type icon (Router, Switch, AP) with visual differentiation
- [x] **DEV-04**: Device cards show a status indicator (up/down/degraded)
- [x] **DEV-05**: User can edit device properties after creation
- [x] **DEV-06**: User can remove a device from the topology

### Links

- [x] **LINK-01**: Links between devices are visualized as lines on the canvas
- [x] **LINK-02**: Links display bandwidth capacity labels
- [x] **LINK-03**: Links show live throughput (TX/RX) from Prometheus metrics
- [x] **LINK-04**: Links are color-coded by utilization level
- [ ] **LINK-05**: User can click a link to see per-interface statistics (TX/RX, errors, drops)

### Metrics

- [x] **METR-01**: Device cards display live CPU utilization from Prometheus
- [x] **METR-02**: Device cards display live memory utilization from Prometheus
- [x] **METR-03**: Device cards display device uptime from Prometheus
- [x] **METR-04**: Device cards display temperature from Prometheus (where available)
- [x] **METR-05**: Metrics update in real-time via WebSocket push
- [ ] **METR-06**: User can configure global polling interval
- [ ] **METR-07**: User can configure per-device polling interval override

### Alerts

- [x] **ALRT-01**: Devices visually change (color/icon) when they go down
- [ ] **ALRT-02**: Links visually change when they go down or degrade
- [ ] **ALRT-03**: Alert states reflect Prometheus alerting rules

### Integration

- [x] **INTG-01**: All metrics are sourced from an existing Prometheus instance via PromQL
- [ ] **INTG-02**: User can click a device to open its Grafana dashboard in a new tab
- [ ] **INTG-03**: User can click a metric to open the relevant Grafana panel
- [x] **INTG-04**: SNMP is used for topology discovery (LLDP/CDP neighbors, interfaces)
- [x] **INTG-05**: Multi-vendor support — works with any device exposing standard SNMP MIBs

### Routing

- [ ] **ROUT-01**: User can view BGP session status and neighbor details per device
- [ ] **ROUT-02**: User can view OSPF neighbor status per device
- [ ] **ROUT-03**: User can view route count summaries per device

### UX

- [x] **UX-01**: Dark theme UI matching the reference design
- [x] **UX-02**: User can search for devices by hostname or IP and canvas focuses on result
- [x] **UX-03**: Keyboard shortcuts for common actions (search, add device, zoom)
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
| CANV-01 | Phase 2 | Complete |
| CANV-02 | Phase 2 | Complete |
| CANV-03 | Phase 2 | Complete |
| CANV-04 | Phase 2 | Complete |
| CANV-05 | Phase 2 | Complete |
| CANV-06 | Phase 4 | Pending |
| DEV-01 | Phase 1 | Complete |
| DEV-02 | Phase 1 | Complete |
| DEV-03 | Phase 2 | Complete |
| DEV-04 | Phase 2 | Complete |
| DEV-05 | Phase 1 | Complete |
| DEV-06 | Phase 1 | Complete |
| LINK-01 | Phase 2 | Complete |
| LINK-02 | Phase 2 | Complete |
| LINK-03 | Phase 3 | Complete |
| LINK-04 | Phase 3 | Complete |
| LINK-05 | Phase 4 | Pending |
| METR-01 | Phase 3 | Complete |
| METR-02 | Phase 3 | Complete |
| METR-03 | Phase 3 | Complete |
| METR-04 | Phase 3 | Complete |
| METR-05 | Phase 3 | Complete |
| METR-06 | Phase 4 | Pending |
| METR-07 | Phase 4 | Pending |
| ALRT-01 | Phase 3 | Complete |
| ALRT-02 | Phase 4 | Pending |
| ALRT-03 | Phase 4 | Pending |
| INTG-01 | Phase 3 | Complete |
| INTG-02 | Phase 4 | Pending |
| INTG-03 | Phase 4 | Pending |
| INTG-04 | Phase 1 | Complete |
| INTG-05 | Phase 1 | Complete |
| ROUT-01 | Phase 5 | Pending |
| ROUT-02 | Phase 5 | Pending |
| ROUT-03 | Phase 5 | Pending |
| UX-01 | Phase 2 | Complete |
| UX-02 | Phase 2 | Complete |
| UX-03 | Phase 4 | Complete |
| UX-04 | Phase 4 | Pending |

**Coverage:**
- v1 requirements: 39 total
- Mapped to phases: 39
- Unmapped: 0

---
*Requirements defined: 2026-03-05*
*Last updated: 2026-03-07 after Phase 3 completion review*
