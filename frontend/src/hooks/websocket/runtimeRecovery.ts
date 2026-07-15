/**
 * Defines deterministic runtime stream recovery decisions without React, timers, or network effects.
 */

/** Maximum time allowed for WebSocket recovery before the runtime-only HTTP fallback begins. */
export const RUNTIME_RECOVERY_DEADLINE_MS = 5_000;

/** Maximum time allowed for the runtime-only HTTP recovery request. */
export const RUNTIME_HTTP_FALLBACK_DEADLINE_MS = 10_000;

/** Identifies one locally applied position in a server runtime stream. */
export interface RuntimeCursor {
  streamId: string;
  version: number;
}

/** Tracks the single active runtime recovery generation. */
export type RuntimeRecoveryState =
  | { phase: 'idle'; generation: number }
  | {
      phase: 'stream';
      generation: number;
      startedAt: number;
      reason: string;
      targetVersion: number | null;
    }
  | {
      phase: 'http-fallback';
      generation: number;
      startedAt: number;
      reason: string;
    }
  | {
      phase: 'failed';
      generation: number;
      reason: string;
    };

/** Describes the cursor range carried by a compacted runtime replay. */
export interface RuntimeReplayRange {
  runtime_stream_id: string;
  from_version: number;
  version: number;
}

/** Classifies whether a runtime replay can advance the locally applied cursor. */
export type RuntimeReplayDecision =
  | { kind: 'apply'; cursor: RuntimeCursor }
  | {
      kind: 'reject';
      reason:
        | 'invalid_cursor'
        | 'invalid_stream'
        | 'wrong_stream'
        | 'range_gap'
        | 'stale_range'
        | 'invalid_range';
    };

/** Describes the result of enforcing the ready barrier against locally applied state. */
export type RuntimeReadyDecision =
  | {
      kind: 'complete';
      state: RuntimeRecoveryState;
      cursor: RuntimeCursor;
    }
  | {
      kind: 'reject';
      reason:
        | 'missing_cursor'
        | 'invalid_cursor'
        | 'invalid_ready'
        | 'recovery_failed'
        | 'wrong_stream'
        | 'version_mismatch';
      state: RuntimeRecoveryState;
      cursor: RuntimeCursor | null;
    };

interface BeginRuntimeRecoveryOptions {
  now: number;
  reason: string;
  targetVersion?: number | null;
}

/** Creates the initial idle runtime recovery state. */
export function createRuntimeRecoveryState(): RuntimeRecoveryState {
  return { phase: 'idle', generation: 0 };
}

/** Starts one recovery generation or raises the target of the active stream recovery. */
export function beginRuntimeRecovery(
  state: RuntimeRecoveryState,
  { now, reason, targetVersion = null }: BeginRuntimeRecoveryOptions,
): RuntimeRecoveryState {
  if (state.phase === 'failed' || state.phase === 'http-fallback') {
    return state;
  }
  if (
    !isRuntimeRecoveryTime(now) ||
    (state.phase === 'stream' && (!isRuntimeRecoveryTime(state.startedAt) || now < state.startedAt))
  ) {
    return state;
  }

  const normalizedTargetVersion =
    targetVersion !== null && isRuntimeVersion(targetVersion) ? targetVersion : null;
  if (state.phase === 'stream') {
    const nextTargetVersion =
      normalizedTargetVersion === null
        ? state.targetVersion
        : state.targetVersion === null
          ? normalizedTargetVersion
          : Math.max(state.targetVersion, normalizedTargetVersion);
    return nextTargetVersion === state.targetVersion
      ? state
      : { ...state, targetVersion: nextTargetVersion };
  }

  return {
    phase: 'stream',
    generation: state.generation + 1,
    startedAt: now,
    reason,
    targetVersion: normalizedTargetVersion,
  };
}

/** Moves the active stream recovery into HTTP fallback once, after its fixed deadline. */
export function advanceRuntimeRecoveryDeadline(
  state: RuntimeRecoveryState,
  now: number,
): RuntimeRecoveryState {
  if (
    state.phase !== 'stream' ||
    !isRuntimeRecoveryTime(state.startedAt) ||
    !isRuntimeRecoveryTime(now) ||
    now < state.startedAt ||
    now - state.startedAt < RUNTIME_RECOVERY_DEADLINE_MS
  ) {
    return state;
  }

  return {
    phase: 'http-fallback',
    generation: state.generation,
    startedAt: state.startedAt,
    reason: state.reason,
  };
}

/** Classifies a replay using exact stream and overlapping-range rules. */
export function classifyRuntimeReplay(
  cursor: RuntimeCursor,
  replay: RuntimeReplayRange,
): RuntimeReplayDecision {
  if (!isRuntimeCursor(cursor)) {
    return { kind: 'reject', reason: 'invalid_cursor' };
  }
  if (
    !isRuntimeVersion(replay.from_version) ||
    !isRuntimeVersion(replay.version) ||
    replay.from_version > replay.version
  ) {
    return { kind: 'reject', reason: 'invalid_range' };
  }
  if (replay.runtime_stream_id.trim().length === 0) {
    return { kind: 'reject', reason: 'invalid_stream' };
  }
  if (replay.runtime_stream_id !== cursor.streamId) {
    return { kind: 'reject', reason: 'wrong_stream' };
  }
  if (replay.version <= cursor.version) {
    return { kind: 'reject', reason: 'stale_range' };
  }
  if (replay.from_version > cursor.version) {
    return { kind: 'reject', reason: 'range_gap' };
  }

  return {
    kind: 'apply',
    cursor: { streamId: cursor.streamId, version: replay.version },
  };
}

/** Applies a snapshot cursor while deliberately retaining the ready recovery barrier. */
export function applyRuntimeRecoverySnapshot(
  state: RuntimeRecoveryState,
  cursor: RuntimeCursor,
): { state: RuntimeRecoveryState; cursor: RuntimeCursor } {
  return { state, cursor };
}

/** Completes recovery only when ready exactly matches the locally applied stream cursor. */
export function applyRuntimeRecoveryReady(
  state: RuntimeRecoveryState,
  cursor: RuntimeCursor | null,
  ready: RuntimeCursor | null,
): RuntimeReadyDecision {
  if (state.phase === 'failed') {
    return { kind: 'reject', reason: 'recovery_failed', state, cursor };
  }
  if (cursor === null || ready === null) {
    return { kind: 'reject', reason: 'missing_cursor', state, cursor };
  }
  if (!isRuntimeCursor(cursor)) {
    return { kind: 'reject', reason: 'invalid_cursor', state, cursor };
  }
  if (!isRuntimeCursor(ready)) {
    return { kind: 'reject', reason: 'invalid_ready', state, cursor };
  }
  if (ready.streamId !== cursor.streamId) {
    return { kind: 'reject', reason: 'wrong_stream', state, cursor };
  }
  if (ready.version !== cursor.version) {
    return { kind: 'reject', reason: 'version_mismatch', state, cursor };
  }

  return {
    kind: 'complete',
    state:
      state.phase === 'stream' || state.phase === 'http-fallback'
        ? { phase: 'idle', generation: state.generation }
        : state,
    cursor,
  };
}

/** Marks the active recovery generation failed without starting another generation. */
export function failRuntimeRecovery(
  state: RuntimeRecoveryState,
  reason: string,
): RuntimeRecoveryState {
  if (state.phase !== 'stream' && state.phase !== 'http-fallback') {
    return state;
  }
  return { phase: 'failed', generation: state.generation, reason };
}

function isRuntimeVersion(value: number): boolean {
  return Number.isSafeInteger(value) && value >= 0;
}

function isRuntimeCursor(cursor: RuntimeCursor): boolean {
  return cursor.streamId.trim().length > 0 && isRuntimeVersion(cursor.version);
}

function isRuntimeRecoveryTime(value: number): boolean {
  return Number.isFinite(value) && value >= 0;
}
