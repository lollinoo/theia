import { useState, useEffect, useCallback, useRef } from 'react';
import { triggerBackup, triggerBulkDownload, fetchBackupJob } from '../../api/client';
import { ValidationError, ServerError } from '../../api/errors';
import type { Device } from '../../types/api';

interface BulkBackupPanelProps {
  devices: Device[];
}

type DevicePhase = 'checking' | 'queued' | 'skipped' | 'running' | 'success' | 'failed';

type DeviceEntry = {
  deviceId: string;
  deviceName: string;
  phase: DevicePhase;
  reason?: string;
  jobId?: string;
};

const TERMINAL = new Set<DevicePhase>(['skipped', 'success', 'failed']);

function getDeviceName(d: Device): string {
  return d.tags?.display_name || d.sys_name || d.ip;
}

export function BulkBackupPanel({ devices: allDevices }: BulkBackupPanelProps) {
  const devices = allDevices.filter((d) => d.device_type !== 'virtual');
  const [phase, setPhase] = useState<'idle' | 'running' | 'done'>('idle');
  const [entries, setEntries] = useState<DeviceEntry[]>([]);
  const [error, setError] = useState('');
  const [downloading, setDownloading] = useState(false);
  const pollRef = useRef<ReturnType<typeof setInterval> | null>(null);
  const entriesRef = useRef<DeviceEntry[]>([]);

  // Cleanup on unmount
  useEffect(() => () => {
    if (pollRef.current) clearInterval(pollRef.current);
  }, []);

  // Detect completion: all entries terminal → done
  useEffect(() => {
    if (phase !== 'running' || entries.length === 0) return;
    if (entries.every((e) => TERMINAL.has(e.phase))) {
      if (pollRef.current) { clearInterval(pollRef.current); pollRef.current = null; }
      setPhase('done');
    }
  }, [entries, phase]);

  // Helper: patch a single entry and keep ref in sync
  const patchEntry = useCallback((deviceId: string, patch: Partial<DeviceEntry>) => {
    setEntries((prev) => {
      const next = prev.map((e) => (e.deviceId === deviceId ? { ...e, ...patch } : e));
      entriesRef.current = next;
      return next;
    });
  }, []);

  // Poll active jobs for status updates
  const startPolling = useCallback(() => {
    if (pollRef.current) return;
    pollRef.current = setInterval(async () => {
      const current = entriesRef.current;
      const active = current.filter((e) => e.jobId && !TERMINAL.has(e.phase));
      if (active.length === 0) return;

      const results = await Promise.all(
        active.map(async (entry) => {
          try {
            const job = await fetchBackupJob(entry.jobId!);
            return {
              deviceId: entry.deviceId,
              phase: job.status as DevicePhase,
              reason: job.error_message || undefined,
            };
          } catch {
            return null;
          }
        }),
      );

      setEntries((prev) => {
        const next = prev.map((e) => {
          const r = results.find((x) => x?.deviceId === e.deviceId);
          if (!r) return e;
          return { ...e, phase: r.phase, reason: r.reason };
        });
        entriesRef.current = next;
        return next;
      });
    }, 2000);
  }, []);

  const handleStart = () => {
    setPhase('running');
    setError('');

    // Build entries — pre-check eligibility instantly on client side
    const initial: DeviceEntry[] = devices.map((d) => {
      const name = getDeviceName(d);
      if (!d.backup_supported) {
        return { deviceId: d.id, deviceName: name, phase: 'skipped' as const, reason: 'backup not supported for this vendor' };
      }
      if (!d.ssh_profile_id) {
        return { deviceId: d.id, deviceName: name, phase: 'skipped' as const, reason: 'no SSH profile assigned' };
      }
      return { deviceId: d.id, deviceName: name, phase: 'checking' as const };
    });

    setEntries(initial);
    entriesRef.current = initial;

    const eligible = initial.filter((e) => e.phase === 'checking');
    if (eligible.length === 0) return; // completion useEffect handles phase transition

    // Fire parallel backup triggers — each device independently
    for (const entry of eligible) {
      triggerBackup(entry.deviceId)
        .then((job) => {
          patchEntry(entry.deviceId, { phase: 'queued', jobId: job.id });
          startPolling();
        })
        .catch((err) => {
          let reason: string;
          if (err instanceof ServerError) {
            reason = err.correlationId ? `server error (ref: ${err.correlationId})` : 'server error';
          } else if (err instanceof ValidationError) {
            reason = err.message;
          } else {
            const msg = err instanceof Error ? err.message : 'backup failed';
            reason = msg.includes('unreachable') ? 'device unreachable'
              : msg.includes('no SSH') ? 'no SSH profile assigned'
              : msg.includes('not supported') ? 'backup not supported for this vendor'
              : msg;
          }
          patchEntry(entry.deviceId, { phase: 'skipped', reason });
        });
    }
  };

  const successCount = entries.filter((e) => e.phase === 'success').length;
  const failedCount = entries.filter((e) => e.phase === 'failed').length;
  const skippedCount = entries.filter((e) => e.phase === 'skipped').length;
  const doneCount = entries.filter((e) => TERMINAL.has(e.phase)).length;
  const activeCount = entries.length - doneCount;

  const handleDownloadZip = async () => {
    setDownloading(true);
    try {
      const ids = entries.filter((e) => e.phase === 'success').map((e) => e.deviceId);
      await triggerBulkDownload(ids);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Download failed');
    } finally {
      setDownloading(false);
    }
  };

  return (
    <div className="space-y-4 transition-colors duration-200">
      {/* Summary */}
      <div className="rounded-lg bg-surface-high p-3 text-xs text-on-bg-secondary">
        {devices.length} device{devices.length !== 1 ? 's' : ''} in scope
      </div>

      {/* Idle: start button */}
      {phase === 'idle' && (
        <button
          onClick={handleStart}
          className="w-full rounded-md bg-primary px-3 py-2.5 text-xs font-medium text-white hover:bg-primary/90 transition-colors"
        >
          Backup All Devices
        </button>
      )}

      {/* Live device list */}
      {entries.length > 0 && (
        <div className="space-y-1.5">
          {/* Progress header */}
          <div className="flex items-center justify-between text-xs text-on-bg-secondary">
            <span>
              {phase === 'running'
                ? `Processing... ${doneCount}/${entries.length}`
                : `Complete — ${successCount} succeeded, ${failedCount} failed, ${skippedCount} skipped`}
            </span>
            {phase === 'running' && activeCount > 0 && (
              <span className="text-primary animate-pulse">running</span>
            )}
          </div>

          {/* Progress bar */}
          <div className="h-1.5 w-full rounded-full bg-elevated overflow-hidden">
            <div
              className={`h-full transition-all duration-300 ${
                failedCount > 0 && successCount === 0 ? 'bg-status-down' : 'bg-primary'
              }`}
              style={{ width: `${entries.length > 0 ? (doneCount / entries.length) * 100 : 0}%` }}
            />
          </div>

          {/* Per-device status */}
          <div className="space-y-1 max-h-60 overflow-y-auto">
            {entries.map((e) => (
              <div
                key={e.deviceId}
                className={`flex items-center justify-between rounded-md border px-3 py-1.5 ${
                  e.phase === 'skipped'
                    ? 'border-yellow-500/20 bg-yellow-500/5'
                    : e.phase === 'failed'
                      ? 'border-status-down/20 bg-status-down/5'
                      : e.phase === 'success'
                        ? 'border-status-up/20 bg-status-up/5'
                        : 'border-outline'
                }`}
              >
                <span className="text-[10px] text-on-bg truncate mr-2">{e.deviceName}</span>
                <div className="flex items-center gap-2 shrink-0">
                  {e.reason && e.phase === 'skipped' && (
                    <span className="text-[9px] text-yellow-400 truncate max-w-[140px]">{e.reason}</span>
                  )}
                  {e.reason && e.phase === 'failed' && (
                    <span className="text-[9px] text-status-down truncate max-w-[140px]">{e.reason}</span>
                  )}
                  <span
                    className={`text-[10px] font-medium ${
                      e.phase === 'success' ? 'text-status-up'
                        : e.phase === 'failed' ? 'text-status-down'
                          : e.phase === 'skipped' ? 'text-yellow-400'
                            : e.phase === 'checking' ? 'text-blue-400 animate-pulse'
                              : e.phase === 'running' ? 'text-primary animate-pulse'
                                : 'text-on-bg-secondary'
                    }`}
                  >
                    {e.phase === 'checking' ? 'checking...' : e.phase}
                  </span>
                </div>
              </div>
            ))}
          </div>
        </div>
      )}

      {/* Done: download */}
      {phase === 'done' && successCount > 0 && (
        <div className="space-y-2">
          <p className="text-xs text-on-bg-secondary">
            Download files individually from each device's backup history, or download all as a zip.
          </p>
          <button
            onClick={() => { void handleDownloadZip(); }}
            disabled={downloading}
            className="w-full rounded-md border border-primary bg-primary/10 px-3 py-2.5 text-xs font-medium text-primary hover:bg-primary/20 disabled:opacity-50 transition-colors"
          >
            {downloading ? 'Preparing zip...' : 'Download All as ZIP'}
          </button>
        </div>
      )}

      {/* No eligible devices */}
      {phase === 'done' && successCount === 0 && failedCount === 0 && (
        <div className="rounded-md border border-status-down/20 bg-status-down/5 p-3 text-xs text-status-down">
          No devices were eligible for backup. Ensure devices have a supported vendor and an SSH profile assigned.
        </div>
      )}

      {error && (
        <div className="rounded-md border border-status-down/20 bg-status-down/5 p-3 text-xs text-status-down">
          {error}
        </div>
      )}
    </div>
  );
}
