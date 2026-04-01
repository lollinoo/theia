# Phase 10: Virtual Node Forms and Context Menu - Context

**Gathered:** 2026-04-01
**Status:** Ready for planning

<domain>
## Phase Boundary

Users can create virtual nodes through the existing AddDevicePanel with a Physical/Virtual segmented toggle, link them to physical devices via an adapted LinkCreatePanel, and get a context menu that only shows relevant actions for virtual nodes.

Requirements: VIRT-10, VIRT-11, VIRT-12, VIRT-13, VIRT-16.

</domain>

<decisions>
## Implementation Decisions

### Form Toggle UX
- **D-01:** AddDevicePanel gets a segmented control (two-segment pill) at the top: "Physical Device" | "Virtual Node". Selecting one swaps the entire field set below.
- **D-02:** Physical mode shows the current full form (IP, SNMP, Prometheus, vendor, SSH, areas). Virtual mode shows only: Display Name (required), Subtype (required), IP (optional), Areas (optional).
- **D-03:** Switching modes resets all form fields to defaults. No state preserved between modes. Clean slate prevents confusing leftover values.

### Virtual Subtype Selector
- **D-04:** Subtypes presented as a 2x2 grid of icon radio cards. Each card shows the Material Symbol icon + label (Internet/language, Cloud/cloud, Server/dns, Generic/hub).
- **D-05:** Selected card has primary-color border and subtle background highlight. Default selection: Internet (first card).

### Link Creation for Virtual
- **D-06:** When a virtual device is selected on either side of LinkCreatePanel, its interface selector is hidden entirely. A "(virtual node -- no interface)" label replaces it.
- **D-07:** Only the physical device's interface selector remains visible and required.
- **D-08:** Frontend inline validation: when both devices are virtual, show error message "At least one device must be physical" below the second device selector and disable the Create button. No server roundtrip needed.

### Context Menu Filtering
- **D-09:** Virtual node context menu hides "Open WebFig" and "Per-Interface Stats" entirely. Virtual nodes show only "Open in Grafana" and "Configure".
- **D-10:** Physical device context menu unchanged (all 4 items).

### API Payload Extension
- **D-11:** `CreateDevicePayload` in `client.ts` needs a `device_type` field. Virtual form submits `device_type: "virtual"` with `tags: { display_name, virtual_subtype }`. Physical form omits `device_type` (backend defaults to auto-detection).

### Claude's Discretion
- Segmented control styling details (exact Tailwind classes, active/inactive states)
- Icon radio card internal layout and spacing
- How to structure the conditional rendering in AddDevicePanel (inline branches vs extracted sub-components)
- Whether to extract virtual validation as a shared util or inline it

</decisions>

<canonical_refs>
## Canonical References

**Downstream agents MUST read these before planning or implementing.**

### Frontend Components
- `frontend/src/components/AddDevicePanel.tsx` — Current add device form (~475 lines), all form state local, validation at line 94-98, submit at lines 102-124
- `frontend/src/components/LinkCreatePanel.tsx` — Link creation form (~400 lines), SearchableDeviceSelect (lines 28-130), InterfaceSelect (lines 167-219), getDeviceInterfaces (lines 132-165), validation at lines 265-272
- `frontend/src/components/ContextMenu.tsx` — Generic data-driven context menu renderer, ContextMenuItem interface (lines 4-11)
- `frontend/src/components/Canvas.tsx` — Context menu item definitions (lines 298-325), device menu always 4 items, no conditional filtering
- `frontend/src/components/DeviceCard.tsx` — Virtual card rendering branch (isVirtual), subtype icon mapping

### API Client
- `frontend/src/api/client.ts` — CreateDevicePayload interface (lines 160-171, currently no device_type field), createDevice function (lines 173-185)

### Type System
- `frontend/src/types/api.ts` — DeviceType union (includes 'virtual'), Device interface, parseDeviceType

### Phase 8 Context (Backend decisions)
- `.planning/phases/08-virtual-device-backend/08-CONTEXT.md` — D-08 (virtual skips SNMP), D-09 (requires display_name + virtual_subtype), D-12 (empty if_name for virtual), D-13 (both-virtual rejected)

### Phase 9 Context (Rendering decisions)
- `.planning/phases/09-virtual-node-rendering/09-CONTEXT.md` — D-12 (subtype icon mapping: internet→language, cloud→cloud, server→dns, generic→hub)

### Design System
- `frontend/src/index.css` — Design tokens, Material Symbols @font-face
- `frontend/tailwind.config.js` — Custom theme tokens

</canonical_refs>

<code_context>
## Existing Code Insights

### Reusable Assets
- `MaterialIcon` component: Renders subtype icons at custom sizes — reuse in icon radio cards
- `StatusDot` component: Already used in virtual cards, available for form preview
- `SearchableDeviceSelect` in LinkCreatePanel: Custom dropdown with search — already handles device list filtering
- `ContextMenuItem` interface: Data-driven, supports `disabled`, `icon`, `separator` — just filter the items array
- Existing area multi-select chip UI in AddDevicePanel — reuse for virtual form

### Established Patterns
- Conditional form sections via boolean flags (`isV3`, `needsAuth`, `needsPriv`) — extend with `isVirtual` flag
- `MetricsMode` union type drives conditional rendering — same pattern for device type mode
- Context menu items built as array in Canvas.tsx — add `device_type` check to filter array
- Form submit builds payload object conditionally — add virtual branch for payload construction

### Integration Points
- `AddDevicePanel.tsx`: Add segmented control, `isVirtual` state, conditional field rendering, payload construction for virtual
- `client.ts`: Extend `CreateDevicePayload` with optional `device_type` field
- `LinkCreatePanel.tsx`: Detect virtual devices in selection, hide InterfaceSelect, validate both-virtual
- `Canvas.tsx` lines 298-309: Filter context menu items array based on `device.device_type`

</code_context>

<specifics>
## Specific Ideas

- Segmented control should match the Neon Topography aesthetic — `bg-surface` inactive segments, `bg-primary` active segment with `text-on-primary`
- Icon radio cards should render the actual Material Symbol icons from the font subset (language, cloud, dns, hub) so users preview what the canvas node will look like
- Virtual form should feel lightweight compared to the physical form — fewer fields, more whitespace, the icon cards are the visual centerpiece

</specifics>

<deferred>
## Deferred Ideas

None — discussion stayed within phase scope.

</deferred>

---

*Phase: 10-virtual-node-forms*
*Context gathered: 2026-04-01*
