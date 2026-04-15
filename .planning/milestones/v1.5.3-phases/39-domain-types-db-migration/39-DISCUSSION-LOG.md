# Phase 39: Domain Types & DB Migration - Discussion Log

> **Audit trail only.** Do not use as input to planning, research, or execution agents.
> Decisions are captured in CONTEXT.md — this log preserves the alternatives considered.

**Date:** 2026-04-12
**Phase:** 39-domain-types-db-migration
**Areas discussed:** Type placement + PollClass, Vendor YAML OID grouping, Interval source of truth, Migration shape + override semantics

---

## Type placement + PollClass

### Where should VolatilityClass and PollClass types live?

| Option | Description | Selected |
|--------|-------------|----------|
| internal/domain (Recommended) | Shared language used by repo, service, scheduler, collector, state. No cyclic imports. | ✓ |
| internal/state (next to HealthStatus) | Co-located with store, but would invert layering (state imported by domain repos). | |
| New internal/pipeline package | New shared package for pipeline-wide types. Overkill for a handful of enums. | |

### PollClass abstraction, or map DeviceType directly to intervals?

| Option | Description | Selected |
|--------|-------------|----------|
| PollClass enum (Recommended) | DeviceType → PollClass → interval. Multiple types can share a class. Decouples UX categories from polling behavior. | ✓ |
| Direct DeviceType mapping | Skip PollClass; map DeviceType straight to interval. Simpler but less flexible. | |

### PollClass bucket identity?

| Option | Description | Selected |
|--------|-------------|----------|
| core / standard / low (Recommended) | Research doc values. Semantic importance tier. 3 buckets. | ✓ |
| fast / normal / slow | Cadence-descriptive. Couples name to frequency. | |
| Match DeviceType 1:1 (4 buckets) | Rename of DeviceType for polling. | |

### Follow-up: 3-bucket vs 4-interval mismatch

Roadmap says router=30s/switch=60s/AP=120s/virtual=300s (4 distinct intervals) but 3-bucket PollClass can only produce 3 intervals. Conflict surfaced after initial 3 decisions.

| Option | Description | Selected |
|--------|-------------|----------|
| 4 buckets matching roadmap | Preserve roadmap intervals exactly by expanding to 4 PollClass values. | |
| 3 buckets, adjust intervals (Recommended) | core=30s (router+switch), standard=60s (ap+unknown), low=300s (virtual). Deviates from roadmap switch=60s and AP=120s. | ✓ |
| 3 buckets, roadmap-matching intervals | core=30s (router), standard=60s (switch+ap), low=300s (virtual). Drops AP's 120s tier. | |

**User's choice:** 3 buckets with adjusted intervals.
**Notes:** Accepted roadmap deviation — router+switch fold into core. ROADMAP.md success criterion 2 needs updating in this phase's execution commits.

---

## Vendor YAML OID grouping

### How should the vendor YAML express OID volatility groups?

| Option | Description | Selected |
|--------|-------------|----------|
| Nested groups (Recommended) | Split snmp: into { static, operational, performance } sub-maps. Clean, explicit, matches domain types. | ✓ |
| Flat list with class annotations | Array of entries tagged with class. More flexible but less type-safe. | |
| Keep flat, add minimal class hints | Don't restructure; add a separate PollGroup metadata block. Minimal churn but awkward. | |

### Where should standard-MIB operational OIDs (sysUpTime, ifOperStatus) live?

| Option | Description | Selected |
|--------|-------------|----------|
| default.yaml only (Recommended) | Standard MIB OIDs never vary by vendor. Vendor files override only if needed. | ✓ |
| Every vendor YAML (duplicated) | Explicit but duplicative. | |
| Hardcoded constants in collector package | Bake into internal/collector/operational.go as const. | |

### Do we populate the 'static' OID group now (ifTable, LLDP), or leave it empty?

| Option | Description | Selected |
|--------|-------------|----------|
| Empty placeholder for now (Recommended) | Add static: {} group to schema; ifTable/LLDP stay in snmp/discovery.go. Phase 40 decides whether to migrate. | ✓ |
| Populate fully now | Move ifTable/LLDP OIDs into YAML. Expands scope. | |

---

## Interval source of truth

### Where should the PollClass interval values live?

| Option | Description | Selected |
|--------|-------------|----------|
| Hardcoded consts in domain (Recommended) | time.Duration constants in internal/domain. Mirrors Phase 38 hardcoded thresholds. | ✓ |
| Seed settings rows in the migration | Add poll_interval_* settings, exposed via /api/v1/settings on day one. | |
| Defer to Phase 41 entirely | No interval mapping in Phase 39. Risk: Phase 40 collectors need effective interval. | |

### Should this phase also expose operational/static class intervals?

| Option | Description | Selected |
|--------|-------------|----------|
| Yes — all three class intervals (Recommended) | Complete typed foundation: performance (PollClass), operational, static. | ✓ |
| No — only PollClass performance intervals | Smaller scope; operational/static left to Phase 41. | |

### Operational and static class default intervals?

| Option | Description | Selected |
|--------|-------------|----------|
| Operational=60s, Static=300s (Recommended) | Research doc values. Balanced. | ✓ |
| Operational=30s, Static=300s | Aggressive reachability. Doubles op load for marginal benefit given soft/hard down model. | |
| Operational=60s, Static=900s | Slower discovery walks; topology drifts longer. | |

---

## Migration shape + override semantics

### SQL-only backfill vs Go-level data migration?

| Option | Description | Selected |
|--------|-------------|----------|
| Go-level data migration (Recommended) | migrateEncryptSNMPCredentials pattern. Single source of truth for classification logic. | ✓ |
| Pure SQL up.sql (CASE device_type) | Classification logic exists twice (SQL + Go) and can drift. | |

### What does poll_interval_override apply to?

| Option | Description | Selected |
|--------|-------------|----------|
| Performance class only (Recommended) | Static and operational stay system-defined. Research doc rec. | ✓ |
| All classes uniformly | Risk: user accidentally pins a device to 10s static polls. | |
| Separate override column per class | Maximum flexibility, more schema surface. | |

### Override column type: raw seconds or higher-level label?

| Option | Description | Selected |
|--------|-------------|----------|
| Nullable INTEGER (seconds) (Recommended) | NULL = use class default. Raw seconds. Matches operator mental model. | ✓ |
| Nullable PollClass label | More abstract; loses flexibility. | |

### When device_type changes after migration, what happens to poll_class?

| Option | Description | Selected |
|--------|-------------|----------|
| Re-apply on type change (Recommended) | DeviceService re-classifies unless override is set. Keeps auto-classification current. | ✓ |
| Stick with stored value | User must manually fix stale classifications. | |

---

## Claude's Discretion

- Exact type/variable naming (`PollClassCoreInterval` vs `CoreInterval`)
- File layout within `internal/domain/` (new file vs extending device.go)
- Whether Registry exposes three `Resolve{Static,Operational,Performance}OIDs` methods or one `ResolveVolatilityTier(tier)` method
- Test fixture update scope
- Migration number `000016_*`
- `ClassifyPollClass` fallback behavior for empty/unknown device_type strings

## Deferred Ideas

- Settings-driven intervals (configurable later, THRESH-01 pattern)
- Static-group OID migration (Phase 40 may reopen)
- Per-class override columns (not chosen — performance only)
- 4-bucket PollClass variant
- Frontend override UI (Phase 44)
- API validation for override minimum (Phase 40/42)
