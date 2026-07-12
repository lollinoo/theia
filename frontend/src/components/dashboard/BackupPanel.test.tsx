/**
 * Exercises single-device backup panel host-key recovery behavior.
 */
import { act, fireEvent, render, screen } from '@testing-library/react';
import { useLayoutEffect, useRef } from 'react';
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

interface Deferred<T> {
  promise: Promise<T>;
  resolve: (value: T) => void;
  reject: (reason?: unknown) => void;
}

interface RenderFrame {
  deviceId: string;
  text: string;
}

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

function deferred<T>(): Deferred<T> {
  let resolve!: (value: T) => void;
  let reject!: (reason?: unknown) => void;
  const promise = new Promise<T>((resolvePromise, rejectPromise) => {
    resolve = resolvePromise;
    reject = rejectPromise;
  });
  return { promise, resolve, reject };
}

function mockBackupFiles(count: number, jobId: string) {
  return Array.from({ length: count }, (_, index) => ({
    id: `file-${jobId}-${index}`,
    job_id: jobId,
    file_type: 'export',
    file_name: `${jobId}-${index}.rsc`,
    file_hash: `hash-${index}`,
    size_bytes: 100,
    created_at: '2026-06-28T00:00:00Z',
  }));
}

function BackupPanelRenderProbe({
  device,
  captureFrame,
}: {
  device: Device;
  captureFrame: (frame: RenderFrame) => void;
}) {
  const containerRef = useRef<HTMLDivElement>(null);
  useLayoutEffect(() => {
    captureFrame({ deviceId: device.id, text: containerRef.current?.textContent ?? '' });
  });

  return (
    <div ref={containerRef}>
      <BackupPanel device={device} />
    </div>
  );
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
});

describe('BackupPanel latest backup device ownership', () => {
  it('ignores a previous device response that resolves after the current device', async () => {
    const deviceARequest = deferred<BackupJob | null>();
    const deviceBRequest = deferred<BackupJob | null>();
    vi.mocked(fetchLatestBackupJob).mockImplementation((deviceId) =>
      deviceId === 'dev-a' ? deviceARequest.promise : deviceBRequest.promise,
    );
    const deviceA = mockDevice({ id: 'dev-a', sys_name: 'router-a' });
    const deviceB = mockDevice({ id: 'dev-b', sys_name: 'router-b' });

    const view = render(<BackupPanel device={deviceA} />);
    view.rerender(<BackupPanel device={deviceB} />);

    await act(async () => {
      deviceBRequest.resolve(
        mockBackupJob({
          id: 'job-b',
          device_id: 'dev-b',
          status: 'success',
          files: mockBackupFiles(2, 'job-b'),
        }),
      );
      await deviceBRequest.promise;
    });
    expect(screen.getByText('2 files')).toBeInTheDocument();

    await act(async () => {
      deviceARequest.resolve(
        mockBackupJob({
          id: 'job-a',
          device_id: 'dev-a',
          status: 'success',
          files: mockBackupFiles(1, 'job-a'),
        }),
      );
      await deviceARequest.promise;
    });

    expect(screen.getByText('2 files')).toBeInTheDocument();
    expect(screen.queryByText('1 files')).not.toBeInTheDocument();
  });

  it('ignores a previous device rejection after the current device loads', async () => {
    const deviceARequest = deferred<BackupJob | null>();
    const deviceBRequest = deferred<BackupJob | null>();
    vi.mocked(fetchLatestBackupJob).mockImplementation((deviceId) =>
      deviceId === 'dev-a' ? deviceARequest.promise : deviceBRequest.promise,
    );
    const view = render(<BackupPanel device={mockDevice({ id: 'dev-a' })} />);
    view.rerender(<BackupPanel device={mockDevice({ id: 'dev-b' })} />);

    await act(async () => {
      deviceBRequest.resolve(
        mockBackupJob({
          id: 'job-b',
          device_id: 'dev-b',
          status: 'success',
          files: mockBackupFiles(2, 'job-b'),
        }),
      );
      await deviceBRequest.promise;
    });

    await act(async () => {
      deviceARequest.reject(new Error('device A unavailable'));
      await Promise.resolve();
    });

    expect(screen.getByText('2 files')).toBeInTheDocument();
    expect(screen.queryByText(/device A unavailable/i)).not.toBeInTheDocument();
  });

  it('never renders the previous device latest backup in the next device commit', async () => {
    const deviceBRequest = deferred<BackupJob | null>();
    const frames: RenderFrame[] = [];
    const captureFrame = (frame: RenderFrame) => frames.push(frame);
    vi.mocked(fetchLatestBackupJob)
      .mockResolvedValueOnce(
        mockBackupJob({
          id: 'job-a',
          device_id: 'dev-a',
          status: 'success',
          files: mockBackupFiles(1, 'job-a'),
        }),
      )
      .mockReturnValueOnce(deviceBRequest.promise);
    const view = render(
      <BackupPanelRenderProbe device={mockDevice({ id: 'dev-a' })} captureFrame={captureFrame} />,
    );
    await flushPromises();
    expect(screen.getByText('1 files')).toBeInTheDocument();

    view.rerender(
      <BackupPanelRenderProbe device={mockDevice({ id: 'dev-b' })} captureFrame={captureFrame} />,
    );

    const firstDeviceBFrame = frames.find((frame) => frame.deviceId === 'dev-b');
    expect(firstDeviceBFrame).toBeDefined();
    expect(firstDeviceBFrame?.text).not.toContain('1 files');
  });

  it('never renders the previous device latest backup error in the next device commit', async () => {
    const deviceBRequest = deferred<BackupJob | null>();
    const frames: RenderFrame[] = [];
    const captureFrame = (frame: RenderFrame) => frames.push(frame);
    vi.mocked(fetchLatestBackupJob)
      .mockRejectedValueOnce(new Error('device A unavailable'))
      .mockReturnValueOnce(deviceBRequest.promise);
    const view = render(
      <BackupPanelRenderProbe device={mockDevice({ id: 'dev-a' })} captureFrame={captureFrame} />,
    );
    await flushPromises();
    expect(screen.getByText(/device A unavailable/i)).toBeInTheDocument();

    view.rerender(
      <BackupPanelRenderProbe device={mockDevice({ id: 'dev-b' })} captureFrame={captureFrame} />,
    );

    const firstDeviceBFrame = frames.find((frame) => frame.deviceId === 'dev-b');
    expect(firstDeviceBFrame).toBeDefined();
    expect(firstDeviceBFrame?.text).not.toContain('device A unavailable');
  });

  it('handles a latest backup rejection after unmount', async () => {
    const request = deferred<BackupJob | null>();
    vi.mocked(fetchLatestBackupJob).mockReturnValue(request.promise);
    const view = render(<BackupPanel device={mockDevice()} />);

    view.unmount();
    await act(async () => {
      request.reject(new Error('request completed after unmount'));
      await Promise.resolve();
    });
  });

  it('shows a useful error when the current device latest backup request fails', async () => {
    const request = deferred<BackupJob | null>();
    vi.mocked(fetchLatestBackupJob).mockReturnValue(request.promise);
    render(<BackupPanel device={mockDevice()} />);

    await act(async () => {
      request.reject(new Error('network unavailable'));
      await Promise.resolve();
    });

    expect(screen.getByRole('alert')).toHaveTextContent(
      'Failed to load latest backup: network unavailable',
    );
    expect(screen.queryByText('No backups yet')).not.toBeInTheDocument();
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
