import {
  Background,
  type Connection,
  ConnectionMode,
  type EdgeChange,
  MiniMap,
  type NodeChange,
  type OnMove,
  ReactFlow,
  SelectionMode,
  applyEdgeChanges,
  useNodesInitialized,
  useNodesState,
  useReactFlow,
  useStore,
} from '@xyflow/react';
import { memo, useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { removeDeviceFromCanvasMap } from '../api/client';
import { adaptAreaColor, useTheme } from '../contexts/ThemeContext';
import { useKeyboardShortcuts } from '../hooks/useKeyboardShortcuts';
import { useWinboxFlow } from '../hooks/useWinboxFlow';
import type { Area, CanvasMap, Device, Link } from '../types/api';
import type { AlertDTO, PrometheusStatusPayload, SnapshotPayload } from '../types/metrics';
import { ContextMenu } from './ContextMenu';
import DeviceCard, { resolveDeviceNodeReadabilityScale, type DeviceNode } from './DeviceCard';
import LinkEdge, { type LinkEdgeType } from './LinkEdge';
import { LinkLabelLayer } from './LinkLabelLayer';
import SearchOverlay from './SearchOverlay';
import { ShortcutHelp } from './ShortcutHelp';
import { SidePanel } from './SidePanel';
import { Toolbar } from './Toolbar';
import ZoomControls from './ZoomControls';
import { CanvasDiagnosticsPanel } from './canvas/CanvasDiagnosticsPanel';
import { CanvasOverlays } from './canvas/CanvasOverlays';
import { CanvasPanels } from './canvas/CanvasPanels';
import {
  recordCanvasDiagnosticEvent,
  updateCanvasDiagnosticsState,
} from './canvas/canvasDiagnostics';
import {
  buildDeviceContextMenuItems,
  isGhostDeviceNode,
  topologyFitViewPadding,
} from './canvas/canvasHelpers';
import { getCanvasDetailDeviceId } from './canvas/detailSubscription';
import { buildRuntimeState } from './canvas/runtimeAdapters';
import { useAreaFilteredTopology } from './canvas/useAreaFilteredTopology';
import { useCanvasData } from './canvas/useCanvasData';
import { useCanvasFrameMetrics } from './canvas/useCanvasFrameMetrics';
import { useCanvasMenus } from './canvas/useCanvasMenus';
import { minimapColorForDevice, resolveDeviceMonitoringState } from './deviceVisualState';
import { resolveLinkBadgeScale } from './linkSemantics';

const nodeTypes = { device: DeviceCard };
const edgeTypes = { link: LinkEdge };
const emptyAlerts: AlertDTO[] = [];
const minimapStyle = {
  backgroundColor: 'var(--nt-surface-container)',
  border: '1px solid var(--nt-outline)',
  borderRadius: 16,
  boxShadow: 'var(--nt-shadow-floating)',
};
const minimapMaskColor = 'var(--nt-minimap-mask, rgba(45, 45, 61, 0.55))';
const canvasDiagnosticsStorageKey = 'theia.canvas.diagnostics';
const canvasInteractionIdleDelayMs = 140;
const deviceNodeReadabilityScaleProperty = '--theia-device-node-readability-scale';
const linkBadgeReadabilityScaleProperty = '--theia-link-badge-readability-scale';

function topologyMinimapNodeColor(node: DeviceNode): string {
  const data = node.data;
  const device =
    data.device.status === data.runtime.status
      ? data.device
      : { ...data.device, status: data.runtime.status };
  return minimapColorForDevice({
    device,
    metrics: data.runtime.metrics,
    isGhost: isGhostDeviceNode(node),
  });
}

const TopologyMiniMap = memo(function TopologyMiniMap() {
  return (
    <MiniMap<DeviceNode>
      pannable
      zoomable
      className="!m-0 !right-4 !bottom-0"
      nodeColor={topologyMinimapNodeColor}
      style={minimapStyle}
      maskColor={minimapMaskColor}
    />
  );
});

function initialCanvasDiagnosticsVisible(): boolean {
  const queryEnabled = new URLSearchParams(window.location.search).get('canvasDiagnostics') === '1';
  const storageEnabled = window.localStorage.getItem(canvasDiagnosticsStorageKey) === 'true';
  if (queryEnabled) {
    window.localStorage.setItem(canvasDiagnosticsStorageKey, 'true');
  }
  return queryEnabled || storageEnabled;
}

function isCanvasDiagnosticsShortcut(event: KeyboardEvent): boolean {
  const isPhysicalD = event.code === 'KeyD' || event.key.toLowerCase() === 'd';
  return event.altKey && (event.ctrlKey || event.metaKey) && isPhysicalD;
}

function selectedNodeIdsFromSignature(signature: string): Set<string> {
  return new Set(signature === '' ? [] : signature.split('\u0000'));
}

function applyEdgeEmphasis(edges: LinkEdgeType[], selectedIds: Set<string>): LinkEdgeType[] {
  if (selectedIds.size === 0) {
    let changed = false;
    const nextEdges = edges.map((edge) => {
      if (!edge.data?.emphasis) return edge;
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

function setEdgeInteractionMode(
  edges: LinkEdgeType[],
  interactionMode: 'idle' | 'interactive',
): LinkEdgeType[] {
  let changed = false;
  const nextEdges = edges.map((edge) => {
    if (!edge.data) return edge;
    if ((edge.data.interactionMode ?? 'idle') === interactionMode) return edge;
    changed = true;
    return { ...edge, data: { ...edge.data, interactionMode } };
  });
  return changed ? nextEdges : edges;
}

interface CanvasProps {
  snapshot: SnapshotPayload | null;
  alerts?: AlertDTO[];
  reconnecting: boolean;
  prometheusStatus: PrometheusStatusPayload | null;
  selectedAreaId: string | null;
  mapId: string | null;
  mapName: string;
  visible?: boolean;
  fitViewRevision?: number;
  topologyRefreshRevision?: number;
  maps?: CanvasMap[];
  areas?: Area[];
  onDevicesChange?: (devices: Device[]) => void;
  onLinksChange?: (links: Link[]) => void;
  onAreaSelect?: (areaId: string | null) => void;
  onMapSelect?: (map: CanvasMap) => void;
  onManageMaps?: () => void;
  onTopologyAreasChange?: (areas: Area[]) => void;
  onTopologyLoadingChange?: (loading: boolean) => void;
  onDetailDeviceChange?: (deviceId: string | null) => void;
  onInteractionActiveChange?: (active: boolean) => void;
}

export default function Canvas({
  snapshot,
  alerts = emptyAlerts,
  reconnecting,
  prometheusStatus,
  selectedAreaId,
  mapId,
  mapName,
  visible = true,
  fitViewRevision,
  topologyRefreshRevision,
  onDevicesChange,
  onLinksChange,
  onAreaSelect,
  onTopologyAreasChange,
  onTopologyLoadingChange,
  onDetailDeviceChange,
  onInteractionActiveChange,
}: CanvasProps) {
  const [nodes, setNodes, onNodesChange] = useNodesState<DeviceNode>([]);
  const [edges, setEdges] = useState<LinkEdgeType[]>([]);
  const [selectedNodeCount, setSelectedNodeCount] = useState(0);
  const [diagnosticsVisible, setDiagnosticsVisible] = useState(initialCanvasDiagnosticsVisible);
  const [canvasInteractionActive, setCanvasInteractionActive] = useState(false);
  const canvasRootRef = useRef<HTMLDivElement | null>(null);
  const deviceNodeReadabilityScaleRef = useRef('1');
  const linkBadgeReadabilityScaleRef = useRef('1');
  const highlightTimerRef = useRef<number | null>(null);
  const interactionIdleTimerRef = useRef<number | null>(null);
  const areaColorNodeCacheRef = useRef(
    new Map<string, { source: DeviceNode; colorSignature: string; node: DeviceNode }>(),
  );
  const ghostNodeMeasurementCacheRef = useRef(
    new Map<string, NonNullable<DeviceNode['measured']>>(),
  );
  const lastProjectionDiagnosticsSignatureRef = useRef<string>('');
  const reactFlow = useReactFlow<DeviceNode, LinkEdgeType>();
  useCanvasFrameMetrics();
  const { resolvedTheme } = useTheme();
  const {
    bridgeChecked,
    bridgeRunning,
    deviceWinboxState,
    winboxError,
    openDeviceMenu: refreshWinboxFlow,
    launchWinbox,
    clearWinboxError,
    setDeviceWinboxAvailability,
  } = useWinboxFlow();
  const {
    deviceMenu,
    setDeviceMenu,
    edgeMenu,
    setEdgeMenu,
    panelContent,
    setPanelContent,
    showShortcuts,
    setShowShortcuts,
    showSearch,
    setShowSearch,
    editMode,
    setEditMode,
    shortcuts,
    getPanelTitle,
  } = useCanvasMenus({ reactFlow });
  const selectedMapKey = mapId ?? '__default__';
  const previousMapKeyRef = useRef<string | null>(null);
  const previousFitViewRevisionRef = useRef(fitViewRevision);
  const previousTopologyRefreshRevisionRef = useRef(topologyRefreshRevision);

  useEffect(() => {
    onDetailDeviceChange?.(getCanvasDetailDeviceId(panelContent));
  }, [panelContent, onDetailDeviceChange]);

  useEffect(() => () => onDetailDeviceChange?.(null), [onDetailDeviceChange]);

  useEffect(() => {
    if (previousMapKeyRef.current === null) {
      previousMapKeyRef.current = selectedMapKey;
      return;
    }
    if (previousMapKeyRef.current === selectedMapKey) {
      return;
    }

    previousMapKeyRef.current = selectedMapKey;
    setPanelContent(null);
    setDeviceMenu(null);
    setEdgeMenu(null);
    setShowSearch(false);
    setSelectedNodeCount(0);
    ghostNodeMeasurementCacheRef.current.clear();
    setNodes((currentNodes) => {
      let changed = false;
      const nextNodes = currentNodes.map((node) => {
        if (!node.selected) {
          return node;
        }
        changed = true;
        return { ...node, selected: false };
      });
      return changed ? nextNodes : currentNodes;
    });
    setEdges((currentEdges) => {
      let changed = false;
      const nextEdges = currentEdges.map((edge) => {
        if (!edge.selected) {
          return edge;
        }
        changed = true;
        return { ...edge, selected: false };
      });
      return changed ? nextEdges : currentEdges;
    });
  }, [
    selectedMapKey,
    setDeviceMenu,
    setEdgeMenu,
    setEdges,
    setNodes,
    setPanelContent,
    setShowSearch,
  ]);

  const setDeviceNodeReadabilityScale = useCallback((zoom: number) => {
    const nextScale = String(resolveDeviceNodeReadabilityScale(zoom));
    if (deviceNodeReadabilityScaleRef.current === nextScale) {
      return;
    }

    deviceNodeReadabilityScaleRef.current = nextScale;
    canvasRootRef.current?.style.setProperty(deviceNodeReadabilityScaleProperty, nextScale);
  }, []);

  const setLinkBadgeReadabilityScale = useCallback((zoom: number) => {
    const nextScale = String(resolveLinkBadgeScale(zoom));
    if (linkBadgeReadabilityScaleRef.current === nextScale) {
      return;
    }

    linkBadgeReadabilityScaleRef.current = nextScale;
    canvasRootRef.current?.style.setProperty(linkBadgeReadabilityScaleProperty, nextScale);
  }, []);

  useEffect(() => {
    canvasRootRef.current?.style.setProperty(
      deviceNodeReadabilityScaleProperty,
      deviceNodeReadabilityScaleRef.current,
    );
    canvasRootRef.current?.style.setProperty(
      linkBadgeReadabilityScaleProperty,
      linkBadgeReadabilityScaleRef.current,
    );
  }, []);

  useEffect(() => {
    onInteractionActiveChange?.(canvasInteractionActive);
  }, [canvasInteractionActive, onInteractionActiveChange]);

  useEffect(() => () => onInteractionActiveChange?.(false), [onInteractionActiveChange]);

  const beginCanvasInteraction = useCallback(() => {
    if (interactionIdleTimerRef.current !== null) {
      window.clearTimeout(interactionIdleTimerRef.current);
      interactionIdleTimerRef.current = null;
    }
    setCanvasInteractionActive(true);
  }, []);

  const endCanvasInteraction = useCallback(() => {
    if (interactionIdleTimerRef.current !== null) {
      window.clearTimeout(interactionIdleTimerRef.current);
    }
    interactionIdleTimerRef.current = window.setTimeout(() => {
      interactionIdleTimerRef.current = null;
      setCanvasInteractionActive(false);
    }, canvasInteractionIdleDelayMs);
  }, []);

  const handleCanvasMove = useCallback<OnMove>(
    (_event, viewport) => {
      setDeviceNodeReadabilityScale(viewport.zoom);
      setLinkBadgeReadabilityScale(viewport.zoom);
    },
    [setDeviceNodeReadabilityScale, setLinkBadgeReadabilityScale],
  );

  const handleSelectionChange = useCallback(
    ({ nodes: selectedNodes }: { nodes: DeviceNode[] }) => {
      setSelectedNodeCount(selectedNodes.length);
      if (selectedNodes.length > 1 && editMode) {
        setPanelContent({
          type: 'bulkEdit',
          data: { deviceIds: selectedNodes.map((n) => n.id) },
        });
      }
    },
    [editMode, setPanelContent],
  );

  useKeyboardShortcuts(shortcuts);

  useEffect(() => {
    const handleDiagnosticsShortcut = (event: KeyboardEvent) => {
      if (!isCanvasDiagnosticsShortcut(event)) {
        return;
      }

      event.preventDefault();
      setDiagnosticsVisible((current) => {
        const next = !current;
        window.localStorage.setItem(canvasDiagnosticsStorageKey, String(next));
        return next;
      });
    };

    window.addEventListener('keydown', handleDiagnosticsShortcut, true);
    return () => {
      window.removeEventListener('keydown', handleDiagnosticsShortcut, true);
    };
  }, []);

  const openEdgeMenu = useCallback(
    (event: MouseEvent | React.MouseEvent<SVGPathElement>, edgeID: string) => {
      // Bridge health is only checked for device menus because WinBox launch
      // is not available from edge menus.
      setEdgeMenu({ edgeID, x: event.clientX, y: event.clientY });
    },
    [setEdgeMenu],
  );
  const openSelfLinkDetails = useCallback(
    (link: Link) => {
      setPanelContent({ type: 'link-details', data: { link } });
    },
    [setPanelContent],
  );
  const openDeviceMenu = useCallback(
    (event: React.MouseEvent, deviceId: string) => {
      refreshWinboxFlow(deviceId);
      setDeviceMenu({ deviceId, x: event.clientX, y: event.clientY });
    },
    [refreshWinboxFlow, setDeviceMenu],
  );

  const {
    devices,
    setDevices,
    topologyLinks,
    topologyAreas = [],
    loading,
    error,
    renderedMapKey,
    loadTopology,
    runtimeSummary,
    grafanaUrlRef,
    deviceGrafanaUrlsRef,
    refreshSettings,
    topologyRecoveryNotice,
    dismissTopologyRecoveryNotice,
    retryTopologyRefresh,
    updateNodePosition,
  } = useCanvasData({
    mapId,
    mapName,
    snapshot,
    alerts,
    reconnecting,
    prometheusStatus,
    editMode,
    openDeviceMenu,
    openEdgeMenu,
    openSelfLinkDetails,
    reactFlow,
    nodes,
    setNodes,
    setEdges,
    onDevicesChange,
    onLinksChange,
    onTopologyAreasChange,
  });

  useEffect(() => {
    onTopologyLoadingChange?.(loading);
  }, [loading, onTopologyLoadingChange]);

  useEffect(() => () => onTopologyLoadingChange?.(false), [onTopologyLoadingChange]);

  useEffect(() => {
    if (topologyRefreshRevision === undefined) {
      return;
    }
    if (previousTopologyRefreshRevisionRef.current === topologyRefreshRevision) {
      return;
    }

    previousTopologyRefreshRevisionRef.current = topologyRefreshRevision;
    void loadTopology(true);
  }, [loadTopology, topologyRefreshRevision]);

  const handleRemoveDeviceFromMap = useCallback(
    async (deviceId: string) => {
      if (!mapId) {
        return;
      }
      await removeDeviceFromCanvasMap(mapId, deviceId);
      setPanelContent(null);
      await loadTopology(true);
    },
    [loadTopology, mapId, setPanelContent],
  );

  const effectiveAreaId = selectedAreaId;
  const selectedTopologyMapKey = mapId === null ? 'default:' : `map:${mapId}`;
  const nodesInitialized = useNodesInitialized();
  const flowViewportReady = useStore((state) => state.width > 0 && state.height > 0);

  // Area filtering: derive filtered devices/links and ghost devices
  const { filteredDevices, filteredLinks, ghostDevices } = useAreaFilteredTopology(
    devices,
    topologyLinks,
    effectiveAreaId,
  );
  const runtimeState = useMemo(
    () =>
      buildRuntimeState({
        devices,
        links: topologyLinks,
        snapshot,
        alerts,
        prometheusStatus,
      }),
    [devices, topologyLinks, snapshot, alerts, prometheusStatus],
  );

  // Build a lookup for area colors (adapted for current theme)
  const areaColorMap = useMemo(() => {
    const map = new Map<string, string>();
    for (const area of topologyAreas) {
      map.set(area.id, adaptAreaColor(area.color, resolvedTheme));
    }
    return map;
  }, [topologyAreas, resolvedTheme]);

  // Inject map-local visual colors and area colors into node data.
  const nodesWithAreaColor = useMemo(() => {
    const previousCache = areaColorNodeCacheRef.current;

    const nextCache = new Map<
      string,
      { source: DeviceNode; colorSignature: string; node: DeviceNode }
    >();
    const nextNodes = nodes.map((n) => {
      const colors = (n.data.device.area_ids ?? [])
        .map((id) => areaColorMap.get(id))
        .filter((c): c is string => !!c);
      const newColors = colors.length > 0 ? colors : undefined;
      const visualColor =
        n.data.device.device_type === 'virtual'
          ? (n.data.device.map_visual_color ?? undefined)
          : undefined;
      const colorSignature = `${visualColor ?? ''}\u0001${(newColors ?? []).join('\u0000')}`;
      const cached = previousCache.get(n.id);
      if (cached?.source === n && cached.colorSignature === colorSignature) {
        nextCache.set(n.id, cached);
        return cached.node;
      }

      const prev = n.data.areaColors;
      const colorsEqual =
        prev?.length === newColors?.length && (prev ?? []).every((c, i) => c === newColors?.[i]);
      const node =
        colorsEqual && n.data.visualColor === visualColor
          ? n
          : { ...n, data: { ...n.data, areaColors: newColors, visualColor } };
      nextCache.set(n.id, { source: n, colorSignature, node });
      return node;
    });
    areaColorNodeCacheRef.current = nextCache;
    return nextNodes;
  }, [nodes, areaColorMap]);

  // Inject areaColor into edge data when both endpoints share at least one area
  const edgesWithAreaColor = useMemo(() => {
    if (areaColorMap.size === 0) return edges;
    const deviceAreaMap = new Map<string, string[]>();
    for (const device of devices) {
      deviceAreaMap.set(device.id, device.area_ids ?? []);
    }
    return edges.map((e) => {
      const srcAreas = new Set(deviceAreaMap.get(e.source) ?? []);
      const tgtAreas = deviceAreaMap.get(e.target) ?? [];
      const sharedArea = tgtAreas.find((a) => srcAreas.has(a));
      const color = sharedArea ? areaColorMap.get(sharedArea) : undefined;
      if (color === e.data?.areaColor) return e;
      return { ...e, data: { ...e.data!, areaColor: color } };
    });
  }, [edges, areaColorMap, devices]);

  const selectedRealNodeIdSignature = useMemo(
    () =>
      nodes
        .filter((node) => node.selected && !isGhostDeviceNode(node))
        .map((node) => node.id)
        .sort()
        .join('\u0000'),
    [nodes],
  );
  const ghostDeviceIds = useMemo(
    () => new Set(ghostDevices.map((device) => device.id)),
    [ghostDevices],
  );
  const handleNodesChange = useCallback(
    (changes: NodeChange<DeviceNode>[]) => {
      if (!effectiveAreaId || ghostDeviceIds.size === 0) {
        onNodesChange(changes);
        return;
      }

      const canonicalChanges: NodeChange<DeviceNode>[] = [];
      for (const change of changes) {
        if (change.type === 'add') {
          canonicalChanges.push(change);
          continue;
        }

        if (!ghostDeviceIds.has(change.id)) {
          canonicalChanges.push(change);
          continue;
        }

        if (change.type === 'dimensions' && change.dimensions) {
          ghostNodeMeasurementCacheRef.current.set(change.id, {
            width: change.dimensions.width,
            height: change.dimensions.height,
          });
        }
      }

      if (canonicalChanges.length > 0) {
        onNodesChange(canonicalChanges);
      }
    },
    [effectiveAreaId, ghostDeviceIds, onNodesChange],
  );

  // Build display nodes/edges by filtering the full node/edge set and adding ghost nodes
  const displayNodes = useMemo(() => {
    if (!effectiveAreaId) {
      return nodesWithAreaColor;
    }

    const filteredDeviceIds = new Set(filteredDevices.map((d) => d.id));
    // Keep existing nodes that are in the filtered area
    const areaNodes = nodesWithAreaColor.filter((n) => filteredDeviceIds.has(n.id));

    // Create ghost nodes for cross-area link endpoints
    const ghostNodes: DeviceNode[] = ghostDevices.map((device) => {
      const existingNode = nodesWithAreaColor.find((n) => n.id === device.id);

      // Position ghost near its connected real node (offset by 200px per RESEARCH recommendation)
      const connectedLink = filteredLinks.find(
        (l) => l.source_device_id === device.id || l.target_device_id === device.id,
      );
      const connectedRealDeviceId = connectedLink
        ? connectedLink.source_device_id === device.id
          ? connectedLink.target_device_id
          : connectedLink.source_device_id
        : null;
      const connectedRealNode = connectedRealDeviceId
        ? areaNodes.find((n) => n.id === connectedRealDeviceId)
        : null;

      const basePos =
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
        position: basePos,
        measured: ghostNodeMeasurementCacheRef.current.get(device.id),
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
          onGhostClick: (deviceId: string) => {
            const clickedDevice = devices.find((d) => d.id === deviceId);
            if (clickedDevice?.area_ids?.[0] && onAreaSelect) {
              onAreaSelect(clickedDevice.area_ids[0]);
            }
          },
        },
      };
    });

    return [...areaNodes, ...ghostNodes];
  }, [
    effectiveAreaId,
    nodesWithAreaColor,
    filteredDevices,
    filteredLinks,
    ghostDevices,
    devices,
    runtimeState,
    onAreaSelect,
  ]);

  const displayEdges = useMemo(() => {
    const selectedIds = selectedNodeIdsFromSignature(selectedRealNodeIdSignature);
    if (!effectiveAreaId) {
      return applyEdgeEmphasis(edgesWithAreaColor, selectedIds);
    }
    // Filter edges to only include filtered links
    const filteredLinkIds = new Set(filteredLinks.map((l) => l.id));
    const areaEdges = edgesWithAreaColor.filter((e) => filteredLinkIds.has(e.id));
    return applyEdgeEmphasis(areaEdges, selectedIds);
  }, [effectiveAreaId, edgesWithAreaColor, filteredLinks, selectedRealNodeIdSignature]);

  const renderEdges = useMemo(
    () => setEdgeInteractionMode(displayEdges, canvasInteractionActive ? 'interactive' : 'idle'),
    [displayEdges, canvasInteractionActive],
  );

  useEffect(() => {
    const selectedEdgeCount = displayEdges.filter((edge) => edge.selected).length;
    updateCanvasDiagnosticsState({
      graph: {
        canonicalNodeCount: nodes.length,
        canonicalEdgeCount: edges.length,
        displayedNodeCount: displayNodes.length,
        displayedEdgeCount: displayEdges.length,
        ghostNodeCount: ghostDevices.length,
        selectedAreaId: effectiveAreaId,
        selectedNodeCount,
        selectedEdgeCount,
      },
    });

    const projectionSignature = [
      effectiveAreaId ?? 'global',
      nodes.length,
      edges.length,
      displayNodes.length,
      displayEdges.length,
      ghostDevices.length,
      selectedNodeCount,
      selectedEdgeCount,
    ].join(':');
    if (lastProjectionDiagnosticsSignatureRef.current === projectionSignature) {
      return;
    }
    lastProjectionDiagnosticsSignatureRef.current = projectionSignature;
    recordCanvasDiagnosticEvent({
      level: 'info',
      source: 'projection',
      event: 'projection.area.changed',
      message: 'Canvas area projection changed',
      metadata: {
        selectedAreaId: effectiveAreaId,
        canonicalNodeCount: nodes.length,
        canonicalEdgeCount: edges.length,
        displayedNodeCount: displayNodes.length,
        displayedEdgeCount: displayEdges.length,
        ghostNodeCount: ghostDevices.length,
      },
    });
  }, [
    effectiveAreaId,
    nodes.length,
    edges.length,
    displayNodes.length,
    displayEdges,
    displayEdges.length,
    ghostDevices.length,
    selectedNodeCount,
  ]);

  // fitView when effective area changes to re-center on filtered subset
  const prevAreaRef = useRef<string | null>(null);
  const canFitVisibleTopology =
    visible &&
    flowViewportReady &&
    nodesInitialized &&
    displayNodes.length > 0 &&
    renderedMapKey === selectedTopologyMapKey;
  useEffect(() => {
    if (canFitVisibleTopology && prevAreaRef.current !== effectiveAreaId) {
      let canceled = false;
      const frameId = window.requestAnimationFrame(() => {
        if (canceled) {
          return;
        }
        prevAreaRef.current = effectiveAreaId;
        reactFlow.fitView({ padding: topologyFitViewPadding, duration: 280 });
        recordCanvasDiagnosticEvent({
          level: 'debug',
          source: 'reactflow',
          event: 'reactflow.fit_view',
          message: 'React Flow fitView requested after area change',
          metadata: {
            selectedAreaId: effectiveAreaId,
            displayedNodeCount: displayNodes.length,
          },
        });
      });
      return () => {
        canceled = true;
        window.cancelAnimationFrame(frameId);
      };
    }
    return undefined;
  }, [canFitVisibleTopology, effectiveAreaId, displayNodes.length, reactFlow]);

  useEffect(() => {
    if (fitViewRevision === undefined) {
      return;
    }
    if (previousFitViewRevisionRef.current === fitViewRevision) {
      return;
    }
    if (!canFitVisibleTopology) {
      return;
    }

    let canceled = false;
    const frameId = window.requestAnimationFrame(() => {
      if (canceled) {
        return;
      }
      previousFitViewRevisionRef.current = fitViewRevision;
      reactFlow.fitView({ padding: topologyFitViewPadding, duration: 280 });
      recordCanvasDiagnosticEvent({
        level: 'debug',
        source: 'reactflow',
        event: 'reactflow.fit_view',
        message: 'React Flow fitView requested after canvas context activation',
        metadata: {
          selectedAreaId: effectiveAreaId,
          mapId: mapId ?? 'default',
          displayedNodeCount: displayNodes.length,
        },
      });
    });
    return () => {
      canceled = true;
      window.cancelAnimationFrame(frameId);
    };
  }, [
    canFitVisibleTopology,
    displayNodes.length,
    effectiveAreaId,
    fitViewRevision,
    mapId,
    reactFlow,
  ]);

  useEffect(() => {
    setNodes((prev) => prev.map((n) => ({ ...n, data: { ...n.data, editMode } })));
    if (!editMode) setSelectedNodeCount(0);
  }, [editMode, setNodes]);
  useEffect(
    () => () => {
      if (highlightTimerRef.current !== null) window.clearTimeout(highlightTimerRef.current);
      if (interactionIdleTimerRef.current !== null) {
        window.clearTimeout(interactionIdleTimerRef.current);
      }
    },
    [],
  );

  const handleEdgesChange = useCallback((changes: EdgeChange[]) => {
    setEdges((cur) => applyEdgeChanges(changes, cur));
  }, []);
  const handleConnect = useCallback(
    (connection: Connection) => {
      if (
        !editMode ||
        !connection.source ||
        !connection.target ||
        connection.source === connection.target
      )
        return;
      setPanelContent({
        type: 'create-link',
        data: {
          initialSourceDeviceId: connection.source,
          initialTargetDeviceId: connection.target,
        },
      });
    },
    [editMode, setPanelContent],
  );

  function focusOnDevice(deviceID: string) {
    // If the target device is not in the current area view, switch to its area first
    const device = devices.find((d) => d.id === deviceID);
    if (device && effectiveAreaId && !device.area_ids?.includes(effectiveAreaId)) {
      // Switch to the device's area (or global if unassigned)
      onAreaSelect?.(device.area_ids?.[0] ?? null);
      // Defer the focus/highlight until after the area switch triggers a re-render
      window.requestAnimationFrame(() => {
        window.requestAnimationFrame(() => {
          const target = reactFlow.getNodes().find((n) => n.id === deviceID);
          if (!target) return;
          reactFlow.setCenter(target.position.x + 110, target.position.y + 44, {
            zoom: 1.2,
            duration: 500,
          });
          setNodes((cur) =>
            cur.map((n) => ({
              ...n,
              data: { ...n.data, highlighted: n.id === deviceID },
            })),
          );
          if (highlightTimerRef.current !== null) window.clearTimeout(highlightTimerRef.current);
          highlightTimerRef.current = window.setTimeout(() => {
            setNodes((cur) =>
              cur.map((n) =>
                n.id === deviceID ? { ...n, data: { ...n.data, highlighted: false } } : n,
              ),
            );
          }, 2000);
        });
      });
      return;
    }

    const target = reactFlow.getNodes().find((n) => n.id === deviceID);
    if (!target) return;
    reactFlow.setCenter(target.position.x + 110, target.position.y + 44, {
      zoom: 1.2,
      duration: 500,
    });
    setNodes((cur) =>
      cur.map((n) => ({
        ...n,
        data: { ...n.data, highlighted: n.id === deviceID },
      })),
    );
    if (highlightTimerRef.current !== null) window.clearTimeout(highlightTimerRef.current);
    highlightTimerRef.current = window.setTimeout(() => {
      setNodes((cur) =>
        cur.map((n) => (n.id === deviceID ? { ...n, data: { ...n.data, highlighted: false } } : n)),
      );
    }, 2000);
  }

  // Resolve Grafana URL for a device (per-device override or global)
  function grafanaUrl(deviceId?: string): string {
    if (deviceId) return deviceGrafanaUrlsRef.current.get(deviceId) || grafanaUrlRef.current;
    return grafanaUrlRef.current;
  }

  if (loading) {
    return (
      <div className="topology-backdrop flex h-full items-center justify-center bg-bg">
        <div className="rounded-[28px] border border-outline bg-surface/88 px-6 py-5 text-center shadow-canvas backdrop-blur-sm">
          <div className="mx-auto mb-3 h-10 w-10 animate-spin rounded-full border-2 border-outline-subtle border-t-primary" />
          <p className="text-sm uppercase tracking-[0.28em] text-on-bg-secondary">
            Loading topology...
          </p>
        </div>
      </div>
    );
  }

  if (error) {
    return (
      <div className="topology-backdrop flex h-full items-center justify-center bg-bg px-6">
        <div className="max-w-md rounded-[28px] border border-outline bg-surface/88 px-6 py-6 text-center shadow-canvas backdrop-blur-sm">
          <p className="text-sm uppercase tracking-[0.28em] text-status-down">Topology Error</p>
          <h2 className="mt-3 text-2xl font-semibold tracking-tight text-on-bg">
            Canvas data could not load
          </h2>
          <p className="mt-3 text-sm text-on-bg-secondary">{error}</p>
          <button
            type="button"
            onClick={() => {
              void loadTopology();
            }}
            className="mt-6 rounded-full border border-primary/40 bg-primary/10 px-5 py-2 text-sm font-medium text-primary transition-colors duration-150 hover:bg-primary/20"
          >
            Retry
          </button>
        </div>
      </div>
    );
  }

  return (
    <div
      ref={canvasRootRef}
      data-testid="topology-canvas-root"
      className={`topology-backdrop relative h-full w-full bg-bg ${canvasInteractionActive ? 'topology-interacting' : ''}`}
    >
      {showSearch && <SearchOverlay devices={devices} onSelectDevice={focusOnDevice} />}
      <Toolbar
        onSearch={() => setShowSearch((s) => !s)}
        onAddDevice={() => setPanelContent({ type: 'addDevice' })}
        onCreateLink={() => setPanelContent({ type: 'create-link' })}
        onAlerts={() => setPanelContent({ type: 'alerts' })}
        onSettings={() => setPanelContent({ type: 'settings' })}
        onToggleEditMode={() => setEditMode((m) => !m)}
        editMode={editMode}
        alertCount={runtimeSummary.alertCount}
      />
      {deviceMenu &&
        (() => {
          const d = devices.find((dev) => dev.id === deviceMenu.deviceId);
          const gUrl = grafanaUrl(d?.id);
          const isVirtual = d?.device_type === 'virtual';
          const hasWinboxProfile = deviceWinboxState[deviceMenu.deviceId];
          const winboxDisabled = hasWinboxProfile === false;
          const winboxTitle =
            hasWinboxProfile === false
              ? 'No WinBox profile designated'
              : bridgeChecked && !bridgeRunning
                ? 'WinBox bridge appears unavailable - click to try launch anyway'
                : undefined;
          const items = buildDeviceContextMenuItems({
            isVirtual,
            grafanaEnabled: Boolean(gUrl),
            winboxDisabled,
            winboxTitle,
            onOpenWinbox: () => {
              if (d) void launchWinbox(d.id);
              setDeviceMenu(null);
            },
            onOpenGrafana: () => {
              if (gUrl) window.open(gUrl, '_blank');
              setDeviceMenu(null);
            },
            onConfigure: () => {
              if (d)
                setPanelContent({
                  type: 'deviceConfig',
                  data: { deviceId: d.id },
                });
              setDeviceMenu(null);
            },
          });
          return (
            <ContextMenu
              position={{ x: deviceMenu.x, y: deviceMenu.y }}
              onClose={() => setDeviceMenu(null)}
              items={items}
            />
          );
        })()}

      {edgeMenu &&
        (() => {
          const me = edges.find((e) => e.id === edgeMenu.edgeID);
          const ml = me?.data?.link;
          const dMap = new Map(devices.map((d) => [d.id, d]));
          const sd = ml ? dMap.get(ml.source_device_id) : undefined;
          const gUrl = grafanaUrl(sd?.id);
          return (
            <ContextMenu
              position={{ x: edgeMenu.x, y: edgeMenu.y }}
              onClose={() => setEdgeMenu(null)}
              items={[
                {
                  label: 'Per-Interface Stats',
                  icon: 'devices',
                  onClick: () => {
                    if (ml)
                      setPanelContent({
                        type: 'interfaceStats',
                        data: { linkId: ml.id },
                      });
                    setEdgeMenu(null);
                  },
                },
                {
                  label: gUrl ? 'Open in Grafana' : 'Open in Grafana (not configured)',
                  icon: 'hub',
                  onClick: () => {
                    if (gUrl) window.open(gUrl, '_blank');
                    setEdgeMenu(null);
                  },
                },
                {
                  label: 'View Details',
                  icon: 'search',
                  onClick: () => {
                    const el = edges.find((e) => e.id === edgeMenu.edgeID)?.data?.link;
                    if (el)
                      setPanelContent({
                        type: 'link-details',
                        data: { link: el },
                      });
                    setEdgeMenu(null);
                  },
                },
              ]}
            />
          );
        })()}

      <SidePanel
        open={!!panelContent}
        onClose={() => setPanelContent(null)}
        title={getPanelTitle()}
        testId={getCanvasDetailDeviceId(panelContent) !== null ? 'device-detail-panel' : undefined}
      >
        <CanvasPanels
          panelContent={panelContent}
          setPanelContent={setPanelContent}
          alerts={alerts}
          devices={devices}
          topologyLinks={topologyLinks}
          topologyAreas={topologyAreas}
          loadTopology={loadTopology}
          setDevices={setDevices}
          setNodes={setNodes}
          reactFlow={reactFlow}
          runtimeState={runtimeState}
          mapId={mapId}
          mapName={mapName}
          editMode={editMode}
          onRemoveDeviceFromMap={handleRemoveDeviceFromMap}
          onSettingsChange={refreshSettings}
          onWinBoxAvailabilityChange={(deviceId, hasWinboxProfile) => {
            setDeviceWinboxAvailability(deviceId, hasWinboxProfile);
          }}
        />
      </SidePanel>

      <ShortcutHelp open={showShortcuts} onClose={() => setShowShortcuts(false)} />
      <CanvasOverlays
        editMode={editMode}
        reconnecting={reconnecting}
        topologyRecoveryNotice={topologyRecoveryNotice}
        dismissTopologyRecoveryNotice={dismissTopologyRecoveryNotice}
        retryTopologyRefresh={retryTopologyRefresh}
        selectedNodeCount={selectedNodeCount}
        prometheusDiagnosticsVisible={runtimeSummary.prometheusDiagnosticsVisible}
        onBulkEditClick={() => {
          if (!editMode) return;
          const selectedNodes = reactFlow.getNodes().filter((n) => n.selected);
          if (selectedNodes.length > 1) {
            setPanelContent({
              type: 'bulkEdit',
              data: { deviceIds: selectedNodes.map((n) => n.id) },
            });
          }
        }}
      />
      {/* WinBox launch error toast */}
      {winboxError && (
        <div
          data-testid="winbox-error-toast"
          className="absolute top-20 bottom-auto left-1/2 z-50 flex -translate-x-1/2 items-center gap-2 rounded-full border border-status-down/35 bg-surface-container-high px-4 py-2.5 text-xs text-status-down shadow-floating sm:top-auto sm:bottom-16"
        >
          <span>{winboxError}</span>
          <button type="button" onClick={clearWinboxError} className="ml-1 hover:opacity-70">
            &times;
          </button>
        </div>
      )}

      <ZoomControls
        onZoomIn={() => {
          void reactFlow.zoomIn({ duration: 200 });
        }}
        onZoomOut={() => {
          void reactFlow.zoomOut({ duration: 200 });
        }}
        onFitView={() => {
          void reactFlow.fitView({
            padding: topologyFitViewPadding,
            duration: 280,
          });
          recordCanvasDiagnosticEvent({
            level: 'debug',
            source: 'reactflow',
            event: 'reactflow.fit_view',
            message: 'React Flow fitView requested from zoom controls',
          });
        }}
      />
      <CanvasDiagnosticsPanel
        open={diagnosticsVisible}
        onClose={() => {
          window.localStorage.setItem(canvasDiagnosticsStorageKey, 'false');
          setDiagnosticsVisible(false);
        }}
        onForceRefresh={() => window.__THEIA_CANVAS_FORCE_REFRESH__?.()}
        onFitView={() => {
          void reactFlow.fitView({
            padding: topologyFitViewPadding,
            duration: 280,
          });
          recordCanvasDiagnosticEvent({
            level: 'debug',
            source: 'reactflow',
            event: 'reactflow.fit_view',
            message: 'React Flow fitView requested from diagnostics panel',
          });
        }}
      />

      <ReactFlow
        data-testid="topology-canvas"
        nodes={displayNodes}
        edges={renderEdges}
        nodeTypes={nodeTypes}
        edgeTypes={edgeTypes}
        onNodesChange={handleNodesChange}
        onEdgesChange={handleEdgesChange}
        onConnect={handleConnect}
        onConnectStart={beginCanvasInteraction}
        onConnectEnd={endCanvasInteraction}
        onMoveStart={beginCanvasInteraction}
        onMove={handleCanvasMove}
        onMoveEnd={endCanvasInteraction}
        onSelectionChange={handleSelectionChange}
        onPaneClick={() => {
          setEdgeMenu(null);
          setDeviceMenu(null);
          setPanelContent(null);
          setShowSearch(false);
          setShowShortcuts(false);
        }}
        onNodeClick={(_ev, node) => {
          if (isGhostDeviceNode(node)) return;
          // Check if multiple nodes are selected (including the just-clicked one)
          const selectedNodes = reactFlow.getNodes().filter((n) => n.selected);
          if (selectedNodes.length > 1 && editMode) {
            setPanelContent({
              type: 'bulkEdit',
              data: { deviceIds: selectedNodes.map((n) => n.id) },
            });
          } else {
            const cd = devices.find((d) => d.id === node.id);
            if (cd)
              setPanelContent({
                type: 'deviceDetails',
                data: { deviceId: cd.id },
              });
          }
        }}
        onEdgeClick={(_ev, edge) => {
          const lk = edge.data?.link;
          if (!lk) return;
          setPanelContent({
            type: 'link-details',
            data: { link: lk },
          });
        }}
        onNodeDragStart={beginCanvasInteraction}
        onNodeDragStop={(_ev, node) => {
          endCanvasInteraction();
          if (isGhostDeviceNode(node)) return;
          updateNodePosition(node.id, node.position);
        }}
        selectionOnDrag={editMode}
        selectionMode={SelectionMode.Partial}
        selectionKeyCode="Shift"
        connectionMode={ConnectionMode.Loose}
        minZoom={0.1}
        maxZoom={2}
        fitView
        fitViewOptions={{ padding: topologyFitViewPadding }}
        onlyRenderVisibleElements
        nodesDraggable={editMode}
        panOnDrag
        zoomOnScroll
        zoomOnDoubleClick={false}
        connectionLineStyle={{ stroke: 'var(--color-edge-default)', strokeWidth: 6 }}
        proOptions={{ hideAttribution: false }}
        className="bg-transparent"
      >
        <Background color="var(--color-edge-muted)" gap={30} size={1.1} />
        <LinkLabelLayer />
        <TopologyMiniMap />
      </ReactFlow>
    </div>
  );
}
