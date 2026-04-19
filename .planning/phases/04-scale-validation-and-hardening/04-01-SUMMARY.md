---
phase: 04-scale-validation-and-hardening
plan: 01
subsystem: testing
tags: [scalelab, postgres, metrics, docker, validation]
requires:
  - phase: 03-frontend-incremental-reconciliation
    provides: stable runtime-only reconciliation, coalesced structural refresh scheduling, and canvas instrumentation
provides:
  - repeatable 300-device scale-lab profile coverage
  - hybrid replay fixture coverage for realistic topology validation
  - committed synthetic and hybrid evidence artifacts under the phase directory
  - operator runbook for PostgreSQL stack, metrics capture, and browser proof
affects: [phase-04-02, scalelab, metrics, validation, docker]
tech-stack:
  added: []
  patterns:
    - checked-in replay fixtures under internal/scalelab/testdata
    - phase evidence captured under .planning/phases/04-scale-validation-and-hardening/evidence/{mode}
key-files:
  created:
    - internal/scalelab/testdata/wisp-hybrid.json
    - scripts/phase4-validate.sh
    - .planning/phases/04-scale-validation-and-hardening/04-VALIDATION-RUNBOOK.md
    - .planning/phases/04-scale-validation-and-hardening/evidence/synthetic/scale-300-baseline.json
    - .planning/phases/04-scale-validation-and-hardening/evidence/wisp/scale-wisp-hybrid.json
  modified:
    - cmd/theia-scale-lab/main.go
    - internal/scalelab/builtin.go
    - internal/scalelab/scalelab_test.go
    - Makefile
key-decisions:
  - "Made the 300-device target a first-class scale-lab profile instead of encoding it only in shell automation."
  - "Used a checked-in wisp-hybrid replay fixture plus committed metrics captures as the realistic supplement instead of introducing a new E2E or browser automation layer."
patterns-established:
  - "Phase validation scripts should write durable evidence artifacts with stable filenames before downstream triage work starts."
  - "Browser proof remains manual and must cite both window.__THEIA_CANVAS_METRICS__ and the matching saved metrics.prom path."
requirements-completed: [SCAL-01, SCAL-02, SCAL-03]
duration: 14 min
completed: 2026-04-19
---

# Phase 4 Plan 01: Scale Validation Workflow Summary

**Repeatable Phase 4 validation with a first-class 300-device profile, committed synthetic and hybrid evidence artifacts, live `/metrics` captures, and a manual browser-proof runbook**

## Performance

- **Duration:** 14 min
- **Started:** 2026-04-19T11:35:00Z
- **Completed:** 2026-04-19T11:49:02Z
- **Tasks:** 2
- **Files modified:** 14

## Accomplishments

- Added a first-class `300` scale-lab profile with the milestone’s exact interval and burst defaults, and locked it in `internal/scalelab` tests.
- Added `internal/scalelab/testdata/wisp-hybrid.json` and verified it replays a mixed hub, ring, edge, and unresolved-neighbor slice with `16` resolved and `2` unresolved observations.
- Added `make phase4-scale-lab`, `make phase4-validate`, `scripts/phase4-validate.sh`, and `04-VALIDATION-RUNBOOK.md`, then captured the first live PostgreSQL evidence pass into committed `synthetic/` and `wisp/` phase artifacts.

## Task Commits

Each task was committed atomically:

1. **Task 1: Add the target-scale `300` profile and a checked-in realistic replay fixture to scale-lab** - `db577a4` (feat)
2. **Task 2: Add a Phase 4 validation driver, Make targets, and runbook that capture the first evidence pass** - `e189a1c` (feat)

**Plan metadata:** committed separately after self-check.

## Verification

- `go test ./internal/scalelab -count=1` passed.
- `bash -n scripts/phase4-validate.sh` passed.
- `go run ./cmd/theia-scale-lab -profile 300 -scenario baseline >/tmp/theia-phase4-scale.json` passed.
- `make phase4-scale-lab` passed and wrote `.planning/phases/04-scale-validation-and-hardening/evidence/synthetic/scale-300-baseline.json` plus `scale-300-burst-adds.json`.
- `make dev-postgres` and `make seed` passed in the local Docker stack.
- `bash scripts/phase4-validate.sh synthetic http://localhost:8080 .planning/phases/04-scale-validation-and-hardening/evidence/synthetic` passed and captured `metrics.prom` with `theia_refresh_snapshot_build_seconds`, `theia_refresh_topology_reload_total{reason="startup"} 1`, `theia_refresh_topology_reload_total{reason="topology_dirty"} 1`, and `theia_state_changes_dropped_total 0`.
- `bash scripts/phase4-validate.sh wisp http://localhost:8080 .planning/phases/04-scale-validation-and-hardening/evidence/wisp` passed and captured `scale-wisp-hybrid.json` plus a second live `metrics.prom`.
- Browser proof via `window.__THEIA_CANVAS_METRICS__` was not executed in this terminal-only environment; the runbook documents the required manual console inspection.

## Evidence Captured

- `.planning/phases/04-scale-validation-and-hardening/evidence/synthetic/scale-300-baseline.json` records `598` resolved observations, `299` created link events, and `897` noop link events across two replay passes.
- `.planning/phases/04-scale-validation-and-hardening/evidence/synthetic/scale-300-burst-adds.json` records the `burst-adds` scenario with `15` added devices and `628` resolved observations.
- `.planning/phases/04-scale-validation-and-hardening/evidence/wisp/scale-wisp-hybrid.json` records the realistic supplement with `18` observations, `16` resolved observations, and `2` unresolved neighbors.
- `.planning/phases/04-scale-validation-and-hardening/evidence/*/metrics.prom` preserves the live refresh metric families required by the plan for downstream triage.

## Files Created/Modified

- `cmd/theia-scale-lab/main.go` - updates the CLI help text so `300` is a documented built-in profile.
- `internal/scalelab/builtin.go` - adds the first-class `300` profile with the exact Phase 4 workload defaults.
- `internal/scalelab/scalelab_test.go` - extends profile coverage and validates that the hybrid fixture produces both resolved and unresolved replay counts.
- `internal/scalelab/testdata/wisp-hybrid.json` - adds the checked-in realistic replay slice.
- `Makefile` - adds `phase4-scale-lab` and `phase4-validate` entrypoints with default Phase 4 output paths.
- `scripts/phase4-validate.sh` - drives synthetic and hybrid scale-lab runs, writes evidence READMEs, and scrapes live `/metrics`.
- `.planning/phases/04-scale-validation-and-hardening/04-VALIDATION-RUNBOOK.md` - documents the PostgreSQL stack, seeding, optional WISP path, and manual browser proof steps.
- `.planning/phases/04-scale-validation-and-hardening/evidence/` - stores the first committed synthetic and hybrid evidence pass for Plan `04-02`.

## Decisions Made

- Kept the validation workflow on the existing harness and observability surfaces: built-in scale-lab profiles, Docker/PostgreSQL, `/metrics`, and `window.__THEIA_CANVAS_METRICS__`.
- Captured both synthetic and hybrid evidence under stable phase-directory filenames so Plan `04-02` can triage concrete artifacts instead of rerunning discovery.
- Left browser proof manual in the runbook because the phase explicitly reuses existing instrumentation instead of adding a new browser automation dependency.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 1 - Bug] Corrected mode-specific evidence manifests**
- **Found during:** Task 2 (Add a Phase 4 validation driver, Make targets, and runbook that capture the first evidence pass)
- **Issue:** The initial `README.md` output listed artifact filenames that were not actually produced in a given mode, which made the committed evidence manifest inaccurate.
- **Fix:** Updated `scripts/phase4-validate.sh` to emit mode-specific file lists and corrected the generated `synthetic/README.md` and `wisp/README.md` artifacts to match the captured evidence.
- **Files modified:** `scripts/phase4-validate.sh`, `.planning/phases/04-scale-validation-and-hardening/evidence/synthetic/README.md`, `.planning/phases/04-scale-validation-and-hardening/evidence/wisp/README.md`
- **Verification:** `bash -n scripts/phase4-validate.sh` passed and both evidence READMEs were rechecked for exact file lists.
- **Committed in:** `e189a1c`

---

**Total deviations:** 1 auto-fixed (1 bug)
**Impact on plan:** The fix kept the generated evidence chain accurate without changing plan scope.

## Issues Encountered

- `.planning/` is ignored by the repository, so the runbook, evidence artifacts, and summary deliverables require explicit `git add -f` when they must be committed as plan outputs.

## User Setup Required

None - no external service configuration required. The remaining browser proof is a manual validation step described in the runbook, not a setup dependency.

## Next Phase Readiness

- Plan `04-02` can start from the committed evidence in `.planning/phases/04-scale-validation-and-hardening/evidence/synthetic/` and `.planning/phases/04-scale-validation-and-hardening/evidence/wisp/`.
- The remaining live gap is the manual browser inspection of `window.__THEIA_CANVAS_METRICS__` against the saved `metrics.prom` files; that should be recorded during Plan `04-02` or final phase verification.

## Self-Check: PASSED

- Verified `.planning/phases/04-scale-validation-and-hardening/04-01-SUMMARY.md` exists.
- Verified task commits `db577a4` and `e189a1c` resolve in git history.
- Verified committed evidence files exist for the synthetic baseline, synthetic burst-adds, hybrid replay, and both `metrics.prom` captures.

---
*Phase: 04-scale-validation-and-hardening*
*Completed: 2026-04-19*
