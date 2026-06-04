import type { PositionState } from '../../hooks/usePositions';
import type { Device, Link } from '../../types/api';
import type { DeviceNode } from '../DeviceCard';
import type { LinkEdgeType } from '../LinkEdge';
import { type LinkEdgeData } from '../linkSemantics';
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
}

interface ManualNodePositionUpdatePlan {
  nodes: DeviceNode[];
  positionMap: Map<string, PositionState>;
  positionPayload: ReturnType<typeof buildPositionPayload>;
  buildEdges: (currentEdges: LinkEdgeType[]) => LinkEdgeType[];
}

export function buildManualNodePositionUpdate({
  deviceId,
  position,
  nodes,
  devices,
  links,
  openEdgeMenu,
}: ManualNodePositionUpdateInput): ManualNodePositionUpdatePlan | null {
  const nextNodes = nodes.map((node) =>
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
  const changed = nextNodes.some((node, index) => node !== nodes[index]);
  if (!changed) {
    return null;
  }

  const devicesById = new Map(devices.map((device) => [device.id, device]));

  return {
    nodes: nextNodes,
    positionMap: nodePositionsToPositionMap(nextNodes),
    positionPayload: buildPositionPayload(nextNodes),
    buildEdges: (currentEdges) => {
      const existingEdgeData = new Map<string, LinkEdgeData>(
        currentEdges.map((edge) => [edge.id, edge.data ?? {}]),
      );
      return buildTopologyEdges(links, devicesById, nextNodes, existingEdgeData, openEdgeMenu);
    },
  };
}
