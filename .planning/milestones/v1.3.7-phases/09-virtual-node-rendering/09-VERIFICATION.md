---
status: passed
phase: 09-virtual-node-rendering
verifier: gsd-verifier
verified_at: "2026-04-01T19:59:00.000Z"
score: 6/6
requirements: [VIRT-06, VIRT-07, VIRT-08, VIRT-09, VIRT-14, VIRT-15]
---

# Phase 9: Virtual Node Rendering — Verification Report

## Must-Have Truths

| # | Truth | Status | Evidence |
|---|-------|--------|----------|
| 1 | Virtual node renders as compact card with correct Material Symbol icon for its subtype | PASS | `DeviceCard.tsx` `if (data.isVirtual)` branch with `subtypeIconMap` mapping internet->language, cloud->cloud, server->dns, generic->hub; `MaterialIcon` rendered at `size={24}` |
| 2 | Virtual node with IP shows StatusDot next to display name and IP address line in a 200px card | PASS | Conditional `w-[200px]` class, `<StatusDot status={statusForDot} />` rendered when `hasIP`, IP body section with `data.device.ip` |
| 3 | Virtual node without IP shows only icon and label in a 160px card with no body section | PASS | `w-[160px]` class when `!hasIP`, body section gated on `{hasIP && (...)}` |
| 4 | Material Symbols font subset includes language, cloud, and dns glyphs | PASS | `subset-material-icons.sh` UNICODES string includes U+E894 (language), U+E2BD (cloud), U+E875 (dns); woff2 file non-empty; `index.css` updated to "24 icons" |
| 5 | Link edge connecting to virtual node displays real interface tx/rx throughput | PASS | `findLinkMetrics` in `canvasHelpers.ts` falls back to `link.target_device_id` when source has no metrics |
| 6 | Link bandwidth label for virtual links shows only real interface speed with no mismatch indicator | PASS | `edgeBuilder.ts` `isVirtualLink` early-return sets `speedMismatch: false`, uses `sourceIsVirtual ? targetInterface : sourceInterface` for real speed |

## Requirement Coverage

| Req ID | Description | Plan | Status |
|--------|-------------|------|--------|
| VIRT-06 | Virtual node subtype icons | 09-01 | Verified |
| VIRT-07 | IP-bearing virtual card (200px, StatusDot) | 09-01 | Verified |
| VIRT-08 | No-IP virtual card (160px, icon-only) | 09-01 | Verified |
| VIRT-09 | Font subset with required glyphs | 09-01 | Verified |
| VIRT-14 | Virtual link throughput display | 09-02 | Verified |
| VIRT-15 | Virtual link bandwidth (no mismatch) | 09-02 | Verified |

## Test Suite

- 209 tests across 31 files -- all pass
- 6 new virtual DeviceCard tests (VIRT-06, VIRT-07, VIRT-08)
- 3 new MaterialIcon glyph tests (VIRT-09)
- 7 new edgeBuilder virtual link tests (VIRT-14, VIRT-15)
- 0 regressions

## Human Verification

Two items require visual verification on the live canvas:
1. Font glyphs (language, cloud, dns) render correctly in the browser
2. Edge labels display correct bandwidth and throughput for virtual links
