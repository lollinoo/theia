import { ReconnectBanner } from '../ReconnectBanner';
import type { TopologyRecoveryNotice } from './useCanvasData';

interface CanvasOverlaysProps {
  editMode: boolean;
  reconnecting: boolean;
  topologyRecoveryNotice: TopologyRecoveryNotice | null;
  dismissTopologyRecoveryNotice: () => void;
  retryTopologyRefresh: () => void;
  selectedNodeCount: number;
  prometheusDiagnosticsVisible: boolean;
  onBulkEditClick?: () => void;
}

export function CanvasOverlays({
  editMode,
  reconnecting,
  topologyRecoveryNotice,
  dismissTopologyRecoveryNotice,
  retryTopologyRefresh,
  selectedNodeCount,
  prometheusDiagnosticsVisible,
  onBulkEditClick,
}: CanvasOverlaysProps) {
  return (
    <>
      {/* Bottom-center stacking container for all status pills */}
      <div className="absolute bottom-16 left-1/2 z-50 -translate-x-1/2 flex flex-col items-center gap-2 pointer-events-none">
        {selectedNodeCount > 1 && (
          <button
            type="button"
            onClick={onBulkEditClick}
            className="pointer-events-auto flex items-center gap-2.5 rounded-full border border-primary/30 bg-surface-container-high/95 px-4 py-2 shadow-floating backdrop-blur-sm transition-colors hover:bg-surface-container-high"
          >
            <span className="flex h-5 min-w-[20px] items-center justify-center rounded-full bg-primary/20 px-1.5 text-[11px] font-bold text-primary">
              {selectedNodeCount}
            </span>
            <span className="text-sm text-on-bg-secondary">nodes selected</span>
            {editMode && <span className="text-xs text-primary font-medium">Edit</span>}
          </button>
        )}
        {prometheusDiagnosticsVisible && (
          <div className="pointer-events-none flex items-center gap-2.5 rounded-full border border-warning/30 bg-surface-container-high/95 px-4 py-2.5 shadow-floating backdrop-blur-sm">
            <span className="h-2 w-2 flex-none rounded-full bg-warning" />
            <p className="text-sm text-warning">Prometheus degraded</p>
            <span className="text-xs text-on-bg-secondary">Diagnostics only</span>
          </div>
        )}
        {topologyRecoveryNotice && (
          <div
            className={`pointer-events-auto flex items-center gap-2.5 rounded-full px-4 py-2.5 shadow-floating backdrop-blur-sm ${
              topologyRecoveryNotice.tone === 'warning'
                ? 'border border-warning/30 bg-surface-container-high/95'
                : 'border border-status-up/30 bg-surface-container-high/95'
            }`}
          >
            <span
              className={`h-2 w-2 flex-none rounded-full ${
                topologyRecoveryNotice.tone === 'warning'
                  ? 'bg-warning animate-pulse'
                  : 'bg-status-up'
              }`}
            />
            <p
              className={`text-sm ${
                topologyRecoveryNotice.tone === 'warning' ? 'text-warning' : 'text-status-up'
              }`}
            >
              {topologyRecoveryNotice.message}
            </p>
            {topologyRecoveryNotice.tone === 'warning' && topologyRecoveryNotice.actionLabel && (
              <button
                type="button"
                onClick={retryTopologyRefresh}
                className="text-xs font-medium text-warning hover:text-warning/80"
              >
                {topologyRecoveryNotice.actionLabel}
              </button>
            )}
            <button
              type="button"
              onClick={dismissTopologyRecoveryNotice}
              className="text-on-bg-secondary hover:text-on-bg"
              title="Dismiss"
            >
              <svg className="h-3.5 w-3.5" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                <path
                  strokeLinecap="round"
                  strokeLinejoin="round"
                  strokeWidth={2}
                  d="M6 18L18 6M6 6l12 12"
                />
              </svg>
            </button>
          </div>
        )}
        {editMode && (
          <div className="pointer-events-auto flex items-center gap-2.5 rounded-full border border-primary/30 bg-surface-container-high/95 px-4 py-2.5 shadow-floating backdrop-blur-sm">
            <svg
              fill="none"
              viewBox="0 0 24 24"
              stroke="currentColor"
              className="h-4 w-4 text-primary"
            >
              <path
                strokeLinecap="round"
                strokeLinejoin="round"
                strokeWidth={2}
                d="M15.232 5.232l3.536 3.536m-2.036-5.036a2.5 2.5 0 113.536 3.536L6.5 21.036H3v-3.572L16.732 3.732z"
              />
            </svg>
            <p className="text-sm text-primary">Edit Mode</p>
            <span className="text-xs text-on-bg-secondary">Press E to exit</span>
          </div>
        )}
      </div>
      <ReconnectBanner visible={reconnecting} />
    </>
  );
}
