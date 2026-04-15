# Phase 41: Jittered Scheduler - Discussion Log

> **Audit trail only.** Do not use as input to planning, research, or execution agents.
> Decisions are captured in CONTEXT.md — this log preserves the alternatives considered.

**Date:** 2026-04-12
**Phase:** 41-Jittered Scheduler
**Areas discussed:** Interval contract, Concurrency policy, Resync responsiveness, Backlog behavior

---

## Interval contract

| Option | Description | Selected |
|--------|-------------|----------|
| Honor the current Phase 39 constants | Keep `core=30s`, `standard=60s`, `low=300s`, with `operational=60s` and `static=300s`; focus this phase on scheduler behavior only. | ✓ |
| Correct the interval model now | Rework the interval model in this phase so discovery/static moves to a true minutes-scale cadence and other intervals can be revisited. | |
| Hybrid | Keep current performance intervals, but change only static/discovery to a true minutes-scale cadence now. | |

**User's choice:** Honor the current Phase 39 constants.
**Notes:** User chose to keep Phase 41 narrow and not reopen interval configurability or a deeper cadence-model correction in this phase.

---

## Concurrency policy

| Option | Description | Selected |
|--------|-------------|----------|
| Single global cap, FIFO dispatch | One shared cap, with tasks handled in arrival order. | |
| Single global cap, but priority by volatility | One shared cap, but ready tasks are dispatched `performance` first, then `operational`, then `static`. | ✓ |
| Separate caps per tier | Stronger tier isolation, but more tuning and complexity immediately. | |

**User's choice:** Single global cap, but priority by volatility.
**Notes:** User wanted clear bias toward fast performance polling without introducing multiple independent pools in this phase.

---

## Resync responsiveness

| Option | Description | Selected |
|--------|-------------|----------|
| Periodic refresh only | Scheduler re-reads device inventory on a refresh cadence and does not receive direct push notifications from CRUD paths. | ✓ |
| Immediate push-based updates | Device add/update/remove paths notify the scheduler directly so it can react immediately. | |
| Hybrid | Keep periodic refresh as the source of truth, but add a best-effort poke channel for quicker pickup of obvious changes. | |

**User's choice:** Periodic refresh only.
**Notes:** User preferred loose coupling to the existing cache/invalidation model over a second direct-update path in this phase.

---

## Backlog behavior

| Option | Description | Selected |
|--------|-------------|----------|
| Queue every overdue run | Preserve literal schedule fidelity even if backlog snowballs. | |
| Coalesce to one pending run per device+tier | Collapse duplicate overdue firings so at most one queued/in-flight run exists per `device + volatility class`. | ✓ |
| Allow performance to skip ahead, but queue all static/operational runs | Mixed semantics across tiers to protect fast polls more aggressively. | |

**User's choice:** Coalesce to one pending run per device+tier.
**Notes:** User chose bounded backlog behavior over exact replay of every missed interval.

---

## the agent's Discretion

- Exact refresh cadence for periodic inventory sync.
- Exact heap/queue/channel implementation details.
- Exact source of the global concurrency-limit value.
- Exact post-initial-fire jitter formula, as long as deterministic offsets and burst-avoidance tests hold.

## Deferred Ideas

- Minutes-scale discovery/static cadence rework beyond the current Phase 39 interval model.
- Settings/API/UI interval configurability.
- Push-based scheduler notifications from CRUD/service paths.
- Separate per-tier concurrency pools/caps.
