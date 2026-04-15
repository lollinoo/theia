---
phase: 42-pipeline-orchestrator-cutover
plan: 04
subsystem: backend
tags: [go, snmp, websocket, health, pipeline]
requires:
  - phase: 42-pipeline-orchestrator-cutover
    provides: "PipelineOrchestrator runtime, snapshot getters, and fixed-tick broadcast loop from plan 03"
provides:
  - "Main entrypoint cutover from Poller plus MetricsCollector to PipelineOrchestrator"
  - "Collector SNMP client factory helper with settings-driven timeout and retry parsing"
  - "Health and router plumbing based on a Status() string provider instead of *worker.Poller"
affects: [pipeline-orchestrator, app-entrypoint, health-endpoint, websocket-bootstrap]
tech-stack:
  added: []
  patterns: [single runtime owner in main.go, settings-driven SNMP client factory, status-provider API seam]
key-files:
  created: []
  modified: [cmd/theia/main.go, cmd/theia/main_test.go, internal/api/router.go, internal/api/health_handler.go, internal/api/health_handler_test.go]
key-decisions:
  - "cmd/theia/main.go now boots only PipelineOrchestrator for live polling, snapshot sourcing, and Prometheus availability signaling."
  - "The collector SNMP client helper stays unconnected and settings-driven so collector-owned Connect/Close remains the only lifecycle path."
  - "Health and router wiring depend on a minimal Status() string interface so PipelineOrchestrator can report running and stopped without concrete Poller coupling."
patterns-established:
  - "Runtime cutovers in main.go should switch getter and status seams together: WebSocket bootstrap uses runtime getters and health uses a Status() provider."
  - "Collector construction in main.go shares one SNMP client factory so timeout and retry parsing stays centralized."
requirements-completed: [PIPE-03]
duration: 4m
completed: 2026-04-13
---

# Phase 42 Plan 04: Entrypoint Cutover Summary

**Main runtime boot now uses only PipelineOrchestrator, with WebSocket bootstrap and health status wired through pipeline-compatible provider seams**

## Performance

- **Duration:** 4m
- **Started:** 2026-04-13T08:28:28Z
- **Completed:** 2026-04-13T08:32:38Z
- **Tasks:** 2
- **Files modified:** 5

## Accomplishments

- Replaced `worker.NewPoller(...)` plus `worker.NewMetricsCollector(...)` startup in `cmd/theia/main.go` with `state.Store`, `scheduler.Scheduler`, per-tier collectors, and `worker.NewPipelineOrchestrator(...)`.
- Added `newCollectorSNMPClientFunc(...)` plus regression coverage so collector wiring still parses SNMP timeout and retry settings with `5s` and `1` fallbacks.
- Generalized `internal/api` health and router status plumbing to a `Status() string` provider and passed the pipeline runtime through the router without changing the health JSON contract.

## Task Commits

Each task was committed atomically:

1. **Task 1: Replace startup wiring with PipelineOrchestrator and shared collector dependencies** - `be78c41` (test), `0c1cc6d` (feat)
2. **Task 2: Generalize health/router status plumbing from concrete Poller to status provider** - `5bb37f2` (test), `2714d57` (fix)

## Files Modified

- `cmd/theia/main.go` - boots the pipeline runtime, shares collector dependencies, and routes WebSocket plus health status through pipeline getters.
- `cmd/theia/main_test.go` - covers the collector SNMP client factory helper for fallback and parsed settings behavior.
- `internal/api/router.go` - accepts a generic status provider for health wiring.
- `internal/api/health_handler.go` - reads `Status() string` from an interface while preserving `snmp_poller` in the response.
- `internal/api/health_handler_test.go` - verifies running, stopped, and nil-provider behavior through a fake status provider.

## Decisions Made

- Kept the topology notify channel shared between `DeviceService` and `PipelineOrchestrator`; only the live runtime owner changed.
- Used a package-level SNMP client constructor seam in `cmd/theia/main.go` so the helper can be regression-tested without opening real SNMP sessions.
- Preserved the `snmp_poller` health component key even after removing concrete `*worker.Poller` coupling, avoiding API churn during the cutover.

## Deviations from Plan

None - plan goals and acceptance criteria landed as requested.

## Issues Encountered

None after the entrypoint and API seams were updated.

## User Setup Required

None - no external setup or migration commands required.

## Next Phase Readiness

- The backend cutover is complete: `main.go` now wires the live runtime, WebSocket bootstrap, and health reporting to `PipelineOrchestrator`.
- Phase 43 can build detail subscriptions on top of the single runtime owner without legacy Poller or MetricsCollector startup remaining in `main.go`.

## Self-Check: PASSED

- Summary file exists at `.planning/phases/42-pipeline-orchestrator-cutover/42-04-SUMMARY.md`.
- Verified task commits `be78c41`, `0c1cc6d`, `5bb37f2`, and `2714d57` exist in `git log --oneline --all`.
- Verified `go test ./cmd/theia -count=1` and `go test ./internal/api -count=1` pass.
