# Project Retrospective

*A living document updated after each milestone. Lessons feed forward into future planning.*

## Milestone: v1.3.0 — Frontend Redesign

**Shipped:** 2026-03-27
**Phases:** 7 | **Plans:** 21 | **Tasks:** 53

### What Was Built
- Complete Neon Topography design system with 34 semantic CSS tokens, dark/light themes, FOWT prevention
- OSPF Area Hub view with floating navigation pill, area cards with bloom effects, atmospheric watermark, and area-filtered topology
- Area backend: SQLite persistence, REST API CRUD, device-to-area assignment with cascading deletes
- Redesigned devices page with custom FilterSelect dropdowns, 9-column sortable table, icon actions, three empty states
- Canvas decomposition from 1283-line monolith into 7 focused modules with three-view architecture
- Material Symbols woff2 subset (21 icons, 29KB) with shared MaterialIcon component
- 193 frontend tests across 30 test files

### What Worked
- Phase-based execution with atomic commits per task provided clear rollback points
- TDD approach (red-green commits) for FilterSelect and DeviceRow caught regressions early
- Canvas decomposition as Phase 4 plan 01 (before any new features) prevented merge conflicts in the monolith
- Parallel gap closure phases (6 + 7) after milestone audit efficiently closed remaining issues
- Source audit tests (font-mono-metrics, form-input-audit, canvas-token-audit, no-line-audit) catch regressions without rendering — fast and reliable

### What Was Inefficient
- Phase 5 Plan 02 SUMMARY.md was never created — documentation gap discovered during verification
- REQUIREMENTS.md traceability became stale as Phase 5 reused COMP-07/08/09 IDs already attributed to Phase 2
- Phase 2 had 6 plans which could have been condensed — some plans took only 2-3 minutes
- Milestone audit after all phases revealed integration gaps (area state refresh, Settings CTA wiring) that would have been cheaper to catch mid-milestone

### Patterns Established
- Source audit tests: read file content with fs.readFileSync, assert on class names/tokens — no DOM rendering needed
- Neon Topography token naming: bg-surface, bg-surface-high, bg-elevated, text-on-bg, text-on-bg-secondary
- font-mono for all technical values (timestamps, file sizes, OIDs, port numbers, metrics, code)
- No-line rule: surface color tiers for depth, never 1px borders for section separation
- transition-colors duration-200 on all panel containers for theme switching

### Key Lessons
1. Run milestone audit after ~70% of phases complete (not at the end) — catches integration gaps when fixing is cheaper
2. Keep REQUIREMENTS.md traceability table updated in real-time as phases execute, not retroactively
3. Canvas decomposition should precede any feature work on the canvas — monolith changes cause cascading merge conflicts
4. Source audit tests are the most cost-effective way to enforce design system compliance across many files
5. Gap closure phases work well as focused, small-scope phases — Phase 6 and 7 were each under 10 minutes total

### Cost Observations
- Model mix: ~30% opus (orchestration, planning, discussion), ~70% sonnet (execution, research, testing)
- Total execution time: ~135 min across 21 plans (6.4 min avg)
- Fastest plans: Phase 2 component restyling (~3.2 min avg) — well-scoped, repetitive pattern
- Slowest plans: Phase 4 area hub (~8.5 min avg) — new architecture, multiple integration points

---

## Milestone: v1.3.7 — Virtual/Representative Nodes

**Shipped:** 2026-04-02
**Phases:** 3 | **Plans:** 6 | **Tasks:** 13

### What Was Built
- Virtual device type as first-class domain entity with DeviceTypeVirtual constant and partial unique IP index (migration 000009)
- Full API for virtual device CRUD with subtype tags, SNMP poller exclusion, and virtual-side link validation
- Compact virtual cards with subtype-specific Material Symbol icons (internet/cloud/server/generic), IP-conditional sizing (160px/200px)
- Virtual link edge labels with single-side bandwidth computation and mismatch suppression
- Dual-mode AddDevicePanel with Physical/Virtual segmented toggle and 2x2 subtype icon cards
- Virtual-aware LinkCreatePanel with both-virtual rejection and id-based Canvas context menu filtering
- 224 frontend tests across 33 test files; 53+ Go backend tests

### What Worked
- Three-phase structure (backend → rendering → forms) created clean dependency chains with no circular dependencies
- Early-return branch pattern (matching ghost node pattern) kept virtual code isolated from physical code paths
- Defense-in-depth approach: both-virtual rejection in frontend AND backend, poller skip alongside probeDevice guard
- Worktree-based parallel plan execution (09-01 and 09-02 ran in parallel worktrees successfully)
- All 6 plans executed without blockers — clean phase planning with discuss-phase pre-work

### What Was Inefficient
- REQUIREMENTS.md traceability checkboxes never updated during execution (all 16 stayed "Pending" despite being satisfied)
- Nyquist VALIDATION.md files remained in draft state — validation was done via VERIFICATION.md behavioral spot-checks instead
- CLI `milestone complete` tool only detected Phase 10 (not 8-9) — required manual archive correction
- Phase 9 Plan 02 had to duplicate the DeviceType 'virtual' addition from Plan 01 due to parallel worktree execution

### Patterns Established
- Virtual device early-return branch pattern: check deviceType early, return before regular validation
- isVirtualLink early-return in edge builder: detect virtual device_type, compute single-side bandwidth
- Stable id-based context menu filtering using Set.has() — robust against label text changes
- DeviceMode segmented control pattern for form mode switching with full state reset
- findLinkMetrics dual-lookup: try source device first, fall back to target for virtual-source links

### Key Lessons
1. Keep REQUIREMENTS.md traceability updated during execution — lesson repeated from v1.3.0 (still not automated)
2. milestone complete CLI tool does not correctly detect phases from the progress table — manual verification needed
3. Parallel worktree execution requires explicit prerequisite handling when plans share file modifications
4. Three-phase backend→rendering→forms structure works well for new entity types — reusable pattern
5. Defense-in-depth (frontend + backend validation) is worth the duplication for user-facing constraints

### Cost Observations
- Model mix: ~25% opus (orchestration, audit), ~75% sonnet (execution, verification, integration check)
- Total execution time: ~25 min across 6 plans (4.2 min avg)
- Fastest plans: Phase 9 plans (~3.5 min avg) — well-scoped rendering changes
- Slowest plans: Phase 8 Plan 02 (6 min) — touched 6 files across api and worker packages

---

## Milestone: v1.3.8 — CI/CD

**Shipped:** 2026-04-03
**Phases:** 4 | **Plans:** 6 | **Tasks:** 10

### What Was Built
- GitHub Actions CI workflow with parallel Go backend (CGO_ENABLED=1) and frontend Vitest jobs
- Tag-triggered release pipeline building and pushing Docker images to GHCR with semver and :staging tags
- Staging Docker Compose stack with Watchtower auto-updates pulling :staging images (30s polling)
- Production Docker Compose stack with version-pinned GHCR images and fail-fast enforcement
- Makefile release workflow with 5 pre-flight checks (clean worktree, master branch, semver, tag uniqueness, VERSION param)
- Frontend version injection chain: Dockerfile ARG → ENV → Vite define → TypeScript constant
- Optimized .dockerignore files (20+ entries for backend, 7 for frontend)

### What Worked
- Single-day execution for the entire CI/CD milestone — focused infrastructure phases are fast
- Verification-driven gap closure: milestone audit caught the Makefile regression immediately
- Phase 14 gap closure was trivial (~2 min) because the target state was fully documented in Phase 12-01 summary
- Git commit references in SUMMARYs made regression root cause analysis straightforward
- Infrastructure-only phases have high verification confidence via behavioral spot-checks (no UI rendering needed)

### What Was Inefficient
- Parallel worktree merge in Phase 13-02 silently overwrote Phase 12-01's Makefile changes — no merge conflict because both touched different sections
- REQUIREMENTS.md traceability checkboxes again never updated during execution (third milestone with this issue)
- Phase 12 VERIFICATION.md was generated by verifier before phase 13 started — it verified Makefile state that was later destroyed
- gsd-tools `phase complete` reported wrong plan counts for Phase 13 (1/2 instead of 2/2)

### Patterns Established
- `git describe --tags --always` as VERSION source of truth — no file to maintain
- `${THEIA_VERSION:?}` fail-fast pattern for production compose version enforcement
- Watchtower label-scoped auto-update: `WATCHTOWER_LABEL_ENABLE=true` + per-container labels
- 5-check release pre-flight pattern: VERSION param, clean worktree, master branch, semver validation, tag uniqueness
- `.env.*.example` pattern for documenting GHCR auth setup per environment

### Key Lessons
1. Parallel worktree merges can silently overwrite earlier phase changes — diff the Makefile (or any shared file) after merging parallel work
2. Milestone audits are essential for catching cross-phase regressions, especially when phases share files
3. Infrastructure phases don't need Nyquist validation — behavioral spot-checks are sufficient
4. Phase 14's gap closure was fast because Phase 12-01's SUMMARY.md documented the exact target state — good summaries make regressions trivially fixable
5. REQUIREMENTS.md traceability STILL not updated during execution — this needs to be automated in gsd-tools (third milestone identifying this gap)

### Cost Observations
- Model mix: ~20% opus (orchestration, audit), ~80% sonnet (execution, verification)
- Total execution time: ~30 min across 6 plans (5 min avg)
- Fastest plans: Phase 14-01 (2 min) — single Makefile fix with known target state
- Slowest plans: Phase 12-02 (8 min) — ci.yml release job with GHCR metadata, multi-stage Docker builds

---

## Milestone: v1.4.0 — Backup & Restore

**Shipped:** 2026-04-07
**Phases:** 8 | **Plans:** 19 | **Commits:** 64

### What Was Built
- Instance backup pipeline: VACUUM INTO + device config files → verified .tar.gz archive with manifest, SHA-256 checksums, integrity checks
- Backup management UI with async creation, list, download, delete, and collapsible SettingsPanel sections
- Restore pipeline with dry-run validation, cross-version migration, restart-based DB swap, and .pre-restore.bak safety net
- Scheduled automatic backups for both instance and device configs with configurable intervals and retention policies
- Server-side input validation on all mutation endpoints with correlation IDs on 500 errors, settings key allowlist
- Frontend validation parity: shared validation.ts, blur+submit timing, typed ValidationError/ServerError classes
- Audit gap closure: zero raw err.Error() in 500s, typed restore errors, dead code removal

### What Worked
- Milestone audit at ~85% completion (after Phase 21, before Phase 22) caught exactly the right gaps — SC-9 raw err.Error() and untyped restore errors
- Gap closure phase (Phase 22) was surgical: 2 plans, 5 commits, closed all 3 audit gaps cleanly
- Vertical slice structure for backup phases (15-17: core → API → restore) enabled incremental testability
- writeError variadic extension preserved all existing call sites while adding correlation ID infrastructure
- Settings key allowlist + type dispatch pattern provided comprehensive input validation with a single gatekeeper function
- Shared validation.ts with string|null return pattern enabled consistent validation across all form components

### What Was Inefficient
- No formal REQUIREMENTS.md created for this milestone — requirements tracked only via ROADMAP.md success criteria
- Phase 20 VERIFICATION.md flagged SC-9 as failed but the phase was marked complete — gap was deferred to Phase 22 instead of fixing inline
- Some SUMMARY.md one_liner fields were empty or malformed (showing "One-liner:" placeholder), degrading automatic extraction
- Phase 22 Nyquist VALIDATION.md was not created — minor since it was a 2-plan gap closure phase

### Patterns Established
- VACUUM INTO for non-locking DB backup snapshots
- Marker-file pattern (.theia-restore-pending) for restart-based DB restore with log.Fatalf/log.Printf severity split
- Tar archive allowlist extraction with path traversal, symlink, and absolute path rejection
- writeError variadic pattern: existing `writeError(w, code, msg)` calls unchanged, new `writeError(w, code, msg, err)` logs real error with correlation ID
- Settings key allowlist with validSettingKeys map + validateSetting type dispatch (numeric/URL/timezone/interval-allowlist)
- fieldErrors Record<string,string> + handleBlur factory as standard form validation pattern
- Typed error catch chain order: ServerError → ValidationError → Error → unknown fallback

### Key Lessons
1. Milestone audit timing matters: running at ~85% (not 100%) allows gap closure phases to be part of the same milestone
2. Empty one_liner fields in SUMMARYs should be caught by the CLI tool — needs validation guardrail
3. Vertical slice phases (domain → API → UI) are ideal for backup/restore features — each phase is independently testable
4. Formal REQUIREMENTS.md still has value even when ROADMAP.md has success criteria — the traceability table forces explicit coverage tracking
5. writeError variadic pattern was the right call — extending the existing function instead of adding a new one avoided 50+ call site changes

### Cost Observations
- Commits: 64 across 19 plans (3.4 commits/plan avg)
- Files modified: 107 (+18,458 / -154 lines)
- Timeline: 3 days (2026-04-05 → 2026-04-07)
- Fastest phases: Phase 22 (audit gap closure, 2 plans) — surgical scope
- Slowest phases: Phase 20 (input validation, 3 plans) — touched every handler file in internal/api/

---

## Milestone: v1.5.0 — WinBox Integration

**Shipped:** 2026-04-10
**Phases:** 9 | **Plans:** 19 | **Commits:** 79

### What Was Built
- Multi-profile credential system: `ssh_profiles` → `credential_profiles` with free-text `role`, `device_credential_profiles` join table, zero-data-loss migration from single FK to many-to-many
- WinBox bridge binary: CGO-free Go HTTP server, Origin + Host dual-validation, `/health` + `/launch` endpoints, WinBox auto-discovery — 6 platform targets built in CI matrix
- System tray integration: fyne.io/systray icon with Start/Stop/Open Config/Quit; `--no-tray` headless mode; config persisted to `os.UserConfigDir()/winbox-bridge/config.json`
- Frontend credential profile manager: create/edit/delete/assign profiles, WinBox designation toggle, 3-state health indicator
- WebSocket delta payloads: FNV-64a per-device hashing, only changed devices broadcast per cycle, `mergeSnapshotDelta` frontend deep-merge
- Gap closure and hardcoded-port elimination: bridge port settings-driven from Theia UI, stale REQUIREMENTS.md checkboxes corrected, dead `testSSHProfile` removed

### What Worked
- TDD throughout: failing tests committed before implementation in every phase — caught regressions before code review
- Gap closure phases (30 and 31) added mid-milestone after audit worked perfectly — audit-driven scope expansion without derailing the milestone
- Vertical slice pattern (schema → backend API → frontend → bridge binary → system tray) created clean dependency chains with no circular blockers
- Milestone audit at ~78% completion (before phases 30-31) identified exactly the gaps that became the next two phases
- CGO-free bridge binary strategy: all 6 targets compile in CI without platform-native runners (except macOS systray CGO — handled with a separate runner job)
- Injectable `startProcess` var and `loadConfigFrom/saveConfigTo` path helpers kept bridge binary fully testable without OS-level mocking

### What Was Inefficient
- Several SUMMARY.md `one_liner` fields left as placeholder ("One-liner:") — degraded automatic accomplishment extraction in `milestone complete`
- Milestone audit `status: gaps_found` was not updated after phases 30-31 closed the gaps — stale audit file created confusion at completion time
- Phase 29 VERIFICATION.md required `human_needed` status (systray requires desktop session) — could plan for cross-platform CI test earlier
- `winbox-bridge-windows-amd64.exe` build artifact landed in project root untracked and was not in `.gitignore` (only `/winbox-bridge` and `/winbox-bridge.exe` were covered)

### Patterns Established
- `device_credential_profiles` join table pattern: clear-then-set transaction for exclusive designation (WinBox flag); idempotent `Clear` method
- Testable config helpers: `loadConfigFrom(path)` / `saveConfigTo(path)` for path injection; public `loadConfig()` / `saveConfig()` delegate to `configFilePath()`
- `ServerManager` mutex-protected lifecycle: `Start` captures local `srv` var (not field) to prevent nil dereference race with `Stop`
- FNV-64a delta hashing: `prevHashes` on collector protected by existing `c.mu` RWMutex; `buildDelta` returns `nil` to skip broadcast entirely
- `securityCheck(expectedHost string)` param pattern: removes hardcoded port, enables dynamic port config from settings
- Platform binary gitignore pattern: `/winbox-bridge`, `/winbox-bridge.exe` — should also cover `/winbox-bridge-*` for platform-suffixed variants

### Key Lessons
1. Milestone audit at ~78% completion (before gap closure phases) is the right timing — gaps became the scope for the next two phases
2. `one_liner` field in SUMMARY.md matters for automated tooling — always fill it, never leave the placeholder
3. Plan for platform-specific binary gitignore variants (`/binary-name-*`) when cross-compiling with OS/arch suffixes
4. Stale milestone audit files (status: gaps_found after gaps are closed) create confusion — update or re-run audit after gap closure phases complete
5. Injectable test helpers (path injection, process injection) for OS-dependent behavior are worth writing upfront — eliminates heavy mocking later
6. CGO-free bridge strategy is the right default; platform-specific CGO requirements (macOS systray) can be isolated to separate CI jobs

### Cost Observations
- Commits: 79 across 19 plans (4.2 commits/plan avg)
- Files modified: 138 (+16,404 / -1,764 lines)
- Timeline: 3 days (2026-04-07 → 2026-04-10)
- Fastest phases: Phase 30 (gap closure docs, 1 plan) and Phase 28 (delta hashing, 2 plans) — well-scoped, clear target state
- Slowest phases: Phase 29 (system tray, 3 plans) — multi-plan with platform-specific CGO, CI matrix, and human verification required

---

## Milestone: v1.5.3 — SNMP Pipeline Architecture

**Shipped:** 2026-04-15
**Phases:** 12 | **Plans:** 34 | **Tasks:** 63

### What Was Built
- Backend state engine with health/hysteresis, staleness tracking, and diff-suppressed websocket change emission
- Volatility-tiered SNMP collectors, vendor OID classification, and per-device polling classes/overrides
- Jittered scheduler plus PipelineOrchestrator cutover as the sole production polling/runtime path
- Frontend health, freshness, cadence, and override UX on top of backend-owned polling metadata
- Targeted detail-on-demand websocket updates that now deliver selected-device link metrics to interface panels
- Finalized runtime/UAT closure, 9/9 Nyquist validation coverage, and 19/19 planning traceability before archival

### What Worked
- The milestone stayed on a clean architectural dependency chain: state engine → collectors → scheduler → pipeline cutover → frontend integration
- Gap-closure phases 45-49 let the audit findings be closed inside the same milestone instead of spilling into the next release
- Preserving the existing `snapshot` / `snapshot_delta` contract reduced regression risk while the runtime internals changed significantly
- Closing Phase 47 before archival prevented avoidable documentation debt from entering the milestone archive

### What Was Inefficient
- Summary one-liner quality was inconsistent, which degraded automated accomplishment extraction in `milestone complete`
- The milestone completion CLI archived the stale pre-Phase-47 roadmap state and required manual correction afterward
- Planning traceability cleanup was known before the end of the milestone but was deferred until a dedicated closeout phase

### Patterns Established
- State-engine + scheduler + orchestrator runtime split for live SNMP polling
- Targeted `snapshot_delta` detail updates for selected devices without widening overview broadcasts
- Milestone-close documentation repair as an explicit, auditable phase instead of an implicit cleanup step

### Key Lessons
1. Keep SUMMARY one-liners high quality because milestone tooling depends on them directly.
2. If an audit uncovers planning-traceability debt, assign it an explicit phase early instead of assuming completion-time cleanup.
3. Preserve stable wire contracts while swapping runtime internals whenever possible; it kept the v1.5.3 cutover tractable.
4. Validate archive outputs, not just source planning files, because the completion CLI can still snapshot stale state.

### Cost Observations
- Timeline: 4 days (2026-04-12 → 2026-04-15)
- Milestone size: 12 phases, 34 plans, 63 tasks
- Work mix skewed heavily toward execution/verification, with the final phases focused on audit closure rather than net-new runtime surface

---

## Cross-Milestone Trends

### Process Evolution

| Milestone | Phases | Plans | Key Change |
|-----------|--------|-------|------------|
| v1.3.0 | 7 | 21 | First GSD milestone; established phase-based execution with Nyquist validation |
| v1.3.7 | 3 | 6 | Clean backend→rendering→forms dependency chain; parallel worktree execution |
| v1.3.8 | 4 | 6 | Infrastructure-only milestone; milestone audit caught parallel worktree regression |
| v1.4.0 | 8 | 19 | Largest milestone; audit-driven gap closure phase; no REQUIREMENTS.md (tracked via ROADMAP SC) |
| v1.5.0 | 9 | 19 | Full credential redesign + bridge binary + system tray; TDD throughout; audit at 78% added 2 gap phases |

### Cumulative Quality

| Milestone | Tests | Test Files | Source LOC |
|-----------|-------|------------|-----------|
| v1.3.0 | 193 | 30 | 14,130 TS |
| v1.3.7 | 224 | 33 | ~15,000 TS + Go tests |
| v1.3.8 | 224 | 33 | ~15,000 TS + Go tests (infra-only, no new tests) |
| v1.4.0 | 224+ | 33+ | ~15,000 TS + 53+ Go tests (+18,458 lines added) |
| v1.5.0 | 224+ | 33+ | ~31,000+ TS + Go (+16,404 lines added, 138 files) |

### Top Lessons (Verified Across Milestones)

1. Source audit tests are the highest-leverage testing pattern for design system compliance
2. Canvas decomposition before feature work prevents monolith merge conflicts
3. Keep REQUIREMENTS.md traceability updated during execution (resolved in v1.5.0 with formal 20-req REQUIREMENTS.md and per-phase tracking)
4. Backend→rendering→forms phase structure works well for new entity types; vertical slices (domain→API→UI) work well for backup/restore features
5. Milestone audits catch cross-phase regressions that execution-time verification misses (v1.3.8, v1.4.0, v1.5.0)
6. Good SUMMARY.md documentation makes gap closure trivially fast — always fill `one_liner` field (v1.3.8, v1.4.0, v1.5.0)
7. Run milestone audit at ~78-85% completion to allow gap closure within the same milestone (v1.4.0, v1.5.0)
8. Extending existing patterns (writeError variadic, injectable helpers) is better than new functions when many call sites exist (v1.4.0, v1.5.0)
9. TDD (failing tests committed before implementation) catches regressions before code review — highest-value practice in v1.5.0
10. Plan gitignore entries for all binary name variants (`/binary-name-*`) when cross-compiling with OS/arch suffixes
