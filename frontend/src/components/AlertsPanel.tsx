import { MaterialIcon } from './MaterialIcon';
import type { AlertsPanelModel } from './panelModels';

interface AlertsPanelProps {
  model: AlertsPanelModel;
}

function severityBadge(severity: string) {
  if (severity === 'critical') {
    return (
      <span className="inline-flex items-center rounded-full bg-status-down/15 px-2 py-0.5 text-[10px] font-semibold uppercase tracking-wider text-status-down">
        Critical
      </span>
    );
  }
  if (severity === 'warning') {
    return (
      <span className="inline-flex items-center rounded-full bg-yellow-400/15 px-2 py-0.5 text-[10px] font-semibold uppercase tracking-wider text-yellow-400">
        Warning
      </span>
    );
  }
  return (
    <span className="inline-flex items-center rounded-full bg-text-secondary/15 px-2 py-0.5 text-[10px] font-semibold uppercase tracking-wider text-on-bg-secondary">
      {severity}
    </span>
  );
}

function stateBadge(state: string) {
  if (state === 'firing') {
    return (
      <span className="h-2 w-2 flex-none rounded-full bg-status-down animate-pulse shadow-[0_0_10px_rgba(255,23,68,var(--nt-glow-shadow-opacity))] motion-reduce:animate-none" />
    );
  }
  return (
    <span className="h-2 w-2 flex-none rounded-full bg-status-up shadow-[0_0_6px_rgba(0,230,118,var(--nt-glow-shadow-opacity))]" />
  );
}

export function AlertsPanel({ model }: AlertsPanelProps) {
  const { activeAlertCount, firingAlerts, resolvedAlerts, prometheusDiagnostics } = model;
  const hiddenActiveAlerts = Math.max(activeAlertCount - firingAlerts.length, 0);

  return (
    <div className="space-y-4">
      {/* Prometheus diagnostics */}
      {prometheusDiagnostics && (
        <div className="space-y-2">
          <div className="flex items-start gap-2.5 rounded-lg border border-red-500/25 bg-red-500/8 p-3">
            <span className="mt-0.5 h-2 w-2 flex-none rounded-full bg-red-400 animate-pulse motion-reduce:animate-none" />
            <div className="min-w-0">
              <p className="text-sm font-medium text-red-300">{prometheusDiagnostics.title}</p>
              <p className="mt-0.5 text-xs text-red-300/70">{prometheusDiagnostics.detail}</p>
            </div>
          </div>
        </div>
      )}

      {/* Firing alerts */}
      {firingAlerts.length > 0 ? (
        <div className="space-y-2">
          <p className="text-xs font-medium uppercase tracking-widest text-on-bg-secondary">
            Active ({activeAlertCount})
          </p>
          {hiddenActiveAlerts > 0 && (
            <p className="text-[11px] text-on-bg-secondary/70">
              Showing {firingAlerts.length} alert row{firingAlerts.length === 1 ? '' : 's'} while
              normalized runtime reports {activeAlertCount} active alerts.
            </p>
          )}
          {firingAlerts.map((alert, i) => (
            <div
              key={`${alert.deviceId}-${alert.alertName}-${i}`}
              className="rounded-lg bg-elevated shadow-panel p-3 space-y-1.5 transition-colors duration-200"
            >
              <div className="flex items-center gap-2">
                {stateBadge(alert.state)}
                <span className="text-sm font-medium text-on-bg truncate">{alert.alertName}</span>
                {severityBadge(alert.severity)}
              </div>
              <p className="text-xs text-on-bg-secondary">{alert.summary}</p>
              <p className="text-[11px] text-on-bg-secondary/70">{alert.deviceLabel}</p>
            </div>
          ))}
        </div>
      ) : (
        <div className="flex flex-col items-center justify-center py-8 text-center">
          <MaterialIcon name="check_circle" className="text-status-up/50 mb-2" size={32} />
          <p className="text-sm text-on-bg-secondary">No active alerts</p>
          <p className="text-xs text-on-bg-secondary/60 mt-0.5">All systems operational</p>
        </div>
      )}

      {/* Resolved alerts */}
      {resolvedAlerts.length > 0 && (
        <div className="space-y-2">
          <p className="text-xs font-medium uppercase tracking-widest text-on-bg-secondary">
            Resolved ({resolvedAlerts.length})
          </p>
          {resolvedAlerts.map((alert, i) => (
            <div
              key={`${alert.deviceId}-${alert.alertName}-resolved-${i}`}
              className="rounded-lg bg-surface-high p-3 space-y-1.5 opacity-60 transition-colors duration-200"
            >
              <div className="flex items-center gap-2">
                {stateBadge(alert.state)}
                <span className="text-sm font-medium text-on-bg truncate">{alert.alertName}</span>
                {severityBadge(alert.severity)}
              </div>
              <p className="text-xs text-on-bg-secondary">{alert.summary}</p>
              <p className="text-[11px] text-on-bg-secondary/70">{alert.deviceLabel}</p>
            </div>
          ))}
        </div>
      )}
    </div>
  );
}
