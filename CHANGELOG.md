# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added
- Version injection into Go binary via build-time ldflags
- Version info in `/api/v1/health` response and startup log
- Makefile release workflow (`make version`, `make bump`, `make release`)
- CHANGELOG.md
- Topology observation materialization with unresolved-neighbor tracking and canonical link convergence
- Discovery mode persistence for devices and settings: `off`, `lldp`, `lldp_cdp`, and `bootstrap_once`
- Frontend topology discovery controls in add-device, device-config, and settings flows
- Observability registry coverage for discovery counts, unknown neighbors, link upserts, and topology materialization
- Polling-budget controls and worker-class budgeting for static follow-up reprobes
- Event-driven cache and websocket topology updates with incremental dirty-device application
- PostgreSQL production validation tooling and runbooks
- PostgreSQL backup/restore preflight checks for required 17.x `pg_dump` and `pg_restore` tools with redacted diagnostics
- Scale-lab replay and soak harness with built-in topology stress profiles
- Readiness audit artifacts for the pipeline rework rollout

### Changed
- Static SNMP discovery now respects topology discovery mode instead of always walking LLDP/CDP
- Bootstrap-once discovery now runs as a bounded workflow with persisted bootstrap state and delayed follow-up handling
- Topology links are now materialized from persisted observations instead of direct one-pass neighbor upserts
- Overview websocket broadcasts now react to topology/cache changes instead of relying on broad periodic refreshes
- Device/link cache updates now apply incrementally, reducing full-snapshot churn during topology changes
- Link details and canvas rendering now expose richer LLDP/self-link semantics and pending-port states
- Bootstrap-once UX copy now explains that one automatic follow-up may run to fill missing port details

### Fixed
- Virtual no-IP devices now stay unmonitored across worker, state, and frontend flows
- Reverse-direction neighbor discoveries now reorient and enrich existing links instead of creating broken duplicates
- LLDP self-links and duplicate neighbor variants are deduplicated and rendered consistently
- Incomplete LLDP links refresh live as new discoveries enrich missing source/target interface data
- Delayed LLDP follow-ups now retry under static-budget pressure instead of failing silently
- Stale bootstrap windows now close correctly and eligible peers can be reopened when a neighbor still needs one-shot discovery
- Regular static polling no longer reopens `bootstrap_once` discovery windows
- Changing topology discovery mode on a device now triggers the expected immediate one-shot reprobe when eligible
- Reverse link enrichment now reconciles peer bootstrap metadata so completed links do not leave neighbors stuck in `followup_scheduled`
- Backend logs now describe unresolved neighbors as off-map observations, and frontend status text no longer presents queued follow-ups as errors
- Setup documentation now describes PostgreSQL instance backup/restore support and PostgreSQL client tool requirements instead of outdated PostgreSQL-disabled guidance

## [1.0.0] - 2026-03-20

### Added
- SNMP v2c/v3 device discovery with auto-detection of vendor, model, and firmware
- Real-time network topology visualization with React Flow
- WebSocket-driven live metrics (CPU, memory, uptime, temperature)
- Prometheus integration for SNMP metrics collection
- SNMP credential profiles with encrypted storage
- SSH backup support with profile-based credential management
- Configurable vendor SNMP OID mappings (MikroTik, Ubiquiti, Cisco, generic)
- Docker Compose production deployment with nginx reverse proxy
- SNMP simulator test infrastructure (router, switch, AP)
- SQLite database with automatic migrations
- Security hardening (rate limiting, input validation, request size limits)
- Consistent error handling across all API handlers

### Fixed
- Embedded vendor YAML files in binary for production deploys
- nginx backend proxy port configuration via envsubst

## [0.2.0] - 2026-03-14

### Added
- Initial working prototype with basic SNMP discovery
- Device and link management API
- Frontend topology viewer
