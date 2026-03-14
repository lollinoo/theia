type WSMessageType = 'snapshot' | 'metrics' | 'link_metrics' | 'alert' | 'prometheus_status';
type APIRecord = Record<string, unknown>;

export interface DeviceMetricsDTO {
  device_id: string;
  cpu_percent: number | null;
  mem_percent: number | null;
  temp_celsius: number | null;
  uptime_secs: number | null;
  collected_at: string;
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
  alerts: AlertDTO[];
}

export interface PrometheusStatusPayload {
  available: boolean;
  error?: string;
}

export interface WSMessage {
  type: WSMessageType;
  payload: unknown;
}

export interface SnapshotWSMessage extends Omit<WSMessage, 'type' | 'payload'> {
  type: 'snapshot';
  payload: SnapshotPayload;
}

export interface PrometheusStatusWSMessage extends Omit<WSMessage, 'type' | 'payload'> {
  type: 'prometheus_status';
  payload: PrometheusStatusPayload;
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

export function parseDeviceMetrics(value: unknown): DeviceMetricsDTO {
  if (!isRecord(value)) {
    throw new Error('invalid device metrics payload');
  }

  return {
    device_id: readString(value, 'device_id'),
    cpu_percent: readNullableNumber(value, 'cpu_percent'),
    mem_percent: readNullableNumber(value, 'mem_percent'),
    temp_celsius: readNullableNumber(value, 'temp_celsius'),
    uptime_secs: readNullableNumber(value, 'uptime_secs'),
    collected_at: readString(value, 'collected_at'),
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
  const alerts = Array.isArray(value.alerts) ? value.alerts : [];

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
    alerts: alerts.map(parseAlert),
  };
}

export function parseWSMessage(value: unknown): WSMessage | SnapshotWSMessage | PrometheusStatusWSMessage {
  if (!isRecord(value)) {
    throw new Error('invalid websocket message');
  }

  const type = readString(value, 'type');
  if (
    type !== 'snapshot' &&
    type !== 'metrics' &&
    type !== 'link_metrics' &&
    type !== 'alert' &&
    type !== 'prometheus_status'
  ) {
    throw new Error(`unsupported websocket message type: ${type}`);
  }

  if (type === 'snapshot') {
    return {
      type,
      payload: parseSnapshotPayload(value.payload),
    };
  }

  if (type === 'prometheus_status') {
    const p = isRecord(value.payload) ? value.payload : {};
    return {
      type,
      payload: {
        available: p.available === true,
        error: typeof p.error === 'string' ? p.error : undefined,
      },
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
    return 'text-yellow-400';
  }
  return 'text-status-up';
}

export function utilizationColor(value: number): string {
  if (value > 0.8) {
    return '#ff1744';
  }
  if (value >= 0.5) {
    return '#ffc107';
  }
  return '#00c853';
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
