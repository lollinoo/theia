import { computeForceLayout } from '../../hooks/useAutoLayout';

const NODE_COLUMN_GAP = 466;
const NODE_ROW_GAP = 256;
const POSITION_EPSILON = 0.5;

export interface TopologyImportLayoutPosition {
  x: number;
  y: number;
  pinned?: boolean;
}

export interface TopologyImportLayoutLink {
  id: string;
  source: string;
  target: string;
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

function compareText(left: string, right: string): number {
  return left.localeCompare(right);
}

function uniqueSorted(values: Iterable<string>): string[] {
  return [...new Set(values)].sort(compareText);
}

function canonicalLayoutLinks(
  links: TopologyImportLayoutLink[],
  deviceIds: Set<string>,
): Array<{ source: string; target: string }> {
  const unique = new Map<string, { source: string; target: string }>();

  for (const link of links) {
    if (link.source === link.target || !deviceIds.has(link.source) || !deviceIds.has(link.target)) {
      continue;
    }
    const [source, target] = [link.source, link.target].sort(compareText);
    unique.set(`${source}\u0000${target}`, { source, target });
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
