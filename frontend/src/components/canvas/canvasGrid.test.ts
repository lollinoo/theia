/**
 * Exercises canvas grid snapping behavior so movement and persistence share one normalization contract.
 */
import type { NodeChange } from '@xyflow/react';
import { describe, expect, it } from 'vitest';

import type { DeviceNode } from '../DeviceCard';
import {
  canvasSnapGrid,
  snapNodeChangesToGrid,
  snapNodesToGrid,
  snapPositionToGrid,
} from './canvasGrid';

function node(id: string, x: number, y: number): DeviceNode {
  return {
    id,
    type: 'device',
    position: { x, y },
    data: {},
  } as DeviceNode;
}

describe('canvas grid', () => {
  it('uses a 30 by 30 canvas grid', () => {
    expect(canvasSnapGrid).toEqual([30, 30]);
  });

  it('snaps positive and negative coordinates to the nearest grid lines', () => {
    expect(snapPositionToGrid({ x: 44, y: -16 }, [30, 30])).toEqual({ x: 30, y: -30 });
  });

  it('supports independent custom grid steps', () => {
    expect(snapPositionToGrid({ x: 44, y: 46 }, [20, 25])).toEqual({ x: 40, y: 50 });
  });

  it('normalizes generated negative zero', () => {
    const snapped = snapPositionToGrid({ x: -1, y: -0 }, [30, 30]);

    expect(snapped).toEqual({ x: 0, y: 0 });
    expect(Object.is(snapped.x, -0)).toBe(false);
    expect(Object.is(snapped.y, -0)).toBe(false);
  });

  it('leaves coordinates unchanged when values or grid steps are not finite and positive', () => {
    const invalidCoordinates = { x: Number.POSITIVE_INFINITY, y: Number.NaN };
    const invalidGrid = { x: 44, y: 46 };

    expect(snapPositionToGrid(invalidCoordinates, [30, 30])).toBe(invalidCoordinates);
    expect(snapPositionToGrid(invalidGrid, [0, Number.POSITIVE_INFINITY])).toBe(invalidGrid);
    expect(snapPositionToGrid(invalidGrid, [-30, Number.NaN])).toBe(invalidGrid);
  });

  it('reuses an already aligned position reference', () => {
    const aligned = { x: 30, y: -60, metadata: 'preserved' };

    expect(snapPositionToGrid(aligned, [30, 30])).toBe(aligned);
  });

  it('snaps node positions while reusing aligned node references', () => {
    const aligned = node('aligned', 30, 60);
    const unaligned = node('unaligned', 44, 46);
    const nodes = [aligned, unaligned];

    const snapped = snapNodesToGrid(nodes, [30, 30]);

    expect(snapped).not.toBe(nodes);
    expect(snapped[0]).toBe(aligned);
    expect(snapped[1]).not.toBe(unaligned);
    expect(snapped[1]).toMatchObject({ position: { x: 30, y: 60 } });
  });

  it('reuses the node array when every position is aligned', () => {
    const nodes = [node('a', 30, 60), node('b', -90, 0)];

    expect(snapNodesToGrid(nodes, [30, 30])).toBe(nodes);
  });

  it('snaps position changes without modifying other change types', () => {
    const positionChange: NodeChange<DeviceNode> = {
      id: 'a',
      type: 'position',
      position: { x: 44, y: 46 },
    };
    const selectChange: NodeChange<DeviceNode> = {
      id: 'a',
      type: 'select',
      selected: true,
    };
    const changes = [positionChange, selectChange];

    const snapped = snapNodeChangesToGrid(changes, [30, 30]);

    expect(snapped).not.toBe(changes);
    expect(snapped[0]).not.toBe(positionChange);
    expect(snapped[0]).toMatchObject({ position: { x: 30, y: 60 } });
    expect(snapped[1]).toBe(selectChange);
  });

  it('reuses the change array when no position changes need snapping', () => {
    const changes: NodeChange<DeviceNode>[] = [
      {
        id: 'a',
        type: 'position',
        position: { x: 30, y: 60 },
      },
      {
        id: 'a',
        type: 'select',
        selected: true,
      },
    ];

    expect(snapNodeChangesToGrid(changes, [30, 30])).toBe(changes);
  });
});
