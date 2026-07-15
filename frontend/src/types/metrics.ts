/**
 * Defines metrics type contracts shared across frontend modules.
 * Keeps backend-facing domain shapes explicit at compile time.
 */
/** WSMessageType enumerates server-pushed canvas runtime protocol messages understood by the client. */
export type WSMessageType =
  | 'ready'
  | 'snapshot'
  | 'snapshot_delta'
  | 'runtime_delta'
  | 'runtime_replay'
  | 'topology_delta'
  | 'metrics'
  | 'link_metrics'
  | 'alert'
  | 'prometheus_status'
  | 'polling_health_changed'
  | 'resync_required'
  | 'topology_changed';
type APIRecord = Record<string, unknown>;

/** RuntimeReason explains why a runtime field is healthy, stale, missing, or unsupported. */
export type RuntimeReason =
  | 'ok'
  | 'awaiting_poll'
  | 'stale'
  | 'device_unreachable'
  | 'upstream_unavailable'
  | 'no_data'
  | 'unmonitored'
  | 'unsupported';

/** OperationalStatus is the high-level runtime state displayed on device nodes. */
export type OperationalStatus = 'up' | 'down' | 'probing' | 'unknown' | 'unmonitored';
/** ReachabilityStatus captures ping/SNMP reachability nuance used by health badges. */
export type ReachabilityStatus = 'up' | 'soft_down' | 'hard_down' | 'unknown' | 'unmonitored';
/** HealthStatus summarizes telemetry and reachability into a dashboard health band. */
export type HealthStatus = 'healthy' | 'warning' | 'critical' | 'unknown';
/** FreshnessStatus reports whether the last collected sample is current enough to trust. */
export type FreshnessStatus = 'fresh' | 'stale' | 'awaiting_poll' | 'unmonitored';
/** MetricsStatus reports whether metric fields are complete, partial, unavailable, or disabled. */
export type MetricsStatus = 'available' | 'partial' | 'unavailable' | 'unmonitored';
/** LinkMetricsStatus reports the availability of link throughput and utilization telemetry. */
export type LinkMetricsStatus = 'available' | 'partial' | 'unavailable';
/** PrimaryHealth is the compact device health state used for node severity styling. */
export type PrimaryHealth =
  | 'probing'
  | 'up_fresh'
  | 'up_stale'
  | 'snmp_degraded'
  | 'unreachable'
  | 'quarantined';
/** RuntimeFlag adds operational context that should not replace the primary health state. */
export type RuntimeFlag =
  | 'deadline_missed'
  | 'overloaded'
  | 'background_pending'
  | 'partial_telemetry'
  | 'degraded_risk'
  | 'persistence_lagging';
/** FieldState tracks per-metric completeness for mixed healthy/degraded telemetry rows. */
export type FieldState = 'ok' | 'missing' | 'error' | 'stale';
/** ReachabilityEvidence preserves tri-state collector evidence instead of coercing to boolean. */
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

/** DeviceRuntimeDTO is the runtime overlay for one canonical device. */
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

/** LinkRuntimeDTO is the runtime overlay for one canonical link. */
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

/** DeviceMetricsDTO is a compatibility alias for older component names. */
export type DeviceMetricsDTO = DeviceRuntimeDTO;
/** LinkMetricsDTO is a compatibility alias for older component names. */
export type LinkMetricsDTO = LinkRuntimeDTO;

/** AlertDTO is a compact alert summary pushed over WebSocket and used for node/edge severity. */
export interface AlertDTO {
  device_id: string;
  severity: string;
  alert_name: string;
  state: string;
  summary: string;
}

/** AlertStatus is the derived visual severity used by topology nodes and links. */
export type AlertStatus = 'normal' | 'degraded' | 'down';

/** SnapshotPayload is the complete runtime state indexed by canonical device/link IDs. */
export interface SnapshotPayload {
  devices: Record<string, DeviceRuntimeDTO>;
  links: Record<string, LinkRuntimeDTO>;
}

/** DeviceRuntimePatch is a sparse device update that must include the device ID it applies to. */
export type DeviceRuntimePatch = Partial<DeviceRuntimeDTO> & Pick<DeviceRuntimeDTO, 'device_id'>;
/** LinkRuntimePatch is a sparse link update that must include the link ID it applies to. */
export type LinkRuntimePatch = Partial<LinkRuntimeDTO> & Pick<LinkRuntimeDTO, 'link_id'>;

/** RuntimePatchPayload groups sparse runtime deltas by canonical device and link IDs. */
export interface RuntimePatchPayload {
  devices: Record<string, DeviceRuntimePatch>;
  links: Record<string, LinkRuntimePatch>;
}

/** PrometheusStatusPayload reports backend Prometheus integration availability. */
export interface PrometheusStatusPayload {
  enabled?: boolean;
  available: boolean;
  error?: string;
}

/** ResyncRequiredPayload tells the client why overview recovery must use the stream or legacy HTTP. */
export interface ResyncRequiredPayload {
  scope: 'overview';
  reason:
    | 'client_resync_scheduled'
    | 'client_missing_runtime_snapshot'
    | 'state_changes_dropped'
    | 'hub_buffer_full';
  strategy?: 'stream';
  target_version?: number;
  runtime_stream_id?: string;
}

/** TopologyChangedPayload advertises canonical topology changes and the recommended resync endpoint. */
export interface TopologyChangedPayload {
  topology_version?: number | string;
  reason?: string;
  recommended_endpoint?: string;
}

/** ReadyPayload confirms the runtime synchronization result and its exact ready barrier. */
export interface ReadyPayload {
  runtime_version?: number;
  runtime_stream_id?: string;
  runtime_identity?: string;
  alert_version?: number;
  sync_mode?: string;
}

/** SnapshotEnvelopePayload wraps a full runtime snapshot with version and identity metadata. */
export interface SnapshotEnvelopePayload {
  version: number | null;
  runtime_stream_id?: string;
  runtime_identity?: string;
  snapshot: SnapshotPayload;
}

/** SnapshotDeltaEnvelopePayload wraps legacy full-shape deltas with a base version. */
export interface SnapshotDeltaEnvelopePayload {
  base_version?: number;
  version?: number;
  runtime_identity?: string;
  delta: SnapshotPayload;
}

/** RuntimeDeltaEnvelopePayload wraps sparse runtime deltas with base-version validation data. */
export interface RuntimeDeltaEnvelopePayload {
  base_version?: number;
  version?: number;
  runtime_stream_id?: string;
  runtime_identity?: string;
  delta: RuntimePatchPayload;
}

/** RuntimeReplayEnvelopePayload wraps a required resumable sparse runtime range. */
export interface RuntimeReplayEnvelopePayload {
  from_version: number;
  version: number;
  runtime_stream_id: string;
  delta: RuntimePatchPayload;
}

/** Describes the polling health queue payload contract used by the frontend domain model. */
export interface PollingHealthQueuePayload {
  ready_depth: number;
  lag_seconds: number;
  active_workers: number;
  configured_workers: number;
}

/** Describes the polling health warning payload contract used by the frontend domain model. */
export interface PollingHealthWarningPayload {
  code: string;
  message: string;
}

/** PollingHealthPayload summarizes scheduler pressure and worker health. */
export interface PollingHealthPayload {
  essential_overloaded: boolean;
  degraded_risk: boolean;
  essential_queue_lag_seconds: number;
  deadline_miss_total: number;
  active_workers: number;
  configured_workers: number;
  queues?: Record<string, PollingHealthQueuePayload>;
  warnings?: PollingHealthWarningPayload[];
}

/** WSMessage is the untyped envelope shape before payload-specific parsing. */
export interface WSMessage {
  type: WSMessageType;
  payload: unknown;
}

/** Describes the snapshot wsmessage contract used by the frontend domain model. */
export interface SnapshotWSMessage extends Omit<WSMessage, 'type' | 'payload'> {
  type: 'snapshot';
  payload: SnapshotEnvelopePayload;
}

/** Describes the snapshot delta wsmessage contract used by the frontend domain model. */
export interface SnapshotDeltaWSMessage extends Omit<WSMessage, 'type' | 'payload'> {
  type: 'snapshot_delta';
  payload: SnapshotDeltaEnvelopePayload;
}

/** Describes the runtime delta wsmessage contract used by the frontend domain model. */
export interface RuntimeDeltaWSMessage extends Omit<WSMessage, 'type' | 'payload'> {
  type: 'runtime_delta';
  payload: RuntimeDeltaEnvelopePayload;
}

/** Describes the runtime replay wsmessage contract used by the frontend domain model. */
export interface RuntimeReplayWSMessage extends Omit<WSMessage, 'type' | 'payload'> {
  type: 'runtime_replay';
  payload: RuntimeReplayEnvelopePayload;
}

/** Describes the polling health changed wsmessage contract used by the frontend domain model. */
export interface PollingHealthChangedWSMessage extends Omit<WSMessage, 'type' | 'payload'> {
  type: 'polling_health_changed';
  payload: PollingHealthPayload;
}

/** Describes the prometheus status wsmessage contract used by the frontend domain model. */
export interface PrometheusStatusWSMessage extends Omit<WSMessage, 'type' | 'payload'> {
  type: 'prometheus_status';
  payload: PrometheusStatusPayload;
}

/** Describes the resync required wsmessage contract used by the frontend domain model. */
export interface ResyncRequiredWSMessage extends Omit<WSMessage, 'type' | 'payload'> {
  type: 'resync_required';
  payload: ResyncRequiredPayload;
}

/** Describes the alert wsmessage contract used by the frontend domain model. */
export interface AlertWSMessage extends Omit<WSMessage, 'type' | 'payload'> {
  type: 'alert';
  payload: AlertEnvelopePayload;
}

/** Describes the topology changed wsmessage contract used by the frontend domain model. */
export interface TopologyChangedWSMessage extends Omit<WSMessage, 'type' | 'payload'> {
  type: 'topology_changed';
  payload: TopologyChangedPayload;
}

/** Describes the ready wsmessage contract used by the frontend domain model. */
export interface ReadyWSMessage extends Omit<WSMessage, 'type' | 'payload'> {
  type: 'ready';
  payload: ReadyPayload;
}

/** Describes the alert envelope payload contract used by the frontend domain model. */
export interface AlertEnvelopePayload {
  version?: number;
  alerts: AlertDTO[];
}

// isRecord narrows unknown websocket payloads before strict runtime parsing.
function isRecord(value: unknown): value is APIRecord {
  return typeof value === 'object' && value !== null && !Array.isArray(value);
}

// readString reads optional string metadata with the existing fallback behavior.
function readString(record: APIRecord, key: string, fallback = ''): string {
  const value = record[key];
  return typeof value === 'string' ? value : fallback;
}

// readFiniteNumber reads optional finite numbers with the existing fallback behavior.
function readFiniteNumber(record: APIRecord, key: string, fallback = 0): number {
  const value = record[key];
  return typeof value === 'number' && Number.isFinite(value) ? value : fallback;
}

const MAX_POLLING_HEALTH_QUEUES = 16;
const MAX_POLLING_HEALTH_WARNINGS = 16;

// parsePollingHealthQueues normalizes bounded queue telemetry for websocket health updates.
function parsePollingHealthQueues(
  value: unknown,
): Record<string, PollingHealthQueuePayload> | undefined {
  if (!isRecord(value)) {
    return undefined;
  }

  const queues: Record<string, PollingHealthQueuePayload> = {};
  for (const [name, queue] of Object.entries(value).slice(0, MAX_POLLING_HEALTH_QUEUES)) {
    if (name.length === 0 || !isRecord(queue)) {
      continue;
    }
    queues[name] = {
      ready_depth: readFiniteNumber(queue, 'ready_depth'),
      lag_seconds: readFiniteNumber(queue, 'lag_seconds'),
      active_workers: readFiniteNumber(queue, 'active_workers'),
      configured_workers: readFiniteNumber(queue, 'configured_workers'),
    };
  }

  return Object.keys(queues).length > 0 ? queues : undefined;
}

// parsePollingHealthWarnings normalizes bounded warning telemetry for websocket health updates.
function parsePollingHealthWarnings(value: unknown): PollingHealthWarningPayload[] | undefined {
  if (!Array.isArray(value)) {
    return undefined;
  }

  const warnings: PollingHealthWarningPayload[] = [];
  for (const warning of value.slice(0, MAX_POLLING_HEALTH_WARNINGS)) {
    if (!isRecord(warning)) {
      continue;
    }
    const code = readString(warning, 'code');
    const message = readString(warning, 'message');
    if (code === '' && message === '') {
      continue;
    }
    warnings.push({ code, message });
  }

  return warnings.length > 0 ? warnings : undefined;
}

// readRequiredString reads mandatory string fields and rejects absent or empty values by default.
function readRequiredString(record: APIRecord, key: string, allowEmpty = false): string {
  const value = record[key];
  if (typeof value === 'string' && (allowEmpty || value.length > 0)) {
    return value;
  }
  throw new Error(`invalid required field: ${key}`);
}

// readRequiredRuntimeVersion accepts only JavaScript integers that can represent a uint64 cursor exactly.
function readRequiredRuntimeVersion(record: APIRecord, key: string): number {
  const value = record[key];
  if (typeof value === 'number' && Number.isSafeInteger(value) && value >= 0) {
    return value;
  }
  throw new Error(`invalid runtime replay version: ${key}`);
}

// readRequiredNullableString accepts explicit null while rejecting absent or non-string values.
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

// readRequiredNullableNumber accepts explicit null while rejecting absent or non-finite values.
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

// readRequiredEnum validates mandatory enum fields against the caller-provided allowlist.
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

// readRuntimeFlags validates runtime flag arrays without silently dropping invalid flags.
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

// readRuntimeFlagsPatch treats null patch values as clearing the runtime flag list.
function readRuntimeFlagsPatch(record: APIRecord, key: string): RuntimeFlag[] {
  if (record[key] === null) {
    return [];
  }
  return readRuntimeFlags(record, key);
}

// readFieldStates validates grouped field-state metadata for device runtime.
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

// readRequiredCount validates non-negative counters and truncates fractional values.
function readRequiredCount(record: APIRecord, key: string): number {
  const value = record[key];
  if (typeof value === 'number' && Number.isFinite(value) && value >= 0) {
    return Math.trunc(value);
  }
  throw new Error(`invalid required field: ${key}`);
}

// parseDeviceRuntime strictly parses a complete device runtime snapshot.
/** Parses and normalizes one device runtime payload from snapshots, deltas, or tests. */
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

/** parseDeviceMetrics is a compatibility alias for components that still use metrics naming. */
export const parseDeviceMetrics = parseDeviceRuntime;

/** Parses one link runtime payload and applies safe defaults for missing telemetry. */
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

/** parseLinkMetrics is a compatibility alias for components that still use metrics naming. */
export const parseLinkMetrics = parseLinkRuntime;

/** Parses a compact alert summary, preserving empty strings for optional display fields. */
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

// parseSnapshotPayload parses full device and link runtime maps and verifies key identity.
/** Parses a full runtime snapshot and indexes devices/links by their canonical IDs. */
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

// parseDeviceRuntimePatch parses partial runtime updates while keeping device_id mandatory.
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

// parseLinkRuntimePatch parses partial link runtime updates while keeping link_id mandatory.
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

// parseRuntimePatchPayload parses runtime delta maps and verifies patch key identity.
/** Parses sparse runtime deltas while rejecting patches that lack their required IDs. */
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
/** Merges legacy full-shape snapshot deltas into a cloned snapshot without mutating the previous state. */
export function mergeSnapshotDelta(
  existing: SnapshotPayload,
  delta: SnapshotPayload,
): SnapshotPayload {
  return {
    devices: { ...existing.devices, ...delta.devices },
    links: { ...existing.links, ...delta.links },
  };
}

// mergeRuntimeDeltaPatch applies partial runtime updates without changing untouched records.
/** Merges sparse runtime patches into a cloned snapshot without mutating the previous state. */
export function mergeRuntimeDeltaPatch(
  existing: SnapshotPayload,
  delta: RuntimePatchPayload,
): SnapshotPayload {
  let devices = existing.devices;
  let links = existing.links;
  let changed = false;

  // mergeRecord preserves immutable identity fields while applying one parsed patch.
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

  // runtimePatchValueEqual compares JSON-like runtime values without reference sensitivity.
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

// parseWSMessage validates known websocket message shapes and preserves legacy passthrough payloads.
/** Parses WebSocket envelopes and narrows payloads by message type for the runtime hook. */
export function parseWSMessage(
  value: unknown,
):
  | WSMessage
  | SnapshotWSMessage
  | SnapshotDeltaWSMessage
  | RuntimeDeltaWSMessage
  | RuntimeReplayWSMessage
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
    type !== 'runtime_replay' &&
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
        ...(typeof payload.runtime_stream_id === 'string'
          ? { runtime_stream_id: payload.runtime_stream_id }
          : {}),
        runtime_identity:
          typeof payload.runtime_identity === 'string' ? payload.runtime_identity : undefined,
        alert_version:
          typeof payload.alert_version === 'number' && Number.isFinite(payload.alert_version)
            ? payload.alert_version
            : undefined,
        ...(typeof payload.sync_mode === 'string' ? { sync_mode: payload.sync_mode } : {}),
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
          ...(typeof payload.runtime_stream_id === 'string'
            ? { runtime_stream_id: payload.runtime_stream_id }
            : {}),
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
          ...(typeof payload.runtime_stream_id === 'string'
            ? { runtime_stream_id: payload.runtime_stream_id }
            : {}),
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

  if (type === 'runtime_replay') {
    if (!isRecord(value.payload)) {
      throw new Error('invalid runtime replay payload');
    }
    const payload = value.payload;
    const fromVersion = readRequiredRuntimeVersion(payload, 'from_version');
    const version = readRequiredRuntimeVersion(payload, 'version');
    if (fromVersion > version) {
      throw new Error('invalid runtime replay version range');
    }

    return {
      type,
      payload: {
        from_version: fromVersion,
        version,
        runtime_stream_id: readRequiredString(payload, 'runtime_stream_id'),
        delta: parseRuntimePatchPayload(payload.delta),
      },
    } as RuntimeReplayWSMessage;
  }

  if (type === 'polling_health_changed') {
    const payload = isRecord(value.payload) ? value.payload : {};
    return {
      type,
      payload: {
        essential_overloaded: payload.essential_overloaded === true,
        degraded_risk: payload.degraded_risk === true,
        essential_queue_lag_seconds: readFiniteNumber(payload, 'essential_queue_lag_seconds'),
        deadline_miss_total: readFiniteNumber(payload, 'deadline_miss_total'),
        active_workers: readFiniteNumber(payload, 'active_workers'),
        configured_workers: readFiniteNumber(payload, 'configured_workers'),
        queues: parsePollingHealthQueues(payload.queues),
        warnings: parsePollingHealthWarnings(payload.warnings),
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
        ...(payload.strategy === 'stream' ? { strategy: payload.strategy } : {}),
        ...(typeof payload.target_version === 'number' &&
        Number.isSafeInteger(payload.target_version) &&
        payload.target_version >= 0
          ? { target_version: payload.target_version }
          : {}),
        ...(typeof payload.runtime_stream_id === 'string'
          ? { runtime_stream_id: payload.runtime_stream_id }
          : {}),
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

// formatUptime formats seconds into the compact uptime labels used by the canvas UI.
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

// metricColor maps percentage-style metrics to semantic text color classes.
export function metricColor(value: number): string {
  if (value > 85) {
    return 'text-status-down';
  }
  if (value >= 60) {
    return 'text-warning';
  }
  return 'text-status-up';
}

// utilizationColor maps normalized link utilization to semantic CSS custom properties.
export function utilizationColor(value: number): string {
  if (value > 0.8) {
    return 'var(--color-status-down)';
  }
  if (value >= 0.5) {
    return 'var(--color-status-probing)';
  }
  return 'var(--color-status-up)';
}

// alertStatusForDevice derives device alert status from active firing alerts.
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

// isPrometheusUnavailable reports unavailable Prometheus only when it is enabled.
export function isPrometheusUnavailable(status: PrometheusStatusPayload | null): boolean {
  return status !== null && status.enabled !== false && !status.available;
}

// formatThroughput formats bits-per-second values for compact link labels.
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
