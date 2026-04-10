# Phase 25: Frontend — Credential Profile Manager + WinBox Actions - Context

**Gathered:** 2026-04-07
**Status:** Ready for planning

<domain>
## Phase Boundary

Deliver the complete frontend for the v1.5.0 credential system and WinBox integration:
1. Rename `SSHProfileManager` → `CredentialProfileManager` with a `role` field (global CRUD in SettingsPanel)
2. Replace the single `ssh_profile_id` dropdown in `DeviceConfigPanel` with a multi-profile assignment section (list assigned profiles, add/remove, designate WinBox profile)
3. Add "Open in WinBox" to the canvas context menu and device table row, with 3-state disabled/enabled logic
4. Implement bridge health check (poll `http://localhost:1337/health` on mount + 30 s interval); expose status via WinBox button tooltips only

Requirements in scope: WINBOX-01, WINBOX-02, WINBOX-03, BRIDGE-05, CRED-03, CRED-05.

</domain>

<decisions>
## Implementation Decisions

### Global Credential Manager (SettingsPanel)
- **D-01:** Rename `SSHProfileManager` → `CredentialProfileManager`. Rename the component file, update the import in `SettingsPanel.tsx`.
- **D-02:** SettingsPanel section header changes from "SSH Profiles" → "Credential Profiles".
- **D-03:** Add a `role` free-text input to the profile form, pre-filled with `'Admin'` for new profiles. No autocomplete suggestions — plain text input, consistent with Phase 23 decision (free-text, not enum).
- **D-04:** Update all API client calls in the manager from old `/ssh-profiles` endpoints to the new `/credential-profiles` endpoints (Phase 24 renamed these).

### Per-Device Assignment UI (DeviceConfigPanel)
- **D-05:** Replace the existing `ssh_profile_id` `<select>` dropdown in `DeviceConfigPanel` with a new "Credentials" section. The section lists all profiles assigned to the device, with:
  - Profile name + role badge per row
  - A WinBox designator indicator (filled key icon = designated, outline key = not designated) that can be clicked to set/change the WinBox profile
  - A remove button (–) per row
  - An "+ Add" button that opens a `<select>` from available global credential profiles not yet assigned to this device
- **D-06:** The WinBox designator is a single-select toggle: clicking an un-designated row's key icon sends `PUT /api/v1/devices/{id}/winbox-profile` with that profile_id. The previous designated profile loses its designation automatically (handled by the backend).
- **D-07:** When no profiles are assigned, show a placeholder: "No credentials assigned. Add a profile to enable WinBox launch."
- **D-08:** The "+ Add" selection calls `POST /api/v1/devices/{id}/credential-profiles`; remove calls `DELETE /api/v1/devices/{id}/credential-profiles/{profileId}`; WinBox designation calls `PUT /api/v1/devices/{id}/winbox-profile`.

### WinBox Action — 3-State UX (Canvas + Device Table)
- **D-09:** The "Open in WinBox" action appears in both the canvas context menu (via `ContextMenu` items array) and the device table row actions.
- **D-10:** Three distinct states:
  - **State 1 — No WinBox profile:** `disabled=true`, tooltip: "No WinBox profile designated"
  - **State 2 — Profile set, bridge not running:** `disabled=true`, tooltip: "WinBox bridge not running — download from Settings"
  - **State 3 — Profile set, bridge running:** `disabled=false`, click fetches `GET /api/v1/devices/{id}/winbox-credentials` and launches WinBox via the bridge `POST http://localhost:1337/launch`
- **D-11:** The existing `ContextMenu` component already accepts `disabled` and implicitly supports tooltip via the `title` attribute on the button element; use this pattern. For the device table, use the same `title` attribute approach on the action icon button.
- **D-12:** Virtual devices do not get the WinBox action (same as the existing pattern: virtual nodes only get Configure in the context menu).

### Bridge Health Check
- **D-13:** Poll `GET http://localhost:1337/health` on app mount and on a 30-second interval. Store result in a shared React context or a custom hook `useBridgeHealth()` returning `{ bridgeRunning: boolean }`.
- **D-14:** Bridge status is exposed only via WinBox button tooltips (State 2 tooltip text). No persistent indicator anywhere in the UI — not in the SettingsPanel, not in the toolbar.
- **D-15:** If the health check request fails (network error or non-2xx), treat as `bridgeRunning = false`. No error logging to the console beyond a silent catch.

### TypeScript Types
- **D-16:** Rename `SSHProfile` type → `CredentialProfile` in `frontend/src/types/api.ts`. Add `role: string` field. Update `parseSSHProfilesResponse` → `parseCredentialProfilesResponse`.
- **D-17:** Add `DeviceCredentialProfile` type: `{ profile_id: string; name: string; role: string; is_winbox: boolean }` — returned by `GET /api/v1/devices/{id}/credential-profiles`.
- **D-18:** Update `Device` type: the `ssh_profile_id` field stays for now (Phase 27 drops it); no changes needed to the `Device` type for this phase.

### Claude's Discretion
- Exact hook name and file path for `useBridgeHealth`
- Whether `useBridgeHealth` lives in a Context provider (app-wide) or as a local hook instantiated per page that needs it — both are acceptable
- Loading skeleton or empty state styling for the Credentials section while the assignment list is fetching
- Exact visual treatment for the role badge in the assignment list (pill vs plain text)
- Order of items in the canvas context menu (WinBox after Grafana, before Configure, seems natural)

</decisions>

<canonical_refs>
## Canonical References

**Downstream agents MUST read these before planning or implementing.**

### Existing Components (being modified)
- `frontend/src/components/SSHProfileManager.tsx` — existing global manager being renamed/extended; full form and list patterns reuse here
- `frontend/src/components/DeviceConfigPanel.tsx` — existing device config panel; the ssh_profile_id dropdown section is being replaced (search for `sshProfileId` and `ssh_profile_id` in this file)
- `frontend/src/components/Canvas.tsx` — canvas context menu items array (~line 302); WinBox item added here; virtual device filter applied here
- `frontend/src/components/ContextMenu.tsx` — context menu component; `disabled` prop and icon pattern already present
- `frontend/src/components/SettingsPanel.tsx` — renders `SSHProfileManager`; rename import here
- `frontend/src/components/dashboard/DeviceTable.tsx` — device table with row actions; `onSSHCredentials` callback is the current row action pattern

### Types + API Client
- `frontend/src/types/api.ts` — `SSHProfile` type (line ~300) being renamed to `CredentialProfile` with `role` field added
- `frontend/src/api/client.ts` — API call functions; new credential profile + assignment + bridge functions added here

### Backend API Contracts (Phase 24 output)
- `internal/api/credential_profile_handler.go` — credential profile CRUD (`/api/v1/credential-profiles`)
- `internal/api/router.go` — all route registrations including the 7 new Phase 24 routes

### Requirements
- `.planning/REQUIREMENTS.md` §Credential Profiles — CRED-03, CRED-05
- `.planning/REQUIREMENTS.md` §WinBox Bridge — BRIDGE-05
- `.planning/REQUIREMENTS.md` §WinBox UI — WINBOX-01, WINBOX-02, WINBOX-03

</canonical_refs>

<code_context>
## Existing Code Insights

### Reusable Assets
- `ContextMenu` component (`ContextMenu.tsx`) — already handles `disabled`, `icon`, `separator`, `variant` props; no changes needed to the component itself; just add the WinBox item to the items array in `Canvas.tsx`
- `ProfileForm` pattern in `SSHProfileManager.tsx` — the form component with inline field validation, error display, and save/cancel buttons; reuse directly with role field addition
- `MaterialIcon` component — already used in context menu items; use `open_in_new` or `key` icon for WinBox action
- `writeError` / `ServerError` / `ValidationError` patterns — consistent error handling already in the form

### Established Patterns
- Context menu items are built inline in `Canvas.tsx` as a `(ContextMenuItem & { id: string })[]` array, then filtered for virtual nodes — follow this exact pattern for the WinBox item
- `DeviceConfigPanel` fetches data on mount with `useEffect(() => { void load(); }, [])` and keeps local state arrays — same pattern for the assignment list fetch
- Device table row actions use callback props (`onSSHCredentials`, `onBackup`, etc.) passed down from `Dashboard.tsx` — add `onWinBox: (device: Device) => void` following this pattern
- All form inputs use the shared `inputClass` and `labelClass` constants — reuse in the updated manager

### Integration Points
- `SettingsPanel.tsx:~line 378-382` — renders `SSHProfileManager`; rename the import and component usage to `CredentialProfileManager`
- `Canvas.tsx:~line 302-312` — allItems array for the context menu; add WinBox item here, filtered out for virtual devices
- `DeviceConfigPanel.tsx:~line 63-64, 99, 134, 246, 473-484` — all `sshProfileId` / `ssh_profile_id` references being replaced
- `api/client.ts` — needs: `fetchCredentialProfiles`, `createCredentialProfile`, `updateCredentialProfile`, `deleteCredentialProfile`, `fetchDeviceCredentialProfiles`, `assignCredentialProfile`, `unassignCredentialProfile`, `setWinBoxProfile`, `clearWinBoxProfile`, `fetchWinBoxCredentials`
- Bridge: `http://localhost:1337/health` (GET, health check) and `http://localhost:1337/launch` (POST with `{ ip, username, password }`)

</code_context>

<specifics>
## Specific Ideas

- Mockup confirmed: DeviceConfigPanel Credentials section shows profile name + role badge, key icon WinBox toggle (filled = designated), and minus button per row, with an "+ Add" button at the top right
- WinBox tooltip text confirmed: "No WinBox profile designated" (State 1) and "WinBox bridge not running — download from Settings" (State 2)
- Role field: plain free-text input, pre-filled 'Admin', no autocomplete

</specifics>

<deferred>
## Deferred Ideas

- Bridge status indicator in SettingsPanel — suggested but explicitly deferred; tooltip-only is the chosen approach
- Common role suggestions (datalist autocomplete on role field) — raised but deferred; free-text is cleaner
- `onSSHCredentials` callback rename in `DeviceTable` — if Phase 24 renamed the concept, the prop may need renaming; leave to planner's discretion

</deferred>

---

*Phase: 25-frontend-credential-profile-manager-winbox-actions*
*Context gathered: 2026-04-07*
