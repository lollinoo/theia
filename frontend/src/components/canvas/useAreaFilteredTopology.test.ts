import { renderHook } from '@testing-library/react';
import { describe, expect, it } from 'vitest';
import type { Device, Link } from '../../types/api';
import { useAreaFilteredTopology } from './useAreaFilteredTopology';

function mockDevice(overrides: Partial<Device> = {}): Device {
  return {
    id: 'dev-1',
    hostname: 'router-01',
    ip: '10.0.0.1',
    device_type: 'router',
    status: 'up',
    sys_name: 'router-01',
    sys_descr: 'RouterOS',
    hardware_model: 'RB4011',
    vendor: 'mikrotik',
    managed: true,
    interfaces: [],
    backup_supported: false,
    metrics_source: 'prometheus',
    prometheus_label_name: 'instance',
    prometheus_label_value: '10.0.0.1:9100',
    area_ids: [],
    ...overrides,
  };
}

const deviceA = mockDevice({ id: 'a', area_ids: ['area-1'], sys_name: 'Router-A', ip: '10.0.0.1' });
const deviceB = mockDevice({ id: 'b', area_ids: ['area-1'], sys_name: 'Router-B', ip: '10.0.0.2' });
const deviceC = mockDevice({ id: 'c', area_ids: ['area-2'], sys_name: 'Router-C', ip: '10.0.0.3' });
const deviceD = mockDevice({ id: 'd', area_ids: [], sys_name: 'Unassigned', ip: '10.0.0.4' }); // no areas

const linkAB: Link = {
  id: 'l1',
  source_device_id: 'a',
  target_device_id: 'b',
  source_if_name: 'ether1',
  target_if_name: 'ether1',
  discovery_protocol: 'lldp',
  source_if_speed: 0,
  source_if_oper_status: '',
  target_if_speed: 0,
  target_if_oper_status: '',
};
const linkAC: Link = {
  id: 'l2',
  source_device_id: 'a',
  target_device_id: 'c',
  source_if_name: 'ether2',
  target_if_name: 'ether1',
  discovery_protocol: 'lldp',
  source_if_speed: 0,
  source_if_oper_status: '',
  target_if_speed: 0,
  target_if_oper_status: '',
};
const linkCD: Link = {
  id: 'l3',
  source_device_id: 'c',
  target_device_id: 'd',
  source_if_name: 'ether3',
  target_if_name: 'ether1',
  discovery_protocol: 'lldp',
  source_if_speed: 0,
  source_if_oper_status: '',
  target_if_speed: 0,
  target_if_oper_status: '',
};

const allDevices = [deviceA, deviceB, deviceC, deviceD];
const allLinks = [linkAB, linkAC, linkCD];

describe('useAreaFilteredTopology', () => {
  it('returns all devices and links when selectedAreaId is null (Global view)', () => {
    const { result } = renderHook(() => useAreaFilteredTopology(allDevices, allLinks, null));

    expect(result.current.filteredDevices).toEqual(allDevices);
    expect(result.current.filteredLinks).toEqual(allLinks);
    expect(result.current.ghostDevices).toEqual([]);
  });

  it('returns only area devices when selectedAreaId is set', () => {
    const { result } = renderHook(() => useAreaFilteredTopology(allDevices, allLinks, 'area-1'));

    expect(result.current.filteredDevices.map((d) => d.id)).toEqual(['a', 'b']);
  });

  it('returns links where at least one endpoint is in the area', () => {
    const { result } = renderHook(() => useAreaFilteredTopology(allDevices, allLinks, 'area-1'));

    // linkAB: both in area-1 -> included
    // linkAC: a in area-1, c in area-2 -> included (cross-area)
    // linkCD: neither in area-1 -> excluded
    expect(result.current.filteredLinks.map((l) => l.id)).toEqual(['l1', 'l2']);
  });

  it('identifies ghost devices for cross-area links', () => {
    const { result } = renderHook(() => useAreaFilteredTopology(allDevices, allLinks, 'area-1'));

    // linkAC crosses area-1 to area-2, so deviceC is a ghost
    expect(result.current.ghostDevices.map((d) => d.id)).toEqual(['c']);
  });

  it('returns empty ghostDevices when all link endpoints are in the area', () => {
    // Only use devices and links within area-1
    const { result } = renderHook(() =>
      useAreaFilteredTopology([deviceA, deviceB], [linkAB], 'area-1'),
    );

    expect(result.current.ghostDevices).toEqual([]);
  });

  it('excludes unassigned devices when selectedAreaId is set (per D-14)', () => {
    const { result } = renderHook(() => useAreaFilteredTopology(allDevices, allLinks, 'area-1'));

    // deviceD has no area_id -> should not be in filteredDevices
    const ids = result.current.filteredDevices.map((d) => d.id);
    expect(ids).not.toContain('d');
  });
});
