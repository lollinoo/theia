import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { act, render, screen } from '@testing-library/react';
import { ReactFlowProvider } from '@xyflow/react';
import DeviceCard from './DeviceCard';
import type { Device } from '../types/api';
import type { DeviceNodeData, DeviceNode } from './DeviceCard';
import type { NodeProps } from '@xyflow/react';
import type { DeviceMetricsDTO } from '../types/metrics';

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
    cpu_percent: 42,
    mem_percent: 68,
    temp_celsius: 55,
    uptime_secs: 86400,
    collected_at: '2026-04-13T11:59:45Z',
    health: 'healthy',
    stale: false,
    last_polled_at: '2026-04-13T11:59:30Z',
    expected_poll_interval_seconds: 30,
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

  it('renders device name and IP', () => {
    renderDeviceCard();

    // DeviceCard uses displayName() which shows tags.display_name || sys_name || ip
    // Our mock has sys_name = 'router-01'
    expect(screen.getByText('router-01')).toBeInTheDocument();
    expect(screen.getByText('10.0.0.1')).toBeInTheDocument();
  });

  it('displays IP label correctly for IPv4 addresses', () => {
    renderDeviceCard();

    expect(screen.getByText('IP:')).toBeInTheDocument();
  });

  it('displays IP label correctly for IPv6 addresses', () => {
    renderDeviceCard({ device: mockDevice({ ip: '2001:db8::10' }) });

    expect(screen.getByText('IP:')).toBeInTheDocument();
    expect(screen.queryByText('MAC:')).toBeNull();
  });

  it('displays MAC label for MAC addresses', () => {
    renderDeviceCard({ device: mockDevice({ ip: 'aa:bb:cc:dd:ee:ff' }) });

    expect(screen.getByText('MAC:')).toBeInTheDocument();
  });

  it('shows vendor icon for non-default vendors', () => {
    const { container } = renderDeviceCard({ device: mockDevice({ vendor: 'mikrotik' }) });

    // Vendor icon renders as a single SVG in the header
    const svgs = container.querySelectorAll('svg[aria-hidden="true"]');
    expect(svgs.length).toBeGreaterThanOrEqual(1);
  });

  it('renders metrics section with placeholder values when no metrics', () => {
    renderDeviceCard({ metrics: null });

    expect(screen.getByText('CPU')).toBeInTheDocument();
    expect(screen.getByText('MEM')).toBeInTheDocument();
    expect(screen.getByText('TEMP')).toBeInTheDocument();
    expect(screen.getByText('UP')).toBeInTheDocument();
    // Without metrics, should show placeholder values
    expect(screen.getAllByText('--%')).toHaveLength(2); // CPU and MEM
    expect(screen.getByText('N/A')).toBeInTheDocument(); // TEMP
    expect(screen.getByText('--')).toBeInTheDocument(); // UP
  });

  it('renders metrics when provided', () => {
    renderDeviceCard({
      metrics: {
        device_id: 'dev-1',
        cpu_percent: 42,
        mem_percent: 68,
        temp_celsius: 55,
        uptime_secs: 86400,
        collected_at: '2026-01-01T00:00:00Z',
      },
    });

    expect(screen.getByText('42%')).toBeInTheDocument();
    expect(screen.getByText('68%')).toBeInTheDocument();
    expect(screen.getByText('55C')).toBeInTheDocument();
    expect(screen.getByText('1d')).toBeInTheDocument();
  });

  it('shows display_name from tags when available', () => {
    renderDeviceCard({
      device: mockDevice({
        tags: { display_name: 'Core Router' },
        sys_name: 'router-01',
      }),
    });

    expect(screen.getByText('Core Router')).toBeInTheDocument();
  });

  it('renders without crashing', () => {
    const { container } = renderDeviceCard();
    expect(container.firstChild).toBeTruthy();
  });

  it('uses default outline border when no areaColors', () => {
    const { container } = renderDeviceCard();
    const html = container.innerHTML;
    expect(html).toContain('var(--color-outline)');
  });

  it('renders area color as wrapper border when areaColors set', () => {
    const { container } = renderDeviceCard({ areaColors: ['#ff6600'] });
    const html = container.innerHTML;
    // jsdom converts hex to rgb in inline styles
    expect(html).toContain('rgb(255, 102, 0)');
  });

  it('does not render decorative bottom ports', () => {
    const { container } = renderDeviceCard();
    // The old card had 6 decorative dot divs in a bottom row
    const portDots = container.querySelectorAll('.absolute.-bottom-1 .rounded-full');
    expect(portDots.length).toBe(0);
  });

  it('shows generic icon for default vendor', () => {
    const { container } = renderDeviceCard({ device: mockDevice({ vendor: 'default' }) });

    // Generic vendor icon still renders as SVG
    const svgs = container.querySelectorAll('svg[aria-hidden="true"]');
    expect(svgs.length).toBeGreaterThanOrEqual(1);
  });

  it('renders metrics section with bg-surface-high surface tier', () => {
    const { container } = renderDeviceCard();
    // The metrics area should use bg-surface-high instead of border-t
    const metricsArea = container.querySelector('.bg-surface-high');
    expect(metricsArea).not.toBeNull();
    // Should NOT have a border-t separator
    const html = container.innerHTML;
    expect(html).not.toContain('border-t border-outline');
  });

  it('renders ghost node with hostname only and no metrics', () => {
    renderDeviceCard({
      device: mockDevice({ sys_name: 'Ghost-Router' }),
      isGhost: true,
    });

    expect(screen.getByText('Ghost-Router')).toBeInTheDocument();
    // Ghost nodes should NOT show metric labels
    expect(screen.queryByText('CPU')).toBeNull();
    expect(screen.queryByText('MEM')).toBeNull();
    expect(screen.queryByText('TEMP')).toBeNull();
    expect(screen.queryByText('UP')).toBeNull();
  });

  it('ghost node has dashed border', () => {
    const { container } = renderDeviceCard({
      device: mockDevice({ sys_name: 'Ghost-Switch' }),
      isGhost: true,
    });

    const html = container.innerHTML;
    expect(html).toContain('border-dashed');
  });

  it('metric value cells have font-mono class (JetBrains Mono — COMP-01)', () => {
    const { container } = renderDeviceCard();
    // All 4 metric value cells (CPU, MEM, TEMP, UP) should have font-mono
    const fontMonoEls = container.querySelectorAll('.font-mono');
    // At least 4 font-mono elements for metric cells plus the IP address
    expect(fontMonoEls.length).toBeGreaterThanOrEqual(4);
  });

  it('card wrapper uses primary color when highlighted', () => {
    const { container } = renderDeviceCard({ highlighted: true });
    const html = container.innerHTML;
    expect(html).toContain('var(--color-primary)');
  });

  it('card wrapper uses outline color in default state when not highlighted or selected', () => {
    const { container } = renderDeviceCard();
    const html = container.innerHTML;
    expect(html).toContain('var(--color-outline)');
  });

  it('renders virtual node with subtype icon and display name (VIRT-06)', () => {
    renderDeviceCard({
      device: mockDevice({
        device_type: 'virtual',
        ip: '',
        sys_name: '',
        tags: { display_name: 'ISP Gateway', virtual_subtype: 'internet' },
      }),
      isVirtual: true,
      subtype: 'internet',
    });

    expect(screen.getByText('ISP Gateway')).toBeInTheDocument();
    expect(screen.getByText('language')).toBeInTheDocument(); // Material Symbol ligature
    // Should NOT show physical card metrics
    expect(screen.queryByText('CPU')).toBeNull();
    expect(screen.queryByText('MEM')).toBeNull();
  });

  it('renders virtual node with IP at 200px width with StatusDot (VIRT-07)', () => {
    const { container } = renderDeviceCard({
      device: mockDevice({
        device_type: 'virtual',
        ip: '192.168.1.1',
        sys_name: '',
        tags: { display_name: 'Cloud VPN', virtual_subtype: 'cloud' },
      }),
      isVirtual: true,
      subtype: 'cloud',
    });

    expect(screen.getByText('Cloud VPN')).toBeInTheDocument();
    expect(screen.getByText('192.168.1.1')).toBeInTheDocument();
    expect(screen.getByText('IP:')).toBeInTheDocument();
    // 200px width class
    const html = container.innerHTML;
    expect(html).toContain('w-[200px]');
  });

  it('renders virtual node without IP at 160px width with no body (VIRT-08)', () => {
    const { container } = renderDeviceCard({
      device: mockDevice({
        device_type: 'virtual',
        ip: '',
        sys_name: '',
        tags: { display_name: 'AWS Cloud', virtual_subtype: 'cloud' },
      }),
      isVirtual: true,
      subtype: 'cloud',
    });

    expect(screen.getByText('AWS Cloud')).toBeInTheDocument();
    expect(screen.queryByText('IP:')).toBeNull();
    const html = container.innerHTML;
    expect(html).toContain('w-[160px]');
  });

  it('virtual node without IP does not render freshness or polling metadata even if metrics exist', () => {
    renderDeviceCard({
      device: mockDevice({
        device_type: 'virtual',
        ip: '',
        sys_name: '',
        tags: { display_name: 'AWS Cloud', virtual_subtype: 'cloud' },
      }),
      isVirtual: true,
      subtype: 'cloud',
      metrics: mockMetrics({
        health: 'warning',
        last_polled_at: '2026-04-13T11:59:20Z',
        expected_poll_interval_seconds: 60,
      }),
    });

    expect(screen.queryByText('Fresh · 40s ago')).toBeNull();
    expect(screen.queryByText('Polling every 1m')).toBeNull();
    expect(screen.queryByText('Warning')).toBeNull();
  });

  it('virtual node has dashed border and opaque bg-surface (D-01, D-02)', () => {
    const { container } = renderDeviceCard({
      device: mockDevice({
        device_type: 'virtual',
        ip: '',
        tags: { display_name: 'Test', virtual_subtype: 'generic' },
      }),
      isVirtual: true,
      subtype: 'generic',
    });

    const html = container.innerHTML;
    expect(html).toContain('border-dashed');
    expect(html).toContain('bg-surface');
    // Must NOT use semi-transparent bg-surface/80 — area color would bleed through
    expect(html).not.toContain('bg-surface/80');
  });

  it('virtual node uses hub icon for generic subtype fallback (D-12)', () => {
    renderDeviceCard({
      device: mockDevice({
        device_type: 'virtual',
        ip: '',
        tags: { display_name: 'Generic Node' },
      }),
      isVirtual: true,
      subtype: 'generic',
    });

    expect(screen.getByText('hub')).toBeInTheDocument();
  });

  it('virtual node shows area color wrapper when areaColors set (D-11)', () => {
    const { container } = renderDeviceCard({
      device: mockDevice({
        device_type: 'virtual',
        ip: '',
        tags: { display_name: 'Area Node', virtual_subtype: 'server' },
      }),
      isVirtual: true,
      subtype: 'server',
      areaColors: ['#ff6600'],
    });

    const html = container.innerHTML;
    expect(html).toContain('rgb(255, 102, 0)');
  });

  it('virtual node without IP and with area does not show animate-pulse glow', () => {
    const { container } = renderDeviceCard({
      device: mockDevice({
        device_type: 'virtual',
        ip: '',
        status: 'unknown',
        tags: { display_name: 'Area Node', virtual_subtype: 'internet' },
      }),
      isVirtual: true,
      subtype: 'internet',
      areaColors: ['#2979FF'],
    });

    const html = container.innerHTML;
    // Virtual nodes without IP should never pulse — they are not "offline"
    expect(html).not.toContain('animate-pulse');
  });

  it('virtual node with area has opaque interior so area color only shows as border', () => {
    const { container } = renderDeviceCard({
      device: mockDevice({
        device_type: 'virtual',
        ip: '',
        tags: { display_name: 'Border Test', virtual_subtype: 'cloud' },
      }),
      isVirtual: true,
      subtype: 'cloud',
      areaColors: ['#ff6600'],
    });

    const html = container.innerHTML;
    // Inner card must use fully opaque bg-surface (not bg-surface/80)
    // so the wrapper area color only shows through the 1.5px padding as a border
    expect(html).not.toContain('bg-surface/80');
  });

  it('renders explicit health label next to the status dot on physical cards', () => {
    renderDeviceCard({
      metrics: mockMetrics({ health: 'warning' }),
    });

    expect(screen.getByText('Warning')).toBeInTheDocument();
  });

  it('prefers device probing status over stale warning health on physical cards', () => {
    renderDeviceCard({
      device: mockDevice({ status: 'probing' }),
      metrics: mockMetrics({ health: 'warning' }),
    });

    expect(screen.getByText('Probing')).toBeInTheDocument();
    expect(screen.queryByText('Warning')).toBeNull();
  });

  it('prefers device down status over stale warning health on physical cards', () => {
    renderDeviceCard({
      device: mockDevice({ status: 'down' }),
      metrics: mockMetrics({ health: 'warning' }),
    });

    expect(screen.getByText('Down')).toBeInTheDocument();
    expect(screen.queryByText('Warning')).toBeNull();
  });

  it('renders freshness metadata from last_polled_at and expected interval on physical cards', () => {
    renderDeviceCard({
      metrics: mockMetrics({
        health: 'warning',
        last_polled_at: '2026-04-13T11:59:20Z',
        expected_poll_interval_seconds: 30,
      }),
    });

    expect(screen.getByText('Fresh · 40s ago')).toBeInTheDocument();
  });

  it('updates freshness metadata over time without requiring a click rerender', async () => {
    renderDeviceCard({
      metrics: mockMetrics({
        last_polled_at: '2026-04-13T12:00:00Z',
        expected_poll_interval_seconds: 30,
      }),
    });

    expect(screen.getByText('Fresh · 0s ago')).toBeInTheDocument();

    for (let i = 0; i < 7; i += 1) {
      await act(async () => {
        await vi.advanceTimersByTimeAsync(1_000);
      });
    }

    expect(screen.getByText('Fresh · 7s ago')).toBeInTheDocument();
  });

  it('renders waiting-for-first-poll freshness copy when timestamp missing', () => {
    renderDeviceCard({
      metrics: mockMetrics({
        last_polled_at: undefined,
      }),
    });

    expect(screen.getByText('Dead · Waiting for first poll')).toBeInTheDocument();
  });

  it('renders polling cadence copy from expected interval', () => {
    renderDeviceCard({
      metrics: mockMetrics({
        expected_poll_interval_seconds: 300,
      }),
    });

    expect(screen.getByText('Polling every 5m')).toBeInTheDocument();
  });

  it('preserves detail/model row while showing phase 44 metadata', () => {
    renderDeviceCard({
      device: mockDevice({
        sys_name: 'router-01',
        hardware_model: 'CCR2004',
      }),
      metrics: mockMetrics({
        health: 'healthy',
      }),
    });

    expect(screen.getByText('CCR2004')).toBeInTheDocument();
    expect(screen.getByText('Fresh · 30s ago')).toBeInTheDocument();
    expect(screen.getByText('Polling every 30s')).toBeInTheDocument();
  });

  it('shows freshness and polling metadata even when device status is down', () => {
    renderDeviceCard({
      device: mockDevice({ status: 'down' }),
      metrics: mockMetrics({
        health: 'critical',
        last_polled_at: '2026-04-13T11:57:30Z',
        expected_poll_interval_seconds: 30,
      }),
    });

    expect(screen.getByText('Stale · 2m ago')).toBeInTheDocument();
    expect(screen.getByText('Polling every 30s')).toBeInTheDocument();
  });

  it('renders virtual node health label next to the status dot from backend metrics', () => {
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
        health: 'critical',
      }),
    });

    expect(screen.getByText('Critical')).toBeInTheDocument();
  });

  it('prefers virtual probing status over stale warning health when the node has an IP', () => {
    renderDeviceCard({
      device: mockDevice({
        device_type: 'virtual',
        ip: '192.168.1.1',
        status: 'probing',
        sys_name: '',
        tags: { display_name: 'Cloud VPN', virtual_subtype: 'cloud' },
      }),
      isVirtual: true,
      subtype: 'cloud',
      metrics: mockMetrics({
        health: 'warning',
      }),
    });

    expect(screen.getByText('Probing')).toBeInTheDocument();
    expect(screen.queryByText('Warning')).toBeNull();
  });

  it('renders virtual node freshness and cadence metadata from overview snapshot metrics', () => {
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
        health: 'warning',
        last_polled_at: '2026-04-13T11:59:20Z',
        expected_poll_interval_seconds: 60,
      }),
    });

    expect(screen.getByText('Fresh · 40s ago')).toBeInTheDocument();
    expect(screen.getByText('Polling every 1m')).toBeInTheDocument();
  });
});
