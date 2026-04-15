---
phase: 45
slug: polling-cadence-gap-closure
status: verified
threats_open: 0
asvs_level: 1
created: 2026-04-13
---

# Phase 45 - Security

> Per-phase security contract: threat register, accepted risks, and audit trail.

---

## Trust Boundaries

| Boundary | Description | Data Crossing |
|----------|-------------|---------------|
| device update API -> scheduler runtime | Persisted override changes cross from HTTP/service code into the long-running scheduler loop. | `poll_interval_override`, device metadata, effective cadence |
| service layer -> scheduler command queue | Concurrent device edits inject targeted work into a single-goroutine runtime owner. | `ReduePerformanceTask(device, changedAt)` requests |
| collector results -> state store | Different volatility classes write shared runtime state for one device. | `LastPolledAt`, `ExpectedInterval`, `Stale`, metrics, reachability |
| state store -> websocket snapshot | Backend freshness/cadence metadata becomes operator-visible canvas/detail state. | `last_polled_at`, `expected_poll_interval_seconds`, `snapshot_delta` |

---

## Security Audit 2026-04-13

| Metric | Count |
|--------|-------|
| Threats found | 6 |
| Closed | 6 |
| Open | 0 |
| Unregistered flags | 0 |

---

## Threat Register

| Threat ID | Category | Component | Disposition | Mitigation | Status |
|-----------|----------|-----------|-------------|------------|--------|
| T-45-01 | Tampering | `internal/scheduler/scheduler.go` targeted override path | mitigate | Touch only `TaskKey(deviceID, performance)` and recompute cadence via `EffectiveInterval(device, performance)` so override saves cannot retime operational/static tasks. | closed |
| T-45-02 | Denial of Service | scheduler ready/heap state | mitigate | Reuse keyed heap items plus `pending`/ready semantics instead of creating duplicate work or broad refreshes. | closed |
| T-45-03 | Elevation of Privilege | `internal/service/device_service.go` override bridge | accept | Accepted risk AR-45-03: Phase 45 preserves the existing unauthenticated device-update surface and does not widen privilege scope. | closed |
| T-45-04 | Tampering | `internal/state/store.go` mixed-tier state writes | mitigate | Restrict `LastPolledAt`, `ExpectedInterval`, and `Stale=false` to performance-owned and legacy paths. | closed |
| T-45-05 | Denial of Service | overview websocket freshness/cadence semantics | mitigate | Keep overview freshness metadata performance-owned and prove later operational/static polls do not mask stale performance state. | closed |
| T-45-06 | Tampering | `internal/worker/pipeline.go` targeted detail delta path | mitigate | Keep detail traffic on `snapshot_delta` while preserving performance-owned freshness/cadence metadata after operational sends. | closed |

*Status: open · closed*
*Disposition: mitigate (implementation required) · accept (documented risk) · transfer (third-party)*

---

## Accepted Risks Log

| Risk ID | Threat Ref | Rationale | Accepted By | Date |
|---------|------------|-----------|-------------|------|
| AR-45-03 | T-45-03 | `DeviceService.UpdateDevice(...)` still rides the project's pre-existing device-update trust boundary; this phase only propagates already-persisted override changes into the scheduler runtime. | Codex retroactive audit | 2026-04-13 |

*Accepted risks do not resurface in future audit runs.*

---

## Unregistered Flags

No unregistered flags. `45-01-SUMMARY.md` and `45-02-SUMMARY.md` do not contain a `## Threat Flags` section.

---

## Threat Verification Evidence

| Threat ID | Evidence |
|-----------|----------|
| T-45-01 | `internal/scheduler/scheduler.go:344-392` updates only `TaskKey(deviceID, performance)` and recomputes cadence with `EffectiveInterval(device, domain.VolatilityClassPerformance)` at `:357-358`; regression coverage for heap/missing/unmanaged cases at `internal/scheduler/scheduler_test.go:246-322`, `:477-516`, `:518-542`. |
| T-45-02 | `internal/scheduler/scheduler.go:360-373` reuses existing keyed items across in-flight, queued, heap, and immediate-ready states; `internal/scheduler/scheduler.go:423-462` coalesces pending reruns; tests cover queued/in-flight/completion semantics at `internal/scheduler/scheduler_test.go:324-393`, `:395-475`, `:763-808`, `:884-925`. |
| T-45-03 | Accepted as AR-45-03 in this file; bridge remains narrowly scoped in `internal/service/device_service.go:258-307`, with no new auth boundary added by Phase 45. |
| T-45-04 | `internal/state/store.go:150-163` calls `applyFreshnessMetadata` only for performance and legacy paths; helper is isolated at `internal/state/store.go:359-365`; mixed-tier regressions live at `internal/state/store_test.go:99-187`, `:189-254`, `:256-354`. |
| T-45-05 | `internal/worker/snapshot_builder.go:80-102` emits overview freshness/cadence from `state.DeviceState`; mixed-tier overview regression at `internal/worker/pipeline_test.go:816-915` proves operational/static polls do not overwrite performance-owned metadata. |
| T-45-06 | `internal/worker/pipeline.go:320-340` keeps targeted detail sends on `ws.MessageTypeSnapshotDelta`; `internal/worker/snapshot_builder.go:115-138` sources detail freshness/cadence from `state.DeviceState`; operational-detail preservation is asserted at `internal/worker/pipeline_test.go:1034-1100`. |

---

## Security Audit Trail

| Audit Date | Threats Total | Closed | Open | Run By |
|------------|---------------|--------|------|--------|
| 2026-04-13 | 6 | 6 | 0 | Codex |

---

## Sign-Off

- [x] All threats have a disposition (mitigate / accept / transfer)
- [x] Accepted risks documented in Accepted Risks Log
- [x] `threats_open: 0` confirmed
- [x] `status: verified` set in frontmatter

**Approval:** verified 2026-04-13
