/**
 * Renders device details panel UI behavior for the Theia frontend.
 * Keeps this component's state and interaction boundary explicit for maintainers.
 */
import { type ReactNode, useState } from 'react';
import type { Device } from '../types/api';
import { type DeviceMetricsDTO, formatUptime } from '../types/metrics';
import { MaterialIcon } from './MaterialIcon';

interface DeviceDetailsPanelProps {
  device: Device;
  detailMetrics: DeviceMetricsDTO | null;
  interfaceStats?: ReactNode;
}

function formatEmpty(value: string | number | null | undefined): string {
  if (value === null || value === undefined || value === '') return '-';
  return String(value);
}

function formatPercent(value: number | null | undefined): string {
  if (value === null || value === undefined) return '-';
  return `${new Intl.NumberFormat('en-US', { maximumFractionDigits: 2 }).format(value)}%`;
}

function formatTemperature(value: number | null | undefined): string {
  return value === null || value === undefined ? '-' : `${value} C`;
}

function formatReachable(value: DeviceMetricsDTO['network_reachable']): string {
  if (value === 'true') return 'yes';
  if (value === 'false') return 'no';
  return 'unknown';
}

function formatTimestamp(value: string | null | undefined): string {
  if (!value) return '-';
  const parsed = new Date(value);
  if (Number.isNaN(parsed.getTime())) return value;
  return new Intl.DateTimeFormat('en-US', {
    month: 'short',
    day: 'numeric',
    year: 'numeric',
    hour: '2-digit',
    minute: '2-digit',
    second: '2-digit',
    hour12: false,
    timeZone: 'UTC',
    timeZoneName: 'short',
  }).format(parsed);
}

function DetailRow({
  label,
  value,
  valueClassName = 'text-on-bg',
}: {
  label: string;
  value: string;
  valueClassName?: string;
}) {
  return (
    <div className="grid grid-cols-[minmax(0,1fr)_minmax(0,1fr)] items-center gap-3">
      <span className="min-w-0 text-xs uppercase text-on-bg-secondary">{label}</span>
      <span className={`min-w-0 break-words text-right text-sm ${valueClassName}`}>{value}</span>
    </div>
  );
}

function reachabilityClass(value: DeviceMetricsDTO['network_reachable']): string {
  if (value === 'true') return 'text-status-up';
  if (value === 'false') return 'text-status-down';
  return 'text-on-bg-secondary';
}

/** Renders the DeviceDetailsPanel component within the UI component boundary. */
export function DeviceDetailsPanel({
  device,
  detailMetrics,
  interfaceStats,
}: DeviceDetailsPanelProps) {
  const [interfacesExpanded, setInterfacesExpanded] = useState(false);
  const deviceLabel =
    device.tags?.display_name || device.sys_name || device.hostname || device.ip || device.id;
  const modelLabel = [device.vendor, device.hardware_model].filter(Boolean).join(' ');

  return (
    <div className="space-y-5 p-4 transition-colors duration-200">
      <div className="space-y-2 rounded-lg bg-surface-high p-3">
        <p className="text-xs font-medium uppercase text-on-bg-secondary">Device Summary</p>
        <div>
          <p className="text-sm font-medium text-on-bg">{deviceLabel}</p>
          <p className="mt-0.5 font-mono text-xs text-on-bg-secondary">{formatEmpty(device.ip)}</p>
        </div>
        <div className="grid grid-cols-2 gap-3 text-xs">
          <div>
            <p className="uppercase text-on-bg-secondary">Model</p>
            <p className="mt-0.5 text-on-bg">{modelLabel || '-'}</p>
          </div>
          <div>
            <p className="uppercase text-on-bg-secondary">Metrics</p>
            <p className="mt-0.5 text-on-bg">{formatEmpty(device.metrics_source)}</p>
          </div>
        </div>
      </div>

      <div className="space-y-2 rounded-lg bg-surface-high p-3">
        <p className="text-xs font-medium uppercase text-on-bg-secondary">Device Notes</p>
        <p className="whitespace-pre-wrap text-sm text-on-bg">
          {device.notes?.trim() ? device.notes : 'No notes saved.'}
        </p>
      </div>

      <div className="space-y-3" data-testid="device-detail-runtime">
        <div className="flex items-center justify-between">
          <p className="text-xs font-medium uppercase text-on-bg-secondary">
            Live Detail Telemetry
          </p>
          <span className="text-xs text-on-bg-secondary/70">
            {detailMetrics?.freshness ?? 'unknown'}
          </span>
        </div>

        {detailMetrics ? (
          <div className="space-y-2 rounded-lg bg-surface-high p-3">
            <DetailRow label="Operational status" value={detailMetrics.operational_status} />
            <DetailRow label="Primary health" value={detailMetrics.primary_health} />
            <DetailRow label="Reachability" value={detailMetrics.reachability} />
            <DetailRow
              label="Network reachable"
              value={formatReachable(detailMetrics.network_reachable)}
              valueClassName={reachabilityClass(detailMetrics.network_reachable)}
            />
            <DetailRow
              label="SNMP reachable"
              value={formatReachable(detailMetrics.snmp_reachable)}
              valueClassName={reachabilityClass(detailMetrics.snmp_reachable)}
            />
            <DetailRow label="Metrics status" value={detailMetrics.metrics_status} />
            <DetailRow
              label="Expected interval"
              value={
                detailMetrics.expected_poll_interval_seconds != null
                  ? `${detailMetrics.expected_poll_interval_seconds}s`
                  : '-'
              }
            />
            <DetailRow label="Last poll" value={formatTimestamp(detailMetrics.last_polled_at)} />
            <DetailRow
              label="Runtime uptime"
              value={
                detailMetrics.uptime_secs != null ? formatUptime(detailMetrics.uptime_secs) : '-'
              }
            />
            <DetailRow label="CPU" value={formatPercent(detailMetrics.cpu_percent)} />
            <DetailRow label="Memory" value={formatPercent(detailMetrics.mem_percent)} />
            <DetailRow label="Temperature" value={formatTemperature(detailMetrics.temp_celsius)} />
            <DetailRow label="Active alerts" value={String(detailMetrics.firing_alert_count)} />
          </div>
        ) : (
          <p className="rounded-lg border border-outline-subtle bg-elevated px-3 py-2 text-xs text-on-bg-secondary">
            No live telemetry available for this device.
          </p>
        )}
      </div>

      {interfaceStats && (
        <div className="overflow-hidden rounded-lg bg-surface-high">
          <button
            type="button"
            aria-label={`Interfaces ${interfacesExpanded ? 'Hide' : 'Show'}`}
            aria-expanded={interfacesExpanded}
            aria-controls="device-interface-stats"
            onClick={() => setInterfacesExpanded((value) => !value)}
            className="flex w-full items-center justify-between gap-3 px-3 py-2 text-left transition-colors hover:bg-elevated"
          >
            <span className="flex min-w-0 items-center gap-2">
              <MaterialIcon
                name="expand_more"
                size={18}
                className={`text-on-bg-secondary transition-transform ${interfacesExpanded ? '' : '-rotate-90'}`}
              />
              <span className="text-xs font-medium uppercase text-on-bg-secondary">Interfaces</span>
            </span>
            <span className="text-xs text-on-bg-secondary">
              {interfacesExpanded ? 'Hide' : 'Show'}
            </span>
          </button>
          {interfacesExpanded && <div id="device-interface-stats">{interfaceStats}</div>}
        </div>
      )}
    </div>
  );
}
