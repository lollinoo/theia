import { fireEvent, render, screen } from '@testing-library/react';
import type React from 'react';
import { describe, expect, it, vi } from 'vitest';
import type { Device, Link } from '../../types/api';
import type { AlertDTO, DeviceMetricsDTO } from '../../types/metrics';
import type { DeviceNode } from '../DeviceCard';
import type { AlertsPanelModel } from '../panelModels';
import { CanvasPanels } from './CanvasPanels';
import { buildRuntimeState } from './runtimeAdapters';

vi.mock('../DeviceConfigPanel', () => ({
  DeviceConfigPanel: (props: {
    device: Device;
    readOnly?: boolean;
    onDeviceUpdated?: (device: Device) => void;
    onWinBoxAvailabilityChange?: (hasWinboxProfile: boolean) => void;
  }) => (
    <div>
      <div>Config device:{props.device.hostname}</div>
      <div>Device config read-only:{String(props.readOnly)}</div>
      <button type="button" onClick={() => props.onWinBoxAvailabilityChange?.(true)}>
        Notify WinBox
      </button>
      <button
        type="button"
        onClick={() =>
          props.onDeviceUpdated?.({
            ...props.device,
            ip: '10.0.0.2',
            prometheus_label_value: '10.0.0.2:9100',
          })
        }
      >
        Save IP change
      </button>
    </div>
  ),
}));

vi.mock('../DeviceDetailsPanel', () => ({
  DeviceDetailsPanel: (props: {
    device: { hostname: string };
    interfaceStats?: React.ReactNode;
  }) => (
    <div>
      <div>Details device:{props.device.hostname}</div>
      {props.interfaceStats}
    </div>
  ),
}));

vi.mock('../LinkDetailsPanel', () => ({
  LinkDetailsPanel: (props: { readOnly?: boolean; link: { target_if_name: string } }) => (
    <div>
      {props.readOnly ? 'Link Details Read Only' : 'Link Details Editable'}:
      {props.link.target_if_name}
    </div>
  ),
}));

vi.mock('../AlertsPanel', () => ({
  AlertsPanel: (props: { model: AlertsPanelModel }) => (
    <div>
      Alert count: {props.model.firingAlerts.length}
      {props.model.firingAlerts[0]
        ? ` ${props.model.firingAlerts[0].deviceLabel} ${props.model.firingAlerts[0].alertName}`
        : ''}
    </div>
  ),
}));

vi.mock('./InterfaceStatsPanelRoutes', () => ({
  LinkInterfaceStatsPanelRoute: (props: { link: { source_if_name: string } }) => (
    <div>Link interface model:{props.link.source_if_name}</div>
  ),
  DeviceInterfaceStatsPanelRoute: (props: { device: { id: string } }) => (
    <div>Device interface model:{props.device.id}</div>
  ),
}));

function mockDevice(overrides: Partial<Device> = {}): Device {
  return {
    id: 'dev-1',
    hostname: 'router-01',
    ip: '10.0.0.1',
    device_type: 'router',
    poll_class: 'core',
    poll_interval_override: null,
    status: 'up',
    sys_name: 'router-01',
    sys_descr: 'RouterOS',
    hardware_model: 'RB4011',
    vendor: 'mikrotik',
    managed: true,
    interfaces: [],
    backup_supported: true,
    metrics_source: 'snmp',
    prometheus_label_name: 'instance',
    prometheus_label_value: '10.0.0.1:9100',
    area_ids: [],
    ...overrides,
  };
}

function mockLink(overrides: Partial<Link> = {}): Link {
  return {
    id: 'link-1',
    source_device_id: 'dev-1',
    source_if_name: 'ether1',
    target_device_id: 'dev-2',
    target_if_name: 'ether2',
    discovery_protocol: 'lldp',
    source_if_speed: 1_000_000_000,
    source_if_oper_status: 'up',
    target_if_speed: 1_000_000_000,
    target_if_oper_status: 'up',
    ...overrides,
  };
}

function mockAlert(overrides: Partial<AlertDTO> = {}): AlertDTO {
  return {
    device_id: 'dev-1',
    severity: 'critical',
    alert_name: 'DeviceDown',
    state: 'firing',
    summary: 'router unreachable',
    ...overrides,
  };
}

function mockMetrics(overrides: Partial<DeviceMetricsDTO> = {}): DeviceMetricsDTO {
  return {
    device_id: 'dev-1',
    operational_status: 'up',
    primary_health: 'up_fresh',
    runtime_flags: [],
    field_states: { uptime: 'missing', cpu: 'ok', memory: 'ok' },
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
    last_collected_at: '2026-04-27T08:30:00Z',
    last_polled_at: '2026-04-27T08:30:00Z',
    expected_poll_interval_seconds: 30,
    cpu_percent: 42,
    mem_percent: 68,
    temp_celsius: null,
    uptime_secs: null,
    ...overrides,
  };
}

describe('CanvasPanels', () => {
  it('forwards WinBox availability updates for the open device config panel', () => {
    const onWinBoxAvailabilityChange = vi.fn();
    const device = mockDevice();
    const runtimeState = buildRuntimeState({
      devices: [device],
      links: [],
      snapshot: null,
      alerts: [],
      prometheusStatus: null,
    });

    render(
      <CanvasPanels
        panelContent={{ type: 'deviceConfig', data: { deviceId: device.id } }}
        setPanelContent={vi.fn()}
        devices={[device]}
        topologyLinks={[]}
        loadTopology={vi.fn().mockResolvedValue(undefined)}
        setDevices={vi.fn()}
        setNodes={vi.fn()}
        reactFlow={{} as never}
        runtimeState={runtimeState}
        onWinBoxAvailabilityChange={onWinBoxAvailabilityChange}
      />,
    );

    fireEvent.click(screen.getByRole('button', { name: 'Notify WinBox' }));

    expect(onWinBoxAvailabilityChange).toHaveBeenCalledWith(device.id, true);
  });

  it('looks up the live device for device config panels by device id', () => {
    const staleDevice = mockDevice({ hostname: 'stale-router' });
    const liveDevice = mockDevice({ hostname: 'live-router' });
    const runtimeState = buildRuntimeState({
      devices: [liveDevice],
      links: [],
      snapshot: null,
      alerts: [],
      prometheusStatus: null,
    });

    render(
      <CanvasPanels
        panelContent={{
          type: 'deviceConfig',
          data: { deviceId: staleDevice.id, device: staleDevice },
        }}
        setPanelContent={vi.fn()}
        devices={[liveDevice]}
        topologyLinks={[]}
        loadTopology={vi.fn().mockResolvedValue(undefined)}
        setDevices={vi.fn()}
        setNodes={vi.fn()}
        reactFlow={{} as never}
        runtimeState={runtimeState}
      />,
    );

    expect(screen.getByText('Config device:live-router')).toBeInTheDocument();
  });

  it('passes device config panels as read-only unless edit mode is enabled', () => {
    const device = mockDevice();
    const runtimeState = buildRuntimeState({
      devices: [device],
      links: [],
      snapshot: null,
      alerts: [],
      prometheusStatus: null,
    });

    const { rerender } = render(
      <CanvasPanels
        panelContent={{ type: 'deviceConfig', data: { deviceId: device.id } }}
        setPanelContent={vi.fn()}
        devices={[device]}
        topologyLinks={[]}
        loadTopology={vi.fn().mockResolvedValue(undefined)}
        setDevices={vi.fn()}
        setNodes={vi.fn()}
        reactFlow={{} as never}
        runtimeState={runtimeState}
        editMode={false}
      />,
    );

    expect(screen.getByText('Device config read-only:true')).toBeInTheDocument();

    rerender(
      <CanvasPanels
        panelContent={{ type: 'deviceConfig', data: { deviceId: device.id } }}
        setPanelContent={vi.fn()}
        devices={[device]}
        topologyLinks={[]}
        loadTopology={vi.fn().mockResolvedValue(undefined)}
        setDevices={vi.fn()}
        setNodes={vi.fn()}
        reactFlow={{} as never}
        runtimeState={runtimeState}
        editMode
      />,
    );

    expect(screen.getByText('Device config read-only:false')).toBeInTheDocument();
  });

  it('clears node metrics immediately when a device config update changes the IP', () => {
    const device = mockDevice();
    const runtimeState = buildRuntimeState({
      devices: [device],
      links: [],
      snapshot: null,
      alerts: [],
      prometheusStatus: null,
    });
    const setNodes = vi.fn();

    render(
      <CanvasPanels
        panelContent={{ type: 'deviceConfig', data: { deviceId: device.id } }}
        setPanelContent={vi.fn()}
        devices={[device]}
        topologyLinks={[]}
        loadTopology={vi.fn().mockResolvedValue(undefined)}
        setDevices={vi.fn()}
        setNodes={setNodes}
        reactFlow={{} as never}
        runtimeState={runtimeState}
        editMode
      />,
    );

    fireEvent.click(screen.getByRole('button', { name: 'Save IP change' }));

    const updateNodes = setNodes.mock.calls[0]?.[0] as
      | ((nodes: DeviceNode[]) => DeviceNode[])
      | undefined;
    if (!updateNodes) {
      throw new Error('expected setNodes updater to be called');
    }
    const previousNode: DeviceNode = {
      id: device.id,
      type: 'device',
      position: { x: 0, y: 0 },
      data: {
        device,
        runtime: {
          status: device.status,
          metrics: mockMetrics(),
          alertStatus: 'normal',
          monitoringState: 'monitorable',
        },
        pinned: false,
      },
    };

    const [updatedNode] = updateNodes([previousNode]);

    expect(updatedNode.data.device.ip).toBe('10.0.0.2');
    expect(updatedNode.data.runtime.metrics).toBeNull();
  });

  it('renders link details in read-only mode when edit mode is disabled', () => {
    const sourceDevice = mockDevice();
    const targetDevice = mockDevice({
      id: 'dev-2',
      hostname: 'router-02',
      ip: '10.0.0.2',
      sys_name: 'router-02',
    });
    const runtimeState = buildRuntimeState({
      devices: [sourceDevice, targetDevice],
      links: [mockLink()],
      snapshot: null,
      alerts: [],
      prometheusStatus: null,
    });

    render(
      <CanvasPanels
        panelContent={{ type: 'link-details', data: { link: mockLink() } }}
        setPanelContent={vi.fn()}
        devices={[sourceDevice, targetDevice]}
        topologyLinks={[mockLink()]}
        loadTopology={vi.fn().mockResolvedValue(undefined)}
        setDevices={vi.fn()}
        setNodes={vi.fn()}
        reactFlow={{} as never}
        runtimeState={runtimeState}
      />,
    );

    expect(screen.getByText('Link Details Read Only:ether2')).toBeInTheDocument();
  });

  it('updates link details read-only state when edit mode changes', () => {
    const sourceDevice = mockDevice();
    const targetDevice = mockDevice({
      id: 'dev-2',
      hostname: 'router-02',
      ip: '10.0.0.2',
      sys_name: 'router-02',
    });
    const runtimeState = buildRuntimeState({
      devices: [sourceDevice, targetDevice],
      links: [mockLink()],
      snapshot: null,
      alerts: [],
      prometheusStatus: null,
    });
    const sharedProps = {
      panelContent: { type: 'link-details', data: { link: mockLink() } },
      setPanelContent: vi.fn(),
      devices: [sourceDevice, targetDevice],
      topologyLinks: [mockLink()],
      loadTopology: vi.fn().mockResolvedValue(undefined),
      setDevices: vi.fn(),
      setNodes: vi.fn(),
      reactFlow: {} as never,
      runtimeState,
    };

    const { rerender } = render(<CanvasPanels {...sharedProps} editMode={false} />);

    expect(screen.getByText('Link Details Read Only:ether2')).toBeInTheDocument();

    rerender(<CanvasPanels {...sharedProps} editMode />);

    expect(screen.getByText('Link Details Editable:ether2')).toBeInTheDocument();
  });

  it('renders live device details from a deviceDetails panel target', () => {
    const staleDevice = mockDevice({ hostname: 'stale-router' });
    const liveDevice = mockDevice({ hostname: 'live-router' });
    const runtimeState = buildRuntimeState({
      devices: [liveDevice],
      links: [],
      snapshot: null,
      alerts: [],
      prometheusStatus: null,
    });

    render(
      <CanvasPanels
        panelContent={{
          type: 'deviceDetails',
          data: { deviceId: staleDevice.id, device: staleDevice },
        }}
        setPanelContent={vi.fn()}
        devices={[liveDevice]}
        topologyLinks={[]}
        loadTopology={vi.fn().mockResolvedValue(undefined)}
        setDevices={vi.fn()}
        setNodes={vi.fn()}
        reactFlow={{} as never}
        runtimeState={runtimeState}
      />,
    );

    expect(screen.getByText('Details device:live-router')).toBeInTheDocument();
  });

  it('prefers the live topology link over stale panel data for link details', () => {
    const sourceDevice = mockDevice();
    const targetDevice = mockDevice({
      id: 'dev-2',
      hostname: 'router-02',
      ip: '10.0.0.2',
      sys_name: 'router-02',
    });
    const runtimeState = buildRuntimeState({
      devices: [sourceDevice, targetDevice],
      links: [mockLink({ target_if_name: 'ether2' })],
      snapshot: null,
      alerts: [],
      prometheusStatus: null,
    });

    render(
      <CanvasPanels
        panelContent={{
          type: 'link-details',
          data: { link: mockLink({ target_if_name: '' }), readOnly: true },
        }}
        setPanelContent={vi.fn()}
        devices={[sourceDevice, targetDevice]}
        topologyLinks={[mockLink({ target_if_name: 'ether2' })]}
        loadTopology={vi.fn().mockResolvedValue(undefined)}
        setDevices={vi.fn()}
        setNodes={vi.fn()}
        reactFlow={{} as never}
        runtimeState={runtimeState}
      />,
    );

    expect(screen.getByText('Link Details Read Only:ether2')).toBeInTheDocument();
  });

  it('passes separate alert state through to the alerts panel', () => {
    const device = mockDevice();
    const runtimeState = buildRuntimeState({
      devices: [device],
      links: [],
      snapshot: null,
      alerts: [mockAlert()],
      prometheusStatus: null,
    });

    render(
      <CanvasPanels
        {...({
          panelContent: { type: 'alerts' },
          setPanelContent: vi.fn(),
          alerts: [mockAlert()],
          devices: [device],
          topologyLinks: [],
          loadTopology: vi.fn().mockResolvedValue(undefined),
          setDevices: vi.fn(),
          setNodes: vi.fn(),
          reactFlow: {} as never,
          runtimeState,
        } as const)}
      />,
    );

    expect(screen.getByText('Alert count: 1 router-01 DeviceDown')).toBeInTheDocument();
  });

  it('routes link interface panels by link id against live topology state', () => {
    const sourceDevice = mockDevice();
    const targetDevice = mockDevice({
      id: 'dev-2',
      hostname: 'router-02',
      ip: '10.0.0.2',
      sys_name: 'router-02',
    });
    const staleLink = mockLink({ source_if_name: 'stale-ether1' });
    const liveLink = mockLink({ source_if_name: 'ether1' });
    const runtimeState = buildRuntimeState({
      devices: [sourceDevice, targetDevice],
      links: [liveLink],
      snapshot: null,
      alerts: [],
      prometheusStatus: null,
    });

    render(
      <CanvasPanels
        panelContent={{
          type: 'interfaceStats',
          data: { linkId: staleLink.id, link: staleLink, sourceDevice, targetDevice },
        }}
        setPanelContent={vi.fn()}
        devices={[sourceDevice, targetDevice]}
        topologyLinks={[liveLink]}
        loadTopology={vi.fn().mockResolvedValue(undefined)}
        setDevices={vi.fn()}
        setNodes={vi.fn()}
        reactFlow={{} as never}
        runtimeState={runtimeState}
      />,
    );

    expect(screen.getByText('Link interface model:ether1')).toBeInTheDocument();
  });

  it('does not render the removed device-scoped interface stats panel', () => {
    const staleDevice = mockDevice({ id: 'dev-2' });
    const liveDevice = mockDevice({ id: 'dev-2', hostname: 'live-router-02' });
    const runtimeState = buildRuntimeState({
      devices: [liveDevice],
      links: [],
      snapshot: null,
      alerts: [],
      prometheusStatus: null,
    });

    render(
      <CanvasPanels
        panelContent={{
          type: 'interfaceStats',
          data: { deviceId: staleDevice.id, device: staleDevice },
        }}
        setPanelContent={vi.fn()}
        devices={[liveDevice]}
        topologyLinks={[]}
        loadTopology={vi.fn().mockResolvedValue(undefined)}
        setDevices={vi.fn()}
        setNodes={vi.fn()}
        reactFlow={{} as never}
        runtimeState={runtimeState}
      />,
    );

    expect(screen.queryByText('Device interface model:dev-2')).not.toBeInTheDocument();
  });
});
