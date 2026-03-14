# Changelog

All notable changes to MikroTik Theia will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

## [0.2.0] - 2026-03-14

### Changed

- Add version tracking, changelog, and release management workflow

## [0.1.0] - 2026-03-14

Initial development release consolidating all work on the `dev-v1` branch.

### Added

- **Core Backend**
  - Domain model and SQLite persistence layer
  - SNMP client with device type detector and discovery service
  - SNMPv3 configuration support
  - Device service layer with async SNMP probing and neighbor handling
  - REST API handlers and background device poller
  - SNMP profile management with metrics source and Prometheus label options
  - SNMP fallback for device metrics when Prometheus lacks data
  - Position handling API and SQLite repository
  - Link CRUD API with interfaces endpoint
  - LLDP/CDP auto-link creation during device probe

- **Prometheus Integration**
  - Prometheus metrics collector and WebSocket integration
  - Prometheus health check endpoint
  - Prometheus + SNMP Fallback metrics source option
  - Prometheus-based interface discovery
  - Fast health check for Prometheus with availability tracking
  - Auto-discovered device hostnames from Prometheus sysName metric
  - Device status override via blackbox_exporter probe results
  - Device status tracking and recovery notifications

- **Real-time Pipeline**
  - WebSocket integration for real-time metrics display on canvas
  - WebSocket reconnect UI
  - Independent Prometheus health monitoring goroutine

- **Frontend - Canvas & Visualization**
  - Interactive topology canvas with React Flow
  - Real-time metrics display on canvas links (TX/RX)
  - Device status-based link and TX/RX box coloring
  - Edit mode functionality for node repositioning
  - Zoom controls
  - Click-to-configure behavior for devices and links
  - Prometheus offline status override for prometheus-only devices

- **Frontend - UI Components**
  - Toolbar with action buttons and notification badges
  - Context menu with WebFig quick access
  - Side panel framework
  - AlertsPanel with centralized alert display
  - AddDevicePanel and DeviceConfigPanel
  - SettingsPanel
  - LinkCreatePanel with searchable device selector and interface filtering
  - LinkDetailsPanel
  - InterfaceStatsPanel with live WebSocket data
  - SearchOverlay with hostname search support
  - Keyboard shortcuts system (ShortcutHelp)
  - Grafana deep-links with per-device URL and global fallback

- **Infrastructure**
  - Docker dev environment with hot-reload
  - Production docker-compose stack with nginx frontend
  - Vite config with WebSocket proxy

### Fixed

- API response parsing for non-array `data` fields
- SNMP API payload format and device tags alignment
- Duplicate links, text overflow, and missing hover feedback
- Add Device shortcut changed from Ctrl+N to A to avoid browser conflicts
- Grafana deep-link per-device URL with global fallback
- Background image upload feature removed (was causing issues)

### Changed

- Alert status applied immediately, metric updates deferred, alert status preserved during silent refreshes
- Port matching regex uses explicit `[0-9]` digit range instead of `\d`
- `onClose` prop renamed to `_onClose` in LinkDetailsPanel to avoid naming conflict

[Unreleased]: https://github.com/lollinoo/mikrotik-theia/compare/v0.2.0...HEAD
[0.2.0]: https://github.com/lollinoo/mikrotik-theia/compare/v0.1.0...v0.2.0
[0.1.0]: https://github.com/lollinoo/mikrotik-theia/releases/tag/v0.1.0
