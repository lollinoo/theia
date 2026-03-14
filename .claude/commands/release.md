---
name: release
description: Manage semantic versioning, changelogs, conventional commits, and release tags for MikroTik Theia. Use when the user wants to prepare a release, bump a version, preview changelog entries, check release status, push release tags, or commit changes with conventional commit messages. Triggers on "release", "version bump", "cut a release", "tag", "what version", "changelog preview", "commit this", "commit changes", "commit work". Do NOT trigger on general git operations unrelated to releasing or committing.
---

# Release Management

Manages semantic versioning, changelog generation, conventional commits, and release tagging for MikroTik Theia.

## Files

- `VERSION` — single line with current version (e.g. `0.1.0`)
- `CHANGELOG.md` — [Keep a Changelog](https://keepachangelog.com/en/1.1.0/) format
- Git tags — `v{VERSION}` format (e.g. `v0.1.0`)

## Versioning Scheme

[Semantic Versioning](https://semver.org/):

- **MAJOR** (X.0.0) — breaking API, database schema, or config format changes
- **MINOR** (0.X.0) — new features, endpoints, UI components, integrations
- **PATCH** (0.0.X) — bug fixes, performance improvements, behavioral no-ops

While in `0.x.y` (pre-1.0), minor bumps may include breaking changes.

## Pre-Flight Checks (for release operations only)

Before any release operation that modifies files or creates tags (`bump`, `push`), verify the following. These checks do NOT apply to `/release commit`.

1. **No uncommitted changes to tracked files** — Run `git status --porcelain` and check for entries starting with `M`, `A`, `D`, `R`, or `U` (modified, added, deleted, renamed, or unmerged tracked files). Untracked files (`??`) are fine and should NOT block a release.
2. **Correct branch** — Warn if not on `main` or `dev-v1`. Releases from feature branches are usually mistakes.
3. **No unpushed commits** — `git log @{u}..HEAD --oneline` should be empty. Releasing with unpushed commits means the tag references code the remote doesn't have.
4. **VERSION file exists** — If missing, create it with `0.0.0` and inform the user.
5. **CHANGELOG.md exists** — If missing, create the skeleton with the `[Unreleased]` header and inform the user.

## Commands

### `/release commit [description]`

Stage and commit changes using conventional commit format. Analyzes the diff to determine the appropriate commit type and scope, then creates a brief, well-formatted commit message.

**Steps:**

1. Run `git status --porcelain` to see what has changed.
2. Run `git diff --stat` (for unstaged) and `git diff --cached --stat` (for staged) to understand the scope of changes.
3. If there are no changes to commit, inform the user and stop.
4. Read the actual diffs to understand what changed:
   - `git diff` for unstaged changes
   - `git diff --cached` for already-staged changes
5. Determine the commit type from the nature of the changes:
   - New files/features/functionality → `feat`
   - Bug fixes, error corrections → `fix`
   - Code restructuring without behavior change → `refactor`
   - Performance improvements → `perf`
   - Documentation only → `docs`
   - Build system, CI, dependencies → `build` or `ci`
   - Tests → `test`
   - Maintenance, cleanup → `chore`
6. Determine the scope from which area of the codebase changed:
   - `internal/` backend code → scope by package (e.g. `snmp`, `metrics`, `ws`)
   - `frontend/src/components/` → scope by component area (e.g. `canvas`, `toolbar`, `alerts`)
   - `frontend/src/hooks/` → scope by hook name
   - Multiple areas → omit scope or use broader scope
   - Config/infra files → scope like `docker`, `vite`, `ci`
7. Write a brief commit message — one line, under 72 characters, lowercase after the prefix. The message should describe the **what** concisely, not the **how**.

   Format: `type(scope): description` or `type: description`

   **Good examples:**
   - `feat(canvas): add device status-based link coloring`
   - `fix(api): handle non-array data fields in response parsing`
   - `refactor: rename onClose prop to avoid naming conflict`
   - `docs: add setup guide for dev and production environments`

   **Bad examples (too long, too vague):**
   - `feat: Enhance device management by adding device interface status tracking, updating UI components for better device identification, and implementing a searchable device selection in the link creation panel`
   - `fix: fix bug`
   - `update code`

8. If the user provided a description argument, use it to inform the commit message but still apply conventional commit formatting.
9. Stage the appropriate files:
   - If files are already staged, respect that selection.
   - If nothing is staged, stage all modified/new files relevant to the change. Do NOT stage files that look like secrets (`.env`, credentials, keys) — warn the user instead.
10. Show the proposed commit message and staged files to the user. Ask for confirmation.
11. Create the commit using a HEREDOC for the message:
    ```bash
    git commit -m "$(cat <<'EOF'
    type(scope): description
    EOF
    )"
    ```
12. Show the result with `git log --oneline -1`.

**When the user provides a description:**
If the user runs `/release commit added search functionality`, use their description to craft the message: `feat: add search functionality`. Infer the type from their words — "added" → `feat`, "fixed" → `fix`, "updated/changed" → `refactor`, etc.

### `/release bump [major|minor|patch]`

Bump the version, update changelog, commit, and tag.

**Steps:**

1. Run all pre-flight checks (see above).
2. Read `VERSION` to get the current version.
3. If bump type is omitted, auto-detect (see Auto-Detection below) and confirm with the user before continuing.
4. Calculate the new version.
5. Collect commits since the last tag:
   ```bash
   git tag -l "v*" --sort=-v:refname | head -1
   ```
   - If a tag exists: `git log --format="%H %s" v{OLD}...HEAD`
   - If no tag exists (first release): `git log --format="%H %s"`
6. Categorize commits and build changelog entries (see Changelog Generation below).
7. Insert the new version section into `CHANGELOG.md`, moving any `[Unreleased]` content into it. Preserve a fresh empty `[Unreleased]` section above.
8. Update comparison links at the bottom of `CHANGELOG.md`:
   ```markdown
   [Unreleased]: https://github.com/lollinoo/mikrotik-theia/compare/v{NEW}...HEAD
   [X.Y.Z]: https://github.com/lollinoo/mikrotik-theia/compare/v{OLD}...v{NEW}
   ```
   For the very first release (no previous tag):
   ```markdown
   [X.Y.Z]: https://github.com/lollinoo/mikrotik-theia/releases/tag/v{NEW}
   ```
9. Write the new version to `VERSION`.
10. Stage `VERSION` and `CHANGELOG.md`.
11. Commit: `release: v{NEW}`
12. Create annotated tag: `git tag -a v{NEW} -m "Release v{NEW}"`
13. Show a summary of what was released and ask if they want to push.

### `/release changelog`

Preview what would go into the next release, without modifying anything.

**Steps:**

1. Read current version from `VERSION`.
2. Collect commits since last tag (or all commits if no tag exists).
3. Categorize and display in changelog format.
4. Suggest bump type based on auto-detection.
5. Show count of commits and date range.

### `/release status`

Show the current versioning state.

**Steps:**

1. Read `VERSION`.
2. List tags: `git tag -l "v*" --sort=-v:refname | head -5`
3. Count commits since last tag.
4. Check for unpushed tags: `git push --tags --dry-run 2>&1`
5. Show current branch and remote tracking status.

### `/release push`

Push the latest tag and commits to the remote.

**Steps:**

1. Show the tag and branch that will be pushed, and ask for confirmation.
2. Push the branch: `git push origin {branch}`
3. Push the tag: `git push origin v{VERSION}`
4. Offer to create a GitHub release (see GitHub Releases below).

### `/release undo`

Undo the most recent release if it hasn't been pushed.

**Steps:**

1. Find the most recent tag: `git tag -l "v*" --sort=-v:refname | head -1`
2. Check if the tag has been pushed: `git ls-remote --tags origin | grep {TAG}`
3. If pushed, warn the user and refuse — pushed releases need manual cleanup.
4. If not pushed:
   - Delete the local tag: `git tag -d v{VERSION}`
   - Reset the commit: `git reset --soft HEAD~1`
   - Restore `VERSION` and `CHANGELOG.md` from the pre-release state
   - Inform the user what was undone

## Changelog Generation

Raw commit messages are often too long or inconsistent for a changelog. Transform them into concise, user-facing entries.

### Categorization

Map conventional commit prefixes to changelog sections:

| Prefix | Changelog Section |
|--------|------------------|
| `feat:` / `feat(scope):` | **Added** |
| `fix:` / `fix(scope):` | **Fixed** |
| `refactor:` / `refactor(scope):` | **Changed** |
| `perf:` / `perf(scope):` | **Changed** |
| `build:` / `ci:` | **Changed** (only if user-facing) |
| `docs:` | Omit (unless user requests) |
| `chore:` / `test:` | Omit |

Commits without a conventional prefix: categorize by reading the message content — if it describes a new feature, put it under **Added**, etc.

### Writing Good Entries

- **Summarize, don't copy.** A commit message like "feat: Enhance device management by adding device interface status tracking, updating UI components for better device identification, and implementing a searchable device selection in the link creation panel" should become a few distinct entries:
  - Add device interface status tracking
  - Add searchable device selector to link creation panel
  - Improve device identification in UI components
- **Lead with a verb** — Add, Fix, Improve, Remove, Update.
- **Keep each entry to one line** — if a commit did multiple things, split into separate entries.
- **Group by area when helpful** — use bold sub-headers like **Core Backend**, **Frontend**, **Infrastructure** when there are many entries.
- **Drop noise** — merge commits, version bumps, trivial refactors that don't affect behavior.

### Unreleased Section

Between releases, notable changes can be accumulated under `[Unreleased]` at the top of CHANGELOG.md. When `/release bump` runs, this content gets folded into the new version section. Always leave a fresh empty `[Unreleased]` header after a release.

## Auto-Detection for Bump Type

When the user runs `/release bump` without specifying a type:

1. Scan commit messages for signals:
   - `BREAKING CHANGE` in body or `!:` in subject → suggest **major**
   - Any `feat:` commits → suggest **minor**
   - Only `fix:` / `refactor:` / `perf:` → suggest **patch**
2. Show the reasoning: "Found 3 feat commits and 2 fixes — suggesting **minor** bump."
3. Always confirm with the user before proceeding.

## Commit Convention

This project uses conventional commits:

```
type(scope): description

Types: feat, fix, refactor, perf, docs, build, ci, test, chore
Scope: optional — e.g. (canvas), (api), (snmp), (prometheus)
```

## GitHub Releases

After pushing a tag, offer to create a GitHub release. Extract the changelog section for the version and pass it to `gh`:

```bash
gh release create v{VERSION} \
  --title "v{VERSION}" \
  --notes "$(awk '/^## \[{VERSION}\]/{found=1; next} /^## \[/{if(found) exit} found' CHANGELOG.md)"
```

This `awk` approach is more robust than `sed` for extracting a section between two `## [` headers.
