/**
 * Exercises pure runtime recovery transitions so stream resume behavior stays deterministic.
 */
import { describe, expect, it } from 'vitest';

import {
  advanceRuntimeRecoveryDeadline,
  applyRuntimeRecoveryReady,
  applyRuntimeRecoverySnapshot,
  beginRuntimeRecovery,
  classifyRuntimeReplay,
  createRuntimeRecoveryState,
  failRuntimeRecovery,
} from './runtimeRecovery';

describe('runtime recovery transitions', () => {
  it('enters stream recovery once and increments the generation only for a later recovery', () => {
    const idle = createRuntimeRecoveryState();
    const first = beginRuntimeRecovery(idle, {
      now: 1_000,
      reason: 'base_version_mismatch',
      targetVersion: 12,
    });
    const duplicate = beginRuntimeRecovery(first, {
      now: 1_250,
      reason: 'duplicate_gap',
      targetVersion: 12,
    });

    expect(first).toEqual({
      phase: 'stream',
      generation: 1,
      startedAt: 1_000,
      reason: 'base_version_mismatch',
      targetVersion: 12,
    });
    expect(duplicate).toBe(first);

    const snapshot = applyRuntimeRecoverySnapshot(first, {
      streamId: 'runtime-stream-1',
      version: 12,
    });
    const completed = applyRuntimeRecoveryReady(snapshot.state, snapshot.cursor, {
      streamId: 'runtime-stream-1',
      version: 12,
    });
    expect(completed.kind).toBe('complete');
    if (completed.kind !== 'complete') {
      throw new Error('expected recovery to complete');
    }

    expect(
      beginRuntimeRecovery(completed.state, {
        now: 2_000,
        reason: 'later_gap',
        targetVersion: 13,
      }),
    ).toEqual({
      phase: 'stream',
      generation: 2,
      startedAt: 2_000,
      reason: 'later_gap',
      targetVersion: 13,
    });
  });

  it('raises a server target without replacing the active recovery or its deadline origin', () => {
    const first = beginRuntimeRecovery(createRuntimeRecoveryState(), {
      now: 1_000,
      reason: 'client_gap',
      targetVersion: 12,
    });

    expect(
      beginRuntimeRecovery(first, {
        now: 4_000,
        reason: 'server_marker',
        targetVersion: 15,
      }),
    ).toEqual({
      phase: 'stream',
      generation: 1,
      startedAt: 1_000,
      reason: 'client_gap',
      targetVersion: 15,
    });
    expect(
      beginRuntimeRecovery(first, {
        now: 4_000,
        reason: 'server_marker',
        targetVersion: 11,
      }),
    ).toBe(first);
  });

  it('enters HTTP fallback once when the active stream deadline expires', () => {
    const stream = beginRuntimeRecovery(createRuntimeRecoveryState(), {
      now: 10_000,
      reason: 'runtime_gap',
      targetVersion: 8,
    });

    expect(advanceRuntimeRecoveryDeadline(stream, 14_999)).toBe(stream);

    const fallback = advanceRuntimeRecoveryDeadline(stream, 15_000);
    expect(fallback).toEqual({
      phase: 'http-fallback',
      generation: 1,
      startedAt: 10_000,
      reason: 'runtime_gap',
    });
    expect(advanceRuntimeRecoveryDeadline(fallback, 20_000)).toBe(fallback);
  });

  it('keeps an HTTP snapshot behind the ready barrier and can fail the same generation', () => {
    const stream = beginRuntimeRecovery(createRuntimeRecoveryState(), {
      now: 10_000,
      reason: 'runtime_gap',
      targetVersion: 8,
    });
    const fallback = advanceRuntimeRecoveryDeadline(stream, 15_000);
    const snapshot = applyRuntimeRecoverySnapshot(fallback, {
      streamId: 'runtime-stream-1',
      version: 20,
    });

    expect(snapshot.state).toBe(fallback);
    expect(snapshot.cursor).toEqual({ streamId: 'runtime-stream-1', version: 20 });

    const completed = applyRuntimeRecoveryReady(snapshot.state, snapshot.cursor, {
      streamId: 'runtime-stream-1',
      version: 20,
    });
    expect(completed).toEqual({
      kind: 'complete',
      state: { phase: 'idle', generation: 1 },
      cursor: { streamId: 'runtime-stream-1', version: 20 },
    });
    expect(failRuntimeRecovery(fallback, 'runtime overview unavailable')).toEqual({
      phase: 'failed',
      generation: 1,
      reason: 'runtime overview unavailable',
    });
  });

  it('does not fail an idle recovery state', () => {
    const idle = createRuntimeRecoveryState();

    expect(failRuntimeRecovery(idle, 'socket unavailable')).toBe(idle);
  });

  it('fails active stream and HTTP fallback recovery without changing generation', () => {
    const stream = beginRuntimeRecovery(createRuntimeRecoveryState(), {
      now: 1_000,
      reason: 'runtime_gap',
      targetVersion: 8,
    });
    const fallback = advanceRuntimeRecoveryDeadline(stream, 6_000);

    expect(failRuntimeRecovery(stream, 'stream unavailable')).toEqual({
      phase: 'failed',
      generation: 1,
      reason: 'stream unavailable',
    });
    expect(failRuntimeRecovery(fallback, 'runtime overview unavailable')).toEqual({
      phase: 'failed',
      generation: 1,
      reason: 'runtime overview unavailable',
    });
  });

  it('keeps failed recovery terminal until a fresh state is created', () => {
    const failed = {
      phase: 'failed' as const,
      generation: 3,
      reason: 'runtime overview unavailable',
    };

    expect(
      beginRuntimeRecovery(failed, {
        now: 10_000,
        reason: 'later_gap',
        targetVersion: 20,
      }),
    ).toBe(failed);

    expect(
      beginRuntimeRecovery(createRuntimeRecoveryState(), {
        now: 10_000,
        reason: 'fresh_gap',
        targetVersion: 20,
      }),
    ).toMatchObject({ phase: 'stream', generation: 1 });
  });

  it.each([
    Number.NaN,
    Number.POSITIVE_INFINITY,
    Number.NEGATIVE_INFINITY,
    -1,
  ])('does not begin recovery with invalid time %s', (now) => {
    const idle = createRuntimeRecoveryState();

    expect(
      beginRuntimeRecovery(idle, {
        now,
        reason: 'runtime_gap',
        targetVersion: 8,
      }),
    ).toBe(idle);
  });

  it.each([
    Number.NaN,
    Number.POSITIVE_INFINITY,
    Number.NEGATIVE_INFINITY,
    999,
    -1,
  ])('does not mutate active recovery with invalid or retrograde time %s', (now) => {
    const stream = beginRuntimeRecovery(createRuntimeRecoveryState(), {
      now: 1_000,
      reason: 'runtime_gap',
      targetVersion: 8,
    });

    expect(
      beginRuntimeRecovery(stream, {
        now,
        reason: 'server_marker',
        targetVersion: 12,
      }),
    ).toBe(stream);
  });

  it.each([
    Number.NaN,
    Number.POSITIVE_INFINITY,
    Number.NEGATIVE_INFINITY,
    999,
    -1,
  ])('does not advance the deadline with invalid or retrograde time %s', (now) => {
    const stream = beginRuntimeRecovery(createRuntimeRecoveryState(), {
      now: 1_000,
      reason: 'runtime_gap',
      targetVersion: 8,
    });

    expect(advanceRuntimeRecoveryDeadline(stream, now)).toBe(stream);
  });

  it.each([
    Number.NaN,
    Number.POSITIVE_INFINITY,
    Number.NEGATIVE_INFINITY,
    -1,
    1.5,
    Number.MAX_SAFE_INTEGER + 1,
  ])('normalizes invalid initial target version %s to no target', (targetVersion) => {
    expect(
      beginRuntimeRecovery(createRuntimeRecoveryState(), {
        now: 1_000,
        reason: 'runtime_gap',
        targetVersion,
      }),
    ).toEqual({
      phase: 'stream',
      generation: 1,
      startedAt: 1_000,
      reason: 'runtime_gap',
      targetVersion: null,
    });
  });

  it.each([
    Number.NaN,
    Number.POSITIVE_INFINITY,
    Number.NEGATIVE_INFINITY,
    -1,
    12.5,
    Number.MAX_SAFE_INTEGER + 1,
  ])('ignores invalid active target version %s', (targetVersion) => {
    const stream = beginRuntimeRecovery(createRuntimeRecoveryState(), {
      now: 1_000,
      reason: 'runtime_gap',
      targetVersion: 8,
    });

    expect(
      beginRuntimeRecovery(stream, {
        now: 2_000,
        reason: 'server_marker',
        targetVersion,
      }),
    ).toBe(stream);
  });
});

describe('classifyRuntimeReplay', () => {
  const cursor = { streamId: 'runtime-stream-1', version: 10 };

  it('applies a compacted replay that overlaps the locally applied cursor', () => {
    expect(
      classifyRuntimeReplay(cursor, {
        runtime_stream_id: 'runtime-stream-1',
        from_version: 7,
        version: 12,
      }),
    ).toEqual({
      kind: 'apply',
      cursor: { streamId: 'runtime-stream-1', version: 12 },
    });
  });

  it.each([
    [
      'wrong stream',
      { runtime_stream_id: 'runtime-stream-2', from_version: 7, version: 12 },
      'wrong_stream',
    ],
    [
      'a range starting after the cursor',
      { runtime_stream_id: 'runtime-stream-1', from_version: 11, version: 12 },
      'range_gap',
    ],
    [
      'a stale range',
      { runtime_stream_id: 'runtime-stream-1', from_version: 7, version: 9 },
      'stale_range',
    ],
    [
      'an inverted range',
      { runtime_stream_id: 'runtime-stream-1', from_version: 13, version: 12 },
      'invalid_range',
    ],
    [
      'a fractional range',
      { runtime_stream_id: 'runtime-stream-1', from_version: 7.5, version: 12 },
      'invalid_range',
    ],
    [
      'a blank replay stream',
      { runtime_stream_id: '   ', from_version: 7, version: 12 },
      'invalid_stream',
    ],
  ])('rejects %s exactly', (_label, replay, reason) => {
    expect(classifyRuntimeReplay(cursor, replay)).toEqual({ kind: 'reject', reason });
  });

  it.each([
    [{ streamId: '', version: 10 }],
    [{ streamId: 'runtime-stream-1', version: 10.5 }],
  ])('rejects an invalid locally applied cursor before classifying a replay', (invalidCursor) => {
    expect(
      classifyRuntimeReplay(invalidCursor, {
        runtime_stream_id: 'runtime-stream-1',
        from_version: 7,
        version: 12,
      }),
    ).toEqual({ kind: 'reject', reason: 'invalid_cursor' });
  });
});

describe('runtime ready barrier', () => {
  it('updates the cursor from a snapshot without completing recovery', () => {
    const state = beginRuntimeRecovery(createRuntimeRecoveryState(), {
      now: 1_000,
      reason: 'server_marker',
      targetVersion: 12,
    });

    expect(
      applyRuntimeRecoverySnapshot(state, {
        streamId: 'runtime-stream-1',
        version: 12,
      }),
    ).toEqual({
      state,
      cursor: { streamId: 'runtime-stream-1', version: 12 },
    });
  });

  it.each([
    ['version', { streamId: 'runtime-stream-1', version: 13 }, 'version_mismatch'],
    ['stream', { streamId: 'runtime-stream-2', version: 12 }, 'wrong_stream'],
  ])('rejects a mismatched ready %s without advancing the applied cursor', (_label, ready, reason) => {
    const state = beginRuntimeRecovery(createRuntimeRecoveryState(), {
      now: 1_000,
      reason: 'server_marker',
      targetVersion: 12,
    });
    const cursor = { streamId: 'runtime-stream-1', version: 12 };

    expect(applyRuntimeRecoveryReady(state, cursor, ready)).toEqual({
      kind: 'reject',
      reason,
      state,
      cursor,
    });
  });

  it('rejects matching ready after recovery has failed', () => {
    const state = {
      phase: 'failed' as const,
      generation: 1,
      reason: 'runtime overview unavailable',
    };
    const cursor = { streamId: 'runtime-stream-1', version: 12 };

    expect(applyRuntimeRecoveryReady(state, cursor, cursor)).toEqual({
      kind: 'reject',
      reason: 'recovery_failed',
      state,
      cursor,
    });
  });

  it('accepts matching ready while idle for the normal ready path', () => {
    const state = { phase: 'idle' as const, generation: 3 };
    const cursor = { streamId: 'runtime-stream-1', version: 12 };

    expect(applyRuntimeRecoveryReady(state, cursor, cursor)).toEqual({
      kind: 'complete',
      state,
      cursor,
    });
  });

  it.each([
    [
      'invalid local cursor',
      { streamId: '', version: 12 },
      { streamId: 'runtime-stream-1', version: 12 },
      'invalid_cursor',
    ],
    [
      'blank ready stream',
      { streamId: 'runtime-stream-1', version: 12 },
      { streamId: '   ', version: 12 },
      'invalid_ready',
    ],
    [
      'fractional ready version',
      { streamId: 'runtime-stream-1', version: 12 },
      { streamId: 'runtime-stream-1', version: 12.5 },
      'invalid_ready',
    ],
  ])('rejects %s before evaluating the ready barrier', (_label, cursor, ready, reason) => {
    const state = beginRuntimeRecovery(createRuntimeRecoveryState(), {
      now: 1_000,
      reason: 'server_marker',
      targetVersion: 12,
    });

    expect(applyRuntimeRecoveryReady(state, cursor, ready)).toEqual({
      kind: 'reject',
      reason,
      state,
      cursor,
    });
  });
});
