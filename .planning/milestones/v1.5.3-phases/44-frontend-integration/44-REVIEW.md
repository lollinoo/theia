---
phase: 44-frontend-integration
reviewed: 2026-04-13T17:41:10Z
depth: standard
files_reviewed: 19
files_reviewed_list:
  - internal/worker/snapshot_builder.go
  - internal/worker/snapshot_builder_test.go
  - internal/api/device_handler.go
  - internal/api/device_handler_test.go
  - internal/service/device_service.go
  - internal/service/device_service_test.go
  - frontend/src/types/api.ts
  - frontend/src/api/client.ts
  - frontend/src/api/client.test.ts
  - frontend/src/utils/freshness.ts
  - frontend/src/utils/freshness.test.ts
  - frontend/src/components/DeviceCard.tsx
  - frontend/src/components/DeviceCard.test.tsx
  - frontend/src/components/canvas/nodeBuilder.ts
  - frontend/src/components/canvas/nodeBuilder.test.ts
  - frontend/src/components/canvas/useCanvasData.ts
  - frontend/src/components/canvas/useCanvasData.test.ts
  - frontend/src/components/DeviceConfigPanel.tsx
  - frontend/src/components/DeviceConfigPanel.test.tsx
findings:
  critical: 0
  warning: 6
  info: 0
  total: 6
status: issues_found
---
# Phase 44: Code Review Report

**Reviewed:** 2026-04-13T17:41:10Z
**Depth:** standard
**Files Reviewed:** 19
**Status:** issues_found

## Summary

Reviewed the backend device/snapshot integration and the corresponding frontend API, canvas, and device-config changes. No critical security issues stood out, but there are several correctness regressions where the frontend and backend contracts diverge, plus a few user-facing bugs in the new device-config and canvas flows.

Static review only; tests were not executed as part of this review.

## Warnings

### WR-01: `metrics_source` contract is out of sync across the API and frontend

**Files:** `internal/api/device_handler.go:54-55`, `frontend/src/types/api.ts:34`, `frontend/src/types/api.ts:191-195`, `frontend/src/components/DeviceConfigPanel.tsx:79-80`, `frontend/src/components/DeviceConfigPanel.tsx:720-736`

**Issue:** The backend still validates `metrics_source` against only `prometheus` and `snmp`, but the frontend parses and exposes both `prometheus_snmp_fallback` and `none`. As a result, selecting "Prometheus + SNMP Fallback" in the device config panel, or resaving a virtual no-IP device whose current source is `none`, will produce a `400 invalid metrics_source`.

**Fix:**
```go
var validMetricsSources = map[string]bool{
	"prometheus":                true,
	"snmp":                      true,
	"prometheus_snmp_fallback":  true,
	"none":                      true,
	"":                          true,
}
```
If `none` is meant to stay internal-only, then strip it from frontend saves instead of advertising it in the shared API types.

### WR-02: device-type values accepted by the API are downgraded to `unknown` in the frontend

**Files:** `internal/api/device_handler.go:49-52`, `frontend/src/types/api.ts:1`, `frontend/src/types/api.ts:119-128`

**Issue:** The API accepts `access_point` and `firewall`, but the frontend only models `router | switch | ap | virtual | unknown`. Any device returned as `access_point` or `firewall` will therefore be parsed as `unknown`, which breaks type-specific UI behavior and makes the API contract inconsistent.

**Fix:**
```ts
export type DeviceType =
  | 'router'
  | 'switch'
  | 'ap'
  | 'firewall'
  | 'virtual'
  | 'unknown';

function parseDeviceType(value: unknown): DeviceType {
  switch (value) {
    case 'access_point':
      return 'ap';
    case 'router':
    case 'switch':
    case 'ap':
    case 'firewall':
    case 'virtual':
      return value;
    default:
      return 'unknown';
  }
}
```
Alternatively, tighten the backend allowlist so it only emits values the frontend already supports.

### WR-03: virtual devices without an IP cannot be saved from the config panel

**Files:** `frontend/src/components/DeviceConfigPanel.tsx:290-291`, `frontend/src/components/DeviceConfigPanel.tsx:320-323`, `internal/api/device_handler.go:152-153`, `internal/api/device_handler.go:342-345`

**Issue:** The backend explicitly allows virtual devices with an empty IP, but `handleEditSave` always runs `validateIPOrHostname(ip.trim())`, which rejects `''` before any request is sent. That makes the device-config form unusable for the no-IP virtual nodes introduced in this phase.

**Fix:**
```ts
const trimmedIP = ip.trim();
if (!(isVirtual && trimmedIP === '')) {
  const ipErr = validateIPOrHostname(trimmedIP);
  if (ipErr) errors.ip = ipErr;
}

const updated = await updateDevice(device.id, {
  hostname: device.hostname,
  ip: trimmedIP,
  // ...
});
```

### WR-04: the Grafana URL field auto-saves invalid values before validation runs

**Files:** `frontend/src/components/DeviceConfigPanel.tsx:275-282`, `frontend/src/components/DeviceConfigPanel.tsx:473-477`

**Issue:** The Grafana URL input schedules `updateSetting()` on every change, while validation only runs on blur. That means an invalid value such as `not-a-url` can be persisted 500 ms after typing, even though the field later shows a validation error.

**Fix:**
```ts
function scheduleGrafanaUpdate(value: string) {
  if (grafanaTimerRef.current !== null) window.clearTimeout(grafanaTimerRef.current);

  const err = value.trim() === '' ? null : validateURL(value, 'Grafana URL');
  if (err) {
    setFieldError('grafanaUrl', err);
    return;
  }

  grafanaTimerRef.current = window.setTimeout(() => {
    void updateSetting(grafanaKey, value).then(() => {
      showSaved(setSavedGrafana, savedGrafanaTimerRef);
      onSettingsChange?.();
    });
  }, 500);
}
```

### WR-05: IPv6 addresses are labeled as `MAC` in `DeviceCard`

**File:** `frontend/src/components/DeviceCard.tsx:273-274`

**Issue:** The card uses `ip.includes(':') && !ip.includes('.')` to decide between `MAC` and `IP`. Every pure IPv6 address matches that heuristic, so valid IPv6 devices render with a `MAC:` label.

**Fix:**
```ts
const macPattern = /^([0-9A-Fa-f]{2}[:-]){5}[0-9A-Fa-f]{2}$/;
const addressLabel = macPattern.test(data.device.ip) ? 'MAC' : 'IP';
```

### WR-06: failed legacy-edge migrations permanently delete the stored edge data

**File:** `frontend/src/components/canvas/useCanvasData.ts:183-207`

**Issue:** The migration from `manualEdgeStorageKey` catches and ignores per-edge `createLink()` failures, then unconditionally removes the localStorage entry. If the backend is unavailable or only some calls fail, the user loses those manually created edges with no retry path.

**Fix:**
```ts
const results = await Promise.allSettled(
  storedManual.map((edge) =>
    createLink({
      source_device_id: edge.source,
      source_if_name: '',
      target_device_id: edge.target,
      target_if_name: '',
    }),
  ),
);

const failed = storedManual.filter((_, i) => results[i].status === 'rejected');
if (failed.length === 0) {
  window.localStorage.removeItem(manualEdgeStorageKey);
} else {
  window.localStorage.setItem(manualEdgeStorageKey, JSON.stringify(failed));
}
```

---

_Reviewed: 2026-04-13T17:41:10Z_
_Reviewer: Claude (gsd-code-reviewer)_
_Depth: standard_
