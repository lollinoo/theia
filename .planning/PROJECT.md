# MikroTik Theia

## What This Is

A modern, interactive network topology visualizer that provides a bird's-eye view of network infrastructure. Built as a React + Go web application integrating with Prometheus/Grafana for real-time stats on routers, switches, and their links — displayed on a drag-and-drop canvas. Think of it as a modern replacement for MikroTik's The Dude, with full infrastructure management capabilities including one-click WinBox access and SSH-based config backups.

## Core Value

Network operators can see their entire topology at a glance with live stats on every device and link, drill into Grafana for deep dives, and manage devices directly — all from a single interactive map.

## Current State

Shipped `v1.5.3 SNMP Pipeline Architecture` on 2026-04-15. Theia now runs on a pipeline-based SNMP runtime with a state engine, volatility-tiered collectors, jittered scheduling, backend-owned health/freshness, targeted detail subscriptions, one-click WinBox launch, and backup/restore workflows.

- Live device cards now render backend-computed health, freshness, and effective polling cadence.
- Operators can override per-device performance polling cadence, and selected-device interface panels refresh through targeted websocket detail deltas.
- v1.5.3 closed cleanly: 12 phases, 34 plans, 63 tasks, 19/19 requirements satisfied, 9/9 Nyquist coverage, audit passed.

## Next Milestone Goals

- Define the next operator-facing scope with `$gsd-new-milestone` and create a fresh `.planning/REQUIREMENTS.md`.
- Decide whether to productize deferred configurability for thresholds and non-performance cadences.
- Build on the shipped pipeline architecture rather than reopening the old poll-and-dump model.

<details>
<summary>Shipped Milestone Details: v1.5.3 SNMP Pipeline Architecture</summary>

**Goal:** Rearchitect the SNMP polling and metrics pipeline from a single-cadence poll-and-dump model to a tiered, state-engine-driven architecture that decouples data collection from frontend delivery.

**Highlights:**
- State engine with backend-owned health, hysteresis, and diff-based change emission
- Static/operational/performance collector split with per-device poll classification and override support
- Jittered scheduler plus PipelineOrchestrator cutover as the sole production polling path
- Frontend health/freshness/cadence UI and immediate override re-due behavior
- Targeted detail-on-demand websocket updates that include selected-device link metrics
- Finalized live-runtime/UAT evidence, 9/9 Nyquist validation coverage, and 19/19 traceability cleanup
</details>

## Previous Milestones

**v1.5.1 Production Stability** — Shipped 2026-04-11. Post-v1.5.0 stability fixes: lazy credential fetches, deferred bridge health, compact payloads, LLDP improvements.

**v1.5.0 WinBox Integration** — Shipped 2026-04-10. One-click WinBox launch from topology map, credential profiles, bridge binary, system tray.

## Requirements

### Validated

<!-- Shipped and confirmed valuable. -->

- ✓ Design token system (Neon Topography), dark/light themes, FOWT prevention — v1.3.0/Phase 1
- ✓ Component restyling (DeviceCard, ContextMenu, NavBar, AlertsPanel, SidePanel) — v1.3.0/Phase 2
- ✓ Area backend: SQLite persistence, REST CRUD, device-to-area assignment, cascading deletes — v1.3.0/Phase 3
- ✓ Area Hub view with aggregate stats, per-area cards, bloom effects, atmospheric watermark — v1.3.0/Phase 4
- ✓ Redesigned Devices page with FilterSelect, sortable table, icon actions — v1.3.0/Phase 5
- ✓ Canvas token migration and theme compliance — v1.3.0/Phase 6-7
- ✓ Virtual device type as first-class entity with subtypes, partial unique IP index — v1.3.7/Phase 8
- ✓ Virtual node rendering (compact cards, dashed borders, subtype icons) — v1.3.7/Phase 9
- ✓ Virtual node forms (AddDevicePanel dual-mode, LinkCreatePanel virtual-aware) — v1.3.7/Phase 10
- ✓ CI pipeline (GitHub Actions), release pipeline (GHCR images, semver tags) — v1.3.8/Phase 11-12
- ✓ Deployment stacks (staging + production with Watchtower, .env.prod.example) — v1.3.8/Phase 13-14
- ✓ Instance backup pipeline (VACUUM INTO, tar.gz, manifest, SHA-256, integrity check) — v1.4.0/Phase 15
- ✓ Backup API + management UI (create, list, download, delete from SettingsPanel) — v1.4.0/Phase 16
- ✓ Restore pipeline (dry-run, cross-version migration, restart-based swap, .pre-restore.bak) — v1.4.0/Phase 17
- ✓ Scheduled instance + device backups with configurable intervals and retention — v1.4.0/Phase 18-19
- ✓ Server-side input validation + threat hardening (correlation IDs, settings allowlist) — v1.4.0/Phase 20
- ✓ Frontend validation parity (shared validation.ts, typed ValidationError/ServerError) — v1.4.0/Phase 21-22
- ✓ Credential profiles with custom role labels and many-to-many device assignment — v1.5.0/Phase 23
- ✓ Per-device WinBox profile designation and Settings-driven bridge downloads — v1.5.0/Phases 24-25
- ✓ Origin + Host dual-validation, WinBox-only bridge execution, and system-tray/headless bridge lifecycle — v1.5.0/Phases 26, 29, 31
- ✓ WebSocket delta payloads and schema cleanup around credential migration — v1.5.0/Phases 27-28
- ✓ SNMP discovery/LLDP stability fixes, lazy credential loading, deferred bridge health checks, and compact device payloads — v1.5.1/Phases 32-37
- ✓ State engine with backend-owned health/reachability/staleness and diff-suppressed change emission — v1.5.3/Phase 38
- ✓ OID volatility tiers, per-device poll classification, and persisted poll overrides — v1.5.3/Phase 39
- ✓ Stateless static/operational/performance collectors with SNMP-primary metrics and safe counter-rate handling — v1.5.3/Phase 40
- ✓ Jittered per-device scheduler with concurrency limits and separated discovery vs metrics cadences — v1.5.3/Phase 41
- ✓ Pipeline cutover: `PipelineOrchestrator` now replaces `Poller` + `MetricsCollector` while preserving the existing overview payload contract — v1.5.3/Phase 42
- ✓ Detail-on-demand websocket subscriptions and targeted selected-device link-metric delivery — v1.5.3/Phases 43 & 46
- ✓ Frontend health/freshness/cadence UI plus immediate override re-due behavior — v1.5.3/Phases 44-45
- ✓ Finalized live-runtime/UAT proof, 9/9 Nyquist validation coverage, and 19/19 milestone traceability closure — v1.5.3/Phases 47-49

### Active

- None yet — define the next milestone via `$gsd-new-milestone`.

### Out of Scope

<!-- Explicit boundaries. Includes reasoning to prevent re-adding. -->

- Role-based access control in Theia itself — credential roles describe device access levels, not Theia user permissions
- WinBox fallback / auto-detection of "best" profile — user must explicitly designate the WinBox profile
- SSH terminal / interactive SSH in the browser — out of scope
- Support for applications other than WinBox — WinBox-only bridge; no arbitrary process execution
- Bridge auto-update / auto-start on OS boot — distribution is manual download; tray handles start/stop

## Context

- Tech stack: React 18 + TypeScript frontend, Go 1.24 backend, SQLite (CGO), Vite, Tailwind v3 Neon Topography design system
- Current shipped surface: topology canvas, WinBox bridge, backup/restore, and pipeline-driven SNMP polling with detail-on-demand websocket delivery
- Runtime architecture: state engine + volatility-tiered collectors + jittered scheduler + PipelineOrchestrator
- Bridge binary: CGO-free, cross-compiled for 6 targets, distributed as GitHub Release assets
- Credentials at rest encrypted with AES-256-GCM (`THEIA_ENCRYPTION_KEY`); never exposed in API responses (`json:"-"`)
- WinBox credentials = SSH credentials on MikroTik (same username/password for SSH and WinBox)
- Local bridge runs entirely on the user's machine; no credentials touch a remote server

## Constraints

- **Data source**: Must integrate with existing Prometheus instance — no duplicate metric collection
- **Scale**: Must handle 100+ devices without performance degradation on the canvas
- **Tech stack**: React frontend, Go backend — chosen for performance and ecosystem fit
- **SNMP compatibility**: Must work with any device exposing standard SNMP MIBs
- **Real-time**: Configurable polling intervals, not just static snapshots
- **Security**: Bridge validates CORS Origin + Host headers; WinBox-only execution hardcoded
- **No server-side credentials for WinBox**: IP + credentials sent only to localhost bridge, never to backend

## Key Decisions

| Decision | Rationale | Outcome |
|----------|-----------|---------|
| Tailwind v4 + @theme inline tokens | Design system that doesn't fight ReactFlow; custom tokens scale | ✓ Good |
| ReactFlow v12 (@xyflow/react) | v11→v12 for colorMode integration and API improvements | ✓ Good |
| SQLite with CGO (mattn/go-sqlite3) | Simplicity for single-node deploy, VACUUM INTO for backups | ✓ Good |
| AES-256-GCM for credentials at rest | THEIA_ENCRYPTION_KEY approach; simple, auditable | ✓ Good |
| Restart-based restore (stage + marker + os.Exit) | Avoids live DB swap complexity; safe and auditable | ✓ Good |
| Local Go Bridge on localhost (configurable port) | No OS-level protocol handlers, no installers, zero config | ✓ Good — system tray, config persistence, headless mode shipped |
| Join table (device_credential_profiles) over FK | Many-to-many credential assignment without breaking backups | ✓ Good |
| Free-text role labels (not enum) | Forward-compatible; user defines "Admin", "Backup", custom labels | ✓ Good |
| Atomic WinBox designation (clear-then-set transaction) | Exactly one WinBox profile per device at all times | ✓ Good |
| FNV-64a delta hashing for WebSocket | Fast, non-cryptographic; benign collisions only cause an extra send | ✓ Good |
| CGO-free bridge binary | Cross-compile all 6 targets in CI without native runners | ✓ Good (macOS systray needs CGO native runner — accepted) |
| Dynamic bridge port from settings | Eliminates hardcoded :1337; operators configure per environment | ✓ Good |
| Volatility-tiered polling split | Static, operational, and performance work need different cadences and payload semantics | ✓ Good — v1.5.3 shipped |
| Device-type-derived poll classification with per-device override | Sensible defaults plus operator control without a heavyweight scheduling UI | ✓ Good — v1.5.3 shipped |
| State engine: `sync.RWMutex` over channel-based actor | Atomic snapshot reads are a natural fit for RLock; research consensus | ✓ Good — Phase 38 shipped with zero data races under `-race` |
| Hysteresis thresholds hardcoded (70/60/90/80) | Ship fast; THRESH-01/02 configurability deferred | ✓ Good — flap-prevention proven via oscillating-69/71 test |
| Lock-and-emit-outside-lock invariant | Prevents lock-during-send deadlocks; consumers recover via `Snapshot()` | ✓ Good — validated under concurrent burst tests |
| PipelineOrchestrator atomic cutover | Replaces Poller + MetricsCollector in one runtime swap while keeping WS snapshot/delta semantics stable | ✓ Good — Phase 42 verified in code and approved after live smoke checks |

## Evolution

This document evolves at phase transitions and milestone boundaries.

**After each phase transition** (via `/gsd-transition`):
1. Requirements invalidated? → Move to Out of Scope with reason
2. Requirements validated? → Move to Validated with phase reference
3. New requirements emerged? → Add to Active
4. Decisions to log? → Add to Key Decisions
5. "What This Is" still accurate? → Update if drifted

**After each milestone** (via `/gsd-complete-milestone`):
1. Full review of all sections
2. Core Value check — still the right priority?
3. Audit Out of Scope — reasons still valid?
4. Update Context with current state

---
*Last updated: 2026-04-15 after v1.5.3 milestone completion*
