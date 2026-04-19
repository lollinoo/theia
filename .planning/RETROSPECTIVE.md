# Project Retrospective

*A living document updated after each milestone. Lessons feed forward into future planning.*

## Milestone: v1.5.8 — Live Refresh Hardening

**Shipped:** 2026-04-19
**Phases:** 4 | **Plans:** 11 | **Sessions:** 4

### What Was Built

- Backend and frontend refresh-path instrumentation for snapshot cost, reload causes, overflow, and canvas/layout timing
- Explicit Prometheus timeout policy and visible runtime request outcome metrics
- Explicit shared-overflow recovery with `resync_required` and trusted full snapshot replacement
- Identity-based canvas reconciliation that keeps runtime-only updates off the structural reload path
- Repeatable 300-device PostgreSQL validation with synthetic, WISP, and slow-Prometheus evidence

### What Worked

- Instrumentation landed before behavior changes, so later tuning and validation were evidence-driven.
- The milestone kept to narrow, testable seam changes instead of broad rewrites in fragile modules.
- Phase-local regression coverage plus the existing scale-lab harness kept validation targeted and fast.

### What Was Inefficient

- Phase 4 needed extra reruns to close browser-proof and live Prometheus evidence gaps that were not paired cleanly on the first pass.
- Nyquist closure lagged behind implementation in Phases 1-3 and became milestone-end debt instead of phase-end closure.

### Patterns Established

- Measure first, then change behavior, then validate at scale.
- Prefer explicit degradation paths such as `resync_required` over implicit freshness heuristics.
- Use topology identity drift, not reconnect events, to decide whether the graph should rebuild.

### Key Lessons

1. Validation artifacts need explicit pairing rules from the first pass, or closeout turns into evidence triage instead of straightforward verification.
2. Explicit timeout and recovery contracts are easier to test and operate than hidden fallback behavior.

### Cost Observations

- Model mix: not captured in milestone artifacts
- Sessions: 4
- Notable: eleven two-task plans kept the milestone small-batch and test-heavy, which reduced rework even when final evidence needed reruns.

---

## Cross-Milestone Trends

### Process Evolution

| Milestone | Sessions | Phases | Key Change |
|-----------|----------|--------|------------|
| v1.5.8 | 4 | 4 | Observability-first hardening followed by explicit recovery semantics and target-scale validation |

### Cumulative Quality

| Milestone | Tests | Coverage | Zero-Dep Additions |
|-----------|-------|----------|-------------------|
| v1.5.8 | Targeted backend, frontend, and scale-lab regression coverage | Milestone requirements 17/17 satisfied | 0 |

### Top Lessons (Verified Across Milestones)

1. Small seam extractions with explicit verification scale better than big-bang rewrites in side-effect-heavy code.
2. Live evidence quality matters as much as implementation quality when milestone acceptance depends on operational proof.
