import type { CanvasMeasurementTrigger } from './canvasInstrumentation';

export type StructuralRefreshCause =
  | 'backend-reconnected'
  | 'backend-resync-required'
  | 'topology-changed';

export interface TopologyRecoveryNotice {
  tone: 'success' | 'warning';
  message: string;
  actionLabel?: string;
}

const topologyRefreshRetryActionLabel = 'Retry topology refresh';
const topologyRefreshDelayedMessage = 'Live topology refresh delayed';

export function measurementTriggerForCauses(
  causes: ReadonlySet<StructuralRefreshCause>,
): CanvasMeasurementTrigger {
  if (causes.has('backend-reconnected') || causes.has('backend-resync-required')) {
    return 'backend_reconnected';
  }

  return 'topology_changed';
}

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
      message: 'Live topology resynced',
    };
  }

  return {
    tone: 'success',
    message: 'Topology refreshed after reconnect',
  };
}

export function buildTopologyRecoveryFailureNotice(): TopologyRecoveryNotice {
  return {
    tone: 'warning',
    message: topologyRefreshDelayedMessage,
    actionLabel: topologyRefreshRetryActionLabel,
  };
}
