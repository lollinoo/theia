import { describe, it, expect } from 'vitest';
import { render, screen } from '@testing-library/react';
import { ReactFlowProvider } from '@xyflow/react';
import DeviceCard from './DeviceCard';
import type { Device } from '../types/api';
import type { DeviceNodeData } from './DeviceCard';
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
    ...overrides,
  };
}

function makeNodeProps(data: DeviceNodeData): NodeProps<DeviceNodeData> {
  return {
    id: 'node-1',
    data,
    type: 'device',
    selected: false,
    isConnectable: true,
    xPos: 0,
    yPos: 0,
    zIndex: 0,
    dragging: false,
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

  it('uses default ring when no areaColor', () => {
    const { container } = renderDeviceCard();
    const html = container.innerHTML;
    expect(html).toContain('ring-1');
    expect(html).toContain('ring-outline');
  });

  it('renders area color as ring when areaColor set', () => {
    const { container } = renderDeviceCard({ areaColor: '#ff6600' });
    const html = container.innerHTML;
    expect(html).toContain('--tw-ring-color');
    expect(html).toContain('#ff6600');
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

  it('card outer div has ring-primary class when highlighted (cardRingClass glow ring)', () => {
    const { container } = renderDeviceCard({ highlighted: true });
    const html = container.innerHTML;
    expect(html).toContain('ring-primary');
  });

  it('card outer div has ring-1 ring-outline default state when not highlighted or selected', () => {
    const { container } = renderDeviceCard();
    const html = container.innerHTML;
    expect(html).toContain('ring-1');
    expect(html).toContain('ring-outline');
  });
});
