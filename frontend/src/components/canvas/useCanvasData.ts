import { useCallback, useEffect, useRef, useState } from 'react';
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
  resolveDeviceMonitoringState,
  sanitizeDeviceMetricsForDisplay,
} from '../deviceVisualState';
import {
  buildPositionPayload,
  buildThroughputLabel,
  findLinkMetrics,
  manualEdgeStorageKey,
  staleThresholdMs,
  viewportSize,
} from './canvasHelpers';
import { measureCanvasAsyncWork, measureCanvasWork, type CanvasMeasurementTrigger } from './canvasInstrumentation';
import { alertStatusForLink, buildTopologyEdges } from './edgeBuilder';
import { buildTopologyNodes } from './nodeBuilder';
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
  prometheusAlertDismissed: boolean;
  setPrometheusAlertDismissed: React.Dispatch<React.SetStateAction<boolean>>;
  showRecoveryToast: boolean;
  setShowRecoveryToast: React.Dispatch<React.SetStateAction<boolean>>;
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

interface LoadTopologyOptions {
  suppressBlockingError?: boolean;
  rethrowOnError?: boolean;
}

const structuralRefreshDebounceMs = 250;
const topologyRefreshRetryActionLabel = 'Retry topology refresh';
const topologyRefreshDelayedMessage = 'Live topology refresh delayed';
const emptyAlerts: AlertDTO[] = [];

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

function buildEffectiveSnapshot(
  fetchedDevices: Device[],
  pendingSnapshot: SnapshotPayload | null,
  prometheusStatus: PrometheusStatusPayload | null,
): SnapshotPayload | null {
  if (!pendingSnapshot) {
    return null;
  }

  if (!isPrometheusUnavailable(prometheusStatus)) {
    return pendingSnapshot;
  }

  const effectiveStatuses = { ...pendingSnapshot.device_statuses };
  for (const device of fetchedDevices) {
    const source = device.metrics_source || 'prometheus';
    if (source === 'prometheus' || source === 'prometheus_snmp_fallback') {
      effectiveStatuses[device.id] = 'down';
    }
  }

  return {
    ...pendingSnapshot,
    device_statuses: effectiveStatuses,
  };
}

function mergeRuntimeDeviceFields(
  device: Device,
  currentDevice: Device | undefined,
  effectiveSnapshot: SnapshotPayload | null,
): Device {
  const baseDevice = currentDevice
    ? {
        ...device,
        status: currentDevice.status,
        sys_name: currentDevice.sys_name,
        hardware_model: currentDevice.hardware_model,
      }
    : device;

  if (!effectiveSnapshot) {
    return baseDevice;
  }

  const snapshotStatus = effectiveSnapshot.device_statuses[device.id];

  if (!snapshotStatus) {
    return baseDevice;
  }

  return {
    ...baseDevice,
    ...(snapshotStatus ? { status: snapshotStatus as Device['status'] } : {}),
  };
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
  const prometheusStatusRef = useRef<PrometheusStatusPayload | null>(null);
  const alertsRef = useRef<AlertDTO[]>(alerts);
  const devicesRef = useRef<Device[]>([]);
  const lastSnapshotTimeRef = useRef<number | null>(null);
  const staleAppliedRef = useRef(false);
  const lastTopologyIdentityRef = useRef<string | null>(null);
  const lastUsablePositionStateRef = useRef('');
  const currentNodePositionsRef = useRef<Map<string, PositionState>>(new Map());
  const grafanaUrlRef = useRef<string>('');
  const deviceGrafanaUrlsRef = useRef<Map<string, string>>(new Map());
  const [prometheusAlertDismissed, setPrometheusAlertDismissed] = useState(false);

  const { fetchPositions, savePositions } = usePositions();

  // Track Prometheus recovery transition and auto-dismiss recovery toast.
  const prevPromDownRef = useRef<boolean | null>(null);
  const [showRecoveryToast, setShowRecoveryToast] = useState(false);
  const [topologyRecoveryNotice, setTopologyRecoveryNotice] = useState<TopologyRecoveryNotice | null>(null);
  const structuralRefreshTimerRef = useRef<number | null>(null);
  const pendingStructuralRefreshCausesRef = useRef<Set<StructuralRefreshCause>>(new Set());
  const lastStructuralRefreshCausesRef = useRef<Set<StructuralRefreshCause>>(new Set());

  // Keep refs in sync so async loadTopology and snapshot effect can read the latest state
  useEffect(() => {
    snapshotRef.current = snapshot;
  }, [snapshot]);
  useEffect(() => {
    prometheusStatusRef.current = prometheusStatus;
  }, [prometheusStatus]);
  useEffect(() => {
    alertsRef.current = alerts;
  }, [alerts]);
  devicesRef.current = devices;
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

        const devicesByID = new Map(fetchedDevices.map((device) => [device.id, device]));
        const linksByID = new Map(fetchedLinks.map((link) => [link.id, link]));
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
        const effectiveSnapshot = buildEffectiveSnapshot(
          fetchedDevices,
          snapshotRef.current,
          prometheusStatusRef.current,
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
          setDevices(fetchedDevices);
          setTopologyLinks(fetchedLinks);
          setNodes((currentNodes) =>
            currentNodes.map((node) => {
              const fetchedDevice = devicesByID.get(node.id);
              if (!fetchedDevice) {
                return node;
              }

              const nextDevice = mergeRuntimeDeviceFields(
                fetchedDevice,
                node.data.device,
                effectiveSnapshot,
              );
              const nextMetrics = effectiveSnapshot
                ? sanitizeDeviceMetricsForDisplay(
                    nextDevice,
                    effectiveSnapshot.device_metrics[node.id] ?? null,
                  )
                : node.data.metrics;

              return {
                ...node,
                data: {
                  ...node.data,
                  device: nextDevice,
                  editMode,
                  onContextMenu: openDeviceMenu,
                  alertStatus: alertStatusForDevice(node.id, alertsRef.current),
                  metrics: nextMetrics,
                  monitoringState: resolveDeviceMonitoringState(nextDevice),
                  isVirtual: fetchedDevice.device_type === 'virtual',
                  subtype: fetchedDevice.device_type === 'virtual'
                    ? (nextDevice.tags?.virtual_subtype ?? 'generic')
                    : undefined,
                },
              };
            }),
          );
          setEdges((currentEdges) =>
            currentEdges.map((edge) => {
              const fetchedLink = linksByID.get(edge.id);
              if (!fetchedLink) {
                return edge;
              }

              if (!effectiveSnapshot) {
                return {
                  ...edge,
                  data: edge.data
                    ? {
                        ...edge.data,
                        link: fetchedLink,
                      }
                    : edge.data,
                };
              }

              const sourceDeviceStatus = effectiveSnapshot.device_statuses[fetchedLink.source_device_id];
              const targetDeviceStatus = effectiveSnapshot.device_statuses[fetchedLink.target_device_id];
              const bothDown = sourceDeviceStatus === 'down' && targetDeviceStatus === 'down';
              const metrics = bothDown
                ? null
                : findLinkMetrics(effectiveSnapshot.link_metrics, fetchedLink);

              return {
                ...edge,
                data: edge.data
                  ? {
                      ...edge.data,
                      link: fetchedLink,
                      alertStatus: alertStatusForLink(fetchedLink, alertsRef.current),
                      sourceDeviceStatus,
                      targetDeviceStatus,
                      metrics,
                      throughputLabel: metrics ? buildThroughputLabel(metrics) : undefined,
                      utilization: metrics?.utilization ?? null,
                    }
                  : edge.data,
              };
            }),
          );
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

        const nextNodes = buildTopologyNodes(
          fetchedDevices,
          effectivePositions,
          computedPositions,
          defaultPosition,
          editMode,
          openDeviceMenu,
          effectiveSnapshot,
          alertsRef.current,
          fetchedLinks,
          openSelfLinkDetails,
          currentNodePositionsRef.current,
          placementDeviceIds,
        );

        let nextEdges = buildTopologyEdges(fetchedLinks, devicesByID, nextNodes, undefined, openEdgeMenu, alertsRef.current);

        // Merge pending snapshot link metrics into edges and mark snapshot time
        // so the stale-data timer doesn't immediately clear the metrics.
        if (effectiveSnapshot) {
          lastSnapshotTimeRef.current = Date.now();
          nextEdges = nextEdges.map((edge) => {
            const link = edge.data?.link;
            if (!link) return edge;
            const srcStatus = effectiveSnapshot.device_statuses[link.source_device_id];
            const tgtStatus = effectiveSnapshot.device_statuses[link.target_device_id];
            const bothDown = srcStatus === 'down' && tgtStatus === 'down';
            const metrics = bothDown ? null : findLinkMetrics(effectiveSnapshot.link_metrics, link);
            return {
              ...edge,
              data: {
                ...edge.data!,
                alertStatus: alertStatusForLink(link, alertsRef.current),
                sourceDeviceStatus: srcStatus,
                targetDeviceStatus: tgtStatus,
                metrics: metrics,
                throughputLabel: metrics ? buildThroughputLabel(metrics) : undefined,
                utilization: metrics?.utilization ?? null,
              },
            };
          });
        }

        // Apply all state updates together as urgent (not in startTransition).
        // Previously these were wrapped in startTransition which made them
        // low-priority, allowing WebSocket snapshot effects (which depend on
        // devices.length) to interrupt and race with the transition's setNodes,
        // sometimes causing all canvas nodes to vanish after a device delete.
        setDevices(fetchedDevices);
        setTopologyLinks(fetchedLinks);
        setNodes((currentNodes) => {
          const prevNodesByID = new Map(currentNodes.map((node) => [node.id, node]));
          return nextNodes.map((node) => {
            const previousNode = prevNodesByID.get(node.id);
            if (!previousNode) {
              return node;
            }

            return {
              ...node,
              selected: previousNode.selected,
              dragging: previousNode.dragging,
              data: {
                ...node.data,
                highlighted: previousNode.data.highlighted,
                alertStatus: node.data.alertStatus,
              },
            };
          });
        });
        setEdges(nextEdges);

        const nextPositionPayload = buildPositionPayload(nextNodes);
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

  // Prometheus recovery toast effect
  useEffect(() => {
    if (prometheusStatus === null) return;

    const promDown = isPrometheusUnavailable(prometheusStatus);

    // Detect recovery only for configured Prometheus connections.
    if (prevPromDownRef.current === true && !promDown && prometheusStatus.enabled !== false && prometheusStatus.available) {
      setShowRecoveryToast(true);
      setPrometheusAlertDismissed(false);
      const timer = window.setTimeout(() => setShowRecoveryToast(false), 8000);
      prevPromDownRef.current = promDown;
      return () => { window.clearTimeout(timer); };
    }

    if (!promDown) {
      setPrometheusAlertDismissed(false);
    }

    prevPromDownRef.current = promDown;
  }, [prometheusStatus?.enabled, prometheusStatus?.available]);

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
      lastSnapshotTimeRef.current = Date.now();
      staleAppliedRef.current = false;

      // Compute effective device statuses: override prometheus-only devices to 'down'
      // when Prometheus is unreachable (no probe_success data available).
      const promDown = isPrometheusUnavailable(prometheusStatus);
      const effectiveStatuses = { ...snapshot.device_statuses };

      if (promDown) {
        // Use ref to always read the latest devices without adding devices.length
        // to the dependency array (which would cause redundant re-triggers after
        // loadTopology).
        for (const d of devicesRef.current) {
          const src = d.metrics_source || 'prometheus';
          if (src === 'prometheus' || src === 'prometheus_snmp_fallback') {
            effectiveStatuses[d.id] = 'down';
          }
          // 'none' and 'snmp' sources are unaffected by Prometheus availability
        }
      }

      // Apply all snapshot data (status and metrics) in a single
      // urgent update so the first snapshot on page load renders immediately
      // instead of being split across an urgent + deferred transition pass.
      setNodes((currentNodes) =>
        currentNodes.map((node) => {
          const newStatus = effectiveStatuses[node.id];
          const updatedDevice = newStatus
            ? {
                ...node.data.device,
                ...(newStatus ? { status: newStatus as Device['status'] } : {}),
              }
            : node.data.device;

          // Preserve derived runtime metadata like health and freshness cadence
          // even when device status is down.
          const nodeMetrics = sanitizeDeviceMetricsForDisplay(
            updatedDevice,
            snapshot.device_metrics[node.id] ?? null,
          );

          return {
            ...node,
            data: {
              ...node.data,
              alertStatus: alertStatusForDevice(node.id, alertsRef.current),
              device: updatedDevice,
              monitoringState: resolveDeviceMonitoringState(updatedDevice),
              metrics: nodeMetrics,
            },
          };
        }),
      );

      // Sync devices state with runtime statuses so panels (e.g. LinkCreatePanel) see them.
      if (Object.keys(effectiveStatuses).length > 0) {
        setDevices((prev) =>
          prev.map((d) => {
            const newStatus = effectiveStatuses[d.id];
            if (!newStatus) return d;
            return {
              ...d,
              ...(newStatus ? { status: newStatus as Device['status'] } : {}),
            };
          }),
        );
      }

      setEdges((currentEdges) =>
        currentEdges.map((edge) => {
          const link = edge.data?.link;
          if (!link) return edge;
          const srcStatus = effectiveStatuses[link.source_device_id];
          const tgtStatus = effectiveStatuses[link.target_device_id];

          // When both link endpoints are down, null out link metrics so throughput
          // labels don't show stale last-known values.
          const bothDown = srcStatus === 'down' && tgtStatus === 'down';
          const metrics = bothDown ? null : findLinkMetrics(snapshot.link_metrics, link);

          return {
            ...edge,
            data: {
              ...edge.data,
              alertStatus: alertStatusForLink(link, alertsRef.current),
              sourceDeviceStatus: srcStatus,
              targetDeviceStatus: tgtStatus,
              metrics: metrics,
              throughputLabel: metrics ? buildThroughputLabel(metrics) : undefined,
              utilization: metrics?.utilization ?? null,
            },
          };
        }),
      );
    });
  }, [snapshot, setNodes, prometheusStatus?.available]);

  // Stale data timer
  useEffect(() => {
    const interval = window.setInterval(() => {
      if (lastSnapshotTimeRef.current === null || staleAppliedRef.current) {
        return;
      }

      if (Date.now() - lastSnapshotTimeRef.current <= staleThresholdMs) {
        return;
      }

      staleAppliedRef.current = true;

      setNodes((currentNodes) =>
        currentNodes.map((node) => ({
          ...node,
          data: {
            ...node.data,
            // Local stale fallback blanks numeric values only; health and
            // derived freshness metadata remain intact.
            metrics: node.data.metrics
              ? {
                  ...node.data.metrics,
                  cpu_percent: null,
                  mem_percent: null,
                  temp_celsius: null,
                  uptime_secs: null,
                }
              : null,
            alertStatus: alertStatusForDevice(node.id, alertsRef.current),
          },
        })),
      );

      setEdges((currentEdges) =>
        currentEdges.map((edge) => ({
          ...edge,
          data: {
            ...edge.data,
            metrics: null,
            throughputLabel: undefined,
            utilization: null,
            alertStatus: edge.data?.link ? alertStatusForLink(edge.data.link, alertsRef.current) : undefined,
          },
        })),
      );
    }, 10_000);

    return () => {
      window.clearInterval(interval);
    };
  }, [setNodes]);

  useEffect(() => {
    setNodes((currentNodes) =>
      currentNodes.map((node) => ({
        ...node,
        data: {
          ...node.data,
          alertStatus: alertStatusForDevice(node.id, alerts),
        },
      })),
    );

    setEdges((currentEdges) =>
      currentEdges.map((edge) => ({
        ...edge,
        data: edge.data
          ? {
              ...edge.data,
              alertStatus: edge.data.link ? alertStatusForLink(edge.data.link, alerts) : undefined,
            }
          : edge.data,
      })),
    );
  }, [alerts, setEdges, setNodes]);

  return {
    devices,
    setDevices,
    topologyLinks,
    loading,
    error,
    loadTopology,
    grafanaUrlRef,
    deviceGrafanaUrlsRef,
    refreshSettings,
    prometheusAlertDismissed,
    setPrometheusAlertDismissed,
    showRecoveryToast,
    setShowRecoveryToast,
    topologyRecoveryNotice,
    dismissTopologyRecoveryNotice,
    retryTopologyRefresh,
  };
}
