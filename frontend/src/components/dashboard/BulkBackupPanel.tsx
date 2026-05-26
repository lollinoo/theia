import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import {
  cancelBulkBackupRun,
  fetchBulkBackupRun,
  fetchLatestBulkBackupRun,
  pauseBulkBackupRun,
  resumeBulkBackupRun,
  startBulkBackupRun,
  triggerBulkDownload,
} from '../../api/client';
import { ServerError, ValidationError } from '../../api/errors';
import type {
  BulkBackupRun,
  BulkBackupRunItem,
  BulkBackupRunStatus,
  Device,
} from '../../types/api';

interface BulkBackupPanelProps {
  devices: Device[];
}

type DevicePhase =
  | 'checking'
  | 'queued'
  | 'skipped'
  | 'running'
  | 'success'
  | 'failed'
  | 'cancelled';

type DeviceEntry = {
  deviceId: string;
  deviceName: string;
  phase: DevicePhase;
  reason?: string;
  jobId?: string;
};

type BulkBackupSession = {
  phase: 'idle' | 'running' | 'done';
  runId?: string;
  runStatus?: BulkBackupRunStatus;
  entries: DeviceEntry[];
  error: string;
  downloading: boolean;
};

const BULK_BACKUP_GROUP_SIZE = 10;
const BULK_DEVICE_BATCH_SIZE = 100;
const TERMINAL = new Set<DevicePhase>(['skipped', 'success', 'failed', 'cancelled']);
const RUN_TERMINAL = new Set(['success', 'partial', 'failed', 'cancelled']);
const initialBulkBackupSession: BulkBackupSession = {
  phase: 'idle',
  entries: [],
  error: '',
  downloading: false,
};
let bulkBackupSession: BulkBackupSession = initialBulkBackupSession;
const bulkBackupSessionListeners = new Set<() => void>();

function notifyBulkBackupSessionListeners(): void {
  for (const listener of bulkBackupSessionListeners) {
    listener();
  }
}

function setBulkBackupSession(
  next: BulkBackupSession | ((current: BulkBackupSession) => BulkBackupSession),
): BulkBackupSession {
  bulkBackupSession = typeof next === 'function' ? next(bulkBackupSession) : next;
  notifyBulkBackupSessionListeners();
  return bulkBackupSession;
}

function resetBulkBackupSession(): BulkBackupSession {
  return setBulkBackupSession(initialBulkBackupSession);
}

function subscribeBulkBackupSession(listener: () => void): () => void {
  bulkBackupSessionListeners.add(listener);
  return () => {
    bulkBackupSessionListeners.delete(listener);
  };
}

export function __resetBulkBackupSessionForTests(): void {
  bulkBackupSession = initialBulkBackupSession;
  bulkBackupSessionListeners.clear();
}

function getDeviceName(d: Device): string {
  return d.tags?.display_name || d.sys_name || d.ip;
}

function backupRequestErrorMessage(err: unknown): string {
  if (err instanceof ServerError) {
    return err.correlationId ? `server error (ref: ${err.correlationId})` : 'server error';
  }
  if (err instanceof ValidationError) {
    return err.message;
  }
  const msg = err instanceof Error ? err.message : 'backup failed';
  return msg.includes('unreachable')
    ? 'device unreachable'
    : msg.includes('no credential')
      ? 'no credential profile assigned'
      : msg.includes('not supported')
        ? 'backup not supported for this vendor'
        : msg;
}

function chunkArray<T>(items: T[], size: number): T[][] {
  const chunks: T[][] = [];
  for (let index = 0; index < items.length; index += size) {
    chunks.push(items.slice(index, index + size));
  }
  return chunks;
}

function itemToEntry(item: BulkBackupRunItem): DeviceEntry {
  return {
    deviceId: item.device_id,
    deviceName: item.device_name,
    phase: item.status,
    reason: item.reason,
    jobId: item.backup_job_id,
  };
}

function sessionFromRun(run: BulkBackupRun): BulkBackupSession {
  return {
    phase: RUN_TERMINAL.has(run.status) ? 'done' : 'running',
    runId: run.id,
    runStatus: run.status,
    entries: run.items.map(itemToEntry),
    error: run.error_message,
    downloading: bulkBackupSession.downloading,
  };
}

function isActiveRun(run: BulkBackupRun): boolean {
  return (
    run.status === 'running' ||
    run.status === 'pausing' ||
    run.status === 'paused' ||
    run.status === 'cancelling'
  );
}

function shouldPollRun(run: BulkBackupRun): boolean {
  return !RUN_TERMINAL.has(run.status) && run.status !== 'paused';
}

function runStatusLabel(status?: BulkBackupRunStatus): string {
  switch (status) {
    case 'pausing':
      return 'pausing';
    case 'paused':
      return 'paused';
    case 'cancelling':
      return 'stopping';
    default:
      return 'running';
  }
}

function isPendingControlDevice(entry: DeviceEntry): boolean {
  return entry.phase === 'checking' || entry.phase === 'queued';
}

function controlProgressSummary(
  entries: DeviceEntry[],
  status?: BulkBackupRunStatus,
): string | null {
  const completingCount = entries.filter((entry) => entry.phase === 'running').length;
  const pendingCount = entries.filter(isPendingControlDevice).length;

  if (status === 'pausing') {
    const parts = [];
    if (completingCount > 0) parts.push(`${completingCount} completing`);
    if (pendingCount > 0) parts.push(`${pendingCount} will pause`);
    return parts.length > 0 ? parts.join('; ') : null;
  }
  if (status === 'cancelling') {
    const parts = [];
    if (completingCount > 0) parts.push(`${completingCount} completing`);
    if (pendingCount > 0) parts.push(`${pendingCount} will stop`);
    return parts.length > 0 ? parts.join('; ') : null;
  }
  if (status === 'paused') {
    return pendingCount > 0 ? `${pendingCount} paused` : null;
  }
  return null;
}

function deviceStatusLabel(entry: DeviceEntry, runStatus?: BulkBackupRunStatus): string {
  if (entry.phase === 'cancelled') {
    return 'stopped';
  }
  if (runStatus === 'pausing') {
    if (entry.phase === 'running') return 'completing';
    if (isPendingControlDevice(entry)) return 'will pause';
  }
  if (runStatus === 'paused' && isPendingControlDevice(entry)) {
    return 'paused';
  }
  if (runStatus === 'cancelling') {
    if (entry.phase === 'running') return 'completing';
    if (isPendingControlDevice(entry)) return 'will stop';
  }
  return entry.phase === 'checking' ? 'checking...' : entry.phase;
}

function deviceStatusClassName(entry: DeviceEntry, runStatus?: BulkBackupRunStatus): string {
  const label = deviceStatusLabel(entry, runStatus);
  if (entry.phase === 'success') return 'text-status-up';
  if (entry.phase === 'failed') return 'text-status-down';
  if (entry.phase === 'skipped') return 'text-warning';
  if (label === 'will stop' || label === 'stopped') return 'text-status-down';
  if (label === 'will pause' || label === 'paused') return 'text-on-bg-secondary';
  if (label === 'completing' || label === 'checking...' || entry.phase === 'running') {
    return 'text-primary animate-pulse';
  }
  return 'text-on-bg-secondary';
}

function bulkDownloadBatchFilename(batchIndex: number, batchCount: number): string {
  const timestamp = new Date().toISOString().replace(/\D/g, '').slice(0, 14);
  return `THEIA_BACKUPS_batch-${batchIndex + 1}-of-${batchCount}_${timestamp}.zip`;
}

export function BulkBackupPanel({ devices: allDevices }: BulkBackupPanelProps) {
  const devices = useMemo(
    () => allDevices.filter((d) => d.device_type !== 'virtual'),
    [allDevices],
  );
  const deviceIdsKey = devices.map((d) => d.id).join('\0');
  const [session, setSession] = useState<BulkBackupSession>(() => bulkBackupSession);
  const { phase, entries, error, downloading } = session;
  const [selectedDeviceIds, setSelectedDeviceIds] = useState<Set<string>>(
    () => new Set(devices.map((d) => d.id)),
  );
  const [controlBusy, setControlBusy] = useState<'pause' | 'resume' | 'stop' | null>(null);
  const pollRef = useRef<ReturnType<typeof setInterval> | null>(null);
  const entriesRef = useRef<DeviceEntry[]>([]);
  const mountedRef = useRef(false);
  const runIdRef = useRef<string | undefined>(bulkBackupSession.runId);

  // Poll the persistent run for status updates.
  const startPolling = useCallback((runId?: string) => {
    const id = runId ?? runIdRef.current;
    if (!id) return;
    if (pollRef.current) return;
    pollRef.current = setInterval(async () => {
      try {
        const run = await fetchBulkBackupRun(id);
        const next = sessionFromRun(run);
        entriesRef.current = next.entries;
        runIdRef.current = next.runId;
        setBulkBackupSession((current) => ({ ...next, downloading: current.downloading }));
        if ((next.phase === 'done' || run.status === 'paused') && pollRef.current) {
          clearInterval(pollRef.current);
          pollRef.current = null;
        }
      } catch {
        // Keep the last known progress visible; the next poll may recover.
      }
    }, 2000);
  }, []);

  useEffect(() => {
    mountedRef.current = true;
    const unsubscribe = subscribeBulkBackupSession(() => {
      setSession(bulkBackupSession);
    });
    setSession(bulkBackupSession);
    void fetchLatestBulkBackupRun()
      .then((run) => {
        if (
          !mountedRef.current ||
          !run ||
          !isActiveRun(run) ||
          bulkBackupSession.phase !== 'idle'
        ) {
          return;
        }
        const next = sessionFromRun(run);
        entriesRef.current = next.entries;
        runIdRef.current = next.runId;
        setBulkBackupSession(next);
        if (shouldPollRun(run)) {
          startPolling(run.id);
        }
      })
      .catch(() => {});
    return () => {
      mountedRef.current = false;
      unsubscribe();
    };
  }, [startPolling]);

  useEffect(() => {
    entriesRef.current = entries;
    runIdRef.current = session.runId;
  }, [entries, session.runId]);

  useEffect(() => {
    if (phase !== 'idle') return;
    setSelectedDeviceIds(new Set(devices.map((d) => d.id)));
  }, [deviceIdsKey, phase]);

  // Cleanup on unmount
  useEffect(
    () => () => {
      if (pollRef.current) clearInterval(pollRef.current);
    },
    [],
  );

  // Detect completion: all entries terminal → done
  useEffect(() => {
    if (phase !== 'running' || entries.length === 0) return;
    if (entries.every((e) => TERMINAL.has(e.phase))) {
      if (pollRef.current) {
        clearInterval(pollRef.current);
        pollRef.current = null;
      }
      setBulkBackupSession((current) => ({ ...current, phase: 'done' }));
    }
  }, [entries, phase]);

  useEffect(() => {
    if (phase !== 'running') return;
    if (session.runStatus !== 'paused' && entries.some((entry) => !TERMINAL.has(entry.phase))) {
      startPolling(session.runId);
    }
  }, [entries, phase, session.runId, session.runStatus, startPolling]);

  const selectedDevices = devices.filter((device) => selectedDeviceIds.has(device.id));
  const selectedCount = selectedDeviceIds.size;

  const setAllDevicesSelected = () => {
    setSelectedDeviceIds(new Set(devices.map((device) => device.id)));
  };

  const clearSelectedDevices = () => {
    setSelectedDeviceIds(new Set());
  };

  const toggleSelectedDevice = (deviceID: string) => {
    setSelectedDeviceIds((prev) => {
      const next = new Set(prev);
      if (next.has(deviceID)) {
        next.delete(deviceID);
      } else {
        next.add(deviceID);
      }
      return next;
    });
  };

  const handleStart = async () => {
    if (bulkBackupSession.phase === 'running') {
      return;
    }
    if (selectedDevices.length === 0) {
      setBulkBackupSession((current) => ({
        ...current,
        error: 'Select at least one device to back up.',
      }));
      return;
    }
    const preliminaryEntries: DeviceEntry[] = selectedDevices.map((device) => {
      return {
        deviceId: device.id,
        deviceName: getDeviceName(device),
        phase: 'checking' as const,
      };
    });
    entriesRef.current = preliminaryEntries;
    setBulkBackupSession({
      phase: 'running',
      entries: preliminaryEntries,
      error: '',
      downloading: false,
    });

    try {
      const run = await startBulkBackupRun(selectedDevices.map((device) => device.id));
      const next = sessionFromRun(run);
      entriesRef.current = next.entries;
      runIdRef.current = next.runId;
      setBulkBackupSession(next);
      if (shouldPollRun(run) && next.entries.some((entry) => !TERMINAL.has(entry.phase))) {
        startPolling(run.id);
      }
    } catch (err) {
      const reason = backupRequestErrorMessage(err);
      setBulkBackupSession({
        phase: 'idle',
        entries: [],
        error: reason,
        downloading: false,
      });
    }
  };

  const successCount = entries.filter((e) => e.phase === 'success').length;
  const failedCount = entries.filter((e) => e.phase === 'failed').length;
  const skippedCount = entries.filter((e) => e.phase === 'skipped').length;
  const stoppedCount = entries.filter((e) => e.phase === 'cancelled').length;
  const doneCount = entries.filter((e) => TERMINAL.has(e.phase)).length;
  const activeCount = entries.length - doneCount;
  const downloadBatchCount = Math.ceil(successCount / BULK_DEVICE_BATCH_SIZE);
  const controlSummary = controlProgressSummary(entries, session.runStatus);
  const canPause = phase === 'running' && session.runId && session.runStatus === 'running';
  const canResume = phase === 'running' && session.runId && session.runStatus === 'paused';
  const canStop =
    phase === 'running' &&
    session.runId &&
    (session.runStatus === 'running' ||
      session.runStatus === 'pausing' ||
      session.runStatus === 'paused' ||
      session.runStatus === 'cancelling');

  const applyRunUpdate = (run: BulkBackupRun) => {
    const next = sessionFromRun(run);
    entriesRef.current = next.entries;
    runIdRef.current = next.runId;
    setBulkBackupSession((current) => ({ ...next, downloading: current.downloading }));
    if (shouldPollRun(run)) {
      startPolling(run.id);
    } else if (pollRef.current) {
      clearInterval(pollRef.current);
      pollRef.current = null;
    }
  };

  const handlePause = async () => {
    if (!session.runId || controlBusy) return;
    setControlBusy('pause');
    try {
      applyRunUpdate(await pauseBulkBackupRun(session.runId));
    } catch (err) {
      setBulkBackupSession((current) => ({ ...current, error: backupRequestErrorMessage(err) }));
    } finally {
      setControlBusy(null);
    }
  };

  const handleResume = async () => {
    if (!session.runId || controlBusy) return;
    setControlBusy('resume');
    try {
      applyRunUpdate(await resumeBulkBackupRun(session.runId));
    } catch (err) {
      setBulkBackupSession((current) => ({ ...current, error: backupRequestErrorMessage(err) }));
    } finally {
      setControlBusy(null);
    }
  };

  const handleStop = async () => {
    if (!session.runId || controlBusy) return;
    setControlBusy('stop');
    try {
      applyRunUpdate(await cancelBulkBackupRun(session.runId));
    } catch (err) {
      setBulkBackupSession((current) => ({ ...current, error: backupRequestErrorMessage(err) }));
    } finally {
      setControlBusy(null);
    }
  };

  const handleDownloadZip = async () => {
    setBulkBackupSession((current) => ({ ...current, downloading: true, error: '' }));
    try {
      const ids = entries.filter((e) => e.phase === 'success').map((e) => e.deviceId);
      const batches = chunkArray(ids, BULK_DEVICE_BATCH_SIZE);
      for (let index = 0; index < batches.length; index++) {
        const result = await triggerBulkDownload(batches[index], {
          filename:
            batches.length > 1 ? bulkDownloadBatchFilename(index, batches.length) : undefined,
        });
        if (result === 'cancelled') {
          break;
        }
      }
    } catch (err) {
      setBulkBackupSession((current) => ({
        ...current,
        error: err instanceof Error ? err.message : 'Download failed',
      }));
    } finally {
      setBulkBackupSession((current) => ({ ...current, downloading: false }));
    }
  };

  return (
    <div className="space-y-4 transition-colors duration-200">
      {/* Summary */}
      <div className="rounded-lg bg-surface-high p-3 text-xs text-on-bg-secondary">
        {devices.length} device{devices.length !== 1 ? 's' : ''} in scope
        {phase === 'idle' && (
          <>
            {' '}
            · {selectedCount} selected · groups of {BULK_BACKUP_GROUP_SIZE}
          </>
        )}
      </div>

      {phase === 'idle' && devices.length > 0 && (
        <div className="space-y-2 rounded-lg border border-outline bg-surface p-2">
          <div className="flex items-center justify-between gap-2 text-xs">
            <span className="text-on-bg-secondary">
              {selectedCount} of {devices.length} selected
            </span>
            <div className="flex items-center gap-2">
              <button
                type="button"
                onClick={setAllDevicesSelected}
                className="text-primary hover:text-primary/80"
              >
                Select all
              </button>
              <button
                type="button"
                onClick={clearSelectedDevices}
                className="text-on-bg-secondary hover:text-on-bg"
              >
                Clear
              </button>
            </div>
          </div>
          <div className="max-h-48 space-y-1 overflow-y-auto">
            {devices.map((device) => {
              const name = getDeviceName(device);
              return (
                <label
                  key={device.id}
                  className="flex items-center gap-2 rounded-md px-2 py-1 text-xs text-on-bg hover:bg-surface-high"
                >
                  <input
                    type="checkbox"
                    checked={selectedDeviceIds.has(device.id)}
                    onChange={() => toggleSelectedDevice(device.id)}
                    aria-label={`Select ${name}`}
                    className="h-3.5 w-3.5"
                  />
                  <span className="truncate">{name}</span>
                </label>
              );
            })}
          </div>
        </div>
      )}

      {/* Idle: start button */}
      {phase === 'idle' && (
        <button
          type="button"
          onClick={() => {
            void handleStart();
          }}
          disabled={selectedCount === 0}
          className="w-full rounded-md bg-primary px-3 py-2.5 text-xs font-medium text-white hover:bg-primary/90 disabled:opacity-50 transition-colors"
        >
          {selectedCount === devices.length ? 'Backup All Devices' : 'Backup Selected Devices'}
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
                : `Complete — ${successCount} succeeded, ${failedCount} failed, ${skippedCount} skipped${
                    stoppedCount > 0 ? `, ${stoppedCount} stopped` : ''
                  }`}
            </span>
            {phase === 'running' && activeCount > 0 && (
              <span
                className={
                  session.runStatus === 'paused'
                    ? 'text-on-bg-secondary'
                    : 'text-primary animate-pulse'
                }
              >
                {runStatusLabel(session.runStatus)}
              </span>
            )}
          </div>
          {phase === 'running' && controlSummary && (
            <div className="rounded-md border border-outline bg-surface px-3 py-2 text-xs text-on-bg-secondary">
              {controlSummary}
            </div>
          )}

          {phase === 'running' && (canPause || canResume || canStop) && (
            <div className="flex gap-2">
              {canPause && (
                <button
                  type="button"
                  onClick={() => {
                    void handlePause();
                  }}
                  disabled={controlBusy !== null}
                  className="flex-1 rounded-md border border-outline bg-surface px-3 py-2 text-xs font-medium text-on-bg-secondary hover:bg-surface-high disabled:opacity-50 transition-colors"
                >
                  {controlBusy === 'pause' ? 'Pausing...' : 'Pause'}
                </button>
              )}
              {canResume && (
                <button
                  type="button"
                  onClick={() => {
                    void handleResume();
                  }}
                  disabled={controlBusy !== null}
                  className="flex-1 rounded-md border border-primary bg-primary/10 px-3 py-2 text-xs font-medium text-primary hover:bg-primary/20 disabled:opacity-50 transition-colors"
                >
                  {controlBusy === 'resume' ? 'Resuming...' : 'Resume'}
                </button>
              )}
              {canStop && (
                <button
                  type="button"
                  onClick={() => {
                    void handleStop();
                  }}
                  disabled={controlBusy !== null || session.runStatus === 'cancelling'}
                  className="flex-1 rounded-md border border-status-down/30 bg-status-down/10 px-3 py-2 text-xs font-medium text-status-down hover:bg-status-down/15 disabled:opacity-50 transition-colors"
                >
                  {controlBusy === 'stop' || session.runStatus === 'cancelling'
                    ? 'Stopping...'
                    : 'Stop'}
                </button>
              )}
            </div>
          )}

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
            {entries.map((e) => {
              const statusLabel = deviceStatusLabel(e, session.runStatus);
              return (
                <div
                  key={e.deviceId}
                  className={`flex items-center justify-between rounded-md border px-3 py-1.5 ${
                    e.phase === 'skipped'
                      ? 'border-warning/30 bg-warning/10'
                      : e.phase === 'failed' || e.phase === 'cancelled'
                        ? 'border-status-down/20 bg-status-down/5'
                        : e.phase === 'success'
                          ? 'border-status-up/20 bg-status-up/5'
                          : 'border-outline'
                  }`}
                >
                  <span className="text-[10px] text-on-bg truncate mr-2">{e.deviceName}</span>
                  <div className="flex items-center gap-2 shrink-0">
                    {e.reason && e.phase === 'skipped' && (
                      <span className="text-[9px] text-warning truncate max-w-[140px]">
                        {e.reason}
                      </span>
                    )}
                    {e.reason && e.phase === 'failed' && (
                      <span className="text-[9px] text-status-down truncate max-w-[140px]">
                        {e.reason}
                      </span>
                    )}
                    <span
                      className={`text-[10px] font-medium ${deviceStatusClassName(
                        e,
                        session.runStatus,
                      )}`}
                    >
                      {statusLabel}
                    </span>
                  </div>
                </div>
              );
            })}
          </div>
        </div>
      )}

      {/* Done: download */}
      {phase === 'done' && successCount > 0 && (
        <div className="space-y-2">
          <p className="text-xs text-on-bg-secondary">
            {downloadBatchCount > 1
              ? `Downloads will be split into ${downloadBatchCount} ZIP files of up to ${BULK_DEVICE_BATCH_SIZE} devices each.`
              : "Download files individually from each device's backup history, or download all as a zip."}
          </p>
          <button
            type="button"
            onClick={() => {
              void handleDownloadZip();
            }}
            disabled={downloading}
            className="w-full rounded-md border border-primary bg-primary/10 px-3 py-2.5 text-xs font-medium text-primary hover:bg-primary/20 disabled:opacity-50 transition-colors"
          >
            {downloading
              ? 'Preparing zip...'
              : downloadBatchCount > 1
                ? `Download ${downloadBatchCount} ZIP files`
                : 'Download All as ZIP'}
          </button>
        </div>
      )}

      {phase === 'done' && (
        <button
          type="button"
          onClick={resetBulkBackupSession}
          className="w-full rounded-md border border-outline bg-surface px-3 py-2 text-xs font-medium text-on-bg-secondary hover:bg-surface-high transition-colors"
        >
          Start New Bulk Backup
        </button>
      )}

      {/* No eligible devices */}
      {phase === 'done' &&
        successCount === 0 &&
        failedCount === 0 &&
        skippedCount === 0 &&
        stoppedCount === 0 &&
        !error && (
          <div className="rounded-md border border-status-down/20 bg-status-down/5 p-3 text-xs text-status-down">
            No devices were eligible for backup. Ensure devices have a supported vendor and an SSH
            profile assigned.
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
