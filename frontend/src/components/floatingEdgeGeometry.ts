/**
 * Builds finite cubic paths between the visible rounded borders of topology nodes.
 */
import type { InternalNode, Node, Rect, XYPosition } from '@xyflow/react';
import type { DeviceNode } from './DeviceCard';

const DEFAULT_NODE_RADIUS = 20;
const EPSILON = 0.000001;
const MAX_AUTOMATIC_BEND = 64;
const MAX_TERMINAL_LEAD = 24;

/** Describes the rendered composite path and its label/control geometry. */
export interface EdgePathModel {
  edgePath: string;
  labelX: number;
  labelY: number;
  source: XYPosition;
  sourceLead: XYPosition;
  sourceControl: XYPosition;
  targetControl: XYPosition;
  targetLead: XYPosition;
  target: XYPosition;
}

interface FloatingEdgePathOptions {
  sourceRect: Rect | null;
  targetRect: Rect | null;
  fallbackSource: XYPosition;
  fallbackTarget: XYPosition;
  parallelIndex: number;
  laneOrientation?: 1 | -1;
  sourceRadius?: number;
  targetRadius?: number;
}

/** One visible border anchor and its outward-facing unit normal. */
export interface FloatingEndpoint {
  point: XYPosition;
  normal: XYPosition;
}

function clamp(value: number, minimum: number, maximum: number): number {
  return Math.min(maximum, Math.max(minimum, value));
}

function finitePoint(point: XYPosition): XYPosition {
  return {
    x: Number.isFinite(point.x) ? point.x : 0,
    y: Number.isFinite(point.y) ? point.y : 0,
  };
}

function validRect(rect: Rect | null): rect is Rect {
  return (
    rect !== null &&
    Number.isFinite(rect.x) &&
    Number.isFinite(rect.y) &&
    Number.isFinite(rect.width) &&
    Number.isFinite(rect.height) &&
    rect.width > 0 &&
    rect.height > 0
  );
}

function rectCenter(rect: Rect): XYPosition {
  return {
    x: rect.x + rect.width / 2,
    y: rect.y + rect.height / 2,
  };
}

function normalize(vector: XYPosition, fallback: XYPosition): XYPosition {
  const length = Math.hypot(vector.x, vector.y);
  if (!Number.isFinite(length) || length < EPSILON) {
    return fallback;
  }
  return { x: vector.x / length, y: vector.y / length };
}

function roundedRectIntersection(
  rect: Rect,
  toward: XYPosition,
  requestedRadius: number,
): FloatingEndpoint {
  const center = rectCenter(rect);
  const direction = normalize({ x: toward.x - center.x, y: toward.y - center.y }, { x: 1, y: 0 });
  const halfWidth = rect.width / 2;
  const halfHeight = rect.height / 2;
  const radius = clamp(
    Number.isFinite(requestedRadius) ? requestedRadius : DEFAULT_NODE_RADIUS,
    0,
    Math.min(halfWidth, halfHeight),
  );
  const horizontalDistance =
    Math.abs(direction.x) < EPSILON ? Number.POSITIVE_INFINITY : halfWidth / Math.abs(direction.x);
  const verticalDistance =
    Math.abs(direction.y) < EPSILON ? Number.POSITIVE_INFINITY : halfHeight / Math.abs(direction.y);
  const outerDistance = Math.min(horizontalDistance, verticalDistance);
  const outerPoint = {
    x: direction.x * outerDistance,
    y: direction.y * outerDistance,
  };

  if (Math.abs(outerPoint.y) <= halfHeight - radius + EPSILON) {
    const normal = { x: Math.sign(direction.x) || 1, y: 0 };
    return {
      point: { x: center.x + normal.x * halfWidth, y: center.y + outerPoint.y },
      normal,
    };
  }

  if (Math.abs(outerPoint.x) <= halfWidth - radius + EPSILON) {
    const normal = { x: 0, y: Math.sign(direction.y) || 1 };
    return {
      point: { x: center.x + outerPoint.x, y: center.y + normal.y * halfHeight },
      normal,
    };
  }

  const cornerCenter = {
    x: (Math.sign(direction.x) || 1) * (halfWidth - radius),
    y: (Math.sign(direction.y) || 1) * (halfHeight - radius),
  };
  const projection = direction.x * cornerCenter.x + direction.y * cornerCenter.y;
  const discriminant = Math.max(
    0,
    projection ** 2 - (cornerCenter.x ** 2 + cornerCenter.y ** 2 - radius ** 2),
  );
  const distance = projection + Math.sqrt(discriminant);
  const localPoint = {
    x: direction.x * distance,
    y: direction.y * distance,
  };
  const normal = normalize(
    {
      x: localPoint.x - cornerCenter.x,
      y: localPoint.y - cornerCenter.y,
    },
    direction,
  );

  return {
    point: { x: center.x + localPoint.x, y: center.y + localPoint.y },
    normal,
  };
}

function fallbackIntersection(
  point: XYPosition,
  toward: XYPosition,
  fallbackNormal: XYPosition,
): FloatingEndpoint {
  return {
    point,
    normal: normalize({ x: toward.x - point.x, y: toward.y - point.y }, fallbackNormal),
  };
}

function outwardControl(
  anchor: XYPosition,
  normal: XYPosition,
  perpendicular: XYPosition,
  controlLength: number,
  bend: number,
): XYPosition {
  const displacement = {
    x: normal.x * controlLength + perpendicular.x * bend,
    y: normal.y * controlLength + perpendicular.y * bend,
  };
  const outwardProjection = displacement.x * normal.x + displacement.y * normal.y;
  const correction = Math.min(0, outwardProjection);

  return {
    x: anchor.x + displacement.x - normal.x * correction,
    y: anchor.y + displacement.y - normal.y * correction,
  };
}

/** Resolves a measured node to the absolute rectangle used by React Flow. */
export function nodeRect<NodeType extends Node>(
  internalNode: InternalNode<NodeType> | null | undefined,
): Rect | null {
  if (!internalNode) return null;

  const { width, height } = internalNode.measured;
  const { x, y } = internalNode.internals.positionAbsolute;
  const rect = {
    x,
    y,
    width: width ?? Number.NaN,
    height: height ?? Number.NaN,
  };
  return validRect(rect) ? rect : null;
}

/** Matches the visible radius of physical, virtual, and ghost device cards. */
export function deviceNodeBorderRadius(
  internalNode: InternalNode<DeviceNode> | null | undefined,
): number {
  if (internalNode?.data.kind === 'ghost-device' || internalNode?.data.isGhost === true) {
    return 16;
  }
  if (internalNode?.data.isVirtual === true) {
    return 24;
  }
  return DEFAULT_NODE_RADIUS;
}

/** Resolves finite source and target anchors on their visible rounded borders. */
export function resolveFloatingEndpoints({
  sourceRect,
  targetRect,
  fallbackSource,
  fallbackTarget,
  sourceToward,
  targetToward,
  sourceRadius = DEFAULT_NODE_RADIUS,
  targetRadius = DEFAULT_NODE_RADIUS,
}: {
  sourceRect: Rect | null;
  targetRect: Rect | null;
  fallbackSource: XYPosition;
  fallbackTarget: XYPosition;
  sourceToward?: XYPosition;
  targetToward?: XYPosition;
  sourceRadius?: number;
  targetRadius?: number;
}): { source: FloatingEndpoint; target: FloatingEndpoint } {
  const safeSource = finitePoint(fallbackSource);
  const safeTarget = finitePoint(fallbackTarget);
  const hasSourceRect = validRect(sourceRect);
  const hasTargetRect = validRect(targetRect);
  const sourceCenter = hasSourceRect ? rectCenter(sourceRect) : safeSource;
  const targetCenter = hasTargetRect ? rectCenter(targetRect) : safeTarget;
  const centerDirection = normalize(
    { x: targetCenter.x - sourceCenter.x, y: targetCenter.y - sourceCenter.y },
    normalize({ x: safeTarget.x - safeSource.x, y: safeTarget.y - safeSource.y }, { x: 1, y: 0 }),
  );
  const safeSourceToward = finitePoint(sourceToward ?? targetCenter);
  const safeTargetToward = finitePoint(targetToward ?? sourceCenter);
  const sourceEndpoint = hasSourceRect
    ? roundedRectIntersection(sourceRect, safeSourceToward, sourceRadius)
    : fallbackIntersection(safeSource, safeSourceToward, centerDirection);
  const targetEndpoint = hasTargetRect
    ? roundedRectIntersection(targetRect, safeTargetToward, targetRadius)
    : fallbackIntersection(safeTarget, safeTargetToward, {
        x: -centerDirection.x,
        y: -centerDirection.y,
      });

  return {
    source: {
      point: finitePoint(sourceEndpoint.point),
      normal: normalize(sourceEndpoint.normal, centerDirection),
    },
    target: {
      point: finitePoint(targetEndpoint.point),
      normal: normalize(targetEndpoint.normal, {
        x: -centerDirection.x,
        y: -centerDirection.y,
      }),
    },
  };
}

/** Builds straight terminal leads around one adaptive cubic core. */
export function buildFloatingEdgePath({
  sourceRect,
  targetRect,
  fallbackSource,
  fallbackTarget,
  parallelIndex,
  laneOrientation = 1,
  sourceRadius = DEFAULT_NODE_RADIUS,
  targetRadius = DEFAULT_NODE_RADIUS,
}: FloatingEdgePathOptions): EdgePathModel {
  const endpoints = resolveFloatingEndpoints({
    sourceRect,
    targetRect,
    fallbackSource,
    fallbackTarget,
    sourceRadius,
    targetRadius,
  });
  const source = endpoints.source.point;
  const target = endpoints.target.point;
  const edgeDirection = normalize(
    { x: target.x - source.x, y: target.y - source.y },
    endpoints.source.normal,
  );
  const perpendicular = {
    x: -edgeDirection.y * laneOrientation,
    y: edgeDirection.x * laneOrientation,
  };
  const distance = Math.hypot(target.x - source.x, target.y - source.y);
  const controlLength = Math.min(clamp(distance * 0.42, 48, 180), distance * 0.45);
  const leadLength = Math.min(MAX_TERMINAL_LEAD, distance * 0.12);
  const curveControlLength = Math.max(0, controlLength - leadLength);
  const sourceLead = finitePoint({
    x: source.x + endpoints.source.normal.x * leadLength,
    y: source.y + endpoints.source.normal.y * leadLength,
  });
  const targetLead = finitePoint({
    x: target.x + endpoints.target.normal.x * leadLength,
    y: target.y + endpoints.target.normal.y * leadLength,
  });
  const lane =
    parallelIndex === 0
      ? 1
      : (parallelIndex % 2 === 1 ? -1 : 1) * (Math.ceil(parallelIndex / 2) + 1);
  const bend = Math.min(MAX_AUTOMATIC_BEND, distance * 0.18) * lane;
  const sourceControl = finitePoint(
    outwardControl(sourceLead, endpoints.source.normal, perpendicular, curveControlLength, bend),
  );
  const targetControl = finitePoint(
    outwardControl(targetLead, endpoints.target.normal, perpendicular, curveControlLength, bend),
  );
  const labelX = (sourceLead.x + 3 * sourceControl.x + 3 * targetControl.x + targetLead.x) / 8;
  const labelY = (sourceLead.y + 3 * sourceControl.y + 3 * targetControl.y + targetLead.y) / 8;

  return {
    edgePath: `M ${source.x},${source.y} L ${sourceLead.x},${sourceLead.y} C ${sourceControl.x},${sourceControl.y} ${targetControl.x},${targetControl.y} ${targetLead.x},${targetLead.y} L ${target.x},${target.y}`,
    labelX,
    labelY,
    source,
    sourceLead,
    sourceControl,
    targetControl,
    targetLead,
    target,
  };
}
