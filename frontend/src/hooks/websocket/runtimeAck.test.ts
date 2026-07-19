/**
 * Exercises bounded runtime ACK scheduling independently from the WebSocket hook lifecycle.
 */
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

import { createRuntimeAckScheduler, type RuntimeAckControl } from './runtimeAck';

describe('createRuntimeAckScheduler', () => {
  beforeEach(() => {
    vi.useFakeTimers();
  });

  afterEach(() => {
    vi.clearAllTimers();
    vi.useRealTimers();
  });

  it('coalesces the newest same-stream cursor behind one 250 ms timer', () => {
    const send = vi.fn();
    const scheduler = createRuntimeAckScheduler({ send });

    scheduler.schedule({ streamId: 'runtime-stream-1', version: 1 });
    scheduler.schedule({ streamId: 'runtime-stream-1', version: 2 });
    scheduler.schedule({ streamId: 'runtime-stream-1', version: 3 });

    expect(vi.getTimerCount()).toBe(1);
    expect(send).not.toHaveBeenCalled();

    vi.advanceTimersByTime(249);
    expect(send).not.toHaveBeenCalled();

    vi.advanceTimersByTime(1);
    expect(send).toHaveBeenCalledTimes(1);
    expect(send).toHaveBeenCalledWith({
      type: 'runtime_ack',
      payload: {
        runtime_stream_id: 'runtime-stream-1',
        runtime_version: 3,
      },
    });
    expect(vi.getTimerCount()).toBe(0);
  });

  it('stays disabled without a stream and ignores stale or different-stream cursors', () => {
    const send = vi.fn();
    const scheduler = createRuntimeAckScheduler({ send });

    scheduler.schedule(null);
    scheduler.schedule({ streamId: '', version: 1 });
    expect(vi.getTimerCount()).toBe(0);

    scheduler.schedule({ streamId: 'runtime-stream-1', version: 3 });
    scheduler.schedule({ streamId: 'runtime-stream-2', version: 10 });
    vi.advanceTimersByTime(250);

    scheduler.schedule({ streamId: 'runtime-stream-1', version: 3 });
    scheduler.schedule({ streamId: 'runtime-stream-1', version: 2 });
    expect(vi.getTimerCount()).toBe(0);
    expect(send).toHaveBeenCalledTimes(1);
    expect(send.mock.calls[0][0].payload).toEqual({
      runtime_stream_id: 'runtime-stream-1',
      runtime_version: 3,
    });
  });

  it('flushes a recovery barrier immediately and leaves no late send behind', () => {
    const send = vi.fn();
    const scheduler = createRuntimeAckScheduler({ send });

    scheduler.schedule({ streamId: 'runtime-stream-1', version: 7 });
    expect(vi.getTimerCount()).toBe(1);

    scheduler.flush();

    expect(send).toHaveBeenCalledTimes(1);
    expect(send.mock.calls[0][0].payload.runtime_version).toBe(7);
    expect(vi.getTimerCount()).toBe(0);

    scheduler.flush();
    vi.advanceTimersByTime(250);
    expect(send).toHaveBeenCalledTimes(1);
  });

  it('cancels pending work idempotently and never sends after cleanup', () => {
    const send = vi.fn();
    const scheduler = createRuntimeAckScheduler({ send });

    scheduler.schedule({ streamId: 'runtime-stream-1', version: 7 });
    expect(vi.getTimerCount()).toBe(1);

    scheduler.cancel();
    scheduler.cancel();

    expect(vi.getTimerCount()).toBe(0);
    vi.advanceTimersByTime(250);
    expect(send).not.toHaveBeenCalled();
  });

  it('resets the monotonic cursor when a full snapshot rotates the server stream', () => {
    const send = vi.fn();
    const scheduler = createRuntimeAckScheduler({ send });

    scheduler.schedule({ streamId: 'runtime-stream-1', version: 7 });
    vi.advanceTimersByTime(250);
    scheduler.schedule({ streamId: 'runtime-stream-1', version: 8 });
    expect(vi.getTimerCount()).toBe(1);

    scheduler.reset();
    scheduler.reset();
    scheduler.schedule({ streamId: 'runtime-stream-2', version: 1 });

    expect(vi.getTimerCount()).toBe(1);
    vi.advanceTimersByTime(250);
    expect(send).toHaveBeenLastCalledWith({
      type: 'runtime_ack',
      payload: {
        runtime_stream_id: 'runtime-stream-2',
        runtime_version: 1,
      },
    });
    expect(send).toHaveBeenCalledTimes(2);
  });

  it('keeps a failed ACK cursor pending and retryable until send succeeds', () => {
    const delivered: RuntimeAckControl[] = [];
    let attempts = 0;
    const scheduler = createRuntimeAckScheduler({
      send(control) {
        attempts += 1;
        if (attempts === 1) {
          throw new Error('socket send failed');
        }
        delivered.push(control);
      },
    });
    const cursor = { streamId: 'runtime-stream-1', version: 7 };

    scheduler.schedule(cursor);
    expect(() => scheduler.flush()).toThrow('socket send failed');
    expect(delivered).toEqual([]);
    expect(vi.getTimerCount()).toBe(0);

    scheduler.schedule(cursor);
    expect(vi.getTimerCount()).toBe(1);
    expect(() => scheduler.flush()).not.toThrow();

    expect(attempts).toBe(2);
    expect(delivered).toEqual([
      {
        type: 'runtime_ack',
        payload: {
          runtime_stream_id: 'runtime-stream-1',
          runtime_version: 7,
        },
      },
    ]);
  });

  it('ignores a retained stale callback without consuming or disturbing the replacement timer', () => {
    type TimerHandle = ReturnType<typeof setTimeout>;
    const callbacks = new Map<TimerHandle, () => void>();
    const scheduledHandles: TimerHandle[] = [];
    let timerSequence = 0;
    const send = vi.fn();
    const scheduler = createRuntimeAckScheduler({
      send,
      setTimeoutFn(handler) {
        timerSequence += 1;
        const handle = timerSequence as TimerHandle;
        callbacks.set(handle, handler);
        scheduledHandles.push(handle);
        return handle;
      },
      clearTimeoutFn() {
        // Retain cleared callbacks to model a callback already queued by the host timer system.
      },
    });

    scheduler.schedule({ streamId: 'runtime-stream-1', version: 7 });
    scheduler.reset();
    scheduler.schedule({ streamId: 'runtime-stream-2', version: 1 });

    callbacks.get(scheduledHandles[0]!)!();
    expect(send).not.toHaveBeenCalled();

    scheduler.schedule({ streamId: 'runtime-stream-2', version: 2 });
    expect(scheduledHandles).toHaveLength(2);

    callbacks.get(scheduledHandles[1]!)!();
    expect(send).toHaveBeenCalledTimes(1);
    expect(send).toHaveBeenCalledWith({
      type: 'runtime_ack',
      payload: {
        runtime_stream_id: 'runtime-stream-2',
        runtime_version: 2,
      },
    });
  });

  it('does not retain a phantom active timer when an injected timeout fires synchronously', () => {
    const send = vi.fn();
    let timerSequence = 0;
    const scheduler = createRuntimeAckScheduler({
      send,
      setTimeoutFn(handler) {
        handler();
        timerSequence += 1;
        return timerSequence as ReturnType<typeof setTimeout>;
      },
      clearTimeoutFn() {},
    });

    scheduler.schedule({ streamId: 'runtime-stream-1', version: 1 });
    scheduler.schedule({ streamId: 'runtime-stream-1', version: 2 });

    expect(send).toHaveBeenCalledTimes(2);
    expect(send.mock.calls.map(([control]) => control.payload.runtime_version)).toEqual([1, 2]);
  });

  it('serializes a flush requested reentrantly while sending the current ACK', () => {
    const versions: number[] = [];
    let requestedNestedFlush = false;
    let scheduler: ReturnType<typeof createRuntimeAckScheduler>;
    scheduler = createRuntimeAckScheduler({
      send(control) {
        versions.push(control.payload.runtime_version);
        if (!requestedNestedFlush) {
          requestedNestedFlush = true;
          scheduler.flush();
        }
      },
    });

    scheduler.schedule({ streamId: 'runtime-stream-1', version: 1 });
    scheduler.flush();

    expect(versions).toEqual([1]);
    expect(vi.getTimerCount()).toBe(0);
  });

  it('preserves a newer ACK emitted by a reentrant schedule and flush transaction', () => {
    const versions: number[] = [];
    let scheduler: ReturnType<typeof createRuntimeAckScheduler>;
    scheduler = createRuntimeAckScheduler({
      send(control) {
        const version = control.payload.runtime_version;
        versions.push(version);
        if (version === 1) {
          scheduler.schedule({ streamId: 'runtime-stream-1', version: 2 });
          scheduler.flush();
        }
      },
    });

    scheduler.schedule({ streamId: 'runtime-stream-1', version: 1 });
    scheduler.flush();
    scheduler.schedule({ streamId: 'runtime-stream-1', version: 2 });
    scheduler.flush();

    expect(versions).toEqual([1, 2]);
    expect(vi.getTimerCount()).toBe(0);
  });
});
