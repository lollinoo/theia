# Phase 44: Frontend Integration - Context

**Gathered:** 2026-04-13
**Status:** Ready for planning

<domain>
## Phase Boundary

Integrate the new SNMP pipeline's backend-owned device state into the canvas-facing frontend so every canvas device card shows backend-computed health, freshness, and polling cadence metadata, and so operators can override a device's polling cadence from the device configuration panel without refreshing the page. This phase is about surfacing backend-owned state cleanly in the existing canvas card/panel system. It does **not** redesign the dashboard/table surfaces, it does **not** invent new frontend-only health logic, and it does **not** expand Phase 43's subscription ownership beyond the canvas device panel.

</domain>

<decisions>
## Implementation Decisions

### Card Status Semantics
- **D-01:** The primary card status signal stays health-owned. The existing status dot / glow on `DeviceCard` must represent the backend `health` enum (`healthy`, `warning`, `critical`, `unknown`), not reachability, freshness, or a frontend-computed composite.
- **D-02:** Freshness remains a separate secondary signal. `Fresh` / `Stale` / `Dead` must never override the card's primary health signal on the canvas.
- **D-03:** The card should expose a compact explicit health label in addition to the existing color signal. Do not rely on color alone.

### Freshness Presentation
- **D-04:** Freshness should show both the tier and the actual age, e.g. `Fresh · 12s ago` or `Dead · 6m ago`.
- **D-05:** Freshness copy uses the plain canonical labels `Fresh`, `Stale`, and `Dead`. The threshold math stays implicit in the UI.
- **D-06:** Freshness remains visually secondary to health on the device card. It belongs in compact metadata, not as a competing header-level signal.

### Polling Cadence Language And Override UX
- **D-07:** Canvas card polling copy must be operator-facing time language such as `Polling every 30s`, not backend jargon like `core polling`.
- **D-08:** The device configuration panel should show the backend-assigned class/default cadence as read-only context, then let the operator keep that default or set a custom seconds override.
- **D-09:** The operator-facing control is override-first, not class-editing-first. Direct editable `core` / `standard` / `low` switching is not the primary interaction in this phase.
- **D-10:** Polling override changes should save inline from the polling section itself and take effect on the next poll cycle without a page refresh.

### Card Layout Integration
- **D-11:** The small health label should live in the header alongside the existing status dot / glow so the primary status meaning is obvious where the eye already lands first.
- **D-12:** Freshness and polling cadence should share one compact metadata row in the card body. Phase 44 should extend the existing card hierarchy rather than redesigning the whole card.
- **D-13:** The existing secondary device detail/model row should remain available for device identity context. Phase 44 should add status metadata, not replace the descriptive device text.

### Data Ownership And Surface Behavior
- **D-14:** Card presentation is backend-owned. The frontend must not derive health severity from raw metrics or invent a composite health/freshness status model.
- **D-15:** Canvas cards should be able to render health, freshness, and polling metadata directly from the shared live snapshot state for the map view itself; opening a device panel is not a prerequisite for the operator to see those card-level states.

### the agent's Discretion
- Exact header markup for the compact health label (plain text, micro-badge, or equivalent), as long as it stays visually subordinate to the hostname and aligned with the existing Neon Topography card system.
- Exact relative-time formatting granularity (`12s ago`, `1m ago`, `5m ago`) and refresh cadence for freshness text.
- Exact badge/metadata styling for `Fresh`, `Stale`, and `Dead`, provided freshness remains a secondary signal.
- Exact migration strategy from the current legacy per-device settings-key polling control to the device-backed poll-class / override model, as long as the resulting UX matches D-08 through D-10.

</decisions>

<canonical_refs>
## Canonical References

**Downstream agents MUST read these before planning or implementing.**

### Phase scope and acceptance criteria
- `.planning/ROADMAP.md` §Phase 44 — canonical goal, success criteria, and dependency boundary with Phases 42 and 43.
- `.planning/REQUIREMENTS.md` — `WS-01`, `WS-03`, `WS-04`, and `POLL-06` define the required frontend-visible outcomes.
- `.planning/PROJECT.md` §Current Milestone / §Key Decisions — milestone intent, backend-owned health/freshness direction, and keep-the-existing-delta-layer constraints.
- `.planning/STATE.md` §Accumulated Context — carries forward phase-to-phase decisions around state ownership, detail subscriptions, and pipeline behavior.

### Prior phase decisions that constrain Phase 44
- `.planning/phases/43-websocket-detail-on-demand/43-CONTEXT.md` — detail subscriptions remain canvas-panel-owned and additive within the shared snapshot contract.
- `.planning/phases/43-websocket-detail-on-demand/43-UI-SPEC.md` — Phase 43 deliberately deferred visible health/freshness/polling presentation to Phase 44 while preserving the 320px side panel and single-snapshot model.
- `.planning/phases/39-domain-types-db-migration/39-CONTEXT.md` — polling model foundation: `poll_class`, `poll_interval_override`, operator-facing override intent, and explicit note that frontend display/control belongs in Phase 44.
- `.planning/phases/38-state-engine/38-CONTEXT.md` — health vs reachability vs staleness are separate backend dimensions, with stale as independent of health.

### Frontend visual language and card constraints
- `.planning/milestones/v1.3.0-phases/02-component-restyling/02-CONTEXT.md` — DeviceCard glow semantics, no-line rule, and severity-aware visual hierarchy.
- `.planning/milestones/v1.3.0-phases/02-component-restyling/02-UI-SPEC.md` §Glow System / §DeviceCard Restyling — existing dot/glow semantics, 10px status marker, and card interaction hierarchy.
- `.planning/DESIGN.md` — Neon Topography design-system rules, especially status panels, glow-node restraint, and the no-line rule.
- `frontend/src/index.css` — current token definitions, theme-adaptive status colors, and existing canvas/component CSS variables.

### Research and rollout guidance
- `.planning/research/SUMMARY.md` §Phase 7: Frontend Integration — recommended operator-facing copy style (`Polling every Ns`) and the low-risk additive nature of the frontend work.
- `.planning/research/FEATURES.md` §Freshness indicators — tier model (`Fresh`, `Stale`, `Dead`) and rationale for showing last-polled information on the map.
- `.planning/research/PITFALLS.md` — especially the freshness-init and additive-snapshot warnings so Phase 44 does not show stale data immediately on load or split state ownership.

### Existing code to modify
- `frontend/src/components/DeviceCard.tsx` — current 260px card structure, header/body split, metrics strip, and memo comparator.
- `frontend/src/components/StatusDot.tsx` — existing 10px glow-node implementation and status color language.
- `frontend/src/components/canvas/nodeBuilder.ts` — current snapshot-to-card node data mapping.
- `frontend/src/components/canvas/useCanvasData.ts` — snapshot merge path, stale-data timer, and topology/device state propagation into canvas nodes.
- `frontend/src/components/DeviceConfigPanel.tsx` — current polling override UI and inline autosave behavior that still uses legacy settings keys.
- `frontend/src/components/Canvas.tsx` — canvas panel ownership, device-panel open/close lifecycle, and the phase's main device-card surface.
- `frontend/src/hooks/useWebSocket.ts` — single snapshot atom and shared `snapshot_delta` merge path.
- `frontend/src/types/metrics.ts` — additive device-metrics fields already available on the client DTO.
- `frontend/src/types/api.ts` — frontend `Device` model, which currently does not surface `poll_class` / `poll_interval_override`.
- `frontend/src/api/client.ts` — `updateDevice()` shape and current lack of polling-field support.
- `internal/ws/messages.go` — snapshot DTO fields for `health`, `reachability`, `stale`, `last_polled_at`, and `expected_poll_interval_seconds`.
- `internal/worker/snapshot_builder.go` — current distinction between overview snapshot building and selected-device detail delta enrichment.
- `internal/api/device_handler.go` — current device update request shape, which does not yet accept polling override fields.
- `internal/service/device_service.go` — device domain update seam and existing `poll_class` / `poll_interval_override` ownership.
- `internal/domain/device.go` and `internal/domain/poll_class.go` — canonical poll-class and override model.

</canonical_refs>

<code_context>
## Existing Code Insights

### Reusable Assets
- `DeviceCard` already has a stable header/body/metrics hierarchy and should absorb Phase 44 as an additive metadata extension rather than a full redesign.
- `StatusDot` already implements the 10px glow-node visual language from the earlier design phases, so Phase 44 can reuse that system rather than inventing new status chrome.
- `useWebSocket()` plus `frontend/src/types/metrics.ts` already maintain one shared snapshot state atom and merge additive `snapshot_delta` payloads without panel-local mirrors.
- `DeviceConfigPanel` already has an inline autosave interaction pattern for the polling section, which matches the user's chosen override UX.

### Established Patterns
- The frontend is expected to consume additive backend-owned fields through the shared snapshot contract, not through a second selected-device state path.
- Device-card visual hierarchy is intentionally dense but ordered: header first, then identity/body metadata, then the metrics strip. Phase 44 should preserve that order.
- Canvas behavior is single-panel-oriented; device-panel lifecycle already owns selected-device detail subscriptions and should remain the only detail-owner surface.
- The backend domain already models `poll_class` and `poll_interval_override`, but the current frontend API/types still lag behind that model.

### Integration Points
- Phase 44 will likely need the overview snapshot/device-card path to surface the same backend-owned health/freshness/polling metadata that Phase 43 proved for selected-device detail.
- `nodeBuilder.ts`, `useCanvasData.ts`, and `DeviceCard.tsx` are the core canvas-card presentation seam for health/freshness/polling metadata.
- `DeviceConfigPanel.tsx`, `frontend/src/types/api.ts`, `frontend/src/api/client.ts`, and `internal/api/device_handler.go` are the main seam for replacing the current legacy per-device settings-key polling override with the device-backed poll model.
- The planner should explicitly account for the current contract gap: `poll_class` / `poll_interval_override` exist in the Go domain but are not yet accepted by `updateDevice()` or represented in the frontend `Device` type.

</code_context>

<specifics>
## Specific Ideas

- Header status should read like a compact operator summary: existing glow node plus a small explicit health label such as `Healthy`, `Warning`, `Critical`, or `Unknown`.
- Freshness should use plain tier labels with relative age, e.g. `Fresh · 12s ago`, `Stale · 2m ago`, `Dead · 9m ago`.
- Polling cadence should read in operator language such as `Polling every 30s`, not in backend class terminology.
- The device configuration panel should communicate the default class/cadence as context, then make the editable action about setting or clearing a seconds override.
- Phase 44 should feel like a clean integration into the existing card system, not like a second mini-dashboard crammed into the node.

</specifics>

<deferred>
## Deferred Ideas

- Direct user editing of `core` / `standard` / `low` as the primary polling control instead of an override-first UX.
- Dashboard/table parity for the same health/freshness/polling metadata if that becomes desirable outside the canvas card surface.
- Freshness muting the whole card or overriding the primary health signal when a device becomes `Dead`.
- Replacing the existing secondary device detail/model row with status metadata or performing a broader card redesign.

</deferred>

---

*Phase: 44-frontend-integration*
*Context gathered: 2026-04-13*
