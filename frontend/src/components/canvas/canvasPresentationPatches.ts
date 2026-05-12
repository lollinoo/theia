import type { AlertDTO, AlertStatus, SnapshotPayload } from '../../types/metrics';
import { alertStatusForDevice } from '../../types/metrics';
import type { DeviceNode } from '../DeviceCard';
import type { LinkEdgeType } from '../LinkEdge';
import { alertStatusForLink } from './edgeBuilder';

export interface CanvasGraphItemIndices {
  nodeIndexById?: ReadonlyMap<string, number>;
  edgeIndexById?: ReadonlyMap<string, number>;
}

export interface CanvasGraphItemsPatch {
  nodes: DeviceNode[];
  edges: LinkEdgeType[];
}

function runtimeAlertStatusForDevice(
  deviceId: string,
  snapshot: SnapshotPayload | null,
  alerts: AlertDTO[],
): AlertStatus {
  return snapshot?.devices[deviceId]?.alert_status ?? alertStatusForDevice(deviceId, alerts);
}

export function patchEditMode(nodes: DeviceNode[], editMode: boolean): DeviceNode[] {
  let nextNodes: DeviceNode[] | null = null;

  nodes.forEach((node, index) => {
    if (node.data.editMode === editMode) {
      return;
    }

    nextNodes ??= nodes.slice();
    nextNodes[index] = {
      ...node,
      data: {
        ...node.data,
        editMode,
      },
    };
  });

  return nextNodes ?? nodes;
}

export function patchHighlightedNode(
  nodes: DeviceNode[],
  nodeIndexById: ReadonlyMap<string, number> | undefined,
  deviceId: string,
  highlighted: boolean,
): DeviceNode[] {
  const nodeIndex = nodeIndexById?.get(deviceId);
  if (nodeIndex === undefined) {
    return nodes;
  }

  const node = nodes[nodeIndex];
  if (!node || node.id !== deviceId || node.data.highlighted === highlighted) {
    return nodes;
  }

  const nextNodes = nodes.slice();
  nextNodes[nodeIndex] = {
    ...node,
    data: {
      ...node.data,
      highlighted,
    },
  };
  return nextNodes;
}

export function clearSelectedGraphItems(
  nodes: DeviceNode[],
  edges: LinkEdgeType[],
  _indices: CanvasGraphItemIndices = {},
): CanvasGraphItemsPatch {
  let nextNodes: DeviceNode[] | null = null;
  let nextEdges: LinkEdgeType[] | null = null;

  nodes.forEach((node, index) => {
    if (!node.selected) {
      return;
    }

    nextNodes ??= nodes.slice();
    nextNodes[index] = {
      ...node,
      selected: false,
    };
  });

  edges.forEach((edge, index) => {
    if (!edge.selected) {
      return;
    }

    nextEdges ??= edges.slice();
    nextEdges[index] = {
      ...edge,
      selected: false,
    };
  });

  return {
    nodes: nextNodes ?? nodes,
    edges: nextEdges ?? edges,
  };
}

export function patchAlertStatuses(
  nodes: DeviceNode[],
  edges: LinkEdgeType[],
  _indices: CanvasGraphItemIndices,
  snapshot: SnapshotPayload | null,
  alerts: AlertDTO[],
): CanvasGraphItemsPatch {
  let nextNodes: DeviceNode[] | null = null;
  let nextEdges: LinkEdgeType[] | null = null;

  nodes.forEach((node, index) => {
    const alertStatus = runtimeAlertStatusForDevice(node.id, snapshot, alerts);
    if (node.data.runtime.alertStatus === alertStatus) {
      return;
    }

    nextNodes ??= nodes.slice();
    nextNodes[index] = {
      ...node,
      data: {
        ...node.data,
        runtime: {
          ...node.data.runtime,
          alertStatus,
        },
      },
    };
  });

  edges.forEach((edge, index) => {
    const alertStatus = edge.data?.link ? alertStatusForLink(edge.data.link, alerts) : undefined;
    if (!edge.data || edge.data.alertStatus === alertStatus) {
      return;
    }

    nextEdges ??= edges.slice();
    nextEdges[index] = {
      ...edge,
      data: {
        ...edge.data,
        alertStatus,
      },
    };
  });

  return {
    nodes: nextNodes ?? nodes,
    edges: nextEdges ?? edges,
  };
}
