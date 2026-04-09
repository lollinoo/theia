---
phase: 28
slug: api-call-optimization-especially-websocket-payload-optimizat
status: validated
nyquist_compliant: true
wave_0_complete: false
created: 2026-04-09
---

# Phase 28 — Validation Strategy

> Per-phase validation contract for hash-based WebSocket delta optimization.

---

## Test Infrastructure

| Property | Value |
|----------|-------|
| **Framework (Go)** | Go standard `testing` package |
| **Framework (Frontend)** | vitest 4.1 + @testing-library/react 16.3 |
| **Config file (Frontend)** | `frontend/vitest.config.ts` |
| **Quick run (Go)** | `docker compose exec -T backend go test ./internal/worker/... ./internal/ws/... -v` |
| **Quick run (Frontend)** | `cd frontend && npx vitest run src/hooks/useWebSocket.test.ts src/types/metrics.test.ts` |
| **Full suite (Go)** | `docker compose exec -T backend go test ./...` |
| **Full suite (Frontend)** | `cd frontend && npx vitest run` |
| **Estimated runtime** | ~15s (Go), ~2s (frontend) |

---

## Sampling Rate

- **After every task commit:** Run quick run commands for the subsystem changed
- **After every plan wave:** Run both full suite commands
- **Before `/gsd-verify-work`:** Both full suites must be green
- **Max feedback latency:** 30 seconds

---

## Per-Task Verification Map

| Task ID | Plan | Wave | Requirement | Test Type | Automated Command | File Exists | Status |
|---------|------|------|-------------|-----------|-------------------|-------------|--------|
| 28-01-T1a | 01 | 1 | No broadcast when nothing changed (function level) | unit | `go test ./internal/worker/... -run TestBuildDelta_NoChanges_ReturnsNil` | ✅ | ✅ green |
| 28-01-T1b | 01 | 1 | No broadcast when nothing changed (integration) | integration | `go test ./internal/worker/... -run TestCollectAndBroadcast_SecondCycle_SkipsBroadcastWhenUnchanged` | ✅ | ✅ green |
| 28-01-T1c | 01 | 1 | Only changed entries in delta payload | unit | `go test ./internal/worker/... -run TestBuildDelta_OneDeviceMetricsChanged` | ✅ | ✅ green |
| 28-01-T1d | 01 | 1 | Only changed entries (mixed sections) | unit | `go test ./internal/worker/... -run TestBuildDelta_MixedChanges` | ✅ | ✅ green |
| 28-01-T1e | 01 | 1 | Delta message type is `snapshot_delta` (broadcast level) | integration | `go test ./internal/worker/... -run TestCollectAndBroadcast_SecondCycle_BroadcastsSnapshotDelta` | ✅ | ✅ green |
| 28-01-T1f | 01 | 1 | All 5 sections hashed | unit | `go test ./internal/worker/... -run TestComputeSnapshotHashes_AllSections` | ✅ | ✅ green |
| 28-01-T1g | 01 | 1 | Alerts compared as whole set | unit | `go test ./internal/worker/... -run TestBuildDelta_AlertsChanged` | ✅ | ✅ green |
| 28-01-T1h | 01 | 1 | First cycle broadcasts full `snapshot` type | integration | `go test ./internal/worker/... -run TestCollectAndBroadcast_FirstCycle_BroadcastsSnapshotType` | ✅ | ✅ green |
| 28-02-T1a | 02 | 1 | Frontend deep-merges snapshot_delta | integration | `cd frontend && npx vitest run src/hooks/useWebSocket.test.ts` | ✅ | ✅ green |
| 28-02-T1b | 02 | 1 | Full snapshot replaces entire state | integration | `cd frontend && npx vitest run src/hooks/useWebSocket.test.ts` | ✅ | ✅ green |
| 28-02-T1c | 02 | 1 | Delta preserves unchanged entries | integration | `cd frontend && npx vitest run src/hooks/useWebSocket.test.ts` | ✅ | ✅ green |
| 28-02-T1d | 02 | 1 | Unknown message types handled gracefully | unit | `cd frontend && npx vitest run src/hooks/useWebSocket.test.ts` | ✅ | ✅ green |
| 28-02-T1e | 02 | 1 | First load: full snapshot + incremental deltas | integration | `cd frontend && npx vitest run src/hooks/useWebSocket.test.ts` | ✅ | ✅ green |
| 28-02-T2a | 02 | 1 | parseWSMessage handles snapshot_delta type | unit | `cd frontend && npx vitest run src/types/metrics.test.ts` | ✅ | ✅ green |
| 28-02-T2b | 02 | 1 | mergeSnapshotDelta pure function behavior | unit | `cd frontend && npx vitest run src/types/metrics.test.ts` | ✅ | ✅ green |

*Status: ⬜ pending · ✅ green · ❌ red · ⚠️ flaky*

---

## Wave 0 Requirements

Existing infrastructure covers all phase requirements.

---

## Manual-Only Verifications

All phase behaviors have automated verification.

---

## Validation Audit 2026-04-09

| Metric | Count |
|--------|-------|
| Gaps found | 4 |
| Resolved | 4 |
| Escalated | 0 |

### Gaps Resolved

| Gap | Type | Test Added |
|-----|------|-----------|
| Broadcast type is `snapshot_delta` on 2nd cycle | MISSING | `TestCollectAndBroadcast_SecondCycle_BroadcastsSnapshotDelta` |
| Skip broadcast when nothing changed (integration) | PARTIAL | `TestCollectAndBroadcast_SecondCycle_SkipsBroadcastWhenUnchanged` |
| First cycle broadcasts `snapshot` type | PARTIAL | `TestCollectAndBroadcast_FirstCycle_BroadcastsSnapshotType` |
| Unknown message types handled gracefully | MISSING | `'does not crash when receiving an unknown message type'` |

Infrastructure added: `internal/ws/hub_broadcast_ch.go` (exports `BroadcastCh()` for test inspection)

---

## Validation Sign-Off

- [x] All tasks have automated verify
- [x] Sampling continuity: no 3 consecutive tasks without automated verify
- [x] Wave 0 covers all MISSING references
- [x] No watch-mode flags
- [x] Feedback latency < 30s
- [x] `nyquist_compliant: true` set in frontmatter

**Approval:** approved 2026-04-09
