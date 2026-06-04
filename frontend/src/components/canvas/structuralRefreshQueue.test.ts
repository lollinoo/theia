import { beforeEach, describe, expect, it, vi } from 'vitest';

import { createStructuralRefreshQueue } from './structuralRefreshQueue';
import type { StructuralRefreshCause } from './topologyRecovery';

describe('createStructuralRefreshQueue', () => {
  beforeEach(() => {
    vi.useFakeTimers();
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
