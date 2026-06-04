import { describe, expect, it } from 'vitest';

import type { PositionState } from '../../hooks/usePositions';
import type { Device } from '../../types/api';
import type { DeviceNode } from '../DeviceCard';
import {
  buildUsablePositionState,
  mergeNodePresentationState,
  nodePositionsToPositionMap,
  positionsChanged,
} from './topologyPositionState';

function device(id: string): Device {
  return {
    id,
    hostname: id,
    ip: '',
    device_type: 'router',
    poll_class: 'standard',
    poll_interval_override: null,
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
      status: 'up',
      pinned: false,
      onContextMenu: () => undefined,
    },
  } as DeviceNode;
}

describe('topology position state helpers', () => {
  it('builds a stable usable-position signature from current or saved positions', () => {
    const devices = [device('dev-b'), device('dev-a'), device('dev-c')];
    const current = new Map<string, PositionState>([['dev-b', { x: 1, y: 2, pinned: false }]]);
    const saved = new Map<string, PositionState>([['dev-a', { x: 3, y: 4, pinned: true }]]);

    expect(buildUsablePositionState(devices, current, saved)).toBe('dev-a|dev-b');
  });

  it('detects position payload changes against saved positions', () => {
    const saved = new Map<string, PositionState>([['dev-1', { x: 1, y: 2, pinned: false }]]);

    expect(
      positionsChanged([{ device_id: 'dev-1', x: 1, y: 2, pinned: false }], saved),
    ).toBe(false);
    expect(positionsChanged([{ device_id: 'dev-1', x: 1, y: 3, pinned: false }], saved)).toBe(
      true,
    );
  });

  it('converts node positions to saveable position state', () => {
    expect(nodePositionsToPositionMap([node('dev-1', 10, 20)])).toEqual(
      new Map([['dev-1', { x: 10, y: 20, pinned: false }]]),
    );
  });

  it('preserves transient node presentation state across topology composition', () => {
    const next = node('dev-1', 10, 20);
    const current = {
      ...node('dev-1', 5, 6),
      selected: true,
      dragging: true,
      width: 120,
      height: 80,
      data: { ...node('dev-1', 5, 6).data, highlighted: true },
    } as DeviceNode;

    expect(mergeNodePresentationState([next], [current])[0]).toMatchObject({
      selected: true,
      dragging: true,
      width: 120,
      height: 80,
      data: { highlighted: true },
    });
  });
});
