---
phase: 02-component-restyling
plan: 05
subsystem: ui
tags: [settings-panel, profile-managers, form-panels, link-details, interface-stats, no-line-rule, form-standardization]

# Dependency graph
requires:
  - phase: 02-component-restyling
    plan: 02
    provides: "Surface tier pattern (bg-surface-high, bg-elevated) and glow conventions"
  - phase: 02-component-restyling
    plan: 04
    provides: "AlertsPanel glow dots pattern, SidePanel/Toolbar no-line styling"
provides:
  - "All panel and form components use standardized input styling (bg-elevated, border-outline-subtle, focus:ring-primary)"
  - "LinkDetailsPanel and InterfaceStatsPanel display metric values in JetBrains Mono"
  - "No-line rule enforced across all panel components"
---

# Plan 02-05 Summary: Panels and Forms Standardization

## What Changed

### Task 1: SettingsPanel, SNMPProfileManager, SSHProfileManager
- **SettingsPanel**: Replaced border separators between sections with surface tier depth (`bg-surface-high`). All form inputs standardized to `bg-elevated border-outline-subtle focus:border-primary focus:ring-primary/30`
- **SNMPProfileManager**: Extracted shared `INPUT_CLASS` and `SELECT_CLASS` constants. Removed layout borders, applied surface tiers for section depth
- **SSHProfileManager**: Same standardized input pattern. Auth method toggle buttons use `border-primary bg-primary/15` active state

### Task 2: AddDevicePanel, DeviceConfigPanel, LinkCreatePanel
- **AddDevicePanel**: Extracted shared `INPUT_CLASS` and `SELECT_CLASS` constants. All inputs standardized. No-line rule applied (removed section borders)
- **DeviceConfigPanel**: All ~20 form inputs converted to standardized pattern. Section separators replaced with surface depth tiers
- **LinkCreatePanel**: Dropdown and input styling standardized. Device selector uses `bg-elevated` with `border-outline-subtle`

### Task 3: LinkDetailsPanel and InterfaceStatsPanel
- **LinkDetailsPanel**: Metric values (bandwidth, utilization, packet stats) now render in `font-mono` (JetBrains Mono). Status badges use semantic token colors
- **InterfaceStatsPanel**: All metric values (speed, MTU, counters) use `font-mono text-[11px]`. Section headers use `text-on-bg-muted` for hierarchy

## Decisions Made
- Extracted shared `INPUT_CLASS` / `SELECT_CLASS` constants in SNMPProfileManager, SSHProfileManager, and AddDevicePanel to reduce duplication across form-heavy components
- Auth method toggle buttons (password/key) in SSHProfileManager use `border-primary bg-primary/15 text-primary` for active state (consistent with Phase 2 primary green accent)

## Files Modified
- `frontend/src/components/SettingsPanel.tsx`
- `frontend/src/components/SNMPProfileManager.tsx`
- `frontend/src/components/SSHProfileManager.tsx`
- `frontend/src/components/AddDevicePanel.tsx`
- `frontend/src/components/DeviceConfigPanel.tsx`
- `frontend/src/components/LinkCreatePanel.tsx`
- `frontend/src/components/LinkDetailsPanel.tsx`
- `frontend/src/components/InterfaceStatsPanel.tsx`

## Verification
- `npx vite build` — clean (no TS errors)
- `npx vitest run` — 72/72 tests passing
- No layout `border-[tblr]` separators in any modified file
- All form inputs use standardized `bg-elevated border-outline-subtle` pattern
- Metric values in LinkDetailsPanel and InterfaceStatsPanel contain `font-mono`
