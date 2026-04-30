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
    const { result } = renderHook(() => usePositions());

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
    const { result } = renderHook(() => usePositions());

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
});
