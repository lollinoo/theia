# Frontend Capability Refactor Design

Date: 2026-04-20
Status: Approved for planning
Scope: Frontend refactor slice to split topology composition, rendering, WinBox flow, form state, and side panels by capability while keeping external contracts stable initially

## Summary

This slice refactors the frontend along existing capability seams instead of continuing to let backend-shaped DTOs leak through rendering, panel orchestration, and form state.

The current risk is concentrated in a few high-gravity paths. `frontend/src/components/Canvas.tsx` coordinates topology composition, runtime state interpretation, bridge and WinBox flow, menu actions, area rendering, and panel orchestration. `frontend/src/components/canvas/useCanvasData.ts` mixes topology loading, runtime merge semantics, Prometheus outage behavior, node and edge mutation, reconnect handling, and rendering preparation. `frontend/src/components/DeviceConfigPanel.tsx`, `frontend/src/components/AddDevicePanel.tsx`, and `frontend/src/components/BulkEditPanel.tsx` use backend DTO shape as live UI state. `frontend/src/components/canvas/CanvasPanels.tsx` still performs panel-specific business adaptation inline.

The design keeps REST and WebSocket contracts unchanged at the boundary at first, but tightens the internal frontend boundary immediately. Backend-specific fields such as `metrics_source`, `prometheus_label_name`, `prometheus_label_value`, and the raw semantics of `prometheus_status` become adapter concerns instead of rendering concerns. Canvas, dashboard, side panels, WinBox interactions, and forms consume focused internal models and view models.

## Current Context

Relevant current files:

- `frontend/src/types/api.ts`: frontend REST contract types and parsing still expose `metrics_source` and Prometheus label fields directly as part of `Device`.
- `frontend/src/types/metrics.ts`: WebSocket contract types and helpers still expose raw snapshot and `prometheus_status` semantics to the rest of the UI.
- `frontend/src/hooks/useWebSocket.ts`: merges incoming snapshot and alert state, then publishes raw runtime payloads to consumers.
- `frontend/src/components/Canvas.tsx`: current canvas facade owns bridge health, WinBox availability, launch flow, area rendering, overlay state, and panel orchestration.
- `frontend/src/components/canvas/useCanvasData.ts`: combines topology fetch lifecycle with runtime interpretation and node and edge mutation.
- `frontend/src/components/canvas/CanvasPanels.tsx`: routes panels and adapts panel data inline.
- `frontend/src/components/AddDevicePanel.tsx`, `frontend/src/components/DeviceConfigPanel.tsx`, and `frontend/src/components/BulkEditPanel.tsx`: keep backend request shape in local form state and couple UI decisions directly to backend-oriented fields.
- `frontend/src/components/AlertsPanel.tsx` and `frontend/src/components/InterfaceStatsPanel.tsx`: render with direct awareness of `metrics_source` and Prometheus outage logic.
- `frontend/src/components/dashboard/DeviceTable.tsx` and `frontend/src/components/dashboard/DeviceRow.tsx`: consume raw snapshot DTO sections directly for runtime display and sorting.
- `docs/superpowers/specs/2026-04-20-backend-capability-refactor-design.md`: previous slice explicitly left frontend refactor for the follow-on slice.

## Goals

- Split frontend gravity files by capability rather than adding another shallow helper extraction.
- Keep external REST and WebSocket contracts stable at the start of the slice.
- Tighten internal frontend boundaries immediately so rendering does not depend directly on backend-specific DTO semantics.
- Separate topology composition, runtime rendering state, WinBox flow, form state and submit mapping, panel orchestration, and dashboard runtime rendering.
- Preserve current visible behavior while making later backend contract simplification possible with limited frontend blast radius.

## Non-Goals

- Changing backend REST request or response shapes in this slice.
- Changing WebSocket payload contracts in this slice.
- Replacing the current canvas, dashboard, or panel UX with a new product design.
- Reorganizing the frontend into a brand-new top-level package hierarchy.
- Moving every UI concern to a new state-management library.
- Broad dashboard redesign beyond the rendering boundary adjustments needed here.

## Design Principles

This slice follows one operational rule: keep external contracts unchanged initially, but tighten internal frontend boundaries immediately.

That means:

- adapters own backend-specific semantics;
- rendering consumes internal models, not raw wire concerns;
- forms own UI-first state and only reconstruct backend DTOs at submit time;
- feature facades remain stable where they already provide useful boundaries;
- the refactor follows existing repository seams instead of inventing new architectural layers unrelated to current structure.

## Proposed Architecture

The frontend becomes a three-layer flow for this feature set:

1. External contracts.
   REST and WebSocket payloads remain represented by `types/api.ts` and `types/metrics.ts`.

2. Internal adapters.
   A small set of capability-focused adapters interpret backend semantics once. This is the only layer that should understand fields like `metrics_source` and the exact meaning of `prometheus_status`.

3. Feature view models.
   Canvas rendering, dashboard rendering, panel rendering, WinBox actions, and form state consume internal models shaped around UI needs rather than transport needs.

This keeps public API boundaries stable while making internal ownership explicit.

### External Contract Boundary

`frontend/src/types/api.ts` and `frontend/src/types/metrics.ts` remain the contract layer for real backend payloads. They continue parsing the current backend responses and WebSocket messages. This slice does not remove backend-shaped fields from those files.

What changes is who consumes them. Raw contract types stop being the default input for rendering and panel logic. They become boundary-only types used by adapters, submit mappers, and API-facing hooks.

### Runtime Adapter Boundary

Runtime adapters own the interpretation of snapshot, alert, and Prometheus availability state.

Responsibilities:

- derive effective device status;
- derive whether Prometheus outage changes rendered availability for a device;
- decide whether link metrics are usable;
- preserve health and freshness semantics already exposed by the backend runtime model;
- expose device and link runtime state through internal models instead of raw DTO sections.

This boundary replaces today’s scattered logic in `useCanvasData.ts`, `AlertsPanel.tsx`, `InterfaceStatsPanel.tsx`, `Canvas.tsx`, and dashboard components.

### Topology Composition Boundary

Topology composition owns the merge of fetched topology and normalized runtime state.

Responsibilities:

- combine devices, links, positions, and runtime state into canvas node and edge view models;
- keep area filtering and ghost-node behavior intact;
- keep topology and runtime merge rules out of `Canvas.tsx`;
- provide focused composition outputs for rendering instead of mutating node and edge semantics inline.

`useAreaFilteredTopology.ts` already reflects a good seam and should remain a focused boundary instead of being pulled back into a larger hook.

### Panel Boundary

`CanvasPanels.tsx` remains the panel router, but it stops preparing business semantics inline.

Responsibilities after the refactor:

- route panel types to the correct panel component;
- pass already adapted view models or form models into those panels;
- keep refresh and close callbacks at the routing layer;
- avoid inline interpretation of runtime semantics or backend DTO fields.

Panel-specific adapters should own the data preparation currently embedded in conditional blocks.

### Form Boundary

Add, edit, and bulk-edit flows move to UI-first form state.

Responsibilities:

- keep local state shaped around UI concepts and validation needs;
- treat backend DTO construction as a submit concern, not a rendering concern;
- centralize current defaulting and fallback behavior for `metrics_source` and Prometheus label values inside submit mappers;
- preserve existing form behavior and validation outcomes.

This keeps backend-specific details confined to form adapter and submit mapping code, which is an acceptable boundary because the form must still speak the current backend contract.

### WinBox Flow Boundary

WinBox availability and launch behavior become their own capability boundary instead of being partially owned by `Canvas.tsx`, `useBridgeHealth.ts`, and `useDeviceWinboxAvailability.ts` independently.

Responsibilities:

- bridge health status;
- per-device WinBox availability state;
- launch action and launch error mapping;
- updates from `DeviceConfigPanel` when WinBox designation changes.

This preserves the existing user-visible flow while removing the current ad hoc orchestration from the canvas container.

### Dashboard Runtime Boundary

Dashboard runtime rendering should consume internal runtime device view models instead of raw snapshot DTO access.

Responsibilities:

- provide row-level runtime values such as uptime and status display;
- keep sorting rules based on internal runtime state;
- reuse runtime interpretation shared with canvas where semantics overlap;
- avoid duplicating snapshot interpretation logic inside table or row components.

## File Structure

### Keep Contract Files

- Keep: `frontend/src/types/api.ts`
  Responsibility: backend REST contract types and parsing only.
- Keep: `frontend/src/types/metrics.ts`
  Responsibility: backend WebSocket contract types and parsing only, plus transport-level helpers.

### Canvas and Runtime

- Keep: `frontend/src/components/Canvas.tsx`
  Responsibility: stable canvas facade and high-level orchestration only.
- Keep but shrink: `frontend/src/components/canvas/useCanvasData.ts`
  Responsibility: topology lifecycle orchestration, not full runtime semantic ownership.
- Keep: `frontend/src/components/canvas/useAreaFilteredTopology.ts`
  Responsibility: area filtering and ghost-device identification.
- Create: `frontend/src/components/canvas/runtimeAdapters.ts`
  Responsibility: normalize runtime device and link state from snapshot, alerts, Prometheus status, and device topology data.
- Create: `frontend/src/components/canvas/topologyComposer.ts`
  Responsibility: combine topology, positions, and normalized runtime state into canvas node and edge view models.
- Create: `frontend/src/components/canvas/panelAdapters.ts`
  Responsibility: prepare panel-specific view models for canvas-routed panels.

### Panels and WinBox

- Keep: `frontend/src/components/canvas/CanvasPanels.tsx`
  Responsibility: panel routing only.
- Create: `frontend/src/hooks/useWinboxFlow.ts`
  Responsibility: bridge health, per-device availability, launch action, and launch error lifecycle.
- Keep but narrow: `frontend/src/hooks/useBridgeHealth.ts`
  Responsibility: low-level bridge health check primitive if still needed by the new WinBox boundary.
- Keep but narrow: `frontend/src/hooks/useDeviceWinboxAvailability.ts`
  Responsibility: low-level availability fetch primitive if still needed by the new WinBox boundary.

### Forms

- Keep: `frontend/src/components/AddDevicePanel.tsx`
  Responsibility: add-device panel container and rendering only.
- Keep: `frontend/src/components/DeviceConfigPanel.tsx`
  Responsibility: device configuration panel container and rendering only.
- Keep: `frontend/src/components/BulkEditPanel.tsx`
  Responsibility: bulk-edit panel container and rendering only.
- Create: `frontend/src/components/forms/deviceFormModels.ts`
  Responsibility: UI-first form model types and initialization helpers for add and edit flows.
- Create: `frontend/src/components/forms/deviceFormSubmitters.ts`
  Responsibility: map form models to backend DTOs for create and update requests.
- Create: `frontend/src/components/forms/bulkEditModels.ts`
  Responsibility: UI-first bulk-edit state and mapping helpers.

### Dashboard

- Keep: `frontend/src/components/dashboard/DeviceTable.tsx`
  Responsibility: table rendering and user interactions only.
- Keep: `frontend/src/components/dashboard/DeviceRow.tsx`
  Responsibility: row rendering only.
- Create: `frontend/src/components/dashboard/runtimeDeviceRows.ts`
  Responsibility: build dashboard row models and sorting inputs from internal runtime state.

## Component Design

### Canvas Facade

`Canvas.tsx` remains the stable feature facade. It keeps ownership of React Flow integration, high-level menu wiring, selected-area handling, and component composition.

It should no longer directly own:

- bridge and WinBox coordination details;
- Prometheus outage interpretation;
- topology and runtime merge rules;
- panel-specific data preparation.

### Runtime Adapters

Runtime adapters are pure transformation helpers that accept fetched devices, links, snapshot state, alerts, and Prometheus availability, then produce normalized runtime state.

They should expose concepts such as:

- effective device status;
- device monitoring and telemetry availability;
- link metrics usability;
- panel-consumable runtime summaries;
- dashboard-consumable runtime summaries.

They should not expose backend-oriented fields as the main output shape.

### Topology Composer

The topology composer is the owner of “how the canvas state is built,” not “how React renders it.”

Responsibilities:

- compose node device state and metrics presentation from topology plus normalized runtime state;
- compose edge runtime state and throughput visibility;
- preserve current ghost-node and area-aware rendering behavior;
- keep node and edge mutation decisions deterministic and testable outside the component body.

### Panel Adapters

Panel adapters should be small focused helpers. They prepare each panel’s inputs from the normalized models instead of having the router or panel reconstruct those semantics ad hoc.

Examples:

- alerts panel model with affected-device groupings;
- interface stats model with “metrics unavailable” already decided;
- device config panel model with UI-friendly field defaults;
- bulk-edit model with common-value and mixed-state semantics.

### Form Models and Submitters

Form model helpers define what the UI edits. Submitters define what the backend receives.

This keeps the form responsibilities split cleanly:

- UI state: form fields, validation state, toggles, defaults, and derived visibility;
- backend DTO mapping: current request schema, metrics source mapping, Prometheus label fallback, and omission of unused fields.

### WinBox Flow

The WinBox boundary is a focused capability layer around launch readiness and launch actions.

It owns:

- current bridge health state;
- launchability for a given device;
- refresh after device config changes;
- user-visible launch failure text.

It does not own the device menu UI itself.

### Dashboard Row Models

Dashboard row-model helpers bridge runtime normalization into dashboard rendering.

They should decide:

- displayed uptime;
- displayed status state and label;
- row sort inputs tied to runtime state;
- any shared display fallbacks that already exist across dashboard and canvas.

`DeviceTable.tsx` and `DeviceRow.tsx` should focus on table and row presentation.

## Data Flow

### External to Internal

1. API hooks and WebSocket hooks receive backend payloads.
2. Contract types parse those payloads into boundary-only DTOs.
3. Runtime adapters and form-model initializers translate DTOs into internal models.

### Canvas Runtime Flow

1. Topology data loads through existing topology lifecycle.
2. WebSocket snapshot, alerts, and Prometheus status update through existing subscription flow.
3. Runtime adapters derive normalized runtime state.
4. Topology composer merges topology, runtime state, positions, and area filtering into node and edge view models.
5. Canvas renders those outputs without reinterpreting backend-specific fields inline.

### Dashboard Runtime Flow

1. Devices and runtime state feed dashboard row-model helpers.
2. Row models expose sort-ready and render-ready runtime fields.
3. Table and row components render those values without raw snapshot interpretation.

### Form Flow

1. Panel opens with a form model initialized from topology device data and panel context.
2. User edits UI-first state.
3. Validation runs against the UI model.
4. Submit mappers translate the UI model back into current backend request DTOs.
5. Successful mutation updates local topology state through the existing callbacks.

### WinBox Flow

1. Canvas or panel requests WinBox state for a device.
2. WinBox boundary consults bridge health and device WinBox designation state.
3. Menu state and actions consume the resulting launchability model.
4. Launch errors and designation changes propagate through the same boundary instead of separate ad hoc paths.

## Error Handling

This slice does not introduce new user-visible failure semantics. It changes where fallback and interpretation live.

- Adapters must tolerate partial but valid external payloads.
- Fallbacks for incomplete contract data happen in adapter and form-model code, not scattered through render branches.
- Panel “no data” behavior remains, but the decision should be centralized in the panel input preparation path.
- WinBox launch and bridge errors keep current messaging expectations while moving into the WinBox capability boundary.

The main regression risk is not transport failure. It is leaving backend-specific interpretation scattered in components after the new adapters exist.

## Migration Strategy

The migration is incremental and in place.

1. Add internal runtime adapters, topology composer helpers, panel adapters, and form-model helpers without immediately deleting old call sites.
2. Move canvas runtime interpretation to the new adapter and composer boundaries.
3. Move panel routing to consume prepared panel models.
4. Move dashboard runtime display to row-model helpers.
5. Move add, edit, and bulk-edit flows to UI-first form models and submit mappers.
6. Move WinBox coordination to a dedicated flow boundary.
7. Remove direct component dependencies on backend-specific DTO semantics that have become redundant.

Temporary duplication during migration is acceptable. The slice is complete only when rendering paths no longer depend directly on backend-specific fields except inside boundary adapters and submit mappers.

## Testing Strategy

Tests should shift to the new boundaries instead of only updating existing container tests.

### Runtime Adapter Tests

Verify:

- effective device status derivation;
- Prometheus outage behavior for Prometheus-only and fallback devices;
- link metrics usability decisions;
- preservation of backend-provided health and freshness signals.

### Topology Composition Tests

Verify:

- merge of topology and runtime state into node and edge models;
- ghost-node behavior remains unchanged;
- area filtering behavior remains unchanged;
- throughput labels or metrics are hidden when link metrics are not usable.

### Panel Adapter Tests

Verify:

- alerts panel grouping and Prometheus outage messaging inputs;
- interface stats panel inputs;
- device config and bulk-edit prepared models.

### Form Model and Submit Mapper Tests

Verify:

- add and edit form initialization from current device data;
- mapping from UI-first form state to unchanged backend DTOs;
- current label fallback and metrics-source behavior.

### WinBox Flow Tests

Verify:

- bridge health state propagation;
- availability refresh per device;
- launch error mapping;
- updates after WinBox designation changes in device configuration.

### Dashboard Rendering Tests

Verify:

- row-model runtime values for status and uptime;
- runtime-based sorting behavior;
- dashboard components render row models without direct raw snapshot coupling.

## Open Decisions Resolved For This Slice

- Scope includes topology composition, WinBox flow, form state, canvas rendering, side panels, and dashboard rendering.
- External contracts stay unchanged at first.
- Internal frontend boundaries tighten immediately.
- The refactor is structural plus boundary-tightening, not a product redesign.

## Expected Outcome

After this slice:

- the frontend still talks to the same backend contracts;
- canvas and dashboard rendering consume internal runtime models rather than backend-shaped DTO semantics;
- panel routing stops doing business adaptation inline;
- forms stop using backend DTO shape as their live state model;
- WinBox coordination is owned by a dedicated boundary;
- a later backend contract simplification can land with much smaller frontend churn.
