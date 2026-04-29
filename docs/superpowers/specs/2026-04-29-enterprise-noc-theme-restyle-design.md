# Theia Enterprise/NOC Theme Restyle Design

## Goal

Restyle the existing Theia frontend theme so both dark mode and light mode are readable, accessible, and visually coherent for an enterprise/NOC operations tool.

This is a restyle of the current UI, not a redesign of product behavior. The implementation must preserve the existing navigation model, topology workflows, toolbar actions, panel behavior, status semantics, area filtering, and topology link telemetry.

## Source Of Truth

The authoritative references are:

- The current React UI implementation in `frontend/src`.
- The existing UI contract in `DESIGN.md`.
- The requirements approved during this design session.

The browser mockup at `frontend/public/mockups/theia-noc-restyle.html` is a visual example only. It demonstrates the approved tone: sober enterprise/NOC styling, stronger light-mode contrast, controlled dark-mode surfaces, and more legible typography. It is not a pixel-perfect implementation target and must not override existing working UI patterns.

## Scope

In scope:

- Rework theme tokens for dark and light mode.
- Improve contrast, typography, spacing, focus states, and surface hierarchy.
- Keep the current UI structure while making it visually consistent with the enterprise/NOC direction.
- Update component styling where current classes produce poor readability, especially in light mode.
- Keep operational data visible and easy to scan on topology nodes and links.
- Update tests and audits so future changes do not reintroduce low contrast or unreadable microtext.

Out of scope:

- New navigation concepts.
- New product workflows.
- Backend or data model changes.
- Pixel-perfect translation of the mockup.
- Removing existing functional controls unless they are redundant text inside the node card body.

## Accessibility Contract

All text must be readable in both themes.

- WCAG AA contrast is the minimum for normal UI text.
- Operationally critical text should target WCAG AAA where practical: node hostname, IP, status labels, link telemetry, form labels, table values, and alert/error content.
- Light mode must never use pale text on pale surfaces. Text colors must come from explicit readable tokens.
- State must not rely on color alone. Existing dot, label, border, and surface treatments must remain available.
- Focus states must remain visible in both themes and use tokenized focus colors.
- Font size must not scale directly with viewport width.
- Essential text must not use sub-10px sizing.
- Long hostnames, IPs, interface names, and telemetry labels must use `min-w-0`, `truncate`, `break-words`, or equivalent layout guards so text does not overflow or overlap.

## Theme Direction

The theme should read as sober enterprise/NOC, not neon-first.

Dark mode:

- Keep the canvas-first operational feel.
- Reduce decorative glow and excessive transparency.
- Use solid dark surfaces with clear elevation.
- Keep semantic state colors visible but less decorative.

Light mode:

- Treat it as a first-class theme, not an inverted dark theme.
- Use dark text on light surfaces with strong secondary text.
- Make panels, nodes, chips, and badges solid enough for repeated operational use.
- Avoid semi-white text, weak borders, and low-contrast glass effects.

Both themes:

- Prefer tokenized color changes in `frontend/src/index.css`.
- Preserve `ThemeContext` behavior: `dark`, `light`, `system`, localStorage key `theia-theme`, and `data-theme` on `<html>`.
- Continue adapting area colors through `adaptAreaColor(area.color, resolvedTheme)` when colors are displayed on runtime UI.

## Typography Contract

Typography should remain technical and compact, but must be more legible.

- Keep the existing font family direction: Outfit for UI, JetBrains Mono for technical values.
- Use mono for IPs, ports, interface names, bandwidth, timestamps, hashes, config filenames, and numeric telemetry.
- Hostnames must be primary text on topology node cards.
- Avoid wide letter spacing for small text unless it remains clearly readable.
- Use line heights that prevent clipping on small screens.
- Do not introduce viewport-width font scaling.
- Use responsive wrapping/truncation rules rather than shrinking text until it becomes unreadable.

Recommended lower bounds:

- Main operational text: 13-15px.
- Node hostname: about 15px or higher, with clear weight and wrapping.
- Technical values and badges: 11-12px minimum when essential.
- Decorative labels may be smaller only when they are not required for operation.

## Component Requirements

### Navigation

`NavigationPill` must keep the current logic:

- THEIA brand and version.
- Area Hub button.
- Global view.
- Area filters with adapted area colors.
- Devices dashboard button.
- Theme toggle.

The restyle may change token usage, contrast, hover states, borders, shadows, and spacing, but must not replace this navigation model.

### Toolbar

`Toolbar` must keep the current action set and behavior:

- Edit mode.
- Search.
- Add device.
- Create link.
- Alerts.
- Settings.

The restyle may make icon buttons more readable and less glass-heavy, but must not remove actions or alter shortcuts.

### Topology Node Card

The visible node card body must contain only:

- Hostname.
- Status dot with status label.
- IP address.
- Fresh/stale telemetry indicator.

The hostname is the primary identity. Device-type labels such as "Core Router" or "Access" must not compete with the hostname inside the card body.

Existing operational affordances should remain where they are part of behavior rather than card body content:

- Selection and hover styling.
- Area color accent.
- React Flow handles in edit mode.
- Context menu behavior.
- Existing ghost/cross-area behavior.
- Existing self-link annotations if they are rendered as separate overlays.

If a device has no IP, the IP slot should use the existing no-IP/unmonitored semantics rather than inventing a new status.

`StatusDot` must preserve the current style and semantics for:

- UP.
- Warning.
- Critical.
- Down.

### Topology Links

Topology links must preserve direct on-link telemetry:

- TX/RX labels remain visible on the line when available.
- Autonegotiation/duplex/speed warning labels remain visible on the line when available.
- Link strokes should become 2px thicker than the current implementation baseline.
- Link labels must remain readable in both themes and should use solid tokenized surfaces.
- Warning/critical/down link states must remain semantically distinct.

### Side Panels And Dashboards

Panels, tables, forms, dashboard rows, filter controls, and modals should keep existing layout and workflow patterns.

The restyle should focus on:

- Better light-mode text contrast.
- Consistent surfaces and borders.
- More legible labels and values.
- Tokenized semantic colors.
- Reduced decorative transparency.

Do not radically change panel structure or introduce a new information architecture.

## Architecture

The implementation should be token-first.

Primary touchpoints:

- `frontend/src/index.css`: theme tokens, React Flow variables, utility classes.
- `frontend/src/contexts/ThemeContext.tsx`: keep current behavior; only adjust color adaptation if necessary.
- `frontend/src/components/DeviceCard.tsx`: simplify visible card body content while preserving behavior.
- `frontend/src/components/StatusDot.tsx` and `deviceVisualState.ts`: preserve status semantics; adjust token references only if needed.
- `frontend/src/components/NavigationPill.tsx`: retain logic and improve styling through existing classes/tokens.
- `frontend/src/components/Toolbar.tsx`: retain buttons and improve styling through existing classes/tokens.
- `frontend/src/components/SidePanel.tsx` and panel components: improve readability through tokenized surfaces and text classes.
- Link edge builder/rendering files: increase stroke width and verify label contrast.

Avoid broad rewrites. Prefer small, targeted changes that use the existing component boundaries.

## Data Flow

No backend or data contract changes are required.

Runtime device status, freshness, polling interval, area colors, link telemetry, TX/RX, duplex/autonegotiation, and alert status should continue to flow through the existing models and adapters.

The visual restyle must not change how status or freshness is computed. It changes presentation only.

## Error Handling And Edge Cases

- Missing IP keeps current no-IP/unmonitored handling.
- Stale, awaiting first poll, SNMP unreachable, and polling disabled states keep current semantic handling.
- Long hostnames must not break card layout.
- Dense area lists in navigation must remain horizontally scrollable or otherwise constrained as today.
- Light-mode glass or overlay surfaces must remain readable over the canvas.
- Low-connectivity/down states must remain visually prominent without relying solely on red.

## Testing Strategy

Automated tests should cover behavior preservation and visual contracts where practical.

Required checks:

- Existing unit tests for `NavigationPill`, `Toolbar`, `StatusDot`, `DeviceCard`, and dashboard rows continue to pass.
- `DeviceCard` tests are updated so physical card content matches the approved body: hostname, status, IP, freshness only.
- Tests verify device type labels and CPU/MEM/UP readouts do not remain in the physical node card body.
- Status dot tests confirm UP, Warning, Critical, and Down semantics still map to the correct tokenized styles.
- Link tests verify stroke width increased by 2px and telemetry labels remain present.
- CSS/audit tests check light-mode text token values are not pale-on-light for primary/secondary operational text.
- Build and type checks pass.

Manual verification:

- Open the dev frontend at `http://10.10.0.38:3000`.
- Check dark and light mode.
- Check topology node cards, link labels, navbar area filters, toolbar, side panels, dashboard tables, forms, alerts, and modals.
- Compare against the mockup only for tone and contrast, not exact layout.

## Acceptance Criteria

The work is accepted when:

- Dark and light mode both feel like first-class themes.
- Light mode no longer has unreadable pale text on light surfaces.
- The node card body shows only hostname, status dot/label, IP, and fresh/stale telemetry.
- TX/RX and autonegotiation labels remain directly visible on topology links.
- Link strokes are 2px thicker than the previous baseline.
- Navigation area filtering and toolbar actions behave the same as before.
- Status dot style and status semantics are preserved.
- Existing user workflows remain recognizable.
- The implementation uses the mockup only as visual guidance and follows the current UI as the real integration target.
