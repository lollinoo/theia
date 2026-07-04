/**
 * Defines bulk backup panel behavior for the operations dashboard.
 * Keeps table, backup, and device-management responsibilities isolated by module.
 */
import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import {
  cancelBulkBackupRun,
  fetchBackupJob,
  fetchBulkBackupRun,
  fetchBulkOperationStatus,
  fetchLatestBulkBackupRun,
  pauseBulkBackupRun,
  resumeBulkBackupRun,
  startBulkBackupRun,
  triggerBulkDownload,
} from '../../api/client';
import { ServerError, ValidationError } from '../../api/errors';
import type {
  BackupJob,
  BulkBackupRun,
  BulkBackupRunItem,
  BulkBackupRunStatus,
  BulkOperationStatus,
  Device,
} from '../../types/api';

interface BulkBackupPanelProps {
  devices: Device[];
}

type DevicePhase =
  | 'checking'
  | 'active'
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
  fileCount: number;
  byteCount: number;
};

type BulkBackupReport = {
  totalCount: number;
  queuedCount: number;
  runningCount: number;
  completedCount: number;
  successCount: number;
  failedCount: number;
  skippedCount: number;
  stoppedCount: number;
  fileCount: number;
  byteCount: number;
  currentDeviceName?: string;
  currentJobId?: string;
};

type BulkBackupSession = {
  phase: 'idle' | 'running' | 'done';
  runId?: string;
  runStatus?: BulkBackupRunStatus;
  startedBy?: string;
  entries: DeviceEntry[];
  report?: BulkBackupReport;
  error: string;
  downloading: boolean;
};

type BulkDownloadCandidate = {
  deviceId: string;
  fileCount: number;
  byteCount: number;
  hasMetadata: boolean;
};

type BulkDownloadLimits = {
  maxDevices: number;
  maxFiles: number;
  maxBytes: number;
};

type BulkDownloadBatchPlan = {
  batches: string[][];
  error?: string;
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

/** Resets bulk backup session for tests for the operations dashboard. */
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

function positiveIntegerOrFallback(value: number | undefined, fallback: number): number {
  return Number.isFinite(value) && value !== undefined && value > 0 ? Math.floor(value) : fallback;
}

function backupJobByteCount(job: BackupJob): number {
  return job.files.reduce((sum, file) => {
    return sum + (Number.isFinite(file.size_bytes) && file.size_bytes > 0 ? file.size_bytes : 0);
  }, 0);
}

function formatByteCount(bytes: number): string {
  if (!Number.isFinite(bytes) || bytes <= 0) return '0 B';
  if (bytes < 1024) return `${bytes} B`;
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`;
  return `${(bytes / (1024 * 1024)).toFixed(1)} MB`;
}

function formatFileCount(count: number): string {
  return `${count} file${count === 1 ? '' : 's'}`;
}

async function loadBulkDownloadCandidates(
  entries: DeviceEntry[],
): Promise<BulkDownloadCandidate[]> {
  const successEntries = entries.filter((entry) => entry.phase === 'success');
  return Promise.all(
    successEntries.map(async (entry) => {
      if (!entry.jobId) {
        return {
          deviceId: entry.deviceId,
          fileCount: 0,
          byteCount: 0,
          hasMetadata: false,
        };
      }
      try {
        const job = await fetchBackupJob(entry.jobId);
        return {
          deviceId: entry.deviceId,
          fileCount: job.files.length,
          byteCount: backupJobByteCount(job),
          hasMetadata: true,
        };
      } catch {
        return {
          deviceId: entry.deviceId,
          fileCount: 0,
          byteCount: 0,
          hasMetadata: false,
        };
      }
    }),
  );
}

function planBulkDownloadBatches(
  candidates: BulkDownloadCandidate[],
  limits: BulkDownloadLimits,
): BulkDownloadBatchPlan {
  const batches: string[][] = [];
  let currentBatch: string[] = [];
  let currentFiles = 0;
  let currentBytes = 0;

  const flush = () => {
    if (currentBatch.length === 0) return;
    batches.push(currentBatch);
    currentBatch = [];
    currentFiles = 0;
    currentBytes = 0;
  };

  for (const candidate of candidates) {
    if (candidate.hasMetadata && limits.maxFiles > 0 && candidate.fileCount > limits.maxFiles) {
      return {
        batches: [],
        error: `Too many backup files selected for bulk download. Maximum ${limits.maxFiles}, selected ${candidate.fileCount}.`,
      };
    }
    if (candidate.hasMetadata && limits.maxBytes > 0 && candidate.byteCount > limits.maxBytes) {
      return {
        batches: [],
        error: `Bulk download is too large. Maximum ${limits.maxBytes} bytes, selected ${candidate.byteCount} bytes.`,
      };
    }

    const exceedsDeviceLimit = limits.maxDevices > 0 && currentBatch.length + 1 > limits.maxDevices;
    const exceedsFileLimit =
      candidate.hasMetadata &&
      limits.maxFiles > 0 &&
      currentFiles + candidate.fileCount > limits.maxFiles;
    const exceedsByteLimit =
      candidate.hasMetadata &&
      limits.maxBytes > 0 &&
      currentBytes + candidate.byteCount > limits.maxBytes;

    if (currentBatch.length > 0 && (exceedsDeviceLimit || exceedsFileLimit || exceedsByteLimit)) {
      flush();
    }

    currentBatch.push(candidate.deviceId);
    if (candidate.hasMetadata) {
      currentFiles += candidate.fileCount;
      currentBytes += candidate.byteCount;
    }
  }

  flush();
  return { batches };
}

function itemToEntry(item: BulkBackupRunItem): DeviceEntry {
  return {
    deviceId: item.device_id,
    deviceName: item.device_name,
    phase: item.status,
    reason: item.reason,
    jobId: item.backup_job_id,
    fileCount: Number.isFinite(item.file_count) ? item.file_count : 0,
    byteCount: Number.isFinite(item.byte_count) ? item.byte_count : 0,
  };
}

function isRunningResource(entry: DeviceEntry): boolean {
  return entry.phase === 'active' || entry.phase === 'running';
}

function reportFromEntries(entries: DeviceEntry[]): BulkBackupReport {
  const runningEntry =
    entries.find((entry) => entry.phase === 'running') ??
    entries.find((entry) => entry.phase === 'active');
  const successCount = entries.filter((entry) => entry.phase === 'success').length;
  const failedCount = entries.filter((entry) => entry.phase === 'failed').length;
  const skippedCount = entries.filter((entry) => entry.phase === 'skipped').length;
  const stoppedCount = entries.filter((entry) => entry.phase === 'cancelled').length;
  const completedCount = successCount + failedCount + skippedCount + stoppedCount;
  const fileCount = entries.reduce((sum, entry) => sum + entry.fileCount, 0);
  const byteCount = entries.reduce((sum, entry) => sum + entry.byteCount, 0);

  return {
    totalCount: entries.length,
    queuedCount: entries.filter(isPendingControlDevice).length,
    runningCount: entries.filter(isRunningResource).length,
    completedCount,
    successCount,
    failedCount,
    skippedCount,
    stoppedCount,
    fileCount,
    byteCount,
    currentDeviceName: runningEntry?.deviceName,
    currentJobId: runningEntry?.jobId,
  };
}

function reportFromRun(run: BulkBackupRun, entries: DeviceEntry[]): BulkBackupReport {
  const entryReport = reportFromEntries(entries);
  const runningCount = run.running_count || entryReport.runningCount;
  const completedCount =
    run.completed_count ||
    run.success_count + run.failed_count + run.skipped_count + run.cancelled_count;
  return {
    totalCount: run.total_count || entryReport.totalCount,
    queuedCount: Math.max(entryReport.queuedCount, run.queued_count - runningCount),
    runningCount,
    completedCount,
    successCount: run.success_count,
    failedCount: run.failed_count,
    skippedCount: run.skipped_count,
    stoppedCount: run.cancelled_count,
    fileCount: run.file_count || entryReport.fileCount,
    byteCount: run.byte_count || entryReport.byteCount,
    currentDeviceName: run.current_device_name || entryReport.currentDeviceName,
    currentJobId: run.current_job_id || entryReport.currentJobId,
  };
}

function sessionFromRun(run: BulkBackupRun): BulkBackupSession {
  const entries = run.items.map(itemToEntry);
  return {
    phase: RUN_TERMINAL.has(run.status) ? 'done' : 'running',
    runId: run.id,
    runStatus: run.status,
    startedBy: run.created_by || undefined,
    entries,
    report: reportFromRun(run, entries),
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
  const activeCount = entries.filter(
    (entry) => entry.phase === 'active' || entry.phase === 'running',
  ).length;
  const pendingCount = entries.filter(isPendingControlDevice).length;

  if (status === 'pausing') {
    const parts = [];
    if (activeCount > 0) parts.push(`${activeCount} completing`);
    if (pendingCount > 0) parts.push(`${pendingCount} will pause`);
    return parts.length > 0 ? parts.join('; ') : null;
  }
  if (status === 'cancelling') {
    const parts = [];
    if (activeCount > 0) parts.push(`${activeCount} completing`);
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
    if (entry.phase === 'active' || entry.phase === 'running') return 'completing';
    if (isPendingControlDevice(entry)) return 'will pause';
  }
  if (runStatus === 'paused' && isPendingControlDevice(entry)) {
    return 'paused';
  }
  if (runStatus === 'cancelling') {
    if (entry.phase === 'active' || entry.phase === 'running') return 'completing';
    if (isPendingControlDevice(entry)) return 'will stop';
  }
  if (entry.phase === 'active') {
    return 'preparing...';
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
  if (
    label === 'completing' ||
    label === 'preparing...' ||
    label === 'checking...' ||
    entry.phase === 'running'
  ) {
    return 'text-primary animate-pulse';
  }
  return 'text-on-bg-secondary';
}

function bulkDownloadBatchFilename(batchIndex: number, batchCount: number): string {
  const timestamp = new Date().toISOString().replace(/\D/g, '').slice(0, 14);
  return `THEIA_BACKUPS_batch-${batchIndex + 1}-of-${batchCount}_${timestamp}.zip`;
}

function bulkBackupExclusionReason(device: Device): string | null {
  if (device.status === 'down') return 'Offline';
  if (device.polling_enabled === false) return 'Polling off';
  return null;
}

function selectableBulkBackupDeviceIds(devices: Device[]): Set<string> {
  return new Set(
    devices
      .filter((device) => bulkBackupExclusionReason(device) === null)
      .map((device) => device.id),
  );
}

/** Renders the BulkBackupPanel component within the operations dashboard. */
export function BulkBackupPanel({ devices: allDevices }: BulkBackupPanelProps) {
  const devices = useMemo(
    () => allDevices.filter((d) => d.device_type !== 'virtual'),
    [allDevices],
  );
  const selectableDeviceIds = useMemo(() => selectableBulkBackupDeviceIds(devices), [devices]);
  const deviceIdsKey = devices
    .map((d) => `${d.id}:${d.status}:${d.polling_enabled === false ? 'polling-off' : 'polling-on'}`)
    .join('\0');
  const [session, setSession] = useState<BulkBackupSession>(() => bulkBackupSession);
  const { phase, entries, error, downloading } = session;
  const [selectedDeviceIds, setSelectedDeviceIds] = useState<Set<string>>(
    () => new Set(selectableDeviceIds),
  );
  const [bulkOperationStatus, setBulkOperationStatus] = useState<BulkOperationStatus | null>(null);
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
    let active = true;
    void fetchBulkOperationStatus()
      .then((status) => {
        if (active) {
          setBulkOperationStatus(status);
        }
      })
      .catch(() => {});
    return () => {
      active = false;
    };
  }, []);

  useEffect(() => {
    entriesRef.current = entries;
    runIdRef.current = session.runId;
  }, [entries, session.runId]);

  useEffect(() => {
    if (phase !== 'idle') return;
    setSelectedDeviceIds(new Set(selectableDeviceIds));
  }, [deviceIdsKey, phase, selectableDeviceIds]);

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

  const selectedDevices = devices.filter(
    (device) => selectedDeviceIds.has(device.id) && selectableDeviceIds.has(device.id),
  );
  const selectedCount = selectedDevices.length;

  const setAllDevicesSelected = () => {
    setSelectedDeviceIds(new Set(selectableDeviceIds));
  };

  const clearSelectedDevices = () => {
    setSelectedDeviceIds(new Set());
  };

  const toggleSelectedDevice = (deviceID: string) => {
    if (!selectableDeviceIds.has(deviceID)) {
      return;
    }
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
        fileCount: 0,
        byteCount: 0,
      };
    });
    entriesRef.current = preliminaryEntries;
    setBulkBackupSession({
      phase: 'running',
      entries: preliminaryEntries,
      report: reportFromEntries(preliminaryEntries),
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

  const report = session.report ?? reportFromEntries(entries);
  const successCount = report.successCount;
  const failedCount = report.failedCount;
  const skippedCount = report.skippedCount;
  const stoppedCount = report.stoppedCount;
  const doneCount = report.completedCount;
  const activeCount = Math.max(report.totalCount - report.completedCount, 0);
  const downloadableSuccessCount = entries.filter((e) => e.phase === 'success').length;
  const bulkBackupGroupSize = positiveIntegerOrFallback(
    bulkOperationStatus?.bulk_backup_run.batch_size,
    BULK_BACKUP_GROUP_SIZE,
  );
  const bulkDownloadBatchSize = positiveIntegerOrFallback(
    bulkOperationStatus?.bulk_download.max_devices,
    BULK_DEVICE_BATCH_SIZE,
  );
  const bulkDownloadMaxFiles = positiveIntegerOrFallback(
    bulkOperationStatus?.bulk_download.max_files,
    0,
  );
  const bulkDownloadMaxBytes = positiveIntegerOrFallback(
    bulkOperationStatus?.bulk_download.max_bytes,
    0,
  );
  const downloadBatchCount = Math.ceil(downloadableSuccessCount / bulkDownloadBatchSize);
  const controlSummary = controlProgressSummary(entries, session.runStatus);
  const pauseSupported = bulkOperationStatus?.bulk_backup_run.can_pause !== false;
  const resumeSupported = bulkOperationStatus?.bulk_backup_run.can_resume !== false;
  const cancelSupported = bulkOperationStatus?.bulk_backup_run.can_cancel !== false;
  const canPause =
    pauseSupported && phase === 'running' && session.runId && session.runStatus === 'running';
  const canResume =
    resumeSupported && phase === 'running' && session.runId && session.runStatus === 'paused';
  const canStop =
    cancelSupported &&
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
      const candidates = await loadBulkDownloadCandidates(entries);
      const { batches, error } = planBulkDownloadBatches(candidates, {
        maxDevices: bulkDownloadBatchSize,
        maxFiles: bulkDownloadMaxFiles,
        maxBytes: bulkDownloadMaxBytes,
      });
      if (error) {
        setBulkBackupSession((current) => ({ ...current, error }));
        return;
      }
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
            · {selectedCount} selected · groups of {bulkBackupGroupSize}
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
              const exclusionReason = bulkBackupExclusionReason(device);
              return (
                <label
                  key={device.id}
                  className={`flex items-center gap-2 rounded-md px-2 py-1 text-xs ${
                    exclusionReason ? 'text-on-bg-muted' : 'text-on-bg hover:bg-surface-high'
                  }`}
                >
                  <input
                    type="checkbox"
                    checked={selectedDeviceIds.has(device.id) && !exclusionReason}
                    onChange={() => toggleSelectedDevice(device.id)}
                    disabled={exclusionReason !== null}
                    aria-label={`Select ${name}`}
                    className="h-3.5 w-3.5"
                  />
                  <span className="truncate">{name}</span>
                  {exclusionReason && (
                    <span className="ml-auto shrink-0 text-[10px] text-on-bg-muted">
                      {exclusionReason}
                    </span>
                  )}
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
                ? `Processing... ${doneCount}/${report.totalCount}`
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
          <div className="rounded-md border border-outline bg-surface px-3 py-2 text-xs text-on-bg-secondary">
            <div>
              Queued {report.queuedCount} · Running {report.runningCount} · Completed{' '}
              {report.completedCount} · Failed {report.failedCount} · Skipped {report.skippedCount}
            </div>
            {(report.fileCount > 0 || report.byteCount > 0) && (
              <div className="mt-1 text-[10px] text-on-bg">
                Files {report.fileCount} · {formatByteCount(report.byteCount)}
              </div>
            )}
            {session.startedBy && (
              <div className="mt-1 truncate text-[10px] text-on-bg">
                Started by {session.startedBy}
              </div>
            )}
            {report.currentDeviceName && (
              <div className="mt-1 truncate text-[10px] text-on-bg">
                Current {report.currentDeviceName}
                {report.currentJobId ? ` · job ${report.currentJobId}` : ''}
              </div>
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
              style={{
                width: `${report.totalCount > 0 ? (doneCount / report.totalCount) * 100 : 0}%`,
              }}
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
                    {e.fileCount > 0 && (
                      <span className="text-[9px] text-on-bg-secondary">
                        {formatFileCount(e.fileCount)} · {formatByteCount(e.byteCount)}
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
      {phase === 'done' && downloadableSuccessCount > 0 && (
        <div className="space-y-2">
          <p className="text-xs text-on-bg-secondary">
            {downloadBatchCount > 1
              ? `Downloads will be split into ${downloadBatchCount} ZIP files of up to ${bulkDownloadBatchSize} devices each.`
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
