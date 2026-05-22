import { fireEvent, render, screen, within } from '@testing-library/react';
import { afterEach, describe, expect, it, vi } from 'vitest';
import type { Device } from '../types/api';
import type { DeviceMetricsDTO } from '../types/metrics';
import DeviceCard, { resolveDeviceNodeReadabilityScale, type DeviceNodeData } from './DeviceCard';
import { resolveDeviceMonitoringState } from './deviceVisualState';

vi.mock('@xyflow/react', () => ({
  Handle: ({ id }: { id?: string }) => <span data-testid={`handle-${id ?? 'default'}`} />,
  Position: {
    Top: 'top',
    Right: 'right',
    Bottom: 'bottom',
    Left: 'left',
  },
  useStore: () => {
    throw new Error('DeviceCard must not subscribe to the React Flow viewport store');
  },
}));

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

function renderDeviceCard(
  data: Partial<DeviceNodeData> & { metrics?: DeviceMetricsDTO | null } = {},
) {
  const device = data.device ?? mockDevice();
  const { metrics, runtime, ...staticData } = data;
  const nodeData: DeviceNodeData = {
    device,
    runtime: runtime ?? {
      status: device.status,
      metrics: metrics ?? null,
      alertStatus: 'normal',
      monitoringState: resolveDeviceMonitoringState(device),
    },
    pinned: false,
    ...staticData,
  };

  return render(
    <DeviceCard
      {...({
        id: 'node-1',
        data: nodeData,
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
      } as never)}
    />,
  );
}

describe('DeviceCard zoom readability', () => {
  it('raises node content scale at low zoom without enlarging at normal zoom', () => {
    expect(resolveDeviceNodeReadabilityScale(1.3)).toBe(1);
    expect(resolveDeviceNodeReadabilityScale(1)).toBe(1);
    expect(resolveDeviceNodeReadabilityScale(0.8)).toBe(1.04);
    expect(resolveDeviceNodeReadabilityScale(0.6)).toBe(1.12);
  });

  it('uses the canvas readability CSS variable without subscribing every card to zoom changes', () => {
    renderDeviceCard({ metrics: mockMetrics() });

    expect(screen.getByTestId('device-node-card').style.transform).toBe('');
    expect(screen.getByTestId('physical-node-hostname').style.fontSize).toBe(
      'calc(15px * var(--theia-device-node-readability-scale, 1))',
    );
    expect(screen.getByTestId('physical-node-status-badge').style.fontSize).toBe(
      'calc(11px * var(--theia-device-node-readability-scale, 1))',
    );
    expect(screen.getByTestId('physical-node-address').style.fontSize).toBe(
      'calc(11px * var(--theia-device-node-readability-scale, 1))',
    );
    expect(screen.getByTestId('physical-runtime-readouts').style.height).toBe(
      'calc(40px * var(--theia-device-node-readability-scale, 1))',
    );
  });

  it('marks detail-only sections while exposing a textless overview status glyph', () => {
    renderDeviceCard({ metrics: mockMetrics() });

    const overview = screen.getByTestId('semantic-overview-node');
    const detail = screen.getByTestId('semantic-detail-node');

    expect(overview).toHaveAttribute('aria-label', 'router-01 Up');
    expect(within(overview).queryByText('router-01')).not.toBeInTheDocument();
    expect(within(overview).queryByText('10.0.0.1')).not.toBeInTheDocument();

    expect(detail).toHaveClass('topology-semantic-card');
    expect(within(detail).getByText('IP 10.0.0.1')).toBeInTheDocument();
    expect(within(detail).getByText('Fresh telemetry')).toBeInTheDocument();
    expect(within(detail).getByText('CPU')).toBeInTheDocument();
    expect(within(detail).getByText('MEM')).toBeInTheDocument();
    expect(within(detail).getByText('Uptime')).toBeInTheDocument();

    expect(screen.getByTestId('physical-node-hostname')).not.toHaveClass(
      'topology-semantic-detail-only',
    );
    expect(screen.getByTestId('physical-node-status-badge')).not.toHaveClass(
      'topology-semantic-detail-only',
    );
    expect(screen.getByTestId('physical-node-address')).toHaveClass(
      'topology-semantic-summary-field',
    );
    expect(screen.getByTestId('physical-node-freshness')).toHaveClass(
      'topology-semantic-detail-only',
    );
    expect(screen.getByTestId('physical-runtime-readouts')).toHaveClass(
      'topology-semantic-detail-only',
    );
  });

  it('keeps the node interaction surface intact while exposing overview status structure', () => {
    const onContextMenu = vi.fn();
    renderDeviceCard({ metrics: mockMetrics(), onContextMenu });

    const card = screen.getByTestId('device-node-card');
    const overview = screen.getByTestId('semantic-overview-node');

    expect(overview).toHaveAttribute('role', 'img');
    expect(overview).toHaveClass('topology-semantic-overview');

    fireEvent.contextMenu(card);

    expect(onContextMenu).toHaveBeenCalledWith(expect.anything(), 'dev-1');
  });
});
