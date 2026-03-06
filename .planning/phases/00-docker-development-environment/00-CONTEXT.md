# Phase 0: Docker Development Environment - Context

**Gathered:** 2026-03-05
**Status:** Executed and verified (Completed)

<domain>
## Phase Boundary

Create a Docker-based development environment for the full MikroTik Theia stack. All building, testing, and verification runs inside containers. The environment includes the Go backend, React frontend (placeholder for Phase 2), SNMP device simulators, and developer tooling (hot-reload, Makefile commands). No Prometheus/Grafana yet — those arrive with Phase 3.

</domain>

<decisions>
## Implementation Decisions

### Container Architecture
- Separate containers per service: Go backend, React frontend, and 3 SNMP simulator devices
- CGO-enabled SQLite using mattn/go-sqlite3 (requires gcc in build container, multi-stage build for final image)
- SNMP simulators use real net-snmp snmpd with custom MIB data files
- 3 simulated devices: one MikroTik router, one Cisco switch, one Ubiquiti AP — covers all vendor detection paths from Phase 1

### Dev Workflow
- Air (cosmtrek/air) for Go hot-reload inside backend container, source code volume-mounted
- Makefile as primary interface: `make dev`, `make test`, `make test-integration`, `make build`, `make clean`, `make seed`, `make verify`
- Docker-compose profiles: "dev" profile (backend + frontend + SNMP sims) and "test" profile (backend + SNMP sims only)
- Named Docker volume for SQLite database — persists across `docker-compose down`, wipeable via `make clean`

### Test Execution
- All tests run inside containers (`make test` executes `go test ./...` inside backend container)
- Build tag separation: unit tests run by default, integration tests use `//go:build integration` tag
- `make test-integration` starts SNMP sims and runs integration-tagged tests against real snmpd
- All verification commands (go vet, go build) run inside containers via `make verify`
- Seed script (`make seed`) pre-populates the 3 SNMP sim devices via the API for manual testing and demos

### Service Dependencies
- No Prometheus or Grafana in this phase — add when Phase 3 is planned
- SNMP sim devices have pre-configured LLDP neighbor relationships: Router <-> Switch <-> AP (simulates real 3-device topology)
- Custom Docker bridge network (e.g., 172.20.0.0/24) with fixed IPs for SNMP sims (172.20.0.10, .11, .12)
- SNMP ports exposed to host (mapped ports like 10161, 10162, 10163) for debugging with host tools like snmpwalk
- Backend container also on the custom network, reaches SNMP sims by fixed IP

### Claude's Discretion
- Exact Dockerfile multi-stage build structure
- Air configuration details (.air.toml)
- snmpd.conf structure and MIB data file format
- Docker-compose health check configuration
- Makefile internal implementation details
- Frontend container placeholder setup (minimal nginx or node dev server)

</decisions>

<specifics>
## Specific Ideas

- SNMP sim devices should have realistic MIB data: proper sysObjectID OIDs for each vendor, interface tables with multiple ports, LLDP neighbor tables linking the 3 devices together
- The seed script should add all 3 sim devices via the REST API so the full discovery + neighbor linking flow is exercised
- `make dev` should be the only command a new developer needs to get the full stack running

</specifics>

<code_context>
## Existing Code Insights

### Reusable Assets
- No existing code yet — greenfield project. Phase 1 plans define the Go project structure (cmd/theia/, internal/domain/, internal/snmp/, etc.)

### Established Patterns
- Phase 1 plans specify: github.com/mattn/go-sqlite3 (CGO), github.com/gosnmp/gosnmp, gopkg.in/yaml.v3
- Go project structure follows standard layout: cmd/, internal/domain/, internal/repository/, internal/snmp/, internal/service/, internal/api/
- Config via YAML + env var overrides (THEIA_* prefix)

### Integration Points
- Docker environment must match Phase 1 plans: Go module path, SQLite DB path (./data/theia.db), config.yaml location, listen address (:8080)
- SNMP sim IPs must be routable from backend container for integration tests
- Seed script targets POST /api/v1/devices endpoint (Phase 1 Plan 03)

</code_context>

<deferred>
## Deferred Ideas

- Prometheus + Grafana containers — Phase 3
- CI/CD pipeline Docker configuration — future consideration
- Production Dockerfile (optimized, no dev tools) — future consideration

</deferred>

---

*Phase: 00-docker-development-environment*
*Context gathered: 2026-03-05*
