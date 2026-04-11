# Roadmap: MikroTik Theia

## Milestones

- **v1.3.0 Frontend Redesign** -- Phases 1-7 (shipped 2026-03-27) -- [archive](milestones/v1.3.0-ROADMAP.md)
- **v1.3.7 Virtual/Representative Nodes** -- Phases 8-10 (shipped 2026-04-02) -- [archive](milestones/v1.3.7-ROADMAP.md)
- **v1.3.8 CI/CD** -- Phases 11-14 (shipped 2026-04-03) -- [archive](milestones/v1.3.8-ROADMAP.md)
- **v1.4.0 Backup & Restore** -- Phases 15-22 (shipped 2026-04-07) -- [archive](milestones/v1.4.0-ROADMAP.md)
- **v1.5.0 WinBox Integration** -- Phases 23-31 (shipped 2026-04-10) -- [archive](milestones/v1.5.0-ROADMAP.md)
- **v1.5.1 Production Stability** -- Phases 32-37 (shipped 2026-04-11) -- [archive](milestones/v1.5.1-ROADMAP.md)

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

- [x] **Phase 32: SNMP Discovery Fixes** -- Correct source_port population and guard stats fetch when source_port is absent (completed 2026-04-10)
- [x] **Phase 33: Real-time Canvas Reactivity** -- Canvas updates hostname, model, and LLDP links live after async probe completes (completed 2026-04-10)
- [x] **Phase 34: Lazy Credential-Profiles Fetch** -- Eliminate per-device GET /credential-profiles on every canvas load (completed 2026-04-10)
- [x] **Phase 35: Lazy Health Check for Bridge Server** -- Defer bridge health poll from Canvas mount to first context menu open (completed 2026-04-10)
- [x] **Phase 36: Remove Per-Interface Stats from /api/v1/devices** -- Remove interface relationships from the devices payload and fetch interfaces lazily on demand (completed 2026-04-10)
- [x] **Phase 37: Better LLDP Handling neighbors** -- Follow-up LLDP neighbor handling improvements, including normalized neighbor matching and parallel uplink correctness (completed 2026-04-11)

</details>

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
| 23. Credential Profile Schema + Domain | v1.5.0 | 2/2 | Complete | 2026-04-07 |
| 24. Backend API -- Profiles, Assignments, WinBox Credentials | v1.5.0 | 3/3 | Complete | 2026-04-07 |
| 25. Frontend -- Credential Profile Manager + WinBox Actions | v1.5.0 | 3/3 | Complete | 2026-04-08 |
| 26. WinBox Bridge Binary | v1.5.0 | 2/2 | Complete | 2026-04-08 |
| 27. Schema Cleanup -- Drop Legacy FK | v1.5.0 | 2/2 | Complete | 2026-04-08 |
| 28. API Call Optimization -- WS Delta Payloads | v1.5.0 | 2/2 | Complete | 2026-04-08 |
| 29. WinBox Bridge System Tray | v1.5.0 | 3/3 | Complete | 2026-04-09 |
| 30. Gap Closure -- Verification Docs + Dead Code | v1.5.0 | 1/1 | Complete | 2026-04-10 |
| 31. Dynamic Bridge Port | v1.5.0 | 1/1 | Complete | 2026-04-10 |
| 32. SNMP Discovery Fixes | v1.5.1 | 1/1 | Complete   | 2026-04-10 |
| 33. Real-time Canvas Reactivity | v1.5.1 | 2/2 | Complete   | 2026-04-10 |
| 34. Lazy Credential-Profiles Fetch | v1.5.1 | 1/1 | Complete   | 2026-04-10 |
| 35. Lazy Health Check for Bridge Server | v1.5.1 | 1/1 | Complete   | 2026-04-10 |
| 36. Remove Per-Interface Stats from /api/v1/devices | v1.5.1 | 1/1 | Complete   | 2026-04-10 |
| 37. Better LLDP Handling neighbors | v1.5.1 | 1/1 | Complete   | 2026-04-11 |
