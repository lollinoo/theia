/**
 * Normalizes backend instance backup payloads into frontend-safe shapes.
 * Keeps API boundary validation close to the transport helpers that consume it.
 */
import {
  type InstanceBackup,
  type InstanceBackupProgress,
  type InstanceBackupStatus,
  type RestoreReport,
  type RestoreStatus,
  type RestoreStatusPhase,
} from '../types/api';

// parseInstanceBackup normalizes backend backup rows while preserving existing field defaults.
export function parseInstanceBackup(data: Record<string, unknown>): InstanceBackup {
  const status = typeof data.status === 'string' ? data.status : 'running';
  const trigger = typeof data.trigger === 'string' ? data.trigger : 'manual';
  const progress = parseInstanceBackupProgress(data.progress);
  return {
    id: typeof data.id === 'string' ? data.id : '',
    file_name: typeof data.file_name === 'string' ? data.file_name : '',
    size_bytes: typeof data.size_bytes === 'number' ? data.size_bytes : 0,
    sha256: typeof data.sha256 === 'string' ? data.sha256 : '',
    app_version: typeof data.app_version === 'string' ? data.app_version : '',
    migration_version: typeof data.migration_version === 'number' ? data.migration_version : 0,
    status: (['running', 'success', 'failed', 'cancelled'].includes(status)
      ? status
      : 'running') as InstanceBackupStatus,
    error_message: typeof data.error_message === 'string' ? data.error_message : '',
    ...(progress ? { progress } : {}),
    trigger: (trigger === 'scheduled' ? 'scheduled' : 'manual') as 'manual' | 'scheduled',
    created_at: typeof data.created_at === 'string' ? data.created_at : '',
  };
}

// parseInstanceBackupProgress preserves optional progress metadata without requiring complete counters.
export function parseInstanceBackupProgress(value: unknown): InstanceBackupProgress | undefined {
  if (!value || typeof value !== 'object') return undefined;
  const record = value as Record<string, unknown>;
  return {
    phase: typeof record.phase === 'string' ? record.phase : '',
    message: typeof record.message === 'string' ? record.message : '',
    current: typeof record.current === 'number' ? record.current : 0,
    total: typeof record.total === 'number' ? record.total : 0,
  };
}

// parseRestoreReport normalizes dry-run and restore responses with zero-value defaults.
export function parseRestoreReport(data: Record<string, unknown>): RestoreReport {
  return {
    valid: typeof data.valid === 'boolean' ? data.valid : false,
    app_version: typeof data.app_version === 'string' ? data.app_version : '',
    git_commit: typeof data.git_commit === 'string' ? data.git_commit : '',
    migration_version: typeof data.migration_version === 'number' ? data.migration_version : 0,
    created_at: typeof data.created_at === 'string' ? data.created_at : '',
    db_size_bytes: typeof data.db_size_bytes === 'number' ? data.db_size_bytes : 0,
    backup_file_count: typeof data.backup_file_count === 'number' ? data.backup_file_count : 0,
    total_size_bytes: typeof data.total_size_bytes === 'number' ? data.total_size_bytes : 0,
    needs_migration: typeof data.needs_migration === 'boolean' ? data.needs_migration : false,
    current_migration_version:
      typeof data.current_migration_version === 'number' ? data.current_migration_version : 0,
    message: typeof data.message === 'string' ? data.message : '',
  };
}

const restoreStatusPhases: RestoreStatusPhase[] = [
  'validation_passed',
  'staged_restart_pending',
  'startup_restore_detected',
  'applying_postgres',
  'postgres_applied',
  'verifying_keyring',
  'running_credential_rewrap',
  'completed',
  'failed_retryable',
  'failed_operator_action_required',
];

/** Parses restore status for the frontend API boundary. */
export function parseRestoreStatus(data: unknown): RestoreStatus | null {
  if (!data || typeof data !== 'object') return null;
  const record = data as Record<string, unknown>;
  const phase = typeof record.phase === 'string' ? record.phase : '';
  return {
    operation_id: typeof record.operation_id === 'string' ? record.operation_id : '',
    phase: (restoreStatusPhases.includes(phase as RestoreStatusPhase)
      ? phase
      : 'validation_passed') as RestoreStatusPhase,
    attempt_count: typeof record.attempt_count === 'number' ? record.attempt_count : 0,
    last_error: typeof record.last_error === 'string' ? record.last_error : '',
    missing_key_id: typeof record.missing_key_id === 'string' ? record.missing_key_id : '',
    created_at: typeof record.created_at === 'string' ? record.created_at : '',
    updated_at: typeof record.updated_at === 'string' ? record.updated_at : '',
  };
}
