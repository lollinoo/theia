---
phase: 46
slug: detail-delta-gap-closure
status: verified
nyquist_compliant: true
wave_0_complete: true
created: 2026-04-15
---

# Phase 46 -- Validation Strategy

> Per-phase validation contract for feedback sampling during execution.

---

## Test Infrastructure

| Property | Value |
|----------|-------|
| **Framework** | Go standard `testing` package plus frontend `npm test` |
| **Config file** | None -- existing backend and frontend test conventions |
| **Quick run command** | `rtk bash -lc 'go test ./internal/worker -count=1 && cd frontend && npm test -- src/types/metrics.test.ts src/hooks/useWebSocket.test.ts'` |
| **Full suite command** | `rtk bash -lc 'go test ./... && cd frontend && npm test -- src/types/metrics.test.ts src/hooks/useWebSocket.test.ts'` |
| **Estimated runtime** | ~120 seconds |

---

## Sampling Rate

- **After every task commit:** Run the exact task command captured in the phase plan.
- **After every plan wave:** Re-run the accepted worker and frontend targeted-merge checks.
- **Before `/gsd-verify-work`:** The accepted automated evidence and finalized `46-HUMAN-UAT.md` closure must both remain valid.
- **Max feedback latency:** 120 seconds

---

## Per-Task Verification Map

| Task ID | Plan | Wave | Requirement | Threat Ref | Secure Behavior | Test Type | Automated Command | Key Artifact | Status |
|---------|------|------|-------------|------------|-----------------|-----------|-------------------|--------------|--------|
| 46-01-T1 | 01 | 1 | WS-02 | -- | Targeted detail deltas include only selected-device `link_metrics` without widening the payload | integration | `cd /home/azmin/projects/theia && rtk go test ./internal/worker -run 'TestBuildDeviceDetailDelta_(EmbedsOptionalDetailFieldsInDeviceMetrics|IncludesSelectedDeviceLinkMetricsOnly)' -count=1 -v` | `internal/worker/snapshot_builder.go` | ✅ green |
| 46-01-T2 | 01 | 1 | WS-02 | -- | Subscribed clients receive selected-device link metrics immediately after the poll while unsubscribed clients remain unaffected | integration | `cd /home/azmin/projects/theia && rtk go test ./internal/worker -run 'TestPipelineOrchestratorRunTask_(PerformancePollSendsOnlySelectedDeviceLinkMetricsToSubscribedClient|DetailDeltaKeepsPerformanceFreshnessMetadataAfterOperationalPoll|DetailDeltaDoesNotReachUnsubscribedClient)' -count=1 -v` | `internal/worker/pipeline.go` | ✅ green |
| 46-01-T3 | 01 | 1 | WS-02 | -- | Frontend shared-snapshot merge keeps sparse targeted `link_metrics` deltas from clearing unrelated device state | frontend | `cd /home/azmin/projects/theia/frontend && rtk npm test -- src/types/metrics.test.ts src/hooks/useWebSocket.test.ts` | `frontend/src/types/metrics.ts`, `frontend/src/hooks/useWebSocket.test.ts` | ✅ green |

*Status: ⬜ pending · ✅ green · ❌ red · ⚠️ flaky*

---

## Wave 0 Requirements

- [x] `internal/worker/snapshot_builder_test.go` covers targeted detail payload shape and selected-device `link_metrics` inclusion.
- [x] `internal/worker/pipeline_test.go` covers subscribed-only delivery, no-leak unsubscribed behavior, and preservation across later operational detail sends.
- [x] `frontend/src/types/metrics.test.ts` and `frontend/src/hooks/useWebSocket.test.ts` cover sparse targeted `snapshot_delta` merges for `link_metrics`.
- [x] `.planning/phases/46-detail-delta-gap-closure/46-REVIEW.md` is already `clean`, so the phase enters validation with no advisory review findings.
- [x] No new framework install was required -- the existing backend and frontend harnesses covered the phase.

---

## Manual-Only Verifications

| Behavior | Requirement | Why Manual | Finalized Proof |
|----------|-------------|------------|-----------------|
| Selected-device interface panel refreshes on targeted detail delta | WS-02 | Requires observing the live browser/websocket session and ensuring an unsubscribed client still receives no targeted payload. | `46-HUMAN-UAT.md` records `passed`: live verification confirmed the selected device panel refreshed TX/RX/utilization immediately after the targeted detail update without waiting for an overview broadcast. |

Finalized HUMAN-UAT summary: `passed: 1`, `issues: 0`, `pending: 0`, `skipped: 0`, `blocked: 0`.

---

## Validation Sign-Off

- [x] All tasks have `<automated>` verify or Wave 0 dependencies.
- [x] Sampling continuity: no 3 consecutive tasks without automated verify.
- [x] Wave 0 covers backend targeted-detail composition, subscriber-only delivery, and frontend sparse-merge regressions.
- [x] No watch-mode flags were used in accepted evidence.
- [x] Feedback latency remains under 120 seconds for the accepted backend and frontend suites.
- [x] `nyquist_compliant: true` is set in frontmatter.

**Approval:** verified 2026-04-15 -- HUMAN-UAT finalized with `passed: 1`, `issues: 0`, `pending: 0`, `skipped: 0`, and `blocked: 0`.
