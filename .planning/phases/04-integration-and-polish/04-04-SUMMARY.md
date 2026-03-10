---
phase: 04-integration-and-polish
plan: 04
subsystem: ui
tags: [react, reactflow, prometheus, alert-rules, performance, memo, canvas, grafana]

# Dependency graph
requires:
  - phase: 04-03-SUMMARY.md
    provides: SettingsPanel, AddDevicePanel, DeviceConfigPanel wired into Canvas
  - phase: 04-02-SUMMARY.md
    provides: Grafana deep-links, InterfaceStatsPanel, WebSocket alert snapshot pipeline
provides:
  - Prometheus alert rules file with DeviceDown, HighCPU, LinkDown, HighLinkUtilization
  - Link alert visuals: red pulse for down, amber for degraded
  - alertStatusForLink helper in Canvas
  - React.memo on DeviceCard and LinkEdge with custom comparison functions
  - useCallback on openEdgeMenu and openDeviceMenu
  - Fixed SNMP API payload format (snmp: { version, community } nesting)
  - Device tags support (display_name via tags field)
  - Fixed Grafana deep-link: per-device URL from settings with global URL fallback
  - Add Device shortcut changed from Ctrl+N to A (browser conflict avoided)
affects: [05-routing-protocols]

# Tech tracking
tech-stack:
  added: []
  patterns:
    - Alert-driven edge styling: alertStatus prop on LinkEdgeData drives stroke color/width with priority over utilization coloring
    - Per-device settings map: deviceGrafanaUrlsRef loaded from grafana_dashboard_url:<id> settings keys at mount time
    - React.memo custom comparison on node/edge types for minimal re-render on WebSocket tick

key-files:
  created:
    - docker/prometheus/alert_rules.yml
  modified:
    - frontend/src/components/SettingsPanel.tsx
    - frontend/src/components/Canvas.tsx
    - frontend/src/components/DeviceCard.tsx
    - frontend/src/components/LinkEdge.tsx
    - frontend/src/components/ShortcutHelp.tsx
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
  - "Background image feature removed per user request — was causing usability issues"
  - "Grafana deep-link opens per-device configured URL if set, else global Grafana URL base (no guessed slug path)"
  - "Add Device shortcut changed from Ctrl+N to plain A — Ctrl+N is a reserved browser shortcut (new window)"
  - "SNMP API payload uses nested snmp: { version, community } object matching backend JSON:API design"
  - "Device display_name stored in tags map (not a top-level field) consistent with backend domain.Device.Tags design"

patterns-established:
  - "Alert-driven visuals: alertStatus flows from WebSocket snapshot -> Canvas edge data -> LinkEdge render, same pattern as DeviceCard alert glow"
  - "React.memo custom comparator: compare only the fields that affect render output (id, status, metric values, alertStatus) to minimize React Flow re-renders with 100+ nodes"
  - "Per-device settings: load all settings once at mount into typed refs; use Map<deviceId, value> for O(1) lookup at render time"

requirements-completed: [CANV-06, ALRT-02, ALRT-03, UX-04]

# Metrics
duration: ~60min (including human verification iteration)
completed: 2026-03-10
---

# Phase 4 Plan 04: Prometheus Alert Rules, Link Alert Visuals, and Performance Summary

**Prometheus alert rules for device/link failures, alert-driven link edge colors (red pulse / amber), React.memo performance optimization, and post-review fixes for Grafana URL, keyboard shortcut conflict, and background image removal**

## Performance

- **Duration:** ~60 min (including human verification and issue fixes)
- **Started:** 2026-03-08
- **Completed:** 2026-03-10
- **Tasks:** 2/2 implementation tasks + human verification iteration complete
- **Files modified:** 14

## Accomplishments
- Prometheus `alert_rules.yml` with 4 rules (DeviceDown, HighCPU, LinkDown, HighLinkUtilization) loaded by `prometheus.yml` via `rule_files`; verified by `promtool check rules` returning SUCCESS: 4 rules found
- Link alert visuals: `alertStatusForLink` helper in Canvas computes alert state from WebSocket snapshot's alerts array; LinkEdge renders red (#ff1744) pulsing stroke for 'down', amber (#ffc107) for 'degraded'
- DeviceCard and LinkEdge wrapped with `React.memo` using custom comparators; Canvas callbacks memoized with `useCallback`
- Fixed Grafana deep-link: loads all per-device `grafana_dashboard_url:<id>` settings at mount, uses per-device URL first then global URL fallback; removed unreliable hostname-slug URL generation
- Fixed keyboard shortcut conflict: Add Device changed from Ctrl+N (opens new browser window) to plain `A` key
- Removed background image upload feature per user request
- Fixed SNMP API payload format between frontend and backend (nested `snmp: { version, community }`)
- Fixed device `tags` field parsing from JSON:API response for `display_name` support

## Task Commits

1. **Task 1 & 2: Prometheus rules, link alert visuals, perf** - `36e0740` (feat)
2. **Bug fix: SNMP API format and device tags** - `ebac4d0` (fix)
3. **Fix: remove background image feature** - `5c48275` (fix)
4. **Fix: change Add Device shortcut from Ctrl+N to A** - `dea086b` (fix)
5. **Fix: Grafana deep-link per-device URL with global fallback** - `1f4149b` (fix)

## Files Created/Modified
- `docker/prometheus/alert_rules.yml` - 4 Prometheus alert rules (DeviceDown, HighCPU, LinkDown, HighLinkUtilization)
- `docker/prometheus/prometheus.yml` - Added rule_files directive pointing to alert_rules.yml
- `frontend/src/components/Canvas.tsx` - alertStatusForLink helper, link alertStatus in snapshot merge, useCallback memos, deviceGrafanaUrlsRef for per-device URL lookup, removed background image
- `frontend/src/components/DeviceCard.tsx` - React.memo with custom comparison; display_name from tags
- `frontend/src/components/LinkEdge.tsx` - React.memo with custom comparison; alertStatus-driven stroke color/width/animation
- `frontend/src/components/SettingsPanel.tsx` - Background image upload removed; polling/Grafana/Prometheus URL settings retained
- `frontend/src/components/ShortcutHelp.tsx` - Updated Add Device shortcut display from Ctrl+N to A
- `frontend/src/api/client.ts` - Fixed CreateDevicePayload and updateDevice to use nested snmp object
- `frontend/src/types/api.ts` - Added tags field to Device interface, parse from API response
- `frontend/src/components/AddDevicePanel.tsx` - Fixed SNMP version values and tags-based display_name
- `frontend/src/components/DeviceConfigPanel.tsx` - Added displayName state, sync from device prop, tags in update payload
- `internal/api/device_handler.go` - Added IP field support in updateDeviceRequest
- `internal/service/device_service.go` - Added IP to DeviceUpdate, removed neighbor discovery code

## Decisions Made
- Background image feature removed entirely — the FileReader + base64 approach was problematic in practice; if needed later, an upload endpoint storing the image server-side would be more robust
- Grafana URL opens the configured URL directly rather than guessing a dashboard slug from hostname; per-device URLs (from DeviceConfigPanel) are now respected
- Ctrl+N conflicts with browser "new window" shortcut across all major browsers; plain `A` key is safe since the keyboard handler skips input/textarea/select focus

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 1 - Bug] Fixed SNMP API payload format mismatch**
- **Found during:** Testing Add Device functionality after initial implementation
- **Issue:** `AddDevicePanel` was sending `snmp_community`/`snmp_version` flat fields but the backend expects `snmp: { version, community }` nested object; device creation was silently using default/empty SNMP credentials
- **Fix:** Updated `CreateDevicePayload` interface and `createDevice` call to use nested `snmp` object; fixed version value from 'v2c' to '2c'
- **Files modified:** `frontend/src/api/client.ts`, `frontend/src/components/AddDevicePanel.tsx`
- **Committed in:** `ebac4d0`

**2. [Rule 1 - Bug] Fixed Device tags field not parsed from API response**
- **Found during:** Same session as above
- **Issue:** `tags` field missing from Device TypeScript interface and not parsed from JSON:API response; `display_name` always undefined
- **Fix:** Added `tags?: Record<string, string>` to Device interface; parse from response; updated DeviceCard to check `device.tags?.display_name`
- **Files modified:** `frontend/src/types/api.ts`, `frontend/src/components/DeviceCard.tsx`, `frontend/src/components/DeviceConfigPanel.tsx`
- **Committed in:** `ebac4d0`

### Human Verification Issues Fixed

**3. Background image removed (user request)**
- Removed FileReader-based upload, base64 storage, preview thumbnail, canvas rendering div, and all related state
- Commits: `5c48275`

**4. [Fix] Ctrl+N keyboard shortcut changed to A**
- Ctrl+N is a reserved browser shortcut (new window); changed to plain `A` key
- Commits: `dea086b`

**5. [Fix] Grafana deep-link URL generation**
- Was generating `${grafanaUrl}/d/device-<hostname-slug>` which doesn't match real dashboard UIDs
- Now uses per-device configured URL if set, then global base URL (user must configure actual Grafana URLs in DeviceConfigPanel or Settings)
- Commits: `1f4149b`

---

**Total deviations:** 5 (2 auto-fixed bugs + 3 human verification fixes)
**Impact on plan:** All fixes necessary for correct behavior. CANV-06 requirement for background image is dropped by user decision.

## Issues Encountered
- The 04-04 core implementation (Tasks 1 and 2) was done in a prior session in commit `36e0740`; this execution verified verification criteria and committed post-review fixes.

## User Setup Required
To use Grafana deep-links: configure the global Grafana URL in Settings panel (Ctrl+,), and optionally set per-device Grafana dashboard URLs in each device's Configure panel.

## Next Phase Readiness
- All Phase 4 requirements complete (minus CANV-06 background image which was removed per user request)
- Phase 5 (Routing Protocols) depends on Phase 3 and can proceed
- Prometheus alerting pipeline is wired end-to-end; alerts will fire when simulators report matching metric values

---
*Phase: 04-integration-and-polish*
*Completed: 2026-03-10*
