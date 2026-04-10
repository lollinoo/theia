# Requirements: MikroTik Theia

**Defined:** 2026-04-07
**Core Value:** Network operators can see their entire topology at a glance with live stats on every device and link, drill into Grafana for deep dives, and manage devices directly — all from a single interactive map.

## v1.5.0 Requirements

Requirements for the WinBox Integration milestone. Each maps to roadmap phases (to be filled during roadmap creation).

### Credential Profiles

- [x] **CRED-01**: User can assign a custom role label to any credential profile (free-text field, e.g. "Admin", "Backup", "Read-only")
- [x] **CRED-02**: A device can have multiple credential profiles associated, one per role
- [x] **CRED-03**: User can explicitly designate one credential profile per device for WinBox access
- [x] **CRED-04**: Existing SSH profiles are automatically migrated to role "Admin" on upgrade
- [x] **CRED-05**: User can view and manage which credential profiles are assigned to a specific device

### WinBox Bridge

- [x] **BRIDGE-01**: User can download the WinBox bridge binary for their platform from Theia Settings
- [x] **BRIDGE-02**: Bridge binary is available for Windows, Linux, and macOS (amd64 + arm64, 6 targets)
- [x] **BRIDGE-03**: Bridge validates both Origin and Host headers on every request (DNS rebinding protection)
- [x] **BRIDGE-04**: Bridge is hardcoded to launch only the WinBox executable — no arbitrary process execution
- [x] **BRIDGE-05**: Frontend detects whether the bridge is running via a health check endpoint

### WinBox UI

- [x] **WINBOX-01**: User can open WinBox pre-authenticated from the canvas device context menu
- [x] **WINBOX-02**: User can open WinBox pre-authenticated from the Devices table row action
- [x] **WINBOX-03**: WinBox action is visually disabled with an explanatory tooltip when no WinBox profile is designated for the device
- [x] **WINBOX-04**: Legacy `ssh_profile_id` FK column is removed from the devices table (cleanup migration)

### WinBox Bridge System Tray

- [x] **TRAY-01**: Bridge binary shows a system tray icon when launched on a desktop system
- [x] **TRAY-02**: User can start and stop the bridge HTTP server from the system tray without restarting the binary
- [x] **TRAY-03**: User can configure the WinBox executable path via the bridge config file, accessible from the tray
- [x] **TRAY-04**: User can configure the bridge listening port via the bridge config file, accessible from the tray
- [x] **TRAY-05**: User can configure the allowed Theia origin via the bridge config file, accessible from the tray
- [x] **TRAY-06**: Bridge supports a `--no-tray` headless mode for servers without a display (starts server directly, exits on SIGINT/SIGTERM)

## Future Requirements

Deferred to future releases. Tracked but not in current roadmap.

### Advanced Profiles

- **CRED-F01**: Explicit is_backup_profile flag per profile (vs. heuristic "first non-WinBox profile") — needed if operators create WinBox-only profiles
- **CRED-F02**: Per-device profile priority ordering

### Bridge Extensibility

- **BRIDGE-F01**: Bridge support for additional MikroTik tools (e.g. Dude, The Dude replacement)
- **BRIDGE-F02**: Bridge auto-start as OS service (systemd / launchd / Windows Service)
- **BRIDGE-F03**: Bridge auto-update mechanism

## Out of Scope

Explicitly excluded. Documented to prevent scope creep.

| Feature | Reason |
|---------|--------|
| In-browser SSH terminal | High complexity, separate milestone scope |
| Role-based access control in Theia | Credential roles describe device access, not Theia user permissions |
| Support for apps other than WinBox | WinBox-only for v1.5.0; bridge hardcoded to prevent arbitrary execution |
| Bridge auto-update / auto-start | Distribution is manual download for v1.5.0 |
| Bridge HTTPS / TLS | HTTP + CORS+Host validation is the established pattern; TLS on localhost requires cert complexity |

## Traceability

Which phases cover which requirements. Updated during roadmap creation.

| Requirement | Phase | Status |
|-------------|-------|--------|
| CRED-01 | Phase 23 | Complete |
| CRED-02 | Phase 23 | Complete |
| CRED-04 | Phase 23 | Complete |
| CRED-03 | Phase 24 | Complete |
| CRED-05 | Phase 24 | Complete |
| BRIDGE-01 | Phase 24 | Complete |
| BRIDGE-02 | Phase 24 | Complete |
| WINBOX-01 | Phase 31 | Complete |
| WINBOX-02 | Phase 31 | Complete |
| WINBOX-03 | Phase 25 | Complete |
| BRIDGE-05 | Phase 31 | Complete |
| BRIDGE-03 | Phase 26 | Complete |
| BRIDGE-04 | Phase 26 | Complete |
| WINBOX-04 | Phase 27 | Complete |
| TRAY-01 | Phase 29 | Complete |
| TRAY-02 | Phase 29 | Complete |
| TRAY-03 | Phase 29 | Complete |
| TRAY-04 | Phase 31 | Complete |
| TRAY-05 | Phase 29 | Complete |
| TRAY-06 | Phase 29 | Complete |

**Coverage:**
- v1.5.0 requirements: 20 total
- Mapped to phases: 20 (100%) ✓
- Unmapped: 0 ✓

---
*Requirements defined: 2026-04-07*
*Last updated: 2026-04-07 after roadmap creation*
