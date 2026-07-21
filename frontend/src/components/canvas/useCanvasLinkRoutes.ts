/**
 * Owns optimistic map-local link route persistence and per-link request serialization.
 */
import type React from 'react';
import { useCallback, useEffect, useLayoutEffect, useRef, useState } from 'react';

import { deleteCanvasMapLinkRoute, saveCanvasMapLinkRoute } from '../../api/client';
import { copyLinkRoute, type LinkRoute } from '../../types/api';
import type { LinkEdgeType } from '../LinkEdge';
import type { LinkRouteEditToken, LinkRouteOwnerToken } from '../linkSemantics';

/** Configures link route persistence against the canonical canvas edge state. */
export interface UseCanvasLinkRoutesOptions {
  mapId: string | null;
  setEdges: React.Dispatch<React.SetStateAction<LinkEdgeType[]>>;
  edgeIndexByIdRef: React.MutableRefObject<Map<string, number>>;
}

/** Exposes stable route mutations and recoverable persistence feedback. */
export interface UseCanvasLinkRoutesResult {
  commitLinkRoute: (edgeId: string, route: LinkRoute | null) => void;
  commitOwnedLinkRoute: (
    edgeId: string,
    route: LinkRoute | null,
    editToken: LinkRouteEditToken | undefined,
  ) => void;
  getLinkRouteEditToken: (edgeId: string) => LinkRouteEditToken | undefined;
  routeOwnerToken: LinkRouteOwnerToken | null;
  resetLinkRoute: (edgeId: string) => void;
  reconcileLinkRouteEdges: (edges: LinkEdgeType[]) => LinkEdgeType[];
  linkRouteError: string | null;
  dismissLinkRouteError: () => void;
}

interface LinkRouteOwner {
  mapId: string | null;
  generation: number;
  ownerToken: LinkRouteOwnerToken | null;
  mounted: boolean;
}

interface PendingLinkRoute {
  route: LinkRoute | null;
  sequence: number;
  owner: LinkRouteOwnerToken;
  authorityEpoch: number;
}

interface LinkRouteQueue {
  key: string;
  mapId: string;
  edgeId: string;
  running: boolean;
  pendingLatest: PendingLinkRoute | null;
  confirmed: LinkRoute | null;
  confirmedSequence: number;
  confirmedInitialized: boolean;
  latestSequence: number;
  overlayActive: boolean;
  overlayRoute: LinkRoute | null;
  overlaySequence: number;
  overlayOwner: LinkRouteOwnerToken | null;
  authorityEpoch: number;
  invalidated: boolean;
}

interface AuthoritativeLinkRoute {
  owner: LinkRouteOwnerToken;
  route: LinkRoute | null;
}

const linkRouteSaveError =
  "Couldn't save link route. The last saved route was restored; try again.";

function copyOptionalRoute(route: LinkRoute | null | undefined): LinkRoute | null {
  return route == null ? null : copyLinkRoute(route);
}

function queueKey(mapId: string, edgeId: string): string {
  return `${mapId}:${edgeId}`;
}

function linkRoutesEqual(
  left: LinkRoute | null | undefined,
  right: LinkRoute | null | undefined,
): boolean {
  if (left == null || right == null) {
    return left == null && right == null;
  }
  if (left.version !== right.version || left.waypoints.length !== right.waypoints.length) {
    return false;
  }
  return left.waypoints.every((waypoint, index) => {
    const other = right.waypoints[index];
    return other !== undefined && waypoint.x === other.x && waypoint.y === other.y;
  });
}

function hasOwner(owner: LinkRouteOwner, mutationOwner: LinkRouteOwnerToken): boolean {
  return owner.mounted && owner.ownerToken === mutationOwner;
}

function hasOverlayOwner(
  queue: LinkRouteQueue,
  overlayOwner: LinkRouteOwnerToken,
  authorityEpoch: number,
  sequence: number,
): boolean {
  return (
    !queue.invalidated &&
    queue.authorityEpoch === authorityEpoch &&
    queue.overlayActive &&
    queue.overlaySequence === sequence &&
    queue.overlayOwner === overlayOwner
  );
}

function clearRouteEditOwnership(edges: LinkEdgeType[]): LinkEdgeType[] {
  let nextEdges: LinkEdgeType[] | null = null;
  edges.forEach((edge, edgeIndex) => {
    if (
      edge.data?.routeEditToken === undefined &&
      edge.data?.routeEditable !== true &&
      edge.data?.onRouteCommit === undefined
    ) {
      return;
    }
    const data = { ...(edge.data ?? {}), routeEditable: false };
    delete data.routeEditToken;
    delete data.onRouteCommit;
    nextEdges ??= edges.slice();
    nextEdges[edgeIndex] = { ...edge, data };
  });
  return nextEdges ?? edges;
}

/** Coordinates optimistic route edits with ordered writes to the owning saved map. */
export function useCanvasLinkRoutes({
  mapId,
  setEdges,
  edgeIndexByIdRef,
}: UseCanvasLinkRoutesOptions): UseCanvasLinkRoutesResult {
  const [linkRouteError, setLinkRouteError] = useState<string | null>(null);
  const ownerRef = useRef<LinkRouteOwner>({
    mapId,
    generation: 0,
    ownerToken: mapId === null ? null : { mapId, generation: 0 },
    mounted: true,
  });
  const queuesRef = useRef(new Map<string, LinkRouteQueue>());
  const editTokensRef = useRef(new Map<string, LinkRouteEditToken>());
  const authoritativeRoutesRef = useRef(new Map<string, AuthoritativeLinkRoute>());
  const committedOwnerTokenRef = useRef(ownerRef.current.ownerToken);

  if (ownerRef.current.mapId !== mapId) {
    const generation = ownerRef.current.generation + 1;
    ownerRef.current = {
      mapId,
      generation,
      ownerToken: mapId === null ? null : { mapId, generation },
      mounted: true,
    };
  }
  const routeOwnerToken = ownerRef.current.ownerToken;

  useLayoutEffect(() => {
    if (committedOwnerTokenRef.current === routeOwnerToken) {
      return;
    }
    committedOwnerTokenRef.current = routeOwnerToken;
    setLinkRouteError(null);
    setEdges((currentEdges) => clearRouteEditOwnership(currentEdges));
  }, [routeOwnerToken, setEdges]);

  useEffect(() => {
    ownerRef.current.mounted = true;
    return () => {
      ownerRef.current = {
        ...ownerRef.current,
        generation: ownerRef.current.generation + 1,
        ownerToken: null,
        mounted: false,
      };
    };
  }, []);

  const getLinkRouteEditToken = useCallback((edgeId: string): LinkRouteEditToken | undefined => {
    const ownerToken = ownerRef.current.ownerToken;
    if (!ownerRef.current.mounted || ownerToken === null) {
      return undefined;
    }
    const key = queueKey(ownerToken.mapId, edgeId);
    const existing = editTokensRef.current.get(key);
    if (existing?.owner === ownerToken) {
      return existing;
    }
    const nextToken: LinkRouteEditToken = {
      owner: ownerToken,
      actionEpoch: existing === undefined ? 0 : existing.actionEpoch + 1,
    };
    editTokensRef.current.set(key, nextToken);
    return nextToken;
  }, []);

  const rotateLinkRouteEditToken = useCallback(
    (edgeId: string): LinkRouteEditToken | undefined => {
      const current = getLinkRouteEditToken(edgeId);
      if (current === undefined) {
        return undefined;
      }
      const nextToken: LinkRouteEditToken = {
        owner: current.owner,
        actionEpoch: current.actionEpoch + 1,
      };
      editTokensRef.current.set(queueKey(current.owner.mapId, edgeId), nextToken);
      return nextToken;
    },
    [getLinkRouteEditToken],
  );

  const ownsEditToken = useCallback((edgeId: string, editToken: LinkRouteEditToken): boolean => {
    return (
      hasOwner(ownerRef.current, editToken.owner) &&
      editTokensRef.current.get(queueKey(editToken.owner.mapId, edgeId)) === editToken
    );
  }, []);

  const patchEdgeRoute = useCallback(
    (
      queue: LinkRouteQueue,
      mutationOwner: LinkRouteOwnerToken,
      route: LinkRoute | null,
      canApply: () => boolean = () => true,
      initializeConfirmed = false,
    ) => {
      setEdges((currentEdges) => {
        if (!hasOwner(ownerRef.current, mutationOwner) || !canApply()) {
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
        const editToken = editTokensRef.current.get(queue.key);
        if (editToken?.owner === mutationOwner) {
          data.routeEditToken = editToken;
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
            if (
              queuesRef.current.get(queue.key) !== queue ||
              queue.invalidated ||
              mutation.authorityEpoch !== queue.authorityEpoch
            ) {
              return;
            }
            queue.confirmed = copyOptionalRoute(savedRoute);
            queue.confirmedSequence = mutation.sequence;
            queue.confirmedInitialized = true;
            if (queue.latestSequence === mutation.sequence) {
              const overlayOwner =
                queue.overlaySequence === mutation.sequence && queue.overlayOwner !== null
                  ? queue.overlayOwner
                  : mutation.owner;
              queue.overlayRoute = copyOptionalRoute(savedRoute);
              queue.overlayActive = true;
              queue.overlaySequence = mutation.sequence;
              queue.overlayOwner = overlayOwner;
              if (hasOwner(ownerRef.current, overlayOwner)) {
                patchEdgeRoute(
                  queue,
                  overlayOwner,
                  queue.overlayRoute,
                  () =>
                    queuesRef.current.get(queue.key) === queue &&
                    queue.latestSequence === mutation.sequence &&
                    hasOverlayOwner(
                      queue,
                      overlayOwner,
                      mutation.authorityEpoch,
                      mutation.sequence,
                    ),
                );
              }
            }
          },
          () => {
            if (
              queuesRef.current.get(queue.key) !== queue ||
              queue.invalidated ||
              mutation.authorityEpoch !== queue.authorityEpoch
            ) {
              return;
            }
            const isLatestMutation = queue.latestSequence === mutation.sequence;
            const hasNoNewerConfirmation = queue.confirmedSequence < mutation.sequence;
            const rollbackOwner =
              queue.overlayActive &&
              queue.overlaySequence === mutation.sequence &&
              queue.overlayOwner !== null
                ? queue.overlayOwner
                : null;
            if (!isLatestMutation || !hasNoNewerConfirmation || rollbackOwner === null) {
              return;
            }

            queue.overlayRoute = copyOptionalRoute(queue.confirmed);
            queue.overlayActive = true;
            if (!hasOwner(ownerRef.current, rollbackOwner)) {
              return;
            }
            patchEdgeRoute(
              queue,
              rollbackOwner,
              queue.confirmed,
              () =>
                queuesRef.current.get(queue.key) === queue &&
                queue.latestSequence === mutation.sequence &&
                queue.confirmedSequence < mutation.sequence &&
                hasOverlayOwner(queue, rollbackOwner, mutation.authorityEpoch, mutation.sequence),
            );
            setLinkRouteError((currentError) =>
              hasOwner(ownerRef.current, rollbackOwner) &&
              queuesRef.current.get(queue.key) === queue &&
              queue.latestSequence === mutation.sequence &&
              queue.confirmedSequence < mutation.sequence &&
              hasOverlayOwner(queue, rollbackOwner, mutation.authorityEpoch, mutation.sequence)
                ? linkRouteSaveError
                : currentError,
            );
          },
        )
        .finally(() => {
          queue.running = false;
          if (queue.invalidated && queue.pendingLatest === null) {
            if (queuesRef.current.get(queue.key) === queue) {
              queuesRef.current.delete(queue.key);
            }
            return;
          }
          drain(queue);
        });
    },
    [patchEdgeRoute],
  );

  const enqueueLinkRoute = useCallback(
    (edgeId: string, route: LinkRoute | null, editToken: LinkRouteEditToken) => {
      if (!ownsEditToken(edgeId, editToken)) {
        return;
      }

      const mutationOwner = editToken.owner;
      const key = queueKey(mutationOwner.mapId, edgeId);
      let queue = queuesRef.current.get(key);
      if (queue === undefined) {
        queue = {
          key,
          mapId: mutationOwner.mapId,
          edgeId,
          running: false,
          pendingLatest: null,
          confirmed: null,
          confirmedSequence: 0,
          confirmedInitialized: false,
          latestSequence: 0,
          overlayActive: false,
          overlayRoute: null,
          overlaySequence: 0,
          overlayOwner: null,
          authorityEpoch: 0,
          invalidated: false,
        };
        queuesRef.current.set(key, queue);
      }

      if (queue.invalidated) {
        queue.invalidated = false;
        queue.confirmed = null;
        queue.confirmedInitialized = false;
      }

      const nextRoute = copyOptionalRoute(route);
      const sequence = queue.latestSequence + 1;
      queue.latestSequence = sequence;
      const mutationEpoch = queue.authorityEpoch;
      queue.pendingLatest = {
        route: nextRoute,
        sequence,
        owner: mutationOwner,
        authorityEpoch: mutationEpoch,
      };
      queue.overlayRoute = copyOptionalRoute(nextRoute);
      queue.overlayActive = true;
      queue.overlaySequence = sequence;
      queue.overlayOwner = mutationOwner;
      patchEdgeRoute(
        queue,
        mutationOwner,
        nextRoute,
        () =>
          !queue.invalidated &&
          queue.authorityEpoch === mutationEpoch &&
          queue.latestSequence === sequence,
        true,
      );
      drainQueue(queue);
    },
    [drainQueue, ownsEditToken, patchEdgeRoute],
  );

  const commitLinkRoute = useCallback(
    (edgeId: string, route: LinkRoute | null) => {
      const editToken = getLinkRouteEditToken(edgeId);
      if (editToken !== undefined) {
        enqueueLinkRoute(edgeId, route, editToken);
      }
    },
    [enqueueLinkRoute, getLinkRouteEditToken],
  );

  const commitOwnedLinkRoute = useCallback(
    (edgeId: string, route: LinkRoute | null, editToken: LinkRouteEditToken | undefined) => {
      if (editToken === undefined || !ownsEditToken(edgeId, editToken)) {
        return;
      }
      enqueueLinkRoute(edgeId, route, editToken);
    },
    [enqueueLinkRoute, ownsEditToken],
  );

  const resetLinkRoute = useCallback(
    (edgeId: string) => {
      const ownerToken = ownerRef.current.ownerToken;
      if (ownerToken === null) {
        return;
      }
      const queue = queuesRef.current.get(queueKey(ownerToken.mapId, edgeId));
      if (queue !== undefined) {
        queue.authorityEpoch += 1;
        queue.pendingLatest = null;
      }
      const editToken = rotateLinkRouteEditToken(edgeId);
      if (editToken !== undefined) {
        enqueueLinkRoute(edgeId, null, editToken);
      }
    },
    [enqueueLinkRoute, rotateLinkRouteEditToken],
  );

  const reconcileLinkRouteEdges = useCallback(
    (edges: LinkEdgeType[]): LinkEdgeType[] => {
      const owner = ownerRef.current;
      if (!owner.mounted || owner.mapId === null || owner.ownerToken === null) {
        return edges;
      }
      const ownerMapId = owner.mapId;
      const currentEdgeIds = new Set(edges.map((edge) => edge.id));

      let reconciledEdges: LinkEdgeType[] | null = null;
      edges.forEach((edge, edgeIndex) => {
        const key = queueKey(ownerMapId, edge.id);
        const queue = queuesRef.current.get(key);
        const fetchedRoute = copyOptionalRoute(edge.data?.route);
        const previousAuthoritativeRoute = authoritativeRoutesRef.current.get(key);
        let actionTokenRotated = false;
        if (
          previousAuthoritativeRoute?.owner === owner.ownerToken &&
          !linkRoutesEqual(previousAuthoritativeRoute.route, fetchedRoute) &&
          queue?.overlayActive !== true
        ) {
          actionTokenRotated = rotateLinkRouteEditToken(edge.id) !== undefined;
        }
        authoritativeRoutesRef.current.set(key, {
          owner: owner.ownerToken!,
          route: copyOptionalRoute(fetchedRoute),
        });
        if (actionTokenRotated) {
          const data = {
            ...(edge.data ?? {}),
            routeEditToken: getLinkRouteEditToken(edge.id),
          };
          reconciledEdges ??= edges.slice();
          reconciledEdges[edgeIndex] = { ...edge, data };
        }
        if (queue === undefined) {
          return;
        }

        const queueIsIdle = !queue.running && queue.pendingLatest === null;
        if (!queue.overlayActive) {
          if (queueIsIdle) {
            queue.confirmed = fetchedRoute;
            queue.confirmedInitialized = true;
            queuesRef.current.delete(queue.key);
          }
          return;
        }

        if (linkRoutesEqual(fetchedRoute, queue.overlayRoute)) {
          if (queueIsIdle) {
            queue.confirmed = copyOptionalRoute(fetchedRoute);
            queue.confirmedInitialized = true;
            queue.overlayActive = false;
            queuesRef.current.delete(queue.key);
          }
          return;
        }

        const data = { ...(edge.data ?? {}) };
        queue.overlayOwner = owner.ownerToken;
        const editToken = getLinkRouteEditToken(edge.id);
        if (editToken !== undefined) {
          data.routeEditToken = editToken;
        }
        if (queue.overlayRoute === null) {
          delete data.route;
        } else {
          data.route = copyLinkRoute(queue.overlayRoute);
        }
        reconciledEdges ??= edges.slice();
        reconciledEdges[edgeIndex] = { ...edge, data };
      });

      for (const [key, queue] of queuesRef.current) {
        if (queue.mapId !== ownerMapId || currentEdgeIds.has(queue.edgeId)) {
          continue;
        }
        if (!queue.running) {
          queuesRef.current.delete(key);
          editTokensRef.current.delete(key);
          authoritativeRoutesRef.current.delete(key);
          continue;
        }
        if (!queue.invalidated) {
          queue.authorityEpoch += 1;
        }
        queue.invalidated = true;
        queue.pendingLatest = null;
        queue.confirmed = null;
        queue.confirmedInitialized = false;
        queue.overlayActive = false;
        queue.overlayRoute = null;
        queue.overlaySequence = 0;
        queue.overlayOwner = null;
        editTokensRef.current.delete(key);
        authoritativeRoutesRef.current.delete(key);
      }

      return reconciledEdges ?? edges;
    },
    [getLinkRouteEditToken, rotateLinkRouteEditToken],
  );

  const dismissLinkRouteError = useCallback(() => {
    setLinkRouteError(null);
  }, []);

  return {
    commitLinkRoute,
    commitOwnedLinkRoute,
    getLinkRouteEditToken,
    routeOwnerToken,
    resetLinkRoute,
    reconcileLinkRouteEdges,
    linkRouteError,
    dismissLinkRouteError,
  };
}
