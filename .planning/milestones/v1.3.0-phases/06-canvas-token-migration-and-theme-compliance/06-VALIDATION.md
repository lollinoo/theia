---
phase: 6
slug: canvas-token-migration-and-theme-compliance
status: draft
nyquist_compliant: false
wave_0_complete: false
created: 2026-03-27
---

# Phase 6 — Validation Strategy

> Per-phase validation contract for feedback sampling during execution.

---

## Test Infrastructure

| Property | Value |
|----------|-------|
| **Framework** | Vitest 4.1 |
| **Config file** | `frontend/vitest.config.ts` |
| **Quick run command** | `cd frontend && npx vitest run -x` |
| **Full suite command** | `cd frontend && npx vitest run` |
| **Estimated runtime** | ~15 seconds |

---

## Sampling Rate

- **After every task commit:** Run `cd frontend && npx vitest run -x`
- **After every plan wave:** Run `cd frontend && npx vitest run`
- **Before `/gsd:verify-work`:** Full suite must be green
- **Max feedback latency:** 15 seconds

---

## Per-Task Verification Map

| Task ID | Plan | Wave | Requirement | Test Type | Automated Command | File Exists | Status |
|---------|------|------|-------------|-----------|-------------------|-------------|--------|
| 06-01-01 | 01 | 1 | FOUND-06, THEME-05 | unit (source scan) | `cd frontend && npx vitest run src/components/__tests__/canvas-token-audit.test.ts -x` | ❌ W0 | ⬜ pending |
| 06-01-02 | 01 | 1 | COMP-12 | unit (source scan) | `cd frontend && npx vitest run src/components/__tests__/no-line-audit.test.ts -x` | ✅ | ⬜ pending |
| 06-02-01 | 02 | 1 | FOUND-06 | unit (source scan) | `cd frontend && npx vitest run src/components/__tests__/canvas-token-audit.test.ts -x` | ❌ W0 | ⬜ pending |
| 06-02-02 | 02 | 1 | THEME-05 | unit (source scan) | `cd frontend && npx vitest run src/components/__tests__/canvas-token-audit.test.ts -x` | ❌ W0 | ⬜ pending |

*Status: ⬜ pending · ✅ green · ❌ red · ⚠️ flaky*

---

## Wave 0 Requirements

- [ ] `frontend/src/components/__tests__/canvas-token-audit.test.ts` — scans target files for stale token patterns and hardcoded hex (covers FOUND-06, THEME-05)

*Existing `no-line-audit.test.ts` already covers COMP-12 and passes.*

---

## Manual-Only Verifications

| Behavior | Requirement | Why Manual | Test Instructions |
|----------|-------------|------------|-------------------|
| MiniMap dots reflect theme-adaptive status colors | THEME-05 | Visual rendering in ReactFlow MiniMap requires browser | Toggle theme, verify MiniMap dots change color |
| Canvas background dots adapt to theme | FOUND-06 | ReactFlow Background component rendering | Toggle theme, verify dot grid color changes |

---

## Validation Sign-Off

- [ ] All tasks have `<automated>` verify or Wave 0 dependencies
- [ ] Sampling continuity: no 3 consecutive tasks without automated verify
- [ ] Wave 0 covers all MISSING references
- [ ] No watch-mode flags
- [ ] Feedback latency < 15s
- [ ] `nyquist_compliant: true` set in frontmatter

**Approval:** pending
