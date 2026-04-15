# Phase 41: Jittered Scheduler - Context

**Gathered:** 2026-04-12
**Status:** Ready for planning

<domain>
## Phase Boundary

Introduce the scheduler layer that decides when each device/volatility-class poll should run, using deterministic per-device offsets and a bounded concurrency model. This phase separates `performance`, `operational`, and `static` scheduling, emits work for the existing collectors, and prevents bursty thundering-herd behavior. It does **not** cut over `main.go`, replace the current `Poller`/`MetricsCollector` wiring, add UI/API override surfaces, or revisit the Phase 39 poll-class model.

</domain>

<decisions>
## Implementation Decisions

### Interval contract
- **D-01:** Phase 41 honors the current Phase 39 interval model rather than reopening interval configurability in this phase.
- **D-02:** Performance polling continues to use the existing `PollClass` durations from `internal/domain/poll_class.go`: `core=30s`, `standard=60s`, `low=300s`.
- **D-03:** Operational polling remains a shared system interval of `60s` for all devices.
- **D-04:** Static/discovery polling remains a shared system interval of `300s` for all devices in this phase, even though broader roadmap language mentions a slower minutes-scale window.

### Concurrency policy
- **D-05:** Use one global SNMP concurrency cap across all scheduler work. Do not introduce separate per-tier pools/caps in this phase.
- **D-06:** When multiple tasks are ready at the same time, dispatch by volatility priority: `performance` first, then `operational`, then `static`.

### Inventory sync model
- **D-07:** Scheduler inventory updates are pull-based and periodic. The scheduler re-syncs devices from the existing cache/repository view on a refresh cadence rather than receiving direct push notifications from CRUD/service paths in this phase.

### Backlog behavior
- **D-08:** Backlog is coalesced to at most one pending run per `device + volatility class`. If a run is already queued or in flight, additional due firings collapse into one pending rerun instead of building an unbounded backlog.
- **D-09:** After a coalesced run completes, the scheduler advances from the latest completion/reinsert point rather than replaying every missed interval.

### the agent's Discretion
- Exact scheduler internals: heap/priority-queue layout, channel structure, task-buffer size, and lifecycle plumbing.
- Exact periodic refresh cadence for device re-sync.
- Exact source of the global concurrency limit (reuse existing worker setting/helper vs add a scheduler-local helper), as long as D-05/D-06 hold.
- Exact post-initial-fire jitter formula beyond the required deterministic FNV-based offset, as long as poll distribution tests prove burst avoidance.
- Whether queue priority is implemented via heap ordering, a ready queue, or equivalent internal structure, as long as it behaves per D-06 and D-08.

</decisions>

<canonical_refs>
## Canonical References

**Downstream agents MUST read these before planning or implementing.**

### Phase scope and locked milestone requirements
- `.planning/ROADMAP.md` §Phase 41 — phase goal, dependency chain, and success criteria for scheduler separation, deterministic offsets, and concurrency limiting.
- `.planning/REQUIREMENTS.md` — `POLL-01`, `POLL-02`, and `POLL-04` define the roadmap-level acceptance targets this phase maps to.
- `.planning/PROJECT.md` §Current Milestone and §Key Decisions — milestone intent, hardcoded-defaults bias, and the requirement to preserve the existing FNV delta WS architecture.
- `.planning/STATE.md` §Accumulated Context — current cross-phase decisions already locked for v1.5.3.

### Prior phase decisions that constrain this phase
- `.planning/phases/39-domain-types-db-migration/39-CONTEXT.md` — `PollClass`/`VolatilityClass`, current hardcoded intervals, and the rule that `PollClass` governs only performance cadence.
- `.planning/phases/40-collectors/40-CONTEXT.md` — collector contracts and the expectation that scheduler output is consumed by the existing `performance` / `operational` / `static` collectors.
- `.planning/phases/38-state-engine/38-CONTEXT.md` — hardcoded-defaults-now pattern and the state-engine/runtime constraints downstream pipeline work must preserve.

### Research guidance for the scheduler shape
- `.planning/research/SUMMARY.md` §Phase 4 Scheduler — recommended deliverables: single-goroutine scheduler, deterministic offset, device sync from cache, and worker-pool consumption model.
- `.planning/research/ARCHITECTURE.md` §Scheduler (`internal/scheduler/`) — reference shape for `PollTask`, scheduler responsibilities, and separation from later pipeline cutover.
- `.planning/research/PITFALLS.md` — anti-patterns to avoid: per-device goroutine timers, scheduler/poller cadence fights, and unbounded backlog behavior.

### Existing code that defines the integration surface
- `internal/domain/poll_class.go` — authoritative current interval constants and `PollClass` semantics this phase chose to honor.
- `internal/collector/results.go` — collector result contracts and volatility-class vocabulary the scheduler must emit work for.
- `internal/cache/cache.go` — invalidation-driven `DeviceLinkCache` behavior that supports the chosen periodic refresh model.
- `internal/worker/poller.go` — current semaphore-bounded worker pattern and background lifecycle shape to mirror where useful.
- `internal/worker/device_backup_scheduler.go` — existing scheduler-style `Start`/`Stop` lifecycle pattern for long-running background loops.
- `cmd/theia/main.go` — current worker wiring and eventual future integration point, even though this phase does not perform the cutover.

</canonical_refs>

<code_context>
## Existing Code Insights

### Reusable Assets
- `internal/domain/poll_class.go`: already provides `PollClass`, `VolatilityClass`, and the interval constants the scheduler should consume directly.
- `internal/collector/results.go`: defines the volatility-class vocabulary and typed collector surfaces the scheduler will target via `PollTask` emission.
- `internal/cache/cache.go`: already gives a simple, invalidation-aware way to periodically re-read the device inventory without coupling the scheduler to CRUD events.
- `internal/worker/poller.go`: demonstrates the existing bounded worker/semaphore pattern and background worker lifecycle used elsewhere in the backend.
- `internal/worker/device_backup_scheduler.go`: shows a second scheduler-style loop with `Start(ctx)` / `Stop()` / `done` handling that fits this codebase's conventions.

### Established Patterns
- Long-running backend workers use `Start(ctx)` / `Stop()` plus an internal `done` channel and atomic running state.
- The existing codebase prefers narrow helpers over broad framework additions; the scheduler should stay backend-local and dependency-light.
- Cache invalidation is pull-on-next-read, not rich event streaming. The chosen periodic refresh model aligns with that.
- Hardcoded operational defaults are acceptable in this milestone when they avoid premature settings/UI surface area.

### Integration Points
- Scheduler inventory source should be the current device view from `DeviceLinkCache.GetDevices()` or an equivalent repo-backed sync path.
- Phase 42 will consume the scheduler's `PollTask` output and wire it to collectors, the state engine, and WS broadcast.
- `cmd/theia/main.go` remains the eventual integration point for worker startup/shutdown, but Phase 41 itself stays below the cutover line.

</code_context>

<specifics>
## Specific Ideas

- User explicitly chose to keep the current Phase 39 interval contract in place for this phase rather than broadening scope into interval configurability or a deeper discovery-cadence rework.
- User wants one shared concurrency cap, but with clear priority for `performance` work when multiple tasks compete for execution slots.
- User preferred loose coupling over immediacy for inventory changes: periodic refresh is acceptable, direct push wiring is not required in this phase.

</specifics>

<deferred>
## Deferred Ideas

- Minutes-scale static/discovery cadence or broader interval-model correction beyond the Phase 39 constants. Not chosen for Phase 41.
- Settings/API/UI-driven interval configurability. Still deferred to a later phase.
- Direct push-based scheduler notifications from CRUD/service paths. Not chosen for Phase 41.
- Separate per-tier concurrency caps or stronger isolation pools. Not chosen for Phase 41.
- Queue-every-overdue-run semantics. Rejected in favor of per-device/per-tier coalescing.

</deferred>

---

*Phase: 41-jittered-scheduler*
*Context gathered: 2026-04-12*
