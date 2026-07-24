/**
 * Exercises node position update topology canvas behavior so refactors preserve the documented contract.
 */
import { describe, expect, it } from 'vitest';

import type { Device, Link } from '../../types/api';
import type { DeviceNode } from '../DeviceCard';
import type { LinkEdgeType } from '../LinkEdge';
import { buildManualNodePositionUpdate } from './nodePositionUpdate';

function device(id: string): Device {
  return {
    id,
    hostname: id,
    ip: '',
    device_type: 'router',
    poll_class: 'standard',
    poll_interval_override: null,
    polling_enabled: true,
    status: 'up',
    sys_name: id,
    sys_descr: '',
    hardware_model: '',
    vendor: '',
    managed: true,
    interfaces: [],
    area_ids: [],
    backup_supported: false,
    metrics_source: 'prometheus',
    prometheus_label_name: '',
    prometheus_label_value: '',
  };
}

function node(id: string, x: number, y: number): DeviceNode {
  return {
    id,
    type: 'device',
    position: { x, y },
    data: {
      device: device(id),
      runtime: {
        status: 'up',
        metrics: null,
        alertStatus: 'none',
        monitoringState: 'active',
      },
      pinned: false,
    },
  } as DeviceNode;
}

function link(id: string, source: string, target: string): Link {
  return {
    id,
    source_device_id: source,
    source_if_name: 'ether1',
    source_if_speed: 1000,
    source_if_oper_status: 'up',
    target_device_id: target,
    target_if_name: 'ether2',
    target_if_speed: 1000,
    target_if_oper_status: 'up',
    discovery_protocol: 'lldp',
  };
}

describe('node position update planning', () => {
  it('pins the moved node, rebuilds edges with existing data, and creates save payload', () => {
    const devices = [device('dev-1'), device('dev-2')];
    const links = [link('link-1', 'dev-1', 'dev-2')];
    const currentEdges = [
      {
        id: 'link-1',
        data: {
          metrics: { source: 'existing' },
        },
      } as unknown as LinkEdgeType,
    ];

    const plan = buildManualNodePositionUpdate({
      deviceId: 'dev-1',
      position: { x: 321, y: 654 },
      nodes: [node('dev-1', 10, 20), node('dev-2', 100, 200)],
      devices,
      links,
      openEdgeMenu: () => undefined,
      snapGrid: null,
    });
    const edges = plan?.buildEdges(currentEdges);

    expect(plan).not.toBeNull();
    expect(plan?.nodes[0]).toMatchObject({
      id: 'dev-1',
      position: { x: 321, y: 654 },
      data: { pinned: true },
    });
    expect(plan?.positionPayload).toEqual([
      { device_id: 'dev-1', x: 321, y: 654, pinned: true },
      { device_id: 'dev-2', x: 100, y: 200, pinned: false },
    ]);
    expect(plan?.positionMap).toEqual(
      new Map([
        ['dev-1', { x: 321, y: 654, pinned: true }],
        ['dev-2', { x: 100, y: 200, pinned: false }],
      ]),
    );
    expect(edges).toHaveLength(1);
    expect(edges?.[0].data?.metrics).toEqual({ source: 'existing' });
  });

  it('snaps only the moved node while preserving legacy positions when a grid is enabled', () => {
    const movedNode = node('dev-1', 10, 20);
    const legacyNode = node('dev-2', 100, 200);
    const plan = buildManualNodePositionUpdate({
      deviceId: 'dev-1',
      position: { x: 321, y: 654 },
      nodes: [movedNode, legacyNode],
      devices: [device('dev-1'), device('dev-2')],
      links: [],
      openEdgeMenu: () => undefined,
      snapGrid: [30, 30],
    });

    expect(plan?.nodes.map((current) => current.position)).toEqual([
      { x: 330, y: 660 },
      { x: 100, y: 200 },
    ]);
    expect(plan?.nodes[1]).toBe(legacyNode);
    expect(plan?.positionPayload).toEqual([
      { device_id: 'dev-1', x: 330, y: 660, pinned: true },
      { device_id: 'dev-2', x: 100, y: 200, pinned: false },
    ]);
    expect(plan?.positionMap).toEqual(
      new Map([
        ['dev-1', { x: 330, y: 660, pinned: true }],
        ['dev-2', { x: 100, y: 200, pinned: false }],
      ]),
    );
  });

  it('does not update ghost devices', () => {
    const ghost = {
      ...node('ghost-dev-1', 10, 20),
      data: {
        ...node('ghost-dev-1', 10, 20).data,
        kind: 'ghost-device',
        isGhost: true,
      },
    } as DeviceNode;

    expect(
      buildManualNodePositionUpdate({
        deviceId: 'ghost-dev-1',
        position: { x: 321, y: 654 },
        nodes: [ghost],
        devices: [device('ghost-dev-1')],
        links: [],
        openEdgeMenu: () => undefined,
        snapGrid: null,
      }),
    ).toBeNull();
  });
});
