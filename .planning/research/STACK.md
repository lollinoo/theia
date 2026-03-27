# Technology Stack: v1.3.0 Frontend Redesign

**Project:** MikroTik Theia -- Neon Topography Design System
**Researched:** 2026-03-25
**Scope:** Stack additions/changes for design system, dark/light theming, font loading, glassmorphism, and area-based navigation. Backend and graph visualization stack are already validated and not re-researched here.

## Executive Summary

The redesign requires three categories of stack changes:

1. **Upgrade Tailwind CSS 3.4 to 4.x** -- Native CSS custom properties via `@theme`, first-party Vite plugin replaces PostCSS pipeline, `@custom-variant` for theme switching. This is the single highest-impact change.
2. **Upgrade React Flow 11 to 12** -- Package rename from `reactflow` to `@xyflow/react`, built-in dark mode via `colorMode` prop, CSS variable-based theming that aligns with the design system.
3. **Add font loading** -- Self-hosted Outfit + JetBrains Mono via Fontsource variable font packages. No Google Fonts CDN dependency.

No new state management, routing, or UI component libraries are needed. The design system is implemented with CSS custom properties and Tailwind utilities -- not a component library.

---

## Recommended Stack Changes

### Tailwind CSS 3.4 -> 4.x (Upgrade)

| Technology | Version | Purpose | Why | Confidence |
|------------|---------|---------|-----|------------|
| tailwindcss | 4.2.2 | Utility-first CSS framework | v4's `@theme` directive generates CSS custom properties AND utility classes from a single declaration. Design tokens defined once, consumed everywhere. Native cascade layers eliminate specificity wars. The Neon Topography color system maps directly to `@theme` namespaces. | HIGH |
| @tailwindcss/vite | 4.2.2 | Vite integration plugin | Replaces the PostCSS plugin chain (`tailwindcss` + `autoprefixer`). Connects directly to Vite's build pipeline for faster HMR and lower overhead. Vendor prefixing handled automatically -- `autoprefixer` is no longer needed. | HIGH |

**What changes:**
- Remove: `tailwindcss` (as PostCSS plugin), `autoprefixer`, `postcss.config.js`
- Remove: `tailwind.config.js` -- all config moves to CSS via `@theme` and `@custom-variant`
- Add: `@tailwindcss/vite` as a Vite plugin in `vite.config.ts`
- Replace: `@tailwind base/components/utilities` directives with `@import "tailwindcss"`

**Migration path:** Run `npx @tailwindcss/upgrade` (requires Node.js 20+). The codemod handles ~90% of utility class renames and config migration. Manual review needed for custom color tokens and the `darkMode: 'class'` config (becomes `@custom-variant` in CSS).

**Why upgrade instead of staying on v3:** Tailwind v4's `@theme` directive is the design system's backbone. It generates CSS custom properties (e.g., `--color-primary-glow`) that are available both as Tailwind utilities (`bg-primary-glow`) AND raw CSS variables (`var(--color-primary-glow)`). This dual access is essential for: (a) Tailwind utility classes in components, (b) CSS variables for React Flow node styling, (c) runtime theme switching by overriding variables per `[data-theme]` selector. Staying on v3 would require maintaining a parallel CSS custom property system outside of Tailwind, doubling the token surface area.

**Browser compatibility:** Tailwind v4 requires Safari 16.4+, Chrome 111+, Firefox 128+. This is a network admin tool, not a public website -- modern browser support is acceptable.

### React Flow 11 -> 12 (Upgrade)

| Technology | Version | Purpose | Why | Confidence |
|------------|---------|---------|-----|------------|
| @xyflow/react | 12.10.1 | Network topology canvas | v12 adds built-in `colorMode` prop ("light" / "dark" / "system") that adds a `.dark` class to the flow container. All internal styles use CSS variables (`--xy-*` namespace) that can be overridden to match Neon Topography tokens. Package renamed from `reactflow` to `@xyflow/react`. | HIGH |

**What changes:**
- Replace: `reactflow` package with `@xyflow/react`
- Update: all imports from `reactflow` to `@xyflow/react`
- Update: style import path
- Add: `colorMode` prop to `<ReactFlow>` component, driven by theme context
- Override: `--xy-*` CSS variables to map to Neon Topography design tokens (e.g., `--xy-background-color` -> `var(--color-surface)`)
- Note: `node.measured.width` / `node.measured.height` replaces direct dimension access (affects layout code in `useAutoLayout.ts`)

**Migration scope:** Moderate. The codebase has ~30 component files. Import path changes are mechanical. The main risk is the `node.measured` change affecting the d3-force layout hook.

### Font Loading (New)

| Technology | Version | Purpose | Why | Confidence |
|------------|---------|---------|-----|------------|
| @fontsource-variable/outfit | 5.2.8 | Display and headlines font (variable weight) | Self-hosted, no CDN dependency, variable font = single file for all weights (400-700). Eliminates extra DNS lookup + TCP connection that Google Fonts would add. Vite bundles the font files with hashed names for cache-busting. | HIGH |
| @fontsource-variable/jetbrains-mono | 5.2.8 | Technical readout font (variable weight) | Same rationale. JetBrains Mono is used for all numerical data, status readouts, and monospace tags in the Neon Topography spec. Variable font covers regular through bold weights in one file. | HIGH |

**Why Fontsource variable packages over static weights:**
- Variable fonts serve one file for the entire weight axis (400-700) instead of four separate files
- Smaller total payload: ~80KB for both variable fonts vs ~200KB for eight static weight files
- Fontsource provides CSS `@font-face` declarations -- just `import '@fontsource-variable/outfit'` in the entry point
- No FOIT/FOUT management needed -- fonts are bundled by Vite and served from the same origin

**Why NOT Google Fonts CDN:**
- Adds external dependency for a self-hosted network monitoring tool (which may run air-gapped)
- Extra DNS resolution + TLS handshake adds 100-300ms to first paint
- Font files cached per-origin by browsers, so CDN "shared cache" benefit no longer applies (partitioned caches since Chrome 86)

### Theme Infrastructure (New -- No Additional Libraries)

| Technology | Version | Purpose | Why | Confidence |
|------------|---------|---------|-----|------------|
| CSS Custom Properties | native | Design token layer | The entire Neon Topography color system is expressed as CSS custom properties, overridden per `[data-theme]` selector. No runtime JS needed for color switching -- the browser's CSS engine handles it. | HIGH |
| React Context (built-in) | React 18 | Theme state provider | A small `ThemeProvider` context manages the active theme ("dark" / "light" / "system"), persists to `localStorage`, and sets `data-theme` attribute on `<html>`. No external theme library needed -- this is ~40 lines of code. | HIGH |
| @custom-variant (Tailwind v4) | native | Dark mode variant | Replaces `darkMode: 'class'` from tailwind.config.js. Configured in CSS: `@custom-variant dark (&:where([data-theme=dark], [data-theme=dark] *))`. Enables `dark:bg-surface` syntax in templates. | HIGH |

**Why NOT a theme library (next-themes, use-dark-mode, etc.):** These libraries solve framework-specific problems (Next.js SSR flash, etc.) that don't apply to a Vite SPA. The theme toggle is trivial: read `localStorage`, apply `data-theme` attribute, listen for `prefers-color-scheme` changes. Adding a library for this introduces an unnecessary dependency.

**Architecture for theme switching:**
```
ThemeProvider (React Context)
  |-- reads: localStorage("theme") || "system"
  |-- sets: document.documentElement.dataset.theme = resolved
  |-- provides: { theme, setTheme, resolvedTheme }
  |
  v
CSS Custom Properties (cascade)
  |-- :root / [data-theme="dark"]  { --color-surface: #161618; ... }
  |-- [data-theme="light"]         { --color-surface: #F5F5F7; ... }
  |
  v
@theme (Tailwind v4)
  |-- Maps: --color-surface -> bg-surface utility
  |-- Maps: --color-primary-glow -> text-primary-glow utility
  |
  v
Components use Tailwind utilities: bg-surface, text-on-surface, etc.
React Flow reads CSS variables: --xy-background-color: var(--color-surface)
```

### Glassmorphism Support (No Additional Libraries)

| Technology | Version | Purpose | Why | Confidence |
|------------|---------|---------|-----|------------|
| CSS backdrop-filter | native | Frosted glass / translucency effects | ~95% global browser support (Chrome 76+, Safari 9+, Firefox 103+, Edge 79+). Tailwind v4 ships `backdrop-blur-*` utilities out of the box. The Neon Topography spec calls for `rgba(255, 255, 255, 0.02)` backgrounds with `backdrop-blur` -- this is pure CSS, no library needed. | HIGH |

**Implementation notes:**
- The `-webkit-backdrop-filter` prefix is still needed for Safari. Tailwind v4 handles vendor prefixing automatically (autoprefixer is built-in).
- The "bloom" effect (large-radius radial blurs behind content) uses CSS `radial-gradient` + `filter: blur(80px)` on pseudo-elements, not `backdrop-filter`. These are decorative layers, not glass effects.
- Performance consideration: `backdrop-filter` triggers compositing. Limit to 3-5 elements on screen (navigation pill, area cards, modals). Do NOT apply to every panel.

---

## Design Token Architecture (CSS-Only)

The Neon Topography design system maps to Tailwind v4 `@theme` as follows:

```css
@import "tailwindcss";

/* Theme switching via data attribute */
@custom-variant dark (&:where([data-theme=dark], [data-theme=dark] *));

/* Register semantic tokens as Tailwind theme variables */
@theme {
  /* These reference CSS variables that change per-theme */
  --color-surface: var(--nt-surface);
  --color-surface-container: var(--nt-surface-container);
  --color-surface-container-high: var(--nt-surface-container-high);
  --color-on-surface: var(--nt-on-surface);
  --color-on-surface-secondary: var(--nt-on-surface-secondary);
  --color-outline: var(--nt-outline);

  /* Functional colors (same in both themes) */
  --color-primary-glow: #00E676;
  --color-area-blue: #2979FF;
  --color-area-purple: #E040FB;
  --color-warning: #FFEA00;
  --color-critical: #FF1744;

  /* Font families */
  --font-display: "Outfit Variable", "Outfit", sans-serif;
  --font-mono: "JetBrains Mono Variable", "JetBrains Mono", monospace;

  /* Elevation shadows */
  --shadow-panel: 0 24px 48px rgba(0, 0, 0, 0.2);
  --shadow-pill: 0 24px 48px rgba(0, 0, 0, 0.5);

  /* Border radii */
  --radius-pill: 9999px;
  --radius-panel: 12px;
}

/* Dark theme (default) */
:root, [data-theme="dark"] {
  --nt-surface: #161618;
  --nt-surface-container: #1E1E21;
  --nt-surface-container-high: #262629;
  --nt-on-surface: #F5F5F7;
  --nt-on-surface-secondary: #8899A6;
  --nt-outline: #333338;
}

/* Light theme */
[data-theme="light"] {
  --nt-surface: #F5F5F7;
  --nt-surface-container: #EAEAEC;
  --nt-surface-container-high: #DEDEE1;
  --nt-on-surface: #161618;
  --nt-on-surface-secondary: #5A5A6E;
  --nt-outline: #D0D0D8;
}
```

This architecture means:
- `bg-surface` in a component always resolves to the correct theme color
- No `dark:` prefix needed for theme-aware colors (the variable handles it)
- `dark:` prefix is still available for one-off overrides
- React Flow's `--xy-*` variables can reference the same tokens

---

## What Does NOT Change

These are already in the stack and validated. No modifications needed for v1.3.0:

| Technology | Current Version | Status |
|------------|----------------|--------|
| React | 18.3 | Keep. React 19 upgrade is independent of the design system work. |
| TypeScript | 5.7 | Keep. No version-specific features needed. |
| Vite | 7.0 | Keep. Compatible with `@tailwindcss/vite` 4.2.x. |
| d3-force | 3.0 | Keep. Layout algorithm is theme-independent. |
| Native WebSocket | browser API | Keep. Real-time metrics pipeline is unchanged. |
| Go backend | 1.24 | Keep. Area CRUD is standard REST, no new backend dependencies. |
| SQLite | mattn/go-sqlite3 | Keep. Areas table is a simple migration. |
| Vitest | 4.1 | Keep. Test runner is style-independent. |

**Note on React 18 vs 19:** The project currently uses React 18.3. Upgrading to React 19 during the design system work adds unnecessary risk. React 19's concurrent features (use, Actions, etc.) are not needed for theming. Keep the React upgrade as a separate effort.

---

## Alternatives Considered

| Category | Recommended | Alternative | Why Not |
|----------|-------------|-------------|---------|
| Theming | CSS custom properties + `@theme` | CSS-in-JS (styled-components, Emotion) | Runtime CSS generation is dying; Tailwind is zero-runtime. Ecosystem is moving away from CSS-in-JS. |
| Theming | CSS custom properties + `@theme` | Tailwind `dark:` prefix only | Would require `dark:` on every color utility in every component. Semantic tokens via CSS variables mean components use `bg-surface` once, and the theme resolves the color. |
| Theme state | React Context (built-in) | next-themes | next-themes solves SSR flash -- irrelevant for a Vite SPA. Adds a dependency for 40 lines of code. |
| Theme state | React Context (built-in) | Zustand theme slice | Overkill. Theme state is a single string ("dark" / "light" / "system") consumed by one place (the `<html>` attribute). React Context with `localStorage` is sufficient. |
| Fonts | Fontsource (self-hosted, npm) | Google Fonts CDN | External dependency unsuitable for air-gapped deployments. Performance penalty from cross-origin font loading. Partitioned browser caches negate CDN sharing benefit. |
| Fonts | Variable font packages | Static weight packages | Variable = 1 file per font family for all weights. Static = 1 file per weight per font. Variable is smaller total size and simpler to manage. |
| Tailwind version | v4 (upgrade) | Stay on v3.4 | v3 can do theming with CSS variables, but requires maintaining a parallel system outside of Tailwind config. v4's `@theme` unifies tokens and utilities. The migration is automated and the codebase is small (~30 components). |
| Component library | None (Tailwind utilities) | shadcn/ui | The Neon Topography design system has a highly custom aesthetic (glassmorphism, bloom effects, no-line rule). Pre-built component libraries fight this rather than help. The component count is small (~15 unique UI patterns). |
| Component library | None (Tailwind utilities) | Radix UI primitives | Considered in initial research, but the app has very few interaction patterns needing accessibility primitives (one dropdown menu, one modal). Not worth the dependency for 2 components. Copy the pattern, don't install the library. |
| CSS approach | Tailwind v4 | CSS Modules | Slower iteration, no design token integration, no utility-first composition in markup. Would require a separate token system. |

---

## Installation (Delta from Current)

```bash
cd /home/azmin/projects/theia/frontend

# Remove deprecated packages
npm uninstall tailwindcss autoprefixer postcss reactflow

# Install Tailwind v4 with Vite plugin
npm install tailwindcss@^4.2.2 @tailwindcss/vite@^4.2.2

# Install React Flow v12
npm install @xyflow/react@^12.10.1

# Install fonts (self-hosted variable fonts)
npm install @fontsource-variable/outfit@^5.2.8 @fontsource-variable/jetbrains-mono@^5.2.8

# PostCSS is still needed by Vite internally but postcss.config.js is removed
# autoprefixer is handled by Tailwind v4 automatically
```

**Files to delete after migration:**
- `frontend/tailwind.config.js` -- config moves to CSS `@theme`
- `frontend/postcss.config.js` -- Vite plugin replaces PostCSS chain

**Files to modify:**
- `frontend/vite.config.ts` -- add `@tailwindcss/vite` plugin
- `frontend/src/index.css` -- replace `@tailwind` directives, add `@theme` tokens, add font imports
- `frontend/src/main.tsx` -- add Fontsource CSS imports
- All `*.tsx` components -- update color class names from old tokens to new semantic tokens

---

## Version Pinning Notes

- **tailwindcss 4.2.2**: Pin to `^4.2.2`. The v4 line is stable (released Jan 2025, now at 4.2.x).
- **@tailwindcss/vite 4.2.2**: Must match tailwindcss version. Pin to `^4.2.2`.
- **@xyflow/react 12.10.1**: Pin to `^12.10.1`. The v12 line has been stable since mid-2024 with regular patch releases.
- **@fontsource-variable/* 5.2.8**: Pin to `^5.2.8`. Fontsource tracks upstream font releases; patches are non-breaking.

---

## Migration Risk Assessment

| Change | Risk | Mitigation |
|--------|------|------------|
| Tailwind v3 -> v4 | MEDIUM | Automated codemod handles ~90%. Small codebase (~30 files). Run codemod, review diff, fix edge cases. |
| reactflow -> @xyflow/react | MEDIUM | Import renames are mechanical. Main risk: `node.measured` change affects `useAutoLayout.ts` layout hook. Test layout behavior after migration. |
| New color tokens | LOW | Mechanical find-and-replace from old tokens (`bg-canvas`, `text-primary`) to new semantic tokens (`bg-surface`, `text-on-surface`). |
| Font swap (IBM Plex Sans -> Outfit) | LOW | CSS-only change in `@theme` font-family declaration. No component changes needed. |
| Glassmorphism effects | LOW | Pure CSS. Progressive enhancement -- falls back to solid background on unsupported browsers (effectively none in 2026). |
| Theme toggle (dark/light) | LOW | New feature, not a migration. ~40 lines for ThemeProvider context + CSS variable overrides per theme. |

---

## Sources

- [Tailwind CSS v4.0 release announcement](https://tailwindcss.com/blog/tailwindcss-v4) -- Architecture overview, Vite plugin, performance
- [Tailwind CSS v4 upgrade guide](https://tailwindcss.com/docs/upgrade-guide) -- Migration steps, codemod tool, breaking changes
- [Tailwind CSS v4 @theme directive docs](https://tailwindcss.com/docs/theme) -- Token namespaces, CSS variable generation
- [Tailwind CSS dark mode docs](https://tailwindcss.com/docs/dark-mode) -- `@custom-variant` configuration, class/data-attribute strategies
- [Tailwind CSS v4 theming patterns (Medium)](https://medium.com/@sir.raminyavari/theming-in-tailwind-css-v4-support-multiple-color-schemes-and-dark-mode-ba97aead5c14) -- Semantic token pattern with `@theme` + CSS variables
- [React Flow v12 release notes](https://xyflow.com/blog/react-flow-12-release) -- Package rename, colorMode, CSS variables
- [React Flow theming docs](https://reactflow.dev/learn/customization/theming) -- CSS variable overrides, dark mode prop
- [React Flow v12 migration guide](https://reactflow.dev/learn/troubleshooting/migrate-to-v12) -- Breaking changes, node.measured
- [React Flow v12 Tailwind CSS 4 compatibility](https://reactflow.dev/whats-new/2025-10-28) -- Updated for Tailwind v4
- [Fontsource: Outfit variable](https://fontsource.org/fonts/outfit/install) -- Installation, variable font support
- [Fontsource: JetBrains Mono variable](https://fontsource.org/fonts/jetbrains-mono/install) -- Installation, variable font support
- [npm: tailwindcss](https://www.npmjs.com/package/tailwindcss) -- Version 4.2.2 verified
- [npm: @xyflow/react](https://www.npmjs.com/package/@xyflow/react) -- Version 12.10.1 verified
- [npm: @fontsource-variable/outfit](https://www.npmjs.com/package/@fontsource-variable/outfit) -- Version 5.2.8 verified
- [npm: @fontsource-variable/jetbrains-mono](https://www.npmjs.com/package/@fontsource-variable/jetbrains-mono) -- Version 5.2.8 verified
- [CSS backdrop-filter browser support (caniuse)](https://caniuse.com/css-backdrop-filter) -- ~95% global support
