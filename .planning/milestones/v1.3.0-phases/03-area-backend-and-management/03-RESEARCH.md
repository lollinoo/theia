# Phase 3: Area Backend and Management - Research

**Researched:** 2026-03-26
**Domain:** Go REST API + SQLite persistence + React CRUD UI (inline list pattern)
**Confidence:** HIGH

## Summary

Phase 3 adds a new "areas" concept to Theia -- OSPF areas that group network devices for organizational purposes. This requires a full vertical slice: SQLite schema (new `areas` table + `area_id` FK on `devices`), Go domain types and repository, REST API handler for area CRUD, device-to-area assignment via the existing device update endpoint, and frontend UI for area management in SettingsPanel plus an area dropdown in DeviceConfigPanel.

The codebase has extremely well-established patterns for every layer of this work. The SNMP Profile feature (domain type, repository, handler, frontend manager component) is a near-perfect template. The area feature is structurally simpler than SNMP profiles (no encryption, no credentials, simpler schema) but adds two novel elements: (1) a curated color palette picker, and (2) bidirectional device assignment from the area edit view.

**Primary recommendation:** Follow the SNMPProfile/SSHProfile pattern exactly for the backend (domain -> repo -> handler -> router), and adapt the SNMPProfileManager component pattern for the frontend AreaManager. The area feature requires no new dependencies -- it uses only existing Go stdlib, uuid, and React patterns already in the codebase.

<user_constraints>
## User Constraints (from CONTEXT.md)

### Locked Decisions
- **D-01:** Area accent colors are selected from a curated palette of 7 swatches hardcoded as a frontend constant: `#00E676` (green), `#2979FF` (blue), `#E040FB` (purple), `#FFEA00` (amber), `#FF6D00` (orange), `#00BCD4` (cyan), `#FF1744` (red)
- **D-02:** Multiple areas can share the same accent color -- no uniqueness enforcement on color
- **D-03:** Default color for new areas is the first swatch (`#00E676` green, the primary system green)
- **D-04:** The database stores the hex color string directly (e.g., `#2979FF`), not a palette index
- **D-05:** Area management lives as a new section in SettingsPanel, positioned above SNMP Profiles and SSH Profiles (below General Settings)
- **D-06:** Area CRUD uses an inline list pattern: collapsed cards show color swatch + name + description preview + device count badge. Click to expand inline for editing. Matches the SNMPProfileManager/SSHProfileManager pattern
- **D-07:** Area management is its own `AreaManager` component (following the SNMPProfileManager/SSHProfileManager pattern), not inline in SettingsPanel
- **D-08:** Expanded area edit view shows the form fields (name, description, color picker) AND a read-only list of assigned devices
- **D-09:** Area edit view supports bidirectional device assignment -- users can add/remove devices from the area's device list, not just from DeviceConfigPanel
- **D-10:** Deleting an area that has assigned devices is allowed -- devices are unassigned (area_id set to NULL) with a confirmation dialog showing the device count
- **D-11:** DeviceConfigPanel area dropdown shows color swatch dot + area name for each option, with "Unassigned" as the first option (no swatch)
- **D-12:** Area dropdown is positioned after hostname/IP fields, before SNMP configuration -- grouped with identity fields
- **D-13:** Area assignment saves with the existing Save button (batched with other field changes), not immediately on dropdown change
- **D-14:** Area description is optional (defaults to empty string)
- **D-15:** Areas display in alphabetical order by name -- no sort_order field
- **D-16:** Area names must be unique, enforced by the backend (409 Conflict on duplicate)

### Claude's Discretion
- Migration number and exact SQL schema (follows existing patterns in `internal/repository/sqlite/migrations/`)
- Area handler structure and helper functions (follow existing device_handler.go pattern)
- AreaManager component internal state management approach
- Exact Tailwind classes for area swatch rendering and inline expansion animation
- Whether to add an `area_id` filter parameter to the GET /api/v1/devices endpoint
- Area name max length validation (if any)
- WebSocket snapshot inclusion of area data (whether areas are pushed via WS or fetched via REST)

### Deferred Ideas (OUT OF SCOPE)
- Canvas.tsx decomposition (750 lines) -- should happen before Phase 4 adds area filtering, not in Phase 3
- Area-filtered topology canvas -- Phase 4 scope (AREA-11)
- Area Hub view with aggregate stats and cards -- Phase 4 scope (AREA-07, AREA-08)
- Floating navigation pill -- Phase 4 scope (AREA-09)
- Atmospheric watermark -- Phase 4 scope (AREA-10)
- Drag-reorder for areas -- deferred; alphabetical sort is sufficient for v1.3.0
- Bulk device assignment (multi-select devices to assign to area) -- nice-to-have, can be added later
</user_constraints>

<phase_requirements>
## Phase Requirements

| ID | Description | Research Support |
|----|-------------|------------------|
| AREA-01 | Database schema includes `areas` table with id, name, description, and accent color fields | Migration 000007 creates `areas` table; schema pattern from `snmp_profiles` table |
| AREA-02 | REST API endpoints for area CRUD (`/api/v1/areas` -- GET, POST, PUT, DELETE) | Handler follows `snmp_profile_handler.go` CRUD pattern with `{"data": ...}` envelope |
| AREA-03 | Devices table has nullable `area_id` foreign key referencing the areas table | Migration 000007 adds `area_id TEXT` column to `devices`; device repo scan updated |
| AREA-04 | REST API supports assigning/unassigning a device to an area via device update endpoint | `updateDeviceRequest` gets `AreaID *string` field; `DeviceUpdate` gets `AreaID **uuid.UUID` (double-pointer pattern) |
| AREA-05 | User can create, edit, and delete areas in the Settings panel | `AreaManager.tsx` component follows `SNMPProfileManager.tsx` inline list CRUD pattern |
| AREA-06 | User can assign a device to an area via a dropdown in DeviceConfigPanel | Area dropdown added after IP field in `DeviceConfigPanel.tsx` with color swatch rendering |
</phase_requirements>

## Standard Stack

### Core
| Library | Version | Purpose | Why Standard |
|---------|---------|---------|--------------|
| Go stdlib `net/http` | 1.24 | HTTP handler for area CRUD | Project uses no external web framework; all handlers use stdlib |
| Go stdlib `database/sql` | 1.24 | SQLite repository for area persistence | All repos use raw `database/sql` with `mattn/go-sqlite3` driver |
| `github.com/google/uuid` | v1.6.0 | UUID generation for area IDs | Used for all entity IDs throughout the project |
| `github.com/golang-migrate/migrate/v4` | v4.19.1 | SQL schema migration for areas table | All schema changes use numbered migration files |
| React | 18.3 | Frontend area management UI | Project SPA framework |
| TypeScript | 5.7 | Frontend type safety | Project uses strict mode throughout |
| Tailwind CSS | 4 (v4 via `@theme`) | Styling for area components | All component styling uses Tailwind utility classes with CSS variable tokens |

### Supporting
No additional libraries needed. Phase 3 is a pure application of existing patterns with no new external dependencies.

### Alternatives Considered
| Instead of | Could Use | Tradeoff |
|------------|-----------|----------|
| Hand-rolled color picker | `react-colorful` | Overkill -- D-01 specifies a 7-swatch fixed palette, not a full color picker |
| Area service layer | Direct repo in handler | Areas have no complex business logic (no async probes, no SNMP); the SNMP profile handler already demonstrates repo-direct pattern |

## Architecture Patterns

### Recommended Project Structure (new files)
```
internal/
  domain/
    area.go                    # Area struct, AreaRepository interface
  repository/
    sqlite/
      area_repo.go             # AreaRepo implements domain.AreaRepository
      migrations/
        000007_areas.up.sql    # CREATE TABLE areas + ALTER TABLE devices ADD area_id
        000007_areas.down.sql  # DROP TABLE areas + remove area_id column
  api/
    area_handler.go            # AreaHandler: CRUD + device count
    area_handler_test.go       # Tests following snmp_profile_handler_test.go pattern
frontend/
  src/
    components/
      AreaManager.tsx          # Inline list CRUD component
    types/
      api.ts                   # Add Area interface, update Device interface
    api/
      client.ts                # Add area CRUD functions
```

### Pattern 1: Domain Type + Repository Interface (area.go)
**What:** Define the Area struct and AreaRepository interface in the domain package
**When to use:** Every new persistent entity follows this pattern
**Example:**
```go
// Source: internal/domain/snmp_profile.go (existing pattern)
package domain

import (
    "time"
    "github.com/google/uuid"
)

// Area represents a logical grouping of network devices (e.g., OSPF area).
type Area struct {
    ID          uuid.UUID `json:"id"`
    Name        string    `json:"name"`
    Description string    `json:"description"`
    Color       string    `json:"color"` // hex color e.g. "#00E676"
    CreatedAt   time.Time `json:"created_at"`
    UpdatedAt   time.Time `json:"updated_at"`
}

// AreaRepository defines persistence operations for areas.
type AreaRepository interface {
    Create(area *Area) error
    GetByID(id uuid.UUID) (*Area, error)
    GetAll() ([]Area, error)
    Update(area *Area) error
    Delete(id uuid.UUID) error
    CountDevicesByAreaID(id uuid.UUID) (int, error)
}
```

### Pattern 2: SQLite Repository (area_repo.go)
**What:** Concrete implementation of AreaRepository using raw SQL
**When to use:** All persistence follows this pattern (no ORM)
**Key details from existing code:**
- Constructor: `NewAreaRepo(db *sql.DB) *AreaRepo`
- UUID stored as TEXT in SQLite, parsed via `uuid.MustParse`
- `Create` generates UUID and timestamps if not set
- `GetAll` returns sorted by name (D-15: alphabetical order)
- `Delete` checks `RowsAffected()` and returns "not found" error if 0
- No encryption needed (unlike SNMP profile repo)
- No onChange channel needed (areas don't feed into the polling/cache system)

### Pattern 3: API Handler (area_handler.go)
**What:** HTTP handler for CRUD operations following the SNMP profile handler pattern
**When to use:** Every REST resource gets its own handler file
**Key details from existing code:**
- Constructor: `NewAreaHandler(repo domain.AreaRepository, deviceRepo domain.DeviceRepository)`
- Response envelope: `{"data": {...}}` for single, `{"data": [...]}` for list
- Unique constraint violation detected via `strings.Contains(err.Error(), "UNIQUE")` -- returns 409 Conflict
- Name validation: `strings.TrimSpace(req.Name) == ""` returns 400
- Delete returns 204 No Content
- Area list response should include `device_count` per area (requires a COUNT query or JOIN)

### Pattern 4: Router Registration (router.go)
**What:** Register area routes in the existing hand-rolled mux
**When to use:** Every new API resource
**Example:**
```go
// Source: internal/api/router.go (existing snmp-profiles pattern)
areaHandler := NewAreaHandler(areaRepo, deviceRepo)

mux.HandleFunc("/api/v1/areas", func(w http.ResponseWriter, r *http.Request) {
    switch r.Method {
    case http.MethodGet:
        areaHandler.HandleList(w, r)
    case http.MethodPost:
        areaHandler.HandleCreate(w, r)
    default:
        writeError(w, http.StatusMethodNotAllowed, "method not allowed")
    }
})

mux.HandleFunc("/api/v1/areas/", func(w http.ResponseWriter, r *http.Request) {
    switch r.Method {
    case http.MethodGet:
        areaHandler.HandleGet(w, r)
    case http.MethodPut:
        areaHandler.HandleUpdate(w, r)
    case http.MethodDelete:
        areaHandler.HandleDelete(w, r)
    default:
        writeError(w, http.StatusMethodNotAllowed, "method not allowed")
    }
})
```

### Pattern 5: Device Update Extension for Area Assignment
**What:** Add `area_id` to the device update flow using the established double-pointer pattern
**When to use:** Nullable FK fields on device updates
**Critical pattern from existing code (ssh_profile_id):**
```go
// Source: internal/service/device_service.go line 33
type DeviceUpdate struct {
    // ... existing fields ...
    AreaID **uuid.UUID // double pointer: nil=not set, *nil=unassign, **=set
}

// Source: internal/api/device_handler.go lines 215-233 (ssh_profile_id pattern)
// In HandleUpdate:
if req.AreaID != nil {
    if *req.AreaID == "" {
        // Explicitly unassign
        update.AreaID = new(*uuid.UUID)
        *update.AreaID = nil
    } else {
        parsed, err := uuid.Parse(*req.AreaID)
        if err != nil {
            writeError(w, http.StatusBadRequest, "invalid area_id")
            return
        }
        // Optionally validate area exists
        update.AreaID = new(*uuid.UUID)
        *update.AreaID = &parsed
    }
}
```

### Pattern 6: Frontend Manager Component (AreaManager.tsx)
**What:** Inline list CRUD component following SNMPProfileManager pattern
**When to use:** Any settings-panel entity management
**Key structural elements from SNMPProfileManager:**
- State: `mode` (`'list' | 'create' | 'edit'`), `editing` (current entity), `confirmDeleteId`
- `load()` function fetches all areas on mount and after mutations
- Mode-based rendering: list view, create form, edit form
- Delete confirmation inline within the list card
- Form component extracted as a child (e.g., `AreaForm`)
- Back arrow button to return to list view from create/edit

### Anti-Patterns to Avoid
- **Do NOT create an AreaService layer:** Areas have no business logic beyond CRUD -- the handler should use the repo directly (same as SNMPProfileHandler)
- **Do NOT add area data to WebSocket snapshots in Phase 3:** Areas are management data, not real-time metrics. Phase 4 may need area info for filtering, but a simple REST fetch is sufficient. Adding it to WS would couple the polling system to static config data
- **Do NOT use the JSON:API resource envelope for areas:** The SNMP profile handler uses a simpler `{"data": {...}}` envelope, not the full `jsonAPIResource` type/id/attributes structure. Areas should follow the simpler pattern since they don't have relationships
- **Do NOT enforce area_id FK in SQLite migrations for existing rows:** The `area_id` column must default to NULL so existing devices are unassigned. Use `ALTER TABLE devices ADD COLUMN area_id TEXT REFERENCES areas(id) ON DELETE SET NULL`

## Don't Hand-Roll

| Problem | Don't Build | Use Instead | Why |
|---------|-------------|-------------|-----|
| UUID generation | Custom ID scheme | `uuid.New()` | All entities use UUID; consistent with codebase |
| Migration management | Manual SQL execution | `golang-migrate` with embedded .sql files | Project already uses this; migration runner handles versioning |
| Color validation | Regex-based hex validation | Simple `strings.HasPrefix(color, "#") && len(color) == 7` | Only 7 valid values in the curated palette; backend just validates format |
| Frontend color picker | Full color picker widget | 7-swatch button grid | D-01 locks this to a curated palette; a full picker is scope creep |

**Key insight:** This phase is pure CRUD with established patterns. The risk is not in technical complexity but in missing integration points (device repo scan columns, device handler update flow, WebSocket message types, frontend type parsing).

## Common Pitfalls

### Pitfall 1: SQLite ALTER TABLE ADD COLUMN with REFERENCES
**What goes wrong:** SQLite has limited ALTER TABLE support. Adding a column with a REFERENCES clause works, but the FK constraint is only enforced if `PRAGMA foreign_keys = ON`. The project enables this via the connection string (`?_foreign_keys=on`).
**Why it happens:** Developers forget that FK constraints need explicit enabling in SQLite.
**How to avoid:** Verify the connection string in `cmd/theia/main.go` already has `_foreign_keys=on` (it does: line 63). The `ON DELETE SET NULL` clause on `area_id` ensures device area assignment is cleared when an area is deleted.
**Warning signs:** Tests pass but FK violations not caught -- means foreign_keys pragma not enabled in test DB.

### Pitfall 2: Missing Column in Device Scan Functions
**What goes wrong:** Adding `area_id` to the `devices` table requires updating EVERY SQL query that reads from devices. The device repo has 3 scan-related code paths: `scanDevice` (single `*sql.Row`), `scanDeviceRow` (from `*sql.Rows`), and ALL `SELECT` statements in `GetByID`, `GetByIP`, `GetBySysName`, `GetAll`.
**Why it happens:** Easy to add the column to the migration and `Create`/`Update` but forget to update all `SELECT` queries and both scan functions.
**How to avoid:** Search for every `SELECT.*FROM devices` in `device_repo.go` (there are 4 query sites + 2 scan functions + the `INSERT` and `UPDATE` statements). All must include `area_id`.
**Warning signs:** `database/sql: Scan error on column index X` at runtime.

### Pitfall 3: Device Handler deviceToResource Missing area_id
**What goes wrong:** The `deviceToResource` helper in `device_handler.go` manually builds the attributes map. If `area_id` is not added there, the frontend never receives area assignment data even though it's stored in the database.
**Why it happens:** The `deviceToResource` function uses explicit field mapping (not reflection), so new fields must be manually added.
**How to avoid:** Add `area_id` to the attributes map in `deviceToResource`, following the `ssh_profile_id` pattern (only include if non-nil).
**Warning signs:** Frontend shows "Unassigned" for all devices even after assignment.

### Pitfall 4: Frontend parseDevicesResponse Missing area_id
**What goes wrong:** The `parseDevicesResponse` function in `frontend/src/types/api.ts` manually extracts each field from the JSON response. If `area_id` is not extracted, the frontend Device type will always have `undefined` for area_id.
**Why it happens:** Same as backend -- manual field extraction, not automatic deserialization.
**How to avoid:** Add `area_id` extraction in `parseDevicesResponse` following the `ssh_profile_id` pattern: `area_id: typeof attributes.area_id === 'string' ? attributes.area_id : undefined`.
**Warning signs:** Area dropdown in DeviceConfigPanel always shows "Unassigned" despite database having the assignment.

### Pitfall 5: Delete Area Cascade vs Device Orphaning
**What goes wrong:** If `ON DELETE SET NULL` is not set on the FK, deleting an area will either fail (if FK enforcement is on) or leave dangling area_id references.
**Why it happens:** Default FK behavior in SQLite with foreign_keys enabled is `NO ACTION` (which rejects the delete if referenced).
**How to avoid:** The migration MUST specify `ON DELETE SET NULL` on the `area_id` column. Per D-10, deleting an area unassigns devices (sets area_id to NULL).
**Warning signs:** 500 error when deleting an area that has assigned devices.

### Pitfall 6: NewRouter Signature Change
**What goes wrong:** Adding `areaRepo` as a parameter to `NewRouter` changes its function signature. All call sites (main.go, and potentially tests) must be updated.
**Why it happens:** The router wires all dependencies manually via constructor injection.
**How to avoid:** Update `cmd/theia/main.go` to create `areaRepo := sqlite.NewAreaRepo(db)` and pass it to `api.NewRouter(...)`. Also update any test files that call `NewRouter`.
**Warning signs:** Compile error in `main.go` or test files.

### Pitfall 7: Area Device Count in List Response
**What goes wrong:** D-06 specifies that collapsed area cards show a "device count badge." If the area list API doesn't include device counts, the frontend needs a separate query per area, causing N+1 requests.
**Why it happens:** The basic CRUD pattern doesn't include computed fields.
**How to avoid:** Include `device_count` in the area list response. Two approaches: (a) LEFT JOIN with COUNT in the GetAll query, or (b) a separate `CountDevicesByAreaID` method called in the handler. Approach (a) is more efficient for the list view -- a single query like `SELECT a.*, COUNT(d.id) as device_count FROM areas a LEFT JOIN devices d ON d.area_id = a.id GROUP BY a.id ORDER BY a.name`.
**Warning signs:** Area list loads slowly or shows no device counts.

## Code Examples

### Migration 000007_areas.up.sql
```sql
-- Source: Pattern from 000001_initial_schema.up.sql + 000002_device_columns.up.sql
CREATE TABLE IF NOT EXISTS areas (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    color TEXT NOT NULL DEFAULT '#00E676',
    created_at DATETIME NOT NULL,
    updated_at DATETIME NOT NULL
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_areas_name ON areas(name);

ALTER TABLE devices ADD COLUMN area_id TEXT REFERENCES areas(id) ON DELETE SET NULL;
```

### Migration 000007_areas.down.sql
```sql
-- SQLite does not support DROP COLUMN directly before 3.35.0,
-- but the golang-migrate down migration can recreate the table.
-- For simplicity, since area_id is nullable and non-destructive:
DROP INDEX IF EXISTS idx_areas_name;
DROP TABLE IF EXISTS areas;
-- Note: ALTER TABLE devices DROP COLUMN area_id; works in SQLite 3.35.0+
-- Debian Bookworm ships SQLite 3.40.1, so this is safe.
ALTER TABLE devices DROP COLUMN area_id;
```

### Area Domain Type (area.go)
```go
// Source: Pattern from internal/domain/snmp_profile.go
package domain

import (
    "time"
    "github.com/google/uuid"
)

type Area struct {
    ID          uuid.UUID `json:"id"`
    Name        string    `json:"name"`
    Description string    `json:"description"`
    Color       string    `json:"color"`
    CreatedAt   time.Time `json:"created_at"`
    UpdatedAt   time.Time `json:"updated_at"`
}

type AreaRepository interface {
    Create(area *Area) error
    GetByID(id uuid.UUID) (*Area, error)
    GetAll() ([]Area, error)
    Update(area *Area) error
    Delete(id uuid.UUID) error
    GetAllWithDeviceCount() ([]AreaWithCount, error)
}

type AreaWithCount struct {
    Area
    DeviceCount int `json:"device_count"`
}
```

### Device Struct Update (device.go addition)
```go
// Add to Device struct (after SSHProfileID field):
AreaID *uuid.UUID `json:"area_id,omitempty"`
```

### DeviceUpdate Struct Extension (device_service.go)
```go
// Add to DeviceUpdate struct:
AreaID **uuid.UUID // double pointer: nil=not set, *nil=unassign, **=set
```

### Area Handler Response Type (area_handler.go)
```go
// Source: Pattern from snmp_profile_handler.go
type areaRequest struct {
    Name        string `json:"name"`
    Description string `json:"description"`
    Color       string `json:"color"`
}

type areaResponse struct {
    ID          string `json:"id"`
    Name        string `json:"name"`
    Description string `json:"description"`
    Color       string `json:"color"`
    DeviceCount int    `json:"device_count"`
    CreatedAt   string `json:"created_at"`
    UpdatedAt   string `json:"updated_at"`
}
```

### Frontend Area Interface (api.ts addition)
```typescript
// Source: Pattern from SNMPProfile interface
export interface Area {
  id: string;
  name: string;
  description: string;
  color: string;
  device_count: number;
  created_at: string;
  updated_at: string;
}

// Add to Device interface:
// area_id?: string;
```

### Frontend API Client Functions (client.ts additions)
```typescript
// Source: Pattern from SNMP profile CRUD functions
export async function fetchAreas(): Promise<Area[]> {
  const payload = await requestJSON('/api/v1/areas');
  // Parse response following existing patterns
}

export async function createArea(payload: { name: string; description: string; color: string }): Promise<Area> {
  // POST /api/v1/areas
}

export async function updateArea(id: string, payload: { name: string; description: string; color: string }): Promise<Area> {
  // PUT /api/v1/areas/{id}
}

export async function deleteArea(id: string): Promise<void> {
  // DELETE /api/v1/areas/{id}
}
```

### Color Palette Constant (frontend constant)
```typescript
// Curated area accent colors per D-01
export const AREA_COLORS = [
  '#00E676', // green (primary, default per D-03)
  '#2979FF', // blue
  '#E040FB', // purple
  '#FFEA00', // amber
  '#FF6D00', // orange
  '#00BCD4', // cyan
  '#FF1744', // red
] as const;
```

### Color Swatch Rendering Pattern
```tsx
{/* Inline color swatch dot for area cards and dropdown options */}
<span
  className="inline-block h-3 w-3 rounded-full shrink-0"
  style={{ backgroundColor: area.color }}
/>
```

## State of the Art

| Old Approach | Current Approach | When Changed | Impact |
|--------------|------------------|--------------|--------|
| SVG inline icons (Phase 1) | MaterialIcon component (Phase 2) | Phase 2 | Use MaterialIcon for area section icons and action buttons |
| Heroicon SVGs | Material Symbols subset font | Phase 2 | Area icons should use MaterialIcon, not inline SVGs |
| Class-based theme | data-theme attribute | Phase 1 | All area UI uses CSS variable tokens, not hardcoded colors |
| Tailwind v3 | Tailwind v4 with `@theme` | Phase 1 | Area component uses v4 token classes |

**Note on Phase 2 completion:** Phase 2 is marked as complete, but some COMP requirements (COMP-05, COMP-07, COMP-08, COMP-09) are still Pending per REQUIREMENTS.md. Phase 3 area management UI should follow the established Phase 2 styling patterns (Neon Topography tokens, no-line rule, glassmorphism for overlays, Material Symbols icons). The area UI will be added to SettingsPanel (COMP-05 pending) and DeviceConfigPanel (COMP-08 pending) -- the planner should be aware that these components may still have Phase 2 restyling pending, but Phase 3 area additions should use the new token system regardless.

## Discretion Recommendations

Based on research of existing patterns, here are recommendations for the Claude's Discretion items:

### Migration Number
**Recommendation:** Use `000007` -- the last migration is `000006_drop_legacy_tables`, so the next sequential number is 000007.

### Area Handler Structure
**Recommendation:** Follow `snmp_profile_handler.go` exactly -- handler struct with repo dependency, request/response types, helper function to convert domain to response. Do NOT create an area service; the handler should use the repo directly.

### AreaManager State Management
**Recommendation:** Use the same `mode`-based state machine as `SNMPProfileManager.tsx`: `'list' | 'create' | 'edit'`. Add a `devicesForArea` state field for the bidirectional device assignment view (D-09). The device list for the expanded area should be fetched via the existing `fetchDevices()` client function and filtered client-side.

### area_id Filter on GET /api/v1/devices
**Recommendation:** Do NOT add area_id filtering to the devices endpoint in Phase 3. Phase 4 (area-filtered canvas) will need this, but Phase 3 only needs device assignment/unassignment. Adding it now is premature and adds untested surface area. The frontend can filter client-side for the AreaManager's device list.

### Area Name Max Length
**Recommendation:** Apply a 100-character limit on area name in the backend handler. This prevents abuse without being overly restrictive. Enforce with `if len(strings.TrimSpace(req.Name)) > 100 { writeError(w, 400, "area name too long (max 100 characters)") }`.

### WebSocket Snapshot Inclusion
**Recommendation:** Do NOT include area data in WebSocket snapshots for Phase 3. Areas are static configuration, not real-time metrics. The frontend should fetch areas via REST on mount. Phase 4 can add area info to snapshots if needed for the filtered canvas view.

## Validation Architecture

### Test Framework
| Property | Value |
|----------|-------|
| Framework (Go) | Go standard `testing` package |
| Framework (Frontend) | Vitest 4.1 with jsdom + @testing-library/react 16.3 |
| Config file (Frontend) | `frontend/vitest.config.ts` |
| Quick run command (Go) | `go test ./internal/api/ -run TestArea -count=1` |
| Quick run command (Frontend) | `cd frontend && npx vitest run src/components/AreaManager.test.tsx` |
| Full suite command (Go) | `go test ./internal/...` |
| Full suite command (Frontend) | `cd frontend && npx vitest run` |

### Phase Requirements to Test Map
| Req ID | Behavior | Test Type | Automated Command | File Exists? |
|--------|----------|-----------|-------------------|-------------|
| AREA-01 | areas table created with correct columns | integration | `go test ./internal/repository/sqlite/ -run TestAreaRepo -count=1` | Wave 0 |
| AREA-02 | Area CRUD API returns correct status codes and JSON | unit | `go test ./internal/api/ -run TestArea -count=1` | Wave 0 |
| AREA-03 | devices.area_id column exists and FK works | integration | `go test ./internal/repository/sqlite/ -run TestDeviceArea -count=1` | Wave 0 |
| AREA-04 | Device update with area_id assigns/unassigns | unit | `go test ./internal/api/ -run TestDeviceUpdateArea -count=1` | Wave 0 |
| AREA-05 | AreaManager renders list, create, edit, delete flows | component | `cd frontend && npx vitest run src/components/AreaManager.test.tsx` | Wave 0 |
| AREA-06 | DeviceConfigPanel area dropdown renders and saves | component | `cd frontend && npx vitest run src/components/DeviceConfigPanel.test.tsx` | Exists (extend) |

### Sampling Rate
- **Per task commit:** `go test ./internal/api/ -run TestArea -count=1 && go test ./internal/repository/sqlite/ -run TestArea -count=1`
- **Per wave merge:** `go test ./internal/... && cd frontend && npx vitest run`
- **Phase gate:** Full suite green before `/gsd:verify-work`

### Wave 0 Gaps
- [ ] `internal/api/area_handler_test.go` -- covers AREA-02 (area CRUD API tests)
- [ ] `internal/repository/sqlite/area_repo_test.go` -- covers AREA-01, AREA-03 (schema and repo tests)
- [ ] `frontend/src/components/AreaManager.test.tsx` -- covers AREA-05 (area management UI)
- [ ] Extend existing `internal/api/device_handler_test.go` -- covers AREA-04 (device area assignment)
- [ ] Extend existing `frontend/src/components/DeviceConfigPanel.test.tsx` -- covers AREA-06 (area dropdown)

## Open Questions

1. **Bidirectional device assignment implementation (D-09)**
   - What we know: Users should be able to add/remove devices from an area's expanded edit view
   - What's unclear: Whether this should use a dedicated API endpoint (e.g., `POST /api/v1/areas/{id}/devices`) or batch individual device updates via the existing `PUT /api/v1/devices/{id}` endpoint
   - Recommendation: Use the existing device update endpoint. The AreaManager fetches all devices, shows a multi-select or add/remove UI, and for each change calls `updateDevice(deviceId, { area_id: areaId })` or `updateDevice(deviceId, { area_id: '' })` to unassign. This avoids a new API endpoint and keeps the device as the single source of truth for area assignment. The downside is multiple API calls for bulk reassignment, but with fewer than 100 devices this is fine.

2. **Device list in area edit view (D-08)**
   - What we know: Expanded area edit should show assigned devices as a read-only list, PLUS allow bidirectional assignment (D-09)
   - What's unclear: Whether "read-only list" and "add/remove" are the same UI or separate sections
   - Recommendation: Show assigned devices as a list with a remove (X) button each, plus an "Add Device" dropdown below that shows unassigned devices. This combines D-08 and D-09 naturally.

3. **Area list response: device_count computation**
   - What we know: D-06 requires a device count badge on collapsed cards
   - What's unclear: Whether to compute in SQL (JOIN) or in Go (separate query)
   - Recommendation: Use a `GetAllWithDeviceCount()` method on the repo that does a LEFT JOIN with COUNT in a single query. This is the most efficient approach for the list view. The individual `GetByID` can return just the area without count.

## Sources

### Primary (HIGH confidence)
- `internal/domain/snmp_profile.go` -- Domain type and repository interface pattern
- `internal/repository/sqlite/snmp_profile_repo.go` -- SQLite repo implementation pattern
- `internal/api/snmp_profile_handler.go` -- REST handler CRUD pattern
- `internal/api/device_handler.go` -- Device update flow, double-pointer nullable FK pattern (lines 215-233)
- `internal/service/device_service.go` -- DeviceUpdate struct double-pointer pattern (line 33)
- `internal/repository/sqlite/device_repo.go` -- Device scan functions, SQL query patterns
- `internal/api/router.go` -- Route registration and NewRouter signature
- `cmd/theia/main.go` -- Dependency wiring and repo initialization
- `frontend/src/components/SNMPProfileManager.tsx` -- Inline list CRUD UI pattern
- `frontend/src/components/DeviceConfigPanel.tsx` -- Device form with dropdown fields
- `frontend/src/components/SettingsPanel.tsx` -- Settings panel section ordering
- `frontend/src/types/api.ts` -- TypeScript type definitions and parse functions
- `frontend/src/api/client.ts` -- API client function patterns

### Secondary (MEDIUM confidence)
- `internal/repository/sqlite/migrations/` -- Migration numbering (000006 is latest, 000007 is next)
- `.planning/DESIGN.md` -- Neon Topography design system, color palette reference

### Tertiary (LOW confidence)
- None -- all findings are from direct codebase inspection

## Metadata

**Confidence breakdown:**
- Standard stack: HIGH -- No new dependencies; pure application of existing patterns
- Architecture: HIGH -- Every pattern verified against existing code in the same codebase
- Pitfalls: HIGH -- All pitfalls derived from direct code inspection of existing integration points (device_repo.go scan functions, deviceToResource helper, parseDevicesResponse)

**Research date:** 2026-03-26
**Valid until:** Indefinite (patterns are internal to this codebase, not external library versions)
