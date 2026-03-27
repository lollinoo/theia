# Phase 2: Component Restyling - Research

**Researched:** 2026-03-25
**Domain:** Frontend component restyling -- CSS, Tailwind v4, Material Symbols icon font, glassmorphism, glow effects, theme correctness
**Confidence:** HIGH

## Summary

Phase 2 transforms all 26+ existing frontend components from their current styling to the Neon Topography design language. The token system from Phase 1 is already in place (`index.css` with `--nt-*` primitives, `@theme inline` semantic mappings, dark/light theme blocks). The work is purely visual -- no new features, routes, or API changes.

The main technical challenges are: (1) integrating Material Symbols Rounded as a self-hosted variable font with subsetting to keep payload under 100KB, (2) implementing the glassmorphism/solid overlay pattern that varies between dark and light themes, (3) enforcing the no-line rule by replacing ~30+ `border-*` layout separators across components with surface tier shifts, and (4) building the severity-scaled glow system for DeviceCard status indicators using only `box-shadow` (no `backdrop-filter` on canvas nodes) for 60fps at 100+ nodes.

**Primary recommendation:** Use `@fontsource-variable/material-symbols-rounded` (consistent with existing Fontsource usage for Outfit and JetBrains Mono), create a shared `<MaterialIcon>` React component, and work in 5 waves from highest visual impact (DeviceCard, ContextMenu) to lowest (DeviceIcon evaluation).

<user_constraints>
## User Constraints (from CONTEXT.md)

### Locked Decisions
- **D-01:** Use Material Symbols (self-hosted variable font, woff2 subset) for all icons across the application
- **D-02:** Icon style is Material Symbols Rounded -- softer corners match panel radius (12px) and pill geometry
- **D-03:** Replace all Phase 1 inline Heroicons SVGs (sun/moon theme toggle) with Material Symbols equivalents for system-wide consistency
- **D-04:** Self-host the font -- no CDN dependency. Subset to only the icons needed (~15-20 icons, ~100KB woff2)
- **D-05:** Glassmorphism (backdrop-blur + translucent bg) is dark-mode only for overlay surfaces (context menu, search overlay), consistent with Phase 1 decision D-07
- **D-06:** Light-mode overlays use tinted solid surfaces: `rgba(255,255,255,0.85)` background, no `backdrop-filter`, subtle border `rgba(0,0,0,0.06)`
- **D-07:** Dark-mode glassmorphism for context menu and search overlay uses medium opacity range (0.06-0.10) -- more substance than the area background spec (0.02) for readability over charcoal
- **D-08:** Canvas node glow uses CSS box-shadow only -- no `backdrop-filter`, no pseudo-element radial gradients. Must maintain 60fps at 100+ nodes
- **D-09:** Glow intensity scales with device status severity: critical states (down, warning) get larger spread and higher opacity shadows; healthy 'up' gets subtle glow
- **D-10:** Off-canvas elements (panel status indicators, future area cards) -- Claude's discretion on whether to use richer bloom (radial-blur) where element count is low
- **D-11:** Remove the 3px colored top border accent. Replace with a Glow Node (rounded-full element with status-colored box-shadow bloom) in card header for status indication
- **D-12:** Internal section separation uses surface color tiers (no-line rule): header on `surface`, body on `bg`, metrics area on `surface-high`. No border separators
- **D-13:** Remove the 6 decorative bottom port dots entirely. ReactFlow handles on hover provide connection points already
- **D-14:** Vendor badge switches from colored tertiary pill to muted JetBrains Mono monospace tag with subtle `surface-high` background -- less attention-grabbing, more terminal feel

### Claude's Discretion
- Exact glow shadow spread/opacity values per status level (within the box-shadow-only constraint)
- DeviceCard hover accent color strategy (primary green vs status-matched)
- Whether off-canvas bloom uses radial-blur or stays box-shadow-only
- Component restyling order and wave grouping for parallel execution
- Exact Material Symbols icon names for each context menu action
- Transition timing and easing for theme-switch animations on restyled components
- How to handle the `metricColor()` function -- whether to keep threshold-based coloring or simplify
- **D-15:** DeviceCard hover accent -- Claude's discretion on whether to use primary green glow or status-matched glow

### Deferred Ideas (OUT OF SCOPE)
- Canvas.tsx decomposition (750 lines) -- should happen before Phase 4 adds area filtering, not in Phase 2
- NavigationPill component -- Phase 4 scope, theme toggle gets absorbed there
- Area-specific accent coloring on DeviceCards -- Phase 4 scope (requires area assignment from Phase 3)
</user_constraints>

<phase_requirements>
## Phase Requirements

| ID | Description | Research Support |
|----|-------------|------------------|
| COMP-01 | DeviceCard restyled with Neon Topography aesthetics -- glow status nodes, Outfit labels, JetBrains Mono values, no-line internal sectioning | Glow system patterns, StatusDot upgrade, vendor tag restyle, metrics area tier shift, decorative port removal |
| COMP-02 | Context menu restyled with Material Symbols icons, separator support, glassmorphism surface, danger styling | MaterialIcon component, ContextMenuItem interface extension, dark/light overlay surface patterns from UI-SPEC |
| COMP-03 | NavBar restyled to Neon Topography design with updated branding and navigation structure | Material Symbols light_mode/dark_mode icons replace inline SVGs, no-line rule on border-b |
| COMP-04 | SidePanel, Toolbar, ZoomControls restyled with surface hierarchy and ghost border fallbacks | Border removal patterns, Material Symbols icon replacement for 6 Toolbar + 3 ZoomControls + 1 SidePanel SVGs |
| COMP-05 | SettingsPanel and sub-panels restyled | No-line rule on 3 `border-t border-outline` section separators, form input standardization |
| COMP-06 | AlertsPanel and SearchOverlay restyled with glassmorphism and glow effects | Glassmorphism dark/solid light pattern, glow on firing alert dots, SearchOverlay overlay surface treatment |
| COMP-07 | Dashboard, DeviceTable, DeviceRow restyled | Typography token application, status glow dots in rows, surface tier for alternating rows |
| COMP-08 | AddDevicePanel, DeviceConfigPanel, LinkCreatePanel form restyling | Standard form input contract (bg-elevated, border-outline-subtle, focus:ring-primary) |
| COMP-09 | LinkDetailsPanel and InterfaceStatsPanel restyled | JetBrains Mono for metric values, border-t removal, surface tier shifts |
| COMP-10 | LinkEdge connections restyled | Label pill backgrounds to use surface tokens, theme-responsive colors already mostly in place |
| COMP-11 | Bloom/radial-blur effects applied behind status-critical elements | Box-shadow-only glow on canvas (D-08), discretion on off-canvas bloom (D-10) |
| COMP-12 | No-line rule enforced -- layout regions use surface color tiers, not 1px borders | ~30+ border pattern instances across 15+ files need replacement |
| THEME-05 | All 25+ components readable and visually correct in both themes | Theme transition CSS, visual audit of all components in dark and light |
</phase_requirements>

## Standard Stack

### Core
| Library | Version | Purpose | Why Standard |
|---------|---------|---------|--------------|
| `@fontsource-variable/material-symbols-rounded` | 5.2.38 | Self-hosted Material Symbols Rounded variable font (woff2) | Consistent with existing `@fontsource-variable/outfit` and `@fontsource-variable/jetbrains-mono` already in project. Single import pattern for all fonts. |
| `tailwindcss` | 4.2.2 | Utility-first CSS with `@theme inline` token system | Already installed and configured in Phase 1 |
| `@xyflow/react` | 12.10.1 | ReactFlow v12 canvas -- custom nodes and edges | Already installed, DeviceCard and LinkEdge are custom node/edge types |

### Supporting
| Library | Version | Purpose | When to Use |
|---------|---------|---------|-------------|
| `vitest` | 4.1.0 | Test runner for component tests | Verify restyled components still render correctly |
| `@testing-library/react` | 16.3.2 | Component rendering tests | Test new MaterialIcon component, updated ContextMenu API |

### Alternatives Considered
| Instead of | Could Use | Tradeoff |
|------------|-----------|----------|
| `@fontsource-variable/material-symbols-rounded` | `material-symbols` (npm, by @marella) | `material-symbols` is community-maintained, 0.42.3; fontsource is better maintained and consistent with project pattern. BUT the full fontsource woff2 is ~4-9MB. Subsetting is required either way. |
| Full variable font | Google Fonts CDN with `&icon_names=` subset | Violates D-04 (no CDN). Self-hosting subset is mandatory. |
| `pyftsubset` / `glyphhanger` build-time subsetting | Ship full font + rely on browser caching | Violates performance constraint (<100KB). Full font is 4-9MB. |

**Installation:**
```bash
cd frontend && npm install @fontsource-variable/material-symbols-rounded
```

**Version verification:** Versions confirmed via `npm view` on 2026-03-25. All packages already installed except `@fontsource-variable/material-symbols-rounded`.

## Architecture Patterns

### Material Symbols Integration Pattern

The project already uses Fontsource for Outfit and JetBrains Mono. Material Symbols follows the same pattern.

**Font Loading (index.css):**
```css
@import "@fontsource-variable/material-symbols-rounded";
```

**Shared Component (MaterialIcon.tsx):**
```typescript
interface MaterialIconProps {
  name: string;
  className?: string;
  size?: number;
}

export function MaterialIcon({ name, className = '', size = 18 }: MaterialIconProps) {
  return (
    <span
      className={`material-symbols-rounded ${className}`}
      aria-hidden="true"
      style={size !== 18 ? { fontSize: size } : undefined}
    >
      {name}
    </span>
  );
}
```

**Base CSS class (index.css):**
```css
@layer base {
  .material-symbols-rounded {
    font-family: 'Material Symbols Rounded Variable', 'Material Symbols Rounded';
    font-weight: normal;
    font-style: normal;
    font-size: 18px;
    line-height: 1;
    letter-spacing: normal;
    text-transform: none;
    display: inline-block;
    white-space: nowrap;
    word-wrap: normal;
    direction: ltr;
    -webkit-font-smoothing: antialiased;
    font-variation-settings: 'FILL' 0, 'wght' 400, 'GRAD' 0, 'opsz' 18;
  }
}
```

### Font Subsetting Strategy

**Problem:** The full `@fontsource-variable/material-symbols-rounded` woff2 is ~4-9MB (3800+ icons). D-04 requires ~100KB woff2 with only ~20 icons.

**Recommended approach -- build-time subsetting:**
1. Install the full fontsource package for development convenience
2. Create a build script using `pyftsubset` (from `fonttools`) or `subset-font` (npm) to extract only the ~20 required icon ligatures
3. Output the subsetted woff2 to `frontend/public/fonts/material-symbols-rounded-subset.woff2`
4. Reference the subset file in a manual `@font-face` declaration instead of the fontsource import

**Alternative simpler approach (recommended for this project):**
1. Download the subset directly from Google Fonts API using `icon_names` parameter:
   `https://fonts.googleapis.com/css2?family=Material+Symbols+Rounded:opsz,wght,FILL,GRAD@18,400,0,0&icon_names=add,close,content_copy,dark_mode,delete,edit,fit_screen,light_mode,link,monitoring,network_ping,notifications,power_settings_new,search,settings,terminal,zoom_in,zoom_out&display=block`
2. Download the resulting woff2 file from the CSS response URL
3. Place in `frontend/public/fonts/` and use a manual `@font-face`
4. This gives a ~2-5KB woff2 file for 18 icons (far under 100KB budget)

**The `@fontsource-variable` package is still installed** as a dev dependency for convenience (provides the full font for development/testing), but the production build references the subset file.

### Surface Tier Depth Pattern (No-Line Rule)

Replace border separators with background color shifts:

```
Container (bg-surface)
  |-- Header section (bg-surface)  -- same tier as container, padding provides separation
  |-- Body section (bg-bg)         -- tier DOWN for depth inversion
  |-- Metrics area (bg-surface-high) -- tier UP for emphasis
```

**Before (border separator):**
```tsx
<div className="border-t border-outline pt-3">
  {/* metrics grid */}
</div>
```

**After (surface tier shift):**
```tsx
<div className="mt-3 rounded-lg bg-surface-high px-3 py-2">
  {/* metrics grid */}
</div>
```

### Overlay Surface Pattern (Dark vs Light)

The glassmorphism tokens are already defined in `index.css` (Phase 1 output). Components use them directly:

**Dark mode (glassmorphism):**
```tsx
<div className="bg-glass-bg border border-glass-border shadow-pill backdrop-blur-[16px]">
```

**Light mode (solid tinted):**
The same classes work because `--nt-glass-bg` resolves to `rgba(255,255,255,0.85)` and `--nt-glass-backdrop` resolves to `none` in light theme. However, `backdrop-blur-[16px]` is always applied in Tailwind. The light theme needs conditional blur removal.

**Solution:** Use a CSS approach rather than conditional React rendering:
```css
@layer base {
  [data-theme="light"] .glass-surface {
    backdrop-filter: none;
  }
}
```
Or use the existing `dark:` variant:
```tsx
<div className="bg-glass-bg border border-glass-border shadow-pill dark:backdrop-blur-[16px]">
```
The project uses `@custom-variant dark` which maps to `[data-theme=dark]`, so `dark:backdrop-blur-[16px]` works correctly.

### Glow Node Pattern

The StatusDot component (currently 10px `h-2.5 w-2.5`) is upgraded to a Glow Node with severity-scaled box-shadow:

```tsx
const glowConfig: Record<StatusDotStatus, string> = {
  up: 'bg-status-up shadow-[0_0_8px_rgba(0,230,118,var(--nt-glow-shadow-opacity))]',
  down: 'bg-status-down shadow-[0_0_16px_rgba(255,23,68,var(--nt-glow-shadow-opacity))] animate-pulse',
  degraded: 'bg-yellow-500 shadow-[0_0_14px_rgba(255,193,7,var(--nt-glow-shadow-opacity))] animate-pulse',
  probing: 'bg-status-probing shadow-[0_0_12px_rgba(255,234,0,var(--nt-glow-shadow-opacity))] animate-pulse',
  unknown: 'bg-status-unknown shadow-[0_0_6px_rgba(158,158,158,var(--nt-glow-shadow-opacity))]',
};
```

The `--nt-glow-shadow-opacity` variable is already theme-aware (0.5 dark, 0.25 light) from Phase 1.

**Important Tailwind v4 note:** Arbitrary shadow values with CSS variables inside `rgba()` may need escaping. Tailwind v4 uses `_` for spaces in arbitrary values. Test this pattern early.

### Theme Transition Pattern

Add to component root elements for smooth theme switching:
```tsx
className="transition-colors duration-200"
```

For box-shadow transitions on glow elements:
```tsx
className="transition-[box-shadow] duration-200"
```

**Caution:** `backdrop-filter` should NOT transition (per UI-SPEC: 0ms instant). The `transition-colors` utility does not affect `backdrop-filter`, so this is safe by default.

### Anti-Patterns to Avoid
- **Do not use `backdrop-filter` on canvas nodes:** Causes severe performance degradation at 100+ nodes. Canvas glow is `box-shadow` only (D-08).
- **Do not add pseudo-element radial gradients on DeviceCard:** Same performance concern. Box-shadow is GPU-composited.
- **Do not use conditional rendering for dark/light theme differences in overlays:** Use CSS variables and the `dark:` variant instead. React re-renders on theme change defeat the <16ms target.
- **Do not add new props to DeviceCard for glow:** The glow is derived from existing `status` and `alertStatus` props. Adding new props breaks the memo comparator.
- **Do not use `@apply` extensively:** Tailwind v4 discourages `@apply` for complex utilities. Use utility classes directly in JSX.

## Don't Hand-Roll

| Problem | Don't Build | Use Instead | Why |
|---------|-------------|-------------|-----|
| Icon font subsetting | Custom build pipeline with fonttools | Google Fonts API `icon_names` parameter to download pre-subset woff2 | Simpler, guaranteed correct ligature mapping, 2-5KB for 18 icons |
| Theme-aware glassmorphism | Conditional React rendering with `useTheme()` | CSS variables (`--nt-glass-bg`, `--nt-glass-backdrop`) + `dark:` variant | Zero JS overhead, <16ms theme switch, no React re-render |
| Icon component | Inline `<span>` with font classes everywhere | Shared `<MaterialIcon>` component | Consistent `aria-hidden`, `font-variation-settings`, sizing |
| Glow intensity per theme | Hardcoded shadow values with ternary | `var(--nt-glow-shadow-opacity)` CSS variable (already in tokens) | Automatically adjusts: 0.5 dark, 0.25 light |

**Key insight:** Phase 1 already built the token infrastructure that makes most Phase 2 changes CSS-only. The design system's CSS variable approach means theme correctness (THEME-05) comes largely for free if components use tokens correctly.

## Common Pitfalls

### Pitfall 1: Tailwind v4 Arbitrary Shadow Syntax
**What goes wrong:** Tailwind v4 changed how arbitrary values are parsed. Complex `box-shadow` values with `rgba()` and CSS variables may fail silently.
**Why it happens:** Tailwind v4 uses `_` for spaces in arbitrary values and has stricter parsing than v3.
**How to avoid:** Test glow shadow classes early in the first task. If arbitrary values fail, define custom shadow utilities in `@theme inline` block or use inline `style` attributes for complex shadows.
**Warning signs:** Missing glow effects, shadow values not appearing in computed styles.

### Pitfall 2: Backdrop-Filter Performance on Canvas
**What goes wrong:** Applying `backdrop-filter: blur()` to DeviceCard nodes causes frame drops below 60fps at 100+ nodes.
**Why it happens:** Each `backdrop-filter` creates a separate compositing layer and requires the browser to read pixels behind the element.
**How to avoid:** Strictly enforce D-08: `box-shadow` only for canvas nodes. `backdrop-filter` is permitted only on overlay surfaces (context menu, search overlay, navbar, toolbar) which have low element counts.
**Warning signs:** Scroll/pan jank on canvas, GPU usage spike in DevTools.

### Pitfall 3: DeviceCard Memo Breakage
**What goes wrong:** Adding new props or changing the class derivation logic causes the custom `memo()` comparator to miss changes or fire unnecessary re-renders.
**Why it happens:** `DeviceCard` uses a manual comparator checking specific prop fields. If new glow classes depend on data not in the comparator, the card won't update. If glow classes create new object references, the card re-renders every frame.
**How to avoid:** Derive all glow/ring classes from existing props (`data.device.status`, `data.alertStatus`, `data.highlighted`, `selected`). Do NOT add new props. Do NOT create new objects in the render path.
**Warning signs:** Status glow not updating when device status changes, or canvas FPS dropping due to excessive DeviceCard re-renders.

### Pitfall 4: Light Theme Glassmorphism Mismatch
**What goes wrong:** Using `backdrop-blur-[16px]` unconditionally means light theme overlays get blur despite D-06 saying no `backdrop-filter` in light mode.
**Why it happens:** Tailwind utility classes are always applied. The CSS variable `--nt-glass-backdrop` is set to `none` in light theme, but Tailwind's `backdrop-blur-[16px]` uses its own value, not the CSS variable.
**How to avoid:** Use `dark:backdrop-blur-[16px]` (which maps to `[data-theme=dark]` via `@custom-variant`) instead of unconditional `backdrop-blur-[16px]`. Or use a custom utility class that reads the CSS variable.
**Warning signs:** Frosted-glass blur visible on context menu in light theme.

### Pitfall 5: No-Line Rule Incomplete Enforcement
**What goes wrong:** Some border-based separators are missed because they're in sub-components or dashboard child components, not just the 26 main component files.
**Why it happens:** The project has 9 additional components in `frontend/src/components/dashboard/` that also use `border-b border-outline` patterns.
**How to avoid:** Run a full grep for `border-[tblr]` and `border border-outline` patterns across ALL component files including dashboard sub-components. Distinguish layout separators (must remove) from ghost border fallbacks (may keep per DESIGN.md).
**Warning signs:** Visible 1px lines between sections in any panel or card, especially in Dashboard view.

### Pitfall 6: Material Symbols Font Not Loading
**What goes wrong:** Icons render as text ligatures ("settings", "search") instead of icon glyphs.
**Why it happens:** The font file isn't loaded, the `@font-face` name doesn't match the CSS class, or Vite isn't processing the import correctly.
**How to avoid:** Verify font loading in browser DevTools Network tab. Ensure `font-family` in `.material-symbols-rounded` CSS class matches the `@font-face` declaration exactly. Test with both dev server and production build.
**Warning signs:** Plain text where icons should be, FOIT (flash of invisible text).

### Pitfall 7: Context Menu Border Radius Mismatch Dark/Light
**What goes wrong:** The HTML mocks specify different border radii: `rounded-[6px]` for dark, `rounded-[10px]` for light.
**Why it happens:** This is an intentional design decision from the mocks, but it's easy to miss because most components use a single radius.
**How to avoid:** Use `dark:rounded-[6px] rounded-[10px]` or conditionally apply via the theme context. Since `@custom-variant dark` targets `[data-theme=dark]`, the `dark:` prefix works.
**Warning signs:** Context menu corners look wrong in one theme.

## Code Examples

### Existing Border Patterns to Replace (No-Line Rule)

Found ~30+ instances across components. Key patterns:

**SettingsPanel.tsx (3 section separators):**
```tsx
// Lines 262, 270, 271: border-t border-outline pt-4
<div className="border-t border-outline pt-4">
  <SNMPProfileManager />
</div>
```

**SidePanel.tsx (2 borders):**
```tsx
// border-l border-outline on container, border-b border-outline on header
className="... bg-surface border-l border-outline ..."
```

**Toolbar.tsx (5 button separators):**
```tsx
// border-b border-outline between each button
${i !== buttons.length - 1 ? 'border-b border-outline' : ''}
```

**DeviceCard.tsx (1 metrics separator):**
```tsx
// Line 170: border-t border-outline
<div className="mt-4 border-t border-outline pt-3">
```

**Dashboard.tsx (1 filter bar separator):**
```tsx
// Line 78: border-b border-outline
<div className="flex items-center gap-3 px-4 py-3 border-b border-outline bg-surface/50">
```

### Inline SVG Count Per Component (Icons to Replace)

| Component | SVG Count | Replacement Icons |
|-----------|-----------|-------------------|
| Toolbar.tsx | 6 | edit, search, add, link, notifications, settings |
| NavBar.tsx | 2 | light_mode, dark_mode |
| SidePanel.tsx | 1 | close |
| ShortcutHelp.tsx | 1 | close |
| SearchOverlay.tsx | 1 | search |
| AlertsPanel.tsx | 1 | check_circle (empty state) |
| ZoomControls.tsx | 0 (text +/-/Fit) | zoom_in, zoom_out, fit_screen |
| **Total** | **12 SVGs + 3 text** | **15 replacements** |

### ContextMenu API Extension

```typescript
// Before
export interface ContextMenuItem {
  label: string;
  onClick: () => void;
  variant?: 'danger' | 'default';
  disabled?: boolean;
}

// After
export interface ContextMenuItem {
  label: string;
  onClick: () => void;
  variant?: 'danger' | 'default';
  disabled?: boolean;
  icon?: string;      // Material Symbols icon name
  separator?: boolean; // Render separator line before this item
}
```

### DeviceCard highlightClass Replacement

```typescript
// Before (current)
const highlightClass =
  data.alertStatus === 'down'
    ? 'ring-2 ring-red-500 shadow-[0_0_28px_rgba(255,23,68,0.45)] animate-pulse'
    : data.alertStatus === 'degraded'
      ? 'ring-2 ring-yellow-500 shadow-[0_0_28px_rgba(255,193,7,0.35)]'
      : data.highlighted
        ? 'ring-2 ring-primary shadow-[0_0_28px_rgba(0,212,255,0.35)]'
        : selected
          ? 'ring-2 ring-primary shadow-[0_0_22px_rgba(0,212,255,0.18)]'
          : 'ring-1 ring-outline';

// After (per UI-SPEC glow system)
const cardRingClass =
  data.alertStatus === 'down'
    ? 'ring-2 ring-status-down shadow-[0_0_28px_rgba(255,23,68,0.45)] animate-pulse'
    : data.alertStatus === 'degraded'
      ? 'ring-2 ring-yellow-500 shadow-[0_0_28px_rgba(255,193,7,0.35)]'
      : data.highlighted
        ? 'ring-2 ring-primary shadow-[0_0_28px_rgba(0,230,118,0.35)]'
        : selected
          ? 'ring-2 ring-primary shadow-[0_0_22px_rgba(0,230,118,0.18)]'
          : 'ring-1 ring-outline';

// + Hover accent (primary green, per UI-SPEC discretion):
// Applied via group-hover on card wrapper
// 'hover:ring-2 hover:ring-primary/60 hover:shadow-[0_0_20px_rgba(0,230,118,0.15)]'
```

## State of the Art

| Old Approach | Current Approach | When Changed | Impact |
|--------------|------------------|--------------|--------|
| Inline Heroicons SVGs | Material Symbols variable font | Phase 2 (now) | Consistent icon system, smaller bundle (font shared across all icons vs individual SVG paths) |
| `border-t border-outline` separators | Surface tier shifts (bg-surface/bg-bg/bg-surface-high) | Phase 2 (now) | No-line rule enforcement, modern design language |
| Static colored borders for status | Box-shadow glow with severity scaling | Phase 2 (now) | Immediate visual hierarchy for network operators |
| Uniform overlay surfaces | Theme-split overlays (glassmorphism dark, solid light) | Phase 2 (now) | Performance-correct light theme, visual polish in dark |

**Deprecated/outdated:**
- Heroicons inline SVGs: Being replaced by Material Symbols (D-03)
- `border-t-[3px] border-tertiary` on DeviceCard: Replaced by Glow Node (D-11)
- Decorative bottom port dots on DeviceCard: Removed entirely (D-13)
- `bg-tertiary/15 text-tertiary` vendor badge: Replaced by monospace tag (D-14)

## Open Questions

1. **Tailwind v4 arbitrary shadow with CSS variables**
   - What we know: The glow system needs `shadow-[0_0_Xpx_rgba(R,G,B,var(--nt-glow-shadow-opacity))]` syntax. Tailwind v4 may not parse CSS variables inside arbitrary values correctly.
   - What's unclear: Whether Tailwind v4's JIT compiler handles `var()` inside arbitrary `box-shadow` values.
   - Recommendation: Test in Wave 1, Task 1 (StatusDot/DeviceCard). If it fails, fall back to inline `style={{ boxShadow: '...' }}` or define named shadow utilities in `@theme inline`.

2. **Material Symbols font subsetting for production**
   - What we know: The full font is 4-9MB. Google Fonts API can serve pre-subset versions via `icon_names` parameter. The subset for ~18 icons would be 2-5KB.
   - What's unclear: Whether the project needs a build-time subsetting pipeline or whether a one-time manual download suffices (icons list is static).
   - Recommendation: One-time manual download from Google Fonts API, place woff2 in `frontend/public/fonts/`. Add a comment in the CSS listing which icons are included. No build pipeline needed since the icon set is static and small.

3. **Fontsource import vs manual @font-face for Material Symbols**
   - What we know: `@import "@fontsource-variable/material-symbols-rounded"` loads the full ~4-9MB font. For development this is fine. For production the subset file is needed.
   - What's unclear: Whether Vite tree-shakes unused font glyphs from Fontsource (it does not -- fonts are opaque binary files).
   - Recommendation: Use the Fontsource import for development (full icon set available for testing), but replace it with a manual `@font-face` pointing to the subset woff2 for production. OR simply use the manual subset approach from the start and skip the Fontsource package entirely, since the project only needs ~18 icons. This is the cleaner approach.

## Validation Architecture

### Test Framework
| Property | Value |
|----------|-------|
| Framework | Vitest 4.1.0 + @testing-library/react 16.3.2 |
| Config file | `frontend/vitest.config.ts` |
| Quick run command | `cd frontend && npx vitest run` |
| Full suite command | `cd frontend && npx vitest run --reporter=verbose` |

### Phase Requirements to Test Map
| Req ID | Behavior | Test Type | Automated Command | File Exists? |
|--------|----------|-----------|-------------------|-------------|
| COMP-01 | DeviceCard renders glow node, no top border, no bottom ports, monospace vendor tag | unit | `cd frontend && npx vitest run src/components/DeviceCard.test.tsx -x` | Exists -- needs update |
| COMP-02 | ContextMenu renders icons, separators, danger styling, glassmorphism classes | unit | `cd frontend && npx vitest run src/components/ContextMenu.test.tsx -x` | Does not exist -- Wave 0 |
| COMP-03 | NavBar renders Material Symbols icons instead of SVGs | unit | `cd frontend && npx vitest run src/components/NavBar.test.tsx -x` | Does not exist -- optional |
| COMP-04 | Toolbar renders without border-b separators | unit | `cd frontend && npx vitest run src/components/Toolbar.test.tsx -x` | Does not exist -- optional |
| COMP-07 | Dashboard renders with token classes | unit | `cd frontend && npx vitest run src/components/Dashboard.test.tsx -x` | Exists -- needs update |
| COMP-12 | No border-* layout separators in restyled components | smoke | `grep -r "border-[tblr]" --include="*.tsx" frontend/src/components/` (manual verify) | N/A (grep audit) |
| THEME-05 | All components render without errors in both themes | integration | `cd frontend && npx vitest run` (full suite) | Covered by existing tests |

### Sampling Rate
- **Per task commit:** `cd frontend && npx vitest run` (2.5s, 55 tests)
- **Per wave merge:** `cd frontend && npx vitest run --reporter=verbose`
- **Phase gate:** Full suite green + manual visual audit in both themes

### Wave 0 Gaps
- [ ] `frontend/src/components/ContextMenu.test.tsx` -- covers COMP-02 (new component API with icon/separator props)
- [ ] Update `DeviceCard.test.tsx` -- verify glow node presence, absence of top border, absence of bottom ports
- [ ] MaterialIcon component test -- verify correct class application, aria-hidden

## Environment Availability

Step 2.6: SKIPPED (no external dependencies identified). Phase 2 is purely frontend CSS/component changes. All tooling (Node.js, npm, Vite, Vitest) is already available in the project's Docker dev environment and was validated working in Phase 1. The Material Symbols font file is downloaded and committed to the repo -- no runtime external dependency.

## Project Constraints (from CLAUDE.md)

- **Styling:** All styling via Tailwind utility classes -- no CSS modules or styled-components
- **Imports:** No `@/` aliases; all imports use relative paths
- **Components:** PascalCase.tsx for components, camelCase.ts for hooks/utilities
- **Tests:** Co-located test files with `.test.ts` / `.test.tsx` suffix
- **Memo:** DeviceCard uses `memo()` with custom comparator -- do not break
- **No ESLint/Prettier:** Vite + TypeScript compiler enforce syntax
- **Single quotes** for string literals, trailing commas in multi-line objects/arrays
- **Named exports** for components/hooks, default export only for primary React components
- **Type-only imports** use `import type` syntax
- **JSX comments** use `{/* SECTION */}` label pattern

## Sources

### Primary (HIGH confidence)
- **Codebase analysis:** Direct reading of all 26+ component files, `index.css` token system, `ThemeProvider.tsx`, test files, `package.json`
- **UI-SPEC:** `.planning/phases/02-component-restyling/02-UI-SPEC.md` -- comprehensive design contract
- **CONTEXT.md:** `.planning/phases/02-component-restyling/02-CONTEXT.md` -- locked decisions D-01 through D-14
- **DESIGN.md:** `.planning/DESIGN.md` -- Neon Topography design system specification
- **HTML mocks:** `.planning/examples_mocks/node_context_menu/{dark,light}/code.html` -- visual targets

### Secondary (MEDIUM confidence)
- [Fontsource Material Symbols documentation](https://fontsource.org/docs/getting-started/material-symbols) -- import patterns, variable font axes
- [Material Symbols guide (Google)](https://developers.google.com/fonts/docs/material_symbols) -- `icon_names` subset parameter, font-variation-settings
- [@fontsource-variable/material-symbols-rounded npm](https://www.npmjs.com/package/@fontsource-variable/material-symbols-rounded) -- version 5.2.38, axes support
- [material-symbols npm](https://www.npmjs.com/package/material-symbols) -- alternative package, version 0.42.3

### Tertiary (LOW confidence)
- [Self-hosting Material Symbols blog](https://www.lachimi.com/self-hosting-material-symbols) -- subsetting approaches (page content was not available for full verification)
- Material Symbols Rounded woff2 file sizes (~4-9MB full) -- multiple sources agree but exact size varies by version

## Metadata

**Confidence breakdown:**
- Standard stack: HIGH -- all packages verified via npm, existing project patterns well-understood
- Architecture: HIGH -- patterns derived directly from codebase analysis and UI-SPEC contract
- Pitfalls: HIGH -- identified from direct code reading and known Tailwind v4 behavior
- Icon subsetting: MEDIUM -- Google Fonts API `icon_names` parameter is documented but I haven't verified the exact output file size for this specific icon set

**Research date:** 2026-03-25
**Valid until:** 2026-04-25 (stable domain -- CSS/Tailwind patterns unlikely to change)
