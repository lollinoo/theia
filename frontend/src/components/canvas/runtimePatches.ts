import type { MouseEvent as ReactMouseEvent } from 'react';

import type { Device, Link } from '../../types/api';
import type { AlertDTO, DeviceMetricsDTO, SnapshotPayload } from '../../types/metrics';
import type { DeviceNode, DeviceNodeRuntimeData } from '../DeviceCard';
import type { LinkEdgeType } from '../LinkEdge';
import type { LinkEdgeData } from '../linkSemantics';
import { alertStatusForLink, buildEdgeData } from './edgeBuilder';
import type { RuntimeState } from './runtimeAdapters';

export interface RuntimePatchPlan {
  deviceIds: Set<string>;
  directLinkIds: Set<string>;
  edgeIds: Set<string>;
}

interface BuildRuntimePatchPlanInput {
  previousSnapshot: SnapshotPayload | null;
  nextSnapshot: SnapshotPayload;
  links: Link[];
}

interface PatchRuntimeNodesInput {
  nodes: DeviceNode[];
  runtimeState: RuntimeState;
  plan: RuntimePatchPlan;
  nodeIndexById?: ReadonlyMap<string, number>;
}

interface PatchRuntimeDevicesInput {
  devices: Device[];
  runtimeState: RuntimeState;
  plan: RuntimePatchPlan;
}

interface PatchRuntimeEdgesInput {
  edges: LinkEdgeType[];
  links: Link[];
  runtimeState: RuntimeState;
  alerts: AlertDTO[];
  onEdgeContextMenu?: (event: MouseEvent | ReactMouseEvent<SVGPathElement>, edgeID: string) => void;
  plan: RuntimePatchPlan;
  edgeIndexById?: ReadonlyMap<string, number>;
}

function collectChangedRuntimeIds<T>(
  previousRecords: Record<string, T> | undefined,
  nextRecords: Record<string, T>,
): Set<string> {
  if (!previousRecords) {
    return new Set(Object.keys(nextRecords));
  }

  const ids = new Set([...Object.keys(previousRecords), ...Object.keys(nextRecords)]);
  const changedIds = new Set<string>();
  for (const id of ids) {
    if (!runtimeValueEqual(previousRecords[id], nextRecords[id])) {
      changedIds.add(id);
    }
  }
  return changedIds;
}

function runtimeValueEqual(left: unknown, right: unknown): boolean {
  if (Object.is(left, right)) {
    return true;
  }
  if (left === null || right === null || left === undefined || right === undefined) {
    return false;
  }
  if (Array.isArray(left) || Array.isArray(right)) {
    if (!Array.isArray(left) || !Array.isArray(right) || left.length !== right.length) {
      return false;
    }
    return left.every((value, index) => runtimeValueEqual(value, right[index]));
  }
  if (typeof left === 'object' && typeof right === 'object') {
    const leftRecord = left as Record<string, unknown>;
    const rightRecord = right as Record<string, unknown>;
    const leftKeys = Object.keys(leftRecord);
    const rightKeys = Object.keys(rightRecord);
    if (leftKeys.length !== rightKeys.length) {
      return false;
    }
    return leftKeys.every((key) => runtimeValueEqual(leftRecord[key], rightRecord[key]));
  }
  return false;
}

function buildRuntimeEdgeData(runtimeState: RuntimeState): Map<string, LinkEdgeData> {
  const edgeDataById = new Map<string, LinkEdgeData>();

  for (const [linkId, runtimeLink] of runtimeState.linksById.entries()) {
    const sourceRuntimeDevice = runtimeState.devicesById.get(runtimeLink.link.source_device_id);
    const targetRuntimeDevice = runtimeState.devicesById.get(runtimeLink.link.target_device_id);

    edgeDataById.set(linkId, {
      sourceDeviceStatus: runtimeLink.sourceDeviceStatus,
      targetDeviceStatus: runtimeLink.targetDeviceStatus,
      sourceDeviceAlertStatus: sourceRuntimeDevice?.alertStatus,
      targetDeviceAlertStatus: targetRuntimeDevice?.alertStatus,
      sourceDeviceHealth: sourceRuntimeDevice?.metrics?.health,
      targetDeviceHealth: targetRuntimeDevice?.metrics?.health,
      sourceDevicePrimaryHealth: sourceRuntimeDevice?.primaryHealth ?? undefined,
      targetDevicePrimaryHealth: targetRuntimeDevice?.primaryHealth ?? undefined,
      sourceDeviceReachability: sourceRuntimeDevice?.metrics?.reachability,
      targetDeviceReachability: targetRuntimeDevice?.metrics?.reachability,
      sourceDeviceNetworkReachable: sourceRuntimeDevice?.metrics?.network_reachable,
      targetDeviceNetworkReachable: targetRuntimeDevice?.metrics?.network_reachable,
      sourceDeviceSnmpReachable: sourceRuntimeDevice?.metrics?.snmp_reachable,
      targetDeviceSnmpReachable: targetRuntimeDevice?.metrics?.snmp_reachable,
      metrics: runtimeLink.metrics,
      throughputLabel: runtimeLink.metricsUsable ? runtimeLink.throughputLabel : undefined,
      utilization: runtimeLink.metricsUsable ? runtimeLink.utilization : null,
    });
  }

  return edgeDataById;
}

function runtimeNodeDataChanged(
  node: DeviceNode,
  runtimeDevice: RuntimeState['devicesById'] extends Map<string, infer Model> ? Model : never,
): boolean {
  const runtimeStatus = runtimeDevice.device.status;

  return (
    node.data.runtime.status !== runtimeStatus ||
    !runtimeMetricRenderEqual(node.data.runtime.metrics, runtimeDevice.metrics) ||
    node.data.runtime.alertStatus !== runtimeDevice.alertStatus ||
    node.data.runtime.monitoringState !== runtimeDevice.monitoringState
  );
}

function buildNodeRuntimeData(
  runtimeDevice: RuntimeState['devicesById'] extends Map<string, infer Model> ? Model : never,
): DeviceNodeRuntimeData {
  return {
    status: runtimeDevice.device.status,
    metrics: runtimeDevice.metrics,
    alertStatus: runtimeDevice.alertStatus,
    monitoringState: runtimeDevice.monitoringState,
  };
}

function runtimeMetricRenderEqual(
  previous: DeviceMetricsDTO | null | undefined,
  next: DeviceMetricsDTO | null | undefined,
): boolean {
  if (previous === next) {
    return true;
  }
  if (!previous || !next) {
    return previous === next;
  }

  return (
    previous.operational_status === next.operational_status &&
    previous.primary_health === next.primary_health &&
    runtimeValueEqual(previous.runtime_flags, next.runtime_flags) &&
    runtimeValueEqual(previous.field_states, next.field_states) &&
    previous.network_reachable === next.network_reachable &&
    previous.snmp_reachable === next.snmp_reachable &&
    previous.reachability === next.reachability &&
    previous.cpu_percent === next.cpu_percent &&
    previous.mem_percent === next.mem_percent &&
    previous.uptime_secs === next.uptime_secs &&
    previous.health === next.health &&
    previous.freshness === next.freshness &&
    previous.primary_reason === next.primary_reason &&
    previous.metrics_status === next.metrics_status &&
    previous.metrics_reason === next.metrics_reason &&
    previous.alert_status === next.alert_status &&
    previous.firing_alert_count === next.firing_alert_count &&
    previous.expected_poll_interval_seconds === next.expected_poll_interval_seconds
  );
}

export function buildRuntimePatchPlan({
  previousSnapshot,
  nextSnapshot,
  links,
}: BuildRuntimePatchPlanInput): RuntimePatchPlan {
  const deviceIds = collectChangedRuntimeIds(previousSnapshot?.devices, nextSnapshot.devices);
  const directLinkIds = collectChangedRuntimeIds(previousSnapshot?.links, nextSnapshot.links);
  const edgeIds = new Set(directLinkIds);

  if (deviceIds.size > 0) {
    for (const link of links) {
      if (deviceIds.has(link.source_device_id) || deviceIds.has(link.target_device_id)) {
        edgeIds.add(link.id);
      }
    }
  }

  return { deviceIds, directLinkIds, edgeIds };
}

export function hasRuntimePatchWork(plan: RuntimePatchPlan): boolean {
  return plan.deviceIds.size > 0 || plan.directLinkIds.size > 0 || plan.edgeIds.size > 0;
}

export function patchRuntimeNodes({
  nodes,
  runtimeState,
  plan,
  nodeIndexById,
}: PatchRuntimeNodesInput): DeviceNode[] {
  if (plan.deviceIds.size === 0) {
    return nodes;
  }

  if (nodeIndexById) {
    let nextNodes: DeviceNode[] | null = null;

    for (const deviceId of plan.deviceIds) {
      const nodeIndex = nodeIndexById.get(deviceId);
      if (nodeIndex === undefined) {
        continue;
      }

      const node = nodes[nodeIndex];
      if (!node || node.id !== deviceId || node.data.kind === 'ghost-device') {
        continue;
      }

      const runtimeDevice = runtimeState.devicesById.get(deviceId);
      if (!runtimeDevice || !runtimeNodeDataChanged(node, runtimeDevice)) {
        continue;
      }

      nextNodes ??= nodes.slice();
      nextNodes[nodeIndex] = {
        ...node,
        data: {
          ...node.data,
          runtime: buildNodeRuntimeData(runtimeDevice),
        },
      };
    }

    return nextNodes ?? nodes;
  }

  let changed = false;
  const nextNodes = nodes.map((node) => {
    if (!plan.deviceIds.has(node.id) || node.data.kind === 'ghost-device') {
      return node;
    }

    const runtimeDevice = runtimeState.devicesById.get(node.id);
    if (!runtimeDevice) {
      return node;
    }

    if (!runtimeNodeDataChanged(node, runtimeDevice)) {
      return node;
    }

    changed = true;
    return {
      ...node,
      data: {
        ...node.data,
        runtime: buildNodeRuntimeData(runtimeDevice),
      },
    };
  });

  return changed ? nextNodes : nodes;
}

export function patchRuntimeDevices({
  devices,
  runtimeState,
  plan,
}: PatchRuntimeDevicesInput): Device[] {
  if (plan.deviceIds.size === 0) {
    return devices;
  }

  let changed = false;
  const nextDevices = devices.map((device) => {
    if (!plan.deviceIds.has(device.id)) {
      return device;
    }

    const runtimeDevice = runtimeState.devicesById.get(device.id)?.device;
    if (!runtimeDevice || runtimeDevice === device) {
      return device;
    }

    changed = true;
    return runtimeDevice;
  });

  return changed ? nextDevices : devices;
}

export function patchRuntimeEdges({
  edges,
  links,
  runtimeState,
  alerts,
  onEdgeContextMenu,
  plan,
  edgeIndexById,
}: PatchRuntimeEdgesInput): LinkEdgeType[] {
  if (plan.edgeIds.size === 0) {
    return edges;
  }

  const linksById = new Map(links.map((link) => [link.id, link]));
  const runtimeDevicesById = new Map(
    Array.from(runtimeState.devicesById.values()).map(
      (runtimeDevice) => [runtimeDevice.device.id, runtimeDevice.device] as const,
    ),
  );
  const runtimeEdgeDataById = buildRuntimeEdgeData(runtimeState);
  let changed = false;
  const buildPatchedEdge = (edge: LinkEdgeType, edgeId: string): LinkEdgeType | null => {
    const link = linksById.get(edgeId) ?? edge.data?.link;
    if (!link || !edge.data) {
      return null;
    }

    const runtimeEdgeData = runtimeEdgeDataById.get(edgeId);
    const nextCoreData = buildEdgeData(
      link,
      runtimeDevicesById,
      {
        ...edge.data,
        ...runtimeEdgeData,
      },
      onEdgeContextMenu ?? edge.data.onContextMenu,
    );
    const nextData: LinkEdgeData = {
      ...nextCoreData,
      alertStatus: alertStatusForLink(link, alerts),
      parallelIndex: edge.data.parallelIndex,
      areaColor: edge.data.areaColor,
      emphasis: edge.data.emphasis,
      interactionMode: edge.data.interactionMode,
    };

    return {
      ...edge,
      data: nextData,
    };
  };

  if (edgeIndexById) {
    let nextEdges: LinkEdgeType[] | null = null;

    for (const edgeId of plan.edgeIds) {
      const edgeIndex = edgeIndexById.get(edgeId);
      if (edgeIndex === undefined) {
        continue;
      }

      const edge = edges[edgeIndex];
      if (!edge || edge.id !== edgeId) {
        continue;
      }

      const patchedEdge = buildPatchedEdge(edge, edgeId);
      if (!patchedEdge) {
        continue;
      }

      nextEdges ??= edges.slice();
      nextEdges[edgeIndex] = patchedEdge;
    }

    return nextEdges ?? edges;
  }

  const nextEdges = edges.map((edge) => {
    if (!plan.edgeIds.has(edge.id)) {
      return edge;
    }

    const patchedEdge = buildPatchedEdge(edge, edge.id);
    if (!patchedEdge) {
      return edge;
    }

    changed = true;
    return patchedEdge;
  });

  return changed ? nextEdges : edges;
}
