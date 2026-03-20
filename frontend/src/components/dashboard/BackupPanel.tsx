import { useState, useEffect, useCallback, useRef } from 'react';
import { type Device, type BackupJob } from '../../types/api';
import { triggerBackup, fetchLatestBackupJob, fetchBackupJob } from '../../api/client';

interface BackupPanelProps {
  device: Device;
}

export function BackupPanel({ device }: BackupPanelProps) {
  const [latest, setLatest] = useState<BackupJob | null>(null);
  const [triggering, setTriggering] = useState(false);
  const [triggerResult, setTriggerResult] = useState<BackupJob | null>(null);
  const [error, setError] = useState('');
  const pollRef = useRef<ReturnType<typeof setInterval> | null>(null);

  const backupSupported = device.backup_supported;

  const loadLatest = useCallback(async () => {
    const job = await fetchLatestBackupJob(device.id);
    setLatest(job);
  }, [device.id]);

  useEffect(() => {
    loadLatest();
  }, [loadLatest]);

  // Cleanup polling on unmount
  useEffect(() => {
    return () => {
      if (pollRef.current) clearInterval(pollRef.current);
    };
  }, []);

  const startPolling = useCallback((jobId: string) => {
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
  }, [loadLatest]);

  const handleBackup = async () => {
    setTriggering(true);
    setError('');
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

  return (
    <div className="space-y-4">
      {/* Device info */}
      <div className="rounded-md border border-border-subtle p-3">
        <div className="text-xs text-text-secondary mb-1">Device</div>
        <div className="text-sm text-text-primary font-medium">
          {device.tags?.display_name || device.sys_name || device.hostname || device.ip}
        </div>
        <div className="text-xs text-text-secondary mt-1">
          {device.vendor} / {device.device_type}
        </div>
      </div>

      {/* Vendor backup support check */}
      {!backupSupported && (
        <div className="rounded-md border border-yellow-500/30 bg-yellow-500/10 p-3 text-xs text-yellow-400">
          Backups are not supported for this device's vendor.
        </div>
      )}

      {/* Backup button */}
      {backupSupported && (
        <>
          <div className="rounded-md border border-border-subtle bg-bg-elevated/30 p-3">
            <div className="text-xs text-text-secondary mb-1.5">Backup creates 4 files:</div>
            <div className="text-[10px] text-text-secondary/70 space-y-0.5">
              <div>Export (default .rsc)</div>
              <div>Export Verbose (.rsc)</div>
              <div>Export Compact (.rsc)</div>
              <div>Binary Backup (.backup)</div>
            </div>
          </div>

          <button
            onClick={handleBackup}
            disabled={triggering}
            className="w-full rounded-md bg-accent px-3 py-2.5 text-xs font-medium text-white hover:bg-accent/90 disabled:opacity-50 transition-colors"
          >
            {triggering ? 'Starting backup...' : 'Backup Now'}
          </button>
        </>
      )}

      {/* Trigger result */}
      {triggerResult && (
        <div className={`rounded-md border p-3 ${
          triggerResult.status === 'failed'
            ? 'border-status-down/20 bg-status-down/5'
            : triggerResult.status === 'success'
              ? 'border-green-500/20 bg-green-500/5'
              : 'border-accent/20 bg-accent/5'
        }`}>
          <div className={`text-xs font-medium mb-1 ${
            triggerResult.status === 'failed'
              ? 'text-status-down'
              : triggerResult.status === 'success'
                ? 'text-green-400'
                : 'text-accent'
          }`}>
            {triggerResult.status === 'pending' && 'Backup queued...'}
            {triggerResult.status === 'running' && 'Backup in progress...'}
            {triggerResult.status === 'success' && `Backup complete — ${triggerResult.files?.length ?? 0} files`}
            {triggerResult.status === 'failed' && 'Backup failed'}
          </div>
          {triggerResult.error_message && (
            <div className="text-[10px] text-text-secondary mt-1">{triggerResult.error_message}</div>
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
        <div className="text-xs text-text-secondary font-medium mb-2">Latest Successful Backup</div>
        {latest ? (
          <div className="rounded-md border border-border-subtle p-3 space-y-1.5">
            <div className="flex justify-between text-xs">
              <span className="text-text-secondary">Date</span>
              <span className="text-text-primary">{formatDate(latest.created_at)}</span>
            </div>
            <div className="flex justify-between text-xs">
              <span className="text-text-secondary">Files</span>
              <span className="text-text-primary">{latest.files?.length ?? 0} files</span>
            </div>
            <div className="flex justify-between text-xs">
              <span className="text-text-secondary">Total Size</span>
              <span className="text-text-primary">{formatSize(totalSize)}</span>
            </div>
            {latest.error_message && (
              <div className="text-[10px] text-yellow-400 mt-1">{latest.error_message}</div>
            )}
          </div>
        ) : (
          <div className="text-xs text-text-secondary">No backups yet</div>
        )}
      </div>
    </div>
  );
}
