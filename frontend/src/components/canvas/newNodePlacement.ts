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
  /** Increments for each heuristic candidate tested for any collision. */
  collisionProbeCandidates?: number;
  /** Increments for each heuristic candidate probed against the exact sweep's area bound. */
  leastOverlapAreaProbes?: number;
  /** Increments for each obstacle rectangle tested by a collision probe or exact score. */
  obstacleIntersectionTests?: number;
  /** Increments for each full constraint visit performed by a collision-free x sweep. */
  collisionSweepConstraintVisits?: number;
  /** Increments for each incremental constraint start or end event applied by an x sweep. */
  collisionSweepEventUpdates?: number;
  /** Increments for each obstacle horizontal-overlap weight evaluated by the exact sweep. */
  horizontalWeightEvaluations?: number;
  /** Increments for each horizontal-weight buffer allocated by the exact sweep. */
  horizontalWeightBufferAllocations?: number;
  /** Increments for each vertical derivative event visited by the exact sweep. */
  verticalSweepEventVisits?: number;
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
  visitedObstacleGenerations: Uint32Array;
  nextVisitGeneration: number;
}

interface CollisionScore {
  overlapArea: number;
  overlapCount: number;
}

interface ScoredCandidate extends CollisionScore {
  topLeft: ScreenPoint;
}

function incrementDiagnostic(
  diagnostics: NewNodePlacementDiagnostics | undefined,
  counter: keyof NewNodePlacementDiagnostics,
  amount = 1,
): void {
  const currentValue = diagnostics?.[counter];
  if (diagnostics && currentValue !== undefined) {
    diagnostics[counter] = currentValue + amount;
  }
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
  return {
    cellSize,
    cells,
    obstacles,
    visitedObstacleGenerations: new Uint32Array(obstacles.length),
    nextVisitGeneration: 1,
  };
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
  incrementDiagnostic(diagnostics, 'exactCandidateScores');
  const candidateRect = { ...topLeft, ...nodeSize };
  const queriedObstacleIds = queryObstacleIds(spatialHash, expandScreenRect(candidateRect, gap));
  const overlapAreas: number[] = [];

  for (const obstacleId of queriedObstacleIds) {
    incrementDiagnostic(diagnostics, 'obstacleIntersectionTests');
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

function beginSpatialHashVisit(spatialHash: SpatialHash): number {
  const visitGeneration = spatialHash.nextVisitGeneration;
  spatialHash.nextVisitGeneration += 1;
  if (spatialHash.nextVisitGeneration > 0xffff_ffff) {
    spatialHash.visitedObstacleGenerations.fill(0);
    spatialHash.nextVisitGeneration = 1;
  }
  return visitGeneration;
}

function candidateCollides(
  topLeft: ScreenPoint,
  nodeSize: ScreenSize,
  spatialHash: SpatialHash,
  gap: number,
  diagnostics?: NewNodePlacementDiagnostics,
): boolean {
  const candidateRect = { ...topLeft, ...nodeSize };
  const queryRect = expandScreenRect(candidateRect, gap);
  const minCellX = Math.floor(queryRect.x / spatialHash.cellSize);
  const maxCellX = Math.floor((queryRect.x + queryRect.width) / spatialHash.cellSize);
  const minCellY = Math.floor(queryRect.y / spatialHash.cellSize);
  const maxCellY = Math.floor((queryRect.y + queryRect.height) / spatialHash.cellSize);
  const visitGeneration = beginSpatialHashVisit(spatialHash);

  for (let cellY = minCellY; cellY <= maxCellY; cellY += 1) {
    for (let cellX = minCellX; cellX <= maxCellX; cellX += 1) {
      for (const obstacleId of spatialHash.cells.get(`${cellX},${cellY}`) ?? []) {
        if (spatialHash.visitedObstacleGenerations[obstacleId] === visitGeneration) continue;
        spatialHash.visitedObstacleGenerations[obstacleId] = visitGeneration;
        incrementDiagnostic(diagnostics, 'obstacleIntersectionTests');
        const obstacle = spatialHash.obstacles[obstacleId];
        if (
          intersectionArea(candidateRect, gap === 0 ? obstacle : expandScreenRect(obstacle, gap)) >
          0
        ) {
          return true;
        }
      }
    }
  }
  return false;
}

function scoreCandidateWithinArea(
  topLeft: ScreenPoint,
  nodeSize: ScreenSize,
  spatialHash: SpatialHash,
  maximumArea: number,
  overlapAreas: number[],
  diagnostics?: NewNodePlacementDiagnostics,
): CollisionScore | null {
  incrementDiagnostic(diagnostics, 'leastOverlapAreaProbes');
  const candidateRect = { ...topLeft, ...nodeSize };
  const minCellX = Math.floor(candidateRect.x / spatialHash.cellSize);
  const maxCellX = Math.floor((candidateRect.x + candidateRect.width) / spatialHash.cellSize);
  const minCellY = Math.floor(candidateRect.y / spatialHash.cellSize);
  const maxCellY = Math.floor((candidateRect.y + candidateRect.height) / spatialHash.cellSize);
  const visitGeneration = beginSpatialHashVisit(spatialHash);
  overlapAreas.length = 0;
  let runningArea = 0;

  for (let cellY = minCellY; cellY <= maxCellY; cellY += 1) {
    for (let cellX = minCellX; cellX <= maxCellX; cellX += 1) {
      for (const obstacleId of spatialHash.cells.get(`${cellX},${cellY}`) ?? []) {
        if (spatialHash.visitedObstacleGenerations[obstacleId] === visitGeneration) continue;
        spatialHash.visitedObstacleGenerations[obstacleId] = visitGeneration;
        incrementDiagnostic(diagnostics, 'obstacleIntersectionTests');
        const overlapArea = intersectionArea(candidateRect, spatialHash.obstacles[obstacleId]);
        if (overlapArea <= 0) continue;

        overlapAreas.push(overlapArea);
        runningArea += overlapArea;
        // All terms are non-negative. The extra tolerance absorbs accumulation-order
        // rounding before rejecting a candidate that cannot match the sweep minimum.
        if (runningArea > maximumArea + overlapAreaTolerance(runningArea, maximumArea)) return null;
      }
    }
  }

  incrementDiagnostic(diagnostics, 'exactCandidateScores');
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

interface CollisionConstraint {
  xStart: number;
  xEnd: number;
  yStart: number;
  yEnd: number;
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

function lowerBound(sortedValues: number[], target: number): number {
  let lower = 0;
  let upper = sortedValues.length;
  while (lower < upper) {
    const middle = lower + Math.floor((upper - lower) / 2);
    if (sortedValues[middle] < target) {
      lower = middle + 1;
    } else {
      upper = middle;
    }
  }
  return lower;
}

function upperBound(sortedValues: number[], target: number): number {
  let lower = 0;
  let upper = sortedValues.length;
  while (lower < upper) {
    const middle = lower + Math.floor((upper - lower) / 2);
    if (sortedValues[middle] <= target) {
      lower = middle + 1;
    } else {
      upper = middle;
    }
  }
  return lower;
}

class CoordinateCoverage {
  private readonly minimumCoverage: Int32Array;
  private readonly lazyCoverage: Int32Array;

  constructor(private readonly coordinates: number[]) {
    const treeSize = Math.max(1, coordinates.length * 4);
    this.minimumCoverage = new Int32Array(treeSize);
    this.lazyCoverage = new Int32Array(treeSize);
  }

  addOpenInterval(start: number, end: number, delta: 1 | -1): void {
    const firstCoveredIndex = upperBound(this.coordinates, start);
    const lastCoveredIndex = lowerBound(this.coordinates, end) - 1;
    if (firstCoveredIndex <= lastCoveredIndex) {
      this.addRange(1, 0, this.coordinates.length - 1, firstCoveredIndex, lastCoveredIndex, delta);
    }
  }

  nearestUncovered(preferred: number): number[] {
    if (this.minimumCoverage[1] > 0) return [];

    const preferredIndex = lowerBound(this.coordinates, preferred);
    const leftIndex = this.findLastUncovered(1, 0, this.coordinates.length - 1, preferredIndex);
    const rightIndex = this.findFirstUncovered(1, 0, this.coordinates.length - 1, preferredIndex);
    if (leftIndex < 0) return rightIndex < 0 ? [] : [this.coordinates[rightIndex]];
    if (rightIndex < 0 || leftIndex === rightIndex) return [this.coordinates[leftIndex]];

    const leftDistance = preferred - this.coordinates[leftIndex];
    const rightDistance = this.coordinates[rightIndex] - preferred;
    if (leftDistance < rightDistance) return [this.coordinates[leftIndex]];
    if (rightDistance < leftDistance) return [this.coordinates[rightIndex]];
    return [this.coordinates[leftIndex], this.coordinates[rightIndex]];
  }

  private addRange(
    node: number,
    left: number,
    right: number,
    updateLeft: number,
    updateRight: number,
    delta: 1 | -1,
  ): void {
    if (updateLeft <= left && right <= updateRight) {
      this.minimumCoverage[node] += delta;
      this.lazyCoverage[node] += delta;
      return;
    }

    this.push(node);
    const middle = left + Math.floor((right - left) / 2);
    if (updateLeft <= middle) {
      this.addRange(node * 2, left, middle, updateLeft, updateRight, delta);
    }
    if (updateRight > middle) {
      this.addRange(node * 2 + 1, middle + 1, right, updateLeft, updateRight, delta);
    }
    this.minimumCoverage[node] = Math.min(
      this.minimumCoverage[node * 2],
      this.minimumCoverage[node * 2 + 1],
    );
  }

  private findLastUncovered(
    node: number,
    left: number,
    right: number,
    maximumIndex: number,
  ): number {
    if (left > maximumIndex || this.minimumCoverage[node] > 0) return -1;
    if (left === right) return this.minimumCoverage[node] === 0 ? left : -1;

    this.push(node);
    const middle = left + Math.floor((right - left) / 2);
    const rightResult = this.findLastUncovered(node * 2 + 1, middle + 1, right, maximumIndex);
    return rightResult >= 0
      ? rightResult
      : this.findLastUncovered(node * 2, left, middle, maximumIndex);
  }

  private findFirstUncovered(
    node: number,
    left: number,
    right: number,
    minimumIndex: number,
  ): number {
    if (right < minimumIndex || this.minimumCoverage[node] > 0) return -1;
    if (left === right) return this.minimumCoverage[node] === 0 ? left : -1;

    this.push(node);
    const middle = left + Math.floor((right - left) / 2);
    const leftResult = this.findFirstUncovered(node * 2, left, middle, minimumIndex);
    return leftResult >= 0
      ? leftResult
      : this.findFirstUncovered(node * 2 + 1, middle + 1, right, minimumIndex);
  }

  private push(node: number): void {
    const delta = this.lazyCoverage[node];
    if (delta === 0) return;
    for (const child of [node * 2, node * 2 + 1]) {
      this.minimumCoverage[child] += delta;
      this.lazyCoverage[child] += delta;
    }
    this.lazyCoverage[node] = 0;
  }
}

function findBestCollisionFreeSweepCandidate(
  usableViewport: ScreenRect,
  nodeSize: ScreenSize,
  obstacles: ScreenRect[],
  gap: number,
  viewportCenter: ScreenPoint,
  visibleNeighborCenters: ScreenPoint[],
  diagnostics?: NewNodePlacementDiagnostics,
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
  const sortedYCoordinates = [
    ...new Set([
      minimumY,
      maximumY,
      preferredY,
      ...constraints.flatMap((constraint) => [
        Math.min(maximumY, Math.max(minimumY, constraint.yStart)),
        Math.min(maximumY, Math.max(minimumY, constraint.yEnd)),
      ]),
    ]),
  ].sort((left, right) => left - right);
  const yCoverage = new CoordinateCoverage(sortedYCoordinates);
  const startEvents = constraints
    .map((constraint, constraintIndex) => ({
      position: constraint.xStart,
      constraintIndex,
    }))
    .sort(
      (left, right) =>
        left.position - right.position || left.constraintIndex - right.constraintIndex,
    );
  const endEvents = constraints
    .map((constraint, constraintIndex) => ({
      position: constraint.xEnd,
      constraintIndex,
    }))
    .sort(
      (left, right) =>
        left.position - right.position || left.constraintIndex - right.constraintIndex,
    );

  let best: { topLeft: ScreenPoint; rank: number[] } | null = null;
  const sortedX = [...xCandidates].sort((left, right) => left - right);
  let startEventIndex = 0;
  let endEventIndex = 0;
  for (const x of sortedX) {
    while (startEventIndex < startEvents.length && startEvents[startEventIndex].position < x) {
      const constraint = constraints[startEvents[startEventIndex].constraintIndex];
      yCoverage.addOpenInterval(constraint.yStart, constraint.yEnd, 1);
      incrementDiagnostic(diagnostics, 'collisionSweepEventUpdates');
      startEventIndex += 1;
    }
    while (endEventIndex < endEvents.length && endEvents[endEventIndex].position <= x) {
      const constraint = constraints[endEvents[endEventIndex].constraintIndex];
      yCoverage.addOpenInterval(constraint.yStart, constraint.yEnd, -1);
      incrementDiagnostic(diagnostics, 'collisionSweepEventUpdates');
      endEventIndex += 1;
    }

    for (const y of yCoverage.nearestUncovered(preferredY)) {
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
): ScreenPoint | null {
  let best: { topLeft: ScreenPoint; rank: number[] } | null = null;
  for (const candidate of candidates) {
    incrementDiagnostic(diagnostics, 'collisionProbeCandidates');
    if (candidateCollides(candidate, nodeSize, spatialHash, gap, diagnostics)) continue;
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
  horizontalWeights: Float64Array,
  nodeHeight: number,
  minimumY: number,
  maximumY: number,
  preferredY: number,
  diagnostics?: NewNodePlacementDiagnostics,
): VerticalMinimum {
  let initialArea = 0;
  let initialAreaCorrection = 0;
  let initialSlope = 0;
  let initialSlopeCorrection = 0;
  for (let obstacleIndex = 0; obstacleIndex < profiles.length; obstacleIndex += 1) {
    const weight = horizontalWeights[obstacleIndex];
    if (weight <= 0) continue;

    const profile = profiles[obstacleIndex];
    const areaTerm =
      weight *
      axisIntersectionLength(minimumY, nodeHeight, profile.obstacle.y, profile.obstacle.height);
    const correctedAreaTerm = areaTerm - initialAreaCorrection;
    const nextInitialArea = initialArea + correctedAreaTerm;
    initialAreaCorrection = nextInitialArea - initialArea - correctedAreaTerm;
    initialArea = nextInitialArea;

    let slopeTerm = 0;
    if (minimumY >= profile.riseStart && minimumY < profile.plateauStart) {
      slopeTerm = weight;
    } else if (minimumY >= profile.plateauEnd && minimumY < profile.fallEnd) {
      slopeTerm = -weight;
    }
    if (slopeTerm !== 0) {
      const correctedSlopeTerm = slopeTerm - initialSlopeCorrection;
      const nextInitialSlope = initialSlope + correctedSlopeTerm;
      initialSlopeCorrection = nextInitialSlope - initialSlope - correctedSlopeTerm;
      initialSlope = nextInitialSlope;
    }
  }

  let currentY = minimumY;
  let currentArea = initialArea;
  let currentSlope = initialSlope;
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
    incrementDiagnostic(diagnostics, 'verticalSweepEventVisits');
    eventIndex += 1;
  }
  while (eventIndex < events.length) {
    const eventPosition = events[eventIndex].position;
    if (eventPosition > maximumY) break;
    considerSegment(eventPosition);

    let slopeChange = 0;
    let slopeChangeCorrection = 0;
    while (eventIndex < events.length && events[eventIndex].position === eventPosition) {
      incrementDiagnostic(diagnostics, 'verticalSweepEventVisits');
      const event = events[eventIndex];
      const slopeTerm = horizontalWeights[event.obstacleIndex] * event.slopeMultiplier;
      const correctedSlopeTerm = slopeTerm - slopeChangeCorrection;
      const nextSlopeChange = slopeChange + correctedSlopeTerm;
      slopeChangeCorrection = nextSlopeChange - slopeChange - correctedSlopeTerm;
      slopeChange = nextSlopeChange;
      eventIndex += 1;
    }
    currentSlope += slopeChange;
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
  heuristicCandidates: ScreenPoint[],
  spatialHash: SpatialHash,
  viewportCenter: ScreenPoint,
  visibleNeighborCenters: ScreenPoint[],
  diagnostics?: NewNodePlacementDiagnostics,
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
  const horizontalWeights = new Float64Array(relevantObstacles.length);
  incrementDiagnostic(diagnostics, 'horizontalWeightBufferAllocations');
  let estimatedMinimumArea = Number.POSITIVE_INFINITY;
  let finalists: ScreenPoint[] = [];

  // Between adjacent x breakpoints every overlap term is bilinear, whose minimum
  // is on a cell boundary. Sweeping y at each x breakpoint avoids an X-by-Y product.
  for (const x of [...xCandidates].sort((left, right) => left - right)) {
    for (let obstacleIndex = 0; obstacleIndex < relevantObstacles.length; obstacleIndex += 1) {
      incrementDiagnostic(diagnostics, 'horizontalWeightEvaluations');
      const obstacle = relevantObstacles[obstacleIndex];
      horizontalWeights[obstacleIndex] = axisIntersectionLength(
        x,
        nodeSize.width,
        obstacle.x,
        obstacle.width,
      );
    }
    const verticalMinimum = findVerticalMinimum(
      profiles,
      events,
      horizontalWeights,
      nodeSize.height,
      minimumY,
      maximumY,
      preferredY,
      diagnostics,
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
  const boundedScoreAreas: number[] = [];
  const maximumFinalistArea =
    estimatedMinimumArea + overlapAreaTolerance(estimatedMinimumArea, estimatedMinimumArea);
  for (const topLeft of heuristicCandidates) {
    const collisionScore = scoreCandidateWithinArea(
      topLeft,
      nodeSize,
      spatialHash,
      maximumFinalistArea,
      boundedScoreAreas,
      diagnostics,
    );
    if (!collisionScore) continue;

    seen.add(`${topLeft.x},${topLeft.y}`);
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
  for (const topLeft of finalists) {
    const key = `${topLeft.x},${topLeft.y}`;
    if (seen.has(key)) continue;
    seen.add(key);
    const collisionScore = scoreCandidate(topLeft, nodeSize, spatialHash, 0, diagnostics);
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
    input.diagnostics,
  );
  if (preferredSweepCandidate) {
    return {
      topLeft: preferredSweepCandidate,
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
    input.diagnostics,
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
    input.diagnostics,
  );
  if (noGapSweepCandidate) {
    return {
      topLeft: noGapSweepCandidate,
      overlapArea: 0,
      overlapCount: 0,
      mode: 'no-gap',
    };
  }

  const selectedCandidate = findLeastOverlapSweepCandidate(
    usableViewport,
    input.nodeSize,
    input.obstacles,
    candidates,
    spatialHash,
    viewportCenter,
    visibleNeighborCenters,
    input.diagnostics,
  );
  if (!selectedCandidate) return null;
  return {
    topLeft: selectedCandidate.topLeft,
    overlapArea: selectedCandidate.overlapArea,
    overlapCount: selectedCandidate.overlapCount,
    mode: 'least-overlap',
  };
}
