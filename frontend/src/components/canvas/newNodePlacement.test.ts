import { describe, expect, it } from 'vitest';

import {
  findNewNodePlacement,
  insetScreenRect,
  NEW_NODE_PREFERRED_GAP_PX,
  NEW_NODE_VIEWPORT_MARGIN_PX,
  type NewNodePlacementResult,
  type ScreenRect,
  type ScreenSize,
} from './newNodePlacement';

function intersectionArea(left: ScreenRect, right: ScreenRect): number {
  const overlapWidth = Math.max(
    0,
    Math.min(left.x + left.width, right.x + right.width) - Math.max(left.x, right.x),
  );
  const overlapHeight = Math.max(
    0,
    Math.min(left.y + left.height, right.y + right.height) - Math.max(left.y, right.y),
  );
  return overlapWidth * overlapHeight;
}

function expandRect(rect: ScreenRect, gap: number): ScreenRect {
  return {
    x: rect.x - gap,
    y: rect.y - gap,
    width: rect.width + gap * 2,
    height: rect.height + gap * 2,
  };
}

function resultRect(result: NewNodePlacementResult, nodeSize: ScreenSize): ScreenRect {
  return { ...result.topLeft, ...nodeSize };
}

function expectContained(
  result: NewNodePlacementResult | null,
  viewport: ScreenRect,
  nodeSize: ScreenSize,
): void {
  expect(result).not.toBeNull();
  if (!result || result.mode === 'oversized') return;

  const usableViewport = insetScreenRect(viewport, NEW_NODE_VIEWPORT_MARGIN_PX);
  expect(usableViewport).not.toBeNull();
  if (!usableViewport) return;

  expect(result.topLeft.x).toBeGreaterThanOrEqual(usableViewport.x);
  expect(result.topLeft.y).toBeGreaterThanOrEqual(usableViewport.y);
  expect(result.topLeft.x + nodeSize.width).toBeLessThanOrEqual(
    usableViewport.x + usableViewport.width,
  );
  expect(result.topLeft.y + nodeSize.height).toBeLessThanOrEqual(
    usableViewport.y + usableViewport.height,
  );
}

describe('findNewNodePlacement', () => {
  it('centers a fitting node inside the inset client-space viewport', () => {
    const viewport = { x: 100, y: 50, width: 800, height: 600 };
    const nodeSize = { width: 370, height: 140 };
    const result = findNewNodePlacement({ viewport, nodeSize, obstacles: [] });

    expect(result).toEqual({
      topLeft: { x: 315, y: 280 },
      overlapArea: 0,
      overlapCount: 0,
      mode: 'preferred-gap',
    });
    expectContained(result, viewport, nodeSize);
  });

  it.each([
    {
      viewport: { x: 0, y: 0, width: 0, height: 600 },
      nodeSize: { width: 100, height: 100 },
    },
    {
      viewport: { x: 0, y: 0, width: 800, height: 600 },
      nodeSize: { width: Number.NaN, height: 100 },
    },
    {
      viewport: { x: 0, y: 0, width: 800, height: 600 },
      nodeSize: { width: 100, height: -1 },
    },
  ])('returns null for invalid geometry', ({ viewport, nodeSize }) => {
    expect(findNewNodePlacement({ viewport, nodeSize, obstacles: [] })).toBeNull();
  });

  it('centers an oversized node to maximize the visible portion', () => {
    expect(
      findNewNodePlacement({
        viewport: { x: 10, y: 20, width: 200, height: 100 },
        nodeSize: { width: 300, height: 160 },
        obstacles: [],
      }),
    ).toMatchObject({
      topLeft: { x: -40, y: -10 },
      mode: 'oversized',
    });
  });

  it('uses preferred-gap placement when the center obstacle has clearance around it', () => {
    const viewport = { x: 40, y: 30, width: 400, height: 260 };
    const nodeSize = { width: 100, height: 80 };
    const obstacle = { x: 210, y: 120, width: 60, height: 80 };
    const result = findNewNodePlacement({
      viewport,
      nodeSize,
      obstacles: [obstacle],
    });

    expect(result).not.toBeNull();
    if (!result) return;

    const placementRect = resultRect(result, nodeSize);
    expect(intersectionArea(placementRect, obstacle)).toBe(0);
    expect(intersectionArea(placementRect, expandRect(obstacle, NEW_NODE_PREFERRED_GAP_PX))).toBe(
      0,
    );
    expect(result.mode).toBe('preferred-gap');
    expectContained(result, viewport, nodeSize);
  });

  it('uses no-gap placement when the reduced viewport only allows touching space', () => {
    const viewport = { x: 0, y: 0, width: 264, height: 132 };
    const nodeSize = { width: 100, height: 100 };
    const obstacle = { x: 116, y: 16, width: 32, height: 100 };
    const result = findNewNodePlacement({
      viewport,
      nodeSize,
      obstacles: [obstacle],
    });

    expect(result).not.toBeNull();
    if (!result) return;

    const placementRect = resultRect(result, nodeSize);
    expect(intersectionArea(placementRect, obstacle)).toBe(0);
    expect(
      intersectionArea(placementRect, expandRect(obstacle, NEW_NODE_PREFERRED_GAP_PX)),
    ).toBeGreaterThan(0);
    expect(result.mode).toBe('no-gap');
    expectContained(result, viewport, nodeSize);
  });

  it('uses least-overlap placement and reports exact collision statistics on a dense map', () => {
    const viewport = { x: 50, y: 20, width: 224, height: 192 };
    const nodeSize = { width: 64, height: 64 };
    const usableViewport = insetScreenRect(viewport, NEW_NODE_VIEWPORT_MARGIN_PX);
    expect(usableViewport).not.toBeNull();
    if (!usableViewport) return;

    const obstacles: ScreenRect[] = [];
    const obstacleSize = 32;
    const right = usableViewport.x + usableViewport.width;
    const bottom = usableViewport.y + usableViewport.height;
    for (let y = usableViewport.y; y < bottom; y += obstacleSize) {
      for (let x = usableViewport.x; x < right; x += obstacleSize) {
        obstacles.push({
          x,
          y,
          width: Math.min(obstacleSize, right - x),
          height: Math.min(obstacleSize, bottom - y),
        });
      }
    }

    const result = findNewNodePlacement({ viewport, nodeSize, obstacles });
    expect(result).not.toBeNull();
    if (!result) return;

    const placementRect = resultRect(result, nodeSize);
    const overlapAreas = obstacles
      .map((obstacle) => intersectionArea(placementRect, obstacle))
      .filter((area) => area > 0);
    expect(result.mode).toBe('least-overlap');
    expect(result.overlapArea).toBe(overlapAreas.reduce((total, area) => total + area, 0));
    expect(result.overlapCount).toBe(overlapAreas.length);
    expectContained(result, viewport, nodeSize);
  });

  it('uses a visible neighbor to resolve otherwise equal placements', () => {
    const viewport = { x: 0, y: 0, width: 264, height: 132 };
    const nodeSize = { width: 100, height: 100 };
    const obstacles = [{ x: 116, y: 16, width: 32, height: 100 }];
    const withoutNeighbor = findNewNodePlacement({ viewport, nodeSize, obstacles });
    const withNeighbor = findNewNodePlacement({
      viewport,
      nodeSize,
      obstacles,
      visibleNeighborCenters: [{ x: 230, y: 66 }],
    });

    expect(withoutNeighbor).toMatchObject({
      topLeft: { x: 16, y: 16 },
      mode: 'no-gap',
    });
    expect(withNeighbor).toMatchObject({
      topLeft: { x: 148, y: 16 },
      mode: 'no-gap',
    });
    expectContained(withoutNeighbor, viewport, nodeSize);
    expectContained(withNeighbor, viewport, nodeSize);
  });

  it('returns an identical result across repeated calls with cloned inputs', () => {
    const input = {
      viewport: { x: 25, y: 45, width: 420, height: 260 },
      nodeSize: { width: 96, height: 72 },
      obstacles: [
        { x: 155, y: 105, width: 80, height: 100 },
        { x: 270, y: 130, width: 70, height: 70 },
      ],
      visibleNeighborCenters: [{ x: 390, y: 165 }],
    };
    const results = Array.from({ length: 20 }, () =>
      findNewNodePlacement({
        viewport: { ...input.viewport },
        nodeSize: { ...input.nodeSize },
        obstacles: input.obstacles.map((obstacle) => ({ ...obstacle })),
        visibleNeighborCenters: input.visibleNeighborCenters.map((center) => ({ ...center })),
      }),
    );

    expect(results[0]).not.toBeNull();
    for (const result of results) {
      expect(result).toEqual(results[0]);
      expectContained(result, input.viewport, input.nodeSize);
    }
  });

  it('does not depend on obstacle insertion order', () => {
    const viewport = { x: 25, y: 45, width: 420, height: 260 };
    const nodeSize = { width: 96, height: 72 };
    const obstacles = [
      { x: 155, y: 105, width: 80, height: 100 },
      { x: 270, y: 130, width: 70, height: 70 },
      { x: 90, y: 190, width: 110, height: 60 },
    ];
    const forward = findNewNodePlacement({ viewport, nodeSize, obstacles });
    const reversed = findNewNodePlacement({
      viewport,
      nodeSize,
      obstacles: [...obstacles].reverse(),
    });

    expect(forward).not.toBeNull();
    expect(reversed).toEqual(forward);
    expectContained(forward, viewport, nodeSize);
    expectContained(reversed, viewport, nodeSize);
  });
});
