# Phase 42: Pipeline Orchestrator & Cutover - Discussion Log

> **Audit trail only.** Do not use as input to planning, research, or execution agents.
> Decisions are captured in CONTEXT.md — this log preserves the alternatives considered.

**Date:** 2026-04-13T06:51:12Z
**Phase:** 42-Pipeline Orchestrator & Cutover
**Areas discussed:** Topology persistence during static polls, Live-state fallback when tiers disagree, WebSocket payload strictness at cutover, Prometheus outage signaling after SNMP-primary cutover

---

## Topology Persistence During Static Polls

| Option | Description | Selected |
|--------|-------------|----------|
| Persist static discovery live through the orchestrator. | Static poll results update DB-backed topology and keep `topology_changed` semantics aligned with the current app behavior. | ✓ |
| Keep static discovery runtime-only in Phase 42. | Lower cutover risk but stops topology auto-updates until a manual reprobe or later phase. | |
| Persist only device metadata, not links/interfaces. | Middle ground, but leaves LLDP/CDP topology behavior inconsistent. | |

**User's choice:** Persist static discovery live through the orchestrator.
**Notes:** The user wants the new pipeline to preserve today’s auto-updating topology behavior. `topologyNotify` timing should remain tied to the broadcast path, not emitted directly by poll workers.

---

## Live-State Fallback When Tiers Disagree

| Option | Description | Selected |
|--------|-------------|----------|
| Keep last-known metrics until they become stale. | Overview remains readable during transient performance misses while operational reachability can still advance independently. | ✓ |
| Clear performance metrics immediately on a failed/missed performance poll. | Stricter, but causes cards and links to flicker empty on transient misses. | |
| Keep device metrics but clear link/counter-derived values immediately. | Mixed semantics inside one snapshot; harder to reason about. | |

**User's choice:** Keep last-known metrics until they become stale.
**Notes:** The user prefers operator stability over aggressive nulling. The overview should show "last known until stale" rather than blinking empty when only one tier is temporarily behind.

---

## WebSocket Payload Strictness At Cutover

| Option | Description | Selected |
|--------|-------------|----------|
| Keep Phase 42 payload strict: same overview shape as today. | Lowest regression risk; cutover only changes the backend source of `snapshot` / `snapshot_delta`. | ✓ |
| Emit additive backend-owned fields in Phase 42, but don’t use them yet. | Prepares later phases earlier, but expands the contract during the riskiest integration phase. | |
| Emit only a small additive subset now, like health/freshness timestamps. | Smaller expansion, but still widens the payload contract during cutover. | |

**User's choice:** Keep Phase 42 payload strict: same overview shape as today.
**Notes:** The user wants the cutover phase kept narrow. Phase 42 should replace the backend source of truth without widening the overview WebSocket contract ahead of Phase 44.

---

## Prometheus Outage Signaling After SNMP-Primary Cutover

| Option | Description | Selected |
|--------|-------------|----------|
| Keep explicit `prometheus_status` messages. | Preserves current UX and keeps enrichment outages visible as a distinct concern. | ✓ |
| Make Prometheus silent unless it changes device-visible state. | Cleaner protocol, but operators lose the dedicated enrichment-availability signal. | |
| Remove `prometheus_status` from WS and expose it only via HTTP health endpoints. | Simpler WS protocol, but regresses current behavior. | |

**User's choice:** Keep explicit `prometheus_status` messages.
**Notes:** Even after SNMP becomes the authoritative overview source, the user wants Prometheus enrichment outages to remain visible to clients through the existing explicit WebSocket signal.

---

## the agent's Discretion

- Exact orchestrator internals, worker-pool structure, and helper layout inside `internal/worker/pipeline.go`
- Exact extraction/reuse strategy for topology persistence logic currently living in `service.DeviceService.probeDevice()`
- Exact broadcast tick interval and delta-hash implementation details, as long as they preserve the current overview contract

## Deferred Ideas

- Adding backend-owned overview fields such as health, freshness, and polling metadata before Phase 44
- Simplifying or removing explicit `prometheus_status` signaling during the cutover

