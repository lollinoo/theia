import { render, screen, waitFor } from '@testing-library/react';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { ConfigViewer } from './ConfigViewer';

vi.mock('../../api/client', () => ({
  fetchLatestBackupJob: vi.fn(),
  fetchBackupFileContent: vi.fn().mockResolvedValue('# config content'),
  backupFileDownloadUrl: vi.fn((id: string) => `/api/v1/backup-files/${id}/download`),
}));

import { fetchLatestBackupJob } from '../../api/client';

beforeEach(() => {
  vi.clearAllMocks();
});

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
});
