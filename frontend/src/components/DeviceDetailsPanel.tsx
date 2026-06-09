/**
 * Renders device details panel UI behavior for the Theia frontend.
 * Keeps this component's state and interaction boundary explicit for maintainers.
 */
import { type ReactNode, useEffect, useRef, useState } from 'react';
import type { DeviceAddressReachabilityResult } from '../api/client';
import type { Device } from '../types/api';
import { type DeviceMetricsDTO, formatUptime } from '../types/metrics';
import { MaterialIcon } from './MaterialIcon';

interface DeviceDetailsPanelProps {
  device: Device;
  detailMetrics: DeviceMetricsDTO | null;
  interfaceStats?: ReactNode;
  onCheckAddressReachability?: (deviceId: string) => Promise<DeviceAddressReachabilityResult[]>;
  onPromoteAddress?: (addressId: string) => Promise<void>;
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

function formatProbePorts(ports: number[] | null | undefined): string {
  return ports && ports.length > 0 ? ports.join(', ') : '-';
}

function portReachabilityLabel(result: DeviceAddressReachabilityResult | undefined): string {
  if (!result) {
    return '';
  }

  const ports = result.reachable_ports ?? [];
  if (ports.length === 0) {
    return result.reachable ? 'reachable' : 'unreachable';
  }

  const allReachable = ports.every((probe) => probe.reachable);
  const allUnreachable = ports.every((probe) => !probe.reachable);

  if (allReachable) {
    return 'reachable';
  }
  if (allUnreachable) {
    return 'unreachable';
  }
  return 'partially reachable';
}

function portReachabilityClass(result: DeviceAddressReachabilityResult | undefined): string {
  if (!result) {
    return 'text-on-bg-secondary';
  }

  const label = portReachabilityLabel(result);
  if (label === 'reachable') {
    return 'text-status-up';
  }
  if (label === 'unreachable') {
    return 'text-status-down';
  }
  return 'text-warning';
}

function portReachabilityRows(
  result: DeviceAddressReachabilityResult | undefined,
): Array<{ port: number; status: string; reachable: boolean }> {
  if (!result || result.reachable_ports.length === 0) {
    return [];
  }

  return result.reachable_ports.map((probe) => ({
    port: probe.port,
    reachable: probe.reachable,
    status: probe.reachable ? 'up' : 'down',
  }));
}

function portStatusClass(reachable: boolean): string {
  return reachable ? 'text-status-up' : 'text-status-down';
}

function portRowsForAddress(result: DeviceAddressReachabilityResult | undefined) {
  const rows = portReachabilityRows(result);
  if (rows.length === 0) {
    return null;
  }

  return rows.map((row) => (
    <div
      key={`${row.port}`}
      className="flex items-center justify-between gap-3 text-xs text-on-bg-secondary"
    >
      <span>Port {row.port}</span>
      <span className={`font-mono ${portStatusClass(row.reachable)}`}>{row.status}</span>
    </div>
  ));
}

function addressResultKey(result: DeviceAddressReachabilityResult): string {
  return result.address_id || result.address;
}

function reachabilityStatus(result: DeviceAddressReachabilityResult | undefined): {
  label: string;
  className: string;
} {
  if (!result) {
    return { label: 'not checked', className: 'text-on-bg-secondary' };
  }
  return { label: portReachabilityLabel(result), className: portReachabilityClass(result) };
}

function actionErrorMessage(error: unknown): string {
  return error instanceof Error ? error.message : 'Address action failed';
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

function metricHealthClass(value: DeviceMetricsDTO['health']): string {
  switch (value) {
    case 'healthy':
      return 'text-status-up';
    case 'warning':
      return 'text-warning';
    case 'critical':
      return 'text-status-down';
    default:
      return 'text-on-bg-secondary';
  }
}

/** Renders the DeviceDetailsPanel component within the UI component boundary. */
export function DeviceDetailsPanel({
  device,
  detailMetrics,
  interfaceStats,
  onCheckAddressReachability,
  onPromoteAddress,
}: DeviceDetailsPanelProps) {
  const [interfacesExpanded, setInterfacesExpanded] = useState(false);
  const [addressReachability, setAddressReachability] = useState<DeviceAddressReachabilityResult[]>(
    [],
  );
  const [addressReachabilityLoading, setAddressReachabilityLoading] = useState(false);
  const [promotingAddressId, setPromotingAddressId] = useState<string | null>(null);
  const [addressActionError, setAddressActionError] = useState<string | null>(null);
  const addressReachabilityRequestRef = useRef(0);
  const addressPromotionRequestRef = useRef(0);
  const deviceLabel =
    device.tags?.display_name || device.sys_name || device.hostname || device.ip || device.id;
  const modelLabel = [device.vendor, device.hardware_model].filter(Boolean).join(' ');
  const reachabilityByKey = new Map(
    addressReachability.flatMap((result) => [
      [addressResultKey(result), result],
      [result.address, result],
    ]),
  );

  useEffect(() => {
    addressReachabilityRequestRef.current += 1;
    addressPromotionRequestRef.current += 1;
    setAddressReachability([]);
    setAddressReachabilityLoading(false);
    setPromotingAddressId(null);
    setAddressActionError(null);
  }, [device.id]);

  async function handleCheckAddressReachability() {
    if (!onCheckAddressReachability) return;
    const requestId = addressReachabilityRequestRef.current + 1;
    addressReachabilityRequestRef.current = requestId;
    setAddressActionError(null);
    setAddressReachabilityLoading(true);
    try {
      const results = await onCheckAddressReachability(device.id);
      if (addressReachabilityRequestRef.current === requestId) {
        setAddressReachability(results);
      }
    } catch (error) {
      if (addressReachabilityRequestRef.current === requestId) {
        setAddressActionError(actionErrorMessage(error));
      }
    } finally {
      if (addressReachabilityRequestRef.current === requestId) {
        setAddressReachabilityLoading(false);
      }
    }
  }

  async function handlePromoteAddress(addressId: string) {
    if (!onPromoteAddress) return;
    const requestId = addressPromotionRequestRef.current + 1;
    addressPromotionRequestRef.current = requestId;
    setAddressActionError(null);
    setPromotingAddressId(addressId);
    try {
      await onPromoteAddress(addressId);
    } catch (error) {
      if (addressPromotionRequestRef.current === requestId) {
        setAddressActionError(actionErrorMessage(error));
      }
    } finally {
      if (addressPromotionRequestRef.current === requestId) {
        setPromotingAddressId(null);
      }
    }
  }

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

      {device.addresses.length > 0 && (
        <div className="space-y-3 rounded-lg bg-surface-high p-3">
          <div className="flex items-center justify-between gap-3">
            <p className="text-xs font-medium uppercase text-on-bg-secondary">Addresses</p>
            {onCheckAddressReachability && (
              <button
                type="button"
                onClick={() => void handleCheckAddressReachability()}
                disabled={addressReachabilityLoading}
                className="rounded-md border border-outline-subtle px-2 py-1 text-xs text-on-bg-secondary transition-colors hover:bg-elevated disabled:cursor-not-allowed disabled:opacity-60"
              >
                Check address reachability
              </button>
            )}
          </div>
          {addressActionError && (
            <p className="rounded-md border border-status-down/40 bg-status-down/10 px-2 py-1 text-xs text-status-down">
              {addressActionError}
            </p>
          )}
          <div className="space-y-2">
            {device.addresses.map((address) => {
              const key = address.id || address.address;
              const result = reachabilityByKey.get(key) ?? reachabilityByKey.get(address.address);
              const status = reachabilityStatus(result);
              const addressId = address.id || address.address;
              return (
                <div
                  key={key}
                  className="rounded-lg border border-outline-subtle bg-elevated px-3 py-2"
                >
                  <div className="flex items-start justify-between gap-3">
                    <div className="min-w-0">
                      <p className="break-all font-mono text-xs text-on-bg">{address.address}</p>
                      <p className="mt-1 text-[11px] uppercase text-on-bg-secondary">
                        {address.label || address.role || 'address'}
                      </p>
                    </div>
                    <span className={`shrink-0 text-xs font-medium ${status.className}`}>
                      {status.label}
                    </span>
                  </div>
                  <div className="mt-2 flex items-center justify-between gap-3 text-xs">
                    <span className="text-on-bg-secondary">Ports</span>
                    <span className="font-mono text-on-bg">
                      {formatProbePorts(result?.probe_ports ?? address.probe_ports)}
                    </span>
                  </div>
                  {portReachabilityRows(result).length > 0 && (
                    <div className="mt-2 space-y-1 text-xs">{portRowsForAddress(result)}</div>
                  )}
                  {result?.error && (
                    <p className="mt-2 break-words text-xs text-status-down">{result.error}</p>
                  )}
                  {!address.is_primary && onPromoteAddress && address.id && (
                    <button
                      type="button"
                      aria-label={`Use ${address.address} as primary`}
                      onClick={() => void handlePromoteAddress(address.id)}
                      disabled={promotingAddressId === addressId}
                      className="mt-2 rounded-md border border-outline-subtle px-2 py-1 text-xs text-on-bg-secondary transition-colors hover:bg-surface-high disabled:cursor-not-allowed disabled:opacity-60"
                    >
                      Use as primary
                    </button>
                  )}
                </div>
              );
            })}
          </div>
        </div>
      )}

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
              label="Metric health"
              value={detailMetrics.health}
              valueClassName={metricHealthClass(detailMetrics.health)}
            />
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
