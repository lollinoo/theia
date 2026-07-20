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

/** Mutable counters for deterministic placement-effort assertions and local profiling. */
export interface NewNodePlacementDiagnostics {
  /** Increments for each uncached exact collision-score evaluation. */
  exactCandidateScores: number;
}

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
  /** Optional mutable effort counters; normal placement callers can omit this. */
  diagnostics?: NewNodePlacementDiagnostics;
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
  diagnostics?: NewNodePlacementDiagnostics,
): CollisionScore {
  if (diagnostics) diagnostics.exactCandidateScores += 1;
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

function scoreCandidateWithCache(
  topLeft: ScreenPoint,
  nodeSize: ScreenSize,
  spatialHash: SpatialHash,
  gap: number,
  diagnostics?: NewNodePlacementDiagnostics,
  cache?: Map<string, CollisionScore>,
): CollisionScore {
  const cacheKey = `${topLeft.x},${topLeft.y}`;
  const cached = cache?.get(cacheKey);
  if (cached) return cached;

  const score = scoreCandidate(topLeft, nodeSize, spatialHash, gap, diagnostics);
  cache?.set(cacheKey, score);
  return score;
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

interface CollisionConstraint {
  xStart: number;
  xEnd: number;
  yStart: number;
  yEnd: number;
}

interface BoundedForbiddenInterval {
  start: number;
  end: number;
  startIncluded: boolean;
  endIncluded: boolean;
}

function collisionConstraints(
  obstacles: ScreenRect[],
  nodeSize: ScreenSize,
  gap: number,
  minimumX: number,
  maximumX: number,
  minimumY: number,
  maximumY: number,
): CollisionConstraint[] {
  const constraints: CollisionConstraint[] = [];
  for (const obstacle of obstacles) {
    const xStart = obstacle.x - gap - nodeSize.width;
    const xEnd = obstacle.x + obstacle.width + gap;
    const yStart = obstacle.y - gap - nodeSize.height;
    const yEnd = obstacle.y + obstacle.height + gap;
    if (xEnd <= minimumX || xStart >= maximumX || yEnd <= minimumY || yStart >= maximumY) {
      continue;
    }
    constraints.push({ xStart, xEnd, yStart, yEnd });
  }
  return constraints.sort((left, right) => left.yStart - right.yStart);
}

function mergeForbiddenIntervals(
  intervals: BoundedForbiddenInterval[],
): BoundedForbiddenInterval[] {
  const merged: BoundedForbiddenInterval[] = [];
  for (const interval of intervals) {
    const previous = merged[merged.length - 1];
    const overlaps =
      previous &&
      (interval.start < previous.end ||
        (interval.start === previous.end && (previous.endIncluded || interval.startIncluded)));
    if (!previous || !overlaps) {
      merged.push({ ...interval });
      continue;
    }

    if (interval.end > previous.end) {
      previous.end = interval.end;
      previous.endIncluded = interval.endIncluded;
    } else if (interval.end === previous.end) {
      previous.endIncluded = previous.endIncluded || interval.endIncluded;
    }
  }
  return merged;
}

function intervalContains(interval: BoundedForbiddenInterval, value: number): boolean {
  if (value > interval.start && value < interval.end) return true;
  if (value === interval.start) return interval.startIncluded;
  if (value === interval.end) return interval.endIncluded;
  return false;
}

function nearestFreeAxisValues(
  minimum: number,
  maximum: number,
  preferred: number,
  intervals: BoundedForbiddenInterval[],
): number[] {
  if (minimum === maximum) {
    return intervals.some((interval) => intervalContains(interval, minimum)) ? [] : [minimum];
  }

  const merged = mergeForbiddenIntervals(intervals);
  const candidates = new Set<number>();
  for (const value of [minimum, maximum, Math.min(maximum, Math.max(minimum, preferred))]) {
    if (!merged.some((interval) => intervalContains(interval, value))) {
      candidates.add(value);
    }
  }
  for (const interval of merged) {
    if (!interval.startIncluded) candidates.add(interval.start);
    if (!interval.endIncluded) candidates.add(interval.end);
  }

  let nearestDistance = Number.POSITIVE_INFINITY;
  const nearest: number[] = [];
  for (const candidate of candidates) {
    const distance = Math.abs(candidate - preferred);
    if (distance < nearestDistance) {
      nearestDistance = distance;
      nearest.length = 0;
      nearest.push(candidate);
    } else if (distance === nearestDistance) {
      nearest.push(candidate);
    }
  }
  return nearest.sort((left, right) => left - right);
}

function findBestCollisionFreeSweepCandidate(
  usableViewport: ScreenRect,
  nodeSize: ScreenSize,
  obstacles: ScreenRect[],
  gap: number,
  viewportCenter: ScreenPoint,
  visibleNeighborCenters: ScreenPoint[],
): ScreenPoint | null {
  const minimumX = usableViewport.x;
  const maximumX = usableViewport.x + usableViewport.width - nodeSize.width;
  const minimumY = usableViewport.y;
  const maximumY = usableViewport.y + usableViewport.height - nodeSize.height;
  const preferredX = viewportCenter.x - nodeSize.width / 2;
  const preferredY = viewportCenter.y - nodeSize.height / 2;
  const constraints = collisionConstraints(
    obstacles,
    nodeSize,
    gap,
    minimumX,
    maximumX,
    minimumY,
    maximumY,
  );
  const xCandidates = new Set<number>([minimumX, maximumX, preferredX]);
  for (const constraint of constraints) {
    if (constraint.xStart >= minimumX && constraint.xStart <= maximumX) {
      xCandidates.add(constraint.xStart);
    }
    if (constraint.xEnd >= minimumX && constraint.xEnd <= maximumX) {
      xCandidates.add(constraint.xEnd);
    }
  }

  let best: { topLeft: ScreenPoint; rank: number[] } | null = null;
  const sortedX = [...xCandidates].sort((left, right) => left - right);
  for (const x of sortedX) {
    const activeYIntervals: BoundedForbiddenInterval[] = [];
    for (const constraint of constraints) {
      if (x <= constraint.xStart || x >= constraint.xEnd) continue;
      const start = Math.max(minimumY, constraint.yStart);
      const end = Math.min(maximumY, constraint.yEnd);
      if (start > end) continue;
      activeYIntervals.push({
        start,
        end,
        startIncluded: constraint.yStart < minimumY,
        endIncluded: constraint.yEnd > maximumY,
      });
    }

    for (const y of nearestFreeAxisValues(minimumY, maximumY, preferredY, activeYIntervals)) {
      const topLeft = { x, y };
      const rank = collisionFreeRank(topLeft, nodeSize, viewportCenter, visibleNeighborCenters);
      if (!best || compareRank(rank, best.rank) < 0) {
        best = { topLeft, rank };
      }
    }
  }
  return best?.topLeft ?? null;
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
  candidates: Iterable<ScreenPoint>,
  nodeSize: ScreenSize,
  spatialHash: SpatialHash,
  gap: number,
  viewportCenter: ScreenPoint,
  visibleNeighborCenters: ScreenPoint[],
  diagnostics?: NewNodePlacementDiagnostics,
  scoreCache?: Map<string, CollisionScore>,
): ScreenPoint | null {
  let best: { topLeft: ScreenPoint; rank: number[] } | null = null;
  for (const candidate of candidates) {
    if (
      scoreCandidateWithCache(candidate, nodeSize, spatialHash, gap, diagnostics, scoreCache)
        .overlapCount > 0
    ) {
      continue;
    }
    const rank = collisionFreeRank(candidate, nodeSize, viewportCenter, visibleNeighborCenters);
    if (!best || compareRank(rank, best.rank) < 0) {
      best = { topLeft: candidate, rank };
    }
  }
  return best?.topLeft ?? null;
}

interface VerticalOverlapProfile {
  obstacle: ScreenRect;
  riseStart: number;
  plateauStart: number;
  plateauEnd: number;
  fallEnd: number;
}

interface VerticalDerivativeEvent {
  position: number;
  obstacleIndex: number;
  slopeMultiplier: 1 | -1;
}

interface VerticalMinimum {
  overlapArea: number;
  yCandidates: number[];
}

function axisIntersectionLength(
  position: number,
  extent: number,
  obstacleStart: number,
  obstacleExtent: number,
): number {
  return Math.max(
    0,
    Math.min(position + extent, obstacleStart + obstacleExtent) - Math.max(position, obstacleStart),
  );
}

function compensatedSum(values: number[]): number {
  let total = 0;
  let correction = 0;
  for (const value of values) {
    const correctedValue = value - correction;
    const nextTotal = total + correctedValue;
    correction = nextTotal - total - correctedValue;
    total = nextTotal;
  }
  return total;
}

function overlapAreaTolerance(left: number, right: number): number {
  // This tolerance only keeps extra sweep finalists; exact scoring decides their final order.
  return Math.max(1, Math.abs(left), Math.abs(right)) * 1e-9;
}

function verticalOverlapProfiles(
  obstacles: ScreenRect[],
  nodeHeight: number,
): {
  profiles: VerticalOverlapProfile[];
  events: VerticalDerivativeEvent[];
} {
  const profiles = obstacles.map((obstacle) => {
    const obstacleBottom = obstacle.y + obstacle.height;
    return {
      obstacle,
      riseStart: obstacle.y - nodeHeight,
      plateauStart: Math.min(obstacle.y, obstacleBottom - nodeHeight),
      plateauEnd: Math.max(obstacle.y, obstacleBottom - nodeHeight),
      fallEnd: obstacleBottom,
    };
  });
  const events = profiles.flatMap<VerticalDerivativeEvent>((profile, obstacleIndex) => [
    { position: profile.riseStart, obstacleIndex, slopeMultiplier: 1 },
    { position: profile.plateauStart, obstacleIndex, slopeMultiplier: -1 },
    { position: profile.plateauEnd, obstacleIndex, slopeMultiplier: -1 },
    { position: profile.fallEnd, obstacleIndex, slopeMultiplier: 1 },
  ]);
  events.sort(
    (left, right) =>
      left.position - right.position ||
      left.obstacleIndex - right.obstacleIndex ||
      left.slopeMultiplier - right.slopeMultiplier,
  );
  return { profiles, events };
}

function findVerticalMinimum(
  profiles: VerticalOverlapProfile[],
  events: VerticalDerivativeEvent[],
  horizontalWeights: number[],
  nodeHeight: number,
  minimumY: number,
  maximumY: number,
  preferredY: number,
): VerticalMinimum {
  const initialAreaTerms: number[] = [];
  const initialSlopeTerms: number[] = [];
  for (let obstacleIndex = 0; obstacleIndex < profiles.length; obstacleIndex += 1) {
    const weight = horizontalWeights[obstacleIndex];
    if (weight <= 0) continue;

    const profile = profiles[obstacleIndex];
    initialAreaTerms.push(
      weight *
        axisIntersectionLength(minimumY, nodeHeight, profile.obstacle.y, profile.obstacle.height),
    );
    if (minimumY >= profile.riseStart && minimumY < profile.plateauStart) {
      initialSlopeTerms.push(weight);
    } else if (minimumY >= profile.plateauEnd && minimumY < profile.fallEnd) {
      initialSlopeTerms.push(-weight);
    }
  }

  let currentY = minimumY;
  let currentArea = compensatedSum(initialAreaTerms);
  let currentSlope = compensatedSum(initialSlopeTerms);
  let minimumArea = Number.POSITIVE_INFINITY;
  const yCandidates = new Set<number>();
  const consider = (y: number, overlapArea: number): void => {
    if (!Number.isFinite(minimumArea)) {
      minimumArea = overlapArea;
      yCandidates.add(y);
      return;
    }
    const tolerance = overlapAreaTolerance(overlapArea, minimumArea);
    if (overlapArea < minimumArea - tolerance) {
      minimumArea = overlapArea;
      yCandidates.clear();
      yCandidates.add(y);
    } else if (Math.abs(overlapArea - minimumArea) <= tolerance) {
      yCandidates.add(y);
    }
  };
  const considerSegment = (endY: number): void => {
    const projectedY = Math.min(endY, Math.max(currentY, preferredY));
    consider(projectedY, currentArea + currentSlope * (projectedY - currentY));
    currentArea += currentSlope * (endY - currentY);
    currentY = endY;
    consider(currentY, currentArea);
  };

  consider(currentY, currentArea);
  let eventIndex = 0;
  while (eventIndex < events.length && events[eventIndex].position <= minimumY) {
    eventIndex += 1;
  }
  while (eventIndex < events.length) {
    const eventPosition = events[eventIndex].position;
    if (eventPosition > maximumY) break;
    considerSegment(eventPosition);

    const slopeChanges: number[] = [];
    while (eventIndex < events.length && events[eventIndex].position === eventPosition) {
      const event = events[eventIndex];
      slopeChanges.push(horizontalWeights[event.obstacleIndex] * event.slopeMultiplier);
      eventIndex += 1;
    }
    currentSlope += compensatedSum(slopeChanges);
  }
  if (currentY < maximumY) considerSegment(maximumY);

  return {
    overlapArea: minimumArea,
    yCandidates: [...yCandidates].sort((left, right) => left - right),
  };
}

function leastOverlapRank(
  topLeft: ScreenPoint,
  nodeSize: ScreenSize,
  collisionScore: CollisionScore,
  viewportCenter: ScreenPoint,
  visibleNeighborCenters: ScreenPoint[],
): number[] {
  const center = candidateCenter(topLeft, nodeSize);
  return [
    collisionScore.overlapArea,
    collisionScore.overlapCount,
    squaredDistance(center, viewportCenter),
    nearestNeighborSquaredDistance(center, visibleNeighborCenters),
    topLeft.y,
    topLeft.x,
  ];
}

function findLeastOverlapSweepCandidate(
  usableViewport: ScreenRect,
  nodeSize: ScreenSize,
  obstacles: ScreenRect[],
  spatialHash: SpatialHash,
  viewportCenter: ScreenPoint,
  visibleNeighborCenters: ScreenPoint[],
  diagnostics?: NewNodePlacementDiagnostics,
  scoreCache?: Map<string, CollisionScore>,
): ScoredCandidate | null {
  const minimumX = usableViewport.x;
  const maximumX = usableViewport.x + usableViewport.width - nodeSize.width;
  const minimumY = usableViewport.y;
  const maximumY = usableViewport.y + usableViewport.height - nodeSize.height;
  const preferredX = viewportCenter.x - nodeSize.width / 2;
  const preferredY = viewportCenter.y - nodeSize.height / 2;
  const relevantObstacles = obstacles
    .filter(
      (obstacle) =>
        obstacle.x + obstacle.width > minimumX &&
        obstacle.x - nodeSize.width < maximumX &&
        obstacle.y + obstacle.height > minimumY &&
        obstacle.y - nodeSize.height < maximumY,
    )
    .sort((left, right) =>
      compareRank(
        [left.x, left.y, left.width, left.height],
        [right.x, right.y, right.width, right.height],
      ),
    );
  if (relevantObstacles.length === 0) return null;

  const xCandidates = new Set<number>([minimumX, maximumX, preferredX]);
  for (const obstacle of relevantObstacles) {
    const obstacleRight = obstacle.x + obstacle.width;
    const criticalValues = [
      obstacle.x - nodeSize.width,
      Math.min(obstacle.x, obstacleRight - nodeSize.width),
      Math.max(obstacle.x, obstacleRight - nodeSize.width),
      obstacleRight,
    ];
    for (const value of criticalValues) {
      if (value >= minimumX && value <= maximumX) xCandidates.add(value);
    }
  }

  const { profiles, events } = verticalOverlapProfiles(relevantObstacles, nodeSize.height);
  let estimatedMinimumArea = Number.POSITIVE_INFINITY;
  let finalists: ScreenPoint[] = [];

  // Between adjacent x breakpoints every overlap term is bilinear, whose minimum
  // is on a cell boundary. Sweeping y at each x breakpoint avoids an X-by-Y product.
  for (const x of [...xCandidates].sort((left, right) => left - right)) {
    const horizontalWeights = relevantObstacles.map((obstacle) =>
      axisIntersectionLength(x, nodeSize.width, obstacle.x, obstacle.width),
    );
    const verticalMinimum = findVerticalMinimum(
      profiles,
      events,
      horizontalWeights,
      nodeSize.height,
      minimumY,
      maximumY,
      preferredY,
    );
    if (!Number.isFinite(estimatedMinimumArea)) {
      estimatedMinimumArea = verticalMinimum.overlapArea;
      finalists = verticalMinimum.yCandidates.map((y) => ({ x, y }));
      continue;
    }

    const tolerance = overlapAreaTolerance(verticalMinimum.overlapArea, estimatedMinimumArea);
    if (verticalMinimum.overlapArea < estimatedMinimumArea - tolerance) {
      estimatedMinimumArea = verticalMinimum.overlapArea;
      finalists = verticalMinimum.yCandidates.map((y) => ({ x, y }));
    } else if (Math.abs(verticalMinimum.overlapArea - estimatedMinimumArea) <= tolerance) {
      finalists.push(...verticalMinimum.yCandidates.map((y) => ({ x, y })));
    }
  }

  let best: { candidate: ScoredCandidate; rank: number[] } | null = null;
  const seen = new Set<string>();
  for (const topLeft of finalists) {
    const key = `${topLeft.x},${topLeft.y}`;
    if (seen.has(key)) continue;
    seen.add(key);
    const collisionScore = scoreCandidateWithCache(
      topLeft,
      nodeSize,
      spatialHash,
      0,
      diagnostics,
      scoreCache,
    );
    const rank = leastOverlapRank(
      topLeft,
      nodeSize,
      collisionScore,
      viewportCenter,
      visibleNeighborCenters,
    );
    if (!best || compareRank(rank, best.rank) < 0) {
      best = {
        candidate: { topLeft, ...collisionScore },
        rank,
      };
    }
  }
  return best?.candidate ?? null;
}

function findLeastOverlapCandidate(
  candidates: ScreenPoint[],
  nodeSize: ScreenSize,
  spatialHash: SpatialHash,
  viewportCenter: ScreenPoint,
  visibleNeighborCenters: ScreenPoint[],
  diagnostics?: NewNodePlacementDiagnostics,
  scoreCache?: Map<string, CollisionScore>,
): ScoredCandidate | null {
  let best: { candidate: ScoredCandidate; rank: number[] } | null = null;
  for (const topLeft of candidates) {
    const collisionScore = scoreCandidateWithCache(
      topLeft,
      nodeSize,
      spatialHash,
      0,
      diagnostics,
      scoreCache,
    );
    const rank = leastOverlapRank(
      topLeft,
      nodeSize,
      collisionScore,
      viewportCenter,
      visibleNeighborCenters,
    );
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
    input.diagnostics,
  );
  if (preferredCandidate) {
    return {
      topLeft: preferredCandidate,
      overlapArea: 0,
      overlapCount: 0,
      mode: 'preferred-gap',
    };
  }

  const preferredSweepCandidate = findBestCollisionFreeSweepCandidate(
    usableViewport,
    input.nodeSize,
    input.obstacles,
    preferredGapPx,
    viewportCenter,
    visibleNeighborCenters,
  );
  if (preferredSweepCandidate) {
    return {
      topLeft: preferredSweepCandidate,
      overlapArea: 0,
      overlapCount: 0,
      mode: 'preferred-gap',
    };
  }

  const actualScoreCache = new Map<string, CollisionScore>();
  const noGapCandidate = findBestCollisionFreeCandidate(
    candidates,
    input.nodeSize,
    spatialHash,
    0,
    viewportCenter,
    visibleNeighborCenters,
    input.diagnostics,
    actualScoreCache,
  );
  if (noGapCandidate) {
    return {
      topLeft: noGapCandidate,
      overlapArea: 0,
      overlapCount: 0,
      mode: 'no-gap',
    };
  }

  const noGapSweepCandidate = findBestCollisionFreeSweepCandidate(
    usableViewport,
    input.nodeSize,
    input.obstacles,
    0,
    viewportCenter,
    visibleNeighborCenters,
  );
  if (noGapSweepCandidate) {
    return {
      topLeft: noGapSweepCandidate,
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
    input.diagnostics,
    actualScoreCache,
  );
  const leastOverlapSweepCandidate = findLeastOverlapSweepCandidate(
    usableViewport,
    input.nodeSize,
    input.obstacles,
    spatialHash,
    viewportCenter,
    visibleNeighborCenters,
    input.diagnostics,
    actualScoreCache,
  );
  const saturatedCandidates = [leastOverlapCandidate, leastOverlapSweepCandidate].filter(
    (candidate): candidate is ScoredCandidate => candidate !== null,
  );
  let selectedCandidate: ScoredCandidate | null = null;
  let selectedRank: number[] | null = null;
  for (const candidate of saturatedCandidates) {
    const rank = leastOverlapRank(
      candidate.topLeft,
      input.nodeSize,
      candidate,
      viewportCenter,
      visibleNeighborCenters,
    );
    if (!selectedRank || compareRank(rank, selectedRank) < 0) {
      selectedCandidate = candidate;
      selectedRank = rank;
    }
  }
  if (!selectedCandidate) return null;
  return {
    topLeft: selectedCandidate.topLeft,
    overlapArea: selectedCandidate.overlapArea,
    overlapCount: selectedCandidate.overlapCount,
    mode: 'least-overlap',
  };
}
