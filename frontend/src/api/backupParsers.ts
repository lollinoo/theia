import {
  type BackupFile,
  type BackupFileContent,
  type BackupJob,
  type BackupStatus,
  type BulkBackupRun,
  type BulkBackupRunItem,
  type BulkBackupRunItemStatus,
  type BulkBackupRunStatus,
  type BulkOperationStatus,
} from '../types/api';
import { recordField, stringField } from './parsers';
import type { BulkBackupResult } from './backup';

// parseBackupFile normalizes backup file rows while preserving empty-string and zero defaults.
export function parseBackupFile(data: Record<string, unknown>): BackupFile {
  return {
    id: typeof data.id === 'string' ? data.id : '',
    job_id: typeof data.job_id === 'string' ? data.job_id : '',
    file_type: typeof data.file_type === 'string' ? data.file_type : '',
    file_name: typeof data.file_name === 'string' ? data.file_name : '',
    file_hash: typeof data.file_hash === 'string' ? data.file_hash : '',
    size_bytes: typeof data.size_bytes === 'number' ? data.size_bytes : 0,
    created_at: typeof data.created_at === 'string' ? data.created_at : '',
  };
}

// parseBackupJob normalizes backup jobs and their nested files.
export function parseBackupJob(data: Record<string, unknown>): BackupJob {
  const status = data.status as string;
  const filesRaw = Array.isArray(data.files) ? data.files : [];
  return {
    id: typeof data.id === 'string' ? data.id : '',
    device_id: typeof data.device_id === 'string' ? data.device_id : '',
    status: (['pending', 'running', 'success', 'failed'].includes(status)
      ? status
      : 'pending') as BackupStatus,
    error_message: typeof data.error_message === 'string' ? data.error_message : '',
    created_at: typeof data.created_at === 'string' ? data.created_at : '',
    files: filesRaw.map((file) => parseBackupFile(file as Record<string, unknown>)),
  };
}

// parseBulkBackupResult normalizes legacy bulk-backup enqueue results.
export function parseBulkBackupResult(data: Record<string, unknown>): BulkBackupResult {
  const status = data.status === 'queued' ? 'queued' : 'skipped';
  return {
    device_id: typeof data.device_id === 'string' ? data.device_id : '',
    device_name: typeof data.device_name === 'string' ? data.device_name : '',
    status,
    reason: typeof data.reason === 'string' ? data.reason : undefined,
    job_id: typeof data.job_id === 'string' ? data.job_id : undefined,
  };
}

const bulkBackupRunStatuses: BulkBackupRunStatus[] = [
  'running',
  'pausing',
  'paused',
  'cancelling',
  'success',
  'partial',
  'failed',
  'cancelled',
];

const bulkBackupRunItemStatuses: BulkBackupRunItemStatus[] = [
  'checking',
  'skipped',
  'active',
  'queued',
  'running',
  'success',
  'failed',
  'cancelled',
];

// parseBulkBackupRunItem normalizes one bulk-run item with the existing checking fallback.
export function parseBulkBackupRunItem(data: Record<string, unknown>): BulkBackupRunItem {
  const status = typeof data.status === 'string' ? data.status : '';
  return {
    id: typeof data.id === 'string' ? data.id : '',
    run_id: typeof data.run_id === 'string' ? data.run_id : '',
    device_id: typeof data.device_id === 'string' ? data.device_id : '',
    device_name: typeof data.device_name === 'string' ? data.device_name : '',
    status: bulkBackupRunItemStatuses.includes(status as BulkBackupRunItemStatus)
      ? (status as BulkBackupRunItemStatus)
      : 'checking',
    reason: typeof data.reason === 'string' ? data.reason : undefined,
    backup_job_id: typeof data.backup_job_id === 'string' ? data.backup_job_id : undefined,
    file_count: typeof data.file_count === 'number' ? data.file_count : 0,
    byte_count: typeof data.byte_count === 'number' ? data.byte_count : 0,
    created_at: typeof data.created_at === 'string' ? data.created_at : '',
    updated_at: typeof data.updated_at === 'string' ? data.updated_at : '',
    completed_at: typeof data.completed_at === 'string' ? data.completed_at : undefined,
  };
}

// parseBulkBackupRun normalizes bulk-run counters, state, and nested items.
export function parseBulkBackupRun(data: Record<string, unknown>): BulkBackupRun {
  const status = typeof data.status === 'string' ? data.status : '';
  const items = Array.isArray(data.items) ? data.items : [];
  return {
    id: typeof data.id === 'string' ? data.id : '',
    status: bulkBackupRunStatuses.includes(status as BulkBackupRunStatus)
      ? (status as BulkBackupRunStatus)
      : 'running',
    batch_size: typeof data.batch_size === 'number' ? data.batch_size : 0,
    total_count: typeof data.total_count === 'number' ? data.total_count : 0,
    queued_count: typeof data.queued_count === 'number' ? data.queued_count : 0,
    running_count: typeof data.running_count === 'number' ? data.running_count : 0,
    completed_count: typeof data.completed_count === 'number' ? data.completed_count : 0,
    success_count: typeof data.success_count === 'number' ? data.success_count : 0,
    failed_count: typeof data.failed_count === 'number' ? data.failed_count : 0,
    skipped_count: typeof data.skipped_count === 'number' ? data.skipped_count : 0,
    cancelled_count: typeof data.cancelled_count === 'number' ? data.cancelled_count : 0,
    file_count: typeof data.file_count === 'number' ? data.file_count : 0,
    byte_count: typeof data.byte_count === 'number' ? data.byte_count : 0,
    current_device_id:
      typeof data.current_device_id === 'string' ? data.current_device_id : undefined,
    current_device_name:
      typeof data.current_device_name === 'string' ? data.current_device_name : undefined,
    current_job_id: typeof data.current_job_id === 'string' ? data.current_job_id : undefined,
    error_message: typeof data.error_message === 'string' ? data.error_message : '',
    cancel_requested: data.cancel_requested === true,
    created_by: typeof data.created_by === 'string' ? data.created_by : '',
    created_at: typeof data.created_at === 'string' ? data.created_at : '',
    started_at: typeof data.started_at === 'string' ? data.started_at : undefined,
    completed_at: typeof data.completed_at === 'string' ? data.completed_at : undefined,
    items: items.map((item) => parseBulkBackupRunItem(item as Record<string, unknown>)),
  };
}

// parseBulkBackupRunResponse extracts the nullable run payload used by latest and conflict responses.
export function parseBulkBackupRunResponse(payload: unknown): BulkBackupRun | null {
  const data = (payload as Record<string, unknown>)?.data;
  if (data === null || typeof data !== 'object') return null;
  return parseBulkBackupRun(data as Record<string, unknown>);
}

// numericField reads numeric metadata fields with the existing zero fallback.
function numericField(record: Record<string, unknown> | undefined, key: string): number {
  return record && typeof record[key] === 'number' ? record[key] : 0;
}

// booleanField reads boolean metadata fields with the existing false fallback.
function booleanField(record: Record<string, unknown> | undefined, key: string): boolean {
  return record?.[key] === true;
}

// parseBulkOperationStatus normalizes bulk backup/download capability metadata.
export function parseBulkOperationStatus(payload: unknown): BulkOperationStatus {
  const payloadRecord = recordField(payload) ?? {};
  const data = recordField(payloadRecord.data) ?? {};
  const bulkBackup = recordField(data.bulk_backup) ?? {};
  const bulkBackupConcurrency = recordField(bulkBackup.concurrency) ?? {};
  const bulkBackupLegacyEndpoint = recordField(bulkBackup.legacy_endpoint) ?? {};
  const bulkBackupRun = recordField(data.bulk_backup_run) ?? {};
  const bulkDownload = recordField(data.bulk_download) ?? {};

  return {
    bulk_backup: {
      max_devices: numericField(bulkBackup, 'max_devices'),
      max_queued_jobs: numericField(bulkBackup, 'max_queued_jobs'),
      concurrency: {
        max_concurrent: numericField(bulkBackupConcurrency, 'max_concurrent'),
        configurable: booleanField(bulkBackupConcurrency, 'configurable'),
        distributed: booleanField(bulkBackupConcurrency, 'distributed'),
        distributed_max_concurrent: numericField(
          bulkBackupConcurrency,
          'distributed_max_concurrent',
        ),
      },
      legacy_endpoint: {
        path: stringField(bulkBackupLegacyEndpoint, 'path'),
        deprecated: booleanField(bulkBackupLegacyEndpoint, 'deprecated'),
      },
    },
    bulk_backup_run: {
      max_devices: numericField(bulkBackupRun, 'max_devices'),
      max_queued_jobs: numericField(bulkBackupRun, 'max_queued_jobs'),
      batch_size: numericField(bulkBackupRun, 'batch_size'),
      max_active_runs: numericField(bulkBackupRun, 'max_active_runs'),
      configurable_concurrency: booleanField(bulkBackupRun, 'configurable_concurrency'),
      distributed: booleanField(bulkBackupRun, 'distributed'),
      distributed_max_active_runs: numericField(bulkBackupRun, 'distributed_max_active_runs'),
      can_pause: booleanField(bulkBackupRun, 'can_pause'),
      can_resume: booleanField(bulkBackupRun, 'can_resume'),
      can_cancel: booleanField(bulkBackupRun, 'can_cancel'),
    },
    bulk_download: {
      max_devices: numericField(bulkDownload, 'max_devices'),
      max_files: numericField(bulkDownload, 'max_files'),
      max_bytes: numericField(bulkDownload, 'max_bytes'),
      max_concurrent_per_actor: numericField(bulkDownload, 'max_concurrent_per_actor'),
      max_concurrent_global: numericField(bulkDownload, 'max_concurrent_global'),
      distributed: booleanField(bulkDownload, 'distributed'),
      distributed_max_concurrent_per_actor: numericField(
        bulkDownload,
        'distributed_max_concurrent_per_actor',
      ),
      distributed_max_concurrent_global: numericField(
        bulkDownload,
        'distributed_max_concurrent_global',
      ),
    },
  };
}

// parseBackupFileContent normalizes inline content and accepts a caller-provided fallback URL.
export function parseBackupFileContent(
  data: Record<string, unknown>,
  fallbackDownloadUrl: string,
): BackupFileContent {
  const content = typeof data.content === 'string' ? data.content : '';
  const inline = data.inline !== false;
  const downloadURL =
    typeof data.download_url === 'string' && data.download_url
      ? data.download_url
      : fallbackDownloadUrl;
  const reason = typeof data.reason === 'string' ? data.reason : undefined;

  return {
    content,
    inline,
    download_url: downloadURL,
    ...(reason ? { reason } : {}),
    size_bytes: typeof data.size_bytes === 'number' ? data.size_bytes : 0,
    max_inline_size_bytes:
      typeof data.max_inline_size_bytes === 'number' ? data.max_inline_size_bytes : 0,
  };
}
