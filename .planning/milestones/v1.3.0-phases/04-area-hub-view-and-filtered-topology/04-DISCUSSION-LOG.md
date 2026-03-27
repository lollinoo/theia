# Phase 4: Area Hub View and Filtered Topology - Discussion Log

> **Audit trail only.** Do not use as input to planning, research, or execution agents.
> Decisions are captured in CONTEXT.md — this log preserves the alternatives considered.

**Date:** 2026-03-26
**Phase:** 04-area-hub-view-and-filtered-topology
**Areas discussed:** Navigation architecture, Area card content, Cross-area link behavior, Hub-to-canvas transition, Watermark styling, Area card bloom/hover effects, Empty states, Pill visual styling

---

## Navigation Architecture

### Hub View Placement

| Option | Description | Selected |
|--------|-------------|----------|
| Third view in App.tsx | Hub as new ActiveView alongside canvas/dashboard. Nav pill shared between Hub and Canvas | |
| Nav pill replaces NavBar | Pill becomes sole navigation element. Handles area selection, view switching, theme toggle | ✓ |
| Hub overlays the canvas | Hub is modal/overlay on top of always-mounted canvas | |

**User's choice:** Nav pill replaces NavBar
**Notes:** User chose the most ambitious option — single pill as the only navigation element in the app.

### Canvas.tsx Decomposition

| Option | Description | Selected |
|--------|-------------|----------|
| Yes, decompose first | Split Canvas.tsx (1294 lines) before adding area filtering | ✓ |
| No, add to Canvas.tsx as-is | Keep monolithic, add area filtering directly | |
| Minimal extraction only | Extract only area-related logic into separate hook | |

**User's choice:** Yes, decompose first
**Notes:** Multiple prior phases flagged this as prerequisite. File grew from ~750 to 1294 lines.

### Pill Design

| Option | Description | Selected |
|--------|-------------|----------|
| Two-tier pill | Top row: view tabs. Bottom row: area buttons | |
| Single pill, context-aware | One row that adapts content based on current view | ✓ |
| Match mock exactly | Single row with just area buttons, other controls elsewhere | |

**User's choice:** Single pill, context-aware
**Notes:** Pill shows area buttons on Hub/Topology, simplifies on Devices view.

### Global View Destination

| Option | Description | Selected |
|--------|-------------|----------|
| Area Hub page | Global = Hub view with aggregate stats and area cards | ✓ |
| Unfiltered topology canvas | Global = full canvas with all devices | |
| Hub is default landing | App opens to Hub, Global stays on Hub | |

**User's choice:** Area Hub page

### Area Selection Destination

| Option | Description | Selected |
|--------|-------------|----------|
| Filtered topology canvas | Clicking area switches to canvas filtered to that area | ✓ |
| Area detail page | Shows area-specific stats page, separate button for canvas | |
| Hub with area highlighted | Stays on Hub, highlights selected area card | |

**User's choice:** Filtered topology canvas

### THEIA Branding

| Option | Description | Selected |
|--------|-------------|----------|
| Inside the pill, left side | THEIA text at left edge of pill before Hub icon | ✓ |
| Drop it entirely | No branding, browser tab only | |
| Fixed corner, outside pill | Small text in corner independent of pill | |

**User's choice:** Inside the pill, left side

### Area Overflow

| Option | Description | Selected |
|--------|-------------|----------|
| Horizontal scroll inside pill | Max-width pill with scrollable area buttons, fade edge hints | ✓ |
| Dropdown overflow menu | First N inline, +X more dropdown | |

**User's choice:** Horizontal scroll inside pill

---

## Area Card Content

### Hub Aggregate Stats

| Option | Description | Selected |
|--------|-------------|----------|
| Uptime + Health + Device Count | Network Uptime, Aggregate Health %, Total Devices, Active Links | ✓ |
| Match mock, keep routes | Include Total Routes despite PROJECT.md exclusion | |
| Uptime + Health only | Just two stats, simpler | |

**User's choice:** Uptime + Health + Device Count (4 stats total including Active Links)
**Notes:** Routes explicitly excluded per PROJECT.md decision. Device count and active links replace it.

### Per-Area Card Stats

| Option | Description | Selected |
|--------|-------------|----------|
| Health + Devices + Active Links | Health status, device count, active link count | ✓ |
| Health + Devices + Up/Down split | Health, total devices, then X up / Y down split | |
| You decide | Claude picks best combination | |

**User's choice:** Health + Devices + Active Links

### Health Calculation

| Option | Description | Selected |
|--------|-------------|----------|
| Percentage of devices 'up' | (up devices / total devices) * 100, with Optimal/Degraded/Critical thresholds | ✓ |
| Weighted by device importance | Core routers count more (needs new field) | |
| You decide | Claude picks approach | |

**User's choice:** Percentage of devices 'up'

---

## Cross-Area Link Behavior

### Filtered View Links

| Option | Description | Selected |
|--------|-------------|----------|
| Show as stub with ghost node | Inter-area links shown, remote device as muted ghost node | ✓ |
| Hide cross-area links entirely | Only show fully-within-area devices and links | |
| Show link, hide remote device | Link fades toward canvas edge, no ghost node | |

**User's choice:** Show as stub with ghost node
**Notes:** Ghost nodes show hostname only, muted styling, no metrics.

### Ghost Node Click Behavior

| Option | Description | Selected |
|--------|-------------|----------|
| Yes, switch to that area | Clicking ghost navigates to that device's area | ✓ |
| Open device config panel | Opens config panel, no area switch | |
| You decide | Claude picks | |

**User's choice:** Yes, switch to that area

---

## Hub-to-Canvas Transition

### Transition Style

| Option | Description | Selected |
|--------|-------------|----------|
| Instant swap | Simple view switch, pill highlight as orientation cue | ✓ |
| Crossfade transition | 200-300ms fade between views | |
| Zoom effect | Camera zooms into area card | |
| You decide | Claude picks | |

**User's choice:** Instant swap

### Unassigned Devices

| Option | Description | Selected |
|--------|-------------|----------|
| Hidden when any area is active | Only visible in Global/full topology view | ✓ |
| Separate 'Unassigned' section on Hub | Hub shows extra section for unassigned devices | |
| Both | Hidden on canvas, shown on Hub | |

**User's choice:** Hidden when any area is active

---

## Watermark Styling

### Size and Position

| Option | Description | Selected |
|--------|-------------|----------|
| Bottom-left, ~1.5rem (text-2xl) | Small, subtle label text. Low opacity, pointer-events-none | ✓ |
| Bottom-left, large (4-6rem) | Large faded text matching mock | |
| Center of canvas | Centered, extremely low opacity | |

**User's choice:** Bottom-left, ~1.5rem
**Notes:** User clarified: "bottom left but with very small text. it doesn't need to be too big."

### Animation

| Option | Description | Selected |
|--------|-------------|----------|
| Simple fade (150ms) | Old text fades out, new text fades in | ✓ |
| No animation | Instant text change | |
| You decide | Claude picks | |

**User's choice:** Simple fade

---

## Area Card Bloom/Hover Effects

### Bloom Style

| Option | Description | Selected |
|--------|-------------|----------|
| Radial blur bloom matching mock | Large radial blur in accent color, ~0.10 default, ~0.20 hover. Light: subtle, no blur | ✓ |
| Box-shadow glow only | Simpler, consistent with canvas nodes | |
| You decide | Claude picks | |

**User's choice:** Radial blur bloom matching mock

### Border Accent

| Option | Description | Selected |
|--------|-------------|----------|
| Hover border accent | Default: surface border. Hover: area accent color, 200ms transition | ✓ |
| Always show accent border | Constant area-colored border | |
| No border accent, glow only | No-line rule, bloom only | |

**User's choice:** Hover border accent

---

## Empty States

### Hub with No Areas

| Option | Description | Selected |
|--------|-------------|----------|
| Prompt to create areas | Show global stats + CTA card linking to Settings > Areas | ✓ |
| Just show stats, no prompt | Empty area section, no guidance | |
| You decide | Claude designs empty state | |

**User's choice:** Prompt to create areas

### No Links in Area

| Option | Description | Selected |
|--------|-------------|----------|
| Just show devices, no special state | Normal device nodes, no empty state message | ✓ |
| Show devices + subtle hint | Devices + small "No links in this area" label | |

**User's choice:** Just show devices, no special state

---

## Pill Visual Styling

### Surface Treatment

| Option | Description | Selected |
|--------|-------------|----------|
| Glassmorphism dark, solid light | Follows established overlay pattern from Phase 2 | ✓ |
| Solid surface in both themes | No glassmorphism anywhere | |
| You decide | Claude styles within design language | |

**User's choice:** Glassmorphism dark, solid light

### Area Color Indicators

| Option | Description | Selected |
|--------|-------------|----------|
| Small color dot before area name | 6-8px dot in accent color, active area glows | ✓ |
| Text color changes to area color | Active button text turns accent color | |
| Underline accent | Small underline bar in accent color | |

**User's choice:** Small color dot before area name

---

## Claude's Discretion

- Canvas.tsx decomposition strategy
- Exact pill measurements and spacing
- Hub layout grid and responsive breakpoints
- Ghost node visual design
- Material Symbols icon choices for pill
- Health "Network Uptime" data source
- Pill scroll implementation details

## Deferred Ideas

- Animated link throughput (CANVAS-01)
- Canvas-integrated area zones (POLISH-03)
- Theme switch transitions (POLISH-01)
- Area detail page (intermediate Hub-to-canvas view)
