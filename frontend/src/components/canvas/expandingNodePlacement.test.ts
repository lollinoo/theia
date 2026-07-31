import { describe, expect, it } from 'vitest';
import { findCollisionFreeExpandingPlacement } from './expandingNodePlacement';
import type { ScreenRect } from './newNodePlacement';

function intersectionArea(left: ScreenRect, right: ScreenRect): number {
  const width = Math.max(
    0,
    Math.min(left.x + left.width, right.x + right.width) - Math.max(left.x, right.x),
  );
  const height = Math.max(
    0,
    Math.min(left.y + left.height, right.y + right.height) - Math.max(left.y, right.y),
  );
  return width * height;
}

describe('findCollisionFreeExpandingPlacement', () => {
  it('expands beyond a saturated viewport instead of accepting overlap', () => {
    const viewport = { x: 0, y: 0, width: 200, height: 140 };
    const nodeSize = { width: 60, height: 40 };
    const obstacle = { ...viewport };

    const result = findCollisionFreeExpandingPlacement({
      viewport,
      nodeSize,
      obstacles: [obstacle],
      marginPx: 0,
      preferredGapPx: 12,
    });

    expect(result).not.toBeNull();
    if (!result) return;

    const placedRect = { ...result.topLeft, ...nodeSize };
    expect(result.overlapArea).toBe(0);
    expect(result.overlapCount).toBe(0);
    expect(intersectionArea(placedRect, obstacle)).toBe(0);
    expect(
      placedRect.x < viewport.x ||
        placedRect.y < viewport.y ||
        placedRect.x + placedRect.width > viewport.x + viewport.width ||
        placedRect.y + placedRect.height > viewport.y + viewport.height,
    ).toBe(true);
  });

  it('returns the same collision-free result for the same geometry', () => {
    const input = {
      viewport: { x: 20, y: 30, width: 220, height: 160 },
      nodeSize: { width: 80, height: 50 },
      obstacles: [{ x: 20, y: 30, width: 220, height: 160 }],
      marginPx: 0,
      preferredGapPx: 8,
    };

    expect(findCollisionFreeExpandingPlacement(input)).toEqual(
      findCollisionFreeExpandingPlacement(input),
    );
  });

  it('expands until an oversized node fits without using the oversized fallback', () => {
    const result = findCollisionFreeExpandingPlacement({
      viewport: { x: 0, y: 0, width: 100, height: 80 },
      nodeSize: { width: 180, height: 120 },
      obstacles: [],
      marginPx: 0,
    });

    expect(result).toMatchObject({
      overlapArea: 0,
      overlapCount: 0,
      mode: 'preferred-gap',
    });
  });

  it('returns null for invalid geometry', () => {
    expect(
      findCollisionFreeExpandingPlacement({
        viewport: { x: 0, y: 0, width: 200, height: 140 },
        nodeSize: { width: Number.NaN, height: 40 },
        obstacles: [],
      }),
    ).toBeNull();
  });
});
