/**
 * Builds reusable waypoint-driven link paths with straight terminal leads.
 */
import type { Rect, XYPosition } from '@xyflow/react';
import type { LinkRoute } from '../types/api';
import {
  type EdgePathModel,
  type FloatingEndpoint,
  resolveFloatingEndpoints,
} from './floatingEdgeGeometry';

const EPSILON = 0.000001;
const MAX_TERMINAL_LEAD = 24;
const TERMINAL_LEAD_RATIO = 0.12;
const ROUTE_SAMPLE_INTERVALS = 12;
const INSERTION_REFINEMENT_STEPS = 4;
const SELF_LINK_ANCHOR_SPREAD = Math.PI / 12;

/** One ordered cubic span and the waypoint-array index inserted on that span. */
export interface EditableCubicSegment {
  start: XYPosition;
  control1: XYPosition;
  control2: XYPosition;
  end: XYPosition;
  insertIndex: number;
}

/** Complete render and interaction geometry for one manual link route. */
export interface EditableEdgePathModel extends EdgePathModel {
  segments: EditableCubicSegment[];
  waypoints: XYPosition[];
}

interface EditableLinkPathOptions {
  sourceRect: Rect | null;
  targetRect: Rect | null;
  fallbackSource: XYPosition;
  fallbackTarget: XYPosition;
  route: LinkRoute;
  parallelIndex: number;
  laneOrientation?: 1 | -1;
  sourceRadius?: number;
  targetRadius?: number;
}

function finitePoint(point: XYPosition): XYPosition {
  return {
    x: Number.isFinite(point.x) ? point.x : 0,
    y: Number.isFinite(point.y) ? point.y : 0,
  };
}

function distance(left: XYPosition, right: XYPosition): number {
  return Math.hypot(right.x - left.x, right.y - left.y);
}

function pointsCoincide(left: XYPosition, right: XYPosition): boolean {
  return distance(left, right) < EPSILON;
}

function matchingRect(left: Rect | null, right: Rect | null): Rect | null {
  return left !== null &&
    right !== null &&
    left.x === right.x &&
    left.y === right.y &&
    left.width === right.width &&
    left.height === right.height
    ? left
    : null;
}

function rectCenter(rect: Rect): XYPosition {
  return { x: rect.x + rect.width / 2, y: rect.y + rect.height / 2 };
}

function rotatedToward(
  rect: Rect,
  toward: XYPosition,
  fallbackToward: XYPosition,
  radians: number,
): XYPosition {
  const center = rectCenter(rect);
  let vector = { x: toward.x - center.x, y: toward.y - center.y };
  if (Math.hypot(vector.x, vector.y) < EPSILON) {
    vector = { x: fallbackToward.x - center.x, y: fallbackToward.y - center.y };
  }
  if (Math.hypot(vector.x, vector.y) < EPSILON) {
    vector = { x: 1, y: 0 };
  }
  const cosine = Math.cos(radians);
  const sine = Math.sin(radians);
  return {
    x: center.x + vector.x * cosine - vector.y * sine,
    y: center.y + vector.x * sine + vector.y * cosine,
  };
}

function manualEndpoints(
  options: EditableLinkPathOptions,
  waypoints: readonly XYPosition[],
): { source: FloatingEndpoint; target: FloatingEndpoint } {
  const firstWaypoint = waypoints[0]!;
  const lastWaypoint = waypoints[waypoints.length - 1]!;
  let endpoints = resolveFloatingEndpoints({
    ...options,
    sourceToward: firstWaypoint,
    targetToward: lastWaypoint,
  });

  // A one-direction self-link needs two nearby anchors rather than painting both terminals together.
  const sharedNodeRect = matchingRect(options.sourceRect, options.targetRect);
  if (sharedNodeRect && pointsCoincide(endpoints.source.point, endpoints.target.point)) {
    endpoints = resolveFloatingEndpoints({
      ...options,
      sourceToward: rotatedToward(
        sharedNodeRect,
        firstWaypoint,
        options.fallbackSource,
        -SELF_LINK_ANCHOR_SPREAD,
      ),
      targetToward: rotatedToward(
        sharedNodeRect,
        lastWaypoint,
        options.fallbackTarget,
        SELF_LINK_ANCHOR_SPREAD,
      ),
    });
  }

  return endpoints;
}

function terminalLead(endpoint: FloatingEndpoint, toward: XYPosition): XYPosition {
  const length = Math.min(
    MAX_TERMINAL_LEAD,
    distance(endpoint.point, toward) * TERMINAL_LEAD_RATIO,
  );
  return finitePoint({
    x: endpoint.point.x + endpoint.normal.x * length,
    y: endpoint.point.y + endpoint.normal.y * length,
  });
}

function parameterize(points: readonly XYPosition[]): number[] {
  const parameters = [0];
  for (let index = 1; index < points.length; index += 1) {
    const chord = distance(points[index - 1]!, points[index]!);
    parameters.push(parameters[index - 1]! + Math.max(EPSILON, Math.sqrt(chord)));
  }
  return parameters;
}

function endpointTangent(
  direction: XYPosition,
  left: XYPosition,
  right: XYPosition,
  parameterSpan: number,
): XYPosition {
  const magnitude = distance(left, right) / Math.max(parameterSpan, EPSILON);
  return finitePoint({ x: direction.x * magnitude, y: direction.y * magnitude });
}

function internalTangent(
  points: readonly XYPosition[],
  parameters: readonly number[],
  index: number,
): XYPosition {
  const previousSpan = Math.max(parameters[index]! - parameters[index - 1]!, EPSILON);
  const nextSpan = Math.max(parameters[index + 1]! - parameters[index]!, EPSILON);
  const spanSum = previousSpan + nextSpan;
  const previous = points[index - 1]!;
  const current = points[index]!;
  const next = points[index + 1]!;
  const previousSlope = {
    x: (current.x - previous.x) / previousSpan,
    y: (current.y - previous.y) / previousSpan,
  };
  const nextSlope = {
    x: (next.x - current.x) / nextSpan,
    y: (next.y - current.y) / nextSpan,
  };

  return finitePoint({
    x: (previousSlope.x * nextSpan + nextSlope.x * previousSpan) / spanSum,
    y: (previousSlope.y * nextSpan + nextSlope.y * previousSpan) / spanSum,
  });
}

function buildSplineSegments(
  points: readonly XYPosition[],
  sourceNormal: XYPosition,
  targetNormal: XYPosition,
): EditableCubicSegment[] {
  const parameters = parameterize(points);
  const tangents = points.map((point, index) => {
    if (index === 0) {
      return endpointTangent(sourceNormal, point, points[1]!, parameters[1]! - parameters[0]!);
    }
    if (index === points.length - 1) {
      return endpointTangent(
        { x: -targetNormal.x, y: -targetNormal.y },
        points[index - 1]!,
        point,
        parameters[index]! - parameters[index - 1]!,
      );
    }
    return internalTangent(points, parameters, index);
  });

  return points.slice(0, -1).map((start, index) => {
    const end = points[index + 1]!;
    const span = Math.max(parameters[index + 1]! - parameters[index]!, EPSILON);
    const tangentScale = span / 3;
    return {
      start,
      control1: finitePoint({
        x: start.x + tangents[index]!.x * tangentScale,
        y: start.y + tangents[index]!.y * tangentScale,
      }),
      control2: finitePoint({
        x: end.x - tangents[index + 1]!.x * tangentScale,
        y: end.y - tangents[index + 1]!.y * tangentScale,
      }),
      end,
      insertIndex: index,
    };
  });
}

function cubicPoint(segment: EditableCubicSegment, t: number): XYPosition {
  const inverse = 1 - t;
  return finitePoint({
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
  });
}

function closestPointOnLine(start: XYPosition, end: XYPosition, point: XYPosition) {
  const vector = { x: end.x - start.x, y: end.y - start.y };
  const lengthSquared = vector.x ** 2 + vector.y ** 2;
  const ratio =
    lengthSquared < EPSILON ** 2
      ? 0
      : Math.min(
          1,
          Math.max(
            0,
            ((point.x - start.x) * vector.x + (point.y - start.y) * vector.y) / lengthSquared,
          ),
        );
  const closest = {
    x: start.x + vector.x * ratio,
    y: start.y + vector.y * ratio,
  };
  return {
    point: closest,
    ratio,
    distanceSquared: (closest.x - point.x) ** 2 + (closest.y - point.y) ** 2,
  };
}

function sampledHalfLengthPoint(
  source: XYPosition,
  segments: readonly EditableCubicSegment[],
  target: XYPosition,
): XYPosition {
  const points = [source];
  for (const segment of segments) {
    if (!pointsCoincide(points[points.length - 1]!, segment.start)) {
      points.push(segment.start);
    }
    for (let sample = 1; sample <= ROUTE_SAMPLE_INTERVALS; sample += 1) {
      points.push(cubicPoint(segment, sample / ROUTE_SAMPLE_INTERVALS));
    }
  }
  if (!pointsCoincide(points[points.length - 1]!, target)) {
    points.push(target);
  }

  const lengths = points.slice(1).map((point, index) => distance(points[index]!, point));
  const halfLength = lengths.reduce((total, length) => total + length, 0) / 2;
  let traversed = 0;
  for (let index = 0; index < lengths.length; index += 1) {
    const length = lengths[index]!;
    if (traversed + length >= halfLength) {
      const ratio = length < EPSILON ? 0 : (halfLength - traversed) / length;
      const start = points[index]!;
      const end = points[index + 1]!;
      return finitePoint({
        x: start.x + (end.x - start.x) * ratio,
        y: start.y + (end.y - start.y) * ratio,
      });
    }
    traversed += length;
  }
  return finitePoint(points[points.length - 1] ?? source);
}

/** Builds one complete waypoint-driven edge path model. */
export function buildEditableLinkPath(options: EditableLinkPathOptions): EditableEdgePathModel {
  const waypoints = options.route.waypoints.map(finitePoint);
  const endpoints = manualEndpoints(options, waypoints);
  const source = endpoints.source.point;
  const target = endpoints.target.point;
  const sourceLead = terminalLead(endpoints.source, waypoints[0]!);
  const targetLead = terminalLead(endpoints.target, waypoints[waypoints.length - 1]!);
  const segments = buildSplineSegments(
    [sourceLead, ...waypoints, targetLead],
    endpoints.source.normal,
    endpoints.target.normal,
  );
  const sourceControl = segments[0]!.control1;
  const targetControl = segments[segments.length - 1]!.control2;
  const label = sampledHalfLengthPoint(source, segments, target);
  const cubicCommands = segments
    .map(
      (segment) =>
        `C ${segment.control1.x},${segment.control1.y} ${segment.control2.x},${segment.control2.y} ${segment.end.x},${segment.end.y}`,
    )
    .join(' ');

  return {
    edgePath: `M ${source.x},${source.y} L ${sourceLead.x},${sourceLead.y} ${cubicCommands} L ${target.x},${target.y}`,
    labelX: label.x,
    labelY: label.y,
    source,
    sourceLead,
    sourceControl,
    targetControl,
    targetLead,
    target,
    segments,
    waypoints,
  };
}

/** Finds a bounded-sampling approximation of the nearest route insertion point. */
export function nearestRouteInsertion(
  segments: readonly EditableCubicSegment[],
  point: XYPosition,
): { insertIndex: number; point: XYPosition } {
  const safePoint = finitePoint(point);
  if (segments.length === 0) {
    return { insertIndex: 0, point: safePoint };
  }

  let bestSegment = segments[0]!;
  let bestStart = 0;
  let bestEnd = 1 / ROUTE_SAMPLE_INTERVALS;
  let bestDistanceSquared = Number.POSITIVE_INFINITY;
  for (const segment of segments) {
    let intervalStart = segment.start;
    for (let sample = 1; sample <= ROUTE_SAMPLE_INTERVALS; sample += 1) {
      const intervalEnd = cubicPoint(segment, sample / ROUTE_SAMPLE_INTERVALS);
      const candidate = closestPointOnLine(intervalStart, intervalEnd, safePoint);
      if (candidate.distanceSquared < bestDistanceSquared) {
        bestDistanceSquared = candidate.distanceSquared;
        bestSegment = segment;
        bestStart = (sample - 1) / ROUTE_SAMPLE_INTERVALS;
        bestEnd = sample / ROUTE_SAMPLE_INTERVALS;
      }
      intervalStart = intervalEnd;
    }
  }

  for (let refinement = 0; refinement < INSERTION_REFINEMENT_STEPS; refinement += 1) {
    const midpoint = (bestStart + bestEnd) / 2;
    const startPoint = cubicPoint(bestSegment, bestStart);
    const midpointPoint = cubicPoint(bestSegment, midpoint);
    const endPoint = cubicPoint(bestSegment, bestEnd);
    const left = closestPointOnLine(startPoint, midpointPoint, safePoint);
    const right = closestPointOnLine(midpointPoint, endPoint, safePoint);
    if (left.distanceSquared <= right.distanceSquared) {
      bestEnd = midpoint;
    } else {
      bestStart = midpoint;
    }
  }

  const startPoint = cubicPoint(bestSegment, bestStart);
  const endPoint = cubicPoint(bestSegment, bestEnd);
  const refined = closestPointOnLine(startPoint, endPoint, safePoint);
  const parameter = bestStart + (bestEnd - bestStart) * refined.ratio;
  return {
    insertIndex: bestSegment.insertIndex,
    point: cubicPoint(bestSegment, parameter),
  };
}
