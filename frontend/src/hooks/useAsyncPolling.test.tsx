/**
 * Exercises asynchronous polling lifecycle behavior.
 */

import { act, renderHook } from '@testing-library/react';
import { type ReactNode, StrictMode, useEffect } from 'react';
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
  it('waits for an old request before polling again after stop and restart', async () => {
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
    act(() => {
      result.current.stop();
      result.current.start();
    });
    await act(async () => {
      await vi.advanceTimersByTimeAsync(10_000);
    });
    expect(poll).toHaveBeenCalledTimes(1);

    await act(async () => {
      oldRequest.resolve('old');
      await oldRequest.promise;
    });
    expect(onResult).not.toHaveBeenCalled();

    await act(async () => {
      await vi.advanceTimersByTimeAsync(999);
    });
    expect(poll).toHaveBeenCalledTimes(1);
    await act(async () => {
      await vi.advanceTimersByTimeAsync(1);
    });
    expect(poll).toHaveBeenCalledTimes(2);
    expect(onResult).toHaveBeenCalledWith('new');
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
    const wrapper = ({ children }: { children: ReactNode }) => <StrictMode>{children}</StrictMode>;

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
