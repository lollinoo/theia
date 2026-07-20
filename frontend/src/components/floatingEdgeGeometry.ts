/**
 * Builds finite cubic paths between the visible rounded borders of topology nodes.
 */
import type { InternalNode, Node, Rect, XYPosition } from '@xyflow/react';
import type { DeviceNode } from './DeviceCard';

const DEFAULT_NODE_RADIUS = 20;
const EPSILON = 0.000001;

/** Describes the rendered cubic and its label/control geometry. */
export interface EdgePathModel {
  edgePath: string;
  labelX: number;
  labelY: number;
  source: XYPosition;
  sourceControl: XYPosition;
  targetControl: XYPosition;
  target: XYPosition;
}

interface FloatingEdgePathOptions {
  sourceRect: Rect | null;
  targetRect: Rect | null;
  fallbackSource: XYPosition;
  fallbackTarget: XYPosition;
  parallelIndex: number;
  sourceRadius?: number;
  targetRadius?: number;
}

interface BorderIntersection {
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
): BorderIntersection {
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
): BorderIntersection {
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

/** Builds one adaptive cubic between live node borders or finite React Flow fallbacks. */
export function buildFloatingEdgePath({
  sourceRect,
  targetRect,
  fallbackSource,
  fallbackTarget,
  parallelIndex,
  sourceRadius = DEFAULT_NODE_RADIUS,
  targetRadius = DEFAULT_NODE_RADIUS,
}: FloatingEdgePathOptions): EdgePathModel {
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
  const sourceIntersection = hasSourceRect
    ? roundedRectIntersection(sourceRect, targetCenter, sourceRadius)
    : fallbackIntersection(safeSource, targetCenter, centerDirection);
  const targetIntersection = hasTargetRect
    ? roundedRectIntersection(targetRect, sourceCenter, targetRadius)
    : fallbackIntersection(safeTarget, sourceCenter, {
        x: -centerDirection.x,
        y: -centerDirection.y,
      });
  const source = finitePoint(sourceIntersection.point);
  const target = finitePoint(targetIntersection.point);
  const edgeDirection = normalize(
    { x: target.x - source.x, y: target.y - source.y },
    centerDirection,
  );
  const perpendicular = { x: -edgeDirection.y, y: edgeDirection.x };
  const distance = Math.hypot(target.x - source.x, target.y - source.y);
  const controlLength = clamp(distance * 0.42, 48, 180);
  const lane =
    parallelIndex === 0
      ? 1
      : (parallelIndex % 2 === 1 ? -1 : 1) * (Math.ceil(parallelIndex / 2) + 1);
  const bend = Math.max(28, Math.min(92, distance * 0.18)) * lane;
  const sourceControl = finitePoint(
    outwardControl(source, sourceIntersection.normal, perpendicular, controlLength, bend),
  );
  const targetControl = finitePoint(
    outwardControl(target, targetIntersection.normal, perpendicular, controlLength, bend),
  );
  const labelX = (source.x + 3 * sourceControl.x + 3 * targetControl.x + target.x) / 8;
  const labelY = (source.y + 3 * sourceControl.y + 3 * targetControl.y + target.y) / 8;

  return {
    edgePath: `M ${source.x},${source.y} C ${sourceControl.x},${sourceControl.y} ${targetControl.x},${targetControl.y} ${target.x},${target.y}`,
    labelX,
    labelY,
    source,
    sourceControl,
    targetControl,
    target,
  };
}
