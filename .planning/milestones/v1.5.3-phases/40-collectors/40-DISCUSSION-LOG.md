# Phase 40: Collectors - Discussion Log

> **Audit trail only.** Do not use as input to planning, research, or execution agents.
> Decisions are captured in CONTEXT.md — this log preserves the alternatives considered.

**Date:** 2026-04-12
**Phase:** 40-collectors
**Areas discussed:** Prometheus enrichment, Static collector scope, Bad counter sample behavior, Partial-result policy

---

## Prometheus enrichment

### Core metric authority

| Option | Description | Selected |
|--------|-------------|----------|
| SNMP authoritative, Prometheus enrichment-only | SNMP owns CPU, memory, temperature, uptime, and counters; Prometheus may add enrichment and fill only what SNMP cannot provide. | ✓ |
| SNMP authoritative, minimal Prometheus | Prometheus limited to alerts, hostnames, and probe status only. | |
| Mixed authority | Prometheus may override SNMP for selected metrics when both exist. | |

**User's choice:** SNMP authoritative, Prometheus enrichment-only.
**Notes:** User wants Phase 40 to establish SNMP as the primary source without completely discarding Prometheus for enrichment.

### Link throughput contract

| Option | Description | Selected |
|--------|-------------|----------|
| SNMP only | Link rates come only from SNMP counters in this phase. | ✓ |
| Prometheus can fill gaps | Use Prometheus link rates when SNMP counters are missing or unusable. | |
| You decide | Leave the exact fallback behavior to implementation. | |

**User's choice:** SNMP only.
**Notes:** This keeps link-rate authority consistent with the SNMP-primary decision.

---

## Static collector scope

| Option | Description | Selected |
|--------|-------------|----------|
| Wrap existing discovery path | Reuse the current SNMP discovery logic and return a typed `StaticResult`. | ✓ |
| Migrate static OIDs now | Move inventory/topology discovery definitions into `vendor.StaticOIDs` during this phase. | |
| Hybrid | Start moving only the clean inventory pieces into YAML while keeping LLDP/CDP where they are. | |

**User's choice:** Wrap existing discovery path.
**Notes:** User wants Phase 40 kept narrow; static YAML migration stays deferred.

---

## Bad counter sample behavior

### Reset / gap / sanity-breach handling

| Option | Description | Selected |
|--------|-------------|----------|
| Discard sample, throughput unknown until next clean sample | Throw away the invalid rate and show no throughput for that interval. | ✓ |
| Discard but keep last known good rate visible | Ignore the bad sample but continue showing the previous rate. | |
| Clamp to interface speed | Force absurd rates down to the speed cap. | |
| You decide | Leave the precise invalid-sample behavior to implementation. | |

**User's choice:** Discard sample, throughput unknown until next clean sample.
**Notes:** User preferred correctness over continuity of display.

### Gap baseline rule

| Option | Description | Selected |
|--------|-------------|----------|
| Next poll becomes a fresh baseline | Any bad or missed interval resets the baseline; the next clean poll computes no rate. | ✓ |
| Only explicit collector errors reset the baseline | Long delays alone do not invalidate the next comparison. | |
| You decide | Leave the gap policy to implementation. | |

**User's choice:** Next poll becomes a fresh baseline.
**Notes:** This locks the roadmap’s “skip rate on first poll after a gap” requirement into the phase context.

---

## Partial-result policy

### Missing metric fields

| Option | Description | Selected |
|--------|-------------|----------|
| Best-effort partial results | Return what was collected; missing fields stay nil. | ✓ |
| Strict core set | CPU, memory, and uptime must all exist or the poll fails. | |
| Tiered strictness | Some fields may be optional, but performance metrics still require a stricter minimum set. | |
| You decide | Leave strictness to implementation. | |

**User's choice:** Best-effort partial results.
**Notes:** This matches the current `snmp.PollDeviceMetrics()` behavior and avoids turning vendor variability into hard failures.

### Error definition

| Option | Description | Selected |
|--------|-------------|----------|
| Transport/connect errors fail the poll; missing OIDs do not | Connectivity and execution failures are fatal, field absence is not. | ✓ |
| Any missing expected OID counts as collector error | Missing vendor metrics are treated as poll failure. | |
| You decide | Leave error classification to implementation. | |

**User's choice:** Transport/connect errors fail the poll; missing OIDs do not.
**Notes:** This preserves best-effort collection while still treating real poll failures as errors.

---

## the agent's Discretion

- Exact collector package layout and result type naming.
- Whether Prometheus enrichment lives in a dedicated collector or helper path.
- Exact rate-computation ownership boundary, as long as the locked sample-handling rules are preserved.

## Deferred Ideas

- Static discovery OID migration into vendor YAML.
- Broader Prometheus fallback or override behavior for link throughput and device metrics.
