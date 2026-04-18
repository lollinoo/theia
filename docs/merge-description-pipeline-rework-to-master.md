# Merge Description: `pipeline-rework` to `master`

## Conventional Commit Title

`feat(pipeline): ship pipeline rework, topology observation flow, and bootstrap discovery controls`

## Summary

This branch reworks the static-discovery and topology-update pipeline from `master` commit `69e74a1` (`chore(frontend): bump version to 1.5.6`) onward.

Committed branch range before the latest working-tree fixes:

- Base branch head: `69e74a1`
- Pipeline-rework head: `177c755`
- Commits included in that committed range: `18`

Latest additions documented and committed on top of `177c755`:

- prevent regular static polls from reopening `bootstrap_once`
- trigger an immediate topology reprobe when discovery mode changes require it
- reconcile peer bootstrap metadata after reverse link enrichment completes a link
- clarify backend/frontend operator messaging for off-map neighbors and queued follow-ups

In total, `pipeline-rework` moves topology handling from direct, always-on LLDP/CDP linking toward a controlled observation-driven workflow with explicit discovery modes, bounded bootstrap windows, incremental cache refreshes, and production validation tooling.

## What This Branch Adds

- Persistent topology observation storage with unresolved-neighbor tracking and canonical link materialization.
- Discovery mode controls across backend and frontend (`off`, `lldp`, `lldp_cdp`, `bootstrap_once`).
- Bootstrap-once state tracking and delayed follow-up scheduling for incomplete LLDP port resolution.
- Incremental cache propagation and event-driven websocket broadcasting for topology updates.
- Observability metrics for discovery throughput, unknown neighbors, link upsert outcomes, and topology materialization latency.
- Polling-budget controls for worker classes, including bounded static reprobe handling.
- PostgreSQL validation helpers and runbooks for production deployment.
- Scale-lab tooling for replay, burst, slowdown, and soak testing scenarios.

## Main Changes

### Topology Discovery Pipeline

- Persist device-level discovery mode and bootstrap state in the data model and database migrations.
- Gate LLDP/CDP discovery by the resolved topology mode instead of walking neighbors unconditionally.
- Introduce `bootstrap_once` as a bounded discovery window that can schedule one delayed follow-up to fill missing interfaces.
- Materialize topology links from persisted observations/unresolved-neighbor records instead of relying on one-pass direct link creation.
- Retry delayed LLDP follow-ups when the static worker budget is temporarily exhausted.
- Close stale bootstrap windows and reopen eligible peers only when a missing-port relationship still needs a one-shot probe.
- Prevent the regular worker static poll path from reopening bootstrap windows after the one-shot workflow is complete.
- Reconcile peer bootstrap metadata immediately when a later discovery enriches the reverse side of an incomplete link.

### Link Canonicalization And Topology Correctness

- Reorient reverse-direction discoveries to the correct canonical source/target ordering.
- Deduplicate LLDP duplicate/self-neighbor variants and keep self-links visible as node annotations instead of noisy parallel edges.
- Refresh incomplete LLDP links in place as new observations arrive so source/target ports converge without duplicate links.
- Improve link edge and link details rendering to communicate pending interface resolution more clearly.

### Cache, Workers, And Broadcast Flow

- Add change-event primitives and incremental cache application for device/link updates.
- Make overview websocket broadcasts event-driven so topology updates propagate with less redundant work.
- Extend scheduler/worker behavior with polling budgets and bounded static reprobe handling.
- Preserve live topology updates while reducing unnecessary full refresh churn.

### Observability, Validation, And Scale Testing

- Add observability registry metrics for discovery neighbors, unknown neighbors, link-upsert results, cache invalidations, and topology materialization timings.
- Add PostgreSQL validation tooling and deployment runbooks.
- Add scale-lab built-in profiles, replay fixtures, and soak scenarios for topology pipeline stress testing.
- Finalize readiness-review artifacts for the pipeline rework rollout.

### Frontend And Operator Experience

- Add topology discovery controls to add-device, device-config, and settings UI flows.
- Surface effective mode, bootstrap state, last discovery time, last result, and queued follow-up expectations in the device config panel.
- Update operator-facing copy so queued follow-ups and off-map neighbors are described as expected bounded behavior rather than generic failure language.

## Latest Additions Since `177c755`

### Discovery Convergence

- `internal/worker/pipeline.go` now treats `bootstrap_once` as `off` for routine static polls so only explicit one-shot triggers can reopen discovery.
- `internal/service/device_service.go` now launches a reprobe when a topology discovery mode change makes discovery newly active for an eligible device.
- `internal/service/static_persistence.go` now reconciles peer bootstrap state when reverse enrichment finishes a previously incomplete LLDP link.

### UX And Log Clarity

- Backend aggregated neighbor logs now say `observed off-map neighbors` instead of `skipped unresolved neighbors`.
- Frontend bootstrap status/result text now uses calmer wording:
  - `Follow-up queued`
  - `Waiting for port details`
  - `Automatic follow-up runs about 20s after last discovery.`
- Add/config panel helper text now explicitly states that bootstrap once may queue one follow-up before returning to `Off`.

## Included Commits

- `8a464d3` `fix(virtual-nodes): correct virtual node status handling`
- `26a7e97` `fix: reorient reverse device neighbor links`
- `78c7c62` `fix(topology): handle self-links and LLDP duplicates`
- `9ddce66` `fix(topology): refresh incomplete LLDP links live`
- `514d584` `fix(topology): show self-links as node annotations`
- `5c5931c` `feat(observability): add phase 0 metrics and phase 1 LLDP dedupe`
- `536a3f7` `feat(topology): add observation materialization and class worker budgets`
- `921ff1b` `feat(cache): apply topology updates incrementally`
- `65e3e78` `feat(ws): make overview broadcast event-driven`
- `25e8022` `feat(postgres): standardize production validation path`
- `eb69ce5` `feat(scalelab): add replay and soak harness`
- `c6cbd20` `feat(audit): finalize readiness review artifacts`
- `1f29619` `feat(topology): persist discovery mode and bootstrap state`
- `2dfd5ee` `feat(topology): gate LLDP/CDP discovery by mode`
- `0327a74` `feat(topology): add bootstrap-once discovery flow`
- `db119b7` `feat(topology): add discovery mode controls to frontend`
- `1fda30e` `fix(topology): retry delayed LLDP followups under budget pressure`
- `177c755` `fix(topology): close stale bootstrap windows and reopen eligible peers`

## Validation Areas

Focus merge verification on:

- adding devices sequentially under `bootstrap_once` and ensuring links converge without discovery loops
- reverse-direction enrichment of incomplete LLDP links and immediate bootstrap-state reconciliation on both peers
- topology-mode changes from UI/API and the resulting one-shot reprobe behavior
- websocket overview freshness during topology updates without redundant full refreshes
- observability metrics and aggregated logs for off-map neighbors
- PostgreSQL validation flow and scale-lab replay/soak commands
