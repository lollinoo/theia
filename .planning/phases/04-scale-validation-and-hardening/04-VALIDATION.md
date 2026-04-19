---
phase: 4
slug: scale-validation-and-hardening
status: complete
nyquist_compliant: true
wave_0_complete: true
created: 2026-04-19
verified_at: 2026-04-19T14:11:47Z
---

# Phase 4 — Validation Strategy

> Per-phase validation contract for feedback sampling during execution.

---

## Test Infrastructure

| Property | Value |
|----------|-------|
| **Framework** | `go test` + Vitest + Docker/PostgreSQL manual stack run |
| **Config file** | `docker-compose.yml`, `docker-compose.wisp-lab.yml`, `frontend/vitest.config.ts`, none for Go beyond package-local tests |
| **Quick run command** | `go test ./internal/scalelab ./internal/worker ./internal/ws ./internal/observability -count=1 && (cd frontend && npm test -- --run src/components/canvas/useCanvasData.test.ts src/hooks/useWebSocket.test.ts)` |
| **Full suite command** | `go test ./internal/scalelab ./internal/worker ./internal/ws ./internal/observability -count=1 && (cd frontend && npm test -- --run src/components/canvas/useCanvasData.test.ts src/hooks/useWebSocket.test.ts && npm run build) && go run ./cmd/theia-scale-lab -profile 300 -scenario baseline` |
| **Estimated runtime** | ~90 seconds |

---

## Sampling Rate

- **After every task commit:** Run `go test ./internal/scalelab ./internal/worker ./internal/ws ./internal/observability -count=1` for backend or harness changes, or `cd frontend && npm test -- --run src/components/canvas/useCanvasData.test.ts src/hooks/useWebSocket.test.ts` for frontend refresh-path changes
- **After every plan wave:** Run `go test ./internal/scalelab ./internal/worker ./internal/ws ./internal/observability -count=1 && (cd frontend && npm test -- --run src/components/canvas/useCanvasData.test.ts src/hooks/useWebSocket.test.ts && npm run build) && go run ./cmd/theia-scale-lab -profile 300 -scenario baseline`
- **Before `$gsd-verify-work`:** Full suite must be green
- **Max feedback latency:** 90 seconds

---

## Per-Task Verification Map

| Task ID | Plan | Wave | Requirement | Threat Ref | Secure Behavior | Test Type | Automated Command | File Exists | Status |
|---------|------|------|-------------|------------|-----------------|-----------|-------------------|-------------|--------|
| 04-01 | 01 | 1 | SCAL-01, SCAL-02, SCAL-03 | T-04-01, T-04-02 | The validation harness reaches the 300-device target, locks the exact `300` and `wisp-hybrid` contract, and records stable JSON or metrics artifacts instead of ad hoc notes | unit | `go test ./internal/scalelab -count=1 && go run ./cmd/theia-scale-lab -profile 300 -scenario baseline` | ✅ | ✅ green |
| 04-02 | 02 | 2 | SCAL-01, SCAL-02, SCAL-03 | T-04-03, T-04-04 | Any in-scope hardening change is covered by targeted regressions and final verification records exact evidence files plus any deferred gaps | unit | `go test ./internal/worker ./internal/ws ./internal/observability -count=1 && (cd frontend && npm test -- --run src/components/canvas/useCanvasData.test.ts src/hooks/useWebSocket.test.ts)` | ✅ | ✅ green |

*Status: ✅ green · ❌ red · ⚠️ flaky*

---

## Wave 0 Requirements

Existing infrastructure covers all phase requirements.

---

## Manual-Only Verifications

None. Phase 4's live browser proof and live Prometheus success or timeout proof are now recorded in the committed evidence set and cited directly by `04-VERIFICATION.md`.

---

## Validation Sign-Off

- [x] All tasks have automated verification or an explicit manual-only reason
- [x] Sampling continuity: no 3 consecutive tasks without automated verify
- [x] Wave 0 covers all missing references
- [x] No watch-mode flags
- [x] Feedback latency < 90s
- [x] `nyquist_compliant: true` set in frontmatter

**Approval:** passed on 2026-04-19 after the Nyquist audit closed the remaining exact-contract coverage gaps and confirmed the completed live evidence set.

## Validation Audit 2026-04-19

| Metric | Count |
|--------|-------|
| Gaps found | 2 |
| Resolved | 2 |
| Escalated | 0 |
