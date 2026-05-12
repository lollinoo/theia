import { describe, expect, it } from 'vitest';

import type { Device, Link } from '../../types/api';
import type { AlertDTO, AlertStatus, SnapshotPayload } from '../../types/metrics';
import type { DeviceNode } from '../DeviceCard';
import type { LinkEdgeType } from '../LinkEdge';
import {
  clearSelectedGraphItems,
  patchAlertStatuses,
  patchEditMode,
  patchHighlightedNode,
} from './canvasPresentationPatches';

function mockDevice(id: string): Device {
  return {
    id,
    hostname: id,
    ip: `10.0.0.${id.slice(-1)}`,
    device_type: 'router',
    poll_class: 'standard',
    poll_interval_override: null,
    status: 'up',
    sys_name: id,
    sys_descr: 'RouterOS',
    hardware_model: 'RB4011',
    vendor: 'mikrotik',
    managed: true,
    interfaces: [],
    area_ids: [],
    backup_supported: true,
    metrics_source: 'prometheus',
    prometheus_label_name: 'instance',
    prometheus_label_value: `10.0.0.${id.slice(-1)}:9100`,
  };
}

function mockNode(
  id: string,
  overrides: {
    alertStatus?: AlertStatus;
    editMode?: boolean;
    highlighted?: boolean;
    selected?: boolean;
  } = {},
): DeviceNode {
  return {
    id,
    type: 'device',
    position: { x: 0, y: 0 },
    selected: overrides.selected,
    data: {
      kind: 'device',
      device: mockDevice(id),
      runtime: {
        status: 'up',
        metrics: null,
        alertStatus: overrides.alertStatus ?? 'normal',
        monitoringState: 'monitorable',
      },
      pinned: false,
      editMode: overrides.editMode,
      highlighted: overrides.highlighted,
      isVirtual: false,
    },
  };
}

function mockLink(
  id: string,
  sourceDeviceId: string,
  targetDeviceId: string,
  sourceIfName: string,
  targetIfName: string,
): Link {
  return {
    id,
    source_device_id: sourceDeviceId,
    source_if_name: sourceIfName,
    source_if_mac: '',
    source_if_speed: 1_000_000_000,
    source_if_oper_status: 'up',
    target_device_id: targetDeviceId,
    target_if_name: targetIfName,
    target_if_mac: '',
    target_if_speed: 1_000_000_000,
    target_if_oper_status: 'up',
    discovery_protocol: 'lldp',
  };
}

function mockEdge(
  link: Link,
  overrides: { alertStatus?: AlertStatus; selected?: boolean } = {},
): LinkEdgeType {
  return {
    id: link.id,
    source: link.source_device_id,
    target: link.target_device_id,
    type: 'link',
    selected: overrides.selected,
    data: {
      link,
      alertStatus: overrides.alertStatus ?? 'normal',
    },
  };
}

describe('canvas presentation patches', () => {
  it('returns the same node array when every node already has the requested edit mode', () => {
    const nodes = [mockNode('dev-a', { editMode: true }), mockNode('dev-b', { editMode: true })];

    const result = patchEditMode(nodes, true);

    expect(result).toBe(nodes);
    expect(result[0]).toBe(nodes[0]);
    expect(result[1]).toBe(nodes[1]);
  });

  it('patches a highlighted node by id without changing other node references', () => {
    const nodes = [mockNode('dev-a'), mockNode('dev-b'), mockNode('dev-c')];
    const nodeIndexById = new Map(nodes.map((node, index) => [node.id, index]));

    const result = patchHighlightedNode(nodes, nodeIndexById, 'dev-b', true);

    expect(result).not.toBe(nodes);
    expect(result[0]).toBe(nodes[0]);
    expect(result[1]).not.toBe(nodes[1]);
    expect(result[1].data.highlighted).toBe(true);
    expect(result[2]).toBe(nodes[2]);
  });

  it('patches alert statuses without changing unaffected node or edge references', () => {
    const nodes = [mockNode('dev-a'), mockNode('dev-b'), mockNode('dev-c')];
    const edgeA = mockEdge(mockLink('link-a', 'dev-a', 'dev-b', 'ether1', 'ether2'));
    const edgeB = mockEdge(mockLink('link-b', 'dev-b', 'dev-c', 'ether3', 'ether4'));
    const edges = [edgeA, edgeB];
    const snapshot = {
      devices: {
        'dev-b': { alert_status: 'degraded' },
      },
      links: {},
    } as unknown as SnapshotPayload;
    const alerts: AlertDTO[] = [
      {
        device_id: 'dev-c',
        severity: 'critical',
        alert_name: 'LinkDown',
        state: 'firing',
        summary: 'ether4 is down',
      },
    ];

    const result = patchAlertStatuses(
      nodes,
      edges,
      {
        nodeIndexById: new Map(nodes.map((node, index) => [node.id, index])),
        edgeIndexById: new Map(edges.map((edge, index) => [edge.id, index])),
      },
      snapshot,
      alerts,
    );

    expect(result.nodes).not.toBe(nodes);
    expect(result.nodes[0]).toBe(nodes[0]);
    expect(result.nodes[1]).not.toBe(nodes[1]);
    expect(result.nodes[1].data.runtime.alertStatus).toBe('degraded');
    expect(result.nodes[2]).not.toBe(nodes[2]);
    expect(result.nodes[2].data.runtime.alertStatus).toBe('down');

    expect(result.edges).not.toBe(edges);
    expect(result.edges[0]).toBe(edges[0]);
    expect(result.edges[1]).not.toBe(edges[1]);
    expect(result.edges[1].data?.alertStatus).toBe('down');
  });

  it('preserves node and edge arrays when clearing selection with nothing selected', () => {
    const nodes = [mockNode('dev-a'), mockNode('dev-b')];
    const edges = [mockEdge(mockLink('link-a', 'dev-a', 'dev-b', 'ether1', 'ether2'))];

    const result = clearSelectedGraphItems(nodes, edges, {
      nodeIndexById: new Map(nodes.map((node, index) => [node.id, index])),
      edgeIndexById: new Map(edges.map((edge, index) => [edge.id, index])),
    });

    expect(result.nodes).toBe(nodes);
    expect(result.edges).toBe(edges);
    expect(result.nodes[0]).toBe(nodes[0]);
    expect(result.nodes[1]).toBe(nodes[1]);
    expect(result.edges[0]).toBe(edges[0]);
  });
});
