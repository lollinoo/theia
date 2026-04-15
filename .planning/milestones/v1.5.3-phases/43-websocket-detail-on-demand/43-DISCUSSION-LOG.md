# Phase 43: WebSocket Detail-on-Demand - Discussion Log

> **Audit trail only.** Do not use as input to planning, research, or execution agents.
> Decisions are captured in CONTEXT.md — this log preserves the alternatives considered.

**Date:** 2026-04-13
**Phase:** 43-websocket-detail-on-demand
**Areas discussed:** Payload contract, Subscription lifecycle, Detail cadence, Detail content

---

## Payload Contract

| Option | Description | Selected |
|--------|-------------|----------|
| `snapshot_delta` only | Reuse the existing `SnapshotPayload` merge path for subscribed detail traffic. | ✓ |
| `device_detail` + separate state | Add a separate detail message type and frontend state path. | |
| `device_detail` normalized into snapshot | Add a second protocol shape, then map it back into the main snapshot state on the client. | |

**User's choice:** `snapshot_delta` only
**Notes:** The user wants one merge path and no panel/canvas split-brain. Any richer detail fields must remain additive within the shared snapshot contract.

---

## Subscription Lifecycle

| Option | Description | Selected |
|--------|-------------|----------|
| Canvas device panel open/close only | Subscribe when a canvas device panel opens; unsubscribe when it closes or switches away. | ✓ |
| Any canvas selection | Subscribe on generic device click/selection, even before a panel opens. | |
| Any device detail surface | Subscribe from dashboard panels and other device surfaces too. | |

**User's choice:** Canvas device panel open/close only
**Notes:** The user explicitly kept Phase 43 aligned with the roadmap's canvas-click scope and did not want broader subscription triggers yet.

---

## Detail Cadence

| Option | Description | Selected |
|--------|-------------|----------|
| Delivery-only acceleration | Keep existing polling cadence; subscribed clients receive detail deltas after normal polls. | ✓ |
| Subscription boosts poll cadence | Increase polling frequency while subscribed. | |
| Hybrid with forced initial poll | Keep current cadence but trigger an extra immediate poll when subscribe starts. | |

**User's choice:** Delivery-only acceleration
**Notes:** The user wants Phase 43 to change delivery timing only. Scheduler reprioritization and forced polls are out of scope here.

---

## Detail Content

| Option | Description | Selected |
|--------|-------------|----------|
| Device-level computed state only | Add health, reachability/freshness, `last_polled_at`, and expected poll interval. | ✓ |
| Device-level + per-interface live metrics | Expand detail payload to include interface-level live data too. | |
| Per-interface live metrics only | Keep device-level computed state for a later phase. | |

**User's choice:** Device-level computed state only
**Notes:** The user wants Phase 43 to prepare the backend-owned device state Phase 44 will present, while leaving per-interface detail on existing REST flows for now.

---

## the agent's Discretion

- Exact field names and placement for additive detail metadata inside the shared snapshot payload.
- Exact server-side subscription bookkeeping and disconnect cleanup implementation.
- Whether to send an immediate cached detail delta on subscribe without triggering extra polling.

## Deferred Ideas

- Dedicated `device_detail` message type / parallel detail state.
- Dashboard-driven or generic-selection-driven subscriptions.
- Subscription-driven poll cadence boosts or forced polls.
- Per-interface live metrics over WebSocket.
