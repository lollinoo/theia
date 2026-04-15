---
phase: 44
slug: frontend-integration
status: verified
nyquist_compliant: true
wave_0_complete: true
created: 2026-04-15
---

# Phase 44 -- Validation Strategy

> Per-phase validation contract for feedback sampling during execution.

---

## Test Infrastructure

| Property | Value |
|----------|-------|
| **Framework** | Go standard `testing` package plus frontend `npm test` |
| **Config file** | None -- existing backend and frontend test conventions |
| **Quick run command** | `rtk bash -lc 'go test ./internal/worker ./internal/api ./internal/service -count=1 && cd frontend && npm test -- src/utils/freshness.test.ts src/components/DeviceCard.test.tsx src/components/canvas/nodeBuilder.test.ts src/components/canvas/useCanvasData.test.ts src/components/DeviceConfigPanel.test.tsx src/api/client.test.ts'` |
| **Full suite command** | `rtk bash -lc 'go test ./internal/worker ./internal/api ./internal/service -count=1 && cd frontend && npm test -- src/utils/freshness.test.ts src/components/DeviceCard.test.tsx src/components/canvas/nodeBuilder.test.ts src/components/canvas/useCanvasData.test.ts src/components/DeviceConfigPanel.test.tsx src/api/client.test.ts'` |
| **Estimated runtime** | ~120 seconds |

---

## Sampling Rate

- **After every task commit:** Run the exact backend or frontend command captured in that task's plan.
- **After every plan wave:** Re-run the accepted backend snapshot/API/service checks and the targeted frontend rendering/interaction suites.
- **Before `/gsd-verify-work`:** The accepted Go and frontend package evidence plus finalized `44-HUMAN-UAT.md` closure must still be valid.
- **Max feedback latency:** 120 seconds

---

## Per-Task Verification Map

| Task ID | Plan | Wave | Requirement | Threat Ref | Secure Behavior | Test Type | Automated Command | Key Artifact | Status |
|---------|------|------|-------------|------------|-----------------|-----------|-------------------|--------------|--------|
| 44-01-T1 | 01 | 1 | WS-01, WS-03, WS-04 | -- | Overview snapshots carry backend-owned health, freshness, and cadence metadata for every device | unit | `cd /home/azmin/projects/theia && rtk go test ./internal/worker -run 'TestBuildPipelineSnapshot' -count=1 -v` | `internal/worker/snapshot_builder.go` | ✅ green |
| 44-01-T2 | 01 | 1 | WS-01, WS-03, WS-04 | -- | Worker regressions lock overview metadata fields and pre-first-poll cadence fallback | unit | `cd /home/azmin/projects/theia && rtk go test ./internal/worker -run 'TestBuildPipelineSnapshot|TestBuildDeviceDetailDelta|TestComputeSnapshotHashes_DeviceMetricHashIncludesDetailFields' -count=1 -v` | `internal/worker/snapshot_builder_test.go` | ✅ green |
| 44-02-T1 | 02 | 1 | POLL-06 | -- | Backend request/response handling preserves keep/clear/set polling override semantics | integration | `cd /home/azmin/projects/theia && rtk go test ./internal/api ./internal/service -run 'Test(DeviceHandlerUpdate_PollIntervalOverride|DeviceHandlerList_IncludesPollClassificationFields|UpdateDevice_PollIntervalOverrideTriState)' -count=1 -v` | `internal/api/device_handler.go`, `internal/service/device_service.go` | ✅ green |
| 44-02-T2 | 02 | 1 | POLL-06 | -- | Frontend parsing and update payloads preserve poll class and override fields on the shared client surface | frontend | `cd /home/azmin/projects/theia/frontend && rtk npm test -- src/api/client.test.ts` | `frontend/src/types/api.ts`, `frontend/src/api/client.ts` | ✅ green |
| 44-03-T1 | 03 | 2 | WS-01, WS-03, WS-04 | -- | Freshness tiers, relative age copy, cadence copy, and health labels stay in a pure helper | frontend | `cd /home/azmin/projects/theia/frontend && rtk npm test -- src/utils/freshness.test.ts` | `frontend/src/utils/freshness.ts` | ✅ green |
| 44-03-T2 | 03 | 2 | WS-01, WS-03, WS-04 | -- | Both device-card branches render backend-owned health/freshness/cadence and preserve metadata through canvas updates | frontend | `cd /home/azmin/projects/theia/frontend && rtk npm test -- src/utils/freshness.test.ts src/components/DeviceCard.test.tsx src/components/canvas/nodeBuilder.test.ts src/components/canvas/useCanvasData.test.ts` | `frontend/src/components/DeviceCard.tsx`, `frontend/src/components/canvas/nodeBuilder.ts`, `frontend/src/components/canvas/useCanvasData.ts` | ✅ green |
| 44-04-T1 | 04 | 2 | POLL-06 | -- | The device configuration panel becomes override-first and device-backed instead of settings-backed | frontend | `cd /home/azmin/projects/theia/frontend && rtk npm test -- src/components/DeviceConfigPanel.test.tsx` | `frontend/src/components/DeviceConfigPanel.tsx` | ✅ green |
| 44-04-T2 | 04 | 2 | POLL-06 | -- | Panel regressions cover default, preset, custom, and clear override flows without page refresh | frontend | `cd /home/azmin/projects/theia/frontend && rtk npm test -- src/components/DeviceConfigPanel.test.tsx` | `frontend/src/components/DeviceConfigPanel.test.tsx` | ✅ green |

*Status: ⬜ pending · ✅ green · ❌ red · ⚠️ flaky*

---

## Wave 0 Requirements

- [x] `internal/worker/snapshot_builder_test.go` covers overview metadata, pre-first-poll fallback, and detail-hash sensitivity.
- [x] `internal/api/device_handler_test.go` and `internal/service/device_service_test.go` cover backend keep/clear/set semantics for polling overrides.
- [x] `frontend/src/utils/freshness.test.ts`, `DeviceCard.test.tsx`, `nodeBuilder.test.ts`, and `useCanvasData.test.ts` cover backend-owned health/freshness/cadence rendering and metadata preservation.
- [x] `frontend/src/components/DeviceConfigPanel.test.tsx` and `frontend/src/api/client.test.ts` cover override-first UI flows and client payload handling.
- [x] No new framework install was required -- the existing backend and frontend harnesses covered the phase.

---

## Manual-Only Verifications

| Behavior | Requirement | Why Manual | Finalized Proof |
|----------|-------------|------------|-----------------|
| Canvas card visual integration | WS-01, WS-03, WS-04 | Visual hierarchy, spacing, and layout preservation require an actual browser check. | `44-HUMAN-UAT.md` records `passed`: physical and virtual cards render consistent backend-driven status, freshness, and cadence metadata after the screenshot-driven regressions were fixed. |
| Polling override interaction flow | POLL-06 | This is an interaction and feedback-flow check rather than a pure code-path assertion. | `44-HUMAN-UAT.md` records `passed`: switching between default, preset, and custom values saved inline, showed `Saved`, kept the panel open, and updated the visible `Polling every ...` label without a page refresh. |
| Next poll cycle runtime effect | POLL-06, WS-04 | End-to-end scheduler timing and websocket propagation require integrated runtime observation. | `44-HUMAN-UAT.md` records `passed`: the saved override propagated through the running system to the canvas without reopening the page. |

Finalized HUMAN-UAT summary: `passed: 3`, `issues: 0`, `pending: 0`, `skipped: 0`, `blocked: 0`.

---

## Validation Sign-Off

- [x] All tasks have `<automated>` verify or Wave 0 dependencies.
- [x] Sampling continuity: no 3 consecutive tasks without automated verify.
- [x] Wave 0 covers backend snapshot/API seams plus frontend rendering and interaction regressions.
- [x] No watch-mode flags were used in accepted evidence.
- [x] Feedback latency remains under 120 seconds for the accepted backend and frontend suites.
- [x] `nyquist_compliant: true` is set in frontmatter.

**Approval:** verified 2026-04-15 -- HUMAN-UAT finalized with `passed: 3`, `issues: 0`, `pending: 0`, `skipped: 0`, and `blocked: 0`.
