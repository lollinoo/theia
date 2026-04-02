# Phase 8: Virtual Device Backend - Context

**Gathered:** 2026-03-31
**Status:** Ready for planning

<domain>
## Phase Boundary

Add backend support for virtual/representative devices as a first-class device type. Virtual devices have no SNMP or Prometheus monitoring — they are user-defined placeholder nodes (Internet, Cloud, Server, Generic). The backend must support creating, storing, and managing virtual devices with appropriate validation, probe behavior, and poller exclusion.

Requirements: VIRT-01 through VIRT-05.

</domain>

<decisions>
## Implementation Decisions

### Node Data Model
- **D-01:** Virtual nodes use a new `DeviceType` value `"virtual"` within the existing `Device` struct. IP, SNMP credentials, and interfaces are optional/empty for virtual nodes. Reuses all existing CRUD, links, positions, and area assignment logic.
- **D-02:** Virtual node subtypes: Internet, Cloud, Server, Generic. Stored as `tags["virtual_subtype"]` on the Device.
- **D-03:** Real devices with IP but no SNMP keep their real DeviceType (switch/router/ap). The system skips SNMP probing and shows no SNMP metrics. Virtual nodes are a distinct type reserved for non-physical concepts.
- **D-04:** Virtual nodes require a user-provided display label at creation time, stored in `tags["display_name"]`.

### Probe Behavior
- **D-05:** Virtual nodes with an IP address are included in the existing Blackbox Exporter `probe_success` status pathway. The MetricsCollector already queries `probe_success{instance=~"<ip>"}` for Prometheus-sourced devices — virtual nodes with IP should be added to that query pool. Status shows green (up) or red (down) based on probe results.
- **D-06:** Virtual nodes without an IP have no probing and status is set to `"unknown"` permanently.
- **D-07:** On creation, virtual nodes with IP get initial status `"unknown"` (not pre-set to "up"). The MetricsCollector will resolve the actual status on its next cycle via `probe_success`.

### API Validation
- **D-08:** POST /api/v1/devices with `device_type: "virtual"` allows empty IP and skips SNMP credential validation.
- **D-09:** Virtual device creation requires both `tags.display_name` (non-empty) and `tags.virtual_subtype` (one of: internet, cloud, server, generic).
- **D-10:** Regular device creation (no device_type or non-virtual) retains the existing `ip is required` validation — no regression.

### Migration
- **D-11:** Migration 000009 converts the existing unique index on `devices(ip)` to a partial unique index `WHERE ip != ''`, allowing multiple virtual devices with empty IP to coexist.

### Link Validation
- **D-12:** Link creation allows empty `if_name` when one device is virtual. Backend stores empty string for the virtual side.
- **D-13:** Link creation rejects both devices being virtual (at least one must have interfaces).

### Claude's Discretion
- Exact implementation of the AddDevice signature change to accept deviceType parameter
- How to thread virtual device IPs into the MetricsCollector probe_success query
- Test structure and mock patterns for new virtual device tests

</decisions>

<canonical_refs>
## Canonical References

**Downstream agents MUST read these before planning or implementing.**

### Domain Model
- `internal/domain/device.go` — Device struct, DeviceType enum, DeviceStatus constants, DeviceRepository interface
- `internal/domain/link.go` — Link struct, LinkRepository interface

### API Layer
- `internal/api/device_handler.go` — Device CRUD HTTP handlers, createDeviceRequest struct, IP validation at line 101
- `internal/api/device_handler_test.go` — Existing test patterns with mockDeviceRepo
- `internal/api/link_handler.go` — Link CRUD HTTP handlers, createLinkRequest struct
- `internal/api/link_handler_test.go` — Existing test patterns with mockLinkRepo

### Service Layer
- `internal/service/device_service.go` — AddDevice (line ~100), probeDevice (line ~139), markDeviceStatus (line ~127), ReprobeDevice (line ~310)

### Workers
- `internal/worker/poller.go` — pollAllDevices (line ~84), Managed skip at line 96
- `internal/worker/poller_test.go` — Existing poller test patterns
- `internal/worker/metrics_collector.go` — QueryProbeStatus integration at line ~612

### Metrics
- `internal/metrics/prometheus.go` — QueryProbeStatus (line ~378), queries `probe_success{instance=~"<ips>"}`

### Migrations
- `internal/repository/sqlite/migrations/000008_multi_area.up.sql` — Latest migration (reference for numbering)

### Previous Planning (read-only reference)
- `.planning/01-virtual-representative-nodes-e-g-internet-without-snmp-prometheus-link-metrics-use-single-real-interface-tx-rx/01-01-PLAN.md` — Previous detailed plan for this exact scope (failed due to squash commit, not implementation issues)
- `.planning/01-virtual-representative-nodes-e-g-internet-without-snmp-prometheus-link-metrics-use-single-real-interface-tx-rx/01-RESEARCH.md` — Previous research document with technical analysis

</canonical_refs>

<code_context>
## Existing Code Insights

### Reusable Assets
- `DeviceType` typed string enum: Extend with `DeviceTypeVirtual` following existing pattern
- `markDeviceStatus()` helper: Reuse for setting virtual device status
- `writeError()` + `decodeJSON()` helpers: Standard API error/decode pattern
- `QueryProbeStatus()`: Already queries Blackbox Exporter for device IPs — extend to include virtual IPs
- `mockDeviceRepo` / `mockLinkRepo`: Existing test mocks for handler tests

### Established Patterns
- DeviceType as typed string constants (`DeviceTypeRouter DeviceType = "router"`)
- `tags` field on Device (`map[string]string`) already used for metadata
- Handler validation: `if req.IP == "" { writeError(w, 400, "ip is required") }` — extend with virtual branch
- Service probe skip: `probeDevice()` already skips SNMP for `MetricsSourcePrometheus` — add virtual skip
- Poller skip: `pollAllDevices()` already skips `!Managed` devices — add virtual skip
- Test pattern: `newTestDeviceHandler(t)` returns handler + mocks

### Integration Points
- `internal/api/device_handler.go` HandleCreate: Add virtual validation branch before SNMP parsing
- `internal/service/device_service.go` AddDevice: Accept deviceType parameter, skip probe for virtual
- `internal/worker/poller.go` pollAllDevices: Add DeviceTypeVirtual skip after Managed check
- `internal/worker/metrics_collector.go`: Include virtual device IPs in probe_success query pool
- `internal/api/link_handler.go` HandleCreate: Relax if_name validation when one device is virtual

</code_context>

<specifics>
## Specific Ideas

- User explicitly wants Internet node to represent WAN/ISP exit traffic
- Server subtype covers cases like NAS or service endpoint visible on network but not SNMP-managed
- Unmanaged switches (IP known, no SNMP) should keep their real device type and show ping status — these are NOT virtual nodes
- Clean rebuild from master — no partial implementation exists in codebase

</specifics>

<deferred>
## Deferred Ideas

None — discussion stayed within phase scope. All backend decisions carried forward from previous attempt's 01-CONTEXT.md.

</deferred>

---

*Phase: 08-virtual-device-backend*
*Context gathered: 2026-03-31*
