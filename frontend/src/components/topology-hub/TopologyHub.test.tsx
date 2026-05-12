import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import type { Area, CanvasMap, Device, Link } from '../../types/api';
import type { DeviceRuntimeDTO, SnapshotPayload } from '../../types/metrics';
import { TopologyHub } from './TopologyHub';

const createCanvasMapAreaMock = vi.fn();
const updateCanvasMapAreaMock = vi.fn();
const deleteCanvasMapAreaMock = vi.fn();
const updateCanvasMapDeviceAreasMock = vi.fn();
const createAreaMock = vi.fn();
const updateAreaMock = vi.fn();
const deleteAreaMock = vi.fn();
const fetchAreasMock = vi.fn();
const fetchDevicesMock = vi.fn();
const updateDeviceMock = vi.fn();

vi.mock('../../contexts/ThemeContext', () => ({
  useTheme: () => ({ theme: 'dark' as const, resolvedTheme: 'dark' as const, setTheme: vi.fn() }),
  adaptAreaColor: (hex: string) => hex,
}));

vi.mock('../../api/client', () => ({
  createArea: (...args: unknown[]) => createAreaMock(...args),
  createCanvasMapArea: (...args: unknown[]) => createCanvasMapAreaMock(...args),
  deleteArea: (...args: unknown[]) => deleteAreaMock(...args),
  updateCanvasMapArea: (...args: unknown[]) => updateCanvasMapAreaMock(...args),
  deleteCanvasMapArea: (...args: unknown[]) => deleteCanvasMapAreaMock(...args),
  fetchAreas: (...args: unknown[]) => fetchAreasMock(...args),
  fetchDevices: (...args: unknown[]) => fetchDevicesMock(...args),
  updateArea: (...args: unknown[]) => updateAreaMock(...args),
  updateCanvasMapDeviceAreas: (...args: unknown[]) => updateCanvasMapDeviceAreasMock(...args),
  updateDevice: (...args: unknown[]) => updateDeviceMock(...args),
}));

function mockArea(overrides: Partial<Area> = {}): Area {
  return {
    id: 'area-1',
    name: 'Backbone',
    description: '',
    color: '#2979FF',
    device_count: 0,
    created_at: '',
    updated_at: '',
    ...overrides,
  };
}

function mockDevice(overrides: Partial<Device> = {}): Device {
  return {
    id: 'device-1',
    hostname: 'router-1',
    ip: '10.0.0.1',
    notes: null,
    device_type: 'router',
    poll_class: 'standard',
    poll_interval_override: null,
    polling_enabled: true,
    status: 'up',
    sys_name: 'router-1',
    sys_descr: '',
    hardware_model: '',
    os_version: '',
    vendor: 'mikrotik',
    managed: true,
    tags: {},
    interfaces: [],
    area_ids: ['area-1'],
    backup_supported: false,
    metrics_source: 'snmp',
    prometheus_label_name: 'instance',
    prometheus_label_value: '10.0.0.1:9100',
    topology_discovery_mode: 'inherit',
    effective_topology_discovery_mode: 'lldp',
    topology_bootstrap_state: 'idle',
    last_topology_discovery_at: null,
    last_topology_discovery_result: '',
    ...overrides,
  };
}

function mockLink(overrides: Partial<Link> = {}): Link {
  return {
    id: 'link-1',
    source_device_id: 'device-1',
    source_if_name: 'ether1',
    target_device_id: 'device-2',
    target_if_name: 'ether2',
    discovery_protocol: 'lldp',
    source_if_speed: 1_000_000_000,
    source_if_oper_status: 'up',
    target_if_speed: 1_000_000_000,
    target_if_oper_status: 'up',
    ...overrides,
  };
}

function mockMap(overrides: Partial<CanvasMap> = {}): CanvasMap {
  return {
    id: 'default',
    name: 'Default',
    description: '',
    source_area_id: null,
    filter: {},
    is_default: true,
    device_count: 1,
    link_count: 1,
    position_count: 1,
    created_at: '',
    updated_at: '',
    ...overrides,
  };
}

function mockDeviceRuntime(overrides: Partial<DeviceRuntimeDTO> = {}): DeviceRuntimeDTO {
  return {
    device_id: 'device-1',
    operational_status: 'up',
    primary_health: 'up_fresh',
    runtime_flags: [],
    field_states: { uptime: 'ok', cpu: 'ok', memory: 'ok' },
    network_reachable: 'true',
    snmp_reachable: 'true',
    reachability: 'up',
    health: 'healthy',
    freshness: 'fresh',
    primary_reason: 'ok',
    metrics_status: 'available',
    metrics_reason: 'ok',
    alert_status: 'normal',
    firing_alert_count: 0,
    last_collected_at: '2026-01-01T00:00:00Z',
    last_polled_at: '2026-01-01T00:00:00Z',
    expected_poll_interval_seconds: 30,
    cpu_percent: 50,
    mem_percent: 25,
    temp_celsius: null,
    uptime_secs: 86400,
    ...overrides,
  };
}

describe('TopologyHub', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    createCanvasMapAreaMock.mockResolvedValue(mockArea({ id: 'area-new', name: 'Hub Area' }));
    updateCanvasMapAreaMock.mockResolvedValue(mockArea());
    deleteCanvasMapAreaMock.mockResolvedValue(undefined);
    updateCanvasMapDeviceAreasMock.mockResolvedValue(mockMap());
    fetchAreasMock.mockResolvedValue([]);
    fetchDevicesMock.mockResolvedValue([]);
  });

  it('renders map-first hub content without legacy global navigation', () => {
    const area = mockArea();
    const onOpenArea = vi.fn();
    const onOpenMap = vi.fn();
    const onSelectMap = vi.fn();
    const onCreateEmptyMap = vi.fn();
    const onCreateMapFromArea = vi.fn();
    const selectedMap = mockMap({ id: 'map-branch', name: 'Branch', is_default: false });
    const snapshot: SnapshotPayload = {
      devices: {
        'device-1': mockDeviceRuntime({
          operational_status: 'down',
          primary_health: 'unreachable',
          network_reachable: 'false',
          reachability: 'hard_down',
          health: 'critical',
          alert_status: 'down',
        }),
      },
      links: {},
    };

    const { container } = render(
      <TopologyHub
        devices={[mockDevice()]}
        areas={[area]}
        links={[mockLink()]}
        snapshot={snapshot}
        maps={[mockMap(), selectedMap]}
        selectedMapId="map-branch"
        selectedMapName="Branch"
        mapsLoading={false}
        mapsError={null}
        savedMapsEnabled={true}
        onOpenArea={onOpenArea}
        onOpenMap={onOpenMap}
        onSelectMap={onSelectMap}
        onCreateEmptyMap={onCreateEmptyMap}
        onCreateMapFromArea={onCreateMapFromArea}
        onDuplicateMap={vi.fn()}
        onDeleteMap={vi.fn()}
        onOpenSettings={vi.fn()}
      />,
    );

    expect(screen.getByRole('heading', { name: 'Topology Hub' })).toBeInTheDocument();
    expect(container).not.toHaveTextContent('OSPF');
    expect(screen.queryByRole('button', { name: 'Open global map' })).not.toBeInTheDocument();
    expect(screen.getByRole('heading', { name: 'Branch', level: 2 })).toBeInTheDocument();
    expect(screen.getByText('Needs attention')).toBeInTheDocument();

    fireEvent.click(screen.getByRole('button', { name: 'Open selected map' }));
    fireEvent.click(screen.getByRole('button', { name: 'Select map Default' }));
    fireEvent.click(screen.getByRole('button', { name: 'Open area Backbone' }));
    fireEvent.click(screen.getByRole('button', { name: 'Create empty map' }));
    fireEvent.click(screen.getByRole('button', { name: 'Create map from area Backbone' }));

    expect(onOpenMap).toHaveBeenCalledWith(selectedMap);
    expect(onSelectMap).toHaveBeenCalledWith(mockMap());
    expect(onOpenArea).toHaveBeenCalledWith('area-1');
    expect(onCreateEmptyMap).toHaveBeenCalledOnce();
    expect(onCreateMapFromArea).toHaveBeenCalledWith(area);
  });

  it('reserves mobile top space for a wrapped navigation pill', () => {
    const { container } = render(
      <TopologyHub
        devices={[mockDevice()]}
        areas={[mockArea()]}
        links={[mockLink()]}
        snapshot={null}
        maps={[mockMap()]}
        selectedMapId="default"
        selectedMapName="Default"
        mapsLoading={false}
        mapsError={null}
        savedMapsEnabled={true}
        onOpenArea={vi.fn()}
        onOpenMap={vi.fn()}
        onSelectMap={vi.fn()}
        onCreateEmptyMap={vi.fn()}
        onCreateMapFromArea={vi.fn()}
        onDuplicateMap={vi.fn()}
        onDeleteMap={vi.fn()}
        onOpenSettings={vi.fn()}
      />,
    );

    const classNames = container.firstElementChild?.className.split(/\s+/) ?? [];

    expect(classNames).toContain('pt-32');
    expect(classNames).toContain('sm:pt-20');
    expect(classNames).not.toContain('pt-20');
  });

  it('describes the map-first topology hub scope in the subtitle', () => {
    render(
      <TopologyHub
        devices={[mockDevice()]}
        areas={[mockArea()]}
        links={[mockLink()]}
        snapshot={null}
        maps={[mockMap()]}
        selectedMapId="default"
        selectedMapName="Default"
        mapsLoading={false}
        mapsError={null}
        savedMapsEnabled={true}
        onOpenArea={vi.fn()}
        onOpenMap={vi.fn()}
        onSelectMap={vi.fn()}
        onCreateEmptyMap={vi.fn()}
        onCreateMapFromArea={vi.fn()}
        onDuplicateMap={vi.fn()}
        onDeleteMap={vi.fn()}
        onOpenSettings={vi.fn()}
      />,
    );

    expect(
      screen.getByText('Saved maps, map-local areas, and topology health'),
    ).toBeInTheDocument();
    expect(screen.queryByText('Network aggregate')).toBeNull();
  });

  it('hides saved map management controls when saved maps are disabled', () => {
    render(
      <TopologyHub
        devices={[mockDevice()]}
        areas={[mockArea()]}
        links={[mockLink()]}
        snapshot={null}
        maps={[mockMap()]}
        selectedMapId="default"
        selectedMapName="Default"
        mapsLoading={false}
        mapsError="Map service unavailable"
        savedMapsEnabled={false}
        onOpenArea={vi.fn()}
        onOpenMap={vi.fn()}
        onSelectMap={vi.fn()}
        onCreateEmptyMap={vi.fn()}
        onCreateMapFromArea={vi.fn()}
        onDuplicateMap={vi.fn()}
        onDeleteMap={vi.fn()}
        onOpenSettings={vi.fn()}
      />,
    );

    expect(screen.queryByRole('button', { name: 'Create map from area Backbone' })).toBeNull();
    expect(screen.queryByRole('button', { name: 'Create empty map' })).toBeNull();
    expect(screen.queryByText('Saved maps')).toBeNull();
    expect(screen.queryByText('Map service unavailable')).toBeNull();
  });

  it('shows saved map management controls when saved maps are enabled', () => {
    render(
      <TopologyHub
        devices={[mockDevice()]}
        areas={[mockArea()]}
        links={[mockLink()]}
        snapshot={null}
        maps={[mockMap()]}
        selectedMapId="default"
        selectedMapName="Default"
        mapsLoading={false}
        mapsError={null}
        savedMapsEnabled={true}
        onOpenArea={vi.fn()}
        onOpenMap={vi.fn()}
        onSelectMap={vi.fn()}
        onCreateEmptyMap={vi.fn()}
        onCreateMapFromArea={vi.fn()}
        onDuplicateMap={vi.fn()}
        onDeleteMap={vi.fn()}
        onOpenSettings={vi.fn()}
      />,
    );

    expect(
      screen.getByRole('button', { name: 'Create map from area Backbone' }),
    ).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Create empty map' })).toBeInTheDocument();
    expect(screen.getByText('Saved maps')).toBeInTheDocument();
  });

  it('allows a non-primary saved map to become primary', () => {
    const defaultMap = mockMap();
    const branchMap = mockMap({ id: 'map-branch', name: 'Branch', is_default: false });
    const onSetPrimaryMap = vi.fn();

    render(
      <TopologyHub
        devices={[mockDevice()]}
        areas={[mockArea()]}
        links={[mockLink()]}
        snapshot={null}
        maps={[defaultMap, branchMap]}
        selectedMapId="default"
        selectedMapName="Default"
        mapsLoading={false}
        mapsError={null}
        savedMapsEnabled={true}
        onOpenArea={vi.fn()}
        onOpenMap={vi.fn()}
        onSelectMap={vi.fn()}
        onCreateEmptyMap={vi.fn()}
        onCreateMapFromArea={vi.fn()}
        onDuplicateMap={vi.fn()}
        onDeleteMap={vi.fn()}
        onSetPrimaryMap={onSetPrimaryMap}
        onOpenSettings={vi.fn()}
      />,
    );

    expect(screen.queryByRole('button', { name: 'Set Default as primary' })).toBeNull();
    fireEvent.click(screen.getByRole('button', { name: 'Set Branch as primary' }));

    expect(onSetPrimaryMap).toHaveBeenCalledWith(branchMap);
  });

  it('shows the saved maps empty state when saved maps are enabled with no maps', () => {
    render(
      <TopologyHub
        devices={[mockDevice()]}
        areas={[mockArea()]}
        links={[mockLink()]}
        snapshot={null}
        maps={[]}
        selectedMapId={null}
        selectedMapName="Default"
        mapsLoading={false}
        mapsError={null}
        savedMapsEnabled={true}
        onOpenArea={vi.fn()}
        onOpenMap={vi.fn()}
        onSelectMap={vi.fn()}
        onCreateEmptyMap={vi.fn()}
        onCreateMapFromArea={vi.fn()}
        onDuplicateMap={vi.fn()}
        onDeleteMap={vi.fn()}
        onOpenSettings={vi.fn()}
      />,
    );

    expect(screen.getByText('Saved maps')).toBeInTheDocument();
    expect(screen.getByText('No saved maps')).toBeInTheDocument();
  });

  it('creates map-local areas from the Topology Hub section for the selected map', async () => {
    const onAreasChange = vi.fn();
    const selectedMap = mockMap({ id: 'map-branch', name: 'Branch', is_default: false });

    render(
      <TopologyHub
        devices={[]}
        areas={[]}
        links={[]}
        snapshot={null}
        maps={[mockMap(), selectedMap]}
        selectedMapId="map-branch"
        selectedMapName="Branch"
        mapsLoading={false}
        mapsError={null}
        savedMapsEnabled={true}
        onOpenArea={vi.fn()}
        onOpenMap={vi.fn()}
        onSelectMap={vi.fn()}
        onCreateEmptyMap={vi.fn()}
        onCreateMapFromArea={vi.fn()}
        onDuplicateMap={vi.fn()}
        onDeleteMap={vi.fn()}
        onAreasChange={onAreasChange}
        onOpenSettings={vi.fn()}
      />,
    );

    await userEvent.click(screen.getByRole('button', { name: 'New area' }));
    await userEvent.type(screen.getByPlaceholderText(/backbone/i), 'Hub Area');
    await userEvent.click(screen.getByRole('button', { name: 'Create Area' }));

    await waitFor(() => {
      expect(createCanvasMapAreaMock).toHaveBeenCalledWith('map-branch', {
        name: 'Hub Area',
        description: '',
        color: '#00E676',
      });
    });
    expect(onAreasChange).toHaveBeenCalled();
  });
});
