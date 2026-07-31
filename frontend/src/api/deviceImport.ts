/** Typed, label-blind client for one-time Prometheus file-SD node imports. */
import { ValidationError } from './errors';
import {
  multipartJSONErrorPayload,
  requestJSON,
  requestJSONWithBody,
  requestMultipartJSON,
} from './transport';

export type DeviceImportMetricsMode = 'prometheus' | 'prometheus_snmp_fallback' | 'snmp';
export type DeviceImportTopologyLayoutScope = 'preserve' | 'reorganize';

export interface DeviceImportConfiguration {
  file: File;
  metrics_mode: DeviceImportMetricsMode;
  map_id: string;
  area_id?: string;
  snmp_profile_id?: string;
  topology_bootstrap_enabled?: boolean;
  topology_layout_scope?: DeviceImportTopologyLayoutScope;
}

export interface DeviceImportResolvedConfiguration {
  metrics_mode: DeviceImportMetricsMode;
  snmp_profile_id: string | null;
  map_id: string;
  area_id: string | null;
  topology_bootstrap_enabled: boolean;
  topology_layout_scope: DeviceImportTopologyLayoutScope;
}

export type DeviceImportPreviewStatus =
  | 'ready'
  | 'invalid'
  | 'skipped_duplicate_in_file'
  | 'skipped_existing';

export type DeviceImportResultStatus =
  | 'invalid'
  | 'skipped_duplicate_in_file'
  | 'skipped_existing'
  | 'created'
  | 'failed'
  | 'not_processed';

export interface DeviceImportDiagnostic {
  group_index: number;
  message: string;
}

export interface DeviceImportPreviewTarget {
  group_index: number;
  item_index: number;
  target: string;
  address: string;
  status: DeviceImportPreviewStatus;
  message?: string;
}

export interface DeviceImportPreviewSummary {
  total: number;
  ready: number;
  invalid: number;
  invalid_groups: number;
  skipped_existing: number;
  skipped_duplicate_in_file: number;
}

export interface DeviceImportPreview {
  file_digest: string;
  configuration: DeviceImportResolvedConfiguration;
  summary: DeviceImportPreviewSummary;
  targets: DeviceImportPreviewTarget[];
  diagnostics: DeviceImportDiagnostic[];
}

export interface DeviceImportResult {
  group_index: number;
  item_index: number;
  target: string;
  address: string;
  status: DeviceImportResultStatus;
  message?: string;
  device_id?: string;
}

export interface DeviceImportCommitSummary {
  total: number;
  created: number;
  skipped: number;
  failed: number;
  not_processed: number;
}

export interface DeviceImportCommitResult {
  file_digest: string;
  configuration: DeviceImportResolvedConfiguration;
  summary: DeviceImportCommitSummary;
  results: DeviceImportResult[];
  diagnostics: DeviceImportDiagnostic[];
  incomplete: boolean;
  topology_run_id?: string;
}

export type DeviceImportTopologyRunState =
  | 'importing'
  | 'discovering'
  | 'reconciling'
  | 'followup'
  | 'ready_for_layout'
  | 'failed'
  | 'completed'
  | 'superseded';

export type DeviceImportTopologyItemState =
  | 'queued'
  | 'running'
  | 'succeeded'
  | 'warning'
  | 'failed';

export interface DeviceImportTopologyRun {
  id: string;
  map_id: string;
  file_digest: string;
  layout_scope: DeviceImportTopologyLayoutScope;
  state: DeviceImportTopologyRunState;
  auto_layout_allowed: boolean;
  backgrounded: boolean;
  layout_input_token?: string;
  failure_code?: string;
  failure_message?: string;
  failure_reference?: string;
  reconcile_attempts: number;
  created_at: string;
  started_at?: string;
  completed_at?: string;
  updated_at: string;
}

export interface DeviceImportTopologyRunItem {
  device_id: string;
  state: DeviceImportTopologyItemState;
  attempt: number;
  result_code?: string;
  message?: string;
  reference?: string;
  neighbor_count: number;
  links_created: number;
  unresolved_neighbors: number;
  started_at?: string;
  completed_at?: string;
  updated_at: string;
}

export interface DeviceImportTopologyRunSnapshot {
  run: DeviceImportTopologyRun;
  items: DeviceImportTopologyRunItem[];
}

export interface DeviceImportTopologyLayoutPosition {
  device_id: string;
  x: number;
  y: number;
  pinned: boolean;
}

export interface DeviceImportTopologyLayoutApply {
  input_token: string;
  positions: DeviceImportTopologyLayoutPosition[];
  reset_link_route_ids: string[];
}

/** Carries authoritative per-target outcomes returned with a non-success commit response. */
export class DeviceImportPartialCommitError extends Error {
  public readonly result: DeviceImportCommitResult;

  public constructor(message: string, result: DeviceImportCommitResult) {
    super(message);
    this.name = 'DeviceImportPartialCommitError';
    this.result = result;
  }
}

const previewStatuses = new Set<DeviceImportPreviewStatus>([
  'ready',
  'invalid',
  'skipped_duplicate_in_file',
  'skipped_existing',
]);

const resultStatuses = new Set<DeviceImportResultStatus>([
  'invalid',
  'skipped_duplicate_in_file',
  'skipped_existing',
  'created',
  'failed',
  'not_processed',
]);

const metricsModes = new Set<DeviceImportMetricsMode>([
  'prometheus',
  'prometheus_snmp_fallback',
  'snmp',
]);

const topologyLayoutScopes = new Set<DeviceImportTopologyLayoutScope>(['preserve', 'reorganize']);

const topologyRunStates = new Set<DeviceImportTopologyRunState>([
  'importing',
  'discovering',
  'reconciling',
  'followup',
  'ready_for_layout',
  'failed',
  'completed',
  'superseded',
]);

const topologyItemStates = new Set<DeviceImportTopologyItemState>([
  'queued',
  'running',
  'succeeded',
  'warning',
  'failed',
]);

function invalidResponse(path: string): never {
  throw new ValidationError(`Invalid device import response: ${path}`);
}

function responseRecord(value: unknown, path: string): Record<string, unknown> {
  if (typeof value !== 'object' || value === null || Array.isArray(value)) {
    return invalidResponse(path);
  }
  return value as Record<string, unknown>;
}

function responseArray(value: unknown, path: string): unknown[] {
  if (!Array.isArray(value)) {
    return invalidResponse(path);
  }
  return value;
}

function responseString(record: Record<string, unknown>, key: string, path: string): string {
  const value = record[key];
  if (typeof value !== 'string') {
    return invalidResponse(`${path}.${key}`);
  }
  return value;
}

function responseOptionalString(
  record: Record<string, unknown>,
  key: string,
  path: string,
): string | undefined {
  const value = record[key];
  if (value === undefined) {
    return undefined;
  }
  if (typeof value !== 'string') {
    return invalidResponse(`${path}.${key}`);
  }
  return value;
}

function responseNullableString(
  record: Record<string, unknown>,
  key: string,
  path: string,
): string | null {
  const value = record[key];
  if (value === null) {
    return null;
  }
  if (typeof value !== 'string') {
    return invalidResponse(`${path}.${key}`);
  }
  return value;
}

function responseCount(record: Record<string, unknown>, key: string, path: string): number {
  const value = record[key];
  if (typeof value !== 'number' || !Number.isInteger(value) || value < 0) {
    return invalidResponse(`${path}.${key}`);
  }
  return value;
}

function responseBoolean(record: Record<string, unknown>, key: string, path: string): boolean {
  const value = record[key];
  if (typeof value !== 'boolean') {
    return invalidResponse(`${path}.${key}`);
  }
  return value;
}

function parseMetricsMode(value: unknown, path: string): DeviceImportMetricsMode {
  if (typeof value !== 'string' || !metricsModes.has(value as DeviceImportMetricsMode)) {
    return invalidResponse(path);
  }
  return value as DeviceImportMetricsMode;
}

function parseTopologyLayoutScope(value: unknown, path: string): DeviceImportTopologyLayoutScope {
  if (
    typeof value !== 'string' ||
    !topologyLayoutScopes.has(value as DeviceImportTopologyLayoutScope)
  ) {
    return invalidResponse(path);
  }
  return value as DeviceImportTopologyLayoutScope;
}

function parseResolvedConfiguration(value: unknown): DeviceImportResolvedConfiguration {
  const record = responseRecord(value, 'configuration');
  return {
    metrics_mode: parseMetricsMode(record.metrics_mode, 'configuration.metrics_mode'),
    snmp_profile_id: responseNullableString(record, 'snmp_profile_id', 'configuration'),
    map_id: responseString(record, 'map_id', 'configuration'),
    area_id: responseNullableString(record, 'area_id', 'configuration'),
    topology_bootstrap_enabled: responseBoolean(
      record,
      'topology_bootstrap_enabled',
      'configuration',
    ),
    topology_layout_scope: parseTopologyLayoutScope(
      record.topology_layout_scope,
      'configuration.topology_layout_scope',
    ),
  };
}

function parseDiagnostic(value: unknown, index: number): DeviceImportDiagnostic {
  const path = `diagnostics[${index}]`;
  const record = responseRecord(value, path);
  return {
    group_index: responseCount(record, 'group_index', path),
    message: responseString(record, 'message', path),
  };
}

function parsePreviewTarget(value: unknown, index: number): DeviceImportPreviewTarget {
  const path = `targets[${index}]`;
  const record = responseRecord(value, path);
  const status = responseString(record, 'status', path);
  if (!previewStatuses.has(status as DeviceImportPreviewStatus)) {
    return invalidResponse(`${path}.status`);
  }
  const message = responseOptionalString(record, 'message', path);
  return {
    group_index: responseCount(record, 'group_index', path),
    item_index: responseCount(record, 'item_index', path),
    target: responseString(record, 'target', path),
    address: responseString(record, 'address', path),
    status: status as DeviceImportPreviewStatus,
    ...(message === undefined ? {} : { message }),
  };
}

function parseResult(value: unknown, index: number): DeviceImportResult {
  const path = `results[${index}]`;
  const record = responseRecord(value, path);
  const status = responseString(record, 'status', path);
  if (!resultStatuses.has(status as DeviceImportResultStatus)) {
    return invalidResponse(`${path}.status`);
  }
  const message = responseOptionalString(record, 'message', path);
  const deviceID = responseOptionalString(record, 'device_id', path);
  return {
    group_index: responseCount(record, 'group_index', path),
    item_index: responseCount(record, 'item_index', path),
    target: responseString(record, 'target', path),
    address: responseString(record, 'address', path),
    status: status as DeviceImportResultStatus,
    ...(message === undefined ? {} : { message }),
    ...(deviceID === undefined ? {} : { device_id: deviceID }),
  };
}

/** Parses and strips a preview response down to the approved public fields. */
export function parseDeviceImportPreview(value: unknown): DeviceImportPreview {
  const record = responseRecord(value, 'root');
  const summary = responseRecord(record.summary, 'summary');
  return {
    file_digest: responseString(record, 'file_digest', 'root'),
    configuration: parseResolvedConfiguration(record.configuration),
    summary: {
      total: responseCount(summary, 'total', 'summary'),
      ready: responseCount(summary, 'ready', 'summary'),
      invalid: responseCount(summary, 'invalid', 'summary'),
      invalid_groups: responseCount(summary, 'invalid_groups', 'summary'),
      skipped_existing: responseCount(summary, 'skipped_existing', 'summary'),
      skipped_duplicate_in_file: responseCount(summary, 'skipped_duplicate_in_file', 'summary'),
    },
    targets: responseArray(record.targets, 'targets').map(parsePreviewTarget),
    diagnostics: responseArray(record.diagnostics, 'diagnostics').map(parseDiagnostic),
  };
}

/** Parses and strips a commit response down to ordered public result fields. */
export function parseDeviceImportCommitResult(value: unknown): DeviceImportCommitResult {
  const record = responseRecord(value, 'root');
  const summary = responseRecord(record.summary, 'summary');
  const topologyRunID = responseOptionalString(record, 'topology_run_id', 'root');
  return {
    file_digest: responseString(record, 'file_digest', 'root'),
    configuration: parseResolvedConfiguration(record.configuration),
    summary: {
      total: responseCount(summary, 'total', 'summary'),
      created: responseCount(summary, 'created', 'summary'),
      skipped: responseCount(summary, 'skipped', 'summary'),
      failed: responseCount(summary, 'failed', 'summary'),
      not_processed: responseCount(summary, 'not_processed', 'summary'),
    },
    results: responseArray(record.results, 'results').map(parseResult),
    diagnostics: responseArray(record.diagnostics, 'diagnostics').map(parseDiagnostic),
    incomplete: responseBoolean(record, 'incomplete', 'root'),
    ...(topologyRunID === undefined ? {} : { topology_run_id: topologyRunID }),
  };
}

/** Parses the durable, credential-free topology Bootstrap-Once progress snapshot. */
export function parseDeviceImportTopologyRunSnapshot(
  value: unknown,
): DeviceImportTopologyRunSnapshot {
  const root = responseRecord(value, 'root');
  const run = responseRecord(root.run, 'run');
  const state = responseString(run, 'state', 'run');
  if (!topologyRunStates.has(state as DeviceImportTopologyRunState)) {
    return invalidResponse('run.state');
  }
  const layoutInputToken = responseOptionalString(run, 'layout_input_token', 'run');
  const startedAt = responseOptionalString(run, 'started_at', 'run');
  const completedAt = responseOptionalString(run, 'completed_at', 'run');
  const failureCode = responseOptionalString(run, 'failure_code', 'run');
  const failureMessage = responseOptionalString(run, 'failure_message', 'run');
  const failureReference = responseOptionalString(run, 'failure_reference', 'run');
  return {
    run: {
      id: responseString(run, 'id', 'run'),
      map_id: responseString(run, 'map_id', 'run'),
      file_digest: responseString(run, 'file_digest', 'run'),
      layout_scope: parseTopologyLayoutScope(run.layout_scope, 'run.layout_scope'),
      state: state as DeviceImportTopologyRunState,
      auto_layout_allowed: responseBoolean(run, 'auto_layout_allowed', 'run'),
      backgrounded: responseBoolean(run, 'backgrounded', 'run'),
      ...(layoutInputToken === undefined ? {} : { layout_input_token: layoutInputToken }),
      ...(failureCode === undefined ? {} : { failure_code: failureCode }),
      ...(failureMessage === undefined ? {} : { failure_message: failureMessage }),
      ...(failureReference === undefined ? {} : { failure_reference: failureReference }),
      reconcile_attempts: responseCount(run, 'reconcile_attempts', 'run'),
      created_at: responseString(run, 'created_at', 'run'),
      ...(startedAt === undefined ? {} : { started_at: startedAt }),
      ...(completedAt === undefined ? {} : { completed_at: completedAt }),
      updated_at: responseString(run, 'updated_at', 'run'),
    },
    items: responseArray(root.items, 'items').map((value, index) => {
      const path = `items[${index}]`;
      const item = responseRecord(value, path);
      const itemState = responseString(item, 'state', path);
      if (!topologyItemStates.has(itemState as DeviceImportTopologyItemState)) {
        return invalidResponse(`${path}.state`);
      }
      const resultCode = responseOptionalString(item, 'result_code', path);
      const message = responseOptionalString(item, 'message', path);
      const reference = responseOptionalString(item, 'reference', path);
      const itemStartedAt = responseOptionalString(item, 'started_at', path);
      const itemCompletedAt = responseOptionalString(item, 'completed_at', path);
      return {
        device_id: responseString(item, 'device_id', path),
        state: itemState as DeviceImportTopologyItemState,
        attempt: responseCount(item, 'attempt', path),
        ...(resultCode === undefined ? {} : { result_code: resultCode }),
        ...(message === undefined ? {} : { message }),
        ...(reference === undefined ? {} : { reference }),
        neighbor_count: responseCount(item, 'neighbor_count', path),
        links_created: responseCount(item, 'links_created', path),
        unresolved_neighbors: responseCount(item, 'unresolved_neighbors', path),
        ...(itemStartedAt === undefined ? {} : { started_at: itemStartedAt }),
        ...(itemCompletedAt === undefined ? {} : { completed_at: itemCompletedAt }),
        updated_at: responseString(item, 'updated_at', path),
      };
    }),
  };
}

function deviceImportFormData(
  configuration: DeviceImportConfiguration,
  expectedFileDigest?: string,
): FormData {
  const form = new FormData();
  form.append('file', configuration.file);
  form.append('metrics_mode', configuration.metrics_mode);
  if (configuration.snmp_profile_id) {
    form.append('snmp_profile_id', configuration.snmp_profile_id);
  }
  form.append('map_id', configuration.map_id);
  if (configuration.area_id) {
    form.append('area_id', configuration.area_id);
  }
  if (configuration.metrics_mode === 'snmp') {
    form.append(
      'topology_bootstrap_enabled',
      String(configuration.topology_bootstrap_enabled ?? true),
    );
    form.append('topology_layout_scope', configuration.topology_layout_scope ?? 'preserve');
  }
  if (expectedFileDigest !== undefined) {
    form.append('expected_file_digest', expectedFileDigest);
  }
  return form;
}

const topologyRunPath = (runID: string) =>
  `/api/v1/admin/device-imports/topology-runs/${encodeURIComponent(runID)}`;

/** Retrieves one actor-scoped durable topology import run. */
export async function fetchDeviceImportTopologyRun(
  runID: string,
): Promise<DeviceImportTopologyRunSnapshot> {
  return parseDeviceImportTopologyRunSnapshot(await requestJSON(topologyRunPath(runID)));
}

/** Retrieves the unfinished topology import for a map, returning null when none exists. */
export async function fetchActiveDeviceImportTopologyRun(
  mapID: string,
): Promise<DeviceImportTopologyRunSnapshot | null> {
  try {
    const payload = await requestJSON(
      `/api/v1/admin/device-imports/topology-runs/active?map_id=${encodeURIComponent(mapID)}`,
    );
    return parseDeviceImportTopologyRunSnapshot(payload);
  } catch (error) {
    if (error instanceof Error && error.message.includes(' failed: 404 ')) {
      return null;
    }
    throw error;
  }
}

/** Requeues selected warning/failed nodes, or all retryable nodes when omitted. */
export async function retryDeviceImportTopologyRun(
  runID: string,
  deviceIDs: string[] = [],
): Promise<void> {
  await requestJSONWithBody(`${topologyRunPath(runID)}/retry`, 'POST', {
    device_ids: deviceIDs,
  });
}

/** Leaves discovery running in the background while the operator uses the partial map. */
export async function continueDeviceImportTopologyRun(runID: string): Promise<void> {
  await requestJSONWithBody(`${topologyRunPath(runID)}/continue`, 'POST', {});
}

/** Prevents a late automatic layout from overwriting the first manual canvas edit. */
export async function markDeviceImportTopologyManualEdit(runID: string): Promise<void> {
  await requestJSONWithBody(`${topologyRunPath(runID)}/manual-edit`, 'POST', {});
}

/** Applies positions and route resets in the backend's optimistic atomic transaction. */
export async function applyDeviceImportTopologyLayout(
  runID: string,
  request: DeviceImportTopologyLayoutApply,
): Promise<void> {
  await requestJSONWithBody(`${topologyRunPath(runID)}/layout`, 'POST', request);
}

/** Uploads the selected browser File for a side-effect-free preview. */
export async function previewDeviceImport(
  configuration: DeviceImportConfiguration,
): Promise<DeviceImportPreview> {
  const payload = await requestMultipartJSON(
    '/api/v1/admin/device-imports/preview',
    deviceImportFormData(configuration),
  );
  return parseDeviceImportPreview(payload);
}

/** Resends the original browser File and preview digest for commit. */
export async function commitDeviceImport(
  configuration: DeviceImportConfiguration,
  expectedFileDigest: string,
): Promise<DeviceImportCommitResult> {
  try {
    const payload = await requestMultipartJSON(
      '/api/v1/admin/device-imports/commit',
      deviceImportFormData(configuration, expectedFileDigest),
    );
    return parseDeviceImportCommitResult(payload);
  } catch (requestError) {
    const payload = multipartJSONErrorPayload(requestError);
    if (payload !== undefined) {
      try {
        const partialResult = parseDeviceImportCommitResult(payload);
        if (partialResult.incomplete || partialResult.results.length > 0) {
          const message =
            requestError instanceof Error ? requestError.message : 'Failed to commit node import';
          throw new DeviceImportPartialCommitError(message, partialResult);
        }
      } catch (parseError) {
        if (parseError instanceof DeviceImportPartialCommitError) {
          throw parseError;
        }
      }
    }
    throw requestError;
  }
}
