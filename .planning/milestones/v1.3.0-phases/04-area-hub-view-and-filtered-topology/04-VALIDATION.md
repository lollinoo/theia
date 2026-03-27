---
phase: 4
slug: area-hub-view-and-filtered-topology
status: validated
nyquist_compliant: true
wave_0_complete: true
created: 2026-03-26
audited: 2026-03-27
---

# Phase 4 — Validation Strategy

> Per-phase validation contract for feedback sampling during execution.

---

## Test Infrastructure

| Property | Value |
|----------|-------|
| **Framework** | Vitest 4.1.0 + @testing-library/react 16.3.2 |
| **Config file** | `frontend/vitest.config.ts` |
| **Quick run command** | `cd frontend && npx vitest run` |
| **Full suite command** | `cd frontend && npx vitest run --reporter=verbose` |
| **Estimated runtime** | ~9 seconds |

---

## Sampling Rate

- **After every task commit:** Run `cd frontend && npx vitest run`
- **After every plan wave:** Run `cd frontend && npx vitest run --reporter=verbose`
- **Before `/gsd:verify-work`:** Full suite must be green
- **Max feedback latency:** 9 seconds

---

## Per-Task Verification Map

| Task ID | Plan | Wave | Requirement | Test Type | Automated Command | File | Status |
|---------|------|------|-------------|-----------|-------------------|------|--------|
| 04-01-01 | 01 | 1 | AREA-11 | compile | `npx tsc --noEmit` | n/a (refactor) | ✅ green |
| 04-01-02 | 01 | 1 | AREA-11 | compile+regression | `npx tsc --noEmit && npx vitest run` | n/a (refactor) | ✅ green |
| 04-01-03 | 01 | 1 | AREA-11 | compile+regression | `npx tsc --noEmit && npx vitest run` | n/a (refactor) | ✅ green |
| 04-02-01 | 02 | 2 | AREA-09 | unit | `npx vitest run src/components/NavigationPill.test.tsx` | NavigationPill.test.tsx | ✅ green |
| 04-02-02 | 02 | 2 | AREA-10 | unit | `npx vitest run src/components/Watermark.test.tsx` | Watermark.test.tsx | ✅ green |
| 04-03-01 | 03 | 2 | AREA-08 | unit | `npx vitest run src/components/AreaCard.test.tsx` | AreaCard.test.tsx | ✅ green |
| 04-03-02 | 03 | 2 | AREA-07 | unit | `npx vitest run src/components/AreaHub.test.tsx` | AreaHub.test.tsx | ✅ green |
| 04-04-01 | 04 | 3 | AREA-11 | unit | `npx vitest run src/components/canvas/useAreaFilteredTopology.test.ts src/components/DeviceCard.test.tsx` | useAreaFilteredTopology.test.ts + DeviceCard.test.tsx | ✅ green |
| 04-04-02 | 04 | 3 | AREA-11 | compile+regression | `npx tsc --noEmit && npx vitest run` | n/a (integration) | ✅ green |
| 04-04-03 | 04 | 3 | AREA-11 | checkpoint | User visual verification | n/a (manual) | ✅ green |

*Status: ⬜ pending · ✅ green · ❌ red · ⚠️ partial*

---

## Wave 0 Requirements

- [x] `frontend/src/components/AreaCard.test.tsx` — 5 tests for AREA-08 (area card content, health labels, counts, glow dot, click)
- [x] `frontend/src/components/AreaHub.test.tsx` — 5 tests for AREA-07 (heading, stat labels, empty state, area cards, health computation)
- [x] `frontend/src/components/NavigationPill.test.tsx` — 7 tests for AREA-09 (branding, area buttons, Global/area click, Devices view, Hub/Devices icons)
- [x] `frontend/src/components/Watermark.test.tsx` — 6 tests for AREA-10 (contextual text, hub/dashboard hidden, pointer-events, positioning)
- [x] `frontend/src/components/canvas/useAreaFilteredTopology.test.ts` — 6 tests for AREA-11 (null filter, area filtering, cross-area links, ghosts, unassigned exclusion)

*Plan 04-01 tasks are pure refactoring (Canvas decomposition) verified by tsc + existing test regression — no new test files needed.*

*All Wave 0 requirements satisfied.*

---

## Manual-Only Verifications

| Behavior | Requirement | Why Manual | Test Instructions |
|----------|-------------|------------|-------------------|
| Glassmorphism backdrop-blur on NavigationPill | AREA-09 | CSS visual effect not testable via DOM | Inspect pill in dark mode: translucent bg + blur visible behind pill |
| Area card bloom/radial blur hover effect | AREA-08 | CSS pseudo-element visual effect | Hover area cards: radial blur intensifies, border transitions to accent color |
| Watermark fade transition timing | AREA-10 | CSS transition timing not unit-testable | Switch areas: watermark text fades out/in over ~150ms |
| Ghost node muted visual styling | AREA-11 | Visual appearance verification | View filtered area with cross-area links: ghost nodes appear smaller/muted |

---

## Validation Audit 2026-03-27

| Metric | Count |
|--------|-------|
| Gaps found | 0 |
| Resolved | 0 |
| Escalated | 0 |

**All requirements fully covered.** Tests created during execution (Plans 02-04) provide comprehensive automated verification for all 5 requirements across 31 targeted tests.

**Test files covering Phase 4 requirements:**
- `NavigationPill.test.tsx` — 7 tests (AREA-09)
- `Watermark.test.tsx` — 6 tests (AREA-10)
- `AreaCard.test.tsx` — 5 tests (AREA-08)
- `AreaHub.test.tsx` — 5 tests (AREA-07)
- `useAreaFilteredTopology.test.ts` — 6 tests (AREA-11)
- `DeviceCard.test.tsx` — 2 ghost node tests (AREA-11)

---

## Validation Sign-Off

- [x] All tasks have `<automated>` verify or Wave 0 dependencies
- [x] Sampling continuity: no 3 consecutive tasks without automated verify
- [x] Wave 0 covers all MISSING references
- [x] No watch-mode flags
- [x] Feedback latency < 9s
- [x] `nyquist_compliant: true` set in frontmatter

**Approval:** complete (5/5 requirements automated)
