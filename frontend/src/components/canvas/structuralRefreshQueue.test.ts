/**
 * Exercises structural refresh queue topology canvas behavior so refactors preserve the documented contract.
 */
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

import { createStructuralRefreshQueue } from './structuralRefreshQueue';
import type { StructuralRefreshCause } from './topologyRecovery';

function deferred<T>() {
  let resolve!: (value: T | PromiseLike<T>) => void;
  let reject!: (reason?: unknown) => void;
  const promise = new Promise<T>((promiseResolve, promiseReject) => {
    resolve = promiseResolve;
    reject = promiseReject;
  });
  return { promise, resolve, reject };
}

describe('createStructuralRefreshQueue', () => {
  beforeEach(() => {
    vi.useFakeTimers();
  });

  afterEach(() => {
    vi.useRealTimers();
  });

  it('coalesces clustered refresh causes into one refresh run', () => {
    const runRefresh = vi.fn();
    const queue = createStructuralRefreshQueue({
      debounceMs: 250,
      runRefresh,
      setTimeoutFn: window.setTimeout.bind(window),
      clearTimeoutFn: window.clearTimeout.bind(window),
    });

    queue.queue('backend-reconnected');
    queue.queue('topology-changed');
    queue.queue('backend-resync-required');

    expect(runRefresh).not.toHaveBeenCalled();

    vi.advanceTimersByTime(250);

    expect(runRefresh).toHaveBeenCalledTimes(1);
    expect(runRefresh).toHaveBeenCalledWith(
      new Set<StructuralRefreshCause>([
        'backend-reconnected',
        'topology-changed',
        'backend-resync-required',
      ]),
    );
  });

  it('opens a new debounce window after a refresh fires', () => {
    const runRefresh = vi.fn();
    const queue = createStructuralRefreshQueue({
      debounceMs: 250,
      runRefresh,
      setTimeoutFn: window.setTimeout.bind(window),
      clearTimeoutFn: window.clearTimeout.bind(window),
    });

    queue.queue('topology-changed');
    vi.advanceTimersByTime(250);
    queue.queue('backend-reconnected');
    vi.advanceTimersByTime(250);

    expect(runRefresh).toHaveBeenCalledTimes(2);
    expect(runRefresh.mock.calls[1][0]).toEqual(
      new Set<StructuralRefreshCause>(['backend-reconnected']),
    );
  });

  it('merges causes into one non-overlapping follow-up after an async refresh resolves', async () => {
    const firstRun = deferred<void>();
    let activeRuns = 0;
    let maximumActiveRuns = 0;
    const firstRunCompletion = firstRun.promise.finally(() => {
      activeRuns -= 1;
    });
    const runRefresh = vi.fn((_causes: Set<StructuralRefreshCause>) => {
      activeRuns += 1;
      maximumActiveRuns = Math.max(maximumActiveRuns, activeRuns);
      if (runRefresh.mock.calls.length === 1) {
        return firstRunCompletion;
      }
      activeRuns -= 1;
      return Promise.resolve();
    });
    const queue = createStructuralRefreshQueue({
      debounceMs: 250,
      runRefresh,
      setTimeoutFn: window.setTimeout.bind(window),
      clearTimeoutFn: window.clearTimeout.bind(window),
    });

    queue.queue('backend-reconnected');
    vi.advanceTimersByTime(250);
    queue.queue('topology-changed');
    queue.queue('backend-resync-required');
    vi.advanceTimersByTime(250);

    expect(runRefresh).toHaveBeenCalledTimes(1);

    firstRun.resolve();
    await firstRunCompletion;

    expect(runRefresh).toHaveBeenCalledTimes(2);
    expect(runRefresh.mock.calls[1][0]).toEqual(
      new Set<StructuralRefreshCause>(['topology-changed', 'backend-resync-required']),
    );
    expect(maximumActiveRuns).toBe(1);
  });

  it('starts the merged follow-up after an async refresh rejects', async () => {
    const firstRun = deferred<void>();
    const runRefresh = vi
      .fn<(causes: Set<StructuralRefreshCause>) => void | Promise<void>>()
      .mockReturnValueOnce(firstRun.promise)
      .mockResolvedValueOnce();
    const queue = createStructuralRefreshQueue({
      debounceMs: 250,
      runRefresh,
      setTimeoutFn: window.setTimeout.bind(window),
      clearTimeoutFn: window.clearTimeout.bind(window),
    });

    queue.queue('backend-reconnected');
    vi.advanceTimersByTime(250);
    queue.queue('topology-changed');

    firstRun.reject(new Error('refresh failed'));
    await firstRun.promise.catch(() => undefined);
    await Promise.resolve();
    await Promise.resolve();

    expect(runRefresh).toHaveBeenCalledTimes(2);
    expect(runRefresh.mock.calls[1][0]).toEqual(
      new Set<StructuralRefreshCause>(['topology-changed']),
    );
  });

  it('contains synchronous refresh failures and immediately runs merged follow-up work', async () => {
    let queue!: ReturnType<typeof createStructuralRefreshQueue>;
    const runRefresh = vi.fn<(causes: Set<StructuralRefreshCause>) => void>(() => {
      if (runRefresh.mock.calls.length === 1) {
        queue.queue('topology-changed');
        throw new Error('synchronous refresh failure');
      }
    });
    queue = createStructuralRefreshQueue({
      debounceMs: 250,
      runRefresh,
      setTimeoutFn: window.setTimeout.bind(window),
      clearTimeoutFn: window.clearTimeout.bind(window),
    });

    queue.queue('backend-reconnected');

    expect(() => vi.advanceTimersByTime(250)).not.toThrow();
    await Promise.resolve();

    expect(runRefresh).toHaveBeenCalledTimes(2);
    expect(runRefresh.mock.calls[1][0]).toEqual(
      new Set<StructuralRefreshCause>(['topology-changed']),
    );
  });

  it('cancels pending follow-up causes without interrupting the active refresh', async () => {
    const firstRun = deferred<void>();
    const runRefresh = vi.fn(() => firstRun.promise);
    const queue = createStructuralRefreshQueue({
      debounceMs: 250,
      runRefresh,
      setTimeoutFn: window.setTimeout.bind(window),
      clearTimeoutFn: window.clearTimeout.bind(window),
    });

    queue.queue('backend-reconnected');
    vi.advanceTimersByTime(250);
    queue.queue('topology-changed');
    queue.cancel();

    expect(runRefresh).toHaveBeenCalledTimes(1);

    firstRun.resolve();
    await firstRun.promise;
    await Promise.resolve();
    vi.advanceTimersByTime(250);

    expect(runRefresh).toHaveBeenCalledTimes(1);
  });

  it('cancels pending refresh work and clears queued causes', () => {
    const runRefresh = vi.fn();
    const queue = createStructuralRefreshQueue({
      debounceMs: 250,
      runRefresh,
      setTimeoutFn: window.setTimeout.bind(window),
      clearTimeoutFn: window.clearTimeout.bind(window),
    });

    queue.queue('topology-changed');
    queue.cancel();
    vi.advanceTimersByTime(250);

    expect(runRefresh).not.toHaveBeenCalled();
  });
});
