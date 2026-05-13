import type { Device, Link } from '../../types/api';
import type { DeviceNode } from '../DeviceCard';
import type { LinkEdgeType } from '../LinkEdge';
import { resolveDeviceMonitoringState } from '../deviceVisualState';
import type { RuntimeState } from './runtimeAdapters';

export interface CanvasRenderProjectionNodeCacheEntry {
  source: DeviceNode;
  colorSignature: string;
  node: DeviceNode;
}

export interface ProjectCanvasRenderGraphInput {
  nodes: DeviceNode[];
  edges: LinkEdgeType[];
  devices: Device[];
  filteredDevices: Device[];
  filteredLinks: Link[];
  ghostDevices: Device[];
  runtimeState: RuntimeState;
  areaColorMap: ReadonlyMap<string, string>;
  effectiveAreaId: string | null;
  selectedRealNodeIds: ReadonlySet<string>;
  ghostMeasurements: ReadonlyMap<string, NonNullable<DeviceNode['measured']>>;
  areaColorNodeCache?: ReadonlyMap<string, CanvasRenderProjectionNodeCacheEntry>;
  onGhostClick: (deviceId: string) => void;
}

export interface ProjectCanvasRenderGraphResult {
  nodesWithAreaColor: DeviceNode[];
  edgesWithAreaColor: LinkEdgeType[];
  displayNodes: DeviceNode[];
  displayEdges: LinkEdgeType[];
  areaColorNodeCache: Map<string, CanvasRenderProjectionNodeCacheEntry>;
}

function projectNodesWithAreaColor(
  nodes: DeviceNode[],
  areaColorMap: ReadonlyMap<string, string>,
  previousCache: ReadonlyMap<string, CanvasRenderProjectionNodeCacheEntry> | undefined,
): Pick<ProjectCanvasRenderGraphResult, 'nodesWithAreaColor' | 'areaColorNodeCache'> {
  const nextCache = new Map<string, CanvasRenderProjectionNodeCacheEntry>();
  const nodesWithAreaColor = nodes.map((node) => {
    const colors = (node.data.device.area_ids ?? [])
      .map((id) => areaColorMap.get(id))
      .filter((color): color is string => !!color);
    const newColors = colors.length > 0 ? colors : undefined;
    const visualColor =
      node.data.device.device_type === 'virtual'
        ? (node.data.device.map_visual_color ?? undefined)
        : undefined;
    const colorSignature = `${visualColor ?? ''}\u0001${(newColors ?? []).join('\u0000')}`;
    const cached = previousCache?.get(node.id);
    if (cached?.source === node && cached.colorSignature === colorSignature) {
      nextCache.set(node.id, cached);
      return cached.node;
    }

    const previousColors = node.data.areaColors;
    const colorsEqual =
      previousColors?.length === newColors?.length &&
      (previousColors ?? []).every((color, index) => color === newColors?.[index]);
    const projectedNode =
      colorsEqual && node.data.visualColor === visualColor
        ? node
        : { ...node, data: { ...node.data, areaColors: newColors, visualColor } };
    nextCache.set(node.id, { source: node, colorSignature, node: projectedNode });
    return projectedNode;
  });

  return { nodesWithAreaColor, areaColorNodeCache: nextCache };
}

function projectEdgesWithAreaColor(
  edges: LinkEdgeType[],
  devices: Device[],
  areaColorMap: ReadonlyMap<string, string>,
): LinkEdgeType[] {
  if (areaColorMap.size === 0) {
    return edges;
  }

  const deviceAreaMap = new Map<string, string[]>();
  for (const device of devices) {
    deviceAreaMap.set(device.id, device.area_ids ?? []);
  }

  return edges.map((edge) => {
    const sourceAreas = new Set(deviceAreaMap.get(edge.source) ?? []);
    const targetAreas = deviceAreaMap.get(edge.target) ?? [];
    const sharedArea = targetAreas.find((areaId) => sourceAreas.has(areaId));
    const color = sharedArea ? areaColorMap.get(sharedArea) : undefined;
    if (color === edge.data?.areaColor) {
      return edge;
    }
    return { ...edge, data: { ...(edge.data ?? {}), areaColor: color } };
  });
}

function applyEdgeEmphasis(
  edges: LinkEdgeType[],
  selectedIds: ReadonlySet<string>,
): LinkEdgeType[] {
  if (selectedIds.size === 0) {
    let changed = false;
    const nextEdges = edges.map((edge) => {
      if (!edge.data?.emphasis || edge.data.emphasis === 'default') return edge;
      changed = true;
      return { ...edge, data: { ...edge.data, emphasis: 'default' as const } };
    });
    return changed ? nextEdges : edges;
  }

  let changed = false;
  const nextEdges = edges.map((edge) => {
    if (!edge.data) return edge;
    const emphasis =
      selectedIds.has(edge.source) || selectedIds.has(edge.target)
        ? ('connected' as const)
        : ('muted' as const);
    if (edge.data.emphasis === emphasis) return edge;
    changed = true;
    return { ...edge, data: { ...edge.data, emphasis } };
  });
  return changed ? nextEdges : edges;
}

function projectDisplayNodes({
  nodesWithAreaColor,
  filteredDevices,
  filteredLinks,
  ghostDevices,
  runtimeState,
  effectiveAreaId,
  ghostMeasurements,
  onGhostClick,
}: Pick<
  ProjectCanvasRenderGraphInput,
  | 'filteredDevices'
  | 'filteredLinks'
  | 'ghostDevices'
  | 'runtimeState'
  | 'effectiveAreaId'
  | 'ghostMeasurements'
  | 'onGhostClick'
> & {
  nodesWithAreaColor: DeviceNode[];
}): DeviceNode[] {
  if (!effectiveAreaId) {
    return nodesWithAreaColor;
  }

  const filteredDeviceIds = new Set(filteredDevices.map((device) => device.id));
  const areaNodes = nodesWithAreaColor.filter((node) => filteredDeviceIds.has(node.id));
  const nodesById = new Map(nodesWithAreaColor.map((node) => [node.id, node]));
  const areaNodesById = new Map(areaNodes.map((node) => [node.id, node]));

  const ghostNodes: DeviceNode[] = ghostDevices.map((device) => {
    const existingNode = nodesById.get(device.id);
    const connectedLink = filteredLinks.find(
      (link) => link.source_device_id === device.id || link.target_device_id === device.id,
    );
    const connectedRealDeviceId = connectedLink
      ? connectedLink.source_device_id === device.id
        ? connectedLink.target_device_id
        : connectedLink.source_device_id
      : null;
    const connectedRealNode = connectedRealDeviceId
      ? areaNodesById.get(connectedRealDeviceId)
      : undefined;
    const basePosition =
      existingNode?.position ??
      (connectedRealNode
        ? {
            x: connectedRealNode.position.x + 200,
            y: connectedRealNode.position.y,
          }
        : { x: 0, y: 0 });
    const runtimeDevice = runtimeState.devicesById.get(device.id);

    return {
      id: device.id,
      type: 'device',
      position: basePosition,
      measured: ghostMeasurements.get(device.id),
      draggable: false,
      data: {
        kind: 'ghost-device',
        device,
        runtime: existingNode?.data.runtime ?? {
          status: runtimeDevice?.device.status ?? device.status,
          metrics: runtimeDevice?.metrics ?? null,
          alertStatus: runtimeDevice?.alertStatus ?? 'normal',
          monitoringState: runtimeDevice?.monitoringState ?? resolveDeviceMonitoringState(device),
        },
        pinned: false,
        isGhost: true,
        onGhostClick,
      },
    };
  });

  return [...areaNodes, ...ghostNodes];
}

function projectDisplayEdges(
  edgesWithAreaColor: LinkEdgeType[],
  filteredLinks: Link[],
  effectiveAreaId: string | null,
  selectedRealNodeIds: ReadonlySet<string>,
): LinkEdgeType[] {
  if (!effectiveAreaId) {
    return applyEdgeEmphasis(edgesWithAreaColor, selectedRealNodeIds);
  }

  const filteredLinkIds = new Set(filteredLinks.map((link) => link.id));
  const areaEdges = edgesWithAreaColor.filter((edge) => filteredLinkIds.has(edge.id));
  return applyEdgeEmphasis(areaEdges, selectedRealNodeIds);
}

export function projectCanvasRenderGraph(
  input: ProjectCanvasRenderGraphInput,
): ProjectCanvasRenderGraphResult {
  const { nodesWithAreaColor, areaColorNodeCache } = projectNodesWithAreaColor(
    input.nodes,
    input.areaColorMap,
    input.areaColorNodeCache,
  );
  const edgesWithAreaColor = projectEdgesWithAreaColor(
    input.edges,
    input.devices,
    input.areaColorMap,
  );
  const displayNodes = projectDisplayNodes({
    nodesWithAreaColor,
    filteredDevices: input.filteredDevices,
    filteredLinks: input.filteredLinks,
    ghostDevices: input.ghostDevices,
    runtimeState: input.runtimeState,
    effectiveAreaId: input.effectiveAreaId,
    ghostMeasurements: input.ghostMeasurements,
    onGhostClick: input.onGhostClick,
  });
  const displayEdges = projectDisplayEdges(
    edgesWithAreaColor,
    input.filteredLinks,
    input.effectiveAreaId,
    input.selectedRealNodeIds,
  );

  return {
    nodesWithAreaColor,
    edgesWithAreaColor,
    displayNodes,
    displayEdges,
    areaColorNodeCache,
  };
}
