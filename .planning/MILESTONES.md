# Milestones

## v1.5.3 SNMP Pipeline Architecture (Shipped: 2026-04-15)

**Phases completed:** 12 phases, 34 plans, 63 tasks
**Timeline:** 4 days (2026-04-12 -> 2026-04-15)

**Key accomplishments:**

- State engine shipped with backend-owned health/staleness, hysteresis, soft-down/hard-down transitions, and diff-suppressed websocket change emission.
- Volatility-tiered collectors, OID classification, and per-device polling classes/overrides replaced the old single-cadence polling foundation.
- Jittered scheduling and PipelineOrchestrator cutover made the new pipeline the sole production polling/runtime path.
- Frontend device cards now render backend-owned health, freshness, and cadence metadata, and persisted polling overrides re-due the affected device immediately.
- Targeted detail-on-demand websocket updates now deliver selected-device link metrics so interface panels refresh without widening overview broadcasts.
- The milestone closed cleanly with finalized live-runtime proof, 9/9 Nyquist validation coverage, explicit 19/19 traceability, and a passing milestone audit.

---

## v1.5.1 Production Stability (Shipped: 2026-04-11)

**Phases completed:** 6 phases, 7 plans, retroactive tracking closed on 2026-04-11
**Files changed:** 173 (+5,533 / -17,707)
**Timeline:** 2 days (2026-04-10 -> 2026-04-11)

**Key accomplishments:**

- SNMP discovery now resolves LLDP local ports correctly and skips wasteful stats walks when source ports are absent
- Canvas updates hostname, hardware model, and discovered LLDP links live after asynchronous probe completion
- Credential-profile fetches were made lazy so canvas and dashboard no longer trigger per-device profile fan-out on initial load
- Bridge health polling now starts only after the first device context menu open
- Device list payloads stay compact while interface details are fetched on demand by the panels that need them
- LLDP neighbor follow-up work was accepted as complete, including more tolerant managed-neighbor matching and distinct handling for parallel uplinks

---

## v1.5.0 WinBox Integration (Shipped: 2026-04-10)

**Phases completed:** 9 phases, 19 plans, 21 tasks

**Key accomplishments:**

- SQLite migration 000012 renames ssh_profiles to credential_profiles with role='Admin' default, creates device_credential_profiles join table seeded from existing FK, and introduces CredentialProfile domain type replacing SSHProfile
- Pure rename of all Go SSHProfile consumers to CredentialProfile: repository, service, API handler, router, main.go, and all test files — zero behavior changes, full test suite passes
- One-liner:
- One-liner:
- One-liner:
- 1. [Rule 2 - Missing required field] Added `role: 'Admin'` to SSHCredentialForm.tsx createCredentialProfile call
- 1. [Rule 1 - Bug] waitFor + fake timers timeout in useBridgeHealth tests
- Standalone CGO-free Go HTTP server on localhost:1337 with Origin+Host validation, /health and /launch endpoints, WinBox auto-discovery, and 20 unit tests.
- Makefile `bridge-build-all` target and CI `build-bridge` job cross-compiling winbox-bridge for 6 targets (Windows/Linux/macOS x amd64/arm64) with CGO_ENABLED=0 and publishing as GitHub Release assets via softprops/action-gh-release@v2.
- One-liner:
- All frontend ssh_profile_id references removed: Device type cleaned, BulkBackupPanel uses fetchDeviceCredentialProfiles for eligibility, SSHCredentialForm migrated to dedicated assignment API
- Frontend deep-merges sparse WebSocket delta payloads into React state using SnapshotDeltaWSMessage type and mergeSnapshotDelta pure function — completing the client side of the 28-01 server-side delta optimization
- Config JSON persistence with os.UserConfigDir(), mutex-protected ServerManager start/stop lifecycle, and dynamic host-header validation replacing hardcoded port 1337
- fyne.io/systray tray icon with Start/Stop/Status/Open Config/Quit menu — wired to ServerManager, config reloaded on each Start click, systray.Run() blocks main()
- One-liner:
- Marked 4 stale REQUIREMENTS.md checkboxes complete (CRED-03, CRED-05, BRIDGE-01, BRIDGE-02) and removed dead `testSSHProfile` referencing a defunct API endpoint from client.ts
- Settings-driven bridge port replaces every hardcoded localhost:1337 — operators configure non-default ListenPort via SettingsPanel and all health checks and WinBox launches use it automatically

---

## v1.4.0 Backup & Restore (Shipped: 2026-04-07)

**Phases completed:** 8 phases, 19 plans, 29 tasks

**Delivered:** Full disaster recovery — export, verify, and restore the entire Theia application state (database + device config files) from a single archive.

**Key accomplishments:**

- Instance backup pipeline — VACUUM INTO + device configs into verified .tar.gz archives with manifest, SHA-256 checksums, and integrity checks
- Backup management UI — create, list, download, delete instance backups from SettingsPanel with async creation flow
- Restore pipeline with dry-run validation, cross-version migration, restart-based DB swap, and .pre-restore.bak safety net
- Scheduled automatic backups (instance + device) with configurable intervals (6h/12h/24h/48h/7d) and retention policies
- Server-side input validation and threat hardening — correlation IDs on all 500s, settings key allowlist, retention sweep timeout/batching
- Frontend validation parity — shared validation.ts, blur+submit timing, typed ValidationError/ServerError classes with correlation ID display

---

## v1.3.8 CI/CD (Shipped: 2026-04-03)

**Phases completed:** 3 phases, 5 plans, 10 tasks

**Key accomplishments:**

- Tag-based Makefile release workflow with git describe versioning and frontend version injection via Dockerfile ARG -> Vite define -> TypeScript constant
- Tag-triggered GitHub Actions release job building and pushing backend/frontend Docker images to GHCR with semver and :staging tags
- Staging Docker Compose stack pulling GHCR images with Watchtower auto-update on ports 3001/8081
- Production compose rewritten to pull from GHCR with version enforcement, Makefile updated with GHCR-pull prod targets and staging targets, .env.prod.example documents THEIA_VERSION and GHCR auth setup
- Gap closure: restored Makefile to Phase 12-01 state after parallel worktree regression

---

## v1.3.7 Virtual/Representative Nodes (Shipped: 2026-04-02)

**Phases completed:** 3 phases, 6 plans, 13 tasks
**Files changed:** 48 (+5,575 / -286)
**Timeline:** 2 days (2026-03-31 → 2026-04-01)
**Requirements:** 16/16 satisfied

**Key accomplishments:**

- Virtual device type as first-class domain entity with partial unique IP index for empty-IP coexistence
- Full API for virtual device CRUD with subtype tags, SNMP/poller exclusion, and virtual-side link validation
- Compact virtual cards with subtype-specific Material Symbol icons (160px/200px variants) and dashed borders
- Virtual link edge labels with single-side bandwidth, mismatch suppression, and findLinkMetrics fallback
- Dual-mode AddDevicePanel with Physical/Virtual toggle and 2x2 subtype icon cards
- Virtual-aware LinkCreatePanel, both-virtual rejection, and id-based Canvas context menu filtering

---

## v1.3.0 Frontend Redesign (Shipped: 2026-03-27)

**Phases completed:** 7 phases, 21 plans, 53 tasks

**Key accomplishments:**

- Tailwind v4 + @theme inline token system with dark/light Neon Topography palettes, ReactFlow v12, Fontsource fonts, and FOWT prevention script
- ThemeProvider context with localStorage persistence and OS preference detection, all ReactFlow imports migrated to @xyflow/react v12 with colorMode integration, and sun/moon toggle in NavBar
- All 45+ hardcoded hex color values replaced with CSS variable token references across 32 source files, with full Tailwind class name migration to new design token system
- Self-hosted Material Symbols Rounded subset woff2 (25KB, 19 icons) with shared MaterialIcon React component and 6 passing unit tests
- Restyled DeviceCard with severity-scaled glow nodes, surface tier metrics area, monospace vendor tag; upgraded StatusDot and LinkEdge label pills to Neon Topography aesthetics
- ContextMenu with glassmorphism/icons/separators, NavBar Material Symbols theme toggle, SearchOverlay theme-split glass overlay -- all using dark-only backdrop-blur per D-05/D-06
- Toolbar, SidePanel, ZoomControls restyled with Material Symbols icons and no-line rule; AlertsPanel with glow status dots and surface tier cards; ShortcutHelp and ReconnectBanner visually aligned
- Complete area CRUD REST API with SQLite persistence, device-to-area FK assignment, ON DELETE SET NULL cascade, and 14 passing behavioral tests
- AreaManager component with inline list CRUD (7-color swatch picker, device count badges, bidirectional device assignment), DeviceConfigPanel area dropdown with color preview, and 7 new passing Vitest tests
- Canvas.tsx decomposed from 1283 to 204 lines across 7 focused modules, with three-view App.tsx architecture and WebSocket lift for cross-view metric sharing
- Floating NavigationPill with glassmorphism surface, area buttons with color dots, and atmospheric Watermark replacing NavBar as sole navigation element
- AreaHub view with four aggregate stat cards (uptime/health/devices/links), per-area cards with accent color bloom effects, and health computation wired into App.tsx
- Area-filtered canvas with useAreaFilteredTopology hook, ghost node rendering, NavigationPill routing fixes, and visual verification
- Material Symbols font expanded with 4 action icons, FilterSelect dropdown component with TDD tests, and areas/snapshot props threaded from App to Dashboard
- SidePanel chrome tightened with font-mono metric rendering across all 6 dashboard sub-panels and consistent Neon Topography form tokens
- Canvas token audit test created; 4 files migrated from stale/hardcoded values to valid Tailwind v4 theme tokens
- Canvas.tsx and CanvasOverlays.tsx fully migrated from stale tokens and hardcoded hex to valid Tailwind v4 theme tokens
- SettingsPanel dev badge migrated from stale yellow-500/yellow-400 to semantic warning tokens, with Phase 2 VERIFICATION.md documenting all 13 requirements as satisfied

---
