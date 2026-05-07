import { describe, expect, it } from 'vitest';
import type { Area, CanvasMap, Device, Link } from '../../types/api';
import type { SnapshotPayload } from '../../types/metrics';
import { buildTopologyHubModel } from './topologyHubModel';

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
    device_count: 2,
    link_count: 1,
    position_count: 2,
    created_at: '',
    updated_at: '',
    ...overrides,
  };
}

describe('buildTopologyHubModel', () => {
  it('uses runtime snapshot status for aggregate and area health', () => {
    const areas = [mockArea()];
    const devices = [
      mockDevice({ id: 'device-1', hostname: 'router-1', area_ids: ['area-1'], status: 'up' }),
      mockDevice({ id: 'device-2', hostname: 'router-2', area_ids: [], status: 'up' }),
    ];
    const links = [mockLink()];
    const maps = [mockMap()];
    const snapshot = {
      devices: {
        'device-1': { status: 'down', alert_status: 'critical' },
      },
      links: {},
    } as unknown as SnapshotPayload;

    const model = buildTopologyHubModel({ areas, devices, links, snapshot, maps });

    expect(model.aggregate).toEqual({
      totalDevices: 2,
      activeLinks: 1,
      degradedDevices: 1,
      healthPercentage: 50,
    });
    expect(model.areas[0]).toMatchObject({
      area: areas[0],
      deviceCount: 1,
      activeLinkCount: 1,
      degradedDeviceCount: 1,
      healthPercentage: 0,
      healthLabel: 'Needs attention',
    });
    expect(model.maps).toBe(maps);
  });

  it('falls back to persisted device status when runtime snapshot has no device status', () => {
    const devices = [
      mockDevice({ id: 'device-1', hostname: 'router-1', status: 'down' }),
      mockDevice({ id: 'device-2', hostname: 'router-2', area_ids: ['area-1'], status: 'up' }),
    ];

    const model = buildTopologyHubModel({
      areas: [mockArea()],
      devices,
      links: [],
      snapshot: { devices: {}, links: {} } as SnapshotPayload,
      maps: [],
    });

    expect(model.aggregate.degradedDevices).toBe(1);
    expect(model.aggregate.healthPercentage).toBe(50);
    expect(model.areas[0].degradedDeviceCount).toBe(1);
    expect(model.areas[0].healthLabel).toBe('Needs attention');
  });

  it('counts unassigned devices', () => {
    const assigned = mockDevice({ id: 'device-1', area_ids: ['area-1'] });
    const unassigned = mockDevice({ id: 'device-2', hostname: 'router-2', area_ids: [] });
    const missingAreaIds = mockDevice({
      id: 'device-3',
      hostname: 'router-3',
      area_ids: undefined as unknown as string[],
    });

    const model = buildTopologyHubModel({
      areas: [mockArea()],
      devices: [assigned, unassigned, missingAreaIds],
      links: [],
      snapshot: null,
      maps: [],
    });

    expect(model.unassignedDevices.map((device) => device.id)).toEqual(['device-2', 'device-3']);
  });

  it('counts cross-area links where at least one endpoint is in an area', () => {
    const backbone = mockArea({ id: 'area-1', name: 'Backbone' });
    const edge = mockArea({ id: 'area-2', name: 'Edge' });
    const devices = [
      mockDevice({ id: 'device-1', area_ids: ['area-1'] }),
      mockDevice({ id: 'device-2', hostname: 'router-2', area_ids: ['area-2'] }),
      mockDevice({ id: 'device-3', hostname: 'router-3', area_ids: [] }),
    ];
    const links = [
      mockLink({ id: 'link-1', source_device_id: 'device-1', target_device_id: 'device-2' }),
      mockLink({ id: 'link-2', source_device_id: 'device-1', target_device_id: 'device-3' }),
      mockLink({ id: 'link-3', source_device_id: 'device-3', target_device_id: 'unknown' }),
    ];

    const model = buildTopologyHubModel({
      areas: [backbone, edge],
      devices,
      links,
      snapshot: null,
      maps: [],
    });

    expect(model.areas.map((areaModel) => [areaModel.area.id, areaModel.activeLinkCount])).toEqual([
      ['area-1', 2],
      ['area-2', 1],
    ]);
  });
});
