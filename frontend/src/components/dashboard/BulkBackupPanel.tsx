import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import {
  fetchBackupJob,
  fetchDeviceCredentialProfiles,
  triggerBulkBackup,
  triggerBulkDownload,
} from '../../api/client';
import { ServerError, ValidationError } from '../../api/errors';
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

type BulkBackupSession = {
  phase: 'idle' | 'running' | 'done';
  entries: DeviceEntry[];
  error: string;
  downloading: boolean;
};

const BULK_BACKUP_GROUP_SIZE = 10;
const BULK_DEVICE_BATCH_SIZE = 100;
const TERMINAL = new Set<DevicePhase>(['skipped', 'success', 'failed']);
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

function jobStatusToDevicePhase(status: string): DevicePhase {
  if (status === 'pending') return 'queued';
  if (status === 'running' || status === 'success' || status === 'failed') return status;
  return 'queued';
}

function chunkArray<T>(items: T[], size: number): T[][] {
  const chunks: T[][] = [];
  for (let index = 0; index < items.length; index += size) {
    chunks.push(items.slice(index, index + size));
  }
  return chunks;
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
  const pollRef = useRef<ReturnType<typeof setInterval> | null>(null);
  const entriesRef = useRef<DeviceEntry[]>([]);
  const mountedRef = useRef(false);

  useEffect(() => {
    mountedRef.current = true;
    const unsubscribe = subscribeBulkBackupSession(() => {
      setSession(bulkBackupSession);
    });
    setSession(bulkBackupSession);
    return () => {
      mountedRef.current = false;
      unsubscribe();
    };
  }, []);

  useEffect(() => {
    entriesRef.current = entries;
  }, [entries]);

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
              phase: jobStatusToDevicePhase(job.status),
              reason: job.error_message || undefined,
            };
          } catch {
            return null;
          }
        }),
      );

      setBulkBackupSession((current) => {
        const next = current.entries.map((e) => {
          const r = results.find((x) => x?.deviceId === e.deviceId);
          if (!r) return e;
          return { ...e, phase: r.phase, reason: r.reason };
        });
        entriesRef.current = next;
        return { ...current, entries: next };
      });
    }, 2000);
  }, []);

  useEffect(() => {
    if (phase !== 'running') return;
    if (entries.some((entry) => entry.jobId && !TERMINAL.has(entry.phase))) {
      startPolling();
    }
  }, [entries, phase, startPolling]);

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
    setBulkBackupSession({ phase: 'running', entries: [], error: '', downloading: false });

    const onlineSelectedDevices = selectedDevices.filter((device) => device.status !== 'down');
    const offlineEntries: DeviceEntry[] = selectedDevices
      .filter((device) => device.status === 'down')
      .map((device) => ({
        deviceId: device.id,
        deviceName: getDeviceName(device),
        phase: 'skipped' as const,
        reason: 'device offline',
      }));
    const preliminaryEntries: DeviceEntry[] = selectedDevices.map((device) => {
      if (device.status === 'down') {
        return {
          deviceId: device.id,
          deviceName: getDeviceName(device),
          phase: 'skipped' as const,
          reason: 'device offline',
        };
      }
      return {
        deviceId: device.id,
        deviceName: getDeviceName(device),
        phase: 'checking' as const,
      };
    });
    entriesRef.current = preliminaryEntries;
    setBulkBackupSession((current) => ({ ...current, entries: preliminaryEntries }));

    if (onlineSelectedDevices.length === 0) {
      setBulkBackupSession({
        phase: 'running',
        entries: offlineEntries,
        error: '',
        downloading: false,
      });
      return;
    }

    // Pre-fetch credential profile assignments for all devices in parallel.
    // Eligibility requires at least one assigned credential profile (replaces legacy ssh_profile_id check).
    const profileResults = await Promise.allSettled(
      onlineSelectedDevices.map((d) => fetchDeviceCredentialProfiles(d.id)),
    );
    const deviceHasProfile = new Map<string, boolean>();
    for (let i = 0; i < onlineSelectedDevices.length; i++) {
      const result = profileResults[i];
      deviceHasProfile.set(
        onlineSelectedDevices[i].id,
        result.status === 'fulfilled' && result.value.length > 0,
      );
    }

    // Build entries — pre-check eligibility
    const initial: DeviceEntry[] = offlineEntries.concat(
      onlineSelectedDevices.map((d) => {
        const name = getDeviceName(d);
        if (!d.backup_supported) {
          return {
            deviceId: d.id,
            deviceName: name,
            phase: 'skipped' as const,
            reason: 'backup not supported for this vendor',
          };
        }
        if (!deviceHasProfile.get(d.id)) {
          return {
            deviceId: d.id,
            deviceName: name,
            phase: 'skipped' as const,
            reason: 'no credential profile assigned',
          };
        }
        return { deviceId: d.id, deviceName: name, phase: 'checking' as const };
      }),
    );

    entriesRef.current = initial;
    setBulkBackupSession((current) => ({ ...current, entries: initial }));

    const eligible = initial.filter((e) => e.phase === 'checking');
    if (eligible.length === 0) return; // completion useEffect handles phase transition

    let queuedCount = 0;
    for (const batch of chunkArray(eligible, BULK_BACKUP_GROUP_SIZE)) {
      try {
        const results = await triggerBulkBackup(batch.map((entry) => entry.deviceId));
        const resultByDevice = new Map(results.map((result) => [result.device_id, result]));
        const batchIDs = new Set(batch.map((entry) => entry.deviceId));
        const nextEntries = entriesRef.current.map((entry) => {
          if (entry.phase !== 'checking' || !batchIDs.has(entry.deviceId)) return entry;
          const result = resultByDevice.get(entry.deviceId);
          if (!result) {
            return {
              ...entry,
              phase: 'skipped' as const,
              reason: 'backup request returned no result',
            };
          }
          if (result.status === 'queued' && result.job_id) {
            queuedCount++;
            return { ...entry, phase: 'queued' as const, jobId: result.job_id };
          }
          return {
            ...entry,
            phase: 'skipped' as const,
            reason: result.reason || 'backup job was not queued',
          };
        });
        entriesRef.current = nextEntries;
        setBulkBackupSession((current) => ({ ...current, entries: nextEntries }));
      } catch (err) {
        const reason = backupRequestErrorMessage(err);
        const nextEntries = entriesRef.current.map((entry) =>
          entry.phase === 'checking' ? { ...entry, phase: 'skipped' as const, reason } : entry,
        );
        entriesRef.current = nextEntries;
        setBulkBackupSession((current) => ({ ...current, entries: nextEntries, error: reason }));
        break;
      }
    }
    if (queuedCount > 0 && mountedRef.current) {
      startPolling();
    }
  };

  const successCount = entries.filter((e) => e.phase === 'success').length;
  const failedCount = entries.filter((e) => e.phase === 'failed').length;
  const skippedCount = entries.filter((e) => e.phase === 'skipped').length;
  const doneCount = entries.filter((e) => TERMINAL.has(e.phase)).length;
  const activeCount = entries.length - doneCount;
  const downloadBatchCount = Math.ceil(successCount / BULK_DEVICE_BATCH_SIZE);

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
                    ? 'border-warning/30 bg-warning/10'
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
                    className={`text-[10px] font-medium ${
                      e.phase === 'success'
                        ? 'text-status-up'
                        : e.phase === 'failed'
                          ? 'text-status-down'
                          : e.phase === 'skipped'
                            ? 'text-warning'
                            : e.phase === 'checking'
                              ? 'text-primary animate-pulse'
                              : e.phase === 'running'
                                ? 'text-primary animate-pulse'
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
      {phase === 'done' && successCount === 0 && failedCount === 0 && !error && (
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
