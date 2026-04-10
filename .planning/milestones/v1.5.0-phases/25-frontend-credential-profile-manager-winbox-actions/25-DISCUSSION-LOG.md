# Phase 25: Frontend — Credential Profile Manager + WinBox Actions - Discussion Log

> **Audit trail only.** Do not use as input to planning, research, or execution agents.
> Decisions captured in CONTEXT.md — this log preserves the discussion.

**Date:** 2026-04-07
**Phase:** 25-frontend-credential-profile-manager-winbox-actions
**Mode:** discuss
**Areas analyzed:** Per-device assignment UI, WinBox action 3-state UX, Bridge health check display, Global credential manager

## Gray Areas Presented

| Area | Options Offered | Selected |
|------|----------------|----------|
| Per-device assignment UI | Inline in DeviceConfigPanel / Dedicated context menu item | Inline in DeviceConfigPanel |
| WinBox action bridge-down state | Disabled + tooltip / Enabled + error on click | Disabled + tooltip |
| Bridge status display | Tooltip-only / Also in SettingsPanel | Tooltip-only on WinBox button |
| Bridge health polling | On mount + 30s interval / On-demand only | On mount + 30s interval |
| Role field behavior | Free-text defaulting to 'Admin' / Free-text with suggestions | Free-text defaulting to 'Admin' |
| Manager section header | "Credential Profiles" / "Credentials" | Credential Profiles |

## Discussion Notes

### Per-device assignment UI
User confirmed the mockup: profile name + role badge + key icon WinBox toggle (filled = designated) + remove button, with "+ Add" at top right. All inline inside the existing DeviceConfigPanel.

### WinBox action states
User confirmed the 3-state model from the mockup:
- State 1: No profile → disabled, tooltip "No WinBox profile designated"
- State 2: Profile set, bridge not running → disabled, tooltip "WinBox bridge not running — download from Settings"
- State 3: Profile set + bridge running → enabled

### Bridge health check
Tooltip-only chosen (no persistent indicator in Settings). On mount + 30s interval polling. Status lives in a `useBridgeHealth` hook.

### Global credential manager
Direct rename SSHProfileManager → CredentialProfileManager. Role field added as free-text pre-filled 'Admin'. Section header: "Credential Profiles".

## Corrections Made

No corrections — all recommendations accepted.
