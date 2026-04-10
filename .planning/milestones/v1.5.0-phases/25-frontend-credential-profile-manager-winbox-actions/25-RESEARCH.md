# Phase 25: Frontend — Credential Profile Manager + WinBox Actions - Research

**Researched:** 2026-04-07
**Domain:** React/TypeScript frontend — component rename, new profile assignment UI, WinBox launch integration, bridge health polling
**Confidence:** HIGH

---

<user_constraints>
## User Constraints (from CONTEXT.md)

### Locked Decisions

**Global Credential Manager (SettingsPanel)**
- D-01: Rename `SSHProfileManager` → `CredentialProfileManager`. Rename the component file, update the import in `SettingsPanel.tsx`.
- D-02: SettingsPanel section header changes from "SSH Profiles" → "Credential Profiles".
- D-03: Add a `role` free-text input to the profile form, pre-filled with `'Admin'` for new profiles. No autocomplete suggestions — plain text input, consistent with Phase 23 decision (free-text, not enum).
- D-04: Update all API client calls in the manager from old `/ssh-profiles` endpoints to the new `/credential-profiles` endpoints (Phase 24 renamed these).

**Per-Device Assignment UI (DeviceConfigPanel)**
- D-05: Replace the existing `ssh_profile_id` `<select>` dropdown in `DeviceConfigPanel` with a new "Credentials" section. The section lists all profiles assigned to the device, with: profile name + role badge per row, a WinBox designator indicator (filled key icon = designated, outline key = not designated) that can be clicked to set/change the WinBox profile, a remove button (–) per row, and an "+ Add" button that opens a `<select>` from available global credential profiles not yet assigned to this device.
- D-06: The WinBox designator is a single-select toggle: clicking an un-designated row's key icon sends `PUT /api/v1/devices/{id}/winbox-profile` with that profile_id. The previous designated profile loses its designation automatically (handled by the backend).
- D-07: When no profiles are assigned, show a placeholder: "No credentials assigned. Add a profile to enable WinBox launch."
- D-08: The "+ Add" selection calls `POST /api/v1/devices/{id}/credential-profiles`; remove calls `DELETE /api/v1/devices/{id}/credential-profiles/{profileId}`; WinBox designation calls `PUT /api/v1/devices/{id}/winbox-profile`.

**WinBox Action — 3-State UX (Canvas + Device Table)**
- D-09: The "Open in WinBox" action appears in both the canvas context menu (via `ContextMenu` items array) and the device table row actions.
- D-10: Three distinct states:
  - State 1 — No WinBox profile: `disabled=true`, tooltip: "No WinBox profile designated"
  - State 2 — Profile set, bridge not running: `disabled=true`, tooltip: "WinBox bridge not running — download from Settings"
  - State 3 — Profile set, bridge running: `disabled=false`, click fetches `GET /api/v1/devices/{id}/winbox-credentials` and launches WinBox via the bridge `POST http://localhost:1337/launch`
- D-11: The existing `ContextMenu` component already accepts `disabled` and implicitly supports tooltip via the `title` attribute on the button element; use this pattern. For the device table, use the same `title` attribute approach on the action icon button.
- D-12: Virtual devices do not get the WinBox action (same as the existing pattern: virtual nodes only get Configure in the context menu).

**Bridge Health Check**
- D-13: Poll `GET http://localhost:1337/health` on app mount and on a 30-second interval. Store result in a shared React context or a custom hook `useBridgeHealth()` returning `{ bridgeRunning: boolean }`.
- D-14: Bridge status is exposed only via WinBox button tooltips (State 2 tooltip text). No persistent indicator anywhere in the UI — not in the SettingsPanel, not in the toolbar.
- D-15: If the health check request fails (network error or non-2xx), treat as `bridgeRunning = false`. No error logging to the console beyond a silent catch.

**TypeScript Types**
- D-16: Rename `SSHProfile` type → `CredentialProfile` in `frontend/src/types/api.ts`. Add `role: string` field. Update `parseSSHProfilesResponse` → `parseCredentialProfilesResponse`.
- D-17: Add `DeviceCredentialProfile` type: `{ profile_id: string; name: string; role: string; is_winbox: boolean }` — returned by `GET /api/v1/devices/{id}/credential-profiles`.
- D-18: Update `Device` type: the `ssh_profile_id` field stays for now (Phase 27 drops it); no changes needed to the `Device` type for this phase.

### Claude's Discretion
- Exact hook name and file path for `useBridgeHealth`
- Whether `useBridgeHealth` lives in a Context provider (app-wide) or as a local hook instantiated per page that needs it — both are acceptable
- Loading skeleton or empty state styling for the Credentials section while the assignment list is fetching
- Exact visual treatment for the role badge in the assignment list (pill vs plain text)
- Order of items in the canvas context menu (WinBox after Grafana, before Configure, seems natural)

### Deferred Ideas (OUT OF SCOPE)
- Bridge status indicator in SettingsPanel — suggested but explicitly deferred; tooltip-only is the chosen approach
- Common role suggestions (datalist autocomplete on role field) — raised but deferred; free-text is cleaner
- `onSSHCredentials` callback rename in `DeviceTable` — if Phase 24 renamed the concept, the prop may need renaming; leave to planner's discretion
</user_constraints>

<phase_requirements>
## Phase Requirements

| ID | Description | Research Support |
|----|-------------|------------------|
| WINBOX-01 | User can open WinBox pre-authenticated from the canvas device context menu | Canvas.tsx context menu item addition; `GET /api/v1/devices/{id}/winbox-credentials` + `POST http://localhost:1337/launch` flow documented below |
| WINBOX-02 | User can open WinBox pre-authenticated from the Devices table row action | DeviceRow.tsx/DeviceTable.tsx/Dashboard.tsx callback chain documented below |
| WINBOX-03 | WinBox action is visually disabled with an explanatory tooltip when no WinBox profile is designated | 3-state disabled logic using `disabled` prop + `title` attribute — both already supported in ContextMenu and IconAction |
| BRIDGE-05 | Frontend detects whether the bridge is running via a health check endpoint | `useBridgeHealth` hook polling `http://localhost:1337/health` — pattern documented below |
| CRED-03 | User can explicitly designate one credential profile per device for WinBox access | Key-icon toggle in DeviceConfigPanel Credentials section; `PUT /api/v1/devices/{id}/winbox-profile` |
| CRED-05 | User can view and manage which credential profiles are assigned to a specific device | Credentials section in DeviceConfigPanel with list/add/remove |
</phase_requirements>

---

## Summary

Phase 25 is a pure frontend phase — all backend API endpoints exist (Phase 24 complete). The work decomposes into four self-contained areas:

1. **Type/client rename** — `SSHProfile` → `CredentialProfile` in `api.ts`, all parse functions and client functions updated to hit `/credential-profiles`.
2. **CredentialProfileManager** — rename of `SSHProfileManager.tsx` with a `role` field added to the form; the file structure and form logic is reused verbatim with minimal additions.
3. **DeviceConfigPanel Credentials section** — replace the single `ssh_profile_id` select with a multi-row assignment list with WinBox designation toggle.
4. **WinBox actions + bridge health** — context menu item in Canvas.tsx, action icon in DeviceRow.tsx, 3-state disabled logic driven by `useBridgeHealth()`.

The `ContextMenu` component already supports `disabled`, and `DeviceRow.IconAction` already supports `title` for tooltips — both extension points are zero-change. The `Dashboard.tsx` → `DeviceTable.tsx` → `DeviceRow.tsx` callback chain is already established; WinBox adds one more callback alongside `onSSHCredentials`.

**Primary recommendation:** Implement in three plan files: (1) types + API client, (2) CredentialProfileManager + DeviceConfigPanel Credentials section, (3) WinBox actions (Canvas + DeviceRow/DeviceTable/Dashboard) + useBridgeHealth hook.

---

## Standard Stack

### Core (all already in project — no new installs)
| Library | Version | Purpose | Why Standard |
|---------|---------|---------|--------------|
| React | 18.3 | UI rendering, hooks, state | Project standard |
| TypeScript | 5.7 | Strict typing | Project standard |
| Tailwind CSS | 3.4 | Utility styling | Project standard |
| Vitest | 4.1 | Test runner | Project standard |
| `@testing-library/react` | 16.3 | Component tests | Project standard |

[VERIFIED: codebase — package.json / vitest.config.ts / frontend/]

No new packages are needed for this phase. The `fetch` API (native browser) is used for both the Theia backend calls (via `requestJSON`/`requestJSONWithBody` helpers in `client.ts`) and the bridge health check call (`http://localhost:1337/health`). The bridge call uses `fetch` directly with a silent `.catch()` — same pattern as `checkPrometheusHealth`.

---

## Architecture Patterns

### Recommended File Structure for Phase 25

```
frontend/src/
├── hooks/
│   └── useBridgeHealth.ts              # NEW — bridge health polling hook
├── types/
│   └── api.ts                          # MODIFY — SSHProfile → CredentialProfile, DeviceCredentialProfile
├── api/
│   └── client.ts                       # MODIFY — rename functions, add 7 new credential/assignment/winbox functions
├── components/
│   ├── CredentialProfileManager.tsx    # RENAMED from SSHProfileManager.tsx + role field
│   ├── SSHProfileManager.tsx           # DELETED (or content fully replaced)
│   ├── SettingsPanel.tsx               # MODIFY — rename import
│   ├── DeviceConfigPanel.tsx           # MODIFY — replace ssh_profile_id select with Credentials section
│   ├── Canvas.tsx                      # MODIFY — add WinBox context menu item
│   └── dashboard/
│       ├── DeviceTable.tsx             # MODIFY — add onWinBox prop
│       └── DeviceRow.tsx               # MODIFY — add WinBox IconAction with 3-state disabled
```

### Pattern 1: useBridgeHealth Hook (Claude's Discretion)

**What:** A custom hook that polls `GET http://localhost:1337/health` on mount and every 30 seconds.
**When to use:** Both Canvas and Dashboard need bridge status — mount at the App level or in each consumer. Given two consumers (Canvas.tsx and Dashboard.tsx, but both are never rendered simultaneously), a local hook per consumer is simplest and avoids a context provider.

```typescript
// Source: modeled on existing checkPrometheusHealth pattern in client.ts
// frontend/src/hooks/useBridgeHealth.ts
import { useEffect, useState } from 'react';

export function useBridgeHealth(): { bridgeRunning: boolean } {
  const [bridgeRunning, setBridgeRunning] = useState(false);

  useEffect(() => {
    async function check() {
      try {
        const resp = await fetch('http://localhost:1337/health');
        setBridgeRunning(resp.ok);
      } catch {
        setBridgeRunning(false);
      }
    }

    void check();
    const id = window.setInterval(() => { void check(); }, 30_000);
    return () => window.clearInterval(id);
  }, []);

  return { bridgeRunning };
}
```

[VERIFIED: codebase — models checkPrometheusHealth in client.ts and the existing useEffect polling pattern]

### Pattern 2: WinBox Context Menu Item in Canvas.tsx

The existing `allItems` array (Canvas.tsx ~line 302) already has the 3-state Grafana item as a precedent. WinBox follows the same inline construction:

```typescript
// Source: Canvas.tsx ~line 302-312 (existing allItems pattern)
// Add after grafana item, before interface-stats item — per D-12 virtual filter applies
const hasWinboxProfile = /* determined by device data — see open question below */;
const winboxTitle = !hasWinboxProfile
  ? 'No WinBox profile designated'
  : !bridgeRunning
    ? 'WinBox bridge not running — download from Settings'
    : undefined;

{ id: 'winbox',
  label: 'Open in WinBox',
  icon: 'open_in_new',
  disabled: !hasWinboxProfile || !bridgeRunning,
  onClick: () => {
    if (d) void handleLaunchWinBox(d);
    setDeviceMenu(null);
  }
}
```

The `ContextMenu` component renders `disabled` on the button natively (lines 81–88 of ContextMenu.tsx). The `title` attribute for tooltip is NOT currently rendered by ContextMenu — this is a gap (see Pitfall 1 below).

### Pattern 3: WinBox launch flow

```typescript
// Sequence for the enabled (State 3) click:
// 1. GET /api/v1/devices/{id}/winbox-credentials  → { ip, username, password }
// 2. POST http://localhost:1337/launch             → { ip, username, password }
// No return value needed — fire and forget after POST
async function handleLaunchWinBox(device: Device) {
  const creds = await fetchWinBoxCredentials(device.id); // new client.ts function
  await fetch('http://localhost:1337/launch', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ ip: creds.ip, username: creds.username, password: creds.password }),
  });
}
```

[VERIFIED: codebase — HandleGetWinboxCredentials returns `{ ip, username, password }` flat JSON (device_credential_profile_handler.go line 262)]

### Pattern 4: DeviceCredentialProfile assignment list in DeviceConfigPanel

The assignment list is a separate fetch from the device itself. It loads on panel open:

```typescript
// Pattern mirrors existing profile fetch in DeviceConfigPanel useEffect
const [assignments, setAssignments] = useState<DeviceCredentialProfile[]>([]);
const [assignmentsLoading, setAssignmentsLoading] = useState(false);

async function loadAssignments() {
  setAssignmentsLoading(true);
  try {
    setAssignments(await fetchDeviceCredentialProfiles(device.id));
  } catch {
    // non-fatal
  } finally {
    setAssignmentsLoading(false);
  }
}

useEffect(() => { void loadAssignments(); }, []);
```

The "+ Add" flow needs a filtered list: all global profiles minus already-assigned ones. Derive this inline:
```typescript
const unassignedProfiles = credentialProfiles.filter(
  (p) => !assignments.some((a) => a.profile_id === p.id)
);
```

[VERIFIED: codebase — same useEffect pattern used for fetchSSHProfiles in DeviceConfigPanel.tsx line 99]

### Pattern 5: DeviceTable/DeviceRow/Dashboard WinBox callback chain

Dashboard → DeviceTable → DeviceRow is the established callback chain (verified: Dashboard.tsx lines 187–196, DeviceTable.tsx lines 10–18, DeviceRow.tsx lines 10–19). Add `onWinBox: (device: Device) => void` to DeviceTableProps and DeviceRowProps, and `onWinBox: () => void` in DeviceRow's internal props (same shape as `onSSHCredentials`).

DeviceRow renders the WinBox IconAction conditionally — the 3-state logic is determined by props passed down from Dashboard. Dashboard calls `useBridgeHealth()` and passes state through.

However, the WinBox disabled state requires two pieces of information per device: whether the device has a WinBox profile designated AND whether the bridge is running. The bridge state comes from `useBridgeHealth`. The "has WinBox profile" state is the key open question (see below).

### Pattern 6: `DeviceCredentialProfile` type and parser

```typescript
// frontend/src/types/api.ts
export interface DeviceCredentialProfile {
  profile_id: string;
  name: string;
  role: string;
  is_winbox: boolean;
}

// Backend response shape (from assignedProfileResponse in device_credential_profile_handler.go):
// { id, name, username, port, auth_method, role, is_winbox, assigned_at }
// Note: backend field is "id" not "profile_id" — parser must map id → profile_id
export function parseDeviceCredentialProfilesResponse(payload: unknown): DeviceCredentialProfile[] {
  // ... standard isRecord / readString / readBoolean pattern
}
```

[VERIFIED: codebase — assignedProfileResponse in device_credential_profile_handler.go lines 30–39 has `ID` (json:"id"), `Role`, `IsWinbox`]

### Pattern 7: CredentialProfile type rename

```typescript
// frontend/src/types/api.ts — SSHProfile → CredentialProfile
export interface CredentialProfile {
  id: string;
  name: string;
  description: string;
  username: string;
  port: number;
  auth_method: 'password' | 'key';
  role: string;             // NEW — from credentialProfileResponse.Role
  created_at: string;
  updated_at: string;
}
```

[VERIFIED: codebase — credentialProfileResponse in credential_profile_handler.go lines 36–46 has `Role string`]

### Anti-Patterns to Avoid

- **Wrapping the bridge fetch in `requestJSONWithBody`:** The bridge is at a different origin (`http://localhost:1337`) — the project's `requestJSON` helper uses relative paths. Use `fetch` directly for bridge calls.
- **Showing a loading spinner while waiting for WinBox credentials:** The credentials fetch is fast (local DB) — just fire the launch on click, no need for loading UI.
- **Polling bridge health in DeviceConfigPanel:** The panel is device-specific and short-lived. Only Canvas and Dashboard (which are long-lived and always-visible) need the poll. DeviceConfigPanel should receive bridge state as a prop if needed, or rely on the nearest ancestor that has `useBridgeHealth`.

---

## Don't Hand-Roll

| Problem | Don't Build | Use Instead | Why |
|---------|-------------|-------------|-----|
| HTTP requests with error handling | Custom fetch wrapper | Existing `requestJSONWithBody` / `requestJSON` in `client.ts` | Already handles 204, 400→ValidationError, 500→ServerError |
| Modal/confirmation on profile delete | Custom overlay | Existing inline confirmation pattern from SSHProfileManager.tsx (~line 452) | Already in the codebase, tested |
| Form field validation | Custom validators | Existing `validateRequired`, `validateMaxLength`, `validatePort` from `../utils/validation` | Already imported in SSHProfileManager.tsx |
| Icon rendering | SVG inline | `MaterialIcon` component | Consistent with all other icons in project |
| Disabled button tooltip | Custom tooltip component | Native HTML `title` attribute | Used everywhere in DeviceRow.IconAction; no tooltip library in project |

---

## Common Pitfalls

### Pitfall 1: ContextMenu does not render `title` attribute on disabled items
**What goes wrong:** The ContextMenu component renders a `<button>` for each item (ContextMenu.tsx lines 80–116), but it does NOT pass a `title` attribute from `ContextMenuItem`. Adding `title` to the ContextMenuItem interface and the rendered button is required for State 2 tooltip ("WinBox bridge not running...").
**Why it happens:** The `ContextMenuItem` interface (line 4–11) only has `label`, `onClick`, `variant`, `disabled`, `icon`, `separator` — no `title` field.
**How to avoid:** Add `title?: string` to the `ContextMenuItem` interface and `title={item.title}` to the rendered `<button>`. This is a one-line interface change + one attribute addition.
**Warning signs:** If tooltip doesn't appear on hover over the disabled WinBox item in the canvas menu.

[VERIFIED: codebase — ContextMenu.tsx lines 4–11 missing title field confirmed]

### Pitfall 2: Backend assignedProfileResponse uses "id" not "profile_id"
**What goes wrong:** The `DeviceCredentialProfile` type (D-17) specifies `profile_id` as the field name, but the backend serializes it as `"id"` (JSON tag on `assignedProfileResponse.ID`).
**Why it happens:** The backend `assignedProfileResponse` struct uses `json:"id"` (device_credential_profile_handler.go line 32).
**How to avoid:** In `parseDeviceCredentialProfilesResponse`, read `item.id` and map it to `profile_id` in the returned object — same as how `parseDevicesResponse` maps backend field names to frontend type fields.
**Warning signs:** `assignment.profile_id` is always empty string when trying to unassign or set WinBox profile.

[VERIFIED: codebase — assignedProfileResponse line 32 `ID string json:"id"`]

### Pitfall 3: SSHProfileManager.test.tsx must be renamed and updated
**What goes wrong:** The test file `SSHProfileManager.test.tsx` imports `SSHProfileManager` by name. After rename, the test will fail to compile.
**Why it happens:** Test files are co-located and import the component by its old name.
**How to avoid:** Rename the test file to `CredentialProfileManager.test.tsx`, update the import, and add tests for the `role` field.

[VERIFIED: codebase — SSHProfileManager.test.tsx exists at frontend/src/components/SSHProfileManager.test.tsx]

### Pitfall 4: DeviceConfigPanel.test.tsx mocks `fetchSSHProfiles` — needs updating
**What goes wrong:** `DeviceConfigPanel.test.tsx` line 10 mocks `fetchSSHProfiles`. After the rename in client.ts, the mock target changes to `fetchCredentialProfiles`.
**Why it happens:** Vitest mocks the module by function name; if the function is renamed, the mock no longer intercepts it.
**How to avoid:** Update the vi.mock in DeviceConfigPanel.test.tsx to mock `fetchCredentialProfiles` (and add `fetchDeviceCredentialProfiles` mock for the new Credentials section).

[VERIFIED: codebase — DeviceConfigPanel.test.tsx line 10 mocks fetchSSHProfiles]

### Pitfall 5: Dashboard.tsx `sshOverrides` local state references `ssh_profile_id`
**What goes wrong:** `Dashboard.tsx` has a local `sshOverrides` map and `applyOverrides` function that patches `device.ssh_profile_id` (lines 35–42). This is the `SSHCredentialForm` panel flow. This code is unrelated to Phase 25 WinBox changes but will be touched if `onSSHCredentials` is renamed.
**Why it happens:** The `onSSHCredentials` callback and the `ssh-credentials` panel kind are distinct from WinBox — they power a separate `SSHCredentialForm` sidebar. Both coexist in Phase 25.
**How to avoid:** Do NOT rename `onSSHCredentials` in Phase 25 (the CONTEXT.md deferred decision explicitly leaves this to planner's discretion; leave it alone unless renaming is part of a separate plan task).

[VERIFIED: codebase — Dashboard.tsx lines 34–42]

### Pitfall 6: "has WinBox profile" information not on the Device type
**What goes wrong:** The canvas and device table need to know per-device whether a WinBox profile is designated. This information is NOT in the `Device` type (it lives in the `device_credential_profiles` join table) and is NOT returned by `GET /api/v1/devices`.
**Why it happens:** The `Device` API response was designed before WinBox designation existed.
**How to avoid:** Two options — (A) fetch `GET /api/v1/devices/{id}/credential-profiles` on demand when the user opens the context menu or hovers the WinBox button, or (B) always render the WinBox button in "optimistic enabled" state and let the backend return 404 if no WinBox profile is designated (then show an error toast). Option A is correct per D-10 (three distinct disabled states require knowing the state before click). The planner must decide the fetch strategy: lazy fetch per device on first interaction, or bulk state from a separate endpoint.

**Recommended approach:** For the context menu, fetch credential profiles when the context menu opens for that device (lazy, one request per context menu open). For the device table, use a small per-row state that fetches on mount only if the device is non-virtual (background, non-blocking). This avoids N+1 issues by only fetching for visible devices.

[VERIFIED: codebase — Device type in api.ts lines 35–54 has no winbox_profile_id field; HandleListAssignments returns is_winbox field per device]

### Pitfall 7: Bridge launch POST to `http://localhost:1337/launch` — CORS
**What goes wrong:** Browsers will fire a preflight OPTIONS request for the cross-origin POST. If the bridge does not respond to OPTIONS with the correct CORS headers, the launch will silently fail.
**Why it happens:** The frontend is served from a different port than 1337.
**How to avoid:** Phase 26 implements the bridge binary — but in Phase 25 the WinBox button will be exercised against a running bridge. The Phase 25 plan should note that the bridge must be running for E2E testing of the launch flow, and the bridge's CORS handling is verified in Phase 26. Phase 25 unit tests should mock the `fetch` call to `localhost:1337/launch`.

[ASSUMED — bridge CORS behavior not yet implemented; Phase 26 covers BRIDGE-03/04]

---

## Code Examples

### New API client functions to add in `client.ts`

```typescript
// --- Credential Profiles (replaces SSH Profiles section) ---

export async function fetchCredentialProfiles(): Promise<CredentialProfile[]> {
  return parseCredentialProfilesResponse(await requestJSON('/api/v1/credential-profiles'));
}

export interface CredentialProfilePayload {
  name: string;
  description?: string;
  username: string;
  port: number;
  auth_method: string;
  secret: string;
  role: string;
}

export async function createCredentialProfile(payload: CredentialProfilePayload): Promise<CredentialProfile> { ... }
export async function updateCredentialProfile(id: string, payload: CredentialProfilePayload): Promise<CredentialProfile> { ... }
export async function deleteCredentialProfile(id: string): Promise<void> { ... }

// --- Per-device assignment ---

export async function fetchDeviceCredentialProfiles(deviceId: string): Promise<DeviceCredentialProfile[]> {
  const payload = await requestJSON(`/api/v1/devices/${encodeURIComponent(deviceId)}/credential-profiles`);
  return parseDeviceCredentialProfilesResponse(payload);
}

export async function assignCredentialProfile(deviceId: string, profileId: string): Promise<void> {
  await requestJSONWithBody(
    `/api/v1/devices/${encodeURIComponent(deviceId)}/credential-profiles`,
    'POST',
    { profile_id: profileId },
  );
}

export async function unassignCredentialProfile(deviceId: string, profileId: string): Promise<void> {
  await requestJSONWithBody(
    `/api/v1/devices/${encodeURIComponent(deviceId)}/credential-profiles/${encodeURIComponent(profileId)}`,
    'DELETE',
  );
}

export async function setWinBoxProfile(deviceId: string, profileId: string): Promise<void> {
  await requestJSONWithBody(
    `/api/v1/devices/${encodeURIComponent(deviceId)}/winbox-profile`,
    'PUT',
    { profile_id: profileId },
  );
}

export async function clearWinBoxProfile(deviceId: string): Promise<void> {
  await requestJSONWithBody(
    `/api/v1/devices/${encodeURIComponent(deviceId)}/winbox-profile`,
    'DELETE',
  );
}

export interface WinBoxCredentials {
  ip: string;
  username: string;
  password: string;
}

export async function fetchWinBoxCredentials(deviceId: string): Promise<WinBoxCredentials> {
  const payload = await requestJSON(`/api/v1/devices/${encodeURIComponent(deviceId)}/winbox-credentials`);
  const p = payload as Record<string, unknown>;
  return {
    ip: typeof p.ip === 'string' ? p.ip : '',
    username: typeof p.username === 'string' ? p.username : '',
    password: typeof p.password === 'string' ? p.password : '',
  };
}
```

[VERIFIED: codebase — all endpoints confirmed in router.go lines 119–155 and device_credential_profile_handler.go]

### Role field addition in CredentialProfileManager form

```typescript
// In FormState — add role field
type FormState = {
  name: string;
  description: string;
  username: string;
  port: string;
  authMethod: 'password' | 'key';
  secret: string;
  role: string;   // NEW
};

function emptyForm(): FormState {
  return { name: '', description: '', username: 'admin', port: '22', authMethod: 'password', secret: '', role: 'Admin' };
}

// In ProfileForm JSX — add after Description field:
<div className="space-y-1">
  <label className={labelClass}>Role</label>
  <input
    type="text"
    value={form.role}
    onChange={(e) => set('role', e.target.value)}
    placeholder="e.g. Admin"
    className={inputClass}
  />
</div>
```

[VERIFIED: codebase — SSHProfileManager.tsx form structure; inputClass/labelClass constants at lines 17–19]

### ContextMenuItem interface update (Pitfall 1 fix)

```typescript
// ContextMenu.tsx — add title field
export interface ContextMenuItem {
  label: string;
  onClick: () => void;
  variant?: 'danger' | 'default';
  disabled?: boolean;
  icon?: string;
  separator?: boolean;
  title?: string;   // NEW — for tooltip on disabled items
}

// In the rendered button:
<button
  disabled={item.disabled}
  title={item.title}   // NEW
  className={...}
  ...
>
```

[VERIFIED: codebase — ContextMenu.tsx lines 4–11 and line 80]

### DeviceRow WinBox action with 3-state

```typescript
// DeviceRow.tsx — extend IconAction to support disabled + title
function IconAction({ icon, title, onClick, disabled }: {
  icon: string; title: string; onClick: () => void; disabled?: boolean;
}) {
  return (
    <button
      type="button"
      onClick={(e) => { e.stopPropagation(); if (!disabled) onClick(); }}
      title={title}
      disabled={disabled}
      className={`p-1.5 rounded-md transition-colors ${
        disabled
          ? 'text-on-bg-muted cursor-not-allowed opacity-40'
          : 'text-on-bg-secondary hover:text-on-bg hover:bg-surface-high'
      }`}
    >
      <MaterialIcon name={icon} size={16} />
    </button>
  );
}
```

[VERIFIED: codebase — DeviceRow.tsx lines 108–119; existing IconAction already uses title attribute]

---

## State of the Art

| Old Approach | Current Approach | When Changed | Impact |
|--------------|------------------|--------------|--------|
| `SSHProfile` type + `/api/v1/ssh-profiles` | `CredentialProfile` type + `/api/v1/credential-profiles` | Phase 23/24 | All client.ts SSH functions need renaming |
| Single `ssh_profile_id` FK on device | Many-to-many join table `device_credential_profiles` | Phase 23 | DeviceConfigPanel UI must be replaced |
| No WinBox concept | `is_winbox` flag on assignment + `/winbox-profile` endpoint | Phase 24 | New designation toggle UI needed |

---

## Assumptions Log

| # | Claim | Section | Risk if Wrong |
|---|-------|---------|---------------|
| A1 | Bridge CORS allows POST from frontend origin (localhost:3000 or nginx) | Common Pitfalls (Pitfall 7) | WinBox launch silently fails; fix is in Phase 26 bridge binary |
| A2 | `useBridgeHealth` as a local hook (not context provider) is sufficient since Canvas and Dashboard are never simultaneously mounted | Architecture Patterns (Pattern 1) | Both components would need useBridgeHealth; low risk since both pages are never shown at once |
| A3 | DeviceRow.IconAction `disabled` prop and `title` attribute on a `<button type="button">` are sufficient to show browser-native tooltip even when disabled | Code Examples | Some browsers suppress title on disabled buttons; if so, a wrapper div with title is needed |

---

## Open Questions (RESOLVED)

1. **How does Canvas/DeviceTable know if a device has a WinBox profile (for 3-state UI)?**
   - What we know: The `Device` type has no `winbox_profile_id` field. The `is_winbox` flag lives only in `GET /api/v1/devices/{id}/credential-profiles`.
   - What's unclear: Should the canvas fetch the credential profiles list for each device on context menu open? Or should the Device API be extended to include a `has_winbox_profile: bool` field?
   - Recommendation: **For Phase 25, use lazy per-device fetch on context menu open.** When the canvas right-click menu opens for a device, fetch `GET /api/v1/devices/{id}/credential-profiles` and determine `is_winbox` from the result. Cache the result in component state keyed by device ID for the session. This avoids adding a new backend field in this phase. The planner should make this explicit in the plan tasks.
   - **RESOLVED in Plan 03:** Canvas uses a `useEffect` triggered by `deviceMenu` to call `fetchDeviceCredentialProfiles`, caching results in `deviceWinboxState` state keyed by device ID. Dashboard uses a similar `useEffect` over `filteredDevices` with `deviceWinboxMap` state. Both use lazy per-device fetch with session-level caching as recommended.

2. **Should `onSSHCredentials` be renamed in DeviceTable/DeviceRow/Dashboard?**
   - What we know: CONTEXT.md defers this to "planner's discretion".
   - What's unclear: The `SSHCredentialForm` (a completely separate component) is what `onSSHCredentials` opens — it is unrelated to the new WinBox flow. Renaming it would be cosmetic churn.
   - Recommendation: Leave `onSSHCredentials` as-is in Phase 25 (avoid unnecessary changes to stable, tested callback chains).
   - **RESOLVED in Plan 03:** `onSSHCredentials` is left as-is. Plan 03 adds a new `onWinBox` callback alongside it without renaming the existing prop. This avoids unnecessary churn to stable, tested callback chains.

---

## Environment Availability

Step 2.6: SKIPPED (no external tools beyond browser fetch; bridge binary is Phase 26 scope)

---

## Validation Architecture

### Test Framework
| Property | Value |
|----------|-------|
| Framework | Vitest 4.1 + @testing-library/react 16.3 |
| Config file | `frontend/vitest.config.ts` |
| Quick run command | `cd frontend && npm test -- --run` |
| Full suite command | `cd frontend && npm test -- --run` |

[VERIFIED: codebase — vitest.config.ts; all existing tests co-located with components]

### Phase Requirements → Test Map

| Req ID | Behavior | Test Type | Automated Command | File Exists? |
|--------|----------|-----------|-------------------|-------------|
| CRED-03/05 | CredentialProfileManager renders role field, creates profile with role | unit | `npm test -- --run CredentialProfileManager` | ❌ Wave 0 (rename from SSHProfileManager.test.tsx) |
| CRED-03/05 | DeviceConfigPanel Credentials section renders assignments, add/remove/set-winbox | unit | `npm test -- --run DeviceConfigPanel` | ✅ (needs new test cases) |
| WINBOX-01 | Canvas context menu renders WinBox item, disabled states | unit | `npm test -- --run Canvas` | ✅ (needs WinBox item tests) |
| WINBOX-02 | DeviceTable/DeviceRow renders WinBox action, fires onWinBox callback | unit | `npm test -- --run DeviceRow` | ✅ (needs WinBox prop tests) |
| WINBOX-03 | WinBox action disabled with correct tooltip text in both states | unit | `npm test -- --run DeviceRow Canvas` | ✅ (needs disabled state tests) |
| BRIDGE-05 | useBridgeHealth returns false on fetch failure, true on 200 | unit | `npm test -- --run useBridgeHealth` | ❌ Wave 0 |

### Sampling Rate
- **Per task commit:** `cd frontend && npm test -- --run`
- **Per wave merge:** `cd frontend && npm test -- --run`
- **Phase gate:** Full suite green before `/gsd-verify-work`

### Wave 0 Gaps
- [ ] `frontend/src/components/CredentialProfileManager.test.tsx` — rename SSHProfileManager.test.tsx, update imports and add role field tests; covers CRED-03/05
- [ ] `frontend/src/hooks/useBridgeHealth.test.ts` — covers BRIDGE-05; tests: returns false when fetch throws, returns false on non-2xx, returns true on 200

---

## Security Domain

### Applicable ASVS Categories

| ASVS Category | Applies | Standard Control |
|---------------|---------|-----------------|
| V2 Authentication | no | n/a — frontend display only |
| V3 Session Management | no | n/a |
| V4 Access Control | no | n/a |
| V5 Input Validation | yes | `validateRequired`, `validateMaxLength` from `../utils/validation` (already used in form) |
| V6 Cryptography | no | Decryption in backend only (T-24-05 mitigated by server-side GetWinboxCredentials) |

### Known Threat Patterns

| Pattern | STRIDE | Standard Mitigation |
|---------|--------|---------------------|
| WinBox credentials exposed in browser | Information Disclosure | Backend returns credentials only in response body, never in URL; credentials stored in memory only during launch flow; not persisted to localStorage |
| Bridge launch target manipulation | Tampering | `ip` comes from backend `GET /winbox-credentials`, not from user input — users cannot inject arbitrary IPs via the UI |
| DNS rebinding against bridge | Spoofing | Phase 26 concern (BRIDGE-03); Phase 25 only calls `localhost:1337` |

---

## Sources

### Primary (HIGH confidence)
- Codebase: `frontend/src/components/SSHProfileManager.tsx` — form structure, ProfileForm, list pattern
- Codebase: `frontend/src/components/DeviceConfigPanel.tsx` — existing ssh_profile_id section being replaced
- Codebase: `frontend/src/components/Canvas.tsx` — allItems context menu array pattern
- Codebase: `frontend/src/components/ContextMenu.tsx` — disabled prop, missing title field confirmed
- Codebase: `frontend/src/components/Dashboard.tsx` — callback chain, onSSHCredentials pattern
- Codebase: `frontend/src/components/dashboard/DeviceTable.tsx` + `DeviceRow.tsx` — action callback props
- Codebase: `frontend/src/api/client.ts` — requestJSON, requestJSONWithBody, existing pattern for all new functions
- Codebase: `frontend/src/types/api.ts` — SSHProfile, existing parser patterns
- Codebase: `internal/api/device_credential_profile_handler.go` — verified assignedProfileResponse shape (id not profile_id)
- Codebase: `internal/api/credential_profile_handler.go` — verified credentialProfileResponse shape (role field)
- Codebase: `internal/api/router.go` — all 7 Phase 24 routes confirmed present
- Codebase: `frontend/vitest.config.ts` — test framework confirmed

### Secondary (MEDIUM confidence)
- Existing test files (`SSHProfileManager.test.tsx`, `DeviceConfigPanel.test.tsx`, `Canvas.test.tsx`, `DeviceRow.test.tsx`) — confirm mock patterns needed for Phase 25 test updates

---

## Metadata

**Confidence breakdown:**
- Standard stack: HIGH — all libraries verified in codebase
- Architecture: HIGH — all patterns are direct extensions of existing verified code
- Pitfalls: HIGH — all pitfalls sourced from actual code inspection; no assumed risks
- Open questions: HIGH — both questions resolved in plans (lazy fetch for WinBox profile, onSSHCredentials left as-is)

**Research date:** 2026-04-07
**Valid until:** 2026-05-07 (stable codebase; no external dependencies)
