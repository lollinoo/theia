# Roadmap: MikroTik Theia

## Milestones

- ✅ **v1.3.0 Frontend Redesign** — Phases 1-7 (shipped 2026-03-27) — [archive](milestones/v1.3.0-ROADMAP.md)
- ✅ **v1.3.7 Virtual/Representative Nodes** — Phases 8-10 (shipped 2026-04-02) — [archive](milestones/v1.3.7-ROADMAP.md)
- ✅ **v1.3.8 CI/CD** — Phases 11-14 (shipped 2026-04-03) — [archive](milestones/v1.3.8-ROADMAP.md)
- ✅ **v1.4.0 Backup & Restore** — Phases 15-22 (shipped 2026-04-07) — [archive](milestones/v1.4.0-ROADMAP.md)

## Phases

<details>
<summary>✅ v1.3.0 Frontend Redesign (Phases 1-7) — SHIPPED 2026-03-27</summary>

- [x] Phase 1: Design Token Foundation and Theme Infrastructure (3/3 plans) — completed 2026-03-25
- [x] Phase 2: Component Restyling (6/6 plans) — completed 2026-03-26
- [x] Phase 3: Area Backend and Management (2/2 plans) — completed 2026-03-26
- [x] Phase 4: Area Hub View and Filtered Topology (4/4 plans) — completed 2026-03-26
- [x] Phase 5: Redesign the Devices Page (3/3 plans) — completed 2026-03-26
- [x] Phase 6: Canvas Token Migration and Theme Compliance (2/2 plans) — completed 2026-03-27
- [x] Phase 7: SettingsPanel Verification and Phase 2 Closure (1/1 plan) — completed 2026-03-27

</details>

<details>
<summary>✅ v1.3.7 Virtual/Representative Nodes (Phases 8-10) — SHIPPED 2026-04-02</summary>

- [x] Phase 8: Virtual Device Backend (2/2 plans) — completed 2026-04-01
- [x] Phase 9: Virtual Node Rendering (2/2 plans) — completed 2026-04-01
- [x] Phase 10: Virtual Node Forms (2/2 plans) — completed 2026-04-01

</details>

<details>
<summary>✅ v1.3.8 CI/CD (Phases 11-14) — SHIPPED 2026-04-03</summary>

- [x] Phase 11: CI Pipeline (1/1 plan) — completed 2026-04-03
- [x] Phase 12: Release Pipeline (2/2 plans) — completed 2026-04-03
- [x] Phase 13: Deployment Stacks (2/2 plans) — completed 2026-04-03
- [x] Phase 14: Fix Makefile Release Regression (1/1 plan) — completed 2026-04-03

</details>

<details>
<summary>✅ v1.4.0 Backup & Restore (Phases 15-22) — SHIPPED 2026-04-07</summary>

- [x] Phase 15: Backup Core (2/2 plans) — completed 2026-04-07
- [x] Phase 16: Backup API & Management UI (2/2 plans) — completed 2026-04-07
- [x] Phase 17: Restore Pipeline (2/2 plans) — completed 2026-04-07
- [x] Phase 18: Backup Scheduler & Retention (2/2 plans) — completed 2026-04-07
- [x] Phase 19: Device Config Backup Scheduler (2/2 plans) — completed 2026-04-07
- [x] Phase 20: Server-Side Validation & Threat Hardening (2/2 plans) — completed 2026-04-07
- [x] Phase 21: Frontend Validation Parity (2/2 plans) — completed 2026-04-07
- [x] Phase 22: Validation Integration & Closure (1/1 plan) — completed 2026-04-07

</details>

### v1.5.0 WinBox Integration (In Progress)

**Milestone Goal:** One-click WinBox launch from the topology map, backed by a role-aware multi-profile credential system.

- [x] **Phase 23: Credential Profile Schema + Domain** - Join table, role column, BackupService update, data migration preserving encrypted credentials (completed 2026-04-07)
- [ ] **Phase 24: Backend API — Profiles, Assignments, WinBox Credentials** - 7 new routes, per-device assignment management, WinBox credential endpoint, bridge download delivery
- [ ] **Phase 25: Frontend — Credential Profile Manager + WinBox Actions** - Profile manager UI, per-device assignment, role field, WinBox actions in canvas and table, bridge health check
- [ ] **Phase 26: WinBox Bridge Binary** - CGO-free Go binary for 6 targets, CORS+Host dual-validation, hardcoded WinBox-only execution
- [ ] **Phase 27: Schema Cleanup — Drop Legacy FK** - SQLite 12-step table-recreation migration dropping legacy ssh_profile_id FK column

## Phase Details

### Phase 23: Credential Profile Schema + Domain
**Goal**: The data model supports multiple credential profiles per device with custom role labels, and existing SSH profiles are automatically migrated to "Admin"
**Depends on**: Phase 22 (v1.4.0 complete)
**Requirements**: CRED-01, CRED-02, CRED-04
**Success Criteria** (what must be TRUE):
  1. A credential profile record has a `role` text field that accepts any user-defined string (e.g. "Admin", "Read-only")
  2. The `device_credential_profiles` join table allows multiple profiles per device with no duplicates
  3. On upgrade, every existing SSH profile gains a `role` of "Admin" automatically — zero data loss, `encrypted_secret` preserved
  4. `BackupService` resolves credential profiles via the new join table and continues to perform successful backups without regression
  5. The `json:"-"` annotation on `encrypted_secret` ensures credentials are never exposed in API responses
**Plans:** 2/2 plans complete
Plans:
- [x] 23-01-PLAN.md — Migration 000012 + CredentialProfile domain type
- [x] 23-02-PLAN.md — Repository, service, handler, router rename + test updates

### Phase 24: Backend API — Profiles, Assignments, WinBox Credentials
**Goal**: The backend exposes full CRUD for credential profiles and per-device assignments, plus a WinBox credential endpoint and bridge binary download
**Depends on**: Phase 23
**Requirements**: CRED-03, CRED-05, BRIDGE-01, BRIDGE-02
**Success Criteria** (what must be TRUE):
  1. User can create, read, update, and delete credential profiles via REST API
  2. User can list all credential profiles assigned to a specific device
  3. User can designate exactly one profile per device as the WinBox profile via API
  4. A dedicated endpoint returns the WinBox credential (IP + decrypted username/password) for a device — only when a WinBox profile is designated
  5. Bridge binaries for all 6 targets (Windows/Linux/macOS × amd64/arm64) are downloadable from Theia Settings via the API
**Plans:** 3 plans
Plans:
- [ ] 24-01-PLAN.md — Migration 000013 + repo methods + route rename + config field
- [ ] 24-02-PLAN.md — Device assignment handler + WinBox endpoints + router wiring
- [ ] 24-03-PLAN.md — Bridge binary download handler

### Phase 25: Frontend — Credential Profile Manager + WinBox Actions
**Goal**: Users can manage credential profiles and launch WinBox from the topology map and device table
**Depends on**: Phase 24
**Requirements**: WINBOX-01, WINBOX-02, WINBOX-03, BRIDGE-05
**Success Criteria** (what must be TRUE):
  1. User can open WinBox pre-authenticated from the canvas device context menu
  2. User can open WinBox pre-authenticated from the Devices table row action
  3. WinBox action is visually disabled with an explanatory tooltip when no WinBox profile is designated for the device
  4. Frontend detects whether the bridge is running via a health check endpoint and reflects bridge status to the user
  5. User can view, create, edit, delete, and assign credential profiles to a device from within Theia UI
**Plans**: TBD
**UI hint**: yes

### Phase 26: WinBox Bridge Binary
**Goal**: A locally-runnable Go binary accepts WinBox launch requests from Theia and opens WinBox pre-authenticated, with security validated from day one
**Depends on**: Phase 23
**Requirements**: BRIDGE-03, BRIDGE-04
**Success Criteria** (what must be TRUE):
  1. Bridge rejects any request whose `Origin` header does not match the configured Theia origin
  2. Bridge rejects any request whose `Host` header is not `localhost:1337`, preventing DNS rebinding attacks
  3. Bridge is hardcoded to launch only the WinBox executable — passing arbitrary executable paths is rejected
  4. Bridge compiles without CGO and produces working binaries for all 6 targets (Windows amd64/arm64, Linux amd64/arm64, macOS amd64/arm64)
**Plans**: TBD

### Phase 27: Schema Cleanup — Drop Legacy FK
**Goal**: The `devices` table no longer carries the legacy `ssh_profile_id` FK column — schema is clean post-migration
**Depends on**: Phase 25, Phase 26
**Requirements**: WINBOX-04
**Success Criteria** (what must be TRUE):
  1. `ssh_profile_id` column is absent from the `devices` table after upgrade
  2. SQLite 12-step table-recreation migration executes without data loss on a populated database
  3. All existing device records survive the migration with all other fields intact
  4. Application starts and operates normally with no references to the dropped column
**Plans**: TBD

## Progress

| Phase | Milestone | Plans Complete | Status | Completed |
|-------|-----------|----------------|--------|-----------|
| 1. Design Token Foundation | v1.3.0 | 3/3 | Complete | 2026-03-25 |
| 2. Component Restyling | v1.3.0 | 6/6 | Complete | 2026-03-26 |
| 3. Area Backend and Management | v1.3.0 | 2/2 | Complete | 2026-03-26 |
| 4. Area Hub View and Filtered Topology | v1.3.0 | 4/4 | Complete | 2026-03-26 |
| 5. Redesign the Devices Page | v1.3.0 | 3/3 | Complete | 2026-03-26 |
| 6. Canvas Token Migration | v1.3.0 | 2/2 | Complete | 2026-03-27 |
| 7. SettingsPanel Verification | v1.3.0 | 1/1 | Complete | 2026-03-27 |
| 8. Virtual Device Backend | v1.3.7 | 2/2 | Complete | 2026-04-01 |
| 9. Virtual Node Rendering | v1.3.7 | 2/2 | Complete | 2026-04-01 |
| 10. Virtual Node Forms | v1.3.7 | 2/2 | Complete | 2026-04-01 |
| 11. CI Pipeline | v1.3.8 | 1/1 | Complete | 2026-04-03 |
| 12. Release Pipeline | v1.3.8 | 2/2 | Complete | 2026-04-03 |
| 13. Deployment Stacks | v1.3.8 | 2/2 | Complete | 2026-04-03 |
| 14. Fix Makefile Release Regression | v1.3.8 | 1/1 | Complete | 2026-04-03 |
| 15. Backup Core | v1.4.0 | 2/2 | Complete | 2026-04-07 |
| 16. Backup API & Management UI | v1.4.0 | 2/2 | Complete | 2026-04-07 |
| 17. Restore Pipeline | v1.4.0 | 2/2 | Complete | 2026-04-07 |
| 18. Backup Scheduler & Retention | v1.4.0 | 2/2 | Complete | 2026-04-07 |
| 19. Device Config Backup Scheduler | v1.4.0 | 2/2 | Complete | 2026-04-07 |
| 20. Server-Side Validation & Threat Hardening | v1.4.0 | 2/2 | Complete | 2026-04-07 |
| 21. Frontend Validation Parity | v1.4.0 | 2/2 | Complete | 2026-04-07 |
| 22. Validation Integration & Closure | v1.4.0 | 1/1 | Complete | 2026-04-07 |
| 23. Credential Profile Schema + Domain | v1.5.0 | 2/2 | Complete   | 2026-04-07 |
| 24. Backend API — Profiles, Assignments, WinBox Credentials | v1.5.0 | 0/3 | Not started | — |
| 25. Frontend — Credential Profile Manager + WinBox Actions | v1.5.0 | 0/? | Not started | — |
| 26. WinBox Bridge Binary | v1.5.0 | 0/? | Not started | — |
| 27. Schema Cleanup — Drop Legacy FK | v1.5.0 | 0/? | Not started | — |
