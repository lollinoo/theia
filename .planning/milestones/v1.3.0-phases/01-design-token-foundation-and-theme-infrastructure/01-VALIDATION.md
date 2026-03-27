---
phase: 1
slug: design-token-foundation-and-theme-infrastructure
status: draft
nyquist_compliant: true
wave_0_complete: true
created: 2026-03-25
---

# Phase 1 — Validation Strategy

> Per-phase validation contract for feedback sampling during execution.

---

## Test Infrastructure

| Property | Value |
|----------|-------|
| **Framework** | vitest 4.1 |
| **Config file** | `frontend/vitest.config.ts` |
| **Quick run command** | `cd frontend && npx vitest run --reporter=verbose` |
| **Full suite command** | `cd frontend && npx vitest run` |
| **Estimated runtime** | ~15 seconds |

---

## Sampling Rate

- **After every task commit:** Run `cd frontend && npx vitest run --reporter=verbose`
- **After every plan wave:** Run `cd frontend && npx vitest run`
- **Before `/gsd:verify-work`:** Full suite must be green
- **Max feedback latency:** 15 seconds (tests), 30 seconds (vite build)

---

## Per-Task Verification Map

| Task ID | Plan | Wave | Requirement | Test Type | Automated Command | File Exists | Status |
|---------|------|------|-------------|-----------|-------------------|-------------|--------|
| 01-01-01 | 01 | 1 | FOUND-01 | grep | `grep 'var(--color-' frontend/src/index.css` | n/a (grep) | pending |
| 01-01-02 | 01 | 1 | FOUND-02 | build | `cd frontend && npx vite build` | n/a (build) | pending |
| 01-01-03 | 01 | 1 | FOUND-04 | grep | `grep 'outfit' frontend/src/index.css` | n/a (grep) | pending |
| 01-01-04 | 01 | 1 | FOUND-05 | grep | `grep 'jetbrains-mono' frontend/src/index.css` | n/a (grep) | pending |
| 01-02-00 | 02 | 2 | THEME-01/02/03 | unit | `cd frontend && npx vitest run src/contexts/ThemeContext.test.tsx` | W0 (created by Task 0) | pending |
| 01-02-01 | 02 | 2 | FOUND-03 | grep | `grep '@xyflow/react' frontend/package.json` | n/a (grep) | pending |
| 01-02-02 | 02 | 2 | THEME-01 | unit + manual | `npx vitest run src/contexts/ThemeContext.test.tsx` + browser toggle | ThemeContext.test.tsx | pending |
| 01-02-03 | 02 | 2 | THEME-02 | unit + manual | `npx vitest run src/contexts/ThemeContext.test.tsx` + refresh test | ThemeContext.test.tsx | pending |
| 01-02-04 | 02 | 2 | THEME-03 | unit + manual | `npx vitest run src/contexts/ThemeContext.test.tsx` + OS pref test | ThemeContext.test.tsx | pending |
| 01-02-05 | 02 | 2 | THEME-04 | manual | FOWT absence test | N/A | pending |
| 01-03-01 | 03 | 3 | FOUND-06 | grep | `grep -rE "'#[0-9a-fA-F]{3,8}'" frontend/src/ --include='*.tsx' --include='*.ts' \| grep -v test \| grep -v index.css` | n/a (grep) | pending |
| 01-03-02 | 03 | 3 | FOUND-06 | build + test | `cd frontend && npx vite build && npm test` | n/a | pending |

*Status: pending / green / red / flaky*

---

## Wave 0 Requirements

- [x] `frontend/src/contexts/ThemeContext.test.tsx` — Unit tests for ThemeProvider (toggle, persist, system preference detection). Created by Plan 01-02, Task 0 (Wave 0).
- [x] `frontend/src/index.css` — CSS token definitions via `@theme` directive. Created by Plan 01-01, Task 2.
- [x] `frontend/package.json` — Updated dependencies (tailwindcss v4, @xyflow/react v12, fontsource packages). Created by Plan 01-01, Task 1.

*Existing vitest infrastructure covers build verification. ThemeContext.test.tsx provides automated coverage for THEME-01/02/03.*

---

## Manual-Only Verifications

| Behavior | Requirement | Why Manual | Automated Coverage | Test Instructions |
|----------|-------------|------------|-------------------|-------------------|
| Theme toggle switches dark/light | THEME-01 | Visual confirmation of full-app theme change | Unit tests verify context state change + DOM attribute | Click sun/moon icon in NavBar, verify entire app changes theme |
| Theme persists across refresh | THEME-02 | Requires real browser refresh cycle | Unit tests verify localStorage read/write | Set theme, refresh page, verify same theme loads |
| OS preference detected on first visit | THEME-03 | Requires OS-level dark mode toggle | Unit tests verify matchMedia resolution | Clear localStorage, toggle OS dark mode, reload, verify match |
| No flash of wrong theme | THEME-04 | Timing-sensitive visual check (sub-frame) | No automated equivalent — FOWT is a paint-timing issue | Set light theme, refresh, observe no dark flash before light renders |

---

## Validation Sign-Off

- [x] All tasks have `<automated>` verify or Wave 0 dependencies
- [x] Sampling continuity: no 3 consecutive tasks without automated verify
- [x] Wave 0 covers all MISSING references (ThemeContext.test.tsx created in Plan 02 Task 0)
- [x] No watch-mode flags
- [x] Feedback latency < 30s (tests ~15s, build ~15-30s — acceptable with note)
- [x] `nyquist_compliant: true` set in frontmatter

**Approval:** approved
