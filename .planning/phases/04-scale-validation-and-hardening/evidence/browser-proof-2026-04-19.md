# Browser Proof: 2026-04-19

## Source

- Captured from `window.__THEIA_CANVAS_METRICS__` during a live browser session on 2026-04-19.
- Persisted from the operator-supplied trace shared after Phase 4 execution.

## Observed Sample

- Event count: `50`
- Event names present: `theia:canvas:snapshot-apply`
- Triggers present: `snapshot`
- Time window: `2026-04-19T12:45:22.339Z` to `2026-04-19T12:48:10.600Z`
- Duration range: `0.0ms` to `0.4ms`
- Average duration: `0.066ms`
- Total sampled duration: `3.3ms`

## Interpretation

- The supplied browser trace contains repeated `theia:canvas:snapshot-apply` events only.
- No `theia:canvas:topology-load` events were present in the supplied sample.
- No `theia:canvas:layout` events were present in the supplied sample.

This is consistent with Phase 3's reconciliation contract: runtime-only updates stayed on the in-place snapshot apply path during the sampled window instead of triggering structural topology reload or layout churn.

## Limitations

- The operator note did not identify whether this browser session should be paired to `evidence/synthetic-final/metrics.prom` or `evidence/wisp-final/metrics.prom`.
- This browser proof closes the manual `window.__THEIA_CANVAS_METRICS__` gap for `SCAL-02`, but it does not change the separate live slow-Prometheus limitation documented in `04-VERIFICATION.md`.
