import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { renderHook, act } from '@testing-library/react';
import { useBridgeHealth } from './useBridgeHealth';

describe('useBridgeHealth', () => {
  beforeEach(() => {
    vi.useFakeTimers();
    vi.stubGlobal('fetch', vi.fn());
  });

  afterEach(() => {
    vi.useRealTimers();
    vi.restoreAllMocks();
  });

  it('returns bridgeRunning=true when health check succeeds', async () => {
    (global.fetch as ReturnType<typeof vi.fn>).mockResolvedValue({ ok: true });
    const { result } = renderHook(() => useBridgeHealth('1337'));
    // Flush the initial check promise
    await act(async () => { await vi.advanceTimersByTimeAsync(0); });
    expect(result.current.bridgeRunning).toBe(true);
  });

  it('returns bridgeRunning=false when health check fails', async () => {
    (global.fetch as ReturnType<typeof vi.fn>).mockRejectedValue(new Error('network'));
    const { result } = renderHook(() => useBridgeHealth('1337'));
    await act(async () => { await vi.advanceTimersByTimeAsync(0); });
    expect(result.current.bridgeRunning).toBe(false);
  });

  it('returns bridgeRunning=false when response is not ok', async () => {
    (global.fetch as ReturnType<typeof vi.fn>).mockResolvedValue({ ok: false });
    const { result } = renderHook(() => useBridgeHealth('1337'));
    await act(async () => { await vi.advanceTimersByTimeAsync(0); });
    expect(result.current.bridgeRunning).toBe(false);
  });

  it('polls on 15s interval', async () => {
    (global.fetch as ReturnType<typeof vi.fn>).mockResolvedValue({ ok: true });
    renderHook(() => useBridgeHealth('1337'));
    await act(async () => { await vi.advanceTimersByTimeAsync(0); });
    expect(global.fetch).toHaveBeenCalledTimes(1);
    await act(async () => { await vi.advanceTimersByTimeAsync(15_000); });
    expect(global.fetch).toHaveBeenCalledTimes(2);
  });

  it('cleans up interval on unmount', async () => {
    (global.fetch as ReturnType<typeof vi.fn>).mockResolvedValue({ ok: true });
    const { unmount } = renderHook(() => useBridgeHealth('1337'));
    await act(async () => { await vi.advanceTimersByTimeAsync(0); });
    unmount();
    await act(async () => { await vi.advanceTimersByTimeAsync(60_000); });
    // After unmount + 60s, fetch should have been called only once (initial) or twice (initial + one interval before unmount)
    expect((global.fetch as ReturnType<typeof vi.fn>).mock.calls.length).toBeLessThanOrEqual(2);
  });

  it('uses the provided bridgePort in the health URL', async () => {
    (global.fetch as ReturnType<typeof vi.fn>).mockResolvedValue({ ok: true });
    renderHook(() => useBridgeHealth('9000'));
    await act(async () => { await vi.advanceTimersByTimeAsync(0); });
    expect(global.fetch).toHaveBeenCalledWith('http://localhost:9000/health');
  });
});
