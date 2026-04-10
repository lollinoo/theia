# Phase 28: API Call Optimization — WebSocket Delta Payloads - Context

**Gathered:** 2026-04-08
**Status:** Ready for planning

<domain>
## Phase Boundary

Optimize the `/api/v1/ws` WebSocket endpoint to send delta payloads instead of full snapshots every polling cycle. At 77+ devices the current design broadcasts ~55 KB uncompressed per cycle even when nothing changed. This phase introduces hash-based change detection on the backend and a new `snapshot_delta` message type. No changes to REST endpoints, polling intervals, or SNMP collection logic.

Requirements in scope: None formally numbered — this is a performance improvement phase.

</domain>

<decisions>
## Implementation Decisions

### Delta Detection
- **D-01:** Use hash-based change detection. After each collection cycle, serialize each device's metrics contribution to a canonical string and compute a hash (FNV-32 or FNV-64 — Go stdlib `hash/fnv`). Compare against the previous cycle's hash map. Only entries with a changed hash are included in the delta payload.
- **D-02:** The hash state map (`prevHashes`) lives in `MetricsCollector` (not the Hub), co-located with the data that changes. Keyed by `device_id` per section.

### Delta Message Protocol
- **D-03:** Add a new message type `MessageTypeSnapshotDelta` (new constant in `internal/ws/messages.go`). Wire format is identical to `SnapshotPayload` but contains only the changed entries in each section. Frontend distinguishes it from `MessageTypeSnapshot` by the `type` field.
- **D-04:** First connect continues to receive a full `MessageTypeSnapshot` (existing behavior unchanged). Reconnects also get the full snapshot — no additional resync mechanism needed.
- **D-05:** If zero entries changed across all sections, the broadcast is skipped entirely (no empty delta sent).

### Delta Coverage — All 5 Sections
- **D-06:** All five sections are diffed:
  - `device_metrics` — per `device_id` hash
  - `link_metrics` — per `device_id` hash (covers the whole interface array for that device)
  - `device_statuses` — per `device_id` hash
  - `device_hostnames` — per `device_id` hash
  - `alerts` — diffed as a set keyed by `(device_id, alert_name)`; the full current alert set replaces the previous if any alert entry changed
- **D-07:** Alerts use a whole-set hash rather than per-alert hashing: hash the sorted serialization of all active alerts. If the hash changes, include the full current alert array in the delta (no per-alert diffing).

### Batching / Cadence
- **D-08:** No new polling interval or WS-push interval. The WebSocket push cadence remains coupled to the existing `polling_interval_seconds` setting (default 60s). Benefit comes purely from reduced payload size when few devices change.

### REST API Scope
- **D-09:** REST endpoints are out of scope. The phase title's "API call optimization" refers to the WS payload specifically. REST calls are infrequent (app init + user actions) and not a bottleneck at 77+ device scale.

### Frontend Merge
- **D-10:** When the frontend receives a `snapshot_delta` message, it deep-merges the delta into the existing snapshot state — overwriting only the entries present in the delta, leaving unchanged entries intact. Full `snapshot` messages continue to replace the entire state (existing behavior).

### Claude's Discretion
- Exact FNV variant (32 vs. 64) — either works; choose based on collision tolerance
- Whether `prevHashes` uses a single flat map or per-section maps
- Frontend merge implementation details (spread operator vs. `Object.assign` vs. `structuredClone` partial)
- Unit test strategy for hash comparison and merge behavior

</decisions>

<canonical_refs>
## Canonical References

**Downstream agents MUST read these before planning or implementing.**

### WebSocket Layer
- `internal/ws/messages.go` — `SnapshotPayload`, `MessageTypeSnapshot`, existing unused `MessageTypeMetrics`/`MessageTypeLinkMetrics`; new `MessageTypeSnapshotDelta` type goes here
- `internal/ws/hub.go` — `Hub.Broadcast()` and `Hub.Run()` — broadcast dispatch to all clients
- `internal/ws/handler.go` — initial full snapshot on connect (lines 18-79)

### Metrics Collection
- `internal/worker/metrics_collector.go` — `buildSnapshot()` (lines 271-737); `prevCounters` map (lines 614-643) is an existing delta pattern to reference
- `internal/worker/settings.go` — `GetPollingInterval()` (lines 12-22); polling interval drives broadcast cadence

### Frontend
- `frontend/src/hooks/useWebSocket.ts` — WS consumer; needs new branch for `snapshot_delta` message type (line 107 currently replaces entire snapshot)
- `frontend/src/types/metrics.ts` — `SnapshotPayload` type and `parseSnapshotPayload()`; needs `SnapshotDeltaPayload` type + parse function

</canonical_refs>

<code_context>
## Existing Code Insights

### Reusable Assets
- `internal/worker/metrics_collector.go` `prevCounters` map (lines 614-643) — existing per-device delta pattern for SNMP link counter rates; the new `prevHashes` map follows the same lifecycle and ownership model
- `hash/fnv` — Go stdlib, no new dependency; `fnv.New32()` produces a 32-bit hash suitable for change detection
- `SnapshotPayload` struct — the delta payload uses the same type, just with sparse maps; no new DTO types needed on the backend

### Established Patterns
- Backend uses `map[string]T` keyed by `device_id` for all per-device data; delta payload preserves this shape (sparse maps are idiomatic Go)
- Frontend `useWebSocket.ts` dispatches on `msg.type` — adding a `"snapshot_delta"` case is a clean extension of the existing dispatch pattern
- `parseSnapshotPayload()` in `metrics.ts` already handles nullable fields; delta parse can reuse the same field-level parsers

### Integration Points
- `MetricsCollector.buildSnapshot()` → must split into: (1) build full snapshot, (2) compute hashes, (3) diff against `prevHashes`, (4) build sparse delta payload, (5) update `prevHashes`, (6) send delta (or skip if empty)
- `Hub.Broadcast()` is the single call site for all WS messages; delta goes through the same channel
- `handler.go` initial snapshot send (on new client connect) is separate from the broadcast and must NOT be changed to send a delta

</code_context>

<specifics>
## Specific Ideas

- "Skip the broadcast entirely if zero entries changed" — no empty delta noise on the wire
- Alerts use whole-set diffing (hash all active alerts together) rather than per-alert tracking — simpler and alerts are the smallest section

</specifics>

<deferred>
## Deferred Ideas

- Sub-interval WS push (e.g., 15s delta cadence independent of SNMP poll) — discussed, not needed for the 77-device scale problem. Could revisit if polling is reduced to 30s but operators want faster live updates.
- REST batch endpoint (POST /api/v1/devices/batch) — out of scope for this phase; WS is the bottleneck.
- Per-alert diffing (instead of whole-set hash for alerts) — over-engineering for a section that's already small.

None — discussion stayed within phase scope.

</deferred>

---

*Phase: 28-api-call-optimization-especially-websocket-payload-optimizat*
*Context gathered: 2026-04-08*
