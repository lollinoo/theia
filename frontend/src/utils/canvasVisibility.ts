/**
 * Provides canvas visibility utility behavior shared by frontend workflows.
 * Keeps non-UI policy and formatting rules reusable across components.
 */
import type { Transform } from '@xyflow/react';

const DEFAULT_OFFSCREEN_MARGIN_PX = 160;

interface IsNodeVisibleInViewportParams {
  nodeX: number;
  nodeY: number;
  nodeWidth: number;
  nodeHeight: number;
  viewportWidth: number;
  viewportHeight: number;
  transform: Transform;
  marginPx?: number;
}

/** Identifies node visible in viewport for the shared frontend utility layer. */
export function isNodeVisibleInViewport({
  nodeX,
  nodeY,
  nodeWidth,
  nodeHeight,
  viewportWidth,
  viewportHeight,
  transform,
  marginPx = DEFAULT_OFFSCREEN_MARGIN_PX,
}: IsNodeVisibleInViewportParams): boolean {
  if (viewportWidth <= 0 || viewportHeight <= 0) {
    return true;
  }

  const [translateX, translateY, zoom] = transform;
  const screenX = nodeX * zoom + translateX;
  const screenY = nodeY * zoom + translateY;
  const screenWidth = Math.max(0, nodeWidth * zoom);
  const screenHeight = Math.max(0, nodeHeight * zoom);

  return (
    screenX + screenWidth >= -marginPx &&
    screenY + screenHeight >= -marginPx &&
    screenX <= viewportWidth + marginPx &&
    screenY <= viewportHeight + marginPx
  );
}
