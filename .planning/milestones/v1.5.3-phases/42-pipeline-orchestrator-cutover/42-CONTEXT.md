# Phase 42: Pipeline Orchestrator & Cutover - Context

**Gathered:** 2026-04-13
**Status:** Ready for planning

<domain>
## Phase Boundary

Replace the legacy `Poller` plus `MetricsCollector` runtime with a single `PipelineOrchestrator` that wires the Phase 41 scheduler to the Phase 40 collectors to the Phase 38 state engine and then back into the existing WebSocket broadcast flow. This phase is the atomic backend cutover: `main.go` must start only the new orchestrator, overview WebSocket clients must keep receiving the existing `snapshot` / `snapshot_delta` contract without regression, and live topology updates discovered by static polls must continue to persist and fan out like they do today.

This phase does **not** widen the overview WebSocket payload shape for frontend consumption, and it does **not** implement Phase 43 detail subscriptions or Phase 44 health/freshness UI features. Those later phases may extend the contract after the cutover is stable.

</domain>

<decisions>
## Implementation Decisions

### Cutover shape and ownership
- **D-01:** Phase 42 is an atomic replacement, not a side-by-side migration. `main.go` must stop wiring `worker.Poller` and `worker.MetricsCollector`; `PipelineOrchestrator` becomes the sole polling, state-update, snapshot, and broadcast path.
- **D-02:** The orchestrator owns the full runtime chain: scheduler task consumption, collector dispatch, state-engine updates, broadcast tick, and Prometheus enrichment health signaling. No old collection loop remains active in parallel.

### Static discovery persistence
- **D-03:** Static collector results should persist discovered topology live through the orchestrator, not stay runtime-only. Device/interface/link discovery changes found on static polls must continue to update persisted topology so the map stays self-healing without manual reprobe.
- **D-04:** `topology_changed` timing stays broadcast-owned. Poll workers may persist topology and signal `topologyNotify`, but the signal must still be drained after the next snapshot/delta broadcast rather than emitted directly from the worker path. This preserves the existing "fresh snapshot first, topology event second" ordering.

### Live-state behavior when polling tiers disagree
- **D-05:** Overview snapshots keep the last-known performance metrics until the stale threshold is crossed, even if a newer performance poll failed or is temporarily late.
- **D-06:** Reachability and status may still evolve independently from operational polls while performance metrics remain last-known. Operators should see "device reachable, metrics last seen recently" rather than cards and links flickering empty on transient misses.
- **D-07:** Clearing performance/link values immediately on a missed performance poll is explicitly not desired for the overview path in this phase.

### WebSocket contract during cutover
- **D-08:** Phase 42 keeps the overview WebSocket payload shape strict: same `snapshot` / `snapshot_delta` sections and same overview-facing field set as today.
- **D-09:** Additive backend-owned fields such as health/freshness/polling metadata are deferred to later phases. The cutover should change the backend source of truth, not expand the client contract during the highest-risk integration phase.
- **D-10:** Because the overview payload stays stable, the new orchestrator must preserve existing frontend semantics around snapshot replacement, delta merging, and reconnect bootstrap.

### Prometheus signaling
- **D-11:** `prometheus_status` remains an explicit WebSocket message after the cutover, even though SNMP is the authoritative overview data source and Prometheus is enrichment-only.
- **D-12:** Prometheus enrichment outages should remain visible to clients without being conflated with SNMP pipeline health. Removing or hiding the current status signal would be a behavior regression in this phase.

### the agent's Discretion
- Exact internal shape of `internal/worker/pipeline.go`, including worker-pool layout, internal channels, and helper boundaries.
- Whether topology persistence logic is extracted from `service.DeviceService.probeDevice()` into a reusable helper/service or reimplemented in a thinner orchestrator-specific path, as long as D-03 and D-04 hold.
- Exact broadcast tick interval and snapshot-build helper structure, provided the fixed-tick broadcast model from research is preserved and overview payload shape remains unchanged.
- Exact strategy for bridging collector results into persisted topology updates and state-store updates, provided the scheduler/collector/state separation remains intact.
- Exact delta-hash implementation approach during cutover, provided it remains compatible with the existing `snapshot_delta` client contract.

</decisions>

<canonical_refs>
## Canonical References

**Downstream agents MUST read these before planning or implementing.**

### Phase scope and milestone contract
- `.planning/ROADMAP.md` §Phase 42 — canonical scope, success criteria, and dependency chain for the orchestrator cutover.
- `.planning/REQUIREMENTS.md` — `PIPE-03` is the direct requirement for this phase; surrounding `POLL-*`, `PIPE-*`, and `WS-*` entries define adjacent contract boundaries.
- `.planning/PROJECT.md` §Current Milestone and §Key Decisions — SNMP-primary direction, preserved FNV-64a delta model, and milestone-level constraints.
- `.planning/STATE.md` §Accumulated Context and §Blockers/Concerns — cross-phase locked decisions and the existing warning that Phase 42 is the highest-risk integration point.

### Prior phase decisions that constrain Phase 42
- `.planning/phases/38-state-engine/38-CONTEXT.md` — state-engine responsibilities, `Changes()` / `Snapshot()` contract, cache coexistence, and reachability/health semantics.
- `.planning/phases/39-domain-types-db-migration/39-CONTEXT.md` — `PollClass`, `VolatilityClass`, per-device override semantics, and domain-layer polling vocabulary.
- `.planning/phases/40-collectors/40-CONTEXT.md` — collector contract, SNMP-primary behavior, best-effort partial results, and static collector scope.
- `.planning/phases/41-jittered-scheduler/41-CONTEXT.md` — global concurrency cap, performance-first priority, periodic inventory refresh, and coalesced backlog behavior.

### Research guidance for the cutover
- `.planning/research/SUMMARY.md` §Phase 5: Pipeline Orchestrator + Cutover — recommended orchestrator responsibilities, atomic cutover rationale, and cutover risk notes.
- `.planning/research/ARCHITECTURE.md` §4 Pipeline Orchestrator (`internal/worker/pipeline.go`) — reference wiring, broadcast-tick model, and state-store-driven snapshot building.
- `.planning/research/ARCHITECTURE.md` §Phase 5: Pipeline Orchestrator — expected deliverables and integration test strategy.
- `.planning/research/PITFALLS.md` — especially the cutover-related pitfalls around Poller/MetricsCollector coupling, cache invalidation scope, topologyNotify timing, delta hashing, and frontend split-brain.

### Existing runtime and integration seams
- `cmd/theia/main.go` — current worker wiring, `topologyNotify`, WebSocket handler construction, and the exact old-to-new cutover point.
- `internal/worker/poller.go` — legacy reprobe loop being removed/replaced.
- `internal/worker/metrics_collector.go` — legacy snapshot building, delta broadcast, Prometheus health signaling, and current `topologyNotify` drain behavior that Phase 42 must preserve semantically.
- `internal/scheduler/scheduler.go` and `internal/scheduler/types.go` — scheduler lifecycle, `Tasks()`, `Complete()`, `PollTask`, and expected interval semantics.
- `internal/collector/results.go` — shared collector result contracts and `ToStoreUpdate()` bridges into the state engine.
- `internal/collector/performance.go` — performance collection semantics and counter snapshot output.
- `internal/collector/operational.go` — operational reachability/uptime semantics.
- `internal/collector/static.go` — static discovery result shape and the source of persisted topology updates.
- `internal/state/store.go` — state update, snapshot, stale handling, and change emission contracts.
- `internal/service/device_service.go` — current topology persistence behavior and `topologyNotify` signaling to preserve or extract.
- `internal/cache/cache.go` — inventory cache semantics the scheduler currently relies on.
- `internal/ws/handler.go` — initial snapshot and Prometheus-status bootstrap contract that the new orchestrator must continue to satisfy.
- `internal/ws/messages.go` — current `SnapshotPayload`, DTOs, and message types that Phase 42 must preserve.

### Frontend contract to preserve during cutover
- `frontend/src/hooks/useWebSocket.ts` — current `snapshot`, `snapshot_delta`, `prometheus_status`, and `topology_changed` client handling.
- `frontend/src/types/metrics.ts` — frontend parser and delta-merge contract that must remain backward-compatible through the cutover.

</canonical_refs>

<code_context>
## Existing Code Insights

### Reusable Assets
- `scheduler.NewScheduler(...)` plus `Tasks()` / `Complete()` already provide the orchestrator input queue and completion feedback loop needed for worker dispatch.
- `collector.PerformanceCollector`, `OperationalCollector`, and `StaticCollector` already expose the three volatility-specific polling surfaces the orchestrator must call.
- `collector/results.go` already provides typed result contracts and `ToStoreUpdate()` helpers for bridging collector output into `state.Store.Update()`.
- `state.Store` already provides the in-memory runtime source of truth via `Update()`, `Snapshot()`, `Changes()`, `Start()`, `Stop()`, and stale tracking.
- `ws.Handler` already abstracts "current snapshot" and "current Prometheus availability" behind injected getter functions, so the cutover path in `main.go` is narrow.

### Established Patterns
- Background workers in this codebase use `Start(ctx)` / `Stop()` lifecycle methods with internal `done` channels and clear ownership of goroutine shutdown.
- The frontend expects one overview snapshot state atom fed by `snapshot` and `snapshot_delta`; Phase 42 should not introduce a second parallel message model.
- `topologyNotify` is intentionally drained after broadcast in the legacy collector so clients receive fresh snapshot data before `topology_changed`; preserve that ordering.
- Scheduler inventory synchronization is already pull-based and periodic via `DeviceLinkCache`; Phase 42 should integrate with that instead of inventing push-based invalidation wiring mid-cutover.

### Integration Points
- `cmd/theia/main.go` is the single authoritative cutover site: remove old worker construction, construct the orchestrator, and pass orchestrator snapshot/status getters to `ws.NewHandler(...)`.
- `internal/worker/pipeline.go` should become the runtime owner that connects scheduler task dispatch, collector execution, state updates, topology persistence, fixed-tick broadcasts, and Prometheus health reporting.
- Topology persistence likely needs an extracted seam from `DeviceService.probeDevice()` or equivalent helper so static collector results can update DB-backed device/link topology without reusing the old Poller path.
- The broadcast path must continue producing `SnapshotPayload` compatible with `frontend/src/types/metrics.ts` and current delta merging.

</code_context>

<specifics>
## Specific Ideas

- The user wants Phase 42 narrowly focused on the backend cutover, not on sneaking Phase 44 payload changes into the riskiest integration phase.
- The user explicitly wants topology auto-updates preserved under the new pipeline rather than deferred behind manual reprobes.
- The user prefers operator-readable stability over aggressive nulling: last-known metrics should remain visible until the state is genuinely stale.
- The current explicit `prometheus_status` signal should survive the cutover so enrichment outages remain visible as a distinct concern from SNMP polling health.

</specifics>

<deferred>
## Deferred Ideas

- Emitting additive overview snapshot fields such as backend-computed health, freshness, or polling metadata is deferred to later milestone phases, primarily Phase 44.
- Any protocol simplification that removes or hides `prometheus_status` is deferred; Phase 42 prioritizes behavioral parity during cutover.
- Runtime-only static discovery behavior was rejected for this phase; if revisited later, it should be treated as a deliberate product change rather than a cutover shortcut.

</deferred>

---

*Phase: 42-pipeline-orchestrator-cutover*
*Context gathered: 2026-04-13*
