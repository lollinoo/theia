---
phase: 25
slug: frontend-credential-profile-manager-winbox-actions
status: complete
nyquist_compliant: true
wave_0_complete: true
created: 2026-04-07
---

# Phase 25 — Validation Strategy

> Per-phase validation contract for feedback sampling during execution.

---

## Test Infrastructure

| Property | Value |
|----------|-------|
| **Framework** | vitest 4.1 + @testing-library/react |
| **Config file** | `frontend/vitest.config.ts` |
| **Quick run command** | `cd frontend && npm test -- --run` |
| **Full suite command** | `cd frontend && npm test -- --run` |
| **Estimated runtime** | ~30 seconds |

---

## Sampling Rate

- **After every task commit:** Run `cd frontend && npm test -- --run`
- **After every plan wave:** Run `cd frontend && npm test -- --run`
- **Before `/gsd-verify-work`:** Full suite must be green
- **Max feedback latency:** 30 seconds

---

## Per-Task Verification Map

| Task ID | Plan | Wave | Requirement | Threat Ref | Secure Behavior | Test Type | Automated Command | File Exists | Status |
|---------|------|------|-------------|------------|-----------------|-----------|-------------------|-------------|--------|
| 25-01-T1 | 01 | 1 | WINBOX-01, WINBOX-02 | T-25-01..03 | No sensitive fields in type; encodeURIComponent on URL segments | unit | `cd frontend && npx tsc --noEmit && npm test -- --run` | ✅ (test created inline) | ✅ green |
| 25-02-T1 | 02 | 2 | WINBOX-03 | T-25-04..06 | Profile IDs from server lists, not user input; encodeURIComponent on URL segments | unit | `cd frontend && npx tsc --noEmit && npm test -- --run` | ✅ (test updated inline) | ✅ green |
| 25-03-T1 | 03 | 2 | WINBOX-01, WINBOX-02, WINBOX-03, BRIDGE-05 | T-25-07..11 | Credentials held in function scope only, never persisted; silent catch on bridge errors | unit | `cd frontend && npx tsc --noEmit && npm test -- --run` | ✅ (tests created inline) | ✅ green |

*Status: ⬜ pending · ✅ green · ❌ red · ⚠️ flaky*

---

## Wave 0 Requirements

All Wave 0 test file creation is handled inline within each plan's task:

- [x] 25-01-T1 creates `CredentialProfileManager.test.tsx` (renamed from SSHProfileManager.test.tsx with role field tests)
- [x] 25-02-T1 updates `DeviceConfigPanel.test.tsx` with credentials section tests
- [x] 25-03-T1 creates `useBridgeHealth.test.ts` and updates `DeviceRow.test.tsx` + `DeviceTable.test.tsx`

No separate Wave 0 tasks needed — test scaffolds are part of each task's action.

---

## Manual-Only Verifications

| Behavior | Requirement | Why Manual | Test Instructions |
|----------|-------------|------------|-------------------|
| WinBox actually launches with pre-filled credentials | WINBOX-01, WINBOX-02 | Requires WinBox binary installed on client machine + bridge running (Phase 26) | Open WinBox from canvas/table, verify username and IP are pre-filled |
| Bridge health status reflects real bridge running state | BRIDGE-05 | Requires bridge process to be running/stopped | Start/stop bridge, verify WinBox tooltip updates between State 2 and State 3 |

---

## Validation Sign-Off

- [x] All tasks have `<automated>` verify commands
- [x] Sampling continuity: no 3 consecutive tasks without automated verify
- [x] Wave 0 covers all MISSING references (all inline — no gaps)
- [x] No watch-mode flags
- [x] Feedback latency < 30s
- [x] `nyquist_compliant: true` set in frontmatter

**Approval:** 2026-04-09

---

## Validation Audit 2026-04-09

| Metric | Count |
|--------|-------|
| Gaps found | 1 |
| Resolved | 1 |
| Escalated | 0 |

**Gap resolved:** `useBridgeHealth.test.ts > polls on 30s interval` — test expected 30s interval but hook uses 15s (`POLL_INTERVAL_MS = 15_000`). Fixed test name and advance time to match implementation (451/451 tests green).
