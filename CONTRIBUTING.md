# Contributing to Theia

Thanks for taking the time to improve Theia. This guide explains the expected workflow for contributing code, documentation, tests, and operational improvements.

For project context, start with [README.md](README.md). For full setup, deployment, configuration, WISP lab, and API details, read [SETUP.md](SETUP.md).

## Ways to Contribute

Useful contributions include:

- bug fixes with focused regression coverage;
- documentation improvements;
- backend service, API, repository, and worker fixes;
- frontend usability, accessibility, and reliability improvements;
- WISP lab, seed script, and validation improvements;
- test coverage for existing behavior;
- small features that fit the current architecture and operational goals.

Before starting a large feature or behavior change, open an issue or discussion with the intended scope, user impact, and testing plan. Small fixes and documentation updates can go straight to a pull request.

## Opening Issues

Open an issue when you want to report a bug, request a feature, propose an operational improvement, or discuss a change before writing code.

Before opening a new issue:

- search existing issues and pull requests for related work;
- confirm the behavior against the current `master` branch when possible;
- check [SETUP.md](SETUP.md) for documented setup, deployment, WISP lab, and troubleshooting steps;
- remove secrets, IPs, credentials, tokens, backup contents, and customer-identifying data from logs or screenshots.

For bug reports, include:

- a clear title that names the failing area;
- the expected behavior and actual behavior;
- steps to reproduce the issue;
- the environment, such as OS, Docker version, browser, deployment mode, and relevant Theia commit or image tag;
- logs, screenshots, API responses, or browser console output when useful;
- whether the issue affects backend, frontend, WISP lab, bridge connector, backup/restore, auth/RBAC, or deployment.

For feature requests, include:

- the problem or operator workflow the feature would solve;
- the users affected by the change;
- the desired behavior;
- any constraints around security, deployment, backward compatibility, or operations;
- examples from real network workflows when they help explain the need.

Do not open public issues for sensitive security vulnerabilities or exposed secrets. If you discover a security issue, avoid publishing exploit details, credentials, production URLs, or customer data. Open a minimal issue asking for a private security contact path, or contact the maintainer privately if you already have a trusted channel.

## Local Development

The standard local environment is Docker-first. No local Go or Node.js installation is required for the normal development stack.

Prerequisites:

- Docker 24+
- Docker Compose
- Make
- `curl`

Start the full development stack:

```bash
make dev
```

Open http://localhost:3000 and sign in with:

- username: `administrator`
- password: `theia`

The first login requires a password change.

For a self-contained demo topology:

```bash
make wisp-lab
make wisp-seed-all
```

Useful commands:

```bash
make logs             # Follow backend logs
make stop             # Stop the development stack
make clean            # Stop containers and remove development volumes
make test             # Run backend unit tests inside compose
make backend-fast     # Backend vet, build, vulnerability scan, tests, and coverage gate
make frontend-fast    # Frontend install, Biome check, coverage, typecheck, and build
make browser-e2e      # Install Playwright Chromium and run browser E2E tests
make bridge-build-all # Build WinBox Bridge Connector binaries
```

See [SETUP.md](SETUP.md) for production, staging, keyring, API, and troubleshooting details.

## Development Workflow

Keep changes focused and easy to review.

- Use a topic branch for each contribution.
- Keep commits atomic: one coherent change per commit.
- Use Conventional Commit messages, such as `fix: handle empty device names` or `docs: add contributing guide`.
- Prefer the existing architecture and local helper APIs over new patterns.
- Avoid unrelated refactors, formatting churn, and opportunistic cleanup.
- Do not commit generated artifacts, local config, secrets, backups, or ignored files.
- Include tests or explain why a documentation-only or tooling-only change does not need them.

## Code Style

Backend code is Go under `cmd/` and `internal/`. Keep service, repository, API, worker, and domain boundaries clear. Use exported comments for exported functions and public types when they are not self-evident. Add comments for complex behavior and non-obvious decisions, not for obvious assignments.

Frontend code is React, TypeScript, Vite, Tailwind, Biome, Vitest, and Playwright under `frontend/`. Follow the existing component and hook patterns. Tests that cause React state updates should wrap those updates in `act(...)` before asserting the browser-visible behavior.

Database changes belong in `internal/repository/postgres/migrations/` and should include tests or verification notes for migration behavior.

## Testing Expectations

Choose verification based on the scope of the change.

Backend:

```bash
go test ./... -count=1
make backend-fast
```

Use package-focused tests for narrow backend changes, for example:

```bash
go test ./internal/service ./internal/api -count=1
```

Frontend:

```bash
npm --prefix frontend run check
npm --prefix frontend run typecheck
npm --prefix frontend run test
```

Use coverage when thresholds or broad frontend behavior matter:

```bash
npm --prefix frontend run test:coverage
make frontend-fast
```

Browser workflows:

```bash
make browser-e2e
```

Run browser E2E tests for changes that cross backend, auth, routing, canvas maps, realtime updates, or browser-only behavior.

## Security and Secrets

Never commit secrets or environment-specific values.

Do not commit:

- `.env`, `.env.prod`, `.env.staging`, or production overrides;
- `config.yaml`;
- database passwords or DSNs with credentials;
- `THEIA_ENCRYPTION_KEY`, `THEIA_ENCRYPTION_KEYS`, session secrets, metrics tokens, or bridge secrets;
- real SNMP communities, SNMPv3 credentials, SSH keys, WinBox credentials, backup archives, or database dumps;
- local Docker volumes, build outputs, coverage outputs, or ignored files.

If a change touches authentication, credential storage, backup/restore, bridge launch, audit logs, or authorization, include the security impact in the pull request description.

## Pull Requests

Pull requests should be small enough to review confidently.

Include:

- what changed and why;
- linked issue or production problem when applicable;
- commands run for verification;
- screenshots or screen recordings for visible UI changes;
- migration notes for database schema changes;
- operator impact for configuration, deployment, backup, restore, or bridge changes.

Before requesting review, check:

- the change is scoped to the stated problem;
- related docs are updated;
- tests or verification commands are included;
- new configuration is documented in [SETUP.md](SETUP.md);
- secrets and generated files are not included;
- license and attribution notices are preserved.

## License and Attribution

Theia is licensed under the [Apache License 2.0](LICENSE).

Copyright 2026 Lorenzo Oliva.

Redistributions must preserve the license terms and applicable attribution notices. The project attribution notice is provided in [NOTICE](NOTICE).
