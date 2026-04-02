---
phase: 10
slug: virtual-node-forms
status: draft
nyquist_compliant: false
wave_0_complete: false
created: 2026-04-01
---

# Phase 10 — Validation Strategy

> Per-phase validation contract for feedback sampling during execution.

---

## Test Infrastructure

| Property | Value |
|----------|-------|
| **Framework** | Vitest 4.1 + @testing-library/react 16.3 |
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
| 10-01-01 | 01 | 1 | VIRT-10 | unit | `cd frontend && npx vitest run src/components/AddDevicePanel.test.tsx -t "segmented"` | Existing file, needs new tests | pending |
| 10-01-02 | 01 | 1 | VIRT-11 | unit | `cd frontend && npx vitest run src/components/AddDevicePanel.test.tsx -t "virtual"` | Existing file, needs new tests | pending |
| 10-02-01 | 02 | 2 | VIRT-12 | unit | `cd frontend && npx vitest run src/components/LinkCreatePanel.test.tsx -t "virtual"` | Does not exist | pending |
| 10-02-02 | 02 | 2 | VIRT-13 | unit | `cd frontend && npx vitest run src/components/LinkCreatePanel.test.tsx -t "both virtual"` | Does not exist | pending |
| 10-02-03 | 02 | 2 | VIRT-16 | unit | `cd frontend && npx vitest run src/components/Canvas.test.tsx -t "virtual"` | Needs new tests | pending |

*Status: pending / green / red / flaky*

---

## Wave 0 Requirements

- [ ] `frontend/src/components/LinkCreatePanel.test.tsx` — stubs for VIRT-12 and VIRT-13 (virtual interface hiding + both-virtual validation)
- [ ] New test cases in `frontend/src/components/AddDevicePanel.test.tsx` — stubs for VIRT-10 and VIRT-11 (segmented control, virtual form fields)
- [ ] New test cases in `frontend/src/components/Canvas.test.tsx` — stubs for VIRT-16 (context menu filtering for virtual devices)

---

## Manual-Only Verifications

| Behavior | Requirement | Why Manual | Test Instructions |
|----------|-------------|------------|-------------------|
| Segmented control visual styling matches Neon Topography | VIRT-10 | Visual/aesthetic check | 1. Open Add Device panel 2. Verify pill toggle renders with bg-primary active state 3. Toggle between modes and verify smooth transition |
| Icon radio cards show correct Material Symbols | VIRT-11 | Icon rendering requires browser font | 1. Switch to Virtual mode 2. Verify 2x2 grid shows language/cloud/dns/hub icons 3. Select each and verify border highlight |

*All behavioral requirements have automated verification above.*

---

## Validation Sign-Off

- [ ] All tasks have `<automated>` verify or Wave 0 dependencies
- [ ] Sampling continuity: no 3 consecutive tasks without automated verify
- [ ] Wave 0 covers all MISSING references
- [ ] No watch-mode flags
- [ ] Feedback latency < 15s
- [ ] `nyquist_compliant: true` set in frontmatter

**Approval:** pending
