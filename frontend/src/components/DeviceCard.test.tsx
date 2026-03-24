import { describe, it, expect } from 'vitest';
import { render, screen } from '@testing-library/react';
import { ReactFlowProvider } from 'reactflow';
import DeviceCard from './DeviceCard';
import type { Device } from '../types/api';
import type { DeviceNodeData } from './DeviceCard';
import type { NodeProps } from 'reactflow';

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

  it('shows vendor badge for non-default vendors', () => {
    renderDeviceCard({ device: mockDevice({ vendor: 'mikrotik' }) });

    expect(screen.getByText('Mikrotik')).toBeInTheDocument();
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
});
