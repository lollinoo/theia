---
phase: 02-component-restyling
plan: 06
subsystem: ui
tags: [dashboard, device-table, device-row, backup-panel, config-viewer, ssh-credential-form, vendor-settings, no-line-audit, theme-05]

# Dependency graph
requires:
  - phase: 02-component-restyling
    plan: 02
    provides: "Surface tier pattern, glow conventions, DeviceCard/StatusDot restyling"
  - phase: 02-component-restyling
    plan: 04
    provides: "Navigation control styling, AlertsPanel glow dots"
  - phase: 02-component-restyling
    plan: 03
    provides: "Overlay glassmorphism pattern, ContextMenu icons/separators"
provides:
  - "Dashboard and all sub-components restyled with Neon Topography tokens"
  - "Full no-line audit passed — zero layout border separators remain"
  - "THEME-05 verified — zero hardcoded hex colors, all components use token system"
---

# Plan 02-06 Summary: Dashboard + No-Line Audit + THEME-05

## What Changed

### Task 1: Dashboard, DeviceTable, DeviceRow
- **Dashboard**: Filter bar inputs standardized to `bg-elevated border-outline-subtle`. Tab buttons use primary green active state. No `border-b` on filter bar
- **DeviceTable**: Header row uses `bg-surface-high` instead of border-bottom. Sort icons use `text-on-bg-muted`
- **DeviceRow**: Status dot uses `bg-{status}` token colors. Action buttons use `text-on-bg-secondary hover:text-on-bg`. Row hover uses `hover:bg-surface-high/50`
- Added 2 new Dashboard tests (no-border filter bar, token class verification)

### Task 2a: BackupPanel, BackupHistoryTable, BulkBackupPanel, ConfigViewer
- **BackupPanel**: Section headers use surface tier depth. Action buttons standardized to token colors
- **BackupHistoryTable**: Table header uses `bg-surface-high`. Status badges use semantic token colors
- **BulkBackupPanel**: Progress indicators use primary green. Border separators removed
- **ConfigViewer**: Code display uses `font-mono bg-surface-high`. Tab controls use `border-primary` active state

### Task 2b: SSHCredentialForm, VendorSettingsPanel, Canvas.tsx
- **SSHCredentialForm**: Extracted shared `INPUT_CLASS` / `SELECT_CLASS` constants. Auth method toggle uses primary green active styling
- **VendorSettingsPanel**: OID input fields use `font-mono`. Extracted shared input class constant
- **Canvas.tsx**: Overlay elements (loading spinner, empty state) use token colors. Loading spinner uses `border-outline border-t-primary`

### Task 3: No-Line Audit + THEME-05 Verification
- **No-line audit**: Grep for `border-[tblr]` across all component files — only functional borders remain (spinner animations, test assertions). Zero layout separators found
- **THEME-05**: Grep for hardcoded hex colors (`#[0-9a-fA-F]`) across all component files — zero matches. All colors use Neon Topography token system
- Build and full test suite pass cleanly

## Decisions Made
- Retained spinner animation borders (`border-t-primary` on rotating elements) as functional, not subject to no-line rule
- Retained semantic status borders on Prometheus status cards (`border-red`, `border-yellow`) per prior decision in 02-04

## Files Modified
- `frontend/src/components/Dashboard.tsx`
- `frontend/src/components/Dashboard.test.tsx`
- `frontend/src/components/dashboard/DeviceTable.tsx`
- `frontend/src/components/dashboard/DeviceRow.tsx`
- `frontend/src/components/dashboard/BackupPanel.tsx`
- `frontend/src/components/dashboard/BackupHistoryTable.tsx`
- `frontend/src/components/dashboard/BulkBackupPanel.tsx`
- `frontend/src/components/dashboard/ConfigViewer.tsx`
- `frontend/src/components/dashboard/SSHCredentialForm.tsx`
- `frontend/src/components/dashboard/VendorSettingsPanel.tsx`
- `frontend/src/components/Canvas.tsx`

## Verification
- `npx vite build` — clean (no TS errors)
- `npx vitest run` — 72/72 tests passing (includes 2 new Dashboard tests)
- No-line audit: `grep -r "border-[tblr]"` returns only functional borders (spinners) and test assertions
- THEME-05: `grep -r "#[0-9a-fA-F]{3,8}"` returns zero matches in component files
- Both dark and light themes build correctly with complete token coverage
