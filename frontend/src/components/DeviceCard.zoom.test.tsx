import { readFileSync } from 'fs';
import { join } from 'path';
import { fireEvent, render, screen } from '@testing-library/react';
import { describe, expect, it, vi } from 'vitest';
import type { Device } from '../types/api';
import type { DeviceMetricsDTO } from '../types/metrics';
import DeviceCard, { resolveDeviceNodeReadabilityScale, type DeviceNodeData } from './DeviceCard';
import { resolveDeviceMonitoringState } from './deviceVisualState';

const css = readFileSync(join(__dirname, '../index.css'), 'utf-8').replace(/\r\n/g, '\n');

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
      'calc(15px * var(--theia-device-node-readability-scale, 1) * var(--theia-device-node-identity-scale, 1))',
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

  it('marks zoom bands on secondary content while keeping the card as the overview surface', () => {
    renderDeviceCard({ metrics: mockMetrics() });

    const detail = screen.getByTestId('semantic-detail-node');

    expect(screen.queryByTestId('semantic-overview-node')).not.toBeInTheDocument();
    expect(detail).toHaveClass('topology-semantic-card');
    expect(screen.getByText('IP 10.0.0.1')).toBeInTheDocument();
    expect(screen.getByText('Fresh telemetry')).toBeInTheDocument();
    expect(screen.getByText('CPU')).toBeInTheDocument();
    expect(screen.getByText('MEM')).toBeInTheDocument();
    expect(screen.getByText('Uptime')).toBeInTheDocument();

    expect(screen.getByTestId('physical-node-hostname')).not.toHaveClass(
      'topology-semantic-detail-only',
    );
    expect(screen.getByText('router-01')).toHaveClass('topology-semantic-identity-text');
    expect(screen.getByTestId('physical-node-status-badge')).not.toHaveClass(
      'topology-semantic-detail-only',
    );
    expect(screen.getByTestId('physical-node-status-badge')).toHaveClass(
      'topology-semantic-status-badge',
    );
    expect(screen.getByText('Up')).toHaveClass('topology-semantic-status-label');
    expect(screen.getByTestId('physical-node-address')).toHaveClass(
      'topology-semantic-summary-field',
    );
    expect(screen.getByTestId('physical-node-freshness')).toHaveClass(
      'topology-semantic-detail-only',
    );
    expect(screen.getByTestId('physical-runtime-readouts')).toHaveClass(
      'topology-semantic-detail-only',
    );
    expect(screen.getByTestId('device-node-card')).toHaveAttribute(
      'data-topology-node-variant',
      'physical',
    );
    expect(screen.getByTestId('device-node-card')).toHaveClass('w-[268px]');
  });

  it('uses the same low-zoom identity scale and wrap hook for virtual cards', () => {
    const hostname = 'virtual-core-distribution-node-with-a-long-name';
    renderDeviceCard({
      device: mockDevice({
        id: 'virtual-1',
        device_type: 'virtual',
        hostname,
        sys_name: hostname,
        ip: '172.16.0.1',
      }),
      isVirtual: true,
      metrics: mockMetrics({ device_id: 'virtual-1' }),
    });

    expect(screen.getByTestId('virtual-node-hostname').style.fontSize).toBe(
      'calc(17px * var(--theia-device-node-readability-scale, 1) * var(--theia-device-node-identity-scale, 1))',
    );
    expect(screen.getByText(hostname)).toHaveClass('topology-semantic-identity-text');
    expect(screen.getByTestId('virtual-node-status-badge')).toHaveClass(
      'topology-semantic-status-badge',
    );
    expect(screen.getByTestId('device-node-card')).toHaveAttribute(
      'data-topology-node-variant',
      'virtual-monitorable',
    );
    expect(screen.getByTestId('device-node-card')).toHaveClass('w-[292px]');
  });

  it('keeps a low-zoom status dot available for unmonitored virtual cards', () => {
    renderDeviceCard({
      device: mockDevice({
        id: 'virtual-unmonitored',
        device_type: 'virtual',
        hostname: 'cloud-edge',
        sys_name: 'cloud-edge',
        ip: '',
      }),
      isVirtual: true,
      metrics: null,
    });

    const fallbackStatus = screen.getByTestId('virtual-node-low-zoom-status-badge');

    expect(screen.getByTestId('device-node-card')).toHaveAttribute(
      'data-topology-node-variant',
      'virtual-unmonitored',
    );
    expect(screen.getByTestId('device-node-card')).toHaveClass('w-[232px]');
    expect(screen.queryByTestId('virtual-node-status-badge')).not.toBeInTheDocument();
    expect(fallbackStatus).toHaveClass(
      'topology-semantic-low-zoom-only',
      'topology-semantic-status-badge',
    );
    expect(fallbackStatus).toHaveAttribute('aria-label', 'Unmonitored');
  });

  it('keeps ghost cards on the same semantic identity path', () => {
    renderDeviceCard({
      kind: 'ghost-device',
      isGhost: true,
      device: mockDevice({
        id: 'ghost-1',
        hostname: 'remote-router',
        sys_name: 'remote-router',
        ip: '10.20.30.40',
      }),
    });

    expect(screen.getByTestId('device-node-card')).toHaveAttribute(
      'data-topology-node-variant',
      'ghost-device',
    );
    expect(screen.getByText('cross-area')).toHaveClass('topology-semantic-detail-only');
    expect(screen.getByText('remote-router')).toHaveClass('topology-semantic-identity-text');
  });

  it('keeps the node interaction surface intact across semantic zoom bands', () => {
    const onContextMenu = vi.fn();
    renderDeviceCard({ metrics: mockMetrics(), onContextMenu });

    const card = screen.getByTestId('device-node-card');

    expect(card).toHaveClass('topology-node-card', 'topology-render-contained');

    fireEvent.contextMenu(card);

    expect(onContextMenu).toHaveBeenCalledWith(expect.anything(), 'dev-1');
  });

  it('keeps self-link annotations hidden in overview without geometry overrides', () => {
    renderDeviceCard({
      metrics: mockMetrics(),
      selfLinks: [
        {
          id: 'self-link-1',
          source_device_id: 'dev-1',
          source_if_name: 'ether1',
          target_device_id: 'dev-1',
          target_if_name: 'ether9',
          discovery_protocol: 'lldp',
          source_if_speed: 0,
          source_if_oper_status: 'up',
          target_if_speed: 0,
          target_if_oper_status: 'up',
        },
      ],
    });

    expect(screen.getByRole('button', { name: /view details for self link/i })).toHaveClass(
      'topology-semantic-detail-only',
    );
    expect(css).toContain('--theia-device-node-identity-scale: 1.8;');
    expect(css).toContain('--theia-device-node-identity-scale: 1.45;');
    expect(css).toContain('[data-topology-zoom-band="overview"] .topology-node-card');
    expect(css).toContain('.topology-node-card[data-topology-node-variant="physical"]');
    expect(css).toContain('.topology-node-card[data-topology-node-variant="virtual-monitorable"]');
    expect(css).toContain('.topology-node-card[data-topology-node-variant="virtual-unmonitored"]');
    expect(css).toContain('height: 140px;');
    expect(css).toContain('height: 118px;');
    expect(css).toContain('height: 92px;');
    expect(css).toContain(
      '[data-topology-zoom-band="overview"] .topology-node-card .topology-semantic-header',
    );
    expect(css).toContain('[data-topology-zoom-band="overview"] .topology-semantic-detail-only');
    expect(css).toContain('[data-topology-zoom-band="overview"] .topology-semantic-summary-row');
    expect(css).toContain('[data-topology-zoom-band="overview"] .topology-semantic-summary-field');
    expect(css).toContain('[data-topology-zoom-band="overview"] .topology-semantic-status-label');
    expect(css).toContain('.topology-semantic-low-zoom-only');
    expect(css).toContain('clip: rect(0, 0, 0, 0);');
    expect(css).toContain('overflow-wrap: anywhere;');
    expect(css).toContain('-webkit-line-clamp: 2;');
    expect(css).toContain('.topology-virtual-node-icon-shell');
    expect(css).not.toContain('.topology-semantic-overview');
    expect(css).not.toContain('width: max-content');
    expect(css).not.toContain('max-width: 220px');
    expect(css).not.toContain('max-width: 280px');
    expect(css).not.toContain('min-height: 68px');
    expect(css).not.toContain('[data-testid=');
  });
});
