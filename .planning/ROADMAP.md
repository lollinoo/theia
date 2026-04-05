# Roadmap: MikroTik Theia

## Milestones

- ✅ **v1.3.0 Frontend Redesign** — Phases 1-7 (shipped 2026-03-27) — [archive](milestones/v1.3.0-ROADMAP.md)
- ✅ **v1.3.7 Virtual/Representative Nodes** — Phases 8-10 (shipped 2026-04-02) — [archive](milestones/v1.3.7-ROADMAP.md)
- ✅ **v1.3.8 CI/CD** — Phases 11-14 (shipped 2026-04-03) — [archive](milestones/v1.3.8-ROADMAP.md)

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

### 🚧 v1.4.0 Backup & Restore (In Progress)

**Milestone Goal:** Full disaster recovery — export, verify, and restore the entire Theia application state (database + device config files) from a single archive.

- [ ] **Phase 15: Backup Core** - Domain types, migration, repository, backup service (VACUUM INTO, tar.gz archive, integrity verification, manifest)
- [ ] **Phase 16: Backup API & Management UI** - HTTP handlers (create, list, download, delete), router registration, frontend backup panel
- [ ] **Phase 17: Restore Pipeline** - Archive validation, encryption key check, migration support, staging + restart-based restore, frontend upload
- [ ] **Phase 18: Backup Scheduler & Retention** - Background worker for scheduled backups, configurable interval, retention policy (keep last N), settings integration

## Phase Details

### Phase 15: Backup Core
**Goal**: `InstanceBackupService.Create()` produces a valid `.tar.gz` archive from live DB + device config files
**Depends on**: nothing (greenfield vertical slice)
**Plans:** 2 plans
Plans:
- [ ] 15-01-PLAN.md — Domain types, migration, settings constants, and repository CRUD implementation
- [ ] 15-02-PLAN.md — InstanceBackupService (Create, List, GetByID, Delete) and main.go wiring

**Success Criteria** (what must be TRUE):
  1. `InstanceBackup` domain type and `InstanceBackupRepository` interface defined
  2. `instance_backups` migration creates the metadata table
  3. `InstanceBackupRepo` implements CRUD for backup metadata
  4. `InstanceBackupService.Create()` produces a `.tar.gz` containing `manifest.json`, `theia.db` (via VACUUM INTO), and device config files
  5. `PRAGMA integrity_check` and SHA-256 checksums verify backup integrity
  6. Manifest includes app version, migration version, encryption key hash, and file checksums

### Phase 16: Backup API & Management UI
**Goal**: Users can create, list, download, and delete instance backups from the UI
**Depends on**: Phase 15
**Success Criteria** (what must be TRUE):
  1. `POST /api/v1/instance-backups` triggers on-demand backup creation
  2. `GET /api/v1/instance-backups` lists all backup metadata
  3. `GET /api/v1/instance-backups/{id}/download` streams `.tar.gz` archive
  4. `DELETE /api/v1/instance-backups/{id}` removes archive + DB record
  5. Frontend backup management panel with create, list, download, delete actions
  6. Router bypasses body size and JSON content-type middleware for download endpoint

### Phase 17: Restore Pipeline
**Goal**: Users can upload a backup archive and restore the full application state (with container restart)
**Depends on**: Phase 15, Phase 16
**Success Criteria** (what must be TRUE):
  1. `POST /api/v1/instance-backups/restore` accepts archive upload (bypasses 1MB limit)
  2. Pre-restore validation: archive integrity, manifest parsing, DB integrity check, encryption key match
  3. Cross-version migration on restored DB when migration version differs
  4. Restart-based restore: stage files, write marker, exit; `main.go` applies on restart
  5. Frontend upload UI with confirmation dialog showing manifest details
  6. Current DB backed up as `.pre-restore.bak` before swap

### Phase 18: Backup Scheduler & Retention
**Goal**: Automatic backups run on schedule with old archives cleaned up per retention policy
**Depends on**: Phase 15, Phase 16
**Success Criteria** (what must be TRUE):
  1. `BackupScheduler` worker follows same `Start/Stop` pattern as `Poller`
  2. Reads interval from settings each cycle (changes take effect without restart)
  3. Setting interval to 0 disables scheduling
  4. Retention policy deletes archives beyond configured count
  5. New settings: `instance_backup_interval_hours`, `instance_backup_retention_count`
  6. Frontend settings for schedule interval and retention count

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
| 15. Backup Core | v1.4.0 | 0/2 | In progress | — |
| 16. Backup API & Management UI | v1.4.0 | 0/? | Not started | — |
| 17. Restore Pipeline | v1.4.0 | 0/? | Not started | — |
| 18. Backup Scheduler & Retention | v1.4.0 | 0/? | Not started | — |
