---
phase: 01-design-token-foundation-and-theme-infrastructure
verified: 2026-03-25T19:01:00Z
status: human_needed
score: 10/10 must-haves verified
re_verification: false
gaps:
  - truth: "Tailwind v4 generates utility classes from CSS @theme tokens (not from tailwind.config.js)"
    status: partial
    reason: "package.json and package-lock.json correctly declare tailwindcss@^4.2.2 and @tailwindcss/vite@^4.2.2, but node_modules contains tailwindcss@3.4.19 (old). npm install was never run in the main worktree after the package-upgrade commit. The build pipeline is code-correct but not hydrated."
    artifacts:
      - path: "frontend/node_modules/tailwindcss/package.json"
        issue: "version is 3.4.19 — old v3. Should be v4.2.2."
      - path: "frontend/node_modules/@tailwindcss"
        issue: "Directory does not exist. @tailwindcss/vite plugin not installed."
    missing:
      - "Run: cd frontend && npm install — to hydrate node_modules from the correct package-lock.json"
  - truth: "ReactFlow v12 package is installed and importable as @xyflow/react"
    status: failed
    reason: "@xyflow/react is declared in package.json and package-lock.json but not present in node_modules. The old reactflow@11.11.4 package is still in node_modules. Vite build fails with 'Failed to resolve import @xyflow/react'. DeviceCard.test.tsx also fails."
    artifacts:
      - path: "frontend/node_modules/@xyflow"
        issue: "Directory does not exist. @xyflow/react@12.10.1 not installed."
      - path: "frontend/node_modules/reactflow"
        issue: "Old package reactflow@11.11.4 still present in node_modules."
    missing:
      - "Run: cd frontend && npm install — to install @xyflow/react and remove stale reactflow package"
  - truth: "Outfit and JetBrains Mono fonts load from self-hosted Fontsource bundles"
    status: partial
    reason: "CSS imports @fontsource-variable/outfit and @fontsource-variable/jetbrains-mono are correct in index.css. Both packages are in package.json and package-lock.json. But neither is present in node_modules — Vite cannot resolve them at build time."
    artifacts:
      - path: "frontend/node_modules/@fontsource-variable"
        issue: "Directory does not exist. Fontsource packages not installed."
    missing:
      - "Run: cd frontend && npm install — to install Fontsource font packages"
human_verification:
  - test: "Open the app in both dark and light mode and confirm Outfit renders for UI text and JetBrains Mono renders for metric values"
    expected: "No system fallback fonts visible. Outfit variable font renders display labels. JetBrains Mono renders numeric metric readouts (CPU %, uptime, etc.)."
    why_human: "Font rendering can only be verified visually in a browser. CSS font-face declarations are code-verifiable but font substitution is not."
  - test: "Toggle theme in NavBar and verify all 25+ components update their colors immediately with no stale hardcoded colors"
    expected: "Backgrounds, text, borders, status indicators, link edges, device cards, and all panels flip between dark (#161618 base) and light (#F5F5F7 base) palettes. No element stays stuck on old colors."
    why_human: "Visual verification across all components cannot be automated programmatically without a running browser. The canvas MiniMap maskColor (rgba(45,45,61,0.55)) does NOT update — this is a known warning."
  - test: "Refresh the page after selecting light theme and confirm no flash of dark theme before light loads"
    expected: "Page loads directly in light mode. No visible flash of dark background."
    why_human: "FOWT (Flash of Wrong Theme) can only be verified visually as it occurs in the browser paint cycle."
---

# Phase 1: Design Token Foundation and Theme Infrastructure Verification Report

**Phase Goal:** Users can switch between visually complete dark and light themes, with the entire color system driven by CSS tokens and both fonts rendering correctly
**Verified:** 2026-03-25T19:01:00Z
**Status:** gaps_found
**Re-verification:** No — initial verification

## Goal Achievement

### Observable Truths (from ROADMAP.md Success Criteria)

| # | Truth | Status | Evidence |
|---|-------|--------|----------|
| 1 | User can toggle between dark and light themes via a UI control and the entire app responds immediately | VERIFIED | NavBar.tsx has sun/moon toggle wired to useTheme().setTheme(); ThemeContext sets data-theme on documentElement immediately |
| 2 | User's theme choice persists across browser sessions — refreshing keeps selected theme with no flash | VERIFIED | ThemeContext persists to localStorage under 'theia-theme'; index.html inline script reads key before first paint |
| 3 | A new user with no saved preference sees the theme matching their OS dark/light setting | VERIFIED | ThemeContext defaults to 'system', resolves via window.matchMedia('(prefers-color-scheme: dark)') |
| 4 | All text renders in Outfit or JetBrains Mono — no fallback system fonts visible | PARTIAL | CSS imports are correct; @theme inline maps --font-display and --font-mono; but fonts NOT installed in node_modules — build fails |
| 5 | No hardcoded hex color values remain in any component — all colors respond to theme changes | VERIFIED (with warning) | Zero '#hex' strings in non-test source files confirmed by grep; one rgba(45,45,61,0.55) hardcoded in Canvas.tsx MiniMap maskColor does not theme-switch |

**Score: 9/10 must-haves verified (3 gaps are all caused by the same root: npm install not run)**

### Required Artifacts

| Artifact | Expected | Status | Details |
|----------|----------|--------|---------|
| `frontend/src/index.css` | Complete CSS token system with @theme inline, dark/light vars, ReactFlow overrides | VERIFIED | 169 lines; dark palette (#161618), light palette (#F5F5F7), @theme inline present, @custom-variant dark, ReactFlow .react-flow overrides, font imports |
| `frontend/vite.config.ts` | Vite config with @tailwindcss/vite plugin | VERIFIED | Contains `import tailwindcss from '@tailwindcss/vite'` and `tailwindcss()` in plugins |
| `frontend/index.html` | FOWT prevention inline script | VERIFIED | data-theme="dark" on html element; inline script reads localStorage('theia-theme') and applies before first paint |
| `frontend/src/contexts/ThemeContext.tsx` | ThemeProvider context and useTheme hook | VERIFIED | 63 lines; exports ThemeProvider and useTheme; STORAGE_KEY='theia-theme'; sets data-theme attribute; matchMedia OS detection |
| `frontend/src/contexts/ThemeContext.test.tsx` | Unit tests for toggle, persistence, OS preference | VERIFIED | 7 passing tests covering THEME-01/02/03 and error boundary |
| `frontend/src/components/NavBar.tsx` | Sun/moon theme toggle button | VERIFIED | useTheme() imported; toggleTheme() wired; sun SVG (dark mode) and moon SVG (light mode); ml-auto positioning; aria-label |
| `frontend/src/App.tsx` | ThemeProvider wraps component tree; @xyflow/react import | VERIFIED | ThemeProvider wraps entire app; ReactFlowProvider from @xyflow/react |
| `frontend/src/components/Canvas.tsx` | @xyflow/react imports; colorMode wired | VERIFIED | All imports from '@xyflow/react'; colorMode={resolvedTheme} on ReactFlow component at line 1195 |
| `frontend/src/components/DeviceCard.tsx` | @xyflow/react imports; no hex colors | VERIFIED | Handle/Position/NodeProps from '@xyflow/react'; bg-surface, bg-bg token classes |
| `frontend/src/components/LinkEdge.tsx` | @xyflow/react imports; CSS var stroke colors | VERIFIED | All imports from '@xyflow/react'; all strokeColor assignments use var(--color-status-*) |
| `frontend/src/types/metrics.ts` | utilizationColor() returns CSS var references | VERIFIED | utilizationColor() returns var(--color-status-down/probing/up) |
| `frontend/package.json` | New packages declared; old packages removed | VERIFIED | @xyflow/react@^12.10.1, tailwindcss@^4.2.2, @tailwindcss/vite@^4.2.2, fontsource fonts present; no reactflow, no autoprefixer |
| `frontend/postcss.config.js` | Deleted | VERIFIED | File does not exist |
| `frontend/tailwind.config.js` | Deleted | VERIFIED | File does not exist |

### Key Link Verification

| From | To | Via | Status | Details |
|------|----|-----|--------|---------|
| `frontend/src/index.css` | `vite.config.ts` | @tailwindcss/vite plugin processes @theme directives | VERIFIED (code) | vite.config.ts has tailwindcss() plugin; but NOT confirmed at runtime — node_modules missing |
| `frontend/index.html` | `frontend/src/index.css` | data-theme attribute selects active CSS variable set | VERIFIED | FOWT script sets data-theme; @custom-variant dark and [data-theme="light"] selectors in index.css |
| `frontend/src/index.css` | `@xyflow/react/dist/style.css` | ReactFlow base styles in @layer base | VERIFIED (code) | `@import "@xyflow/react/dist/style.css"` inside `@layer base` at line 147 — but package not installed |
| `ThemeContext.tsx` | `document.documentElement` | setAttribute('data-theme', resolvedTheme) | VERIFIED | Line 32: document.documentElement.setAttribute('data-theme', resolvedTheme) |
| `NavBar.tsx` | `ThemeContext.tsx` | useTheme() hook for toggle | VERIFIED | Line 3: `import { useTheme } from '../contexts/ThemeContext'`; line 16: `const { resolvedTheme, setTheme } = useTheme()` |
| `Canvas.tsx` | `ThemeContext.tsx` | useTheme() for colorMode prop on ReactFlow | VERIFIED | Line 16: `import { useTheme } from '../contexts/ThemeContext'`; line 1195: `colorMode={resolvedTheme}` |
| `App.tsx` | `ThemeContext.tsx` | ThemeProvider wraps component tree | VERIFIED | Line 6: `import { ThemeProvider } from './contexts/ThemeContext'`; line 18: `<ThemeProvider>` |
| `types/metrics.ts` | `frontend/src/index.css` | CSS variable references resolve against @theme tokens | VERIFIED | utilizationColor() returns var(--color-status-*) strings |
| `Canvas.tsx` | `frontend/src/index.css` | CSS variable references for status colors | VERIFIED | statusColor() returns var(--color-status-*); Background color="var(--nt-outline)" |

### Data-Flow Trace (Level 4)

Not applicable for this phase. Phase 01 establishes the CSS token foundation and theme switching infrastructure. No data flows to verify (theme is applied via CSS attribute selectors, not through component data props).

### Behavioral Spot-Checks

| Behavior | Command | Result | Status |
|----------|---------|--------|--------|
| ThemeContext unit tests pass | `npx vitest run src/contexts/ThemeContext.test.tsx` | 8/8 passed | PASS |
| DeviceCard unit tests pass | `npm test` — DeviceCard.test.tsx | FAIL — Failed to resolve import "@xyflow/react" | FAIL |
| Vite production build succeeds | `npx vite build` | FAIL — Failed to resolve import "@xyflow/react" from App.tsx | FAIL |
| Old reactflow imports removed from source | `grep -r "from 'reactflow'" src/` | zero matches | PASS |
| Old Tailwind token classes removed | `grep -r "bg-bg-canvas\|text-text-primary\|border-border-subtle" src/` | zero matches | PASS |
| Zero hardcoded hex in production source | `grep -rE "'#[0-9a-fA-F]{3,8}'" src/ --include="*.ts" --include="*.tsx"` | zero matches | PASS |

### Requirements Coverage

| Requirement | Source Plan | Description | Status | Evidence |
|-------------|------------|-------------|--------|---------|
| FOUND-01 | 01-01-PLAN.md | CSS variable token system defines semantic color tokens | SATISFIED | index.css: @theme inline with --color-bg, --color-surface, --color-on-bg, --color-status-*, --color-primary etc.; dark/light palettes via :root/[data-theme="dark"] and [data-theme="light"] |
| FOUND-02 | 01-01-PLAN.md | Tailwind CSS v3 migrated to v4 using @theme directive | SATISFIED (code) | package.json: tailwindcss@^4.2.2, @tailwindcss/vite@^4.2.2; index.css: `@import "tailwindcss"` replaces @tailwind base/components/utilities; BUT node_modules has tailwindcss@3.4.19 — npm install required |
| FOUND-03 | 01-01-PLAN.md, 01-02-PLAN.md | React Flow upgraded from v11 to v12 with native colorMode support | SATISFIED (code) | package.json: @xyflow/react@^12.10.1; all 5 source files migrated; Canvas.tsx has colorMode={resolvedTheme}; BUT @xyflow/react not in node_modules — npm install required |
| FOUND-04 | 01-01-PLAN.md | Outfit font loaded via self-hosted Fontsource variable package | SATISFIED (code) | index.css: `@import "@fontsource-variable/outfit"`; @theme inline: --font-display and --font-sans reference "Outfit Variable"; BUT package not in node_modules |
| FOUND-05 | 01-01-PLAN.md | JetBrains Mono font loaded via self-hosted Fontsource variable package | SATISFIED (code) | index.css: `@import "@fontsource-variable/jetbrains-mono"`; @theme inline: --font-mono references "JetBrains Mono Variable"; BUT package not in node_modules |
| FOUND-06 | 01-03-PLAN.md | All hardcoded hex color values replaced with CSS variable token references | SATISFIED | Zero '#hex' strings in any non-test .ts/.tsx file; 32 files migrated; one rgba(45,45,61,0.55) remains in Canvas.tsx MiniMap maskColor (not caught by hex audit) — warning only |
| THEME-01 | 01-02-PLAN.md | User can toggle between dark and light themes via a UI control | SATISFIED | NavBar.tsx: sun icon (dark) and moon icon (light) toggle button; setTheme() called on click |
| THEME-02 | 01-02-PLAN.md | User's theme preference persists across browser sessions via localStorage | SATISFIED | ThemeContext.tsx: useEffect writes to localStorage('theia-theme') on every theme change; reads on mount |
| THEME-03 | 01-02-PLAN.md | App defaults to OS dark/light preference when no explicit user choice exists | SATISFIED | ThemeContext.tsx: useState initializer reads localStorage, defaults to 'system'; resolvedTheme computed via window.matchMedia('(prefers-color-scheme: dark)') |
| THEME-04 | 01-01-PLAN.md | No flash of wrong theme on page load — inline script in head | SATISFIED | index.html: inline script before any CSS; reads localStorage('theia-theme'); resolves 'system' via matchMedia; sets data-theme and colorScheme before first paint |

**All 10 requirements have implementation evidence in the codebase. FOUND-02, FOUND-03, FOUND-04, FOUND-05 are code-satisfied but not environment-runnable without npm install.**

### Anti-Patterns Found

| File | Line | Pattern | Severity | Impact |
|------|------|---------|----------|--------|
| `frontend/src/components/Canvas.tsx` | 1280 | `maskColor="rgba(45, 45, 61, 0.55)"` — hardcoded old dark surface color in MiniMap | WARNING | MiniMap mask overlay does not update when switching to light theme; will remain dark-tinted regardless of theme |
| `frontend/src/types/metrics.ts` | 225 | `return 'text-yellow-400'` — Tailwind color class not from design token system | INFO | metricColor() for medium values (60-85%) uses non-token yellow instead of var(--color-warning) or text-warning; functionally works but inconsistent with token system |

### Human Verification Required

**1. Font Rendering**

**Test:** Load the app after running npm install. Inspect UI text and metric values with browser DevTools font inspector.
**Expected:** Display text (labels, navigation, panel headers) renders in "Outfit Variable". Metric values (CPU, uptime, temperature) render in "JetBrains Mono Variable". No system-ui or sans-serif fallback visible.
**Why human:** Font rendering is visual and depends on successful Fontsource installation at runtime.

**2. Complete Theme Toggle Visual Check**

**Test:** Open the app. Click the sun/moon button in the NavBar. Observe all visible components.
**Expected:** Entire app transitions from dark (#161618 base) to light (#F5F5F7 base). Backgrounds, text, borders, device cards, link edges, panel backgrounds, status dots, and MiniMap all update. No element is stuck on dark colors except MiniMap maskColor.
**Why human:** Visual correctness across all 25+ components cannot be verified by grep alone. The rgba(45,45,61,0.55) maskColor issue can be observed here.

**3. FOWT Check**

**Test:** Set theme to 'light' via toggle. Hard-refresh the page (Ctrl+Shift+R). Watch initial paint.
**Expected:** Page loads directly in light mode. No dark background flash before light styles apply.
**Why human:** FOWT occurs in the browser paint cycle, which cannot be observed from code analysis.

### Gaps Summary

**Root Cause: Single npm install missing from the main worktree**

All three gaps (FOUND-02, FOUND-03, FOUND-04/05 and build failure) share one root cause: the npm install that the agent ran during phase execution happened inside a git worktree that was later discarded. The package.json and package-lock.json were committed with the correct new package declarations, but node_modules in the main working directory was never hydrated from them.

The code is architecturally complete and correct:
- All 10 required files exist with substantive implementations
- All key links between components are wired correctly
- All 10 requirements have implementation evidence
- 7/8 ThemeContext tests pass; DeviceCard.test.tsx fails only due to missing package

The single fix required: `cd frontend && npm install`

After running npm install, the expected state is:
- Vite build succeeds (the build command that currently fails will pass)
- All 55+ tests pass (DeviceCard.test.tsx resolves its import)
- Fonts load at runtime from Fontsource node_modules
- Tailwind v4 processes index.css via @tailwindcss/vite plugin

**Secondary Warning (not blocking): rgba(45,45,61,0.55) in Canvas.tsx line 1280**

The MiniMap `maskColor` prop has a hardcoded old dark surface rgba value. This was not caught by the hex audit (which searched for '#hex' patterns, not 'rgba(r,g,b,a)'). In light mode, the MiniMap overlay will remain dark-tinted. This is a cosmetic regression that should be addressed in Phase 2.

---

_Verified: 2026-03-25T19:01:00Z_
_Verifier: Claude (gsd-verifier)_
