---
phase: 04-integration-and-polish
plan: 04
subsystem: ui
tags: [react, reactflow, prometheus, alert-rules, background-image, performance, memo, canvas]

# Dependency graph
requires:
  - phase: 04-03-SUMMARY.md
    provides: SettingsPanel, AddDevicePanel, DeviceConfigPanel wired into Canvas
  - phase: 04-02-SUMMARY.md
    provides: Grafana deep-links, InterfaceStatsPanel, WebSocket alert snapshot pipeline
provides:
  - Canvas background image upload stored as base64 in settings API
  - Prometheus alert rules file with DeviceDown, HighCPU, LinkDown, HighLinkUtilization
  - Link alert visuals: red pulse for down, amber for degraded
  - alertStatusForLink helper in Canvas
  - React.memo on DeviceCard and LinkEdge with custom comparison functions
  - useCallback on openEdgeMenu and openDeviceMenu
  - Fixed SNMP API payload format (snmp: { version, community } nesting)
  - Device tags support (display_name via tags field)
affects: [05-routing-protocols]

# Tech tracking
tech-stack:
  added: []
  patterns:
    - Alert-driven edge styling: alertStatus prop on LinkEdgeData drives stroke color/width with priority over utilization coloring
    - Background image rendering: z-index 0 div with backgroundImage CSS behind React Flow z-index 1+, pointer-events none
    - React.memo custom comparison on node/edge types for minimal re-render on WebSocket tick

key-files:
  created:
    - docker/prometheus/alert_rules.yml
  modified:
    - frontend/src/components/SettingsPanel.tsx
    - frontend/src/components/Canvas.tsx
    - frontend/src/components/DeviceCard.tsx
    - frontend/src/components/LinkEdge.tsx
    - frontend/src/api/client.ts
    - frontend/src/types/api.ts
    - frontend/src/components/AddDevicePanel.tsx
    - frontend/src/components/DeviceConfigPanel.tsx
    - docker/prometheus/prometheus.yml
    - internal/api/device_handler.go
    - internal/service/device_service.go

key-decisions:
  - "LinkDown alert severity is warning (not critical) — a link down is less severe than a device going completely unreachable"
  - "Link alert status uses best-effort interface name matching via alert summary string (Prometheus labels vary by exporter config)"
  - "Background image rendered as a z-index 0 positioned div with 0.15 opacity so topology nodes/links remain readable"
  - "SNMP API payload uses nested snmp: { version, community } object matching backend JSON:API design; flat snmp_community/snmp_version fields were wrong"
  - "Device display_name stored in tags map (not a top-level field) consistent with backend domain.Device.Tags design"

patterns-established:
  - "Alert-driven visuals: alertStatus flows from WebSocket snapshot -> Canvas edge data -> LinkEdge render, same pattern as DeviceCard alert glow"
  - "React.memo custom comparator: compare only the fields that affect render output (id, status, metric values, alertStatus) to minimize React Flow re-renders with 100+ nodes"

requirements-completed: [CANV-06, ALRT-02, ALRT-03, UX-04]

# Metrics
duration: ~30min
completed: 2026-03-10
---

# Phase 4 Plan 04: Background Image, Alert Rules, Link Alert Visuals, and Performance Summary

**Canvas background image upload to base64 settings, Prometheus alert rules for device/link failures, alert-driven link edge colors (red pulse / amber), and React.memo performance optimization for 100+ devices**

## Performance

- **Duration:** ~30 min
- **Started:** 2026-03-08
- **Completed:** 2026-03-10
- **Tasks:** 2/2 implementation tasks complete (Task 3 is human verification checkpoint)
- **Files modified:** 11

## Accomplishments
- Background image upload in SettingsPanel: FileReader reads image as base64 data URL, stored in settings API under `canvas_background_image`, rendered as CSS background behind React Flow with 0.15 opacity
- Prometheus `alert_rules.yml` with 4 rules (DeviceDown, HighCPU, LinkDown, HighLinkUtilization) loaded by `prometheus.yml` via `rule_files`; verified by `promtool check rules` returning SUCCESS: 4 rules found
- Link alert visuals: `alertStatusForLink` helper in Canvas computes alert state from WebSocket snapshot's alerts array; LinkEdge renders red (#ff1744) pulsing stroke for 'down', amber (#ffc107) for 'degraded'
- DeviceCard and LinkEdge wrapped with `React.memo` using custom comparators; Canvas callbacks memoized with `useCallback`
- Bug fix: SNMP API payload format aligned between frontend (AddDevicePanel, DeviceConfigPanel) and backend (nested `snmp: { version, community }` object)
- Bug fix: Device `tags` field parsed from API response and used for `display_name` in DeviceCard

## Task Commits

1. **Task 1 & 2: Background image, Prometheus rules, link alert visuals, perf** - `36e0740` (feat)
2. **Bug fix: SNMP API format and device tags** - `ebac4d0` (fix)

## Files Created/Modified
- `docker/prometheus/alert_rules.yml` - 4 Prometheus alert rules (DeviceDown, HighCPU, LinkDown, HighLinkUtilization)
- `docker/prometheus/prometheus.yml` - Added rule_files directive pointing to alert_rules.yml
- `frontend/src/components/SettingsPanel.tsx` - Background image upload section with FileReader, preview thumbnail, remove button
- `frontend/src/components/Canvas.tsx` - Background image CSS layer, alertStatusForLink helper, link alertStatus in snapshot merge, useCallback memos
- `frontend/src/components/DeviceCard.tsx` - React.memo with custom comparison; display_name from tags
- `frontend/src/components/LinkEdge.tsx` - React.memo with custom comparison; alertStatus-driven stroke color/width/animation
- `frontend/src/api/client.ts` - Fixed CreateDevicePayload and updateDevice to use nested snmp object
- `frontend/src/types/api.ts` - Added tags field to Device interface, parse from API response
- `frontend/src/components/AddDevicePanel.tsx` - Fixed SNMP version values and tags-based display_name
- `frontend/src/components/DeviceConfigPanel.tsx` - Added displayName state, sync from device prop, tags in update payload
- `internal/api/device_handler.go` - Added IP field support in updateDeviceRequest
- `internal/service/device_service.go` - Added IP to DeviceUpdate, removed neighbor discovery code

## Decisions Made
- LinkDown severity is `warning` not `critical` — a link going down is less severe than a device becoming entirely unreachable
- Alert-to-link matching uses best-effort summary string matching because Prometheus label structure varies across SNMP exporters
- Background image at 0.15 opacity behind React Flow so it doesn't overwhelm the topology visualization

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 1 - Bug] Fixed SNMP API payload format mismatch**
- **Found during:** Testing Add Device functionality after Task 2 completion
- **Issue:** `AddDevicePanel` was sending `snmp_community`/`snmp_version` flat fields but the backend `createDevice` handler expects `snmp: { version, community }` nested object; device creation was silently using default/empty SNMP credentials
- **Fix:** Updated `CreateDevicePayload` interface and `createDevice` call in AddDevicePanel to use nested `snmp` object; fixed version value from 'v2c' to '2c' to match backend enum
- **Files modified:** `frontend/src/api/client.ts`, `frontend/src/components/AddDevicePanel.tsx`
- **Verification:** TypeScript check passes, build succeeds
- **Committed in:** `ebac4d0`

**2. [Rule 1 - Bug] Fixed Device tags field not parsed from API response**
- **Found during:** Same bug fix session
- **Issue:** `tags` field was not in Device TypeScript interface and not parsed from JSON:API response, so `display_name` tag was always undefined; DeviceCard was always showing hostname instead of user-configured display name
- **Fix:** Added `tags?: Record<string, string>` to Device interface; parse tags from response attributes; updated DeviceCard `displayName()` to check `device.tags?.display_name` first
- **Files modified:** `frontend/src/types/api.ts`, `frontend/src/components/DeviceCard.tsx`, `frontend/src/components/DeviceConfigPanel.tsx`
- **Verification:** TypeScript check passes, build succeeds
- **Committed in:** `ebac4d0`

---

**Total deviations:** 2 auto-fixed (both Rule 1 - Bug)
**Impact on plan:** Essential corrections for correctness - the Add Device and Edit Device features would silently produce wrong SNMP credentials without these fixes. No scope creep.

## Issues Encountered
- The 04-04 core implementation (Tasks 1 and 2) was done in a prior session in commit `36e0740`; this execution verified all verification criteria still passed and committed outstanding bug fixes.

## User Setup Required
None - no external service configuration required.

## Next Phase Readiness
- All Phase 4 requirements complete: context menus, side panels, Grafana links, interface stats, polling config, add/edit/delete device, keyboard shortcuts, background image, alert visuals, performance
- Phase 5 (Routing Protocols) depends on Phase 3 and can proceed independently
- Prometheus alerting pipeline is wired end-to-end; alerts will fire when simulators report matching metric values

---
*Phase: 04-integration-and-polish*
*Completed: 2026-03-10*
