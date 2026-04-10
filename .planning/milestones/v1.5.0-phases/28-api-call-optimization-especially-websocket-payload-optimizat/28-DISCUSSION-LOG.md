# Phase 28: API Call Optimization — Discussion Log

> **Audit trail only.** Do not use as input to planning, research, or execution agents.
> Decisions captured in CONTEXT.md — this log preserves the discussion.

**Date:** 2026-04-08
**Phase:** 28-api-call-optimization-especially-websocket-payload-optimizat
**Mode:** discuss

## Context Gathered Before Discussion

**Codebase analysis (via Explore subagent):**
- Current system broadcasts full `SnapshotPayload` every polling cycle (~55 KB uncompressed at 77 devices)
- No delta tracking exists at the payload level
- SNMP link counter delta computation already exists (`prevCounters` in `metrics_collector.go:614-643`)
- Existing but unused `MessageTypeMetrics` and `MessageTypeLinkMetrics` constants in `messages.go`
- Hub sends identical serialized bytes to all connected clients
- Frontend replaces entire snapshot state on each `snapshot` message

## Gray Areas Identified

| Area | Question |
|------|----------|
| Delta detection | How to detect which devices changed between cycles |
| Delta protocol | Wire format for delta messages |
| Batching | Whether to add a sub-interval WS push cadence |
| REST scope | Whether REST endpoints are in scope |
| Delta coverage | Which of the 5 SnapshotPayload sections to diff |

## Discussion

### Delta Detection
- **User chose:** Hash-based (Recommended)
- **Over:** Field-level comparison, generation counters
- **Rationale:** Serialize device metrics to canonical string, hash with FNV, compare to previous cycle. Simple, no extra per-field bookkeeping.

### Delta Protocol
- **User chose:** New snapshot_delta type (Recommended)
- **Over:** Hybrid full+partial, reuse existing unused types
- **Rationale:** Clean separation — `snapshot_delta` message type with same payload shape but sparse maps. First connect still gets full snapshot.

### Batching
- **User chose:** Keep 60s cycle, reduce payload only (Recommended)
- **Over:** Add faster WS sub-interval, configurable separate interval
- **Rationale:** Simpler — no new timers, no decoupling from SNMP poll cycle. Benefit comes from reduced payload size.

### REST Scope
- **User chose:** WebSocket only (Recommended)
- **Over:** WS + batch device fetch, full REST optimization
- **Rationale:** REST calls are infrequent; WS is the bottleneck at scale. "Especially" in phase title confirmed WS is primary.

### Delta Coverage
- **User chose:** All 5 sections, per-section diffing (Recommended)
- **Over:** Metrics only, leave alerts always-full
- **Rationale:** All 5 sections diffed for maximum payload reduction. Alerts use whole-set hash (hash all active alerts together) rather than per-alert tracking.

## Corrections Made

No corrections — all recommended options confirmed.
