export type WSMessageType =
  | 'ready'
  | 'snapshot'
  | 'snapshot_delta'
  | 'runtime_delta'
  | 'topology_delta'
  | 'metrics'
  | 'link_metrics'
  | 'alert'
  | 'prometheus_status'
  | 'polling_health_changed'
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
export type PrimaryHealth =
  | 'probing'
  | 'up_fresh'
  | 'up_stale'
  | 'snmp_degraded'
  | 'unreachable'
  | 'quarantined';
export type RuntimeFlag =
  | 'deadline_missed'
  | 'overloaded'
  | 'background_pending'
  | 'partial_telemetry'
  | 'degraded_risk'
  | 'persistence_lagging';
export type FieldState = 'ok' | 'missing' | 'error' | 'stale';
export type ReachabilityEvidence = 'true' | 'false' | 'unknown';

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
const primaryHealthStates = [
  'probing',
  'up_fresh',
  'up_stale',
  'snmp_degraded',
  'unreachable',
  'quarantined',
] as const;
const runtimeFlags = [
  'deadline_missed',
  'overloaded',
  'background_pending',
  'partial_telemetry',
  'degraded_risk',
  'persistence_lagging',
] as const;
const fieldStates = ['ok', 'missing', 'error', 'stale'] as const;
const reachabilityEvidenceStates = ['true', 'false', 'unknown'] as const;

export interface DeviceRuntimeDTO {
  device_id: string;
  operational_status: OperationalStatus;
  primary_health: PrimaryHealth;
  runtime_flags: RuntimeFlag[];
  field_states: Record<'uptime' | 'cpu' | 'memory', FieldState>;
  network_reachable: ReachabilityEvidence;
  snmp_reachable: ReachabilityEvidence;
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

export type DeviceRuntimePatch = Partial<DeviceRuntimeDTO> & Pick<DeviceRuntimeDTO, 'device_id'>;
export type LinkRuntimePatch = Partial<LinkRuntimeDTO> & Pick<LinkRuntimeDTO, 'link_id'>;

export interface RuntimePatchPayload {
  devices: Record<string, DeviceRuntimePatch>;
  links: Record<string, LinkRuntimePatch>;
}

export interface PrometheusStatusPayload {
  enabled?: boolean;
  available: boolean;
  error?: string;
}

export interface ResyncRequiredPayload {
  scope: 'overview';
  reason:
    | 'client_resync_scheduled'
    | 'client_missing_runtime_snapshot'
    | 'state_changes_dropped'
    | 'hub_buffer_full';
}

export interface TopologyChangedPayload {
  topology_version?: number | string;
  reason?: string;
  recommended_endpoint?: string;
}

export interface ReadyPayload {
  runtime_version?: number;
  runtime_identity?: string;
  alert_version?: number;
}

export interface SnapshotEnvelopePayload {
  version: number | null;
  runtime_identity?: string;
  snapshot: SnapshotPayload;
}

export interface SnapshotDeltaEnvelopePayload {
  base_version?: number;
  version?: number;
  runtime_identity?: string;
  delta: SnapshotPayload;
}

export interface RuntimeDeltaEnvelopePayload {
  base_version?: number;
  version?: number;
  runtime_identity?: string;
  delta: RuntimePatchPayload;
}

export interface PollingHealthPayload {
  essential_overloaded: boolean;
  degraded_risk: boolean;
  essential_queue_lag_seconds: number;
  deadline_miss_total: number;
  active_workers: number;
  configured_workers: number;
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

export interface RuntimeDeltaWSMessage extends Omit<WSMessage, 'type' | 'payload'> {
  type: 'runtime_delta';
  payload: RuntimeDeltaEnvelopePayload;
}

export interface PollingHealthChangedWSMessage extends Omit<WSMessage, 'type' | 'payload'> {
  type: 'polling_health_changed';
  payload: PollingHealthPayload;
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

export interface TopologyChangedWSMessage extends Omit<WSMessage, 'type' | 'payload'> {
  type: 'topology_changed';
  payload: TopologyChangedPayload;
}

export interface ReadyWSMessage extends Omit<WSMessage, 'type' | 'payload'> {
  type: 'ready';
  payload: ReadyPayload;
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

function readRequiredString(record: APIRecord, key: string, allowEmpty = false): string {
  const value = record[key];
  if (typeof value === 'string' && (allowEmpty || value.length > 0)) {
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

function readRuntimeFlags(record: APIRecord, key: string): RuntimeFlag[] {
  const value = record[key];
  if (!Array.isArray(value)) {
    throw new Error(`invalid required field: ${key}`);
  }

  return value.map((entry) => {
    if (typeof entry === 'string' && runtimeFlags.includes(entry as RuntimeFlag)) {
      return entry as RuntimeFlag;
    }
    throw new Error(`invalid required field: ${key}`);
  });
}

function readRuntimeFlagsPatch(record: APIRecord, key: string): RuntimeFlag[] {
  if (record[key] === null) {
    return [];
  }
  return readRuntimeFlags(record, key);
}

function readFieldStates(
  record: APIRecord,
  key: string,
): Record<'uptime' | 'cpu' | 'memory', FieldState> {
  const value = record[key];
  if (!isRecord(value)) {
    throw new Error(`invalid required field: ${key}`);
  }

  return {
    uptime: readRequiredEnum(value, 'uptime', fieldStates),
    cpu: readRequiredEnum(value, 'cpu', fieldStates),
    memory: readRequiredEnum(value, 'memory', fieldStates),
  };
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
      primary_health: readRequiredEnum(value, 'primary_health', primaryHealthStates),
      runtime_flags: readRuntimeFlags(value, 'runtime_flags'),
      field_states: readFieldStates(value, 'field_states'),
      network_reachable: readRequiredEnum(value, 'network_reachable', reachabilityEvidenceStates),
      snmp_reachable: readRequiredEnum(value, 'snmp_reachable', reachabilityEvidenceStates),
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
      expected_poll_interval_seconds: readRequiredNullableNumber(
        value,
        'expected_poll_interval_seconds',
      ),
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
      source_if_name: readRequiredString(value, 'source_if_name', true),
      target_if_name: readRequiredString(value, 'target_if_name', true),
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
      summary: readRequiredString(value, 'summary', true),
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

function parseDeviceRuntimePatch(value: unknown): DeviceRuntimePatch {
  if (!isRecord(value)) {
    throw new Error('invalid device runtime patch payload');
  }

  try {
    const patch: DeviceRuntimePatch = {
      device_id: readRequiredString(value, 'device_id'),
    };

    if ('operational_status' in value) {
      patch.operational_status = readRequiredEnum(value, 'operational_status', operationalStatuses);
    }
    if ('primary_health' in value) {
      patch.primary_health = readRequiredEnum(value, 'primary_health', primaryHealthStates);
    }
    if ('runtime_flags' in value) {
      patch.runtime_flags = readRuntimeFlagsPatch(value, 'runtime_flags');
    }
    if ('field_states' in value) {
      patch.field_states = readFieldStates(value, 'field_states');
    }
    if ('network_reachable' in value) {
      patch.network_reachable = readRequiredEnum(
        value,
        'network_reachable',
        reachabilityEvidenceStates,
      );
    }
    if ('snmp_reachable' in value) {
      patch.snmp_reachable = readRequiredEnum(value, 'snmp_reachable', reachabilityEvidenceStates);
    }
    if ('reachability' in value) {
      patch.reachability = readRequiredEnum(value, 'reachability', reachabilityStatuses);
    }
    if ('health' in value) {
      patch.health = readRequiredEnum(value, 'health', healthStatuses);
    }
    if ('freshness' in value) {
      patch.freshness = readRequiredEnum(value, 'freshness', freshnessStatuses);
    }
    if ('primary_reason' in value) {
      patch.primary_reason = readRequiredEnum(value, 'primary_reason', runtimeReasons);
    }
    if ('metrics_status' in value) {
      patch.metrics_status = readRequiredEnum(value, 'metrics_status', metricsStatuses);
    }
    if ('metrics_reason' in value) {
      patch.metrics_reason = readRequiredEnum(value, 'metrics_reason', runtimeReasons);
    }
    if ('alert_status' in value) {
      patch.alert_status = readRequiredEnum(value, 'alert_status', alertStatuses);
    }
    if ('firing_alert_count' in value) {
      patch.firing_alert_count = readRequiredCount(value, 'firing_alert_count');
    }
    if ('last_collected_at' in value) {
      patch.last_collected_at = readRequiredNullableString(value, 'last_collected_at');
    }
    if ('last_polled_at' in value) {
      patch.last_polled_at = readRequiredNullableString(value, 'last_polled_at');
    }
    if ('expected_poll_interval_seconds' in value) {
      patch.expected_poll_interval_seconds = readRequiredNullableNumber(
        value,
        'expected_poll_interval_seconds',
      );
    }
    if ('cpu_percent' in value) {
      patch.cpu_percent = readRequiredNullableNumber(value, 'cpu_percent');
    }
    if ('mem_percent' in value) {
      patch.mem_percent = readRequiredNullableNumber(value, 'mem_percent');
    }
    if ('temp_celsius' in value) {
      patch.temp_celsius = readRequiredNullableNumber(value, 'temp_celsius');
    }
    if ('uptime_secs' in value) {
      patch.uptime_secs = readRequiredNullableNumber(value, 'uptime_secs');
    }

    return patch;
  } catch {
    throw new Error('invalid device runtime patch payload');
  }
}

function parseLinkRuntimePatch(value: unknown): LinkRuntimePatch {
  if (!isRecord(value)) {
    throw new Error('invalid link runtime patch payload');
  }

  try {
    const patch: LinkRuntimePatch = {
      link_id: readRequiredString(value, 'link_id'),
    };

    if ('source_device_id' in value) {
      patch.source_device_id = readRequiredString(value, 'source_device_id');
    }
    if ('target_device_id' in value) {
      patch.target_device_id = readRequiredString(value, 'target_device_id');
    }
    if ('source_if_name' in value) {
      patch.source_if_name = readRequiredString(value, 'source_if_name', true);
    }
    if ('target_if_name' in value) {
      patch.target_if_name = readRequiredString(value, 'target_if_name', true);
    }
    if ('metrics_status' in value) {
      patch.metrics_status = readRequiredEnum(value, 'metrics_status', linkMetricsStatuses);
    }
    if ('metrics_reason' in value) {
      patch.metrics_reason = readRequiredEnum(value, 'metrics_reason', runtimeReasons);
    }
    if ('last_collected_at' in value) {
      patch.last_collected_at = readRequiredNullableString(value, 'last_collected_at');
    }
    if ('tx_bps' in value) {
      patch.tx_bps = readRequiredNullableNumber(value, 'tx_bps');
    }
    if ('rx_bps' in value) {
      patch.rx_bps = readRequiredNullableNumber(value, 'rx_bps');
    }
    if ('utilization' in value) {
      patch.utilization = readRequiredNullableNumber(value, 'utilization');
    }

    return patch;
  } catch {
    throw new Error('invalid link runtime patch payload');
  }
}

export function parseRuntimePatchPayload(value: unknown): RuntimePatchPayload {
  if (!isRecord(value)) {
    throw new Error('invalid runtime patch payload');
  }

  if (!('devices' in value) || !isRecord(value.devices)) {
    throw new Error('invalid runtime patch payload');
  }

  if (!('links' in value) || !isRecord(value.links)) {
    throw new Error('invalid runtime patch payload');
  }

  const devices = Object.fromEntries(
    Object.entries(value.devices).map(([deviceId, runtime]) => {
      const parsedRuntime = parseDeviceRuntimePatch(runtime);
      if (parsedRuntime.device_id !== deviceId) {
        throw new Error('invalid runtime patch payload');
      }

      return [deviceId, parsedRuntime] as const;
    }),
  );

  const links = Object.fromEntries(
    Object.entries(value.links).map(([linkId, runtime]) => {
      const parsedRuntime = parseLinkRuntimePatch(runtime);
      if (parsedRuntime.link_id !== linkId) {
        throw new Error('invalid runtime patch payload');
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

export function mergeRuntimeDeltaPatch(
  existing: SnapshotPayload,
  delta: RuntimePatchPayload,
): SnapshotPayload {
  let devices = existing.devices;
  let links = existing.links;
  let changed = false;

  function mergeRecord<T extends object, K extends keyof T>(
    current: T,
    patch: Partial<T>,
    idKey: K,
  ): T {
    const next = { ...current, ...patch, [idKey]: current[idKey] } as T;
    const recordChanged = (Object.keys(patch) as Array<keyof T>).some(
      (key) => !runtimePatchValueEqual(current[key], next[key]),
    );
    return recordChanged ? next : current;
  }

  function runtimePatchValueEqual(left: unknown, right: unknown): boolean {
    if (Object.is(left, right)) {
      return true;
    }
    if (left === null || right === null || left === undefined || right === undefined) {
      return false;
    }
    if (Array.isArray(left) || Array.isArray(right)) {
      if (!Array.isArray(left) || !Array.isArray(right) || left.length !== right.length) {
        return false;
      }
      return left.every((value, index) => runtimePatchValueEqual(value, right[index]));
    }
    if (typeof left === 'object' && typeof right === 'object') {
      const leftRecord = left as Record<string, unknown>;
      const rightRecord = right as Record<string, unknown>;
      const leftKeys = Object.keys(leftRecord);
      const rightKeys = Object.keys(rightRecord);
      if (leftKeys.length !== rightKeys.length) {
        return false;
      }
      return leftKeys.every((key) => runtimePatchValueEqual(leftRecord[key], rightRecord[key]));
    }
    return false;
  }

  for (const [deviceId, patch] of Object.entries(delta.devices)) {
    const current = devices[deviceId];
    if (!current) {
      continue;
    }
    const next = mergeRecord(current, patch, 'device_id');
    if (next !== current) {
      if (devices === existing.devices) {
        devices = { ...existing.devices };
      }
      devices[deviceId] = next;
      changed = true;
    }
  }

  for (const [linkId, patch] of Object.entries(delta.links)) {
    const current = links[linkId];
    if (!current) {
      continue;
    }
    const next = mergeRecord(current, patch, 'link_id');
    if (next !== current) {
      if (links === existing.links) {
        links = { ...existing.links };
      }
      links[linkId] = next;
      changed = true;
    }
  }

  return changed ? { devices, links } : existing;
}

export function parseWSMessage(
  value: unknown,
):
  | WSMessage
  | SnapshotWSMessage
  | SnapshotDeltaWSMessage
  | RuntimeDeltaWSMessage
  | PrometheusStatusWSMessage
  | PollingHealthChangedWSMessage
  | ResyncRequiredWSMessage
  | AlertWSMessage
  | TopologyChangedWSMessage
  | ReadyWSMessage {
  if (!isRecord(value)) {
    throw new Error('invalid websocket message');
  }

  const type = readString(value, 'type');
  if (
    type !== 'ready' &&
    type !== 'snapshot' &&
    type !== 'snapshot_delta' &&
    type !== 'runtime_delta' &&
    type !== 'topology_delta' &&
    type !== 'metrics' &&
    type !== 'link_metrics' &&
    type !== 'alert' &&
    type !== 'prometheus_status' &&
    type !== 'polling_health_changed' &&
    type !== 'resync_required' &&
    type !== 'topology_changed'
  ) {
    throw new Error(`unsupported websocket message type: ${type}`);
  }

  if (type === 'ready') {
    const payload = isRecord(value.payload) ? value.payload : {};
    return {
      type,
      payload: {
        runtime_version:
          typeof payload.runtime_version === 'number' && Number.isFinite(payload.runtime_version)
            ? payload.runtime_version
            : undefined,
        runtime_identity:
          typeof payload.runtime_identity === 'string' ? payload.runtime_identity : undefined,
        alert_version:
          typeof payload.alert_version === 'number' && Number.isFinite(payload.alert_version)
            ? payload.alert_version
            : undefined,
      },
    } as ReadyWSMessage;
  }

  if (type === 'snapshot') {
    const payload = isRecord(value.payload) ? value.payload : {};
    if ('version' in payload || 'snapshot' in payload) {
      const version =
        typeof payload.version === 'number' && Number.isFinite(payload.version)
          ? payload.version
          : null;
      return {
        type,
        payload: {
          version,
          runtime_identity:
            typeof payload.runtime_identity === 'string' ? payload.runtime_identity : undefined,
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
      const baseVersion =
        typeof payload.base_version === 'number' && Number.isFinite(payload.base_version)
          ? payload.base_version
          : undefined;
      const version =
        typeof payload.version === 'number' && Number.isFinite(payload.version)
          ? payload.version
          : undefined;
      return {
        type,
        payload: {
          base_version: baseVersion,
          version,
          runtime_identity:
            typeof payload.runtime_identity === 'string' ? payload.runtime_identity : undefined,
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

  if (type === 'runtime_delta') {
    const payload = isRecord(value.payload) ? value.payload : {};
    if ('delta' in payload || 'version' in payload || 'base_version' in payload) {
      const baseVersion =
        typeof payload.base_version === 'number' && Number.isFinite(payload.base_version)
          ? payload.base_version
          : undefined;
      const version =
        typeof payload.version === 'number' && Number.isFinite(payload.version)
          ? payload.version
          : undefined;
      return {
        type,
        payload: {
          base_version: baseVersion,
          version,
          runtime_identity:
            typeof payload.runtime_identity === 'string' ? payload.runtime_identity : undefined,
          delta: parseRuntimePatchPayload(payload.delta),
        },
      } as RuntimeDeltaWSMessage;
    }
    return {
      type,
      payload: {
        delta: parseRuntimePatchPayload(value.payload),
      },
    } as RuntimeDeltaWSMessage;
  }

  if (type === 'polling_health_changed') {
    const payload = isRecord(value.payload) ? value.payload : {};
    return {
      type,
      payload: {
        essential_overloaded: payload.essential_overloaded === true,
        degraded_risk: payload.degraded_risk === true,
        essential_queue_lag_seconds:
          typeof payload.essential_queue_lag_seconds === 'number'
            ? payload.essential_queue_lag_seconds
            : 0,
        deadline_miss_total:
          typeof payload.deadline_miss_total === 'number' ? payload.deadline_miss_total : 0,
        active_workers: typeof payload.active_workers === 'number' ? payload.active_workers : 0,
        configured_workers:
          typeof payload.configured_workers === 'number' ? payload.configured_workers : 0,
      },
    } as PollingHealthChangedWSMessage;
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
      version =
        typeof payload.version === 'number' && Number.isFinite(payload.version)
          ? payload.version
          : undefined;
    } else if (isRecord(payload) && 'alerts' in payload && payload.alerts === null) {
      alerts = [];
      version =
        typeof payload.version === 'number' && Number.isFinite(payload.version)
          ? payload.version
          : undefined;
    } else if (isRecord(payload)) {
      alerts = [parseAlert(payload)];
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
    if (
      reason !== 'client_resync_scheduled' &&
      reason !== 'client_missing_runtime_snapshot' &&
      reason !== 'state_changes_dropped' &&
      reason !== 'hub_buffer_full'
    ) {
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
    const payload = isRecord(value.payload) ? value.payload : {};
    const topologyVersion = payload.topology_version;
    return {
      type,
      payload: {
        topology_version:
          typeof topologyVersion === 'number' || typeof topologyVersion === 'string'
            ? topologyVersion
            : undefined,
        reason: typeof payload.reason === 'string' ? payload.reason : undefined,
        recommended_endpoint:
          typeof payload.recommended_endpoint === 'string'
            ? payload.recommended_endpoint
            : undefined,
      },
    } as TopologyChangedWSMessage;
  }

  if (type === 'topology_delta') {
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
