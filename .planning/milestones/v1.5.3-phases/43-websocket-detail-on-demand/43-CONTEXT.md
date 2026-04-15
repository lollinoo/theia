# Phase 43: WebSocket Detail-on-Demand - Context

**Gathered:** 2026-04-13
**Status:** Ready for planning

<domain>
## Phase Boundary

Add per-client WebSocket detail subscriptions for a selected canvas device so subscribed clients receive richer, more immediate device-level updates without changing the normal overview broadcast path for everyone else. This phase extends the existing WebSocket contract and runtime wiring; it does **not** broaden subscription triggers beyond the canvas device panel, it does **not** change scheduler cadence or add subscription-driven polling, and it does **not** deliver the Phase 44 frontend presentation work for health/freshness/polling labels.

</domain>

<decisions>
## Implementation Decisions

### Detail message contract
- **D-01:** Detail traffic must reuse the existing `snapshot_delta` message type and the shared `SnapshotPayload` merge path. Do **not** introduce a parallel `device_detail` DTO hierarchy or a second frontend state atom for selected-device data.
- **D-02:** Any richer detail fields required for subscriptions must be added as additive optional fields/sections within the shared snapshot contract so overview and detail traffic merge identically on the frontend.

### Subscription lifecycle
- **D-03:** Phase 43 subscriptions are triggered only by opening a device panel from the canvas.
- **D-04:** Closing that canvas device panel, or switching away from it, must send `unsubscribe_detail` and immediately stop per-client detail delivery.
- **D-05:** Dashboard panels, generic selection, and other device surfaces do **not** participate in detail subscriptions in this phase.

### Detail delivery cadence
- **D-06:** Detail-on-demand does **not** change scheduler behavior or poll frequency. Subscription changes delivery timing, not collection timing.
- **D-07:** Subscribed clients should receive detail `snapshot_delta` pushes after the device's normal poll completions rather than waiting for the overview broadcast tick.

### Detail content scope
- **D-08:** Richer detail is limited to backend-computed device-level state needed by later frontend work: health, reachability/freshness, `last_polled_at`, and expected polling interval.
- **D-09:** Per-interface inventory and other device configuration detail remain on the existing REST path in this phase. No new per-interface WebSocket detail section is required for Phase 43.

### the agent's Discretion
- Exact field names and placement for the additive device-level detail metadata inside the shared snapshot payload.
- Exact server-side subscription data structure, locking, and cleanup behavior on disconnect, as long as one active canvas device panel maps cleanly to one active subscription in this phase.
- Whether subscribe sends an immediate cached `snapshot_delta` in addition to post-poll updates, as long as no extra polling is triggered and the shared merge semantics remain intact.

</decisions>

<canonical_refs>
## Canonical References

**Downstream agents MUST read these before planning or implementing.**

### Phase scope and requirement
- `.planning/ROADMAP.md` §Phase 43 — canonical goal, acceptance criteria, and boundary with Phase 44.
- `.planning/REQUIREMENTS.md` — `WS-02` defines the required subscribe/unsubscribe behavior.
- `.planning/PROJECT.md` §Current Milestone — quality tiers/detail-on-demand milestone intent and the decision to keep the existing FNV-64a overview delta approach.
- `.planning/STATE.md` §Accumulated Context / §Blockers — carries forward the explicit note that Phase 43 should resolve scheduler interaction without widening scope.

### Prior phase decisions that constrain Phase 43
- `.planning/phases/38-state-engine/38-CONTEXT.md` — runtime state model and `state.Store` per-device lookup/snapshot seams.
- `.planning/phases/41-jittered-scheduler/41-CONTEXT.md` — current cadence contract and the decision not to reopen scheduler behavior casually.
- `.planning/phases/42-pipeline-orchestrator-cutover/42-CONTEXT.md` — overview WebSocket payload stability and the requirement that later phases remain additive.

### Research guidance
- `.planning/research/SUMMARY.md` §Phase 6: WebSocket Detail-on-Demand — recommended implementation slices and the open scheduler question this context resolves.
- `.planning/research/PITFALLS.md` §Pitfall 8 — avoid frontend split-brain by keeping detail updates on the same snapshot merge path.
- `.planning/research/PITFALLS.md` §detail hashing warnings — if snapshot DTO fields expand, hash computation must stay in sync with the new additive fields.
- `.planning/research/ARCHITECTURE.md` §WebSocket Detail-on-Demand — client subscription tracking, inbound WS message parsing, and targeted detail delivery shape.

### Existing code to modify
- `internal/ws/messages.go` — shared WebSocket message types and snapshot DTOs.
- `internal/ws/hub.go` — client lifecycle, targeted per-client send path, and read-pump behavior.
- `internal/ws/handler.go` — connection bootstrap and client registration.
- `internal/worker/pipeline.go` — post-poll runtime seam for emitting subscribed detail deltas without disturbing overview broadcast timing.
- `internal/state/store.go` — per-device runtime state reads for detail payload construction.
- `frontend/src/hooks/useWebSocket.ts` — single snapshot state atom plus outbound subscribe/unsubscribe support.
- `frontend/src/types/metrics.ts` — shared snapshot payload parser and `snapshot_delta` merge contract.
- `frontend/src/components/Canvas.tsx` — canvas device panel lifecycle that owns subscribe/unsubscribe triggers.
- `frontend/src/components/canvas/CanvasPanels.tsx` — current device panel switch/close behavior.
- `frontend/src/components/InterfaceStatsPanel.tsx` — existing mix of WebSocket snapshot data and REST interface inventory that Phase 43 must not break.

</canonical_refs>

<code_context>
## Existing Code Insights

### Reusable Assets
- `ws.Hub.SendTo(...)` already provides a targeted single-client delivery path that detail subscriptions can build on.
- `state.Store.GetDevice(...)` and `state.Store.Snapshot()` already expose per-device and overview state reads without new storage layers.
- `PipelineOrchestrator.runTask(...)` in `internal/worker/pipeline.go` is the natural post-poll seam for emitting subscribed detail deltas.
- `useWebSocket()` already merges `snapshot_delta` into one frontend snapshot atom, which directly supports the chosen no-split-brain design.
- `Canvas` and `CanvasPanels` already give Phase 43 one clear UI owner for subscribe/unsubscribe events: the canvas device side panel.

### Established Patterns
- The backend/frontend overview contract is built around `snapshot` + `snapshot_delta`, not parallel message families.
- The frontend keeps a single live snapshot state atom; new real-time behavior should merge into that state instead of creating panel-local mirrors.
- The orchestrator already separates poll completion from the 5s overview broadcast tick, which enables an additive detail delivery lane without rewriting overview delivery.
- Canvas UI behavior is single-panel-oriented, making one-device subscription ownership a natural fit for this phase.

### Integration Points
- `internal/ws/hub.go` and `internal/ws/handler.go` need inbound client message handling plus per-client subscription tracking.
- `internal/ws/messages.go` and `frontend/src/types/metrics.ts` need additive snapshot fields for the richer device-level detail metadata.
- `internal/worker/pipeline.go` needs a subscribed-client detail emission path after normal poll completions while preserving the fixed-tick overview broadcast.
- `frontend/src/hooks/useWebSocket.ts` must gain outbound subscribe/unsubscribe support without changing the one-snapshot-state model.
- `frontend/src/components/Canvas.tsx` and `frontend/src/components/canvas/CanvasPanels.tsx` are the intended subscription trigger/cleanup points for this phase.

</code_context>

<specifics>
## Specific Ideas

- Detail updates must stay on `snapshot_delta`; no parallel detail DTO/state path.
- Subscription scope is intentionally narrow: canvas device panel open/close only.
- "Higher frequency" means faster delivery after existing polls, not faster polling.
- Richer detail in Phase 43 is device-level backend state only; per-interface live detail remains deferred.

</specifics>

<deferred>
## Deferred Ideas

- Dedicated `device_detail` WebSocket message types or a separate frontend detail state model.
- Triggering subscriptions from dashboard panels or generic device selection outside the canvas device panel lifecycle.
- Subscription-driven poll cadence boosts or forced one-off polls on subscribe.
- Per-interface live metrics or interface inventory delivery over WebSocket as part of Phase 43.

</deferred>

---

*Phase: 43-websocket-detail-on-demand*
*Context gathered: 2026-04-13*
