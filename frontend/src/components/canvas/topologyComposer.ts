/**
 * Defines topology composer behavior for the topology canvas.
 * Documents how canonical topology data is projected into the interactive view layer.
 */
import type { SnapGrid } from '@xyflow/react';

import type { Link, LinkRoute, LinkRouteMap } from '../../types/api';
import type { AlertDTO } from '../../types/metrics';
import type { DeviceNode } from '../DeviceCard';
import type { LinkEdgeType } from '../LinkEdge';
import { type LinkEdgeData } from '../linkSemantics';
import { buildTopologyEdges } from './edgeBuilder';
import { buildTopologyNodes } from './nodeBuilder';
import type { RuntimeState } from './runtimeAdapters';

interface ComposeCanvasTopologyInput {
  devices: Parameters<typeof buildTopologyNodes>[0];
  links: Link[];
  linkRoutes?: LinkRouteMap;
  onLinkRouteCommit?: (edgeId: string, route: LinkRoute | null) => void;
  runtimeState: RuntimeState;
  savedPositions: Map<string, { x: number; y: number; pinned?: boolean }>;
  computedPositions: Map<string, { x: number; y: number }>;
  currentPositions: Map<string, { x: number; y: number; pinned?: boolean }>;
  explicitPositions: Map<string, { x: number; y: number }>;
  editMode: boolean;
  openDeviceMenu: (event: React.MouseEvent, deviceId: string) => void;
  openEdgeMenu: (event: MouseEvent | React.MouseEvent<SVGPathElement>, edgeId: string) => void;
  openSelfLinkDetails?: (link: Link) => void;
  placementDeviceIds: Set<string>;
  alerts: AlertDTO[];
  snapGrid: SnapGrid | null;
}

interface ComposeCanvasTopologyResult {
  nodes: DeviceNode[];
  edges: LinkEdgeType[];
}

/**
 * Converts runtime snapshots into the edge data consumed by React Flow edges.
 * Static topology stays unchanged while status, health, and throughput remain live.
 */
function buildRuntimeEdgeData(
  runtimeState: RuntimeState,
  links: Link[],
  linkRoutes: LinkRouteMap,
  editMode: boolean,
  onLinkRouteCommit?: (edgeId: string, route: LinkRoute | null) => void,
): Map<string, LinkEdgeData> {
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

  for (const link of links) {
    edgeDataById.set(link.id, {
      ...edgeDataById.get(link.id),
      route: linkRoutes[link.id],
      routeEditable: editMode && onLinkRouteCommit !== undefined,
      onRouteCommit: onLinkRouteCommit,
    });
  }

  return edgeDataById;
}

const emptyLinkRoutes: LinkRouteMap = {};

/**
 * Builds React Flow nodes and edges from static topology plus runtime overlays.
 * The helper isolates composition ordering so the hook only applies the result.
 */
export function composeCanvasTopology({
  devices,
  links,
  linkRoutes = emptyLinkRoutes,
  onLinkRouteCommit,
  runtimeState,
  savedPositions,
  computedPositions,
  currentPositions,
  explicitPositions,
  editMode,
  openDeviceMenu,
  openEdgeMenu,
  openSelfLinkDetails,
  placementDeviceIds,
  alerts,
  snapGrid,
}: ComposeCanvasTopologyInput): ComposeCanvasTopologyResult {
  const nodes = buildTopologyNodes(
    devices,
    savedPositions,
    computedPositions,
    explicitPositions,
    editMode,
    openDeviceMenu,
    null,
    alerts,
    links,
    openSelfLinkDetails,
    currentPositions,
    placementDeviceIds,
    snapGrid,
  ).map((node) => {
    const runtimeDevice = runtimeState.devicesById.get(node.id);
    if (!runtimeDevice) {
      return node;
    }

    return {
      ...node,
      data: {
        ...node.data,
        runtime: {
          status: runtimeDevice.device.status,
          metrics: runtimeDevice.metrics,
          alertStatus: runtimeDevice.alertStatus,
          monitoringState: runtimeDevice.monitoringState,
        },
      },
    };
  });

  const runtimeDevices = devices.map(
    (device) => runtimeState.devicesById.get(device.id)?.device ?? device,
  );
  const runtimeDevicesById = new Map(runtimeDevices.map((device) => [device.id, device]));
  const edges = buildTopologyEdges(
    links,
    runtimeDevicesById,
    nodes,
    buildRuntimeEdgeData(runtimeState, links, linkRoutes, editMode, onLinkRouteCommit),
    openEdgeMenu,
    alerts,
  );

  return { nodes, edges };
}

export type { ComposeCanvasTopologyInput, ComposeCanvasTopologyResult };
