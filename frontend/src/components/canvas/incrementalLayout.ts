import {
  type AutoLayoutEdge,
  type AutoLayoutNode,
  computeForceLayout,
} from '../../hooks/useAutoLayout';
import type { Device, Link } from '../../types/api';

interface PositionLike {
  x: number;
  y: number;
  pinned?: boolean;
}

interface BuildIncrementalLayoutInputsParams {
  devices: Device[];
  links: Link[];
  placementDeviceIds: Set<string>;
  effectivePositions: Map<string, PositionLike>;
}

interface ComputeIncrementalLayoutPositionsParams {
  layoutNodes: AutoLayoutNode[];
  layoutEdges: AutoLayoutEdge[];
  placementDeviceIds: Set<string>;
  width: number;
  height: number;
  layoutEngine?: (
    nodes: AutoLayoutNode[],
    edges: AutoLayoutEdge[],
    width: number,
    height: number,
  ) => Map<string, { x: number; y: number }>;
}

interface IncrementalLayoutInputs {
  layoutNodes: AutoLayoutNode[];
  layoutEdges: AutoLayoutEdge[];
  impactedDeviceIds: Set<string>;
}

function hasUsablePosition(position: PositionLike | undefined): position is PositionLike {
  return position !== undefined && Number.isFinite(position.x) && Number.isFinite(position.y);
}

function buildPlacementAdjacency(links: Link[], placementDeviceIds: Set<string>) {
  const adjacency = new Map<string, Set<string>>();

  for (const deviceId of placementDeviceIds) {
    adjacency.set(deviceId, new Set());
  }

  for (const link of links) {
    if (link.source_device_id === link.target_device_id) continue;
    const sourceNeedsPlacement = placementDeviceIds.has(link.source_device_id);
    const targetNeedsPlacement = placementDeviceIds.has(link.target_device_id);

    if (!sourceNeedsPlacement || !targetNeedsPlacement) continue;

    adjacency.get(link.source_device_id)?.add(link.target_device_id);
    adjacency.get(link.target_device_id)?.add(link.source_device_id);
  }

  return adjacency;
}

function collectPlacementComponent(
  startId: string,
  adjacency: Map<string, Set<string>>,
  visited: Set<string>,
): string[] {
  const queue = [startId];
  const component: string[] = [];
  visited.add(startId);

  while (queue.length > 0) {
    const current = queue.shift()!;
    component.push(current);

    for (const neighbor of [...(adjacency.get(current) ?? new Set<string>())].sort()) {
      if (visited.has(neighbor)) continue;
      visited.add(neighbor);
      queue.push(neighbor);
    }
  }

  return component;
}

export function buildIncrementalLayoutInputs({
  devices,
  links,
  placementDeviceIds,
  effectivePositions,
}: BuildIncrementalLayoutInputsParams): IncrementalLayoutInputs {
  if (placementDeviceIds.size === 0) {
    return {
      layoutNodes: [],
      layoutEdges: [],
      impactedDeviceIds: new Set(),
    };
  }

  const deviceIds = new Set(devices.map((device) => device.id));
  const validPlacementDeviceIds = new Set(
    [...placementDeviceIds].filter((deviceId) => deviceIds.has(deviceId)),
  );
  const impactedDeviceIds = new Set<string>();
  const placementAdjacency = buildPlacementAdjacency(links, validPlacementDeviceIds);
  const visitedPlacementDeviceIds = new Set<string>();

  for (const placementDeviceId of [...validPlacementDeviceIds].sort()) {
    if (visitedPlacementDeviceIds.has(placementDeviceId)) continue;

    const component = collectPlacementComponent(
      placementDeviceId,
      placementAdjacency,
      visitedPlacementDeviceIds,
    );

    for (const deviceId of component) {
      impactedDeviceIds.add(deviceId);
    }
  }

  for (const link of links) {
    if (link.source_device_id === link.target_device_id) continue;

    const sourceNeedsPlacement = validPlacementDeviceIds.has(link.source_device_id);
    const targetNeedsPlacement = validPlacementDeviceIds.has(link.target_device_id);

    if (sourceNeedsPlacement === targetNeedsPlacement) continue;

    const anchorDeviceId = sourceNeedsPlacement ? link.target_device_id : link.source_device_id;
    const anchorPosition = effectivePositions.get(anchorDeviceId);

    if (deviceIds.has(anchorDeviceId) && hasUsablePosition(anchorPosition)) {
      impactedDeviceIds.add(anchorDeviceId);
    }
  }

  return {
    layoutNodes: devices
      .filter((device) => impactedDeviceIds.has(device.id))
      .map((device) => {
        const position = effectivePositions.get(device.id);
        const needsPlacement = validPlacementDeviceIds.has(device.id);

        return {
          id: device.id,
          x: position?.x,
          y: position?.y,
          pinned: !needsPlacement && hasUsablePosition(position),
        };
      }),
    layoutEdges: links
      .filter(
        (link) =>
          link.source_device_id !== link.target_device_id &&
          impactedDeviceIds.has(link.source_device_id) &&
          impactedDeviceIds.has(link.target_device_id),
      )
      .map((link) => ({
        source: link.source_device_id,
        target: link.target_device_id,
      })),
    impactedDeviceIds,
  };
}

export function computeIncrementalLayoutPositions({
  layoutNodes,
  layoutEdges,
  placementDeviceIds,
  width,
  height,
  layoutEngine = computeForceLayout,
}: ComputeIncrementalLayoutPositionsParams): Map<string, { x: number; y: number }> {
  if (layoutNodes.length === 0) {
    return new Map();
  }

  const layoutPositions = layoutEngine(layoutNodes, layoutEdges, width, height);
  const computedPositions = new Map<string, { x: number; y: number }>();

  for (const deviceId of placementDeviceIds) {
    const position = layoutPositions.get(deviceId);
    if (!position || !Number.isFinite(position.x) || !Number.isFinite(position.y)) continue;
    computedPositions.set(deviceId, position);
  }

  return computedPositions;
}
