# Phase 24: Backend API — Profiles, Assignments, WinBox Credentials - Discussion Log

> **Audit trail only.** Do not use as input to planning, research, or execution agents.
> Decisions captured in CONTEXT.md — this log preserves the discussion.

**Date:** 2026-04-07
**Phase:** 24-backend-api-profiles-assignments-winbox-credentials
**Mode:** discuss
**Areas discussed:** API path rename, device assignment endpoint design, WinBox flag/designation endpoint, bridge binary download infrastructure

## Areas Discussed

### API Path Rename
| Question | Answer |
|----------|--------|
| Should `/ssh-profiles` → `/credential-profiles` happen in Phase 24? | **Rename now** — Phase 25 rebuilds frontend anyway; clean break better now than never |

### Assignment Endpoint Design
| Question | Answer |
|----------|--------|
| How to structure per-device credential profile assignment endpoints? | **Nested under device** — `GET/POST/DELETE /api/v1/devices/{id}/credential-profiles[/{profileId}]` mirrors domain relationship |

### WinBox Flag + Designation
| Question | Answer |
|----------|--------|
| How to expose WinBox profile designation in the API? | **Dedicated PUT endpoint** — `PUT /devices/{id}/winbox-profile` with `{profile_id}` body; atomic, single concern |

### Bridge Binary Download
| Question | Answer |
|----------|--------|
| How to handle bridge binary download when Phase 26 builds the actual binary? | **Endpoint infra only** — build endpoint serving from `bridge_binaries_dir`; returns 404 until Phase 26 drops binaries there |

## Corrections Made

No corrections — all recommended options confirmed by user.

## Key Context from Codebase Analysis

- Phase 23 left `device_credential_profiles` without `is_winbox` — explicitly deferred to this phase (D-08 in Phase 23 context)
- `CredentialProfileRepo.IsInUse` currently checks legacy `ssh_profile_id` FK — Phase 24 updates it to use join table
- Existing instance backup handler is the reference pattern for streaming file downloads (bypass JSON middleware, Content-Disposition header)
- `internal/crypto/encrypt.go` `Decrypt` function is the right tool for the WinBox credentials endpoint
- 7 new routes maps exactly to the ROADMAP description: "7 new routes, per-device assignment management, WinBox credential endpoint, bridge download delivery"
