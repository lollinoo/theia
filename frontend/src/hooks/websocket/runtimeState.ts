/**
 * Coordinates runtime state WebSocket lifecycle and runtime update semantics.
 * Keeps reconnect, resync, and subscription behavior isolated from canvas rendering.
 */
import type { ResyncRequiredPayload } from '../../types/metrics';

type RuntimeDeltaMessageType = 'snapshot_delta' | 'runtime_delta';

interface RuntimeDeltaEnvelope {
  base_version?: number;
  version?: number;
  runtime_stream_id?: string;
  runtime_identity?: string;
}

interface RuntimeDeltaClientState {
  currentVersion: number | null;
  currentStreamId?: string | null;
  hasRuntimeSnapshot: boolean;
}

/** Describes the runtime delta resync diagnostic reason contract used by the React hook lifecycle. */
export type RuntimeDeltaResyncDiagnosticReason =
  | 'base_version_mismatch'
  | 'invalid_delta_version'
  | 'missing_base_snapshot'
  | 'runtime_stream_mismatch';

/** Describes the runtime delta decision contract used by the React hook lifecycle. */
export type RuntimeDeltaDecision =
  | {
      kind: 'apply_unversioned';
    }
  | {
      kind: 'apply_versioned';
      nextVersion: number;
      runtimeIdentity?: string;
    }
  | {
      kind: 'ignore_stale';
      messageType: RuntimeDeltaMessageType;
      baseVersion: number;
      version: number;
      currentVersion: number;
    }
  | {
      kind: 'reject_missing_unversioned_base';
    }
  | {
      kind: 'request_resync';
      payloadReason: ResyncRequiredPayload['reason'];
      diagnosticReason: RuntimeDeltaResyncDiagnosticReason;
    };

/** Should ignore stale runtime snapshot for the React hook lifecycle. */
export function shouldIgnoreStaleRuntimeSnapshot(
  incomingVersion: number | null,
  currentVersion: number | null,
  hasRuntimeSnapshot: boolean,
): boolean {
  return (
    incomingVersion !== null &&
    currentVersion !== null &&
    hasRuntimeSnapshot &&
    incomingVersion < currentVersion
  );
}

/** Classify runtime delta for the React hook lifecycle. */
export function classifyRuntimeDelta(
  messageType: RuntimeDeltaMessageType,
  payload: RuntimeDeltaEnvelope,
  state: RuntimeDeltaClientState,
): RuntimeDeltaDecision {
  const hasVersionEnvelope = payload.version !== undefined || payload.base_version !== undefined;
  const hasCurrentStreamValue =
    state.currentStreamId !== undefined && state.currentStreamId !== null;
  const hasIncomingStreamValue = payload.runtime_stream_id !== undefined;
  const hasEstablishedStream = hasCurrentStreamValue && state.currentStreamId!.trim().length > 0;
  const hasIncomingStream = hasIncomingStreamValue && payload.runtime_stream_id!.trim().length > 0;
  if (
    (hasCurrentStreamValue && !hasEstablishedStream) ||
    (hasIncomingStreamValue && !hasIncomingStream)
  ) {
    return {
      kind: 'request_resync',
      payloadReason: 'client_resync_scheduled',
      diagnosticReason: 'runtime_stream_mismatch',
    };
  }
  if (!hasVersionEnvelope && !hasEstablishedStream && !hasIncomingStream) {
    // Older backend streams sent unversioned deltas; keep their local reject behavior unchanged.
    return state.hasRuntimeSnapshot
      ? { kind: 'apply_unversioned' }
      : { kind: 'reject_missing_unversioned_base' };
  }

  if (
    (hasEstablishedStream && payload.runtime_stream_id !== state.currentStreamId) ||
    (!hasEstablishedStream && hasIncomingStream)
  ) {
    return {
      kind: 'request_resync',
      payloadReason: 'client_resync_scheduled',
      diagnosticReason: 'runtime_stream_mismatch',
    };
  }

  if (payload.version === undefined || payload.base_version === undefined) {
    return {
      kind: 'request_resync',
      payloadReason: 'client_resync_scheduled',
      diagnosticReason: 'invalid_delta_version',
    };
  }

  if (
    !isRuntimeVersion(payload.version) ||
    !isRuntimeVersion(payload.base_version) ||
    payload.version <= payload.base_version
  ) {
    return {
      kind: 'request_resync',
      payloadReason: 'client_resync_scheduled',
      diagnosticReason: 'invalid_delta_version',
    };
  }

  if (state.currentVersion === null || !state.hasRuntimeSnapshot) {
    return {
      kind: 'request_resync',
      payloadReason: 'client_missing_runtime_snapshot',
      diagnosticReason: 'missing_base_snapshot',
    };
  }

  if (payload.base_version < state.currentVersion) {
    return {
      kind: 'ignore_stale',
      messageType,
      baseVersion: payload.base_version,
      version: payload.version,
      currentVersion: state.currentVersion,
    };
  }

  if (payload.base_version > state.currentVersion) {
    return {
      kind: 'request_resync',
      payloadReason: 'client_resync_scheduled',
      diagnosticReason: 'base_version_mismatch',
    };
  }

  return {
    kind: 'apply_versioned',
    nextVersion: payload.version,
    runtimeIdentity: payload.runtime_identity,
  };
}

function isRuntimeVersion(value: number): boolean {
  return Number.isSafeInteger(value) && value >= 0;
}
