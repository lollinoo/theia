import { useCallback, useEffect, useState } from 'react';
import {
  backupFileDownloadUrl,
  fetchBackupFileContent,
  fetchLatestBackupJob,
} from '../../api/client';
import { type BackupFile, type BackupJob } from '../../types/api';

interface ConfigViewerProps {
  deviceId: string;
}

const FILE_TYPE_LABELS: Record<string, string> = {
  running: 'Default',
  verbose: 'Verbose',
  compact: 'Compact',
  binary: 'Binary',
};

const FILE_TYPE_ORDER = ['running', 'verbose', 'compact', 'binary'];

export function ConfigViewer({ deviceId }: ConfigViewerProps) {
  const [job, setJob] = useState<BackupJob | null>(null);
  const [loading, setLoading] = useState(true);
  const [activeTab, setActiveTab] = useState('running');
  const [content, setContent] = useState<string | null>(null);
  const [contentLoading, setContentLoading] = useState(false);
  const [copied, setCopied] = useState(false);

  const load = useCallback(async () => {
    try {
      const data = await fetchLatestBackupJob(deviceId);
      setJob(data);
      // Default to first available file type
      if (data?.files?.length) {
        const firstType = FILE_TYPE_ORDER.find((t) => data.files.some((f) => f.file_type === t));
        if (firstType) setActiveTab(firstType);
      }
    } catch (err) {
      console.error('Failed to fetch config:', err);
    } finally {
      setLoading(false);
    }
  }, [deviceId]);

  useEffect(() => {
    load();
  }, [load]);

  const activeFile: BackupFile | undefined = job?.files?.find((f) => f.file_type === activeTab);
  const contentLines = (() => {
    if (!content) {
      return [] as Array<{ key: string; line: string; number: number }>;
    }

    const seen = new Map<string, number>();
    return content.split('\n').map((line, index) => {
      const occurrence = (seen.get(line) ?? 0) + 1;
      seen.set(line, occurrence);
      return {
        key: `${line}-${occurrence}`,
        line,
        number: index + 1,
      };
    });
  })();

  // Load text content when tab changes
  useEffect(() => {
    if (!activeFile || activeTab === 'binary') {
      setContent(null);
      return;
    }
    setContentLoading(true);
    fetchBackupFileContent(activeFile.id)
      .then(setContent)
      .catch(() => setContent(null))
      .finally(() => setContentLoading(false));
  }, [activeFile?.id, activeTab]);

  const handleCopy = async () => {
    if (!content) return;
    try {
      await navigator.clipboard.writeText(content);
      setCopied(true);
      setTimeout(() => setCopied(false), 2000);
    } catch {
      const textarea = document.createElement('textarea');
      textarea.value = content;
      document.body.appendChild(textarea);
      textarea.select();
      document.execCommand('copy');
      document.body.removeChild(textarea);
      setCopied(true);
      setTimeout(() => setCopied(false), 2000);
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

  if (loading) {
    return <div className="text-xs text-on-bg-secondary">Loading configuration...</div>;
  }

  if (!job || !job.files?.length) {
    return <div className="text-xs text-on-bg-secondary">No configuration backup available</div>;
  }

  const availableTypes = FILE_TYPE_ORDER.filter((t) => job.files.some((f) => f.file_type === t));

  return (
    <div className="space-y-3 transition-colors duration-200">
      {/* Tab selector */}
      <div className="flex gap-1 pb-2">
        {availableTypes.map((type) => (
          <button
            key={type}
            type="button"
            onClick={() => setActiveTab(type)}
            className={`rounded-md px-2.5 py-1 text-[10px] font-medium transition-colors ${
              activeTab === type
                ? 'bg-primary text-white'
                : 'text-on-bg-secondary hover:text-on-bg hover:bg-elevated'
            }`}
          >
            {FILE_TYPE_LABELS[type] || type}
          </button>
        ))}
      </div>

      {/* Partial success warning */}
      {job.status === 'success' && job.error_message?.startsWith('partial:') && (
        <div className="rounded-md border border-yellow-500/20 bg-yellow-500/5 px-3 py-2 text-[10px] text-yellow-400">
          <div>Some backup types failed to export. Completed files are shown below.</div>
          {job.error_message.replace('partial: ', '').trim() && (
            <div className="mt-1 text-yellow-400/70">
              {job.error_message.replace('partial: ', '')}
            </div>
          )}
        </div>
      )}

      {/* Metadata */}
      {activeFile && (
        <div className="flex items-center justify-between">
          <div className="space-y-0.5">
            <div className="text-[10px] text-on-bg-secondary font-mono">
              {formatDate(activeFile.created_at)} / {formatSize(activeFile.size_bytes)}
            </div>
            {activeFile.file_hash && (
              <div className="text-[10px] text-on-bg-secondary font-mono">
                SHA-256: {activeFile.file_hash.substring(0, 16)}...
              </div>
            )}
          </div>
          {activeTab === 'binary' ? (
            <a
              href={backupFileDownloadUrl(activeFile.id)}
              className="rounded-md bg-surface-high px-2.5 py-1 text-[10px] font-medium text-on-bg-secondary hover:text-on-bg hover:bg-elevated transition-colors"
              download
            >
              Download .backup
            </a>
          ) : (
            <button
              type="button"
              onClick={handleCopy}
              className="rounded-md bg-surface-high px-2.5 py-1 text-[10px] font-medium text-on-bg-secondary hover:text-on-bg hover:bg-elevated transition-colors"
            >
              {copied ? 'Copied!' : 'Copy'}
            </button>
          )}
        </div>
      )}

      {/* Content area */}
      {activeTab === 'binary' ? (
        <div className="rounded-lg bg-surface-high p-4 text-center">
          <div className="text-xs text-on-bg-secondary mb-2">Binary backup file</div>
          <div className="text-[10px] text-on-bg-secondary/70 font-mono mb-3">
            {activeFile?.file_name}
          </div>
          {activeFile && (
            <a
              href={backupFileDownloadUrl(activeFile.id)}
              className="inline-block rounded-md bg-primary px-3 py-1.5 text-xs font-medium text-white hover:bg-primary/90 transition-colors"
              download
            >
              Download {formatSize(activeFile.size_bytes)}
            </a>
          )}
        </div>
      ) : contentLoading ? (
        <div className="text-xs text-on-bg-secondary">Loading file content...</div>
      ) : content ? (
        <div className="rounded-lg bg-surface-high overflow-auto max-h-[calc(100vh-220px)]">
          <pre className="font-mono text-[11px] leading-[1.6] p-0 m-0">
            <code>
              {contentLines.map(({ key, line, number }) => (
                <div key={key} className="flex hover:bg-elevated/30">
                  <span className="select-none text-on-bg-secondary/50 text-right pr-3 pl-2 min-w-[3rem]">
                    {number}
                  </span>
                  <span className="pl-3 pr-3 text-on-bg whitespace-pre">{line}</span>
                </div>
              ))}
            </code>
          </pre>
        </div>
      ) : (
        <div className="text-xs text-on-bg-secondary">No content available</div>
      )}
    </div>
  );
}
