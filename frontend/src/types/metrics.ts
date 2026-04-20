export type WSMessageType =
  'snapshot'
  | 'snapshot_delta'
  | 'metrics'
  | 'link_metrics'
  | 'alert'
  | 'prometheus_status'
  | 'resync_required'
  | 'topology_changed';
type APIRecord = Record<string, unknown>;

export interface DeviceMetricsDTO {
  device_id: string;
  cpu_percent: number | null;
  mem_percent: number | null;
  collected_at: string;
  health?: string;
  reachability?: string;
  stale?: boolean;
  temp_celsius?: number | null;
  uptime_secs?: number | null;
  last_polled_at?: string;
  expected_poll_interval_seconds?: number | null;
}

export interface LinkMetricsDTO {
  device_id: string;
  if_name: string;
  tx_bps: number | null;
  rx_bps: number | null;
  utilization: number | null;
  collected_at: string;
}

export interface AlertDTO {
  device_id: string;
  severity: string;
  alert_name: string;
  state: string;
  summary: string;
}

export type AlertStatus = 'normal' | 'degraded' | 'down';

export interface SnapshotPayload {
  device_metrics: Record<string, DeviceMetricsDTO>;
  link_metrics: Record<string, LinkMetricsDTO[]>;
  device_statuses: Record<string, string>;
}

export interface PrometheusStatusPayload {
  enabled?: boolean;
  available: boolean;
  error?: string;
}

export interface ResyncRequiredPayload {
  scope: 'overview';
  reason: 'client_resync_scheduled' | 'state_changes_dropped' | 'hub_buffer_full';
}

export interface SnapshotEnvelopePayload {
  version: number | null;
  snapshot: SnapshotPayload;
}

export interface SnapshotDeltaEnvelopePayload {
  base_version?: number;
  version?: number;
  delta: SnapshotPayload;
}

export interface WSMessage {
  type: WSMessageType;
  payload: unknown;
}

export interface SnapshotWSMessage extends Omit<WSMessage, 'type' | 'payload'> {
  type: 'snapshot';
  payload: SnapshotEnvelopePayload;
}

export interface SnapshotDeltaWSMessage extends Omit<WSMessage, 'type' | 'payload'> {
  type: 'snapshot_delta';
  payload: SnapshotDeltaEnvelopePayload;
}

export interface PrometheusStatusWSMessage extends Omit<WSMessage, 'type' | 'payload'> {
  type: 'prometheus_status';
  payload: PrometheusStatusPayload;
}

export interface ResyncRequiredWSMessage extends Omit<WSMessage, 'type' | 'payload'> {
  type: 'resync_required';
  payload: ResyncRequiredPayload;
}

export interface AlertWSMessage extends Omit<WSMessage, 'type' | 'payload'> {
  type: 'alert';
  payload: AlertEnvelopePayload;
}

export interface AlertEnvelopePayload {
  version?: number;
  alerts: AlertDTO[];
}

function isRecord(value: unknown): value is APIRecord {
  return typeof value === 'object' && value !== null && !Array.isArray(value);
}

function readString(record: APIRecord, key: string, fallback = ''): string {
  const value = record[key];
  return typeof value === 'string' ? value : fallback;
}

function readNullableNumber(record: APIRecord, key: string): number | null {
  const value = record[key];
  if (value === null || value === undefined) {
    return null;
  }
  return typeof value === 'number' && Number.isFinite(value) ? value : null;
}

function readOptionalString(record: APIRecord, key: string): string | undefined {
  const value = record[key];
  return typeof value === 'string' ? value : undefined;
}

function readOptionalBoolean(record: APIRecord, key: string): boolean | undefined {
  const value = record[key];
  return typeof value === 'boolean' ? value : undefined;
}

export function parseDeviceMetrics(value: unknown): DeviceMetricsDTO {
  if (!isRecord(value)) {
    throw new Error('invalid device metrics payload');
  }

  return {
    device_id: readString(value, 'device_id'),
    cpu_percent: readNullableNumber(value, 'cpu_percent'),
    mem_percent: readNullableNumber(value, 'mem_percent'),
    collected_at: readString(value, 'collected_at'),
    temp_celsius: readNullableNumber(value, 'temp_celsius') ?? undefined,
    uptime_secs: readNullableNumber(value, 'uptime_secs') ?? undefined,
    last_polled_at: readOptionalString(value, 'last_polled_at'),
    expected_poll_interval_seconds: readNullableNumber(value, 'expected_poll_interval_seconds') ?? undefined,
    health: readOptionalString(value, 'health'),
    reachability: readOptionalString(value, 'reachability'),
    stale: readOptionalBoolean(value, 'stale'),
  };
}

export function parseLinkMetrics(value: unknown): LinkMetricsDTO {
  if (!isRecord(value)) {
    throw new Error('invalid link metrics payload');
  }

  return {
    device_id: readString(value, 'device_id'),
    if_name: readString(value, 'if_name'),
    tx_bps: readNullableNumber(value, 'tx_bps'),
    rx_bps: readNullableNumber(value, 'rx_bps'),
    utilization: readNullableNumber(value, 'utilization'),
    collected_at: readString(value, 'collected_at'),
  };
}

export function parseAlert(value: unknown): AlertDTO {
  if (!isRecord(value)) {
    throw new Error('invalid alert payload');
  }

  return {
    device_id: readString(value, 'device_id'),
    severity: readString(value, 'severity'),
    alert_name: readString(value, 'alert_name'),
    state: readString(value, 'state'),
    summary: readString(value, 'summary'),
  };
}

export function parseSnapshotPayload(value: unknown): SnapshotPayload {
  if (!isRecord(value)) {
    throw new Error('invalid snapshot payload');
  }

  const deviceMetrics = isRecord(value.device_metrics) ? value.device_metrics : {};
  const linkMetrics = isRecord(value.link_metrics) ? value.link_metrics : {};
  const deviceStatuses = isRecord(value.device_statuses) ? value.device_statuses : {};

  return {
    device_metrics: Object.fromEntries(
      Object.entries(deviceMetrics).map(([deviceId, metrics]) => [
        deviceId,
        parseDeviceMetrics(metrics),
      ]),
    ),
    link_metrics: Object.fromEntries(
      Object.entries(linkMetrics).map(([deviceId, metrics]) => [
        deviceId,
        Array.isArray(metrics) ? metrics.map(parseLinkMetrics) : [],
      ]),
    ),
    device_statuses: Object.fromEntries(
      Object.entries(deviceStatuses)
        .filter(([, v]) => typeof v === 'string')
        .map(([k, v]) => [k, v as string]),
    ),
  };
}

/**
 * Deep-merges a sparse delta payload into an existing snapshot.
 * Only entries present in the delta overwrite existing entries.
 */
export function mergeSnapshotDelta(
  existing: SnapshotPayload,
  delta: SnapshotPayload,
): SnapshotPayload {
  const deviceMetrics = { ...existing.device_metrics };

  for (const [deviceId, nextMetrics] of Object.entries(delta.device_metrics)) {
    const previousMetrics = deviceMetrics[deviceId];
    if (!previousMetrics) {
      deviceMetrics[deviceId] = nextMetrics;
      continue;
    }

    const mergedMetrics = { ...previousMetrics, ...nextMetrics };

    if (
      nextMetrics.last_polled_at === undefined
      && nextMetrics.collected_at
      && nextMetrics.collected_at !== previousMetrics.collected_at
    ) {
      delete mergedMetrics.last_polled_at;
    }

    deviceMetrics[deviceId] = mergedMetrics;
  }

  return {
    device_metrics: deviceMetrics,
    link_metrics: { ...existing.link_metrics, ...delta.link_metrics },
    device_statuses: { ...existing.device_statuses, ...delta.device_statuses },
  };
}

export function parseWSMessage(
  value: unknown,
): WSMessage | SnapshotWSMessage | SnapshotDeltaWSMessage | PrometheusStatusWSMessage | ResyncRequiredWSMessage | AlertWSMessage {
  if (!isRecord(value)) {
    throw new Error('invalid websocket message');
  }

  const type = readString(value, 'type');
  if (
    type !== 'snapshot' &&
    type !== 'snapshot_delta' &&
    type !== 'metrics' &&
    type !== 'link_metrics' &&
    type !== 'alert' &&
    type !== 'prometheus_status' &&
    type !== 'resync_required' &&
    type !== 'topology_changed'
  ) {
    throw new Error(`unsupported websocket message type: ${type}`);
  }

  if (type === 'snapshot') {
    const payload = isRecord(value.payload) ? value.payload : {};
    if ('version' in payload || 'snapshot' in payload) {
      const version = typeof payload.version === 'number' && Number.isFinite(payload.version)
        ? payload.version
        : null;
      return {
        type,
        payload: {
          version,
          snapshot: parseSnapshotPayload(payload.snapshot),
        },
      };
    }
    return {
      type,
      payload: {
        version: null,
        snapshot: parseSnapshotPayload(value.payload),
      },
    };
  }

  if (type === 'snapshot_delta') {
    const payload = isRecord(value.payload) ? value.payload : {};
    if ('delta' in payload || 'version' in payload || 'base_version' in payload) {
      const baseVersion = typeof payload.base_version === 'number' && Number.isFinite(payload.base_version)
        ? payload.base_version
        : undefined;
      const version = typeof payload.version === 'number' && Number.isFinite(payload.version)
        ? payload.version
        : undefined;
      return {
        type,
        payload: {
          base_version: baseVersion,
          version,
          delta: parseSnapshotPayload(payload.delta),
        },
      } as SnapshotDeltaWSMessage;
    }
    return {
      type,
      payload: {
        delta: parseSnapshotPayload(value.payload),
      },
    } as SnapshotDeltaWSMessage;
  }

  if (type === 'prometheus_status') {
    const p = isRecord(value.payload) ? value.payload : {};
    return {
      type,
      payload: {
        enabled: typeof p.enabled === 'boolean' ? p.enabled : undefined,
        available: p.available === true,
        error: typeof p.error === 'string' ? p.error : undefined,
      },
    };
  }

  if (type === 'alert') {
    const payload = value.payload;
    const alerts = Array.isArray(payload)
      ? payload.map(parseAlert)
      : isRecord(payload) && Array.isArray(payload.alerts)
        ? payload.alerts.map(parseAlert)
        : isRecord(payload)
          ? [parseAlert(payload)]
          : [];
    const version = isRecord(payload) && typeof payload.version === 'number' && Number.isFinite(payload.version)
      ? payload.version
      : undefined;

    return {
      type,
      payload: {
        version,
        alerts,
      },
    } as AlertWSMessage;
  }

  if (type === 'resync_required') {
    const payload = isRecord(value.payload) ? value.payload : {};
    const scope = readString(payload, 'scope');
    const reason = readString(payload, 'reason');

    if (scope !== 'overview') {
      throw new Error(`unsupported resync scope: ${scope}`);
    }
    if (reason !== 'client_resync_scheduled' && reason !== 'state_changes_dropped' && reason !== 'hub_buffer_full') {
      throw new Error(`unsupported resync reason: ${reason}`);
    }

    return {
      type,
      payload: {
        scope,
        reason,
      },
    } as ResyncRequiredWSMessage;
  }

  if (type === 'topology_changed') {
    return {
      type,
      payload: null,
    };
  }

  return {
    type,
    payload: value.payload,
  };
}

export function formatUptime(secs: number): string {
  const totalSeconds = Math.max(0, Math.floor(secs));
  const days = Math.floor(totalSeconds / 86_400);
  const hours = Math.floor((totalSeconds % 86_400) / 3_600);
  const minutes = Math.floor((totalSeconds % 3_600) / 60);

  if (days > 0) {
    return hours > 0 ? `${days}d ${hours}h` : `${days}d`;
  }

  if (hours > 0) {
    return minutes > 0 ? `${hours}h ${minutes}m` : `${hours}h`;
  }

  if (minutes > 0) {
    return `${minutes}m`;
  }

  return `${totalSeconds}s`;
}

export function metricColor(value: number): string {
  if (value > 85) {
    return 'text-status-down';
  }
  if (value >= 60) {
    return 'text-warning';
  }
  return 'text-status-up';
}

export function utilizationColor(value: number): string {
  if (value > 0.8) {
    return 'var(--color-status-down)';
  }
  if (value >= 0.5) {
    return 'var(--color-status-probing)';
  }
  return 'var(--color-status-up)';
}

export function alertStatusForDevice(deviceId: string, alerts: AlertDTO[]): AlertStatus {
  const activeAlerts = alerts.filter(
    (alert) => alert.device_id === deviceId && alert.state === 'firing',
  );

  if (activeAlerts.some((alert) => alert.severity === 'critical')) {
    return 'down';
  }

  if (activeAlerts.some((alert) => alert.severity === 'warning')) {
    return 'degraded';
  }

  return 'normal';
}

export function isPrometheusUnavailable(status: PrometheusStatusPayload | null): boolean {
  return status !== null && status.enabled !== false && !status.available;
}

export function formatThroughput(bps: number): string {
  if (bps <= 0) {
    return '0 bps';
  }

  if (bps >= 1_000_000_000) {
    return `${(bps / 1_000_000_000).toFixed(1)} Gbps`;
  }

  if (bps >= 1_000_000) {
    return `${(bps / 1_000_000).toFixed(1)} Mbps`;
  }

  if (bps >= 1_000) {
    return `${Math.round(bps / 1_000)} Kbps`;
  }

  return `${Math.round(bps)} bps`;
}
