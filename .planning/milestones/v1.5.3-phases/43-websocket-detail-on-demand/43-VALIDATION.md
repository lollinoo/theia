---
phase: 43
slug: websocket-detail-on-demand
status: verified
nyquist_compliant: true
wave_0_complete: true
created: 2026-04-15
---

# Phase 43 -- Validation Strategy

> Per-phase validation contract for feedback sampling during execution.

---

## Test Infrastructure

| Property | Value |
|----------|-------|
| **Framework** | Go standard `testing` package plus frontend `npm test` |
| **Config file** | None -- existing Go and frontend test conventions |
| **Quick run command** | `rtk bash -lc 'go test ./internal/ws -count=1 && go test ./internal/worker -count=1'` |
| **Full suite command** | `rtk bash -lc 'go test ./internal/ws -count=1 && go test ./internal/worker -count=1 && cd frontend && npm test -- src/types/metrics.test.ts src/hooks/useWebSocket.test.ts src/components/canvas/detailSubscription.test.ts'` |
| **Estimated runtime** | ~45 seconds |

---

## Sampling Rate

- **After every task commit:** Run the exact backend or frontend command captured in that task's plan.
- **After every plan wave:** Re-run the accepted backend package checks and the frontend targeted lifecycle suites.
- **Before `/gsd-verify-work`:** Backend websocket and worker tests plus the frontend socket/detail lifecycle tests must still be green.
- **Max feedback latency:** 60 seconds

---

## Per-Task Verification Map

| Task ID | Plan | Wave | Requirement | Threat Ref | Secure Behavior | Test Type | Automated Command | Key Artifact | Status |
|---------|------|------|-------------|------------|-----------------|-----------|-------------------|--------------|--------|
| 43-01-T1 | 01 | 1 | WS-02 | -- | Shared websocket contract gains additive detail fields and typed control parsing without a second message family | unit | `cd /home/azmin/projects/theia && rtk go test ./internal/ws -run 'TestParseClientControlMessage|TestCloneSnapshot' -count=1 -v` | `internal/ws/messages.go` | ✅ green |
| 43-01-T2 | 01 | 1 | WS-02 | -- | Each client tracks one active detail subscription and disconnect cleanup clears stale state | unit | `cd /home/azmin/projects/theia && rtk go test ./internal/ws -run 'TestHub(Set|Detail|Clear|Remove)' -count=1 -v` | `internal/ws/hub.go` | ✅ green |
| 43-02-T1 | 02 | 2 | WS-02 | -- | A one-device detail delta stays on the shared snapshot contract and hashes detail fields correctly | unit | `cd /home/azmin/projects/theia && rtk go test ./internal/worker -run 'TestBuildDeviceDetailDelta|TestComputeSnapshotHashes_DeviceMetricHashIncludesDetailFields' -count=1 -v` | `internal/worker/snapshot_builder.go` | ✅ green |
| 43-02-T2 | 02 | 2 | WS-02 | -- | Targeted post-poll detail delivery reaches only subscribed clients and does not widen payload scope | integration | `cd /home/azmin/projects/theia && rtk go test ./internal/worker -run 'TestPipelineOrchestratorRunTask_(PerformancePollSendsDetailDeltaToSubscribedClient|OperationalPollSendsDetailDeltaToSubscribedClient|DetailDeltaDoesNotReachUnsubscribedClient|DetailDeltaOmitsLinkMetrics)' -count=1 -v` | `internal/worker/pipeline.go` | ✅ green |
| 43-03-T1 | 03 | 2 | WS-02 | -- | The shared frontend websocket hook sends detail control messages and keeps one socket path | frontend | `cd /home/azmin/projects/theia/frontend && rtk npm test -- src/types/metrics.test.ts src/hooks/useWebSocket.test.ts` | `frontend/src/types/metrics.ts`, `frontend/src/hooks/useWebSocket.ts` | ✅ green |
| 43-03-T2 | 03 | 2 | WS-02 | -- | Canvas owns the detail subscription lifecycle and only device-scoped panels participate | frontend | `cd /home/azmin/projects/theia/frontend && rtk npm test -- src/components/canvas/detailSubscription.test.ts src/hooks/useWebSocket.test.ts` | `frontend/src/App.tsx`, `frontend/src/components/Canvas.tsx`, `frontend/src/components/canvas/detailSubscription.ts` | ✅ green |

*Status: ⬜ pending · ✅ green · ❌ red · ⚠️ flaky*

---

## Wave 0 Requirements

- [x] `internal/ws/messages_test.go` and `internal/ws/hub_test.go` cover control parsing, one-detail-device ownership, unsubscribe cleanup, and disconnect cleanup.
- [x] `internal/worker/snapshot_builder_test.go` and `internal/worker/pipeline_test.go` cover targeted detail payload shape, hash sensitivity, subscribed delivery, and unsubscribed silence.
- [x] `frontend/src/types/metrics.test.ts`, `frontend/src/hooks/useWebSocket.test.ts`, and `frontend/src/components/canvas/detailSubscription.test.ts` cover shared-snapshot parsing, control-message lifecycle, reconnect behavior, and canvas ownership rules.
- [x] The accepted backend spot-checks remain `rtk go test ./internal/ws -count=1` and `rtk go test ./internal/worker -count=1`.
- [x] No new framework install was required -- existing Go and frontend test harnesses already covered the phase.

---

## Manual-Only Verifications

| Behavior | Requirement | Why Manual | Test Instructions |
|----------|-------------|------------|-------------------|
| -- | -- | All phase behaviors have automated verification. | -- |

*All phase behaviors have automated verification.*

---

## Validation Sign-Off

- [x] All tasks have `<automated>` verify or Wave 0 dependencies.
- [x] Sampling continuity: no 3 consecutive tasks without automated verify.
- [x] Wave 0 covers backend websocket, worker targeted-delivery, and frontend lifecycle regressions.
- [x] No watch-mode flags were used in accepted evidence.
- [x] Feedback latency remains under 60 seconds for the accepted backend and frontend suites.
- [x] `nyquist_compliant: true` is set in frontmatter.

**Approval:** verified 2026-04-15
