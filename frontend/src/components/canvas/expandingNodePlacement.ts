import {
  findNewNodePlacement,
  NEW_NODE_PREFERRED_GAP_PX,
  NEW_NODE_VIEWPORT_MARGIN_PX,
  type NewNodePlacementInput,
  type NewNodePlacementResult,
  type ScreenRect,
  type ScreenSize,
} from './newNodePlacement';

function isValidRect(rect: ScreenRect): boolean {
  return (
    Number.isFinite(rect.x) &&
    Number.isFinite(rect.y) &&
    Number.isFinite(rect.width) &&
    rect.width > 0 &&
    Number.isFinite(rect.height) &&
    rect.height > 0
  );
}

function isValidSize(size: ScreenSize): boolean {
  return (
    Number.isFinite(size.width) && size.width > 0 && Number.isFinite(size.height) && size.height > 0
  );
}

function maximumObstacleDistance(viewport: ScreenRect, obstacles: ScreenRect[]): number {
  return obstacles.reduce(
    (maximum, obstacle) =>
      Math.max(
        maximum,
        viewport.x - obstacle.x,
        viewport.y - obstacle.y,
        obstacle.x + obstacle.width - (viewport.x + viewport.width),
        obstacle.y + obstacle.height - (viewport.y + viewport.height),
      ),
    0,
  );
}

/** Finds a collision-free point, expanding symmetrically beyond the initial viewport when needed. */
export function findCollisionFreeExpandingPlacement(
  input: NewNodePlacementInput,
): NewNodePlacementResult | null {
  if (
    !isValidRect(input.viewport) ||
    !isValidSize(input.nodeSize) ||
    input.obstacles.some((obstacle) => !isValidRect(obstacle))
  ) {
    return null;
  }

  const margin = input.marginPx ?? NEW_NODE_VIEWPORT_MARGIN_PX;
  const preferredGap = input.preferredGapPx ?? NEW_NODE_PREFERRED_GAP_PX;
  if (
    !Number.isFinite(margin) ||
    margin < 0 ||
    !Number.isFinite(preferredGap) ||
    preferredGap < 0
  ) {
    return null;
  }

  const largestObstacleDimension = input.obstacles.reduce(
    (largest, obstacle) => Math.max(largest, obstacle.width, obstacle.height),
    0,
  );
  const expansionStep =
    Math.max(input.nodeSize.width, input.nodeSize.height, largestObstacleDimension, 1) +
    (margin + preferredGap) * 2;
  const maximumRing =
    Math.ceil(maximumObstacleDistance(input.viewport, input.obstacles) / expansionStep) +
    input.obstacles.length +
    4;

  for (let ring = 0; ring <= maximumRing; ring += 1) {
    const expansion = ring * expansionStep;
    const result = findNewNodePlacement({
      ...input,
      viewport: {
        x: input.viewport.x - expansion,
        y: input.viewport.y - expansion,
        width: input.viewport.width + expansion * 2,
        height: input.viewport.height + expansion * 2,
      },
    });
    if (
      result &&
      result.overlapArea === 0 &&
      result.overlapCount === 0 &&
      result.mode !== 'oversized' &&
      result.mode !== 'least-overlap'
    ) {
      return result;
    }
  }

  return null;
}
