import { MaterialIcon } from './MaterialIcon';
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

  // Categorize devices affected by Prometheus outage
  const promOnlyDevices = promDown
    ? devices.filter((d) => {
        const src = d.metrics_source || 'prometheus';
        return src === 'prometheus';
      })
    : [];
  const snmpFallbackDevices = promDown
    ? devices.filter((d) => d.metrics_source === 'prometheus_snmp_fallback')
    : [];

  return (
    <div className="space-y-4">
      {/* Prometheus status */}
      {promDown && (
        <div className="space-y-2">
          <div className="flex items-start gap-2.5 rounded-lg border border-red-500/25 bg-red-500/8 p-3">
            <span className="mt-0.5 h-2 w-2 flex-none rounded-full bg-red-400 animate-pulse motion-reduce:animate-none" />
            <div className="min-w-0">
              <p className="text-sm font-medium text-red-300">Prometheus unreachable</p>
              <p className="mt-0.5 text-xs text-red-300/70">
                Metrics and probe status unavailable. Devices relying on Prometheus are marked offline.
              </p>
            </div>
          </div>

          {promOnlyDevices.length > 0 && (
            <div className="rounded-lg border border-red-500/15 bg-red-500/5 p-3">
              <p className="text-[10px] font-semibold uppercase tracking-wider text-red-400/80 mb-1.5">
                Offline — no fallback ({promOnlyDevices.length})
              </p>
              <div className="space-y-1">
                {promOnlyDevices.map((d) => (
                  <div key={d.id} className="flex items-center gap-2 text-xs text-red-300/80">
                    <span className="h-1.5 w-1.5 flex-none rounded-full bg-red-400" />
                    {d.tags?.display_name || d.sys_name || d.ip}
                  </div>
                ))}
              </div>
            </div>
          )}

          {snmpFallbackDevices.length > 0 && (
            <div className="rounded-lg border border-yellow-500/15 bg-yellow-500/5 p-3">
              <p className="text-[10px] font-semibold uppercase tracking-wider text-yellow-400/80 mb-1.5">
                SNMP fallback active ({snmpFallbackDevices.length})
              </p>
              <p className="text-xs text-yellow-300/60 mb-1.5">
                Metrics via SNMP. Probe status unavailable.
              </p>
              <div className="space-y-1">
                {snmpFallbackDevices.map((d) => (
                  <div key={d.id} className="flex items-center gap-2 text-xs text-yellow-300/80">
                    <span className="h-1.5 w-1.5 flex-none rounded-full bg-yellow-400" />
                    {d.tags?.display_name || d.sys_name || d.ip}
                  </div>
                ))}
              </div>
            </div>
          )}
        </div>
      )}

      {/* Firing alerts */}
      {firingAlerts.length > 0 ? (
        <div className="space-y-2">
          <p className="text-xs font-medium uppercase tracking-widest text-on-bg-secondary">
            Active ({firingAlerts.length})
          </p>
          {firingAlerts.map((alert, i) => (
            <div
              key={`${alert.device_id}-${alert.alert_name}-${i}`}
              className="rounded-lg bg-elevated shadow-panel p-3 space-y-1.5 transition-colors duration-200"
            >
              <div className="flex items-center gap-2">
                {stateBadge(alert.state)}
                <span className="text-sm font-medium text-on-bg truncate">
                  {alert.alert_name}
                </span>
                {severityBadge(alert.severity)}
              </div>
              <p className="text-xs text-on-bg-secondary">{alert.summary}</p>
              <p className="text-[11px] text-on-bg-secondary/70">
                {deviceLabel(alert.device_id)}
              </p>
            </div>
          ))}
        </div>
      ) : !promDown ? (
        <div className="flex flex-col items-center justify-center py-8 text-center">
          <MaterialIcon name="check_circle" className="text-status-up/50 mb-2" size={32} />
          <p className="text-sm text-on-bg-secondary">No active alerts</p>
          <p className="text-xs text-on-bg-secondary/60 mt-0.5">All systems operational</p>
        </div>
      ) : null}

      {/* Resolved alerts */}
      {resolvedAlerts.length > 0 && (
        <div className="space-y-2">
          <p className="text-xs font-medium uppercase tracking-widest text-on-bg-secondary">
            Resolved ({resolvedAlerts.length})
          </p>
          {resolvedAlerts.map((alert, i) => (
            <div
              key={`${alert.device_id}-${alert.alert_name}-resolved-${i}`}
              className="rounded-lg bg-surface-high p-3 space-y-1.5 opacity-60 transition-colors duration-200"
            >
              <div className="flex items-center gap-2">
                {stateBadge(alert.state)}
                <span className="text-sm font-medium text-on-bg truncate">
                  {alert.alert_name}
                </span>
                {severityBadge(alert.severity)}
              </div>
              <p className="text-xs text-on-bg-secondary">{alert.summary}</p>
              <p className="text-[11px] text-on-bg-secondary/70">
                {deviceLabel(alert.device_id)}
              </p>
            </div>
          ))}
        </div>
      )}
    </div>
  );
}
