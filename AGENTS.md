# Agent Notes

## Error Log Protocol

- When a command, CI job, or verification step fails, add a dated entry here before retrying or switching commands.
- Each entry must include the failing command or job, the observed error, the root cause when known, and the replacement command or mitigation.

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
