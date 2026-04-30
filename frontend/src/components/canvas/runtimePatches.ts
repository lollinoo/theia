import type { MouseEvent as ReactMouseEvent } from 'react';

import type { Device, Link } from '../../types/api';
import type { AlertDTO, SnapshotPayload } from '../../types/metrics';
import type { DeviceNode } from '../DeviceCard';
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
    if (previousRecords[id] !== nextRecords[id]) {
      changedIds.add(id);
    }
  }
  return changedIds;
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
}: PatchRuntimeNodesInput): DeviceNode[] {
  if (plan.deviceIds.size === 0) {
    return nodes;
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

    changed = true;
    return {
      ...node,
      data: {
        ...node.data,
        device: runtimeDevice.device,
        metrics: runtimeDevice.metrics,
        alertStatus: runtimeDevice.alertStatus,
        monitoringState: runtimeDevice.monitoringState,
        isVirtual: runtimeDevice.device.device_type === 'virtual',
        subtype:
          runtimeDevice.device.device_type === 'virtual'
            ? (runtimeDevice.device.tags?.virtual_subtype ?? 'generic')
            : undefined,
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

  const nextEdges = edges.map((edge) => {
    if (!plan.edgeIds.has(edge.id)) {
      return edge;
    }

    const link = linksById.get(edge.id) ?? edge.data?.link;
    if (!link || !edge.data) {
      return edge;
    }

    const runtimeEdgeData = runtimeEdgeDataById.get(edge.id);
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
    };

    changed = true;
    return {
      ...edge,
      data: nextData,
    };
  });

  return changed ? nextEdges : edges;
}
