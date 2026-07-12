/**
 * Exercises single-device backup panel host-key recovery behavior.
 */
import { act, fireEvent, render, screen } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import {
  fetchBackupJob,
  fetchBackupJobs,
  fetchLatestBackupJob,
  resetSSHHostKey,
  triggerBackup,
} from '../../api/client';
import type { BackupJob, Device } from '../../types/api';
import { BackupPanel } from './BackupPanel';

vi.mock('../../api/client', () => ({
  fetchBackupJob: vi.fn(),
  fetchBackupJobs: vi.fn(),
  fetchLatestBackupJob: vi.fn(),
  resetSSHHostKey: vi.fn(),
  triggerBackup: vi.fn(),
}));

function mockDevice(overrides: Partial<Device> = {}): Device {
  return {
    id: 'dev-1',
    hostname: 'router-01',
    ip: '10.8.20.1',
    device_type: 'router',
    status: 'up',
    sys_name: 'router-01',
    sys_descr: 'RouterOS',
    hardware_model: 'RB4011',
    vendor: 'mikrotik',
    managed: true,
    interfaces: [],
    backup_supported: true,
    metrics_source: 'snmp',
    prometheus_label_name: 'instance',
    prometheus_label_value: '10.8.20.1:9100',
    area_ids: [],
    ...overrides,
  };
}

function mockBackupJob(overrides: Partial<BackupJob> = {}): BackupJob {
  return {
    id: 'job-1',
    device_id: 'dev-1',
    status: 'pending',
    error_message: '',
    created_at: '2026-06-28T00:00:00Z',
    files: [],
    ...overrides,
  };
}

function deferred<T>() {
  let resolve!: (value: T) => void;
  let reject!: (reason?: unknown) => void;
  const promise = new Promise<T>((resolvePromise, rejectPromise) => {
    resolve = resolvePromise;
    reject = rejectPromise;
  });
  return { promise, resolve, reject };
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
  vi.mocked(fetchBackupJobs).mockResolvedValue([]);
  vi.mocked(fetchLatestBackupJob).mockResolvedValue(null);
  vi.mocked(triggerBackup).mockResolvedValue(mockBackupJob({ status: 'pending' }));
  vi.mocked(resetSSHHostKey).mockResolvedValue({
    target: '10.8.20.1',
    port: 22,
    removed: true,
  });
});

afterEach(() => {
  vi.clearAllTimers();
  vi.useRealTimers();
});

describe('BackupPanel active job rehydration', () => {
  it('restores and polls an active backup job after the panel is reopened', async () => {
    vi.mocked(fetchBackupJobs).mockResolvedValue([
      mockBackupJob({ id: 'job-active', status: 'running' }),
      mockBackupJob({ id: 'job-old', status: 'success' }),
    ]);
    vi.mocked(fetchBackupJob).mockResolvedValue(
      mockBackupJob({ id: 'job-active', status: 'success' }),
    );

    const firstView = render(<BackupPanel device={mockDevice()} />);

    await flushPromises();
    expect(screen.getAllByText('Backup in progress...').length).toBeGreaterThan(0);

    firstView.unmount();
    render(<BackupPanel device={mockDevice()} />);

    await flushPromises();
    expect(screen.getAllByText('Backup in progress...').length).toBeGreaterThan(0);

    await act(async () => {
      await vi.advanceTimersByTimeAsync(2000);
    });

    expect(fetchBackupJob).toHaveBeenCalledWith('job-active');
  });

  it('waits for a slow active-job request to settle before scheduling the next poll', async () => {
    const firstPoll = deferred<BackupJob>();
    vi.mocked(fetchBackupJobs).mockResolvedValue([
      mockBackupJob({ id: 'job-active', status: 'running' }),
    ]);
    vi.mocked(fetchBackupJob)
      .mockImplementationOnce(() => firstPoll.promise)
      .mockResolvedValue(mockBackupJob({ id: 'job-active', status: 'running' }));

    render(<BackupPanel device={mockDevice()} />);
    await flushPromises();

    await act(async () => {
      await vi.advanceTimersByTimeAsync(2000);
    });
    expect(fetchBackupJob).toHaveBeenCalledTimes(1);

    await act(async () => {
      await vi.advanceTimersByTimeAsync(10_000);
    });
    expect(fetchBackupJob).toHaveBeenCalledTimes(1);

    await act(async () => {
      firstPoll.resolve(mockBackupJob({ id: 'job-active', status: 'running' }));
      await firstPoll.promise;
    });
    await act(async () => {
      await vi.advanceTimersByTimeAsync(1999);
    });
    expect(fetchBackupJob).toHaveBeenCalledTimes(1);

    await act(async () => {
      await vi.advanceTimersByTimeAsync(1);
    });
    expect(fetchBackupJob).toHaveBeenCalledTimes(2);
  });

  it('retries polling after a transient active-job failure', async () => {
    vi.mocked(fetchBackupJobs).mockResolvedValue([
      mockBackupJob({ id: 'job-active', status: 'running' }),
    ]);
    vi.mocked(fetchBackupJob)
      .mockRejectedValueOnce(new Error('temporary'))
      .mockResolvedValueOnce(mockBackupJob({ id: 'job-active', status: 'success' }));

    render(<BackupPanel device={mockDevice()} />);
    await flushPromises();

    await act(async () => {
      await vi.advanceTimersByTimeAsync(2000);
    });
    expect(fetchBackupJob).toHaveBeenCalledTimes(1);

    await act(async () => {
      await vi.advanceTimersByTimeAsync(2000);
    });
    expect(fetchBackupJob).toHaveBeenCalledTimes(2);
  });

  it('ignores a terminal poll result that settles after unmount', async () => {
    const pendingPoll = deferred<BackupJob>();
    vi.mocked(fetchBackupJobs).mockResolvedValue([
      mockBackupJob({ id: 'job-active', status: 'running' }),
    ]);
    vi.mocked(fetchBackupJob).mockImplementationOnce(() => pendingPoll.promise);

    const view = render(<BackupPanel device={mockDevice()} />);
    await flushPromises();
    await act(async () => {
      await vi.advanceTimersByTimeAsync(2000);
    });
    expect(fetchLatestBackupJob).toHaveBeenCalledTimes(1);

    view.unmount();
    await act(async () => {
      pendingPoll.resolve(mockBackupJob({ id: 'job-active', status: 'success' }));
      await pendingPoll.promise;
    });

    expect(fetchLatestBackupJob).toHaveBeenCalledTimes(1);
    await act(async () => {
      await vi.advanceTimersByTimeAsync(10_000);
    });
    expect(fetchBackupJob).toHaveBeenCalledTimes(1);
  });
});

describe('BackupPanel host-key mismatch recovery', () => {
  it('offers a confirmed SSH host key reset after a host-key mismatch failure', async () => {
    vi.mocked(fetchBackupJob).mockResolvedValue(
      mockBackupJob({
        status: 'failed',
        error_code: 'ssh_host_key_mismatch',
        error_message: 'SSH connection to 10.8.20.1 failed: SSH host key mismatch for 10.8.20.1:22',
      }),
    );
    vi.spyOn(window, 'confirm').mockReturnValue(true);

    render(<BackupPanel device={mockDevice()} />);

    await act(async () => {
      fireEvent.click(screen.getByRole('button', { name: /backup now/i }));
    });
    await act(async () => {
      await vi.advanceTimersByTimeAsync(2000);
    });

    expect(screen.getByText('SSH host key changed')).toBeInTheDocument();
    await act(async () => {
      fireEvent.click(screen.getByRole('button', { name: /reset ssh host key/i }));
      await Promise.resolve();
    });

    expect(resetSSHHostKey).toHaveBeenCalledWith('dev-1');
    expect(
      screen.getByText('SSH host key reset. Run backup again to trust the new key.'),
    ).toBeInTheDocument();
  });
});
