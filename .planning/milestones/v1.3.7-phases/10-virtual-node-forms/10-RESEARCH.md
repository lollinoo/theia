# Phase 10: Virtual Node Forms - Research

**Researched:** 2026-04-01
**Domain:** React form components, conditional rendering, frontend validation, context menu filtering
**Confidence:** HIGH

## Summary

Phase 10 is a purely frontend phase that modifies four existing components (AddDevicePanel, LinkCreatePanel, Canvas context menu, and client.ts payload) to support virtual node creation and linking. The backend already fully supports virtual devices (Phase 8) -- the `createDeviceRequest` struct accepts `device_type`, the link handler rejects both-virtual links and allows empty `if_name` for virtual sides. The rendering layer (Phase 9) already handles virtual cards via `isVirtual` flag in `nodeBuilder.ts`. This phase bridges the gap by giving users the UI to create and link virtual nodes.

All changes are in `frontend/src/` -- no Go changes needed. The existing conditional rendering patterns in `AddDevicePanel.tsx` (boolean flags like `isV3`, `needsAuth`, `usesPrometheus` that drive `{condition && <JSX>}` blocks) are the established pattern and should be extended with an `isVirtual` flag. The `ContextMenuItem[]` array in Canvas.tsx is data-driven, making context menu filtering a simple `.filter()` on the items array.

**Primary recommendation:** Extend existing form components with a virtual mode toggle, reuse the project's established conditional rendering pattern, and keep all validation inline (no new shared utility files).

<user_constraints>
## User Constraints (from CONTEXT.md)

### Locked Decisions
- D-01: AddDevicePanel gets a segmented control (two-segment pill) at the top: "Physical Device" | "Virtual Node". Selecting one swaps the entire field set below.
- D-02: Physical mode shows the current full form (IP, SNMP, Prometheus, vendor, SSH, areas). Virtual mode shows only: Display Name (required), Subtype (required), IP (optional), Areas (optional).
- D-03: Switching modes resets all form fields to defaults. No state preserved between modes. Clean slate prevents confusing leftover values.
- D-04: Subtypes presented as a 2x2 grid of icon radio cards. Each card shows the Material Symbol icon + label (Internet/language, Cloud/cloud, Server/dns, Generic/hub).
- D-05: Selected card has primary-color border and subtle background highlight. Default selection: Internet (first card).
- D-06: When a virtual device is selected on either side of LinkCreatePanel, its interface selector is hidden entirely. A "(virtual node -- no interface)" label replaces it.
- D-07: Only the physical device's interface selector remains visible and required.
- D-08: Frontend inline validation: when both devices are virtual, show error message "At least one device must be physical" below the second device selector and disable the Create button. No server roundtrip needed.
- D-09: Virtual node context menu hides "Open WebFig" and "Per-Interface Stats" entirely. Virtual nodes show only "Open in Grafana" and "Configure".
- D-10: Physical device context menu unchanged (all 4 items).
- D-11: `CreateDevicePayload` in `client.ts` needs a `device_type` field. Virtual form submits `device_type: "virtual"` with `tags: { display_name, virtual_subtype }`. Physical form omits `device_type` (backend defaults to auto-detection).

### Claude's Discretion
- Segmented control styling details (exact Tailwind classes, active/inactive states)
- Icon radio card internal layout and spacing
- How to structure the conditional rendering in AddDevicePanel (inline branches vs extracted sub-components)
- Whether to extract virtual validation as a shared util or inline it

### Deferred Ideas (OUT OF SCOPE)
None -- discussion stayed within phase scope.
</user_constraints>

<phase_requirements>
## Phase Requirements

| ID | Description | Research Support |
|----|-------------|------------------|
| VIRT-10 | AddDevicePanel has Physical Device / Virtual Node toggle at top of form | D-01 segmented control; existing conditional pattern via boolean flags (isV3, usesPrometheus); form field swapping on mode change |
| VIRT-11 | Virtual form shows subtype radio group, required display name, optional IP | D-02/D-04/D-05 subtype cards; MaterialIcon component reuse; backend validates display_name + virtual_subtype in tags |
| VIRT-12 | LinkCreatePanel hides interface selector for virtual side | D-06/D-07; device.device_type === 'virtual' check on selected device; replace InterfaceSelect with label |
| VIRT-13 | Link creation rejects both devices being virtual | D-08 frontend inline validation; backend also rejects (defense-in-depth); disable Create button + show error |
| VIRT-16 | Canvas context menu for virtual nodes omits WebFig and Per-Interface Stats | D-09/D-10; filter ContextMenuItem[] array in Canvas.tsx based on device.device_type |
</phase_requirements>

## Standard Stack

No new dependencies needed. All work uses existing libraries already in the project.

### Core (already installed)
| Library | Version | Purpose | Why Standard |
|---------|---------|---------|--------------|
| React | ^18.3.1 | Component framework | Already in project |
| @xyflow/react | ^12.10.1 | Canvas / ReactFlow | Already in project |
| Vitest | ^4.1.0 | Test runner | Already in project |
| @testing-library/react | ^16.3.2 | Component testing | Already in project |
| Tailwind CSS | 3.4 (via v4 @import) | Styling | Already in project |

### Supporting (already available)
| Component | Location | Purpose | Reuse For |
|-----------|----------|---------|-----------|
| MaterialIcon | `components/MaterialIcon.tsx` | Renders Material Symbols icons | Subtype icon cards (language, cloud, dns, hub) |
| StatusDot | `components/StatusDot.tsx` | Status indicator dot | Already in virtual card rendering |
| ContextMenu | `components/ContextMenu.tsx` | Data-driven menu renderer | Unchanged -- just filter the items array |
| SearchableDeviceSelect | Inside `LinkCreatePanel.tsx` | Device dropdown with search | Unchanged -- already works with all devices |

**Installation:** None needed -- all dependencies already present.

## Architecture Patterns

### Existing Component Structure (AddDevicePanel.tsx)
```
AddDevicePanel.tsx (474 lines)
  - 20+ useState hooks for form fields
  - useEffect for API data loading (profiles, areas, prometheus)
  - Boolean flags drive conditional rendering:
    isV3, needsAuth, needsPriv, usesPrometheus, usesSNMP
  - handleSubmit builds payload conditionally
  - Single <form> with {condition && <JSX>} blocks
```

### Pattern 1: Virtual Mode Toggle
**What:** Add `isVirtual` boolean state (derived from a `mode` state) at the top of AddDevicePanel. This mirrors the existing `isV3` / `usesSNMP` pattern already used for conditional field rendering.
**When to use:** For the Physical/Virtual segmented control.
**Example:**
```typescript
// Existing pattern in AddDevicePanel.tsx:
const isV3 = version === '3';
const needsAuth = securityLevel === 'authNoPriv' || securityLevel === 'authPriv';

// Extension for virtual mode:
type DeviceMode = 'physical' | 'virtual';
const [deviceMode, setDeviceMode] = useState<DeviceMode>('physical');
const isVirtual = deviceMode === 'virtual';
```

### Pattern 2: Segmented Control Component
**What:** A two-segment pill toggle rendered at the top of the form. Inline component or extracted -- Claude's discretion.
**When to use:** For the Physical Device / Virtual Node toggle.
**Example:**
```typescript
// Segmented control following project design tokens:
// Inactive: bg-surface text-on-bg-secondary
// Active: bg-primary text-white (or text-on-primary)
<div className="flex rounded-lg bg-surface p-0.5">
  <button
    type="button"
    className={`flex-1 rounded-md px-3 py-1.5 text-xs font-medium transition-colors ${
      !isVirtual ? 'bg-primary text-white' : 'text-on-bg-secondary hover:text-on-bg'
    }`}
    onClick={() => handleModeSwitch('physical')}
  >
    Physical Device
  </button>
  <button
    type="button"
    className={`flex-1 rounded-md px-3 py-1.5 text-xs font-medium transition-colors ${
      isVirtual ? 'bg-primary text-white' : 'text-on-bg-secondary hover:text-on-bg'
    }`}
    onClick={() => handleModeSwitch('virtual')}
  >
    Virtual Node
  </button>
</div>
```

### Pattern 3: Icon Radio Cards
**What:** 2x2 grid of selectable cards, each showing a Material Symbol icon and label. Selected card has `border-primary` and `bg-primary/10` background.
**When to use:** For virtual subtype selection (Internet, Cloud, Server, Generic).
**Example:**
```typescript
const subtypes = [
  { value: 'internet', label: 'Internet', icon: 'language' },
  { value: 'cloud', label: 'Cloud', icon: 'cloud' },
  { value: 'server', label: 'Server', icon: 'dns' },
  { value: 'generic', label: 'Generic', icon: 'hub' },
] as const;

// 2x2 grid with radio card semantics:
<div className="grid grid-cols-2 gap-2">
  {subtypes.map((st) => (
    <button
      key={st.value}
      type="button"
      onClick={() => setVirtualSubtype(st.value)}
      className={`flex flex-col items-center gap-1.5 rounded-lg border-2 px-3 py-3 transition-colors ${
        virtualSubtype === st.value
          ? 'border-primary bg-primary/10'
          : 'border-outline-subtle bg-elevated hover:border-outline'
      }`}
    >
      <MaterialIcon name={st.icon} size={24} className={
        virtualSubtype === st.value ? 'text-primary' : 'text-on-bg-secondary'
      } />
      <span className={`text-xs font-medium ${
        virtualSubtype === st.value ? 'text-primary' : 'text-on-bg-secondary'
      }`}>
        {st.label}
      </span>
    </button>
  ))}
</div>
```

### Pattern 4: Mode Switch Resets Form State
**What:** Per D-03, switching modes resets all form fields to defaults. Implement as a `handleModeSwitch` function that calls all setters.
**Example:**
```typescript
function handleModeSwitch(mode: DeviceMode) {
  setDeviceMode(mode);
  setError(null);
  // Reset all shared fields
  setDisplayName('');
  setAreaIds([]);
  // Reset physical-specific fields
  setHostname('');
  setVersion('2c');
  setCommunity('public');
  setUsername('');
  // ... all other physical fields
  // Reset virtual-specific fields
  setVirtualSubtype('internet'); // D-05: default to Internet
  setVirtualIp('');
}
```

### Pattern 5: Virtual Device Payload Construction
**What:** When in virtual mode, submit a different payload structure to createDevice.
**Example:**
```typescript
// In handleSubmit:
if (isVirtual) {
  if (!displayName.trim()) {
    setError('Display Name is required.');
    return;
  }
  await createDevice({
    hostname: displayName.trim(),
    ip: virtualIp.trim(),
    device_type: 'virtual',
    snmp: { version: '2c' }, // placeholder, backend ignores for virtual
    tags: {
      display_name: displayName.trim(),
      virtual_subtype: virtualSubtype,
    },
    area_ids: areaIds.length > 0 ? areaIds : undefined,
  });
  onDeviceAdded();
  return;
}
// ... existing physical submit logic
```

### Pattern 6: Context Menu Filtering in Canvas.tsx
**What:** Filter the context menu items array based on `device.device_type`. Currently at lines 302-307 of Canvas.tsx.
**Example:**
```typescript
// Current: flat array of 4 items
// New: build full array, then filter for virtual
const allItems = [
  { label: 'Open WebFig', icon: 'link', onClick: () => { ... } },
  { label: gUrl ? 'Open in Grafana' : '...', icon: 'hub', onClick: () => { ... } },
  { label: 'Per-Interface Stats', icon: 'devices', onClick: () => { ... } },
  { label: 'Configure', icon: 'settings', onClick: () => { ... } },
];

const isVirtual = d?.device_type === 'virtual';
const items = isVirtual
  ? allItems.filter((item) => item.label !== 'Open WebFig' && item.label !== 'Per-Interface Stats')
  : allItems;
```

### Pattern 7: LinkCreatePanel Virtual Detection
**What:** After device selection, check if selected device is virtual. Hide InterfaceSelect for virtual side, show a label instead. Auto-set if_name to empty string for virtual side.
**Example:**
```typescript
const sourceIsVirtual = sourceDevice?.device_type === 'virtual';
const targetIsVirtual = targetDevice?.device_type === 'virtual';
const bothVirtual = sourceIsVirtual && targetIsVirtual;

// For the interface select section:
{sourceIsVirtual ? (
  <p className="text-xs text-on-bg-secondary italic">(virtual node -- no interface)</p>
) : (
  <InterfaceSelect ... />
)}

// Validation:
// Auto-set empty if_name for virtual side
// Both-virtual check with inline error
```

### Anti-Patterns to Avoid
- **Creating new component files for tiny pieces:** The segmented control and icon cards are small enough to be inline or local to AddDevicePanel. Don't create separate files unless they're clearly reusable elsewhere.
- **Preserving state between mode switches:** D-03 explicitly says "no state preserved between modes." Don't try to memoize values across Physical/Virtual toggles.
- **Filtering context menu items by index:** Use label matching or a stable key, not array indices -- indices can shift if items are reordered later.
- **Submitting if_name for virtual devices:** The backend expects empty `if_name` for virtual sides. Don't send a placeholder string.

## Don't Hand-Roll

| Problem | Don't Build | Use Instead | Why |
|---------|-------------|-------------|-----|
| Icon rendering | Custom SVG icons | `MaterialIcon` component with subset font | Icons already in font subset (language, cloud, dns, hub) from Phase 9 |
| Context menu | New menu component | Existing `ContextMenu` with filtered items array | Data-driven design handles this natively |
| Device search dropdown | New select component | Existing `SearchableDeviceSelect` in LinkCreatePanel | Already handles device list, search, selection |
| Form validation | Validation library (yup, zod) | Inline validation in handleSubmit | Matches existing pattern, only 2-3 checks |

## Common Pitfalls

### Pitfall 1: CreateDevicePayload Missing device_type Field
**What goes wrong:** The `CreateDevicePayload` interface in `client.ts` currently does not have a `device_type` field. If you forget to add it, the backend will treat virtual submissions as regular device creation attempts and fail validation (ip is required).
**Why it happens:** The interface was written before virtual devices existed.
**How to avoid:** Add `device_type?: string` to `CreateDevicePayload` as the very first change. The field is optional so physical device creation (which omits it) continues to work.
**Warning signs:** Virtual device creation returns 400 "ip is required".

### Pitfall 2: Backend SNMP Validation for Virtual Devices
**What goes wrong:** Physical form requires SNMP credentials. Virtual form does not. If the virtual payload includes an empty `snmp` object without a version, the backend might reject it.
**Why it happens:** The backend's virtual path (`deviceType == DeviceTypeVirtual`) runs before SNMP validation, so it actually skips SNMP validation entirely. But the `CreateDevicePayload` TypeScript interface requires `snmp: SNMPPayload` (non-optional).
**How to avoid:** The `snmp` field in CreateDevicePayload is required. For virtual submissions, pass a minimal `{ version: '2c' }` placeholder. The backend ignores it for virtual types.
**Warning signs:** TypeScript compile error if snmp is omitted.

### Pitfall 3: Form State Leak Between Modes
**What goes wrong:** User fills in physical form (IP, SNMP creds), switches to virtual, then back to physical. Old values may persist if mode switch doesn't reset.
**Why it happens:** React useState doesn't automatically reset when you change a separate state variable.
**How to avoid:** Per D-03, the `handleModeSwitch` function must explicitly call every setter to reset to defaults. Include ALL state variables, not just the ones visible in the current mode.
**Warning signs:** Stale SNMP credentials or IP address appearing after toggling.

### Pitfall 4: LinkCreatePanel Submit Validation Not Updated
**What goes wrong:** Current validation requires both `sourceIfName` and `targetIfName`. When one device is virtual, its if_name should be empty string, but current validation blocks empty if_name.
**Why it happens:** The existing validation at line 265 checks `!sourceIfName || !targetIfName`.
**How to avoid:** Update validation to skip if_name check for the virtual side. For virtual devices, auto-set the if_name to empty string (the backend accepts empty if_name for virtual devices per D-12).
**Warning signs:** Create button permanently disabled when one device is virtual.

### Pitfall 5: Context Menu Filtering Referencing Wrong Device
**What goes wrong:** The context menu code in Canvas.tsx accesses the device via `devices.find((dev) => dev.id === deviceMenu.deviceId)`. If `d` is undefined (device not found), the virtual check fails.
**Why it happens:** Race condition between device list updates and context menu opening.
**How to avoid:** Always null-check `d` before checking `d.device_type`. The existing code already does `if (d)` before using `d`, so just add the virtual check in the same guard.
**Warning signs:** Context menu showing all items for a briefly-undefined device.

### Pitfall 6: Virtual Device Label in SearchableDeviceSelect
**What goes wrong:** `deviceLabel()` function in LinkCreatePanel uses `d.ip` as the primary display, with name as secondary. Virtual devices without IP show empty string.
**Why it happens:** Virtual devices can have empty IP. The `deviceLabel` function assumes IP is always present.
**How to avoid:** Update `deviceLabel` to handle virtual devices: show display_name as primary when IP is empty. Example: `const name = d.tags?.display_name || d.sys_name; return d.ip ? (name ? \`${d.ip} -- ${name}\` : d.ip) : (name || '(unnamed)')`.
**Warning signs:** Empty or "-- Name" display in device dropdowns.

## Code Examples

### Current CreateDevicePayload (needs device_type)
```typescript
// Source: frontend/src/api/client.ts lines 160-171
export interface CreateDevicePayload {
  hostname: string;
  ip: string;
  snmp: SNMPPayload;
  tags?: Record<string, string>;
  vendor?: string;
  metrics_source?: string;
  prometheus_label_name?: string;
  prometheus_label_value?: string;
  ssh_profile_id?: string;
  area_ids?: string[];
  // ADD: device_type?: string;
}
```

### Backend Virtual Creation Handler (already implemented)
```go
// Source: internal/api/device_handler.go lines 96-148
// The backend:
// 1. Checks if deviceType == DeviceTypeVirtual
// 2. Validates display_name and virtual_subtype in tags
// 3. Allows empty IP
// 4. Skips SNMP validation entirely
// 5. Calls svc.AddDevice with DeviceTypeVirtual and empty SNMPCredentials
```

### Backend Link Handler Virtual Validation (already implemented)
```go
// Source: internal/api/link_handler.go lines 93-109
// The backend:
// 1. Rejects both-virtual links (400 "at least one device must be non-virtual")
// 2. Allows empty if_name for virtual side only
// 3. Requires if_name for physical side
```

### nodeBuilder Virtual Flag Setup (already implemented)
```typescript
// Source: frontend/src/components/canvas/nodeBuilder.ts lines 39-59
const isVirtual = device.device_type === 'virtual';
// Passes to DeviceCard:
//   isVirtual: true,
//   subtype: deviceData.tags?.virtual_subtype ?? 'generic',
//   metrics: null (virtual has no SNMP metrics)
```

### Existing Conditional Rendering Pattern
```typescript
// Source: frontend/src/components/AddDevicePanel.tsx
// This is the pattern to follow for virtual mode:
const isV3 = version === '3';
const needsAuth = securityLevel === 'authNoPriv' || securityLevel === 'authPriv';
const usesPrometheus = metricsMode === 'prometheus' || metricsMode === 'prometheus_snmp_fallback';

// Conditional rendering:
{usesSNMP && (
  <div className="space-y-3 bg-surface-high rounded-lg p-3">
    {/* SNMP fields */}
  </div>
)}
```

## State of the Art

| Old Approach | Current Approach | When Changed | Impact |
|--------------|------------------|--------------|--------|
| No virtual device support | Backend + rendering ready | Phase 8-9 | Phase 10 only adds forms |
| All devices require IP + SNMP | Virtual skips SNMP, IP optional | Phase 8 | Form must conditionally omit |
| 4-item context menu for all | Need to filter by device_type | Phase 10 | Simple array filter |

**Already completed in prior phases:**
- Backend virtual device creation + validation (Phase 8)
- Backend both-virtual link rejection (Phase 8)
- Backend empty if_name for virtual links (Phase 8)
- Virtual node rendering with subtype icons (Phase 9)
- Material Symbols font subset with language/cloud/dns icons (Phase 9)
- edgeBuilder/nodeBuilder virtual detection (Phase 9)

## Validation Architecture

### Test Framework
| Property | Value |
|----------|-------|
| Framework | Vitest 4.1 + @testing-library/react 16.3 |
| Config file | `frontend/vitest.config.ts` |
| Quick run command | `cd frontend && npx vitest run --reporter=verbose` |
| Full suite command | `cd frontend && npx vitest run` |

### Phase Requirements to Test Map
| Req ID | Behavior | Test Type | Automated Command | File Exists? |
|--------|----------|-----------|-------------------|-------------|
| VIRT-10 | Segmented control renders and toggles mode | unit | `cd frontend && npx vitest run src/components/AddDevicePanel.test.tsx -t "segmented"` | Existing file, needs new tests |
| VIRT-11 | Virtual form shows subtype cards, display name, optional IP; hides SNMP | unit | `cd frontend && npx vitest run src/components/AddDevicePanel.test.tsx -t "virtual"` | Existing file, needs new tests |
| VIRT-12 | LinkCreatePanel hides interface selector for virtual device | unit | `cd frontend && npx vitest run src/components/LinkCreatePanel.test.tsx -t "virtual"` | Does not exist |
| VIRT-13 | Both-virtual validation shows error and disables Create | unit | `cd frontend && npx vitest run src/components/LinkCreatePanel.test.tsx -t "both virtual"` | Does not exist |
| VIRT-16 | Virtual context menu has only 2 items | unit | `cd frontend && npx vitest run src/components/ContextMenu.test.tsx -t "virtual"` | Existing file, needs new tests |

### Sampling Rate
- **Per task commit:** `cd frontend && npx vitest run --reporter=verbose`
- **Per wave merge:** `cd frontend && npx vitest run`
- **Phase gate:** Full suite green before `/gsd:verify-work`

### Wave 0 Gaps
- [ ] `frontend/src/components/LinkCreatePanel.test.tsx` -- covers VIRT-12 and VIRT-13 (virtual interface hiding + both-virtual validation)
- [ ] New test cases in `frontend/src/components/AddDevicePanel.test.tsx` -- covers VIRT-10 and VIRT-11 (segmented control, virtual form fields)
- [ ] New test case in Canvas or ContextMenu test -- covers VIRT-16 (context menu filtering logic; can test the filtering function directly)

## Open Questions

1. **CreateDevicePayload.snmp required field**
   - What we know: The TypeScript interface requires `snmp: SNMPPayload`. The backend ignores it for virtual devices.
   - What's unclear: Should `snmp` be made optional in the interface, or should virtual submissions pass a placeholder?
   - Recommendation: Make `snmp` optional (`snmp?: SNMPPayload`) in `CreateDevicePayload` since virtual devices genuinely don't use SNMP. This is cleaner than a placeholder. The backend already handles the case where SNMP fields are empty/missing for virtual types.

2. **CreateDevicePayload.hostname required field**
   - What we know: The TypeScript interface requires `hostname: string`. For virtual devices, the hostname field is not meaningful.
   - What's unclear: What to set as hostname for virtual devices.
   - Recommendation: Use the display_name as hostname (it's required for virtual devices anyway). The backend stores it but doesn't use it for SNMP discovery on virtual devices.

## Sources

### Primary (HIGH confidence)
- `frontend/src/components/AddDevicePanel.tsx` -- Full source read, 474 lines, all form state and conditional rendering patterns
- `frontend/src/components/LinkCreatePanel.tsx` -- Full source read, 398 lines, SearchableDeviceSelect, InterfaceSelect, validation
- `frontend/src/components/ContextMenu.tsx` -- Full source read, 121 lines, data-driven ContextMenuItem interface
- `frontend/src/components/Canvas.tsx` -- Full source read, 385 lines, context menu item definitions at lines 302-307
- `frontend/src/api/client.ts` -- Full source read, CreateDevicePayload interface at lines 160-171
- `frontend/src/types/api.ts` -- Full source read, Device interface, DeviceType union includes 'virtual'
- `frontend/src/components/DeviceCard.tsx` -- Full source read, virtual card rendering, subtypeIconMap
- `frontend/src/components/canvas/nodeBuilder.ts` -- Full source read, isVirtual flag propagation
- `internal/api/device_handler.go` -- Virtual creation path (lines 96-148), validates display_name + virtual_subtype
- `internal/api/link_handler.go` -- Both-virtual rejection (lines 93-99), empty if_name for virtual (lines 101-109)
- `frontend/src/components/MaterialIcon.tsx` -- MaterialIcon component API

### Secondary (HIGH confidence -- project codebase)
- `frontend/src/components/AddDevicePanel.test.tsx` -- Existing test structure, mock patterns for api/client
- `frontend/src/components/ContextMenu.test.tsx` -- Existing test patterns for ContextMenu
- `frontend/src/components/canvas/CanvasPanels.tsx` -- How AddDevicePanel and LinkCreatePanel are rendered in SidePanel
- `frontend/src/components/canvas/useCanvasMenus.ts` -- Device menu state management
- `frontend/vitest.config.ts` -- Test configuration (jsdom, globals, setup file)
- `.planning/phases/10-virtual-node-forms/10-CONTEXT.md` -- All user decisions (D-01 through D-11)

## Metadata

**Confidence breakdown:**
- Standard stack: HIGH -- all components already exist, no new dependencies
- Architecture: HIGH -- extending established patterns (boolean flags, conditional JSX, data-driven menus)
- Pitfalls: HIGH -- verified against actual source code, backend handler confirms validation flow

**Research date:** 2026-04-01
**Valid until:** 2026-05-01 (stable -- no external dependency changes expected)
