import { computeForceLayout } from '../../hooks/useAutoLayout';
import {
  scoreTopologyLayoutCandidate,
  type TopologyLayoutCandidateGeometryScore,
  type TopologyLayoutGeometryLink,
} from './topologyImportLayoutGeometry';

const NODE_COLUMN_GAP = 466;
const NODE_ROW_GAP = 256;
const POSITION_EPSILON = 0.5;
const REFINEMENT_CANDIDATE_LIMIT = 96;
const REFINEMENT_SWEEP_LIMIT = 2;

export interface TopologyImportLayoutPosition {
  x: number;
  y: number;
  pinned?: boolean;
}

export interface TopologyImportLayoutLink {
  id: string;
  source: string;
  target: string;
  waypoints?: Array<{ x: number; y: number }>;
}

export interface TopologyImportLayoutInput {
  deviceIds: string[];
  links: TopologyImportLayoutLink[];
  positions: Map<string, TopologyImportLayoutPosition>;
  importedDeviceIds: Set<string>;
  attentionDeviceIds: Set<string>;
  scope: 'preserve' | 'reorganize';
}

export interface TopologyImportLayoutResult {
  positions: Array<{
    device_id: string;
    x: number;
    y: number;
    pinned: boolean;
  }>;
  resetLinkRouteIds: string[];
  attentionBandY?: number;
}

interface OccupiedPosition {
  x: number;
  y: number;
}

interface CandidateScore extends TopologyLayoutCandidateGeometryScore {
  seedDistance: number;
  x: number;
  y: number;
}

function compareText(left: string, right: string): number {
  return left.localeCompare(right);
}

function uniqueSorted(values: Iterable<string>): string[] {
  return [...new Set(values)].sort(compareText);
}

function canonicalLayoutLinks(
  links: TopologyImportLayoutLink[],
  deviceIds: Set<string>,
): TopologyImportLayoutLink[] {
  const unique = new Map<string, TopologyImportLayoutLink>();

  for (const link of links) {
    if (link.source === link.target || !deviceIds.has(link.source) || !deviceIds.has(link.target)) {
      continue;
    }
    const [source, target] = [link.source, link.target].sort(compareText);
    const key = `${source}\u0000${target}`;
    const candidate = {
      id: link.id,
      source,
      target,
      waypoints:
        link.source === source
          ? link.waypoints
          : link.waypoints
            ? [...link.waypoints].reverse()
            : [],
    };
    const existing = unique.get(key);
    if (!existing || compareText(candidate.id, existing.id) < 0) unique.set(key, candidate);
  }

  return [...unique.values()].sort(
    (left, right) =>
      compareText(left.source, right.source) || compareText(left.target, right.target),
  );
}

function overlaps(candidate: OccupiedPosition, occupied: OccupiedPosition[]): boolean {
  return occupied.some(
    (position) =>
      Math.abs(candidate.x - position.x) < NODE_COLUMN_GAP &&
      Math.abs(candidate.y - position.y) < NODE_ROW_GAP,
  );
}

function nearestFreePosition(
  desired: OccupiedPosition,
  occupied: OccupiedPosition[],
): OccupiedPosition {
  const origin = {
    x: Math.round(desired.x / NODE_COLUMN_GAP) * NODE_COLUMN_GAP,
    y: Math.round(desired.y / NODE_ROW_GAP) * NODE_ROW_GAP,
  };
  const searchLimit = Math.max(12, occupied.length * 2 + 4);

  for (let radius = 0; radius <= searchLimit; radius += 1) {
    const candidates: OccupiedPosition[] = [];
    for (let xOffset = -radius; xOffset <= radius; xOffset += 1) {
      const yOffset = radius - Math.abs(xOffset);
      candidates.push({
        x: origin.x + xOffset * NODE_COLUMN_GAP,
        y: origin.y + yOffset * NODE_ROW_GAP,
      });
      if (yOffset !== 0) {
        candidates.push({
          x: origin.x + xOffset * NODE_COLUMN_GAP,
          y: origin.y - yOffset * NODE_ROW_GAP,
        });
      }
    }
    candidates.sort((left, right) => left.y - right.y || left.x - right.x);
    const free = candidates.find((candidate) => !overlaps(candidate, occupied));
    if (free) return free;
  }

  return {
    x: origin.x,
    y: origin.y + (searchLimit + 1) * NODE_ROW_GAP,
  };
}

function positionMoved(
  previous: TopologyImportLayoutPosition | undefined,
  next: OccupiedPosition | undefined,
): boolean {
  if (!next) return false;
  if (!previous) return true;
  return (
    Math.abs(previous.x - next.x) > POSITION_EPSILON ||
    Math.abs(previous.y - next.y) > POSITION_EPSILON
  );
}

function gridOrigin(position: OccupiedPosition): OccupiedPosition {
  return {
    x: Math.round(position.x / NODE_COLUMN_GAP) * NODE_COLUMN_GAP,
    y: Math.round(position.y / NODE_ROW_GAP) * NODE_ROW_GAP,
  };
}

function ringCandidates(origin: OccupiedPosition, radius: number): OccupiedPosition[] {
  const candidates: OccupiedPosition[] = [];
  for (let xOffset = -radius; xOffset <= radius; xOffset += 1) {
    const yOffset = radius - Math.abs(xOffset);
    candidates.push({
      x: origin.x + xOffset * NODE_COLUMN_GAP,
      y: origin.y + yOffset * NODE_ROW_GAP,
    });
    if (yOffset !== 0) {
      candidates.push({
        x: origin.x + xOffset * NODE_COLUMN_GAP,
        y: origin.y - yOffset * NODE_ROW_GAP,
      });
    }
  }
  return candidates;
}

function refinementCandidates(
  seed: OccupiedPosition,
  current: OccupiedPosition,
  occupied: OccupiedPosition[],
  minimumY?: number,
): OccupiedPosition[] {
  const origins = [gridOrigin(seed), gridOrigin(current)];
  const unique = new Map<string, OccupiedPosition>();
  const searchLimit = Math.max(12, occupied.length * 2 + 4);

  for (let radius = 0; radius <= searchLimit; radius += 1) {
    const candidates = origins
      .flatMap((origin) => ringCandidates(origin, radius))
      .sort((left, right) => left.y - right.y || left.x - right.x);
    for (const candidate of candidates) {
      const key = `${candidate.x}:${candidate.y}`;
      if (
        unique.has(key) ||
        overlaps(candidate, occupied) ||
        (minimumY !== undefined && candidate.y < minimumY)
      ) {
        continue;
      }
      unique.set(key, candidate);
    }
    if (unique.size >= REFINEMENT_CANDIDATE_LIMIT) break;
  }

  if (unique.size === 0) unique.set(`${current.x}:${current.y}`, current);
  return [...unique.values()];
}

function candidateScore(
  deviceId: string,
  candidate: OccupiedPosition,
  seed: OccupiedPosition,
  positions: ReadonlyMap<string, OccupiedPosition>,
  links: TopologyLayoutGeometryLink[],
): CandidateScore {
  return {
    ...scoreTopologyLayoutCandidate(deviceId, candidate, positions, links),
    seedDistance: (candidate.x - seed.x) ** 2 + (candidate.y - seed.y) ** 2,
    x: candidate.x,
    y: candidate.y,
  };
}

function compareCandidateScores(left: CandidateScore, right: CandidateScore): number {
  return (
    left.nodeLinkIntersections - right.nodeLinkIntersections ||
    left.linkCrossings - right.linkCrossings ||
    left.incidentLinkLength - right.incidentLinkLength ||
    left.seedDistance - right.seedDistance ||
    left.y - right.y ||
    left.x - right.x
  );
}

// Re-score only geometry affected by one moving node. This keeps dense imports bounded while
// preserving deterministic tie-breaking across repeated Bootstrap-Once runs.
function refineLinkAwarePositions(
  movingDeviceIds: string[],
  positions: Map<string, OccupiedPosition>,
  proposed: ReadonlyMap<string, OccupiedPosition>,
  links: TopologyLayoutGeometryLink[],
  minimumY?: number,
): Map<string, OccupiedPosition> {
  const refined = new Map(positions);
  const degree = new Map<string, number>();
  for (const link of links) {
    degree.set(link.source, (degree.get(link.source) ?? 0) + 1);
    degree.set(link.target, (degree.get(link.target) ?? 0) + 1);
  }
  const placementOrder = [...movingDeviceIds].sort(
    (left, right) => (degree.get(right) ?? 0) - (degree.get(left) ?? 0) || compareText(left, right),
  );

  for (let sweep = 0; sweep < REFINEMENT_SWEEP_LIMIT; sweep += 1) {
    let changed = false;
    for (const deviceId of placementOrder) {
      const current = refined.get(deviceId);
      if (!current) continue;
      const seed = proposed.get(deviceId) ?? current;
      const occupied = [...refined.entries()]
        .filter(([candidateId]) => candidateId !== deviceId)
        .map(([, position]) => position);
      let best = current;
      let bestScore = candidateScore(deviceId, current, seed, refined, links);

      for (const candidate of refinementCandidates(seed, current, occupied, minimumY)) {
        const score = candidateScore(deviceId, candidate, seed, refined, links);
        if (compareCandidateScores(score, bestScore) >= 0) continue;
        best = candidate;
        bestScore = score;
      }

      if (positionMoved(current, best)) {
        refined.set(deviceId, best);
        changed = true;
      }
    }
    if (!changed) break;
  }

  return refined;
}

/**
 * Builds a stable, collision-free Bootstrap-Once layout. Existing nodes are obstacles in
 * preserve mode, while reorganize mode lays out the complete map. Parallel links are
 * collapsed only for layout calculation; every affected persisted route is still reset.
 */
export function computeTopologyImportLayout(
  input: TopologyImportLayoutInput,
): TopologyImportLayoutResult {
  const deviceIds = uniqueSorted(input.deviceIds);
  const deviceIDSet = new Set(deviceIds);
  const movingDeviceIds = deviceIds.filter(
    (deviceId) => input.scope === 'reorganize' || input.importedDeviceIds.has(deviceId),
  );
  const movingSet = new Set(movingDeviceIds);
  const normalDeviceIds = movingDeviceIds.filter(
    (deviceId) => !input.attentionDeviceIds.has(deviceId),
  );
  const attentionDeviceIds = movingDeviceIds.filter((deviceId) =>
    input.attentionDeviceIds.has(deviceId),
  );
  const layoutLinks = canonicalLayoutLinks(input.links, deviceIDSet);
  const geometryLinks = layoutLinks.map((link) => ({
    source: link.source,
    target: link.target,
    waypoints:
      !movingSet.has(link.source) && !movingSet.has(link.target) ? link.waypoints : undefined,
  }));
  const columns = Math.max(2, Math.ceil(Math.sqrt(Math.max(1, deviceIds.length))));
  const rows = Math.max(2, Math.ceil(deviceIds.length / columns));
  const proposed = computeForceLayout(
    deviceIds.map((deviceId) => {
      const position = input.positions.get(deviceId);
      return {
        id: deviceId,
        x: position?.x,
        y: position?.y,
        pinned: !movingSet.has(deviceId),
      };
    }),
    layoutLinks,
    columns * NODE_COLUMN_GAP + NODE_COLUMN_GAP,
    rows * NODE_ROW_GAP + NODE_ROW_GAP,
  );

  const occupied: OccupiedPosition[] = [];
  for (const deviceId of deviceIds) {
    if (movingSet.has(deviceId)) continue;
    const fixed = input.positions.get(deviceId) ?? proposed.get(deviceId);
    if (fixed) occupied.push({ x: fixed.x, y: fixed.y });
  }

  const placed = new Map<string, OccupiedPosition>();
  const normalPlacementOrder = [...normalDeviceIds].sort((left, right) => {
    const leftPosition = proposed.get(left) ?? { x: 0, y: 0 };
    const rightPosition = proposed.get(right) ?? { x: 0, y: 0 };
    return (
      leftPosition.x - rightPosition.x ||
      leftPosition.y - rightPosition.y ||
      compareText(left, right)
    );
  });
  for (const deviceId of normalPlacementOrder) {
    const next = nearestFreePosition(proposed.get(deviceId) ?? { x: 0, y: 0 }, occupied);
    placed.set(deviceId, next);
    occupied.push(next);
  }

  const layoutPositions = new Map<string, OccupiedPosition>();
  for (const deviceId of deviceIds) {
    if (input.attentionDeviceIds.has(deviceId)) continue;
    const position =
      placed.get(deviceId) ?? input.positions.get(deviceId) ?? proposed.get(deviceId);
    if (position) layoutPositions.set(deviceId, { x: position.x, y: position.y });
  }
  const refinedNormalPositions = refineLinkAwarePositions(
    normalDeviceIds,
    layoutPositions,
    proposed,
    geometryLinks,
  );
  for (const deviceId of normalDeviceIds) {
    const position = refinedNormalPositions.get(deviceId);
    if (position) placed.set(deviceId, position);
  }
  occupied.length = 0;
  for (const position of refinedNormalPositions.values()) occupied.push(position);

  let attentionBandY: number | undefined;
  if (attentionDeviceIds.length > 0) {
    const occupiedBottom = occupied.reduce(
      (maximum, position) => Math.max(maximum, position.y),
      -NODE_ROW_GAP,
    );
    attentionBandY = Math.ceil((occupiedBottom + NODE_ROW_GAP) / NODE_ROW_GAP) * NODE_ROW_GAP;
    const bandColumns = Math.max(1, columns);

    attentionDeviceIds.forEach((deviceId, index) => {
      const desired = {
        x: (index % bandColumns) * NODE_COLUMN_GAP,
        y: attentionBandY! + Math.floor(index / bandColumns) * NODE_ROW_GAP,
      };
      const next = nearestFreePosition(desired, occupied);
      placed.set(deviceId, next);
      occupied.push(next);
    });
    attentionBandY = Math.min(
      ...attentionDeviceIds.map((deviceId) => placed.get(deviceId)?.y ?? attentionBandY!),
    );

    const attentionPositions = new Map(refinedNormalPositions);
    const attentionSeeds = new Map<string, OccupiedPosition>();
    for (const deviceId of attentionDeviceIds) {
      const position = placed.get(deviceId);
      if (!position) continue;
      attentionPositions.set(deviceId, position);
      attentionSeeds.set(deviceId, position);
    }
    const refinedAttentionPositions = refineLinkAwarePositions(
      attentionDeviceIds,
      attentionPositions,
      attentionSeeds,
      geometryLinks,
      attentionBandY,
    );
    for (const deviceId of attentionDeviceIds) {
      const position = refinedAttentionPositions.get(deviceId);
      if (position) placed.set(deviceId, position);
    }
    attentionBandY = Math.min(
      ...attentionDeviceIds.map((deviceId) => placed.get(deviceId)?.y ?? attentionBandY!),
    );
  }

  const movedDeviceIds = new Set(
    movingDeviceIds.filter((deviceId) =>
      positionMoved(input.positions.get(deviceId), placed.get(deviceId)),
    ),
  );
  const positions = movingDeviceIds
    .map((deviceId) => {
      const position = placed.get(deviceId);
      if (!position) return null;
      return {
        device_id: deviceId,
        x: position.x,
        y: position.y,
        pinned: input.positions.get(deviceId)?.pinned === true,
      };
    })
    .filter((position): position is NonNullable<typeof position> => position !== null)
    .sort((left, right) => compareText(left.device_id, right.device_id));
  const resetLinkRouteIds = uniqueSorted(
    input.links
      .filter((link) => movedDeviceIds.has(link.source) || movedDeviceIds.has(link.target))
      .map((link) => link.id),
  );

  return { positions, resetLinkRouteIds, attentionBandY };
}
