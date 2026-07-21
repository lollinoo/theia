/**
 * Exercises reusable automatic and waypoint-driven link geometry.
 */
import type { Rect, XYPosition } from '@xyflow/react';
import { describe, expect, it } from 'vitest';
import {
  buildEditableLinkPath,
  type EditableCubicSegment,
  type EditableEdgePathModel,
  nearestRouteInsertion,
} from './editableLinkGeometry';
import { resolveFloatingEndpoints } from './floatingEdgeGeometry';

function expectFinitePoint(point: XYPosition) {
  expect(Number.isFinite(point.x)).toBe(true);
  expect(Number.isFinite(point.y)).toBe(true);
}

function expectFiniteModel(model: EditableEdgePathModel) {
  expect(model.edgePath).not.toMatch(/NaN|Infinity/);
  expectFinitePoint(model.source);
  expectFinitePoint(model.sourceControl);
  expectFinitePoint(model.targetControl);
  expectFinitePoint(model.target);
  expectFinitePoint({ x: model.labelX, y: model.labelY });
  for (const segment of model.segments) {
    expectFinitePoint(segment.start);
    expectFinitePoint(segment.control1);
    expectFinitePoint(segment.control2);
    expectFinitePoint(segment.end);
  }
}

function expectOnRoundedBorder(point: XYPosition, rect: Rect, requestedRadius: number) {
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

function expectAimedToward(point: XYPosition, rect: Rect, toward: XYPosition) {
  const center = { x: rect.x + rect.width / 2, y: rect.y + rect.height / 2 };
  const anchorVector = { x: point.x - center.x, y: point.y - center.y };
  const towardVector = { x: toward.x - center.x, y: toward.y - center.y };
  const cross = anchorVector.x * towardVector.y - anchorVector.y * towardVector.x;
  const dot = anchorVector.x * towardVector.x + anchorVector.y * towardVector.y;

  expect(Math.abs(cross)).toBeLessThan(0.001);
  expect(dot).toBeGreaterThan(0);
}

function cubicPoint(segment: EditableCubicSegment, t: number): XYPosition {
  const inverse = 1 - t;
  return {
    x:
      inverse ** 3 * segment.start.x +
      3 * inverse ** 2 * t * segment.control1.x +
      3 * inverse * t ** 2 * segment.control2.x +
      t ** 3 * segment.end.x,
    y:
      inverse ** 3 * segment.start.y +
      3 * inverse ** 2 * t * segment.control1.y +
      3 * inverse * t ** 2 * segment.control2.y +
      t ** 3 * segment.end.y,
  };
}

function sampledHalfLengthPoint(model: EditableEdgePathModel): XYPosition {
  const points: XYPosition[] = [model.source];
  for (const segment of model.segments) {
    const lastPoint = points[points.length - 1]!;
    if (lastPoint.x !== segment.start.x || lastPoint.y !== segment.start.y) {
      points.push(segment.start);
    }
    for (let sample = 1; sample <= 100; sample += 1) {
      points.push(cubicPoint(segment, sample / 100));
    }
  }
  const finalSegmentPoint = points[points.length - 1]!;
  if (finalSegmentPoint.x !== model.target.x || finalSegmentPoint.y !== model.target.y) {
    points.push(model.target);
  }

  const lengths = points
    .slice(1)
    .map((point, index) => Math.hypot(point.x - points[index]!.x, point.y - points[index]!.y));
  const halfLength = lengths.reduce((sum, length) => sum + length, 0) / 2;
  let traversed = 0;
  for (let index = 0; index < lengths.length; index += 1) {
    const length = lengths[index]!;
    if (traversed + length >= halfLength) {
      const ratio = length === 0 ? 0 : (halfLength - traversed) / length;
      return {
        x: points[index]!.x + (points[index + 1]!.x - points[index]!.x) * ratio,
        y: points[index]!.y + (points[index + 1]!.y - points[index]!.y) * ratio,
      };
    }
    traversed += length;
  }
  return points[points.length - 1]!;
}

describe('buildEditableLinkPath', () => {
  it('builds a one-waypoint spline with terminal leads and ordered insertion segments', () => {
    const sourceRect = { x: 0, y: 0, width: 100, height: 60 };
    const targetRect = { x: 500, y: 0, width: 100, height: 60 };
    const waypoint = { x: 280, y: 180 };
    const route = { version: 1 as const, waypoints: [waypoint] };
    const model = buildEditableLinkPath({
      sourceRect,
      targetRect,
      fallbackSource: { x: 100, y: 30 },
      fallbackTarget: { x: 500, y: 30 },
      route,
      parallelIndex: 0,
    });

    expect(model.waypoints).toEqual([waypoint]);
    expect(model.waypoints).not.toBe(route.waypoints);
    expect(model.edgePath).toContain(' L ');
    expect(model.edgePath).toContain(' C ');
    expect(model.edgePath.match(/ C /g)).toHaveLength(2);
    expect(model.segments.map((segment) => segment.insertIndex)).toEqual([0, 1]);
    expect(model.segments.every((segment) => segment.insertIndex >= 0)).toBe(true);
    expect(model.segments[0]?.end).toEqual(waypoint);
    expect(model.segments[1]?.start).toEqual(waypoint);
    expectAimedToward(model.source, sourceRect, waypoint);
    expectAimedToward(model.target, targetRect, waypoint);
    expectOnRoundedBorder(model.source, sourceRect, 20);
    expectOnRoundedBorder(model.target, targetRect, 20);
    expectFiniteModel(model);
  });

  it('does not inject lateral curvature into a collinear manual route', () => {
    const model = buildEditableLinkPath({
      sourceRect: { x: 0, y: 0, width: 100, height: 60 },
      targetRect: { x: 500, y: 0, width: 100, height: 60 },
      fallbackSource: { x: 100, y: 30 },
      fallbackTarget: { x: 500, y: 30 },
      route: { version: 1, waypoints: [{ x: 300, y: 30 }] },
      parallelIndex: 0,
    });

    for (const segment of model.segments) {
      expect(segment.start.y).toBeCloseTo(30);
      expect(segment.control1.y).toBeCloseTo(30);
      expect(segment.control2.y).toBeCloseTo(30);
      expect(segment.end.y).toBeCloseTo(30);
    }
  });

  it('passes through three ordered waypoints and ignores automatic lane offsets', () => {
    const options = {
      sourceRect: { x: 0, y: 20, width: 100, height: 60 },
      targetRect: { x: 620, y: 80, width: 120, height: 70 },
      fallbackSource: { x: 100, y: 50 },
      fallbackTarget: { x: 620, y: 115 },
      route: {
        version: 1 as const,
        waypoints: [
          { x: 180, y: 190 },
          { x: 360, y: -40 },
          { x: 540, y: 220 },
        ],
      },
      sourceRadius: 20,
      targetRadius: 24,
    };
    const base = buildEditableLinkPath({ ...options, parallelIndex: 0, laneOrientation: 1 });
    const offset = buildEditableLinkPath({ ...options, parallelIndex: 5, laneOrientation: -1 });

    expect(base.waypoints).toEqual(options.route.waypoints);
    expect(base.segments.slice(0, -1).map((segment) => segment.end)).toEqual(
      options.route.waypoints,
    );
    expect(base.segments.map((segment) => segment.insertIndex)).toEqual([0, 1, 2, 3]);
    expect(offset.edgePath).toBe(base.edgePath);
    expect(offset.segments).toEqual(base.segments);
  });

  it('aligns endpoint leads and spline tangents with rounded-border normals', () => {
    const sourceRect = { x: 0, y: 0, width: 100, height: 80 };
    const targetRect = { x: 420, y: 180, width: 120, height: 90 };
    const waypoints = [
      { x: 180, y: 180 },
      { x: 350, y: 40 },
    ];
    const options = {
      sourceRect,
      targetRect,
      fallbackSource: { x: 100, y: 40 },
      fallbackTarget: { x: 420, y: 225 },
      route: { version: 1 as const, waypoints },
      parallelIndex: 0,
      sourceRadius: 20,
      targetRadius: 24,
    };
    const model = buildEditableLinkPath(options);
    const endpoints = resolveFloatingEndpoints({
      ...options,
      sourceToward: waypoints[0],
      targetToward: waypoints[waypoints.length - 1],
    });
    const first = model.segments[0]!;
    const last = model.segments[model.segments.length - 1]!;
    const sourceLead = {
      x: first.start.x - model.source.x,
      y: first.start.y - model.source.y,
    };
    const sourceTangent = {
      x: first.control1.x - first.start.x,
      y: first.control1.y - first.start.y,
    };
    const targetLead = {
      x: last.end.x - model.target.x,
      y: last.end.y - model.target.y,
    };
    const targetTangent = {
      x: last.control2.x - last.end.x,
      y: last.control2.y - last.end.y,
    };
    const cross = (left: XYPosition, right: XYPosition) => left.x * right.y - left.y * right.x;
    const dot = (left: XYPosition, right: XYPosition) => left.x * right.x + left.y * right.y;

    for (const vector of [sourceLead, sourceTangent]) {
      expect(Math.abs(cross(vector, endpoints.source.normal))).toBeLessThan(0.001);
      expect(dot(vector, endpoints.source.normal)).toBeGreaterThan(0);
    }
    for (const vector of [targetLead, targetTangent]) {
      expect(Math.abs(cross(vector, endpoints.target.normal))).toBeLessThan(0.001);
      expect(dot(vector, endpoints.target.normal)).toBeGreaterThan(0);
    }
  });

  it('respects physical, virtual, and ghost rounded-border radii', () => {
    const sourceRect = { x: 0, y: 0, width: 100, height: 100 };
    const targetRect = { x: 300, y: 220, width: 100, height: 100 };
    const route = { version: 1 as const, waypoints: [{ x: 200, y: 200 }] };
    const options = {
      sourceRect,
      targetRect,
      fallbackSource: { x: 100, y: 100 },
      fallbackTarget: { x: 300, y: 220 },
      route,
      parallelIndex: 0,
    };
    const physicalToVirtual = buildEditableLinkPath({
      ...options,
      sourceRadius: 20,
      targetRadius: 24,
    });
    const ghost = buildEditableLinkPath({ ...options, sourceRadius: 16, targetRadius: 16 });

    expectOnRoundedBorder(physicalToVirtual.source, sourceRect, 20);
    expectOnRoundedBorder(physicalToVirtual.target, targetRect, 24);
    expectOnRoundedBorder(ghost.source, sourceRect, 16);
    expectOnRoundedBorder(ghost.target, targetRect, 16);
    expect(ghost.source).not.toEqual(physicalToVirtual.source);
  });

  it('keeps a one-waypoint manual self-link on distinct finite border anchors', () => {
    const rect = { x: 0, y: 0, width: 100, height: 60 };
    const model = buildEditableLinkPath({
      sourceRect: rect,
      targetRect: rect,
      fallbackSource: { x: 100, y: 30 },
      fallbackTarget: { x: 0, y: 30 },
      route: { version: 1, waypoints: [{ x: 50, y: -120 }] },
      parallelIndex: 0,
    });

    expect(model.source).not.toEqual(model.target);
    expectOnRoundedBorder(model.source, rect, 20);
    expectOnRoundedBorder(model.target, rect, 20);
    expectFiniteModel(model);
  });

  it('remains finite for coincident fallbacks and repeated waypoints', () => {
    const model = buildEditableLinkPath({
      sourceRect: null,
      targetRect: null,
      fallbackSource: { x: 12, y: 34 },
      fallbackTarget: { x: 12, y: 34 },
      route: {
        version: 1,
        waypoints: [
          { x: 12, y: 34 },
          { x: 12, y: 34 },
          { x: 12 + Number.EPSILON, y: 34 },
        ],
      },
      parallelIndex: 3,
    });

    expect(model.segments).toHaveLength(4);
    expectFiniteModel(model);
  });

  it('places the label at half the sampled composite arc length', () => {
    const model = buildEditableLinkPath({
      sourceRect: { x: 0, y: 0, width: 100, height: 60 },
      targetRect: { x: 700, y: 240, width: 100, height: 60 },
      fallbackSource: { x: 100, y: 30 },
      fallbackTarget: { x: 700, y: 270 },
      route: {
        version: 1,
        waypoints: [
          { x: 140, y: 260 },
          { x: 590, y: 280 },
        ],
      },
      parallelIndex: 0,
    });
    const expected = sampledHalfLengthPoint(model);

    expect(model.labelX).toBeCloseTo(expected.x, 0);
    expect(model.labelY).toBeCloseTo(expected.y, 0);
  });
});

describe('nearestRouteInsertion', () => {
  it('refines the closest point and returns the segment insertion index', () => {
    const segments: EditableCubicSegment[] = [
      {
        start: { x: 0, y: 0 },
        control1: { x: 40, y: 0 },
        control2: { x: 80, y: 0 },
        end: { x: 120, y: 0 },
        insertIndex: 3,
      },
    ];

    const result = nearestRouteInsertion(segments, { x: 45, y: 20 });

    expect(result.insertIndex).toBe(3);
    expect(result.point.x).toBeCloseTo(45, 0);
    expect(result.point.y).toBeCloseTo(0, 8);
  });

  it('selects the nearest cubic from an ordered multi-segment route', () => {
    const straight = (
      start: XYPosition,
      end: XYPosition,
      insertIndex: number,
    ): EditableCubicSegment => ({
      start,
      control1: {
        x: start.x + (end.x - start.x) / 3,
        y: start.y + (end.y - start.y) / 3,
      },
      control2: {
        x: start.x + ((end.x - start.x) * 2) / 3,
        y: start.y + ((end.y - start.y) * 2) / 3,
      },
      end,
      insertIndex,
    });
    const segments = [
      straight({ x: 0, y: 0 }, { x: 100, y: 0 }, 0),
      straight({ x: 100, y: 0 }, { x: 100, y: 100 }, 1),
    ];

    const result = nearestRouteInsertion(segments, { x: 115, y: 72 });

    expect(result.insertIndex).toBe(1);
    expect(result.point.x).toBeCloseTo(100, 8);
    expect(result.point.y).toBeCloseTo(72, 0);
  });
});
