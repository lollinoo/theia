/**
 * Exercises runtime state hook lifecycle behavior so refactors preserve the documented contract.
 */
import { describe, expect, it } from 'vitest';
import { classifyRuntimeDelta, shouldIgnoreStaleRuntimeSnapshot } from './runtimeState';

describe('shouldIgnoreStaleRuntimeSnapshot', () => {
  it('ignores older snapshots only when a versioned runtime base exists', () => {
    expect(shouldIgnoreStaleRuntimeSnapshot(4, 5, true)).toBe(true);
    expect(shouldIgnoreStaleRuntimeSnapshot(5, 5, true)).toBe(false);
    expect(shouldIgnoreStaleRuntimeSnapshot(4, 5, false)).toBe(false);
    expect(shouldIgnoreStaleRuntimeSnapshot(null, 5, true)).toBe(false);
    expect(shouldIgnoreStaleRuntimeSnapshot(4, null, true)).toBe(false);
  });
});

describe('classifyRuntimeDelta', () => {
  it('applies matching versioned deltas and exposes the next runtime identity', () => {
    expect(
      classifyRuntimeDelta(
        'runtime_delta',
        { base_version: 7, version: 8, runtime_identity: 'rt-sha256:next' },
        { currentVersion: 7, hasRuntimeSnapshot: true },
      ),
    ).toEqual({
      kind: 'apply_versioned',
      nextVersion: 8,
      runtimeIdentity: 'rt-sha256:next',
    });
  });

  it('validates a protocol-v2 stream before considering the base version', () => {
    expect(
      classifyRuntimeDelta(
        'runtime_delta',
        {
          base_version: 9,
          version: 10,
          runtime_stream_id: 'runtime-stream-2',
        },
        {
          currentVersion: 7,
          currentStreamId: 'runtime-stream-1',
          hasRuntimeSnapshot: true,
        },
      ),
    ).toEqual({
      kind: 'request_resync',
      payloadReason: 'client_resync_scheduled',
      diagnosticReason: 'runtime_stream_mismatch',
    });

    expect(
      classifyRuntimeDelta(
        'runtime_delta',
        {
          base_version: 7,
          version: 8,
          runtime_stream_id: 'runtime-stream-1',
        },
        {
          currentVersion: 7,
          currentStreamId: 'runtime-stream-1',
          hasRuntimeSnapshot: true,
        },
      ),
    ).toEqual({
      kind: 'apply_versioned',
      nextVersion: 8,
      runtimeIdentity: undefined,
    });
  });

  it('rejects a missing delta stream only after a v2 stream has been established', () => {
    expect(
      classifyRuntimeDelta(
        'runtime_delta',
        { base_version: 7, version: 8 },
        {
          currentVersion: 7,
          currentStreamId: 'runtime-stream-1',
          hasRuntimeSnapshot: true,
        },
      ),
    ).toEqual({
      kind: 'request_resync',
      payloadReason: 'client_resync_scheduled',
      diagnosticReason: 'runtime_stream_mismatch',
    });

    expect(
      classifyRuntimeDelta(
        'runtime_delta',
        { base_version: 7, version: 8 },
        { currentVersion: 7, currentStreamId: null, hasRuntimeSnapshot: true },
      ),
    ).toEqual({
      kind: 'apply_versioned',
      nextVersion: 8,
      runtimeIdentity: undefined,
    });
  });

  it('prioritizes an unestablished incoming v2 stream over an incomplete version envelope', () => {
    expect(
      classifyRuntimeDelta(
        'runtime_delta',
        { runtime_stream_id: 'runtime-stream-1' },
        { currentVersion: null, currentStreamId: null, hasRuntimeSnapshot: true },
      ),
    ).toEqual({
      kind: 'request_resync',
      payloadReason: 'client_resync_scheduled',
      diagnosticReason: 'runtime_stream_mismatch',
    });
  });

  it('requests recovery when an incoming v2 delta has no local stream lineage to match', () => {
    expect(
      classifyRuntimeDelta(
        'runtime_delta',
        {
          base_version: 7,
          version: 8,
          runtime_stream_id: 'runtime-stream-1',
        },
        { currentVersion: 7, currentStreamId: null, hasRuntimeSnapshot: true },
      ),
    ).toEqual({
      kind: 'request_resync',
      payloadReason: 'client_resync_scheduled',
      diagnosticReason: 'runtime_stream_mismatch',
    });
  });

  for (const lineage of [
    {
      label: 'unestablished',
      incomingStreamId: 'runtime-stream-1',
      currentStreamId: null,
    },
    {
      label: 'mismatched',
      incomingStreamId: 'runtime-stream-2',
      currentStreamId: 'runtime-stream-1',
    },
  ]) {
    it.each([
      ['missing base version', { version: 8 }],
      ['missing next version', { base_version: 7 }],
      ['fractional version', { base_version: 7, version: 7.5 }],
      ['unsafe version', { base_version: 7, version: Number.MAX_SAFE_INTEGER + 1 }],
      ['equal version', { base_version: 7, version: 7 }],
      ['backward version', { base_version: 7, version: 6 }],
    ])(`prioritizes ${lineage.label} v2 lineage over %s`, (_label, versionEnvelope) => {
      expect(
        classifyRuntimeDelta(
          'runtime_delta',
          {
            ...versionEnvelope,
            runtime_stream_id: lineage.incomingStreamId,
          },
          {
            currentVersion: 7,
            currentStreamId: lineage.currentStreamId,
            hasRuntimeSnapshot: true,
          },
        ),
      ).toEqual({
        kind: 'request_resync',
        payloadReason: 'client_resync_scheduled',
        diagnosticReason: 'runtime_stream_mismatch',
      });
    });
  }

  it('validates versions after an incoming v2 stream matches local lineage', () => {
    expect(
      classifyRuntimeDelta(
        'runtime_delta',
        {
          base_version: 7,
          version: 7.5,
          runtime_stream_id: 'runtime-stream-1',
        },
        {
          currentVersion: 7,
          currentStreamId: 'runtime-stream-1',
          hasRuntimeSnapshot: true,
        },
      ),
    ).toEqual({
      kind: 'request_resync',
      payloadReason: 'client_resync_scheduled',
      diagnosticReason: 'invalid_delta_version',
    });
  });

  it('keeps the legacy unversioned delta path separate from versioned resync decisions', () => {
    expect(
      classifyRuntimeDelta(
        'snapshot_delta',
        {},
        { currentVersion: null, hasRuntimeSnapshot: false },
      ),
    ).toEqual({ kind: 'reject_missing_unversioned_base' });
    expect(
      classifyRuntimeDelta(
        'snapshot_delta',
        {},
        { currentVersion: null, hasRuntimeSnapshot: true },
      ),
    ).toEqual({ kind: 'apply_unversioned' });
  });

  it('requests resync when the version envelope is incomplete', () => {
    expect(
      classifyRuntimeDelta(
        'runtime_delta',
        { version: 8 },
        { currentVersion: 7, hasRuntimeSnapshot: true },
      ),
    ).toEqual({
      kind: 'request_resync',
      payloadReason: 'client_resync_scheduled',
      diagnosticReason: 'invalid_delta_version',
    });
  });

  it.each([
    [{ base_version: 7, version: 7.5 }],
    [{ base_version: 7, version: Number.MAX_SAFE_INTEGER + 1 }],
    [{ base_version: -1, version: 8 }],
    [{ base_version: 7, version: 7 }],
    [{ base_version: 7, version: 6 }],
  ])('requests resync when a versioned delta cursor is invalid or does not advance', (payload) => {
    expect(
      classifyRuntimeDelta('runtime_delta', payload, {
        currentVersion: 7,
        hasRuntimeSnapshot: true,
      }),
    ).toEqual({
      kind: 'request_resync',
      payloadReason: 'client_resync_scheduled',
      diagnosticReason: 'invalid_delta_version',
    });
  });

  it.each([
    [
      { base_version: 7, version: 8, runtime_stream_id: 'runtime-stream-1' },
      { currentVersion: 7, currentStreamId: '   ', hasRuntimeSnapshot: true },
    ],
    [
      { base_version: 7, version: 8, runtime_stream_id: '   ' },
      { currentVersion: 7, currentStreamId: 'runtime-stream-1', hasRuntimeSnapshot: true },
    ],
    [
      { runtime_stream_id: '   ' },
      { currentVersion: null, currentStreamId: null, hasRuntimeSnapshot: true },
    ],
  ])('requests recovery instead of accepting a blank stream lineage', (payload, state) => {
    expect(classifyRuntimeDelta('runtime_delta', payload, state)).toEqual({
      kind: 'request_resync',
      payloadReason: 'client_resync_scheduled',
      diagnosticReason: 'runtime_stream_mismatch',
    });
  });

  it('requests resync when a versioned delta has no usable base snapshot', () => {
    expect(
      classifyRuntimeDelta(
        'runtime_delta',
        { base_version: 7, version: 8 },
        { currentVersion: null, hasRuntimeSnapshot: false },
      ),
    ).toEqual({
      kind: 'request_resync',
      payloadReason: 'client_missing_runtime_snapshot',
      diagnosticReason: 'missing_base_snapshot',
    });
  });

  it('ignores stale deltas and requests resync for future-base deltas', () => {
    expect(
      classifyRuntimeDelta(
        'snapshot_delta',
        { base_version: 6, version: 7 },
        { currentVersion: 8, hasRuntimeSnapshot: true },
      ),
    ).toEqual({
      kind: 'ignore_stale',
      messageType: 'snapshot_delta',
      baseVersion: 6,
      version: 7,
      currentVersion: 8,
    });
    expect(
      classifyRuntimeDelta(
        'snapshot_delta',
        { base_version: 9, version: 10 },
        { currentVersion: 8, hasRuntimeSnapshot: true },
      ),
    ).toEqual({
      kind: 'request_resync',
      payloadReason: 'client_resync_scheduled',
      diagnosticReason: 'base_version_mismatch',
    });
  });
});
