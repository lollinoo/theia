/**
 * Exercises use positions hook lifecycle behavior so refactors preserve the documented contract.
 */
import { act, renderHook } from '@testing-library/react';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import {
  exportCanvasDiagnostics,
  resetCanvasDiagnostics,
} from '../components/canvas/canvasDiagnostics';
import { usePositions } from './usePositions';

describe('usePositions diagnostics', () => {
  beforeEach(() => {
    vi.useFakeTimers();
    vi.setSystemTime(new Date('2026-04-30T12:00:00Z'));
    resetCanvasDiagnostics();
    vi.stubGlobal('fetch', vi.fn());
  });

  afterEach(() => {
    vi.useRealTimers();
    vi.restoreAllMocks();
  });

  it('records queued and successful debounced saves', async () => {
    vi.mocked(fetch).mockResolvedValue({
      ok: true,
      json: vi.fn().mockResolvedValue({}),
    } as unknown as Response);
    const { result } = renderHook(() => usePositions(null));

    await act(async () => {
      await result.current.savePositions([{ device_id: 'dev-1', x: 10, y: 20, pinned: true }]);
    });

    expect(exportCanvasDiagnostics().diagnostics.positions).toMatchObject({
      pendingSaveCount: 1,
      lastSaveStatus: 'pending',
    });

    await act(async () => {
      await vi.advanceTimersByTimeAsync(1000);
    });

    expect(exportCanvasDiagnostics().diagnostics.positions).toMatchObject({
      pendingSaveCount: 0,
      lastSaveStatus: 'success',
    });
    expect(exportCanvasDiagnostics().events.map((event) => event.event)).toEqual(
      expect.arrayContaining(['positions.save.queued', 'positions.save.succeeded']),
    );
  });

  it('sends the CSRF cookie value when saving positions', async () => {
    Object.defineProperty(document, 'cookie', {
      configurable: true,
      value: 'theme=dark; theia_csrf=position-csrf-token',
    });
    vi.mocked(fetch).mockResolvedValue({
      ok: true,
      json: vi.fn().mockResolvedValue({}),
    } as unknown as Response);
    const { result } = renderHook(() => usePositions(null));

    await act(async () => {
      await result.current.savePositions([{ device_id: 'dev-1', x: 10, y: 20, pinned: true }]);
      await vi.advanceTimersByTimeAsync(1000);
    });

    expect(fetch).toHaveBeenCalledWith(
      '/api/v1/positions',
      expect.objectContaining({
        method: 'PUT',
        headers: expect.objectContaining({ 'X-CSRF-Token': 'position-csrf-token' }),
      }),
    );
  });

  it('records failed position saves without throwing from the debounced flush', async () => {
    vi.mocked(fetch).mockResolvedValue({
      ok: false,
      status: 500,
      statusText: 'server error',
      json: vi.fn().mockResolvedValue({ error: 'database unavailable' }),
    } as unknown as Response);
    const consoleError = vi.spyOn(console, 'error').mockImplementation(() => undefined);
    const { result } = renderHook(() => usePositions(null));

    await act(async () => {
      await result.current.savePositions([{ device_id: 'dev-1', x: 10, y: 20, pinned: true }]);
      await vi.advanceTimersByTimeAsync(1000);
    });

    expect(consoleError).toHaveBeenCalled();
    expect(exportCanvasDiagnostics().diagnostics.positions).toMatchObject({
      pendingSaveCount: 0,
      lastSaveStatus: 'error',
      lastSaveError: expect.stringContaining('database unavailable'),
    });
    expect(exportCanvasDiagnostics().events.map((event) => event.event)).toContain(
      'positions.save.failed',
    );
  });

  it('uses the default positions endpoint when mapId is null', async () => {
    vi.mocked(fetch).mockResolvedValue(jsonResponse({ data: [] }));
    const { result } = renderHook(() => usePositions(null));

    await act(async () => {
      await result.current.fetchPositions();
    });

    expect(fetch).toHaveBeenCalledWith('/api/v1/positions', expect.any(Object));
  });

  it('uses the map positions endpoint when mapId is set', async () => {
    vi.mocked(fetch).mockResolvedValue(jsonResponse({ data: [] }));
    const { result } = renderHook(() => usePositions('map-1'));

    await act(async () => {
      await result.current.fetchPositions();
    });

    expect(fetch).toHaveBeenCalledWith('/api/v1/canvas/maps/map-1/positions', expect.any(Object));
  });

  it('encodes map IDs in positions endpoints', async () => {
    vi.mocked(fetch).mockResolvedValue(jsonResponse({ data: [] }));
    const { result } = renderHook(() => usePositions('floor 1/a'));

    await act(async () => {
      await result.current.fetchPositions();
    });

    expect(fetch).toHaveBeenCalledWith(
      '/api/v1/canvas/maps/floor%201%2Fa/positions',
      expect.any(Object),
    );
  });

  it('ignores stale fetch results after mapId changes', async () => {
    const mapAFetch = createDeferred<Response>();
    const mapBFetch = createDeferred<Response>();
    vi.mocked(fetch).mockReturnValueOnce(mapAFetch.promise).mockReturnValueOnce(mapBFetch.promise);
    const { result, rerender } = renderHook(({ mapId }) => usePositions(mapId), {
      initialProps: { mapId: 'map-a' as string | null },
    });

    let staleFetchResult: Map<string, unknown> | undefined;
    await act(async () => {
      const fetchPromise = result.current.fetchPositions();
      void fetchPromise.then((positions) => {
        staleFetchResult = positions;
      });
    });

    rerender({ mapId: 'map-b' });

    await act(async () => {
      mapAFetch.resolve(
        jsonResponse({
          data: [{ device_id: 'device-a', x: 1, y: 2, pinned: true }],
        }),
      );
      await mapAFetch.promise;
      await flushMicrotasks();
    });

    expect(staleFetchResult).toEqual(new Map());
    expect(Array.from(result.current.positions.keys())).toEqual([]);

    let freshFetchResult: Map<string, unknown> | undefined;
    await act(async () => {
      const fetchPromise = result.current.fetchPositions();
      mapBFetch.resolve(
        jsonResponse({
          data: [{ device_id: 'device-b', x: 3, y: 4, pinned: false }],
        }),
      );
      freshFetchResult = await fetchPromise;
    });

    expect(Array.from(freshFetchResult?.keys() ?? [])).toEqual(['device-b']);
    expect(Array.from(result.current.positions.keys())).toEqual(['device-b']);
  });

  it('flushes pending saves to the previous endpoint when mapId changes without duplicating timers', async () => {
    vi.mocked(fetch).mockResolvedValue(jsonResponse({ data: [] }));
    const { result, rerender } = renderHook(({ mapId }) => usePositions(mapId), {
      initialProps: { mapId: 'map-a' as string | null },
    });

    await act(async () => {
      await result.current.savePositions([{ device_id: 'device-1', x: 1, y: 2, pinned: true }]);
    });

    expect(fetch).not.toHaveBeenCalled();

    rerender({ mapId: 'map-b' });

    expect(fetch).toHaveBeenCalledTimes(1);
    expect(fetch).toHaveBeenCalledWith(
      '/api/v1/canvas/maps/map-a/positions',
      expect.objectContaining({ method: 'PUT' }),
    );

    await act(async () => {
      await vi.runOnlyPendingTimersAsync();
    });

    expect(fetch).toHaveBeenCalledTimes(1);
  });

  it('keeps newer pending save diagnostics when an older map-change flush succeeds', async () => {
    const oldFlush = createDeferred<Response>();
    vi.mocked(fetch).mockImplementation((input, init) => {
      if (init?.method === 'PUT' && input === '/api/v1/canvas/maps/map-a/positions') {
        return oldFlush.promise;
      }
      return Promise.resolve(jsonResponse({ data: [] }));
    });
    const { result, rerender } = renderHook(({ mapId }) => usePositions(mapId), {
      initialProps: { mapId: 'map-a' as string | null },
    });

    await act(async () => {
      await result.current.savePositions([{ device_id: 'device-a', x: 1, y: 2, pinned: true }]);
    });
    rerender({ mapId: 'map-b' });
    await act(async () => {
      await result.current.savePositions([{ device_id: 'device-b', x: 3, y: 4, pinned: false }]);
    });

    await act(async () => {
      oldFlush.resolve(jsonResponse({ data: [] }));
      await oldFlush.promise;
      await flushMicrotasks();
    });

    expect(exportCanvasDiagnostics().diagnostics.positions).toMatchObject({
      pendingSaveCount: 1,
      lastSaveStatus: 'pending',
      lastSaveError: undefined,
    });
  });

  it('keeps newer pending save diagnostics when an older map-change flush fails', async () => {
    const oldFlush = createDeferred<Response>();
    vi.mocked(fetch).mockImplementation((input, init) => {
      if (init?.method === 'PUT' && input === '/api/v1/canvas/maps/map-a/positions') {
        return oldFlush.promise;
      }
      return Promise.resolve(jsonResponse({ data: [] }));
    });
    const consoleError = vi.spyOn(console, 'error').mockImplementation(() => undefined);
    const { result, rerender } = renderHook(({ mapId }) => usePositions(mapId), {
      initialProps: { mapId: 'map-a' as string | null },
    });

    await act(async () => {
      await result.current.savePositions([{ device_id: 'device-a', x: 1, y: 2, pinned: true }]);
    });
    rerender({ mapId: 'map-b' });
    await act(async () => {
      await result.current.savePositions([{ device_id: 'device-b', x: 3, y: 4, pinned: false }]);
    });

    await act(async () => {
      oldFlush.resolve(errorResponse(500, 'server error', { error: 'old save failed' }));
      await oldFlush.promise;
      await flushMicrotasks();
    });

    expect(consoleError).toHaveBeenCalled();
    expect(exportCanvasDiagnostics().diagnostics.positions).toMatchObject({
      pendingSaveCount: 1,
      lastSaveStatus: 'pending',
      lastSaveError: undefined,
    });
  });
});

function jsonResponse(payload: unknown): Response {
  return {
    ok: true,
    json: vi.fn().mockResolvedValue(payload),
  } as unknown as Response;
}

function errorResponse(status: number, statusText: string, payload: unknown): Response {
  return {
    ok: false,
    status,
    statusText,
    json: vi.fn().mockResolvedValue(payload),
  } as unknown as Response;
}

function createDeferred<T>() {
  let resolve!: (value: T) => void;
  let reject!: (reason?: unknown) => void;
  const promise = new Promise<T>((resolvePromise, rejectPromise) => {
    resolve = resolvePromise;
    reject = rejectPromise;
  });

  return { promise, resolve, reject };
}

async function flushMicrotasks() {
  await Promise.resolve();
  await Promise.resolve();
}
