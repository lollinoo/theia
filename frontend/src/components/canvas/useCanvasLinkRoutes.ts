/**
 * Owns optimistic map-local link route persistence and per-link request serialization.
 */
import type React from 'react';
import { useCallback, useEffect, useLayoutEffect, useRef, useState } from 'react';

import { deleteCanvasMapLinkRoute, saveCanvasMapLinkRoute } from '../../api/client';
import { copyLinkRoute, type LinkRoute } from '../../types/api';
import type { LinkEdgeType } from '../LinkEdge';

/** Configures link route persistence against the canonical canvas edge state. */
export interface UseCanvasLinkRoutesOptions {
  mapId: string | null;
  setEdges: React.Dispatch<React.SetStateAction<LinkEdgeType[]>>;
  edgeIndexByIdRef: React.MutableRefObject<Map<string, number>>;
}

/** Exposes stable route mutations and recoverable persistence feedback. */
export interface UseCanvasLinkRoutesResult {
  commitLinkRoute: (edgeId: string, route: LinkRoute | null) => void;
  resetLinkRoute: (edgeId: string) => void;
  linkRouteError: string | null;
  dismissLinkRouteError: () => void;
}

interface LinkRouteOwner {
  mapId: string | null;
  generation: number;
  mounted: boolean;
}

interface PendingLinkRoute {
  route: LinkRoute | null;
  sequence: number;
}

interface LinkRouteQueue {
  key: string;
  mapId: string;
  edgeId: string;
  generation: number;
  running: boolean;
  pendingLatest: PendingLinkRoute | null;
  confirmed: LinkRoute | null;
  confirmedSequence: number;
  confirmedInitialized: boolean;
  latestSequence: number;
}

const linkRouteSaveError =
  "Couldn't save link route. The last saved route was restored; try again.";

function copyOptionalRoute(route: LinkRoute | null | undefined): LinkRoute | null {
  return route == null ? null : copyLinkRoute(route);
}

function queueKey(mapId: string, edgeId: string): string {
  return `${mapId}:${edgeId}`;
}

function hasOwner(owner: LinkRouteOwner, queue: LinkRouteQueue): boolean {
  return owner.mounted && owner.mapId === queue.mapId && owner.generation === queue.generation;
}

/** Coordinates optimistic route edits with ordered writes to the owning saved map. */
export function useCanvasLinkRoutes({
  mapId,
  setEdges,
  edgeIndexByIdRef,
}: UseCanvasLinkRoutesOptions): UseCanvasLinkRoutesResult {
  const [linkRouteError, setLinkRouteError] = useState<string | null>(null);
  const ownerRef = useRef<LinkRouteOwner>({ mapId, generation: 0, mounted: true });
  const queuesRef = useRef(new Map<string, LinkRouteQueue>());

  useLayoutEffect(() => {
    const owner = ownerRef.current;
    if (owner.mapId === mapId) {
      return;
    }
    ownerRef.current = {
      mapId,
      generation: owner.generation + 1,
      mounted: true,
    };
    setLinkRouteError(null);
  }, [mapId]);

  useEffect(() => {
    ownerRef.current.mounted = true;
    return () => {
      ownerRef.current = {
        ...ownerRef.current,
        generation: ownerRef.current.generation + 1,
        mounted: false,
      };
    };
  }, []);

  const patchEdgeRoute = useCallback(
    (
      queue: LinkRouteQueue,
      route: LinkRoute | null,
      canApply: () => boolean = () => true,
      initializeConfirmed = false,
    ) => {
      setEdges((currentEdges) => {
        if (!hasOwner(ownerRef.current, queue) || !canApply()) {
          return currentEdges;
        }

        const edgeIndex = edgeIndexByIdRef.current.get(queue.edgeId);
        if (edgeIndex === undefined) {
          return currentEdges;
        }
        const edge = currentEdges[edgeIndex];
        if (!edge || edge.id !== queue.edgeId) {
          return currentEdges;
        }

        if (initializeConfirmed && !queue.confirmedInitialized) {
          queue.confirmed = copyOptionalRoute(edge.data?.route);
          queue.confirmedInitialized = true;
        }

        const data = { ...(edge.data ?? {}) };
        if (route === null) {
          delete data.route;
        } else {
          data.route = copyLinkRoute(route);
        }

        const nextEdges = currentEdges.slice();
        nextEdges[edgeIndex] = { ...edge, data };
        return nextEdges;
      });
    },
    [edgeIndexByIdRef, setEdges],
  );

  const drainQueue = useCallback(
    function drain(queue: LinkRouteQueue): void {
      if (queue.running || queue.pendingLatest === null) {
        return;
      }

      const mutation = queue.pendingLatest;
      queue.pendingLatest = null;
      queue.running = true;

      const request: Promise<LinkRoute | null> =
        mutation.route === null
          ? deleteCanvasMapLinkRoute(queue.mapId, queue.edgeId).then(() => null)
          : saveCanvasMapLinkRoute(queue.mapId, queue.edgeId, mutation.route);

      void request
        .then(
          (savedRoute) => {
            queue.confirmed = copyOptionalRoute(savedRoute);
            queue.confirmedSequence = mutation.sequence;
            queue.confirmedInitialized = true;
          },
          () => {
            const isLatestMutation = queue.latestSequence === mutation.sequence;
            const hasNoNewerConfirmation = queue.confirmedSequence < mutation.sequence;
            if (
              !isLatestMutation ||
              !hasNoNewerConfirmation ||
              !hasOwner(ownerRef.current, queue)
            ) {
              return;
            }

            patchEdgeRoute(
              queue,
              queue.confirmed,
              () =>
                queue.latestSequence === mutation.sequence &&
                queue.confirmedSequence < mutation.sequence,
            );
            setLinkRouteError((currentError) =>
              hasOwner(ownerRef.current, queue) &&
              queue.latestSequence === mutation.sequence &&
              queue.confirmedSequence < mutation.sequence
                ? linkRouteSaveError
                : currentError,
            );
          },
        )
        .finally(() => {
          queue.running = false;
          drain(queue);
        });
    },
    [patchEdgeRoute],
  );

  const commitLinkRoute = useCallback(
    (edgeId: string, route: LinkRoute | null) => {
      const owner = ownerRef.current;
      if (!owner.mounted || owner.mapId === null) {
        return;
      }

      const key = queueKey(owner.mapId, edgeId);
      let queue = queuesRef.current.get(key);
      if (queue === undefined || queue.generation !== owner.generation) {
        queue = {
          key,
          mapId: owner.mapId,
          edgeId,
          generation: owner.generation,
          running: false,
          pendingLatest: null,
          confirmed: null,
          confirmedSequence: 0,
          confirmedInitialized: false,
          latestSequence: 0,
        };
        queuesRef.current.set(key, queue);
      }

      const nextRoute = copyOptionalRoute(route);
      const sequence = queue.latestSequence + 1;
      queue.latestSequence = sequence;
      queue.pendingLatest = { route: nextRoute, sequence };
      patchEdgeRoute(queue, nextRoute, undefined, true);
      drainQueue(queue);
    },
    [drainQueue, patchEdgeRoute],
  );

  const resetLinkRoute = useCallback(
    (edgeId: string) => {
      commitLinkRoute(edgeId, null);
    },
    [commitLinkRoute],
  );

  const dismissLinkRouteError = useCallback(() => {
    setLinkRouteError(null);
  }, []);

  return {
    commitLinkRoute,
    resetLinkRoute,
    linkRouteError,
    dismissLinkRouteError,
  };
}
