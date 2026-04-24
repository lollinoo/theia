import { act, fireEvent, render, screen } from '@testing-library/react';
import { ReactFlowProvider } from '@xyflow/react';
import type { NodeProps } from '@xyflow/react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import type { Device, Link } from '../types/api';
import type { DeviceMetricsDTO } from '../types/metrics';
import DeviceCard from './DeviceCard';
import type { DeviceNode, DeviceNodeData } from './DeviceCard';

function mockDevice(overrides: Partial<Device> = {}): Device {
  return {
    id: 'dev-1',
    hostname: 'router-01',
    ip: '10.0.0.1',
    device_type: 'router',
    poll_class: 'standard',
    poll_interval_override: null,
    status: 'up',
    sys_name: 'router-01',
    sys_descr: 'RouterOS RB4011',
    hardware_model: 'RB4011',
    vendor: 'mikrotik',
    managed: true,
    interfaces: [],
    backup_supported: true,
    metrics_source: 'prometheus',
    prometheus_label_name: 'instance',
    prometheus_label_value: '10.0.0.1:9100',
    area_ids: [],
    ...overrides,
  };
}

function mockMetrics(overrides: Partial<DeviceMetricsDTO> = {}): DeviceMetricsDTO {
  return {
    device_id: 'dev-1',
    operational_status: 'up',
    primary_health: 'up_fresh',
    runtime_flags: [],
    field_states: { uptime: 'ok', cpu: 'ok', memory: 'ok' },
    network_reachable: 'true',
    snmp_reachable: 'true',
    reachability: 'up',
    cpu_percent: 42,
    mem_percent: 68,
    temp_celsius: 55,
    uptime_secs: 86400,
    health: 'healthy',
    freshness: 'fresh',
    primary_reason: 'ok',
    metrics_status: 'available',
    metrics_reason: 'ok',
    alert_status: 'normal',
    firing_alert_count: 0,
    last_collected_at: '2026-04-13T11:59:45Z',
    last_polled_at: '2026-04-13T11:59:30Z',
    expected_poll_interval_seconds: 30,
    ...overrides,
  };
}

function mockLink(overrides: Partial<Link> = {}): Link {
  return {
    id: 'link-1',
    source_device_id: 'dev-1',
    source_if_name: 'ether1',
    target_device_id: 'dev-1',
    target_if_name: 'ether9',
    discovery_protocol: 'lldp',
    source_if_speed: 0,
    source_if_oper_status: 'up',
    target_if_speed: 0,
    target_if_oper_status: 'up',
    ...overrides,
  };
}

function makeNodeProps(data: DeviceNodeData): NodeProps<DeviceNode> {
  return {
    id: 'node-1',
    data,
    type: 'device',
    selected: false,
    isConnectable: true,
    zIndex: 0,
    dragging: false,
    draggable: true,
    selectable: true,
    deletable: false,
    positionAbsoluteX: 0,
    positionAbsoluteY: 0,
  };
}

function renderDeviceCard(data: Partial<DeviceNodeData> = {}) {
  const nodeData: DeviceNodeData = {
    device: mockDevice(),
    pinned: false,
    ...data,
  };
  const props = makeNodeProps(nodeData);

  return render(
    <ReactFlowProvider>
      <DeviceCard {...props} />
    </ReactFlowProvider>,
  );
}

describe('DeviceCard', () => {
  beforeEach(() => {
    vi.useFakeTimers();
    vi.setSystemTime(new Date('2026-04-13T12:00:00Z'));
  });

  afterEach(() => {
    vi.useRealTimers();
  });

  it('renders compact overview name, type, and IP chip without redundant area badge', () => {
    renderDeviceCard();

    expect(screen.getByText('router-01')).toBeInTheDocument();
    expect(screen.getByText('Router')).toBeInTheDocument();
    expect(screen.getByText('IP 10.0.0.1')).toBeInTheDocument();
    expect(screen.queryByText(/global/i)).toBeNull();
  });

  it('uses MAC chip label for MAC addresses', () => {
    renderDeviceCard({ device: mockDevice({ ip: 'aa:bb:cc:dd:ee:ff' }) });

    expect(screen.getByText('MAC aa:bb:cc:dd:ee:ff')).toBeInTheDocument();
    expect(screen.queryByText(/IP /)).toBeNull();
  });

  it('prefers tags.display_name when available', () => {
    renderDeviceCard({
      device: mockDevice({
        tags: { display_name: 'Core Router' },
        sys_name: 'router-01',
      }),
    });

    expect(screen.getByText('Core Router')).toBeInTheDocument();
  });

  it('keeps overview compact by removing verbose detail rows', () => {
    renderDeviceCard({
      device: mockDevice({
        sys_descr: 'Very long system description',
        hardware_model: 'CCR2004',
      }),
    });

    expect(screen.queryByText('CCR2004')).toBeNull();
    expect(screen.queryByText('Very long system description')).toBeNull();
    expect(screen.queryByText('TEMP')).toBeNull();
  });

  it('renders CPU, MEM, and UP readouts with placeholders when metrics are absent', () => {
    renderDeviceCard({ metrics: null });

    expect(screen.getByText('CPU')).toBeInTheDocument();
    expect(screen.getByText('MEM')).toBeInTheDocument();
    expect(screen.getByText('UP')).toBeInTheDocument();
    expect(screen.queryByText('TEMP')).toBeNull();
    expect(screen.getAllByText('--')).toHaveLength(3);
  });

  it('renders CPU, MEM, and uptime readouts when metrics are present', () => {
    renderDeviceCard({ metrics: mockMetrics({ cpu_percent: 42, mem_percent: 68 }) });

    expect(screen.getByText('42%')).toBeInTheDocument();
    expect(screen.getByText('68%')).toBeInTheDocument();
    expect(screen.getByText('1d')).toBeInTheDocument();
  });

  it('renders freshness copy from the normalized runtime enum while preserving the uptime slot', () => {
    renderDeviceCard({
      metrics: mockMetrics({
        freshness: 'stale',
        cpu_percent: null,
        mem_percent: null,
        uptime_secs: null,
        last_polled_at: '2026-04-13T11:59:20Z',
        expected_poll_interval_seconds: 300,
      }),
    });

    expect(screen.getByText('Stale telemetry')).toBeInTheDocument();
    expect(screen.getByText('Polling every 5m')).toBeInTheDocument();
    expect(screen.getByText('UP')).toBeInTheDocument();
    expect(screen.getAllByText('--')).toHaveLength(3);
  });

  it('renders runtime state badges from polling flags', () => {
    renderDeviceCard({
      metrics: mockMetrics({
        runtime_flags: ['deadline_missed', 'partial_telemetry'],
      }),
    });

    expect(screen.getByText('Late')).toBeInTheDocument();
    expect(screen.getByText('Partial')).toBeInTheDocument();
  });

  it('does not derive freshness text from a client timer', async () => {
    renderDeviceCard({
      metrics: mockMetrics({
        freshness: 'awaiting_poll',
        cpu_percent: null,
        mem_percent: null,
        uptime_secs: null,
        last_polled_at: '2026-04-13T12:00:00Z',
        expected_poll_interval_seconds: 30,
      }),
    });

    expect(screen.getByText('Awaiting first poll')).toBeInTheDocument();

    await act(async () => {
      await vi.advanceTimersByTimeAsync(7_000);
    });

    expect(screen.getByText('Awaiting first poll')).toBeInTheDocument();
  });

  it('renders ghost nodes as cross-area markers without overview metrics', () => {
    renderDeviceCard({
      device: mockDevice({ sys_name: 'Ghost-Router' }),
      isGhost: true,
    });

    expect(screen.getByText('cross-area')).toBeInTheDocument();
    expect(screen.getByText('Ghost-Router')).toBeInTheDocument();
    expect(screen.queryByText('CPU')).toBeNull();
  });

  it('applies area accent styling when area colors are provided', () => {
    const { container } = renderDeviceCard({
      device: mockDevice({ area_ids: ['area-1'] }),
      areaColors: ['#ff6600'],
    });

    expect(container.innerHTML).toContain('rgb(255, 102, 0)');
    expect(screen.queryByText(/1 area/i)).toBeNull();
  });

  it('uses selected token shadow when highlighted', () => {
    const { container } = renderDeviceCard({ highlighted: true });
    expect(container.innerHTML).toContain('var(--color-node-selected)');
  });

  it('renders warning and probing labels from device visual state', () => {
    renderDeviceCard({
      device: mockDevice({ status: 'probing' }),
      metrics: mockMetrics({ health: 'warning' }),
    });

    expect(screen.getByText('Probing')).toBeInTheDocument();
    expect(screen.queryByText('Warning')).toBeNull();
  });

  it('distinguishes critical health from down status in the device badge styling', () => {
    const criticalCard = renderDeviceCard({
      metrics: mockMetrics({ health: 'critical' }),
    });

    expect(screen.getByText('Critical')).toBeInTheDocument();
    expect(criticalCard.container.innerHTML).toContain('var(--nt-node-critical-badge-border)');
    expect(criticalCard.container.innerHTML).not.toContain('var(--nt-node-down-glow)');
    expect(criticalCard.container.innerHTML).not.toContain('topology-node-down-fade');

    criticalCard.unmount();

    const downCard = renderDeviceCard({
      device: mockDevice({ status: 'down' }),
      metrics: mockMetrics({ health: 'critical' }),
    });

    expect(screen.getByText('Down')).toBeInTheDocument();
    expect(downCard.container.innerHTML).toContain('var(--nt-node-down-badge-border)');
    expect(downCard.container.innerHTML).toContain('var(--nt-node-down-ring)');
    expect(downCard.container.innerHTML).toContain('var(--nt-node-down-glow)');
    expect(downCard.container.innerHTML).toContain('topology-node-down-fade');
  });

  it('renders unmonitored virtual nodes with neutral semantics instead of failure UI', () => {
    renderDeviceCard({
      device: mockDevice({
        device_type: 'virtual',
        ip: '',
        status: 'down',
        sys_name: '',
        tags: { display_name: 'AWS Cloud', virtual_subtype: 'cloud' },
      }),
      isVirtual: true,
      subtype: 'cloud',
      metrics: mockMetrics({
        health: 'critical',
        last_polled_at: '2026-04-13T11:59:20Z',
        expected_poll_interval_seconds: 300,
      }),
    });

    expect(screen.getByText('AWS Cloud')).toBeInTheDocument();
    expect(screen.getByText('Cloud')).toBeInTheDocument();
    expect(screen.queryByText('Unmonitored')).toBeNull();
    expect(screen.queryByText('Virtual node')).toBeNull();
    expect(screen.queryByText('Status')).toBeNull();
    expect(screen.queryByText('No IP')).toBeNull();
    expect(screen.queryByText('CPU')).toBeNull();
    expect(screen.queryByText('MEM')).toBeNull();
    expect(screen.queryByText('UP')).toBeNull();
    expect(screen.queryByText(/Fresh ·/)).toBeNull();
    expect(screen.queryByText(/Polling every/)).toBeNull();
  });

  it('renders monitorable virtual nodes with top-right status badge, IP chip, and footer meta', () => {
    renderDeviceCard({
      device: mockDevice({
        device_type: 'virtual',
        ip: '192.168.1.1',
        sys_name: '',
        tags: { display_name: 'Cloud VPN', virtual_subtype: 'cloud' },
      }),
      isVirtual: true,
      subtype: 'cloud',
      metrics: mockMetrics({
        cpu_percent: null,
        mem_percent: null,
        uptime_secs: 86400,
      }),
    });

    expect(screen.queryByText('Virtual node')).toBeNull();
    expect(screen.queryByText('Status')).toBeNull();
    expect(screen.getAllByText('Up')).toHaveLength(1);
    expect(screen.getByText('IP 192.168.1.1')).toBeInTheDocument();
    expect(screen.getByText('Fresh telemetry')).toBeInTheDocument();
    expect(screen.getByText('Polling every 30s')).toBeInTheDocument();
    expect(screen.queryByText('CPU')).toBeNull();
    expect(screen.queryByText('MEM')).toBeNull();
    expect(screen.queryByText('UP')).toBeNull();
    expect(screen.queryByText('TEMP')).toBeNull();
  });

  it('renders monitorable virtual nodes as up even when health is absent', () => {
    renderDeviceCard({
      device: mockDevice({
        device_type: 'virtual',
        ip: '127.0.0.1',
        sys_name: '',
        tags: { display_name: 'Loopback', virtual_subtype: 'server' },
      }),
      isVirtual: true,
      subtype: 'server',
      metrics: mockMetrics({
        health: undefined,
        cpu_percent: null,
        mem_percent: null,
        uptime_secs: null,
      }),
    });

    expect(screen.getAllByText('Up')).toHaveLength(1);
    expect(screen.queryByText('Unknown')).toBeNull();
  });

  it('enforces a 200x160 minimum size and 285x235 maximum size for virtual nodes', () => {
    const unmonitored = renderDeviceCard({
      device: mockDevice({
        device_type: 'virtual',
        ip: '',
      }),
      isVirtual: true,
    });

    const unmonitoredCard = unmonitored.container.querySelector('.group');
    expect(unmonitoredCard?.className).toContain('min-w-[200px]');
    expect(unmonitoredCard?.className).toContain('min-h-[160px]');
    expect(unmonitoredCard?.className).toContain('max-w-[285px]');
    expect(unmonitoredCard?.className).toContain('max-h-[235px]');

    unmonitored.unmount();

    const monitorable = renderDeviceCard({
      device: mockDevice({
        device_type: 'virtual',
        ip: '192.168.1.1',
      }),
      isVirtual: true,
    });

    const monitorableCard = monitorable.container.querySelector('.group');
    expect(monitorableCard?.className).toContain('min-w-[200px]');
    expect(monitorableCard?.className).toContain('min-h-[160px]');
    expect(monitorableCard?.className).toContain('max-w-[285px]');
    expect(monitorableCard?.className).toContain('max-h-[235px]');
  });

  it('truncates long virtual node text inside the size-capped card', () => {
    const longName =
      'Virtual node with an intentionally very long display name for truncation checks';
    const longAddress = 'edge-gateway-with-an-extremely-long-hostname.example.internal';

    renderDeviceCard({
      device: mockDevice({
        device_type: 'virtual',
        ip: longAddress,
        sys_name: '',
        tags: { display_name: longName, virtual_subtype: 'cloud' },
      }),
      isVirtual: true,
      subtype: 'cloud',
    });

    expect(screen.getByText(longName).className).toContain('truncate');
    expect(screen.getByText(`IP ${longAddress}`).className).toContain('truncate');
  });

  it('uses monospace for technical readouts and address chips', () => {
    const { container } = renderDeviceCard({
      metrics: mockMetrics({
        cpu_percent: null,
        mem_percent: null,
        uptime_secs: 86400,
      }),
    });

    expect(container.querySelectorAll('.font-mono').length).toBeGreaterThanOrEqual(3);
  });

  it('keeps unmonitored virtual nodes off failure borders even if raw device data is down', () => {
    const { container } = renderDeviceCard({
      device: mockDevice({
        device_type: 'virtual',
        ip: '',
        status: 'down',
      }),
      isVirtual: true,
      metrics: mockMetrics({ health: 'critical' }),
    });

    expect(container.innerHTML).not.toContain('var(--nt-node-down-glow)');
    expect(container.innerHTML).not.toContain('var(--nt-node-down-badge-border)');
    expect(container.innerHTML).not.toContain('var(--nt-node-critical-badge-border)');
  });

  it('renders a self-link badge and opens link details from the node annotation', () => {
    const selfLink = mockLink();
    const onSelfLinkClick = vi.fn();

    renderDeviceCard({
      selfLinks: [selfLink],
      onSelfLinkClick,
    });

    expect(screen.getByText('Self LLDP')).toBeInTheDocument();
    expect(screen.getByText('ether1 -> ether9')).toBeInTheDocument();

    fireEvent.click(
      screen.getByRole('button', {
        name: 'View details for self link ether1 -> ether9',
      }),
    );

    expect(onSelfLinkClick).toHaveBeenCalledWith(selfLink);
  });
});
