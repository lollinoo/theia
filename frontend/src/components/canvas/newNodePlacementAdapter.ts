import type { ReactFlowInstance, SnapGrid } from '@xyflow/react';

import type { Device, Link } from '../../types/api';
import type { DeviceNode } from '../DeviceCard';
import { type DeviceCardVariant, resolveDeviceCardVariant } from '../deviceCardVariant';
import type { LinkEdgeType } from '../LinkEdge';
import {
  findNewNodePlacement,
  NEW_NODE_PREFERRED_GAP_PX,
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

    const flowPosition = reactFlow.screenToFlowPosition(
      placement.topLeft,
      snapGrid ? { snapToGrid: true, snapGrid } : { snapToGrid: false },
    );
    if (!Number.isFinite(flowPosition.x) || !Number.isFinite(flowPosition.y)) continue;

    positions.set(target.id, flowPosition);
    placedDeviceIds.add(target.id);
    const selectedScreenTopLeft = reactFlow.flowToScreenPosition(flowPosition);
    const selectedScreenRect = {
      ...selectedScreenTopLeft,
      ...target.screenSize,
    };
    obstacles.push(selectedScreenRect);
    obstacleRectsById.set(target.id, selectedScreenRect);
  }

  return { positions, placedDeviceIds };
}
