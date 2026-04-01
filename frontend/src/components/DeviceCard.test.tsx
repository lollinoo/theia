import { describe, it, expect } from 'vitest';
import { render, screen } from '@testing-library/react';
import { ReactFlowProvider } from '@xyflow/react';
import DeviceCard from './DeviceCard';
import type { Device } from '../types/api';
import type { DeviceNodeData, DeviceNode } from './DeviceCard';
import type { NodeProps } from '@xyflow/react';

function mockDevice(overrides: Partial<Device> = {}): Device {
  return {
    id: 'dev-1',
    hostname: 'router-01',
    ip: '10.0.0.1',
    device_type: 'router',
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

  it('virtual node has dashed border and bg-surface/80 (D-01, D-02)', () => {
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
    expect(html).toContain('bg-surface/80');
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
});
