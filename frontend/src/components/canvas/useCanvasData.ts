import type { ReactFlowInstance } from '@xyflow/react';
import { useCallback, useEffect, useMemo, useRef, useState } from 'react';

import {
  createLink,
  fetchCanvasBootstrap,
  fetchCanvasMapBootstrap,
  fetchCanvasMapTopology,
  fetchCanvasTopology,
  fetchDevices,
  fetchLinks,
  fetchSettings,
} from '../../api/client';
import { publishCanvasRuntimeBootstrap } from '../../hooks/canvasRuntimeBootstrap';
import { type PositionState, usePositions } from '../../hooks/usePositions';
import type { Area, Device, DevicePosition, Link } from '../../types/api';
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
  manualEdgeMigrationStorageKey,
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
import {
  type ManualEdgeMigrationResult,
  type ManualEdgeMigrationState,
  migrateStoredManualEdges,
  readManualEdgeMigrationState,
} from './manualEdgeMigration';
import { buildAlertsPanelModel } from './panelAdapters';
import { buildRuntimeState } from './runtimeAdapters';
import {
  buildRuntimePatchPlan,
  hasRuntimePatchWork,
  patchRuntimeEdges,
  patchRuntimeNodes,
} from './runtimePatches';
import { composeCanvasTopology } from './topologyComposer';
import { buildTopologyIdentity, collectPlacementDeviceIds } from './topologyIdentity';

interface UseCanvasDataParams {
  mapId: string | null;
  mapName?: string;
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
  onTopologyAreasChange?: (areas: Area[]) => void;
}

interface UseCanvasDataReturn {
  devices: Device[];
  setDevices: React.Dispatch<React.SetStateAction<Device[]>>;
  topologyLinks: Link[];
  topologyAreas: Area[];
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
  includeRuntimeBootstrap?: boolean;
  forceFitView?: boolean;
}

type LoadTopologyResult = 'applied' | 'stale' | 'failed';

type CanvasTopologySource =
  | {
      status: 'ok';
      devices: Device[];
      links: Link[];
      areas: Area[];
      positions: Map<string, PositionState>;
      etag?: string;
      topologyVersion?: string;
      runtimeVersion?: number;
      runtimeIdentity?: string;
      runtimeSnapshot?: SnapshotPayload;
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

function canvasMapKey(mapId: string | null): string {
  return mapId === null ? 'default:' : `map:${mapId}`;
}

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

function nodePositionsToPositionMap(nodes: DeviceNode[]): Map<string, PositionState> {
  return new Map(
    nodes.map((node) => [
      node.id,
      {
        x: node.position.x,
        y: node.position.y,
        pinned: node.data.pinned ?? false,
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
  mapId: string | null,
  fetchPositions: () => Promise<Map<string, PositionState>>,
  etag: string | null,
  includeRuntimeBootstrap = false,
  forceRuntimeBootstrap = false,
): Promise<CanvasTopologySource> {
  try {
    if (includeRuntimeBootstrap) {
      const result =
        mapId === null
          ? await fetchCanvasBootstrap({ force: forceRuntimeBootstrap })
          : await fetchCanvasMapBootstrap(mapId, { force: forceRuntimeBootstrap });
      const topology = result.topology;
      return {
        status: 'ok',
        devices: topology.devices,
        links: topology.links,
        areas: topology.areas,
        positions: toPositionMap(Object.values(topology.positions)),
        topologyVersion: topology.topology_version,
        runtimeVersion: topology.runtime_version,
        runtimeIdentity: topology.runtime_identity,
        runtimeSnapshot: topology.runtime_snapshot,
        schemaVersion: topology.schema_version,
      };
    }

    const result =
      mapId === null
        ? await fetchCanvasTopology(etag ?? undefined)
        : await fetchCanvasMapTopology(mapId, etag ?? undefined);
    if (result.status === 'not-modified') {
      return {
        status: 'not-modified',
        etag: result.etag,
      };
    }

    const topology = result.topology;
    return {
      status: 'ok',
      devices: topology.devices,
      links: topology.links,
      areas: topology.areas,
      positions: toPositionMap(Object.values(topology.positions)),
      etag: result.etag,
      topologyVersion: topology.topology_version,
      runtimeVersion: topology.runtime_version,
      runtimeIdentity: topology.runtime_identity,
      runtimeSnapshot: topology.runtime_snapshot,
      schemaVersion: topology.schema_version,
    };
  } catch (error) {
    if (includeRuntimeBootstrap && isCanvasTopologyUnsupported(error)) {
      return loadCanvasTopologySource(mapId, fetchPositions, etag, false);
    }
    if (!isCanvasTopologyUnsupported(error)) {
      throw error;
    }
    if (mapId !== null) {
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
    areas: [],
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

function manualEdgeMigrationHasVisibleResult(
  result: ManualEdgeMigrationResult,
  hadPendingStorage: boolean,
): boolean {
  return (
    hadPendingStorage ||
    result.attemptedCount > 0 ||
    result.appliedCount > 0 ||
    result.failedCount > 0 ||
    result.skippedCount > 0 ||
    manualEdgeMigrationStateHasVisibleResult(result.state)
  );
}

function manualEdgeMigrationStateHasVisibleResult(state: ManualEdgeMigrationState): boolean {
  return (
    state.status !== 'idle' ||
    state.attempt_count > 0 ||
    state.pending_count > 0 ||
    state.applied_count > 0 ||
    state.failed_count > 0 ||
    state.skipped_count > 0 ||
    state.last_error !== undefined
  );
}

function updateManualEdgeMigrationDiagnosticsState(state: ManualEdgeMigrationState): void {
  updateCanvasDiagnosticsState({
    manualEdgeMigration: {
      status: state.status,
      pendingCount: state.pending_count,
      appliedCount: state.applied_count,
      failedCount: state.failed_count,
      skippedCount: state.skipped_count,
      attemptCount: state.attempt_count,
      lastAttemptAt: state.last_attempt_at,
      lastCompletedAt: state.last_completed_at,
      lastError: state.last_error,
    },
  });
}

function recordPersistedManualEdgeMigrationDiagnostics(storage: Pick<Storage, 'getItem'>): void {
  if (storage.getItem(manualEdgeMigrationStorageKey) === null) {
    return;
  }

  const state = readManualEdgeMigrationState(storage, manualEdgeMigrationStorageKey);
  if (manualEdgeMigrationStateHasVisibleResult(state)) {
    updateManualEdgeMigrationDiagnosticsState(state);
  }
}

function recordManualEdgeMigrationDiagnostics(
  result: ManualEdgeMigrationResult,
  hadPendingStorage: boolean,
): void {
  if (!manualEdgeMigrationHasVisibleResult(result, hadPendingStorage)) {
    return;
  }

  const state = result.state;
  updateManualEdgeMigrationDiagnosticsState(state);

  const metadata = {
    status: state.status,
    attemptCount: state.attempt_count,
    pendingCount: state.pending_count,
    appliedCount: result.appliedCount,
    failedCount: result.failedCount,
    skippedCount: result.skippedCount,
  };

  if (result.appliedCount > 0) {
    recordCanvasDiagnosticEvent({
      level: 'info',
      source: 'topology',
      event: 'manual_edges.migration.applied',
      message: 'Manual edge localStorage migration applied',
      metadata,
    });
  }

  if (result.failedCount > 0) {
    recordCanvasDiagnosticEvent({
      level: 'warn',
      source: 'topology',
      event: 'manual_edges.migration.failed',
      message: 'Manual edge localStorage migration failed',
      metadata: {
        ...metadata,
        error: state.last_error,
      },
    });
  }

  if (result.skippedCount > 0) {
    recordCanvasDiagnosticEvent({
      level: 'info',
      source: 'topology',
      event: 'manual_edges.migration.skipped',
      message: 'Manual edge localStorage migration skipped existing links',
      metadata,
    });
  }
}

export function useCanvasData({
  mapId,
  mapName,
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
  onTopologyAreasChange,
}: UseCanvasDataParams): UseCanvasDataReturn {
  const mapKey = canvasMapKey(mapId);
  const diagnosticMapId = mapId ?? 'default';
  const diagnosticMapName = mapName ?? 'Default';
  const [devices, setDevices] = useState<Device[]>([]);
  const [topologyLinks, setTopologyLinks] = useState<Link[]>([]);
  const [topologyAreas, setTopologyAreas] = useState<Area[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  const snapshotRef = useRef<SnapshotPayload | null>(null);
  const lastAppliedRuntimeSnapshotRef = useRef<SnapshotPayload | null>(null);
  const alertsRef = useRef<AlertDTO[]>(alerts);
  const devicesRef = useRef<Device[]>([]);
  const topologyLinksRef = useRef<Link[]>([]);
  const nodesRef = useRef<DeviceNode[]>(nodes);
  const activeMapKeyRef = useRef<string>(mapKey);
  const mountedMapKeyRef = useRef<string | null>(null);
  const topologyLoadSequenceRef = useRef(0);
  const nodesOwnerMapKeyRef = useRef<string>(mapKey);
  const lastTopologyIdentityByMapRef = useRef<Map<string, string | null>>(new Map());
  const lastCanvasTopologyEtagByMapRef = useRef<Map<string, string | null>>(new Map());
  const lastUsablePositionStateByMapRef = useRef<Map<string, string>>(new Map());
  const currentNodePositionsByMapRef = useRef<Map<string, Map<string, PositionState>>>(new Map());
  const skippedSavedMapManualEdgeMigrationRef = useRef<Set<string>>(new Set());
  const grafanaUrlRef = useRef<string>('');
  const deviceGrafanaUrlsRef = useRef<Map<string, string>>(new Map());
  const { fetchPositions, savePositions } = usePositions(mapId);

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
  activeMapKeyRef.current = mapKey;
  currentNodePositionsByMapRef.current.set(
    nodesOwnerMapKeyRef.current,
    nodePositionsToPositionMap(nodes),
  );

  // Propagate device state changes to parent (for Dashboard view)
  useEffect(() => {
    onDevicesChange?.(devices);
  }, [devices, onDevicesChange]);

  // Propagate link state changes to parent (for Hub view)
  useEffect(() => {
    onLinksChange?.(topologyLinks);
  }, [topologyLinks, onLinksChange]);

  useEffect(() => {
    onTopologyAreasChange?.(topologyAreas);
  }, [topologyAreas, onTopologyAreasChange]);

  const loadTopology = useCallback(
    async (
      isSilentRefresh = false,
      defaultPosition?: { x: number; y: number },
      trigger: CanvasMeasurementTrigger = 'manual_refresh',
      options: LoadTopologyOptions = {},
    ): Promise<LoadTopologyResult> =>
      measureCanvasAsyncWork('theia:canvas:topology-load', trigger, async () => {
        const requestMapKey = mapKey;
        if (activeMapKeyRef.current !== requestMapKey) {
          return 'stale';
        }
        const requestSequence = topologyLoadSequenceRef.current + 1;
        topologyLoadSequenceRef.current = requestSequence;
        const isCurrentTopologyLoad = () =>
          topologyLoadSequenceRef.current === requestSequence &&
          activeMapKeyRef.current === requestMapKey;
        const requestFitViewAfterLoad = (duration = 320) => {
          window.requestAnimationFrame(() => {
            if (!isCurrentTopologyLoad()) {
              return;
            }
            reactFlow.fitView({ padding: topologyFitViewPadding, duration });
          });
        };

        const loadStartedAt = nowMs();
        const topologyLoadMetadata = {
          reason: trigger,
          silent: isSilentRefresh,
          mapId: diagnosticMapId,
          mapName: diagnosticMapName,
        };
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
          metadata: topologyLoadMetadata,
        });

        if (!isSilentRefresh) {
          setLoading(true);
        }
        setError(null);

        try {
          const includeRuntimeBootstrap =
            options.includeRuntimeBootstrap === true || trigger === 'initial_load';
          const forceRuntimeBootstrap = options.includeRuntimeBootstrap === true;
          const pendingManualEdgeStorageValue = window.localStorage.getItem(manualEdgeStorageKey);
          const hadPendingManualEdgeMigration = pendingManualEdgeStorageValue !== null;
          const canRunLegacyManualEdgeMigration = mapId === null;
          const shouldBypassReadModelEtagForManualEdgeMigration =
            canRunLegacyManualEdgeMigration && hadPendingManualEdgeMigration;
          const lastCanvasTopologyEtag = lastCanvasTopologyEtagByMapRef.current.get(mapKey) ?? null;
          const renderedNodesOwnedByMap = nodesOwnerMapKeyRef.current === mapKey;
          const topologyEtag =
            includeRuntimeBootstrap ||
            shouldBypassReadModelEtagForManualEdgeMigration ||
            !renderedNodesOwnedByMap
              ? null
              : lastCanvasTopologyEtag;
          const topologySource = await loadCanvasTopologySource(
            mapId,
            fetchPositions,
            topologyEtag,
            includeRuntimeBootstrap,
            forceRuntimeBootstrap,
          );
          if (!isCurrentTopologyLoad()) {
            return 'stale';
          }
          if (hadPendingManualEdgeMigration && !canRunLegacyManualEdgeMigration) {
            const skipDiagnosticKey = `${mapKey}:${pendingManualEdgeStorageValue}`;
            if (!skippedSavedMapManualEdgeMigrationRef.current.has(skipDiagnosticKey)) {
              skippedSavedMapManualEdgeMigrationRef.current.add(skipDiagnosticKey);
              recordCanvasDiagnosticEvent({
                level: 'info',
                source: 'topology',
                event: 'manual_edges.migration.skipped_saved_map',
                message: 'Manual edge localStorage migration skipped for saved map',
                metadata: { ...topologyLoadMetadata, mapId },
              });
            }
          }
          if (topologySource.status === 'not-modified') {
            lastCanvasTopologyEtagByMapRef.current.set(
              mapKey,
              topologySource.etag ?? lastCanvasTopologyEtag,
            );
            if (options.forceFitView === true) {
              requestFitViewAfterLoad();
            }
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
                ...topologyLoadMetadata,
                notModified: true,
              },
            });
            return 'applied';
          }

          lastCanvasTopologyEtagByMapRef.current.set(mapKey, topologySource.etag ?? null);
          const fetchedDevices = topologySource.devices;
          const fetchedLinks = topologySource.links;
          const fetchedAreas = topologySource.areas;
          const savedPositions = topologySource.positions;
          const runtimeSnapshot = topologySource.runtimeSnapshot ?? snapshotRef.current;
          if (topologySource.runtimeSnapshot !== undefined) {
            publishCanvasRuntimeBootstrap({
              snapshot: topologySource.runtimeSnapshot,
              runtimeVersion: topologySource.runtimeVersion,
              runtimeIdentity: topologySource.runtimeIdentity,
            });
            snapshotRef.current = topologySource.runtimeSnapshot;
          }
          updateCanvasDiagnosticsState({
            topology: {
              topologyVersion: topologySource.topologyVersion,
              runtimeVersion:
                topologySource.runtimeVersion === undefined
                  ? undefined
                  : String(topologySource.runtimeVersion),
              schemaVersion: topologySource.schemaVersion,
            },
            graph: {
              canonicalNodeCount: fetchedDevices.length,
              canonicalEdgeCount: fetchedLinks.length,
            },
          });

          if (hadPendingManualEdgeMigration && canRunLegacyManualEdgeMigration) {
            const manualEdgeMigrationResult = await migrateStoredManualEdges({
              storage: window.localStorage,
              pendingStorageKey: manualEdgeStorageKey,
              stateStorageKey: manualEdgeMigrationStorageKey,
              existingLinks: fetchedLinks,
              createLink,
            });
            if (!isCurrentTopologyLoad()) {
              return 'stale';
            }
            recordManualEdgeMigrationDiagnostics(
              manualEdgeMigrationResult,
              hadPendingManualEdgeMigration,
            );
            if (manualEdgeMigrationResult.appliedCount > 0) {
              lastCanvasTopologyEtagByMapRef.current.set(mapKey, null);
            }
          } else if (canRunLegacyManualEdgeMigration) {
            recordPersistedManualEdgeMigrationDiagnostics(window.localStorage);
          }

          const topologyIdentity = buildTopologyIdentity(fetchedDevices, fetchedLinks);
          const currentNodePositions =
            currentNodePositionsByMapRef.current.get(mapKey) ?? new Map();
          const structureChanged =
            lastTopologyIdentityByMapRef.current.get(mapKey) !== topologyIdentity.signature;
          const effectivePositions = new Map(savedPositions);
          for (const [deviceId, position] of currentNodePositions.entries()) {
            if (!effectivePositions.has(deviceId)) {
              effectivePositions.set(deviceId, position);
            }
          }
          // Backend reconnects can follow instance restore; fetched persisted
          // positions must override stale pre-restart canvas state.
          const currentPositionsForComposition =
            trigger === 'backend_reconnected'
              ? new Map<string, PositionState>()
              : currentNodePositions;

          const usablePositionState = buildUsablePositionState(
            fetchedDevices,
            currentNodePositions,
            savedPositions,
          );
          const shouldAutoFitView = usablePositionState.length === 0;
          const shouldFitViewAfterLoad =
            options.forceFitView === true || trigger === 'initial_load' || shouldAutoFitView;

          // Read any pending snapshot so first-load metrics are included in the
          // initial node/edge data -- eliminates the race where the WS snapshot
          // arrives before loadTopology resolves and the snapshot effect maps over
          // an empty node array.
          const runtimeState = buildRuntimeState({
            devices: fetchedDevices,
            links: fetchedLinks,
            snapshot: runtimeSnapshot,
            alerts: alertsRef.current,
            prometheusStatus,
          });

          if (!structureChanged) {
            setDevices(fetchedDevices);
            setTopologyLinks(fetchedLinks);
            setTopologyAreas(fetchedAreas);
            const { nodes: nextNodes, edges: nextEdges } = composeCanvasTopology({
              devices: fetchedDevices,
              links: fetchedLinks,
              runtimeState,
              savedPositions: effectivePositions,
              computedPositions: new Map(),
              currentPositions: currentPositionsForComposition,
              defaultPosition,
              editMode,
              openDeviceMenu,
              openEdgeMenu,
              openSelfLinkDetails,
              placementDeviceIds: new Set(),
              alerts: alertsRef.current,
            });
            nodesOwnerMapKeyRef.current = mapKey;
            currentNodePositionsByMapRef.current.set(mapKey, nodePositionsToPositionMap(nextNodes));
            setNodes((currentNodes) => mergeNodePresentationState(nextNodes, currentNodes));
            setEdges(nextEdges);
            lastAppliedRuntimeSnapshotRef.current = snapshotRef.current;
            lastTopologyIdentityByMapRef.current.set(mapKey, topologyIdentity.signature);
            lastUsablePositionStateByMapRef.current.set(mapKey, usablePositionState);
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
                ...topologyLoadMetadata,
                deviceCount: fetchedDevices.length,
                linkCount: fetchedLinks.length,
                positionCount: savedPositions.size,
                placementDeviceCount: 0,
                structureChanged,
              },
            });
            if (shouldFitViewAfterLoad) {
              requestFitViewAfterLoad();
            }
            return 'applied';
          }

          const placementDeviceIds = collectPlacementDeviceIds(
            fetchedDevices,
            currentNodePositions,
            savedPositions,
            currentNodePositions.keys(),
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
            currentPositions: currentPositionsForComposition,
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
          setDevices(fetchedDevices);
          setTopologyLinks(fetchedLinks);
          setTopologyAreas(fetchedAreas);
          nodesOwnerMapKeyRef.current = mapKey;
          currentNodePositionsByMapRef.current.set(
            mapKey,
            nodePositionsToPositionMap(composedNodes),
          );
          setNodes((currentNodes) => {
            return mergeNodePresentationState(composedNodes, currentNodes);
          });
          setEdges(composedEdges);
          lastAppliedRuntimeSnapshotRef.current = runtimeSnapshot;

          const nextPositionPayload = buildPositionPayload(composedNodes);
          if (positionsChanged(nextPositionPayload, savedPositions)) {
            void savePositions(nextPositionPayload);
          }

          if (shouldFitViewAfterLoad) {
            requestFitViewAfterLoad();
          }

          lastTopologyIdentityByMapRef.current.set(mapKey, topologyIdentity.signature);
          lastUsablePositionStateByMapRef.current.set(mapKey, usablePositionState);
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
              ...topologyLoadMetadata,
              deviceCount: fetchedDevices.length,
              linkCount: fetchedLinks.length,
              positionCount: savedPositions.size,
              placementDeviceCount: placementDeviceIds.size,
              structureChanged,
            },
          });
          return 'applied';
        } catch (loadError) {
          if (!isCurrentTopologyLoad()) {
            return 'stale';
          }
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
              ...topologyLoadMetadata,
              error: topologyError.message,
            },
          });

          if (!options.suppressBlockingError) {
            setError(topologyError.message);
          }

          if (options.rethrowOnError) {
            throw topologyError;
          }
          return 'failed';
        } finally {
          if (isCurrentTopologyLoad() && !isSilentRefresh) {
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
      mapId,
      mapKey,
      diagnosticMapId,
      diagnosticMapName,
    ],
  );

  const dismissTopologyRecoveryNotice = useCallback(() => {
    setTopologyRecoveryNotice(null);
  }, []);

  const loadTopologyForConsumer = useCallback(
    async (
      isSilentRefresh = false,
      defaultPosition?: { x: number; y: number },
      trigger: CanvasMeasurementTrigger = 'manual_refresh',
    ) => {
      await loadTopology(isSilentRefresh, defaultPosition, trigger);
    },
    [loadTopology],
  );

  const runStructuralRefresh = useCallback(
    async (causes: Set<StructuralRefreshCause>) => {
      const refreshCauses = new Set(causes);
      lastStructuralRefreshCausesRef.current = refreshCauses;
      setTopologyRecoveryNotice(null);

      try {
        const loadResult = await loadTopology(
          true,
          undefined,
          measurementTriggerForCauses(refreshCauses),
          {
            suppressBlockingError: true,
            rethrowOnError: true,
            includeRuntimeBootstrap: refreshCauses.has('backend-resync-required'),
          },
        );
        if (loadResult !== 'applied') {
          return;
        }
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
      const activeMapKey = activeMapKeyRef.current;
      if (mapKey !== activeMapKey || nodesOwnerMapKeyRef.current !== activeMapKey) {
        return;
      }

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
      const ownerMapKey = nodesOwnerMapKeyRef.current;

      setNodes(nextNodes);
      currentNodePositionsByMapRef.current.set(ownerMapKey, nodePositionsToPositionMap(nextNodes));
      setEdges((currentEdges) => {
        const existingEdgeData = new Map(currentEdges.map((edge) => [edge.id, edge.data ?? {}]));
        return buildTopologyEdges(links, devicesById, nextNodes, existingEdgeData, openEdgeMenu);
      });
      if (ownerMapKey === activeMapKeyRef.current) {
        void savePositions(buildPositionPayload(nextNodes));
      }
    },
    [mapKey, openEdgeMenu, savePositions, setEdges, setNodes],
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
    if (mountedMapKeyRef.current === null) {
      mountedMapKeyRef.current = mapKey;
      return;
    }

    if (mountedMapKeyRef.current === mapKey) {
      return;
    }

    mountedMapKeyRef.current = mapKey;
    void loadTopology(false, undefined, 'manual_refresh', { forceFitView: true });
  }, [loadTopology, mapKey]);

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
    });
  }, [openEdgeMenu, prometheusStatus, setEdges, setNodes, snapshot]);

  useEffect(() => {
    setNodes((currentNodes) => {
      let changed = false;
      const nextNodes = currentNodes.map((node) => {
        const alertStatus = runtimeAlertStatusForDevice(node.id, snapshot, alerts);
        if (node.data.runtime.alertStatus === alertStatus) {
          return node;
        }
        changed = true;
        return {
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
    topologyAreas,
    runtimeSummary,
    loading,
    error,
    loadTopology: loadTopologyForConsumer,
    grafanaUrlRef,
    deviceGrafanaUrlsRef,
    refreshSettings,
    topologyRecoveryNotice,
    dismissTopologyRecoveryNotice,
    retryTopologyRefresh,
    updateNodePosition,
  };
}
