# Phase 6: Canvas Token Migration and Theme Compliance - Research

**Researched:** 2026-03-27
**Domain:** Tailwind CSS v4 token migration, dual-theme compliance, ReactFlow canvas theming
**Confidence:** HIGH

## Summary

Phase 6 closes three gaps identified in the v1.3.0 milestone audit: stale Tailwind v4 token class names in canvas modules (FOUND-06 regression), incomplete dual-theme compliance for canvas overlays (THEME-05), and border token violations in canvas loading/error states (COMP-12). The scope is narrow and well-defined: six files contain all issues, with no new dependencies or architectural changes required.

The primary issues are: (1) stale token class names from the pre-Phase-1 naming scheme (`bg-bg-canvas`, `text-text-primary`, `text-accent`, `border-border-subtle`) that resolve to nonexistent CSS variables in Tailwind v4's `@theme inline` system, (2) hardcoded hex colors in `canvasHelpers.statusColor()` and ReactFlow's `connectionLineStyle` / `Background` props that don't adapt to theme, and (3) fixed Tailwind palette colors (`green-300`, `yellow-300`, `yellow-900`) in overlay toasts that look wrong on light theme.

**Primary recommendation:** Replace all stale token names with their valid `@theme inline` equivalents, convert hardcoded hex values to CSS variable references, and ensure overlay toasts use theme-adaptive tokens. This is a pure find-and-replace migration with no behavioral changes.

<phase_requirements>
## Phase Requirements

| ID | Description | Research Support |
|----|-------------|------------------|
| THEME-05 | All 25+ components readable and visually correct in both dark and light themes | Stale Token Inventory identifies every non-theme-adaptive class in target files; Token Mapping Table provides exact replacements |
| FOUND-06 | All hardcoded hex color values replaced with CSS variable token references | Hardcoded Hex Inventory catalogs all 6 hex values across 2 files; replacement values documented |
| COMP-12 | No-line rule enforced -- layout regions use surface color tiers for depth, not 1px borders | Border Audit section identifies stale `border-border-subtle` patterns; replacements use `border-outline-subtle` |
</phase_requirements>

## Project Constraints (from CLAUDE.md)

- **Tech stack**: React frontend with TypeScript, Tailwind CSS for all styling
- **Naming**: PascalCase.tsx for components, camelCase.ts for utilities; test files co-located with `.test.ts` suffix
- **Code style**: Single quotes, trailing commas, no ESLint config, Tailwind utility classes only (no separate CSS files)
- **Import organization**: Relative paths only, no aliases; `import type` for type-only imports
- **Module design**: Named exports for utilities, default export for primary React components
- **Test framework**: Vitest with jsdom environment; `@testing-library/react` for component tests
- **GSD workflow**: All changes through GSD commands

## Standard Stack

No new libraries required. This phase modifies existing code only.

### Core (already installed)
| Library | Version | Purpose | Relevance |
|---------|---------|---------|-----------|
| Tailwind CSS | 4.x | Utility-first styling with `@theme inline` | Token names must match `@theme` declarations |
| @xyflow/react | 12.x | ReactFlow canvas with CSS variable theming | `connectionLineStyle`, `Background`, `MiniMap` props need var() references |
| Vitest | 4.1 | Test runner | Validation tests for stale token detection |

## Architecture Patterns

### Files In Scope

```
frontend/src/
  App.tsx                            # 1 stale token (bg-bg-canvas, text-text-primary)
  components/
    Canvas.tsx                       # 7 stale tokens, 2 hardcoded hex values
    canvas/
      CanvasOverlays.tsx             # 9 stale tokens, 3 fixed palette colors
      CanvasPanels.tsx               # 1 stale token
      canvasHelpers.ts              # 4 hardcoded hex values in statusColor()
    ReconnectBanner.tsx              # 4 fixed palette colors (dark-only yellow-*)
```

### Token Naming: Stale vs. Valid

In Tailwind v4 with `@theme inline`, class names resolve to `--color-{name}` CSS variables. The `@theme inline` block in `index.css` defines these semantic tokens:

| Stale Class Name | Resolves To (nonexistent) | Valid Class Name | Resolves To (exists) |
|------------------|---------------------------|------------------|----------------------|
| `bg-bg-canvas` | `--color-bg-canvas` | `bg-bg` | `--color-bg` |
| `bg-bg-surface/XX` | `--color-bg-surface` | `bg-surface/XX` | `--color-surface` |
| `text-text-primary` | `--color-text-primary` | `text-on-bg` | `--color-on-bg` |
| `text-text-secondary` | `--color-text-secondary` | `text-on-bg-secondary` | `--color-on-bg-secondary` |
| `border-border-subtle` | `--color-border-subtle` | `border-outline-subtle` | `--color-outline-subtle` |
| `border-t-accent` | `--color-accent` | `border-t-primary` | `--color-primary` |
| `text-accent` | `--color-accent` | `text-primary` | `--color-primary` |
| `border-accent/XX` | `--color-accent` | `border-primary/XX` | `--color-primary` |
| `bg-accent/XX` | `--color-accent` | `bg-primary/XX` | `--color-primary` |

### Stale Token Inventory (per file)

**App.tsx (line 61):**
| Line | Stale | Replacement |
|------|-------|-------------|
| 61 | `bg-bg-canvas text-text-primary` | `bg-bg text-on-bg` |

**Canvas.tsx:**
| Line | Stale/Hardcoded | Replacement |
|------|-----------------|-------------|
| 246 | `bg-bg-canvas` | `bg-bg` |
| 247 | `border-border-subtle bg-bg-surface/85` | `border-outline-subtle bg-surface/85` |
| 248 | `border-border-subtle border-t-accent` | `border-outline-subtle border-t-primary` |
| 249 | `text-text-secondary` | `text-on-bg-secondary` |
| 257 | `bg-bg-canvas` | `bg-bg` |
| 258 | `border-border-subtle bg-bg-surface/85` | `border-outline-subtle bg-surface/85` |
| 259 | `text-status-down` | `text-status-down` (VALID -- keep) |
| 260 | `text-text-primary` | `text-on-bg` |
| 261 | `text-text-secondary` | `text-on-bg-secondary` |
| 263 | `border-accent/40 bg-accent/10 text-accent hover:bg-accent/20` | `border-primary/40 bg-primary/10 text-primary hover:bg-primary/20` |
| 272 | `bg-bg-canvas` | `bg-bg` |
| 339 | `connectionLineStyle={{ stroke: '#4a4a5e' }}` | `connectionLineStyle={{ stroke: 'var(--nt-outline)' }}` |
| 339 | `className="bg-bg-canvas"` | `className="bg-bg"` |
| 340 | `<Background color="#3f3f53"` | `<Background color="var(--nt-outline)"` |

**CanvasOverlays.tsx:**
| Line | Stale/Hardcoded | Replacement |
|------|-----------------|-------------|
| 28 | `border-accent/30 bg-bg-surface/95` | `border-primary/30 bg-surface/95` |
| 29 | `text-accent` | `text-primary` |
| 32 | `text-accent` | `text-primary` |
| 33 | `text-text-secondary` | `text-on-bg-secondary` |
| 38 | `bg-bg-surface/95` | `bg-surface/95` |
| 39 | `bg-green-400` | `bg-status-up` |
| 40 | `text-green-300` | `text-status-up` |
| 38 | `border-green-500/30` | `border-status-up/30` |
| 44 | `text-text-secondary hover:text-text-primary` | `text-on-bg-secondary hover:text-on-bg` |
| 54 | `bg-bg-surface/95` | `bg-surface/95` |
| 55 | `bg-yellow-400` | `bg-warning` |
| 56 | `text-yellow-300` | `text-warning` |
| 54 | `border-yellow-500/30` | `border-warning/30` |
| 63 | `text-yellow-400 hover:text-yellow-300` | `text-warning hover:text-warning/80` |
| 70 | `text-text-secondary hover:text-text-primary` | `text-on-bg-secondary hover:text-on-bg` |

**CanvasPanels.tsx (line 64):**
| Line | Stale | Replacement |
|------|-------|-------------|
| 64 | `text-text-secondary` | `text-on-bg-secondary` |

**canvasHelpers.ts (statusColor function, lines 81-92):**
| Hex Value | Purpose | Replacement |
|-----------|---------|-------------|
| `#00c853` | status up | `var(--color-status-up)` |
| `#ff1744` | status down | `var(--color-status-down)` |
| `#ffc107` | status probing | `var(--color-status-probing)` |
| `#657786` | status unknown | `var(--color-status-unknown)` |

**ReconnectBanner.tsx:**
| Line | Fixed Palette | Replacement |
|------|---------------|-------------|
| 9 | `bg-yellow-900/80` | `bg-warning/15` |
| 9 | `text-yellow-200` | `text-warning` |
| 14 | `border-yellow-200/30 border-t-yellow-200` | `border-warning/30 border-t-warning` |

Note on ReconnectBanner: The `bg-yellow-900/80` is a dark yellowish background intended for dark mode. For dual-theme compliance, use `bg-warning/15` which provides a subtle tinted background derived from the theme-adaptive warning color in both modes.

### Pattern: CSS Variable References in ReactFlow Props

ReactFlow v12 accepts CSS variable references as string values for style props and component props that expect colors. This is already proven in the codebase:

```typescript
// Already working in Canvas.tsx MiniMap callback (line 342):
nodeColor={(n) => {
  if (n.data.isGhost) return 'var(--nt-on-bg-muted)';  // CSS var - works
  if (a === 'down') return 'var(--color-status-down)';  // CSS var - works
  return statusColor(n.data.device.status);              // hex - needs fixing
}}

// MiniMap inline style (line 343):
style={{ backgroundColor: 'var(--nt-surface)' }}  // CSS var - works
maskColor="var(--nt-minimap-mask, ...)"            // CSS var - works
```

So replacing `statusColor()` hex returns with CSS variable references is safe.

### Pattern: Connection Line Theming

```typescript
// Current (hardcoded, dark-only):
connectionLineStyle={{ stroke: '#4a4a5e', strokeWidth: 2 }}

// Fixed (theme-adaptive):
connectionLineStyle={{ stroke: 'var(--nt-outline)', strokeWidth: 2 }}
```

### Pattern: Background Dots Theming

```tsx
// Current (hardcoded, dark-only):
<Background color="#3f3f53" gap={28} size={1.2} />

// Fixed (theme-adaptive):
<Background color="var(--nt-outline)" gap={28} size={1.2} />
```

Note: `--nt-outline` resolves to `#333338` (dark) / `#D0D0D8` (light), which provides appropriate subtle dot colors for both themes. The ReactFlow CSS variable override `--xy-background-pattern-dots-color-default: var(--nt-outline)` already handles default dots, but the explicit `color` prop on `<Background>` overrides it. Switching the prop to `var(--nt-outline)` makes them consistent.

## Don't Hand-Roll

| Problem | Don't Build | Use Instead | Why |
|---------|-------------|-------------|-----|
| Theme-adaptive status colors | New color mapping function | CSS variables (`var(--color-status-up)`) | Already defined in `@theme inline`, auto-switch on theme change |
| Dark/light overlay backgrounds | Conditional classNames per theme | Semantic tokens (`bg-surface`, `bg-warning/15`) | Token system handles both themes automatically |
| MiniMap theme colors | Manual hex switching | CSS variable strings | ReactFlow v12 resolves CSS variables in nodeColor/maskColor props |

## Common Pitfalls

### Pitfall 1: Double-Prefix Token Names
**What goes wrong:** Using `bg-bg-canvas` (which Tailwind interprets as color `bg-canvas` under the `bg-` utility, looking for `--color-bg-canvas`).
**Why it happens:** Pre-Phase-1 naming used `bg-canvas` as a semantic name. After Tailwind v4 migration, the color is just `bg` in `@theme inline`.
**How to avoid:** The valid surface hierarchy is: `bg`, `surface`, `surface-high`, `elevated`. There is no `canvas` or `bg-surface` color token.
**Warning signs:** Tailwind build produces no error for unknown tokens -- the class is generated but the CSS variable resolves to nothing, producing transparent/invisible elements.

### Pitfall 2: accent vs. primary Token
**What goes wrong:** Using `text-accent`, `border-accent`, `bg-accent` which resolve to nonexistent `--color-accent`.
**Why it happens:** "accent" was a conceptual name. The actual token is `primary` (maps to `--color-primary` = neon green/dark green).
**How to avoid:** Use `text-primary`, `border-primary`, `bg-primary` consistently.
**Warning signs:** Elements become invisible or use browser default colors.

### Pitfall 3: Fixed Palette Colors in Themed Overlays
**What goes wrong:** `text-green-300`, `bg-yellow-400`, `text-yellow-300` look fine on dark backgrounds but have poor contrast or wrong visual weight on light backgrounds.
**Why it happens:** Tailwind's numbered palette (e.g., `green-300`) is fixed regardless of theme. The project's semantic tokens (`status-up`, `warning`) are theme-adaptive.
**How to avoid:** Use semantic tokens: `text-status-up` (not `text-green-300`), `text-warning` (not `text-yellow-300`), `bg-warning` (not `bg-yellow-400`).
**Warning signs:** Overlays look washed-out or have jarring contrast on light theme.

### Pitfall 4: statusColor() Returns Used in DOM
**What goes wrong:** The `statusColor()` function returns hardcoded hex like `#00c853`. When used in MiniMap's `nodeColor` callback, the color doesn't change when the theme switches.
**Why it happens:** This function predates the token system and was never updated.
**How to avoid:** Return CSS variable references: `var(--color-status-up)`.
**Warning signs:** MiniMap dots stay neon-bright even on light theme where status colors should be darkened.

### Pitfall 5: Transparent Failures
**What goes wrong:** Stale token names don't cause build errors. An element with `bg-bg-canvas` simply gets `background-color: var(--color-bg-canvas)` which resolves to nothing (transparent).
**Why it happens:** Tailwind v4 generates classes for any color-like token name without validation.
**How to avoid:** Automated test that scans target files for known stale patterns.
**Warning signs:** Elements unexpectedly transparent; visible on one theme but invisible on the other.

## Code Examples

### statusColor() Migration

```typescript
// Before (hardcoded hex, dark-only):
export function statusColor(status: Device['status']): string {
  switch (status) {
    case 'up':    return '#00c853';
    case 'down':  return '#ff1744';
    case 'probing': return '#ffc107';
    default:      return '#657786';
  }
}

// After (CSS variable references, theme-adaptive):
export function statusColor(status: Device['status']): string {
  switch (status) {
    case 'up':    return 'var(--color-status-up)';
    case 'down':  return 'var(--color-status-down)';
    case 'probing': return 'var(--color-status-probing)';
    default:      return 'var(--color-status-unknown)';
  }
}
```

### Overlay Token Migration

```tsx
// Before (CanvasOverlays.tsx edit mode banner):
<div className="... border border-accent/30 bg-bg-surface/95 ...">
  <svg className="text-accent">...</svg>
  <p className="text-accent">Edit Mode</p>
  <span className="text-text-secondary">Press E to exit</span>
</div>

// After:
<div className="... border border-primary/30 bg-surface/95 ...">
  <svg className="text-primary">...</svg>
  <p className="text-primary">Edit Mode</p>
  <span className="text-on-bg-secondary">Press E to exit</span>
</div>
```

### Validation Test Pattern

```typescript
// Scan for stale tokens in target files
import { readFileSync } from 'fs';
import { join } from 'path';

const STALE_PATTERNS = [
  /bg-bg-canvas/,
  /bg-bg-surface/,
  /text-text-primary/,
  /text-text-secondary/,
  /border-border-subtle/,
  /text-accent(?!-)/,
  /border-accent/,
  /bg-accent/,
  /border-t-accent/,
];

const TARGET_FILES = [
  'App.tsx',
  'components/Canvas.tsx',
  'components/canvas/CanvasOverlays.tsx',
  'components/canvas/CanvasPanels.tsx',
  'components/canvas/canvasHelpers.ts',
  'components/ReconnectBanner.tsx',
];
```

## Validation Architecture

### Test Framework
| Property | Value |
|----------|-------|
| Framework | Vitest 4.1 |
| Config file | `frontend/vitest.config.ts` |
| Quick run command | `cd frontend && npx vitest run` |
| Full suite command | `cd frontend && npx vitest run` |

### Phase Requirements -> Test Map
| Req ID | Behavior | Test Type | Automated Command | File Exists? |
|--------|----------|-----------|-------------------|-------------|
| FOUND-06 | No hardcoded hex in target files | unit (source scan) | `cd frontend && npx vitest run src/components/__tests__/canvas-token-audit.test.ts -x` | Wave 0 |
| THEME-05 | Canvas overlays use theme-adaptive tokens | unit (source scan) | `cd frontend && npx vitest run src/components/__tests__/canvas-token-audit.test.ts -x` | Wave 0 |
| COMP-12 | No stale border tokens in canvas files | unit (source scan) | `cd frontend && npx vitest run src/components/__tests__/no-line-audit.test.ts -x` | Exists (already passes) |

### Sampling Rate
- **Per task commit:** `cd frontend && npx vitest run -x`
- **Per wave merge:** `cd frontend && npx vitest run`
- **Phase gate:** Full suite green before `/gsd:verify-work`

### Wave 0 Gaps
- [ ] `frontend/src/components/__tests__/canvas-token-audit.test.ts` -- covers FOUND-06, THEME-05 (scans target files for stale token patterns and hardcoded hex)

*(The existing `no-line-audit.test.ts` already covers COMP-12 and passes.)*

## Sources

### Primary (HIGH confidence)
- **`frontend/src/index.css`** -- Definitive source of truth for all valid token names in `@theme inline` block
- **`frontend/src/components/Canvas.tsx`** -- Direct inspection of all stale tokens and hardcoded hex values
- **`frontend/src/components/canvas/CanvasOverlays.tsx`** -- Direct inspection of overlay stale tokens
- **`frontend/src/components/canvas/CanvasPanels.tsx`** -- Direct inspection (1 stale token)
- **`frontend/src/components/canvas/canvasHelpers.ts`** -- Direct inspection of statusColor() hardcoded hex
- **`frontend/src/components/ReconnectBanner.tsx`** -- Direct inspection of fixed palette colors
- **`frontend/src/App.tsx`** -- Direct inspection (1 line with stale tokens)

### Secondary (HIGH confidence)
- **`frontend/src/components/DeviceCard.tsx`**, **Dashboard.tsx**, **LinkEdge.tsx** -- Reference implementations showing correct token usage patterns established in Phases 2-5

## Metadata

**Confidence breakdown:**
- Standard stack: HIGH -- no new libraries; pure token replacement in existing files
- Architecture: HIGH -- file scope and exact changes fully enumerated by direct source inspection
- Pitfalls: HIGH -- stale token behavior verified by cross-referencing `@theme inline` declarations against class names in target files

**Research date:** 2026-03-27
**Valid until:** 2026-04-27 (stable -- token system is locked; no upstream changes expected)
