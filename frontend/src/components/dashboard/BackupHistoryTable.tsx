/**
 * Defines backup history table behavior for the operations dashboard.
 * Keeps table, backup, and device-management responsibilities isolated by module.
 */
import { useCallback, useEffect, useState } from 'react';
import { backupFileDownloadUrl, deleteBackupJob, fetchBackupJobs } from '../../api/client';
import { useAsyncPolling } from '../../hooks/useAsyncPolling';
import { type BackupJob } from '../../types/api';

interface BackupHistoryTableProps {
  deviceId: string;
  onViewConfig: () => void;
}

const statusColors: Record<string, string> = {
  success: 'text-status-up',
  failed: 'text-status-down',
  running: 'text-status-probing',
  pending: 'text-on-bg-secondary',
};

/** Renders the BackupHistoryTable component within the operations dashboard. */
export function BackupHistoryTable({ deviceId, onViewConfig }: BackupHistoryTableProps) {
  const [jobs, setJobs] = useState<BackupJob[]>([]);
  const [loading, setLoading] = useState(true);
  const [expandedJob, setExpandedJob] = useState<string | null>(null);

  const load = useCallback(async () => {
    return fetchBackupJobs(deviceId);
  }, [deviceId]);

  useEffect(() => {
    let cancelled = false;
    void load()
      .then((data) => {
        if (!cancelled) setJobs(data);
      })
      .catch((error: unknown) => {
        if (!cancelled) console.error('Failed to fetch backup jobs:', error);
      })
      .finally(() => {
        if (!cancelled) setLoading(false);
      });
    return () => {
      cancelled = true;
    };
  }, [load]);

  const polling = useAsyncPolling({
    intervalMs: 2000,
    poll: load,
    onResult: (data) => {
      setJobs(data);
    },
    onError: (error) => console.error('Failed to fetch backup jobs:', error),
  });

  // Poll while any job is pending/running
  const hasActive = jobs.some((job) => job.status === 'pending' || job.status === 'running');
  useEffect(() => {
    if (hasActive) polling.start();
    else polling.stop();
    return polling.stop;
  }, [deviceId, hasActive, polling.start, polling.stop]);

  const handleDelete = async (id: string) => {
    try {
      await deleteBackupJob(id);
      setJobs((prev) => prev.filter((j) => j.id !== id));
    } catch (err) {
      console.error('Failed to delete backup job:', err);
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
    if (bytes === 0) return '-';
    if (bytes < 1024) return `${bytes} B`;
    if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`;
    return `${(bytes / (1024 * 1024)).toFixed(1)} MB`;
  };

  if (loading) {
    return <div className="text-xs text-on-bg-secondary">Loading...</div>;
  }

  if (jobs.length === 0) {
    return <div className="text-xs text-on-bg-secondary">No backups found</div>;
  }

  return (
    <div className="space-y-2 transition-colors duration-200">
      {jobs.map((job) => {
        const fileCount = job.files?.length ?? 0;
        const totalSize = job.files?.reduce((sum, f) => sum + f.size_bytes, 0) ?? 0;
        const isExpanded = expandedJob === job.id;

        return (
          <div key={job.id} className="rounded-lg bg-surface-high overflow-hidden">
            {/* Job summary row */}
            <button
              type="button"
              className="w-full p-3 text-left cursor-pointer hover:bg-elevated/30 transition-colors"
              onClick={() => setExpandedJob(isExpanded ? null : job.id)}
            >
              <div className="flex items-center justify-between mb-1.5">
                <span
                  className={`text-xs font-medium capitalize ${statusColors[job.status] ?? ''}`}
                >
                  {job.status}
                </span>
                <span className="text-[10px] text-on-bg-secondary font-mono">
                  {formatDate(job.created_at)}
                </span>
              </div>
              <div className="flex items-center justify-between text-[10px] text-on-bg-secondary">
                <span className="font-mono">
                  {fileCount} file{fileCount !== 1 ? 's' : ''} / {formatSize(totalSize)}
                </span>
                <span className="text-on-bg-secondary/50">{isExpanded ? '▲' : '▼'}</span>
              </div>
              {job.error_message && (
                <div className="text-[10px] text-status-down mt-1 break-words">
                  {job.error_message}
                </div>
              )}
            </button>

            {/* Expanded file list */}
            {isExpanded && (
              <div className="mt-3 p-3 space-y-2">
                {job.files && job.files.length > 0 ? (
                  job.files.map((file) => (
                    <div key={file.id} className="flex items-center justify-between text-[10px]">
                      <div className="flex-1 min-w-0">
                        <div className="text-on-bg font-mono truncate">{file.file_name}</div>
                        <div className="text-on-bg-secondary">
                          {file.file_type} / {formatSize(file.size_bytes)}
                          {file.file_hash && (
                            <span className="ml-2 font-mono">
                              {file.file_hash.substring(0, 12)}
                            </span>
                          )}
                        </div>
                      </div>
                      <a
                        href={backupFileDownloadUrl(file.id)}
                        className="ml-2 shrink-0 rounded px-2 py-1 font-medium text-primary border border-primary/30 hover:bg-primary/10 transition-colors"
                        download
                      >
                        Download
                      </a>
                    </div>
                  ))
                ) : (
                  <div className="text-[10px] text-on-bg-secondary">No files</div>
                )}

                <div className="flex gap-1.5 pt-1">
                  {job.status === 'success' && (
                    <button
                      type="button"
                      onClick={onViewConfig}
                      className="rounded px-2 py-1 text-[10px] font-medium text-primary border border-primary/30 hover:bg-primary/10 transition-colors"
                    >
                      View Config
                    </button>
                  )}
                  <button
                    type="button"
                    onClick={() => handleDelete(job.id)}
                    className="rounded px-2 py-1 text-[10px] font-medium text-status-down border border-status-down/30 hover:bg-status-down/10 transition-colors"
                  >
                    Delete
                  </button>
                </div>
              </div>
            )}
          </div>
        );
      })}
    </div>
  );
}
