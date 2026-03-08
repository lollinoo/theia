# Phase 4: Integration and Polish - Context

**Gathered:** 2026-03-08
**Status:** Ready for planning

<domain>
## Phase Boundary

Operators can drill from the topology map into Grafana for deep dives, inspect per-interface statistics, configure polling behavior, add/edit/delete devices via UI, and use keyboard shortcuts for common actions. The canvas remains performant at 100+ devices. Background image upload is supported. Prometheus alert rules drive link/device failure visuals.

</domain>

<decisions>
## Implementation Decisions

### Grafana Deep-Links
- Right-click context menu on device cards triggers Grafana navigation
- Right-click context menu also works on link edges (per-interface stats, open in Grafana)
- Convention-based dashboard URL construction (e.g., `device-{hostname}`) with per-device override option
- Context menu items: Open in Grafana, Edit Device, Per-Interface, Configure
- Links open in new browser tab

### Unified Side Panel
- Single reusable slide-out panel component from the right side
- Renders different content based on context: Interface Stats, Settings, Add Device, Device Config
- Fixed width ~320px
- Header shows content title + close button (X)
- Opening new content replaces current panel content (no stacking)
- Escape key closes panel (universal dismiss)
- Smooth slide animation

### Interface Statistics
- Shown in side panel when accessing per-interface stats from link context menu
- Displays both interfaces of the link
- Stats shown: TX/RX throughput, interface speed/duplex, admin/oper status
- Updates in real-time via existing WebSocket infrastructure
- No error/drop counters (not selected)

### Polling Configuration
- Global polling config lives in the Settings section of the side panel
- Per-device polling override via right-click device → 'Configure' → side panel
- Preset dropdown with options: 15s, 30s, 60s, 120s, 300s plus 'Custom...' for arbitrary values
- Changes auto-save with debounce (~500ms), no explicit save button
- Backend already reads polling interval dynamically each cycle

### Add Device UI
- Form in the side panel: hostname/IP, SNMP community, SNMP version (v2c/v3), optional display name
- Triggered via toolbar button or Ctrl/Cmd+N shortcut
- Submit adds device via existing REST API, device appears on canvas

### Device-Specific Config Panel
- Accessed via right-click device → 'Configure'
- Shows: polling interval override, custom Grafana dashboard URL, edit device properties (reuses add device form fields), delete device (with confirmation)

### Keyboard Shortcuts
- Ctrl/Cmd+K: Open search overlay (SearchOverlay already exists)
- Ctrl/Cmd+N: Open add device panel
- +/-/0: Zoom in, zoom out, fit-to-view (ZoomControls already exists)
- Ctrl/Cmd+,: Open settings panel
- ?: Show keyboard shortcut help overlay (list of all shortcuts, dismiss with Escape)
- Escape: Universal close/dismiss for all panels, overlays, menus

### Canvas Toolbar
- Top-right floating bar with icon buttons: search, add device, settings gear
- Icons only with tooltips showing action name + keyboard shortcut (e.g., "Search (Ctrl+K)")
- Matches existing ZoomControls style (bottom-right floating)

### Background Image
- Upload control lives in the global Settings panel
- Image renders behind nodes/links without breaking interaction

### Performance (100+ Devices)
- Rely on React Flow's built-in virtualization
- Optimize with: memoized DeviceCard/LinkEdge components, throttled WebSocket updates, reduced re-renders
- No level-of-detail rendering unless performance testing reveals issues

### Claude's Discretion
- Alert visual design for link down/degraded states (ALRT-02/03)
- Prometheus alert rule integration approach (ALRT-03)
- Context menu styling and animation
- Side panel transition animation details
- Shortcut help overlay layout
- Background image storage approach (local file vs base64 in settings)
- Form validation UX details
- Delete confirmation dialog design

</decisions>

<specifics>
## Specific Ideas

- Context menu pattern inspired by standard desktop right-click menus — clean, dark-themed, consistent with app
- Side panel similar to VS Code's sidebar width and feel
- Shortcut help overlay like GitHub/Gmail's shortcut cheat sheet (? to open)
- Toolbar tooltips should always show the keyboard shortcut alongside the action name

</specifics>

<code_context>
## Existing Code Insights

### Reusable Assets
- `SearchOverlay` component: Already handles Ctrl+K-like search, can wire to shortcut system
- `ZoomControls` component: Floating button pattern to match for toolbar
- `StatusDot` component: Visual state mapping for device status
- `useWebSocket` hook: Real-time data push, extend for interface stats
- `usePositions` hook: Position persistence pattern
- `ReconnectBanner` component: Connection state UI pattern

### Established Patterns
- React Flow for canvas rendering (custom nodes via DeviceCard, custom edges via LinkEdge)
- WebSocket for real-time metric updates with staleness detection
- Settings stored in SQLite via SettingsRepository (Get/Set/GetAll)
- Domain constants for setting keys (`SettingGrafanaURL`, `SettingPollingInterval` already defined)
- Background poller reads polling interval dynamically each cycle

### Integration Points
- `Canvas.tsx`: Main component — add toolbar, side panel, context menu, keyboard handler
- `DeviceCard.tsx`: Add right-click handler for context menu
- `LinkEdge.tsx`: Add right-click handler for context menu
- `internal/domain/settings.go`: Add new setting keys for per-device overrides
- `internal/api/router.go`: Add API endpoints for device CRUD UI, settings management
- `internal/worker/poller.go` and `metrics_collector.go`: Already support dynamic interval

</code_context>

<deferred>
## Deferred Ideas

None — discussion stayed within phase scope

</deferred>

---

*Phase: 04-integration-and-polish*
*Context gathered: 2026-03-08*
