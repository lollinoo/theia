---
phase: 9
slug: virtual-node-rendering
status: draft
nyquist_compliant: false
wave_0_complete: false
created: 2026-03-31
---

# Phase 9 — Validation Strategy

> Per-phase validation contract for feedback sampling during execution.

---

## Test Infrastructure

| Property | Value |
|----------|-------|
| **Framework** | Vitest 4.1.0 + @testing-library/react 16.3 |
| **Config file** | `frontend/vitest.config.ts` |
| **Quick run command** | `cd frontend && npx vitest run --reporter=verbose` |
| **Full suite command** | `cd frontend && npx vitest run` |
| **Estimated runtime** | ~15 seconds |

---

## Sampling Rate

- **After every task commit:** Run `cd frontend && npx vitest run --reporter=verbose`
- **After every plan wave:** Run `cd frontend && npx vitest run`
- **Before `/gsd:verify-work`:** Full suite must be green
- **Max feedback latency:** 15 seconds

---

## Per-Task Verification Map

| Task ID | Plan | Wave | Requirement | Test Type | Automated Command | File Exists | Status |
|---------|------|------|-------------|-----------|-------------------|-------------|--------|
| 09-01-01 | 01 | 1 | VIRT-06 | unit | `cd frontend && npx vitest run src/components/DeviceCard.test.tsx -x` | ✅ (needs virtual cases) | ⬜ pending |
| 09-01-02 | 01 | 1 | VIRT-07 | unit | `cd frontend && npx vitest run src/components/DeviceCard.test.tsx -x` | ✅ (needs virtual cases) | ⬜ pending |
| 09-01-03 | 01 | 1 | VIRT-08 | unit | `cd frontend && npx vitest run src/components/DeviceCard.test.tsx -x` | ✅ (needs virtual cases) | ⬜ pending |
| 09-01-04 | 01 | 1 | VIRT-09 | unit | `cd frontend && npx vitest run src/components/MaterialIcon.test.tsx -x` | ✅ (needs glyph test) | ⬜ pending |
| 09-02-01 | 02 | 1 | VIRT-14 | unit | `cd frontend && npx vitest run src/components/canvas/edgeBuilder.test.ts -x` | ❌ W0 | ⬜ pending |
| 09-02-02 | 02 | 1 | VIRT-15 | unit | `cd frontend && npx vitest run src/components/canvas/edgeBuilder.test.ts -x` | ❌ W0 | ⬜ pending |

*Status: ⬜ pending · ✅ green · ❌ red · ⚠️ flaky*

---

## Wave 0 Requirements

- [ ] `frontend/src/components/canvas/edgeBuilder.test.ts` — stubs for VIRT-14, VIRT-15 (buildEdgeData virtual link tests)
- [ ] Add virtual device test cases to existing `frontend/src/components/DeviceCard.test.tsx` — covers VIRT-06, VIRT-07, VIRT-08
- [ ] Add font glyph verification to `frontend/src/components/MaterialIcon.test.tsx` — covers VIRT-09

*Existing infrastructure covers framework installation; only test stubs needed.*

---

## Manual-Only Verifications

| Behavior | Requirement | Why Manual | Test Instructions |
|----------|-------------|------------|-------------------|
| Visual dashed border appearance | VIRT-06 | CSS rendering not testable in JSDOM | Inspect in browser: virtual node has visible dashed border |
| Icon visual correctness | VIRT-09 | Font glyph rendering not testable in JSDOM | Verify language/cloud/dns icons render in browser at 24px |
| Status glow animation | VIRT-06 | CSS animation not testable in JSDOM | Check glow on up/down/unknown virtual nodes in browser |

---

## Validation Sign-Off

- [ ] All tasks have `<automated>` verify or Wave 0 dependencies
- [ ] Sampling continuity: no 3 consecutive tasks without automated verify
- [ ] Wave 0 covers all MISSING references
- [ ] No watch-mode flags
- [ ] Feedback latency < 15s
- [ ] `nyquist_compliant: true` set in frontmatter

**Approval:** pending
