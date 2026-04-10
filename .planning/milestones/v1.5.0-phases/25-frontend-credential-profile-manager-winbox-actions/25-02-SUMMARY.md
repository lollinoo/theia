---
phase: 25-frontend-credential-profile-manager-winbox-actions
plan: "02"
subsystem: frontend
tags: [react, credentials, winbox, device-config]
dependency_graph:
  requires: ["25-01"]
  provides: ["25-03"]
  affects: ["frontend/src/components/DeviceConfigPanel.tsx"]
tech_stack:
  added: []
  patterns: ["useState for multi-assignment list", "async load-on-mount pattern", "inline confirmation pattern"]
key_files:
  created: []
  modified:
    - frontend/src/components/DeviceConfigPanel.tsx
    - frontend/src/components/DeviceConfigPanel.test.tsx
decisions:
  - "Credentials section replaces ssh_profile_id select dropdown entirely — assignment lifecycle managed via separate API calls"
  - "loadAssignments() defined as inner async function so it can be called both in useEffect mount and after each assignment mutation"
  - "Role pill badge uses rounded-full with bg-surface background to distinguish it from device status badges"
metrics:
  duration: "~20 minutes"
  completed_at: "2026-04-08T10:43:55Z"
  tasks_completed: 1
  tasks_total: 1
  files_changed: 2
---

# Phase 25 Plan 02: Credentials Section in DeviceConfigPanel Summary

Replaced the single `ssh_profile_id` select dropdown in DeviceConfigPanel with a full multi-profile assignment section supporting add, remove, and WinBox designation toggle per profile.

## What Was Implemented

### Part A: Removed old ssh_profile_id infrastructure
- Removed `sshProfileId` and `setSSHProfileId` state variables
- Removed `sshProfiles` state (renamed to `credentialProfiles`)
- Removed `ssh_profile_id` from the `updateDevice` save payload in `handleEditSave`
- Removed `setSSHProfileId(device.ssh_profile_id || '')` from the device-prop-sync `useEffect`
- Removed the old SSH Profile `<select>` dropdown JSX

### Part B: Added Credentials section state
- `assignments: DeviceCredentialProfile[]` — list of profiles currently assigned to the device
- `assignmentsLoading: boolean` — loading state for the assignment list
- `showAddSelect: boolean` — toggle for the "+ Add" inline select
- `removingId: string | null` — tracks which row has an open inline removal confirmation

### Part C: Assignment data loading
- `loadAssignments()` async function calls `fetchDeviceCredentialProfiles(device.id)` and updates state
- Called in the initial `useEffect` mount alongside other data loading
- Re-called after each assignment mutation (assign, unassign, WinBox toggle)

### Part D: Handler functions
- `handleAssign(profileId)` — calls `assignCredentialProfile`, collapses add select, reloads
- `handleUnassign(profileId)` — calls `unassignCredentialProfile`, clears `removingId`, reloads
- `handleToggleWinBox(profileId, currentlyDesignated)` — calls `setWinBoxProfile` or `clearWinBoxProfile` based on current state, reloads

### Part E: Credentials section JSX
The Credentials section renders:
- Section header "Credentials" with "+ Add" button on the right
- When `showAddSelect`: inline `<select>` pre-filtered to unassigned profiles, plus "Dismiss" button
- Empty state: "No credentials assigned. Add a profile to enable WinBox launch." (locked D-07 copy)
- Loading state: "Loading credentials..."
- Per-assignment card (`rounded-lg bg-surface-high p-3`) with:
  - Profile name (`text-sm font-medium`) + role pill badge (`rounded-full`)
  - WinBox key toggle: `text-primary` when designated, `text-on-bg-secondary` otherwise, with correct title attributes
  - Remove button: opens inline confirmation
  - Inline confirmation: "Delete this profile?" with "Keep Profile" / "Delete" buttons

### Part F: Updated tests
- Updated `vi.mock('../api/client')` to include all new functions: `fetchDeviceCredentialProfiles`, `assignCredentialProfile`, `unassignCredentialProfile`, `setWinBoxProfile`, `clearWinBoxProfile`
- Added default mock data for `fetchCredentialProfiles` (2 profiles)
- Added 3 new tests in `DeviceConfigPanel — Credentials section` describe block:
  1. "renders credentials section with assigned profiles" — verifies name and role badge appear
  2. "shows empty state when no profiles assigned" — verifies locked D-07 copy
  3. "shows Add select when + Add is clicked" — verifies select dropdown and Dismiss button appear

## Files Changed

| File | Change |
|------|--------|
| `frontend/src/components/DeviceConfigPanel.tsx` | Replaced SSH Profile dropdown with full Credentials section |
| `frontend/src/components/DeviceConfigPanel.test.tsx` | Updated mocks, added 3 new credential tests |

## Deviations from Plan

None - plan executed exactly as written.

## Test Results

```
Test Files  40 passed (40)
      Tests  431 passed (431)
   Duration  11.29s
```

All tests pass. TypeScript compiles with zero errors.

## Self-Check

Files exist:
- `frontend/src/components/DeviceConfigPanel.tsx` — FOUND
- `frontend/src/components/DeviceConfigPanel.test.tsx` — FOUND

Commits exist:
- `cfe44fe` — feat(25-02): replace ssh_profile_id dropdown with Credentials section

## Self-Check: PASSED
