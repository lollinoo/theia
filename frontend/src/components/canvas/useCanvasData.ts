import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import type { ReactFlowInstance } from '@xyflow/react';

import { fetchDevices, fetchLinks, fetchSettings, createLink } from '../../api/client';
import { computeForceLayout, type AutoLayoutEdge, type AutoLayoutNode } from '../../hooks/useAutoLayout';
import { usePositions, type PositionState } from '../../hooks/usePositions';
import type { Device, Link } from '../../types/api';
import {
  alertStatusForDevice,
  isPrometheusUnavailable,
  type AlertDTO,
  type PrometheusStatusPayload,
  type SnapshotPayload,
} from '../../types/metrics';
import type { DeviceNode } from '../DeviceCard';
import type { LinkEdgeType } from '../LinkEdge';
import {
  buildPositionPayload,
  manualEdgeStorageKey,
  viewportSize,
} from './canvasHelpers';
import { measureCanvasAsyncWork, measureCanvasWork, type CanvasMeasurementTrigger } from './canvasInstrumentation';
import { alertStatusForLink } from './edgeBuilder';
import { composeCanvasTopology } from './topologyComposer';
import { buildTopologyIdentity, collectPlacementDeviceIds } from './topologyIdentity';
import { buildRuntimeState, countActiveAlertsFromRuntimeState } from './runtimeAdapters';

interface UseCanvasDataParams {
  snapshot: SnapshotPayload | null;
  alerts?: AlertDTO[];
  reconnecting: boolean;
  prometheusStatus: PrometheusStatusPayload | null;
  editMode: boolean;
  openDeviceMenu: (event: React.MouseEvent, deviceId: string) => void;
  openEdgeMenu: (event: MouseEvent | React.MouseEvent<SVGPathElement>, edgeID: string) => void;
  openSelfLinkDetails?: (link: Link) => void;
  reactFlow: ReactFlowInstance<DeviceNode, LinkEdgeType>;
  nodes: DeviceNode[];
  setNodes: React.Dispatch<React.SetStateAction<DeviceNode[]>>;
  setEdges: React.Dispatch<React.SetStateAction<LinkEdgeType[]>>;
  onDevicesChange?: (devices: Device[]) => void;
  onLinksChange?: (links: Link[]) => void;
}

interface UseCanvasDataReturn {
  devices: Device[];
  setDevices: React.Dispatch<React.SetStateAction<Device[]>>;
  topologyLinks: Link[];
  runtimeSummary: RuntimeSummary;
  loading: boolean;
  error: string | null;
  loadTopology: (
    isSilentRefresh?: boolean,
    defaultPosition?: { x: number; y: number },
    trigger?: CanvasMeasurementTrigger,
  ) => Promise<void>;
  grafanaUrlRef: React.MutableRefObject<string>;
  deviceGrafanaUrlsRef: React.MutableRefObject<Map<string, string>>;
  refreshSettings: () => void;
  topologyRecoveryNotice: TopologyRecoveryNotice | null;
  dismissTopologyRecoveryNotice: () => void;
  retryTopologyRefresh: () => void;
}

type StructuralRefreshCause =
  | 'backend-reconnected'
  | 'backend-resync-required'
  | 'topology-changed';

export interface TopologyRecoveryNotice {
  tone: 'success' | 'warning';
  message: string;
  actionLabel?: string;
}

interface RuntimeSummary {
  alertCount: number;
  prometheusDiagnosticsVisible: boolean;
}

interface LoadTopologyOptions {
  suppressBlockingError?: boolean;
  rethrowOnError?: boolean;
}

const structuralRefreshDebounceMs = 250;
const topologyRefreshRetryActionLabel = 'Retry topology refresh';
const topologyRefreshDelayedMessage = 'Live topology refresh delayed';
const emptyAlerts: AlertDTO[] = [];

function runtimeAlertStatusForDevice(
  deviceId: string,
  snapshot: SnapshotPayload | null,
  alerts: AlertDTO[],
) {
  return snapshot?.devices[deviceId]?.alert_status ?? alertStatusForDevice(deviceId, alerts);
}

function measurementTriggerForCauses(
  causes: Set<StructuralRefreshCause>,
): CanvasMeasurementTrigger {
  if (
    causes.has('backend-reconnected')
    || causes.has('backend-resync-required')
  ) {
    return 'backend_reconnected';
  }

  return 'topology_changed';
}

function buildTopologyRecoveryNotice(
  causes: Set<StructuralRefreshCause>,
): TopologyRecoveryNotice | null {
  const hasReconnect = causes.has('backend-reconnected');
  const hasResync = causes.has('backend-resync-required');
  const hasTopologyChanged = causes.has('topology-changed');

  if (!hasReconnect && !hasResync) {
    return null;
  }

  if ((hasReconnect && hasResync) || hasTopologyChanged) {
    return {
      tone: 'success',
      message: 'Topology refreshed',
    };
  }

  if (hasResync) {
    return {
      tone: 'success',
      message: 'Live topology resynced',
    };
  }

  return {
    tone: 'success',
    message: 'Topology refreshed after reconnect',
  };
}

function hasUsablePosition(position: PositionState | undefined): position is PositionState {
  return position !== undefined
    && Number.isFinite(position.x)
    && Number.isFinite(position.y);
}

function buildUsablePositionState(
  devices: Device[],
  currentPositions: Map<string, PositionState>,
  savedPositions: Map<string, PositionState>,
): string {
  return devices
    .map((device) => {
      const currentPosition = currentPositions.get(device.id);
      const savedPosition = savedPositions.get(device.id);

      if (hasUsablePosition(currentPosition) || hasUsablePosition(savedPosition)) {
        return device.id;
      }

      return null;
    })
    .filter((deviceId): deviceId is string => deviceId !== null)
    .sort()
    .join('|');
}

function positionsChanged(
  nextPositions: ReturnType<typeof buildPositionPayload>,
  savedPositions: Map<string, PositionState>,
): boolean {
  if (nextPositions.length !== savedPositions.size) {
    return true;
  }

  for (const position of nextPositions) {
    const savedPosition = savedPositions.get(position.device_id);
    if (
      !savedPosition
      || savedPosition.x !== position.x
      || savedPosition.y !== position.y
      || savedPosition.pinned !== position.pinned
    ) {
      return true;
    }
  }

  return false;
}

function buildLayoutInputs(
  devices: Device[],
  links: Link[],
  placementDeviceIds: Set<string>,
  effectivePositions: Map<string, PositionState>,
): { layoutNodes: AutoLayoutNode[]; layoutEdges: AutoLayoutEdge[] } {
  if (placementDeviceIds.size === 0) {
    return {
      layoutNodes: [],
      layoutEdges: [],
    };
  }

  const layoutDeviceIds = new Set(placementDeviceIds);

  for (const link of links) {
    const sourceNeedsPlacement = placementDeviceIds.has(link.source_device_id);
    const targetNeedsPlacement = placementDeviceIds.has(link.target_device_id);

    if (sourceNeedsPlacement === targetNeedsPlacement) {
      continue;
    }

    const anchorDeviceId = sourceNeedsPlacement
      ? link.target_device_id
      : link.source_device_id;
    const anchorPosition = effectivePositions.get(anchorDeviceId);

    if (hasUsablePosition(anchorPosition)) {
      layoutDeviceIds.add(anchorDeviceId);
    }
  }

  return {
    layoutNodes: devices
      .filter((device) => layoutDeviceIds.has(device.id))
      .map((device) => {
        const position = effectivePositions.get(device.id);
        const needsPlacement = placementDeviceIds.has(device.id);

        return {
          id: device.id,
          x: position?.x,
          y: position?.y,
          pinned: !needsPlacement && hasUsablePosition(position),
        };
      }),
    layoutEdges: links
      .filter((link) => layoutDeviceIds.has(link.source_device_id) && layoutDeviceIds.has(link.target_device_id))
      .map((link) => ({
        source: link.source_device_id,
        target: link.target_device_id,
      })),
  };
}

function mergeNodePresentationState(
  nextNodes: DeviceNode[],
  currentNodes: DeviceNode[],
): DeviceNode[] {
  const currentNodesById = new Map(currentNodes.map((node) => [node.id, node]));

  return nextNodes.map((node) => {
    const currentNode = currentNodesById.get(node.id);
    if (!currentNode) {
      return node;
    }

    return {
      ...node,
      selected: currentNode.selected,
      dragging: currentNode.dragging,
      data: {
        ...node.data,
        highlighted: currentNode.data.highlighted,
      },
    };
  });
}

export function useCanvasData({
  snapshot,
  alerts = emptyAlerts,
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
}: UseCanvasDataParams): UseCanvasDataReturn {
  const [devices, setDevices] = useState<Device[]>([]);
  const [topologyLinks, setTopologyLinks] = useState<Link[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  const snapshotRef = useRef<SnapshotPayload | null>(null);
  const alertsRef = useRef<AlertDTO[]>(alerts);
  const devicesRef = useRef<Device[]>([]);
  const topologyLinksRef = useRef<Link[]>([]);
  const nodesRef = useRef<DeviceNode[]>(nodes);
  const lastTopologyIdentityRef = useRef<string | null>(null);
  const lastUsablePositionStateRef = useRef('');
  const currentNodePositionsRef = useRef<Map<string, PositionState>>(new Map());
  const grafanaUrlRef = useRef<string>('');
  const deviceGrafanaUrlsRef = useRef<Map<string, string>>(new Map());
  const { fetchPositions, savePositions } = usePositions();

  const runtimeSummary = useMemo<RuntimeSummary>(() => {
    const runtimeState = buildRuntimeState({
      devices,
      links: topologyLinks,
      snapshot,
      alerts,
      prometheusStatus,
    });

    return {
      alertCount: countActiveAlertsFromRuntimeState(runtimeState, alerts),
      prometheusDiagnosticsVisible: isPrometheusUnavailable(prometheusStatus),
    };
  }, [alerts, devices, prometheusStatus, snapshot, topologyLinks]);

  const [topologyRecoveryNotice, setTopologyRecoveryNotice] = useState<TopologyRecoveryNotice | null>(null);
  const structuralRefreshTimerRef = useRef<number | null>(null);
  const pendingStructuralRefreshCausesRef = useRef<Set<StructuralRefreshCause>>(new Set());
  const lastStructuralRefreshCausesRef = useRef<Set<StructuralRefreshCause>>(new Set());

  // Keep refs in sync so async loadTopology and snapshot effect can read the latest state
  useEffect(() => {
    snapshotRef.current = snapshot;
  }, [snapshot]);
  useEffect(() => {
    alertsRef.current = alerts;
  }, [alerts]);
  devicesRef.current = devices;
  topologyLinksRef.current = topologyLinks;
  nodesRef.current = nodes;
  currentNodePositionsRef.current = new Map(
    nodes.map((node) => [
      node.id,
      {
        x: node.position.x,
        y: node.position.y,
        pinned: node.data.pinned ?? false,
      },
    ]),
  );

  // Propagate device state changes to parent (for Dashboard view)
  useEffect(() => {
    onDevicesChange?.(devices);
  }, [devices, onDevicesChange]);

  // Propagate link state changes to parent (for Hub view)
  useEffect(() => {
    onLinksChange?.(topologyLinks);
  }, [topologyLinks, onLinksChange]);

  const loadTopology = useCallback(
    async (
      isSilentRefresh = false,
      defaultPosition?: { x: number; y: number },
      trigger: CanvasMeasurementTrigger = 'manual_refresh',
      options: LoadTopologyOptions = {},
    ) => measureCanvasAsyncWork('theia:canvas:topology-load', trigger, async () => {
      if (!isSilentRefresh) {
        setLoading(true);
      }
      setError(null);

      try {
        const [fetchedDevices, fetchedLinks, savedPositions] = await Promise.all([
          fetchDevices(),
          fetchLinks(),
          fetchPositions(),
        ]);

        const topologyIdentity = buildTopologyIdentity(fetchedDevices, fetchedLinks);
        const structureChanged = lastTopologyIdentityRef.current !== topologyIdentity.signature;
        const effectivePositions = new Map(savedPositions);
        for (const [deviceId, position] of currentNodePositionsRef.current.entries()) {
          if (!effectivePositions.has(deviceId)) {
            effectivePositions.set(deviceId, position);
          }
        }

        const usablePositionState = buildUsablePositionState(
          fetchedDevices,
          currentNodePositionsRef.current,
          savedPositions,
        );
        const shouldAutoFitView = usablePositionState.length === 0;

        // Read any pending snapshot so first-load metrics are included in the
        // initial node/edge data -- eliminates the race where the WS snapshot
        // arrives before loadTopology resolves and the snapshot effect maps over
        // an empty node array.
        const runtimeState = buildRuntimeState({
          devices: fetchedDevices,
          links: fetchedLinks,
          snapshot: snapshotRef.current,
          alerts: alertsRef.current,
          prometheusStatus,
        });
        const runtimeDevices = fetchedDevices.map(
          (device) => runtimeState.devicesById.get(device.id)?.device ?? device,
        );

        // Migrate localStorage manual edges to backend on first load (best-effort)
        const storedManualRaw = window.localStorage.getItem(manualEdgeStorageKey);
        if (storedManualRaw) {
          try {
            const storedManual = JSON.parse(storedManualRaw) as Array<{
              id: string;
              source: string;
              target: string;
            }>;
            if (Array.isArray(storedManual) && storedManual.length > 0) {
              const results = await Promise.allSettled(storedManual.map((edge) =>
                createLink({
                  source_device_id: edge.source,
                  source_if_name: '',
                  target_device_id: edge.target,
                  target_if_name: '',
                }),
              ));
              const failedMigrations = storedManual.filter(
                (_, index) => results[index]?.status === 'rejected',
              );
              if (failedMigrations.length === 0) {
                window.localStorage.removeItem(manualEdgeStorageKey);
              } else {
                window.localStorage.setItem(
                  manualEdgeStorageKey,
                  JSON.stringify(failedMigrations),
                );
              }
            } else {
              window.localStorage.removeItem(manualEdgeStorageKey);
            }
          } catch {
            window.localStorage.removeItem(manualEdgeStorageKey);
          }
        }

        if (!structureChanged) {
          setDevices(runtimeDevices);
          setTopologyLinks(fetchedLinks);
          const { nodes: nextNodes, edges: nextEdges } = composeCanvasTopology({
            devices: fetchedDevices,
            links: fetchedLinks,
            runtimeState,
            savedPositions: effectivePositions,
            computedPositions: new Map(),
            currentPositions: currentNodePositionsRef.current,
            defaultPosition,
            editMode,
            openDeviceMenu,
            openEdgeMenu,
            openSelfLinkDetails,
            placementDeviceIds: new Set(),
            alerts: alertsRef.current,
          });
          setNodes((currentNodes) => mergeNodePresentationState(nextNodes, currentNodes));
          setEdges(nextEdges);
          lastTopologyIdentityRef.current = topologyIdentity.signature;
          lastUsablePositionStateRef.current = usablePositionState;
          return;
        }

        const placementDeviceIds = collectPlacementDeviceIds(
          fetchedDevices,
          currentNodePositionsRef.current,
          savedPositions,
          currentNodePositionsRef.current.keys(),
        );
        const { width, height } = viewportSize();
        const { layoutNodes, layoutEdges } = buildLayoutInputs(
          fetchedDevices,
          fetchedLinks,
          placementDeviceIds,
          effectivePositions,
        );
        const computedPositions = layoutNodes.length > 0
          ? measureCanvasWork('theia:canvas:layout', trigger, () =>
              computeForceLayout(layoutNodes, layoutEdges, width, height),
            )
          : new Map();

        const { nodes: composedNodes, edges: composedEdges } = composeCanvasTopology({
          devices: fetchedDevices,
          links: fetchedLinks,
          runtimeState,
          savedPositions: effectivePositions,
          computedPositions,
          currentPositions: currentNodePositionsRef.current,
          defaultPosition,
          editMode,
          openDeviceMenu,
          openEdgeMenu,
          openSelfLinkDetails,
          placementDeviceIds,
          alerts: alertsRef.current,
        });

        // Apply all state updates together as urgent (not in startTransition).
        // Previously these were wrapped in startTransition which made them
        // low-priority, allowing WebSocket snapshot effects (which depend on
        // devices.length) to interrupt and race with the transition's setNodes,
        // sometimes causing all canvas nodes to vanish after a device delete.
        setDevices(runtimeDevices);
        setTopologyLinks(fetchedLinks);
        setNodes((currentNodes) => {
          return mergeNodePresentationState(composedNodes, currentNodes);
        });
        setEdges(composedEdges);

        const nextPositionPayload = buildPositionPayload(composedNodes);
        if (positionsChanged(nextPositionPayload, savedPositions)) {
          void savePositions(nextPositionPayload);
        }

        if (trigger === 'initial_load' || shouldAutoFitView) {
          window.requestAnimationFrame(() => {
            reactFlow.fitView({ padding: 0.18, duration: 320 });
          });
        }

        lastTopologyIdentityRef.current = topologyIdentity.signature;
        lastUsablePositionStateRef.current = usablePositionState;
      } catch (loadError) {
        const topologyError = loadError instanceof Error
          ? loadError
          : new Error('Failed to load topology');

        if (!options.suppressBlockingError) {
          setError(topologyError.message);
        }

        if (options.rethrowOnError) {
          throw topologyError;
        }
      } finally {
        if (!isSilentRefresh) {
          setLoading(false);
        }
      }
    }),
    [editMode, openDeviceMenu, openEdgeMenu, openSelfLinkDetails, reactFlow, setNodes, setEdges, fetchPositions, savePositions],
  );

  const dismissTopologyRecoveryNotice = useCallback(() => {
    setTopologyRecoveryNotice(null);
  }, []);

  const runStructuralRefresh = useCallback(
    async (causes: Set<StructuralRefreshCause>) => {
      const refreshCauses = new Set(causes);
      lastStructuralRefreshCausesRef.current = refreshCauses;
      setTopologyRecoveryNotice(null);

      try {
        await loadTopology(
          true,
          undefined,
          measurementTriggerForCauses(refreshCauses),
          {
            suppressBlockingError: true,
            rethrowOnError: true,
          },
        );
        setTopologyRecoveryNotice(buildTopologyRecoveryNotice(refreshCauses));
      } catch {
        setTopologyRecoveryNotice({
          tone: 'warning',
          message: topologyRefreshDelayedMessage,
          actionLabel: topologyRefreshRetryActionLabel,
        });
      }
    },
    [loadTopology],
  );

  const retryTopologyRefresh = useCallback(() => {
    const retryCauses = lastStructuralRefreshCausesRef.current.size > 0
      ? new Set(lastStructuralRefreshCausesRef.current)
      : new Set<StructuralRefreshCause>(['topology-changed']);
    void runStructuralRefresh(retryCauses);
  }, [runStructuralRefresh]);

  const queueStructuralRefresh = useCallback(
    (cause: StructuralRefreshCause) => {
      pendingStructuralRefreshCausesRef.current.add(cause);

      if (structuralRefreshTimerRef.current !== null) {
        return;
      }

      structuralRefreshTimerRef.current = window.setTimeout(() => {
        structuralRefreshTimerRef.current = null;
        const refreshCauses = new Set(pendingStructuralRefreshCausesRef.current);
        pendingStructuralRefreshCausesRef.current.clear();
        void runStructuralRefresh(refreshCauses);
      }, structuralRefreshDebounceMs);
    },
    [runStructuralRefresh],
  );

  // Initial load
  useEffect(() => {
    void loadTopology(false, undefined, 'initial_load');
  }, []);

  // Route reconnect, resync, and topology notifications through one structural
  // refresh scheduler so clustered events produce a single revalidation pass.
  useEffect(() => {
    const handleReconnect = () => {
      queueStructuralRefresh('backend-reconnected');
    };
    const handleResyncRequired = () => {
      queueStructuralRefresh('backend-resync-required');
    };
    const handleTopologyChanged = () => {
      queueStructuralRefresh('topology-changed');
    };

    window.addEventListener('backend-reconnected', handleReconnect);
    window.addEventListener('backend-resync-required', handleResyncRequired);
    window.addEventListener('topology-changed', handleTopologyChanged);

    return () => {
      if (structuralRefreshTimerRef.current !== null) {
        window.clearTimeout(structuralRefreshTimerRef.current);
        structuralRefreshTimerRef.current = null;
      }
      pendingStructuralRefreshCausesRef.current.clear();
      window.removeEventListener('backend-reconnected', handleReconnect);
      window.removeEventListener('backend-resync-required', handleResyncRequired);
      window.removeEventListener('topology-changed', handleTopologyChanged);
    };
  }, [queueStructuralRefresh]);

  // Re-fetch settings (Grafana URLs) on demand; called on mount and after
  // any settings panel or device config panel saves Grafana URL changes.
  const refreshSettings = useCallback(() => {
    fetchSettings()
      .then((settings) => {
        grafanaUrlRef.current = settings['grafana_url'] ?? '';
        // Parse per-device Grafana dashboard URLs stored as grafana_dashboard_url:<device_id>
        const perDeviceUrls = new Map<string, string>();
        for (const [key, value] of Object.entries(settings)) {
          if (key.startsWith('grafana_dashboard_url:') && value) {
            const deviceId = key.slice('grafana_dashboard_url:'.length);
            perDeviceUrls.set(deviceId, value);
          }
        }
        deviceGrafanaUrlsRef.current = perDeviceUrls;
      })
      .catch(() => {
        // Settings fetch failure is non-fatal; Grafana links will be disabled.
      });
  }, []);

  // Fetch settings on mount
  useEffect(() => {
    refreshSettings();
  }, [refreshSettings]);

  useEffect(() => {
    if (topologyRecoveryNotice?.tone !== 'success') {
      return;
    }

    const timer = window.setTimeout(() => {
      setTopologyRecoveryNotice((current) => (
        current?.tone === 'success'
          ? null
          : current
      ));
    }, 4000);

    return () => {
      window.clearTimeout(timer);
    };
  }, [topologyRecoveryNotice]);

  // Apply snapshot data to nodes and edges.
  //
  // IMPORTANT: This effect intentionally does NOT depend on devices.length or
  // topologyLinks.length. Previously those were included to re-apply snapshot
  // data after topology changes (add/delete device or link), but loadTopology
  // already bakes snapshot data into the nodes/edges it builds (via
  // snapshotRef.current). The redundant re-trigger caused a cascade of
  // competing setNodes/setEdges calls that produced rendering glitches:
  //   loadTopology setNodes -> devices.length changes -> snapshot effect
  //   setNodes again -> displayNodes new refs -> StoreUpdater replace cycle.
  // By only reacting to actual snapshot or Prometheus status changes, we
  // eliminate the double-update after add/delete operations.
  //
  // The promDown override uses devicesRef (always current) instead of the
  // devices state variable so it doesn't need devices.length as a dependency.
  useEffect(() => {
    if (snapshot === null) {
      return;
    }

    measureCanvasWork('theia:canvas:snapshot-apply', 'snapshot', () => {
      const runtimeState = buildRuntimeState({
        devices: devicesRef.current,
        links: topologyLinksRef.current,
        snapshot,
        alerts: alertsRef.current,
        prometheusStatus,
      });
      const { nodes: nextNodes, edges: nextEdges } = composeCanvasTopology({
        devices: devicesRef.current,
        links: topologyLinksRef.current,
        runtimeState,
        savedPositions: currentNodePositionsRef.current,
        computedPositions: new Map(),
        currentPositions: currentNodePositionsRef.current,
        defaultPosition: undefined,
        editMode,
        openDeviceMenu,
        openEdgeMenu,
        openSelfLinkDetails,
        placementDeviceIds: new Set(),
        alerts: alertsRef.current,
      });

      setNodes((currentNodes) => mergeNodePresentationState(nextNodes, currentNodes));
      setEdges(nextEdges);
      setDevices(devicesRef.current.map((device) => runtimeState.devicesById.get(device.id)?.device ?? device));
    });
  }, [editMode, openDeviceMenu, openEdgeMenu, openSelfLinkDetails, prometheusStatus, setEdges, setNodes, snapshot]);

  useEffect(() => {
    setNodes((currentNodes) => {
      let changed = false;
      const nextNodes = currentNodes.map((node) => {
        const alertStatus = runtimeAlertStatusForDevice(node.id, snapshot, alerts);
        if (node.data.alertStatus === alertStatus) {
          return node;
        }
        changed = true;
        return {
          ...node,
          data: {
            ...node.data,
            alertStatus,
          },
        };
      });
      return changed ? nextNodes : currentNodes;
    });

    setEdges((currentEdges) => {
      let changed = false;
      const nextEdges = currentEdges.map((edge) => {
        const alertStatus = edge.data?.link ? alertStatusForLink(edge.data.link, alerts) : undefined;
        if (!edge.data || edge.data.alertStatus === alertStatus) {
          return edge;
        }
        changed = true;
        return {
          ...edge,
          data: {
            ...edge.data,
            alertStatus,
          },
        };
      });
      return changed ? nextEdges : currentEdges;
    });
  }, [alerts, setEdges, setNodes]);

  return {
    devices,
    setDevices,
    topologyLinks,
    runtimeSummary,
    loading,
    error,
    loadTopology,
    grafanaUrlRef,
    deviceGrafanaUrlsRef,
    refreshSettings,
    topologyRecoveryNotice,
    dismissTopologyRecoveryNotice,
    retryTopologyRefresh,
  };
}
