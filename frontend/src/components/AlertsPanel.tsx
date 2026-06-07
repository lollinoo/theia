/**
 * Renders alerts panel UI behavior for the Theia frontend.
 * Keeps this component's state and interaction boundary explicit for maintainers.
 */
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
      <span className="inline-flex items-center rounded-full border border-warning/30 bg-warning/10 px-2 py-0.5 text-[10px] font-semibold uppercase tracking-wider text-warning">
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

const alertTitleAcronyms = new Set([
  'API',
  'BGP',
  'CPU',
  'DNS',
  'HTTP',
  'HTTPS',
  'ICMP',
  'IP',
  'LAN',
  'OID',
  'SNMP',
  'SSH',
  'SSL',
  'TCP',
  'TLS',
  'UDP',
  'VPN',
  'WAN',
]);

function readableAlertTitle(alertName: string) {
  const words = alertName
    .trim()
    .split(/[^A-Za-z0-9]+/)
    .flatMap((part) => part.match(/[A-Z]+(?=[A-Z][a-z]|\d|$)|\d+[a-z]+|[A-Z]?[a-z]+|\d+/g) ?? []);

  if (words.length === 0) {
    return alertName;
  }

  return words
    .map((word, index) => {
      const upperWord = word.toUpperCase();
      if (alertTitleAcronyms.has(upperWord)) {
        return upperWord;
      }
      if (index === 0) {
        return word.charAt(0).toUpperCase() + word.slice(1).toLowerCase();
      }
      return word.toLowerCase();
    })
    .join(' ');
}

export function alertRowKey(
  alert: AlertsPanelModel['firingAlerts'][number],
  stateGroup: string,
  occurrence: number,
): string {
  return [
    stateGroup,
    occurrence,
    alert.deviceId,
    alert.alertName,
    alert.deviceLabel,
    alert.severity,
    alert.state,
    alert.summary,
  ].join(':');
}

/** Renders the AlertsPanel component within the UI component boundary. */
export function AlertsPanel({ model }: AlertsPanelProps) {
  const { activeAlertCount, firingAlerts, resolvedAlerts, prometheusDiagnostics } = model;
  const hasActiveAlerts = activeAlertCount > 0 || firingAlerts.length > 0;
  const hiddenActiveAlerts = Math.max(activeAlertCount - firingAlerts.length, 0);
  const activeAlertLabel = activeAlertCount === 1 ? 'active alert' : 'active alerts';
  const hiddenActiveAlertsMessage =
    firingAlerts.length === 0
      ? `Runtime reports ${activeAlertCount} ${activeAlertLabel}, but no individual rows can be shown ` +
        'right now.'
      : `Runtime reports ${activeAlertCount} ${activeAlertLabel}, but only ${firingAlerts.length} can ` +
        'be shown as individual rows right now.';

  return (
    <div className="space-y-4">
      {/* Prometheus diagnostics */}
      {prometheusDiagnostics && (
        <div className="space-y-2">
          <div className="flex items-start gap-2.5 rounded-lg border border-status-down/25 bg-status-down/8 p-3">
            <span className="mt-0.5 h-2 w-2 flex-none rounded-full bg-status-down animate-pulse motion-reduce:animate-none" />
            <div className="min-w-0">
              <p className="text-sm font-medium text-status-down">{prometheusDiagnostics.title}</p>
              <p className="mt-0.5 text-xs text-status-down">{prometheusDiagnostics.detail}</p>
            </div>
          </div>
        </div>
      )}

      {/* Firing alerts */}
      {hasActiveAlerts ? (
        <div className="space-y-2">
          <p className="text-xs font-medium uppercase tracking-widest text-on-bg-secondary">
            Active alerts ({activeAlertCount})
          </p>
          {hiddenActiveAlerts > 0 && (
            <p className="text-[11px] text-on-bg-secondary">{hiddenActiveAlertsMessage}</p>
          )}
          {firingAlerts.map((alert, index) => (
            <div
              key={alertRowKey(alert, 'firing', index)}
              className="rounded-lg bg-elevated shadow-panel p-3 space-y-1.5 transition-colors duration-200"
            >
              <div className="flex items-center gap-2">
                {stateBadge(alert.state)}
                <span className="text-sm font-medium text-on-bg truncate">{alert.deviceLabel}</span>
                {severityBadge(alert.severity)}
              </div>
              <p className="text-xs text-on-bg-secondary">
                Problem: {readableAlertTitle(alert.alertName)}
              </p>
              <p className="text-[11px] text-on-bg-secondary">Details: {alert.summary}</p>
            </div>
          ))}
        </div>
      ) : (
        <div className="flex flex-col items-center justify-center py-8 text-center">
          <MaterialIcon name="check_circle" className="text-status-up/50 mb-2" size={32} />
          <p className="text-sm text-on-bg-secondary">No active alerts</p>
          <p className="text-xs text-on-bg-secondary mt-0.5">All systems operational</p>
        </div>
      )}

      {/* Resolved alerts */}
      {resolvedAlerts.length > 0 && (
        <div className="space-y-2">
          <p className="text-xs font-medium uppercase tracking-widest text-on-bg-secondary">
            Resolved alerts ({resolvedAlerts.length})
          </p>
          {resolvedAlerts.map((alert, index) => (
            <div
              key={alertRowKey(alert, 'resolved', index)}
              className="rounded-lg bg-surface-high p-3 space-y-1.5 opacity-60 transition-colors duration-200"
            >
              <div className="flex items-center gap-2">
                {stateBadge(alert.state)}
                <span className="text-sm font-medium text-on-bg truncate">{alert.deviceLabel}</span>
                {severityBadge(alert.severity)}
              </div>
              <p className="text-xs text-on-bg-secondary">
                Problem: {readableAlertTitle(alert.alertName)}
              </p>
              <p className="text-[11px] text-on-bg-secondary">Details: {alert.summary}</p>
            </div>
          ))}
        </div>
      )}
    </div>
  );
}
