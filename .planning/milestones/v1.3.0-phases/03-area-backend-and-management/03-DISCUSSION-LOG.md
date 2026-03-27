# Phase 3: Area Backend and Management - Discussion Log

> **Audit trail only.** Do not use as input to planning, research, or execution agents.
> Decisions are captured in CONTEXT.md — this log preserves the alternatives considered.

**Date:** 2026-03-26
**Phase:** 03-area-backend-and-management
**Areas discussed:** Color palette, Settings layout, Device assignment UX, Area data model

---

## Color Palette

| Option | Description | Selected |
|--------|-------------|----------|
| Curated swatches | 6-8 preset colors from design system. Consistent with Neon Topography palette, prevents clashing colors. | ✓ |
| Free color picker | Full hex/HSL picker. Maximum flexibility but risk of clashing colors. | |
| Auto-assigned | System assigns next color from rotating palette. Simpler but less control. | |

**User's choice:** Curated swatches (7 colors: green, blue, purple, amber, orange, cyan, red)
**Notes:** Palette derived from existing design system tokens.

### Follow-up: Duplicate colors

| Option | Description | Selected |
|--------|-------------|----------|
| Yes, allow duplicates | Users can pick same color for multiple areas. Simpler logic. | ✓ |
| No, enforce unique colors | Each area must have distinct color. Grey out used swatches. | |

**User's choice:** Allow duplicates

### Follow-up: Palette source

| Option | Description | Selected |
|--------|-------------|----------|
| Frontend constant | Hardcode 7 hex values in frontend. Simple, matches design tokens. | ✓ |
| Backend config | Store palette in settings table. More flexible. | |

**User's choice:** Frontend constant

### Follow-up: Default color

| Option | Description | Selected |
|--------|-------------|----------|
| First swatch (green #00E676) | Default to primary green. User can change before saving. | ✓ |
| Next unused color | Auto-select first unused color. | |
| No default — force pick | User must explicitly select before saving. | |

**User's choice:** First swatch (green)

---

## Settings Layout

### Placement

| Option | Description | Selected |
|--------|-------------|----------|
| New section at top | Above SNMP/SSH profiles. Areas are a primary concept — prominent placement. | ✓ |
| New section after profiles | Below SNMP/SSH profiles. Less prominent. | |
| Tabbed navigation | Convert to tabs: General, Areas, SNMP, SSH. Bigger refactor. | |

**User's choice:** New section at top (above profiles, below general settings)

### CRUD style

| Option | Description | Selected |
|--------|-------------|----------|
| Inline list | Cards with name, swatch, description, device count. Click to expand. Matches profile manager pattern. | ✓ |
| Modal dialog | List + modal form for create/edit. | |
| Side panel | List + side panel for details. | |

**User's choice:** Inline list

### Delete rule

| Option | Description | Selected |
|--------|-------------|----------|
| Allow with unassign | Delete area, unassign devices (NULL area_id). Confirmation dialog with device count. | ✓ |
| Block until empty | Prevent deletion if devices assigned. | |
| Cascade delete devices | Delete area AND devices. Too destructive. | |

**User's choice:** Allow with unassign

### Component structure

| Option | Description | Selected |
|--------|-------------|----------|
| Own component | Create AreaManager following SNMPProfileManager pattern. | ✓ |
| Inline in SettingsPanel | Add logic directly in SettingsPanel.tsx. | |

**User's choice:** Own component (AreaManager)

### Edit view content

| Option | Description | Selected |
|--------|-------------|----------|
| Show device list | Expanded form shows name, description, color + read-only device list. | ✓ |
| Just form fields | Minimal: name, description, color only. | |

**User's choice:** Show device list

### Assignment direction

| Option | Description | Selected |
|--------|-------------|----------|
| Bidirectional | Area edit view can add/remove devices. Same as DeviceConfigPanel but from area perspective. | ✓ |
| DeviceConfigPanel only | Assignment only from device side. Area view is read-only. | |

**User's choice:** Bidirectional

---

## Device Assignment UX

### Dropdown style

| Option | Description | Selected |
|--------|-------------|----------|
| Color swatch + name | Colored dot + area name. "Unassigned" first option (no swatch). | ✓ |
| Name only | Simple text dropdown. No color indicator. | |
| Name + device count | Area name + device count per area. | |

**User's choice:** Color swatch + name

### Save behavior

| Option | Description | Selected |
|--------|-------------|----------|
| With existing Save button | Area dropdown is another field, saved with other changes. Consistent. | ✓ |
| Immediate save on change | Dropdown change immediately updates. Faster but inconsistent. | |

**User's choice:** With existing Save button

### Dropdown position

| Option | Description | Selected |
|--------|-------------|----------|
| After hostname/IP, before SNMP | Grouped with identity fields. High-level organizational field. | ✓ |
| At the very top | First field. Maximum prominence. | |
| At the bottom | After all technical fields. Less prominent. | |

**User's choice:** After hostname/IP, before SNMP

---

## Area Data Model

### Description requirement

| Option | Description | Selected |
|--------|-------------|----------|
| Optional | Defaults to empty. Less friction for quick area creation. | ✓ |
| Required | Force description. Better documentation. | |

**User's choice:** Optional

### Sort order

| Option | Description | Selected |
|--------|-------------|----------|
| Alphabetical only | No sort_order field. Always sorted by name. Simpler. | ✓ |
| With drag reorder | sort_order integer. Users can drag-reorder. | |
| Creation order | Display in creation order. No extra field. | |

**User's choice:** Alphabetical only

### Name uniqueness

| Option | Description | Selected |
|--------|-------------|----------|
| Enforce unique names | Backend returns 409 on duplicate. Name is primary identifier. | ✓ |
| Allow duplicates | Multiple areas with same name. Colors differentiate. | |

**User's choice:** Enforce unique names

### Color storage

| Option | Description | Selected |
|--------|-------------|----------|
| Hex string | Store '#2979FF' directly. Simple, self-contained. | ✓ |
| Palette index | Store integer index (0-6). Smaller but creates coupling. | |

**User's choice:** Hex string

---

## Claude's Discretion

- Migration number and SQL schema details
- Area handler structure and helper functions
- AreaManager component internal state management
- Exact Tailwind classes for area UI
- Whether to add area_id filter to GET /api/v1/devices
- Area name max length validation
- WebSocket snapshot inclusion of area data

## Deferred Ideas

- Canvas.tsx decomposition — before Phase 4
- Drag-reorder for areas — not in v1.3.0
- Bulk device assignment (multi-select) — nice-to-have for later
