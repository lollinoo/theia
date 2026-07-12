/**
 * Defines backup panel behavior for the operations dashboard.
 * Keeps table, backup, and device-management responsibilities isolated by module.
 */
import { useCallback, useEffect, useRef, useState } from 'react';
import {
  fetchBackupJob,
  fetchBackupJobs,
  fetchLatestBackupJob,
  resetSSHHostKey,
  triggerBackup,
} from '../../api/client';
import { type BackupJob, type Device } from '../../types/api';

interface BackupPanelProps {
  device: Device;
}

function isActiveBackupJob(job: BackupJob): boolean {
  return job.status === 'pending' || job.status === 'running';
}

/** Renders the BackupPanel component within the operations dashboard. */
export function BackupPanel({ device }: BackupPanelProps) {
  const [latest, setLatest] = useState<BackupJob | null>(null);
  const [triggering, setTriggering] = useState(false);
  const [triggerResult, setTriggerResult] = useState<BackupJob | null>(null);
  const [error, setError] = useState('');
  const [latestError, setLatestError] = useState('');
  const [hostKeyResetMessage, setHostKeyResetMessage] = useState('');
  const [hostKeyResetError, setHostKeyResetError] = useState('');
  const [resettingHostKey, setResettingHostKey] = useState(false);
  const pollRef = useRef<ReturnType<typeof setInterval> | null>(null);
  const latestRequestRef = useRef(0);
  const activeDeviceIdRef = useRef(device.id);

  const backupSupported = device.backup_supported;

  const loadLatest = useCallback(async () => {
    const requestedDeviceId = device.id;
    if (activeDeviceIdRef.current !== requestedDeviceId) return;
    const requestId = ++latestRequestRef.current;
    try {
      const job = await fetchLatestBackupJob(requestedDeviceId);
      if (
        activeDeviceIdRef.current !== requestedDeviceId ||
        latestRequestRef.current !== requestId
      ) {
        return;
      }
      setLatest(job);
      setLatestError('');
    } catch (err) {
      if (
        activeDeviceIdRef.current !== requestedDeviceId ||
        latestRequestRef.current !== requestId
      ) {
        return;
      }
      setLatest(null);
      setLatestError(
        err instanceof Error
          ? `Failed to load latest backup: ${err.message}`
          : 'Failed to load latest backup',
      );
    }
  }, [device.id]);

  useEffect(() => {
    activeDeviceIdRef.current = device.id;
    setLatest(null);
    setLatestError('');
    void loadLatest();
    return () => {
      latestRequestRef.current += 1;
    };
  }, [device.id, loadLatest]);

  // Cleanup polling on unmount
  useEffect(() => {
    return () => {
      if (pollRef.current) clearInterval(pollRef.current);
    };
  }, []);

  const startPolling = useCallback(
    (jobId: string) => {
      if (pollRef.current) clearInterval(pollRef.current);
      pollRef.current = setInterval(async () => {
        try {
          const job = await fetchBackupJob(jobId);
          setTriggerResult(job);
          if (job.status === 'success' || job.status === 'failed') {
            if (pollRef.current) clearInterval(pollRef.current);
            pollRef.current = null;
            loadLatest();
          }
        } catch {
          // ignore poll errors
        }
      }, 2000);
    },
    [loadLatest],
  );

  useEffect(() => {
    let cancelled = false;
    if (pollRef.current) {
      clearInterval(pollRef.current);
      pollRef.current = null;
    }
    setTriggerResult(null);
    setError('');
    setHostKeyResetMessage('');
    setHostKeyResetError('');

    const loadActiveBackupJob = async () => {
      try {
        const jobs = await fetchBackupJobs(device.id);
        if (cancelled) return;
        const activeJob = jobs.find(isActiveBackupJob);
        if (!activeJob) return;
        setTriggerResult(activeJob);
        startPolling(activeJob.id);
      } catch {
        // The latest-successful backup section is still useful if active-job loading fails.
      }
    };

    void loadActiveBackupJob();
    return () => {
      cancelled = true;
    };
  }, [device.id, startPolling]);

  const handleBackup = async () => {
    setTriggering(true);
    setError('');
    setHostKeyResetMessage('');
    setHostKeyResetError('');
    setTriggerResult(null);
    try {
      const result = await triggerBackup(device.id);
      setTriggerResult(result);
      startPolling(result.id);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Backup failed');
    } finally {
      setTriggering(false);
    }
  };

  const handleResetSSHHostKey = async () => {
    const confirmed = window.confirm(
      'Reset the saved SSH host key for this device? Continue only if the node was intentionally replaced.',
    );
    if (!confirmed) return;

    setResettingHostKey(true);
    setHostKeyResetMessage('');
    setHostKeyResetError('');
    try {
      await resetSSHHostKey(device.id);
      setHostKeyResetMessage('SSH host key reset. Run backup again to trust the new key.');
    } catch (err) {
      setHostKeyResetError(err instanceof Error ? err.message : 'Failed to reset SSH host key');
    } finally {
      setResettingHostKey(false);
    }
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
    if (bytes === 0) return '0 B';
    if (bytes < 1024) return `${bytes} B`;
    if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`;
    return `${(bytes / (1024 * 1024)).toFixed(1)} MB`;
  };

  const totalSize = latest?.files?.reduce((sum, f) => sum + f.size_bytes, 0) ?? 0;
  const activeBackupInProgress = triggerResult ? isActiveBackupJob(triggerResult) : false;
  const hasHostKeyMismatch =
    triggerResult?.status === 'failed' && triggerResult.error_code === 'ssh_host_key_mismatch';

  return (
    <div className="space-y-4 transition-colors duration-200">
      {/* Device info */}
      <div className="rounded-lg bg-surface-high p-3">
        <div className="text-xs text-on-bg-secondary mb-1">Device</div>
        <div className="text-sm text-on-bg font-medium">
          {device.tags?.display_name || device.sys_name || device.hostname || device.ip}
        </div>
        <div className="text-xs text-on-bg-secondary mt-1">
          {device.vendor} / {device.device_type}
        </div>
      </div>

      {/* Vendor backup support check */}
      {!backupSupported && (
        <div className="rounded-md border border-warning/30 bg-warning/10 p-3 text-xs text-warning">
          Backups are not supported for this device's vendor.
        </div>
      )}

      {/* Backup button */}
      {backupSupported && (
        <>
          <div className="rounded-lg bg-surface-high p-3">
            <div className="text-xs text-on-bg-secondary mb-1.5">Backup creates 4 files:</div>
            <div className="text-[10px] text-on-bg-secondary space-y-0.5">
              <div>Export (default .rsc)</div>
              <div>Export Verbose (.rsc)</div>
              <div>Export Compact (.rsc)</div>
              <div>Binary Backup (.backup)</div>
            </div>
          </div>

          <button
            type="button"
            onClick={handleBackup}
            disabled={triggering || activeBackupInProgress}
            className="w-full rounded-md bg-primary px-3 py-2.5 text-xs font-medium text-white hover:bg-primary/90 disabled:opacity-50 transition-colors"
          >
            {triggering
              ? 'Starting backup...'
              : activeBackupInProgress
                ? 'Backup in progress...'
                : 'Backup Now'}
          </button>
        </>
      )}

      {/* Trigger result */}
      {triggerResult && (
        <div
          className={`rounded-md border p-3 ${
            triggerResult.status === 'failed'
              ? 'border-status-down/20 bg-status-down/5'
              : triggerResult.status === 'success'
                ? 'border-status-up/20 bg-status-up/5'
                : 'border-primary/20 bg-primary/5'
          }`}
        >
          <div
            className={`text-xs font-medium mb-1 ${
              triggerResult.status === 'failed'
                ? 'text-status-down'
                : triggerResult.status === 'success'
                  ? 'text-status-up'
                  : 'text-primary'
            }`}
          >
            {triggerResult.status === 'pending' && 'Backup queued...'}
            {triggerResult.status === 'running' && 'Backup in progress...'}
            {triggerResult.status === 'success' &&
              `Backup complete — ${triggerResult.files?.length ?? 0} files`}
            {triggerResult.status === 'failed' && 'Backup failed'}
          </div>
          {triggerResult.error_message && (
            <div className="text-[10px] text-on-bg-secondary mt-1">
              {triggerResult.error_message}
            </div>
          )}
          {hasHostKeyMismatch && (
            <div className="mt-3 rounded-md border border-warning/30 bg-warning/10 p-3">
              <div className="text-xs font-medium text-warning">SSH host key changed</div>
              <div className="mt-1 text-[10px] text-on-bg-secondary">
                Reset the saved key only after confirming this device was replaced.
              </div>
              <button
                type="button"
                onClick={handleResetSSHHostKey}
                disabled={resettingHostKey}
                className="mt-2 rounded px-2 py-1 text-[10px] font-medium text-warning border border-warning/40 hover:bg-warning/10 disabled:opacity-50 transition-colors"
              >
                {resettingHostKey ? 'Resetting...' : 'Reset SSH host key'}
              </button>
              {hostKeyResetMessage && (
                <div className="mt-2 text-[10px] text-status-up">{hostKeyResetMessage}</div>
              )}
              {hostKeyResetError && (
                <div className="mt-2 text-[10px] text-status-down">{hostKeyResetError}</div>
              )}
            </div>
          )}
        </div>
      )}

      {error && (
        <div className="rounded-md border border-status-down/20 bg-status-down/5 p-3 text-xs text-status-down">
          {error}
        </div>
      )}

      {/* Latest backup info */}
      <div>
        <div className="text-xs font-medium text-on-bg-secondary uppercase tracking-[0.12em] mb-2">
          Latest Successful Backup
        </div>
        {latestError && (
          <div className="mb-2 rounded-md border border-status-down/20 bg-status-down/5 p-3 text-xs text-status-down">
            {latestError}
          </div>
        )}
        {latest ? (
          <div className="rounded-lg bg-surface-high p-3 space-y-1.5">
            <div className="flex justify-between text-xs">
              <span className="text-on-bg-secondary">Date</span>
              <span className="text-on-bg font-mono text-[11px]">
                {formatDate(latest.created_at)}
              </span>
            </div>
            <div className="flex justify-between text-xs">
              <span className="text-on-bg-secondary">Files</span>
              <span className="text-on-bg font-mono text-[11px]">
                {latest.files?.length ?? 0} files
              </span>
            </div>
            <div className="flex justify-between text-xs">
              <span className="text-on-bg-secondary">Total Size</span>
              <span className="text-on-bg font-mono text-[11px]">{formatSize(totalSize)}</span>
            </div>
            {latest.error_message && (
              <div className="text-[10px] text-warning mt-1">{latest.error_message}</div>
            )}
          </div>
        ) : (
          <div className="text-xs text-on-bg-secondary">No backups yet</div>
        )}
      </div>
    </div>
  );
}
