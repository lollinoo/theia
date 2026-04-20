import { describe, it, expect, vi } from 'vitest';
import { render, screen } from '@testing-library/react';
import AreaHub from './AreaHub';
import type { Area, Device, Link } from '../types/api';

// Mock AreaCard to isolate AreaHub tests
vi.mock('./AreaCard', () => ({
  default: ({ area, deviceCount, activeLinkCount, healthLabel }: {
    area: { name: string };
    deviceCount: number;
    activeLinkCount: number;
    healthLabel: string;
  }) => (
    <div data-testid={`area-card-${area.name}`}>
      <span>{area.name}</span>
      <span data-testid="device-count">{deviceCount}</span>
      <span data-testid="link-count">{activeLinkCount}</span>
      <span data-testid="health-label">{healthLabel}</span>
    </div>
  ),
}));

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

function mockArea(overrides: Partial<Area> = {}): Area {
  return {
    id: 'area-1',
    name: 'Backbone',
    description: 'Core backbone area',
    color: '#00E676',
    device_count: 5,
    created_at: '2026-01-01T00:00:00Z',
    updated_at: '2026-01-01T00:00:00Z',
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
    source_if_speed: 0,
    source_if_oper_status: '',
    target_if_speed: 0,
    target_if_oper_status: '',
    ...overrides,
  };
}

describe('AreaHub', () => {
  it('renders heading and subtitle', () => {
    render(
        <AreaHub
          devices={[]}
          areas={[]}
          links={[]}
          onAreaSelect={() => {}}
          onOpenSettings={() => {}}
        />,
    );

    expect(screen.getByText('OSPF Area Hub')).toBeInTheDocument();
    expect(screen.getByText('Global Network Aggregate Overview')).toBeInTheDocument();
  });

  it('renders four stat cards with correct labels', () => {
    render(
        <AreaHub
          devices={[mockDevice()]}
          areas={[]}
          links={[mockLink()]}
          onAreaSelect={() => {}}
          onOpenSettings={() => {}}
        />,
    );

    expect(screen.getByText('Aggregate Health')).toBeInTheDocument();
    expect(screen.getByText('Total Devices')).toBeInTheDocument();
    expect(screen.getByText('Active Links')).toBeInTheDocument();
  });

  it('renders empty state CTA when no areas exist', () => {
    render(
        <AreaHub
          devices={[mockDevice()]}
          areas={[]}
          links={[]}
          onAreaSelect={() => {}}
          onOpenSettings={() => {}}
        />,
    );

    expect(screen.getByText('No areas yet')).toBeInTheDocument();
    expect(screen.getByText('Create your first area in Settings')).toBeInTheDocument();
    expect(screen.getByText('Open Settings')).toBeInTheDocument();
  });

  it('renders an AreaCard for each area', () => {
    const areas = [
      mockArea({ id: 'a1', name: 'Backbone' }),
      mockArea({ id: 'a2', name: 'North Region', color: '#2979FF' }),
    ];

    const devices = [
      mockDevice({ id: 'dev-1', area_ids: ['a1'], status: 'up' }),
      mockDevice({ id: 'dev-2', area_ids: ['a2'], status: 'up' }),
    ];

    render(
        <AreaHub
          devices={devices}
          areas={areas}
          links={[]}
          onAreaSelect={() => {}}
          onOpenSettings={() => {}}
        />,
    );

    expect(screen.getByTestId('area-card-Backbone')).toBeInTheDocument();
    expect(screen.getByTestId('area-card-North Region')).toBeInTheDocument();
  });

  it('computes health correctly: all up = Optimal, 80% = Degraded, 70% = Critical', () => {
    // 10 devices, all up => 100% Optimal
    const allUpDevices = Array.from({ length: 10 }, (_, i) =>
      mockDevice({ id: `dev-${i}`, area_ids: ['a1'], status: 'up' }),
    );

    const { rerender } = render(
        <AreaHub
          devices={allUpDevices}
          areas={[mockArea({ id: 'a1', name: 'TestArea' })]}
          links={[]}
          onAreaSelect={() => {}}
          onOpenSettings={() => {}}
        />,
    );

    // With all devices up, area card should show "Optimal"
    expect(screen.getByTestId('health-label')).toHaveTextContent('Optimal');

    // 10 devices, 8 up, 2 down => 80% Degraded
    const mixedDevices = Array.from({ length: 10 }, (_, i) =>
      mockDevice({ id: `dev-${i}`, area_ids: ['a1'], status: i < 8 ? 'up' : 'down' }),
    );

    rerender(
        <AreaHub
          devices={mixedDevices}
          areas={[mockArea({ id: 'a1', name: 'TestArea' })]}
          links={[]}
          onAreaSelect={() => {}}
          onOpenSettings={() => {}}
        />,
    );

    expect(screen.getByTestId('health-label')).toHaveTextContent('Degraded');

    // 10 devices, 7 up, 3 down => 70% Critical
    const criticalDevices = Array.from({ length: 10 }, (_, i) =>
      mockDevice({ id: `dev-${i}`, area_ids: ['a1'], status: i < 7 ? 'up' : 'down' }),
    );

    rerender(
        <AreaHub
          devices={criticalDevices}
          areas={[mockArea({ id: 'a1', name: 'TestArea' })]}
          links={[]}
          onAreaSelect={() => {}}
          onOpenSettings={() => {}}
        />,
    );

    expect(screen.getByTestId('health-label')).toHaveTextContent('Critical');
  });

  it('uses websocket status overrides when computing health', () => {
    const devices = [
      mockDevice({ id: 'dev-1', area_ids: ['a1'], status: 'down' }),
      mockDevice({ id: 'dev-2', area_ids: ['a1'], status: 'up' }),
    ];

    render(
      <AreaHub
        devices={devices}
        areas={[mockArea({ id: 'a1', name: 'TestArea' })]}
        links={[]}
        onAreaSelect={() => {}}
        onOpenSettings={() => {}}
      />,
    );

    expect(screen.getByTestId('health-label')).toHaveTextContent('Critical');
  });
});
