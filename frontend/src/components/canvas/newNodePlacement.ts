export const NEW_NODE_VIEWPORT_MARGIN_PX = 16;
export const NEW_NODE_PREFERRED_GAP_PX = 24;
export const NEW_NODE_CANDIDATE_STEP_PX = 16;
export const NEW_NODE_SPATIAL_CELL_PX = 64;

/** A point measured in browser client-space pixels. */
export interface ScreenPoint {
  x: number;
  y: number;
}

/** Screen-space dimensions measured in pixels. */
export interface ScreenSize {
  width: number;
  height: number;
}

/** A client-space rectangle with a top-left origin. */
export interface ScreenRect extends ScreenPoint, ScreenSize {}

/** Describes which collision fallback produced a node placement. */
export type PlacementMode = 'preferred-gap' | 'no-gap' | 'least-overlap' | 'oversized';

/** Inputs for deterministic node placement within a client-space viewport. */
export interface NewNodePlacementInput {
  viewport: ScreenRect;
  nodeSize: ScreenSize;
  obstacles: ScreenRect[];
  visibleNeighborCenters?: ScreenPoint[];
  marginPx?: number;
  preferredGapPx?: number;
  candidateStepPx?: number;
  spatialCellPx?: number;
}

/** The selected client-space position and its exact obstacle collision statistics. */
export interface NewNodePlacementResult {
  topLeft: ScreenPoint;
  overlapArea: number;
  overlapCount: number;
  mode: PlacementMode;
}

function isValidScreenPoint(point: ScreenPoint): boolean {
  return Number.isFinite(point.x) && Number.isFinite(point.y);
}

function isValidScreenSize(size: ScreenSize): boolean {
  return (
    Number.isFinite(size.width) && size.width > 0 && Number.isFinite(size.height) && size.height > 0
  );
}

function isFiniteNonNegative(value: number): boolean {
  return Number.isFinite(value) && value >= 0;
}

function isFinitePositive(value: number): boolean {
  return Number.isFinite(value) && value > 0;
}

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

interface SpatialHash {
  cellSize: number;
  cells: Map<string, number[]>;
  obstacles: ScreenRect[];
}

interface CollisionScore {
  overlapArea: number;
  overlapCount: number;
}

interface ScoredCandidate extends CollisionScore {
  topLeft: ScreenPoint;
}

function expandScreenRect(rect: ScreenRect, gap: number): ScreenRect {
  return {
    x: rect.x - gap,
    y: rect.y - gap,
    width: rect.width + gap * 2,
    height: rect.height + gap * 2,
  };
}

function forEachSpatialCell(
  rect: ScreenRect,
  cellSize: number,
  visit: (cellKey: string) => void,
): void {
  const minCellX = Math.floor(rect.x / cellSize);
  const maxCellX = Math.floor((rect.x + rect.width) / cellSize);
  const minCellY = Math.floor(rect.y / cellSize);
  const maxCellY = Math.floor((rect.y + rect.height) / cellSize);

  for (let cellY = minCellY; cellY <= maxCellY; cellY += 1) {
    for (let cellX = minCellX; cellX <= maxCellX; cellX += 1) {
      visit(`${cellX},${cellY}`);
    }
  }
}

function buildSpatialHash(obstacles: ScreenRect[], cellSize: number): SpatialHash {
  const cells = new Map<string, number[]>();
  obstacles.forEach((obstacle, obstacleId) => {
    forEachSpatialCell(obstacle, cellSize, (cellKey) => {
      const obstacleIds = cells.get(cellKey);
      if (obstacleIds) {
        obstacleIds.push(obstacleId);
      } else {
        cells.set(cellKey, [obstacleId]);
      }
    });
  });
  return { cellSize, cells, obstacles };
}

function queryObstacleIds(spatialHash: SpatialHash, rect: ScreenRect): Set<number> {
  const obstacleIds = new Set<number>();
  forEachSpatialCell(rect, spatialHash.cellSize, (cellKey) => {
    for (const obstacleId of spatialHash.cells.get(cellKey) ?? []) {
      obstacleIds.add(obstacleId);
    }
  });
  return obstacleIds;
}

function scoreCandidate(
  topLeft: ScreenPoint,
  nodeSize: ScreenSize,
  spatialHash: SpatialHash,
  gap: number,
): CollisionScore {
  const candidateRect = { ...topLeft, ...nodeSize };
  const queriedObstacleIds = queryObstacleIds(spatialHash, expandScreenRect(candidateRect, gap));
  const overlapAreas: number[] = [];

  for (const obstacleId of queriedObstacleIds) {
    const obstacle = spatialHash.obstacles[obstacleId];
    const overlapArea = intersectionArea(
      candidateRect,
      gap === 0 ? obstacle : expandScreenRect(obstacle, gap),
    );
    if (overlapArea > 0) {
      overlapAreas.push(overlapArea);
    }
  }

  overlapAreas.sort((left, right) => left - right);
  return {
    overlapArea: overlapAreas.reduce((total, area) => total + area, 0),
    overlapCount: overlapAreas.length,
  };
}

function squaredDistance(left: ScreenPoint, right: ScreenPoint): number {
  const deltaX = left.x - right.x;
  const deltaY = left.y - right.y;
  return deltaX * deltaX + deltaY * deltaY;
}

function nearestNeighborSquaredDistance(
  point: ScreenPoint,
  visibleNeighborCenters: ScreenPoint[],
): number {
  let nearestDistance = Number.POSITIVE_INFINITY;
  for (const neighborCenter of visibleNeighborCenters) {
    nearestDistance = Math.min(nearestDistance, squaredDistance(point, neighborCenter));
  }
  return nearestDistance;
}

function candidateCenter(topLeft: ScreenPoint, nodeSize: ScreenSize): ScreenPoint {
  return {
    x: topLeft.x + nodeSize.width / 2,
    y: topLeft.y + nodeSize.height / 2,
  };
}

function compareRank(left: number[], right: number[]): number {
  for (let index = 0; index < left.length; index += 1) {
    if (left[index] < right[index]) return -1;
    if (left[index] > right[index]) return 1;
  }
  return 0;
}

function collisionFreeRank(
  topLeft: ScreenPoint,
  nodeSize: ScreenSize,
  viewportCenter: ScreenPoint,
  visibleNeighborCenters: ScreenPoint[],
): number[] {
  const center = candidateCenter(topLeft, nodeSize);
  return [
    squaredDistance(center, viewportCenter),
    nearestNeighborSquaredDistance(center, visibleNeighborCenters),
    topLeft.y,
    topLeft.x,
  ];
}

function axisCandidates(minimum: number, maximum: number, step: number): number[] {
  const values: number[] = [];
  let value = minimum;
  while (value < maximum) {
    values.push(value);
    const nextValue = value + step;
    if (nextValue <= value) break;
    value = nextValue;
  }
  if (values[values.length - 1] !== maximum) {
    values.push(maximum);
  }
  return values;
}

function generateCandidates(
  usableViewport: ScreenRect,
  nodeSize: ScreenSize,
  obstacles: ScreenRect[],
  candidateStepPx: number,
  preferredGapPx: number,
): ScreenPoint[] {
  const minX = usableViewport.x;
  const maxX = usableViewport.x + usableViewport.width - nodeSize.width;
  const minY = usableViewport.y;
  const maxY = usableViewport.y + usableViewport.height - nodeSize.height;
  const candidates: ScreenPoint[] = [];
  const seen = new Set<string>();
  const addCandidate = ({ x, y }: ScreenPoint): void => {
    const clamped = {
      x: Math.min(maxX, Math.max(minX, x)),
      y: Math.min(maxY, Math.max(minY, y)),
    };
    const key = `${clamped.x},${clamped.y}`;
    if (seen.has(key)) return;
    seen.add(key);
    candidates.push(clamped);
  };

  addCandidate({
    x: minX + (maxX - minX) / 2,
    y: minY + (maxY - minY) / 2,
  });
  addCandidate({ x: minX, y: minY });
  addCandidate({ x: maxX, y: minY });
  addCandidate({ x: minX, y: maxY });
  addCandidate({ x: maxX, y: maxY });

  const xCandidates = axisCandidates(minX, maxX, candidateStepPx);
  const yCandidates = axisCandidates(minY, maxY, candidateStepPx);
  for (const y of yCandidates) {
    for (const x of xCandidates) {
      addCandidate({ x, y });
    }
  }

  const addObstacleAdjacentCandidates = (gap: number): void => {
    for (const obstacle of obstacles) {
      const centeredX = obstacle.x + (obstacle.width - nodeSize.width) / 2;
      const centeredY = obstacle.y + (obstacle.height - nodeSize.height) / 2;
      const leftX = obstacle.x - gap - nodeSize.width;
      const rightX = obstacle.x + obstacle.width + gap;
      const aboveY = obstacle.y - gap - nodeSize.height;
      const belowY = obstacle.y + obstacle.height + gap;

      addCandidate({ x: leftX, y: obstacle.y });
      addCandidate({ x: leftX, y: centeredY });
      addCandidate({ x: rightX, y: obstacle.y });
      addCandidate({ x: rightX, y: centeredY });
      addCandidate({ x: obstacle.x, y: aboveY });
      addCandidate({ x: centeredX, y: aboveY });
      addCandidate({ x: obstacle.x, y: belowY });
      addCandidate({ x: centeredX, y: belowY });
    }
  };

  addObstacleAdjacentCandidates(preferredGapPx);
  addObstacleAdjacentCandidates(0);
  return candidates;
}

function findBestCollisionFreeCandidate(
  candidates: ScreenPoint[],
  nodeSize: ScreenSize,
  spatialHash: SpatialHash,
  gap: number,
  viewportCenter: ScreenPoint,
  visibleNeighborCenters: ScreenPoint[],
): ScreenPoint | null {
  let best: { topLeft: ScreenPoint; rank: number[] } | null = null;
  for (const candidate of candidates) {
    if (scoreCandidate(candidate, nodeSize, spatialHash, gap).overlapCount > 0) continue;
    const rank = collisionFreeRank(candidate, nodeSize, viewportCenter, visibleNeighborCenters);
    if (!best || compareRank(rank, best.rank) < 0) {
      best = { topLeft: candidate, rank };
    }
  }
  return best?.topLeft ?? null;
}

function findLeastOverlapCandidate(
  candidates: ScreenPoint[],
  nodeSize: ScreenSize,
  spatialHash: SpatialHash,
  viewportCenter: ScreenPoint,
  visibleNeighborCenters: ScreenPoint[],
): ScoredCandidate | null {
  let best: { candidate: ScoredCandidate; rank: number[] } | null = null;
  for (const topLeft of candidates) {
    const collisionScore = scoreCandidate(topLeft, nodeSize, spatialHash, 0);
    const center = candidateCenter(topLeft, nodeSize);
    const rank = [
      collisionScore.overlapArea,
      collisionScore.overlapCount,
      squaredDistance(center, viewportCenter),
      nearestNeighborSquaredDistance(center, visibleNeighborCenters),
      topLeft.y,
      topLeft.x,
    ];
    if (!best || compareRank(rank, best.rank) < 0) {
      best = {
        candidate: { topLeft, ...collisionScore },
        rank,
      };
    }
  }
  return best?.candidate ?? null;
}

/** Insets a screen rectangle equally on every side, or returns null when no area remains. */
export function insetScreenRect(rect: ScreenRect, inset: number): ScreenRect | null {
  const width = rect.width - inset * 2;
  const height = rect.height - inset * 2;
  if (width <= 0 || height <= 0) return null;
  return { x: rect.x + inset, y: rect.y + inset, width, height };
}

/** Finds a deterministic, viewport-contained screen position for a new topology node. */
export function findNewNodePlacement(input: NewNodePlacementInput): NewNodePlacementResult | null {
  const marginPx = input.marginPx ?? NEW_NODE_VIEWPORT_MARGIN_PX;
  const preferredGapPx = input.preferredGapPx ?? NEW_NODE_PREFERRED_GAP_PX;
  const candidateStepPx = input.candidateStepPx ?? NEW_NODE_CANDIDATE_STEP_PX;
  const spatialCellPx = input.spatialCellPx ?? NEW_NODE_SPATIAL_CELL_PX;

  if (
    !isValidScreenPoint(input.viewport) ||
    !isValidScreenSize(input.viewport) ||
    !isValidScreenSize(input.nodeSize) ||
    input.obstacles.some(
      (obstacle) => !isValidScreenPoint(obstacle) || !isValidScreenSize(obstacle),
    ) ||
    input.visibleNeighborCenters?.some((center) => !isValidScreenPoint(center)) ||
    !isFiniteNonNegative(marginPx) ||
    !isFiniteNonNegative(preferredGapPx) ||
    !isFinitePositive(candidateStepPx) ||
    !isFinitePositive(spatialCellPx)
  ) {
    return null;
  }

  const usableViewport = insetScreenRect(input.viewport, marginPx);
  if (!usableViewport) return null;

  const centeredTopLeft = {
    x: usableViewport.x + (usableViewport.width - input.nodeSize.width) / 2,
    y: usableViewport.y + (usableViewport.height - input.nodeSize.height) / 2,
  };

  const nodeFits =
    input.nodeSize.width <= usableViewport.width && input.nodeSize.height <= usableViewport.height;
  if (!nodeFits) {
    return {
      topLeft: centeredTopLeft,
      overlapArea: 0,
      overlapCount: 0,
      mode: 'oversized',
    };
  }

  if (input.obstacles.length === 0) {
    return {
      topLeft: centeredTopLeft,
      overlapArea: 0,
      overlapCount: 0,
      mode: 'preferred-gap',
    };
  }

  const candidates = generateCandidates(
    usableViewport,
    input.nodeSize,
    input.obstacles,
    candidateStepPx,
    preferredGapPx,
  );
  const spatialHash = buildSpatialHash(input.obstacles, spatialCellPx);
  const viewportCenter = {
    x: usableViewport.x + usableViewport.width / 2,
    y: usableViewport.y + usableViewport.height / 2,
  };
  const visibleNeighborCenters = input.visibleNeighborCenters ?? [];
  const preferredCandidate = findBestCollisionFreeCandidate(
    candidates,
    input.nodeSize,
    spatialHash,
    preferredGapPx,
    viewportCenter,
    visibleNeighborCenters,
  );
  if (preferredCandidate) {
    return {
      topLeft: preferredCandidate,
      overlapArea: 0,
      overlapCount: 0,
      mode: 'preferred-gap',
    };
  }

  const noGapCandidate = findBestCollisionFreeCandidate(
    candidates,
    input.nodeSize,
    spatialHash,
    0,
    viewportCenter,
    visibleNeighborCenters,
  );
  if (noGapCandidate) {
    return {
      topLeft: noGapCandidate,
      overlapArea: 0,
      overlapCount: 0,
      mode: 'no-gap',
    };
  }

  const leastOverlapCandidate = findLeastOverlapCandidate(
    candidates,
    input.nodeSize,
    spatialHash,
    viewportCenter,
    visibleNeighborCenters,
  );
  if (!leastOverlapCandidate) return null;
  return {
    topLeft: leastOverlapCandidate.topLeft,
    overlapArea: leastOverlapCandidate.overlapArea,
    overlapCount: leastOverlapCandidate.overlapCount,
    mode: 'least-overlap',
  };
}
