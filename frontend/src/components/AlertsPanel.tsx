import type { Device } from '../types/api';
import type { AlertDTO, PrometheusStatusPayload } from '../types/metrics';

interface AlertsPanelProps {
  alerts: AlertDTO[];
  devices: Device[];
  prometheusStatus: PrometheusStatusPayload | null;
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
    <span className="inline-flex items-center rounded-full bg-text-secondary/15 px-2 py-0.5 text-[10px] font-semibold uppercase tracking-wider text-text-secondary">
      {severity}
    </span>
  );
}

function stateBadge(state: string) {
  if (state === 'firing') {
    return (
      <span className="h-2 w-2 flex-none rounded-full bg-status-down animate-pulse" />
    );
  }
  return (
    <span className="h-2 w-2 flex-none rounded-full bg-status-up" />
  );
}

export function AlertsPanel({ alerts, devices, prometheusStatus }: AlertsPanelProps) {
  const deviceMap = new Map(devices.map((d) => [d.id, d]));

  function deviceLabel(deviceId: string): string {
    const d = deviceMap.get(deviceId);
    if (!d) return deviceId.slice(0, 8);
    return d.tags?.display_name || d.sys_name || d.ip;
  }

  const firingAlerts = alerts.filter((a) => a.state === 'firing');
  const resolvedAlerts = alerts.filter((a) => a.state !== 'firing');
  const promDown = prometheusStatus !== null && !prometheusStatus.available;

  return (
    <div className="space-y-4">
      {/* Prometheus status */}
      {promDown && (
        <div className="flex items-start gap-2.5 rounded-lg border border-yellow-500/25 bg-yellow-500/8 p-3">
          <span className="mt-0.5 h-2 w-2 flex-none rounded-full bg-yellow-400 animate-pulse" />
          <div className="min-w-0">
            <p className="text-sm font-medium text-yellow-300">Prometheus unreachable</p>
            <p className="mt-0.5 text-xs text-yellow-300/70">
              Metrics collection is paused. Device and link data may be stale.
            </p>
          </div>
        </div>
      )}

      {/* Firing alerts */}
      {firingAlerts.length > 0 ? (
        <div className="space-y-2">
          <p className="text-xs font-medium uppercase tracking-widest text-text-secondary">
            Active ({firingAlerts.length})
          </p>
          {firingAlerts.map((alert, i) => (
            <div
              key={`${alert.device_id}-${alert.alert_name}-${i}`}
              className="rounded-lg border border-border-subtle bg-bg-elevated p-3 space-y-1.5"
            >
              <div className="flex items-center gap-2">
                {stateBadge(alert.state)}
                <span className="text-sm font-medium text-text-primary truncate">
                  {alert.alert_name}
                </span>
                {severityBadge(alert.severity)}
              </div>
              <p className="text-xs text-text-secondary">{alert.summary}</p>
              <p className="text-[11px] text-text-secondary/70">
                {deviceLabel(alert.device_id)}
              </p>
            </div>
          ))}
        </div>
      ) : !promDown ? (
        <div className="flex flex-col items-center justify-center py-8 text-center">
          <svg fill="none" viewBox="0 0 24 24" stroke="currentColor" className="h-8 w-8 text-status-up/50 mb-2">
            <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={1.5} d="M9 12l2 2 4-4m6 2a9 9 0 11-18 0 9 9 0 0118 0z" />
          </svg>
          <p className="text-sm text-text-secondary">No active alerts</p>
          <p className="text-xs text-text-secondary/60 mt-0.5">All systems operational</p>
        </div>
      ) : null}

      {/* Resolved alerts */}
      {resolvedAlerts.length > 0 && (
        <div className="space-y-2">
          <p className="text-xs font-medium uppercase tracking-widest text-text-secondary">
            Resolved ({resolvedAlerts.length})
          </p>
          {resolvedAlerts.map((alert, i) => (
            <div
              key={`${alert.device_id}-${alert.alert_name}-resolved-${i}`}
              className="rounded-lg border border-border-subtle/50 bg-bg-elevated/50 p-3 space-y-1.5 opacity-60"
            >
              <div className="flex items-center gap-2">
                {stateBadge(alert.state)}
                <span className="text-sm font-medium text-text-primary truncate">
                  {alert.alert_name}
                </span>
                {severityBadge(alert.severity)}
              </div>
              <p className="text-xs text-text-secondary">{alert.summary}</p>
              <p className="text-[11px] text-text-secondary/70">
                {deviceLabel(alert.device_id)}
              </p>
            </div>
          ))}
        </div>
      )}
    </div>
  );
}
