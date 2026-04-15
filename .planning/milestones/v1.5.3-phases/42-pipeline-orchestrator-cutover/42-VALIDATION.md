---
phase: 42
slug: pipeline-orchestrator-cutover
status: verified
nyquist_compliant: true
wave_0_complete: true
created: 2026-04-15
---

# Phase 42 -- Validation Strategy

> Per-phase validation contract for feedback sampling during execution.

---

## Test Infrastructure

| Property | Value |
|----------|-------|
| **Framework** | Go standard `testing` package |
| **Config file** | None -- repo Go test conventions |
| **Quick run command** | `rtk go test ./internal/state ./internal/collector ./internal/service ./internal/worker ./internal/api ./cmd/theia -count=1` |
| **Full suite command** | `rtk bash -lc 'go test ./internal/state ./internal/collector ./internal/service ./internal/worker ./internal/api ./cmd/theia -count=1 && go test ./internal/worker -run \"TestPipelineOrchestrator|TestBuildPipelineSnapshot|TestBuildDelta\" -count=1'` |
| **Estimated runtime** | ~90 seconds |

---

## Sampling Rate

- **After every task commit:** Run the exact task command captured in the phase plan.
- **After every plan wave:** Re-run the accepted state, collector, service, worker, API, and bootstrap package checks.
- **Before `/gsd-verify-work`:** The accepted Go package evidence and finalized `42-HUMAN-UAT.md` closure must both remain valid.
- **Max feedback latency:** 90 seconds

---

## Per-Task Verification Map

| Task ID | Plan | Wave | Requirement | Threat Ref | Secure Behavior | Test Type | Automated Command | Key Artifact | Status |
|---------|------|------|-------------|------------|-----------------|-----------|-------------------|--------------|--------|
| 42-01-T1 | 01 | 1 | PIPE-03 | -- | State store merges performance, operational, and static updates without clobbering retained runtime data | unit | `cd /home/azmin/projects/theia && go test ./internal/state -run 'TestStoreUpdate_|TestStoreSnapshot_' -count=1 -v` | `internal/state/store.go` | ✅ green |
| 42-01-T2 | 01 | 1 | PIPE-03 | -- | Collector-to-store adapters stamp explicit volatility classes and preserve tier ownership | unit | `cd /home/azmin/projects/theia && go test ./internal/collector -run 'TestPerformanceResultToStoreUpdate|TestOperationalResultToStoreUpdate' -count=1 -v` | `internal/collector/results.go` | ✅ green |
| 42-02-T1 | 02 | 1 | PIPE-03 | -- | Static discovery persistence becomes reusable outside the legacy probe path | unit | `cd /home/azmin/projects/theia && go test ./internal/service -run 'TestApplyStaticDiscovery' -count=1 -v` | `internal/service/static_persistence.go` | ✅ green |
| 42-02-T2 | 02 | 1 | PIPE-03 | -- | `probeDevice()` reuses the shared helper while keeping topology notification caller-owned | unit | `cd /home/azmin/projects/theia && go test ./internal/service -run 'TestProbeDeviceUsesApplyStaticDiscoveryAndSignalsTopologyNotify' -count=1 -v` | `internal/service/device_service.go` | ✅ green |
| 42-03-T1 | 03 | 2 | PIPE-03 | -- | Snapshot and delta building preserve the existing overview websocket contract | unit | `cd /home/azmin/projects/theia && go test ./internal/worker -run 'TestBuildPipelineSnapshot|TestBuildDelta' -count=1 -v` | `internal/worker/snapshot_builder.go` | ✅ green |
| 42-03-T2 | 03 | 2 | PIPE-03 | -- | `PipelineOrchestrator` consumes scheduler tasks, dispatches collectors, updates state, and completes work items | integration | `cd /home/azmin/projects/theia && go test ./internal/worker -run 'TestPipelineOrchestratorPerformanceTask|TestPipelineOrchestratorStaticTask|TestPipelineOrchestratorStatus' -count=1 -v` | `internal/worker/pipeline.go` | ✅ green |
| 42-03-T3 | 03 | 2 | PIPE-03 | -- | Fixed-tick broadcast, Prometheus status signaling, and post-broadcast `topology_changed` ordering stay intact | integration | `cd /home/azmin/projects/theia && go test ./internal/worker -run 'TestPipelineOrchestratorBroadcastLoop|TestPipelineOrchestratorTopologyChanged|TestPipelineOrchestratorPrometheusStatus' -count=1 -v` | `internal/worker/pipeline.go` | ✅ green |
| 42-04-T1 | 04 | 3 | PIPE-03 | -- | Production bootstrap starts only the pipeline runtime and shared collector dependencies | integration | `cd /home/azmin/projects/theia && go test ./cmd/theia -count=1 -v` | `cmd/theia/main.go` | ✅ green |
| 42-04-T2 | 04 | 3 | PIPE-03 | -- | Health/router plumbing uses a status-provider seam instead of concrete poller coupling | unit | `cd /home/azmin/projects/theia && go test ./internal/api -run 'TestHealthHandler' -count=1 -v` | `internal/api/health_handler.go`, `internal/api/router.go` | ✅ green |

*Status: ⬜ pending · ✅ green · ❌ red · ⚠️ flaky*

---

## Wave 0 Requirements

- [x] `internal/state/store_test.go` locks cross-tier merge behavior, failed-performance retention, cloning, and snapshot invariants.
- [x] `internal/service/static_persistence_test.go` and related service regressions lock reusable static discovery persistence and notify ownership.
- [x] `internal/worker/pipeline_test.go` and `snapshot_builder_test.go` cover runtime task handling, broadcast ordering, topology change sequencing, and websocket snapshot compatibility.
- [x] `internal/api/health_handler_test.go` and `cmd/theia` bootstrap tests cover the status-provider seam and production startup wiring.
- [x] No new framework install was required -- the existing Go test harness covered the cutover.

---

## Manual-Only Verifications

| Behavior | Requirement | Why Manual | Finalized Proof |
|----------|-------------|------------|-----------------|
| Live Cutover Smoke Test | PIPE-03 | Requires the full backend, representative device inventory, websocket traffic, and frontend rendering behavior. | `42-HUMAN-UAT.md` records `passed`: seeded device `gw-core-01` (`23d73e45-7c86-4bf9-ba98-26697bfb25f6`, `172.28.10.10`) received an initial `snapshot` plus ongoing `snapshot_delta` traffic, and backend startup logs showed no `Poller started` or `Metrics collector started` messages. |
| Topology Ordering Check | PIPE-03 | Ordering spans runtime timing, websocket delivery, and frontend state handling. | `42-HUMAN-UAT.md` records `passed`: reseeding the lab topology for `gw-core-01`, `sw-dist-01`, and `ap-office-01` delivered refreshed snapshot data before the `topology_changed` UI effect with no stale-map or split-brain behavior. |
| Classified Scheduling Smoke Test | PIPE-03 | Effective mixed-cadence behavior is operator-visible runtime behavior rather than a pure unit concern. | `42-HUMAN-UAT.md` records `passed`: mixed poll-class seeded lab devices kept last-known performance data visible until staleness and did not flicker empty on transient misses. |

Finalized HUMAN-UAT summary: `passed: 3`, `issues: 0`, `pending: 0`, `skipped: 0`, `blocked: 0`.

---

## Validation Sign-Off

- [x] All tasks have `<automated>` verify or Wave 0 dependencies.
- [x] Sampling continuity: no 3 consecutive tasks without automated verify.
- [x] Wave 0 covers state, service, worker, API, and bootstrap regression coverage.
- [x] No watch-mode flags were used in accepted evidence.
- [x] Feedback latency remains under 90 seconds for the accepted package suites.
- [x] `nyquist_compliant: true` is set in frontmatter.

**Approval:** verified 2026-04-15 -- HUMAN-UAT finalized with `passed: 3`, `issues: 0`, `pending: 0`, `skipped: 0`, and `blocked: 0`.
