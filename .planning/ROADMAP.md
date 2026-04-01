# Roadmap: MikroTik Theia

## Milestones

- ✅ **v1.3.0 Frontend Redesign** — Phases 1-7 (shipped 2026-03-27) — [archive](milestones/v1.3.0-ROADMAP.md)

## Phases

### Active

- [x] **Phase 8: Virtual Device Backend** (2/2 plans) — completed 2026-03-31
- [ ] **Phase 9: Virtual Node Rendering** (1/2 plans complete) — VIRT-06, VIRT-07, VIRT-08, VIRT-09, VIRT-14, VIRT-15
- [ ] **Phase 10: Virtual Node Forms and Context Menu** — VIRT-10, VIRT-11, VIRT-12, VIRT-13, VIRT-16

## Phase Details

### Phase 9: Virtual Node Rendering
**Goal**: Virtual nodes render as compact cards with subtype-specific Material Symbol icons, status indicators for IP-bearing nodes, and link edge labels showing real interface tx/rx throughput
**Depends on**: Phase 8 (Virtual Device Backend)
**Requirements**: VIRT-06, VIRT-07, VIRT-08, VIRT-09, VIRT-14, VIRT-15
**Plans:** 2 plans
Plans:
- [x] 09-01-PLAN.md — Virtual card rendering with type system extension, font subset, and DeviceCard branch
- [ ] 09-02-PLAN.md — Edge builder virtual link adaptation and findLinkMetrics fallback
**Success Criteria** (what must be TRUE):
  1. Virtual node on canvas shows a compact card with the correct Material Symbol icon for its subtype (language=Internet, cloud=Cloud, dns=Server, hub=Generic)
  2. Virtual node with IP shows a StatusDot and IP address line in a 200px card
  3. Virtual node without IP shows only icon and label in a 160px card with no body section
  4. Material Symbols font subset includes language, cloud, and dns glyphs
  5. Link edge connecting to a virtual node displays the real (physical) interface's tx/rx throughput
  6. Link bandwidth label for virtual links shows only the real interface speed with no mismatch indicator

### Phase 10: Virtual Node Forms and Context Menu
**Goal**: Users can create virtual nodes through a toggle-based form with subtype selection, link creation adapts for virtual devices, and context menu omits irrelevant physical-device actions
**Depends on**: Phase 9 (Virtual Node Rendering)
**Requirements**: VIRT-10, VIRT-11, VIRT-12, VIRT-13, VIRT-16
**Success Criteria** (what must be TRUE):
  1. AddDevicePanel shows Physical Device / Virtual Node toggle at top of form
  2. Virtual form shows subtype radio group, required display name, and optional IP field
  3. LinkCreatePanel hides interface selector when one side is a virtual device
  4. Link creation rejects both devices being virtual with a validation error
  5. Canvas context menu for virtual nodes omits WebFig and Per-Interface Stats actions

<details>
<summary>✅ v1.3.0 Frontend Redesign (Phases 1-7) — SHIPPED 2026-03-27</summary>

- [x] Phase 1: Design Token Foundation and Theme Infrastructure (3/3 plans) — completed 2026-03-25
- [x] Phase 2: Component Restyling (6/6 plans) — completed 2026-03-26
- [x] Phase 3: Area Backend and Management (2/2 plans) — completed 2026-03-26
- [x] Phase 4: Area Hub View and Filtered Topology (4/4 plans) — completed 2026-03-26
- [x] Phase 5: Redesign the Devices Page (3/3 plans) — completed 2026-03-26
- [x] Phase 6: Canvas Token Migration and Theme Compliance (2/2 plans) — completed 2026-03-27
- [x] Phase 7: SettingsPanel Verification and Phase 2 Closure (1/1 plan) — completed 2026-03-27

</details>

## Progress

| Phase | Milestone | Plans Complete | Status | Completed |
|-------|-----------|----------------|--------|-----------|
| 1. Design Token Foundation | v1.3.0 | 3/3 | Complete | 2026-03-25 |
| 2. Component Restyling | v1.3.0 | 6/6 | Complete | 2026-03-26 |
| 3. Area Backend and Management | v1.3.0 | 2/2 | Complete | 2026-03-26 |
| 4. Area Hub View and Filtered Topology | v1.3.0 | 4/4 | Complete | 2026-03-26 |
| 5. Redesign the Devices Page | v1.3.0 | 3/3 | Complete | 2026-03-26 |
| 6. Canvas Token Migration | v1.3.0 | 2/2 | Complete | 2026-03-27 |
| 7. SettingsPanel Verification | v1.3.0 | 1/1 | Complete | 2026-03-27 |
| 8. Virtual Device Backend | v1.3.7 | 2/2 | Complete | 2026-03-31 |
| 9. Virtual Node Rendering | v1.3.7 | 1/2 | In progress | — |
| 10. Virtual Node Forms | v1.3.7 | 0/? | Not started | — |
