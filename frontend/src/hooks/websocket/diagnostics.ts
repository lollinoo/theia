/**
 * Coordinates diagnostics WebSocket lifecycle and runtime update semantics.
 * Keeps reconnect, resync, and subscription behavior isolated from canvas rendering.
 */
import { recordCanvasDiagnosticEvent } from '../../components/canvas/canvasDiagnostics';
import type { ResyncRequiredPayload } from '../../types/metrics';

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
