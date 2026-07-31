import { useCallback, useEffect, useMemo, useRef, useState } from 'react';

import { fetchCanvasMapTopology } from '../../api/client';
import {
  applyDeviceImportTopologyLayout,
  continueDeviceImportTopologyRun,
  type DeviceImportTopologyRunSnapshot,
  fetchActiveDeviceImportTopologyRun,
  fetchDeviceImportTopologyRun,
  markDeviceImportTopologyManualEdit,
  retryDeviceImportTopologyRun,
} from '../../api/deviceImport';
import { canvasMapKey } from './canvasTopologySource';
import type { ImportedNodePlacementRequest } from './importedNodePlacementRequest';
import {
  computeTopologyImportLayout,
  type TopologyImportLayoutPosition,
} from './topologyImportLayout';

const POLL_INTERVAL_MS = 1000;

export type DeviceImportTopologyPhase = 'discovery' | 'links' | 'layout' | 'complete';

export interface DeviceImportTopologyProgress {
  total: number;
  completed: number;
  running: number;
  warnings: number;
  failed: number;
  neighbors: number;
  linksCreated: number;
  unresolved: number;
}

interface UseDeviceImportTopologyRunInput {
  mapId: string | null;
  request: ImportedNodePlacementRequest | null | undefined;
  renderedMapKey: string | null;
  nodePositions: Map<string, TopologyImportLayoutPosition>;
  reloadTopology: (force?: boolean) => Promise<unknown>;
  onConsumed?: (requestId: string) => void;
}

export interface DeviceImportTopologyRunController {
  snapshot: DeviceImportTopologyRunSnapshot | null;
  phase: DeviceImportTopologyPhase | null;
  progress: DeviceImportTopologyProgress;
  applyingLayout: boolean;
  mutationBlocked: boolean;
  error: string | null;
  retry: (deviceIDs?: string[]) => Promise<void>;
  continuePartial: () => Promise<void>;
  markManualEdit: () => Promise<void>;
  refresh: () => Promise<void>;
}

function phaseForSnapshot(
  snapshot: DeviceImportTopologyRunSnapshot | null,
): DeviceImportTopologyPhase | null {
  switch (snapshot?.run.state) {
    case 'importing':
    case 'discovering':
    case 'followup':
      return 'discovery';
    case 'reconciling':
    case 'failed':
      return 'links';
    case 'ready_for_layout':
      return 'layout';
    case 'completed':
      return 'complete';
    default:
      return null;
  }
}

function progressForSnapshot(
  snapshot: DeviceImportTopologyRunSnapshot | null,
): DeviceImportTopologyProgress {
  const progress: DeviceImportTopologyProgress = {
    total: snapshot?.items.length ?? 0,
    completed: 0,
    running: 0,
    warnings: 0,
    failed: 0,
    neighbors: 0,
    linksCreated: 0,
    unresolved: 0,
  };
  for (const item of snapshot?.items ?? []) {
    if (item.state === 'running' || item.state === 'queued') progress.running += 1;
    if (item.state === 'succeeded' || item.state === 'warning' || item.state === 'failed') {
      progress.completed += 1;
    }
    if (item.state === 'warning') progress.warnings += 1;
    if (item.state === 'failed') progress.failed += 1;
    progress.neighbors += item.neighbor_count;
    progress.linksCreated += item.links_created;
    progress.unresolved += item.unresolved_neighbors;
  }
  return progress;
}

function errorMessage(error: unknown): string {
  return error instanceof Error ? error.message : 'Topology Bootstrap-Once failed';
}

/**
 * Resumes and coordinates one durable Bootstrap-Once run for the selected map. The backend
 * remains authoritative; polling makes the workflow restart-safe and WebSocket-independent.
 */
export function useDeviceImportTopologyRun({
  mapId,
  request,
  renderedMapKey,
  nodePositions,
  reloadTopology,
  onConsumed,
}: UseDeviceImportTopologyRunInput): DeviceImportTopologyRunController {
  const [snapshot, setSnapshot] = useState<DeviceImportTopologyRunSnapshot | null>(null);
  const [applyingLayout, setApplyingLayout] = useState(false);
  const [manualEditPending, setManualEditPending] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [loadedScopeKey, setLoadedScopeKey] = useState<string | null>(null);
  const snapshotRef = useRef<DeviceImportTopologyRunSnapshot | null>(null);
  const scopeRevisionRef = useRef(0);
  const operationRevisionRef = useRef(0);
  const appliedTokensRef = useRef(new Set<string>());
  const applyingLayoutKeyRef = useRef<string | null>(null);
  const manualEditRunIDsRef = useRef(new Set<string>());
  const manualEditPromisesRef = useRef(new Map<string, Promise<void>>());
  const requestedRunID =
    request?.mapId === mapId && request.topologyRunId ? request.topologyRunId : null;
  const lookupScopeKey = mapId ? `${mapId}:${requestedRunID ?? 'active'}` : null;
  const lookupPending = lookupScopeKey !== null && loadedScopeKey !== lookupScopeKey;
  const reloadTopologyRef = useRef(reloadTopology);
  const onConsumedRef = useRef(onConsumed);
  const nodePositionsRef = useRef(nodePositions);
  reloadTopologyRef.current = reloadTopology;
  onConsumedRef.current = onConsumed;
  nodePositionsRef.current = nodePositions;

  const retainSnapshot = useCallback((next: DeviceImportTopologyRunSnapshot | null) => {
    snapshotRef.current = next;
    setSnapshot(next);
  }, []);

  const refreshSnapshot = useCallback(async () => {
    const scopeRevision = scopeRevisionRef.current;
    const operationRevision = operationRevisionRef.current;
    if (!mapId) {
      retainSnapshot(null);
      setLoadedScopeKey(null);
      return;
    }
    const scopedMapID = mapId;
    const current = snapshotRef.current;
    const runID = current?.run.map_id === scopedMapID ? current.run.id : requestedRunID;
    let next: DeviceImportTopologyRunSnapshot | null;
    try {
      next = runID
        ? await fetchDeviceImportTopologyRun(runID)
        : await fetchActiveDeviceImportTopologyRun(scopedMapID);
    } catch (refreshError) {
      if (
        scopeRevisionRef.current !== scopeRevision ||
        operationRevisionRef.current !== operationRevision
      ) {
        return;
      }
      throw refreshError;
    }
    if (
      scopeRevisionRef.current !== scopeRevision ||
      operationRevisionRef.current !== operationRevision ||
      (next && (next.run.map_id !== scopedMapID || (runID !== null && next.run.id !== runID)))
    ) {
      return;
    }
    retainSnapshot(next);
    setLoadedScopeKey(lookupScopeKey);
    setError(null);
  }, [lookupScopeKey, mapId, requestedRunID, retainSnapshot]);

  const refresh = useCallback(async () => {
    const scopeRevision = scopeRevisionRef.current;
    appliedTokensRef.current.clear();
    setError(null);
    try {
      await refreshSnapshot();
    } catch (refreshError) {
      if (scopeRevisionRef.current === scopeRevision) {
        setError(errorMessage(refreshError));
      }
    }
  }, [refreshSnapshot]);

  useEffect(() => {
    const scopeRevision = scopeRevisionRef.current + 1;
    scopeRevisionRef.current = scopeRevision;
    operationRevisionRef.current += 1;
    retainSnapshot(null);
    setError(null);
    setApplyingLayout(false);
    applyingLayoutKeyRef.current = null;
    setManualEditPending(false);
    setLoadedScopeKey(null);
    appliedTokensRef.current.clear();
    manualEditRunIDsRef.current.clear();
    if (!mapId) return;

    void (
      requestedRunID
        ? fetchDeviceImportTopologyRun(requestedRunID)
        : fetchActiveDeviceImportTopologyRun(mapId)
    )
      .then((next) => {
        if (
          scopeRevisionRef.current !== scopeRevision ||
          (next !== null && next.run.map_id !== mapId)
        )
          return;
        retainSnapshot(next);
        setLoadedScopeKey(lookupScopeKey);
        setError(null);
      })
      .catch((loadError) => {
        if (scopeRevisionRef.current !== scopeRevision) return;
        setError(errorMessage(loadError));
      });
  }, [lookupScopeKey, mapId, requestedRunID, retainSnapshot]);

  useEffect(() => {
    const run = snapshot?.run;
    if (!run || run.state === 'completed' || run.state === 'superseded') return;
    const scopeRevision = scopeRevisionRef.current;
    const operationRevision = operationRevisionRef.current;
    const scopedMapID = mapId;
    const timer = window.setInterval(() => {
      void fetchDeviceImportTopologyRun(run.id)
        .then((next) => {
          if (
            scopeRevisionRef.current !== scopeRevision ||
            operationRevisionRef.current !== operationRevision ||
            snapshotRef.current?.run.id !== run.id ||
            next.run.id !== run.id ||
            next.run.map_id !== scopedMapID
          ) {
            return;
          }
          retainSnapshot(next);
          setError(null);
        })
        .catch((pollError) => {
          if (
            scopeRevisionRef.current === scopeRevision &&
            operationRevisionRef.current === operationRevision
          ) {
            setError(errorMessage(pollError));
          }
        });
    }, POLL_INTERVAL_MS);
    return () => window.clearInterval(timer);
  }, [mapId, retainSnapshot, snapshot?.run.id, snapshot?.run.state]);

  useEffect(() => {
    if (!mapId) return;
    const handleInvalidation = (event: Event) => {
      const detail = (event as CustomEvent<{ run_id?: string; map_id?: string }>).detail;
      if (
        detail?.map_id !== mapId ||
        (snapshotRef.current && detail.run_id !== snapshotRef.current.run.id)
      ) {
        return;
      }
      const scopeRevision = scopeRevisionRef.current;
      void refreshSnapshot().catch((refreshError) => {
        if (scopeRevisionRef.current === scopeRevision) {
          setError(errorMessage(refreshError));
        }
      });
    };
    window.addEventListener('device-import-topology-run-changed', handleInvalidation);
    return () =>
      window.removeEventListener('device-import-topology-run-changed', handleInvalidation);
  }, [mapId, refreshSnapshot]);

  useEffect(() => {
    if (snapshot === null) return;
    const currentSnapshot = snapshot;
    const current = currentSnapshot.run;
    const inputToken = current.layout_input_token;
    const hasIssues = currentSnapshot.items.some(
      (item) =>
        item.state === 'warning' || item.state === 'failed' || item.unresolved_neighbors > 0,
    );
    if (
      current.state !== 'ready_for_layout' ||
      !current.auto_layout_allowed ||
      (hasIssues && !current.backgrounded) ||
      !inputToken ||
      !mapId ||
      current.map_id !== mapId ||
      renderedMapKey !== canvasMapKey(mapId)
    ) {
      return;
    }
    const applicationKey = `${current.id}:${inputToken}`;
    if (appliedTokensRef.current.has(applicationKey)) return;
    appliedTokensRef.current.add(applicationKey);
    const scopeRevision = scopeRevisionRef.current;
    const operationRevision = operationRevisionRef.current;
    applyingLayoutKeyRef.current = applicationKey;
    setApplyingLayout(true);
    setError(null);

    void fetchCanvasMapTopology(mapId)
      .then(async (result) => {
        if (result.status !== 'ok') throw new Error('Map topology was not available for layout');
        if (
          scopeRevisionRef.current !== scopeRevision ||
          operationRevisionRef.current !== operationRevision ||
          snapshotRef.current?.run.id !== current.id ||
          snapshotRef.current.run.map_id !== mapId
        ) {
          return;
        }
        const positions = new Map<string, TopologyImportLayoutPosition>();
        for (const [deviceID, position] of Object.entries(result.topology.positions)) {
          positions.set(deviceID, position);
        }
        for (const [deviceID, position] of nodePositionsRef.current) {
          if (!positions.has(deviceID)) positions.set(deviceID, position);
        }
        const attentionDeviceIds = new Set(
          currentSnapshot.items
            .filter(
              (item) =>
                item.state === 'failed' ||
                item.state === 'warning' ||
                item.unresolved_neighbors > 0,
            )
            .map((item) => item.device_id),
        );
        const layout = computeTopologyImportLayout({
          deviceIds: result.topology.devices.map((device) => device.id),
          links: result.topology.links.map((link) => ({
            id: link.id,
            source: link.source_device_id,
            target: link.target_device_id,
          })),
          positions,
          importedDeviceIds: new Set(currentSnapshot.items.map((item) => item.device_id)),
          attentionDeviceIds,
          scope: current.layout_scope,
        });
        await applyDeviceImportTopologyLayout(current.id, {
          input_token: inputToken,
          positions: layout.positions,
          reset_link_route_ids: layout.resetLinkRouteIds,
        });
        if (
          scopeRevisionRef.current !== scopeRevision ||
          operationRevisionRef.current !== operationRevision ||
          snapshotRef.current?.run.id !== current.id ||
          snapshotRef.current.run.map_id !== mapId
        ) {
          return;
        }
        retainSnapshot({
          ...currentSnapshot,
          run: {
            ...currentSnapshot.run,
            state: 'completed',
            completed_at: new Date().toISOString(),
          },
        });
        await reloadTopologyRef.current(true);
        if (
          scopeRevisionRef.current !== scopeRevision ||
          operationRevisionRef.current !== operationRevision ||
          snapshotRef.current?.run.id !== current.id ||
          snapshotRef.current.run.map_id !== mapId
        ) {
          return;
        }
        if (request?.topologyRunId === current.id) {
          onConsumedRef.current?.(request.requestId);
        }
      })
      .catch((layoutError) => {
        if (
          scopeRevisionRef.current !== scopeRevision ||
          operationRevisionRef.current !== operationRevision ||
          snapshotRef.current?.run.id !== current.id ||
          snapshotRef.current.run.map_id !== mapId
        ) {
          return;
        }
        if (errorMessage(layoutError).toLowerCase().includes('stale')) {
          appliedTokensRef.current.delete(applicationKey);
        }
        setError(errorMessage(layoutError));
      })
      .finally(() => {
        if (
          scopeRevisionRef.current === scopeRevision &&
          applyingLayoutKeyRef.current === applicationKey
        ) {
          applyingLayoutKeyRef.current = null;
          setApplyingLayout(false);
        }
      });
  }, [mapId, renderedMapKey, request, retainSnapshot, snapshot]);

  const continuePartial = useCallback(async () => {
    const current = snapshotRef.current?.run;
    if (!current) return;
    const scopeRevision = scopeRevisionRef.current;
    const stillCurrent = () =>
      scopeRevisionRef.current === scopeRevision &&
      snapshotRef.current?.run.id === current.id &&
      snapshotRef.current.run.map_id === current.map_id;
    setError(null);
    try {
      await continueDeviceImportTopologyRun(current.id);
      if (!stillCurrent()) return;
      await refreshSnapshot();
    } catch (actionError) {
      if (stillCurrent()) setError(errorMessage(actionError));
    }
  }, [refreshSnapshot]);

  const retry = useCallback(
    async (deviceIDs: string[] = []) => {
      const current = snapshotRef.current?.run;
      if (!current) return;
      const scopeRevision = scopeRevisionRef.current;
      const stillCurrent = () =>
        scopeRevisionRef.current === scopeRevision &&
        snapshotRef.current?.run.id === current.id &&
        snapshotRef.current.run.map_id === current.map_id;
      setError(null);
      try {
        await retryDeviceImportTopologyRun(current.id, deviceIDs);
        if (!stillCurrent()) return;
        await refreshSnapshot();
      } catch (actionError) {
        if (stillCurrent()) setError(errorMessage(actionError));
      }
    },
    [refreshSnapshot],
  );

  const markManualEdit = useCallback(async () => {
    if (lookupPending) {
      const lookupError = new Error('Topology import status is still loading');
      setError(lookupError.message);
      throw lookupError;
    }
    const current = snapshotRef.current;
    if (!current?.run.auto_layout_allowed || manualEditRunIDsRef.current.has(current.run.id)) {
      return;
    }
    const existing = manualEditPromisesRef.current.get(current.run.id);
    if (existing) return existing;
    const scopeRevision = scopeRevisionRef.current;
    operationRevisionRef.current += 1;
    const operationRevision = operationRevisionRef.current;
    setManualEditPending(true);
    setError(null);
    const operation = markDeviceImportTopologyManualEdit(current.run.id)
      .then(() => {
        if (
          scopeRevisionRef.current !== scopeRevision ||
          operationRevisionRef.current !== operationRevision ||
          snapshotRef.current?.run.id !== current.run.id ||
          snapshotRef.current.run.map_id !== current.run.map_id
        ) {
          return;
        }
        manualEditRunIDsRef.current.add(current.run.id);
        retainSnapshot({
          ...snapshotRef.current,
          run: {
            ...snapshotRef.current.run,
            state: 'completed',
            auto_layout_allowed: false,
            completed_at: new Date().toISOString(),
          },
        });
      })
      .catch((actionError) => {
        if (
          scopeRevisionRef.current === scopeRevision &&
          operationRevisionRef.current === operationRevision
        ) {
          setError(errorMessage(actionError));
        }
        throw actionError;
      })
      .finally(() => {
        manualEditPromisesRef.current.delete(current.run.id);
        if (
          scopeRevisionRef.current === scopeRevision &&
          operationRevisionRef.current === operationRevision
        ) {
          setManualEditPending(false);
        }
      });
    manualEditPromisesRef.current.set(current.run.id, operation);
    return operation;
  }, [lookupPending, retainSnapshot]);

  const mutationBlocked =
    lookupPending ||
    applyingLayout ||
    manualEditPending ||
    (snapshot !== null &&
      snapshot.run.state !== 'completed' &&
      snapshot.run.state !== 'superseded' &&
      snapshot.run.auto_layout_allowed);

  return useMemo(
    () => ({
      snapshot,
      phase: phaseForSnapshot(snapshot),
      progress: progressForSnapshot(snapshot),
      applyingLayout,
      mutationBlocked,
      error,
      retry,
      continuePartial,
      markManualEdit,
      refresh,
    }),
    [
      applyingLayout,
      continuePartial,
      error,
      markManualEdit,
      mutationBlocked,
      refresh,
      retry,
      snapshot,
    ],
  );
}
