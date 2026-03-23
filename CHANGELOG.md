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
