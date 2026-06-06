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
