# Agent Notes

## Error Log Protocol

- When a command, CI job, or verification step fails, add a dated entry here before retrying or switching commands.
- Each entry must include the failing command or job, the observed error, the root cause when known, and the replacement command or mitigation.

## Commit Message Protocol

- Commit titles must follow Conventional Commits: `<type>(optional-scope): <imperative summary>`.
- Use common types such as `feat`, `fix`, `test`, `docs`, `refactor`, `style`, `perf`, `build`, `ci`, `chore`, and `revert`.
- Manual merge commit titles must follow the same convention instead of using the default `Merge ...` title.

## Error Log

### 2026-05-06 - Local CI Inspection Command

- Failing command: `python /home/azmin/.codex/plugins/cache/openai-curated/github/9d07fd08/skills/gh-fix-ci/scripts/inspect_pr_checks.py --repo . --pr 39 --json`
- Observed error: `/bin/bash: line 1: python: command not found`
- Root cause: this environment exposes Python as `python3`, not `python`.
- Mitigation: use `python3` for Python scripts in this repo environment.

### 2026-05-06 - Local GitHub Checks Command

- Failing command: `gh pr checks 39 --repo lollinoo/theia --json name,state,bucket,link,startedAt,completedAt,workflow`
- Observed error: `unknown flag: --json`
- Root cause: installed `gh` version does not support `--json` for `gh pr checks`.
- Mitigation: use plain `gh pr checks 39 --repo lollinoo/theia`, then inspect run/job details with `gh run view <run_id> --json ...` and `gh run view <run_id> --job <job_id> --log`.

### 2026-05-06 - PR #39 frontend-fast CI

- Failing job: `frontend-fast` in GitHub Actions run `25422394396`, job `74567459433`.
- Observed error: `npm --prefix frontend run check` failed because Biome wanted `frontend/src/components/NavigationPill.tsx` line 41 formatted onto one line.
- Root cause: `make frontend-fast` runs a full frontend Biome check, not only checks for files touched by the feature branch.
- Mitigation: before pushing frontend PRs, run `npm --prefix frontend run check` or `make frontend-fast`, not only targeted `npx biome check <changed files>`.

### 2026-05-06 - frontend `test:coverage` Error Output That Does Not Fail The Gate

- Command: `make frontend-fast`
- Observed output: Vitest printed `Error: useTheme must be used within ThemeProvider` from `frontend/src/contexts/ThemeContext.test.tsx`.
- Root cause: the test intentionally exercises the failure path for calling `useTheme` outside `ThemeProvider`; Vitest reports the thrown error to stderr even though the suite passes.
- Mitigation: do not treat this stderr text alone as a failing gate. Check the command exit code and the Vitest summary; in this run it reported `96 passed` test files and `981 passed` tests.

### 2026-05-06 - Intentional TDD RED For Stale Runtime Snapshot

- Failing command: `npm --prefix frontend test -- --run src/hooks/useWebSocket.test.ts -t "ignores stale full snapshots"`
- Observed error: the new regression test expected `cpu_percent` to remain `70`, but the hook applied an older full snapshot and returned `20`.
- Root cause: `useWebSocket` rejected stale versioned deltas but did not reject older versioned full snapshot envelopes after a newer HTTP runtime bootstrap.
- Mitigation: add stale full-snapshot rejection before applying snapshot envelopes whose version is lower than the current runtime base.

### 2026-05-06 - Stale Snapshot Patch Context Miss

- Failing command: `apply_patch` update for `frontend/src/hooks/useWebSocket.ts`
- Observed error: `apply_patch verification failed: Failed to find expected lines`.
- Root cause: the patch context around the snapshot branch did not match the current file exactly.
- Mitigation: inspect the exact local context with `sed`, then apply a narrower patch around stable neighboring lines.

### 2026-05-06 - GitHub PR Title Edit GraphQL Failure

- Failing command: `gh pr edit 39 --repo lollinoo/theia --title "fix(realtime): keep WebSocket open during HTTP runtime resync"`
- Observed error: `GraphQL: Projects (classic) is being deprecated... (repository.pullRequest.projectCards)`.
- Root cause: this `gh pr edit` path queries deprecated Projects classic fields and fails before applying the title change.
- Mitigation: update PR metadata through the REST endpoint with `gh api repos/<owner>/<repo>/pulls/<number> -X PATCH -f title=...`.

### 2026-05-06 - Missing GitHub CLI PR Update-Branch Command

- Failing command: `gh pr update-branch 39 --repo lollinoo/theia`
- Observed error: `unknown command "update-branch" for "gh pr"`.
- Root cause: the installed GitHub CLI version does not provide `gh pr update-branch`.
- Mitigation: inspect branch freshness with `git merge-base`, `git rev-parse origin/master`, and `git rev-list --left-right --count origin/master...HEAD`; use GitHub REST `PUT /repos/<owner>/<repo>/pulls/<number>/update-branch` only if the PR branch is actually behind.

### 2026-05-06 - Root Package Manifest Probe

- Failing command: `cat package.json`
- Observed error: `cat: package.json: No such file or directory`
- Root cause: this repository has the npm package manifest under `frontend/package.json`, not at the repository root.
- Mitigation: use `npm --prefix frontend ...` or read `frontend/package.json` for frontend scripts.

### 2026-05-06 - Intentional TDD RED For Restored Canvas Positions

- Failing command: `npm --prefix frontend test -- --run src/components/canvas/useCanvasData.test.ts -t "applies restored backend positions"`
- Observed error: the new regression test expected the node position to update from `{x: 10, y: 20}` to the restored backend value `{x: 110.5, y: 220.25}`, but the canvas kept `{x: 10, y: 20}` after `backend-reconnected`.
- Root cause: `buildTopologyNodes` prefers current in-memory positions over freshly fetched persisted positions, so a restore with the same topology can leave pre-restore coordinates visible.
- Mitigation: teach backend reconnect topology refreshes to treat fetched persisted positions as authoritative over stale in-memory positions.
