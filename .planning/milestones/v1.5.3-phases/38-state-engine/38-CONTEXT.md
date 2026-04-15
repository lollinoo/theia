# Phase 38: State Engine - Context

**Gathered:** 2026-04-11
**Status:** Ready for planning

<domain>
## Phase Boundary

Backend holds a single source of truth for all live device state — health, severity, metrics, and staleness — and emits only meaningful changes to the WebSocket delta layer. This phase delivers the `internal/state/` package with thread-safe store, health computation, threshold hysteresis, soft/hard state transitions, staleness detection, and diff-based change emission. It does NOT wire the state engine into the existing MetricsCollector/Poller pipeline (that's Phase 42) or add frontend display (Phase 44).

</domain>

<decisions>
## Implementation Decisions

### Health vs reachability model
- **D-01:** Two-dimensional model — separate `HealthStatus` (healthy/warning/critical/unknown) and `ReachabilityStatus` (up/soft_down/hard_down) as independent enums. Health reflects metric threshold evaluation; reachability reflects SNMP poll success/failure.
- **D-02:** When a device goes soft-down or hard-down, its HealthStatus is frozen at the last known value from when the device was reachable. A device that was "critical" before going unreachable stays "critical" until it recovers and gets re-evaluated.
- **D-03:** Per-metric severity stored alongside overall health — `MetricSeverity` (ok/warning/critical) tracked individually for CPU, memory, and temperature. Overall `HealthStatus` = worst-of all individual metric severities.

### Change emission strategy
- **D-04:** Notify channel pattern — state engine exposes `Changes() <-chan []uuid.UUID`. Consumers (MetricsCollector now, PipelineOrchestrator in Phase 42) read changed device IDs, build WS delta payloads from state engine data, and broadcast. State engine stays decoupled from WebSocket serialization.
- **D-05:** Changes are batched per update cycle — each send on the channel is a `[]uuid.UUID` slice of all devices changed in that update. One delta broadcast per batch, not per device.
- **D-06:** State engine exposes `Snapshot() map[uuid.UUID]DeviceState` for atomic point-in-time reads under RLock. Used by ws.Handler for initial client connection snapshot.

### Cache relationship
- **D-07:** State engine coexists with DeviceLinkCache — separate concerns. State engine holds volatile runtime state (metrics, health, reachability, staleness, failure counts). DeviceLinkCache continues to hold DB-backed config data (hostnames, IPs, interfaces, credentials). Phase 42 decides if cache gets absorbed later.
- **D-08:** State engine lives in new `internal/state/` package. Clean separation from `internal/cache/` — different abstraction, different responsibility. Files: `store.go` (Store, DeviceState, types), `health.go` (threshold evaluation, hysteresis logic), plus corresponding test files.

### Staleness detection
- **D-09:** Active staleness tick — state engine runs a background goroutine that periodically checks all devices against their expected poll interval. When a device crosses the 2x threshold, it marks stale and emits the change through the Changes() channel. Staleness appears in real-time.
- **D-10:** Staleness is a third dimension, independent of health and reachability. A device can be up + healthy + stale (poll overdue but last data was good). `Stale bool` + `LastPolledAt time.Time` fields on DeviceState.

### Prior decisions (from STATE.md)
- **D-11:** sync.RWMutex for store concurrency (not channel-based actor) — atomic snapshot reads are a natural fit for RLock
- **D-12:** Hysteresis thresholds: CPU warn 70%/clear 60%, critical 90%/clear 80%; same pattern for memory, temperature
- **D-13:** Hardcoded sensible defaults — configurable thresholds deferred (THRESH-01/02)
- **D-14:** No new third-party dependencies needed — standard library only

### Claude's Discretion
- Staleness tick interval (hardcoded constant — Claude picks the value)
- Exact `StateUpdate` struct shape for the `Update()` method input
- Internal diff computation approach
- Changes channel buffer size
- Whether `health.go` exports threshold constants or keeps them private

</decisions>

<specifics>
## Specific Ideas

No specific requirements — open to standard approaches. User consistently chose the recommended options, favoring clean separation of concerns, decoupled architecture, and information-rich state (per-metric severity, frozen health on unreachable, three-dimensional state model).

</specifics>

<canonical_refs>
## Canonical References

**Downstream agents MUST read these before planning or implementing.**

### State engine requirements
- `.planning/REQUIREMENTS.md` — STATE-01 through STATE-05 define the five acceptance criteria for this phase
- `.planning/ROADMAP.md` §Phase 38 — Success criteria with specific threshold values and behavioral expectations

### Architecture context
- `.planning/codebase/ARCHITECTURE.md` — Layer descriptions, data flow, key abstractions (DeviceLinkCache, ws.Hub, MetricsCollector)
- `internal/cache/cache.go` — DeviceLinkCache implementation that coexists with the new state engine
- `internal/ws/hub.go` — WebSocket hub that consumers use to broadcast state changes
- `internal/ws/messages.go` — SnapshotPayload, DeviceMetricsDTO, and delta message types that the state engine's consumers will build
- `internal/worker/metrics_collector.go` — Current FNV-64a delta broadcast implementation; Phase 42 replaces this but Phase 38 must be compatible with it
- `internal/domain/device.go` — Existing DeviceStatus, DeviceType, MetricsSource enums
- `internal/domain/metrics.go` — Existing DeviceMetrics, LinkMetrics, AlertState types

### Project decisions
- `.planning/STATE.md` §Decisions — sync.RWMutex choice, hysteresis threshold values, hardcoded defaults decision

</canonical_refs>

<code_context>
## Existing Code Insights

### Reusable Assets
- `domain.DeviceMetrics` — existing metrics struct with CPU/mem/temp/uptime fields; state engine wraps these with severity computation
- `domain.DeviceStatus` — existing up/down/probing/unknown enum; state engine introduces richer ReachabilityStatus that eventually supersedes this
- `ws.Hub.Broadcast(msg Message)` — broadcast entry point that consumers of state engine changes will call
- FNV-64a delta mechanism in `MetricsCollector` — existing `sectionHashes` and `buildDelta` pattern that consumers will use to convert state engine changes into WS deltas

### Established Patterns
- Constructor injection via `New*` functions wired in `main.go` — state engine follows same pattern
- `chan struct{}` invalidation signal (used by DeviceLinkCache) — Changes() channel follows similar producer/consumer pattern
- Function types for injectable behavior (`DiscoverFunc`, `SNMPPollFunc`) — state engine's `Update()` method follows same input pattern
- `sync.Mutex` on DeviceLinkCache, `sync.RWMutex` on ws.Hub — state engine uses RWMutex (already decided)

### Integration Points
- `main.go` — will wire `state.NewStore()` and pass it to MetricsCollector (Phase 42 wires to PipelineOrchestrator)
- `internal/worker/metrics_collector.go` — current consumer of device state; Phase 42 replaces it but Phase 38 must define the interface it will consume
- `internal/ws/handler.go` — on new client connect, will eventually call `store.Snapshot()` instead of `lastSnapshot`

</code_context>

<deferred>
## Deferred Ideas

None — discussion stayed within phase scope.

</deferred>

---

*Phase: 38-state-engine*
*Context gathered: 2026-04-11*
