/**
 * Exercises diagnostics hook lifecycle behavior so refactors preserve the documented contract.
 */
import { describe, expect, it } from 'vitest';

import { buildRuntimeRecoveryDiagnosticMetadata, getRawWebSocketMessageType } from './diagnostics';
import type { RuntimeRecoveryState } from './runtimeRecovery';

describe('websocket diagnostics helpers', () => {
  it('extracts a raw message type only when the payload shape supports it', () => {
    expect(getRawWebSocketMessageType({ type: 'runtime_delta' })).toBe('runtime_delta');
    expect(getRawWebSocketMessageType({ type: 123 })).toBeNull();
    expect(getRawWebSocketMessageType(null)).toBeNull();
    expect(getRawWebSocketMessageType('runtime_delta')).toBeNull();
  });

  it('projects recovery state into stable diagnostic metadata without side effects', () => {
    expect(
      buildRuntimeRecoveryDiagnosticMetadata(
        {
          phase: 'stream',
          generation: 2,
          startedAt: 5_000,
          reason: 'base_version_mismatch',
          targetVersion: 12,
        },
        6_250,
      ),
    ).toEqual({
      phase: 'stream',
      generation: 2,
      durationMs: 1_250,
      reason: 'base_version_mismatch',
      targetVersion: 12,
    });
    expect(buildRuntimeRecoveryDiagnosticMetadata({ phase: 'idle', generation: 2 }, 6_250)).toEqual(
      { phase: 'idle', generation: 2 },
    );
    expect(
      buildRuntimeRecoveryDiagnosticMetadata(
        {
          phase: 'http-fallback',
          generation: 2,
          startedAt: 5_000,
          reason: 'base_version_mismatch',
        },
        6_250,
      ),
    ).toEqual({
      phase: 'http-fallback',
      generation: 2,
      durationMs: 1_250,
      reason: 'base_version_mismatch',
    });
    expect(
      buildRuntimeRecoveryDiagnosticMetadata(
        { phase: 'failed', generation: 2, reason: 'runtime overview unavailable' },
        6_250,
      ),
    ).toEqual({
      phase: 'failed',
      generation: 2,
      reason: 'runtime overview unavailable',
    });
  });

  it.each<Array<[RuntimeRecoveryState, number]>>([
    [
      {
        phase: 'stream',
        generation: 2,
        startedAt: 5_000,
        reason: 'base_version_mismatch',
        targetVersion: 12,
      },
      Number.NaN,
    ],
    [
      {
        phase: 'stream',
        generation: 2,
        startedAt: 5_000,
        reason: 'base_version_mismatch',
        targetVersion: 12,
      },
      Number.POSITIVE_INFINITY,
    ],
    [
      {
        phase: 'stream',
        generation: 2,
        startedAt: 5_000,
        reason: 'base_version_mismatch',
        targetVersion: 12,
      },
      Number.NEGATIVE_INFINITY,
    ],
    [
      {
        phase: 'stream',
        generation: 2,
        startedAt: 5_000,
        reason: 'base_version_mismatch',
        targetVersion: 12,
      },
      4_999,
    ],
    [
      {
        phase: 'stream',
        generation: 2,
        startedAt: Number.NaN,
        reason: 'base_version_mismatch',
        targetVersion: 12,
      },
      6_250,
    ],
    [
      {
        phase: 'http-fallback',
        generation: 2,
        startedAt: Number.POSITIVE_INFINITY,
        reason: 'base_version_mismatch',
      },
      6_250,
    ],
  ])('uses a finite zero duration for invalid or retrograde clocks', (state, now) => {
    const metadata = buildRuntimeRecoveryDiagnosticMetadata(state, now);
    const durationMs = 'durationMs' in metadata ? metadata.durationMs : Number.NaN;

    expect(durationMs).toBe(0);
    expect(Number.isFinite(durationMs)).toBe(true);
  });
});
