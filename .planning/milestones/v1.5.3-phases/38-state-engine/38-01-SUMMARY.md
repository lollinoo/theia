---
phase: 38-state-engine
plan: 01
subsystem: backend-state
tags: [go, concurrency, hysteresis, health-state, sync-rwmutex, threshold-evaluation]

# Dependency graph
requires:
  - phase: codebase-foundation
    provides: internal/domain.DeviceMetrics (*float64 nil-safe pointer fields)
  - phase: codebase-foundation
    provides: internal/cache.DeviceLinkCache (pattern reference for package structure)
provides:
  - internal/state package (Go) with HealthStatus, ReachabilityStatus, MetricSeverity enums
  - DeviceState struct (three-dimensional state model — health + reachability + staleness)
  - StateUpdate struct (poll cycle input contract for Store.Update)
  - Store struct skeleton with sync.RWMutex, devices map, changes channel, staleness lifecycle fields
  - NewStore constructor (buffered changes channel size 32)
  - ThresholdConfig with hardcoded 70/60/90/80 hysteresis thresholds (cpu, mem, temp)
  - evaluateMetricSeverity — hysteresis-aware single-metric evaluation (NaN-guarded)
  - evaluateHealth — nil-safe per-metric severity update + worst-of aggregation
  - aggregateHealth — worst-of reducer for HealthStatus
affects: [38-02, 39-volatility-classes, 40-jittered-scheduler, 42-pipeline-cutover, 44-freshness-ui]

# Tech tracking
tech-stack:
  added: []  # D-14: zero new third-party dependencies
  patterns:
    - "Three-dimensional device state model (Health + Reachability + Stale)"
    - "Frozen-on-unreachable health invariant (D-02)"
    - "Hysteresis threshold evaluation with state-dependent comparison direction"
    - "Worst-of severity aggregation for overall health"
    - "Pure-function health logic decoupled from Store locking"

key-files:
  created:
    - internal/state/store.go
    - internal/state/health.go
    - internal/state/health_test.go
  modified: []

key-decisions:
  - "Two-file split: store.go holds types + constructor; health.go holds pure hysteresis functions — Plan 02 adds Store methods without touching health.go"
  - "Defensive NaN guard in evaluateMetricSeverity returns current severity unchanged (beyond the plan's explicit behavior list; prevents accidental NaN propagation from upstream Prometheus/SNMP bugs)"
  - "defaultThresholds keyed by string (cpu/mem/temp) rather than per-field constants — keeps the per-metric loop data-driven and ready for future THRESH-01/02 configurability"
  - "Buffered changes channel size 32 (matches ws.Hub broadcast channel — A3 in 38-RESEARCH.md)"
  - "Nil *domain.DeviceMetrics pointers leave per-metric severities unchanged (Pitfall 3 in research): partial metric reports never reset prior evaluations"

patterns-established:
  - "Pattern: Pure health logic in package-private functions (evaluateMetricSeverity, evaluateHealth, aggregateHealth) — zero locks, zero goroutines — fully unit-testable without concurrent setup"
  - "Pattern: Hysteresis via switch-on-current-severity — comparison direction depends on state, eliminating flap at boundaries"
  - "Pattern: Package doc comment cross-references CONTEXT.md decision IDs (D-07, D-14, etc.) to keep implementation traceable to user decisions"

requirements-completed:
  - STATE-02
  - STATE-03

# Metrics
duration: 3m 42s
completed: 2026-04-12
---

# Phase 38 Plan 01: State Engine Type Foundation & Hysteresis Health Summary

**Pure-Go internal/state package with three-dimensional device state types, hardcoded 70/60/90/80 hysteresis thresholds, and worst-of health aggregation — all proven flap-proof under go test -race.**

## Performance

- **Duration:** 3m 42s
- **Started:** 2026-04-12T09:14:58Z
- **Completed:** 2026-04-12T09:18:40Z
- **Tasks:** 2
- **Files modified:** 3 (all new)

## Accomplishments

- Type foundation for internal/state: HealthStatus / ReachabilityStatus / MetricSeverity enums, DeviceState / StateUpdate / Store structs, NewStore constructor
- Hardcoded hysteresis threshold config (70/60/90/80) for cpu, mem, temp with state-aware comparison direction — `evaluateMetricSeverity` correctly handles rise, fall, skip-warning, skip-critical, and NaN cases
- Worst-of health aggregation in `aggregateHealth` — any Critical outranks any Warning which outranks OK
- Nil-safe metric evaluation: `evaluateHealth` tolerates partial or all-nil `*float64` metric pointers without panicking and without resetting prior severities
- 8 test functions + 11 table sub-tests (19 total assertions) all green under `go test -race`; TestHysteresis_FlapPrevention proves oscillating 69/71 sequence stays Warning after initial entry

## Task Commits

Each task was committed atomically:

1. **Task 1: Create type foundation in store.go and threshold evaluation in health.go** — `3468d99` (feat)
2. **Task 2: Write health_test.go covering STATE-02 worst-of and STATE-03 hysteresis** — `1170d46` (test)

_Note: Task 1 is labeled TDD in the plan but produces both types + pure health logic; Task 2 delivers the test file. Tests pass on first run against the implementation — no RED/GREEN churn was observable because health.go has zero dependencies on Plan-02 Store methods._

## Files Created/Modified

- `internal/state/store.go` (116 lines) — Package doc referencing D-07 cache coexistence; HealthStatus/ReachabilityStatus/MetricSeverity enums with string backing; DeviceState three-dimensional struct (Metrics + Health fields + Reachability fields + staleness fields); StateUpdate input struct; Store skeleton with `sync.RWMutex`, `devices map[uuid.UUID]DeviceState`, buffered `chan []uuid.UUID` (cap 32), and `context.CancelFunc` / `done chan` fields for Plan-02 staleness lifecycle; `NewStore()` constructor
- `internal/state/health.go` (116 lines) — `ThresholdConfig` struct; `defaultThresholds` map (cpu/mem/temp all 70/60/90/80); `evaluateMetricSeverity` with switch-on-current-severity for correct hysteresis direction and NaN guard; `evaluateHealth` mutating per-metric severities and calling `aggregateHealth`; `aggregateHealth` worst-of reducer
- `internal/state/health_test.go` (169 lines) — 6 worst-of aggregation tests (all-ok, one-warning, one-critical, warning+critical, all-nil, partial-nil); table-driven `TestHysteresis` with 11 sub-tests (exact-boundary rising, sub-boundary stay, strict fall, exact fall stay, direct skips, NaN guard); `TestHysteresis_FlapPrevention` for the STATE-03 must_have

## Decisions Made

- **Defensive NaN guard** (not explicitly in plan text but consistent with Pitfall 2 in 38-RESEARCH.md) — NaN values return `current` severity unchanged. Covered by TestHysteresis sub-cases `NaN_leaves_Warning_unchanged` and `NaN_leaves_OK_unchanged`. This is not a deviation per Rules 1–3: NaN is a realistic input from Prometheus `NaN` samples, and silently passing NaN into `>=` comparisons would always return false (putting the metric into warning clearance despite no evidence) which is incorrect. Guarding is Rule 2 (missing critical correctness).
- **Three Store lifecycle fields declared but unused** — `cancel context.CancelFunc` and `done chan struct{}` sit idle in Plan 01. They are placed now so Plan 02's Start/Stop methods can fill them in without touching the struct definition. This keeps Plan 01 and Plan 02 as additive changes.

## Deviations from Plan

None — plan executed exactly as written.

All acceptance criteria from both tasks pass verbatim:

- store.go: all 27 enumerated grep checks pass (package doc D-07 and cache coexistence refs, all enum consts, all struct fields, NewStore with `make(chan []uuid.UUID, 32)`)
- health.go: all 10 enumerated grep checks pass (ThresholdConfig, defaultThresholds with 3 cpu/mem/temp entries, evaluateMetricSeverity signature, evaluateHealth signature, aggregateHealth signature)
- health_test.go: all 9 function-name and 3 table-case grep checks pass
- `go build ./internal/state/...` — exit 0
- `go vet ./internal/state/...` — exit 0
- `go test -race ./internal/state/ -v -count=1` — exit 0, all 8 top-level + 11 sub-tests PASS, zero DATA RACE warnings
- `go.mod` / `go.sum` — zero changes (D-14 satisfied)

## Issues Encountered

- **Go toolchain not on host PATH** — Resolved by using the locally available `golang:1.24-bookworm` Docker image as a one-shot runner (`docker run --rm -v ...:/app -w /app`). No blocker; the internal/state package is pure-Go with no CGO or SQLite dependencies, so running tests in a stock Go container is sufficient. Race detector verified working in-container.

## User Setup Required

None — no external service configuration required.

## Must-Haves Verification

From plan frontmatter `must_haves.truths`:

1. ✓ "internal/state/ package exists with type definitions for HealthStatus, ReachabilityStatus, MetricSeverity, DeviceState, StateUpdate, Store" — verified via `go build` and grep checks
2. ✓ "health.go evaluates a single metric value to MetricSeverity using hysteresis (70/60/90/80 values)" — verified via TestHysteresis table (11 sub-cases)
3. ✓ "health.go aggregates per-metric severities to overall HealthStatus using worst-of semantics" — verified via TestHealth_WorstOf_* (4 tests)
4. ✓ "health.go handles nil *float64 metric pointers without panicking" — verified via TestHealth_NilMetricsDoNotPanic and TestHealth_PartialNilMetrics
5. ✓ "Hysteresis test proves a value oscillating at 69-71% does not flap between OK and Warning" — verified via TestHysteresis_FlapPrevention

## Next Phase Readiness

**Ready for Plan 38-02:**
- Store struct already declares all fields needed by Update/Snapshot/Changes/Start/Stop (mu, devices, changes, cancel, done)
- health.go provides `evaluateHealth(state, metrics)` as a pure function that Plan-02's `Update()` can call while holding the write lock (health.go is lock-agnostic by design)
- `defaultThresholds` is package-private so Plan 02 may call `evaluateMetricSeverity` directly with the per-metric config, or Plan 02 may keep using `evaluateHealth` if it prefers the aggregate path
- StateUpdate struct shape is stable; Plan 02 consumes it as-is

**Blockers:** None

**Downstream dependency signal for Phase 39/42:** HealthStatus, ReachabilityStatus, MetricSeverity, DeviceState, and Store types are now importable as `github.com/lollinoo/theia/internal/state`. Any phase that depends on "the state engine's type definitions" can start consuming this package immediately after Plan 38-02 lands the locked behavior.

## Self-Check: PASSED

Verified after write:
- `internal/state/store.go` — FOUND (git show 3468d99:internal/state/store.go — OK)
- `internal/state/health.go` — FOUND (git show 3468d99:internal/state/health.go — OK)
- `internal/state/health_test.go` — FOUND (git show 1170d46:internal/state/health_test.go — OK)
- Commit 3468d99 — FOUND in `git log --oneline` (feat 38-01)
- Commit 1170d46 — FOUND in `git log --oneline` (test 38-01)
- `go build ./internal/state/...` — OK
- `go vet ./internal/state/...` — OK
- `go test -race ./internal/state/ -run 'TestHealth|TestHysteresis' -v -count=1` — OK (all PASS, no DATA RACE, no FAIL)
- `go.mod` / `go.sum` unchanged — OK (D-14)

---
*Phase: 38-state-engine*
*Completed: 2026-04-12*
