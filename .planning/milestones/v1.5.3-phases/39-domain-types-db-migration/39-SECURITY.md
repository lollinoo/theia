---
phase: 39
slug: domain-types-db-migration
status: verified
threats_open: 0
asvs_level: 1
created: 2026-04-12
---

# Phase 39 - Security

> Per-phase security contract: threat register, accepted risks, and audit trail.

---

## Trust Boundaries

| Boundary | Description | Data Crossing |
|----------|-------------|---------------|
| domain layer | Phase 39 domain types are pure in-process types; external input reaches them only through existing service, API, and repository paths. | `DeviceType`, `PollClass`, `PollIntervalOverride` metadata |
| YAML -> struct | Embedded vendor YAML is bundled at compile time and is not user-controlled. | Static, operational, and performance OID strings |
| DB vendor configs -> struct | DB-backed vendor JSON remains writable through the existing vendor-config admin path. | Vendor SNMP OID overrides |
| Registry -> SNMP client | Resolved OIDs are passed into `gosnmp`, which validates OID syntax. | OID strings only |
| Migration runtime -> DB | Startup migrations run with server DB credentials and backfill existing device rows. | `device_type`, `poll_class`, `poll_interval_override` row data |
| SNMP probe -> service | Untrusted device identity data crosses into `probeDevice()` and influences `DeviceType` -> `PollClass` classification. | `sysName`, `sysDescr`, `sysObjectID`, vendor, `DeviceType` |
| Operator -> service/API | A future operator-facing override path could supply `poll_interval_override`; in Phase 39 the value is stored and read only. | Integer interval override with availability impact, not secrecy impact |

---

## Security Audit 2026-04-12

| Metric | Count |
|--------|-------|
| Threats found | 14 |
| Closed | 14 |
| Open | 0 |
| Unregistered flags | 0 |

---

## Threat Register

| Threat ID | Category | Component | Disposition | Mitigation | Status |
|-----------|----------|-----------|-------------|------------|--------|
| T-39-01 | Tampering | `PollIntervalOverride *int` | accept | Accepted risk AR-39-01: Phase 39 persists the field only; validation is deferred until a consumer/API exists. | closed |
| T-39-02 | Information Disclosure | `domain.PollClass` JSON serialization | accept | Accepted risk AR-39-02: field rides the existing authenticated device JSON contract. | closed |
| T-39-03 | Denial of Service | `ClassifyPollClass` graceful fallback | mitigate | Default branch falls back to `PollClassStandard`, with explicit unknown-literal coverage in unit tests. | closed |
| T-39-04 | Tampering | DB-stored vendor config JSON with malicious OIDs | accept | Accepted risk AR-39-04: vendor-config JSON trust boundary is pre-existing and OID syntax is validated downstream. | closed |
| T-39-05 | Denial of Service | Missing `snmp.performance` group | mitigate | `PollDeviceMetrics` falls back to `OidHrProcessorLoad`, and schema tests cover missing nested performance config without panic. | closed |
| T-39-06 | Information Disclosure | `OperationalOIDs.SysUpTimeOID` via vendor-config API | accept | Accepted risk AR-39-06: standard MIB OIDs are public operational metadata. | closed |
| T-39-07 | Tampering | Migration idempotency on re-run | mitigate | `migrateDevicePollClass` skips rows whose computed class already matches, with an explicit idempotency test. | closed |
| T-39-08 | Tampering | Down migration data preservation | mitigate | The rollback migration copies an explicit pre-39 column list, including `sys_name_lookup`, and recreates the lookup index. | closed |
| T-39-09 | Tampering | `poll_interval_override` stored without validation | accept | Accepted risk AR-39-09: storage-only in Phase 39; validation is deferred until the override becomes user-settable. | closed |
| T-39-10 | Information Disclosure | `poll_class` column via `/api/v1/devices` | accept | Accepted risk AR-39-10: polling cadence is operational metadata on an existing authenticated contract. | closed |
| T-39-11 | Tampering | `probeDevice()` classification | accept | Accepted risk AR-39-11: malicious SNMP identity claims are a pre-existing detector risk and this phase does not enlarge the surface. | closed |
| T-39-12 | Denial of Service | Reclassify guard | mitigate | Probe paths only reassign `PollClass` when no override exists; behavior is bounded by the existing probe loop and covered by regression tests. | closed |
| T-39-13 | Elevation of Privilege | Override bypass | mitigate | Phase 39 exposes no service-layer write field for `poll_interval_override`; probe logic only reads the value, so the future validation gap is not reachable in this phase. | closed |
| T-39-14 | Information Disclosure | `ROADMAP.md` edit | accept | Accepted risk AR-39-14: roadmap changes are tracked planning artifacts with no secrets. | closed |

*Status: open · closed*
*Disposition: mitigate (implementation required) · accept (documented risk) · transfer (third-party)*

---

## Accepted Risks Log

| Risk ID | Threat Ref | Rationale | Accepted By | Date |
|---------|------------|-----------|-------------|------|
| AR-39-01 | T-39-01 | `poll_interval_override` is persisted but not consumed in Phase 39; unsafe values cannot affect runtime yet. | Codex retroactive audit | 2026-04-12 |
| AR-39-02 | T-39-02 | `poll_class` is exposed only through the existing authenticated device payload. | Codex retroactive audit | 2026-04-12 |
| AR-39-04 | T-39-04 | Vendor-config JSON was already an admin-controlled trust boundary before the nested SNMP schema change. | Codex retroactive audit | 2026-04-12 |
| AR-39-06 | T-39-06 | `sysUpTime` and `ifOperStatus` OIDs are standard MIB identifiers, not secrets. | Codex retroactive audit | 2026-04-12 |
| AR-39-09 | T-39-09 | Validation is deferred until the override has an operator-facing write path and runtime consumer. | Codex retroactive audit | 2026-04-12 |
| AR-39-10 | T-39-10 | `poll_class` adds operational metadata only and does not widen the authenticated `/api/v1/devices` audience. | Codex retroactive audit | 2026-04-12 |
| AR-39-11 | T-39-11 | Device identity spoofing through SNMP discovery is an existing detector limitation; Phase 39 only maps the result into three cadence buckets. | Codex retroactive audit | 2026-04-12 |
| AR-39-14 | T-39-14 | Planning-document edits are versioned and do not carry sensitive data. | Codex retroactive audit | 2026-04-12 |

*Accepted risks do not resurface in future audit runs.*

---

## Threat Verification Evidence

| Threat ID | Evidence |
|-----------|----------|
| T-39-03 | `internal/domain/poll_class.go:73-100`; `internal/domain/poll_class_test.go:11-33` |
| T-39-05 | `internal/snmp/discovery.go:709-713`; `internal/vendor/schema_test.go:143-165` |
| T-39-07 | `internal/repository/sqlite/migrations.go:457-503`; `internal/repository/sqlite/migrations_test.go:355-420` |
| T-39-08 | `internal/repository/sqlite/migrations/000016_device_poll_classification.down.sql:5-48` |
| T-39-12 | `internal/service/device_service.go:190-193`; `internal/service/device_service.go:225-232`; `internal/service/device_service_test.go:1154-1304` |
| T-39-13 | `internal/service/device_service.go:27-36`; `internal/service/device_service.go:190-193`; `internal/service/device_service.go:225-232` |
| Live verification | `go test -race ./internal/domain/...`; `go test -race ./internal/vendor/... ./internal/snmp/...`; `go test ./internal/repository/sqlite/... -run 'TestMigrateDevicePollClass_BackfillsByDeviceType|TestMigrateDevicePollClass_Idempotent' -count=1`; `go test ./internal/service/... -run 'TestProbeDevice_ReclassifyOnTypeChange|TestProbeDevice_RespectsPollIntervalOverride|TestProbeDevice_NoTypeChangeStillSyncsPollClassWhenEmpty' -count=1` |

---

## Security Audit Trail

| Audit Date | Threats Total | Closed | Open | Run By |
|------------|---------------|--------|------|--------|
| 2026-04-12 | 14 | 14 | 0 | Codex |

---

## Sign-Off

- [x] All threats have a disposition (mitigate / accept / transfer)
- [x] Accepted risks documented in Accepted Risks Log
- [x] `threats_open: 0` confirmed
- [x] `status: verified` set in frontmatter

**Approval:** verified 2026-04-12
