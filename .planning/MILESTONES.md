# Milestones

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
