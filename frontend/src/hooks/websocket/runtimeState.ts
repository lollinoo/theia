import type { ResyncRequiredPayload } from '../../types/metrics';

type RuntimeDeltaMessageType = 'snapshot_delta' | 'runtime_delta';

interface RuntimeDeltaEnvelope {
  base_version?: number;
  version?: number;
  runtime_identity?: string;
}

interface RuntimeDeltaClientState {
  currentVersion: number | null;
  hasRuntimeSnapshot: boolean;
}

export type RuntimeDeltaResyncDiagnosticReason =
  | 'base_version_mismatch'
  | 'invalid_delta_version'
  | 'missing_base_snapshot';

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

export function classifyRuntimeDelta(
  messageType: RuntimeDeltaMessageType,
  payload: RuntimeDeltaEnvelope,
  state: RuntimeDeltaClientState,
): RuntimeDeltaDecision {
  const hasVersionEnvelope = payload.version !== undefined || payload.base_version !== undefined;
  if (!hasVersionEnvelope) {
    // Older backend streams sent unversioned deltas; keep their local reject behavior unchanged.
    return state.hasRuntimeSnapshot
      ? { kind: 'apply_unversioned' }
      : { kind: 'reject_missing_unversioned_base' };
  }

  if (payload.version === undefined || payload.base_version === undefined) {
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
