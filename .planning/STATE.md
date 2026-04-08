---
gsd_state_version: 1.0
milestone: v1.5.0
milestone_name: WinBox Integration
status: executing
stopped_at: Completed 25-frontend-credential-profile-manager-winbox-actions-25-02-PLAN.md
last_updated: "2026-04-08T10:44:33.534Z"
last_activity: 2026-04-08
progress:
  total_phases: 5
  completed_phases: 2
  total_plans: 8
  completed_plans: 7
  percent: 88
---

# State: MikroTik Theia

## Project Reference

See: .planning/PROJECT.md (updated 2026-04-07)

**Core value:** Network operators can see their entire topology at a glance with live stats on every device and link, drill into Grafana for deep dives, and manage devices directly — all from a single interactive map.
**Current focus:** Phase 24 — backend-api-profiles-assignments-winbox-credentials

## Current Position

Phase: 24 (backend-api-profiles-assignments-winbox-credentials) — EXECUTING
Plan: 3 of 3
Status: Ready to execute
Last activity: 2026-04-08

```
v1.5.0 Progress: [█████░░░░░] 50% plans (Phase 23 in progress)
```

## Performance Metrics

- Phases complete (v1.5.0): 0/5
- Plans complete (v1.5.0): 1/2 (Phase 23)
- Requirements mapped: 14/14
- 23-01: 2 tasks, 4 files, 2 min

## Decisions

- [23-01] Created credential_profile.go (new file) + deleted ssh_profile.go — cleaner git history than in-place rename
- [23-01] devices.ssh_profile_id FK column NOT dropped — deferred to Phase 27 per D-06
- [23-01] Role field is free-text string, not enum — enables custom labels per CRED-01
- [23-01] SSHAuthMethod type stays in backup.go unmodified per D-04
- [Phase 25-01]: SSHProfile renamed to CredentialProfile with role field; 10 API client functions now hit /credential-profiles endpoints
- [Phase 25-frontend-credential-profile-manager-winbox-actions]: Credentials section replaces ssh_profile_id select dropdown — assignment lifecycle managed via separate API calls

## Accumulated Context

- Phase numbering continues from v1.4.0; last phase was 22, v1.5.0 starts at Phase 23
- ssh_profiles table renamed to credential_profiles (migration 000012 applied on startup)
- credential_profiles has role column (DEFAULT 'Admin') — all existing SSH profiles get 'Admin' role
- device_credential_profiles join table created and seeded from ssh_profile_id FK values
- CredentialProfile domain type (credential_profile.go) replaces SSHProfile — has Role string field
- EncryptedSecret has json:"-" tag — never exposed in API responses (T-23-01 mitigated)
- BackupService and other files still reference domain.SSHProfile — will be updated in 23-02
- WinBox CLI arg format: `winbox <address> <username> <password>`
- Bridge port: localhost:1337 (provisional — confirm during planning)
- Bridge binary: CGO-free, 6 targets (Windows/Linux/macOS x amd64/arm64)
- Bridge security: validate both Origin AND Host headers from day one (DNS rebinding protection)
- Bridge is hardcoded to WinBox only — no arbitrary process execution
- device_credential_profiles join table replaces direct ssh_profile_id FK on devices
- Legacy ssh_profile_id FK column dropped in Phase 27 (last) — SQLite 12-step table-recreation

## Session Continuity

Stopped at: Completed 25-frontend-credential-profile-manager-winbox-actions-25-02-PLAN.md
To resume: execute 23-02-PLAN.md (credential_profile_repo.go rename + BackupService + handler renames)
