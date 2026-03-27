---
phase: 02-component-restyling
plan: 02
subsystem: ui
tags: [device-card, status-dot, link-edge, glow-node, tailwind, neon-topography, surface-tiers]

# Dependency graph
requires:
  - phase: 01-design-token-foundation-and-theme-infrastructure
    provides: "Tailwind v4 token system with --nt-glow-shadow-opacity, surface hierarchy, status colors"
  - phase: 02-component-restyling
    plan: 01
    provides: "Material Symbols Rounded subset font and MaterialIcon component"
provides:
  - "DeviceCard restyled with glow node, surface tier metrics, monospace vendor tag, no bottom ports"
  - "StatusDot with severity-scaled box-shadow glow using theme-aware opacity variable"
  - "LinkEdge label pills using surface token backgrounds with smooth theme transitions"
affects: [02-03, 02-04, 02-05, 02-06]

# Tech tracking
tech-stack:
  added: []
  patterns:
    - "Severity-scaled glow: down=16px, degraded=14px, probing=12px, up=8px, unknown=6px"
    - "Surface tier sectioning replaces border-t separators per no-line rule"
    - "cardRingClass pattern: status-aware ring + shadow derived from existing props"
    - "Vendor badge as monospace tag (font-mono + bg-surface-high) not colored pill"

key-files:
  created: []
  modified:
    - frontend/src/components/StatusDot.tsx
    - frontend/src/components/DeviceCard.tsx
    - frontend/src/components/DeviceCard.test.tsx
    - frontend/src/components/LinkEdge.tsx

key-decisions:
  - "Used var(--nt-glow-shadow-opacity) in Tailwind arbitrary shadow values -- Tailwind v4 parses them correctly"
  - "Primary green glow (#00E676) for hover accent on all cards regardless of status (D-15 discretion)"
  - "DeviceIcon color changed from text-tertiary to text-on-bg-secondary for muted icon appearance"

patterns-established:
  - "Glow node pattern: StatusDot with severity-scaled box-shadow + theme-aware opacity variable"
  - "No-line rule: bg-surface-high replaces border-t border-outline for section separation"
  - "Hover accent: hover:ring-primary/60 + hover:shadow for interactivity signal"
  - "motion-reduce:animate-none on elements with animate-pulse"

requirements-completed: [COMP-01, COMP-10, COMP-11, COMP-12]

# Metrics
duration: 2min
completed: 2026-03-25
---

# Phase 02 Plan 02: Canvas Component Restyling Summary

**Restyled DeviceCard with severity-scaled glow nodes, surface tier metrics area, monospace vendor tag; upgraded StatusDot and LinkEdge label pills to Neon Topography aesthetics**

## Performance

- **Duration:** 2 min
- **Started:** 2026-03-25T21:57:56Z
- **Completed:** 2026-03-25T22:00:20Z
- **Tasks:** 3
- **Files modified:** 4

## Accomplishments
- StatusDot upgraded to severity-scaled glow system: down=16px (most urgent), up=8px (subtle), unknown=6px (dimmest), all using `var(--nt-glow-shadow-opacity)` for automatic dark/light intensity
- DeviceCard restructured: removed 3px top border and decorative bottom ports, replaced metrics border separator with `bg-surface-high` surface tier, vendor badge as monospace tag, green primary hover accent
- LinkEdge label pills updated to `bg-surface` with `border-outline-subtle` and `shadow-pill` token backgrounds
- All components gain `transition-colors duration-200` for smooth theme switching and `motion-reduce:animate-none` for accessibility
- 4 new DeviceCard tests verifying no top border, no ports, monospace vendor badge, surface tier metrics
- All 65 tests pass, Vite build clean

## Task Commits

Each task was committed atomically:

1. **Task 1: Upgrade StatusDot to Glow Node** - `7d204d4` (feat)
2. **Task 2: Restyle DeviceCard** - `a13f6a2` (feat)
3. **Task 3: Update LinkEdge label pills** - `2801f7a` (feat)

## Files Created/Modified
- `frontend/src/components/StatusDot.tsx` - Severity-scaled glow system with theme-aware opacity, motion-reduce support
- `frontend/src/components/DeviceCard.tsx` - Glow node header, surface tier metrics, monospace vendor tag, green hover accent, no bottom ports
- `frontend/src/components/DeviceCard.test.tsx` - 4 new tests: no top border, no bottom ports, monospace vendor badge, surface tier metrics
- `frontend/src/components/LinkEdge.tsx` - Surface token backgrounds on bandwidth and throughput label pills

## Decisions Made
- Used `var(--nt-glow-shadow-opacity)` directly in Tailwind v4 arbitrary shadow values -- confirmed to work in build without needing inline style fallback
- Primary green glow for hover accent (D-15 discretion) -- status glow communicates health, hover glow signals interactivity uniformly
- DeviceIcon color changed from `text-tertiary` to `text-on-bg-secondary` -- the purple accent was too strong for the new muted aesthetic

## Deviations from Plan

None - plan executed exactly as written.

## Issues Encountered
- Worktree was based on an old commit (v1.2.0 release) and needed fast-forward merge to milestone branch before execution. Resolved with `git merge --ff-only gsd/v1.3.0-milestone`.

## User Setup Required
None - no external service configuration required.

## Next Phase Readiness
- Canvas core components (DeviceCard, StatusDot, LinkEdge) are fully restyled to Neon Topography
- Established glow node and surface tier patterns for remaining component restyling plans
- Ready for Plan 02-03 (ContextMenu restyling with glassmorphism) and subsequent waves

## Known Stubs
None - all functionality is fully wired.

## Self-Check: PASSED

All files verified present. All commit hashes verified in git log.

---
*Phase: 02-component-restyling*
*Completed: 2026-03-25*
