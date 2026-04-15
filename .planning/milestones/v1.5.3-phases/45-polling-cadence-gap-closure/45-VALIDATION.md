---
phase: 45
slug: polling-cadence-gap-closure
status: verified
nyquist_compliant: true
wave_0_complete: true
created: 2026-04-15
---

# Phase 45 -- Validation Strategy

> Per-phase validation contract for feedback sampling during execution.

---

## Test Infrastructure

| Property | Value |
|----------|-------|
| **Framework** | Go standard `testing` package plus frontend `npm test` |
| **Config file** | None -- existing backend and frontend test conventions |
| **Quick run command** | `rtk bash -lc 'go test ./internal/scheduler ./internal/service ./internal/state ./internal/worker -count=1 && cd frontend && npm test -- src/components/DeviceCard.test.tsx src/components/canvas/useCanvasData.test.ts src/utils/freshness.test.ts src/utils/polling.test.ts'` |
| **Full suite command** | `rtk bash -lc 'go test ./internal/scheduler ./internal/service ./internal/state ./internal/worker -count=1 && cd frontend && npm test -- src/components/DeviceCard.test.tsx src/components/canvas/useCanvasData.test.ts src/utils/freshness.test.ts src/utils/polling.test.ts'` |
| **Estimated runtime** | ~90 seconds |

---

## Sampling Rate

- **After every task commit:** Run the exact task command captured in the phase plan.
- **After every plan wave:** Re-run the accepted scheduler, service, state, worker, and frontend cadence/freshness checks.
- **Before `/gsd-verify-work`:** The accepted automated evidence and finalized `45-HUMAN-UAT.md` closure must both remain valid.
- **Max feedback latency:** 90 seconds

---

## Per-Task Verification Map

| Task ID | Plan | Wave | Requirement | Threat Ref | Secure Behavior | Test Type | Automated Command | Key Artifact | Status |
|---------|------|------|-------------|------------|-----------------|-----------|-------------------|--------------|--------|
| 45-01-T1 | 01 | 1 | POLL-02, POLL-06 | -- | Scheduler can re-due only the affected performance task immediately after an override change | integration | `cd /home/azmin/projects/theia && rtk go test ./internal/scheduler -run 'TestScheduler(ReduePerformanceTask|Complete_RequeuesImmediatePendingRerun)' -count=1 -v` | `internal/scheduler/scheduler.go` | ✅ green |
| 45-01-T2 | 01 | 1 | POLL-02, POLL-06 | -- | Persisted override changes trigger targeted re-due behavior only when the effective cadence actually changes | integration | `cd /home/azmin/projects/theia && rtk go test ./internal/service ./cmd/theia -run 'Test(UpdateDevice_PollIntervalOverride(TriState|TriggersSchedulerRedueOnChange|DoesNotTriggerSchedulerRedueWhenUnchanged|ReduesNextPerformanceTask)|WirePollRescheduler_AttachesSchedulerToDeviceService)' -count=1 -v` | `internal/service/device_service.go`, `cmd/theia/main.go` | ✅ green |
| 45-02-T1 | 02 | 1 | WS-01, WS-04 | -- | Only performance updates own overview freshness and cadence metadata inside the state store | integration | `cd /home/azmin/projects/theia && rtk go test ./internal/state -run 'TestStoreUpdate_(OperationalPoll|StaticPoll|FailedPerformancePoll)' -count=1 -v` | `internal/state/store.go` | ✅ green |
| 45-02-T2 | 02 | 1 | WS-01, WS-04 | -- | Overview snapshots and targeted detail deltas preserve performance-owned metadata after mixed-tier polls | integration | `cd /home/azmin/projects/theia && rtk go test ./internal/worker -count=1 -v` | `internal/worker/pipeline.go`, `internal/worker/snapshot_builder.go` | ✅ green |

*Status: ⬜ pending · ✅ green · ❌ red · ⚠️ flaky*

---

## Wave 0 Requirements

- [x] `internal/scheduler/scheduler_test.go` covers targeted performance-task re-due behavior and pending rerun handling.
- [x] `internal/service/device_service_test.go` and `cmd/theia/main_test.go` cover persisted-change-only trigger semantics and bootstrap wiring.
- [x] `internal/state/store_test.go` and `internal/worker/pipeline_test.go` cover performance-owned freshness/cadence metadata and mixed-tier preservation.
- [x] `frontend/src/components/DeviceCard.test.tsx`, `frontend/src/components/canvas/useCanvasData.test.ts`, `frontend/src/utils/freshness.test.ts`, and `frontend/src/utils/polling.test.ts` cover frontend cadence/freshness rendering and metadata preservation.
- [x] No new framework install was required -- the existing backend and frontend harnesses covered the phase.

---

## Manual-Only Verifications

| Behavior | Requirement | Why Manual | Finalized Proof |
|----------|-------------|------------|-----------------|
| Live Canvas Cadence + Mixed-Tier Stability | POLL-02, POLL-06, WS-01, WS-04 | Requires observing live websocket timing and rendered browser state through the next performance poll and later operational/static polls. | `45-HUMAN-UAT.md` records `passed`: the canvas/detail panel for `gw-core-01` (`23d73e45-7c86-4bf9-ba98-26697bfb25f6`) updated the visible cadence label immediately after the override save and preserved performance-owned freshness/cadence through later operational/static polls; the recorded before/after cadence changed from `60s` to `30s`. |

Finalized HUMAN-UAT summary: `passed: 1`, `issues: 0`, `pending: 0`, `skipped: 0`, `blocked: 0`.

---

## Validation Sign-Off

- [x] All tasks have `<automated>` verify or Wave 0 dependencies.
- [x] Sampling continuity: no 3 consecutive tasks without automated verify.
- [x] Wave 0 covers scheduler, service, state, worker, and frontend cadence/freshness regressions.
- [x] No watch-mode flags were used in accepted evidence.
- [x] Feedback latency remains under 90 seconds for the accepted backend and frontend suites.
- [x] `nyquist_compliant: true` is set in frontmatter.

**Approval:** verified 2026-04-15 -- HUMAN-UAT finalized with `passed: 1`, `issues: 0`, `pending: 0`, `skipped: 0`, and `blocked: 0`.
