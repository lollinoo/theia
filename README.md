# Theia

Theia is a network topology and operations platform for MikroTik and ISP-style environments. It combines a live topology canvas, SNMP-based discovery, runtime health, device and instance backups, role-based administration, and a local WinBox Bridge Connector into one workflow.

The project is currently built as a Docker-first Go and React application. For complete setup, production, staging, keyring, and API details, see [SETUP.md](SETUP.md).

## Vision

Theia aims to make network state visible and actionable without requiring operators to jump between spreadsheets, monitoring dashboards, terminal sessions, and vendor tools.

The long-term direction is an operations surface that can:

- discover devices and links from real network telemetry;
- visualize topology in maps and operational areas;
- keep runtime health, reachability, and interface state fresh in the browser;
- preserve device and instance backups for recovery;
- launch vendor tools such as WinBox from the topology with audited, per-user access;
- support teams through RBAC, audit trails, and safe credential handling.

## Current State

Theia currently includes:

- **Topology canvas**: interactive React Flow canvas with devices, links, saved maps, areas, ghost devices, visual colors, zoom controls, and runtime overlays.
- **Discovery and monitoring**: SNMP device polling, LLDP/CDP topology discovery, Prometheus integration, interface counters, health normalization, and realtime WebSocket updates.
- **Device operations**: device CRUD, SNMP profiles, credential profiles, WinBox credential assignment, probe ports, notes, tags, and vendor-aware behavior.
- **Backups**: SSH/SFTP device backups, bulk backup runs with pause/resume/cancel support, backup file downloads, PostgreSQL instance backup and restore flows, and backup retention settings.
- **Administration**: first-party password auth, session and CSRF protection, RBAC permissions, admin users, roles, role permissions, password reset, and audit logs.
- **Bridge Connector**: local desktop connector for launching WinBox from Theia with per-user Bridge Secrets and one-time launch tokens.
- **WISP lab**: Docker-based MikroTik-flavoured lab topology with FRRouting, OSPF, BGP/default-route checks, SNMP/LLDP data, and seed scripts for repeatable demos.
- **Deployment surfaces**: development, staging, and production Docker Compose stacks; Makefile targets for common workflows; GHCR-oriented production and staging compose files.

## Architecture

Theia is organized around a Go backend, PostgreSQL persistence, a React/Vite frontend, and optional supporting services.

```text
Browser
  |
  | REST + WebSocket
  v
React/Vite frontend  <---- nginx proxy in production
  |
  v
Go backend API
  |-- PostgreSQL repositories and migrations
  |-- SNMP discovery and polling workers
  |-- Prometheus and SNMP metric collection
  |-- WebSocket runtime snapshot/delta hub
  |-- backup schedulers and restore services
  |-- auth, RBAC, audit, settings, and bridge services
  |
  +--> Network devices via SNMP / SSH / SFTP
  +--> Prometheus / SNMP exporter
  +--> local WinBox Bridge Connector
```

### Backend

The backend entrypoint is `cmd/theia`. It loads configuration, runs PostgreSQL migrations, wires repositories and services, starts polling and backup workers, exposes `/api/v1` REST endpoints, serves `/api/v1/ws` for realtime browser updates, and exposes `/metrics` for Prometheus-compatible scraping.

The main backend packages live under `internal/`:

- `internal/api`: HTTP routing, handlers, middleware, auth and route policy boundaries.
- `internal/service`: domain orchestration for devices, backups, auth, bridge launch flow, settings, vendors, and instance restore.
- `internal/repository/postgres`: PostgreSQL repositories and migrations.
- `internal/snmp`, `internal/collector`, `internal/worker`: discovery, polling, runtime snapshots, and metric collection.
- `internal/ws`: WebSocket message contracts, runtime snapshots, deltas, resync behavior, and hub metrics.
- `internal/domain`: shared domain models and repository interfaces.

### Frontend

The frontend lives in `frontend/` and uses React, Vite, TypeScript, React Flow, D3 force layout helpers, Tailwind, Biome, Vitest, and Playwright.

The top-level app keeps the topology canvas mounted while switching between canvas, topology hub, dashboard, admin, and user settings views. It uses REST endpoints for canonical topology and configuration data, then keeps runtime state fresh through `/api/v1/ws`.

### Data Flow

1. Devices are created manually, through seed scripts, or from lab workflows.
2. The backend probes devices with SNMP and discovers interfaces, LLDP/CDP neighbors, vendor details, and runtime health.
3. Canonical devices, links, maps, positions, areas, credentials, backup records, users, roles, and settings are stored in PostgreSQL.
4. Workers poll runtime metrics from SNMP and Prometheus sources.
5. The WebSocket hub broadcasts full snapshots, deltas, topology changes, and resync notices to browser clients.
6. The frontend combines canonical topology with runtime state to render the live canvas, dashboards, alerts, panels, and admin workflows.

## Installation

The fastest way to run Theia locally is the Docker Compose development stack. No local Go or Node.js install is required for the standard dev path.

Prerequisites:

- Docker 24+
- Docker Compose
- Make
- `curl`

Start the stack:

```bash
git clone https://github.com/lollinoo/theia.git
cd theia
make dev
```

Open http://localhost:3000 and sign in with:

- username: `administrator`
- password: `theia`

The first login requires a password change before normal API and UI workflows continue.

Common development URLs:

- Frontend: http://localhost:3000
- Backend API: http://localhost:8080
- PostgreSQL: `127.0.0.1:5432`
- Prometheus: http://localhost:9090
- SNMP exporter: http://localhost:9116

For a self-contained topology demo:

```bash
make wisp-lab
make wisp-seed-all
```

For the full setup guide, including production, staging, configuration, keyring rotation, API auth, troubleshooting, and WISP lab details, read [SETUP.md](SETUP.md).

## Bridge Connector

The Bridge Connector lets an authenticated user launch WinBox locally from Theia without exposing raw device passwords to the browser.

The high-level flow is:

1. A user generates a personal Bridge Secret in **User Settings -> Bridge Connector**.
2. The user downloads and starts the platform-specific `winbox-bridge` connector.
3. The local connector setup wizard stores the Theia URL, allowed browser origin, bridge port, WinBox path, and Bridge Secret.
4. When the user launches WinBox from Theia, the backend creates a short-lived one-time launch token for the selected device.
5. The browser sends that launch token to the local connector at `http://localhost:<bridge-port>/launch`.
6. The connector sends the launch token plus its Bridge Secret to Theia.
7. The backend validates the user, secret, token, expiry, token reuse, and WinBox credential assignment.
8. The connector receives the device IP, username, and password, then starts WinBox locally.

Bridge authentication is per-user. Connector binaries are served to authenticated users when `THEIA_BRIDGE_BINARIES_DIR` points to built files such as `winbox-bridge-linux-amd64` or `winbox-bridge-windows-amd64.exe`.

For the full connector setup, stable install paths, advanced `config.json` shape, LAN-origin notes, and migration guidance, see [User Settings and Bridge Connector](SETUP.md#user-settings-and-bridge-connector).

## Use Cases

- **ISP topology operations**: model core, POP, edge, tower, and subscriber network layers with maps, areas, link health, and routing lab fixtures.
- **Network discovery**: seed or add devices, run SNMP/LLDP/CDP discovery, and build a topology that reflects observed neighbor data.
- **Live monitoring**: track reachability, interface status, traffic counters, health states, alerts, and Prometheus availability in a live browser session.
- **Operational maps**: create saved maps, materialize area views, preserve manual device placement, and use ghost devices to keep cross-area context visible.
- **Device access**: launch WinBox from a topology node through a local, audited, per-user bridge flow.
- **Backup and recovery**: schedule or run device backups, download backup files, run bulk operations, and create or restore PostgreSQL instance backups.
- **Team administration**: manage users, roles, permissions, audit logs, password resets, and account-level Bridge Connector settings.
- **Development and demos**: use the WISP lab to reproduce topology discovery, OSPF/BGP checks, and radio access-layer scenarios without real hardware.

## Development

The main local task surface is the `Makefile`.

Useful commands:

```bash
make dev              # Start backend, frontend, PostgreSQL, Prometheus, and SNMP exporter
make logs             # Follow backend logs
make stop             # Stop the development stack
make clean            # Stop containers and remove dev volumes
make test             # Run backend unit tests inside compose
make backend-fast     # Backend vet, build, vulnerability scan, tests, and coverage gate
make frontend-fast    # Frontend install, Biome check, coverage, typecheck, and build
make browser-e2e      # Install Playwright Chromium and run browser E2E tests
make wisp-lab         # Start the WISP lab
make wisp-seed-all    # Seed WISP routers and radio access devices into Theia
make bridge-build-all # Build WinBox Bridge Connector binaries
```

Repository layout:

```text
cmd/
  theia/            Go backend entrypoint
  winbox-bridge/    local desktop Bridge Connector
  theia-scale-lab/  scale-lab utility
frontend/           React/Vite frontend
internal/           backend API, services, domain, repositories, workers, collectors
docker/             Prometheus, SNMP, and lab container assets
scripts/            seed, validation, test, backup, and bridge helper scripts
vendors/            vendor capability definitions
docs/               project documentation and planning artifacts
```

Testing expectations depend on the scope of a change. Backend contract changes generally use `go test ./... -count=1` or `make backend-fast`. Frontend changes generally use `npm --prefix frontend run check`, `npm --prefix frontend run typecheck`, and focused or coverage Vitest runs. Cross-cutting browser workflows use `make browser-e2e`.

## Documentation

- [SETUP.md](SETUP.md): complete setup, configuration, deployment, API, bridge, and troubleshooting guide.
- [CONTRIBUTING.md](CONTRIBUTING.md): local development workflow, testing expectations, pull request guidance, and attribution notes.
- `docs/`: project notes, generated plans, and supporting documentation.
- `frontend/e2e/`: browser workflow tests that document key UI behavior.
- `internal/repository/postgres/migrations/`: database schema history and runtime capabilities.

## License

Theia is licensed under the [Apache License 2.0](LICENSE).

The project attribution notice is provided in [NOTICE](NOTICE).

Copyright 2026 Lorenzo Oliva.
