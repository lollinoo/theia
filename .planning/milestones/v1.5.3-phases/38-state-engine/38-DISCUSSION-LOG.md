# Phase 38: State Engine - Discussion Log

> **Audit trail only.** Do not use as input to planning, research, or execution agents.
> Decisions are captured in CONTEXT.md — this log preserves the alternatives considered.

**Date:** 2026-04-11
**Phase:** 38-state-engine
**Areas discussed:** Health vs reachability model, Change emission strategy, Cache relationship, Staleness tick

---

## Health vs reachability model

| Option | Description | Selected |
|--------|-------------|----------|
| Two-dimensional (Recommended) | Separate HealthStatus and ReachabilityStatus enums | ✓ |
| Single merged enum | One OverallStatus combining both dimensions | |
| You decide | Let Claude choose | |

**User's choice:** Two-dimensional — separate HealthStatus (healthy/warning/critical/unknown) and ReachabilityStatus (up/soft_down/hard_down)
**Notes:** None

---

| Option | Description | Selected |
|--------|-------------|----------|
| Freeze last known (Recommended) | Keep last computed HealthStatus when unreachable | ✓ |
| Set to unknown | HealthStatus becomes unknown when unreachable | |
| You decide | Let Claude choose | |

**User's choice:** Freeze last known health on unreachable
**Notes:** None

---

| Option | Description | Selected |
|--------|-------------|----------|
| Per-metric severity (Recommended) | Store severity per metric alongside overall health | ✓ |
| Worst-of only | Just the single HealthStatus enum | |
| You decide | Let Claude choose | |

**User's choice:** Per-metric severity — individual MetricSeverity for CPU, memory, temperature
**Notes:** None

---

## Change emission strategy

| Option | Description | Selected |
|--------|-------------|----------|
| Notify channel (Recommended) | Changes() <-chan []uuid.UUID, decoupled from WS | ✓ |
| Direct WS push | State engine takes *ws.Hub, broadcasts directly | |
| Callback func | OnChange func injected at construction | |

**User's choice:** Notify channel — state engine decoupled from WebSocket concerns
**Notes:** None

---

| Option | Description | Selected |
|--------|-------------|----------|
| Batched per update (Recommended) | One []uuid.UUID slice per update cycle | ✓ |
| One ID at a time | Each changed device emits individually | |
| You decide | Let Claude choose | |

**User's choice:** Batched per update cycle
**Notes:** None

---

| Option | Description | Selected |
|--------|-------------|----------|
| Yes, atomic snapshot (Recommended) | Snapshot() under RLock for initial WS client connect | ✓ |
| Not now | Defer snapshot API to Phase 42 | |
| You decide | Let Claude choose | |

**User's choice:** Yes, atomic Snapshot() method under RLock
**Notes:** None

---

## Cache relationship

| Option | Description | Selected |
|--------|-------------|----------|
| Coexist, separate concerns (Recommended) | State engine = volatile state, DeviceLinkCache = DB config | ✓ |
| State engine absorbs cache | One store for both config and volatile state | |
| You decide | Let Claude choose | |

**User's choice:** Coexist — state engine for volatile runtime state, DeviceLinkCache for DB-backed config
**Notes:** None

---

| Option | Description | Selected |
|--------|-------------|----------|
| New internal/state/ (Recommended) | New package, clean separation | ✓ |
| Extend internal/cache/ | Add to existing cache package | |
| You decide | Let Claude choose | |

**User's choice:** New `internal/state/` package
**Notes:** None

---

## Staleness tick

| Option | Description | Selected |
|--------|-------------|----------|
| Active tick (Recommended) | Background goroutine checks periodically, emits changes | ✓ |
| Passive on read | Staleness computed on the fly when state is read | |
| You decide | Let Claude choose | |

**User's choice:** Active staleness tick with background goroutine
**Notes:** None

---

| Option | Description | Selected |
|--------|-------------|----------|
| Hardcoded 10s (Recommended) | Simple, predictable tick interval | |
| Derived from intervals | Adaptive based on device poll intervals | |
| You decide | Let Claude choose | ✓ |

**User's choice:** You decide — Claude's discretion on tick interval
**Notes:** None

---

| Option | Description | Selected |
|--------|-------------|----------|
| Staleness is a third dimension (Recommended) | Independent bool field, doesn't affect health/reachability | ✓ |
| Stale degrades reachability | Extended staleness triggers soft-down transition | |
| You decide | Let Claude choose | |

**User's choice:** Staleness as independent third dimension — Stale bool + LastPolledAt on DeviceState
**Notes:** None

---

## Claude's Discretion

- Staleness tick interval value
- StateUpdate struct shape
- Internal diff computation approach
- Changes channel buffer size
- Threshold constant visibility (exported vs private)

## Deferred Ideas

None — discussion stayed within phase scope.
