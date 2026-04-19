# Runtime Core Design

Date: 2026-04-19
Status: Approved for planning
Scope: First quality-improvement slice for the runtime hot path only

## Summary

This slice makes the realtime runtime loss-tolerant and non-blocking without taking on the restore refactor yet.

The current risk is concentrated in the coupling between runtime production and WebSocket delivery. Today `internal/ws/hub.go` can still block producers when the broadcast buffer fills, `internal/worker/pipeline.go` publishes transport messages directly from the runtime path, and duplicate lifecycle starts in `internal/scheduler/scheduler.go`, `internal/state/store.go`, and `internal/worker/pipeline.go` panic instead of returning actionable errors.

The design for this slice introduces a versioned overview stream, a lossy per-client delivery model with explicit resync, and idempotent-or-explicit lifecycle behavior across scheduler, store, and pipeline.

## Current Context

Relevant current files:

- `internal/ws/hub.go`: `Broadcast` records backpressure and then performs a blocking send when the hub buffer is full.
- `internal/ws/handler.go`: new clients receive a full snapshot immediately after connect.
- `internal/ws/messages.go`: current protocol includes `snapshot`, `snapshot_delta`, and `resync_required`, but messages are not versioned.
- `internal/worker/pipeline.go`: coalesces dirty runtime updates and broadcasts snapshots or deltas directly to the hub.
- `internal/worker/snapshot_builder.go`: materializes overview snapshots and sparse deltas.
- `internal/state/store.go`: drops change batches non-blockingly but panics on duplicate `Start`.
- `internal/scheduler/scheduler.go`: panics on duplicate `Start`.
- `frontend/src/hooks/useWebSocket.ts` and `frontend/src/types/metrics.ts`: assume the first overview message is a full snapshot and merge deltas without version continuity checks.

## Goals

- Prevent WebSocket pressure from blocking the polling and state-update hot path.
- Make overview delivery loss-tolerant through explicit resync instead of implicit producer blocking.
- Add monotonic snapshot versioning so the client can detect gaps instead of blindly merging stale deltas.
- Replace duplicate-start panics with explicit lifecycle behavior that is safe to test and safe to observe.
- Keep the first slice narrowly focused on runtime hot-path quality.

## Non-Goals

- Extracting restore orchestration out of `cmd/theia/main.go`.
- Refactoring large frontend or backend files beyond the runtime-focused boundaries needed here.
- Normalizing the SNMP/Prometheus domain model.
- Changing deployment defaults for PostgreSQL or filesystem hardening.
- Introducing a new external queue, broker, or pub/sub system.

## Payload Size Expectations

This slice is intentionally a runtime-stability change, not a payload-size optimization project.

- The main expected gains are non-blocking delivery, bounded per-client backlog, explicit resync, and safer lifecycle behavior.
- The initial full overview `snapshot` will remain in the same rough size class as today because the data model is largely unchanged.
- `snapshot_delta` payloads remain sparse and keyed by changed device IDs, but their size still scales with the number of dirty devices and the data sections included for those devices.
- Large reconnect or resync snapshots are still expected for larger fleets until the payload model itself is slimmed down in a later slice.

In practical terms, this work should improve runtime stability much more than it improves wire size.

## Proposed Architecture

The runtime hot path is split into three boundaries.

1. Runtime production boundary.
`Scheduler`, collectors, and `state.Store` remain responsible for polling, computing device runtime state, and emitting change notifications. This boundary owns correctness of runtime state only. It does not wait for browser delivery.

2. Overview stream boundary.
A small worker-owned stream coordinator consumes dirty device IDs, topology-change signals, and Prometheus status changes. It coalesces changes over the existing short window, materializes either a sparse delta or a full snapshot, and assigns a monotonic overview version.

3. Transport boundary.
`internal/ws.Hub` becomes a lossy delivery adapter with bounded per-client state. It accepts overview events without blocking the producer, coalesces or overwrites pending delivery for slow clients, and switches clients into explicit resync mode when continuity can no longer be guaranteed.

This keeps runtime production independent from transport pressure while preserving the current single-process architecture.

## Component Design

### Pipeline Orchestrator

`PipelineOrchestrator` remains the lifecycle owner for the runtime hot path, but it stops treating WebSocket messages as its direct output.

It will:

- continue to run poll workers and state updates;
- continue to coalesce dirty runtime signals;
- delegate overview-event construction to a dedicated stream component;
- stop depending on successful hub delivery to consider a runtime update complete.

### Overview Stream Coordinator

A new focused component under `internal/worker` owns:

- `currentSnapshot`;
- `currentVersion`;
- coalescing of dirty device IDs and topology alerts;
- building `snapshot` or `snapshot_delta` events from `snapshot_builder` helpers;
- deciding when to force a full snapshot;
- converting degraded continuity into explicit `resync_required` flow.

This component is intentionally small and single-purpose. It replaces the current direct coupling between `broadcastDirty`, `broadcastFullSnapshot`, and the raw WebSocket transport.

### WebSocket Hub

`internal/ws.Hub` moves from buffered channel fan-out to bounded per-client delivery state.

For overview traffic, each client has a constant-size mailbox with these semantics:

- at most one pending coalesced delta, or one pending full snapshot;
- if a newer delta supersedes an older pending delta, the hub coalesces or overwrites instead of growing a queue;
- if continuity is lost, the client is marked `needsResync` and pending deltas are discarded;
- the client remains connected;
- detail-subscription state remains supported and is adapted mechanically to the stateful hub.

## Wire Protocol

Backend and frontend change together in the same branch. No compatibility layer is required for shipped clients.

### Snapshot

The full overview snapshot message becomes:

```json
{
  "type": "snapshot",
  "payload": {
    "version": 42,
    "snapshot": {
      "device_metrics": {},
      "link_metrics": {},
      "alerts": [],
      "device_statuses": {},
      "device_hostnames": {},
      "device_models": {}
    }
  }
}
```

### Snapshot Delta

The sparse delta message becomes:

```json
{
  "type": "snapshot_delta",
  "payload": {
    "base_version": 42,
    "version": 43,
    "delta": {
      "device_metrics": {},
      "link_metrics": {},
      "alerts": [],
      "device_statuses": {},
      "device_hostnames": {},
      "device_models": {}
    }
  }
}
```

### Resync Required

The explicit degradation signal remains separate and becomes the contract for lossy recovery:

```json
{
  "type": "resync_required",
  "payload": {
    "scope": "overview",
    "reason": "client_resync_scheduled"
  }
}
```

The next useful overview message for that client is a full `snapshot(version=N)`.

`topology_changed` remains a separate trigger event and is not part of snapshot-version continuity.

## Data Flow

### Nominal Flow

1. A poll task completes.
2. `state.Store` records the update and emits dirty device IDs.
3. The overview stream coordinator coalesces dirty signals.
4. If continuity is preserved, it builds `snapshot_delta(base_version=v, version=v+1)`.
5. The hub enqueues the event into per-client mailboxes without blocking the producer.
6. The frontend applies the delta only when `base_version` matches the local version.

### Degraded Flow

1. A client falls behind or mailbox continuity would be lost.
2. The hub does not block and does not grow an unbounded queue.
3. The client is marked `needsResync`.
4. The hub emits `resync_required` and then the next full `snapshot(version=vN)`.
5. The frontend replaces local overview state with the full snapshot and resumes delta application from `vN`.

### Connect Flow

1. `internal/ws/handler.go` upgrades the socket and registers the client.
2. The handler obtains the latest full overview snapshot plus current version from the stream coordinator.
3. The client receives `snapshot(version=vN)` as its initial overview state.
4. Prometheus status is still sent immediately after connect when enabled.

## Lifecycle Semantics

`Scheduler`, `state.Store`, and `PipelineOrchestrator` change to explicit start semantics.

- `Start(ctx)` returns `error`.
- duplicate `Start` on a running component returns `ErrAlreadyStarted` instead of panicking;
- `Stop()` remains safe and idempotent;
- restarting after `Stop()` remains supported where it is already intended;
- `Status()` remains observational only.

`PipelineOrchestrator.Start(ctx)` becomes responsible for child startup rollback.

- If `state.Store.Start` succeeds and `Scheduler.Start` fails, the pipeline stops the store and returns the startup error.
- Partial runtime startup is not left running.
- Context cancellation remains the normal shutdown path, not an error condition.

## Error Handling

This slice changes the meaning of pressure from failure to controlled degradation.

- Runtime production is considered successful once state is updated and the stream coordinator is notified.
- Slow clients do not block poll workers.
- Lost delta continuity is handled as protocol-level degradation through resync, not by blocking or crashing.
- Background runtime failures continue to be logged and measured, but do not fatal the process.

Restore-related startup errors remain out of scope for this design and are intentionally not mixed into this slice.

## Observability

Existing observability support in `internal/observability/registry.go` is extended rather than replaced.

Required metrics for this slice:

- counters for overview degradation reasons, including at least mailbox overwrite/resync scheduling and full-snapshot fallback;
- a gauge for clients currently marked `needsResync`;
- existing snapshot-build metrics retained for `full` and `dirty` builds;
- existing dropped-state-change metrics retained and tied into resync decisions where applicable.

Required runtime log fields at degradation points:

- client identifier when available;
- degradation reason;
- `base_version` and `version` when applicable.

This is intentionally not a full structured-logging refactor. It is the minimum observability needed to distinguish collector health from realtime-delivery degradation.

## Testing Strategy

This slice adds focused regression coverage at the runtime boundary.

### WebSocket Hub Tests

Extend `internal/ws/hub_test.go` to verify:

- producer-side publish never blocks under saturated hub/client conditions;
- a slow client is degraded to resync instead of disconnected by default;
- pending delta delivery is coalesced or overwritten within bounded memory;
- a degraded client eventually receives `resync_required` followed by a full snapshot.

### Pipeline Tests

Extend `internal/worker/pipeline_test.go` to verify:

- version numbers are monotonic;
- `snapshot_delta.base_version` matches the previous committed overview version;
- dirty delta build falls back to full snapshot when continuity cannot be guaranteed;
- polling completion is not delayed by slow WebSocket clients.

### Lifecycle Tests

Extend:

- `internal/scheduler/scheduler_test.go`;
- `internal/state/store_test.go`;
- `internal/worker/pipeline_test.go`.

Verify:

- duplicate `Start` returns `ErrAlreadyStarted`;
- `Stop()` is idempotent;
- stop then start again works where restart is supported;
- partial child startup is rolled back correctly.

### Frontend Tests

Extend:

- `frontend/src/types/metrics.ts` parsing tests;
- `frontend/src/hooks/useWebSocket.ts` behavior tests.

Verify:

- parsing of the versioned protocol;
- client rejection of deltas with missing or mismatched `base_version`;
- local snapshot replacement after `resync_required` and the next full snapshot.

## Acceptance Criteria

This slice is complete when all of the following are true:

- the runtime hot path no longer blocks on WebSocket hub pressure;
- overview messages are versioned and the client validates continuity;
- slow clients receive explicit resync behavior while remaining connected;
- duplicate lifecycle starts do not panic;
- the new runtime behavior is covered by focused backend and frontend regression tests;
- existing metrics can distinguish state-drop, resync, and full-snapshot fallback scenarios.

## Follow-On Work

Deliberately deferred to later slices:

- restore coordinator extraction from `cmd/theia/main.go`;
- broader runtime/service refactors by capability;
- server-side SNMP/Prometheus model normalization;
- broader observability overhaul, frontend quality gates, and production-default database posture.

### Dedicated Follow-On Slice: Overview Payload Slimming

This is a separate slice from runtime-core hardening and should not be folded into the current implementation.

Goals for the payload-slimming slice:

- reduce full overview snapshot size at connect and resync time;
- reduce average delta size for busy fleets;
- keep overview payloads focused on topology and monitoring summary rather than detail-oriented fields.

Likely scope for that slice:

- split overview-safe fields from detail-only fields;
- trim or re-encode `link_metrics` for overview use;
- revisit whether some sections should move out of the overview snapshot entirely;
- evaluate compression or transport-level optimizations only after the payload model is leaner.

Expected outcome of that slice:

- smaller full snapshots;
- smaller resync cost;
- lower browser parse/merge cost during reconnect and high-churn periods.
