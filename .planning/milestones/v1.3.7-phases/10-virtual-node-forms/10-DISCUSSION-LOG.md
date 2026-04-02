# Phase 10: Virtual Node Forms and Context Menu - Discussion Log

> **Audit trail only.** Do not use as input to planning, research, or execution agents.
> Decisions are captured in CONTEXT.md — this log preserves the alternatives considered.

**Date:** 2026-04-01
**Phase:** 10-virtual-node-forms
**Areas discussed:** Form toggle UX, Virtual subtype selector, Link creation for virtual, Context menu filtering

---

## Form Toggle UX

| Option | Description | Selected |
|--------|-------------|----------|
| Segmented control at top | Two-segment pill (Physical Device / Virtual Node) above all fields, swaps entire field set | ✓ |
| Dropdown at top | Select/dropdown labeled 'Device Type' — less prominent but simpler | |
| Separate panel/route | Completely separate 'Add Virtual Node' panel opened via different button | |

**User's choice:** Segmented control at top
**Notes:** Preferred the visual clarity of a prominent toggle with preview mockup

### Follow-up: Form state on mode switch

| Option | Description | Selected |
|--------|-------------|----------|
| Reset fields on switch | Switching modes clears all fields, clean slate | ✓ |
| Preserve shared fields | Fields in both modes (IP, display name, areas) keep values | |

**User's choice:** Reset fields on switch
**Notes:** Avoids confusing leftover values like SNMP community strings

---

## Virtual Subtype Selector

| Option | Description | Selected |
|--------|-------------|----------|
| Icon radio cards | 2x2 grid of selectable cards with Material Symbol icon + label | ✓ |
| Plain radio buttons | Standard vertical radio list with text labels only | |
| Dropdown select | Single select dropdown with 4 options | |

**User's choice:** Icon radio cards
**Notes:** Previews what the canvas node will look like; visually matches the design system

---

## Link Creation for Virtual

| Option | Description | Selected |
|--------|-------------|----------|
| Hide interface selector entirely | When virtual device selected, interface selector disappears, "(virtual node — no interface)" label shown | ✓ |
| Show disabled selector with message | Selector stays visible but disabled with placeholder text | |
| Auto-fill with placeholder | Field auto-fills with synthetic value like 'virtual-link' | |

**User's choice:** Hide interface selector entirely
**Notes:** Backend already accepts empty if_name for virtual sides (Phase 8 D-12)

### Follow-up: Both-virtual validation

| Option | Description | Selected |
|--------|-------------|----------|
| Inline error on second select | Show validation message below field, disable Create button | ✓ |
| Disable virtual options in second dropdown | Grey out virtual devices when first is virtual | |

**User's choice:** Inline error on second select
**Notes:** No server roundtrip needed, clear message "At least one device must be physical"

---

## Context Menu Filtering

| Option | Description | Selected |
|--------|-------------|----------|
| Hide irrelevant items | Remove WebFig and Per-Interface Stats entirely for virtual nodes | ✓ |
| Show but disable | Keep all 4 items visible, grey out irrelevant ones | |
| Hide WebFig only | Keep Per-Interface Stats for virtual links' physical side | |

**User's choice:** Hide irrelevant items
**Notes:** Virtual nodes show only Open in Grafana and Configure — cleaner 2-item menu

---

## Claude's Discretion

- Segmented control styling details (Tailwind classes, active/inactive states)
- Icon radio card internal layout and spacing
- Conditional rendering structure in AddDevicePanel
- Virtual validation extraction (shared util vs inline)

## Deferred Ideas

None — discussion stayed within phase scope.
