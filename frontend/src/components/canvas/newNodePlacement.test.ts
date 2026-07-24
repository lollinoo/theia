import { describe, expect, it } from 'vitest';

import {
  findNewNodePlacement,
  insetScreenRect,
  NEW_NODE_PREFERRED_GAP_PX,
  NEW_NODE_VIEWPORT_MARGIN_PX,
  type NewNodePlacementInput,
  type NewNodePlacementResult,
  type ScreenPoint,
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

function validationInput(overrides: Partial<NewNodePlacementInput> = {}): NewNodePlacementInput {
  return {
    viewport: { x: 0, y: 0, width: 400, height: 260 },
    nodeSize: { width: 100, height: 80 },
    obstacles: [{ x: 170, y: 100, width: 60, height: 60 }],
    ...overrides,
  };
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

function findExhaustiveZeroOverlapPoint(input: NewNodePlacementInput, gap = 0): ScreenPoint | null {
  const usableViewport = insetScreenRect(
    input.viewport,
    input.marginPx ?? NEW_NODE_VIEWPORT_MARGIN_PX,
  );
  if (
    !usableViewport ||
    input.nodeSize.width > usableViewport.width ||
    input.nodeSize.height > usableViewport.height
  ) {
    return null;
  }

  const minimumX = Math.ceil(usableViewport.x);
  const maximumX = Math.floor(usableViewport.x + usableViewport.width - input.nodeSize.width);
  const minimumY = Math.ceil(usableViewport.y);
  const maximumY = Math.floor(usableViewport.y + usableViewport.height - input.nodeSize.height);

  for (let y = minimumY; y <= maximumY; y += 1) {
    for (let x = minimumX; x <= maximumX; x += 1) {
      const candidate = { x, y, ...input.nodeSize };
      if (
        input.obstacles.every(
          (obstacle) => intersectionArea(candidate, expandRect(obstacle, gap)) === 0,
        )
      ) {
        return { x, y };
      }
    }
  }
  return null;
}

function findExhaustiveMinimumOverlapArea(input: NewNodePlacementInput): number | null {
  const usableViewport = insetScreenRect(
    input.viewport,
    input.marginPx ?? NEW_NODE_VIEWPORT_MARGIN_PX,
  );
  if (
    !usableViewport ||
    input.nodeSize.width > usableViewport.width ||
    input.nodeSize.height > usableViewport.height
  ) {
    return null;
  }

  const minimumX = Math.ceil(usableViewport.x);
  const maximumX = Math.floor(usableViewport.x + usableViewport.width - input.nodeSize.width);
  const minimumY = Math.ceil(usableViewport.y);
  const maximumY = Math.floor(usableViewport.y + usableViewport.height - input.nodeSize.height);
  let minimumOverlapArea = Number.POSITIVE_INFINITY;

  for (let y = minimumY; y <= maximumY; y += 1) {
    for (let x = minimumX; x <= maximumX; x += 1) {
      const candidate = { x, y, ...input.nodeSize };
      const overlapArea = input.obstacles.reduce(
        (total, obstacle) => total + intersectionArea(candidate, obstacle),
        0,
      );
      minimumOverlapArea = Math.min(minimumOverlapArea, overlapArea);
    }
  }
  return minimumOverlapArea;
}

const COMPLETE_SEARCH_REGRESSION_INPUT = {
  viewport: { x: 37, y: 29, width: 224, height: 176 },
  nodeSize: { width: 56, height: 42 },
  obstacles: [
    { x: 81, y: 25, width: 56, height: 13 },
    { x: 144, y: 33, width: 63, height: 54 },
    { x: 45, y: 163, width: 50, height: 15 },
    { x: 191, y: 162, width: 30, height: 22 },
    { x: 140, y: 93, width: 25, height: 48 },
    { x: 92, y: 22, width: 51, height: 38 },
    { x: 142, y: 172, width: 23, height: 22 },
    { x: 112, y: 186, width: 52, height: 35 },
    { x: 151, y: 131, width: 57, height: 38 },
    { x: 157, y: 168, width: 47, height: 50 },
    { x: 105, y: 82, width: 17, height: 43 },
    { x: 67, y: 12, width: 67, height: 54 },
  ],
} satisfies NewNodePlacementInput;

const COMPLETENESS_FIXTURES: { name: string; input: NewNodePlacementInput }[] = [
  {
    name: 'a gap using critical axes from different obstacles',
    input: COMPLETE_SEARCH_REGRESSION_INPUT,
  },
  {
    name: 'a gap along the bottom viewport edge',
    input: {
      viewport: { x: 0, y: 0, width: 96, height: 96 },
      nodeSize: { width: 24, height: 24 },
      obstacles: [{ x: 36, y: 36, width: 24, height: 20 }],
    },
  },
  {
    name: 'a gap along the left viewport edge',
    input: {
      viewport: { x: 10, y: 20, width: 112, height: 96 },
      nodeSize: { width: 24, height: 20 },
      obstacles: [{ x: 50, y: 30, width: 18, height: 70 }],
    },
  },
];

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
      viewport: { x: 0, y: 0, width: 800, height: -1 },
      nodeSize: { width: 100, height: 100 },
    },
    {
      viewport: { x: Number.NaN, y: 0, width: 800, height: 600 },
      nodeSize: { width: 100, height: 100 },
    },
    {
      viewport: { x: 0, y: Number.POSITIVE_INFINITY, width: 800, height: 600 },
      nodeSize: { width: 100, height: 100 },
    },
    {
      viewport: { x: 0, y: 0, width: 800, height: 600 },
      nodeSize: { width: Number.NaN, height: 100 },
    },
    {
      viewport: { x: 0, y: 0, width: 800, height: 600 },
      nodeSize: { width: 0, height: 100 },
    },
    {
      viewport: { x: 0, y: 0, width: 800, height: 600 },
      nodeSize: { width: 100, height: -1 },
    },
    {
      viewport: { x: 0, y: 0, width: 800, height: 600 },
      nodeSize: { width: 100, height: Number.POSITIVE_INFINITY },
    },
  ])('returns null for invalid geometry', ({ viewport, nodeSize }) => {
    expect(findNewNodePlacement({ viewport, nodeSize, obstacles: [] })).toBeNull();
  });

  it.each([
    {
      name: 'a non-finite obstacle coordinate',
      obstacle: { x: Number.NaN, y: 40, width: 80, height: 60 },
    },
    {
      name: 'a non-positive obstacle width',
      obstacle: { x: 100, y: 40, width: 0, height: 60 },
    },
    {
      name: 'a non-positive obstacle height',
      obstacle: { x: 100, y: 40, width: 80, height: -1 },
    },
  ])('returns null for $name', ({ obstacle }) => {
    expect(
      findNewNodePlacement({
        viewport: { x: 0, y: 0, width: 400, height: 260 },
        nodeSize: { width: 100, height: 80 },
        obstacles: [obstacle],
      }),
    ).toBeNull();
  });

  it.each([
    { name: 'a non-finite neighbor x coordinate', center: { x: Number.NaN, y: 80 } },
    {
      name: 'a non-finite neighbor y coordinate',
      center: { x: 120, y: Number.POSITIVE_INFINITY },
    },
  ])('returns null for $name', ({ center }) => {
    expect(
      findNewNodePlacement({
        viewport: { x: 0, y: 0, width: 400, height: 260 },
        nodeSize: { width: 100, height: 80 },
        obstacles: [],
        visibleNeighborCenters: [center],
      }),
    ).toBeNull();
  });

  it.each([
    { name: 'a negative margin', input: validationInput({ marginPx: -1 }) },
    {
      name: 'a non-finite margin',
      input: validationInput({ marginPx: Number.NaN }),
    },
    {
      name: 'a negative preferred gap',
      input: validationInput({ preferredGapPx: -1 }),
    },
    {
      name: 'a non-finite preferred gap',
      input: validationInput({ preferredGapPx: Number.NaN }),
    },
    {
      name: 'a negative candidate step',
      input: validationInput({
        viewport: { x: 0, y: 0, width: 132, height: 132 },
        nodeSize: { width: 100, height: 100 },
        candidateStepPx: -1,
      }),
    },
    {
      name: 'a zero candidate step',
      input: validationInput({
        viewport: { x: 0, y: 0, width: 132, height: 132 },
        nodeSize: { width: 100, height: 100 },
        candidateStepPx: 0,
      }),
    },
    {
      name: 'a non-finite candidate step',
      input: validationInput({ candidateStepPx: Number.NaN }),
    },
    {
      name: 'a negative spatial cell',
      input: validationInput({ spatialCellPx: -1 }),
    },
    {
      name: 'a zero spatial cell',
      input: validationInput({
        viewport: { x: 0, y: 0, width: 100, height: 100 },
        nodeSize: { width: 100, height: 100 },
        obstacles: [{ x: 0, y: 0, width: 20, height: 20 }],
        marginPx: 0,
        preferredGapPx: 0,
        spatialCellPx: 0,
      }),
    },
    {
      name: 'a non-finite spatial cell',
      input: validationInput({ spatialCellPx: Number.NaN }),
    },
  ])('returns null for $name configuration', ({ input }) => {
    expect(findNewNodePlacement(input)).toBeNull();
  });

  it('returns null when the viewport inset leaves no usable area', () => {
    expect(
      findNewNodePlacement({
        viewport: { x: 0, y: 0, width: NEW_NODE_VIEWPORT_MARGIN_PX * 2, height: 200 },
        nodeSize: { width: 10, height: 10 },
        obstacles: [],
      }),
    ).toBeNull();
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

  it('finds a complete preferred-gap placement before accepting a no-gap fallback', () => {
    const input = {
      viewport: { x: 0, y: 0, width: 240, height: 180 },
      nodeSize: { width: 50, height: 36 },
      obstacles: [
        { x: 151, y: 75, width: 68, height: 16 },
        { x: 40, y: 23, width: 22, height: 37 },
        { x: 185, y: 74, width: 23, height: 21 },
        { x: 48, y: 148, width: 33, height: 34 },
        { x: 155, y: 89, width: 53, height: 45 },
        { x: -12, y: 31, width: 69, height: 13 },
        { x: -7, y: 100, width: 59, height: 49 },
        { x: 188, y: 119, width: 44, height: 58 },
      ],
    } satisfies NewNodePlacementInput;
    const exhaustivePoint = findExhaustiveZeroOverlapPoint(input, NEW_NODE_PREFERRED_GAP_PX);
    expect(exhaustivePoint).not.toBeNull();

    const result = findNewNodePlacement(input);
    expect(result).not.toBeNull();
    if (!result) return;

    const placementRect = resultRect(result, input.nodeSize);
    expect(result.mode).toBe('preferred-gap');
    expect(result.overlapArea).toBe(0);
    expect(result.overlapCount).toBe(0);
    expect(
      input.obstacles.every(
        (obstacle) =>
          intersectionArea(placementRect, expandRect(obstacle, NEW_NODE_PREFERRED_GAP_PX)) === 0,
      ),
    ).toBe(true);
    expectContained(result, input.viewport, input.nodeSize);
  });

  it('finds a no-gap placement when free critical axes come from different obstacles', () => {
    const { viewport, nodeSize, obstacles } = COMPLETE_SEARCH_REGRESSION_INPUT;
    const result = findNewNodePlacement(COMPLETE_SEARCH_REGRESSION_INPUT);

    expect(result).toMatchObject({
      overlapArea: 0,
      overlapCount: 0,
      mode: 'no-gap',
    });
    expectContained(result, viewport, nodeSize);
    if (!result) return;

    const placementRect = resultRect(result, nodeSize);
    expect(obstacles.every((obstacle) => intersectionArea(placementRect, obstacle) === 0)).toBe(
      true,
    );
  });

  it.each(COMPLETENESS_FIXTURES)(
    'returns a contained zero-overlap result for $name when an exhaustive pixel scan finds one',
    ({ input }) => {
      const exhaustivePoint = findExhaustiveZeroOverlapPoint(input);
      expect(exhaustivePoint).not.toBeNull();
      if (!exhaustivePoint) return;

      const result = findNewNodePlacement(input);
      expect(result).not.toBeNull();
      if (!result) return;

      const placementRect = resultRect(result, input.nodeSize);
      expect(result.overlapArea).toBe(0);
      expect(result.overlapCount).toBe(0);
      expect(['preferred-gap', 'no-gap']).toContain(result.mode);
      expect(
        input.obstacles.every((obstacle) => intersectionArea(placementRect, obstacle) === 0),
      ).toBe(true);
      expectContained(result, input.viewport, input.nodeSize);
    },
  );

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

  it('uses the global minimum total overlap area when every contained position collides', () => {
    const input = {
      viewport: { x: 0, y: 0, width: 160, height: 130 },
      nodeSize: { width: 50, height: 36 },
      obstacles: [
        { x: 111, y: 36, width: 64, height: 16 },
        { x: 131, y: 58, width: 51, height: 56 },
        { x: 12, y: 11, width: 31, height: 65 },
        { x: 34, y: 105, width: 51, height: 65 },
        { x: 122, y: 86, width: 45, height: 64 },
        { x: 27, y: 1, width: 64, height: 44 },
        { x: 42, y: 67, width: 33, height: 38 },
        { x: 8, y: 57, width: 28, height: 18 },
      ],
    } satisfies NewNodePlacementInput;
    const oracleMinimumOverlapArea = findExhaustiveMinimumOverlapArea(input);
    expect(oracleMinimumOverlapArea).toBe(6);

    const result = findNewNodePlacement(input);
    expect(result).not.toBeNull();
    if (!result) return;

    const placementRect = resultRect(result, input.nodeSize);
    const exactOverlapArea = input.obstacles.reduce(
      (total, obstacle) => total + intersectionArea(placementRect, obstacle),
      0,
    );
    expect(result.mode).toBe('least-overlap');
    expect(result.overlapArea).toBe(exactOverlapArea);
    expect(result.overlapArea).toBe(oracleMinimumOverlapArea);
    expectContained(result, input.viewport, input.nodeSize);
  });

  it('preserves secondary ranking candidates along a flat exact-overlap minimum', () => {
    const input = {
      viewport: {
        x: -6.355029419064522,
        y: 9.320330461487174,
        width: 195.3349528312683,
        height: 66.95976541843265,
      },
      nodeSize: {
        width: 158.42322935838013,
        height: 24.44093485154612,
      },
      obstacles: [
        {
          x: 135.33327535931,
          y: -7.41893988704679,
          width: 46.2550193653442,
          height: 91.66594740469009,
        },
        {
          x: -86.98435049752527,
          y: 29.430537507538304,
          width: 90.24474427523091,
          height: 32.579944527707994,
        },
      ],
    } satisfies NewNodePlacementInput;

    expect(findNewNodePlacement(input)).toEqual({
      topLeft: {
        x: 9.644970580935478,
        y: 29.430537507538304,
      },
      overlapArea: 800.0721590301928,
      overlapCount: 1,
      mode: 'least-overlap',
    });
  });

  it('bounds dense placement sweep work without changing the exact result', () => {
    let seed = 12345;
    const random = (): number => {
      seed = (Math.imul(seed, 1_664_525) + 1_013_904_223) >>> 0;
      return seed / 4_294_967_296;
    };
    const diagnostics = {
      exactCandidateScores: 0,
      collisionProbeCandidates: 0,
      leastOverlapAreaProbes: 0,
      obstacleIntersectionTests: 0,
      collisionSweepConstraintVisits: 0,
      collisionSweepEventUpdates: 0,
      horizontalWeightEvaluations: 0,
      horizontalWeightBufferAllocations: 0,
      verticalSweepEventVisits: 0,
    };
    const obstacles = Array.from({ length: 400 }, () => ({
      x: 16 + random() * 1038,
      y: 16 + random() * 728,
      width: 370,
      height: 140,
    }));

    const result = findNewNodePlacement({
      viewport: { x: 0, y: 0, width: 1440, height: 900 },
      nodeSize: { width: 370, height: 140 },
      obstacles,
      diagnostics,
    });

    expect(result).toEqual({
      topLeft: { x: 16, y: 744 },
      overlapArea: 136_355.24828116415,
      overlapCount: 18,
      mode: 'least-overlap',
    });
    expect.soft(diagnostics.exactCandidateScores).toBeGreaterThan(0);
    expect.soft(diagnostics.exactCandidateScores).toBeLessThan(64);
    expect.soft(diagnostics.collisionProbeCandidates).toBeGreaterThan(0);
    expect.soft(diagnostics.leastOverlapAreaProbes).toBeGreaterThan(0);
    expect.soft(diagnostics.leastOverlapAreaProbes).toBeLessThan(10_000);
    expect.soft(diagnostics.obstacleIntersectionTests).toBeGreaterThan(0);
    expect.soft(diagnostics.obstacleIntersectionTests).toBeLessThan(100_000);
    expect.soft(diagnostics.collisionSweepEventUpdates).toBeGreaterThan(0);
    expect
      .soft(diagnostics.collisionSweepConstraintVisits + diagnostics.collisionSweepEventUpdates)
      .toBeLessThan(5_000);
    expect.soft(diagnostics.horizontalWeightEvaluations).toBeGreaterThan(0);
    expect.soft(diagnostics.horizontalWeightEvaluations).toBeLessThan(400_000);
    expect.soft(diagnostics.horizontalWeightBufferAllocations).toBeGreaterThan(0);
    expect.soft(diagnostics.horizontalWeightBufferAllocations).toBeLessThan(4);
    expect.soft(diagnostics.verticalSweepEventVisits).toBeGreaterThan(0);
    expect.soft(diagnostics.verticalSweepEventVisits).toBeLessThan(2_500_000);
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
