# Roadmap: MikroTik Theia

## Milestones

- **v1.3.0 Frontend Redesign** -- Phases 1-7 (shipped 2026-03-27) -- [archive](milestones/v1.3.0-ROADMAP.md)
- **v1.3.7 Virtual/Representative Nodes** -- Phases 8-10 (shipped 2026-04-02) -- [archive](milestones/v1.3.7-ROADMAP.md)
- **v1.3.8 CI/CD** -- Phases 11-14 (shipped 2026-04-03) -- [archive](milestones/v1.3.8-ROADMAP.md)
- **v1.4.0 Backup & Restore** -- Phases 15-22 (shipped 2026-04-07) -- [archive](milestones/v1.4.0-ROADMAP.md)
- **v1.5.0 WinBox Integration** -- Phases 23-31 (shipped 2026-04-10) -- [archive](milestones/v1.5.0-ROADMAP.md)
- **v1.5.1 Production Stability** -- Phases 32-37 (shipped 2026-04-11) -- [archive](milestones/v1.5.1-ROADMAP.md)
- **v1.5.3 SNMP Pipeline Architecture** -- Phases 38-49 (shipped 2026-04-15) -- [archive](milestones/v1.5.3-ROADMAP.md)

## Phases

<details>
<summary>v1.3.0 Frontend Redesign (Phases 1-7) -- SHIPPED 2026-03-27</summary>

- [x] Phase 1: Design Token Foundation and Theme Infrastructure (3/3 plans) -- completed 2026-03-25
- [x] Phase 2: Component Restyling (6/6 plans) -- completed 2026-03-26
- [x] Phase 3: Area Backend and Management (2/2 plans) -- completed 2026-03-26
- [x] Phase 4: Area Hub View and Filtered Topology (4/4 plans) -- completed 2026-03-26
- [x] Phase 5: Redesign the Devices Page (3/3 plans) -- completed 2026-03-26
- [x] Phase 6: Canvas Token Migration and Theme Compliance (2/2 plans) -- completed 2026-03-27
- [x] Phase 7: SettingsPanel Verification and Phase 2 Closure (1/1 plan) -- completed 2026-03-27

</details>

<details>
<summary>v1.3.7 Virtual/Representative Nodes (Phases 8-10) -- SHIPPED 2026-04-02</summary>

- [x] Phase 8: Virtual Device Backend (2/2 plans) -- completed 2026-04-01
- [x] Phase 9: Virtual Node Rendering (2/2 plans) -- completed 2026-04-01
- [x] Phase 10: Virtual Node Forms (2/2 plans) -- completed 2026-04-01

</details>

<details>
<summary>v1.3.8 CI/CD (Phases 11-14) -- SHIPPED 2026-04-03</summary>

- [x] Phase 11: CI Pipeline (1/1 plan) -- completed 2026-04-03
- [x] Phase 12: Release Pipeline (2/2 plans) -- completed 2026-04-03
- [x] Phase 13: Deployment Stacks (2/2 plans) -- completed 2026-04-03
- [x] Phase 14: Fix Makefile Release Regression (1/1 plan) -- completed 2026-04-03

</details>

<details>
<summary>v1.4.0 Backup & Restore (Phases 15-22) -- SHIPPED 2026-04-07</summary>

- [x] Phase 15: Backup Core (2/2 plans) -- completed 2026-04-07
- [x] Phase 16: Backup API & Management UI (2/2 plans) -- completed 2026-04-07
- [x] Phase 17: Restore Pipeline (2/2 plans) -- completed 2026-04-07
- [x] Phase 18: Backup Scheduler & Retention (2/2 plans) -- completed 2026-04-07
- [x] Phase 19: Device Config Backup Scheduler (2/2 plans) -- completed 2026-04-07
- [x] Phase 20: Server-Side Validation & Threat Hardening (2/2 plans) -- completed 2026-04-07
- [x] Phase 21: Frontend Validation Parity (2/2 plans) -- completed 2026-04-07
- [x] Phase 22: Validation Integration & Closure (1/1 plan) -- completed 2026-04-07

</details>

<details>
<summary>v1.5.0 WinBox Integration (Phases 23-31) -- SHIPPED 2026-04-10</summary>

- [x] Phase 23: Credential Profile Schema + Domain (2/2 plans) -- completed 2026-04-07
- [x] Phase 24: Backend API -- Profiles, Assignments, WinBox Credentials (3/3 plans) -- completed 2026-04-07
- [x] Phase 25: Frontend -- Credential Profile Manager + WinBox Actions (3/3 plans) -- completed 2026-04-08
- [x] Phase 26: WinBox Bridge Binary (2/2 plans) -- completed 2026-04-08
- [x] Phase 27: Schema Cleanup -- Drop Legacy FK (2/2 plans) -- completed 2026-04-08
- [x] Phase 28: API Call Optimization -- WS Delta Payloads (2/2 plans) -- completed 2026-04-08
- [x] Phase 29: WinBox Bridge System Tray (3/3 plans) -- completed 2026-04-09
- [x] Phase 30: Gap Closure -- Verification Docs + Dead Code (1/1 plan) -- completed 2026-04-10
- [x] Phase 31: Dynamic Bridge Port (1/1 plan) -- completed 2026-04-10

</details>

<details>
<summary>v1.5.1 Production Stability (Phases 32-37) -- SHIPPED 2026-04-11</summary>

- [x] Phase 32: SNMP Discovery Fixes (1/1 plan) -- completed 2026-04-10
- [x] Phase 33: Real-time Canvas Reactivity (2/2 plans) -- completed 2026-04-10
- [x] Phase 34: Lazy Credential-Profiles Fetch (1/1 plan) -- completed 2026-04-10
- [x] Phase 35: Lazy Health Check for Bridge Server (1/1 plan) -- completed 2026-04-10
- [x] Phase 36: Remove Per-Interface Stats from /api/v1/devices (1/1 plan) -- completed 2026-04-10
- [x] Phase 37: Better LLDP Handling neighbors (1/1 plan) -- completed 2026-04-11

</details>

<details>
<summary>v1.5.3 SNMP Pipeline Architecture (Phases 38-49) -- SHIPPED 2026-04-15</summary>

- [x] Phase 38: State Engine (2/2 plans) -- completed 2026-04-12
- [x] Phase 39: Domain Types & DB Migration (4/4 plans) -- completed 2026-04-12
- [x] Phase 40: Collectors (4/4 plans) -- completed 2026-04-12
- [x] Phase 41: Jittered Scheduler (3/3 plans) -- completed 2026-04-12
- [x] Phase 42: Pipeline Orchestrator & Cutover (4/4 plans) -- completed 2026-04-13
- [x] Phase 43: WebSocket Detail-on-Demand (3/3 plans) -- completed 2026-04-13
- [x] Phase 44: Frontend Integration (4/4 plans) -- completed 2026-04-13
- [x] Phase 45: Polling Cadence Gap Closure (2/2 plans) -- completed 2026-04-13
- [x] Phase 46: Detail Delta Gap Closure (1/1 plan) -- completed 2026-04-14
- [x] Phase 47: Audit Traceability Cleanup (1/1 plan) -- completed 2026-04-15
- [x] Phase 48: Live Runtime Verification Closure (2/2 plans) -- completed 2026-04-14
- [x] Phase 49: Nyquist Validation Backfill (4/4 plans) -- completed 2026-04-15

</details>

No active milestone. Start the next one with `$gsd-new-milestone`.
