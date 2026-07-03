/**
 * Exercises config viewer operations dashboard behavior so refactors preserve the documented contract.
 */
import { act, fireEvent, render, screen, waitFor } from '@testing-library/react';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { ConfigViewer } from './ConfigViewer';

vi.mock('../../api/client', () => ({
  fetchLatestBackupJob: vi.fn(),
  fetchBackupFileContent: vi.fn(),
  backupFileDownloadUrl: vi.fn(),
}));

import {
  backupFileDownloadUrl,
  fetchBackupFileContent,
  fetchLatestBackupJob,
} from '../../api/client';

beforeEach(() => {
  vi.resetAllMocks();
  vi.mocked(fetchBackupFileContent).mockResolvedValue({
    content: '# config content',
    inline: true,
    download_url: '/api/v1/backup-files/f-1/download',
    size_bytes: 16,
    max_inline_size_bytes: 1048576,
  });
  vi.mocked(backupFileDownloadUrl).mockImplementation(
    (id: string) => `/api/v1/backup-files/${id}/download`,
  );
});

function deferred<T>() {
  let resolve!: (value: T) => void;
  const promise = new Promise<T>((res) => {
    resolve = res;
  });
  return { promise, resolve };
}

describe('ConfigViewer', () => {
  it('renders yellow partial-backup banner', async () => {
    vi.mocked(fetchLatestBackupJob).mockResolvedValue({
      id: 'job-1',
      device_id: 'dev-1',
      status: 'success',
      error_message: 'partial: verbose export failed',
      created_at: '2026-01-01T00:00:00Z',
      files: [
        {
          id: 'f-1',
          job_id: 'job-1',
          file_type: 'running',
          file_name: 'export.rsc',
          file_hash: 'abc123',
          size_bytes: 100,
          created_at: '2026-01-01T00:00:00Z',
        },
      ],
    });

    render(<ConfigViewer deviceId="dev-1" />);

    await waitFor(() => {
      expect(
        screen.getByText('Some backup types failed to export. Completed files are shown below.'),
      ).toBeInTheDocument();
    });

    expect(screen.getByText('verbose export failed')).toBeInTheDocument();
  });

  it('does not show banner for normal success job', async () => {
    vi.mocked(fetchLatestBackupJob).mockResolvedValue({
      id: 'job-2',
      device_id: 'dev-1',
      status: 'success',
      error_message: '',
      created_at: '2026-01-01T00:00:00Z',
      files: [
        {
          id: 'f-2',
          job_id: 'job-2',
          file_type: 'running',
          file_name: 'export.rsc',
          file_hash: 'def456',
          size_bytes: 200,
          created_at: '2026-01-01T00:00:00Z',
        },
      ],
    });

    render(<ConfigViewer deviceId="dev-1" />);

    await waitFor(() => {
      expect(screen.getByText('# config content')).toBeInTheDocument();
    });

    expect(
      screen.queryByText('Some backup types failed to export. Completed files are shown below.'),
    ).not.toBeInTheDocument();
  });

  it('shows a download link instead of copy for non-inline text backups', async () => {
    vi.mocked(fetchLatestBackupJob).mockResolvedValue({
      id: 'job-3',
      device_id: 'dev-1',
      status: 'success',
      error_message: '',
      created_at: '2026-01-01T00:00:00Z',
      files: [
        {
          id: 'f-large',
          job_id: 'job-3',
          file_type: 'running',
          file_name: 'large.rsc',
          file_hash: 'abc123',
          size_bytes: 1048577,
          created_at: '2026-01-01T00:00:00Z',
        },
      ],
    });
    vi.mocked(fetchBackupFileContent).mockResolvedValue({
      content: '',
      inline: false,
      download_url: '/api/v1/backup-files/f-large/download',
      reason: 'too_large',
      size_bytes: 1048577,
      max_inline_size_bytes: 1048576,
    });

    render(<ConfigViewer deviceId="dev-1" />);

    const downloadLinks = await screen.findAllByRole('link', { name: /download/i });
    expect(downloadLinks).toHaveLength(2);
    for (const link of downloadLinks) {
      expect(link).toHaveAttribute('href', '/api/v1/backup-files/f-large/download');
    }
    expect(screen.queryByRole('button', { name: /^copy$/i })).not.toBeInTheDocument();
    expect(screen.getByText('Preview unavailable')).toBeInTheDocument();
  });

  it('infers restored generic text backup tabs from filenames', async () => {
    vi.mocked(fetchLatestBackupJob).mockResolvedValue({
      id: 'job-restored',
      device_id: 'dev-1',
      status: 'success',
      error_message: '',
      created_at: '2026-01-01T00:00:00Z',
      files: [
        {
          id: 'f-default',
          job_id: 'job-restored',
          file_type: 'rsc',
          file_name: '20260703_120000_router.rsc',
          file_hash: 'default123',
          size_bytes: 100,
          created_at: '2026-01-01T00:00:00Z',
        },
        {
          id: 'f-verbose',
          job_id: 'job-restored',
          file_type: 'rsc',
          file_name: '20260703_120000_router_verbose.rsc',
          file_hash: 'verbose123',
          size_bytes: 120,
          created_at: '2026-01-01T00:00:00Z',
        },
        {
          id: 'f-compact',
          job_id: 'job-restored',
          file_type: 'rsc',
          file_name: '20260703_120000_router_compact.rsc',
          file_hash: 'compact123',
          size_bytes: 80,
          created_at: '2026-01-01T00:00:00Z',
        },
      ],
    });
    vi.mocked(fetchBackupFileContent).mockImplementation((fileId: string) =>
      Promise.resolve({
        content: `# ${fileId} config`,
        inline: true,
        download_url: `/api/v1/backup-files/${fileId}/download`,
        size_bytes: 100,
        max_inline_size_bytes: 1048576,
      }),
    );

    render(<ConfigViewer deviceId="dev-1" />);

    expect(await screen.findByRole('button', { name: 'Default' })).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Verbose' })).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Compact' })).toBeInTheDocument();
    expect(await screen.findByText('# f-default config')).toBeInTheDocument();

    fireEvent.click(screen.getByRole('button', { name: 'Verbose' }));

    expect(await screen.findByText('# f-verbose config')).toBeInTheDocument();
    expect(fetchBackupFileContent).toHaveBeenCalledWith('f-verbose');
  });

  it('ignores stale content responses after switching text tabs', async () => {
    const runningContent = deferred<Awaited<ReturnType<typeof fetchBackupFileContent>>>();

    vi.mocked(fetchLatestBackupJob).mockResolvedValue({
      id: 'job-4',
      device_id: 'dev-1',
      status: 'success',
      error_message: '',
      created_at: '2026-01-01T00:00:00Z',
      files: [
        {
          id: 'f-running',
          job_id: 'job-4',
          file_type: 'running',
          file_name: 'running.rsc',
          file_hash: 'run123',
          size_bytes: 100,
          created_at: '2026-01-01T00:00:00Z',
        },
        {
          id: 'f-verbose',
          job_id: 'job-4',
          file_type: 'verbose',
          file_name: 'verbose.rsc',
          file_hash: 'verb123',
          size_bytes: 120,
          created_at: '2026-01-01T00:00:00Z',
        },
      ],
    });
    vi.mocked(fetchBackupFileContent).mockImplementation((fileId: string) => {
      if (fileId === 'f-running') {
        return runningContent.promise;
      }
      return Promise.resolve({
        content: '# verbose config',
        inline: true,
        download_url: '/api/v1/backup-files/f-verbose/download',
        size_bytes: 120,
        max_inline_size_bytes: 1048576,
      });
    });

    render(<ConfigViewer deviceId="dev-1" />);

    await waitFor(() => {
      expect(fetchBackupFileContent).toHaveBeenCalledWith('f-running');
    });

    fireEvent.click(screen.getByRole('button', { name: 'Verbose' }));

    expect(await screen.findByText('# verbose config')).toBeInTheDocument();

    await act(async () => {
      runningContent.resolve({
        content: '# running stale',
        inline: false,
        download_url: '/api/v1/backup-files/f-running/download',
        reason: 'too_large',
        size_bytes: 1048577,
        max_inline_size_bytes: 1048576,
      });
      await runningContent.promise;
    });

    expect(screen.getByText('# verbose config')).toBeInTheDocument();
    expect(screen.queryByText('# running stale')).not.toBeInTheDocument();
    expect(screen.queryByText('Preview unavailable')).not.toBeInTheDocument();
    expect(
      screen
        .queryAllByRole('link', { name: /download/i })
        .some((link) => link.getAttribute('href') === '/api/v1/backup-files/f-running/download'),
    ).toBe(false);
  });

  it('ignores stale job loads after switching devices', async () => {
    const deviceAJob = deferred<Awaited<ReturnType<typeof fetchLatestBackupJob>>>();

    vi.mocked(fetchLatestBackupJob).mockImplementation((requestedDeviceId: string) => {
      if (requestedDeviceId === 'device-a') {
        return deviceAJob.promise;
      }
      return Promise.resolve({
        id: 'job-b',
        device_id: 'device-b',
        status: 'success',
        error_message: '',
        created_at: '2026-01-01T00:00:00Z',
        files: [
          {
            id: 'b-running',
            job_id: 'job-b',
            file_type: 'running',
            file_name: 'device-b.rsc',
            file_hash: 'b123',
            size_bytes: 100,
            created_at: '2026-01-01T00:00:00Z',
          },
        ],
      });
    });
    vi.mocked(fetchBackupFileContent).mockImplementation((fileId: string) => {
      if (fileId === 'a-running') {
        return Promise.resolve({
          content: '# device a stale',
          inline: false,
          download_url: '/api/v1/backup-files/a-running/download',
          reason: 'too_large',
          size_bytes: 1048577,
          max_inline_size_bytes: 1048576,
        });
      }
      return Promise.resolve({
        content: '# device b config',
        inline: true,
        download_url: '/api/v1/backup-files/b-running/download',
        size_bytes: 100,
        max_inline_size_bytes: 1048576,
      });
    });

    const { rerender } = render(<ConfigViewer deviceId="device-a" />);

    await waitFor(() => {
      expect(fetchLatestBackupJob).toHaveBeenCalledWith('device-a');
    });

    rerender(<ConfigViewer deviceId="device-b" />);

    expect(await screen.findByText('# device b config')).toBeInTheDocument();

    await act(async () => {
      deviceAJob.resolve({
        id: 'job-a',
        device_id: 'device-a',
        status: 'success',
        error_message: '',
        created_at: '2026-01-01T00:00:00Z',
        files: [
          {
            id: 'a-running',
            job_id: 'job-a',
            file_type: 'running',
            file_name: 'device-a.rsc',
            file_hash: 'a123',
            size_bytes: 1048577,
            created_at: '2026-01-01T00:00:00Z',
          },
        ],
      });
      await deviceAJob.promise;
    });

    expect(screen.getByText('# device b config')).toBeInTheDocument();
    expect(screen.queryByText('# device a stale')).not.toBeInTheDocument();
    expect(screen.queryByText('Preview unavailable')).not.toBeInTheDocument();
    expect(
      screen
        .queryAllByRole('link', { name: /download/i })
        .some((link) => link.getAttribute('href') === '/api/v1/backup-files/a-running/download'),
    ).toBe(false);
  });
});
