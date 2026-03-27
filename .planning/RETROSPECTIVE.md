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

## Cross-Milestone Trends

### Process Evolution

| Milestone | Phases | Plans | Key Change |
|-----------|--------|-------|------------|
| v1.3.0 | 7 | 21 | First GSD milestone; established phase-based execution with Nyquist validation |

### Cumulative Quality

| Milestone | Tests | Test Files | Source LOC |
|-----------|-------|------------|-----------|
| v1.3.0 | 193 | 30 | 14,130 TS |

### Top Lessons (Verified Across Milestones)

1. Source audit tests are the highest-leverage testing pattern for design system compliance
2. Canvas decomposition before feature work prevents monolith merge conflicts
