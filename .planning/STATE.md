---
gsd_state_version: 1.0
milestone: v1.5.0
milestone_name: WinBox Integration
status: verifying
stopped_at: Completed 27-02-PLAN.md
last_updated: "2026-04-08T13:11:10.711Z"
last_activity: 2026-04-08
progress:
  total_phases: 5
  completed_phases: 5
  total_plans: 12
  completed_plans: 12
  percent: 100
---

# State: MikroTik Theia

## Project Reference

See: .planning/PROJECT.md (updated 2026-04-07)

**Core value:** Network operators can see their entire topology at a glance with live stats on every device and link, drill into Grafana for deep dives, and manage devices directly — all from a single interactive map.
**Current focus:** Phase 27 — schema-cleanup-drop-legacy-fk

## Current Position

Phase: 27 (schema-cleanup-drop-legacy-fk) — EXECUTING
Plan: 2 of 2
Status: Phase complete — ready for verification
Last activity: 2026-04-08

```
v1.5.0 Progress: [████████░░] 80% phases (4/5 complete)
```

## Performance Metrics

- Phases complete (v1.5.0): 4/5
- Plans complete (v1.5.0): 10/10
- Requirements mapped: 14/14

## Decisions

- [23-01] Created credential_profile.go (new file) + deleted ssh_profile.go — cleaner git history than in-place rename
- [23-01] devices.ssh_profile_id FK column NOT dropped — deferred to Phase 27 per D-06
- [23-01] Role field is free-text string, not enum — enables custom labels per CRED-01
- [23-01] SSHAuthMethod type stays in backup.go unmodified per D-04
- [Phase 25-01]: SSHProfile renamed to CredentialProfile with role field; 10 API client functions now hit /credential-profiles endpoints
- [Phase 25-frontend-credential-profile-manager-winbox-actions]: Credentials section replaces ssh_profile_id select dropdown — assignment lifecycle managed via separate API calls
- [Phase 25]: waitFor + fake timers causes timeout in Vitest — replaced with act + advanceTimersByTimeAsync for useBridgeHealth tests
- [Phase 25]: deviceWinboxMap uses lazy fetch per-device on first render/menu-open rather than upfront batch
- [Phase 26-01]: CORS preflight handled in securityCheck middleware — Origin+Host validation and CORS headers co-located
- [Phase 26-01]: startProcess injectable var pattern for WinBox process testability without OS-level mocking
- [Phase 26-winbox-bridge-binary]: Matrix strategy (6 parallel jobs) over single loop in CI for build-bridge — softprops/action-gh-release@v2 pinned major version uses GITHUB_TOKEN automatically
- [Phase 27]: Migration 000014 uses SQLite 12-step recreation with PRAGMA foreign_keys=off/on; GetBackupProfileForDevice resolves credentials via join table (ORDER BY is_winbox ASC)
- [Phase 27-02]: SSHCredentialForm uses assignCredentialProfile/unassignCredentialProfile instead of updateDevice(ssh_profile_id) — T-27-07 mitigation
- [Phase 27-02]: Dashboard currentProfileId for SSHCredentialForm fetched via fetchDeviceCredentialProfiles on panel open (Option A — live source of truth after ssh_profile_id removal)
- [Phase 27-02]: BulkEditPanel SSH Profile section removed — bulk credential assignment not supported after ssh_profile_id removal

## Accumulated Context

- Phase numbering continues from v1.4.0; last phase was 22, v1.5.0 starts at Phase 23
- ssh_profiles table renamed to credential_profiles (migration 000012 applied on startup)
- credential_profiles has role column (DEFAULT 'Admin') — all existing SSH profiles get 'Admin' role
- device_credential_profiles join table created and seeded from ssh_profile_id FK values
- CredentialProfile domain type (credential_profile.go) replaces SSHProfile — has Role string field
- EncryptedSecret has json:"-" tag — never exposed in API responses (T-23-01 mitigated)
- BackupService and other files still reference domain.SSHProfile — will be updated in 23-02
- WinBox CLI arg format: `winbox <address> <username> <password>`
- Bridge port: localhost:1337 (confirmed)
- Bridge binary: CGO-free, 6 targets (Windows/Linux/macOS x amd64/arm64) — BUILT AND VERIFIED
- Bridge security: validate both Origin AND Host headers from day one (DNS rebinding protection)
- Bridge is hardcoded to WinBox only — no arbitrary process execution
- device_credential_profiles join table replaces direct ssh_profile_id FK on devices
- Legacy ssh_profile_id FK column dropped in Phase 27 (last) — SQLite 12-step table-recreation

## Session Continuity

Stopped at: Completed 27-02-PLAN.md
To resume: /gsd-execute-phase 27 (Schema Cleanup — Drop Legacy FK)
