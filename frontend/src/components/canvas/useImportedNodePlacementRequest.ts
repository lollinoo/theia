import { useEffect, useRef, useState } from 'react';

import { canvasMapKey } from './canvasTopologySource';
import type { ImportedNodePlacementRequest } from './importedNodePlacementRequest';

type ImportedNodePlacementOutcome = 'applied' | 'pending' | 'failed';

interface UseImportedNodePlacementRequestInput {
  request: ImportedNodePlacementRequest | null | undefined;
  activeMapId: string | null;
  renderedMapKey: string | null;
  topologyRevision: string;
  placeImportedNodes: (deviceIds: Iterable<string>) => Promise<ImportedNodePlacementOutcome>;
  onConsumed?: (requestId: string) => void;
}

/**
 * Applies an import placement request only after its destination topology is rendered.
 * Pending requests are retried when the rendered device set changes.
 */
export function useImportedNodePlacementRequest({
  request,
  activeMapId,
  renderedMapKey,
  topologyRevision,
  placeImportedNodes,
  onConsumed,
}: UseImportedNodePlacementRequestInput): void {
  const [settledAttemptRevision, setSettledAttemptRevision] = useState(0);
  const completedRequestIdsRef = useRef(new Set<string>());
  const inFlightRequestIdsRef = useRef(new Set<string>());
  const lastAttemptRef = useRef<{
    request: ImportedNodePlacementRequest;
    topologyRevision: string;
  } | null>(null);
  const mountedRef = useRef(true);
  const placeImportedNodesRef = useRef(placeImportedNodes);
  const onConsumedRef = useRef(onConsumed);
  const currentScopeRef = useRef({
    requestId: request?.requestId ?? null,
    activeMapId,
    renderedMapKey,
  });

  placeImportedNodesRef.current = placeImportedNodes;
  onConsumedRef.current = onConsumed;
  currentScopeRef.current = {
    requestId: request?.requestId ?? null,
    activeMapId,
    renderedMapKey,
  };

  useEffect(
    () => () => {
      mountedRef.current = false;
    },
    [],
  );

  useEffect(() => {
    if (!request || request.mapId !== activeMapId || renderedMapKey !== canvasMapKey(activeMapId)) {
      lastAttemptRef.current = null;
      return;
    }
    if (
      completedRequestIdsRef.current.has(request.requestId) ||
      inFlightRequestIdsRef.current.has(request.requestId)
    ) {
      return;
    }

    const lastAttempt = lastAttemptRef.current;
    if (lastAttempt?.request === request && lastAttempt.topologyRevision === topologyRevision) {
      return;
    }

    lastAttemptRef.current = { request, topologyRevision };
    inFlightRequestIdsRef.current.add(request.requestId);

    void Promise.resolve()
      .then(() => placeImportedNodesRef.current(request.deviceIds))
      .then((outcome) => {
        const currentScope = currentScopeRef.current;
        if (
          outcome !== 'applied' ||
          currentScope.requestId !== request.requestId ||
          currentScope.activeMapId !== request.mapId ||
          currentScope.renderedMapKey !== canvasMapKey(request.mapId)
        ) {
          return;
        }

        completedRequestIdsRef.current.add(request.requestId);
        onConsumedRef.current?.(request.requestId);
      })
      .catch(() => undefined)
      .finally(() => {
        inFlightRequestIdsRef.current.delete(request.requestId);
        if (mountedRef.current) {
          setSettledAttemptRevision((revision) => revision + 1);
        }
      });
  }, [activeMapId, renderedMapKey, request, settledAttemptRevision, topologyRevision]);
}
