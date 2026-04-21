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

export type RuntimeReason =
  | 'ok'
  | 'awaiting_poll'
  | 'stale'
  | 'device_unreachable'
  | 'upstream_unavailable'
  | 'no_data'
  | 'unmonitored'
  | 'unsupported';

export type OperationalStatus = 'up' | 'down' | 'probing' | 'unknown' | 'unmonitored';
export type ReachabilityStatus = 'up' | 'soft_down' | 'hard_down' | 'unknown' | 'unmonitored';
export type HealthStatus = 'healthy' | 'warning' | 'critical' | 'unknown';
export type FreshnessStatus = 'fresh' | 'stale' | 'awaiting_poll' | 'unmonitored';
export type MetricsStatus = 'available' | 'partial' | 'unavailable' | 'unmonitored';
export type LinkMetricsStatus = 'available' | 'partial' | 'unavailable';

const runtimeReasons = [
  'ok',
  'awaiting_poll',
  'stale',
  'device_unreachable',
  'upstream_unavailable',
  'no_data',
  'unmonitored',
  'unsupported',
] as const;

const operationalStatuses = ['up', 'down', 'probing', 'unknown', 'unmonitored'] as const;
const reachabilityStatuses = ['up', 'soft_down', 'hard_down', 'unknown', 'unmonitored'] as const;
const healthStatuses = ['healthy', 'warning', 'critical', 'unknown'] as const;
const freshnessStatuses = ['fresh', 'stale', 'awaiting_poll', 'unmonitored'] as const;
const metricsStatuses = ['available', 'partial', 'unavailable', 'unmonitored'] as const;
const linkMetricsStatuses = ['available', 'partial', 'unavailable'] as const;
const alertStatuses = ['normal', 'degraded', 'down'] as const;

export interface DeviceRuntimeDTO {
  device_id: string;
  operational_status: OperationalStatus;
  reachability: ReachabilityStatus;
  health: HealthStatus;
  freshness: FreshnessStatus;
  primary_reason: RuntimeReason;
  metrics_status: MetricsStatus;
  metrics_reason: RuntimeReason;
  alert_status: AlertStatus;
  firing_alert_count: number;
  last_collected_at: string | null;
  last_polled_at: string | null;
  expected_poll_interval_seconds: number | null;
  cpu_percent: number | null;
  mem_percent: number | null;
  temp_celsius: number | null;
  uptime_secs: number | null;
}

export interface LinkRuntimeDTO {
  link_id: string;
  source_device_id: string;
  target_device_id: string;
  source_if_name: string;
  target_if_name: string;
  metrics_status: LinkMetricsStatus;
  metrics_reason: RuntimeReason;
  last_collected_at: string | null;
  tx_bps: number | null;
  rx_bps: number | null;
  utilization: number | null;
}

export type DeviceMetricsDTO = DeviceRuntimeDTO;
export type LinkMetricsDTO = LinkRuntimeDTO;

export interface AlertDTO {
  device_id: string;
  severity: string;
  alert_name: string;
  state: string;
  summary: string;
}

export type AlertStatus = 'normal' | 'degraded' | 'down';

export interface SnapshotPayload {
  devices: Record<string, DeviceRuntimeDTO>;
  links: Record<string, LinkRuntimeDTO>;
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

function readRequiredString(record: APIRecord, key: string): string {
  const value = record[key];
  if (typeof value === 'string' && value.length > 0) {
    return value;
  }
  throw new Error(`invalid required field: ${key}`);
}

function readRequiredNullableString(record: APIRecord, key: string): string | null {
  if (!(key in record)) {
    throw new Error(`invalid required field: ${key}`);
  }

  const value = record[key];
  if (value === null) {
    return null;
  }

  if (typeof value === 'string') {
    return value;
  }

  throw new Error(`invalid required field: ${key}`);
}

function readRequiredNullableNumber(record: APIRecord, key: string): number | null {
  if (!(key in record)) {
    throw new Error(`invalid required field: ${key}`);
  }

  const value = record[key];
  if (value === null) {
    return null;
  }

  if (typeof value === 'number' && Number.isFinite(value)) {
    return value;
  }

  throw new Error(`invalid required field: ${key}`);
}

function readRequiredEnum<T extends string>(
  record: APIRecord,
  key: string,
  allowedValues: readonly T[],
): T {
  const value = record[key];
  if (typeof value === 'string' && allowedValues.includes(value as T)) {
    return value as T;
  }
  throw new Error(`invalid required field: ${key}`);
}

function readRequiredCount(record: APIRecord, key: string): number {
  const value = record[key];
  if (typeof value === 'number' && Number.isFinite(value) && value >= 0) {
    return Math.trunc(value);
  }
  throw new Error(`invalid required field: ${key}`);
}

export function parseDeviceRuntime(value: unknown): DeviceRuntimeDTO {
  if (!isRecord(value)) {
    throw new Error('invalid device runtime payload');
  }

  try {
    return {
      device_id: readRequiredString(value, 'device_id'),
      operational_status: readRequiredEnum(value, 'operational_status', operationalStatuses),
      reachability: readRequiredEnum(value, 'reachability', reachabilityStatuses),
      health: readRequiredEnum(value, 'health', healthStatuses),
      freshness: readRequiredEnum(value, 'freshness', freshnessStatuses),
      primary_reason: readRequiredEnum(value, 'primary_reason', runtimeReasons),
      metrics_status: readRequiredEnum(value, 'metrics_status', metricsStatuses),
      metrics_reason: readRequiredEnum(value, 'metrics_reason', runtimeReasons),
      alert_status: readRequiredEnum(value, 'alert_status', alertStatuses),
      firing_alert_count: readRequiredCount(value, 'firing_alert_count'),
      last_collected_at: readRequiredNullableString(value, 'last_collected_at'),
      last_polled_at: readRequiredNullableString(value, 'last_polled_at'),
      expected_poll_interval_seconds: readRequiredNullableNumber(value, 'expected_poll_interval_seconds'),
      cpu_percent: readRequiredNullableNumber(value, 'cpu_percent'),
      mem_percent: readRequiredNullableNumber(value, 'mem_percent'),
      temp_celsius: readRequiredNullableNumber(value, 'temp_celsius'),
      uptime_secs: readRequiredNullableNumber(value, 'uptime_secs'),
    };
  } catch {
    throw new Error('invalid device runtime payload');
  }
}

export const parseDeviceMetrics = parseDeviceRuntime;

export function parseLinkRuntime(value: unknown): LinkRuntimeDTO {
  if (!isRecord(value)) {
    throw new Error('invalid link runtime payload');
  }

  try {
    return {
      link_id: readRequiredString(value, 'link_id'),
      source_device_id: readRequiredString(value, 'source_device_id'),
      target_device_id: readRequiredString(value, 'target_device_id'),
      source_if_name: readRequiredString(value, 'source_if_name'),
      target_if_name: readRequiredString(value, 'target_if_name'),
      metrics_status: readRequiredEnum(value, 'metrics_status', linkMetricsStatuses),
      metrics_reason: readRequiredEnum(value, 'metrics_reason', runtimeReasons),
      last_collected_at: readRequiredNullableString(value, 'last_collected_at'),
      tx_bps: readRequiredNullableNumber(value, 'tx_bps'),
      rx_bps: readRequiredNullableNumber(value, 'rx_bps'),
      utilization: readRequiredNullableNumber(value, 'utilization'),
    };
  } catch {
    throw new Error('invalid link runtime payload');
  }
}

export const parseLinkMetrics = parseLinkRuntime;

export function parseAlert(value: unknown): AlertDTO {
  if (!isRecord(value)) {
    throw new Error('invalid alert payload');
  }

  try {
    return {
      device_id: readRequiredString(value, 'device_id'),
      severity: readRequiredString(value, 'severity'),
      alert_name: readRequiredString(value, 'alert_name'),
      state: readRequiredString(value, 'state'),
      summary: readRequiredString(value, 'summary'),
    };
  } catch {
    throw new Error('invalid alert payload');
  }
}

export function parseSnapshotPayload(value: unknown): SnapshotPayload {
  if (!isRecord(value)) {
    throw new Error('invalid snapshot payload');
  }

   if (!('devices' in value) || !isRecord(value.devices)) {
     throw new Error('invalid snapshot payload');
   }

   if (!('links' in value) || !isRecord(value.links)) {
     throw new Error('invalid snapshot payload');
   }

   const devices = Object.fromEntries(
     Object.entries(value.devices).map(([deviceId, runtime]) => {
       const parsedRuntime = parseDeviceRuntime(runtime);
       if (parsedRuntime.device_id !== deviceId) {
         throw new Error('invalid snapshot payload');
       }

       return [deviceId, parsedRuntime] as const;
     }),
   );

   const links = Object.fromEntries(
     Object.entries(value.links).map(([linkId, runtime]) => {
       const parsedRuntime = parseLinkRuntime(runtime);
       if (parsedRuntime.link_id !== linkId) {
         throw new Error('invalid snapshot payload');
       }

       return [linkId, parsedRuntime] as const;
     }),
   );

   return {
     devices,
     links,
   };
}

/**
 * Replaces atomic device/link records only for keys present in the delta.
 */
export function mergeSnapshotDelta(
  existing: SnapshotPayload,
  delta: SnapshotPayload,
): SnapshotPayload {
  return {
    devices: { ...existing.devices, ...delta.devices },
    links: { ...existing.links, ...delta.links },
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
    let alerts: AlertDTO[];
    let version: number | undefined;

    if (Array.isArray(payload)) {
      alerts = payload.map(parseAlert);
    } else if (isRecord(payload) && Array.isArray(payload.alerts)) {
      alerts = payload.alerts.map(parseAlert);
      version = typeof payload.version === 'number' && Number.isFinite(payload.version)
        ? payload.version
        : undefined;
    } else {
      throw new Error('invalid alert payload');
    }

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
