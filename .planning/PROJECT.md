# MikroTik Theia

## What This Is

A modern, interactive network topology visualizer that provides a bird's-eye view of network infrastructure. Built as a React + Go web application integrating with Prometheus/Grafana for real-time stats on routers, switches, and their links — displayed on a drag-and-drop canvas. Think of it as a modern replacement for MikroTik's The Dude, with full infrastructure management capabilities including one-click WinBox access and SSH-based config backups.

## Core Value

Network operators can see their entire topology at a glance with live stats on every device and link, drill into Grafana for deep dives, and manage devices directly — all from a single interactive map.

## Current State: v1.5.0 WinBox Integration — SHIPPED 2026-04-10

**Shipped:** One-click WinBox launch from the topology map, backed by a role-aware multi-profile credential system. A standalone cross-platform bridge binary handles launches with DNS-rebinding protection and persists config via system tray.

**Next milestone:** TBD — run `/gsd-new-milestone` to define v1.6.0

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
- ✓ Credential profiles with custom role labels (free-text: Admin, Backup, Read-only, etc.) — v1.5.0/Phase 23
- ✓ Multiple credential profiles per device via join table; one-to-many, no duplicates — v1.5.0/Phase 23
- ✓ Existing SSH profiles auto-migrated to role "Admin" on upgrade — v1.5.0/Phase 23
- ✓ Per-device WinBox profile designation (exactly one, atomically set) — v1.5.0/Phase 24
- ✓ Bridge binary download from Theia Settings (6 targets: Win/Linux/macOS × amd64/arm64) — v1.5.0/Phase 24
- ✓ Origin + Host dual-validation on every bridge request (DNS rebinding protection) — v1.5.0/Phase 26
- ✓ Bridge hardcoded to WinBox only — no arbitrary process execution — v1.5.0/Phase 26
- ✓ "Open in WinBox" pre-authenticated from canvas context menu and Devices table — v1.5.0/Phase 25
- ✓ WinBox action disabled with tooltip when no WinBox profile designated — v1.5.0/Phase 25
- ✓ Frontend bridge health indicator (3-state: running/stopped/unknown) — v1.5.0/Phase 25
- ✓ System tray icon — start/stop server, open config, quit without process restart — v1.5.0/Phase 29
- ✓ `--no-tray` headless mode (SIGINT/SIGTERM shutdown) for servers without display — v1.5.0/Phase 29
- ✓ Bridge port settings-driven; frontend reads `bridge_port` from Theia Settings at startup — v1.5.0/Phase 31
- ✓ WebSocket delta payloads (FNV-64a hashing, only changed devices broadcast) — v1.5.0/Phase 28
- ✓ Legacy `ssh_profile_id` FK dropped from devices table (clean schema post-migration) — v1.5.0/Phase 27

### Active

<!-- Next milestone scope — TBD -->

_(Run `/gsd-new-milestone` to define v1.6.0 requirements)_

### Out of Scope

<!-- Explicit boundaries. Includes reasoning to prevent re-adding. -->

- Role-based access control in Theia itself — credential roles describe device access levels, not Theia user permissions
- WinBox fallback / auto-detection of "best" profile — user must explicitly designate the WinBox profile
- SSH terminal / interactive SSH in the browser — out of scope
- Support for applications other than WinBox — WinBox-only bridge; no arbitrary process execution
- Bridge auto-update / auto-start on OS boot — distribution is manual download; tray handles start/stop

## Context

- Tech stack: React 18 + TypeScript frontend, Go 1.24 backend, SQLite (CGO), Vite, Tailwind v3 Neon Topography design system
- Current codebase: ~16,400 lines added in v1.5.0 across 138 files; credential system, bridge binary, system tray
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
*Last updated: 2026-04-10 after v1.5.0 milestone (WinBox Integration — shipped)*
