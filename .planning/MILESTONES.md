# Milestones

## v1.5.8 — Live Refresh Hardening

**Shipped:** 2026-04-19
**Phases:** 4
**Plans:** 11
**Tasks:** 22
**Git Range:** `a395a76` → `4dbfe2a`
**Archive:** [ROADMAP](./milestones/v1.5.8-ROADMAP.md) · [REQUIREMENTS](./milestones/v1.5.8-REQUIREMENTS.md) · [AUDIT](./milestones/v1.5.8-MILESTONE-AUDIT.md)

### Delivered

- Instrumented the backend refresh path and the browser canvas so snapshot cost, reload reasons, overflow, and layout churn became measurable.
- Bounded Prometheus runtime work and made shared-overflow recovery explicit through `resync_required` plus full snapshot replacement.
- Refactored frontend reconciliation so runtime-only changes stop short of structural reload, layout churn, and viewport reset.
- Validated the hardened model at 300 devices on PostgreSQL with repeatable synthetic and WISP evidence, plus live Prometheus success and timeout captures.

### Known Deferred Items

- 5 closeout debt items remain accepted at archive time. See `.planning/STATE.md` and the archived milestone audit for details.
