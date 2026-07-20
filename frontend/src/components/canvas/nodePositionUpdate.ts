/**
 * Defines node position update behavior for the topology canvas.
 * Documents how canonical topology data is projected into the interactive view layer.
 */
import type { SnapGrid } from '@xyflow/react';

import type { PositionState } from '../../hooks/usePositions';
import type { Device, Link } from '../../types/api';
import type { DeviceNode } from '../DeviceCard';
import type { LinkEdgeType } from '../LinkEdge';
import { type LinkEdgeData } from '../linkSemantics';
import { snapNodesToGrid } from './canvasGrid';
import { buildPositionPayload, isGhostDeviceNode } from './canvasHelpers';
import { buildTopologyEdges } from './edgeBuilder';
import { nodePositionsToPositionMap } from './topologyPositionState';

interface ManualNodePositionUpdateInput {
  deviceId: string;
  position: { x: number; y: number };
  nodes: DeviceNode[];
  devices: Device[];
  links: Link[];
  openEdgeMenu: (event: MouseEvent | React.MouseEvent<SVGPathElement>, edgeID: string) => void;
  snapGrid: SnapGrid | null;
}

interface ManualNodePositionUpdatePlan {
  nodes: DeviceNode[];
  positionMap: Map<string, PositionState>;
  positionPayload: ReturnType<typeof buildPositionPayload>;
  buildEdges: (currentEdges: LinkEdgeType[]) => LinkEdgeType[];
}

// buildManualNodePositionUpdate pins one node and returns the dependent position and edge updates.
export function buildManualNodePositionUpdate({
  deviceId,
  position,
  nodes,
  devices,
  links,
  openEdgeMenu,
  snapGrid,
}: ManualNodePositionUpdateInput): ManualNodePositionUpdatePlan | null {
  const positionedNodes = nodes.map((node) =>
    node.id === deviceId && !isGhostDeviceNode(node)
      ? {
          ...node,
          position,
          data: {
            ...node.data,
            pinned: true,
          },
        }
      : node,
  );
  const moved = positionedNodes.some((node, index) => node !== nodes[index]);
  if (!moved) {
    return null;
  }
  const nextNodes = snapGrid ? snapNodesToGrid(positionedNodes, snapGrid) : positionedNodes;

  const devicesById = new Map(devices.map((device) => [device.id, device]));

  return {
    nodes: nextNodes,
    positionMap: nodePositionsToPositionMap(nextNodes),
    positionPayload: buildPositionPayload(nextNodes),
    // buildEdges rebuilds affected edge geometry while preserving existing edge presentation data.
    buildEdges: (currentEdges) => {
      const existingEdgeData = new Map<string, LinkEdgeData>(
        currentEdges.map((edge) => [edge.id, edge.data ?? {}]),
      );
      return buildTopologyEdges(links, devicesById, nextNodes, existingEdgeData, openEdgeMenu);
    },
  };
}
