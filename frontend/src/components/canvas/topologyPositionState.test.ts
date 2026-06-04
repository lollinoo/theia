import { describe, expect, it } from 'vitest';

import type { PositionState } from '../../hooks/usePositions';
import type { Device } from '../../types/api';
import type { DeviceNode } from '../DeviceCard';
import {
  buildTopologyCompositionPositionPlan,
  buildTopologyPositionSavePlan,
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

function ghostNode(id: string, x: number, y: number): DeviceNode {
  return {
    ...node(id, x, y),
    data: {
      ...node(id, x, y).data,
      kind: 'ghost-device',
      isGhost: true,
    },
  } as DeviceNode;
}

describe('topology position state helpers', () => {
  it('builds effective positions from saved positions plus unsaved current holes', () => {
    const saved = new Map<string, PositionState>([
      ['dev-1', { x: 10, y: 20, pinned: true }],
      ['dev-2', { x: 30, y: 40, pinned: false }],
    ]);
    const current = new Map<string, PositionState>([
      ['dev-1', { x: 100, y: 200, pinned: false }],
      ['dev-3', { x: 50, y: 60, pinned: false }],
    ]);

    const plan = buildTopologyCompositionPositionPlan({
      trigger: 'manual_refresh',
      savedPositions: saved,
      currentNodePositions: current,
    });

    expect(plan.effectivePositions).toEqual(
      new Map<string, PositionState>([
        ['dev-1', { x: 10, y: 20, pinned: true }],
        ['dev-2', { x: 30, y: 40, pinned: false }],
        ['dev-3', { x: 50, y: 60, pinned: false }],
      ]),
    );
    expect(plan.currentPositionsForComposition).toBe(current);
  });

  it('drops current composition positions after backend reconnect', () => {
    const saved = new Map<string, PositionState>([['dev-1', { x: 10, y: 20, pinned: true }]]);
    const current = new Map<string, PositionState>([
      ['dev-1', { x: 100, y: 200, pinned: false }],
      ['dev-2', { x: 50, y: 60, pinned: false }],
    ]);

    const plan = buildTopologyCompositionPositionPlan({
      trigger: 'backend_reconnected',
      savedPositions: saved,
      currentNodePositions: current,
    });

    expect(plan.effectivePositions).toEqual(
      new Map<string, PositionState>([
        ['dev-1', { x: 10, y: 20, pinned: true }],
        ['dev-2', { x: 50, y: 60, pinned: false }],
      ]),
    );
    expect(plan.currentPositionsForComposition).toEqual(new Map());
  });

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

  it('builds a position save plan that prunes ghost nodes and detects changes', () => {
    const saved = new Map<string, PositionState>([['dev-1', { x: 1, y: 2, pinned: false }]]);

    expect(
      buildTopologyPositionSavePlan(
        [node('dev-1', 10, 20), ghostNode('ghost-dev-1', 30, 40)],
        saved,
      ),
    ).toEqual({
      shouldSave: true,
      payload: [{ device_id: 'dev-1', x: 10, y: 20, pinned: false }],
    });
  });

  it('builds a position save plan that skips unchanged positions', () => {
    const saved = new Map<string, PositionState>([['dev-1', { x: 10, y: 20, pinned: false }]]);

    expect(buildTopologyPositionSavePlan([node('dev-1', 10, 20)], saved)).toEqual({
      shouldSave: false,
      payload: [{ device_id: 'dev-1', x: 10, y: 20, pinned: false }],
    });
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
