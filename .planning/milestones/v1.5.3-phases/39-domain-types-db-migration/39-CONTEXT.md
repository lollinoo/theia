# Phase 39: Domain Types & DB Migration - Context

**Gathered:** 2026-04-12
**Status:** Ready for planning

<domain>
## Phase Boundary

Add typed foundations (`VolatilityClass`, `PollClass`) to the domain layer, restructure vendor YAML to express OID groups per volatility tier, and add `poll_class` + `poll_interval_override` columns to the `devices` table with backfill. This phase delivers **typed scaffolding only** — no scheduler behavior, no collector wiring, no frontend changes. Those live in Phases 40–44. The acceptance test is that Phase 40 collectors and Phase 41 scheduler can import `domain.PollClass`, `domain.VolatilityClass`, and the per-class interval constants, and that every device row carries a correctly auto-classified `poll_class`.

</domain>

<decisions>
## Implementation Decisions

### Type placement & naming

- **D-01:** `PollClass` and `VolatilityClass` enums live in `internal/domain/` (new types, package-level). Research doc recommendation. Avoids cyclic imports — scheduler, collector, state, and repo all depend on domain, so domain is the shared vocabulary.
- **D-02:** `PollClass` is introduced as a distinct enum, NOT a rename of `DeviceType`. Mapping flows `DeviceType → PollClass → interval`. Rationale: multiple device types can share a class; decouples user-facing categorization from polling behavior; lets future device types slot into existing classes without new intervals.
- **D-03:** `PollClass` values: `core`, `standard`, `low`. Semantic (importance tier), 3 buckets. Mirrors research doc's naming.
- **D-04:** DeviceType → PollClass mapping:
  - `router` → `core`
  - `switch` → `core`
  - `ap` → `standard`
  - `unknown` → `standard` (fallback)
  - `virtual` → `low`
- **D-05:** `VolatilityClass` values: `static`, `operational`, `performance`. Matches the state engine / research doc vocabulary.

### Interval constants (single source of truth)

- **D-06:** Default intervals live as hardcoded `time.Duration` constants in `internal/domain/` (e.g., `domain.PollClassCoreInterval = 30 * time.Second`). Ship fast; no DB surface; mirrors the Phase 38 "hardcoded thresholds now, configurable later" pattern (D-12/D-13 from 38-CONTEXT.md).
- **D-07:** Performance-class intervals (what `PollClass` governs):
  - `core` = 30s
  - `standard` = 60s
  - `low` = 300s
- **D-08:** Operational class default = 60s (shared across all devices, not PollClass-scoped). Static class default = 300s (shared). Rationale: operational = reachability checks (sysUpTime, ifOperStatus); static = inventory/topology walks that rarely change. These are system-defined, not user-tunable in this milestone.
- **D-09:** **ROADMAP DEVIATION noted** — roadmap Phase 39 success criterion 2 reads "router=30s, switch=60s, AP=120s, virtual=300s" (4 distinct per-device-type intervals). The 3-bucket `PollClass` model collapses switch into `core` (30s) and promotes AP from 120s to 60s. Plan task: update ROADMAP.md during this phase's execution to reflect `core=30s / standard=60s / low=300s` with the new DeviceType mapping and regenerate the success criterion text so goal-backward verification passes.

### Vendor YAML OID grouping

- **D-10:** Restructure `snmp:` in vendor YAMLs into three nested sub-maps: `static`, `operational`, `performance`. Existing flat OIDs (`cpu_oid`, `memory_used_oid`, `memory_total_oid`, `temperature_oid`, `temperature_scale`) move under `snmp.performance`. `internal/vendor/schema.go` `SNMPConfig` type is rewritten to match.
- **D-11:** Standard-MIB operational OIDs (`sysUpTime`, `ifOperStatus`) live **only in `default.yaml`** under `snmp.operational`. Vendor-specific YAMLs (mikrotik, ubiquiti) override only when they genuinely diverge from standard MIBs (rare). Rationale: standard OIDs never vary by vendor; duplicating them across files is pure maintenance debt.
- **D-12:** `snmp.static` group is added as an **empty placeholder** in all vendor YAMLs (schema exists, no OIDs populated). `ifTable` / `ifXTable` / LLDP / CDP walks stay in `internal/snmp/discovery.go` as today. Phase 40 decides whether to migrate those into the YAML. Keeps Phase 39 scope minimal and avoids touching discovery.go.
- **D-13:** Registry resolution: `Registry.ResolveSNMPConfig(vendorName)` is extended (or replaced by three methods — `ResolveStaticOIDs`, `ResolveOperationalOIDs`, `ResolvePerformanceOIDs`) so collectors (Phase 40) can pull OIDs scoped to the tier they're polling. Vendor-specific values still override defaults tier-by-tier.
- **D-14:** All three existing vendor YAML files (`default.yaml`, `mikrotik.yaml`, `ubiquiti.yaml`) must be migrated in the same commit as the schema change. No back-compat for the old flat shape — we own every YAML in the tree. Runtime DB records (`vendor_configs` table) are re-seeded from the embedded YAMLs via existing migration seeding logic.

### Device table migration + classification

- **D-15:** New migration `000016_device_poll_classification.up.sql` / `.down.sql`:
  ```sql
  ALTER TABLE devices ADD COLUMN poll_class TEXT NOT NULL DEFAULT 'standard';
  ALTER TABLE devices ADD COLUMN poll_interval_override INTEGER; -- nullable; seconds
  ```
  Note: down migration must drop both columns (use the standard SQLite table-rebuild pattern established in prior migrations).
- **D-16:** Backfill happens in a **Go-level data migration function** (not inline SQL). Follows the `migrateEncryptSNMPCredentials` pattern in `internal/repository/sqlite/migrations.go`: after `RunMigrations` applies the SQL, a new `migrateDevicePollClass(db)` function reads every device row, calls the same `domain.ClassifyPollClass(deviceType)` helper the runtime uses, and updates `poll_class` to the computed value. Rationale: single source of truth for classification — the SQL CASE statement in an `.up.sql` and the Go helper would drift.
- **D-17:** `poll_interval_override` semantics: applies to the **performance class only**. Static and operational intervals are always system-defined (D-08). Override is a raw `*int` seconds value on `domain.Device` — `nil` = use class default, `*int = 15` = poll CPU/mem/temp every 15s regardless of class. Research doc recommendation.
- **D-18:** Override storage: `poll_interval_override INTEGER NULL` (nullable integer, seconds). NOT a higher-level `poll_class_override` label. Rationale: matches how operators think ("poll this device every 15 seconds"), maximum flexibility, matches research doc.
- **D-19:** Auto-reclassification on `device_type` change: `DeviceService.Update()` / any path that mutates `device_type` re-applies `ClassifyPollClass(newType)` to `poll_class` **unless** `poll_interval_override` is already set (user took manual control — don't stomp). Single call site in DeviceService keeps the logic in one place. Also applies after `ReprobeDevice` detects a device_type change.

### Domain surface (checklist for planner)

- **D-20:** New exports in `internal/domain/`:
  - `type PollClass string` + constants `PollClassCore`, `PollClassStandard`, `PollClassLow`
  - `type VolatilityClass string` + constants `VolatilityClassStatic`, `VolatilityClassOperational`, `VolatilityClassPerformance`
  - Interval constants (names tentative): `PollClassCoreInterval`, `PollClassStandardInterval`, `PollClassLowInterval`, `OperationalClassInterval`, `StaticClassInterval` — all `time.Duration`.
  - `func ClassifyPollClass(deviceType DeviceType) PollClass` — the classification helper consumed by both runtime and migration.
  - `func (PollClass) Interval() time.Duration` — convenience accessor.
  - `Device` struct gains two fields: `PollClass PollClass` and `PollIntervalOverride *int` (seconds). JSON tags: `"poll_class"`, `"poll_interval_override"`.
- **D-21:** `DeviceRepository` interface is unchanged; the two new columns flow through existing `Create` / `Update` / `GetByID` / `GetAll` methods via the repo implementation in `internal/repository/sqlite/device_repo.go`. Plan must update: column list in INSERT, SELECT, UPDATE statements; scanDevice; and any test fixtures.

### Claude's Discretion

- Exact type/variable naming (e.g., `PollClassCoreInterval` vs `CoreInterval` — whatever reads best in-package).
- File layout within `internal/domain/` (new `poll_class.go` file vs extending `device.go`).
- Migration number (`000016_*` is the next available based on current tree at `000015_scale_indexes.up.sql`).
- Whether `ClassifyPollClass` returns `PollClassStandard` for empty string or `DeviceTypeUnknown` explicitly (map both to standard).
- Whether the three `Resolve{Static,Operational,Performance}OIDs` registry methods are separate or one `ResolveVolatilityTier(tier)` method — planner picks based on ergonomics.
- Test fixture updates scope — planner decides which existing tests need re-seeding.

### Folded Todos

None — no backlog todos matched this phase.

</decisions>

<canonical_refs>
## Canonical References

**Downstream agents MUST read these before planning or implementing.**

### Phase 39 scope & roadmap
- `.planning/REQUIREMENTS.md` — POLL-03 (OID volatility classes in vendor config), POLL-05 (auto-classify by device type with override)
- `.planning/ROADMAP.md` §Phase 39 — Success criteria (note D-09 deviation pending ROADMAP update in this phase's execution)
- `.planning/PROJECT.md` §Current Milestone — "Device classification: auto by device_type, user-overridable per device"

### Architecture research (authoritative for shape and rationale)
- `.planning/research/ARCHITECTURE.md` §Domain Type Additions — PollClass, VolatilityClass shapes the decisions here follow
- `.planning/research/ARCHITECTURE.md` §Database Migration — the poll_class / poll_interval_override columns
- `.planning/research/ARCHITECTURE.md` §Suggested Build Order Phase 2 — confirms "Phase 39 = domain types + migration"
- `.planning/research/ARCHITECTURE.md` §Scheduler Poll Frequency Classification table — original (4-interval) mapping that this phase deviates from (see D-09)

### Prior phase context
- `.planning/phases/38-state-engine/38-CONTEXT.md` — D-12/D-13 "hardcoded thresholds, configurable later" pattern this phase mirrors for intervals
- `.planning/STATE.md` §Accumulated Context — confirms hardcoded-defaults-ship-fast approach and DeviceLinkCache coexistence constraint

### Code to modify (existing files)
- `internal/domain/device.go` — Device struct, DeviceType enum (this phase adds PollClass field + poll_class_override field)
- `internal/domain/settings.go` — reference only; no new settings in this phase (D-06 keeps intervals as consts, not settings rows)
- `internal/vendor/schema.go` — SNMPConfig rewrite for nested static/operational/performance groups
- `internal/vendor/registry.go` — ResolveSNMPConfig rework for per-tier resolution (D-13)
- `internal/vendor/data/default.yaml` — restructure snmp: section + add standard operational OIDs (D-11)
- `internal/vendor/data/mikrotik.yaml` — restructure snmp: section (vendor overrides)
- `internal/vendor/data/ubiquiti.yaml` — restructure snmp: section (vendor overrides)
- `internal/repository/sqlite/migrations/000016_device_poll_classification.up.sql` — new migration
- `internal/repository/sqlite/migrations/000016_device_poll_classification.down.sql` — new migration
- `internal/repository/sqlite/migrations.go` — register migrateDevicePollClass Go data migration following migrateEncryptSNMPCredentials pattern
- `internal/repository/sqlite/device_repo.go` — extend INSERT/SELECT/UPDATE column lists + scanDevice for new fields
- `internal/service/device_service.go` — auto-reclassify on device_type change (D-19)
- `.planning/ROADMAP.md` §Phase 39 success criterion 2 — update to reflect core/standard/low numbers (D-09)

### Existing patterns to follow
- `internal/repository/sqlite/migrations.go` — `migrateEncryptSNMPCredentials` is the exact pattern for D-16 (Go-level data migration called from RunMigrations after SQL applies)
- `internal/state/store.go` / `internal/state/health.go` — Phase 38's "hardcoded defaults now" pattern for threshold constants

</canonical_refs>

<code_context>
## Existing Code Insights

### Reusable Assets
- `domain.DeviceType` enum (router/switch/ap/virtual/unknown) — source for classification
- `migrateEncryptSNMPCredentials` in `internal/repository/sqlite/migrations.go` — exact template for D-16 Go-level data migration
- `Registry.ResolveSNMPConfig` — existing method whose semantics this phase extends tier-by-tier
- `golang-migrate/migrate/v4` wiring in `runSQLiteMigrations` — automatic pickup of numbered migrations from `migrations/*.sql` via embed.FS, no manual registration
- `lastPreMigrateVersion = 5` constant + `ensureMigrationVersion` — safe to add new migrations after; no touching legacy seeding
- State-engine pattern of hardcoded constants co-located with type declarations (`internal/state/health.go` `defaultThresholds`)

### Established Patterns
- Domain enums as typed strings with `Type<Value>` constants — `DeviceType`, `DeviceStatus`, `MetricsSource` all use this shape; PollClass and VolatilityClass follow it
- Constructor injection of dependencies via `New*` in main.go — no new wiring in this phase since state and collectors aren't consuming these types yet; just types + DB schema
- Vendor YAML is loaded via `LoadRegistryFromYAML` (embedded) and synced to DB via `vendor_configs` table — schema change in YAML + struct is automatically picked up on next boot
- Migration down files exist for every up; down must handle SQLite's no-drop-column-pre-3.35 quirk (see `000014_drop_ssh_profile_id.down.sql` for the table-rebuild pattern)
- Go-level data migrations run AFTER SQL migrations inside `RunMigrations`, inside the same function call — they see the new schema

### Integration Points
- `RunMigrations` in `internal/repository/sqlite/migrations.go` — hook for `migrateDevicePollClass` call (after `migrateEncryptSNMPCredentials`)
- `DeviceService.Update()` and/or `ReprobeDevice()` in `internal/service/device_service.go` — the call sites for D-19 auto-reclassification
- `device_repo.go` — INSERT, SELECT, UPDATE column lists (lines ~80-90 for Create; Update methods; scanDevice helper) need the two new columns wired through
- `internal/vendor/registry_test.go` / `schema_test.go` — will need updates for the new nested SNMPConfig shape
- Future consumers (Phase 40 collectors, Phase 41 scheduler) — this phase's exports are their upstream contract

</code_context>

<specifics>
## Specific Ideas

- User consistently chose the recommended (research doc) option across all four areas, favoring the typed-foundation approach: shared domain types, nested vendor YAML, hardcoded interval consts, Go-level backfill that reuses the runtime classification helper.
- User accepted the 3-bucket / roadmap-mismatch tradeoff rather than expanding to 4 buckets, prioritizing the cleaner research-doc enum over literal roadmap compliance — but this phase must therefore update ROADMAP.md text as part of its work.

</specifics>

<deferred>
## Deferred Ideas

- **Settings-driven intervals** (THRESH-01-style): exposing `poll_interval_core`, `poll_interval_standard`, `poll_interval_low`, `poll_interval_operational`, `poll_interval_static` as configurable settings rows + admin UI. Deferred to a future milestone — same story as Phase 38 hysteresis thresholds. Hardcoded constants ship now; configurability follows the same pattern later.
- **Static-group OID migration**: moving `ifTable` / `ifXTable` / LLDP / CDP walks out of `internal/snmp/discovery.go` and into vendor YAML `snmp.static`. Skipped per D-12. Phase 40 may reopen this when wiring StaticCollector.
- **Per-class override columns**: `poll_interval_operational_override` and `poll_interval_static_override`. Skipped per D-17 — static and operational are system-defined until THRESH-style configurability lands.
- **4-bucket PollClass**: a `core / high / standard / low` split that would preserve the roadmap's literal 4 distinct per-device-type intervals. Not chosen; would expand surface area without operational benefit.
- **Frontend poll_class display + override UI**: adding a "Polling every Ns" label + override form field on device cards. Belongs to Phase 44 (Frontend Integration) per roadmap; Phase 39 only exposes the underlying fields via the existing device JSON contract.
- **API validation for `poll_interval_override`**: enforcing a minimum (e.g., ≥10s per PROJECT.md "Real-time sub-second polling out of scope" constraint). Should be added in Phase 40/42 when the scheduler actually consumes the override — Phase 39 just stores it.

</deferred>

---

*Phase: 39-domain-types-db-migration*
*Context gathered: 2026-04-12*
