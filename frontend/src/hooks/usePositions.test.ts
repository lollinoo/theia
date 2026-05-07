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

  it('flushes pending saves to the previous endpoint when mapId changes', async () => {
    vi.mocked(fetch).mockResolvedValue(jsonResponse({ data: [] }));
    const { result, rerender } = renderHook(({ mapId }) => usePositions(mapId), {
      initialProps: { mapId: 'map-a' as string | null },
    });

    await act(async () => {
      await result.current.savePositions([
        { device_id: 'device-1', x: 1, y: 2, pinned: true },
      ]);
    });

    rerender({ mapId: 'map-b' });

    await act(async () => {
      await vi.runOnlyPendingTimersAsync();
    });

    expect(fetch).toHaveBeenCalledWith(
      '/api/v1/canvas/maps/map-a/positions',
      expect.objectContaining({ method: 'PUT' }),
    );
  });
});

function jsonResponse(payload: unknown): Response {
  return {
    ok: true,
    json: vi.fn().mockResolvedValue(payload),
  } as unknown as Response;
}
