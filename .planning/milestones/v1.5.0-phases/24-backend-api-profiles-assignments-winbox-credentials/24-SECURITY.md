---
phase: 24
slug: backend-api-profiles-assignments-winbox-credentials
status: verified
threats_open: 0
asvs_level: 1
created: 2026-04-09
---

# Phase 24 — Security

> Per-phase security contract: threat register, accepted risks, and audit trail.

---

## Trust Boundaries

| Boundary | Description | Data Crossing |
|----------|-------------|---------------|
| client → API (repo layer) | Assignment and WinBox operations accept UUIDs from URL paths | UUIDs (low sensitivity) |
| API → SQLite | SQL queries use parameterized statements from user-supplied IDs | DB queries (internal) |
| client → /devices/{id}/credential-profiles | Untrusted UUID in path and profile_id in body | Profile assignment (medium sensitivity) |
| client → /devices/{id}/winbox-credentials | Returns decrypted password — highest sensitivity endpoint | Plaintext password (high sensitivity) |
| handler → BackupService | Decryption happens in service layer, never in handler | Encrypted secret → plaintext (high sensitivity) |
| client → /bridge/download/{os}/{arch} | Untrusted os/arch values in URL path | Binary file download (low sensitivity) |
| handler → filesystem | File path constructed from validated allowlist values | Binary file path (low sensitivity) |

---

## Threat Register

| Threat ID | Category | Component | Disposition | Mitigation | Status |
|-----------|----------|-----------|-------------|------------|--------|
| T-24-01 | Tampering | credential_profile_repo.go | mitigate | All 6 new repo methods use parameterized queries (`?` placeholders) — no string interpolation of user input | closed |
| T-24-02 | Tampering | IsInUse (D-14) | mitigate | Updated to check canonical `device_credential_profiles` table; old FK column no longer consulted | closed |
| T-24-03 | Information Disclosure | credential_profile_handler.go | mitigate | `credentialProfileResponse` struct omits `EncryptedSecret` entirely; `GetWinboxAssignment` returns it only for server-side use | closed |
| T-24-04 | Tampering | config.go BridgeBinariesDir | accept | Config field is server-side only; not exposed via API; read from trusted config.yaml or env var | closed |
| T-24-05 | Information Disclosure | HandleGetWinboxCredentials | mitigate | Decrypted password ONLY returned by this single endpoint; handler never holds encryption key; password decrypted in BackupService.GetWinboxCredentials | closed |
| T-24-06 | Information Disclosure | HandleListAssignments | mitigate | Response struct (`assignedProfileResponse`) explicitly omits `encrypted_secret`; only returns name/username/port/role/is_winbox | closed |
| T-24-07 | Tampering | HandleAssign body | mitigate | `profile_id` parsed via `uuid.Parse` — rejects non-UUID input; FK constraints prevent assignment to nonexistent device/profile | closed |
| T-24-08 | Tampering | HandleUnassign path | mitigate | Both `deviceID` and `profileID` extracted and parsed via `uuid.Parse` from URL path segments; invalid UUIDs return 400 | closed |
| T-24-09 | Denial of Service | HandleSetWinbox transaction | mitigate | Transaction scope minimal (2 UPDATE statements); SQLite `busy_timeout=5000ms` configured at connection level | closed |
| T-24-10 | Information Disclosure | WinBox password empty | mitigate | `GetWinboxCredentials` returns error when decrypted password is empty string — prevents silent auth failure | closed |
| T-24-11 | Tampering | HandleDownload path traversal | mitigate | `os` and `arch` validated against allowlists (`validOS`, `validArch` maps) before constructing filename; `filepath.Join` used; filename derived from validated values only, never from raw user input | closed |
| T-24-12 | Tampering | HandleDownload arbitrary file read | mitigate | Filename pattern is hardcoded `winbox-bridge-{os}-{arch}[.exe]` — attacker cannot inject `..` or arbitrary filenames because os/arch must match the allowlist exactly | closed |
| T-24-13 | Information Disclosure | HandleDownload error messages | accept | Error messages reveal whether a binary exists for a valid platform — low-sensitivity operational information, not a security concern | closed |
| T-24-14 | Denial of Service | Large binary download | accept | `http.ServeFile` handles Content-Length and range requests efficiently; bridge binaries are ~10MB; no amplification vector | closed |

*Status: open · closed*
*Disposition: mitigate (implementation required) · accept (documented risk) · transfer (third-party)*

---

## Accepted Risks Log

| Risk ID | Threat Ref | Rationale | Accepted By | Date |
|---------|------------|-----------|-------------|------|
| AR-24-01 | T-24-04 | `BridgeBinariesDir` is a server-side config field read from trusted config.yaml or env var; never exposed via API | Plan author | 2026-04-09 |
| AR-24-02 | T-24-13 | Download endpoint error messages disclose platform availability — operational info with no meaningful attack value | Plan author | 2026-04-09 |
| AR-24-03 | T-24-14 | Bridge binaries are ~10MB; `http.ServeFile` range-request handling prevents amplification; no DoS mitigation required | Plan author | 2026-04-09 |

---

## Security Audit Trail

| Audit Date | Threats Total | Closed | Open | Run By |
|------------|---------------|--------|------|--------|
| 2026-04-09 | 14 | 14 | 0 | gsd-secure-phase (automated) |

---

## Sign-Off

- [x] All threats have a disposition (mitigate / accept / transfer)
- [x] Accepted risks documented in Accepted Risks Log
- [x] `threats_open: 0` confirmed
- [x] `status: verified` set in frontmatter

**Approval:** verified 2026-04-09
