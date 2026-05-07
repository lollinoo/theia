import { fireEvent, render, screen } from '@testing-library/react';
import { describe, expect, it, vi } from 'vitest';
import type { Area, CanvasMap, Device, Link } from '../../types/api';
import type { SnapshotPayload } from '../../types/metrics';
import { TopologyHub } from './TopologyHub';

function mockArea(overrides: Partial<Area> = {}): Area {
  return {
    id: 'area-1',
    name: 'Backbone',
    description: '',
    color: '#2979FF',
    device_count: 0,
    created_at: '',
    updated_at: '',
    ...overrides,
  };
}

function mockDevice(overrides: Partial<Device> = {}): Device {
  return {
    id: 'device-1',
    hostname: 'router-1',
    ip: '10.0.0.1',
    notes: null,
    device_type: 'router',
    poll_class: 'standard',
    poll_interval_override: null,
    polling_enabled: true,
    status: 'up',
    sys_name: 'router-1',
    sys_descr: '',
    hardware_model: '',
    os_version: '',
    vendor: 'mikrotik',
    managed: true,
    tags: {},
    interfaces: [],
    area_ids: ['area-1'],
    backup_supported: false,
    metrics_source: 'snmp',
    prometheus_label_name: 'instance',
    prometheus_label_value: '10.0.0.1:9100',
    topology_discovery_mode: 'inherit',
    effective_topology_discovery_mode: 'lldp',
    topology_bootstrap_state: 'idle',
    last_topology_discovery_at: null,
    last_topology_discovery_result: '',
    ...overrides,
  };
}

function mockLink(overrides: Partial<Link> = {}): Link {
  return {
    id: 'link-1',
    source_device_id: 'device-1',
    source_if_name: 'ether1',
    target_device_id: 'device-2',
    target_if_name: 'ether2',
    discovery_protocol: 'lldp',
    source_if_speed: 1_000_000_000,
    source_if_oper_status: 'up',
    target_if_speed: 1_000_000_000,
    target_if_oper_status: 'up',
    ...overrides,
  };
}

function mockMap(overrides: Partial<CanvasMap> = {}): CanvasMap {
  return {
    id: 'default',
    name: 'Default',
    description: '',
    source_area_id: null,
    filter: {},
    is_default: true,
    device_count: 1,
    link_count: 1,
    position_count: 1,
    created_at: '',
    updated_at: '',
    ...overrides,
  };
}

describe('TopologyHub', () => {
  it('renders runtime-aware hub content without legacy OSPF text', () => {
    const area = mockArea();
    const onOpenGlobal = vi.fn();
    const onOpenArea = vi.fn();
    const onCreateMapFromArea = vi.fn();
    const snapshot = {
      devices: { 'device-1': { status: 'down', alert_status: 'critical' } },
      links: {},
    } as unknown as SnapshotPayload;

    const { container } = render(
      <TopologyHub
        devices={[mockDevice()]}
        areas={[area]}
        links={[mockLink()]}
        snapshot={snapshot}
        maps={[mockMap()]}
        mapsLoading={false}
        mapsError={null}
        savedMapsEnabled={true}
        onOpenGlobal={onOpenGlobal}
        onOpenArea={onOpenArea}
        onOpenMap={vi.fn()}
        onCreateMapFromArea={onCreateMapFromArea}
        onDuplicateMap={vi.fn()}
        onDeleteMap={vi.fn()}
        onOpenSettings={vi.fn()}
      />,
    );

    expect(screen.getByRole('heading', { name: 'Topology Hub' })).toBeInTheDocument();
    expect(container).not.toHaveTextContent('OSPF');
    expect(screen.getByText('Needs attention')).toBeInTheDocument();

    fireEvent.click(screen.getByRole('button', { name: 'Open global map' }));
    fireEvent.click(screen.getByRole('button', { name: 'Open area Backbone' }));
    fireEvent.click(screen.getByRole('button', { name: 'Create map from area Backbone' }));

    expect(onOpenGlobal).toHaveBeenCalledOnce();
    expect(onOpenArea).toHaveBeenCalledWith('area-1');
    expect(onCreateMapFromArea).toHaveBeenCalledWith(area);
  });

  it('hides saved map management controls when saved maps are disabled', () => {
    render(
      <TopologyHub
        devices={[mockDevice()]}
        areas={[mockArea()]}
        links={[mockLink()]}
        snapshot={null}
        maps={[mockMap()]}
        mapsLoading={false}
        mapsError="Map service unavailable"
        savedMapsEnabled={false}
        onOpenGlobal={vi.fn()}
        onOpenArea={vi.fn()}
        onOpenMap={vi.fn()}
        onCreateMapFromArea={vi.fn()}
        onDuplicateMap={vi.fn()}
        onDeleteMap={vi.fn()}
        onOpenSettings={vi.fn()}
      />,
    );

    expect(screen.queryByRole('button', { name: 'Create map from area Backbone' })).toBeNull();
    expect(screen.queryByText('Saved maps')).toBeNull();
    expect(screen.queryByText('Map service unavailable')).toBeNull();
  });

  it('shows saved map management controls when saved maps are enabled', () => {
    render(
      <TopologyHub
        devices={[mockDevice()]}
        areas={[mockArea()]}
        links={[mockLink()]}
        snapshot={null}
        maps={[mockMap()]}
        mapsLoading={false}
        mapsError={null}
        savedMapsEnabled={true}
        onOpenGlobal={vi.fn()}
        onOpenArea={vi.fn()}
        onOpenMap={vi.fn()}
        onCreateMapFromArea={vi.fn()}
        onDuplicateMap={vi.fn()}
        onDeleteMap={vi.fn()}
        onOpenSettings={vi.fn()}
      />,
    );

    expect(screen.getByRole('button', { name: 'Create map from area Backbone' })).toBeInTheDocument();
    expect(screen.getByText('Saved maps')).toBeInTheDocument();
  });
});
