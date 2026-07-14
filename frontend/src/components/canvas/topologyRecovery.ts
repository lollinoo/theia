/**
 * Defines topology recovery behavior for the topology canvas.
 * Documents how canonical topology data is projected into the interactive view layer.
 */
import type { CanvasMeasurementTrigger } from './canvasInstrumentation';

/** Describes the structural refresh cause contract used by the topology canvas. */
export type StructuralRefreshCause =
  | 'backend-reconnected'
  | 'backend-resync-required'
  | 'topology-changed';

/** Describes the topology recovery notice contract used by the topology canvas. */
export interface TopologyRecoveryNotice {
  tone: 'success' | 'warning';
  message: string;
  actionLabel?: string;
}

const topologyRefreshRetryActionLabel = 'Retry topology refresh';
const topologyRefreshDelayedMessage = 'Live topology refresh delayed';

// measurementTriggerForCauses maps coalesced structural causes to instrumentation triggers.
export function measurementTriggerForCauses(
  causes: ReadonlySet<StructuralRefreshCause>,
): CanvasMeasurementTrigger {
  if (causes.has('backend-reconnected') || causes.has('backend-resync-required')) {
    return 'backend_reconnected';
  }

  return 'topology_changed';
}

// buildTopologyRecoveryNotice builds user-facing recovery copy for successful structural refreshes.
export function buildTopologyRecoveryNotice(
  causes: ReadonlySet<StructuralRefreshCause>,
): TopologyRecoveryNotice | null {
  const hasReconnect = causes.has('backend-reconnected');
  const hasResync = causes.has('backend-resync-required');
  const hasTopologyChanged = causes.has('topology-changed');

  if (!hasReconnect && !hasResync) {
    return null;
  }

  if ((hasReconnect && hasResync) || hasTopologyChanged) {
    return {
      tone: 'success',
      message: 'Topology refreshed',
    };
  }

  if (hasResync) {
    return {
      tone: 'success',
      message: 'Topology refreshed after backend resync',
    };
  }

  return {
    tone: 'success',
    message: 'Topology refreshed after reconnect',
  };
}

// buildTopologyRecoveryFailureNotice builds the retryable notice for failed structural refreshes.
export function buildTopologyRecoveryFailureNotice(): TopologyRecoveryNotice {
  return {
    tone: 'warning',
    message: topologyRefreshDelayedMessage,
    actionLabel: topologyRefreshRetryActionLabel,
  };
}
