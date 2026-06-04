import { act, fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import { useEffect } from 'react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import App from './App';
import type { Area, CanvasMap, Device, Link } from './types/api';
import type { DeviceRuntimeDTO, SnapshotPayload } from './types/metrics';

const fetchAreasMock = vi.fn<() => Promise<Area[]>>();
const fetchCanvasMapsMock = vi.fn<() => Promise<CanvasMap[]>>();
const createCanvasMapMock =
  vi.fn<
    (payload: {
      name: string;
      description?: string;
      source_area_id?: string | null;
      source_map_id?: string | null;
      filter?: CanvasMap['filter'];
    }) => Promise<CanvasMap>
  >();
const duplicateCanvasMapMock =
  vi.fn<(id: string, payload: { name: string }) => Promise<CanvasMap>>();
const updateCanvasMapMock = vi.fn<(id: string, payload: { name: string }) => Promise<CanvasMap>>();
const deleteCanvasMapMock = vi.fn<(id: string) => Promise<void>>();
const setCanvasMapPrimaryMock = vi.fn<(id: string) => Promise<CanvasMap>>();
const useWebSocketMock = vi.fn();
const watermarkPropsMock = vi.hoisted(() => vi.fn());
const adminDashboardPropsMock = vi.hoisted(() => vi.fn());
const hasPermissionMock = vi.hoisted(() => vi.fn());
const reactActWarningPattern = /not wrapped in act/;
const originalConsoleError = console.error;
let appActWarnings: unknown[][] = [];
let consoleErrorSpy: ReturnType<typeof vi.spyOn>;

vi.mock('./api/client', () => ({
  fetchAreas: () => fetchAreasMock(),
  fetchCanvasMaps: () => fetchCanvasMapsMock(),
  createCanvasMap: (...args: Parameters<typeof createCanvasMapMock>) =>
    createCanvasMapMock(...args),
  duplicateCanvasMap: (...args: Parameters<typeof duplicateCanvasMapMock>) =>
    duplicateCanvasMapMock(...args),
  updateCanvasMap: (...args: Parameters<typeof updateCanvasMapMock>) =>
    updateCanvasMapMock(...args),
  deleteCanvasMap: (...args: Parameters<typeof deleteCanvasMapMock>) =>
    deleteCanvasMapMock(...args),
  setCanvasMapPrimary: (...args: Parameters<typeof setCanvasMapPrimaryMock>) =>
    setCanvasMapPrimaryMock(...args),
}));

vi.mock('./hooks/useWebSocket', () => ({
  useWebSocket: (...args: unknown[]) => useWebSocketMock(...args),
}));

vi.mock('./contexts/ThemeContext', () => ({
  ThemeProvider: ({ children }: { children: React.ReactNode }) => <>{children}</>,
}));

vi.mock('./contexts/AuthContext', () => ({
  useAuth: () => ({
    status: 'authenticated',
    user: null,
    error: null,
    refresh: vi.fn(),
    login: vi.fn(),
    logout: vi.fn(),
    changePassword: vi.fn(),
    hasPermission: hasPermissionMock,
  }),
}));

vi.mock('@xyflow/react', () => ({
  ReactFlowProvider: ({ children }: { children: React.ReactNode }) => <>{children}</>,
}));

vi.mock('./components/Watermark', () => ({
  Watermark: (props: { compact?: boolean; hidden?: boolean; mapName?: string }) => {
    watermarkPropsMock(props);
    return (
      <div
        data-testid="watermark"
        data-compact={String(props.compact ?? false)}
        data-hidden={String(props.hidden ?? false)}
        data-map-name={props.mapName ?? ''}
      />
    );
  },
}));

vi.mock('./components/NavigationPill', () => ({
  default: ({
    areas,
    maps,
    selectedMapId,
    selectedMapName,
    onViewChange,
    onAreaSelect,
    onMapSelect,
    onManageMaps,
    canViewAdmin,
    userLabel,
  }: {
    areas: Area[];
    maps: CanvasMap[];
    selectedMapId: string | null;
    selectedMapName: string;
    canViewAdmin: boolean;
    userLabel: string;
    onViewChange: (view: 'hub' | 'canvas' | 'dashboard' | 'admin') => void;
    onAreaSelect: (areaId: string | null) => void;
    onMapSelect: (map: CanvasMap) => void;
    onManageMaps: () => void;
  }) => (
    <div data-testid="navigation-pill">
      <span>{`pill-map:${selectedMapId ?? 'default'}:${selectedMapName}:${maps.length}`}</span>
      <span>{`pill-areas:${areas.map((area) => area.name).join('|')}`}</span>
      <span>{`pill-user:${userLabel}`}</span>
      <button type="button" onClick={() => onViewChange('hub')}>
        Hub
      </button>
      <button type="button" onClick={() => onViewChange('dashboard')}>
        Dashboard
      </button>
      {canViewAdmin && (
        <button type="button" onClick={() => onViewChange('admin')}>
          Admin
        </button>
      )}
      <button
        type="button"
        onClick={() =>
          onMapSelect({
            id: 'map-1',
            name: 'Backbone',
            description: '',
            source_area_id: 'area-1',
            filter: { area_id: 'area-1' },
            is_default: false,
            device_count: 1,
            link_count: 1,
            position_count: 1,
            created_at: '2026-01-01T00:00:00Z',
            updated_at: '2026-01-02T00:00:00Z',
          })
        }
      >
        Pill Open Backbone map
      </button>
      <button type="button" onClick={() => onAreaSelect(areas[0]?.id ?? 'area-1')}>
        Pill area Backbone
      </button>
      <button type="button" onClick={() => onAreaSelect(null)}>
        Pill all areas
      </button>
      <button type="button" onClick={onManageMaps}>
        Pill Manage maps
      </button>
    </div>
  ),
}));

vi.mock('./components/Canvas', () => ({
  default: ({
    mapId,
    mapName,
    selectedAreaId,
    fitViewRevision,
    topologyRefreshRevision,
    chromeHidden,
    onChromeHiddenChange,
    onDevicesChange,
    onLinksChange,
    onTopologyAreasChange,
    onInteractionActiveChange,
  }: {
    mapId: string | null;
    mapName: string;
    selectedAreaId: string | null;
    fitViewRevision?: number;
    topologyRefreshRevision?: number;
    chromeHidden?: boolean;
    onChromeHiddenChange?: (hidden: boolean) => void;
    onDevicesChange: (devices: Device[]) => void;
    onLinksChange: (links: Link[]) => void;
    onTopologyAreasChange: (areas: Area[]) => void;
    onInteractionActiveChange: (active: boolean) => void;
  }) => {
    useEffect(() => {
      onDevicesChange([
        {
          id: 'dev-1',
          hostname: 'router-01',
          ip: '10.0.0.1',
          device_type: 'router',
          poll_class: 'standard',
          poll_interval_override: null,
          polling_enabled: true,
          status: 'up',
          sys_name: 'router-01',
          sys_descr: 'RouterOS 7.15.1',
          hardware_model: 'RB5009',
          vendor: 'mikrotik',
          managed: true,
          interfaces: [],
          area_ids: ['area-1'],
          backup_supported: true,
          metrics_source: 'snmp',
          prometheus_label_name: 'instance',
          prometheus_label_value: '10.0.0.1:9100',
        },
      ]);
      onLinksChange([
        {
          id: 'link-1',
          source_device_id: 'dev-1',
          source_if_name: 'ether1',
          target_device_id: 'dev-2',
          target_if_name: 'ether2',
          discovery_protocol: 'lldp',
          source_if_speed: 1,
          source_if_oper_status: 'up',
          target_if_speed: 1,
          target_if_oper_status: 'up',
        },
      ]);
      onTopologyAreasChange(
        mapId?.startsWith('default-map')
          ? [
              {
                id: 'area-1',
                name: 'Backbone',
                description: 'Default map area',
                color: '#00E676',
                device_count: 1,
                created_at: '2026-01-01T00:00:00Z',
                updated_at: '2026-01-01T00:00:00Z',
              },
            ]
          : [
              {
                id: 'map-area-1',
                name: 'Map Local Area',
                description: 'Map scoped area',
                color: '#2979FF',
                device_count: 1,
                created_at: '2026-01-01T00:00:00Z',
                updated_at: '2026-01-01T00:00:00Z',
              },
            ],
      );
    }, [mapId, mapName, onDevicesChange, onLinksChange, onTopologyAreasChange]);

    useEffect(() => {
      if ((topologyRefreshRevision ?? 0) === 0) {
        return;
      }

      onTopologyAreasChange([
        {
          id: 'area-hub',
          name: 'Hub Area',
          description: 'Refreshed map area',
          color: '#123ABC',
          device_count: 1,
          created_at: '2026-01-01T00:00:00Z',
          updated_at: '2026-01-01T00:00:00Z',
        },
      ]);
    }, [onTopologyAreasChange, topologyRefreshRevision]);

    return (
      <div data-testid="canvas">
        <span>{`map:${mapId ?? 'default'}:${mapName}`}</span>
        <span>{`area:${selectedAreaId ?? 'all'}`}</span>
        <span>{`fit:${fitViewRevision ?? 0}`}</span>
        <span>{`refresh:${topologyRefreshRevision ?? 0}`}</span>
        <span>{`chrome:${chromeHidden ? 'hidden' : 'visible'}`}</span>
        <button type="button" onClick={() => onChromeHiddenChange?.(!chromeHidden)}>
          Toggle canvas chrome
        </button>
        <button type="button" onClick={() => onInteractionActiveChange(true)}>
          Start interaction
        </button>
        <button type="button" onClick={() => onInteractionActiveChange(false)}>
          End interaction
        </button>
      </div>
    );
  },
}));

vi.mock('./components/topology-hub/TopologyHub', () => ({
  default: ({
    devices,
    areas,
    links,
    snapshot,
    maps,
    selectedMapId,
    selectedMapName,
    mapsLoading,
    mapsError,
    savedMapsEnabled,
    onOpenArea,
    onSelectMap,
    onOpenMap,
    onRenameMap,
    onDeleteMap,
    onSetPrimaryMap,
    onCreateEmptyMap,
    onCreateMapFromArea,
    onAreasChange,
  }: {
    devices: Device[];
    areas: Area[];
    links: Link[];
    snapshot: SnapshotPayload | null;
    maps: CanvasMap[];
    selectedMapId: string | null;
    selectedMapName: string;
    mapsLoading: boolean;
    mapsError: string | null;
    savedMapsEnabled: boolean;
    onOpenArea: (areaId: string) => void;
    onSelectMap: (map: CanvasMap) => void;
    onOpenMap: (map: CanvasMap) => void;
    onRenameMap?: (map: CanvasMap) => void;
    onDeleteMap: (map: CanvasMap) => void;
    onSetPrimaryMap: (map: CanvasMap) => void;
    onCreateEmptyMap: () => void;
    onCreateMapFromArea: (area: Area) => void;
    onAreasChange?: () => void | Promise<void>;
  }) => {
    const selectedMap =
      maps.find((map) => map.id === selectedMapId) ??
      maps.find((map) => map.is_default && selectedMapId === null) ??
      maps[0];

    return (
      <div data-testid="topology-hub">
        <span>{`devices:${devices.length}`}</span>
        <span>{`hub-areas:${areas.map((area) => area.name).join('|')}`}</span>
        <span>{`links:${links.length}`}</span>
        <span>{`snapshot:${snapshot?.devices['dev-1']?.operational_status ?? 'none'}`}</span>
        <span>{`maps:${maps.length}`}</span>
        <span>{`selected-map:${selectedMapId ?? 'none'}:${selectedMapName}`}</span>
        <span>{`loading:${String(mapsLoading)}`}</span>
        <span>{`error:${mapsError ?? 'none'}`}</span>
        <span>{`savedMapsEnabled:${String(savedMapsEnabled)}`}</span>
        <button
          type="button"
          onClick={() => {
            if (selectedMap) onOpenMap(selectedMap);
          }}
        >
          Open selected map
        </button>
        <button type="button" onClick={onCreateEmptyMap}>
          Create empty map
        </button>
        {onAreasChange && (
          <button
            type="button"
            onClick={() => {
              void onAreasChange();
            }}
          >
            Refresh map areas
          </button>
        )}
        {maps.map((map) => (
          <div key={map.id}>
            <button type="button" onClick={() => onSelectMap(map)}>
              {`Select hub map ${map.name}`}
            </button>
            <button type="button" onClick={() => onDeleteMap(map)}>
              {`Delete map ${map.name}`}
            </button>
            {onRenameMap && (
              <button type="button" onClick={() => onRenameMap(map)}>
                {`Rename map ${map.name}`}
              </button>
            )}
            {!map.is_default && (
              <button type="button" onClick={() => onSetPrimaryMap(map)}>
                {`Set primary map ${map.name}`}
              </button>
            )}
          </div>
        ))}
        {areas.map((area) => (
          <div key={area.id}>
            <button type="button" onClick={() => onOpenArea(area.id)}>
              {`Open area ${area.name}`}
            </button>
            <button type="button" onClick={() => onCreateMapFromArea(area)}>
              {`Create map from area ${area.name}`}
            </button>
          </div>
        ))}
      </div>
    );
  },
}));

vi.mock('./components/AdminDashboard', () => ({
  AdminDashboard: (props: { visible?: boolean }) => {
    adminDashboardPropsMock(props);
    return <div data-testid="admin-dashboard">Admin</div>;
  },
}));

vi.mock('./components/Dashboard', () => ({
  Dashboard: ({
    devices,
    areas,
    selectedAreaId,
    snapshot,
    onOpenMap,
  }: {
    devices: Device[];
    areas: Area[];
    selectedAreaId?: string | null;
    snapshot: SnapshotPayload | null;
    onOpenMap?: () => void;
  }) => (
    <div data-testid="dashboard">
      <span>{`devices:${devices.length}`}</span>
      <span>{`dashboard-areas:${areas.map((area) => area.name).join('|')}`}</span>
      <span>{`selected-area:${selectedAreaId ?? 'all'}`}</span>
      <span>{`status:${snapshot?.devices['dev-1']?.operational_status ?? 'none'}`}</span>
      <button type="button" onClick={onOpenMap}>
        Open map
      </button>
    </div>
  ),
}));

function mockArea(overrides: Partial<Area> = {}): Area {
  return {
    id: 'area-1',
    name: 'Backbone',
    description: 'Core',
    color: '#00E676',
    device_count: 1,
    created_at: '2026-01-01T00:00:00Z',
    updated_at: '2026-01-01T00:00:00Z',
    ...overrides,
  };
}

function mockMap(overrides: Partial<CanvasMap> = {}): CanvasMap {
  return {
    id: 'map-1',
    name: 'Backbone',
    description: '',
    source_area_id: 'area-1',
    filter: { area_id: 'area-1' },
    is_default: false,
    device_count: 1,
    link_count: 1,
    position_count: 1,
    created_at: '2026-01-01T00:00:00Z',
    updated_at: '2026-01-02T00:00:00Z',
    ...overrides,
  };
}

// mockRuntimeDevice returns a complete runtime DTO so App mocks stay type-aligned with WebSocket data.
function mockRuntimeDevice(overrides: Partial<DeviceRuntimeDTO> = {}): DeviceRuntimeDTO {
  return {
    device_id: 'dev-1',
    operational_status: 'down',
    primary_health: 'unreachable',
    runtime_flags: [],
    field_states: { uptime: 'ok', cpu: 'ok', memory: 'ok' },
    network_reachable: 'false',
    snmp_reachable: 'false',
    reachability: 'hard_down',
    health: 'critical',
    freshness: 'fresh',
    primary_reason: 'device_unreachable',
    metrics_status: 'unavailable',
    metrics_reason: 'device_unreachable',
    alert_status: 'down',
    firing_alert_count: 1,
    last_collected_at: null,
    last_polled_at: null,
    expected_poll_interval_seconds: null,
    cpu_percent: null,
    mem_percent: null,
    temp_celsius: null,
    uptime_secs: null,
    ...overrides,
  };
}

// mockSnapshot creates the minimal complete runtime snapshot used by App navigation tests.
function mockSnapshot(overrides: Partial<SnapshotPayload> = {}): SnapshotPayload {
  return {
    devices: { 'dev-1': mockRuntimeDevice() },
    links: {},
    ...overrides,
  };
}

// renderApp waits for App's initial async fetch/effect updates so tests do not leak act warnings.
async function renderApp() {
  const result = render(<App />);
  await waitFor(() => expect(fetchAreasMock).toHaveBeenCalled());
  return result;
}

// clickButton keeps App-level event updates and follow-up effects inside React's async act boundary.
async function clickButton(name: string) {
  await act(async () => {
    fireEvent.click(screen.getByRole('button', { name }));
  });
}

describe('App', () => {
  beforeEach(() => {
    appActWarnings = [];
    consoleErrorSpy = vi.spyOn(console, 'error').mockImplementation((...args: unknown[]) => {
      if (args.some((arg) => typeof arg === 'string' && reactActWarningPattern.test(arg))) {
        appActWarnings.push(args);
        return;
      }

      originalConsoleError(...args);
    });
    window.localStorage.clear();
    fetchAreasMock.mockReset();
    fetchCanvasMapsMock.mockReset();
    createCanvasMapMock.mockReset();
    duplicateCanvasMapMock.mockReset();
    updateCanvasMapMock.mockReset();
    deleteCanvasMapMock.mockReset();
    setCanvasMapPrimaryMock.mockReset();
    useWebSocketMock.mockReset();
    watermarkPropsMock.mockClear();
    adminDashboardPropsMock.mockClear();
    hasPermissionMock.mockReset();
    hasPermissionMock.mockReturnValue(false);
    fetchAreasMock.mockResolvedValue([mockArea()]);
    fetchCanvasMapsMock.mockResolvedValue([]);
    createCanvasMapMock.mockResolvedValue(mockMap());
    duplicateCanvasMapMock.mockResolvedValue(mockMap({ id: 'map-copy', name: 'Backbone Copy' }));
    updateCanvasMapMock.mockResolvedValue(mockMap({ id: 'map-1', name: 'Backbone Renamed' }));
    deleteCanvasMapMock.mockResolvedValue(undefined);
    setCanvasMapPrimaryMock.mockResolvedValue(mockMap({ is_default: true }));
    useWebSocketMock.mockReturnValue({
      snapshot: mockSnapshot(),
      alerts: [],
      reconnecting: false,
      prometheusStatus: null,
    });
  });

  afterEach(() => {
    const warnings = [...appActWarnings];
    consoleErrorSpy.mockRestore();
    // App warnings are noisy in CI, so this suite fails immediately when a test leaks one.
    expect(warnings).toEqual([]);
  });

  it('passes visibility state to the mounted admin dashboard', async () => {
    hasPermissionMock.mockImplementation(
      (permission: string) => permission === 'admin:dashboard:read',
    );

    await renderApp();

    fireEvent.click(await screen.findByRole('button', { name: 'Admin' }));
    await waitFor(() => {
      expect(adminDashboardPropsMock).toHaveBeenLastCalledWith({ visible: true });
    });
    await clickButton('Dashboard');
    await waitFor(() => {
      expect(adminDashboardPropsMock).toHaveBeenLastCalledWith({ visible: false });
    });
  });

  it('uses the loaded default saved map id instead of the legacy global map context', async () => {
    const defaultMap = mockMap({
      id: 'default-map-id',
      name: 'Default',
      source_area_id: null,
      filter: {},
      is_default: true,
    });
    fetchCanvasMapsMock.mockResolvedValue([defaultMap]);

    await renderApp();

    await waitFor(() =>
      expect(screen.getByTestId('navigation-pill')).toHaveTextContent(
        'pill-map:default-map-id:Default',
      ),
    );
    expect(screen.getByTestId('canvas')).toHaveTextContent('map:default-map-id:Default');
  });

  it('lets the canvas hide and restore the navigation pill while keeping a visible restore control', async () => {
    await renderApp();

    await waitFor(() => expect(screen.getByTestId('canvas')).toBeInTheDocument());

    expect(screen.getByTestId('navigation-pill')).toBeInTheDocument();
    expect(screen.getByTestId('canvas')).toHaveTextContent('chrome:visible');
    await clickButton('Toggle canvas chrome');

    expect(screen.queryByTestId('navigation-pill')).not.toBeInTheDocument();
    expect(screen.getByTestId('canvas')).toHaveTextContent('chrome:hidden');
    expect(screen.getByTestId('canvas')).toHaveTextContent('fit:0');
    expect(screen.getByTestId('watermark')).toHaveAttribute('data-compact', 'true');
    await clickButton('Toggle canvas chrome');

    expect(screen.getByTestId('navigation-pill')).toBeInTheDocument();
    expect(screen.getByTestId('canvas')).toHaveTextContent('chrome:visible');
    expect(screen.getByTestId('watermark')).toHaveAttribute('data-compact', 'false');
  });

  it('passes the selected map name to the canvas watermark', async () => {
    fetchCanvasMapsMock.mockResolvedValue([
      mockMap({
        id: 'default-map-id',
        name: 'Primary Ops',
        source_area_id: null,
        filter: {},
        is_default: true,
      }),
    ]);

    await renderApp();

    await waitFor(() =>
      expect(screen.getByTestId('watermark')).toHaveAttribute('data-map-name', 'Primary Ops'),
    );
  });

  it('does not mount the legacy global canvas before saved maps resolve', async () => {
    let resolveMaps: (maps: CanvasMap[]) => void = () => {};
    fetchCanvasMapsMock.mockReturnValue(
      new Promise<CanvasMap[]>((resolve) => {
        resolveMaps = resolve;
      }),
    );

    await renderApp();

    expect(screen.getByTestId('navigation-pill')).toHaveTextContent('pill-map:default:Default');
    expect(screen.queryByTestId('canvas')).not.toBeInTheDocument();

    await act(async () => {
      resolveMaps([
        mockMap({
          id: 'primary-map-id',
          name: 'Primary Ops',
          source_area_id: null,
          filter: {},
          is_default: true,
        }),
      ]);
    });

    expect(await screen.findByTestId('canvas')).toHaveTextContent('map:primary-map-id:Primary Ops');
  });

  it('uses the promoted primary saved map on refresh instead of the old Default map', async () => {
    fetchCanvasMapsMock.mockResolvedValue([
      mockMap({
        id: 'old-default-map',
        name: 'Default',
        source_area_id: null,
        filter: {},
        is_default: false,
      }),
      mockMap({
        id: 'primary-ops-map',
        name: 'Primary Ops',
        source_area_id: null,
        filter: {},
        is_default: true,
      }),
    ]);

    await renderApp();

    await waitFor(() =>
      expect(screen.getByTestId('navigation-pill')).toHaveTextContent(
        'pill-map:primary-ops-map:Primary Ops',
      ),
    );
    expect(screen.getByTestId('canvas')).toHaveTextContent('map:primary-ops-map:Primary Ops');
  });

  it('wires canvas devices links and snapshot into TopologyHub and Dashboard', async () => {
    await renderApp();

    await waitFor(() => expect(fetchAreasMock).toHaveBeenCalled());
    await clickButton('Hub');
    await waitFor(() => expect(fetchCanvasMapsMock).toHaveBeenCalled());
    expect(await screen.findByTestId('topology-hub')).toHaveTextContent('devices:1');
    expect(screen.getByTestId('topology-hub')).toHaveTextContent('links:1');
    expect(screen.getByTestId('topology-hub')).toHaveTextContent('snapshot:down');
    expect(screen.getByTestId('topology-hub')).toHaveTextContent('maps:0');
    expect(screen.getByTestId('topology-hub')).toHaveTextContent('savedMapsEnabled:true');
    expect(createCanvasMapMock).not.toHaveBeenCalled();
    expect(duplicateCanvasMapMock).not.toHaveBeenCalled();
    expect(deleteCanvasMapMock).not.toHaveBeenCalled();
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument();
    await clickButton('Dashboard');
    expect(await screen.findByTestId('dashboard')).toHaveTextContent('devices:1');
    expect(screen.getByTestId('dashboard')).toHaveTextContent('status:down');
  });

  it('uses cached saved maps when opening the hub instead of starting a blocking refetch', async () => {
    const defaultMap = mockMap({
      id: 'default-map-id',
      name: 'Default',
      source_area_id: null,
      filter: {},
      is_default: true,
    });
    fetchCanvasMapsMock.mockResolvedValue([defaultMap]);

    await renderApp();

    await waitFor(() =>
      expect(screen.getByTestId('navigation-pill')).toHaveTextContent(
        'pill-map:default-map-id:Default',
      ),
    );
    expect(fetchCanvasMapsMock).toHaveBeenCalledTimes(1);
    await clickButton('Hub');

    expect(await screen.findByTestId('topology-hub')).toHaveTextContent('maps:1');
    expect(screen.getByTestId('topology-hub')).toHaveTextContent('loading:false');
    expect(fetchCanvasMapsMock).toHaveBeenCalledTimes(1);
  });

  it('keeps the canvas mounted and opacity-masked while the hub is active', async () => {
    await renderApp();

    await waitFor(() => expect(fetchAreasMock).toHaveBeenCalled());
    await clickButton('Hub');

    const canvasViewport = screen.getByTestId('canvas').parentElement;
    expect(canvasViewport).not.toHaveClass('hidden');
    expect(canvasViewport).toHaveClass('opacity-0');
    expect(canvasViewport).not.toHaveClass('invisible');
    expect(canvasViewport).toHaveAttribute('inert');
    expect(canvasViewport?.className).toContain('absolute');
    expect(canvasViewport?.className).toContain('inset-0');
  });

  it('passes map-local areas into Devices instead of global areas', async () => {
    await renderApp();

    await waitFor(() => expect(fetchAreasMock).toHaveBeenCalled());
    await waitFor(() =>
      expect(screen.getByTestId('navigation-pill')).toHaveTextContent('pill-areas:Map Local Area'),
    );
    await clickButton('Dashboard');

    expect(await screen.findByTestId('dashboard')).toHaveTextContent(
      'dashboard-areas:Map Local Area',
    );
    expect(screen.getByTestId('dashboard')).not.toHaveTextContent('dashboard-areas:Backbone');
  });

  it('refreshes selected map-local areas after Topology Hub area changes', async () => {
    const defaultMap = mockMap({
      id: 'default-map-id',
      name: 'Default',
      source_area_id: null,
      filter: {},
      is_default: true,
    });
    fetchCanvasMapsMock.mockResolvedValue([defaultMap]);

    await renderApp();

    await waitFor(() =>
      expect(screen.getByTestId('navigation-pill')).toHaveTextContent(
        'pill-map:default-map-id:Default',
      ),
    );
    await clickButton('Hub');

    await waitFor(() =>
      expect(screen.getByTestId('topology-hub')).toHaveTextContent('hub-areas:Backbone'),
    );
    await clickButton('Refresh map areas');

    await waitFor(() => expect(screen.getByTestId('canvas')).toHaveTextContent('refresh:1'));
    await waitFor(() =>
      expect(screen.getByTestId('topology-hub')).toHaveTextContent('hub-areas:Hub Area'),
    );
  });

  it('keeps Devices active when selecting a map from the navigation pill', async () => {
    await renderApp();

    await waitFor(() => expect(fetchAreasMock).toHaveBeenCalled());
    await clickButton('Dashboard');
    expect(await screen.findByTestId('dashboard')).toHaveTextContent('devices:1');
    await clickButton('Pill Open Backbone map');

    expect(screen.getByTestId('navigation-pill')).toHaveTextContent('pill-map:map-1:Backbone');
    expect(screen.getByTestId('dashboard').parentElement?.className).toContain('h-full');
    expect(screen.getByTestId('canvas').parentElement).toHaveClass('opacity-0');
  });

  it('opens the canvas and requests fit view when selecting a map from the hub navigation pill', async () => {
    await renderApp();

    await waitFor(() => expect(fetchAreasMock).toHaveBeenCalled());
    await clickButton('Hub');
    expect(screen.getByTestId('topology-hub').parentElement?.className).toContain('h-full');
    await clickButton('Pill Open Backbone map');

    expect(screen.getByTestId('canvas')).toHaveTextContent('map:map-1:Backbone');
    expect(screen.getByTestId('canvas')).toHaveTextContent('fit:1');
    expect(screen.getByTestId('canvas').parentElement?.className).toContain('h-full');
    expect(screen.getByTestId('canvas').parentElement).toHaveClass('opacity-100');
    expect(screen.getByTestId('topology-hub').parentElement).toHaveClass('opacity-0');
  });

  it('opens the selected map-local area in Canvas from the navigation pill', async () => {
    await renderApp();

    await waitFor(() => expect(fetchAreasMock).toHaveBeenCalled());
    await waitFor(() =>
      expect(screen.getByTestId('navigation-pill')).toHaveTextContent('pill-areas:Map Local Area'),
    );
    await clickButton('Dashboard');
    await clickButton('Pill area Backbone');

    expect(screen.getByTestId('canvas')).toHaveTextContent('area:map-area-1');
    expect(screen.getByTestId('canvas').parentElement?.className).toContain('h-full');
    expect(screen.getByTestId('canvas').parentElement).toHaveClass('opacity-100');
    expect(screen.getByTestId('dashboard').parentElement).toHaveClass('opacity-0');
  });

  it('returns from Devices to the currently selected map', async () => {
    await renderApp();

    await waitFor(() => expect(fetchAreasMock).toHaveBeenCalled());
    await clickButton('Dashboard');
    await clickButton('Pill Open Backbone map');
    expect(screen.getByTestId('dashboard').parentElement?.className).toContain('h-full');
    await clickButton('Open map');

    expect(screen.getByTestId('canvas')).toHaveTextContent('map:map-1:Backbone');
    expect(screen.getByTestId('canvas').parentElement?.className).toContain('h-full');
    expect(screen.getByTestId('canvas').parentElement).toHaveClass('opacity-100');
    expect(screen.getByTestId('dashboard').parentElement).toHaveClass('opacity-0');
  });

  it('keeps websocket runtime updates paused briefly after canvas interaction ends', async () => {
    await renderApp();

    await waitFor(() => expect(fetchAreasMock).toHaveBeenCalled());
    expect(useWebSocketMock).toHaveBeenLastCalledWith(
      '/api/v1/ws',
      null,
      expect.objectContaining({ runtimeUpdatesPaused: false }),
    );

    vi.useFakeTimers();
    try {
      await clickButton('Start interaction');
      expect(useWebSocketMock).toHaveBeenLastCalledWith(
        '/api/v1/ws',
        null,
        expect.objectContaining({ runtimeUpdatesPaused: true }),
      );
      await clickButton('End interaction');
      expect(useWebSocketMock).toHaveBeenLastCalledWith(
        '/api/v1/ws',
        null,
        expect.objectContaining({ runtimeUpdatesPaused: true }),
      );

      act(() => {
        vi.advanceTimersByTime(1499);
      });
      expect(useWebSocketMock).toHaveBeenLastCalledWith(
        '/api/v1/ws',
        null,
        expect.objectContaining({ runtimeUpdatesPaused: true }),
      );

      act(() => {
        vi.advanceTimersByTime(1);
      });
      expect(useWebSocketMock).toHaveBeenLastCalledWith(
        '/api/v1/ws',
        null,
        expect.objectContaining({ runtimeUpdatesPaused: false }),
      );
    } finally {
      vi.useRealTimers();
    }
  });

  it('keeps the canvas watermark visible while canvas interaction pauses runtime updates', async () => {
    await renderApp();

    await waitFor(() => expect(fetchAreasMock).toHaveBeenCalled());
    await clickButton('Start interaction');

    expect(screen.getByTestId('watermark')).toHaveAttribute('data-hidden', 'false');
    expect(watermarkPropsMock.mock.lastCall?.[0]).not.toHaveProperty('hidden');
  });

  it('anchors the canvas watermark inside the canvas viewport wrapper', async () => {
    await renderApp();

    await waitFor(() => expect(fetchAreasMock).toHaveBeenCalled());

    const canvasViewport = screen.getByTestId('watermark').parentElement;
    expect(canvasViewport?.className).toContain('absolute');
    expect(canvasViewport?.className).toContain('inset-0');
    expect(canvasViewport?.className).toContain('h-full');
  });

  it('passes selected map props from the navigation pill to Canvas', async () => {
    await renderApp();

    await waitFor(() => expect(fetchAreasMock).toHaveBeenCalled());
    expect(screen.getByTestId('canvas')).toHaveTextContent('map:default:Default');
    await clickButton('Pill Open Backbone map');
    expect(screen.getByTestId('canvas')).toHaveTextContent('map:map-1:Backbone');
    await clickButton('Pill Manage maps');
    expect(screen.getByTestId('topology-hub').parentElement?.className).toContain('h-full');
    expect(screen.getByTestId('canvas').parentElement).toHaveClass('opacity-0');
  });

  it('requests fit view when selecting a map from the navigation pill on the canvas', async () => {
    await renderApp();

    await waitFor(() => expect(fetchAreasMock).toHaveBeenCalled());
    expect(screen.getByTestId('canvas')).toHaveTextContent('fit:0');
    await clickButton('Pill Open Backbone map');

    expect(screen.getByTestId('canvas')).toHaveTextContent('map:map-1:Backbone');
    expect(screen.getByTestId('canvas')).toHaveTextContent('fit:1');
    expect(screen.getByTestId('canvas').parentElement?.className).toContain('h-full');
    expect(screen.getByTestId('canvas').parentElement).toHaveClass('opacity-100');
  });

  it('lets the navigation pill select maps and filter areas without leaving the selected map', async () => {
    await renderApp();

    await waitFor(() => expect(fetchAreasMock).toHaveBeenCalled());
    await waitFor(() =>
      expect(screen.getByTestId('navigation-pill')).toHaveTextContent('pill-areas:Map Local Area'),
    );
    await clickButton('Pill Open Backbone map');
    expect(screen.getByTestId('canvas')).toHaveTextContent('map:map-1:Backbone');
    expect(screen.getByTestId('navigation-pill')).toHaveTextContent('pill-map:map-1:Backbone');
    await clickButton('Pill area Backbone');
    expect(screen.getByTestId('canvas')).toHaveTextContent('map:map-1:Backbone');
    await clickButton('Pill all areas');
    expect(screen.getByTestId('canvas')).toHaveTextContent('map:map-1:Backbone');
  });

  it('creates a blank saved map from the hub empty-map action', async () => {
    const createdMap = mockMap({
      id: 'map-empty',
      name: 'Blank Map',
      source_area_id: null,
      filter: {},
      device_count: 0,
      link_count: 0,
      position_count: 0,
    });
    createCanvasMapMock.mockResolvedValue(createdMap);
    fetchCanvasMapsMock.mockResolvedValue([]);

    await renderApp();

    await waitFor(() => expect(fetchAreasMock).toHaveBeenCalled());
    await clickButton('Hub');
    await waitFor(() => expect(fetchCanvasMapsMock).toHaveBeenCalled());
    await clickButton('Create empty map');
    fireEvent.change(await screen.findByLabelText('Map name'), {
      target: { value: 'Blank Map' },
    });
    await clickButton('Create map');

    await waitFor(() =>
      expect(createCanvasMapMock).toHaveBeenCalledWith({
        name: 'Blank Map',
        source_area_id: null,
        filter: {},
      }),
    );
    expect(await screen.findByText('map:map-empty:Blank Map')).toBeInTheDocument();
    expect(screen.getByTestId('navigation-pill')).toHaveTextContent('pill-map:map-empty:Blank Map');
  });

  it('uses a dedicated delete map dialog without refetching maps after confirmed deletion', async () => {
    const confirmSpy = vi.spyOn(window, 'confirm').mockReturnValue(false);
    const savedMap = mockMap({ id: 'map-delete', name: 'Branch' });
    fetchCanvasMapsMock.mockResolvedValue([savedMap]);

    await renderApp();

    await waitFor(() => expect(fetchAreasMock).toHaveBeenCalled());
    await clickButton('Hub');
    await waitFor(() => expect(fetchCanvasMapsMock).toHaveBeenCalled());
    const fetchCountBeforeDelete = fetchCanvasMapsMock.mock.calls.length;
    await clickButton('Delete map Branch');

    expect(confirmSpy).not.toHaveBeenCalled();
    const dialog = await screen.findByRole('dialog', { name: 'Delete map' });
    expect(dialog).toHaveTextContent('Branch');

    fireEvent.click(within(dialog).getByRole('button', { name: 'Delete map' }));

    await waitFor(() => expect(deleteCanvasMapMock).toHaveBeenCalledWith('map-delete'));
    expect(fetchCanvasMapsMock).toHaveBeenCalledTimes(fetchCountBeforeDelete);
    expect(screen.queryByRole('dialog', { name: 'Delete map' })).not.toBeInTheDocument();
  });

  it('renames the selected saved map from a dedicated dialog and updates local selection', async () => {
    const branchMap = mockMap({ id: 'map-rename', name: 'Branch' });
    const renamedMap = { ...branchMap, name: 'Branch Renamed' };
    fetchCanvasMapsMock.mockResolvedValue([branchMap]);
    updateCanvasMapMock.mockResolvedValue(renamedMap);

    await renderApp();

    await waitFor(() => expect(fetchAreasMock).toHaveBeenCalled());
    await clickButton('Hub');
    await waitFor(() =>
      expect(screen.getByTestId('topology-hub')).toHaveTextContent(
        'selected-map:map-rename:Branch',
      ),
    );
    await clickButton('Rename map Branch');
    const dialog = await screen.findByRole('dialog', { name: 'Rename map' });
    expect(within(dialog).getByLabelText('Map name')).toHaveValue('Branch');

    fireEvent.change(within(dialog).getByLabelText('Map name'), {
      target: { value: ' Branch Renamed ' },
    });
    fireEvent.click(within(dialog).getByRole('button', { name: 'Rename map' }));

    await waitFor(() =>
      expect(updateCanvasMapMock).toHaveBeenCalledWith('map-rename', {
        name: 'Branch Renamed',
      }),
    );
    expect(screen.queryByRole('dialog', { name: 'Rename map' })).not.toBeInTheDocument();
    expect(screen.getByTestId('navigation-pill')).toHaveTextContent(
      'pill-map:map-rename:Branch Renamed',
    );
    expect(screen.getByTestId('topology-hub')).toHaveTextContent(
      'selected-map:map-rename:Branch Renamed',
    );
  });

  it('opens the selected hub map instead of forcing the default map', async () => {
    const defaultMap = mockMap({
      id: 'default-map-id',
      name: 'Default',
      source_area_id: null,
      filter: {},
      is_default: true,
    });
    const branchMap = mockMap({
      id: 'map-branch',
      name: 'Branch',
      is_default: false,
    });
    fetchCanvasMapsMock.mockResolvedValue([defaultMap, branchMap]);

    await renderApp();

    await waitFor(() => expect(fetchAreasMock).toHaveBeenCalled());
    await waitFor(() =>
      expect(screen.getByTestId('navigation-pill')).toHaveTextContent(
        'pill-map:default-map-id:Default',
      ),
    );
    await clickButton('Hub');
    await waitFor(() =>
      expect(screen.getByTestId('topology-hub')).toHaveTextContent(
        'selected-map:default-map-id:Default',
      ),
    );
    await clickButton('Select hub map Branch');
    expect(screen.getByTestId('topology-hub')).toHaveTextContent('selected-map:map-branch:Branch');
    expect(screen.getByTestId('topology-hub').parentElement?.className).toContain('h-full');
    await clickButton('Open selected map');

    expect(screen.getByTestId('canvas')).toHaveTextContent('map:map-branch:Branch');
    expect(screen.getByTestId('canvas').parentElement?.className).toContain('h-full');
    expect(screen.getByTestId('canvas').parentElement).toHaveClass('opacity-100');
  });

  it('sets a saved map as primary from the hub and selects it locally', async () => {
    const defaultMap = mockMap({
      id: 'default-map-id',
      name: 'Default',
      source_area_id: null,
      filter: {},
      is_default: true,
    });
    const branchMap = mockMap({
      id: 'map-branch',
      name: 'Branch',
      is_default: false,
    });
    const promotedBranch = { ...branchMap, is_default: true };
    fetchCanvasMapsMock.mockResolvedValue([defaultMap, branchMap]);
    setCanvasMapPrimaryMock.mockResolvedValue(promotedBranch);

    await renderApp();

    await waitFor(() =>
      expect(screen.getByTestId('navigation-pill')).toHaveTextContent(
        'pill-map:default-map-id:Default',
      ),
    );
    await clickButton('Hub');
    await waitFor(() =>
      expect(screen.getByTestId('topology-hub')).toHaveTextContent(
        'selected-map:default-map-id:Default',
      ),
    );
    await clickButton('Set primary map Branch');

    await waitFor(() => expect(setCanvasMapPrimaryMock).toHaveBeenCalledWith('map-branch'));
    expect(screen.getByTestId('navigation-pill')).toHaveTextContent('pill-map:map-branch:Branch');
    expect(screen.getByTestId('topology-hub')).toHaveTextContent('selected-map:map-branch:Branch');
  });

  it('keeps map-local areas when selecting and opening the same hub map again', async () => {
    const savedMap = mockMap({
      id: 'map-1',
      name: 'Backbone',
      is_default: false,
    });
    fetchCanvasMapsMock.mockResolvedValue([savedMap]);

    await renderApp();

    await waitFor(() => expect(fetchAreasMock).toHaveBeenCalled());
    await clickButton('Pill Open Backbone map');
    await waitFor(() =>
      expect(screen.getByTestId('navigation-pill')).toHaveTextContent('pill-areas:Map Local Area'),
    );
    await clickButton('Hub');
    expect(screen.getByTestId('topology-hub')).toHaveTextContent('hub-areas:Map Local Area');
    await clickButton('Select hub map Backbone');
    expect(screen.getByTestId('topology-hub')).toHaveTextContent('hub-areas:Map Local Area');
    expect(screen.getByTestId('navigation-pill')).toHaveTextContent('pill-areas:Map Local Area');
    await clickButton('Open selected map');
    expect(screen.getByTestId('canvas')).toHaveTextContent('map:map-1:Backbone');
    expect(screen.getByTestId('navigation-pill')).toHaveTextContent('pill-areas:Map Local Area');
    expect(screen.getByTestId('canvas').parentElement?.className).toContain('h-full');
    expect(screen.getByTestId('canvas').parentElement).toHaveClass('opacity-100');
  });

  it('creates a map from the active map-local area with the source map context', async () => {
    const createdMap = mockMap({
      id: 'map-from-area',
      name: 'Map Local Area Copy',
      source_area_id: 'map-area-1',
      filter: { area_id: 'map-area-1' },
    });
    createCanvasMapMock.mockResolvedValue(createdMap);

    await renderApp();

    await waitFor(() => expect(fetchAreasMock).toHaveBeenCalled());
    await clickButton('Pill Open Backbone map');
    await clickButton('Hub');

    expect(await screen.findByTestId('topology-hub')).toHaveTextContent('hub-areas:Map Local Area');
    fireEvent.click(
      screen.getByRole('button', {
        name: 'Create map from area Map Local Area',
      }),
    );
    fireEvent.change(await screen.findByLabelText('Map name'), {
      target: { value: 'Map Local Area Copy' },
    });
    await clickButton('Create map');

    await waitFor(() =>
      expect(createCanvasMapMock).toHaveBeenCalledWith({
        name: 'Map Local Area Copy',
        source_area_id: 'map-area-1',
        source_map_id: 'map-1',
        filter: {
          area_id: 'map-area-1',
          include_cross_area_links: true,
          include_ghost_devices: true,
        },
      }),
    );
    expect(await screen.findByText('map:map-from-area:Map Local Area Copy')).toBeInTheDocument();
  });

  it('creates a map from a default-map area with the default saved map as source context', async () => {
    const defaultMap = mockMap({
      id: 'default-map',
      name: 'Default',
      source_area_id: null,
      filter: {},
      is_default: true,
    });
    const createdMap = mockMap({
      id: 'global-area-copy',
      name: 'Backbone Copy',
      source_area_id: 'area-1',
      filter: { area_id: 'area-1' },
    });
    fetchCanvasMapsMock.mockResolvedValue([defaultMap]);
    createCanvasMapMock.mockResolvedValue(createdMap);

    await renderApp();

    await waitFor(() => expect(fetchAreasMock).toHaveBeenCalled());
    await waitFor(() =>
      expect(screen.getByTestId('navigation-pill')).toHaveTextContent('pill-map:default-map'),
    );
    await clickButton('Hub');

    expect(await screen.findByTestId('topology-hub')).toHaveTextContent('hub-areas:Backbone');
    await clickButton('Create map from area Backbone');
    fireEvent.change(await screen.findByLabelText('Map name'), {
      target: { value: 'Backbone Copy' },
    });
    await clickButton('Create map');

    await waitFor(() =>
      expect(createCanvasMapMock).toHaveBeenCalledWith({
        name: 'Backbone Copy',
        source_area_id: 'area-1',
        source_map_id: 'default-map',
        filter: {
          area_id: 'area-1',
          include_cross_area_links: true,
          include_ghost_devices: true,
        },
      }),
    );
  });

  it('keeps a saved map as the source after opening one of its areas from the hub', async () => {
    const createdMap = mockMap({
      id: 'map-from-opened-area',
      name: 'Opened Area Copy',
      source_area_id: 'map-area-1',
      filter: { area_id: 'map-area-1' },
    });
    createCanvasMapMock.mockResolvedValue(createdMap);

    await renderApp();

    await waitFor(() => expect(fetchAreasMock).toHaveBeenCalled());
    await clickButton('Pill Open Backbone map');
    await clickButton('Hub');

    expect(await screen.findByTestId('topology-hub')).toHaveTextContent('hub-areas:Map Local Area');
    await clickButton('Open area Map Local Area');

    expect(screen.getByTestId('canvas')).toHaveTextContent('map:map-1:Backbone');
    await clickButton('Hub');
    fireEvent.click(
      screen.getByRole('button', {
        name: 'Create map from area Map Local Area',
      }),
    );
    fireEvent.change(await screen.findByLabelText('Map name'), {
      target: { value: 'Opened Area Copy' },
    });
    await clickButton('Create map');

    await waitFor(() =>
      expect(createCanvasMapMock).toHaveBeenCalledWith({
        name: 'Opened Area Copy',
        source_area_id: 'map-area-1',
        source_map_id: 'map-1',
        filter: {
          area_id: 'map-area-1',
          include_cross_area_links: true,
          include_ghost_devices: true,
        },
      }),
    );
  });
});
