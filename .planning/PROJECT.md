# Theia

## Current State

Theia shipped **v1.5.8 Live Refresh Hardening** on 2026-04-19. This milestone added backend and browser-visible refresh-path instrumentation, explicit Prometheus timeout and resync behavior, identity-based incremental canvas reconciliation, and repeatable 300-device PostgreSQL validation evidence.

The shipped state is operationally stronger without changing the core contract: topology still bootstraps over REST, and runtime state still overlays through WebSocket snapshots and deltas.

## Next Milestone Goals

- Define a fresh milestone and requirements with `$gsd-new-milestone`.
- Decide whether the next milestone should prioritize validation-proof follow-up debt, architecture follow-ups, or broader product expansion.
- Refill the Active requirements list before adding any new roadmap phases.

## Deferred Follow-Up At Close

- Finish Nyquist closure for Phases 1-3.
- Pair the `SCAL-02` browser proof to one exact successful `metrics.prom` artifact.
- Add one dedicated live reconnect-storm or slow-client fault-injection path if the next milestone needs stronger target-scale proof.

<details>
<summary>Project reference at v1.5.8 ship</summary>

## What This Is

Theia is a network monitoring platform for daily operators that fetches and enriches device data through SNMP and Prometheus. It currently focuses on MikroTik environments, but the architecture is intended to expand to additional vendors over time. The existing system already combines a persisted topology model with live runtime updates rendered in a browser canvas.

## Core Value

Operators can trust the topology view and live device state to stay accurate and responsive as the monitored fleet grows.

## Requirements

### Validated

- ✓ Operators can manage monitored devices, links, positions, and related topology inventory through the existing backend and UI workflows — existing
- ✓ The platform can collect and enrich runtime data through SNMP and Prometheus, then publish live snapshots and deltas over WebSocket — existing
- ✓ Operators can view topology on a canvas with persisted positions plus live overlays for health, metrics, alerts, and reachability — existing
- ✓ The system already supports operational workflows beyond monitoring, including vendor-driven behavior and backup/restore capabilities — existing
- ✓ The system exposes actionable measurements for snapshot construction, topology reload causes, queue overflow, scheduler lag, and canvas/layout cost before and after fixes — validated in Phase 1
- ✓ Prometheus enrichment and alert queries have hard timeouts plus observability for latency, timeout, and error behavior — validated in Phase 2
- ✓ Backpressure in runtime state propagation degrades explicitly to full snapshot/resync flows instead of implicit freshness loss — validated in Phase 2
- ✓ Runtime-only updates for health, metrics, alerts, and reachability do not trigger full topology reloads or unnecessary graph rebuilds — validated in Phase 4
- ✓ The canvas remains stable and responsive while monitoring 300 devices on PostgreSQL in the real Docker stack — validated in Phase 4
- ✓ Target-scale validation now includes live Prometheus runtime success and timeout evidence on the dev PostgreSQL stack — validated in Phase 4

### Active

- None. The current milestone goals were validated through Phase 4.

### Out of Scope

- Broad rewrite of the frontend canvas or the backend live-update architecture — this milestone should harden the current REST bootstrap + WebSocket reconciliation contract, not replace it
- Large-scale refactoring of every high-side-effect module — only small extractions directly needed to reduce risk in the live refresh work belong in scope
- New vendor expansion beyond the current MikroTik focus — vendor breadth is deferred until the live refresh path is stable
- Authentication, authorization, and wider security hardening — important, but not part of this milestone's performance/resilience goal

## Context

Theia is a brownfield project with an existing Go backend, React frontend, PostgreSQL/SQLite persistence layer, and a live polling/broadcast pipeline that already supports topology visualization and runtime monitoring. Multiple operators are expected to use it every day, so perceived UI stability matters as much as backend correctness.

The current milestone was driven by scaling pain in the live refresh path. The intended system contract is already visible in the codebase and project notes: persistent topology should bootstrap through REST, while runtime state should flow through WebSocket snapshots and deltas without rebuilding the graph unless nodes or links actually change.

The clearest bottlenecks were concentrated in four areas: frontend topology reloads and layout work, Prometheus calls without bounded client timeouts, fixed-size drop-on-backpressure queues in runtime propagation, and high side-effect density in a handful of large modules. The existing `.planning/codebase/` documents remained the primary reference for current architecture and fragile areas.

Validation leaned on what the repository already provided: targeted backend and frontend tests plus the `cmd/theia-scale-lab` harness for synthetic load and burst scenarios. Phase 4 completed with repeatable synthetic and WISP evidence, browser proof for runtime-only canvas updates, and live Prometheus success and timeout validation without requiring additional seam-local code changes.

## Constraints

- **Architecture**: Keep the REST topology bootstrap plus WebSocket runtime overlay pattern — because this is already the intended contract and changing it now would increase risk
- **Scope**: Focus this milestone on performance and resilience of live refresh behavior — because the current need is operational stability, not a broader product redesign
- **Scale Target**: Optimize and validate against 300 devices on PostgreSQL in the real Docker stack — because this is the concrete acceptance target
- **Vendor Focus**: Prioritize MikroTik support in the current work — because that is where active development is concentrated today
- **Change Strategy**: Prefer small, testable extractions over big-bang rewrites in large modules — because the existing high side-effect density makes broad edits fragile
- **Verification**: Prefer targeted integration and scale scenarios over introducing a large new E2E suite — because the repository already has test hooks and a scale harness that fit this problem

## Key Decisions

| Decision | Rationale | Outcome |
|----------|-----------|---------|
| Keep static topology loaded via REST and reconcile runtime state via WebSocket | The existing architecture already separates persistent topology from volatile runtime state; the main issue is contract discipline, not the overall pattern | Phases 3-4 complete |
| Restrict topology reloads to real structural changes in nodes or links | Runtime-only changes should not force graph rebuilds, re-layouts, or costly canvas recovery | Phases 3-4 complete |
| Add observability before major behavior changes | Prometheus latency, snapshot cost, queue overflow, and canvas rebuild cost need measurement before tuning | Phase 1 complete |
| Prioritize bounded Prometheus queries and explicit backpressure recovery in the first intervention | These are the clearest backend paths that can stall workers or hide stale state under load | Phase 2 complete |
| Validate against 300 devices on PostgreSQL in the real Docker stack | Success must be measured in the target operating environment, not only in local low-scale development | Phase 4 complete |
| Close slow-Prometheus validation with live success and timeout evidence instead of broader code changes | The remaining risk was verification coverage, not a reproduced seam defect in the implementation | Phase 4 complete |

## Evolution

This document evolves at phase transitions and milestone boundaries.

**After each phase transition** (via `$gsd-transition`):
1. Requirements invalidated? → Move to Out of Scope with reason
2. Requirements validated? → Move to Validated with phase reference
3. New requirements emerged? → Add to Active
4. Decisions to log? → Add to Key Decisions
5. "What This Is" still accurate? → Update if drifted

**After each milestone** (via `$gsd-complete-milestone`):
1. Full review of all sections
2. Core Value check — still the right priority?
3. Audit Out of Scope — reasons still valid?
4. Update Context with current state

</details>

---
*Last updated: 2026-04-19 after v1.5.8 milestone*
