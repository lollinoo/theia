/**
 * Coordinates canvas data state for the topology canvas.
 * Keeps canvas lifecycle, projected graph state, and cleanup behavior explicit for callers.
 */
import type { ReactFlowInstance, SnapGrid } from '@xyflow/react';
import { useCallback, useEffect, useMemo, useRef, useState } from 'react';

import { createLink, fetchGrafanaDashboardConfig, fetchSettings } from '../../api/client';
import { publishCanvasRuntimeBootstrap } from '../../hooks/canvasRuntimeBootstrap';
import { type PositionState, usePositions } from '../../hooks/usePositions';
import type { Area, Device, GrafanaDashboardConfig, Link } from '../../types/api';
import {
  type AlertDTO,
  isPrometheusUnavailable,
  type PrometheusStatusPayload,
  type SnapshotPayload,
} from '../../types/metrics';
import type { DeviceNode } from '../DeviceCard';
import type { LinkEdgeType } from '../LinkEdge';
import { type LinkEdgeData } from '../linkSemantics';
import { applyAlertStatusPatch } from './alertStatusPatch';
import { updateCanvasDiagnosticsState } from './canvasDiagnostics';
import { snapNodesToGrid } from './canvasGrid';
import { buildPositionPayload, topologyFitViewPadding, viewportSize } from './canvasHelpers';
import {
  type CanvasMeasurementTrigger,
  measureCanvasAsyncWork,
  measureCanvasWork,
} from './canvasInstrumentation';
import { recordCanvasLayoutCompleted, recordCanvasLayoutStarted } from './canvasLayoutDiagnostics';
import { refreshCanvasSettings } from './canvasSettingsRefresh';
import {
  recordCanvasTopologyLoadFailed,
  recordCanvasTopologyLoadStarted,
  recordCanvasTopologyLoadSucceeded,
} from './canvasTopologyDiagnostics';
import { canvasMapKey, loadCanvasTopologySource } from './canvasTopologySource';
import { buildTopologyEdges } from './edgeBuilder';
import {
  buildIncrementalLayoutInputs,
  computeIncrementalLayoutPositions,
} from './incrementalLayout';
import {
  prepareManualEdgeMigrationForTopologyLoad,
  recordSavedMapManualEdgeMigrationSkip,
  runDefaultMapManualEdgeMigrationForTopologyLoad,
} from './manualEdgeMigrationOrchestrator';
import { type ScreenRect } from './newNodePlacement';
import { buildExplicitNodePlacements } from './newNodePlacementAdapter';
import { buildManualNodePositionUpdate } from './nodePositionUpdate';
import { buildAlertsPanelModel } from './panelAdapters';
import { buildRuntimeState } from './runtimeAdapters';
import { applyRuntimeSnapshotPatch } from './runtimeSnapshotPatch';
import {
  createStructuralRefreshQueue,
  type StructuralRefreshQueue,
} from './structuralRefreshQueue';
import {
  buildCanvasTopologyCompositionCacheKey,
  createCanvasTopologyCompositionCache,
} from './topologyCompositionCache';
import { buildTopologyIdentity, collectPlacementDeviceIds } from './topologyIdentity';
import {
  buildNotModifiedTopologyLoadPlan,
  buildShouldFitViewAfterTopologyLoad,
  buildTopologySourceRequestPlan,
} from './topologyLoadPlan';
import {
  buildTopologyCompositionPositionPlan,
  buildTopologyPositionSavePlan,
  buildUsablePositionState,
  mergeNodePresentationState,
  nodePositionsToPositionMap,
} from './topologyPositionState';
import {
  buildTopologyRecoveryFailureNotice,
  buildTopologyRecoveryNotice,
  measurementTriggerForCauses,
  type StructuralRefreshCause,
  type TopologyRecoveryNotice,
} from './topologyRecovery';

export type { TopologyRecoveryNotice } from './topologyRecovery';

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
  getCanvasClientRect: () => ScreenRect | null;
  nodes: DeviceNode[];
  setNodes: React.Dispatch<React.SetStateAction<DeviceNode[]>>;
  setEdges: React.Dispatch<React.SetStateAction<LinkEdgeType[]>>;
  snapGrid: SnapGrid | null;
  nodeIndexByIdRef?: React.MutableRefObject<Map<string, number>>;
  edgeIndexByIdRef?: React.MutableRefObject<Map<string, number>>;
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
  renderedMapKey: string | null;
  loadTopology: (isSilentRefresh?: boolean, trigger?: CanvasMeasurementTrigger) => Promise<void>;
  requestNewNodePlacement: (deviceId: string) => Promise<void>;
  grafanaUrlRef: React.MutableRefObject<string>;
  grafanaDashboardConfigRef: React.MutableRefObject<GrafanaDashboardConfig | null>;
  refreshSettings: () => void;
  topologyRecoveryNotice: TopologyRecoveryNotice | null;
  dismissTopologyRecoveryNotice: () => void;
  retryTopologyRefresh: () => void;
  updateNodePosition: (deviceId: string, position: { x: number; y: number }) => void;
  snapCurrentNodePositions: (grid: SnapGrid) => void;
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

const structuralRefreshDebounceMs = 250;
const emptyAlerts: AlertDTO[] = [];

// nowMs prefers high-resolution browser timing for topology diagnostics.
function nowMs(): number {
  return typeof performance !== 'undefined' && typeof performance.now === 'function'
    ? performance.now()
    : Date.now();
}

// useCanvasData orchestrates topology loading, runtime patches, position saves, and recovery notices.
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
  getCanvasClientRect,
  nodes,
  setNodes,
  setEdges,
  snapGrid,
  nodeIndexByIdRef,
  edgeIndexByIdRef,
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
  const [renderedMapKey, setRenderedMapKey] = useState<string | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  const snapshotRef = useRef<SnapshotPayload | null>(null);
  const lastAppliedRuntimeSnapshotRef = useRef<SnapshotPayload | null>(null);
  const alertsRef = useRef<AlertDTO[]>(alerts);
  const devicesRef = useRef<Device[]>([]);
  const topologyLinksRef = useRef<Link[]>([]);
  const nodesRef = useRef<DeviceNode[]>(nodes);
  const snapGridRef = useRef<SnapGrid | null>(snapGrid);
  const activeMapKeyRef = useRef<string>(mapKey);
  const mountedMapKeyRef = useRef<string | null>(null);
  const topologyLoadSequenceRef = useRef(0);
  const nodesOwnerMapKeyRef = useRef<string>(mapKey);
  const lastTopologyIdentityByMapRef = useRef<Map<string, string | null>>(new Map());
  const lastCanvasTopologyEtagByMapRef = useRef<Map<string, string | null>>(new Map());
  const lastUsablePositionStateByMapRef = useRef<Map<string, string>>(new Map());
  const currentNodePositionsByMapRef = useRef<Map<string, Map<string, PositionState>>>(new Map());
  const pendingNewNodePlacementIdsByMapRef = useRef<Map<string, Set<string>>>(new Map());
  const topologyCompositionCacheRef = useRef(createCanvasTopologyCompositionCache());
  const skippedSavedMapManualEdgeMigrationRef = useRef<Set<string>>(new Set());
  const grafanaUrlRef = useRef<string>('');
  const grafanaDashboardConfigRef = useRef<GrafanaDashboardConfig | null>(null);
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
  const structuralRefreshRunnerRef = useRef<
    (causes: Set<StructuralRefreshCause>) => void | Promise<void>
  >(() => undefined);
  const structuralRefreshQueueRef = useRef<StructuralRefreshQueue | null>(null);
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
  snapGridRef.current = snapGrid;
  activeMapKeyRef.current = mapKey;
  currentNodePositionsByMapRef.current.set(
    nodesOwnerMapKeyRef.current,
    nodePositionsToPositionMap(nodes),
  );

  useEffect(() => {
    return () => {
      pendingNewNodePlacementIdsByMapRef.current.delete(mapKey);
    };
  }, [mapKey]);

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

  const snapCurrentNodePositions = useCallback(
    (grid: SnapGrid) => {
      const currentNodes = nodesRef.current;
      const normalizedNodes = snapNodesToGrid(currentNodes, grid);
      if (normalizedNodes === currentNodes) {
        return;
      }

      const ownerMapKey = nodesOwnerMapKeyRef.current;
      nodesRef.current = normalizedNodes;
      setNodes(normalizedNodes);
      currentNodePositionsByMapRef.current.set(
        ownerMapKey,
        nodePositionsToPositionMap(normalizedNodes),
      );
      const devicesById = new Map(devicesRef.current.map((device) => [device.id, device]));
      setEdges((currentEdges) => {
        const existingEdgeData = new Map<string, LinkEdgeData>(
          currentEdges.map((edge) => [edge.id, edge.data ?? {}]),
        );
        return buildTopologyEdges(
          topologyLinksRef.current,
          devicesById,
          normalizedNodes,
          existingEdgeData,
          openEdgeMenu,
          alertsRef.current,
        );
      });
      if (ownerMapKey === activeMapKeyRef.current) {
        void savePositions(buildPositionPayload(normalizedNodes));
      }
    },
    [openEdgeMenu, savePositions, setEdges, setNodes],
  );

  const loadTopology = useCallback(
    async (
      isSilentRefresh = false,
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
        recordCanvasTopologyLoadStarted(topologyLoadMetadata);

        if (!isSilentRefresh) {
          setLoading(true);
        }
        setError(null);

        try {
          const manualEdgeMigrationPlan = prepareManualEdgeMigrationForTopologyLoad({
            storage: window.localStorage,
            mapId,
          });
          const lastCanvasTopologyEtag =
            lastCanvasTopologyEtagByMapRef.current.get(requestMapKey) ?? null;
          const topologyRequestPlan = buildTopologySourceRequestPlan({
            trigger,
            options,
            mapKey,
            nodesOwnerMapKey: nodesOwnerMapKeyRef.current,
            lastCanvasTopologyEtag,
            manualEdgeMigrationPlan,
          });
          const topologySource = await loadCanvasTopologySource({
            mapId,
            fetchPositions,
            etag: topologyRequestPlan.etag,
            includeRuntimeBootstrap: topologyRequestPlan.includeRuntimeBootstrap,
            forceRuntimeBootstrap: topologyRequestPlan.forceRuntimeBootstrap,
          });
          if (!isCurrentTopologyLoad()) {
            return 'stale';
          }
          if (topologySource.status === 'ok' && topologySource.runtimeSnapshot !== undefined) {
            publishCanvasRuntimeBootstrap({
              snapshot: topologySource.runtimeSnapshot,
              runtimeStreamId: topologySource.runtimeStreamId,
              runtimeVersion: topologySource.runtimeVersion,
              runtimeIdentity: topologySource.runtimeIdentity,
            });
            snapshotRef.current = topologySource.runtimeSnapshot;
          }
          recordSavedMapManualEdgeMigrationSkip({
            plan: manualEdgeMigrationPlan,
            mapId,
            mapKey,
            skippedKeys: skippedSavedMapManualEdgeMigrationRef.current,
            topologyLoadMetadata,
          });
          if (topologySource.status === 'not-modified') {
            const notModifiedPlan = buildNotModifiedTopologyLoadPlan({
              responseEtag: topologySource.etag,
              lastCanvasTopologyEtag,
              forceFitView: options.forceFitView === true,
            });
            lastCanvasTopologyEtagByMapRef.current.set(requestMapKey, notModifiedPlan.etag);
            if (notModifiedPlan.shouldFitView) {
              requestFitViewAfterLoad();
            }
            recordCanvasTopologyLoadSucceeded({
              metadata: topologyLoadMetadata,
              durationMs: nowMs() - loadStartedAt,
              notModified: true,
            });
            return 'applied';
          }

          const fetchedDevices = topologySource.devices;
          const fetchedLinks = topologySource.links;
          const fetchedAreas = topologySource.areas;
          const savedPositions = topologySource.positions;
          const runtimeSnapshot = topologySource.runtimeSnapshot ?? snapshotRef.current;
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

          const manualEdgeMigrationResult = await runDefaultMapManualEdgeMigrationForTopologyLoad({
            plan: manualEdgeMigrationPlan,
            storage: window.localStorage,
            existingLinks: fetchedLinks,
            createLink,
            isCurrentTopologyLoad,
          });
          if (!isCurrentTopologyLoad()) {
            return 'stale';
          }
          if (manualEdgeMigrationResult.status === 'stale') {
            return 'stale';
          }
          const nextCanvasTopologyEtag =
            manualEdgeMigrationResult.appliedCount > 0 ? null : (topologySource.etag ?? null);

          const topologyIdentity = buildTopologyIdentity(fetchedDevices, fetchedLinks);
          const currentNodePositions =
            currentNodePositionsByMapRef.current.get(mapKey) ?? new Map();
          const structureChanged =
            lastTopologyIdentityByMapRef.current.get(mapKey) !== topologyIdentity.signature;
          const { effectivePositions, currentPositionsForComposition } =
            buildTopologyCompositionPositionPlan({
              trigger,
              savedPositions,
              currentNodePositions,
            });

          const usablePositionState = buildUsablePositionState(
            fetchedDevices,
            currentNodePositions,
            savedPositions,
          );
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
          const pendingDeviceIds = new Set(
            pendingNewNodePlacementIdsByMapRef.current.get(requestMapKey) ?? [],
          );
          const canvasRect = getCanvasClientRect();
          const explicitPlacement =
            pendingDeviceIds.size > 0 && canvasRect !== null
              ? buildExplicitNodePlacements({
                  reactFlow,
                  canvasRect,
                  devices: fetchedDevices,
                  links: fetchedLinks,
                  deviceIds: pendingDeviceIds,
                  snapGrid: snapGridRef.current,
                })
              : {
                  positions: new Map<string, { x: number; y: number }>(),
                  placedDeviceIds: new Set<string>(),
                };
          const shouldFitViewAfterLoad =
            pendingDeviceIds.size === 0 &&
            buildShouldFitViewAfterTopologyLoad({
              trigger,
              forceFitView: options.forceFitView === true,
              usablePositionState,
            });
          const consumeAppliedExplicitPlacements = () => {
            const pendingForMap = pendingNewNodePlacementIdsByMapRef.current.get(requestMapKey);
            if (!pendingForMap) return;
            for (const deviceId of explicitPlacement.placedDeviceIds) {
              pendingForMap.delete(deviceId);
            }
            if (pendingForMap.size === 0) {
              pendingNewNodePlacementIdsByMapRef.current.delete(requestMapKey);
            }
          };
          const composeTopologyWithCache = (
            computedPositions: Map<string, { x: number; y: number }>,
            placementDeviceIds: Set<string>,
          ) => {
            const compositionInput = {
              devices: fetchedDevices,
              links: fetchedLinks,
              runtimeState,
              savedPositions: effectivePositions,
              computedPositions,
              currentPositions: currentPositionsForComposition,
              explicitPositions: explicitPlacement.positions,
              editMode,
              openDeviceMenu,
              openEdgeMenu,
              openSelfLinkDetails,
              placementDeviceIds,
              alerts: alertsRef.current,
              snapGrid: snapGridRef.current,
            };
            return topologyCompositionCacheRef.current.compose(
              compositionInput,
              buildCanvasTopologyCompositionCacheKey({
                mapKey,
                topologySignature: topologyIdentity.signature,
                topologyVersion: topologySource.topologyVersion,
                topologyEtag: topologySource.etag,
                schemaVersion: topologySource.schemaVersion,
                devices: fetchedDevices,
                links: fetchedLinks,
                savedPositions: effectivePositions,
                computedPositions,
                currentPositions: currentPositionsForComposition,
                explicitPositions: explicitPlacement.positions,
                editMode,
                snapGrid: snapGridRef.current,
                placementDeviceIds,
                runtimeIdentity: topologySource.runtimeIdentity,
                runtimeVersion: topologySource.runtimeVersion,
                runtimeSnapshot,
                alerts: alertsRef.current,
                prometheusStatus,
                openDeviceMenu,
                openEdgeMenu,
                openSelfLinkDetails,
              }),
            );
          };

          if (!structureChanged) {
            setDevices(fetchedDevices);
            setTopologyLinks(fetchedLinks);
            setTopologyAreas(fetchedAreas);
            const { nodes: nextNodes, edges: nextEdges } = composeTopologyWithCache(
              new Map<string, { x: number; y: number }>(),
              new Set(),
            );
            nodesOwnerMapKeyRef.current = mapKey;
            setRenderedMapKey(mapKey);
            currentNodePositionsByMapRef.current.set(mapKey, nodePositionsToPositionMap(nextNodes));
            setNodes((currentNodes) => mergeNodePresentationState(nextNodes, currentNodes));
            setEdges(nextEdges);
            lastAppliedRuntimeSnapshotRef.current = snapshotRef.current;
            const positionSavePlan = buildTopologyPositionSavePlan(nextNodes, savedPositions);
            if (positionSavePlan.shouldSave) {
              void savePositions(positionSavePlan.payload);
            }
            consumeAppliedExplicitPlacements();
            lastTopologyIdentityByMapRef.current.set(mapKey, topologyIdentity.signature);
            lastUsablePositionStateByMapRef.current.set(mapKey, usablePositionState);
            recordCanvasTopologyLoadSucceeded({
              metadata: topologyLoadMetadata,
              durationMs: nowMs() - loadStartedAt,
              deviceCount: fetchedDevices.length,
              linkCount: fetchedLinks.length,
              positionCount: savedPositions.size,
              placementDeviceCount: explicitPlacement.placedDeviceIds.size,
              structureChanged,
            });
            if (shouldFitViewAfterLoad) {
              requestFitViewAfterLoad();
            }
            lastCanvasTopologyEtagByMapRef.current.set(requestMapKey, nextCanvasTopologyEtag);
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
                  recordCanvasLayoutStarted({
                    reason: trigger,
                    nodeCount: layoutNodes.length,
                    edgeCount: layoutEdges.length,
                  });
                  const positions = computeIncrementalLayoutPositions({
                    layoutNodes,
                    layoutEdges,
                    placementDeviceIds,
                    width,
                    height,
                  });
                  recordCanvasLayoutCompleted({
                    reason: trigger,
                    nodeCount: layoutNodes.length,
                    durationMs: nowMs() - layoutStartedAt,
                  });
                  return positions;
                })
              : new Map();

          const { nodes: composedNodes, edges: composedEdges } = composeTopologyWithCache(
            computedPositions,
            placementDeviceIds,
          );

          // Apply all state updates together as urgent (not in startTransition).
          // Previously these were wrapped in startTransition which made them
          // low-priority, allowing WebSocket snapshot effects (which depend on
          // devices.length) to interrupt and race with the transition's setNodes,
          // sometimes causing all canvas nodes to vanish after a device delete.
          setDevices(fetchedDevices);
          setTopologyLinks(fetchedLinks);
          setTopologyAreas(fetchedAreas);
          nodesOwnerMapKeyRef.current = mapKey;
          setRenderedMapKey(mapKey);
          currentNodePositionsByMapRef.current.set(
            mapKey,
            nodePositionsToPositionMap(composedNodes),
          );
          setNodes((currentNodes) => {
            return mergeNodePresentationState(composedNodes, currentNodes);
          });
          setEdges(composedEdges);
          lastAppliedRuntimeSnapshotRef.current = runtimeSnapshot;

          const positionSavePlan = buildTopologyPositionSavePlan(composedNodes, savedPositions);
          if (positionSavePlan.shouldSave) {
            void savePositions(positionSavePlan.payload);
          }
          consumeAppliedExplicitPlacements();

          if (shouldFitViewAfterLoad) {
            requestFitViewAfterLoad();
          }

          lastTopologyIdentityByMapRef.current.set(mapKey, topologyIdentity.signature);
          lastUsablePositionStateByMapRef.current.set(mapKey, usablePositionState);
          const appliedPlacementDeviceIds = new Set([
            ...placementDeviceIds,
            ...explicitPlacement.placedDeviceIds,
          ]);
          recordCanvasTopologyLoadSucceeded({
            metadata: topologyLoadMetadata,
            durationMs: nowMs() - loadStartedAt,
            deviceCount: fetchedDevices.length,
            linkCount: fetchedLinks.length,
            positionCount: savedPositions.size,
            placementDeviceCount: appliedPlacementDeviceIds.size,
            structureChanged,
          });
          lastCanvasTopologyEtagByMapRef.current.set(requestMapKey, nextCanvasTopologyEtag);
          return 'applied';
        } catch (loadError) {
          if (!isCurrentTopologyLoad()) {
            return 'stale';
          }
          const topologyError =
            loadError instanceof Error ? loadError : new Error('Failed to load topology');
          recordCanvasTopologyLoadFailed({
            metadata: topologyLoadMetadata,
            durationMs: nowMs() - loadStartedAt,
            error: topologyError.message,
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
      getCanvasClientRect,
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

  const requestNewNodePlacement = useCallback(
    async (deviceId: string) => {
      if (deviceId.length === 0) return;

      const pendingForMap =
        pendingNewNodePlacementIdsByMapRef.current.get(mapKey) ?? new Set<string>();
      pendingForMap.add(deviceId);
      pendingNewNodePlacementIdsByMapRef.current.set(mapKey, pendingForMap);

      lastCanvasTopologyEtagByMapRef.current.set(mapKey, null);
      await loadTopology(true, 'manual_refresh');
    },
    [loadTopology, mapKey],
  );

  const dismissTopologyRecoveryNotice = useCallback(() => {
    setTopologyRecoveryNotice(null);
  }, []);

  const loadTopologyForConsumer = useCallback(
    async (isSilentRefresh = false, trigger: CanvasMeasurementTrigger = 'manual_refresh') => {
      await loadTopology(isSilentRefresh, trigger);
    },
    [loadTopology],
  );

  const runStructuralRefresh = useCallback(
    async (causes: Set<StructuralRefreshCause>) => {
      const refreshCauses = new Set(causes);
      lastStructuralRefreshCausesRef.current = refreshCauses;
      setTopologyRecoveryNotice(null);

      try {
        const loadResult = await loadTopology(true, measurementTriggerForCauses(refreshCauses), {
          suppressBlockingError: true,
          rethrowOnError: true,
          includeRuntimeBootstrap: refreshCauses.has('backend-resync-required'),
        });
        if (loadResult !== 'applied') {
          return;
        }
        setTopologyRecoveryNotice(buildTopologyRecoveryNotice(refreshCauses));
      } catch {
        setTopologyRecoveryNotice(buildTopologyRecoveryFailureNotice());
      }
    },
    [loadTopology],
  );

  useEffect(() => {
    structuralRefreshRunnerRef.current = (causes) => {
      return runStructuralRefresh(causes);
    };
  }, [runStructuralRefresh]);

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

      const updatePlan = buildManualNodePositionUpdate({
        deviceId,
        position,
        nodes: nodesRef.current,
        devices: devicesRef.current,
        links: topologyLinksRef.current,
        openEdgeMenu,
        snapGrid: snapGridRef.current,
      });
      if (updatePlan === null) {
        return;
      }

      const ownerMapKey = nodesOwnerMapKeyRef.current;

      setNodes(updatePlan.nodes);
      currentNodePositionsByMapRef.current.set(ownerMapKey, updatePlan.positionMap);
      setEdges(updatePlan.buildEdges);
      if (ownerMapKey === activeMapKeyRef.current) {
        void savePositions(updatePlan.positionPayload);
      }
    },
    [mapKey, openEdgeMenu, savePositions, setEdges, setNodes],
  );

  const queueStructuralRefresh = useCallback((cause: StructuralRefreshCause) => {
    if (structuralRefreshQueueRef.current === null) {
      structuralRefreshQueueRef.current = createStructuralRefreshQueue({
        debounceMs: structuralRefreshDebounceMs,
        runRefresh: (causes) => structuralRefreshRunnerRef.current(causes),
        setTimeoutFn: window.setTimeout.bind(window),
        clearTimeoutFn: window.clearTimeout.bind(window),
      });
    }

    structuralRefreshQueueRef.current.queue(cause);
  }, []);

  // Initial load
  useEffect(() => {
    void loadTopology(false, 'initial_load');
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
    void loadTopology(false, 'manual_refresh', { forceFitView: true });
  }, [loadTopology, mapKey]);

  useEffect(() => {
    window.__THEIA_CANVAS_FORCE_REFRESH__ = () => {
      void loadTopology(true, 'manual_refresh');
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
    const handleResyncRequired = (event: Event) => {
      const detail = 'detail' in event ? event.detail : undefined;
      if (
        typeof detail === 'object' &&
        detail !== null &&
        'strategy' in detail &&
        detail.strategy === 'stream'
      ) {
        return;
      }
      queueStructuralRefresh('backend-resync-required');
    };
    const handleTopologyChanged = () => {
      queueStructuralRefresh('topology-changed');
    };

    window.addEventListener('backend-reconnected', handleReconnect);
    window.addEventListener('backend-resync-required', handleResyncRequired);
    window.addEventListener('topology-changed', handleTopologyChanged);

    return () => {
      structuralRefreshQueueRef.current?.cancel();
      window.removeEventListener('backend-reconnected', handleReconnect);
      window.removeEventListener('backend-resync-required', handleResyncRequired);
      window.removeEventListener('topology-changed', handleTopologyChanged);
    };
  }, [queueStructuralRefresh]);

  // Re-fetch settings (Grafana URLs) on demand; called on mount and after
  // any settings panel or device config panel saves Grafana URL changes.
  const refreshSettings = useCallback(() => {
    refreshCanvasSettings({
      fetchSettings,
      fetchGrafanaDashboardConfig,
      grafanaUrlRef,
      grafanaDashboardConfigRef,
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

    lastAppliedRuntimeSnapshotRef.current = applyRuntimeSnapshotPatch({
      previousSnapshot: lastAppliedRuntimeSnapshotRef.current,
      snapshot,
      devices: devicesRef.current,
      links: topologyLinksRef.current,
      alerts: alertsRef.current,
      prometheusStatus,
      setNodes,
      setEdges,
      openEdgeMenu,
      nodeIndexById: nodeIndexByIdRef?.current,
      edgeIndexById: edgeIndexByIdRef?.current,
    });
  }, [
    edgeIndexByIdRef,
    nodeIndexByIdRef,
    openEdgeMenu,
    prometheusStatus,
    setEdges,
    setNodes,
    snapshot,
  ]);

  useEffect(() => {
    applyAlertStatusPatch({
      snapshot,
      alerts,
      setNodes,
      setEdges,
      nodeIndexById: nodeIndexByIdRef?.current,
      edgeIndexById: edgeIndexByIdRef?.current,
    });
  }, [alerts, edgeIndexByIdRef, nodeIndexByIdRef, setEdges, setNodes]);

  return {
    devices,
    setDevices,
    topologyLinks,
    topologyAreas,
    runtimeSummary,
    loading,
    error,
    renderedMapKey,
    loadTopology: loadTopologyForConsumer,
    requestNewNodePlacement,
    grafanaUrlRef,
    grafanaDashboardConfigRef,
    refreshSettings,
    topologyRecoveryNotice,
    dismissTopologyRecoveryNotice,
    retryTopologyRefresh,
    updateNodePosition,
    snapCurrentNodePositions,
  };
}
