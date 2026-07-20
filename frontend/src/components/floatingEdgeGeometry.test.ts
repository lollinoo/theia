/**
 * Exercises floating edge geometry so links remain anchored to live node borders.
 */
import type { InternalNode } from '@xyflow/react';
import { describe, expect, it } from 'vitest';
import type { DeviceNode } from './DeviceCard';
import {
  buildFloatingEdgePath,
  deviceNodeBorderRadius,
  type EdgePathModel,
  nodeRect,
} from './floatingEdgeGeometry';

function expectFiniteModel(model: EdgePathModel) {
  expect(model.edgePath).not.toMatch(/NaN|Infinity/);
  for (const point of [
    model.source,
    model.sourceControl,
    model.targetControl,
    model.target,
    { x: model.labelX, y: model.labelY },
  ]) {
    expect(Number.isFinite(point.x)).toBe(true);
    expect(Number.isFinite(point.y)).toBe(true);
  }
}

function expectOnRoundedBorder(
  point: { x: number; y: number },
  rect: { x: number; y: number; width: number; height: number },
  requestedRadius: number,
) {
  const radius = Math.min(requestedRadius, rect.width / 2, rect.height / 2);
  const nearestX = Math.max(rect.x + radius, Math.min(point.x, rect.x + rect.width - radius));
  const nearestY = Math.max(rect.y + radius, Math.min(point.y, rect.y + rect.height - radius));
  const distanceFromInnerRect = Math.hypot(point.x - nearestX, point.y - nearestY);

  expect(
    point.x === rect.x ||
      point.x === rect.x + rect.width ||
      point.y === rect.y ||
      point.y === rect.y + rect.height ||
      Math.abs(distanceFromInnerRect - radius) < 0.001,
  ).toBe(true);
}

function cubicMidpoint(model: EdgePathModel) {
  const t = 0.5;
  const inverse = 1 - t;
  return {
    x:
      inverse ** 3 * model.source.x +
      3 * inverse ** 2 * t * model.sourceControl.x +
      3 * inverse * t ** 2 * model.targetControl.x +
      t ** 3 * model.target.x,
    y:
      inverse ** 3 * model.source.y +
      3 * inverse ** 2 * t * model.sourceControl.y +
      3 * inverse * t ** 2 * model.targetControl.y +
      t ** 3 * model.target.y,
  };
}

describe('nodeRect', () => {
  it('resolves measured dimensions at the internal absolute position', () => {
    const internalNode = {
      measured: { width: 132, height: 58 },
      internals: { positionAbsolute: { x: 24, y: 36 } },
    } as InternalNode<DeviceNode>;

    expect(nodeRect(internalNode)).toEqual({
      x: 24,
      y: 36,
      width: 132,
      height: 58,
    });
  });

  it('rejects missing, zero-sized, and non-finite measurements', () => {
    expect(nodeRect(undefined)).toBeNull();
    expect(
      nodeRect({
        measured: { width: 0, height: 58 },
        internals: { positionAbsolute: { x: 24, y: 36 } },
      } as InternalNode<DeviceNode>),
    ).toBeNull();
    expect(
      nodeRect({
        measured: { width: 132, height: Number.NaN },
        internals: { positionAbsolute: { x: 24, y: 36 } },
      } as InternalNode<DeviceNode>),
    ).toBeNull();
  });
});

describe('deviceNodeBorderRadius', () => {
  it('matches physical, virtual, and ghost card corner geometry', () => {
    const internalNode = (data: Partial<DeviceNode['data']>) =>
      ({ data }) as InternalNode<DeviceNode>;

    expect(deviceNodeBorderRadius(internalNode({}))).toBe(20);
    expect(deviceNodeBorderRadius(internalNode({ isVirtual: true }))).toBe(24);
    expect(deviceNodeBorderRadius(internalNode({ kind: 'ghost-device', isGhost: true }))).toBe(16);
  });
});

describe('buildFloatingEdgePath', () => {
  it('anchors horizontally separated nodes on their right and left borders', () => {
    const sourceRect = { x: 10, y: 20, width: 100, height: 60 };
    const targetRect = { x: 310, y: 20, width: 100, height: 60 };
    const model = buildFloatingEdgePath({
      sourceRect,
      targetRect,
      fallbackSource: { x: 110, y: 50 },
      fallbackTarget: { x: 310, y: 50 },
      parallelIndex: 0,
      sourceRadius: 20,
      targetRadius: 20,
    });

    expect(model.source).toEqual({ x: 110, y: 50 });
    expect(model.target).toEqual({ x: 310, y: 50 });
    expect(model.sourceControl.x).toBeGreaterThan(model.source.x);
    expect(model.targetControl.x).toBeLessThan(model.target.x);
    expectOnRoundedBorder(model.source, sourceRect, 20);
    expectOnRoundedBorder(model.target, targetRect, 20);
  });

  it('anchors vertically separated nodes on their bottom and top borders', () => {
    const sourceRect = { x: 40, y: 10, width: 80, height: 60 };
    const targetRect = { x: 40, y: 210, width: 80, height: 60 };
    const model = buildFloatingEdgePath({
      sourceRect,
      targetRect,
      fallbackSource: { x: 80, y: 70 },
      fallbackTarget: { x: 80, y: 210 },
      parallelIndex: 0,
      sourceRadius: 20,
      targetRadius: 20,
    });

    expect(model.source).toEqual({ x: 80, y: 70 });
    expect(model.target).toEqual({ x: 80, y: 210 });
    expect(model.sourceControl.y).toBeGreaterThan(model.source.y);
    expect(model.targetControl.y).toBeLessThan(model.target.y);
    expectOnRoundedBorder(model.source, sourceRect, 20);
    expectOnRoundedBorder(model.target, targetRect, 20);
  });

  it('intersects diagonal center rays with rounded corners', () => {
    const sourceRect = { x: 0, y: 0, width: 100, height: 100 };
    const targetRect = { x: 200, y: 200, width: 100, height: 100 };
    const model = buildFloatingEdgePath({
      sourceRect,
      targetRect,
      fallbackSource: { x: 100, y: 100 },
      fallbackTarget: { x: 200, y: 200 },
      parallelIndex: 0,
      sourceRadius: 20,
      targetRadius: 24,
    });

    expect(model.source.x).toBeCloseTo(model.source.y, 5);
    expect(model.source.x).toBeCloseTo(80 + 20 / Math.sqrt(2), 5);
    expect(model.source.x).toBeLessThan(100);
    expect(model.target.x).toBeCloseTo(model.target.y, 5);
    expect(model.target.x).toBeCloseTo(224 - 24 / Math.sqrt(2), 5);
    expect(model.target.x).toBeLessThan(224);
    expectOnRoundedBorder(model.source, sourceRect, 20);
    expectOnRoundedBorder(model.target, targetRect, 24);
  });

  it('uses finite fallback geometry for missing or zero-sized rectangles', () => {
    const model = buildFloatingEdgePath({
      sourceRect: { x: 0, y: 0, width: 0, height: 60 },
      targetRect: null,
      fallbackSource: { x: 12, y: 34 },
      fallbackTarget: { x: 12, y: 34 },
      parallelIndex: 2,
    });

    expect(model.source).toEqual({ x: 12, y: 34 });
    expect(model.target).toEqual({ x: 12, y: 34 });
    expectFiniteModel(model);
  });

  it('keeps a visible bend between nearby aligned nodes', () => {
    const sourceRect = { x: 0, y: 0, width: 100, height: 60 };
    const targetRect = { x: 108, y: 0, width: 100, height: 60 };
    const model = buildFloatingEdgePath({
      sourceRect,
      targetRect,
      fallbackSource: { x: 100, y: 30 },
      fallbackTarget: { x: 108, y: 30 },
      parallelIndex: 0,
      sourceRadius: 20,
      targetRadius: 20,
    });

    expect(Math.abs(model.sourceControl.y - model.source.y)).toBeGreaterThanOrEqual(28);
    expect(Math.abs(model.targetControl.y - model.target.y)).toBeGreaterThanOrEqual(28);
    expectOnRoundedBorder(model.source, sourceRect, 20);
    expectOnRoundedBorder(model.target, targetRect, 20);
  });

  it('alternates and separates parallel cubic lanes', () => {
    const options = {
      sourceRect: { x: 0, y: 0, width: 100, height: 60 },
      targetRect: { x: 300, y: 0, width: 100, height: 60 },
      fallbackSource: { x: 100, y: 30 },
      fallbackTarget: { x: 300, y: 30 },
      sourceRadius: 20,
      targetRadius: 20,
    };
    const base = buildFloatingEdgePath({ ...options, parallelIndex: 0 });
    const second = buildFloatingEdgePath({ ...options, parallelIndex: 1 });
    const third = buildFloatingEdgePath({ ...options, parallelIndex: 2 });

    expect(new Set([base.edgePath, second.edgePath, third.edgePath])).toHaveLength(3);
    expect(Math.sign(base.sourceControl.y - base.source.y)).toBe(
      Math.sign(third.sourceControl.y - third.source.y),
    );
    expect(Math.sign(second.sourceControl.y - second.source.y)).toBe(
      -Math.sign(base.sourceControl.y - base.source.y),
    );
    expect(Math.abs(second.sourceControl.y - second.source.y)).toBeGreaterThan(
      Math.abs(base.sourceControl.y - base.source.y),
    );
    expect(Math.abs(third.sourceControl.y - third.source.y)).toBeGreaterThan(
      Math.abs(base.sourceControl.y - base.source.y),
    );
  });

  it('evaluates the cubic midpoint for its label anchor', () => {
    const model = buildFloatingEdgePath({
      sourceRect: { x: 0, y: 0, width: 100, height: 60 },
      targetRect: { x: 260, y: 120, width: 140, height: 80 },
      fallbackSource: { x: 100, y: 30 },
      fallbackTarget: { x: 260, y: 160 },
      parallelIndex: 1,
      sourceRadius: 20,
      targetRadius: 24,
    });
    const midpoint = cubicMidpoint(model);

    expect(model.labelX).toBeCloseTo(midpoint.x, 8);
    expect(model.labelY).toBeCloseTo(midpoint.y, 8);
    expectFiniteModel(model);
  });
});
