/**
 * Defines canvas presentation patches behavior for the topology canvas.
 * Documents how canonical topology data is projected into the interactive view layer.
 */
import type { AlertDTO, AlertStatus, SnapshotPayload } from '../../types/metrics';
import { alertStatusForDevice } from '../../types/metrics';
import type { DeviceNode } from '../DeviceCard';
import type { LinkEdgeType } from '../LinkEdge';
import { alertStatusForLink } from './edgeBuilder';

/** Describes the canvas graph item indices contract used by the topology canvas. */
export interface CanvasGraphItemIndices {
  nodeIndexById?: ReadonlyMap<string, number>;
  edgeIndexById?: ReadonlyMap<string, number>;
}

/** Describes the canvas graph items patch contract used by the topology canvas. */
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

/** Preserves live edge selection while accepting refreshed topology and route data. */
export function mergeEdgeSelectionState(
  nextEdges: LinkEdgeType[],
  currentEdges: LinkEdgeType[],
): LinkEdgeType[] {
  const currentById = new Map(currentEdges.map((edge) => [edge.id, edge]));
  let mergedEdges: LinkEdgeType[] | null = null;
  nextEdges.forEach((edge, index) => {
    const current = currentById.get(edge.id);
    if (!current || edge.selected === current.selected) return;
    mergedEdges ??= nextEdges.slice();
    mergedEdges[index] = { ...edge, selected: current.selected };
  });
  return mergedEdges ?? nextEdges;
}

/** Patches edit mode for the topology canvas. */
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

/** Patches highlighted node for the topology canvas. */
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

/** Patches highlighted node transition for the topology canvas. */
export function patchHighlightedNodeTransition(
  nodes: DeviceNode[],
  nodeIndexById: ReadonlyMap<string, number> | undefined,
  previousDeviceId: string | null | undefined,
  deviceId: string,
): DeviceNode[] {
  let nextNodes = nodes;
  if (
    previousDeviceId !== null &&
    previousDeviceId !== undefined &&
    previousDeviceId !== deviceId
  ) {
    nextNodes = patchHighlightedNode(nextNodes, nodeIndexById, previousDeviceId, false);
  }
  return patchHighlightedNode(nextNodes, nodeIndexById, deviceId, true);
}

/** Clears selected graph items for the topology canvas. */
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

function patchAlertNodesByScan(
  nodes: DeviceNode[],
  snapshot: SnapshotPayload | null,
  alerts: AlertDTO[],
): DeviceNode[] {
  let nextNodes: DeviceNode[] | null = null;

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

  return nextNodes ?? nodes;
}

function patchAlertNodesByIndex(
  nodes: DeviceNode[],
  nodeIndexById: ReadonlyMap<string, number>,
  snapshot: SnapshotPayload | null,
  alerts: AlertDTO[],
): DeviceNode[] {
  let nextNodes: DeviceNode[] | null = null;

  for (const [deviceId, nodeIndex] of nodeIndexById) {
    const node = nodes[nodeIndex];
    if (!node || node.id !== deviceId) {
      continue;
    }

    const alertStatus = runtimeAlertStatusForDevice(deviceId, snapshot, alerts);
    if (node.data.runtime.alertStatus === alertStatus) {
      continue;
    }

    nextNodes ??= nodes.slice();
    nextNodes[nodeIndex] = {
      ...node,
      data: {
        ...node.data,
        runtime: {
          ...node.data.runtime,
          alertStatus,
        },
      },
    };
  }

  return nextNodes ?? nodes;
}

function alertStatusForEdge(edge: LinkEdgeType, alerts: AlertDTO[]): AlertStatus | undefined {
  return edge.data?.link ? alertStatusForLink(edge.data.link, alerts) : undefined;
}

function patchAlertEdgesByScan(edges: LinkEdgeType[], alerts: AlertDTO[]): LinkEdgeType[] {
  let nextEdges: LinkEdgeType[] | null = null;

  edges.forEach((edge, index) => {
    const alertStatus = alertStatusForEdge(edge, alerts);
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

  return nextEdges ?? edges;
}

function patchAlertEdgesByIndex(
  edges: LinkEdgeType[],
  edgeIndexById: ReadonlyMap<string, number>,
  alerts: AlertDTO[],
): LinkEdgeType[] {
  let nextEdges: LinkEdgeType[] | null = null;

  for (const [edgeId, edgeIndex] of edgeIndexById) {
    const edge = edges[edgeIndex];
    if (!edge || edge.id !== edgeId || !edge.data) {
      continue;
    }

    const alertStatus = alertStatusForEdge(edge, alerts);
    if (edge.data.alertStatus === alertStatus) {
      continue;
    }

    nextEdges ??= edges.slice();
    nextEdges[edgeIndex] = {
      ...edge,
      data: {
        ...edge.data,
        alertStatus,
      },
    };
  }

  return nextEdges ?? edges;
}

/** Patches alert statuses for the topology canvas. */
export function patchAlertStatuses(
  nodes: DeviceNode[],
  edges: LinkEdgeType[],
  indices: CanvasGraphItemIndices,
  snapshot: SnapshotPayload | null,
  alerts: AlertDTO[],
): CanvasGraphItemsPatch {
  return {
    nodes: indices.nodeIndexById
      ? patchAlertNodesByIndex(nodes, indices.nodeIndexById, snapshot, alerts)
      : patchAlertNodesByScan(nodes, snapshot, alerts),
    edges: indices.edgeIndexById
      ? patchAlertEdgesByIndex(edges, indices.edgeIndexById, alerts)
      : patchAlertEdgesByScan(edges, alerts),
  };
}
