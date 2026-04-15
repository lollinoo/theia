---
gsd_state_version: 1.0
milestone: v1.5.3
milestone_name: SNMP Pipeline Architecture
status: completed
stopped_at: Archived v1.5.3 milestone
last_updated: "2026-04-15T07:53:54Z"
last_activity: 2026-04-15
progress:
  total_phases: 12
  completed_phases: 12
  total_plans: 34
  completed_plans: 34
  percent: 100
---

# Project State

## Project Reference

See: .planning/PROJECT.md (updated 2026-04-15)

**Core value:** Network operators can see their entire topology at a glance with live stats on every device and link, drill into Grafana for deep dives, and manage devices directly -- all from a single interactive map.
**Current focus:** Planning next milestone

## Current Position

Phase: None
Plan: None
Status: v1.5.3 shipped and archived — start the next milestone when ready
Last activity: 2026-04-15

Progress: [████████████████████████] 12/12 phases complete

## Performance Metrics

**Velocity:**

- Total plans completed: 34 (v1.5.3)
- Average duration: --
- Total execution time: --

**By Phase:**

| Phase | Plans | Total | Avg/Plan |
|-------|-------|-------|----------|
| 38 | 2 | - | - |
| 40 | 4 | - | - |
| 41 | 3 | - | - |
| 42 | 4 | - | - |
| 46 | 1 | - | - |
| 48 | 2 | - | - |
| 49 | 4 | - | - |

**Recent Trend:**

- Last 5 plans: --
- Trend: --

*Updated after each plan completion*
| Phase 38-state-engine P01 | 3m42s | 2 tasks | 3 files |
| Phase 38-state-engine P02 | 3m 25s | 2 tasks | 2 files |
| Phase 42 P01 | 6 min | 2 tasks | 4 files |
| Phase 42 P02 | 8m53s | 2 tasks | 3 files |
| Phase 42-pipeline-orchestrator-cutover P04 | 4m | 2 tasks | 5 files |
| Phase 43 P01 | 1m | 2 tasks | 4 files |
| Phase 43 P02 | 3m | 2 tasks | 4 files |
| Phase 43 P03 | 1m | 2 tasks | 8 files |
| Phase 44 P01 | 3m | 2 tasks | 2 files |
| Phase 44 P02 | 3m28s | 2 tasks | 7 files |
| Phase 44 P03 | 13m | 2 tasks | 8 files |
| Phase 44 P04 | 2m | 2 tasks | 2 files |
| Phase 45 P01 | 10m13s | 2 tasks | 6 files |
| Phase 45 P02 | 7m5s | 2 tasks | 3 files |
| Phase 46 P01 | 6m50s | 3 tasks | 6 files |
| Phase 47 P01 | 6m | 2 tasks | 8 files |
| Phase 48 P01 | 17m | 3 tasks | 5 files |
| Phase 48 P02 | 6m | 2 tasks | 5 files |

## Accumulated Context

### Decisions

- v1.5.3 keeps existing FNV-64a delta WS approach (not replaced with per-device patches)
- State engine uses sync.RWMutex (not channel-based actor) -- two of three research files agree, atomic snapshot reads are a natural fit for RLock
- Hysteresis thresholds use ARCHITECTURE.md values (CPU warn 70%/clear 60%, critical 90%/clear 80%) -- wider gap prevents flapping better
- Device classification: auto by device_type, user-overridable per device
- Thresholds: hardcoded sensible defaults -- ship fast, configurable later (THRESH-01/02 deferred)
- No new third-party dependencies needed -- golang.org/x/sync promoted from indirect to direct
- [Phase 38-state-engine]: internal/state package seeded with three-dimensional DeviceState (health + reachability + stale) and hardcoded 70/60/90/80 hysteresis thresholds; health logic is lock-agnostic pure functions so Plan 02 only has to add Store methods
- [Phase 38-state-engine]: NaN guard added to evaluateMetricSeverity (Rule 2 correctness): NaN values return current severity unchanged, preventing Prometheus NaN samples from silently clearing warnings via >= comparison false-returns
- [Phase 38-state-engine]: Plan 02: Store API complete — Update/Snapshot/GetDevice/Changes/Remove/Start/Stop implemented with lock-and-emit-outside-lock invariant; staleness tick hardcoded at 5s; field-by-field diff (no reflect) using time.Time.Equal for monotonic-clock safety
- [Phase 38-state-engine]: Plan 02: Update() deep-copies incoming Metrics via cloneMetrics before evaluateHealth — extends Pitfall-6 tamper-protection from Snapshot outbound to Update inbound; callers retaining a reference to passed *DeviceMetrics cannot corrupt stored state
- [Phase 38-state-engine]: Plan 02: Start() panics on second call (surface wiring mistakes early); Stop() is idempotent-safe (allows defer s.Stop() even if Start was never called); Health enum normalized to Unknown when first Update is a failure to prevent empty-string enum leaking to Changes
- [Phase 38-state-engine]: Code-review-fix pass (5 warnings): Stop() re-creates s.done (restart-after-stop works); lifecycleMu protects Start/Stop; aggregateHealth returns Unknown for all-empty severities (not Healthy); PollSuccess=true with nil Metrics resets Health to Unknown (no stale-frozen value); Snapshot().CollectedAt always reflects latest poll even when metric values are byte-identical
- [Phase 42]: Operational updates now own reachability, consecutive-failure tracking, and uptime while preserving last-known performance and link data.
- [Phase 42]: No additional store fields were needed beyond VolatilityClass and LinkMetrics; compatibility for unstamped callers is handled by a temporary legacy update path.
- [Phase 42]: Performance updates merge CPU, memory, temperature, and non-empty link metrics without clearing last-known values on failed polls.
- [Phase 42]: Shared static discovery persistence now lives in DeviceService.ApplyStaticDiscovery so legacy probing and the future orchestrator share one topology-write seam.
- [Phase 42]: ApplyStaticDiscovery reports TopologyChanged but never writes TopologyNotify; probeDevice remains the caller-owned notification point.
- [Phase 42-pipeline-orchestrator-cutover]: Main entrypoint now boots only PipelineOrchestrator for live polling, snapshot sourcing, and Prometheus availability signaling.
- [Phase 42-pipeline-orchestrator-cutover]: Health and router wiring now depend on a Status() string provider so PipelineOrchestrator can report running/stopped without concrete Poller coupling.
- [Phase 43]: Detail control traffic reuses snapshot_delta and shared snapshot payload — No second message family or socket path was introduced.
- [Phase 43]: Each WebSocket client stores exactly one active detailDeviceID under hub mutex — subscribe_detail replaces previous selection immediately.
- [Phase 43]: Malformed or unsupported inbound control frames are logged and ignored — Bad client payloads do not tear down the socket.
- [Phase 44]: Overview card metadata stays in the existing device_metrics map; no new websocket message type or parallel payload was introduced.
- [Phase 44]: Cadence precedence is runtime ExpectedInterval, then PollIntervalOverride, then PollClass.Interval() so cards remain deterministic before first poll.
- [Phase 44]: PUT /api/v1/devices now rejects poll_interval_override values outside 5..3600 before persistence, satisfying the poll override threat model at the handler boundary.
- [Phase 44]: Tri-state override semantics now use optionalPollIntervalOverride in the handler and **int in service.DeviceUpdate so omitted, null, and numeric values survive the REST seam unchanged.
- [Phase 44]: Frontend polling contract changes stay on the existing Device parser and updateDevice() API, avoiding a second polling-specific client surface before the UI plans land.
- [Phase 44]: DeviceCard primary status dot, glow, and explicit label now map directly from metrics.health on both card branches.
- [Phase 44]: Freshness and cadence copy live in a pure helper reused by both physical and virtual card rendering paths.
- [Phase 44]: Local stale fallback blanks only numeric metric fields and preserves backend-owned health and cadence metadata on node.data.metrics.
- [Phase 44]: Polling override state now derives from device poll fields while Grafana URL remains settings-backed.
- [Phase 44]: Polling saves stay inline with debounce, call updateDevice, and validate custom seconds before the API call.
- [Phase 45]: Targeted performance re-due requests are funneled through a buffered scheduler command channel so heap and ready state remain owned by the scheduler goroutine.
- [Phase 45]: DeviceService compares previous and persisted override values after repository update; omitted, unrelated, and same-value edits do not schedule extra work.
- [Phase 45]: Production bootstrap uses a dedicated wirePollRescheduler helper so tests can prove the live scheduler is attached before the API path can save overrides.
- [Phase 45]: Runtime proof stays on the existing snapshot / snapshot_delta websocket contract; no new message family or payload widening was introduced.
- [Phase 45]: Performance and legacy compatibility updates are the only paths allowed to stamp LastPolledAt, ExpectedInterval, and Stale=false.
- [Phase 45]: Task 2 required regression-coverage changes only because Task 1's store fix already corrected the runtime metadata flow end to end.
- [Phase 46]: Targeted detail stays on snapshot_delta and now populates only link_metrics[device.id] alongside the existing device_metrics and device_statuses sections.
- [Phase 46]: publishSubscribedDetailDelta() remained the only runtime delivery seam; the closure work was payload composition plus tighter regressions, not a new send path.
- [Phase 46]: Frontend correctness stayed on the existing mergeSnapshotDelta shared atom path; no second websocket, cache, or panel-local state was introduced.
- [Phase 48]: Phase 48 Plan 01 records only user-confirmed live pass observations, including concrete device identifiers and cadence values, in the final HUMAN-UAT artifacts.
- [Phase 48]: Phase 48 Plan 01 preserves the established HUMAN-UAT frontmatter shape and records approval through Current Test text, result lines, summary counts, and Gaps.
- [Phase 48]: Phase 40 closes with one passed live SNMP proof and one skipped Prometheus probe check, where probe_success absence is recorded as an accepted environment limitation rather than unresolved debt.
- [Phase 48]: The milestone audit now removes stale live-runtime debt only where finalized HUMAN-UAT artifacts prove closure, while leaving planning traceability and Nyquist gaps explicit.
- [Phase 49]: Phases 39 through 46 now all have `*-VALIDATION.md` artifacts with `nyquist_compliant: true`, and automated-only phases 39, 41, and 43 explicitly state that no manual-only verification debt remains.
- [Phase 49]: The v1.5.3 milestone audit now reports Nyquist coverage `overall: 9/9` with no missing phases; remaining milestone debt is planning traceability cleanup only.

### Pending Todos

None yet.

### Blockers/Concerns

None. Phase 47 closed the remaining planning traceability drift, and the milestone is ready for archival.

## Session Continuity

Last session: 2026-04-14T20:59:03.939Z
Stopped at: Completed 48-02-PLAN.md
Resume file: None
