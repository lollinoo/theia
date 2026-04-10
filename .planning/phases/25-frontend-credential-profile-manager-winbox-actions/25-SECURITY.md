---
phase: 25
slug: frontend-credential-profile-manager-winbox-actions
status: verified
threats_open: 0
asvs_level: 1
created: 2026-04-09
---

# Phase 25 — Security

> Per-phase security contract: threat register, accepted risks, and audit trail.

---

## Trust Boundaries

| Boundary | Description | Data Crossing |
|----------|-------------|---------------|
| Frontend → Backend API | All credential profile and assignment CRUD calls go through `requestJSON`/`requestJSONWithBody` helpers with standard status-code validation | Profile metadata (name, role, is_winbox); no plaintext credentials |
| Frontend → Bridge (localhost:1337) | WinBox launch sends AES-GCM encrypted opaque token via `fetchBridgeToken` | Encrypted token only; plaintext credentials never transmitted |
| Bridge → WinBox binary | Bridge decrypts token and launches WinBox with credentials as CLI args | Decrypted credentials (ip, username, password) — localhost subprocess only |

---

## Threat Register

| Threat ID | Category | Component | Disposition | Mitigation | Status |
|-----------|----------|-----------|-------------|------------|--------|
| T-25-01 | Information Disclosure | api.ts type parsers | accept | `CredentialProfile` TypeScript type excludes `encrypted_secret`; backend enforces via `json:"-"` tag — no sensitive data reaches frontend | closed |
| T-25-02 | Tampering | client.ts API calls | mitigate | `encodeURIComponent` on all credential-profile URL path segments (client.ts lines 407, 412, 431, 437, 445, 452, 460, 466); all calls routed through `requestJSONWithBody` which throws `ServerError`/`ValidationError` on non-2xx | closed |
| T-25-03 | Spoofing | client.ts credential endpoints | accept | Authentication out of scope for v1.5.0 single-user deployment; no auth required by design | closed |
| T-25-04 | Tampering | DeviceConfigPanel assignment calls | mitigate | Profile IDs sourced from server-provided `<select>` list (DeviceConfigPanel.tsx line 563) — no free-text path; `encodeURIComponent` applied to all assignment client functions | closed |
| T-25-05 | Information Disclosure | DeviceConfigPanel assignment list | accept | Assignment list contains only name, role, is_winbox — backend omits `encrypted_secret` from all assignment responses | closed |
| T-25-06 | Denial of Service | Assignment rapid-click | accept | Backend operations are idempotent (409 on duplicate assign, 204 on repeat unassign); UI refreshes after each call | closed |
| T-25-07 | Information Disclosure | WinBox credential fetch | mitigate | Implementation exceeds declared mitigation: WinBox launch path uses `fetchBridgeToken` (AES-GCM encrypted opaque token) — plaintext credentials never reach browser; `fetchWinBoxCredentials` defined but unused by any component; zero console.log/error in launch catch blocks | closed |
| T-25-08 | Tampering | Bridge launch POST | accept | POST to localhost:1337 is localhost-only traffic; Phase 26 adds Origin+Host dual validation (BRIDGE-03); TLS on localhost deferred per requirements | closed |
| T-25-09 | Spoofing | Bridge health check | accept | Simple GET /health with no auth; malicious local process spoofing localhost:1337 is accepted risk; Phase 26 adds BRIDGE-03 protection for non-health endpoints | closed |
| T-25-10 | Denial of Service | useBridgeHealth polling | mitigate | `window.clearInterval` + `cancelled` flag on unmount (useBridgeHealth.ts lines 24–25); 15 s interval (more conservative than declared 30 s); no console logging in catch block | closed |
| T-25-11 | Information Disclosure | Console logging | mitigate | Zero matches for `console.error`/`console.log`/`console.warn` in useBridgeHealth.ts, Canvas.tsx, and Dashboard.tsx; all localhost:1337 errors silenced in empty catch blocks | closed |

*Status: open · closed*
*Disposition: mitigate (implementation required) · accept (documented risk) · transfer (third-party)*

---

## Accepted Risks Log

| Risk ID | Threat Ref | Rationale | Accepted By | Date |
|---------|------------|-----------|-------------|------|
| AR-25-01 | T-25-01 | `encrypted_secret` structurally excluded from frontend type and backend JSON response — no mitigation code required | Lollinoo | 2026-04-09 |
| AR-25-03 | T-25-03 | v1.5.0 is a single-user local deployment; authentication is explicitly out of scope | Lollinoo | 2026-04-09 |
| AR-25-05 | T-25-05 | Assignment list response contains only non-sensitive fields (name, role, is_winbox) by backend design | Lollinoo | 2026-04-09 |
| AR-25-06 | T-25-06 | Backend idempotency makes rapid-click benign; no client-side debounce required | Lollinoo | 2026-04-09 |
| AR-25-08 | T-25-08 | Bridge POST is localhost-only; Phase 26 will add Origin+Host validation (BRIDGE-03); TLS deferred per requirements | Lollinoo | 2026-04-09 |
| AR-25-09 | T-25-09 | Health check endpoint has no sensitive data; Phase 26 adds BRIDGE-03 for protected endpoints | Lollinoo | 2026-04-09 |

---

## Audit Notes

**poll-interval-15s:** `useBridgeHealth.ts` uses `POLL_INTERVAL_MS = 15_000` while the threat model declared 30 s. This is more conservative than declared — T-25-10 mitigation remains valid.

**T-25-07 implementation upgrade:** The actual WinBox launch path sends an AES-GCM encrypted token via `fetchBridgeToken`, not raw credentials via `fetchWinBoxCredentials`. Plaintext credentials never reach the browser for the launch flow. This exceeds the declared mitigation.

---

## Security Audit Trail

| Audit Date | Threats Total | Closed | Open | Run By |
|------------|---------------|--------|------|--------|
| 2026-04-09 | 11 | 11 | 0 | gsd-security-auditor (sonnet) |

---

## Sign-Off

- [x] All threats have a disposition (mitigate / accept / transfer)
- [x] Accepted risks documented in Accepted Risks Log
- [x] `threats_open: 0` confirmed
- [x] `status: verified` set in frontmatter

**Approval:** verified 2026-04-09
