---
phase: 28
slug: api-call-optimization-especially-websocket-payload-optimizat
status: verified
threats_open: 0
asvs_level: 1
created: 2026-04-09
---

# Phase 28 — Security

> Per-phase security contract: threat register, accepted risks, and audit trail.

---

## Trust Boundaries

| Boundary | Description | Data Crossing |
|----------|-------------|---------------|
| Server → WebSocket clients | MetricsCollector pushes delta/full snapshots to all connected browser clients | Device metrics (non-sensitive, same as existing full snapshot) |
| Frontend ↔ WS message handler | useWebSocket processes incoming server-push messages and merges into React state | Parsed metric snapshot data |

No new trust boundaries were introduced. Phase 28 optimizes an existing server-push broadcast mechanism — no new input vectors, no new authentication paths.

---

## Threat Register

### Plan 28-01 — Backend WebSocket Delta (MetricsCollector)

| Threat ID | Category | Component | Disposition | Mitigation | Status |
|-----------|----------|-----------|-------------|------------|--------|
| T-28-01 | DoS | MetricsCollector (FNV-64a hashing) | accept | Hash collisions cause benign extra full-section sends at worst — no security impact, only minor overhead | closed |
| T-28-02 | Information Disclosure | WS delta payloads | accept | Delta payloads contain the same data as full snapshots already broadcast to all clients — no new data exposed | closed |
| T-28-03 | Tampering | prevHashes (server-side in-memory) | accept | prevHashes is server-side only, not exposed to clients — clients cannot influence it | closed |

### Plan 28-02 — Frontend Delta Parsing (useWebSocket)

| Threat ID | Category | Component | Disposition | Mitigation | Status |
|-----------|----------|-----------|-------------|------------|--------|
| T-28-04 | Spoofing | WS snapshot_delta message injection | accept | Same-origin, server-push only WebSocket — clients do not send data; frontend only processes messages from its own server | closed |
| T-28-05 | Tampering | Frontend delta merge (mergeSnapshotDelta) | accept | Additive merge (spread operator); worst case shows stale data for entries not in delta — same risk as corrupted full snapshot | closed |
| T-28-06 | Information Disclosure | Frontend delta message type | accept | No new data exposed; delta fields are a subset of existing full snapshot fields | closed |

*Status: open · closed*
*Disposition: mitigate (implementation required) · accept (documented risk) · transfer (third-party)*

---

## Accepted Risks Log

| Risk ID | Threat Ref | Rationale | Accepted By | Date |
|---------|------------|-----------|-------------|------|
| AR-28-01 | T-28-01 | FNV-64a collisions cause benign overhead (extra send) — not a security impact. Low probability; worst case is slightly suboptimal bandwidth. | gsd-secure-phase | 2026-04-09 |
| AR-28-02 | T-28-02 | Delta payloads expose no new data beyond existing full snapshots. All WS clients are authenticated users of the same session. | gsd-secure-phase | 2026-04-09 |
| AR-28-03 | T-28-03 | prevHashes is in-memory server-side state; no client API to read or modify it. | gsd-secure-phase | 2026-04-09 |
| AR-28-04 | T-28-04 | WebSocket is server-push only (same-origin). Frontend read pump only detects disconnects — clients cannot inject messages. | gsd-secure-phase | 2026-04-09 |
| AR-28-05 | T-28-05 | Additive merge means corrupted delta shows stale/incorrect data for affected devices — no worse than corrupted full snapshot, which is the existing risk. | gsd-secure-phase | 2026-04-09 |
| AR-28-06 | T-28-06 | No new fields or sensitive data introduced. Delta is a sparse subset of existing snapshot schema. | gsd-secure-phase | 2026-04-09 |

---

## Security Audit Trail

| Audit Date | Threats Total | Closed | Open | Run By |
|------------|---------------|--------|------|--------|
| 2026-04-09 | 6 | 6 | 0 | gsd-secure-phase (automated) |

---

## Sign-Off

- [x] All threats have a disposition (mitigate / accept / transfer)
- [x] Accepted risks documented in Accepted Risks Log
- [x] `threats_open: 0` confirmed
- [x] `status: verified` set in frontmatter

**Approval:** verified 2026-04-09
