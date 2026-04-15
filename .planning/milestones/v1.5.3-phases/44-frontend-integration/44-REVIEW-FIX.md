---
phase: 44
fixed_at: 2026-04-13T19:05:34Z
review_path: /home/azmin/projects/theia/.planning/phases/44-frontend-integration/44-REVIEW.md
iteration: 1
findings_in_scope: 6
fixed: 4
skipped: 2
status: partial
---
# Phase 44: Code Review Fix Report

**Fixed at:** 2026-04-13T19:05:34Z
**Source review:** `/home/azmin/projects/theia/.planning/phases/44-frontend-integration/44-REVIEW.md`
**Iteration:** 1

**Summary:**
- Findings in scope: 6
- Fixed: 4
- Already fixed at HEAD: 2
- Remaining unresolved findings: 0

## Applied Fixes

### WR-02: device-type values accepted by the API are downgraded to `unknown` in the frontend

**Files modified:** `frontend/src/types/api.ts`, `frontend/src/types/api.test.ts`
**Commit:** `bb8bbdb`
**Applied fix:** Extended the frontend device-type model to include `firewall` and normalized backend `access_point` values to frontend `ap` during response parsing.

### WR-04: the Grafana URL field auto-saves invalid values before validation runs

**Files modified:** `frontend/src/components/DeviceConfigPanel.tsx`, `frontend/src/components/DeviceConfigPanel.test.tsx`
**Commit:** `08f7d3c`
**Applied fix:** Moved Grafana URL validation into the debounced autosave path so invalid values stop the save timer and surface a field error before `updateSetting()` runs.

### WR-05: IPv6 addresses are labeled as `MAC` in `DeviceCard`

**Files modified:** `frontend/src/components/DeviceCard.tsx`, `frontend/src/components/DeviceCard.test.tsx`
**Commit:** `50f017e`
**Applied fix:** Replaced the colon-based address heuristic with explicit MAC-address detection so IPv6 addresses keep the `IP` label while actual MAC values still render as `MAC`.

### WR-06: failed legacy-edge migrations permanently delete the stored edge data

**Files modified:** `frontend/src/components/canvas/useCanvasData.ts`, `frontend/src/components/canvas/useCanvasData.test.ts`
**Commit:** `bf0a027`
**Applied fix:** Changed manual-edge migration to keep only the failed edge records in `localStorage` and clear the storage key only after all `createLink()` calls succeed.

## Already Fixed At HEAD

### WR-01: `metrics_source` contract is out of sync across the API and frontend

**Files checked:** `internal/api/device_handler.go`, `frontend/src/types/api.ts`
**Reason:** Already fixed at current HEAD. The backend `validMetricsSources` allowlist already accepts `prometheus_snmp_fallback` and `none`, and the frontend still parses those values.

### WR-03: virtual devices without an IP cannot be saved from the config panel

**Files checked:** `frontend/src/components/DeviceConfigPanel.tsx`, `internal/api/device_handler.go`
**Reason:** Already fixed at current HEAD. `handleEditSave()` already skips empty-IP validation for virtual devices and the backend still accepts empty-IP virtual saves.

## Tests Run

- `rtk npm --prefix frontend test -- src/types/api.test.ts` — passed
- `rtk npm --prefix frontend test -- src/components/DeviceConfigPanel.test.tsx` — passed
- `rtk npm --prefix frontend test -- src/components/DeviceCard.test.tsx` — passed
- `rtk npm --prefix frontend test -- src/components/canvas/useCanvasData.test.ts` — passed
- `rtk npm --prefix frontend test -- src/types/api.test.ts src/components/DeviceConfigPanel.test.tsx src/components/DeviceCard.test.tsx src/components/canvas/useCanvasData.test.ts` — passed

## Remaining Findings

None. All in-scope warnings are either fixed in this pass or verified as already fixed at current HEAD.

---

_Fixed: 2026-04-13T19:05:34Z_
_Fixer: Codex (gsd-code-fixer)_
_Iteration: 1_
