import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { renderHook, act } from '@testing-library/react';
import { useBridgeHealth } from './useBridgeHealth';
import {
  BRIDGE_HEALTH_TIMEOUT_MESSAGE,
  BRIDGE_HEALTH_UNREACHABLE_MESSAGE,
  BRIDGE_REQUEST_TIMEOUT_MS,
} from '../utils/bridgeRequests';

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
    expect(result.current.bridgeChecked).toBe(false);
    expect(result.current.bridgeError).toBeNull();
    act(() => {
      result.current.checkBridgeHealth();
    });
    await act(async () => { await vi.advanceTimersByTimeAsync(0); });
    expect(result.current.bridgeRunning).toBe(true);
    expect(result.current.bridgeChecked).toBe(true);
    expect(result.current.bridgeError).toBeNull();
  });

  it('returns bridgeRunning=false when health check fails', async () => {
    (global.fetch as ReturnType<typeof vi.fn>).mockRejectedValue(new Error('network'));
    const { result } = renderHook(() => useBridgeHealth('1337'));
    act(() => {
      result.current.checkBridgeHealth();
    });
    await act(async () => { await vi.advanceTimersByTimeAsync(0); });
    expect(result.current.bridgeRunning).toBe(false);
    expect(result.current.bridgeChecked).toBe(true);
    expect(result.current.bridgeError).toBe(BRIDGE_HEALTH_UNREACHABLE_MESSAGE);
  });

  it('returns bridgeRunning=false when response is not ok', async () => {
    (global.fetch as ReturnType<typeof vi.fn>).mockResolvedValue({ ok: false, status: 503 });
    const { result } = renderHook(() => useBridgeHealth('1337'));
    act(() => {
      result.current.checkBridgeHealth();
    });
    await act(async () => { await vi.advanceTimersByTimeAsync(0); });
    expect(result.current.bridgeRunning).toBe(false);
    expect(result.current.bridgeChecked).toBe(true);
    expect(result.current.bridgeError).toBe('WinBox bridge health check failed (503).');
  });

  it('times out a hanging health check and reports a bridge error', async () => {
    (global.fetch as ReturnType<typeof vi.fn>).mockImplementation(() => new Promise(() => {}));
    const { result } = renderHook(() => useBridgeHealth('1337'));
    act(() => {
      result.current.checkBridgeHealth();
    });
    await act(async () => { await vi.advanceTimersByTimeAsync(BRIDGE_REQUEST_TIMEOUT_MS); });
    expect(result.current.bridgeRunning).toBe(false);
    expect(result.current.bridgeChecked).toBe(true);
    expect(result.current.bridgeError).toBe(BRIDGE_HEALTH_TIMEOUT_MESSAGE);
  });

  it('does not fetch until the context menu triggers the check', async () => {
    (global.fetch as ReturnType<typeof vi.fn>).mockResolvedValue({ ok: true });
    renderHook(() => useBridgeHealth('1337'));
    await act(async () => { await vi.advanceTimersByTimeAsync(60_000); });
    expect(global.fetch).not.toHaveBeenCalled();
  });

  it('does not start polling after a manual trigger', async () => {
    (global.fetch as ReturnType<typeof vi.fn>).mockResolvedValue({ ok: true });
    const { result } = renderHook(() => useBridgeHealth('1337'));
    act(() => {
      result.current.checkBridgeHealth();
    });
    await act(async () => { await vi.advanceTimersByTimeAsync(0); });
    await act(async () => { await vi.advanceTimersByTimeAsync(60_000); });
    expect(global.fetch).toHaveBeenCalledTimes(1);
  });

  it('fetches again each time the trigger is called', async () => {
    (global.fetch as ReturnType<typeof vi.fn>).mockResolvedValue({ ok: true });
    const { result } = renderHook(() => useBridgeHealth('1337'));
    act(() => {
      result.current.checkBridgeHealth();
    });
    await act(async () => { await vi.advanceTimersByTimeAsync(0); });
    act(() => {
      result.current.checkBridgeHealth();
    });
    await act(async () => { await vi.advanceTimersByTimeAsync(0); });
    expect(global.fetch).toHaveBeenCalledTimes(2);
  });

  it('uses the provided bridgePort in the health URL', async () => {
    (global.fetch as ReturnType<typeof vi.fn>).mockResolvedValue({ ok: true });
    const { result } = renderHook(() => useBridgeHealth('9000'));
    act(() => {
      result.current.checkBridgeHealth();
    });
    await act(async () => { await vi.advanceTimersByTimeAsync(0); });
    expect(global.fetch).toHaveBeenCalledWith(
      'http://localhost:9000/health',
      expect.objectContaining({ signal: expect.any(AbortSignal) }),
    );
  });

  it('keeps bridgeRunning=false before the first trigger', async () => {
    (global.fetch as ReturnType<typeof vi.fn>).mockResolvedValue({ ok: true });
    const { result } = renderHook(() => useBridgeHealth('1337'));
    await act(async () => { await vi.advanceTimersByTimeAsync(0); });
    expect(global.fetch).not.toHaveBeenCalled();
    expect(result.current.bridgeRunning).toBe(false);
    expect(result.current.bridgeChecked).toBe(false);
    expect(result.current.bridgeError).toBeNull();
  });

  it('uses the latest bridgePort after rerender', async () => {
    (global.fetch as ReturnType<typeof vi.fn>).mockResolvedValue({ ok: true });
    const { result, rerender } = renderHook(
      ({ bridgePort }) => useBridgeHealth(bridgePort),
      { initialProps: { bridgePort: '1337' } },
    );
    act(() => {
      result.current.checkBridgeHealth();
    });
    await act(async () => { await vi.advanceTimersByTimeAsync(0); });
    rerender({ bridgePort: '9000' });
    act(() => {
      result.current.checkBridgeHealth();
    });
    await act(async () => { await vi.advanceTimersByTimeAsync(0); });
    expect(global.fetch).toHaveBeenNthCalledWith(
      1,
      'http://localhost:1337/health',
      expect.objectContaining({ signal: expect.any(AbortSignal) }),
    );
    expect(global.fetch).toHaveBeenNthCalledWith(
      2,
      'http://localhost:9000/health',
      expect.objectContaining({ signal: expect.any(AbortSignal) }),
    );
  });
});
