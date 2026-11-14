/**
 * Exercises asynchronous polling lifecycle behavior.
 */

import { act, renderHook } from '@testing-library/react';
import React, { type ReactNode, StrictMode, useEffect } from 'react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { useAsyncPolling } from './useAsyncPolling';

function deferred<T>() {
  let resolve!: (value: T) => void;
  const promise = new Promise<T>((resolvePromise) => {
    resolve = resolvePromise;
  });
  return { promise, resolve };
}

beforeEach(() => {
  vi.useFakeTimers();
});

afterEach(() => {
  vi.clearAllTimers();
  vi.useRealTimers();
});

describe('useAsyncPolling', () => {
  it('schedules a new poll immediately after stop and restart without waiting for an old request', async () => {
    const oldRequest = deferred<string>();
    const poll = vi
      .fn()
      .mockImplementationOnce(() => oldRequest.promise)
      .mockResolvedValue('new');
    const onResult = vi.fn();
    const { result } = renderHook(() => useAsyncPolling({ intervalMs: 1000, poll, onResult }));

    act(() => result.current.start());
    await act(async () => {
      await vi.advanceTimersByTimeAsync(1000);
    });
    expect(poll).toHaveBeenCalledTimes(1);

    // Stop and restart for a new version while old request is still in flight
    act(() => {
      result.current.stop();
      result.current.start();
    });

    // It should immediately schedule the new version instead of being blocked behind oldRequest
    await act(async () => {
      await vi.advanceTimersByTimeAsync(1000);
    });
    expect(poll).toHaveBeenCalledTimes(2);

    // When the old request finally resolves, its result is discarded
    await act(async () => {
      oldRequest.resolve('old');
      await oldRequest.promise;
    });
    expect(onResult).not.toHaveBeenCalledWith('old');
    expect(onResult).toHaveBeenCalledWith('new');
  });

  it('pauses polling when document is hidden and resumes when visible', async () => {
    const poll = vi.fn().mockResolvedValue('result');
    const onResult = vi.fn();
    const { result } = renderHook(() => useAsyncPolling({ intervalMs: 1000, poll, onResult }));

    let visibilityState = 'visible';
    Object.defineProperty(document, 'visibilityState', {
      configurable: true,
      get: () => visibilityState,
    });

    act(() => result.current.start());

    // Switch to hidden before timer fires
    visibilityState = 'hidden';
    document.dispatchEvent(new Event('visibilitychange'));

    await act(async () => {
      await vi.advanceTimersByTimeAsync(5000);
    });
    expect(poll).not.toHaveBeenCalled();

    // Switch back to visible
    visibilityState = 'visible';
    document.dispatchEvent(new Event('visibilitychange'));

    await act(async () => {
      await vi.advanceTimersByTimeAsync(1000);
    });
    expect(poll).toHaveBeenCalledTimes(1);
    expect(onResult).toHaveBeenCalledWith('result');
  });

  it('returns stable memoized controls across renders', () => {
    const poll = vi.fn().mockResolvedValue('test');
    const onResult = vi.fn();
    const { result, rerender } = renderHook(() =>
      useAsyncPolling({ intervalMs: 1000, poll, onResult }),
    );

    const initialControls = result.current;
    rerender();
    expect(result.current).toBe(initialControls);
    expect(result.current.start).toBe(initialControls.start);
    expect(result.current.stop).toBe(initialControls.stop);
  });

  it('clears a scheduled poll when unmounted', async () => {
    const poll = vi.fn().mockResolvedValue('unused');
    const onResult = vi.fn();
    const { result, unmount } = renderHook(() =>
      useAsyncPolling({ intervalMs: 1000, poll, onResult }),
    );

    act(() => result.current.start());
    unmount();
    await act(async () => {
      await vi.advanceTimersByTimeAsync(10_000);
    });

    expect(poll).not.toHaveBeenCalled();
    expect(onResult).not.toHaveBeenCalled();
  });

  it('retries after a rejected poll', async () => {
    const poll = vi
      .fn()
      .mockRejectedValueOnce(new Error('temporary'))
      .mockResolvedValue('recovered');
    const onResult = vi.fn();
    const onError = vi.fn();
    const { result } = renderHook(() =>
      useAsyncPolling({ intervalMs: 1000, poll, onResult, onError }),
    );

    act(() => result.current.start());
    await act(async () => {
      await vi.advanceTimersByTimeAsync(1000);
    });
    expect(onError).toHaveBeenCalledTimes(1);

    await act(async () => {
      await vi.advanceTimersByTimeAsync(1000);
    });
    expect(poll).toHaveBeenCalledTimes(2);
    expect(onResult).toHaveBeenCalledWith('recovered');
  });

  it('keeps one polling loop under StrictMode effect replay', async () => {
    const poll = vi.fn().mockResolvedValue('result');
    const onResult = vi.fn();
    const wrapper = ({ children }: { children: ReactNode }) =>
      React.createElement(StrictMode, null, children);

    renderHook(
      () => {
        const polling = useAsyncPolling({ intervalMs: 1000, poll, onResult });
        useEffect(() => {
          polling.start();
          return polling.stop;
        }, [polling.start, polling.stop]);
      },
      { wrapper },
    );

    await act(async () => {
      await vi.advanceTimersByTimeAsync(1000);
    });

    expect(poll).toHaveBeenCalledTimes(1);
    expect(onResult).toHaveBeenCalledTimes(1);
  });
});
