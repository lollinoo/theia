import { render, screen } from '@testing-library/react';
import { afterEach, describe, expect, it, vi } from 'vitest';
import type { Device } from '../types/api';
import type { DeviceMetricsDTO } from '../types/metrics';
import DeviceCard, { resolveDeviceNodeReadabilityScale, type DeviceNodeData } from './DeviceCard';

let mockZoom = 1;

vi.mock('@xyflow/react', () => ({
  Handle: ({ id }: { id?: string }) => <span data-testid={`handle-${id ?? 'default'}`} />,
  Position: {
    Top: 'top',
    Right: 'right',
    Bottom: 'bottom',
    Left: 'left',
  },
  useStore: (selector: (state: { transform: [number, number, number] }) => unknown) =>
    selector({ transform: [0, 0, mockZoom] }),
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

function renderDeviceCard(data: Partial<DeviceNodeData> = {}) {
  const nodeData: DeviceNodeData = {
    device: mockDevice(),
    pinned: false,
    ...data,
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
  afterEach(() => {
    mockZoom = 1;
  });

  it('raises node content scale at low zoom without enlarging at normal zoom', () => {
    expect(resolveDeviceNodeReadabilityScale(1.3)).toBe(1);
    expect(resolveDeviceNodeReadabilityScale(1)).toBe(1);
    expect(resolveDeviceNodeReadabilityScale(0.8)).toBe(1.12);
    expect(resolveDeviceNodeReadabilityScale(0.6)).toBe(1.27);
  });

  it('applies readable low-zoom sizing to physical node content without scaling the frame', () => {
    mockZoom = 0.6;

    renderDeviceCard({ metrics: mockMetrics() });

    expect(screen.getByTestId('device-node-card').style.transform).toBe('');
    expect(screen.getByTestId('physical-node-hostname')).toHaveStyle({
      fontSize: '19.05px',
    });
    expect(screen.getByTestId('physical-node-status-badge')).toHaveStyle({
      fontSize: '13.97px',
    });
    expect(screen.getByTestId('physical-node-address')).toHaveStyle({
      fontSize: '13.97px',
    });
    expect(screen.getByTestId('physical-runtime-readouts')).toHaveStyle({
      height: '50.8px',
    });
  });
});
