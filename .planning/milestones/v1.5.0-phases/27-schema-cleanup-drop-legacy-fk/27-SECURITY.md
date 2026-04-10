---
phase: 27
slug: schema-cleanup-drop-legacy-fk
status: verified
threats_open: 0
asvs_level: 1
created: 2026-04-09
---

# Phase 27 — Security

> Per-phase security contract: threat register, accepted risks, and audit trail.

---

## Trust Boundaries

| Boundary | Description | Data Crossing |
|----------|-------------|---------------|
| migration -> database | Schema change modifies live SQLite data in-place via 12-step table-recreation | Device rows (non-sensitive; credentials stored in separate encrypted column) |
| backup_service -> credential_profile_repo | New credential resolution path for backup operations via join table | CredentialProfile with EncryptedSecret (decrypted only at use site) |
| frontend -> API | BulkBackupPanel makes N additional API calls (fetchDeviceCredentialProfiles per device) before backup starts | Credential profile metadata (no secrets) |

---

## Threat Register

| Threat ID | Category | Component | Disposition | Mitigation | Status |
|-----------|----------|-----------|-------------|------------|--------|
| T-27-01 | Tampering | migration 000014 | mitigate | Table recreation wrapped in `PRAGMA foreign_keys=off/on`; rows copied by explicit column name (not `SELECT *`) preventing column mismatch | closed |
| T-27-02 | Denial of Service | migration 000014 | mitigate | Migration runs inside golang-migrate transaction management; on failure, dirty flag prevents partial state; next startup retries cleanly | closed |
| T-27-03 | Information Disclosure | GetBackupProfileForDevice | mitigate | Returns `CredentialProfile` with `EncryptedSecret` (`json:"-"` prevents API exposure); decryption only inside `BackupService.decryptSecret()` — no new exposure surface | closed |
| T-27-04 | Elevation of Privilege | backup credential resolution | accept | `GetBackupProfileForDevice` picks first non-WinBox profile (`ORDER BY is_winbox ASC`) matching pre-existing single-profile behavior; future CRED-F01 (explicit is_backup_profile flag) deferred | closed |
| T-27-05 | Tampering | rollback safety | mitigate | Down migration uses simple `ALTER TABLE devices ADD COLUMN ssh_profile_id TEXT DEFAULT NULL`; no data loss; existing `device_credential_profiles` data unaffected in either direction | closed |
| T-27-06 | Denial of Service | BulkBackupPanel | accept | Pre-fetching credential profiles for all devices adds N API calls; calls are parallel via `Promise.allSettled`, each is a lightweight DB query; acceptable at supported scale (100+ devices) | closed |
| T-27-07 | Information Disclosure | SSHCredentialForm | mitigate | SSHCredentialForm migrated from `updateDevice({ssh_profile_id})` to dedicated `assignCredentialProfile`/`unassignCredentialProfile` API — profile IDs no longer exposed in device update request logs | closed |

*Status: open · closed*
*Disposition: mitigate (implementation required) · accept (documented risk) · transfer (third-party)*

---

## Accepted Risks Log

| Risk ID | Threat Ref | Rationale | Accepted By | Date |
|---------|------------|-----------|-------------|------|
| AR-27-01 | T-27-04 | `GetBackupProfileForDevice` selection order (non-WinBox first) matches pre-existing single-profile behavior. Explicit `is_backup_profile` flag deferred to CRED-F01 milestone item. | plan | 2026-04-08 |
| AR-27-02 | T-27-06 | N parallel `fetchDeviceCredentialProfiles` calls via `Promise.allSettled` are acceptable at 100+ device scale; each call is a lightweight indexed join query. No rate-limiting needed at current scale. | plan | 2026-04-08 |

---

## Security Audit Trail

| Audit Date | Threats Total | Closed | Open | Run By |
|------------|---------------|--------|------|--------|
| 2026-04-09 | 7 | 7 | 0 | gsd-secure-phase (automated) |

---

## Sign-Off

- [x] All threats have a disposition (mitigate / accept / transfer)
- [x] Accepted risks documented in Accepted Risks Log
- [x] `threats_open: 0` confirmed
- [x] `status: verified` set in frontmatter

**Approval:** verified 2026-04-09
