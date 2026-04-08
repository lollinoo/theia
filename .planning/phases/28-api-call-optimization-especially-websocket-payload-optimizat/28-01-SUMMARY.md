---
phase: 28-api-call-optimization
plan: "01"
subsystem: worker/ws
tags: [websocket, performance, delta, hashing]
dependency_graph:
  requires: []
  provides: [snapshot_delta message type, hash-based delta detection in MetricsCollector]
  affects: [internal/ws/messages.go, internal/worker/metrics_collector.go]
tech_stack:
  added: [hash/fnv (stdlib)]
  patterns: [FNV-64a per-device hashing, sparse delta broadcast, prevHashes nil-guarded first cycle]
key_files:
  created: []
  modified:
    - internal/ws/messages.go
    - internal/worker/metrics_collector.go
    - internal/worker/metrics_collector_test.go
decisions:
  - FNV-64a chosen for hash function — fast, non-cryptographic, collision risk is benign (would only cause an extra full-section send at worst)
  - Alerts hashed as a whole set (single alertsHash) rather than per-device — alerts are a small indivisible set; per-device alert hashing would add complexity for minimal benefit
  - prevHashes stored on MetricsCollector protected by the existing c.mu RWMutex — no separate mutex needed
  - buildDelta returns nil to signal "skip broadcast entirely" rather than an empty payload — cleaner than sending a zero-content delta
metrics:
  duration: "~10 minutes"
  completed: "2026-04-08T15:49:31Z"
  tasks_completed: 1
  tasks_total: 1
  files_modified: 3
---

# Phase 28 Plan 01: WebSocket Delta Payload Optimization Summary

FNV-64a hash-based change detection in MetricsCollector with sparse `snapshot_delta` broadcast — only changed device entries are sent on subsequent cycles, full snapshot only on first cycle or client reconnect.

## Tasks Completed

| # | Task | Commit | Files |
|---|------|--------|-------|
| 1 | Add MessageTypeSnapshotDelta constant and hash-based delta detection | 65fe264 | internal/ws/messages.go, internal/worker/metrics_collector.go, internal/worker/metrics_collector_test.go |

## What Was Built

### `internal/ws/messages.go`
- Added `MessageTypeSnapshotDelta = "snapshot_delta"` constant alongside the existing `MessageTypeSnapshot`.

### `internal/worker/metrics_collector.go`
- Added `sectionHashes` struct holding per-device-id `uint64` hash maps for all 5 snapshot sections: `deviceMetrics`, `linkMetrics`, `deviceStatuses`, `deviceHostnames`, and a whole-set `alertsHash`.
- Added `prevHashes *sectionHashes` field to `MetricsCollector`, initialized to `nil` (first cycle detection).
- Added `computeSectionHash(data string) uint64` — pure FNV-64a hash of a canonical string.
- Added `computeSnapshotHashes(snapshot *ws.SnapshotPayload) *sectionHashes` — iterates all 5 sections, hashes each entry per device_id, and produces the whole-set alerts hash.
- Added `buildDelta(current, currentHashes, prevHashes) *ws.SnapshotPayload` — compares hashes section by section, populates a sparse payload with only changed entries, returns `nil` if nothing changed.
- Modified `collectAndBroadcast`: replaced the unconditional `hub.Broadcast(MessageTypeSnapshot)` with conditional delta logic: first cycle sends full `snapshot`, subsequent cycles send sparse `snapshot_delta` or skip entirely when nothing changed.

### `internal/worker/metrics_collector_test.go`
Added 8 new tests:
1. `TestComputeSectionHash_Deterministic` — same input produces same hash; different input produces different hash
2. `TestComputeSnapshotHashes_AllSections` — all 5 sections populated and hashed correctly
3. `TestBuildDelta_NoChanges_ReturnsNil` — identical hashes produce nil delta
4. `TestBuildDelta_OneDeviceMetricsChanged` — one device change produces sparse delta with only that device
5. `TestBuildDelta_AlertsChanged` — changed alertsHash includes full alerts array
6. `TestBuildDelta_MixedChanges` — 2 metric changes + 1 status change produces correct sparse delta
7. `TestCollectAndBroadcast_FirstCycle_SendsFullSnapshot` — prevHashes set after first cycle
8. `TestCollectAndBroadcast_SecondCycle_SendsDelta` — prevHashes remains set after second cycle

## Deviations from Plan

None — plan executed exactly as written.

## Threat Flags

None. This is a server-side optimization of an existing broadcast mechanism. No new trust boundaries, no new input vectors, no new data exposed.

## Known Stubs

None. All delta logic is fully wired. The frontend receives `snapshot_delta` messages through the existing WebSocket connection — no frontend changes are needed for this optimization (clients that don't handle `snapshot_delta` will simply ignore it until they are updated).

## Self-Check

### Files Exist
- `internal/ws/messages.go` — modified with `MessageTypeSnapshotDelta`
- `internal/worker/metrics_collector.go` — modified with sectionHashes, prevHashes, delta functions
- `internal/worker/metrics_collector_test.go` — modified with 8 new tests

### Commits Exist
- `65fe264` — feat(28-01): implement hash-based WebSocket delta detection

## Self-Check: PASSED
