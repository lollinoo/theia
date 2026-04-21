# Frontend Quality Tooling Design

Date: 2026-04-21
Status: Approved for planning
Scope: Introduce a dedicated frontend formatter and linter with a blocking CI gate so formatting, lint, tests, typecheck, and build quality stay uniform as the frontend grows.

## Summary

This slice adds a simple but strict frontend quality gate built around one tool: Biome.

Today the frontend already has test, coverage, typecheck, and production build entrypoints, but it has no dedicated formatter or linter. The result is predictable drift: formatting becomes inconsistent, large component files become harder to scan, and review time gets spent on style noise instead of behavior. In a frontend that already contains large components and runtime-heavy UI code, that drift directly increases the cost of refactors and defect isolation.

The design introduces Biome as the official frontend formatter and linter, applies one repo-wide normalization pass across `frontend/`, and makes the existing `frontend-fast` CI job fail on any formatting, lint, test, typecheck, or build regression. The goal is not to redesign frontend architecture or force broad code churn beyond this quality boundary. The goal is to establish one automatic standard that keeps future changes readable and cheap to review.

## Current Context

Relevant current files and behaviors:

- `frontend/package.json`: defines `test`, `test:coverage`, `typecheck`, `build`, and Playwright commands, but no formatter or linter scripts.
- `frontend/vitest.config.ts`: already enforces frontend coverage thresholds, so tests are treated as part of the required gate.
- `.github/workflows/ci.yml`: currently runs `frontend-fast`, but that gate only covers install, frontend coverage tests, typecheck, and build.
- `Makefile`: `frontend-fast` currently runs `npm --prefix frontend ci`, `npm --prefix frontend run test:coverage`, `npm --prefix frontend run typecheck`, and `npm --prefix frontend run build`.
- The repo does not currently contain frontend-local config for `eslint`, `prettier`, or `biome`.
- The frontend contains increasingly large React and runtime composition files, which makes inconsistent formatting and missing static checks more expensive than in a small UI.

## Goals

- Introduce one official frontend formatter and linter.
- Make frontend formatting and lint compliance a blocking part of CI.
- Keep the gate simple to understand and easy to run locally.
- Preserve the existing `frontend-fast` job name while expanding its responsibility.
- Keep test, typecheck, and production build checks inside the same frontend gate.
- Normalize the current `frontend/` tree once so the project does not remain in a mixed-style state.

## Non-Goals

- Refactoring large frontend components purely because the new tooling touches them.
- Introducing warning-only enforcement or a long transition phase.
- Splitting frontend quality into several separate CI jobs in this slice.
- Reworking browser E2E coverage or backend quality tooling.
- Adding heavily opinionated style or naming rules that force broad architectural churn.

## Approach Options Considered

### 1. Separate Prettier And ESLint

This is the most conventional JavaScript setup and would be familiar to almost every frontend engineer.

It was rejected for this slice because the repo currently has no dedicated frontend quality tool at all, and the immediate need is to establish one simple, low-friction gate. Adding two tools, two configuration surfaces, and the usual overlap between formatting and lint concerns creates more setup and more maintenance than needed for the current goal.

### 2. Biome On Touched Files Only

This would reduce the size of the first formatting diff.

It was rejected because it leaves the frontend in a mixed state for an extended period. That would weaken the main benefit of the slice, which is making review and refactor work less noisy across the whole frontend, not only in recently touched files.

### 3. Biome For The Whole Frontend With Blocking CI

This is the selected approach.

It gives the repo one tool for both formatting and linting, one initial cleanup pass, and one stable CI rule set from that point forward. That matches the user goal of introducing a gate that is simple, rigid, and immediately useful.

## Proposed Architecture

The frontend quality gate should continue to use the existing `frontend-fast` entrypoint, but expand its responsibilities.

### Tooling Ownership

Biome becomes the only formatter and linter for the frontend slice.

Its responsibilities are:

- code formatting;
- import and style normalization handled by its formatter;
- lint checks that catch real correctness and maintainability issues without forcing broad refactors.

This slice should keep the Biome configuration intentionally small. The design should prefer Biome defaults and only introduce explicit configuration where the repo needs predictability or exclusions.

### Frontend Gate Ownership

`frontend-fast` remains the single required frontend quality gate in both local workflows and CI.

Its responsibilities are:

- install frontend dependencies;
- verify formatting and lint compliance through Biome in check mode;
- run frontend tests;
- run frontend typecheck;
- run frontend production build.

This preserves one clear answer to the question: "What must pass before a frontend-affecting change is mergeable?"

## Gate Definition

`frontend-fast` should run these checks in stable order:

1. dependency install
2. Biome check
3. frontend tests
4. frontend typecheck
5. frontend production build

The gate is blocking. It fails immediately if any of the following occur:

- formatting drift;
- lint violations;
- failing frontend tests;
- TypeScript build graph errors;
- production build failures.

The CI workflow should continue to expose this gate through the existing `frontend-fast` job name so the required PR checks remain stable and easy to recognize.

## Scope Boundaries

### In Scope

- Add Biome as a frontend dev dependency.
- Add frontend-local Biome configuration.
- Add npm scripts for formatting and checking.
- Run one initial formatting and lint-fix pass where safe and tool-supported.
- Update `Makefile` and CI so `frontend-fast` includes Biome checks alongside test, typecheck, and build.
- Normalize the current frontend files covered by the formatter.

### Out Of Scope

- Refactoring frontend modules only to satisfy aesthetic preferences.
- Reorganizing component boundaries unless a minimal local fix is required to satisfy a correctness lint rule.
- Creating a separate CI job only for formatter or lint.
- Adding autofix behavior to CI.
- Extending the same tool choice to the backend in this slice.

## File Coverage And Exclusions

Biome should cover frontend source files and relevant frontend config files where the tool applies cleanly.

Coverage should include at least:

- `frontend/src/**/*.{ts,tsx}`;
- frontend config files such as Vite, Vitest, Playwright, and TypeScript config files where supported;
- frontend JSON files that Biome can format safely, such as `package.json`.

Biome should exclude generated or disposable outputs such as:

- `frontend/coverage/`;
- `frontend/test-results/`;
- `frontend/playwright-report/`;
- other generated build outputs already treated as artifacts.

The design should prefer explicit exclusions in config over relying on engineers to remember ad hoc CLI patterns.

## Lint Policy

This slice is about enforcing a minimum quality floor, not introducing a stylistic ideology.

The lint policy should therefore start narrow:

- keep defaults where they produce practical signal;
- prefer rules that catch correctness, suspicious patterns, or readability issues with high agreement value;
- avoid rules that force widespread naming, file-organization, or React-pattern rewrites unless the repo already expects them.

If enabling a rule would require widespread structural edits across the frontend, that rule belongs in a later slice, not here.

## Rollout Strategy

The rollout is immediate and blocking.

1. Add Biome dependency, config, and scripts.
2. Run one normalization pass across the targeted frontend files.
3. Fix any real lint failures with the smallest local changes that make the code valid.
4. Update `frontend-fast` and CI to run the Biome gate before the existing test, typecheck, and build checks.
5. Keep the gate blocking from the first merged version.

There is no warning-only phase, no legacy carve-out, and no touched-files-only fallback. The repo should move from no frontend formatter/linter to one enforced standard in a single slice.

## Local And CI Ergonomics

The local and CI commands should match closely.

The frontend should expose clear npm scripts such as:

- `format` for applying Biome formatting;
- `format:check` or `lint`/`check` for non-mutating CI validation;
- existing `test`, `typecheck`, and `build` commands unchanged where possible.

`Makefile` should remain the repo-standard local entrypoint through `make frontend-fast`, so engineers do not need separate tribal knowledge for CI versus local verification.

## Risks And Mitigations

### Risk: First Diff Is Noisy

An initial whole-frontend formatting pass will create a larger one-time diff.

Mitigation:

- keep the slice tightly focused on tooling and minimal lint fixes;
- avoid opportunistic refactors in the same change;
- keep the initial pass isolated so later diffs become smaller and clearer.

### Risk: Biome Surfaces Real Existing Issues

Some files may fail lint for reasons beyond formatting.

Mitigation:

- fix only the issues required for the gate to pass;
- prefer the smallest behavior-preserving edits;
- defer broad cleanup work to later slices.

### Risk: Gate Runtime Grows Slightly

Adding Biome increases the work done inside `frontend-fast`.

Mitigation:

- use one combined tool rather than separate formatter and linter stacks;
- keep the job count unchanged;
- place Biome before tests so obvious quality failures exit early.

## Testing And Verification Expectations

This slice is complete when:

- `make frontend-fast` runs formatter/lint validation, tests, typecheck, and build successfully;
- the `frontend-fast` CI job runs the same quality steps;
- formatting drift or lint regressions fail locally and in CI;
- the frontend tree is no longer in a mixed-format state after the initial normalization pass.

## Open Decisions Resolved In This Design

- Tool choice: Biome, not Prettier plus ESLint.
- Enforcement mode: blocking immediately, not warning-only.
- Rollout scope: whole frontend, not touched-files-only.
- CI shape: keep one `frontend-fast` gate rather than adding separate jobs.
