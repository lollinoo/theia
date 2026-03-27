# Phase 3: Area Backend and Management - Context

**Gathered:** 2026-03-26
**Status:** Ready for planning

<domain>
## Phase Boundary

Deliver the complete area backend (database schema, REST API for area CRUD, device-area assignment) and the frontend area management UI in SettingsPanel plus device-to-area assignment dropdown in DeviceConfigPanel. This phase does NOT build the Area Hub view, nav pill, watermark, or filtered canvas — those are Phase 4. The backend and management UI established here provides the data layer that Phase 4 consumes.

</domain>

<decisions>
## Implementation Decisions

### Color Palette
- **D-01:** Area accent colors are selected from a curated palette of 7 swatches hardcoded as a frontend constant: `#00E676` (green), `#2979FF` (blue), `#E040FB` (purple), `#FFEA00` (amber), `#FF6D00` (orange), `#00BCD4` (cyan), `#FF1744` (red)
- **D-02:** Multiple areas can share the same accent color — no uniqueness enforcement on color
- **D-03:** Default color for new areas is the first swatch (`#00E676` green, the primary system color)
- **D-04:** The database stores the hex color string directly (e.g., `#2979FF`), not a palette index

### Settings Layout
- **D-05:** Area management lives as a new section in SettingsPanel, positioned above SNMP Profiles and SSH Profiles (below General Settings)
- **D-06:** Area CRUD uses an inline list pattern: collapsed cards show color swatch + name + description preview + device count badge. Click to expand inline for editing. Matches the SNMPProfileManager/SSHProfileManager pattern
- **D-07:** Area management is its own `AreaManager` component (following the SNMPProfileManager/SSHProfileManager pattern), not inline in SettingsPanel
- **D-08:** Expanded area edit view shows the form fields (name, description, color picker) AND a read-only list of assigned devices
- **D-09:** Area edit view supports bidirectional device assignment — users can add/remove devices from the area's device list, not just from DeviceConfigPanel
- **D-10:** Deleting an area that has assigned devices is allowed — devices are unassigned (area_id set to NULL) with a confirmation dialog showing the device count

### Device Assignment UX
- **D-11:** DeviceConfigPanel area dropdown shows color swatch dot + area name for each option, with "Unassigned" as the first option (no swatch)
- **D-12:** Area dropdown is positioned after hostname/IP fields, before SNMP configuration — grouped with identity fields
- **D-13:** Area assignment saves with the existing Save button (batched with other field changes), not immediately on dropdown change

### Area Data Model
- **D-14:** Area description is optional (defaults to empty string)
- **D-15:** Areas display in alphabetical order by name — no sort_order field
- **D-16:** Area names must be unique, enforced by the backend (409 Conflict on duplicate)

### Claude's Discretion
- Migration number and exact SQL schema (follows existing patterns in `internal/repository/sqlite/migrations/`)
- Area handler structure and helper functions (follow existing device_handler.go pattern)
- AreaManager component internal state management approach
- Exact Tailwind classes for area swatch rendering and inline expansion animation
- Whether to add an `area_id` filter parameter to the GET /api/v1/devices endpoint
- Area name max length validation (if any)
- WebSocket snapshot inclusion of area data (whether areas are pushed via WS or fetched via REST)

</decisions>

<canonical_refs>
## Canonical References

**Downstream agents MUST read these before planning or implementing.**

### Design System
- `.planning/DESIGN.md` — Neon Topography design system: colors (area-specific accents in Section 2), typography, elevation, no-line rule
- `.planning/examples_mocks/ospf_area_hub/dark/screen.png` — Area Hub mock showing how area colors are used (Phase 4 target, but informs color palette choices)
- `.planning/examples_mocks/ospf_area_hub/light/screen.png` — Area Hub light mock

### Requirements
- `.planning/REQUIREMENTS.md` — AREA-01 through AREA-06 define Phase 3 acceptance criteria
- `.planning/ROADMAP.md` — Phase 3 success criteria and dependency chain

### Prior Phase Context
- `.planning/phases/01-design-token-foundation-and-theme-infrastructure/01-CONTEXT.md` — D-11: Area accent tokens already in token system; D-04/D-05: Light theme surface hierarchy
- `.planning/phases/02-component-restyling/02-CONTEXT.md` — D-01/D-04: Material Symbols for all icons; D-05/D-06: Glassmorphism dark-only; D-12: No-line rule

### Backend Patterns (existing code to follow)
- `internal/domain/device.go` — Domain struct and repository interface pattern
- `internal/api/device_handler.go` — Handler CRUD pattern, JSON:API response envelope
- `internal/api/router.go` — Route registration pattern (hand-rolled mux)
- `internal/repository/sqlite/migrations/` — Migration file naming convention (000007 is next)

### Frontend Patterns (existing code to follow)
- `frontend/src/components/SNMPProfileManager.tsx` — Profile manager component pattern (inline list CRUD) — area management should follow this
- `frontend/src/components/DeviceConfigPanel.tsx` — Where area dropdown will be added
- `frontend/src/components/SettingsPanel.tsx` — Where AreaManager component will be imported
- `frontend/src/types/api.ts` — TypeScript type definitions (Area type goes here)
- `frontend/src/api/client.ts` — API client functions (area CRUD functions go here)

</canonical_refs>

<code_context>
## Existing Code Insights

### Reusable Assets
- `SNMPProfileManager.tsx` / `SSHProfileManager.tsx` — Inline list CRUD pattern with expand/collapse. AreaManager should follow this structure closely
- `internal/api/device_handler.go` — JSON:API envelope pattern (`{"data": {...}}`) for consistent API responses
- `internal/repository/sqlite/` — SQLite repo pattern with constructor injection and `(result, error)` returns
- CSS token system (`--nt-*` variables) — All area UI uses existing tokens; area accent colors map to the curated palette
- `MaterialIcon` component — Use for area section icons and action buttons

### Established Patterns
- Go: domain interface → sqlite repo → service (optional) → API handler → router registration
- Frontend: TypeScript interface in `types/api.ts` → API functions in `api/client.ts` → Component
- All styling via Tailwind utility classes with CSS variable tokens
- Handler pattern: parse request → validate → call repo/service → write response via `writeError` or JSON marshal

### Integration Points
- `internal/domain/device.go` — Device struct needs nullable `AreaID *uuid.UUID` field
- `internal/api/device_handler.go` — `updateDeviceRequest` needs `AreaID` field (double-pointer pattern for nullable)
- `internal/api/router.go` — New `/api/v1/areas` and `/api/v1/areas/` route registrations
- `cmd/theia/main.go` — Wire AreaRepo into router, pass to device handler
- `frontend/src/types/api.ts` — Add `Area` interface and `area_id?: string` to `Device`
- `frontend/src/api/client.ts` — Add area CRUD functions + area list fetch
- `frontend/src/components/SettingsPanel.tsx` — Import and render `AreaManager`
- `frontend/src/components/DeviceConfigPanel.tsx` — Add area dropdown after IP field
- `internal/ws/messages.go` — Consider including area data in WebSocket snapshots for Phase 4 readiness

</code_context>

<specifics>
## Specific Ideas

- Area management should feel like a "first-class" section in Settings — prominent placement above profiles signals that areas are a core organizational concept
- The inline list pattern keeps area management consistent with SNMP/SSH profiles — the SettingsPanel has an established visual rhythm
- Bidirectional assignment (from both area and device sides) is important for usability — a network operator organizing 100+ devices needs to work from the area perspective, not just device-by-device
- The curated palette ensures all area colors look good against both dark charcoal and light gray backgrounds without design system clashing
- Hex storage decouples the DB from the frontend palette — if colors are ever adjusted, existing areas keep their original color

</specifics>

<deferred>
## Deferred Ideas

- Canvas.tsx decomposition (750 lines) — should happen before Phase 4 adds area filtering, not in Phase 3
- Area-filtered topology canvas — Phase 4 scope (AREA-11)
- Area Hub view with aggregate stats and cards — Phase 4 scope (AREA-07, AREA-08)
- Floating navigation pill — Phase 4 scope (AREA-09)
- Atmospheric watermark — Phase 4 scope (AREA-10)
- Drag-reorder for areas — deferred; alphabetical sort is sufficient for v1.3.0
- Bulk device assignment (multi-select devices → assign to area) — nice-to-have, can be added later

</deferred>

---

*Phase: 03-area-backend-and-management*
*Context gathered: 2026-03-26*
