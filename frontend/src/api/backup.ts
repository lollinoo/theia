/**
 * Provides frontend API helpers for backup endpoints.
 * Keeps request construction and backend response handling out of UI components.
 */
import {
  type BackupFileContent,
  type BackupJob,
  type BulkBackupRun,
  type BulkOperationStatus,
} from '../types/api';
import {
  parseBackupFileContent,
  parseBackupJob,
  parseBulkBackupResult,
  parseBulkBackupRunResponse,
  parseBulkOperationStatus,
} from './backupParsers';
import { ServerError, ValidationError } from './errors';
import { type ErrorPayload, headersWithCsrf, requestJSON, requestJSONWithBody } from './transport';

/** Describes the bulk backup result contract used by the frontend API boundary. */
export type BulkBackupResult = {
  device_id: string;
  device_name: string;
  status: 'queued' | 'skipped';
  reason?: string;
  job_id?: string;
};

// triggerBackup starts a device backup and parses the created job envelope.
export async function triggerBackup(deviceId: string): Promise<BackupJob> {
  const response = await requestJSONWithBody(
    `/api/v1/devices/${encodeURIComponent(deviceId)}/backups`,
    'POST',
  );
  const data = (response as Record<string, unknown>)?.data as Record<string, unknown>;
  return parseBackupJob(data);
}

// triggerBulkBackup starts the legacy bulk-backup endpoint and preserves readable limit errors.
export async function triggerBulkBackup(deviceIds: string[]): Promise<BulkBackupResult[]> {
  const payload = await requestBulkJSON(
    '/api/v1/backups/bulk',
    { device_ids: deviceIds },
    'bulk backup',
  );
  const data = (payload as Record<string, unknown>)?.data;
  if (!Array.isArray(data)) return [];
  return data.map((item) => parseBulkBackupResult(item as Record<string, unknown>));
}

// startBulkBackupRun starts a tracked bulk backup run and accepts conflict payloads as current state.
export async function startBulkBackupRun(deviceIds: string[]): Promise<BulkBackupRun> {
  const payload = await requestBulkJSON(
    '/api/v1/backups/bulk-runs',
    { device_ids: deviceIds },
    'bulk backup',
    { returnConflictPayload: true },
  );
  const run = parseBulkBackupRunResponse(payload);
  if (!run) throw new Error('bulk backup run response is missing');
  return run;
}

// fetchBulkOperationStatus loads backup and download operation capability metadata.
export async function fetchBulkOperationStatus(): Promise<BulkOperationStatus> {
  const payload = await requestJSON('/api/v1/backups/bulk/status');
  return parseBulkOperationStatus(payload);
}

// fetchLatestBulkBackupRun returns the current or latest run, preserving null when none exists.
export async function fetchLatestBulkBackupRun(): Promise<BulkBackupRun | null> {
  const payload = await requestJSON('/api/v1/backups/bulk-runs/latest');
  return parseBulkBackupRunResponse(payload);
}

// fetchBulkBackupRun loads one tracked bulk backup run by encoded ID.
export async function fetchBulkBackupRun(runId: string): Promise<BulkBackupRun> {
  const payload = await requestJSON(`/api/v1/backups/bulk-runs/${encodeURIComponent(runId)}`);
  const run = parseBulkBackupRunResponse(payload);
  if (!run) throw new Error('bulk backup run not found');
  return run;
}

// cancelBulkBackupRun requests cancellation and parses the updated run state.
export async function cancelBulkBackupRun(runId: string): Promise<BulkBackupRun> {
  const payload = await requestBulkJSON(
    `/api/v1/backups/bulk-runs/${encodeURIComponent(runId)}/cancel`,
    {},
    'bulk backup cancel',
  );
  const run = parseBulkBackupRunResponse(payload);
  if (!run) throw new Error('bulk backup run response is missing');
  return run;
}

// pauseBulkBackupRun pauses a tracked run and parses the updated run state.
export async function pauseBulkBackupRun(runId: string): Promise<BulkBackupRun> {
  const payload = await requestBulkJSON(
    `/api/v1/backups/bulk-runs/${encodeURIComponent(runId)}/pause`,
    {},
    'bulk backup pause',
  );
  const run = parseBulkBackupRunResponse(payload);
  if (!run) throw new Error('bulk backup run response is missing');
  return run;
}

// resumeBulkBackupRun resumes a tracked run and parses the updated run state.
export async function resumeBulkBackupRun(runId: string): Promise<BulkBackupRun> {
  const payload = await requestBulkJSON(
    `/api/v1/backups/bulk-runs/${encodeURIComponent(runId)}/resume`,
    {},
    'bulk backup resume',
  );
  const run = parseBulkBackupRunResponse(payload);
  if (!run) throw new Error('bulk backup run response is missing');
  return run;
}

// fetchBackupJobs loads all backup jobs for a device and preserves empty-list fallback behavior.
export async function fetchBackupJobs(deviceId: string): Promise<BackupJob[]> {
  const payload = await requestJSON(`/api/v1/devices/${encodeURIComponent(deviceId)}/backups`);
  const data = (payload as Record<string, unknown>)?.data;
  if (!Array.isArray(data)) return [];
  return data.map((item) => parseBackupJob(item as Record<string, unknown>));
}

// fetchBackupJob loads one backup job by encoded ID.
export async function fetchBackupJob(jobId: string): Promise<BackupJob> {
  const payload = await requestJSON(`/api/v1/backup-jobs/${encodeURIComponent(jobId)}`);
  const data = (payload as Record<string, unknown>)?.data as Record<string, unknown>;
  return parseBackupJob(data);
}

// fetchLatestBackupJob returns the latest device backup or null for any unavailable response.
export async function fetchLatestBackupJob(deviceId: string): Promise<BackupJob | null> {
  try {
    const payload = await requestJSON(
      `/api/v1/devices/${encodeURIComponent(deviceId)}/backups/latest`,
    );
    const data = (payload as Record<string, unknown>)?.data as Record<string, unknown>;
    return parseBackupJob(data);
  } catch {
    return null;
  }
}

// deleteBackupJob removes one backup job and its files.
export async function deleteBackupJob(jobId: string): Promise<void> {
  await requestJSONWithBody(`/api/v1/backup-jobs/${encodeURIComponent(jobId)}`, 'DELETE');
}

// backupFileDownloadUrl builds the direct file download URL with an encoded file ID.
export function backupFileDownloadUrl(fileId: string): string {
  return `/api/v1/backup-files/${encodeURIComponent(fileId)}/download`;
}

// fetchBackupFileContent loads inline content metadata and provides the legacy download URL fallback.
export async function fetchBackupFileContent(fileId: string): Promise<BackupFileContent> {
  const payload = await requestJSON(`/api/v1/backup-files/${encodeURIComponent(fileId)}/content`);
  const payloadRecord =
    typeof payload === 'object' && payload !== null ? (payload as Record<string, unknown>) : {};
  const data =
    typeof payloadRecord.data === 'object' && payloadRecord.data !== null
      ? (payloadRecord.data as Record<string, unknown>)
      : {};
  return parseBackupFileContent(data, backupFileDownloadUrl(fileId));
}

// bulkDownloadUrl preserves the historical endpoint helper for form-post downloads.
export function bulkDownloadUrl(_deviceIds: string[]): string {
  // We use a form POST for the download, so return the endpoint URL
  return '/api/v1/backups/bulk-download';
}

/** Describes the bulk download result contract used by the frontend API boundary. */
export type BulkDownloadResult = 'saved' | 'cancelled';

/** Describes the bulk download options contract used by the frontend API boundary. */
export type BulkDownloadOptions = {
  filename?: string;
};

// triggerBulkDownload posts selected devices and streams or downloads the returned ZIP archive.
export async function triggerBulkDownload(
  deviceIds: string[],
  options: BulkDownloadOptions = {},
): Promise<BulkDownloadResult> {
  const suggestedFilename = options.filename ?? defaultBulkDownloadFilename();
  const response = await fetch('/api/v1/backups/bulk-download', {
    method: 'POST',
    headers: headersWithCsrf({ 'Content-Type': 'application/json' }),
    body: JSON.stringify({ device_ids: deviceIds }),
  });
  if (!response.ok) {
    const payload = (await response.json().catch(() => null)) as Record<string, unknown> | null;
    const errorMessage =
      payload && typeof payload.error === 'string' ? payload.error : response.statusText;
    if (response.status === 413) {
      throw new ValidationError(formatBulkLimitMessage(errorMessage));
    }
    throw new Error(errorMessage);
  }
  const disposition = response.headers.get('Content-Disposition') ?? '';
  const match = disposition.match(/filename="(.+?)"/);
  const filename = options.filename ?? match?.[1] ?? suggestedFilename;
  const saveTarget = prepareStreamingSaveTarget(filename);

  return saveDownloadResponse(response, filename, saveTarget);
}

// requestBulkJSON wraps mutating bulk endpoints with CSRF and bulk-specific error mapping.
async function requestBulkJSON(
  path: string,
  body: unknown,
  operation: string,
  options: { returnConflictPayload?: boolean } = {},
): Promise<unknown> {
  const response = await fetch(path, {
    method: 'POST',
    headers: headersWithCsrf({
      Accept: 'application/json',
      'Content-Type': 'application/json',
    }),
    body: JSON.stringify(body),
  });
  const payload = (await response.json().catch(() => null)) as ErrorPayload | unknown;
  if (!response.ok) {
    if (response.status === 409 && options.returnConflictPayload) {
      return payload;
    }
    const errorMessage =
      typeof payload === 'object' &&
      payload !== null &&
      'error' in payload &&
      typeof payload.error === 'string'
        ? payload.error
        : response.statusText;
    if (response.status === 413) {
      throw new ValidationError(formatBulkLimitMessage(errorMessage));
    }
    if (response.status === 400 || response.status === 409) {
      throw new ValidationError(errorMessage);
    }
    if (response.status === 500) {
      const refMatch = /ref:\s*([a-zA-Z0-9-]+)/.exec(errorMessage);
      const correlationId = refMatch ? refMatch[1] : undefined;
      const userMessage = correlationId
        ? `Something went wrong (ref: ${correlationId})`
        : 'Something went wrong';
      throw new ServerError(userMessage, correlationId);
    }
    throw new Error(`${operation} failed: ${response.status} ${errorMessage}`);
  }
  return payload;
}

// formatBulkLimitMessage converts backend limit errors into user-facing bulk operation messages.
function formatBulkLimitMessage(message: string): string {
  const match =
    /^bulk (backup(?: run)?|download) exceeds (devices|queued jobs|files|bytes) limit: requested (\d+), maximum (\d+)$/i.exec(
      message,
    );
  if (!match) {
    return message;
  }
  const [, operation, limit, requested, maximum] = match;
  const normalizedOperation = operation.toLowerCase().replace(/ run$/, '');
  const normalizedLimit = limit.toLowerCase();
  if (normalizedOperation === 'backup' && normalizedLimit === 'devices') {
    return `Too many devices selected for bulk backup. Maximum ${maximum}, requested ${requested}.`;
  }
  if (normalizedOperation === 'backup' && normalizedLimit === 'queued jobs') {
    return `Too many backup jobs would be queued. Maximum ${maximum}, requested ${requested}.`;
  }
  if (normalizedOperation === 'download' && normalizedLimit === 'devices') {
    return `Too many devices selected for bulk download. Maximum ${maximum}, requested ${requested}.`;
  }
  if (normalizedOperation === 'download' && normalizedLimit === 'files') {
    return `Too many backup files selected for bulk download. Maximum ${maximum}, requested ${requested}.`;
  }
  if (normalizedOperation === 'download' && normalizedLimit === 'bytes') {
    return `Bulk download is too large. Maximum ${maximum} bytes, requested ${requested} bytes.`;
  }
  return message;
}

type SaveFilePicker = (options: {
  suggestedName?: string;
  types?: Array<{
    description: string;
    accept: Record<string, string[]>;
  }>;
}) => Promise<{
  createWritable: () => Promise<WritableStream<Uint8Array>>;
}>;

type StreamingSaveTarget = Promise<{
  createWritable: () => Promise<WritableStream<Uint8Array>>;
} | null> | null;

// browserSaveFilePicker returns the File System Access picker when the browser provides it.
function browserSaveFilePicker(): SaveFilePicker | undefined {
  const candidate = (globalThis as { showSaveFilePicker?: unknown }).showSaveFilePicker;
  return typeof candidate === 'function' ? (candidate as SaveFilePicker) : undefined;
}

// defaultBulkDownloadFilename builds the legacy timestamped archive filename.
function defaultBulkDownloadFilename(): string {
  return `${new Date().toISOString().replace(/[-:T]/g, '').slice(0, 15)}_THEIA_BACKUPS.zip`;
}

// prepareStreamingSaveTarget opens an optional streaming save target and maps user cancel to null.
function prepareStreamingSaveTarget(filename: string): StreamingSaveTarget {
  const saveFilePicker = browserSaveFilePicker();
  if (!saveFilePicker) {
    return null;
  }
  return saveFilePicker({
    suggestedName: filename,
    types: [
      {
        description: 'ZIP archive',
        accept: { 'application/zip': ['.zip'] },
      },
    ],
  }).catch((error) => {
    if (error instanceof DOMException && error.name === 'AbortError') {
      return null;
    }
    throw error;
  });
}

// saveDownloadResponse writes the ZIP through streaming save when possible and falls back to blob download.
async function saveDownloadResponse(
  response: Response,
  filename: string,
  saveTarget: StreamingSaveTarget,
): Promise<BulkDownloadResult> {
  if (response.body && saveTarget) {
    try {
      const handle = await saveTarget;
      if (!handle) {
        await response.body.cancel();
        return 'cancelled';
      }
      const writable = await handle.createWritable().catch(async (error) => {
        await response.body?.cancel();
        throw error;
      });
      await response.body.pipeTo(writable);
      return 'saved';
    } catch (error) {
      if (error instanceof DOMException && error.name === 'AbortError') {
        return 'cancelled';
      }
      throw error;
    }
  }

  const blob = await response.blob();
  saveBlob(blob, filename);
  return 'saved';
}

// saveBlob triggers the browser fallback download for non-streaming environments.
function saveBlob(blob: Blob, filename: string): void {
  const url = URL.createObjectURL(blob);
  const a = document.createElement('a');
  a.href = url;
  a.download = filename;
  document.body.appendChild(a);
  a.click();
  document.body.removeChild(a);
  URL.revokeObjectURL(url);
}
