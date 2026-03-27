---
phase: 5
slug: redesign-the-devices-page
status: validated
nyquist_compliant: true
wave_0_complete: true
created: 2026-03-26
---

# Phase 5 — Validation Strategy

> Per-phase validation contract for feedback sampling during execution.

---

## Test Infrastructure

| Property | Value |
|----------|-------|
| **Framework** | vitest + @testing-library/react |
| **Config file** | `frontend/vitest.config.ts` |
| **Quick run command** | `cd frontend && npx vitest run --reporter=verbose` |
| **Full suite command** | `cd frontend && npx vitest run --reporter=verbose` |
| **Estimated runtime** | ~10 seconds |

---

## Sampling Rate

- **After every task commit:** Run `cd frontend && npx vitest run --reporter=verbose`
- **After every plan wave:** Run `cd frontend && npx vitest run --reporter=verbose`
- **Before `/gsd:verify-work`:** Full suite must be green
- **Max feedback latency:** 10 seconds

---

## Per-Task Verification Map

| Task ID | Plan | Wave | Requirement | Test Type | Automated Command | File Exists | Status |
|---------|------|------|-------------|-----------|-------------------|-------------|--------|
| 05-01-01 | 01 | 1 | COMP-07 | unit | `cd frontend && npx vitest run src/components/dashboard/FilterSelect.test.tsx` | ✅ | ✅ green |
| 05-02-01 | 02 | 2 | COMP-07 | unit | `cd frontend && npx vitest run src/components/Dashboard.test.tsx src/components/dashboard/DeviceTable.test.tsx src/components/dashboard/DeviceRow.test.tsx` | ✅ | ✅ green |
| 05-03-01a | 03 | 1 | COMP-09 | source audit | `cd frontend && npx vitest run src/components/__tests__/font-mono-metrics.test.ts` | ✅ | ✅ green |
| 05-03-01b | 03 | 1 | COMP-08 | source audit | `cd frontend && npx vitest run src/components/__tests__/form-input-audit.test.ts` | ✅ | ✅ green |

*Status: ⬜ pending · ✅ green · ❌ red · ⚠️ flaky*

---

## Wave 0 Requirements

- Existing test infrastructure covers framework installation
- Component tests may need mock updates for new area data props

*All Wave 0 requirements satisfied.*

---

## Manual-Only Verifications

| Behavior | Requirement | Why Manual | Test Instructions |
|----------|-------------|------------|-------------------|
| Visual styling matches Neon Topography tokens | COMP-07 | CSS visual verification | Inspect rendered table in browser, verify token usage via DevTools |
| Sticky header behavior during scroll | COMP-07 | Scroll interaction | Load 20+ devices, scroll table body, verify header stays fixed |
| Skeleton loading animation | COMP-07 | Animation timing | Refresh page with slow network, verify skeleton rows appear |
| Icon font glyph rendering | COMP-07 | Font rendering | Click action icon buttons, verify icons render (not blank rectangles) |
| FilterSelect dropdown z-stacking | COMP-07 | z-index context | Open FilterSelect, scroll table, verify dropdown above table rows |
| Theme switching transitions | COMP-08 | CSS animation | Toggle dark/light theme, observe smooth 200ms transitions |

---

## Validation Sign-Off

- [x] All tasks have `<automated>` verify or Wave 0 dependencies
- [x] Sampling continuity: no 3 consecutive tasks without automated verify
- [x] Wave 0 covers all MISSING references
- [x] No watch-mode flags
- [x] Feedback latency < 10s
- [x] `nyquist_compliant: true` set in frontmatter

**Approval:** passed

---

## Validation Audit 2026-03-27

| Metric | Count |
|--------|-------|
| Gaps found | 3 |
| Resolved | 3 |
| Escalated | 0 |

**Tests added:**
- `font-mono-metrics.test.ts`: 4 new cases for Phase 5 sub-panels (BackupHistoryTable, ConfigViewer, BackupPanel, VendorSettingsPanel)
- `form-input-audit.test.ts`: 4 SidePanel chrome assertions + 2 sub-panel transition-colors assertions
