/**
 * Exercises optimistic map-local link route persistence, ordering, rollback, and owner isolation.
 */
import { act, renderHook } from '@testing-library/react';
import { useLayoutEffect, useRef, useState } from 'react';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import type { LinkRoute } from '../../types/api';
import type { LinkEdgeType } from '../LinkEdge';
import { useCanvasLinkRoutes } from './useCanvasLinkRoutes';

const apiMocks = vi.hoisted(() => ({
  saveCanvasMapLinkRoute: vi.fn(),
  deleteCanvasMapLinkRoute: vi.fn(),
}));

vi.mock('../../api/client', () => apiMocks);

const ORIGINAL_ROUTE: LinkRoute = {
  version: 1,
  waypoints: [{ x: 10, y: 20 }],
};
const FIRST_ROUTE: LinkRoute = {
  version: 1,
  waypoints: [{ x: 30, y: 40 }],
};
const SECOND_ROUTE: LinkRoute = {
  version: 1,
  waypoints: [{ x: 50, y: 60 }],
};
const THIRD_ROUTE: LinkRoute = {
  version: 1,
  waypoints: [{ x: 70, y: 80 }],
};

function deferred<T>() {
  let resolve!: (value: T | PromiseLike<T>) => void;
  let reject!: (reason?: unknown) => void;
  const promise = new Promise<T>((promiseResolve, promiseReject) => {
    resolve = promiseResolve;
    reject = promiseReject;
  });
  return { promise, resolve, reject };
}

function mockEdge(id: string, route?: LinkRoute): LinkEdgeType {
  return {
    id,
    source: `${id}-source`,
    target: `${id}-target`,
    data: route === undefined ? {} : { route },
  } as LinkEdgeType;
}

function useLinkRouteHarness(mapId: string | null, initialEdges: LinkEdgeType[]) {
  const [edges, setEdges] = useState(initialEdges);
  const edgeIndexByIdRef = useRef<Map<string, number>>(new Map());

  useLayoutEffect(() => {
    edgeIndexByIdRef.current = new Map(edges.map((edge, index) => [edge.id, index]));
  }, [edges]);

  const controller = useCanvasLinkRoutes({ mapId, setEdges, edgeIndexByIdRef });
  return { ...controller, edges, replaceEdges: setEdges };
}

async function flushAsyncWork() {
  await Promise.resolve();
  await Promise.resolve();
}

describe('useCanvasLinkRoutes', () => {
  beforeEach(() => {
    apiMocks.saveCanvasMapLinkRoute.mockReset();
    apiMocks.deleteCanvasMapLinkRoute.mockReset();
  });

  it('optimistically replaces only the indexed edge before its save resolves', async () => {
    const save = deferred<LinkRoute>();
    apiMocks.saveCanvasMapLinkRoute.mockReturnValueOnce(save.promise);
    const firstEdge = mockEdge('link-a', ORIGINAL_ROUTE);
    const untouchedEdge = mockEdge('link-b', SECOND_ROUTE);
    const { result } = renderHook(() => useLinkRouteHarness('map-a', [firstEdge, untouchedEdge]));

    await act(async () => {
      result.current.commitLinkRoute('link-a', FIRST_ROUTE);
      await flushAsyncWork();
    });

    expect(result.current.edges[0]).not.toBe(firstEdge);
    expect(result.current.edges[0]?.data?.route).toEqual(FIRST_ROUTE);
    expect(result.current.edges[1]).toBe(untouchedEdge);
    expect(apiMocks.saveCanvasMapLinkRoute).toHaveBeenCalledWith('map-a', 'link-a', FIRST_ROUTE);
  });

  it('runs persistence for different links independently', async () => {
    const firstSave = deferred<LinkRoute>();
    const secondSave = deferred<LinkRoute>();
    apiMocks.saveCanvasMapLinkRoute.mockImplementation((_mapId, linkId) =>
      linkId === 'link-a' ? firstSave.promise : secondSave.promise,
    );
    const { result } = renderHook(() =>
      useLinkRouteHarness('map-a', [
        mockEdge('link-a', ORIGINAL_ROUTE),
        mockEdge('link-b', ORIGINAL_ROUTE),
      ]),
    );

    await act(async () => {
      result.current.commitLinkRoute('link-a', FIRST_ROUTE);
      result.current.commitLinkRoute('link-b', SECOND_ROUTE);
      await flushAsyncWork();
    });

    expect(apiMocks.saveCanvasMapLinkRoute).toHaveBeenCalledTimes(2);
    expect(apiMocks.saveCanvasMapLinkRoute).toHaveBeenCalledWith('map-a', 'link-a', FIRST_ROUTE);
    expect(apiMocks.saveCanvasMapLinkRoute).toHaveBeenCalledWith('map-a', 'link-b', SECOND_ROUTE);
  });

  it('serializes one link and replaces an unsent intermediate route with the latest route', async () => {
    const firstSave = deferred<LinkRoute>();
    const latestSave = deferred<LinkRoute>();
    apiMocks.saveCanvasMapLinkRoute
      .mockReturnValueOnce(firstSave.promise)
      .mockReturnValueOnce(latestSave.promise);
    const { result } = renderHook(() =>
      useLinkRouteHarness('map-a', [mockEdge('link-a', ORIGINAL_ROUTE)]),
    );

    await act(async () => {
      result.current.commitLinkRoute('link-a', FIRST_ROUTE);
      await flushAsyncWork();
    });
    act(() => {
      result.current.commitLinkRoute('link-a', SECOND_ROUTE);
      result.current.commitLinkRoute('link-a', THIRD_ROUTE);
    });

    expect(apiMocks.saveCanvasMapLinkRoute).toHaveBeenCalledTimes(1);
    expect(result.current.edges[0]?.data?.route).toEqual(THIRD_ROUTE);

    await act(async () => {
      firstSave.resolve(FIRST_ROUTE);
      await firstSave.promise;
      await flushAsyncWork();
    });

    expect(apiMocks.saveCanvasMapLinkRoute).toHaveBeenCalledTimes(2);
    expect(apiMocks.saveCanvasMapLinkRoute).toHaveBeenLastCalledWith(
      'map-a',
      'link-a',
      THIRD_ROUTE,
    );
  });

  it('keeps one write queue when the same map and link are revisited before a save settles', async () => {
    const firstSave = deferred<LinkRoute>();
    const revisitedSave = deferred<LinkRoute>();
    apiMocks.saveCanvasMapLinkRoute
      .mockReturnValueOnce(firstSave.promise)
      .mockReturnValueOnce(revisitedSave.promise);
    const { result, rerender } = renderHook(
      ({ mapId }) => useLinkRouteHarness(mapId, [mockEdge('link-a', ORIGINAL_ROUTE)]),
      { initialProps: { mapId: 'map-a' as string | null } },
    );

    await act(async () => {
      result.current.commitLinkRoute('link-a', FIRST_ROUTE);
      await flushAsyncWork();
    });

    rerender({ mapId: 'map-b' });
    act(() => {
      result.current.replaceEdges([mockEdge('link-a', THIRD_ROUTE)]);
    });
    rerender({ mapId: 'map-a' });
    act(() => {
      result.current.replaceEdges([mockEdge('link-a', ORIGINAL_ROUTE)]);
      result.current.commitLinkRoute('link-a', SECOND_ROUTE);
    });

    expect(result.current.edges[0]?.data?.route).toEqual(SECOND_ROUTE);
    expect(apiMocks.saveCanvasMapLinkRoute).toHaveBeenCalledTimes(1);

    await act(async () => {
      firstSave.resolve(FIRST_ROUTE);
      await firstSave.promise;
      await flushAsyncWork();
    });

    expect(apiMocks.saveCanvasMapLinkRoute).toHaveBeenCalledTimes(2);
    expect(apiMocks.saveCanvasMapLinkRoute).toHaveBeenLastCalledWith(
      'map-a',
      'link-a',
      SECOND_ROUTE,
    );
    expect(result.current.edges[0]?.data?.route).toEqual(SECOND_ROUTE);

    await act(async () => {
      revisitedSave.resolve(SECOND_ROUTE);
      await revisitedSave.promise;
      await flushAsyncWork();
    });
  });

  it('overlays stale refresh edges while a PUT is pending and until the saved route catches up', async () => {
    const save = deferred<LinkRoute>();
    apiMocks.saveCanvasMapLinkRoute.mockReturnValueOnce(save.promise);
    const { result } = renderHook(() =>
      useLinkRouteHarness('map-a', [mockEdge('link-a', ORIGINAL_ROUTE)]),
    );

    await act(async () => {
      result.current.commitLinkRoute('link-a', FIRST_ROUTE);
      await flushAsyncWork();
    });

    const staleEdge = mockEdge('link-a', ORIGINAL_ROUTE);
    const staleRefresh = [staleEdge];
    const optimisticEdges = result.current.reconcileLinkRouteEdges(staleRefresh);

    expect(optimisticEdges).not.toBe(staleRefresh);
    expect(optimisticEdges[0]).not.toBe(staleEdge);
    expect(optimisticEdges[0]?.data?.route).toEqual(FIRST_ROUTE);
    expect(staleEdge.data?.route).toEqual(ORIGINAL_ROUTE);

    const optimisticRoute = optimisticEdges[0]?.data?.route;
    if (optimisticRoute === undefined) {
      throw new Error('expected an optimistic route overlay');
    }
    optimisticRoute.waypoints[0]!.x = 999;
    expect(result.current.reconcileLinkRouteEdges(staleRefresh)[0]?.data?.route).toEqual(
      FIRST_ROUTE,
    );

    await act(async () => {
      save.resolve(FIRST_ROUTE);
      await save.promise;
      await flushAsyncWork();
    });

    expect(result.current.reconcileLinkRouteEdges(staleRefresh)[0]?.data?.route).toEqual(
      FIRST_ROUTE,
    );
    const caughtUpRefresh = [mockEdge('link-a', FIRST_ROUTE)];
    const acceptedEdges = result.current.reconcileLinkRouteEdges(caughtUpRefresh);
    expect(acceptedEdges).toBe(caughtUpRefresh);
    expect(acceptedEdges[0]).toBe(caughtUpRefresh[0]);
  });

  it('evicts a confirmed overlay when authoritative topology no longer contains the link', async () => {
    const save = deferred<LinkRoute>();
    apiMocks.saveCanvasMapLinkRoute.mockReturnValueOnce(save.promise);
    const { result } = renderHook(() =>
      useLinkRouteHarness('map-a', [mockEdge('link-a', ORIGINAL_ROUTE)]),
    );

    await act(async () => {
      result.current.commitLinkRoute('link-a', FIRST_ROUTE);
      await flushAsyncWork();
      save.resolve(FIRST_ROUTE);
      await save.promise;
      await flushAsyncWork();
    });

    const absentRefresh: LinkEdgeType[] = [];
    expect(result.current.reconcileLinkRouteEdges(absentRefresh)).toBe(absentRefresh);

    const readdedWithAutomaticRouting = [mockEdge('link-a')];
    const reconciledReaddedEdges = result.current.reconcileLinkRouteEdges(
      readdedWithAutomaticRouting,
    );
    expect(reconciledReaddedEdges).toBe(readdedWithAutomaticRouting);
    expect(reconciledReaddedEdges[0]?.data).not.toHaveProperty('route');
  });

  it('does not let a tombstoned in-flight PUT patch a link that reappears automatically', async () => {
    const save = deferred<LinkRoute>();
    apiMocks.saveCanvasMapLinkRoute.mockReturnValueOnce(save.promise);
    const { result } = renderHook(() =>
      useLinkRouteHarness('map-a', [mockEdge('link-a', ORIGINAL_ROUTE)]),
    );

    await act(async () => {
      result.current.commitLinkRoute('link-a', FIRST_ROUTE);
      await flushAsyncWork();
    });
    const absentRefresh: LinkEdgeType[] = [];
    expect(result.current.reconcileLinkRouteEdges(absentRefresh)).toBe(absentRefresh);
    act(() => {
      result.current.replaceEdges(absentRefresh);
    });

    const readdedEdge = mockEdge('link-a');
    act(() => {
      result.current.replaceEdges([readdedEdge]);
    });

    await act(async () => {
      save.resolve(FIRST_ROUTE);
      await save.promise;
      await flushAsyncWork();
    });

    expect(result.current.edges[0]).toBe(readdedEdge);
    expect(result.current.edges[0]?.data).not.toHaveProperty('route');
    expect(result.current.linkRouteError).toBeNull();
  });

  it('serializes a new edit behind a tombstoned request when the link reappears', async () => {
    const staleSave = deferred<LinkRoute>();
    const readdedSave = deferred<LinkRoute>();
    apiMocks.saveCanvasMapLinkRoute
      .mockReturnValueOnce(staleSave.promise)
      .mockReturnValueOnce(readdedSave.promise);
    const { result } = renderHook(() =>
      useLinkRouteHarness('map-a', [mockEdge('link-a', ORIGINAL_ROUTE)]),
    );

    await act(async () => {
      result.current.commitLinkRoute('link-a', FIRST_ROUTE);
      await flushAsyncWork();
    });
    const absentRefresh: LinkEdgeType[] = [];
    result.current.reconcileLinkRouteEdges(absentRefresh);
    act(() => {
      result.current.replaceEdges(absentRefresh);
    });

    const readdedWithAutomaticRouting = [mockEdge('link-a')];
    expect(result.current.reconcileLinkRouteEdges(readdedWithAutomaticRouting)).toBe(
      readdedWithAutomaticRouting,
    );
    act(() => {
      result.current.replaceEdges(readdedWithAutomaticRouting);
    });
    act(() => {
      result.current.commitLinkRoute('link-a', SECOND_ROUTE);
    });

    expect(result.current.edges[0]?.data?.route).toEqual(SECOND_ROUTE);
    expect(apiMocks.saveCanvasMapLinkRoute).toHaveBeenCalledTimes(1);

    await act(async () => {
      staleSave.resolve(FIRST_ROUTE);
      await staleSave.promise;
      await flushAsyncWork();
    });

    expect(apiMocks.saveCanvasMapLinkRoute).toHaveBeenCalledTimes(2);
    expect(apiMocks.saveCanvasMapLinkRoute).toHaveBeenLastCalledWith(
      'map-a',
      'link-a',
      SECOND_ROUTE,
    );
    expect(result.current.edges[0]?.data?.route).toEqual(SECOND_ROUTE);

    await act(async () => {
      readdedSave.resolve(SECOND_ROUTE);
      await readdedSave.promise;
      await flushAsyncWork();
    });
  });

  it('keeps a DELETE overlay across stale refreshes until automatic routing catches up', async () => {
    const deletion = deferred<void>();
    apiMocks.deleteCanvasMapLinkRoute.mockReturnValueOnce(deletion.promise);
    const { result } = renderHook(() =>
      useLinkRouteHarness('map-a', [mockEdge('link-a', ORIGINAL_ROUTE)]),
    );

    await act(async () => {
      result.current.resetLinkRoute('link-a');
      await flushAsyncWork();
    });

    const staleEdge = mockEdge('link-a', ORIGINAL_ROUTE);
    const staleRefresh = [staleEdge];
    const pendingDeleteEdges = result.current.reconcileLinkRouteEdges(staleRefresh);
    expect(pendingDeleteEdges[0]?.data).not.toHaveProperty('route');
    expect(staleEdge.data?.route).toEqual(ORIGINAL_ROUTE);

    await act(async () => {
      deletion.resolve();
      await deletion.promise;
      await flushAsyncWork();
    });

    expect(result.current.reconcileLinkRouteEdges(staleRefresh)[0]?.data).not.toHaveProperty(
      'route',
    );
    const caughtUpRefresh = [mockEdge('link-a')];
    expect(result.current.reconcileLinkRouteEdges(caughtUpRefresh)).toBe(caughtUpRefresh);
  });

  it('advances the confirmed route on success and restores it when the latest save fails', async () => {
    const firstSave = deferred<LinkRoute>();
    const secondSave = deferred<LinkRoute>();
    apiMocks.saveCanvasMapLinkRoute
      .mockReturnValueOnce(firstSave.promise)
      .mockReturnValueOnce(secondSave.promise);
    const { result } = renderHook(() =>
      useLinkRouteHarness('map-a', [mockEdge('link-a', ORIGINAL_ROUTE)]),
    );

    await act(async () => {
      result.current.commitLinkRoute('link-a', FIRST_ROUTE);
      await flushAsyncWork();
      firstSave.resolve(FIRST_ROUTE);
      await firstSave.promise;
      await flushAsyncWork();
    });

    await act(async () => {
      result.current.commitLinkRoute('link-a', SECOND_ROUTE);
      await flushAsyncWork();
      secondSave.reject(new Error('route save failed'));
      await secondSave.promise.catch(() => undefined);
      await flushAsyncWork();
    });

    expect(result.current.edges[0]?.data?.route).toEqual(FIRST_ROUTE);
    expect(result.current.linkRouteError).toMatch(/couldn't save.*restored/i);
  });

  it('does not roll back or report a superseded failure while a newer route is queued', async () => {
    const failedSave = deferred<LinkRoute>();
    const latestSave = deferred<LinkRoute>();
    apiMocks.saveCanvasMapLinkRoute
      .mockReturnValueOnce(failedSave.promise)
      .mockReturnValueOnce(latestSave.promise);
    const { result } = renderHook(() =>
      useLinkRouteHarness('map-a', [mockEdge('link-a', ORIGINAL_ROUTE)]),
    );

    await act(async () => {
      result.current.commitLinkRoute('link-a', FIRST_ROUTE);
      await flushAsyncWork();
    });
    act(() => {
      result.current.commitLinkRoute('link-a', SECOND_ROUTE);
    });

    await act(async () => {
      failedSave.reject(new Error('superseded failure'));
      await failedSave.promise.catch(() => undefined);
      await flushAsyncWork();
    });

    expect(result.current.edges[0]?.data?.route).toEqual(SECOND_ROUTE);
    expect(result.current.linkRouteError).toBeNull();
    expect(apiMocks.saveCanvasMapLinkRoute).toHaveBeenLastCalledWith(
      'map-a',
      'link-a',
      SECOND_ROUTE,
    );

    await act(async () => {
      latestSave.resolve(SECOND_ROUTE);
      await latestSave.promise;
      await flushAsyncWork();
    });
  });

  it('ignores a stale failure after the active map changes', async () => {
    const oldMapSave = deferred<LinkRoute>();
    apiMocks.saveCanvasMapLinkRoute.mockReturnValueOnce(oldMapSave.promise);
    const mapBEdge = mockEdge('link-a', THIRD_ROUTE);
    const { result, rerender } = renderHook(
      ({ mapId }) => useLinkRouteHarness(mapId, [mockEdge('link-a', ORIGINAL_ROUTE)]),
      { initialProps: { mapId: 'map-a' as string | null } },
    );

    await act(async () => {
      result.current.commitLinkRoute('link-a', FIRST_ROUTE);
      await flushAsyncWork();
    });

    rerender({ mapId: 'map-b' });
    act(() => {
      result.current.replaceEdges([mapBEdge]);
    });

    await act(async () => {
      oldMapSave.reject(new Error('late old-map failure'));
      await oldMapSave.promise.catch(() => undefined);
      await flushAsyncWork();
    });

    expect(result.current.edges).toEqual([mapBEdge]);
    expect(result.current.linkRouteError).toBeNull();
  });

  it('retains an old-map failure rollback for reconciliation when that map is revisited', async () => {
    const oldMapSave = deferred<LinkRoute>();
    apiMocks.saveCanvasMapLinkRoute.mockReturnValueOnce(oldMapSave.promise);
    const { result, rerender } = renderHook(
      ({ mapId }) => useLinkRouteHarness(mapId, [mockEdge('link-a', ORIGINAL_ROUTE)]),
      { initialProps: { mapId: 'map-a' as string | null } },
    );

    await act(async () => {
      result.current.commitLinkRoute('link-a', FIRST_ROUTE);
      await flushAsyncWork();
    });
    rerender({ mapId: 'map-b' });
    const mapBEdge = mockEdge('link-a', THIRD_ROUTE);
    act(() => {
      result.current.replaceEdges([mapBEdge]);
    });

    await act(async () => {
      oldMapSave.reject(new Error('late old-map failure'));
      await oldMapSave.promise.catch(() => undefined);
      await flushAsyncWork();
    });

    expect(result.current.edges).toEqual([mapBEdge]);
    expect(result.current.linkRouteError).toBeNull();

    rerender({ mapId: 'map-a' });
    const refreshedMapAEdges = [mockEdge('link-a', ORIGINAL_ROUTE)];
    expect(result.current.reconcileLinkRouteEdges(refreshedMapAEdges)).toBe(refreshedMapAEdges);
  });

  it('uses DELETE and removes optimistic route data for automatic routing', async () => {
    const deletion = deferred<void>();
    apiMocks.deleteCanvasMapLinkRoute.mockReturnValueOnce(deletion.promise);
    const { result } = renderHook(() =>
      useLinkRouteHarness('map-a', [mockEdge('link-a', ORIGINAL_ROUTE)]),
    );

    await act(async () => {
      result.current.commitLinkRoute('link-a', null);
      await flushAsyncWork();
    });

    expect(apiMocks.deleteCanvasMapLinkRoute).toHaveBeenCalledWith('map-a', 'link-a');
    expect(apiMocks.saveCanvasMapLinkRoute).not.toHaveBeenCalled();
    expect(result.current.edges[0]?.data?.route).toBeUndefined();
    expect(result.current.edges[0]?.data).not.toHaveProperty('route');
  });

  it('reset delegates to the same idempotent DELETE mutation path', async () => {
    apiMocks.deleteCanvasMapLinkRoute.mockResolvedValueOnce(undefined);
    const { result } = renderHook(() =>
      useLinkRouteHarness('map-a', [mockEdge('link-a', ORIGINAL_ROUTE)]),
    );

    await act(async () => {
      result.current.resetLinkRoute('link-a');
      await flushAsyncWork();
    });

    expect(apiMocks.deleteCanvasMapLinkRoute).toHaveBeenCalledOnce();
    expect(apiMocks.deleteCanvasMapLinkRoute).toHaveBeenCalledWith('map-a', 'link-a');
    expect(result.current.edges[0]?.data).not.toHaveProperty('route');
  });

  it('dismisses only the failure notice and keeps the restored route intact', async () => {
    const failedSave = deferred<LinkRoute>();
    apiMocks.saveCanvasMapLinkRoute.mockReturnValueOnce(failedSave.promise);
    const { result } = renderHook(() =>
      useLinkRouteHarness('map-a', [mockEdge('link-a', ORIGINAL_ROUTE)]),
    );

    await act(async () => {
      result.current.commitLinkRoute('link-a', FIRST_ROUTE);
      await flushAsyncWork();
      failedSave.reject(new Error('route save failed'));
      await failedSave.promise.catch(() => undefined);
      await flushAsyncWork();
    });
    const restoredEdges = result.current.edges;

    act(() => {
      result.current.dismissLinkRouteError();
    });

    expect(result.current.linkRouteError).toBeNull();
    expect(result.current.edges).toBe(restoredEdges);
    expect(result.current.edges[0]?.data?.route).toEqual(ORIGINAL_ROUTE);
  });

  it('does not patch edge state or report a late failure after unmount', async () => {
    const failedSave = deferred<LinkRoute>();
    apiMocks.saveCanvasMapLinkRoute.mockReturnValueOnce(failedSave.promise);
    const setEdges = vi.fn();
    const edgeIndexByIdRef = { current: new Map([['link-a', 0]]) };
    const { result, unmount } = renderHook(() =>
      useCanvasLinkRoutes({ mapId: 'map-a', setEdges, edgeIndexByIdRef }),
    );

    act(() => {
      result.current.commitLinkRoute('link-a', FIRST_ROUTE);
    });
    expect(setEdges).toHaveBeenCalledOnce();

    unmount();
    setEdges.mockClear();
    await act(async () => {
      failedSave.reject(new Error('late unmounted failure'));
      await failedSave.promise.catch(() => undefined);
      await flushAsyncWork();
    });

    expect(setEdges).not.toHaveBeenCalled();
  });

  it('does not mutate or persist routes without a resolved map ID', async () => {
    const edge = mockEdge('link-a', ORIGINAL_ROUTE);
    const { result } = renderHook(() => useLinkRouteHarness(null, [edge]));

    await act(async () => {
      result.current.commitLinkRoute('link-a', FIRST_ROUTE);
      result.current.resetLinkRoute('link-a');
      await flushAsyncWork();
    });

    expect(result.current.edges).toEqual([edge]);
    expect(apiMocks.saveCanvasMapLinkRoute).not.toHaveBeenCalled();
    expect(apiMocks.deleteCanvasMapLinkRoute).not.toHaveBeenCalled();
    expect(result.current.linkRouteError).toBeNull();
  });
});
