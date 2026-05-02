import type { Link } from '../../types/api';
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
  runtimeState: RuntimeState;
  savedPositions: Map<string, { x: number; y: number; pinned?: boolean }>;
  computedPositions: Map<string, { x: number; y: number }>;
  currentPositions: Map<string, { x: number; y: number; pinned?: boolean }>;
  defaultPosition: { x: number; y: number } | undefined;
  editMode: boolean;
  openDeviceMenu: (event: React.MouseEvent, deviceId: string) => void;
  openEdgeMenu: (event: MouseEvent | React.MouseEvent<SVGPathElement>, edgeId: string) => void;
  openSelfLinkDetails?: (link: Link) => void;
  placementDeviceIds: Set<string>;
  alerts: AlertDTO[];
}

interface ComposeCanvasTopologyResult {
  nodes: DeviceNode[];
  edges: LinkEdgeType[];
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

export function composeCanvasTopology({
  devices,
  links,
  runtimeState,
  savedPositions,
  computedPositions,
  currentPositions,
  defaultPosition,
  editMode,
  openDeviceMenu,
  openEdgeMenu,
  openSelfLinkDetails,
  placementDeviceIds,
  alerts,
}: ComposeCanvasTopologyInput): ComposeCanvasTopologyResult {
  const nodes = buildTopologyNodes(
    devices,
    savedPositions,
    computedPositions,
    defaultPosition,
    editMode,
    openDeviceMenu,
    null,
    alerts,
    links,
    openSelfLinkDetails,
    currentPositions,
    placementDeviceIds,
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
    buildRuntimeEdgeData(runtimeState),
    openEdgeMenu,
    alerts,
  );

  return { nodes, edges };
}

export type { ComposeCanvasTopologyInput, ComposeCanvasTopologyResult };
