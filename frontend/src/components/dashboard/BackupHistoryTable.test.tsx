/**
 * Exercises backup history polling lifecycle behavior.
 */
import { act, render, screen } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { deleteBackupJob, fetchBackupJobs } from '../../api/client';
import type { BackupJob } from '../../types/api';
import { BackupHistoryTable } from './BackupHistoryTable';

vi.mock('../../api/client', () => ({
  backupFileDownloadUrl: vi.fn((id: string) => `/backups/${id}`),
  deleteBackupJob: vi.fn(),
  fetchBackupJobs: vi.fn(),
}));

function mockBackupJob(overrides: Partial<BackupJob> = {}): BackupJob {
  return {
    id: 'job-1',
    device_id: 'dev-1',
    status: 'running',
    error_message: '',
    created_at: '2026-06-28T00:00:00Z',
    files: [],
    ...overrides,
  };
}

function deferred<T>() {
  let resolve!: (value: T) => void;
  const promise = new Promise<T>((resolvePromise) => {
    resolve = resolvePromise;
  });
  return { promise, resolve };
}

async function flushPromises() {
  await act(async () => {
    await Promise.resolve();
    await Promise.resolve();
  });
}

beforeEach(() => {
  vi.useFakeTimers();
  vi.resetAllMocks();
  vi.mocked(deleteBackupJob).mockResolvedValue(undefined);
  vi.spyOn(console, 'error').mockImplementation(() => undefined);
});

afterEach(() => {
  vi.clearAllTimers();
  vi.useRealTimers();
  vi.restoreAllMocks();
});

describe('BackupHistoryTable polling', () => {
  it('does not overlap a slow request and stops after the active job completes', async () => {
    const firstPoll = deferred<BackupJob[]>();
    vi.mocked(fetchBackupJobs)
      .mockResolvedValueOnce([mockBackupJob()])
      .mockImplementationOnce(() => firstPoll.promise)
      .mockResolvedValue([mockBackupJob({ status: 'success' })]);

    render(<BackupHistoryTable deviceId="dev-1" onViewConfig={vi.fn()} />);
    await flushPromises();
    expect(screen.getByText('running')).toBeInTheDocument();

    await act(async () => {
      await vi.advanceTimersByTimeAsync(2000);
    });
    expect(fetchBackupJobs).toHaveBeenCalledTimes(2);

    await act(async () => {
      await vi.advanceTimersByTimeAsync(10_000);
    });
    expect(fetchBackupJobs).toHaveBeenCalledTimes(2);

    await act(async () => {
      firstPoll.resolve([mockBackupJob({ status: 'success' })]);
      await firstPoll.promise;
    });
    expect(screen.getByText('success')).toBeInTheDocument();

    await act(async () => {
      await vi.advanceTimersByTimeAsync(10_000);
    });
    expect(fetchBackupJobs).toHaveBeenCalledTimes(2);
  });

  it('retries after a failed poll while an active job remains visible', async () => {
    vi.mocked(fetchBackupJobs)
      .mockResolvedValueOnce([mockBackupJob()])
      .mockRejectedValueOnce(new Error('temporary'))
      .mockResolvedValueOnce([mockBackupJob({ status: 'success' })]);

    render(<BackupHistoryTable deviceId="dev-1" onViewConfig={vi.fn()} />);
    await flushPromises();

    await act(async () => {
      await vi.advanceTimersByTimeAsync(2000);
    });
    expect(fetchBackupJobs).toHaveBeenCalledTimes(2);

    await act(async () => {
      await vi.advanceTimersByTimeAsync(2000);
    });
    expect(fetchBackupJobs).toHaveBeenCalledTimes(3);
    expect(screen.getByText('success')).toBeInTheDocument();
  });

  it('does not let an old device poll overwrite a newer device load', async () => {
    const oldDevicePoll = deferred<BackupJob[]>();
    vi.mocked(fetchBackupJobs).mockImplementation((deviceId: string) => {
      if (deviceId === 'dev-2') {
        return Promise.resolve([
          mockBackupJob({ id: 'job-new', device_id: 'dev-2', status: 'success' }),
        ]);
      }
      if (vi.mocked(fetchBackupJobs).mock.calls.length === 1) {
        return Promise.resolve([mockBackupJob({ id: 'job-old', status: 'running' })]);
      }
      return oldDevicePoll.promise;
    });

    const view = render(<BackupHistoryTable deviceId="dev-1" onViewConfig={vi.fn()} />);
    await flushPromises();
    await act(async () => {
      await vi.advanceTimersByTimeAsync(2000);
    });

    view.rerender(<BackupHistoryTable deviceId="dev-2" onViewConfig={vi.fn()} />);
    await flushPromises();
    expect(screen.getByText('success')).toBeInTheDocument();

    await act(async () => {
      oldDevicePoll.resolve([mockBackupJob({ id: 'job-old', status: 'failed' })]);
      await oldDevicePoll.promise;
    });

    expect(screen.getByText('success')).toBeInTheDocument();
    expect(screen.queryByText('failed')).not.toBeInTheDocument();
  });
});
