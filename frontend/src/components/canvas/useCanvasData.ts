import type { ReactFlowInstance } from '@xyflow/react';
import { useCallback, useEffect, useMemo, useRef, useState } from 'react';

import {
  createLink,
  fetchCanvasTopology,
  fetchDevices,
  fetchLinks,
  fetchSettings,
} from '../../api/client';
import { type PositionState, usePositions } from '../../hooks/usePositions';
import type { Device, DevicePosition, Link } from '../../types/api';
import {
  type AlertDTO,
  type PrometheusStatusPayload,
  type SnapshotPayload,
  alertStatusForDevice,
  isPrometheusUnavailable,
} from '../../types/metrics';
import type { DeviceNode } from '../DeviceCard';
import type { LinkEdgeType } from '../LinkEdge';
import { recordCanvasDiagnosticEvent, updateCanvasDiagnosticsState } from './canvasDiagnostics';
import {
  buildPositionPayload,
  isGhostDeviceNode,
  manualEdgeStorageKey,
  topologyFitViewPadding,
  viewportSize,
} from './canvasHelpers';
import {
  type CanvasMeasurementTrigger,
  measureCanvasAsyncWork,
  measureCanvasWork,
} from './canvasInstrumentation';
import { alertStatusForLink, buildTopologyEdges } from './edgeBuilder';
import {
  buildIncrementalLayoutInputs,
  computeIncrementalLayoutPositions,
} from './incrementalLayout';
import { buildAlertsPanelModel } from './panelAdapters';
import { buildRuntimeState } from './runtimeAdapters';
import {
  buildRuntimePatchPlan,
  hasRuntimePatchWork,
  patchRuntimeDevices,
  patchRuntimeEdges,
  patchRuntimeNodes,
} from './runtimePatches';
import { composeCanvasTopology } from './topologyComposer';
import { buildTopologyIdentity, collectPlacementDeviceIds } from './topologyIdentity';

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
  updateNodePosition: (deviceId: string, position: { x: number; y: number }) => void;
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

type CanvasTopologySource =
  | {
      status: 'ok';
      devices: Device[];
      links: Link[];
      positions: Map<string, PositionState>;
      etag?: string;
      topologyVersion?: string;
      runtimeVersion?: string;
      schemaVersion?: number;
    }
  | {
      status: 'not-modified';
      etag?: string;
    };

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
  if (causes.has('backend-reconnected') || causes.has('backend-resync-required')) {
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
  return position !== undefined && Number.isFinite(position.x) && Number.isFinite(position.y);
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
      !savedPosition ||
      savedPosition.x !== position.x ||
      savedPosition.y !== position.y ||
      savedPosition.pinned !== position.pinned
    ) {
      return true;
    }
  }

  return false;
}

function toPositionMap(positions: Iterable<DevicePosition>): Map<string, PositionState> {
  return new Map(
    Array.from(positions).map((position) => [
      position.device_id,
      {
        x: position.x,
        y: position.y,
        pinned: position.pinned,
      },
    ]),
  );
}

function isCanvasTopologyUnsupported(error: unknown): boolean {
  if (typeof error !== 'object' || error === null || !('status' in error)) {
    return false;
  }

  const status = (error as { status?: unknown }).status;
  return status === 404 || status === 405 || status === 501;
}

async function loadCanvasTopologySource(
  fetchPositions: () => Promise<Map<string, PositionState>>,
  etag: string | null,
): Promise<CanvasTopologySource> {
  try {
    const result = await fetchCanvasTopology(etag ?? undefined);
    if (result.status === 'not-modified') {
      return {
        status: 'not-modified',
        etag: result.etag,
      };
    }

    return {
      status: 'ok',
      devices: result.topology.devices,
      links: result.topology.links,
      positions: toPositionMap(Object.values(result.topology.positions)),
      etag: result.etag,
      topologyVersion: result.topology.topology_version,
      runtimeVersion: result.topology.runtime_version,
      schemaVersion: result.topology.schema_version,
    };
  } catch (error) {
    if (!isCanvasTopologyUnsupported(error)) {
      throw error;
    }
  }

  const [devices, links, positions] = await Promise.all([
    fetchDevices(),
    fetchLinks(),
    fetchPositions(),
  ]);

  return {
    status: 'ok',
    devices,
    links,
    positions,
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
      width: currentNode.width,
      height: currentNode.height,
      initialWidth: currentNode.initialWidth,
      initialHeight: currentNode.initialHeight,
      measured: currentNode.measured,
      data: {
        ...node.data,
        highlighted: currentNode.data.highlighted,
      },
    };
  });
}

function nowMs(): number {
  return typeof performance !== 'undefined' && typeof performance.now === 'function'
    ? performance.now()
    : Date.now();
}

function roundDurationMs(durationMs: number): number {
  return Number(Math.max(0, durationMs).toFixed(3));
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
  const lastAppliedRuntimeSnapshotRef = useRef<SnapshotPayload | null>(null);
  const alertsRef = useRef<AlertDTO[]>(alerts);
  const devicesRef = useRef<Device[]>([]);
  const topologyLinksRef = useRef<Link[]>([]);
  const nodesRef = useRef<DeviceNode[]>(nodes);
  const lastTopologyIdentityRef = useRef<string | null>(null);
  const lastCanvasTopologyEtagRef = useRef<string | null>(null);
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
    const alertsPanelModel = buildAlertsPanelModel({ alerts, runtimeState });

    return {
      alertCount: alertsPanelModel.activeAlertCount,
      prometheusDiagnosticsVisible: isPrometheusUnavailable(prometheusStatus),
    };
  }, [alerts, devices, prometheusStatus, snapshot, topologyLinks]);

  const [topologyRecoveryNotice, setTopologyRecoveryNotice] =
    useState<TopologyRecoveryNotice | null>(null);
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
    ) =>
      measureCanvasAsyncWork('theia:canvas:topology-load', trigger, async () => {
        const loadStartedAt = nowMs();
        updateCanvasDiagnosticsState({
          topology: {
            lastTopologyLoadReason: trigger,
            lastTopologyLoadStatus: 'loading',
            lastTopologyLoadError: undefined,
          },
        });
        recordCanvasDiagnosticEvent({
          level: 'info',
          source: 'topology',
          event: 'topology.load.started',
          message: 'Canvas topology load started',
          metadata: {
            reason: trigger,
            silent: isSilentRefresh,
          },
        });

        if (!isSilentRefresh) {
          setLoading(true);
        }
        setError(null);

        try {
          const topologySource = await loadCanvasTopologySource(
            fetchPositions,
            lastCanvasTopologyEtagRef.current,
          );
          if (topologySource.status === 'not-modified') {
            lastCanvasTopologyEtagRef.current =
              topologySource.etag ?? lastCanvasTopologyEtagRef.current;
            updateCanvasDiagnosticsState({
              topology: {
                lastTopologyLoadAt: new Date().toISOString(),
                lastTopologyLoadReason: trigger,
                lastTopologyLoadDurationMs: roundDurationMs(nowMs() - loadStartedAt),
                lastTopologyLoadStatus: 'success',
                lastTopologyLoadError: undefined,
              },
            });
            recordCanvasDiagnosticEvent({
              level: 'info',
              source: 'topology',
              event: 'topology.load.succeeded',
              message: 'Canvas topology read model not modified',
              metadata: {
                reason: trigger,
                notModified: true,
              },
            });
            return;
          }

          lastCanvasTopologyEtagRef.current = topologySource.etag ?? null;
          const fetchedDevices = topologySource.devices;
          const fetchedLinks = topologySource.links;
          const savedPositions = topologySource.positions;
          updateCanvasDiagnosticsState({
            topology: {
              topologyVersion: topologySource.topologyVersion,
              runtimeVersion: topologySource.runtimeVersion,
              schemaVersion: topologySource.schemaVersion,
            },
            graph: {
              canonicalNodeCount: fetchedDevices.length,
              canonicalEdgeCount: fetchedLinks.length,
            },
          });

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
                const results = await Promise.allSettled(
                  storedManual.map((edge) =>
                    createLink({
                      source_device_id: edge.source,
                      source_if_name: '',
                      target_device_id: edge.target,
                      target_if_name: '',
                    }),
                  ),
                );
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
            lastAppliedRuntimeSnapshotRef.current = snapshotRef.current;
            lastTopologyIdentityRef.current = topologyIdentity.signature;
            lastUsablePositionStateRef.current = usablePositionState;
            updateCanvasDiagnosticsState({
              topology: {
                lastTopologyLoadAt: new Date().toISOString(),
                lastTopologyLoadReason: trigger,
                lastTopologyLoadDurationMs: roundDurationMs(nowMs() - loadStartedAt),
                lastTopologyLoadStatus: 'success',
                lastTopologyLoadError: undefined,
              },
              graph: {
                canonicalNodeCount: fetchedDevices.length,
                canonicalEdgeCount: fetchedLinks.length,
              },
              layout: {
                pendingLayout: false,
              },
            });
            recordCanvasDiagnosticEvent({
              level: 'info',
              source: 'topology',
              event: 'topology.load.succeeded',
              message: 'Canvas topology load succeeded',
              metadata: {
                reason: trigger,
                deviceCount: fetchedDevices.length,
                linkCount: fetchedLinks.length,
                positionCount: savedPositions.size,
                placementDeviceCount: 0,
                structureChanged,
              },
            });
            return;
          }

          const placementDeviceIds = collectPlacementDeviceIds(
            fetchedDevices,
            currentNodePositionsRef.current,
            savedPositions,
            currentNodePositionsRef.current.keys(),
          );
          const { width, height } = viewportSize();
          const { layoutNodes, layoutEdges } = buildIncrementalLayoutInputs({
            devices: fetchedDevices,
            links: fetchedLinks,
            placementDeviceIds,
            effectivePositions,
          });
          const computedPositions =
            layoutNodes.length > 0
              ? measureCanvasWork('theia:canvas:layout', trigger, () => {
                  const layoutStartedAt = nowMs();
                  updateCanvasDiagnosticsState({
                    layout: {
                      pendingLayout: true,
                      lastLayoutReason: trigger,
                      lastLayoutNodeCount: layoutNodes.length,
                    },
                  });
                  recordCanvasDiagnosticEvent({
                    level: 'debug',
                    source: 'layout',
                    event: 'layout.started',
                    message: 'Canvas incremental layout started',
                    metadata: {
                      reason: trigger,
                      nodeCount: layoutNodes.length,
                      edgeCount: layoutEdges.length,
                    },
                  });
                  const positions = computeIncrementalLayoutPositions({
                    layoutNodes,
                    layoutEdges,
                    placementDeviceIds,
                    width,
                    height,
                  });
                  updateCanvasDiagnosticsState({
                    layout: {
                      lastLayoutAt: new Date().toISOString(),
                      lastLayoutDurationMs: roundDurationMs(nowMs() - layoutStartedAt),
                      lastLayoutNodeCount: layoutNodes.length,
                      lastLayoutReason: trigger,
                      pendingLayout: false,
                    },
                  });
                  recordCanvasDiagnosticEvent({
                    level: 'info',
                    source: 'layout',
                    event: 'layout.completed',
                    message: 'Canvas incremental layout completed',
                    metadata: {
                      reason: trigger,
                      nodeCount: layoutNodes.length,
                    },
                  });
                  return positions;
                })
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
          lastAppliedRuntimeSnapshotRef.current = snapshotRef.current;

          const nextPositionPayload = buildPositionPayload(composedNodes);
          if (positionsChanged(nextPositionPayload, savedPositions)) {
            void savePositions(nextPositionPayload);
          }

          if (trigger === 'initial_load' || shouldAutoFitView) {
            window.requestAnimationFrame(() => {
              reactFlow.fitView({ padding: topologyFitViewPadding, duration: 320 });
            });
          }

          lastTopologyIdentityRef.current = topologyIdentity.signature;
          lastUsablePositionStateRef.current = usablePositionState;
          updateCanvasDiagnosticsState({
            topology: {
              lastTopologyLoadAt: new Date().toISOString(),
              lastTopologyLoadReason: trigger,
              lastTopologyLoadDurationMs: roundDurationMs(nowMs() - loadStartedAt),
              lastTopologyLoadStatus: 'success',
              lastTopologyLoadError: undefined,
            },
            graph: {
              canonicalNodeCount: fetchedDevices.length,
              canonicalEdgeCount: fetchedLinks.length,
            },
            layout: {
              pendingLayout: false,
            },
          });
          recordCanvasDiagnosticEvent({
            level: 'info',
            source: 'topology',
            event: 'topology.load.succeeded',
            message: 'Canvas topology load succeeded',
            metadata: {
              reason: trigger,
              deviceCount: fetchedDevices.length,
              linkCount: fetchedLinks.length,
              positionCount: savedPositions.size,
              placementDeviceCount: placementDeviceIds.size,
              structureChanged,
            },
          });
        } catch (loadError) {
          const topologyError =
            loadError instanceof Error ? loadError : new Error('Failed to load topology');
          updateCanvasDiagnosticsState({
            topology: {
              lastTopologyLoadAt: new Date().toISOString(),
              lastTopologyLoadReason: trigger,
              lastTopologyLoadDurationMs: roundDurationMs(nowMs() - loadStartedAt),
              lastTopologyLoadStatus: 'error',
              lastTopologyLoadError: topologyError.message,
            },
            layout: {
              pendingLayout: false,
            },
          });
          recordCanvasDiagnosticEvent({
            level: 'error',
            source: 'topology',
            event: 'topology.load.failed',
            message: 'Canvas topology load failed',
            metadata: {
              reason: trigger,
              error: topologyError.message,
            },
          });

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
    [
      editMode,
      openDeviceMenu,
      openEdgeMenu,
      openSelfLinkDetails,
      reactFlow,
      setNodes,
      setEdges,
      fetchPositions,
      savePositions,
    ],
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
        await loadTopology(true, undefined, measurementTriggerForCauses(refreshCauses), {
          suppressBlockingError: true,
          rethrowOnError: true,
        });
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
    const retryCauses =
      lastStructuralRefreshCausesRef.current.size > 0
        ? new Set(lastStructuralRefreshCausesRef.current)
        : new Set<StructuralRefreshCause>(['topology-changed']);
    void runStructuralRefresh(retryCauses);
  }, [runStructuralRefresh]);

  const updateNodePosition = useCallback(
    (deviceId: string, position: { x: number; y: number }) => {
      let changed = false;
      const nextNodes = nodesRef.current.map((node) =>
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
      changed = nextNodes.some((node, index) => node !== nodesRef.current[index]);
      if (!changed) {
        return;
      }

      const devicesById = new Map(devicesRef.current.map((device) => [device.id, device]));
      const links = topologyLinksRef.current;

      setNodes(nextNodes);
      setEdges((currentEdges) => {
        const existingEdgeData = new Map(currentEdges.map((edge) => [edge.id, edge.data ?? {}]));
        return buildTopologyEdges(links, devicesById, nextNodes, existingEdgeData, openEdgeMenu);
      });
      void savePositions(buildPositionPayload(nextNodes));
    },
    [openEdgeMenu, savePositions, setEdges, setNodes],
  );

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

  useEffect(() => {
    window.__THEIA_CANVAS_FORCE_REFRESH__ = () => {
      void loadTopology(true, undefined, 'manual_refresh');
    };

    return () => {
      if (window.__THEIA_CANVAS_FORCE_REFRESH__) {
        window.__THEIA_CANVAS_FORCE_REFRESH__ = undefined;
      }
    };
  }, [loadTopology]);

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
      setTopologyRecoveryNotice((current) => (current?.tone === 'success' ? null : current));
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
      if (devicesRef.current.length === 0) {
        return;
      }

      const patchPlan = buildRuntimePatchPlan({
        previousSnapshot: lastAppliedRuntimeSnapshotRef.current,
        nextSnapshot: snapshot,
        links: topologyLinksRef.current,
      });
      lastAppliedRuntimeSnapshotRef.current = snapshot;

      if (!hasRuntimePatchWork(patchPlan)) {
        return;
      }

      const runtimeState = buildRuntimeState({
        devices: devicesRef.current,
        links: topologyLinksRef.current,
        snapshot,
        alerts: alertsRef.current,
        prometheusStatus,
      });

      setNodes((currentNodes) =>
        patchRuntimeNodes({
          nodes: currentNodes,
          runtimeState,
          plan: patchPlan,
        }),
      );
      setEdges((currentEdges) =>
        patchRuntimeEdges({
          edges: currentEdges,
          links: topologyLinksRef.current,
          runtimeState,
          alerts: alertsRef.current,
          onEdgeContextMenu: openEdgeMenu,
          plan: patchPlan,
        }),
      );
      setDevices((currentDevices) =>
        patchRuntimeDevices({
          devices: currentDevices,
          runtimeState,
          plan: patchPlan,
        }),
      );
    });
  }, [openEdgeMenu, prometheusStatus, setEdges, setNodes, snapshot]);

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
        const alertStatus = edge.data?.link
          ? alertStatusForLink(edge.data.link, alerts)
          : undefined;
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
    updateNodePosition,
  };
}
