/** Typed, label-blind client for one-time Prometheus file-SD node imports. */
import { ValidationError } from './errors';
import { multipartJSONErrorPayload, requestMultipartJSON } from './transport';

export type DeviceImportMetricsMode = 'prometheus' | 'prometheus_snmp_fallback' | 'snmp';

export interface DeviceImportConfiguration {
  file: File;
  metrics_mode: DeviceImportMetricsMode;
  map_id: string;
  area_id?: string;
  snmp_profile_id?: string;
}

export interface DeviceImportResolvedConfiguration {
  metrics_mode: DeviceImportMetricsMode;
  snmp_profile_id: string | null;
  map_id: string;
  area_id: string | null;
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

function parseResolvedConfiguration(value: unknown): DeviceImportResolvedConfiguration {
  const record = responseRecord(value, 'configuration');
  return {
    metrics_mode: parseMetricsMode(record.metrics_mode, 'configuration.metrics_mode'),
    snmp_profile_id: responseNullableString(record, 'snmp_profile_id', 'configuration'),
    map_id: responseString(record, 'map_id', 'configuration'),
    area_id: responseNullableString(record, 'area_id', 'configuration'),
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
  if (expectedFileDigest !== undefined) {
    form.append('expected_file_digest', expectedFileDigest);
  }
  return form;
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
