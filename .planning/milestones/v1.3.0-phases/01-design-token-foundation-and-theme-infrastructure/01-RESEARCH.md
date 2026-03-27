# Phase 1: Design Token Foundation and Theme Infrastructure - Research

**Researched:** 2026-03-25
**Domain:** CSS design token architecture, Tailwind CSS v4 migration, React Flow v12 migration, self-hosted font loading, dark/light theme switching
**Confidence:** HIGH

## Summary

Phase 1 is the foundation that every subsequent phase depends on. It delivers three tightly coupled changes: (1) a CSS custom property token system with dark and light Neon Topography palettes consumed via Tailwind v4's `@theme` directive, (2) library upgrades (Tailwind v3 to v4, reactflow v11 to @xyflow/react v12) that unlock native CSS variable theming, and (3) self-hosted fonts (Outfit + JetBrains Mono) replacing IBM Plex Sans. The phase also requires replacing all hardcoded hex color values in 8 source files (45 occurrences total) with semantic token references, building a ThemeProvider context (~40 lines), adding a FOWT prevention inline script to index.html, and wiring a sun/moon toggle button into the NavBar.

The technical risk is low-to-medium. Tailwind v4 has an official automated codemod (`npx @tailwindcss/upgrade`) that handles ~90% of the migration. React Flow v12's breaking changes are mechanical import renames plus a `node.measured` API change that does NOT affect this codebase (useAutoLayout.ts uses d3-force directly, not React Flow node dimensions). The hardcoded hex replacement is tedious but mechanical. The highest risk is the Tailwind v4 `@theme` + CSS variable indirection pattern for per-theme tokens -- this requires a specific architecture (define raw variables in `:root`/`[data-theme]` selectors, then reference them inside `@theme inline`) that is well-documented but easy to get wrong.

**Primary recommendation:** Execute in this order: (1) Tailwind v4 upgrade via codemod, (2) React Flow v12 package swap, (3) font installation, (4) CSS token system with dark/light variables, (5) ThemeProvider + FOWT script, (6) hardcoded hex replacement, (7) NavBar toggle button. Each step is independently verifiable.

<user_constraints>
## User Constraints (from CONTEXT.md)

### Locked Decisions
- **D-01:** Theme toggle lives in the current NavBar, right side, as a sun/moon icon swap button
- **D-02:** Icon style is a single icon that swaps between sun (light mode active) and moon (dark mode active) -- not a pill slider
- **D-03:** This toggle location is temporary -- it gets absorbed into the NavigationPill in Phase 2
- **D-04:** Light theme base background is cool gray `#F5F5F7` (Apple-style, not pure white)
- **D-05:** Light theme surface hierarchy: Background `#F5F5F7` > Surface `#EDEDF0` > Elevated `#FFFFFF` > Text `#1A1A1C`
- **D-06:** Primary green accent `#00E676` stays the same hue in light theme -- glow intensity reduced (shadow opacity ~0.25 instead of ~0.5, bloom opacity ~0.06 instead of ~0.15)
- **D-07:** Glassmorphism is a dark-mode-only signature. Light theme uses tinted solid surfaces: `rgba(255,255,255,0.85)` background, no `backdrop-filter`, subtle border `rgba(0,0,0,0.06)`
- **D-08:** Status colors are theme-invariant -- same hex values in both dark and light themes. Glow intensity varies by theme, hue does not
- **D-09:** Status 'up' color aligns with primary glow: `#00E676` (not the old `#00c853`)
- **D-10:** Full status palette: Up `#00E676`, Down `#FF1744`, Probing `#FFEA00`, Unknown `#9E9E9E`
- **D-11:** Area-specific accent colors (Secondary `#2979FF` blue, Tertiary `#E040FB` purple) are included in the Phase 1 token system even though areas are not built until Phase 3/4. Complete token system from day one.

### Claude's Discretion
- Token naming conventions (semantic names that bridge DESIGN.md vocabulary and Tailwind conventions)
- Number of surface tiers beyond the 3 defined for light theme (dark theme may need more granularity per DESIGN.md)
- Exact glow/bloom CSS values -- guidelines are set (reduced intensity in light mode), Claude tunes specific values
- ReactFlow v12 migration approach and `useAutoLayout.ts` adaptation for `node.measured` API change
- Tailwind v4 migration strategy (codemod-first vs manual)
- FOWT prevention inline script implementation

### Deferred Ideas (OUT OF SCOPE)
None -- discussion stayed within phase scope
</user_constraints>

<phase_requirements>
## Phase Requirements

| ID | Description | Research Support |
|----|-------------|------------------|
| FOUND-01 | CSS variable token system defines semantic color tokens (surface, text, status, accent) mapped to primitive values per theme | Tailwind v4 `@theme` directive + CSS custom properties in `:root`/`[data-theme]` selectors; architecture pattern documented below |
| FOUND-02 | Tailwind CSS v3 migrated to v4 using `@theme` directive for native CSS variable integration | Official codemod `npx @tailwindcss/upgrade` handles ~90%; manual fixups for `@custom-variant dark`, PostCSS removal, Vite plugin addition |
| FOUND-03 | React Flow upgraded from v11 (`reactflow`) to v12 (`@xyflow/react`) with native `colorMode` support | Package rename, import path changes, `colorMode` prop, `--xy-*` CSS variable overrides; breaking changes documented below |
| FOUND-04 | Outfit font loaded via self-hosted Fontsource variable package for display and UI text | `@fontsource-variable/outfit@5.2.8` -- single import in entry CSS, variable font covers all weights |
| FOUND-05 | JetBrains Mono font loaded via self-hosted Fontsource variable package for technical readouts | `@fontsource-variable/jetbrains-mono@5.2.8` -- same pattern as Outfit |
| FOUND-06 | All hardcoded hex color values replaced with CSS variable token references | 45 occurrences across 8 files identified; full audit below with per-file counts |
| THEME-01 | User can toggle between dark and light themes via a UI control | Sun/moon icon button in NavBar right side (D-01, D-02); ThemeProvider context sets `data-theme` attribute |
| THEME-02 | User's theme preference persists across browser sessions via localStorage | ThemeProvider reads/writes `localStorage('theia-theme')`; pattern documented below |
| THEME-03 | App defaults to OS dark/light preference when no explicit user choice exists | `window.matchMedia('(prefers-color-scheme: dark)')` in ThemeProvider with `'system'` as default; listener for OS changes |
| THEME-04 | No flash of wrong theme (FOWT) on page load -- inline script in `<head>` applies theme before render | Synchronous inline `<script>` in index.html `<head>` before any CSS; reads localStorage, sets `data-theme` attribute before first paint |
</phase_requirements>

## Standard Stack

### Core (Delta from Current)

| Library | Version | Purpose | Why Standard |
|---------|---------|---------|--------------|
| tailwindcss | 4.2.2 | Utility-first CSS framework with native CSS variable tokens | `@theme` directive generates CSS custom properties AND utility classes from one declaration; backbone of the entire token system |
| @tailwindcss/vite | 4.2.2 | Vite build plugin replacing PostCSS pipeline | Direct Vite integration, faster HMR, built-in vendor prefixing (replaces autoprefixer) |
| @xyflow/react | 12.10.1 | Network topology canvas (replaces `reactflow` package) | Native `colorMode` prop, `--xy-*` CSS variable namespace for theme integration, active maintenance |
| @fontsource-variable/outfit | 5.2.8 | Display and UI font (self-hosted variable) | Single file covers all weights (400-700), no CDN dependency, ~40KB WOFF2 |
| @fontsource-variable/jetbrains-mono | 5.2.8 | Technical readout font (self-hosted variable) | Monospace for metrics/code, variable font, ~50KB WOFF2 |

### Removed

| Library | Version | Reason |
|---------|---------|--------|
| tailwindcss (as PostCSS plugin) | 3.4.17 | Replaced by `@tailwindcss/vite` |
| autoprefixer | 10.4.20 | Built into Tailwind v4 |
| postcss | 8.4.49 | No longer needed as standalone config (Vite handles internally) |
| reactflow | 11.11.4 | Renamed to `@xyflow/react` |

### No New Libraries Required

| Category | Decision | Rationale |
|----------|----------|-----------|
| Theme state | React Context (built-in) | ThemeProvider is ~40 lines; no external theme library needed for a Vite SPA |
| Theme library | NOT next-themes, use-dark-mode | Those solve SSR flash (Next.js); irrelevant for client-rendered Vite SPA |
| State management | NOT Zustand | Theme state is a single string; React Context sufficient |
| CSS approach | NOT CSS-in-JS | Tailwind is zero-runtime; CSS variables handle theming without JS |

**Version verification:** All package versions verified against npm registry on 2026-03-25:
- `npm view tailwindcss version` = 4.2.2
- `npm view @tailwindcss/vite version` = 4.2.2
- `npm view @xyflow/react version` = 12.10.1
- `npm view @fontsource-variable/outfit version` = 5.2.8
- `npm view @fontsource-variable/jetbrains-mono version` = 5.2.8

**Installation:**
```bash
cd frontend

# Remove deprecated packages
npm uninstall reactflow autoprefixer

# Install Tailwind v4 with Vite plugin
npm install -D tailwindcss@^4.2.2 @tailwindcss/vite@^4.2.2

# Install React Flow v12
npm install @xyflow/react@^12.10.1

# Install fonts
npm install @fontsource-variable/outfit@^5.2.8 @fontsource-variable/jetbrains-mono@^5.2.8
```

Note: `postcss` stays as a devDependency (Vite uses it internally) but `postcss.config.js` is deleted. The old `tailwindcss` package (3.4.17) is replaced by the new version -- npm handles this automatically.

## Architecture Patterns

### Recommended File Changes

```
frontend/
├── index.html                        # ADD: FOWT prevention inline script in <head>
├── postcss.config.js                 # DELETE: replaced by @tailwindcss/vite
├── tailwind.config.js                # DELETE: config moves to CSS @theme
├── vite.config.ts                    # MODIFY: add @tailwindcss/vite plugin
├── src/
│   ├── main.tsx                      # MODIFY: update style import path
│   ├── index.css                     # REWRITE: @import "tailwindcss", @theme, @custom-variant, token definitions
│   ├── contexts/
│   │   └── ThemeContext.tsx           # NEW: ThemeProvider + useTheme hook
│   ├── components/
│   │   ├── NavBar.tsx                # MODIFY: add theme toggle button
│   │   ├── Canvas.tsx                # MODIFY: update reactflow imports, add colorMode prop, replace hex colors
│   │   ├── DeviceCard.tsx            # MODIFY: update reactflow imports, replace hex colors
│   │   ├── LinkEdge.tsx              # MODIFY: update reactflow imports, replace hex colors
│   │   └── icons/DeviceIcon.tsx      # MODIFY: replace hex SVG fills with currentColor or token
│   ├── types/
│   │   └── metrics.ts                # MODIFY: replace hex color returns with CSS variable references
│   └── App.tsx                       # MODIFY: wrap with ThemeProvider, update reactflow imports
```

### Pattern 1: CSS Token System with Per-Theme Variables (Tailwind v4)

**What:** Define primitive CSS variables in `:root`/`[data-theme]` selectors, then reference them in `@theme inline` to generate both CSS variables AND Tailwind utility classes.

**Why `@theme inline`:** The `inline` keyword tells Tailwind to use the variable reference itself (not resolve it at build time), so the value changes at runtime when `data-theme` switches.

**Complete index.css architecture:**

```css
@import "tailwindcss";
@import "@fontsource-variable/outfit";
@import "@fontsource-variable/jetbrains-mono";

/* --- Theme variant: dark mode triggered by data-theme attribute --- */
@custom-variant dark (&:where([data-theme=dark], [data-theme=dark] *));

/* --- Primitive token values per theme --- */

/* Dark theme (default) */
:root,
[data-theme="dark"] {
  color-scheme: dark;

  /* Surface hierarchy (from DESIGN.md) */
  --nt-bg: #161618;
  --nt-surface: #1E1E21;
  --nt-surface-high: #262629;
  --nt-elevated: #2E2E32;

  /* Text */
  --nt-on-bg: #F5F5F7;
  --nt-on-bg-secondary: #8899A6;
  --nt-on-bg-muted: #657786;

  /* Borders */
  --nt-outline: #333338;
  --nt-outline-subtle: #2A2A2E;

  /* Glassmorphism (dark-mode-only per D-07) */
  --nt-glass-bg: rgba(255, 255, 255, 0.02);
  --nt-glass-border: rgba(255, 255, 255, 0.06);
  --nt-glass-backdrop: blur(16px);

  /* Glow intensity */
  --nt-glow-shadow-opacity: 0.5;
  --nt-glow-bloom-opacity: 0.15;

  /* Shadows */
  --nt-shadow-panel: 0 24px 48px rgba(0, 0, 0, 0.2);
  --nt-shadow-pill: 0 24px 48px rgba(0, 0, 0, 0.5);
  --nt-shadow-canvas: 0 24px 60px rgba(0, 0, 0, 0.28);
}

/* Light theme (per D-04, D-05, D-06, D-07) */
[data-theme="light"] {
  color-scheme: light;

  /* Surface hierarchy */
  --nt-bg: #F5F5F7;
  --nt-surface: #EDEDF0;
  --nt-surface-high: #E4E4E8;
  --nt-elevated: #FFFFFF;

  /* Text */
  --nt-on-bg: #1A1A1C;
  --nt-on-bg-secondary: #5A5A6E;
  --nt-on-bg-muted: #8A8A93;

  /* Borders */
  --nt-outline: #D0D0D8;
  --nt-outline-subtle: #E0E0E5;

  /* Light mode: tinted solid surfaces, not glassmorphism (D-07) */
  --nt-glass-bg: rgba(255, 255, 255, 0.85);
  --nt-glass-border: rgba(0, 0, 0, 0.06);
  --nt-glass-backdrop: none;

  /* Reduced glow intensity (D-06) */
  --nt-glow-shadow-opacity: 0.25;
  --nt-glow-bloom-opacity: 0.06;

  /* Shadows */
  --nt-shadow-panel: 0 24px 48px rgba(0, 0, 0, 0.08);
  --nt-shadow-pill: 0 24px 48px rgba(0, 0, 0, 0.15);
  --nt-shadow-canvas: 0 24px 60px rgba(0, 0, 0, 0.12);
}

/* --- Tailwind theme: maps primitive vars to utility classes --- */
@theme inline {
  /* Surface colors */
  --color-bg: var(--nt-bg);
  --color-surface: var(--nt-surface);
  --color-surface-high: var(--nt-surface-high);
  --color-elevated: var(--nt-elevated);

  /* Text colors */
  --color-on-bg: var(--nt-on-bg);
  --color-on-bg-secondary: var(--nt-on-bg-secondary);
  --color-on-bg-muted: var(--nt-on-bg-muted);

  /* Borders */
  --color-outline: var(--nt-outline);
  --color-outline-subtle: var(--nt-outline-subtle);

  /* Status colors -- theme-invariant per D-08 */
  --color-status-up: #00E676;
  --color-status-down: #FF1744;
  --color-status-probing: #FFEA00;
  --color-status-unknown: #9E9E9E;

  /* Accent colors */
  --color-primary: #00E676;
  --color-secondary: #2979FF;
  --color-tertiary: #E040FB;
  --color-warning: #FFEA00;
  --color-critical: #FF1744;

  /* Glass */
  --color-glass-bg: var(--nt-glass-bg);
  --color-glass-border: var(--nt-glass-border);

  /* Typography */
  --font-display: "Outfit Variable", "Outfit", system-ui, sans-serif;
  --font-mono: "JetBrains Mono Variable", "JetBrains Mono", ui-monospace, monospace;
  --font-sans: "Outfit Variable", "Outfit", system-ui, sans-serif;

  /* Shadows */
  --shadow-panel: var(--nt-shadow-panel);
  --shadow-pill: var(--nt-shadow-pill);
  --shadow-canvas: var(--nt-shadow-canvas);

  /* Radii */
  --radius-pill: 9999px;
  --radius-panel: 12px;
}

/* --- React Flow v12 theme overrides --- */
.react-flow {
  --xy-background-pattern-dots-color-default: var(--nt-outline);
  --xy-edge-stroke-default: var(--nt-outline);
  --xy-edge-stroke-width-default: 2;
  --xy-node-background-color-default: var(--nt-surface);
  --xy-node-border-default: 1px solid var(--nt-outline);
  --xy-node-color-default: var(--nt-on-bg);
  --xy-minimap-background-color-default: var(--nt-surface);
  --xy-controls-button-background-color-default: var(--nt-surface);
  --xy-controls-button-background-color-hover-default: var(--nt-surface-high);
  --xy-controls-button-color-default: var(--nt-on-bg);
  --xy-controls-button-border-color-default: var(--nt-outline);
  --xy-handle-background-color-default: var(--nt-on-bg-secondary);
  --xy-handle-border-color-default: var(--nt-bg);
  --xy-selection-background-color-default: rgba(0, 230, 118, 0.08);
  --xy-selection-border-default: 1px dotted rgba(0, 230, 118, 0.5);
}

/* --- Base styles --- */
@layer base {
  html, body, #root {
    min-height: 100%;
  }

  html {
    background-color: var(--nt-bg);
  }

  body {
    margin: 0;
    background-color: var(--nt-bg);
    color: var(--nt-on-bg);
  }
}
```

**Generated utility classes:** `bg-bg`, `bg-surface`, `bg-surface-high`, `bg-elevated`, `text-on-bg`, `text-on-bg-secondary`, `text-on-bg-muted`, `text-primary`, `text-secondary`, `text-critical`, `border-outline`, `font-display`, `font-mono`, `shadow-panel`, `rounded-panel`, etc.

### Pattern 2: ThemeProvider as Thin Data-Theme Attribute Setter

**What:** Minimal React context that manages the `data-theme` attribute on `<html>`, persists to localStorage, and detects OS preference.

**Architecture:** ThemeProvider sets a DOM attribute, NOT a CSS class. This aligns with the `@custom-variant dark (&:where([data-theme=dark], ...))` configuration and avoids collision with React Flow's own `.dark` class.

```typescript
// contexts/ThemeContext.tsx
import { createContext, useContext, useEffect, useMemo, useState, type ReactNode } from 'react';

type ThemePreference = 'dark' | 'light' | 'system';
type ResolvedTheme = 'dark' | 'light';

interface ThemeContextValue {
  theme: ThemePreference;
  resolvedTheme: ResolvedTheme;
  setTheme: (theme: ThemePreference) => void;
}

const STORAGE_KEY = 'theia-theme';

const ThemeContext = createContext<ThemeContextValue | null>(null);

function getSystemTheme(): ResolvedTheme {
  return window.matchMedia('(prefers-color-scheme: dark)').matches ? 'dark' : 'light';
}

export function ThemeProvider({ children }: { children: ReactNode }) {
  const [theme, setThemeState] = useState<ThemePreference>(() => {
    const stored = localStorage.getItem(STORAGE_KEY);
    if (stored === 'dark' || stored === 'light' || stored === 'system') return stored;
    return 'system'; // Default: follow OS (THEME-03)
  });

  const resolvedTheme = useMemo<ResolvedTheme>(() => {
    return theme === 'system' ? getSystemTheme() : theme;
  }, [theme]);

  // Apply data-theme attribute to <html> (THEME-01, THEME-02)
  useEffect(() => {
    document.documentElement.setAttribute('data-theme', resolvedTheme);
    document.documentElement.style.colorScheme = resolvedTheme;
  }, [resolvedTheme]);

  // Persist preference (THEME-02)
  useEffect(() => {
    localStorage.setItem(STORAGE_KEY, theme);
  }, [theme]);

  // Listen for OS theme changes when in 'system' mode (THEME-03)
  useEffect(() => {
    if (theme !== 'system') return;
    const mql = window.matchMedia('(prefers-color-scheme: dark)');
    const handler = () => setThemeState((prev) => prev === 'system' ? 'system' : prev);
    // Force re-resolve by toggling back to system
    const realHandler = () => {
      // Trigger re-render to recalculate resolvedTheme
      setThemeState('system');
    };
    mql.addEventListener('change', realHandler);
    return () => mql.removeEventListener('change', realHandler);
  }, [theme]);

  const setTheme = (newTheme: ThemePreference) => setThemeState(newTheme);

  return (
    <ThemeContext.Provider value={{ theme, resolvedTheme, setTheme }}>
      {children}
    </ThemeContext.Provider>
  );
}

export function useTheme(): ThemeContextValue {
  const ctx = useContext(ThemeContext);
  if (!ctx) throw new Error('useTheme must be used within ThemeProvider');
  return ctx;
}
```

### Pattern 3: FOWT Prevention Inline Script (THEME-04)

**What:** Synchronous inline script in `index.html` `<head>` that applies the correct theme before first paint.

**Why inline and synchronous:** The browser parses `<head>` before rendering any content. An inline script runs before external CSS is fetched. This prevents any visible flash of the wrong theme.

```html
<!doctype html>
<html lang="en" data-theme="dark">
  <head>
    <meta charset="UTF-8" />
    <meta name="viewport" content="width=device-width, initial-scale=1.0" />
    <title>Theia</title>
    <script>
      // FOWT prevention: apply saved theme before first paint
      (function() {
        var stored = localStorage.getItem('theia-theme');
        var theme;
        if (stored === 'light') {
          theme = 'light';
        } else if (stored === 'dark') {
          theme = 'dark';
        } else {
          // 'system' or no preference: check OS
          theme = window.matchMedia('(prefers-color-scheme: dark)').matches ? 'dark' : 'light';
        }
        document.documentElement.setAttribute('data-theme', theme);
        document.documentElement.style.colorScheme = theme;
      })();
    </script>
  </head>
  <body>
    <div id="root"></div>
    <script type="module" src="/src/main.tsx"></script>
  </body>
</html>
```

**Key detail:** The default `data-theme="dark"` on `<html>` is a fallback for users with JavaScript disabled. The inline script overrides it immediately.

### Pattern 4: React Flow v12 ColorMode Integration

**What:** Pass `colorMode` prop to `<ReactFlow>` driven by the ThemeProvider context, and override `--xy-*` CSS variables to match Neon Topography tokens.

```tsx
// In Canvas.tsx (or wherever ReactFlow is rendered)
import { useTheme } from '../contexts/ThemeContext';

function Canvas() {
  const { resolvedTheme } = useTheme();
  // ...
  return (
    <ReactFlow
      colorMode={resolvedTheme}
      nodes={nodes}
      edges={edges}
      // ... other props
    >
      <Background color="var(--nt-outline)" gap={28} size={1.2} />
      <MiniMap />
    </ReactFlow>
  );
}
```

### Pattern 5: Token Naming Convention (Claude's Discretion)

**Recommendation:** Use a two-tier naming system:

| Tier | Prefix | Example Variable | Example Utility | Purpose |
|------|--------|-----------------|-----------------|---------|
| Primitive | `--nt-*` | `--nt-bg`, `--nt-surface` | (no utility -- internal only) | Raw values that change per theme |
| Semantic | `--color-*`, `--font-*`, `--shadow-*` | `--color-bg`, `--font-display` | `bg-bg`, `font-display` | Tailwind-consumed tokens |

The `--nt-*` prefix (Neon Topography) keeps primitive tokens namespaced away from Tailwind's `--color-*` namespace. The `@theme inline` block bridges the two.

**Token-to-old-class mapping (migration guide):**

| Old Class | New Class | Notes |
|-----------|-----------|-------|
| `bg-bg-canvas` | `bg-bg` | Renamed for brevity |
| `bg-bg-surface` | `bg-surface` | Direct map |
| `bg-bg-elevated` | `bg-elevated` | Direct map |
| `text-text-primary` | `text-on-bg` | Follows Material Design "on-surface" convention |
| `text-text-secondary` | `text-on-bg-secondary` | Same |
| `border-border-subtle` | `border-outline` | Aligns with DESIGN.md "outline" terminology |
| `text-accent` | `text-primary` | Green is the primary color in Neon Topography |
| `border-accent-purple` | `border-tertiary` | Purple is the tertiary accent |
| `bg-status-up` | `bg-status-up` | Unchanged |

### Anti-Patterns to Avoid

- **Fat theme context:** Do NOT pass a theme object with color values through React context. Use CSS custom properties on `<html>` and let CSS cascade handle everything. This avoids re-rendering 100+ canvas nodes on theme switch.
- **`dark:` prefix everywhere:** Do NOT use Tailwind's `dark:bg-x` pattern for theme-aware colors. The CSS variable indirection means `bg-surface` resolves to the correct color in both themes. Reserve `dark:` only for rare one-off overrides.
- **Hardcoded hex in glow shadows:** Do NOT use `shadow-[0_0_14px_rgba(0,200,83,0.55)]`. Define glow shadows as CSS custom properties so they respect theme-specific opacity values.
- **Removing postcss entirely from devDependencies:** PostCSS is still used internally by Vite. Only delete `postcss.config.js`, not the `postcss` package itself. Let the Tailwind codemod handle the dependency cleanup.

## Don't Hand-Roll

| Problem | Don't Build | Use Instead | Why |
|---------|-------------|-------------|-----|
| Theme switching logic | Custom DOM manipulation + event listeners | ThemeProvider context (~40 lines) + CSS `data-theme` cascade | CSS cascade handles 100% of visual changes without React re-renders |
| CSS variable token system | Manually maintaining `:root` vars AND Tailwind config in sync | Tailwind v4 `@theme inline` directive | Single source of truth: `@theme` generates both CSS vars and utility classes |
| Dark mode variant | Manual `[data-theme=dark] .component` selectors | `@custom-variant dark` in Tailwind | Enables `dark:bg-x` syntax across all utilities for one-off overrides |
| Font loading infrastructure | Manual `@font-face` declarations with preload links | Fontsource packages (`import '@fontsource-variable/outfit'`) | Fontsource provides optimized `@font-face` with `font-display: swap`, WOFF2, proper unicode-range |
| React Flow theme mapping | Manual CSS overrides on `.react-flow__*` selectors | `colorMode` prop + `--xy-*` variable overrides | v12 designed for this; `colorMode` adds correct class, variables cascade automatically |
| FOWT prevention | React-based theme detection on mount | Inline `<script>` in `<head>` (before CSS) | React hydrates AFTER first paint; by then the wrong theme has already flashed |
| Upgrade codemod | Manual find-and-replace of Tailwind classes | `npx @tailwindcss/upgrade` official tool | Handles shadow-sm to shadow-xs renames, `!important` syntax changes, config migration |

## Common Pitfalls

### Pitfall 1: Tailwind v4 `@theme` vs `@theme inline` Confusion

**What goes wrong:** Defining theme tokens with `@theme { --color-surface: var(--nt-surface); }` (without `inline`) causes Tailwind to resolve the variable at build time instead of keeping the `var()` reference. Result: all themes resolve to the dark theme values because `:root` is the build-time context.

**Why it happens:** `@theme` without `inline` inlines the resolved value. `@theme inline` preserves the `var()` reference so runtime theme switching works.

**How to avoid:** Always use `@theme inline` for tokens that reference per-theme CSS variables. Only use plain `@theme` for static values (like `--color-primary: #00E676` which is the same in both themes).

**Warning signs:** Theme toggle changes the `data-theme` attribute in DevTools but no colors change visually.

### Pitfall 2: React Flow v12 Style Import Location

**What goes wrong:** After migrating to `@xyflow/react`, the style import `import '@xyflow/react/dist/style.css'` is placed in a component file (like `Canvas.tsx` or `main.tsx`). With Tailwind v4, this can cause style ordering issues where React Flow's defaults override custom theme variables.

**Why it happens:** Tailwind v4 uses CSS layers. React Flow styles imported outside the layer system may have higher specificity.

**How to avoid:** Import React Flow styles in `index.css` within a `@layer base` block, AFTER the Tailwind import:

```css
@import "tailwindcss";

@layer base {
  @import "@xyflow/react/dist/style.css";
}
```

Remove the `import 'reactflow/dist/style.css'` from `main.tsx`.

### Pitfall 3: Hardcoded Hex in JavaScript Return Values

**What goes wrong:** Files like `types/metrics.ts` and `LinkEdge.tsx` return hex color strings from JavaScript functions (e.g., `return '#ff1744'`). These are used as inline styles or passed to React Flow edge props. They cannot be replaced with Tailwind utility classes because they are runtime values.

**Why it happens:** Status colors and utilization-based colors are computed in JS and applied via `style` props or React Flow edge data.

**How to avoid:** Replace hex returns with CSS variable references using `getComputedStyle`:

```typescript
// Before
return '#ff1744';

// After -- use CSS variable reference for inline styles
return 'var(--color-status-down)';
```

OR use a lookup object that reads from CSS:

```typescript
const STATUS_COLORS = {
  up: 'var(--color-status-up)',
  down: 'var(--color-status-down)',
  probing: 'var(--color-status-probing)',
  unknown: 'var(--color-status-unknown)',
} as const;
```

This works because `style={{ color: 'var(--color-status-up)' }}` is valid CSS and resolves at render time.

### Pitfall 4: useAutoLayout.ts Does NOT Need node.measured Changes

**What goes wrong:** Developers see the React Flow v12 migration guide warning about `node.measured` and assume `useAutoLayout.ts` needs changes.

**Why it does NOT apply:** The codebase's `useAutoLayout.ts` (actually `computeForceLayout`) uses d3-force directly. It never reads `node.width` or `node.height` from React Flow nodes. It receives plain `{ id, x, y, pinned }` objects and outputs `{ x, y }` positions. The `node.measured` breaking change only affects code that reads dimensions from React Flow's internal node representation.

**How to verify:** Search `useAutoLayout.ts` for `node.width`, `node.height`, `measured` -- none exist. The function signature takes `AutoLayoutNode[]` which is a custom interface with only `id`, `x`, `y`, `pinned`.

### Pitfall 5: Tailwind v4 Important Modifier Syntax Change

**What goes wrong:** The codebase uses `!bg-[#8899a6]` syntax (exclamation at the start). Tailwind v4 moves the important modifier to the end: `bg-[#8899a6]!`.

**Why it happens:** Breaking syntax change in Tailwind v4.

**How to avoid:** The official codemod (`npx @tailwindcss/upgrade`) handles this automatically. Verify after running by searching for `!` at the start of class names.

### Pitfall 6: Tailwind v4 Shadow Utility Renames

**What goes wrong:** Tailwind v4 renamed `shadow-sm` to `shadow-xs`, `shadow` to `shadow-sm`, etc. Existing shadow classes break silently (they generate no CSS).

**How to avoid:** The codemod handles this. Verify by checking the Tailwind v4 upgrade guide rename table.

### Pitfall 7: Font Loading CLS on Canvas Nodes

**What goes wrong:** Variable fonts load after React Flow measures node dimensions. When the font arrives and text reflows, node dimensions are stale, causing misaligned edges and overlapping nodes.

**How to avoid:** Fontsource variable fonts are bundled by Vite and served from the same origin. They typically load within the first paint cycle. However, to be safe:
1. Add `<link rel="preload">` hints for the most critical font weights in `index.html`
2. Use `font-display: swap` (Fontsource default)
3. React Flow v12's `node.measured` system will re-measure after font load, auto-correcting layouts

## Hardcoded Hex Audit

Complete inventory of every hex color reference in non-test source files that must be replaced:

### File: `src/components/Canvas.tsx` (10 occurrences)

| Line | Current Value | Token Reference | Context |
|------|--------------|-----------------|---------|
| 228 | `'#00c853'` | `'var(--color-status-up)'` | Status color function return |
| 230 | `'#ff1744'` | `'var(--color-status-down)'` | Status color function return |
| 232 | `'#ffc107'` | `'var(--color-status-probing)'` | Status color function return |
| 234 | `'#657786'` | `'var(--color-status-unknown)'` | Status color function return |
| 1259 | `stroke: '#4a4a5e'` | `stroke: 'var(--color-outline)'` | Connection line style |
| 1263 | `color="#3f3f53"` | `color="var(--nt-outline)"` | Background dots color |
| 1269 | `'#ff1744'` | `'var(--color-status-down)'` | MiniMap alert color |
| 1270 | `'#ffc107'` | `'var(--color-status-probing)'` | MiniMap alert color |
| 1274 | `'#363647'` | `'var(--nt-surface)'` | MiniMap background |
| 1275 | `'#4a4a5e'` | `'var(--color-outline)'` | MiniMap border |

### File: `src/components/LinkEdge.tsx` (15 occurrences)

All hex values in this file are status/utilization colors returned from conditional logic. Replace with `var(--color-status-*)` and `var(--color-outline)` references.

### File: `src/components/DeviceCard.tsx` (4 occurrences)

| Line | Current Value | Token Reference | Context |
|------|--------------|-----------------|---------|
| 24 | `!bg-[#8899a6]` | `bg-on-bg-secondary` (after removing `!` prefix) | Handle color |
| 133 | `bg-[#1a1a24]` | `bg-surface` | Card header background |
| 149 | `bg-[#12121a]` | `bg-bg` | Card body background |
| 219 | `bg-[#8899a6]/50` | `bg-on-bg-muted/50` | Placeholder handle |

### File: `src/components/icons/DeviceIcon.tsx` (7 occurrences)

All SVG `fill` and `stroke` attributes use `#2d2d3d` (the old canvas background). Replace with `currentColor` or `var(--nt-bg)`.

### File: `src/types/metrics.ts` (3 occurrences)

| Line | Current Value | Token Reference |
|------|--------------|-----------------|
| 232 | `'#ff1744'` | `'var(--color-status-down)'` |
| 235 | `'#ffc107'` | `'var(--color-status-probing)'` |
| 237 | `'#00c853'` | `'var(--color-status-up)'` |

### File: `src/components/InterfaceStatsPanel.tsx` (1 occurrence)

| Line | Current Value | Token Reference |
|------|--------------|-----------------|
| 28 | `'#657786'` | `'var(--color-status-unknown)'` |

### File: `src/components/LinkDetailsPanel.tsx` (1 occurrence)

| Line | Current Value | Token Reference |
|------|--------------|-----------------|
| 63 | `color: '#666'` | `color: 'var(--nt-on-bg-muted)'` |

### File: `src/components/LinkCreatePanel.tsx` (1 occurrence)

| Line | Current Value | Token Reference |
|------|--------------|-----------------|
| 209 | `color: '#666'` | `color: 'var(--nt-on-bg-muted)'` |

### File: `src/index.css` (3 occurrences)

| Line | Current Value | Token Reference |
|------|--------------|-----------------|
| 17 | `#2d2d3d` | `var(--nt-bg)` |
| 20 | `#2d2d3d` | `var(--nt-bg)` |
| 21 | `#e1e8ed` | `var(--nt-on-bg)` |

**Total: 45 hex occurrences across 9 files** (8 source + 1 CSS). Also note that old status color values change per D-09 and D-10: `#00c853` becomes `#00E676`, `#ffc107` becomes `#FFEA00`, `#657786` becomes `#9E9E9E`.

## React Flow v12 Migration Specifics

### Breaking Changes Affecting This Codebase

| Change | Files Affected | Action |
|--------|---------------|--------|
| Package rename `reactflow` to `@xyflow/react` | `App.tsx`, `Canvas.tsx`, `DeviceCard.tsx`, `LinkEdge.tsx`, `DeviceCard.test.tsx`, `main.tsx` | Find-replace imports |
| Default export removed (`import ReactFlow from 'reactflow'` to `import { ReactFlow } from '@xyflow/react'`) | `Canvas.tsx` | Already uses named import -- no change needed |
| Style import path change | `main.tsx` | Move to `index.css` within `@layer base` |
| `colorMode` prop (new) | `Canvas.tsx` | Add `colorMode={resolvedTheme}` prop |

### Breaking Changes NOT Affecting This Codebase

| Change | Why Not Affected |
|--------|-----------------|
| `node.measured.width`/`height` | `useAutoLayout.ts` uses d3-force with custom types, never reads React Flow node dimensions |
| `parentNode` to `parentId` | Codebase does not use parent/child node grouping |
| `xPos`/`yPos` to `positionAbsoluteX`/`positionAbsoluteY` | Not referenced anywhere in codebase |
| `onEdgeUpdate` to `onReconnect` | Not used in Canvas.tsx |
| Immutable update requirement | Canvas.tsx already uses spread operators for node/edge updates |
| `nodeInternals` to `nodeLookup` | Not referenced in codebase |

### Import Mapping

```
// Before (6 files)
import { ... } from 'reactflow';
import 'reactflow/dist/style.css';

// After
import { ... } from '@xyflow/react';
// Style import moves to index.css: @import "@xyflow/react/dist/style.css" inside @layer base
```

## Code Examples

### Vite Config Update

```typescript
// frontend/vite.config.ts
import { defineConfig, loadEnv } from 'vite';
import react from '@vitejs/plugin-react';
import tailwindcss from '@tailwindcss/vite';

export default defineConfig(({ mode }) => {
  const env = loadEnv(mode, process.cwd(), '');
  const apiTarget = env.VITE_API_URL || 'http://backend:8080';
  const wsTarget = apiTarget.replace(/^http/i, 'ws');

  return {
    plugins: [react(), tailwindcss()],
    server: {
      host: '0.0.0.0',
      port: 3000,
      proxy: {
        '/api/v1/ws': { target: wsTarget, changeOrigin: true, ws: true },
        '/api': { target: apiTarget, changeOrigin: true, ws: true },
      },
    },
  };
});
```

### NavBar Theme Toggle (D-01, D-02)

```tsx
// In NavBar.tsx -- add theme toggle button to the right side
import { useTheme } from '../contexts/ThemeContext';

export function NavBar({ activeView, onViewChange }: NavBarProps) {
  const { resolvedTheme, setTheme } = useTheme();
  // ...

  return (
    <div className="fixed top-0 left-0 right-0 z-30 flex h-10 items-center border-b border-outline bg-surface/90 px-4 backdrop-blur-xl">
      {/* ... existing brand + tabs ... */}
      <div className="ml-auto">
        <button
          onClick={() => setTheme(resolvedTheme === 'dark' ? 'light' : 'dark')}
          className="rounded-md p-1.5 text-on-bg-secondary hover:text-on-bg hover:bg-surface-high transition-colors"
          aria-label={resolvedTheme === 'dark' ? 'Switch to light theme' : 'Switch to dark theme'}
        >
          {resolvedTheme === 'dark' ? (
            <svg /* moon icon */ width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2">
              <path d="M21 12.79A9 9 0 1 1 11.21 3 7 7 0 0 0 21 12.79z" />
            </svg>
          ) : (
            <svg /* sun icon */ width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2">
              <circle cx="12" cy="12" r="5" /><line x1="12" y1="1" x2="12" y2="3" /><line x1="12" y1="21" x2="12" y2="23" /><line x1="4.22" y1="4.22" x2="5.64" y2="5.64" /><line x1="18.36" y1="18.36" x2="19.78" y2="19.78" /><line x1="1" y1="12" x2="3" y2="12" /><line x1="21" y1="12" x2="23" y2="12" /><line x1="4.22" y1="19.78" x2="5.64" y2="18.36" /><line x1="18.36" y1="5.64" x2="19.78" y2="4.22" />
            </svg>
          )}
        </button>
      </div>
    </div>
  );
}
```

### Status Color Lookup Pattern (replacing hardcoded hex)

```typescript
// types/metrics.ts or a shared constants file
export const STATUS_COLORS = {
  up: 'var(--color-status-up)',
  down: 'var(--color-status-down)',
  probing: 'var(--color-status-probing)',
  unknown: 'var(--color-status-unknown)',
} as const;

export function metricColor(status: string): string {
  if (status === 'down' || status === 'critical') return STATUS_COLORS.down;
  if (status === 'probing' || status === 'warning') return STATUS_COLORS.probing;
  return STATUS_COLORS.up;
}
```

## State of the Art

| Old Approach | Current Approach | When Changed | Impact |
|--------------|------------------|--------------|--------|
| Tailwind `darkMode: 'class'` in JS config | `@custom-variant dark` in CSS | Tailwind v4 (Jan 2025) | Config moves entirely to CSS; `tailwind.config.js` deleted |
| `@tailwind base/components/utilities` | `@import "tailwindcss"` | Tailwind v4 | Single import replaces three directives |
| PostCSS plugin chain | `@tailwindcss/vite` plugin | Tailwind v4 | Faster HMR, autoprefixer built-in |
| `reactflow` package | `@xyflow/react` | React Flow v12 (Jul 2024) | Package rename, CSS variable theming, colorMode prop |
| `shadow-sm` | `shadow-xs` (renamed) | Tailwind v4 | Entire shadow scale shifted; codemod handles |
| `!important` prefix `!flex` | Suffix `flex!` | Tailwind v4 | Codemod handles |

## Open Questions

1. **Tailwind v4 codemod behavior with existing custom colors**
   - What we know: The codemod handles ~90% of migration including config-to-CSS conversion
   - What's unclear: How the codemod handles the existing custom `colors` object in `tailwind.config.js` -- does it auto-generate `@theme` entries, or does it leave them for manual migration?
   - Recommendation: Run the codemod first, inspect the diff, then manually adjust the `@theme` block. The custom colors need to be replaced anyway (old palette to Neon Topography palette).

2. **Fontsource variable font `@font-face` declarations and `font-display`**
   - What we know: Fontsource provides `@font-face` via CSS import; variable fonts use a single file
   - What's unclear: Whether Fontsource's default `font-display` is `swap` (desired) or `auto`
   - Recommendation: After installing, inspect the imported CSS to verify `font-display: swap`. If `auto`, override in `index.css`.

3. **React Flow v12 `colorMode` interaction with `data-theme` attribute**
   - What we know: React Flow v12 adds a `.dark` or `.light` class to its wrapper based on `colorMode` prop
   - What's unclear: Whether this class conflicts with or complements the `data-theme` attribute on `<html>`
   - Recommendation: Both should work independently. The `@custom-variant dark` uses `data-theme` attribute, not a class. React Flow's `.dark` class only affects `--xy-*` variables within the `.react-flow` scope. Verify after implementation.

## Environment Availability

| Dependency | Required By | Available | Version | Fallback |
|------------|------------|-----------|---------|----------|
| Node.js | Tailwind v4 codemod (requires 20+) | Yes | 24.13.1 | -- |
| npm | Package installation | Yes | 11.8.0 | -- |
| Vite | Build system | Yes (devDependency) | 7.0.6 | -- |

**Missing dependencies with no fallback:** None -- all required tools are available.

## Validation Architecture

### Test Framework

| Property | Value |
|----------|-------|
| Framework | Vitest 4.1 + @testing-library/react 16.3 |
| Config file | `frontend/vitest.config.ts` |
| Quick run command | `cd frontend && npx vitest run` |
| Full suite command | `cd frontend && npx vitest run` |

### Phase Requirements to Test Map

| Req ID | Behavior | Test Type | Automated Command | File Exists? |
|--------|----------|-----------|-------------------|-------------|
| FOUND-01 | CSS tokens define semantic colors mapped per theme | unit | `cd frontend && npx vitest run src/contexts/ThemeContext.test.tsx -t "token"` | No -- Wave 0 |
| FOUND-02 | Tailwind v4 migration -- app builds and renders | smoke | `cd frontend && npx tsc -b && npx vite build` | N/A (build command) |
| FOUND-03 | React Flow v12 -- canvas renders with new package | unit | `cd frontend && npx vitest run src/components/DeviceCard.test.tsx` | Yes (update imports) |
| FOUND-04 | Outfit font loaded | manual-only | Visual check: inspect computed `font-family` in DevTools | N/A |
| FOUND-05 | JetBrains Mono font loaded | manual-only | Visual check: inspect computed `font-family` on metric values | N/A |
| FOUND-06 | No hardcoded hex in components | unit | `grep -rn "bg-\[#\|text-\[#\|border-\[#" frontend/src/ --include='*.tsx'` returns 0 | N/A (grep check) |
| THEME-01 | Toggle switches between dark/light | unit | `cd frontend && npx vitest run src/contexts/ThemeContext.test.tsx -t "toggle"` | No -- Wave 0 |
| THEME-02 | Preference persists in localStorage | unit | `cd frontend && npx vitest run src/contexts/ThemeContext.test.tsx -t "persist"` | No -- Wave 0 |
| THEME-03 | Defaults to OS preference | unit | `cd frontend && npx vitest run src/contexts/ThemeContext.test.tsx -t "system"` | No -- Wave 0 |
| THEME-04 | No FOWT on page load | manual-only | Hard refresh in light mode: no dark flash visible | N/A |

### Sampling Rate

- **Per task commit:** `cd frontend && npx vitest run` (all 47 existing tests + new theme tests)
- **Per wave merge:** `cd frontend && npx tsc -b && npx vitest run` (type check + test suite)
- **Phase gate:** Full suite green + `tsc -b` clean + visual verification of both themes

### Wave 0 Gaps

- [ ] `frontend/src/contexts/ThemeContext.test.tsx` -- unit tests for ThemeProvider (toggle, persist, system preference detection)
- [ ] Update `frontend/src/components/DeviceCard.test.tsx` -- fix imports from `reactflow` to `@xyflow/react`
- [ ] `cd frontend && npx tsc -b` -- verify TypeScript compilation succeeds after all import changes

## Project Constraints (from CLAUDE.md)

The following directives from CLAUDE.md apply to this phase:

- **Naming:** TypeScript components in `PascalCase.tsx`, hooks in `camelCase.ts` with `use` prefix
- **Naming:** New context file should be `ThemeContext.tsx` in a `contexts/` directory (following established `hooks/` pattern)
- **Imports:** No `@/` path aliases; all imports use relative paths
- **Imports:** `import type` syntax for type-only imports
- **Module design:** Named exports for hooks/utilities; default export only for primary React components
- **Code style:** Tailwind CSS for all styling, no separate CSS modules
- **Code style:** Single quotes for string literals; trailing commas in multi-line objects
- **Testing:** Test files co-located with source as `*.test.ts` / `*.test.tsx`
- **Testing:** `vitest run` for test execution
- **Build:** `tsc -b && vite build` for production build
- **GSD workflow:** All work through GSD commands, not direct repo edits

## Sources

### Primary (HIGH confidence)
- [Tailwind CSS v4 upgrade guide](https://tailwindcss.com/docs/upgrade-guide) -- migration steps, codemod, breaking changes
- [Tailwind CSS v4 `@theme` directive docs](https://tailwindcss.com/docs/theme) -- token namespaces, CSS variable generation, `@theme inline`
- [Tailwind CSS v4 dark mode docs](https://tailwindcss.com/docs/dark-mode) -- `@custom-variant` configuration
- [React Flow v12 migration guide](https://reactflow.dev/learn/troubleshooting/migrate-to-v12) -- breaking changes, `node.measured`, package rename
- [React Flow v12 theming docs](https://reactflow.dev/learn/customization/theming) -- CSS variable overrides, `colorMode` prop, complete `--xy-*` variable list
- [React Flow v12 dark mode example](https://reactflow.dev/examples/styling/dark-mode) -- `colorMode` prop usage
- [React Flow ColorMode type reference](https://reactflow.dev/api-reference/types/color-mode) -- prop type definition
- [Fontsource: Outfit variable](https://fontsource.org/fonts/outfit/install) -- installation, variable font support
- [Fontsource: JetBrains Mono variable](https://fontsource.org/fonts/jetbrains-mono/install) -- installation
- npm registry: `tailwindcss@4.2.2`, `@tailwindcss/vite@4.2.2`, `@xyflow/react@12.10.1`, `@fontsource-variable/outfit@5.2.8`, `@fontsource-variable/jetbrains-mono@5.2.8` -- versions verified 2026-03-25
- Codebase analysis: `frontend/tailwind.config.js`, `frontend/postcss.config.js`, `frontend/vite.config.ts`, `frontend/index.html`, `frontend/src/index.css`, `frontend/src/main.tsx`, `frontend/src/App.tsx`, `frontend/src/hooks/useAutoLayout.ts`

### Secondary (MEDIUM confidence)
- [Tailwind v4 theming patterns (Medium)](https://medium.com/@sir.raminyavari/theming-in-tailwind-css-v4-support-multiple-color-schemes-and-dark-mode-ba97aead5c14) -- semantic token pattern
- [Tailwind CSS v4 CSS variable dark mode discussion](https://github.com/tailwindlabs/tailwindcss/discussions/15083) -- recommended `@layer theme` + `@variant dark` pattern
- [React Flow Tailwind CSS 4 compatibility](https://reactflow.dev/whats-new/2025-10-28) -- style import location change for v4
- [Flash of Unstyled Dark Theme prevention](https://webcloud.se/blog/2020-04-06-flash-of-unstyled-dark-theme/) -- inline-script pattern
- Codebase grep audit: 45 hardcoded hex occurrences across 9 files (verified via `grep -rn`)

### Tertiary (LOW confidence)
- None -- all findings verified against primary or secondary sources

## Metadata

**Confidence breakdown:**
- Standard stack: HIGH -- all package versions verified on npm, migration guides are official and authoritative
- Architecture: HIGH -- CSS variable + `@theme inline` pattern verified against Tailwind v4 docs and community discussions; ThemeProvider pattern is well-established
- Migration scope: HIGH -- React Flow v12 breaking changes cross-referenced against actual codebase imports; `useAutoLayout.ts` confirmed not affected by `node.measured` change
- Hardcoded hex audit: HIGH -- grep audit of entire source tree, every file and line documented
- Pitfalls: HIGH -- each pitfall verified against official docs or confirmed via codebase inspection

**Research date:** 2026-03-25
**Valid until:** 2026-04-25 (stable libraries, low change rate)

---
*Phase: 01-design-token-foundation-and-theme-infrastructure*
*Research completed: 2026-03-25*
