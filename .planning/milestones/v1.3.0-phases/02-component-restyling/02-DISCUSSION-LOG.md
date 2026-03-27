# Phase 2: Component Restyling - Discussion Log

> **Audit trail only.** Do not use as input to planning, research, or execution agents.
> Decisions are captured in CONTEXT.md — this log preserves the alternatives considered.

**Date:** 2026-03-25
**Phase:** 02-component-restyling
**Areas discussed:** Icon strategy, Overlay surfaces in light mode, Bloom/glow performance, DeviceCard visual depth

---

## Icon Strategy

| Option | Description | Selected |
|--------|-------------|----------|
| Material Symbols (self-hosted font) | Google's variable icon font — supports weight/fill/optical-size axes. ~100KB woff2 subset. | ✓ |
| Lucide React (SVG components) | Tree-shakeable SVG icon library (~300 icons). Import only what you use. | |
| Inline SVGs (hand-crafted) | Copy SVG paths for the ~15 icons needed. Zero runtime dependency. | |

**User's choice:** Material Symbols (self-hosted font)
**Notes:** None

### Icon Style

| Option | Description | Selected |
|--------|-------------|----------|
| Rounded (Recommended) | Softer corners match the panel radius (12px) and pill geometry. | ✓ |
| Outlined | Clean line icons with sharp corners. More technical/utilitarian feel. | |
| Sharp | Angular, geometric. Stronger contrast but may clash with rounded panels. | |

**User's choice:** Rounded
**Notes:** None

### Phase 1 Icon Cleanup

| Option | Description | Selected |
|--------|-------------|----------|
| Replace now for consistency | All icons system-wide use Material Symbols Rounded. | ✓ |
| Keep existing, replace in Phase 4 | NavBar toggle works fine with inline SVGs. Less churn now. | |

**User's choice:** Replace now for consistency
**Notes:** None

---

## Overlay Surfaces in Light Mode

| Option | Description | Selected |
|--------|-------------|----------|
| Tinted solid surfaces (per D-07) | rgba(255,255,255,0.85) background, no backdrop-filter, subtle border. | ✓ |
| Frosted glass in both themes | Keep backdrop-blur everywhere. White-tinted glass in light mode. | |
| You decide | Let Claude pick. | |

**User's choice:** Tinted solid surfaces (per D-07)
**Notes:** Consistent with Phase 1 decision D-07.

### Dark-Mode Glass Opacity

| Option | Description | Selected |
|--------|-------------|----------|
| Subtle glass (0.03-0.05) | Very transparent — ethereal, editorial feel. | |
| Medium glass (0.06-0.10) | Balanced translucency. Background shows through but text readable. | ✓ |
| You decide | Let Claude tune opacity. | |

**User's choice:** Medium glass (0.06-0.10)
**Notes:** None

---

## Bloom/Glow Performance

### Canvas Glow

| Option | Description | Selected |
|--------|-------------|----------|
| Box-shadow only (Recommended) | Pure CSS box-shadow. GPU-composited, no paint invalidation. | ✓ |
| Pseudo-element radial gradient | ::before/::after with radial-gradient. More diffuse bloom but triggers paint. | |
| You decide | Let Claude benchmark. | |

**User's choice:** Box-shadow only
**Notes:** None

### Off-Canvas Bloom

| Option | Description | Selected |
|--------|-------------|----------|
| Yes — full bloom off-canvas | Use radial-blur (80px+) behind status indicators and area cards. | |
| Box-shadow everywhere | Consistent approach. Simpler to maintain. | |
| You decide | Let Claude use richer bloom where performance allows. | ✓ |

**User's choice:** You decide
**Notes:** Claude has discretion for off-canvas elements.

### Glow Scaling

| Option | Description | Selected |
|--------|-------------|----------|
| Yes — severity-scaled glow | Critical states get larger spread and higher opacity. Draws eye to problems. | ✓ |
| Uniform glow per status | Same shadow size/opacity for all statuses, just different colors. | |
| You decide | Let Claude design appropriate scaling. | |

**User's choice:** Severity-scaled glow
**Notes:** None

---

## DeviceCard Visual Depth

### Card Accent

| Option | Description | Selected |
|--------|-------------|----------|
| Glow node replaces top border | Remove colored top border. Status via glow dot with box-shadow bloom. | ✓ |
| Keep top border + add glow node | Colored border stays as accent. Glow node added separately. | |
| You decide | Let Claude determine visual hierarchy. | |

**User's choice:** Glow node replaces top border
**Notes:** None

### Card Section Separation

| Option | Description | Selected |
|--------|-------------|----------|
| Surface color shift (Recommended) | Header on surface, body on bg, metrics on surface-high. No borders. | ✓ |
| Spacing only | Remove border, rely on extra vertical padding. | |
| You decide | Let Claude apply no-line rule. | |

**User's choice:** Surface color shift
**Notes:** None

### Decorative Port Dots

| Option | Description | Selected |
|--------|-------------|----------|
| Remove them | No functional purpose. ReactFlow handles on hover already provide connection points. | ✓ |
| Keep but restyle | Keep dots but restyle to match Neon Topography. | |
| You decide | Let Claude decide. | |

**User's choice:** Remove them
**Notes:** None

### Vendor Badge

| Option | Description | Selected |
|--------|-------------|----------|
| Keep colored pill | Tertiary-colored pill badge stays. Quick vendor identifier. | |
| Muted monospace tag | JetBrains Mono, smaller text, subtle surface-high background. Terminal feel. | ✓ |
| You decide | Let Claude pick. | |

**User's choice:** Muted monospace tag
**Notes:** None

### Hover Accent

| Option | Description | Selected |
|--------|-------------|----------|
| Primary green glow | Hover shows green ring/outline glow. Consistent primary accent. | |
| Status-matched glow | Hover glow matches device's current status color. | |
| You decide | Let Claude determine most effective hover feedback. | ✓ |

**User's choice:** You decide
**Notes:** Claude has discretion.

---

## Claude's Discretion

- DeviceCard hover accent color strategy
- Off-canvas bloom approach (radial-blur vs box-shadow)
- Exact glow shadow spread/opacity values per status level
- Component restyling order and wave grouping
- Exact Material Symbols icon names
- metricColor() function handling

## Deferred Ideas

- Canvas.tsx decomposition — before Phase 4, not Phase 2
- NavigationPill — Phase 4 scope
- Area-specific accent coloring on DeviceCards — Phase 4 (requires Phase 3 area assignment)
