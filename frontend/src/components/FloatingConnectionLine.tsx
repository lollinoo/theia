/**
 * Renders the in-progress connection with the same floating geometry as saved links.
 */
import type { ConnectionLineComponentProps } from '@xyflow/react';
import type { JSX } from 'react';
import type { DeviceNode } from './DeviceCard';
import { buildFloatingEdgePath, deviceNodeBorderRadius, nodeRect } from './floatingEdgeGeometry';

/** Renders a pointer-transparent connection preview between live rounded node borders. */
export function FloatingConnectionLine({
  connectionLineStyle,
  fromNode,
  fromX,
  fromY,
  toNode,
  toX,
  toY,
}: ConnectionLineComponentProps<DeviceNode>): JSX.Element {
  const laneOrientation = toNode === null || fromNode.id <= toNode.id ? 1 : -1;
  const { edgePath } = buildFloatingEdgePath({
    sourceRect: nodeRect(fromNode),
    targetRect: nodeRect(toNode),
    fallbackSource: { x: fromX, y: fromY },
    fallbackTarget: { x: toX, y: toY },
    parallelIndex: 0,
    laneOrientation,
    sourceRadius: deviceNodeBorderRadius(fromNode),
    targetRadius: deviceNodeBorderRadius(toNode),
  });

  return (
    <path
      d={edgePath}
      pointerEvents="none"
      style={{
        fill: 'none',
        stroke: 'var(--color-edge-default)',
        strokeWidth: 10,
        ...connectionLineStyle,
      }}
    />
  );
}
