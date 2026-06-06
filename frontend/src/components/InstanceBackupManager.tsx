/**
 * Renders instance backup manager UI behavior for the Theia frontend.
 * Keeps this component's state and interaction boundary explicit for maintainers.
 */
import { useCallback, useEffect, useRef, useState } from 'react';
import {
  cancelInstanceBackup,
  createInstanceBackup,
  deleteInstanceBackup,
  fetchInstanceBackups,
  fetchRestoreStatus,
  fetchSettings,
  instanceBackupDownloadUrl,
  restoreInstanceBackup,
  updateSetting,
} from '../api/client';
import { ServerError, ValidationError } from '../api/errors';
import type { InstanceBackup, RestoreReport, RestoreStatus } from '../types/api';
import { validateIntervalAllowlist, validateRetentionCount } from '../utils/validation';

const statusColors: Record<string, string> = {
  success: 'text-status-up',
  failed: 'text-status-down',
  running: 'text-status-probing',
  cancelled: 'text-on-bg-secondary',
};

const statusIcons: Record<string, string> = {
  success: '\u2713', // checkmark
  failed: '\u2717', // X mark
  running: '\u25CF', // filled circle
  cancelled: '\u25CB', // hollow circle
};

/** Renders the InstanceBackupManager component within the UI component boundary. */
export function InstanceBackupManager() {
  const [backups, setBackups] = useState<InstanceBackup[]>([]);
  const [loading, setLoading] = useState(true);
  const [creating, setCreating] = useState(false);
  const [createError, setCreateError] = useState('');
  const [confirmDeleteId, setConfirmDeleteId] = useState<string | null>(null);
  const [expandedErrorId, setExpandedErrorId] = useState<string | null>(null);
  const pollRef = useRef<ReturnType<typeof setInterval> | null>(null);
  const confirmTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  const [restoreFile, setRestoreFile] = useState<File | null>(null);
  const [restoreReport, setRestoreReport] = useState<RestoreReport | null>(null);
  const [restoreError, setRestoreError] = useState('');
  const [restoreStatus, setRestoreStatus] = useState<RestoreStatus | null>(null);
  const [restoreLoading, setRestoreLoading] = useState(false);
  const [restoreConfirmed, setRestoreConfirmed] = useState(false);
  const [showRestoreModal, setShowRestoreModal] = useState(false);
  const fileInputRef = useRef<HTMLInputElement>(null);
  const [scheduleInterval, setScheduleInterval] = useState('0');
  const [retentionCount, setRetentionCount] = useState('5');
  const [savedSchedule, setSavedSchedule] = useState(false);
  const [savedRetention, setSavedRetention] = useState(false);
  const scheduleTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  const retentionTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  const savedScheduleTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  const savedRetentionTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  const [fieldErrors, setFieldErrors] = useState<Record<string, string>>({});

  const stopPolling = useCallback(() => {
    if (pollRef.current) {
      clearInterval(pollRef.current);
      pollRef.current = null;
    }
  }, []);

  const startPolling = useCallback(() => {
    stopPolling();
    pollRef.current = setInterval(async () => {
      try {
        const data = await fetchInstanceBackups();
        setBackups(data);
        const running = data.find((b) => b.status === 'running');
        if (!running) {
          stopPolling();
          setCreating(false);
        }
      } catch {
        // ignore poll errors
      }
    }, 2000);
  }, [stopPolling]);

  // Initial load + resume polling if a backup is already running
  useEffect(() => {
    void (async () => {
      try {
        const data = await fetchInstanceBackups();
        setBackups(data);
        const running = data.find((b) => b.status === 'running');
        if (running) {
          setCreating(true);
          startPolling();
        }
        const settings = await fetchSettings();
        if (settings['instance_backup_interval_hours'] !== undefined) {
          setScheduleInterval(settings['instance_backup_interval_hours']);
        }
        if (settings['instance_backup_retention_count'] !== undefined) {
          setRetentionCount(settings['instance_backup_retention_count']);
        }
        setRestoreStatus(await fetchRestoreStatus());
      } catch {
        // non-fatal
      } finally {
        setLoading(false);
      }
    })();
    return () => stopPolling();
  }, [startPolling, stopPolling]);

  // Re-fetch when backend reconnects (e.g. after restore restart)
  useEffect(() => {
    const handleReconnect = () => {
      void (async () => {
        try {
          const data = await fetchInstanceBackups();
          setBackups(data);
          setRestoreStatus(await fetchRestoreStatus());
          setCreating(false);
        } catch {
          // non-fatal
        }
      })();
    };
    window.addEventListener('backend-reconnected', handleReconnect);
    return () => window.removeEventListener('backend-reconnected', handleReconnect);
  }, []);

  // Cleanup on unmount
  useEffect(() => {
    return () => {
      stopPolling();
      if (confirmTimerRef.current) clearTimeout(confirmTimerRef.current);
      if (scheduleTimerRef.current) clearTimeout(scheduleTimerRef.current);
      if (retentionTimerRef.current) clearTimeout(retentionTimerRef.current);
      if (savedScheduleTimerRef.current) clearTimeout(savedScheduleTimerRef.current);
      if (savedRetentionTimerRef.current) clearTimeout(savedRetentionTimerRef.current);
    };
  }, [stopPolling]);

  function setFieldError(field: string, error: string | null) {
    setFieldErrors((prev) => {
      if (error) return { ...prev, [field]: error };
      const next = { ...prev };
      delete next[field];
      return next;
    });
  }

  function showSaved(
    setter: (v: boolean) => void,
    timerRef: React.MutableRefObject<ReturnType<typeof setTimeout> | null>,
  ) {
    setter(true);
    if (timerRef.current !== null) clearTimeout(timerRef.current);
    timerRef.current = setTimeout(() => setter(false), 2000);
  }

  function handleScheduleChange(value: string) {
    const err = validateIntervalAllowlist(value);
    if (err) {
      setFieldError('scheduleInterval', err);
      setScheduleInterval(value);
      return;
    }
    setFieldError('scheduleInterval', null);
    setScheduleInterval(value);
    if (scheduleTimerRef.current !== null) clearTimeout(scheduleTimerRef.current);
    scheduleTimerRef.current = setTimeout(() => {
      void updateSetting('instance_backup_interval_hours', value).then(() =>
        showSaved(setSavedSchedule, savedScheduleTimerRef),
      );
    }, 500);
  }

  function handleRetentionChange(value: string) {
    const err = validateRetentionCount(value);
    if (err) {
      setFieldError('retentionCount', err);
      setRetentionCount(value);
      return;
    }
    setFieldError('retentionCount', null);
    setRetentionCount(value);
    if (retentionTimerRef.current !== null) clearTimeout(retentionTimerRef.current);
    const num = parseInt(value, 10);
    retentionTimerRef.current = setTimeout(() => {
      void updateSetting('instance_backup_retention_count', String(num)).then(() =>
        showSaved(setSavedRetention, savedRetentionTimerRef),
      );
    }, 500);
  }

  function formatDuration(totalSeconds: number): string {
    const hours = Math.floor(totalSeconds / 3600);
    const minutes = Math.floor((totalSeconds % 3600) / 60);
    if (hours > 24) {
      const days = Math.floor(hours / 24);
      const remHours = hours % 24;
      return remHours > 0 ? `${days}d ${remHours}h` : `${days}d`;
    }
    if (hours > 0) return minutes > 0 ? `${hours}h ${minutes}m` : `${hours}h`;
    return `${minutes}m`;
  }

  function computeNextBackupText(): string {
    const intervalHours = parseInt(scheduleInterval, 10);
    if (!intervalHours || intervalHours <= 0) return 'Scheduling disabled';

    const lastSuccessful = backups.find((b) => b.status === 'success');
    if (!lastSuccessful) return 'First backup in ~' + formatDuration(intervalHours * 3600);

    const lastTime = new Date(lastSuccessful.created_at).getTime();
    const nextTime = lastTime + intervalHours * 3600 * 1000;
    const nowMs = Date.now();
    const remainingSec = Math.max(0, Math.floor((nextTime - nowMs) / 1000));

    if (remainingSec <= 0) return 'Next backup: soon';
    return 'Next backup: in ~' + formatDuration(remainingSec);
  }

  const handleCreate = async () => {
    setCreating(true);
    setCreateError('');
    try {
      await createInstanceBackup();
      startPolling();
    } catch (err) {
      if (err instanceof ServerError) {
        const msg = err.correlationId
          ? `Something went wrong (ref: ${err.correlationId})`
          : 'Something went wrong';
        if (msg.includes('409') || msg.toLowerCase().includes('already in progress')) {
          setCreateError('A backup is already in progress.');
        } else {
          setCreateError(msg);
        }
      } else if (err instanceof ValidationError) {
        setCreateError(err.message);
      } else {
        const message = err instanceof Error ? err.message : 'Backup failed';
        if (message.includes('409') || message.toLowerCase().includes('already in progress')) {
          setCreateError('A backup is already in progress.');
        } else {
          setCreateError(message);
        }
      }
      setCreating(false);
    }
  };

  const handleDeleteClick = (id: string) => {
    if (confirmDeleteId === id) {
      // Second click -- execute delete
      void handleDelete(id);
      return;
    }
    // First click -- show "Confirm?" for 3 seconds
    setConfirmDeleteId(id);
    if (confirmTimerRef.current) clearTimeout(confirmTimerRef.current);
    confirmTimerRef.current = setTimeout(() => {
      setConfirmDeleteId(null);
    }, 3000);
  };

  const handleDelete = async (id: string) => {
    setConfirmDeleteId(null);
    try {
      await deleteInstanceBackup(id);
      setBackups((prev) => prev.filter((b) => b.id !== id));
    } catch {
      // non-fatal
    }
  };

  const handleCancel = async (id: string) => {
    try {
      const cancelled = await cancelInstanceBackup(id);
      setBackups((prev) => prev.map((backup) => (backup.id === id ? cancelled : backup)));
      setCreating(cancelled.status === 'running');
      startPolling();
    } catch (err) {
      const message = err instanceof Error ? err.message : 'Cancel failed';
      setCreateError(message);
    }
  };

  const handleRestoreClick = () => {
    fileInputRef.current?.click();
  };

  const handleFileSelected = async (e: React.ChangeEvent<HTMLInputElement>) => {
    const file = e.target.files?.[0];
    if (!file) return;
    // Reset input so same file can be selected again
    e.target.value = '';

    setRestoreFile(file);
    setRestoreError('');
    setRestoreLoading(true);
    setRestoreReport(null);
    setRestoreConfirmed(false);

    try {
      // Dry-run validation first (D-11)
      const report = await restoreInstanceBackup(file, true);
      setRestoreReport(report);
      setShowRestoreModal(true);
    } catch (err) {
      if (err instanceof ServerError) {
        const msg = err.correlationId
          ? `Something went wrong (ref: ${err.correlationId})`
          : 'Something went wrong';
        setRestoreError(msg);
      } else if (err instanceof ValidationError) {
        setRestoreError(err.message);
      } else {
        const message = err instanceof Error ? err.message : 'Validation failed';
        setRestoreError(message);
      }
    } finally {
      setRestoreLoading(false);
    }
  };

  const handleRestoreConfirm = async () => {
    if (!restoreFile || !restoreConfirmed) return;
    setRestoreLoading(true);
    setRestoreError('');

    try {
      await restoreInstanceBackup(restoreFile, false);
      setShowRestoreModal(false);
      setRestoreError('');
      setRestoreFile(null);
      setRestoreReport(null);
      setRestoreConfirmed(false);
      setCreateError('Restore staged. Restart pending.');
    } catch (err) {
      if (err instanceof TypeError) {
        setRestoreError('Restore request was interrupted. Reconnect and check restore status.');
        return;
      }
      const message = err instanceof Error ? err.message : 'Restore failed';
      setRestoreError(message);
    } finally {
      setRestoreLoading(false);
    }
  };

  const handleRestoreCancel = () => {
    setShowRestoreModal(false);
    setRestoreFile(null);
    setRestoreReport(null);
    setRestoreConfirmed(false);
    setRestoreError('');
  };

  const formatBytes = (bytes: number) => {
    if (bytes === 0) return '0 B';
    if (bytes < 1024) return `${bytes} B`;
    if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`;
    return `${(bytes / (1024 * 1024)).toFixed(1)} MB`;
  };

  const formatDate = (dateStr: string) => {
    if (!dateStr) return 'N/A';
    try {
      return new Date(dateStr).toLocaleString();
    } catch {
      return dateStr;
    }
  };

  const formatSize = (bytes: number) => {
    if (bytes === 0) return '\u2014'; // em dash for zero/unknown
    if (bytes < 1024) return `${bytes} B`;
    if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`;
    return `${(bytes / (1024 * 1024)).toFixed(1)} MB`;
  };

  const restoreStatusMessages = (status: RestoreStatus | null): string[] => {
    if (!status) return [];
    switch (status.phase) {
      case 'staged_restart_pending':
        return ['Restore staged. Restart pending.'];
      case 'startup_restore_detected':
      case 'applying_postgres':
      case 'postgres_applied':
      case 'verifying_keyring':
      case 'running_credential_rewrap':
        return ['Restore applying on startup.'];
      case 'completed':
        return ['Restore completed.'];
      case 'failed_retryable':
        return ['Restore failed but can retry on restart.', status.last_error].filter(Boolean);
      case 'failed_operator_action_required': {
        const keyId = status.missing_key_id || 'unknown';
        const guidance =
          keyId === 'legacy'
            ? 'Add legacy=<old secret> to THEIA_ENCRYPTION_KEYS or set THEIA_ENCRYPTION_KEY as fallback, restart, then create and restore-test a fresh backup.'
            : `Add ${keyId}=<old secret> to THEIA_ENCRYPTION_KEYS, restart, then create and restore-test a fresh backup.`;
        return [
          `Restore blocked because key id ${keyId} is missing from THEIA_ENCRYPTION_KEYS.`,
          guidance,
        ];
      }
      default:
        return [];
    }
  };

  const restoreMessages = restoreStatusMessages(restoreStatus);

  const formatProgress = (backup: InstanceBackup): string => {
    if (!backup.progress) return '';
    const message = backup.progress.message || backup.progress.phase;
    if (!backup.progress.total) return message;
    const percentage = Math.max(
      0,
      Math.min(100, Math.round((backup.progress.current / backup.progress.total) * 100)),
    );
    return message ? `${message} (${percentage}%)` : `${percentage}%`;
  };

  const hasRunning = backups.some((b) => b.status === 'running') || creating;

  return (
    <div className="space-y-3 transition-colors duration-200">
      {/* HEADER ROW */}
      <div className="flex items-center justify-between">
        <p className="text-xs font-medium uppercase tracking-widest text-on-bg-secondary">
          Instance Backups
        </p>
        <div className="flex items-center gap-2">
          {/* Hidden file input for restore */}
          <input
            ref={fileInputRef}
            type="file"
            accept=".tar.gz"
            className="hidden"
            onChange={(e) => {
              void handleFileSelected(e);
            }}
          />
          <button
            type="button"
            onClick={handleRestoreClick}
            disabled={restoreLoading || hasRunning}
            className="flex items-center gap-1 rounded-lg border border-primary/30 bg-transparent px-2.5 py-1 text-xs font-medium text-primary hover:bg-primary/10 disabled:opacity-50 transition-colors"
          >
            {restoreLoading ? (
              <>
                <svg className="w-3.5 h-3.5 animate-spin" fill="none" viewBox="0 0 24 24">
                  <circle
                    className="opacity-25"
                    cx="12"
                    cy="12"
                    r="10"
                    stroke="currentColor"
                    strokeWidth="4"
                  />
                  <path
                    className="opacity-75"
                    fill="currentColor"
                    d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4z"
                  />
                </svg>
                Validating...
              </>
            ) : (
              'Restore Backup'
            )}
          </button>
          <button
            type="button"
            onClick={() => {
              void handleCreate();
            }}
            disabled={hasRunning}
            className="flex items-center gap-1 rounded-lg bg-primary px-2.5 py-1 text-xs font-medium text-white hover:bg-primary/90 disabled:opacity-50 transition-colors"
          >
            {creating ? (
              <>
                <svg className="w-3.5 h-3.5 animate-spin" fill="none" viewBox="0 0 24 24">
                  <circle
                    className="opacity-25"
                    cx="12"
                    cy="12"
                    r="10"
                    stroke="currentColor"
                    strokeWidth="4"
                  />
                  <path
                    className="opacity-75"
                    fill="currentColor"
                    d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4z"
                  />
                </svg>
                Creating...
              </>
            ) : (
              'Create Backup'
            )}
          </button>
        </div>
      </div>

      {/* SCHEDULE & RETENTION SETTINGS */}
      <div className="rounded-lg bg-surface-high p-3 space-y-3">
        {/* Schedule Interval Dropdown */}
        <div className="space-y-1">
          <div className="flex items-center justify-between">
            <label className="text-[11px] font-medium text-on-bg-secondary">
              Automatic Backup Schedule
            </label>
            {savedSchedule && <span className="text-[10px] text-status-up font-medium">Saved</span>}
          </div>
          <select
            value={scheduleInterval}
            onChange={(e) => handleScheduleChange(e.target.value)}
            className="w-full rounded-lg border border-outline-subtle bg-elevated px-2.5 py-1.5 text-xs text-on-bg focus:border-primary focus:ring-1 focus:ring-primary/30 focus:outline-none"
          >
            <option value="0">Disabled</option>
            <option value="6">Every 6 hours</option>
            <option value="12">Every 12 hours</option>
            <option value="24">Every 24 hours</option>
            <option value="48">Every 48 hours</option>
            <option value="168">Every 7 days</option>
          </select>
          {/* Next backup helper text */}
          <p className="text-[10px] text-on-bg-muted">{computeNextBackupText()}</p>
        </div>

        {/* Retention Count Input */}
        <div className="space-y-1">
          <div className="flex items-center justify-between">
            <label className="text-[11px] font-medium text-on-bg-secondary">
              Keep last N backups
            </label>
            {savedRetention && (
              <span className="text-[10px] text-status-up font-medium">Saved</span>
            )}
          </div>
          <input
            type="number"
            min={1}
            max={365}
            value={retentionCount}
            onChange={(e) => handleRetentionChange(e.target.value)}
            className={`w-full rounded-lg border bg-elevated px-2.5 py-1.5 text-xs text-on-bg focus:border-primary focus:ring-1 focus:ring-primary/30 focus:outline-none${fieldErrors.retentionCount ? ' border-status-down' : ' border-outline-subtle'}`}
          />
          {fieldErrors.retentionCount && (
            <p className="mt-0.5 text-[10px] text-status-down">{fieldErrors.retentionCount}</p>
          )}
        </div>
      </div>

      {/* ERROR MESSAGE */}
      {createError && (
        <div className="rounded-md border border-status-down/20 bg-status-down/5 p-2 text-xs text-status-down">
          {createError}
        </div>
      )}

      {restoreMessages.length > 0 && (
        <div className="rounded-md border border-outline-subtle bg-surface-high p-2 text-xs text-on-bg-secondary">
          {restoreMessages.map((message) => (
            <p key={message}>{message}</p>
          ))}
        </div>
      )}

      {/* RESTORE ERROR */}
      {restoreError && !showRestoreModal && (
        <div className="rounded-md border border-status-down/20 bg-status-down/5 p-2 text-xs text-status-down">
          {restoreError}
        </div>
      )}

      {/* LOADING STATE */}
      {loading && <p className="text-xs text-on-bg-secondary">Loading backups...</p>}

      {/* EMPTY STATE */}
      {!loading && backups.length === 0 && (
        <p className="text-xs text-on-bg-secondary">
          No instance backups yet. Create one to back up the entire Theia database and
          configuration.
        </p>
      )}

      {/* BACKUP LIST */}
      {!loading && backups.length > 0 && (
        <div className="space-y-1.5">
          {backups.map((backup) => (
            <div key={backup.id} className="rounded-lg bg-surface-high p-2.5 space-y-1">
              {/* Row 1: Status + Filename + Date */}
              <div className="flex items-center justify-between gap-2">
                <div className="flex items-center gap-1.5 min-w-0 flex-1">
                  <span
                    className={`text-xs font-bold ${statusColors[backup.status] ?? 'text-on-bg-secondary'}`}
                  >
                    {statusIcons[backup.status] ?? '?'}
                  </span>
                  <span className="text-xs text-on-bg font-mono truncate">
                    {backup.file_name || 'Backup in progress...'}
                  </span>
                  {backup.trigger === 'scheduled' && (
                    <span className="shrink-0 rounded bg-primary/10 px-1 py-0.5 text-[9px] font-medium text-primary">
                      scheduled
                    </span>
                  )}
                </div>
                <span className="text-[10px] text-on-bg-secondary font-mono shrink-0">
                  {formatDate(backup.created_at)}
                </span>
              </div>

              {/* Row 2: Size + Version + Actions */}
              <div className="flex items-center justify-between">
                <div className="flex items-center gap-3 text-[10px] text-on-bg-secondary">
                  <span className="font-mono">
                    {backup.status === 'failed' ? '\u2014' : formatSize(backup.size_bytes)}
                  </span>
                  {backup.app_version && <span>v{backup.app_version}</span>}
                </div>
                <div className="flex items-center gap-1">
                  {/* Download button -- only for successful backups */}
                  {backup.status === 'success' && (
                    <a
                      href={instanceBackupDownloadUrl(backup.id)}
                      download
                      className="rounded px-2 py-0.5 text-[10px] font-medium text-primary border border-primary/30 hover:bg-primary/10 transition-colors"
                    >
                      Download
                    </a>
                  )}
                  {backup.status === 'running' && (
                    <button
                      type="button"
                      onClick={() => {
                        void handleCancel(backup.id);
                      }}
                      className="rounded px-2 py-0.5 text-[10px] font-medium text-warning border border-warning/30 hover:bg-warning/10 transition-colors"
                    >
                      Cancel
                    </button>
                  )}
                  {/* Delete button with inline confirm */}
                  <button
                    type="button"
                    onClick={() => handleDeleteClick(backup.id)}
                    disabled={backup.status === 'running'}
                    className={`rounded px-2 py-0.5 text-[10px] font-medium transition-colors disabled:opacity-30 ${
                      confirmDeleteId === backup.id
                        ? 'text-white bg-status-down border border-status-down'
                        : 'text-status-down border border-status-down/30 hover:bg-status-down/10'
                    }`}
                  >
                    {confirmDeleteId === backup.id ? 'Confirm?' : 'Delete'}
                  </button>
                </div>
              </div>

              {/* Error message -- expandable on click */}
              {backup.status === 'failed' && backup.error_message && (
                <button
                  type="button"
                  className="block w-full cursor-pointer text-left"
                  onClick={() =>
                    setExpandedErrorId(expandedErrorId === backup.id ? null : backup.id)
                  }
                >
                  <p
                    className={`text-[10px] text-status-down ${expandedErrorId === backup.id ? '' : 'truncate'}`}
                  >
                    {backup.error_message}
                  </p>
                </button>
              )}
              {backup.status === 'running' && backup.progress && (
                <div className="text-[10px] text-on-bg-secondary">{formatProgress(backup)}</div>
              )}
            </div>
          ))}
        </div>
      )}

      {/* RESTORE CONFIRMATION MODAL */}
      {showRestoreModal && restoreReport && (
        <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/50">
          <div className="mx-4 w-full max-w-md rounded-xl bg-surface-high p-5 shadow-xl space-y-4">
            <h3 className="text-sm font-semibold text-on-bg">Confirm Restore</h3>

            {/* Manifest details */}
            <div className="space-y-1.5 rounded-lg bg-bg/50 p-3 text-xs">
              <div className="flex justify-between">
                <span className="text-on-bg-secondary">App Version</span>
                <span className="font-mono text-on-bg">v{restoreReport.app_version}</span>
              </div>
              <div className="flex justify-between">
                <span className="text-on-bg-secondary">Migration Version</span>
                <span className="font-mono text-on-bg">{restoreReport.migration_version}</span>
              </div>
              <div className="flex justify-between">
                <span className="text-on-bg-secondary">Created</span>
                <span className="font-mono text-on-bg">{formatDate(restoreReport.created_at)}</span>
              </div>
              <div className="flex justify-between">
                <span className="text-on-bg-secondary">Database Size</span>
                <span className="font-mono text-on-bg">
                  {formatBytes(restoreReport.db_size_bytes)}
                </span>
              </div>
              <div className="flex justify-between">
                <span className="text-on-bg-secondary">Backup Files</span>
                <span className="font-mono text-on-bg">{restoreReport.backup_file_count}</span>
              </div>
              {restoreReport.needs_migration && (
                <div className="flex justify-between text-status-probing">
                  <span>Migration Required</span>
                  <span className="font-mono">
                    {restoreReport.migration_version} -&gt;{' '}
                    {restoreReport.current_migration_version}
                  </span>
                </div>
              )}
            </div>

            {/* Warning */}
            <p className="text-xs text-status-down font-medium">
              This will replace your entire database and restart the application.
            </p>

            {/* Checkbox (D-12) */}
            <label className="flex items-start gap-2 cursor-pointer">
              <input
                type="checkbox"
                checked={restoreConfirmed}
                onChange={(e) => setRestoreConfirmed(e.target.checked)}
                className="mt-0.5 rounded border-border"
              />
              <span className="text-xs text-on-bg-secondary">
                I understand this will replace all data and restart the application
              </span>
            </label>

            {restoreError && (
              <div className="rounded-md border border-status-down/20 bg-status-down/5 p-2 text-xs text-status-down">
                {restoreError}
              </div>
            )}

            {/* Actions */}
            <div className="flex justify-end gap-2">
              <button
                type="button"
                onClick={handleRestoreCancel}
                className="rounded-lg border border-border px-3 py-1.5 text-xs font-medium text-on-bg-secondary hover:bg-elevated/50 transition-colors"
              >
                Cancel
              </button>
              <button
                type="button"
                onClick={() => {
                  void handleRestoreConfirm();
                }}
                disabled={!restoreConfirmed || restoreLoading}
                className="rounded-lg bg-status-down px-3 py-1.5 text-xs font-medium text-white hover:bg-status-down/90 disabled:opacity-50 transition-colors"
              >
                {restoreLoading ? 'Restoring...' : 'Restore Now'}
              </button>
            </div>
          </div>
        </div>
      )}
    </div>
  );
}
