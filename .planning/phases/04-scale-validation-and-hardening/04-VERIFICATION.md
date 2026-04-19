---
phase: 04-scale-validation-and-hardening
status: partial
verified_at: 2026-04-19T12:00:00Z
requirements: [SCAL-01, SCAL-02, SCAL-03]
human_verification:
  - Inspect `window.__THEIA_CANVAS_METRICS__` in a live browser session on `http://localhost:3000`
---

# Phase 4 Verification

## Result

No in-scope backend or frontend hardening change was necessary for Plan `04-02`. The required reruns completed and produced fresh `synthetic-final/` and `wisp-final/` evidence, but final sign-off remains partial because this terminal-only environment could not execute the live browser proof for `window.__THEIA_CANVAS_METRICS__`, and the dev-postgres rerun did not exercise live Prometheus runtime traffic.

## Automated Checks

- `go test ./internal/worker ./internal/ws ./internal/observability -count=1` passed during Task 1 triage.
- `cd frontend && npm test -- --run src/components/canvas/useCanvasData.test.ts src/hooks/useWebSocket.test.ts` passed during Task 1 triage.
- `make dev-postgres` started the documented PostgreSQL stack and `make seed` completed successfully before the final evidence reruns.
- `bash scripts/phase4-validate.sh synthetic http://localhost:8080 .planning/phases/04-scale-validation-and-hardening/evidence/synthetic-final` passed.
- `bash scripts/phase4-validate.sh wisp http://localhost:8080 .planning/phases/04-scale-validation-and-hardening/evidence/wisp-final` passed.
- `go test ./internal/scalelab ./internal/worker ./internal/ws ./internal/observability -count=1` passed.
- `cd frontend && npm test -- --run src/components/canvas/useCanvasData.test.ts src/hooks/useWebSocket.test.ts && npm run build` passed.

## Scenario Matrix

| scenario | coverage | result | evidence |
| --- | --- | --- | --- |
| Synthetic 300-device baseline | Live rerun against the PostgreSQL dev stack plus scale-lab baseline replay | pass | `.planning/phases/04-scale-validation-and-hardening/evidence/synthetic-final/scale-300-baseline.json`; `.planning/phases/04-scale-validation-and-hardening/evidence/synthetic-final/metrics.prom` |
| Realistic-slice browser proof | WISP hybrid replay plus live metrics capture; required browser proof remains manual | partial | `.planning/phases/04-scale-validation-and-hardening/evidence/wisp-final/scale-wisp-hybrid.json`; `.planning/phases/04-scale-validation-and-hardening/evidence/wisp-final/metrics.prom`; `window.__THEIA_CANVAS_METRICS__` manual step from `04-VALIDATION-RUNBOOK.md` |
| Slow Prometheus responses | Existing targeted backend coverage from Phase 2 plus this plan’s rerun evidence | partial | `.planning/phases/02-backend-live-state-resilience/02-VERIFICATION.md`; `go test ./internal/scalelab ./internal/worker ./internal/ws ./internal/observability -count=1`; live rerun logs showed Prometheus runtime integration disabled |
| Reconnect storms | Existing frontend and backend regression coverage from Phases 2 and 3 | pass (automated) | `.planning/phases/03-frontend-incremental-reconciliation/03-VERIFICATION.md`; `cd frontend && npm test -- --run src/hooks/useWebSocket.test.ts src/components/canvas/useCanvasData.test.ts` |
| Topology-change storms | Existing frontend structural refresh and coalescing regression coverage from Phase 3 | pass (automated) | `.planning/phases/03-frontend-incremental-reconciliation/03-VERIFICATION.md`; `cd frontend && npm test -- --run src/components/canvas/useCanvasData.test.ts` |
| Slow-client or backpressure coverage | Existing explicit resync and overflow regression coverage from Phase 2 plus zero-drop live metrics | pass (automated) | `.planning/phases/02-backend-live-state-resilience/02-VERIFICATION.md`; `go test ./internal/scalelab ./internal/worker ./internal/ws ./internal/observability -count=1`; `.planning/phases/04-scale-validation-and-hardening/evidence/synthetic-final/metrics.prom`; `.planning/phases/04-scale-validation-and-hardening/evidence/wisp-final/metrics.prom` |

## Requirement Traceability

- `SCAL-01` Partial pass: the target `300` profile reran successfully in `.planning/phases/04-scale-validation-and-hardening/evidence/synthetic-final/scale-300-baseline.json` and `.planning/phases/04-scale-validation-and-hardening/evidence/synthetic-final/scale-300-burst-adds.json`. The PostgreSQL-backed live stack also emitted the required refresh metric families in `.planning/phases/04-scale-validation-and-hardening/evidence/synthetic-final/metrics.prom`, including `theia_refresh_snapshot_build_seconds`, `theia_refresh_topology_reload_total`, and `theia_state_changes_dropped_total`.
- `SCAL-02` Partial pass: the realistic supplement reran in `.planning/phases/04-scale-validation-and-hardening/evidence/wisp-final/scale-wisp-hybrid.json`, and both final metrics captures preserve `theia_refresh_snapshot_build_seconds`, `theia_refresh_topology_reload_total`, and `theia_state_changes_dropped_total`. The required browser surface `window.__THEIA_CANVAS_METRICS__` was not inspected from this terminal environment, so live proof that runtime-only updates stayed off the structural reload path is still missing.
- `SCAL-03` Partial pass: the final evidence directories plus Phase 2 and Phase 3 targeted tests cover the requested scenario matrix, and the live metrics continue to show `theia_state_changes_dropped_total 0`. The live rerun did not exercise `window.__THEIA_CANVAS_METRICS__`, and the stack did not emit active `theia_prometheus_runtime_requests_total` traffic during the final run, so slow-Prometheus coverage remains automated-only in this environment.

## Human Verification

- Not executed here: `window.__THEIA_CANVAS_METRICS__` requires a real browser session on `http://localhost:3000`.
- Required follow-up:
  1. Start the documented stack with `make dev-postgres` and `make seed`.
  2. Open the canvas in a browser and inspect `window.__THEIA_CANVAS_METRICS__`.
  3. Confirm runtime-only activity produces `theia:canvas:snapshot-apply` entries without unexpected `theia:canvas:topology-load` or `theia:canvas:layout` churn.
  4. Record the matching evidence path alongside the browser notes: `.planning/phases/04-scale-validation-and-hardening/evidence/synthetic-final/metrics.prom` or `.planning/phases/04-scale-validation-and-hardening/evidence/wisp-final/metrics.prom`.

## Gaps

- Manual browser proof is still open. `window.__THEIA_CANVAS_METRICS__` could not be evaluated from this terminal environment.
- Final live `/metrics` captures show `theia_refresh_snapshot_build_seconds_count{mode="dirty",result="success"} 0`, so the rerun evidence did not demonstrate runtime-only dirty snapshot activity in the live stack.
- Final live `/metrics` captures show only `theia_refresh_topology_reload_total{reason="startup"} 1`; no reconnect, topology-change, or overflow-induced reload reason was exercised live during the rerun.
- The dev-postgres rerun logged Prometheus runtime integration as disabled, so live slow-Prometheus behavior was not exercised even though Phase 2 automated coverage remains in place.
