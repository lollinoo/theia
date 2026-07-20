/**
 * Centralizes canvas grid snapping so interaction, composition, and persistence use identical rules.
 */
import type { NodeChange, SnapGrid, XYPosition } from '@xyflow/react';

import type { DeviceNode } from '../DeviceCard';

/** Grid used by the topology canvas for optional snapped movement. */
export const canvasSnapGrid: SnapGrid = [30, 30];

function decimalPlaces(value: number): number {
  const [coefficient, exponentText] = value.toString().toLowerCase().split('e');
  const fractionLength = coefficient.split('.')[1]?.length ?? 0;
  const exponent = Number(exponentText ?? 0);
  return Math.max(0, fractionLength - exponent);
}

function stabilizeDecimalPlaces(value: number, places: number): number {
  const [coefficient, exponentText] = value.toString().toLowerCase().split('e');
  const exponent = Number(exponentText ?? 0);
  const shifted = Number(`${coefficient}e${exponent + places}`);
  if (!Number.isFinite(shifted)) {
    return value;
  }

  const stabilized = Number(`${Math.round(shifted)}e-${places}`);
  return Number.isFinite(stabilized) ? stabilized : value;
}

function normalizedCoordinate(coordinate: number, step: number): number {
  if (!Number.isFinite(coordinate) || !Number.isFinite(step) || step <= 0) {
    return coordinate;
  }

  const stepCount = Math.round(coordinate / step);
  if (!Number.isFinite(stepCount)) {
    return coordinate;
  }

  const snapped = stabilizeDecimalPlaces(stepCount * step, decimalPlaces(step));
  return Object.is(snapped, -0) ? 0 : snapped;
}

/** Returns a grid-aligned position while retaining the input reference when no value changes. */
export function snapPositionToGrid<T extends XYPosition>(position: T, grid: SnapGrid): T {
  const x = normalizedCoordinate(position.x, grid[0]);
  const y = normalizedCoordinate(position.y, grid[1]);
  return Object.is(x, position.x) && Object.is(y, position.y) ? position : { ...position, x, y };
}

/** Returns nodes with grid-aligned positions and preserves structural sharing for aligned values. */
export function snapNodesToGrid(nodes: DeviceNode[], grid: SnapGrid): DeviceNode[] {
  let changed = false;
  const snappedNodes = nodes.map((node) => {
    const position = snapPositionToGrid(node.position, grid);
    if (position === node.position) {
      return node;
    }

    changed = true;
    return { ...node, position };
  });

  return changed ? snappedNodes : nodes;
}

/** Snaps position changes while leaving every other controlled React Flow change untouched. */
export function snapNodeChangesToGrid(
  changes: NodeChange<DeviceNode>[],
  grid: SnapGrid,
): NodeChange<DeviceNode>[] {
  let changed = false;
  const snappedChanges = changes.map((change) => {
    if (change.type !== 'position') {
      return change;
    }

    const position = change.position ? snapPositionToGrid(change.position, grid) : change.position;
    const positionAbsolute = change.positionAbsolute
      ? snapPositionToGrid(change.positionAbsolute, grid)
      : change.positionAbsolute;
    if (position === change.position && positionAbsolute === change.positionAbsolute) {
      return change;
    }

    changed = true;
    return { ...change, position, positionAbsolute };
  });

  return changed ? snappedChanges : changes;
}
