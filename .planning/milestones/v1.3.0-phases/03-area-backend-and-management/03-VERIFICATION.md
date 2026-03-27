---
phase: 03-area-backend-and-management
verified: 2026-03-26T10:15:15Z
status: human_needed
score: 6/6 must-haves verified
human_verification:
  - test: "Open Settings panel and verify AreaManager form fields render with correct design token styling"
    expected: "AreaManager form inputs use the same visual style as SNMPProfileManager inputs (same border color, background, text color)"
    why_human: "AreaManager.tsx uses stale token names (bg-bg-elevated, text-text-primary, border-border-subtle, focus:border-accent) that are undefined in the Neon Topography CSS token system. Tailwind v4 silently ignores undefined class names. The component renders and functions correctly, but may appear visually inconsistent (e.g., white backgrounds or no border) compared to other panels."
  - test: "Create an area in Settings, restart the app, and verify the area persists"
    expected: "Area created before restart appears in the area list after reload"
    why_human: "Persistence across app restarts requires a running app with real SQLite storage; cannot verify programmatically without running the stack"
  - test: "Assign a device to an area in DeviceConfigPanel, click Save, then reload the page and confirm assignment persists"
    expected: "The area dropdown in DeviceConfigPanel shows the previously assigned area after page reload"
    why_human: "Requires a running app with devices and areas; e2e state round-trip cannot be verified statically"
---

# Phase 3: Area Backend and Management Verification Report

**Phase Goal:** Users can create and manage OSPF areas in Settings and assign devices to areas, backed by persistent storage and REST API
**Verified:** 2026-03-26T10:15:15Z
**Status:** human_needed
**Re-verification:** No — initial verification

## Goal Achievement

### Observable Truths

| # | Truth | Status | Evidence |
|---|-------|--------|----------|
| 1 | User can create a new area in Settings with a name, description, and accent color -- and it persists across app restarts | ? UNCERTAIN | AreaManager component exists with create form, 7-color swatch picker, name/description fields, and calls `createArea()` backed by SQLite migration 000007. Persistence requires human verification with running stack. |
| 2 | User can edit and delete existing areas in Settings | ✓ VERIFIED | AreaManager mode-state machine implements edit (inline form pre-populated, calls `updateArea()`), delete with confirmation showing device count (calls `deleteArea()`), and bidirectional device assignment. 5 Vitest tests pass. |
| 3 | User can assign a device to an area via a dropdown in DeviceConfigPanel -- and unassign it by clearing the selection | ✓ VERIFIED | DeviceConfigPanel has `<select>` dropdown at line 327–338 with "Unassigned" default, populates from `fetchAreas()`, saves `area_id: areaId || ''` on Save (empty string unassigns). 2 dedicated tests pass. |
| 4 | Area data is available via REST API (GET/POST/PUT/DELETE /api/v1/areas) and devices include their area assignment | ✓ VERIFIED | router.go registers all 5 area routes; area_handler.go implements all verbs; device_handler.go includes area_id in request/response (lines 80, 235–247, 403–404); 7 handler tests + 1 device area update test pass. |
| 5 | Area list includes device_count per area computed via LEFT JOIN | ✓ VERIFIED | area_repo.go GetAllWithDeviceCount() executes LEFT JOIN query (lines 78–84); TestAreaRepo_GetAllWithDeviceCount passes. |
| 6 | Deleting an area sets area_id to NULL on assigned devices (ON DELETE SET NULL) | ✓ VERIFIED | Migration 000007 defines `area_id TEXT REFERENCES areas(id) ON DELETE SET NULL`; TestAreaRepo_DeleteSetsDeviceAreaIDToNull passes. |

**Score:** 6/6 truths verified (1 requires human confirmation for full end-to-end persistence)

### Required Artifacts

| Artifact | Expected | Status | Details |
|----------|----------|--------|---------|
| `internal/domain/area.go` | Area struct, AreaWithCount, AreaRepository interface | ✓ VERIFIED | 34 lines; exports Area, AreaWithCount, AreaRepository with 6-method interface |
| `internal/repository/sqlite/area_repo.go` | SQLite AreaRepo implementing AreaRepository | ✓ VERIFIED | 153 lines; full CRUD with GetAllWithDeviceCount LEFT JOIN |
| `internal/repository/sqlite/migrations/000007_areas.up.sql` | areas table schema and devices.area_id column | ✓ VERIFIED | CREATE TABLE areas + UNIQUE INDEX + ALTER TABLE devices ADD COLUMN area_id with ON DELETE SET NULL |
| `internal/api/area_handler.go` | HTTP handler for area CRUD | ✓ VERIFIED | 212 lines; HandleList, HandleCreate, HandleGet, HandleUpdate, HandleDelete with validation and 409 on duplicate name |
| `internal/repository/sqlite/area_repo_test.go` | Integration tests for area repo CRUD and device-area FK | ✓ VERIFIED | 6 tests pass: CreateAndGetByID, GetAllWithDeviceCount, UniqueNameConstraint, DeleteSetsDeviceAreaIDToNull, UpdateAndDelete, GetAll_OrderByName |
| `internal/api/area_handler_test.go` | Unit tests for area handler HTTP endpoints | ✓ VERIFIED | 7 tests pass: List, Create_HappyPath, Create_DefaultColor, Create_DuplicateName_409, Create_EmptyName_400, Delete_204, Delete_NotFound_404 |
| `frontend/src/types/api.ts` | Area interface and area_id on Device | ✓ VERIFIED | Area interface at line 310; area_id on Device at line 49; parseAreasResponse/parseAreaResponse at lines 410/428 |
| `frontend/src/api/client.ts` | fetchAreas, createArea, updateArea, deleteArea functions | ✓ VERIFIED | All 4 functions present at lines 437–457; area_id in updateDevice payload at line 198 |
| `frontend/src/components/AreaManager.tsx` | Area management component with inline list CRUD | ✓ VERIFIED | 387 lines; AREA_COLORS constant, AreaForm child component, AreaManager with list/create/edit modes, device assignment |
| `frontend/src/components/AreaManager.test.tsx` | Component tests for AreaManager CRUD behavior | ✓ VERIFIED | 5 tests pass: empty state, list rendering, create mode, create submission, delete confirmation |
| `frontend/src/components/SettingsPanel.tsx` | AreaManager imported and placed above SNMP Profiles | ✓ VERIFIED | AreaManager at line 264, SNMPProfileManager at line 268 — correct order |
| `frontend/src/components/DeviceConfigPanel.tsx` | Area assignment dropdown with color swatch preview | ✓ VERIFIED | Area select at lines 327–338, color swatch preview at lines 340–350, uses dynamic color from fetched area data |

**Note on DeviceConfigPanel `contains: "AREA_COLORS"` check:** The plan specified `AREA_COLORS` as a must-have pattern in DeviceConfigPanel. The constant is defined in AreaManager.tsx instead. DeviceConfigPanel uses dynamic `areas.find(...)?.color` from API data to show color swatches — which is functionally equivalent and more correct. This is not a gap.

### Key Link Verification

| From | To | Via | Status | Details |
|------|----|-----|--------|---------|
| `internal/api/area_handler.go` | `internal/domain/area.go` | `domain.AreaRepository` interface | ✓ WIRED | `domain.AreaRepository` used as field type; all 6 interface methods called |
| `internal/api/router.go` | `internal/api/area_handler.go` | `NewAreaHandler` constructor + route registration | ✓ WIRED | `areaHandler := NewAreaHandler(areaRepo)` at line 40; all 5 routes registered at lines 204–226 |
| `cmd/theia/main.go` | `internal/repository/sqlite/area_repo.go` | `sqlite.NewAreaRepo` wired into `api.NewRouter` | ✓ WIRED | `areaRepo := sqlite.NewAreaRepo(db)` at line 122; passed to `api.NewRouter(...)` at line 174 |
| `internal/api/device_handler.go` | `internal/service/device_service.go` | `DeviceUpdate.AreaID` double-pointer field | ✓ WIRED | `AreaID **uuid.UUID` at device_service.go line 34; handler sets it at lines 235–247 |
| `frontend/src/components/AreaManager.tsx` | `frontend/src/api/client.ts` | `fetchAreas, createArea, updateArea, deleteArea` imports | ✓ WIRED | All 4 functions imported at lines 4–7 and called in component |
| `frontend/src/components/SettingsPanel.tsx` | `frontend/src/components/AreaManager.tsx` | AreaManager component import and render | ✓ WIRED | `import { AreaManager }` at line 3; `<AreaManager />` at line 264 |
| `frontend/src/components/DeviceConfigPanel.tsx` | `frontend/src/api/client.ts` | `fetchAreas` import for dropdown population | ✓ WIRED | `fetchAreas` imported at line 3; called at line 77 with `setAreas` state setter |

### Data-Flow Trace (Level 4)

| Artifact | Data Variable | Source | Produces Real Data | Status |
|----------|---------------|--------|-------------------|--------|
| `AreaManager.tsx` | `areas: Area[]` | `fetchAreas()` → `/api/v1/areas` → `GetAllWithDeviceCount()` → SQLite LEFT JOIN | Yes — real DB query with device counts | ✓ FLOWING |
| `AreaManager.tsx` | `allDevices: Device[]` | `fetchDevices()` → `/api/v1/devices` → SQLite device table | Yes — real DB query | ✓ FLOWING |
| `DeviceConfigPanel.tsx` | `areas: Area[]` | `fetchAreas()` → `/api/v1/areas` → SQLite | Yes — real DB query | ✓ FLOWING |
| `DeviceConfigPanel.tsx` | `areaId: string` | `device.area_id` prop (from parent device data) | Yes — from device object, initialized from area_id field set by DB | ✓ FLOWING |

### Behavioral Spot-Checks

| Behavior | Command | Result | Status |
|----------|---------|--------|--------|
| Area repo tests pass (6 integration tests with real SQLite) | `docker compose run --rm backend go test ./internal/repository/sqlite/... -run "Area" -v` | 6 tests PASS, migration version=7 | ✓ PASS |
| Area handler tests pass (7 unit tests with mock repo) | `docker compose run --rm backend go test ./internal/api/... -run "TestAreaHandler" -v` | 7 tests PASS | ✓ PASS |
| Device area assignment test passes | `docker compose run --rm backend go test ./internal/api/... -run "TestDeviceHandlerUpdate_AreaID" -v` | 1 test PASS | ✓ PASS |
| AreaManager Vitest tests pass (5 component tests) | `npx vitest run src/components/AreaManager.test.tsx` | 5 tests PASS | ✓ PASS |
| DeviceConfigPanel Vitest tests pass (9 tests including 2 area tests) | `npx vitest run src/components/DeviceConfigPanel.test.tsx` | 9 tests PASS | ✓ PASS |
| TypeScript compiles cleanly | `npx tsc --noEmit` | No output (zero errors) | ✓ PASS |
| Frontend builds cleanly | `npx vite build` | Built in 1.20s, no errors | ✓ PASS |

### Requirements Coverage

| Requirement | Source Plan | Description | Status | Evidence |
|-------------|------------|-------------|--------|----------|
| AREA-01 | 03-01-PLAN.md | Database schema includes `areas` table with id, name, description, and accent color fields | ✓ SATISFIED | migration 000007_areas.up.sql: `CREATE TABLE IF NOT EXISTS areas (id, name, description, color, ...)` |
| AREA-02 | 03-01-PLAN.md | REST API endpoints for area CRUD (`/api/v1/areas` — GET, POST, PUT, DELETE) | ✓ SATISFIED | router.go lines 204–226 register all verbs; area_handler.go implements all 5 handlers |
| AREA-03 | 03-01-PLAN.md | Devices table has nullable `area_id` foreign key referencing the areas table | ✓ SATISFIED | migration: `ALTER TABLE devices ADD COLUMN area_id TEXT REFERENCES areas(id) ON DELETE SET NULL` |
| AREA-04 | 03-01-PLAN.md | REST API supports assigning/unassigning a device to an area via device update endpoint | ✓ SATISFIED | device_handler.go lines 235–247 handle area_id assignment; TestDeviceHandlerUpdate_AreaID passes |
| AREA-05 | 03-02-PLAN.md | User can create, edit, and delete areas in the Settings panel | ✓ SATISFIED | AreaManager.tsx with full CRUD, integrated into SettingsPanel.tsx above SNMPProfileManager; 5 tests pass |
| AREA-06 | 03-02-PLAN.md | User can assign a device to an area via a dropdown in DeviceConfigPanel | ✓ SATISFIED | DeviceConfigPanel area dropdown (line 327), saves with Save button (line 198), 2 tests pass |

All 6 requirements for Phase 3 are satisfied. No orphaned requirements found.

### Anti-Patterns Found

| File | Lines | Pattern | Severity | Impact |
|------|-------|---------|----------|--------|
| `frontend/src/components/AreaManager.tsx` | 24–25, 107, 193, 222, 241, 244–249, 270 | Stale design token names: `border-border-subtle`, `bg-bg-elevated`, `text-text-primary`, `text-text-secondary`, `focus:border-accent` — not defined in the Neon Topography CSS system | ⚠️ Warning | Undefined Tailwind v4 classes silently produce no styling. AreaManager form fields will render without the correct background/border/text colors from the design token system. Functional CRUD works correctly; visual styling is inconsistent with SNMPProfileManager and other panels. Does NOT block goal achievement (all behaviors work), but creates visual regression risk in Phase 4. |

**Note:** The `placeholder="..."` strings matched by the anti-pattern scan are HTML input attributes, not code stubs — false positives, not flagged.

### Human Verification Required

#### 1. AreaManager Visual Styling

**Test:** Open the app, navigate to Settings, observe the Areas section. Interact with the create form (click "New Area"), inspect the input field styling.
**Expected:** Input fields in AreaManager should visually match the styling of input fields in the SNMPProfileManager section directly below it — same border color, same background, same text color.
**Why human:** AreaManager.tsx uses stale token class names (`bg-bg-elevated`, `text-text-primary`, `border-border-subtle`, `focus:border-accent`) that are undefined in the Neon Topography CSS system. Tailwind v4 silently drops unknown classes. The actual rendered output — whether it falls back to browser defaults or happens to look acceptable — requires visual inspection.

#### 2. Area Persistence Across Restarts

**Test:** Create an area named "TestArea" with a blue color swatch. Stop the backend. Start it again. Navigate to Settings > Areas.
**Expected:** "TestArea" appears in the area list after restart.
**Why human:** Requires running the full Docker stack with a persistent SQLite volume. Cannot verify programmatically without executing the app.

#### 3. Device-Area Assignment Round Trip

**Test:** Open a device's config panel. Select an area from the Area dropdown. Click Save. Reload the page. Open the same device's config panel.
**Expected:** The Area dropdown shows the previously selected area after page reload.
**Why human:** Requires running app with devices and areas already created; round-trip state persistence through backend and back to UI cannot be verified statically.

### Gaps Summary

No functional gaps found. All 6 requirements are implemented and tested. The phase goal is achieved at the code level:

- Area CRUD REST API is fully functional with 14 passing Go tests
- SQLite persistence layer with migration 000007 implements the areas table and device FK
- AreaManager component in SettingsPanel provides create/edit/delete with color swatches and device assignment
- DeviceConfigPanel area dropdown saves assignment with the Save button
- TypeScript compiles cleanly; Vite build succeeds; all 7 Vitest tests pass

The only finding is a **warning-level visual issue**: AreaManager.tsx uses stale Tailwind class names from before the Neon Topography token migration. These classes are silently ignored by Tailwind v4, causing AreaManager form fields to render without proper design token styling. The functionality is not impaired. This should be fixed before Phase 4 to maintain visual consistency.

Human verification is required to confirm the visual rendering is acceptable and that the persistence round-trip works with a running stack.

---

_Verified: 2026-03-26T10:15:15Z_
_Verifier: Claude (gsd-verifier)_
