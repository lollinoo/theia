---
phase: 2
slug: component-restyling
status: validated
nyquist_compliant: true
wave_0_complete: true
created: 2026-03-25
audited: 2026-03-26
---

# Phase 2 — Validation Strategy

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
| 02-01-01 | 01 | 1 | COMP-01 | unit | `npx vitest run src/components/DeviceCard.test.tsx` | DeviceCard.test.tsx | ✅ green |
| 02-01-02 | 01 | 1 | COMP-02 | unit | `npx vitest run src/components/ContextMenu.test.tsx` | ContextMenu.test.tsx | ✅ green |
| 02-01-03 | 01 | 1 | COMP-03 | unit | `npx vitest run src/components/NavigationPill.nav.test.tsx` | NavigationPill.nav.test.tsx | ✅ green |
| 02-01-04 | 01 | 1 | COMP-04 | unit | `npx vitest run src/components/Toolbar.test.tsx` | Toolbar.test.tsx | ✅ green |
| 02-02-01 | 02 | 1 | COMP-05 | unit | `npx vitest run src/components/SettingsPanel.test.tsx` | SettingsPanel.test.tsx | ✅ green |
| 02-02-02 | 02 | 1 | COMP-06 | unit | `npx vitest run src/components/AlertsPanel.test.tsx` | AlertsPanel.test.tsx | ✅ green |
| 02-02-03 | 02 | 1 | COMP-07 | unit | `npx vitest run src/components/Dashboard.test.tsx` | Dashboard.test.tsx | ✅ green |
| 02-02-04 | 02 | 1 | COMP-08 | audit | `npx vitest run src/components/__tests__/form-input-audit.test.ts` | form-input-audit.test.ts | ✅ green |
| 02-03-01 | 03 | 2 | COMP-09 | audit | `npx vitest run src/components/__tests__/font-mono-metrics.test.ts` | font-mono-metrics.test.ts | ✅ green |
| 02-03-02 | 03 | 2 | COMP-10 | unit | `npx vitest run src/components/LinkEdge.test.tsx` | LinkEdge.test.tsx | ✅ green |
| 02-03-03 | 03 | 2 | COMP-11 | unit | `npx vitest run src/components/StatusDot.test.tsx` | StatusDot.test.tsx | ✅ green |
| 02-XX-XX | XX | X | COMP-12 | audit | `npx vitest run src/components/__tests__/no-line-audit.test.ts` | no-line-audit.test.ts | ✅ green |
| 02-XX-XX | XX | X | THEME-05 | smoke | `npx vitest run src/components/__tests__/theme05-smoke.test.tsx` | theme05-smoke.test.tsx | ✅ green |

*Status: ⬜ pending · ✅ green · ❌ red · ⚠️ partial*

---

## Wave 0 Requirements

- [x] `frontend/src/components/ContextMenu.test.tsx` — stubs for COMP-02 (icon/separator/danger styling)
- [x] Update `frontend/src/components/DeviceCard.test.tsx` — verify glow node, no top border, no bottom ports
- [x] `frontend/src/components/MaterialIcon.test.tsx` — verify class application, aria-hidden

*All Wave 0 requirements satisfied.*

---

## Manual-Only Verifications

| Behavior | Requirement | Why Manual | Test Instructions |
|----------|-------------|------------|-------------------|
| Glassmorphism blur visible on dark theme surfaces | COMP-05 | Visual rendering not testable via DOM assertions | Toggle to dark theme, open context menu, verify frosted glass blur behind menu |
| Bloom/glow behind status-critical elements | COMP-06 | CSS visual effect, needs visual inspection | View DeviceCard with status=up, verify radial glow visible behind node |
| 60 FPS at 100+ nodes with glow effects | COMP-06 | Performance testing requires browser DevTools | Open canvas with 100+ devices, open DevTools Performance panel, verify no frames below 60fps |
| All 25+ components correct in both themes | THEME-05 | Full visual audit across all views | Switch between dark/light, navigate all views, verify no invisible elements or wrong contrast |
| LinkDetailsPanel metric values in JetBrains Mono | COMP-09 | ~~Fixed~~ — `font-mono` added to port name spans. Automated test now green. | N/A |

---

## Validation Audit 2026-03-26

| Metric | Count |
|--------|-------|
| Gaps found | 11 |
| Resolved | 11 |
| Escalated | 0 |

**All gaps resolved.** COMP-09 LinkDetailsPanel.tsx fix applied — `font-mono` added to port name spans, test un-skipped and green.

**New test files created:**
- `StatusDot.test.tsx` — 5 tests (COMP-11)
- `Toolbar.test.tsx` — 4 tests (COMP-04)
- `NavigationPill.nav.test.tsx` — 4 tests (COMP-03)
- `AlertsPanel.test.tsx` — 4 tests (COMP-06)
- `SettingsPanel.test.tsx` — 3 tests (COMP-05)
- `LinkEdge.test.tsx` — 3 tests (COMP-10)
- `__tests__/no-line-audit.test.ts` — 1 test (COMP-12)
- `__tests__/form-input-audit.test.ts` — 1 test (COMP-08)
- `__tests__/font-mono-metrics.test.ts` — 3 tests, 1 skipped (COMP-09)
- `__tests__/theme05-smoke.test.tsx` — 3 tests (THEME-05)

**Updated test files:**
- `DeviceCard.test.tsx` — 3 new tests added (COMP-01 font-mono + glow ring)

---

## Validation Sign-Off

- [x] All tasks have `<automated>` verify or Wave 0 dependencies
- [x] Sampling continuity: no 3 consecutive tasks without automated verify
- [x] Wave 0 covers all MISSING references
- [x] No watch-mode flags
- [x] Feedback latency < 9s
- [x] `nyquist_compliant: true` set in frontmatter

**Approval:** complete (13/13 requirements automated)
