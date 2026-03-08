---
plan: 04-02
phase: 04-integration-and-polish
status: complete
completed_at: 2026-03-08
---

## Summary

Wired Grafana deep-link navigation from device/link context menus and built the InterfaceStatsPanel showing live TX/RX throughput for link endpoints.

## Key Files

### Created
- `frontend/src/components/InterfaceStatsPanel.tsx` — Side panel content showing both link endpoint interfaces with live TX/RX throughput, utilization bar, speed, and oper status from WebSocket snapshot data

### Modified
- `frontend/src/components/ContextMenu.tsx` — Added `disabled` prop support with visual styling
- `frontend/src/components/Canvas.tsx` — Wired "Open in Grafana" (device+link) and "Per-Interface Stats" (link) context menu items; mounted InterfaceStatsPanel in SidePanel

## Decisions

- Grafana URL sourced from settings API on mount, stored in ref (no re-fetch per render)
- Device dashboard URL convention: `{grafana_url}/d/device-{hostname-slug}`
- Link "Open in Grafana" uses the source device's hostname (convention-based)
- InterfaceStatsPanel receives full snapshot prop from Canvas — live updates without additional subscriptions
- `disabled` ContextMenu items for Grafana when URL is unconfigured

## Verification

- TypeScript: `npx tsc --noEmit` passes
- Build: `npm run build` succeeds (243 modules, 353KB)
