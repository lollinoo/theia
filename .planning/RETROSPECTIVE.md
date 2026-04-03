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

## Cross-Milestone Trends

### Process Evolution

| Milestone | Phases | Plans | Key Change |
|-----------|--------|-------|------------|
| v1.3.0 | 7 | 21 | First GSD milestone; established phase-based execution with Nyquist validation |
| v1.3.7 | 3 | 6 | Clean backend→rendering→forms dependency chain; parallel worktree execution |
| v1.3.8 | 4 | 6 | Infrastructure-only milestone; milestone audit caught parallel worktree regression |

### Cumulative Quality

| Milestone | Tests | Test Files | Source LOC |
|-----------|-------|------------|-----------|
| v1.3.0 | 193 | 30 | 14,130 TS |
| v1.3.7 | 224 | 33 | ~15,000 TS + Go tests |
| v1.3.8 | 224 | 33 | ~15,000 TS + Go tests (infra-only, no new tests) |

### Top Lessons (Verified Across Milestones)

1. Source audit tests are the highest-leverage testing pattern for design system compliance
2. Canvas decomposition before feature work prevents monolith merge conflicts
3. Keep REQUIREMENTS.md traceability updated during execution (repeated lesson: v1.3.0, v1.3.7, v1.3.8 — needs automation)
4. Backend→rendering→forms phase structure works well for new entity types
5. Milestone audits catch cross-phase regressions that execution-time verification misses (v1.3.8)
6. Good SUMMARY.md documentation makes gap closure trivially fast (v1.3.8)
