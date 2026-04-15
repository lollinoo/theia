# Phase 40: Collectors - Context

**Gathered:** 2026-04-12
**Status:** Ready for planning

<domain>
## Phase Boundary

Introduce stateless per-volatility-class collectors that reuse the existing SNMP and Prometheus clients, return typed results, and make data collection independent from scheduler timing and from the Phase 38 state engine. This phase defines the collection contract for `performance`, `operational`, and `static` polls. It does **not** cut over `main.go`, replace `MetricsCollector`, add scheduler behavior, or migrate the static discovery model into vendor YAML.

</domain>

<decisions>
## Implementation Decisions

### Prometheus contract
- **D-01:** SNMP is authoritative for core live metrics in this milestone. Prometheus is enrichment-only.
- **D-02:** Prometheus may add non-authoritative enrichment such as alerts, hostnames, probe status, and supplemental metrics SNMP does not provide, but it must not override SNMP-derived CPU, memory, temperature, uptime, or counter data when both exist.
- **D-03:** Link throughput is SNMP-only in Phase 40. No Prometheus link-rate fallback is used in this phase.

### Static collector scope
- **D-04:** `StaticCollector` should wrap the existing discovery path rather than redesign it. The implementation should reuse the current discovery flow (`snmp.DiscoverDevice()` and related discovery helpers) and return a typed static result for downstream consumers.
- **D-05:** `vendor.StaticOIDs` remains mostly empty in this phase. Migrating ifTable/ifXTable/LLDP/CDP and related static discovery definitions into vendor YAML is explicitly deferred.

### Counter-rate correctness
- **D-06:** Counter-rate samples are discarded when a reset is detected (`new < old`), when the computed rate exceeds 110% of interface speed, or when the sample follows a bad/missed interval that invalidates the comparison.
- **D-07:** When a counter-rate sample is discarded, throughput for that interval is reported as unknown/absent rather than clamped or back-filled with a stale value.
- **D-08:** Any bad or missed interval resets the baseline. The next usable poll is treated as a fresh first sample, so no rate is computed until a second clean sample exists.

### Partial-result policy
- **D-09:** Collectors return best-effort partial results. Missing individual OIDs stay nil/absent and do not fail the whole poll.
- **D-10:** Collector failure is reserved for transport/connect/query execution failures. Missing individual OIDs or vendor-unsupported fields are non-fatal.
- **D-11:** `PerformanceCollector` should preserve the current `snmp.PollDeviceMetrics()` semantics of per-field nils rather than inventing all-or-nothing validation in this phase.

### the agent's Discretion
- Exact package/file layout inside `internal/collector/`.
- Exact typed result names and the method set used to satisfy the common update interface.
- Whether Prometheus enrichment is exposed as a dedicated `PrometheusCollector` or as helper logic used by performance/operational collectors, as long as SNMP remains authoritative per D-01 through D-03.
- Exact ownership boundary for applying counter-rate computation between the collector layer and the downstream state/pipeline layer, as long as collectors stay stateless and the D-06 through D-08 policy is enforced before rates become user-visible.

</decisions>

<canonical_refs>
## Canonical References

**Downstream agents MUST read these before planning or implementing.**

### Phase scope and milestone constraints
- `.planning/ROADMAP.md` §Phase 40 — phase goal, success criteria, and dependency chain into Phases 41-42.
- `.planning/REQUIREMENTS.md` — `PIPE-01`, `PIPE-02`, and `PIPE-04` define the acceptance criteria this phase must satisfy.
- `.planning/PROJECT.md` §Current Milestone and §Key Decisions — SNMP-primary milestone intent, FNV WS delta retention, and hardcoded-defaults bias.
- `.planning/STATE.md` §Accumulated Context — current milestone decisions already locked across Phases 38-39.

### Prior phase decisions that constrain this phase
- `.planning/phases/38-state-engine/38-CONTEXT.md` — state engine contract, `StateUpdate` direction of travel, and runtime/config separation.
- `.planning/phases/39-domain-types-db-migration/39-CONTEXT.md` — `PollClass`, `VolatilityClass`, tiered OID layout, and the explicit deferral of static YAML migration.

### Research guidance
- `.planning/research/ARCHITECTURE.md` §3 Collectors — recommended collector split, result shapes, and reuse of existing `snmp`/`metrics` code.
- `.planning/research/ARCHITECTURE.md` §Pattern 1 and §Pattern 3 — common update interface and counter-rate statefulness guidance.
- `.planning/research/SUMMARY.md` §Phase 3 Collectors — recommended deliverables for this phase.
- `.planning/research/SUMMARY.md` §Pitfalls / counter reset notes — reset, gap, and sanity-bound policy rationale.
- `.planning/research/PITFALLS.md` — current monolithic collector risks and delta/hash coupling that later phases must respect.

### Current code to reuse or split
- `internal/worker/metrics_collector.go` — current monolithic Prometheus/SNMP collection logic, link-counter rate code, and fallback behavior to extract into collector-layer responsibilities.
- `cmd/theia/main.go` — current SNMP metric/link polling factories (`newSNMPMetricsPollFunc`, `newSNMPLinkPollFunc`) to reuse or relocate.
- `internal/snmp/discovery.go` — existing `PollDeviceMetrics`, `PollInterfaceCounters`, and discovery helpers that collectors should wrap rather than duplicate.
- `internal/vendor/schema.go` — volatility-tier SNMP schema definitions introduced in Phase 39.
- `internal/vendor/registry.go` — `ResolvePerformanceOIDs`, `ResolveOperationalOIDs`, and `ResolveStaticOIDs` lookup behavior.
- `internal/vendor/data/default.yaml` — default operational/performance OID values that Phase 40 collectors consume.
- `internal/state/store.go` — current state-store update shape and runtime-state boundary.
- `internal/service/device_service.go` — existing discovery/reprobe path that a static collector may coordinate with or mirror.
- `internal/domain/poll_class.go` — volatility and poll-class enums plus interval constants.
- `internal/domain/device.go` — device source, interface, and poll metadata available to collector inputs.
- `internal/domain/metrics.go` — existing `DeviceMetrics` and `LinkMetrics` shapes the collector outputs should align with.

</canonical_refs>

<code_context>
## Existing Code Insights

### Reusable Assets
- `snmp.PollDeviceMetrics` already returns CPU/memory/temperature/uptime with nil-per-field semantics; this directly supports the chosen best-effort partial-result policy.
- `snmp.PollInterfaceCounters` already walks raw 64-bit interface counters; Phase 40 only needs to wrap it and enforce the reset/gap/sanity rules.
- `vendor.Registry.ResolvePerformanceOIDs` and `ResolveOperationalOIDs` already expose the Phase 39 tiered OID lookup surface the new collectors should consume.
- `MetricsCollector.buildSnapshot()` already contains the Prometheus batching/grouping logic and the current SNMP fallback rules that can be split into collector-friendly pieces.
- `snmp.DiscoverDevice()` and the existing reprobe flow already implement the static discovery behavior that `StaticCollector` should wrap.

### Established Patterns
- Constructor injection from `cmd/theia/main.go` is the normal way to wire runtime dependencies; collector constructors should follow the same pattern.
- Runtime state and DB-backed inventory are intentionally separate: `internal/state` holds volatile state while repositories/services own persisted device/link data.
- Domain enums and typed result structs live in `internal/domain`-adjacent layers; Phase 40 should stay consistent with the typed-string pattern established in prior phases.
- The current backend already tolerates partial metric availability by using nil pointer fields; Phase 40 should preserve that instead of introducing stricter semantics.

### Integration Points
- Phase 40 code will primarily split responsibilities out of `internal/worker/metrics_collector.go` without changing the final cutover point yet.
- Phase 41 scheduler will later dispatch work by `VolatilityClass` and `PollClass`, so Phase 40’s collector interfaces need to be consumable by scheduled tasks.
- Phase 42 pipeline orchestration will consume the typed collector results and feed them into the state engine and existing WS delta delivery path.

</code_context>

<specifics>
## Specific Ideas

- User chose the recommended path across all discussed areas, favoring a narrow collector phase, strong correctness on counter handling, and a clean SNMP-primary contract.
- No bespoke product references or custom UI behavior were requested for this phase; standard backend architecture is acceptable as long as it respects the decisions above.

</specifics>

<deferred>
## Deferred Ideas

- Migrating `ifTable` / `ifXTable` / LLDP / CDP and related static discovery definitions into `vendor.StaticOIDs` remains future work.
- Any broader Prometheus fallback strategy for link throughput or authoritative metric override is deferred; this phase keeps Prometheus in an enrichment-only role.
- Stricter all-or-nothing collector semantics were explicitly not chosen for this phase.

</deferred>

---

*Phase: 40-collectors*
*Context gathered: 2026-04-12*
