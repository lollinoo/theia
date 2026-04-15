# Phase 44: Frontend Integration - Discussion Log

> **Audit trail only.** Do not use as input to planning, research, or execution agents.
> Decisions are captured in CONTEXT.md — this log preserves the alternatives considered.

**Date:** 2026-04-13
**Phase:** 44-frontend-integration
**Areas discussed:** Status hierarchy, Freshness presentation, Polling override model, Card layout density

---

## Status Hierarchy

### Primary Signal Ownership

| Option | Description | Selected |
|--------|-------------|----------|
| Health-first | Primary dot/glow is driven by backend health. | ✓ |
| Reachability/freshness-first | Primary dot/glow emphasizes recency or availability first. | |
| Composite operator state | Frontend fuses health and freshness into one worst-state signal. | |

**User's choice:** Health-first
**Notes:** The user wants the card's strongest signal to follow the backend health enum directly, not a frontend-computed fusion rule.

### Freshness Interaction With Primary Status

| Option | Description | Selected |
|--------|-------------|----------|
| Secondary freshness only | Freshness is shown separately and never overrides the main health signal. | ✓ |
| Mute card when dead | Keep health primary, but visually desaturate the card when freshness is dead. | |
| Override on dead | Let dead/unreachable freshness replace the primary health signal. | |

**User's choice:** Secondary freshness only
**Notes:** The user explicitly kept freshness secondary even when data is overdue.

### Health Label Visibility

| Option | Description | Selected |
|--------|-------------|----------|
| Dot/glow only | Keep health encoded only in color and glow. | |
| Add a small health label | Pair the primary signal with explicit text. | ✓ |
| Text only in panel/detail surfaces | Keep cards minimal and defer explicit health text to expanded views. | |

**User's choice:** Add a small health label
**Notes:** The user wants better scan clarity and not a color-only read.

---

## Freshness Presentation

### Card Copy

| Option | Description | Selected |
|--------|-------------|----------|
| Tier badge + relative age | Show both freshness tier and age, e.g. `Fresh · 12s ago`. | ✓ |
| Tier badge only | Show only `Fresh`, `Stale`, or `Dead`. | |
| Relative age only | Show only the age text and let the operator infer the tier. | |

**User's choice:** Tier badge + relative age
**Notes:** The user wants both category and actual recency on the card.

### Freshness Vocabulary

| Option | Description | Selected |
|--------|-------------|----------|
| Generic tiers, simple copy | Use canonical labels `Fresh`, `Stale`, `Dead`. | ✓ |
| Threshold-aware copy | Explain the threshold state in the label itself. | |
| Age-driven wording | Use looser human phrasing like `Late` or `Expired`. | |

**User's choice:** Generic tiers, simple copy
**Notes:** The user prefers concise, canonical status words over explanatory phrasing.

### Visual Weight

| Option | Description | Selected |
|--------|-------------|----------|
| Subtle metadata | Freshness is visible but secondary to health. | ✓ |
| Header-level signal | Freshness shares near-top billing with health. | |
| Conditionally prominent | Freshness escalates visually when stale/dead. | |

**User's choice:** Subtle metadata
**Notes:** Freshness should be present, but not compete with the primary health read.

---

## Polling Override Model

### Operator Control Model

| Option | Description | Selected |
|--------|-------------|----------|
| Simple seconds override | Show only default/custom interval values. | |
| Class label + optional seconds override | Show class/default context, but let the operator edit only the seconds override. | ✓ |
| Editable class selector | Make class switching a first-class editable control. | |

**User's choice:** Class label + optional seconds override
**Notes:** The user wants the backend class model visible, but not as the main control language.

### Card Copy Style

| Option | Description | Selected |
|--------|-------------|----------|
| Operator-facing time label | Use human-readable cadence like `Polling every 30s`. | ✓ |
| Class-first label | Show `Core`, `Standard`, or `Low` as the main card label. | |
| Two-part label | Show both class and cadence together. | |

**User's choice:** Operator-facing time label
**Notes:** The user prefers operational cadence language over backend jargon on the card.

### Save Behavior

| Option | Description | Selected |
|--------|-------------|----------|
| Inline auto-save | Polling override changes save directly from the polling section. | ✓ |
| Save with the main device form | Apply polling changes only through the general Save action. | |
| Hybrid | Presets auto-save, custom values confirm separately. | |

**User's choice:** Inline auto-save
**Notes:** The user wants polling override behavior to feel immediate and take effect on the next cycle without a full form save or refresh.

---

## Card Layout Density

| Option | Description | Selected |
|--------|-------------|----------|
| Header health + body metadata row | Keep health in the header and place freshness/polling in one compact body row. | ✓ |
| Single body status strip | Keep the header unchanged and group all new metadata in the body. | |
| Replace the current secondary text row | Reuse the detail/model row for the new status metadata. | |

**User's choice:** Header health + body metadata row
**Notes:** The user wants Phase 44 to extend the current card hierarchy rather than redesign it or sacrifice existing device identity text.

---

## the agent's Discretion

- Exact badge/label styling for health and freshness within the existing DeviceCard geometry.
- Relative-time formatting details and refresh cadence.
- Exact UX/mechanics for migrating from the current legacy settings-key polling control to a device-backed override path.

## Deferred Ideas

- Editable class-selector UX (`core` / `standard` / `low`) as the main operator control.
- Freshness overriding or muting the primary health signal.
- Larger card redesign or replacing the existing descriptive detail row with status metadata.
- Dashboard/table parity for the new metadata outside the canvas card surface.
