# Phase 4 Validation Runbook

This runbook is the operator path for the first Phase 4 evidence pass. It keeps the target 300-device validation on the existing Docker/PostgreSQL stack, uses the checked-in scale-lab harness for repeatable JSON artifacts, and relies on `/metrics` plus `window.__THEIA_CANVAS_METRICS__` for live proof.

## Synthetic PostgreSQL Baseline

1. Start the real dev stack on PostgreSQL.

```bash
make dev-postgres
```

2. Seed the default SNMP simulator topology so the backend, Prometheus, and frontend all have live targets.

```bash
make seed
```

3. Run the repeatable Phase 4 synthetic validation flow.

```bash
make phase4-validate
```

4. Confirm the synthetic evidence files exist under `.planning/phases/04-scale-validation-and-hardening/evidence/synthetic/`:

- `scale-300-baseline.json`
- `scale-300-burst-adds.json`
- `metrics.prom`
- `README.md`

## Optional WISP Hybrid Slice

1. Start the existing realistic topology supplement.

```bash
make wisp-lab
```

2. Seed the router and radio overlay devices into the live backend.

```bash
make wisp-seed
make wisp-radio-seed
```

3. Capture the hybrid replay and a matching `/metrics` scrape in a separate evidence directory.

```bash
make phase4-validate PHASE4_MODE=wisp PHASE4_OUT=.planning/phases/04-scale-validation-and-hardening/evidence/wisp
```

4. Confirm the WISP evidence files exist under `.planning/phases/04-scale-validation-and-hardening/evidence/wisp/`:

- `scale-wisp-hybrid.json`
- `metrics.prom`
- `README.md`

## Browser Proof

1. Open `http://localhost:3000` after the backend and frontend are healthy.
2. Open the browser developer tools console.
3. Inspect the recent canvas measurements:

```js
window.__THEIA_CANVAS_METRICS__
```

4. During runtime-only refresh or reconnect activity, confirm the buffer shows `theia:canvas:snapshot-apply` entries without unexpected `theia:canvas:topology-load` or `theia:canvas:layout` churn unless the topology actually changed.
5. Keep the matching `/metrics` capture path beside the browser notes so the proof can cite both the canvas measurement buffer and the saved `metrics.prom` file.

## Summary Notes

- Copy the live observations, evidence paths, and any unexpected reload or layout behavior into `.planning/phases/04-scale-validation-and-hardening/04-01-SUMMARY.md`.
- If the live stack is unavailable, record the exact missing service or command failure in the summary instead of marking the proof as passed.
