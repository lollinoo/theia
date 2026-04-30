import { describe, expect, it, vi } from 'vitest';

import type { AutoLayoutEdge, AutoLayoutNode } from '../../hooks/useAutoLayout';
import type { Device, Link } from '../../types/api';
import {
  buildIncrementalLayoutInputs,
  computeIncrementalLayoutPositions,
} from './incrementalLayout';

function mockDevice(id: string): Device {
  return {
    id,
    hostname: id,
    ip: `10.0.0.${id.replace(/\D/g, '') || '1'}`,
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
    prometheus_label_value: `10.0.0.${id.replace(/\D/g, '') || '1'}:9100`,
  };
}

function mockLink(id: string, source: string, target: string): Link {
  return {
    id,
    source_device_id: source,
    source_if_name: 'ether1',
    target_device_id: target,
    target_if_name: 'ether1',
    source_if_index: null,
    target_if_index: null,
    source_if_speed: null,
    target_if_speed: null,
    discovery_protocol: 'manual',
  };
}

describe('incrementalLayout', () => {
  it('builds a local layout subgraph with positioned neighbors pinned as anchors', () => {
    const devices = ['anchor-1', 'new-1', 'new-2', 'unrelated-1'].map(mockDevice);
    const links = [
      mockLink('link-1', 'anchor-1', 'new-1'),
      mockLink('link-2', 'new-1', 'new-2'),
      mockLink('link-3', 'anchor-1', 'unrelated-1'),
      mockLink('self-1', 'new-1', 'new-1'),
    ];
    const effectivePositions = new Map([
      ['anchor-1', { x: 100, y: 120, pinned: true }],
      ['unrelated-1', { x: 700, y: 720, pinned: true }],
    ]);

    const result = buildIncrementalLayoutInputs({
      devices,
      links,
      placementDeviceIds: new Set(['new-1', 'new-2']),
      effectivePositions,
    });

    expect(result.layoutNodes).toEqual([
      { id: 'anchor-1', x: 100, y: 120, pinned: true },
      { id: 'new-1', x: undefined, y: undefined, pinned: false },
      { id: 'new-2', x: undefined, y: undefined, pinned: false },
    ]);
    expect(result.layoutEdges).toEqual([
      { source: 'anchor-1', target: 'new-1' },
      { source: 'new-1', target: 'new-2' },
    ]);
    expect([...result.impactedDeviceIds].sort()).toEqual(['anchor-1', 'new-1', 'new-2']);
  });

  it('does not schedule layout work when no device needs placement', () => {
    const result = buildIncrementalLayoutInputs({
      devices: [mockDevice('dev-1')],
      links: [],
      placementDeviceIds: new Set(),
      effectivePositions: new Map([['dev-1', { x: 160, y: 180, pinned: true }]]),
    });

    expect(result.layoutNodes).toEqual([]);
    expect(result.layoutEdges).toEqual([]);
    expect(result.impactedDeviceIds.size).toBe(0);
  });

  it('persists computed positions only for devices that need placement', () => {
    const layoutEngine = vi.fn(
      (_nodes: AutoLayoutNode[], _edges: AutoLayoutEdge[], _width: number, _height: number) =>
        new Map([
          ['anchor-1', { x: 999, y: 999 }],
          ['new-1', { x: 240, y: 260 }],
        ]),
    );

    const result = computeIncrementalLayoutPositions({
      layoutNodes: [
        { id: 'anchor-1', x: 100, y: 120, pinned: true },
        { id: 'new-1', pinned: false },
      ],
      layoutEdges: [{ source: 'anchor-1', target: 'new-1' }],
      placementDeviceIds: new Set(['new-1']),
      width: 1200,
      height: 800,
      layoutEngine,
    });

    expect(layoutEngine).toHaveBeenCalledTimes(1);
    expect(result).toEqual(new Map([['new-1', { x: 240, y: 260 }]]));
  });

  it('skips the force engine for an empty incremental plan', () => {
    const layoutEngine = vi.fn(() => new Map<string, { x: number; y: number }>());

    const result = computeIncrementalLayoutPositions({
      layoutNodes: [],
      layoutEdges: [],
      placementDeviceIds: new Set(['new-1']),
      width: 1200,
      height: 800,
      layoutEngine,
    });

    expect(layoutEngine).not.toHaveBeenCalled();
    expect(result.size).toBe(0);
  });
});
