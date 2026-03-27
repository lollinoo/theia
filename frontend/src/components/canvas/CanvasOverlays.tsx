import type { PrometheusStatusPayload } from '../../types/metrics';
import { ReconnectBanner } from '../ReconnectBanner';

interface CanvasOverlaysProps {
  editMode: boolean;
  reconnecting: boolean;
  showRecoveryToast: boolean;
  setShowRecoveryToast: (v: boolean) => void;
  prometheusStatus: PrometheusStatusPayload | null;
  prometheusAlertDismissed: boolean;
  setPrometheusAlertDismissed: (v: boolean) => void;
  setPanelContent: (content: { type: string; data?: unknown } | null) => void;
}

export function CanvasOverlays({
  editMode,
  reconnecting,
  showRecoveryToast,
  setShowRecoveryToast,
  prometheusStatus,
  prometheusAlertDismissed,
  setPrometheusAlertDismissed,
  setPanelContent,
}: CanvasOverlaysProps) {
  return (
    <>
      {editMode && (
        <div className="absolute bottom-16 left-1/2 z-50 -translate-x-1/2 flex items-center gap-2.5 rounded-xl border border-primary/30 bg-surface/95 px-4 py-2.5 shadow-canvas backdrop-blur-sm">
          <svg fill="none" viewBox="0 0 24 24" stroke="currentColor" className="h-4 w-4 text-primary">
            <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M15.232 5.232l3.536 3.536m-2.036-5.036a2.5 2.5 0 113.536 3.536L6.5 21.036H3v-3.572L16.732 3.732z" />
          </svg>
          <p className="text-sm text-primary">Edit Mode</p>
          <span className="text-xs text-on-bg-secondary">Press E to exit</span>
        </div>
      )}
      <ReconnectBanner visible={reconnecting} />
      {showRecoveryToast && (
        <div className="absolute bottom-16 left-1/2 z-50 -translate-x-1/2 flex items-center gap-2.5 rounded-xl border border-status-up/30 bg-surface/95 px-4 py-2.5 shadow-canvas backdrop-blur-sm animate-in fade-in slide-in-from-bottom-2 duration-300">
          <span className="h-2 w-2 flex-none rounded-full bg-status-up" />
          <p className="text-sm text-status-up">Prometheus reconnected</p>
          <button
            type="button"
            onClick={() => setShowRecoveryToast(false)}
            className="text-on-bg-secondary hover:text-on-bg"
            title="Dismiss"
          >
            <svg className="h-3.5 w-3.5" fill="none" viewBox="0 0 24 24" stroke="currentColor">
              <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M6 18L18 6M6 6l12 12" />
            </svg>
          </button>
        </div>
      )}
      {prometheusStatus !== null && !prometheusStatus.available && !prometheusAlertDismissed && (
        <div className="absolute bottom-16 left-1/2 z-50 -translate-x-1/2 flex items-center gap-2.5 rounded-xl border border-warning/30 bg-surface/95 px-4 py-2.5 shadow-canvas backdrop-blur-sm">
          <span className="h-2 w-2 flex-none rounded-full bg-warning animate-pulse" />
          <p className="text-sm text-warning">Prometheus unreachable</p>
          <button
            type="button"
            onClick={() => {
              setPanelContent({ type: 'alerts' });
              setPrometheusAlertDismissed(true);
            }}
            className="text-xs font-medium text-warning hover:text-warning/80"
          >
            Details
          </button>
          <button
            type="button"
            onClick={() => setPrometheusAlertDismissed(true)}
            className="text-on-bg-secondary hover:text-on-bg"
            title="Dismiss"
          >
            <svg className="h-3.5 w-3.5" fill="none" viewBox="0 0 24 24" stroke="currentColor">
              <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M6 18L18 6M6 6l12 12" />
            </svg>
          </button>
        </div>
      )}
    </>
  );
}
