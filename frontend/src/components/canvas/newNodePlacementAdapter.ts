import type { ReactFlowInstance, SnapGrid, XYPosition } from '@xyflow/react';

import type { Device, Link } from '../../types/api';
import type { DeviceNode } from '../DeviceCard';
import { type DeviceCardVariant, resolveDeviceCardVariant } from '../deviceCardVariant';
import type { LinkEdgeType } from '../LinkEdge';
import {
  findNewNodePlacement,
  NEW_NODE_PREFERRED_GAP_PX,
  NEW_NODE_VIEWPORT_MARGIN_PX,
  type PlacementMode,
  type ScreenPoint,
  type ScreenRect,
  type ScreenSize,
} from './newNodePlacement';

/** Inputs required to adapt current React Flow geometry into explicit new-node positions. */
export interface BuildExplicitNodePlacementsInput {
  reactFlow: ReactFlowInstance<DeviceNode, LinkEdgeType>;
  canvasRect: ScreenRect;
  devices: Device[];
  links: Link[];
  deviceIds: ReadonlySet<string>;
  snapGrid?: SnapGrid | null;
}

/** Flow-coordinate positions successfully resolved for exact pending device IDs. */
export interface BuildExplicitNodePlacementsResult {
  positions: Map<string, { x: number; y: number }>;
  placedDeviceIds: Set<string>;
}

const conservativeFlowSizeByVariant: Record<DeviceCardVariant, ScreenSize> = {
  physical: { width: 370, height: 140 },
  'virtual-monitorable': { width: 430, height: 128 },
  'virtual-unmonitored': { width: 350, height: 102 },
};

interface PendingTarget {
  id: string;
  screenSize: ScreenSize;
}

function emptyResult(): BuildExplicitNodePlacementsResult {
  return {
    positions: new Map(),
    placedDeviceIds: new Set(),
  };
}

function isFinitePositive(value: number | undefined): value is number {
  return value !== undefined && Number.isFinite(value) && value > 0;
}

function isValidScreenRect(rect: ScreenRect): boolean {
  return (
    Number.isFinite(rect.x) &&
    Number.isFinite(rect.y) &&
    isFinitePositive(rect.width) &&
    isFinitePositive(rect.height)
  );
}

function conservativeFlowSize(device: Device | undefined): ScreenSize {
  if (!device) return conservativeFlowSizeByVariant.physical;
  return conservativeFlowSizeByVariant[resolveDeviceCardVariant(device)];
}

function resolveNodeFlowSize(node: DeviceNode, matchingDevice: Device | undefined): ScreenSize {
  const fallback = conservativeFlowSize(matchingDevice);
  return {
    width: isFinitePositive(node.measured?.width)
      ? node.measured.width
      : isFinitePositive(node.width)
        ? node.width
        : fallback.width,
    height: isFinitePositive(node.measured?.height)
      ? node.measured.height
      : isFinitePositive(node.height)
        ? node.height
        : fallback.height,
  };
}

function rectanglesIntersect(left: ScreenRect, right: ScreenRect): boolean {
  return (
    left.x < right.x + right.width &&
    left.x + left.width > right.x &&
    left.y < right.y + right.height &&
    left.y + left.height > right.y
  );
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

function expandScreenRect(rect: ScreenRect, gap: number): ScreenRect {
  return {
    x: rect.x - gap,
    y: rect.y - gap,
    width: rect.width + gap * 2,
    height: rect.height + gap * 2,
  };
}

function isContainedInViewport(rect: ScreenRect, viewport: ScreenRect): boolean {
  const tolerance = 1e-7;
  return (
    rect.x + tolerance >= viewport.x &&
    rect.y + tolerance >= viewport.y &&
    rect.x + rect.width <= viewport.x + viewport.width + tolerance &&
    rect.y + rect.height <= viewport.y + viewport.height + tolerance
  );
}

function squaredDistance(left: ScreenPoint, right: ScreenPoint): number {
  const deltaX = left.x - right.x;
  const deltaY = left.y - right.y;
  return deltaX * deltaX + deltaY * deltaY;
}

function compareRank(left: number[], right: number[]): number {
  for (let index = 0; index < left.length; index += 1) {
    if (left[index] < right[index]) return -1;
    if (left[index] > right[index]) return 1;
  }
  return 0;
}

interface ProjectedGridCandidate {
  flowPosition: XYPosition;
  screenRect: ScreenRect;
}

const snappedPlacementSearchRadius = 2;

function nearbyGridPositions(base: XYPosition, grid: SnapGrid): XYPosition[] {
  if (!Number.isFinite(grid[0]) || grid[0] <= 0 || !Number.isFinite(grid[1]) || grid[1] <= 0) {
    return [base];
  }

  const positions: XYPosition[] = [];
  for (let ring = 0; ring <= snappedPlacementSearchRadius; ring += 1) {
    for (let yOffset = -ring; yOffset <= ring; yOffset += 1) {
      for (let xOffset = -ring; xOffset <= ring; xOffset += 1) {
        if (Math.max(Math.abs(xOffset), Math.abs(yOffset)) !== ring) continue;
        positions.push({
          x: base.x + xOffset * grid[0],
          y: base.y + yOffset * grid[1],
        });
      }
    }
  }
  return positions;
}

function collisionRank(
  candidate: ProjectedGridCandidate,
  obstacles: ScreenRect[],
  gap: number,
): { overlapArea: number; overlapCount: number } {
  let overlapArea = 0;
  let overlapCount = 0;
  for (const obstacle of obstacles) {
    const area = intersectionArea(
      candidate.screenRect,
      gap === 0 ? obstacle : expandScreenRect(obstacle, gap),
    );
    if (area <= 0) continue;
    overlapArea += area;
    overlapCount += 1;
  }
  return { overlapArea, overlapCount };
}

function selectProjectedGridCandidate({
  reactFlow,
  baseFlowPosition,
  desiredScreenTopLeft,
  targetSize,
  canvasRect,
  obstacles,
  mode,
  snapGrid,
}: {
  reactFlow: ReactFlowInstance<DeviceNode, LinkEdgeType>;
  baseFlowPosition: XYPosition;
  desiredScreenTopLeft: ScreenPoint;
  targetSize: ScreenSize;
  canvasRect: ScreenRect;
  obstacles: ScreenRect[];
  mode: PlacementMode;
  snapGrid: SnapGrid | null;
}): ProjectedGridCandidate | null {
  const usableViewport = {
    x: canvasRect.x + NEW_NODE_VIEWPORT_MARGIN_PX,
    y: canvasRect.y + NEW_NODE_VIEWPORT_MARGIN_PX,
    width: canvasRect.width - NEW_NODE_VIEWPORT_MARGIN_PX * 2,
    height: canvasRect.height - NEW_NODE_VIEWPORT_MARGIN_PX * 2,
  };
  const flowPositions = snapGrid
    ? nearbyGridPositions(baseFlowPosition, snapGrid)
    : [baseFlowPosition];
  let best: { candidate: ProjectedGridCandidate; rank: number[] } | null = null;

  for (const [candidateIndex, flowPosition] of flowPositions.entries()) {
    if (!Number.isFinite(flowPosition.x) || !Number.isFinite(flowPosition.y)) continue;
    const screenTopLeft = reactFlow.flowToScreenPosition(flowPosition);
    const screenRect = { ...screenTopLeft, ...targetSize };
    if (!isValidScreenRect(screenRect)) continue;

    const candidate = { flowPosition, screenRect };
    const distance = squaredDistance(screenTopLeft, desiredScreenTopLeft);
    let rank: number[];
    if (mode === 'oversized') {
      if (!rectanglesIntersect(screenRect, canvasRect)) continue;
      rank = [distance, candidateIndex];
    } else {
      if (!isContainedInViewport(screenRect, usableViewport)) continue;
      const gap = mode === 'preferred-gap' ? NEW_NODE_PREFERRED_GAP_PX : 0;
      const collision = collisionRank(candidate, obstacles, gap);
      if (mode !== 'least-overlap' && collision.overlapCount > 0) continue;
      rank =
        mode === 'least-overlap'
          ? [collision.overlapArea, collision.overlapCount, distance, candidateIndex]
          : [distance, candidateIndex];
    }

    if (!best || compareRank(rank, best.rank) < 0) {
      best = { candidate, rank };
    }
  }

  return best?.candidate ?? null;
}

function expandedObstacleFilter(canvasRect: ScreenRect, largestTargetSize: ScreenSize): ScreenRect {
  const horizontalExpansion = largestTargetSize.width + NEW_NODE_PREFERRED_GAP_PX;
  const verticalExpansion = largestTargetSize.height + NEW_NODE_PREFERRED_GAP_PX;
  return {
    x: canvasRect.x - horizontalExpansion,
    y: canvasRect.y - verticalExpansion,
    width: canvasRect.width + horizontalExpansion * 2,
    height: canvasRect.height + verticalExpansion * 2,
  };
}

function visibleNeighborCenters(
  targetId: string,
  links: Link[],
  obstacleRectsById: ReadonlyMap<string, ScreenRect>,
  canvasRect: ScreenRect,
): { x: number; y: number }[] {
  const neighborIds = new Set<string>();
  for (const link of links) {
    if (link.source_device_id === targetId && link.target_device_id !== targetId) {
      neighborIds.add(link.target_device_id);
    } else if (link.target_device_id === targetId && link.source_device_id !== targetId) {
      neighborIds.add(link.source_device_id);
    }
  }

  return [...neighborIds]
    .sort((left, right) => left.localeCompare(right))
    .flatMap((neighborId) => {
      const neighborRect = obstacleRectsById.get(neighborId);
      if (!neighborRect || !rectanglesIntersect(neighborRect, canvasRect)) return [];
      return [
        {
          x: neighborRect.x + neighborRect.width / 2,
          y: neighborRect.y + neighborRect.height / 2,
        },
      ];
    });
}

/** Builds deterministic flow-coordinate positions for pending topology devices. */
export function buildExplicitNodePlacements({
  reactFlow,
  canvasRect,
  devices,
  links,
  deviceIds,
  snapGrid = null,
}: BuildExplicitNodePlacementsInput): BuildExplicitNodePlacementsResult {
  const zoom = reactFlow.getViewport().zoom;
  if (!isFinitePositive(zoom) || !isValidScreenRect(canvasRect)) return emptyResult();

  const devicesById = new Map(devices.map((device) => [device.id, device]));
  const pendingTargets: PendingTarget[] = [...deviceIds]
    .sort((left, right) => left.localeCompare(right))
    .flatMap((id) => {
      const targetDevice = devicesById.get(id);
      if (!targetDevice) return [];
      const flowSize = conservativeFlowSize(targetDevice);
      return [
        {
          id,
          screenSize: {
            width: flowSize.width * zoom,
            height: flowSize.height * zoom,
          },
        },
      ];
    });
  if (pendingTargets.length === 0) return emptyResult();

  const largestTargetSize = pendingTargets.reduce<ScreenSize>(
    (largest, target) => ({
      width: Math.max(largest.width, target.screenSize.width),
      height: Math.max(largest.height, target.screenSize.height),
    }),
    { width: 0, height: 0 },
  );
  const obstacleFilter = expandedObstacleFilter(canvasRect, largestTargetSize);
  const obstacles: ScreenRect[] = [];
  const obstacleRectsById = new Map<string, ScreenRect>();

  for (const node of reactFlow.getNodes()) {
    if (deviceIds.has(node.id) || node.hidden === true) continue;

    const flowSize = resolveNodeFlowSize(node, devicesById.get(node.id));
    const [originX, originY] = node.origin ?? [0, 0];
    const screenTopLeft = reactFlow.flowToScreenPosition({
      x: node.position.x - flowSize.width * originX,
      y: node.position.y - flowSize.height * originY,
    });
    const screenRect = {
      ...screenTopLeft,
      width: flowSize.width * zoom,
      height: flowSize.height * zoom,
    };
    if (!isValidScreenRect(screenRect) || !rectanglesIntersect(screenRect, obstacleFilter)) {
      continue;
    }
    obstacles.push(screenRect);
    obstacleRectsById.set(node.id, screenRect);
  }

  const positions = new Map<string, { x: number; y: number }>();
  const placedDeviceIds = new Set<string>();

  for (const target of pendingTargets) {
    const placement = findNewNodePlacement({
      viewport: canvasRect,
      nodeSize: target.screenSize,
      obstacles,
      visibleNeighborCenters: visibleNeighborCenters(
        target.id,
        links,
        obstacleRectsById,
        canvasRect,
      ),
    });
    if (!placement) continue;

    const baseFlowPosition = reactFlow.screenToFlowPosition(
      placement.topLeft,
      snapGrid ? { snapToGrid: true, snapGrid } : { snapToGrid: false },
    );
    if (!Number.isFinite(baseFlowPosition.x) || !Number.isFinite(baseFlowPosition.y)) continue;

    const selectedCandidate = selectProjectedGridCandidate({
      reactFlow,
      baseFlowPosition,
      desiredScreenTopLeft: placement.topLeft,
      targetSize: target.screenSize,
      canvasRect,
      obstacles,
      mode: placement.mode,
      snapGrid,
    });
    if (!selectedCandidate) continue;

    positions.set(target.id, selectedCandidate.flowPosition);
    placedDeviceIds.add(target.id);
    obstacles.push(selectedCandidate.screenRect);
    obstacleRectsById.set(target.id, selectedCandidate.screenRect);
  }

  return { positions, placedDeviceIds };
}
