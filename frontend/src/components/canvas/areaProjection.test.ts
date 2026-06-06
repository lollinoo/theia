/**
 * Exercises area projection topology canvas behavior so refactors preserve the documented contract.
 */
import { describe, expect, it } from 'vitest';

import type { Device, Link } from '../../types/api';
import { projectAreaTopology } from './areaProjection';

function mockDevice(overrides: Partial<Device> = {}): Device {
  return {
    id: 'dev-1',
    hostname: 'router-01',
    ip: '10.0.0.1',
    device_type: 'router',
    poll_class: 'standard',
    poll_interval_override: null,
    polling_enabled: true,
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

function mockLink(overrides: Partial<Link> = {}): Link {
  return {
    id: 'link-1',
    source_device_id: 'a',
    target_device_id: 'b',
    source_if_name: 'ether1',
    target_if_name: 'ether1',
    discovery_protocol: 'lldp',
    source_if_speed: 1_000_000_000,
    source_if_oper_status: 'up',
    target_if_speed: 1_000_000_000,
    target_if_oper_status: 'up',
    ...overrides,
  };
}

const deviceA = mockDevice({ id: 'a', area_ids: ['area-1'] });
const deviceB = mockDevice({ id: 'b', area_ids: ['area-1'] });
const deviceC = mockDevice({ id: 'c', area_ids: ['area-2'] });
const deviceD = mockDevice({ id: 'd', area_ids: [] });
const linkAB = mockLink({ id: 'l1', source_device_id: 'a', target_device_id: 'b' });
const linkAC = mockLink({ id: 'l2', source_device_id: 'a', target_device_id: 'c' });
const linkCD = mockLink({ id: 'l3', source_device_id: 'c', target_device_id: 'd' });

describe('projectAreaTopology', () => {
  it('returns the full topology for global view', () => {
    const projection = projectAreaTopology({
      devices: [deviceA, deviceB, deviceC],
      links: [linkAB, linkAC],
      selectedAreaId: null,
    });

    expect(projection.filteredDevices.map((device) => device.id)).toEqual(['a', 'b', 'c']);
    expect(projection.filteredLinks.map((link) => link.id)).toEqual(['l1', 'l2']);
    expect(projection.ghostDevices).toEqual([]);
  });

  it('filters area devices, keeps cross-area links, and marks remote endpoints as ghosts', () => {
    const projection = projectAreaTopology({
      devices: [deviceA, deviceB, deviceC, deviceD],
      links: [linkAB, linkAC, linkCD],
      selectedAreaId: 'area-1',
    });

    expect(projection.filteredDevices.map((device) => device.id)).toEqual(['a', 'b']);
    expect(projection.filteredLinks.map((link) => link.id)).toEqual(['l1', 'l2']);
    expect(projection.ghostDevices.map((device) => device.id)).toEqual(['c']);
  });

  it('updates when area membership content changes with the same cardinality', () => {
    const before = projectAreaTopology({
      devices: [
        deviceA,
        mockDevice({ id: 'b', area_ids: ['area-1', 'area-2'] }),
        mockDevice({ id: 'c', area_ids: ['area-3', 'area-4'] }),
      ],
      links: [linkAB, linkAC],
      selectedAreaId: 'area-2',
    });
    const after = projectAreaTopology({
      devices: [
        deviceA,
        mockDevice({ id: 'b', area_ids: ['area-1', 'area-3'] }),
        mockDevice({ id: 'c', area_ids: ['area-2', 'area-4'] }),
      ],
      links: [linkAB, linkAC],
      selectedAreaId: 'area-2',
    });

    expect(before.filteredDevices.map((device) => device.id)).toEqual(['b']);
    expect(after.filteredDevices.map((device) => device.id)).toEqual(['c']);
  });
});
