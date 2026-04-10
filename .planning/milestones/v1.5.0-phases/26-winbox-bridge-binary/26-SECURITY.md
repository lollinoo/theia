---
phase: 26
slug: winbox-bridge-binary
status: verified
threats_open: 0
asvs_level: 1
created: 2026-04-09
---

# Phase 26 — Security

> Per-phase security contract: threat register, accepted risks, and audit trail.

---

## Trust Boundaries

| Boundary | Description | Data Crossing |
|----------|-------------|---------------|
| Browser to Bridge | Cross-origin fetch from Theia frontend (localhost:3000) to bridge (localhost:1337). Any webpage could attempt this. | WinBox launch token (AES-GCM encrypted credentials), health status |
| Bridge to OS | Bridge spawns WinBox process. Must be restricted to only the WinBox executable. | WinBox IP, username, password (as argv, not shell string) |
| Local network | Any process on the same machine can reach localhost:1337. Origin+Host validation is the primary defense for /launch. | Same as above |
| CI runner to GitHub Release | CI job uploads binaries as release assets controlled by GITHUB_TOKEN. | Compiled Go binaries |

---

## Threat Register

| Threat ID | Category | Component | Disposition | Mitigation | Status |
|-----------|----------|-----------|-------------|------------|--------|
| T-26-01 | Spoofing | /launch endpoint | mitigate | `securityCheck` middleware validates `Origin` header against `--theia-origin` (exact match). Returns 403 on mismatch. `/launch` is always wrapped by `securityCheck`. Verified by `TestOriginValidation_EvilOriginOnLaunchReturns403`. | closed |
| T-26-02 | Spoofing | /launch endpoint | mitigate | `securityCheck` validates `Host` header is exactly `localhost:{port}`. Returns 403 on IP host (`127.0.0.1:1337`) or other mismatch. Prevents DNS rebinding. Verified by `TestHostValidation_EvilHostOnLaunchReturns403`, `TestHostValidation_IPHostOnLaunchReturns403`. Note: `/health` is intentionally public — non-sensitive status polling. | closed |
| T-26-03 | Tampering | /launch request body | mitigate | `launchRequest` struct has one field: `Token string` (AES-GCM encrypted). No plaintext IP/username/password accepted. Credentials are inside the encrypted token, unreadable without the bridge secret. Attacker cannot forge a valid token without knowing the 32-byte key. Verified by `TestDecryptLaunchToken_*` tests (tampered/wrong-key tokens rejected). Implementation is *stronger* than planned (original plan: 3 plaintext fields; actual: single encrypted token). | closed |
| T-26-04 | Information Disclosure | /launch error response | mitigate | Decrypt failures return `"invalid or tampered token"` (generic). Process start failures return `"failed to launch WinBox"` (generic). Internal error details logged server-side only, never in HTTP response body. | closed |
| T-26-05 | Information Disclosure | Credential in CLI args | accept | WinBox requires username+password as CLI arguments — visible in process listings (`ps aux`). Accepted: (1) bridge runs on user's own machine, (2) WinBox itself requires this invocation pattern, (3) no alternative method exists. See Accepted Risks Log. | closed |
| T-26-06 | Elevation of Privilege | Arbitrary command execution | mitigate | `winboxPath` set once at startup via `discoverWinBox()` — immutable during runtime. `launchRequest` has only `Token` field; the decrypted `launchCredentials` struct contains only `IP`, `Username`, `Password` — no executable or command field. No code path exists to pass an executable name from HTTP request to `startProcess`. | closed |
| T-26-07 | Denial of Service | Rapid /launch requests | accept | Bridge is a local tool for a single user. Rate limiting not warranted — worst case spawns many WinBox windows, which is self-correcting. See Accepted Risks Log. | closed |
| T-26-08 | Tampering | Command injection via IP/username/password | mitigate | Fields passed as separate args: `args := []string{creds.IP, creds.Username, creds.Password}` → `startProcess(winboxPath, args)`. `exec.Command` does NOT invoke a shell — each arg is a separate argv element. Shell metacharacters in any field are inert. | closed |
| T-26-09 | Tampering | CI build artifacts | mitigate | Binaries built from tagged commits in GitHub Actions (deterministic source). `softprops/action-gh-release@v2` uses pinned major version. `GITHUB_TOKEN` scope limited to repo. | closed |
| T-26-10 | Tampering | Makefile local build | accept | Local `make bridge-build-all` runs on developer's machine — same trust level as any local development workflow. No additional risk beyond normal dev. See Accepted Risks Log. | closed |
| T-26-11 | Information Disclosure | Binary contents | mitigate | `-ldflags="-s -w"` strips symbol table and DWARF debug info in both `Makefile` (line 165) and CI workflow. Reduces binary size ~25% and limits information leakage from binary inspection. | closed |

*Status: open · closed*
*Disposition: mitigate (implementation required) · accept (documented risk) · transfer (third-party)*

---

## Accepted Risks Log

| Risk ID | Threat Ref | Rationale | Accepted By | Date |
|---------|------------|-----------|-------------|------|
| AR-26-01 | T-26-05 | WinBox CLI requires credentials as argv — no alternative invocation method exists. Bridge runs on user's own machine. Attacker with local process access already has higher-privilege access. | gsd-security-auditor | 2026-04-09 |
| AR-26-02 | T-26-07 | Local single-user tool — rate limiting adds complexity without meaningful security benefit. Self-correcting: user closes excess WinBox windows. | gsd-security-auditor | 2026-04-09 |
| AR-26-03 | T-26-10 | Local Makefile build is equivalent to any local `go build` — developer machine is a trusted environment. | gsd-security-auditor | 2026-04-09 |

---

## Security Audit Trail

| Audit Date | Threats Total | Closed | Open | Run By |
|------------|---------------|--------|------|--------|
| 2026-04-09 | 11 | 11 | 0 | gsd-security-auditor |

---

## Sign-Off

- [x] All threats have a disposition (mitigate / accept / transfer)
- [x] Accepted risks documented in Accepted Risks Log
- [x] `threats_open: 0` confirmed
- [x] `status: verified` set in frontmatter

**Approval:** verified 2026-04-09
