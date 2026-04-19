# Phase 4 Validation Evidence

- Mode: synthetic
- API base: http://localhost:8080
- Output directory: .planning/phases/04-scale-validation-and-hardening/evidence/synthetic-final

## Evidence Files

- `scale-300-baseline.json`
- `scale-300-burst-adds.json`
- `metrics.prom`

## Required Evidence Surfaces

- `theia_refresh_snapshot_build_seconds`
- `theia_refresh_topology_reload_total`
- `theia_state_changes_dropped_total`
- `window.__THEIA_CANVAS_METRICS__`
