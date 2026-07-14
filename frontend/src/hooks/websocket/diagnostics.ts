/**
 * Coordinates diagnostics WebSocket lifecycle and runtime update semantics.
 * Keeps reconnect, resync, and subscription behavior isolated from canvas rendering.
 */
import { recordCanvasDiagnosticEvent } from '../../components/canvas/canvasDiagnostics';
import type { ResyncRequiredPayload } from '../../types/metrics';
import type { RuntimeRecoveryState } from './runtimeRecovery';

/** Describes stable event metadata derived from the pure runtime recovery state. */
export type RuntimeRecoveryDiagnosticMetadata =
  | { phase: 'idle'; generation: number }
  | {
      phase: 'stream';
      generation: number;
      durationMs: number;
      reason: string;
      targetVersion: number | null;
    }
  | {
      phase: 'http-fallback';
      generation: number;
      durationMs: number;
      reason: string;
    }
  | { phase: 'failed'; generation: number; reason: string };

/** Builds runtime recovery event metadata without recording or mutating diagnostics. */
export function buildRuntimeRecoveryDiagnosticMetadata(
  state: RuntimeRecoveryState,
  now: number,
): RuntimeRecoveryDiagnosticMetadata {
  if (state.phase === 'stream') {
    return {
      phase: state.phase,
      generation: state.generation,
      durationMs: getRuntimeRecoveryDuration(state.startedAt, now),
      reason: state.reason,
      targetVersion: state.targetVersion,
    };
  }
  if (state.phase === 'http-fallback') {
    return {
      phase: state.phase,
      generation: state.generation,
      durationMs: getRuntimeRecoveryDuration(state.startedAt, now),
      reason: state.reason,
    };
  }
  if (state.phase === 'failed') {
    return {
      phase: state.phase,
      generation: state.generation,
      reason: state.reason,
    };
  }
  return { phase: state.phase, generation: state.generation };
}

function getRuntimeRecoveryDuration(startedAt: number, now: number): number {
  if (!Number.isFinite(startedAt) || startedAt < 0 || !Number.isFinite(now) || now < startedAt) {
    return 0;
  }
  const durationMs = now - startedAt;
  return Number.isFinite(durationMs) ? durationMs : 0;
}

/** Dispatch backend resync required for the React hook lifecycle. */
export function dispatchBackendResyncRequired(payload: ResyncRequiredPayload): void {
  window.dispatchEvent(
    new CustomEvent<ResyncRequiredPayload>('backend-resync-required', {
      detail: payload,
    }),
  );
}

/** Records ignored stale runtime delta for the React hook lifecycle. */
export function recordIgnoredStaleRuntimeDelta({
  messageType,
  baseVersion,
  version,
  currentVersion,
}: {
  messageType: 'snapshot_delta' | 'runtime_delta';
  baseVersion: number;
  version: number;
  currentVersion: number;
}): void {
  recordCanvasDiagnosticEvent({
    level: 'debug',
    source: 'runtime',
    event: 'runtime.delta.ignored',
    message: 'Runtime delta ignored because it is older than the current client base',
    metadata: {
      type: messageType,
      reason: 'stale_delta',
      baseVersion,
      version,
      currentVersion,
    },
  });
}

/** Records ignored stale runtime snapshot for the React hook lifecycle. */
export function recordIgnoredStaleRuntimeSnapshot({
  version,
  currentVersion,
  runtimeIdentity,
}: {
  version: number;
  currentVersion: number;
  runtimeIdentity?: string;
}): void {
  recordCanvasDiagnosticEvent({
    level: 'debug',
    source: 'runtime',
    event: 'runtime.snapshot.ignored',
    message: 'Runtime snapshot ignored because it is older than the current client base',
    metadata: {
      reason: 'stale_snapshot',
      version,
      currentVersion,
      runtimeIdentity,
    },
  });
}

/** Returns raw web socket message type for the React hook lifecycle. */
export function getRawWebSocketMessageType(raw: unknown): string | null {
  if (raw === null || typeof raw !== 'object') {
    return null;
  }
  const type = (raw as { type?: unknown }).type;
  return typeof type === 'string' ? type : null;
}
