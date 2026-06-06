/**
 * Exercises bulk backup panel operations dashboard behavior so refactors preserve the documented contract.
 */
import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { ServerError, ValidationError } from '../../api/errors';
import type { Device } from '../../types/api';
import { BulkBackupPanel, __resetBulkBackupSessionForTests } from './BulkBackupPanel';

// Mock API calls — triggerBackup resolves by default; individual tests override as needed.
// fetchDeviceCredentialProfiles returns one profile by default (device is eligible).
vi.mock('../../api/client', () => ({
  triggerBackup: vi.fn().mockResolvedValue({ id: 'job-1', status: 'queued' }),
  startBulkBackupRun: vi.fn().mockResolvedValue({
    id: 'run-1',
    status: 'running',
    batch_size: 10,
    total_count: 1,
    queued_count: 1,
    success_count: 0,
    failed_count: 0,
    skipped_count: 0,
    cancelled_count: 0,
    error_message: '',
    cancel_requested: false,
    created_by: '',
    created_at: '2026-05-26T10:00:00Z',
    items: [
      {
        id: 'item-1',
        run_id: 'run-1',
        device_id: 'dev-1',
        device_name: 'router-01',
        status: 'queued',
        backup_job_id: 'job-1',
        created_at: '2026-05-26T10:00:00Z',
        updated_at: '2026-05-26T10:00:00Z',
      },
    ],
  }),
  fetchLatestBulkBackupRun: vi.fn().mockResolvedValue(null),
  fetchBulkOperationStatus: vi.fn().mockResolvedValue({
    bulk_backup: {
      max_devices: 100,
      max_queued_jobs: 1000,
      concurrency: {
        max_concurrent: 1,
        configurable: false,
        distributed: true,
        distributed_max_concurrent: 1,
      },
      legacy_endpoint: {
        path: '/api/v1/backups/bulk',
        deprecated: true,
      },
    },
    bulk_backup_run: {
      max_devices: 100,
      max_queued_jobs: 1000,
      batch_size: 10,
      max_active_runs: 1,
      configurable_concurrency: false,
      distributed: true,
      distributed_max_active_runs: 1,
      can_pause: true,
      can_resume: true,
      can_cancel: true,
    },
    bulk_download: {
      max_devices: 100,
      max_files: 500,
      max_bytes: 104857600,
      max_concurrent_per_actor: 1,
      max_concurrent_global: 4,
      distributed: true,
      distributed_max_concurrent_per_actor: 1,
      distributed_max_concurrent_global: 4,
    },
  }),
  fetchBulkBackupRun: vi.fn().mockResolvedValue({
    id: 'run-1',
    status: 'success',
    batch_size: 10,
    total_count: 1,
    queued_count: 0,
    success_count: 1,
    failed_count: 0,
    skipped_count: 0,
    cancelled_count: 0,
    error_message: '',
    cancel_requested: false,
    created_by: '',
    created_at: '2026-05-26T10:00:00Z',
    items: [
      {
        id: 'item-1',
        run_id: 'run-1',
        device_id: 'dev-1',
        device_name: 'router-01',
        status: 'success',
        backup_job_id: 'job-1',
        created_at: '2026-05-26T10:00:00Z',
        updated_at: '2026-05-26T10:00:01Z',
        completed_at: '2026-05-26T10:00:01Z',
      },
    ],
  }),
  cancelBulkBackupRun: vi.fn(),
  pauseBulkBackupRun: vi.fn(),
  resumeBulkBackupRun: vi.fn(),
  triggerBulkBackup: vi.fn().mockResolvedValue([
    {
      device_id: 'dev-1',
      device_name: 'router-01',
      status: 'queued',
      job_id: 'job-1',
    },
  ]),
  triggerBulkDownload: vi.fn().mockResolvedValue('saved'),
  fetchBackupJob: vi.fn().mockResolvedValue({ id: 'job-1', status: 'success', error_message: '' }),
  fetchDeviceCredentialProfiles: vi
    .fn()
    .mockResolvedValue([{ profile_id: 'p1', name: 'Admin', role: 'Admin', is_winbox: false }]),
}));

function mockDevice(overrides: Partial<Device> = {}): Device {
  return {
    id: 'dev-1',
    hostname: 'router-01',
    ip: '10.0.0.1',
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
    prometheus_label_value: '10.0.0.1:9100',
    area_ids: [],
    ...overrides,
  };
}

function mockBulkRun(
  overrides: Record<string, unknown> = {},
  items: Record<string, unknown>[] = [],
) {
  return {
    id: 'run-1',
    status: 'running',
    batch_size: 10,
    total_count: items.length,
    queued_count: items.filter((item) =>
      ['checking', 'queued', 'running'].includes(String(item.status)),
    ).length,
    success_count: items.filter((item) => item.status === 'success').length,
    failed_count: items.filter((item) => item.status === 'failed').length,
    skipped_count: items.filter((item) => item.status === 'skipped').length,
    cancelled_count: items.filter((item) => item.status === 'cancelled').length,
    error_message: '',
    cancel_requested: false,
    created_by: '',
    created_at: '2026-05-26T10:00:00Z',
    items,
    ...overrides,
  };
}

function mockRunItem(overrides: Record<string, unknown>) {
  return {
    id: `item-${overrides.device_id ?? 'dev'}`,
    run_id: 'run-1',
    device_id: 'dev-1',
    device_name: 'router-01',
    status: 'queued',
    created_at: '2026-05-26T10:00:00Z',
    updated_at: '2026-05-26T10:00:00Z',
    ...overrides,
  };
}

function mockBulkOperationStatus(
  overrides: {
    bulkBackupRun?: Record<string, unknown>;
    bulkDownload?: Record<string, unknown>;
  } = {},
) {
  return {
    bulk_backup: {
      max_devices: 100,
      max_queued_jobs: 1000,
      concurrency: {
        max_concurrent: 1,
        configurable: false,
        distributed: true,
        distributed_max_concurrent: 1,
      },
      legacy_endpoint: {
        path: '/api/v1/backups/bulk',
        deprecated: true,
      },
    },
    bulk_backup_run: {
      max_devices: 100,
      max_queued_jobs: 1000,
      batch_size: 10,
      max_active_runs: 1,
      configurable_concurrency: false,
      distributed: true,
      distributed_max_active_runs: 1,
      can_pause: true,
      can_resume: true,
      can_cancel: true,
      ...overrides.bulkBackupRun,
    },
    bulk_download: {
      max_devices: 100,
      max_files: 500,
      max_bytes: 104857600,
      max_concurrent_per_actor: 1,
      max_concurrent_global: 4,
      distributed: true,
      distributed_max_concurrent_per_actor: 1,
      distributed_max_concurrent_global: 4,
      ...overrides.bulkDownload,
    },
  };
}

beforeEach(() => {
  vi.clearAllMocks();
  __resetBulkBackupSessionForTests();
});

describe('BulkBackupPanel — startBulkBackupRun .catch handles ServerError', () => {
  it('shows server error ref when startBulkBackupRun throws ServerError', async () => {
    const { startBulkBackupRun } = await import('../../api/client');
    (startBulkBackupRun as ReturnType<typeof vi.fn>).mockRejectedValueOnce(
      new ServerError('internal error, ref: bk001', 'bk001'),
    );

    const devices = [mockDevice()];
    render(<BulkBackupPanel devices={devices} />);

    fireEvent.click(screen.getByText('Backup All Devices'));

    expect(await screen.findAllByText(/server error \(ref: bk001\)/i)).not.toHaveLength(0);
  });

  it('shows server error without ref when startBulkBackupRun throws ServerError without correlationId', async () => {
    const { startBulkBackupRun } = await import('../../api/client');
    (startBulkBackupRun as ReturnType<typeof vi.fn>).mockRejectedValueOnce(
      new ServerError('internal error', undefined),
    );

    const devices = [mockDevice()];
    render(<BulkBackupPanel devices={devices} />);

    fireEvent.click(screen.getByText('Backup All Devices'));

    expect(await screen.findAllByText('server error')).not.toHaveLength(0);
  });
});

describe('BulkBackupPanel — startBulkBackupRun .catch handles ValidationError', () => {
  it('shows ValidationError message when startBulkBackupRun throws ValidationError', async () => {
    const { startBulkBackupRun } = await import('../../api/client');
    (startBulkBackupRun as ReturnType<typeof vi.fn>).mockRejectedValueOnce(
      new ValidationError('Too many devices selected for bulk backup.'),
    );

    const devices = [mockDevice()];
    render(<BulkBackupPanel devices={devices} />);

    fireEvent.click(screen.getByText('Backup All Devices'));

    expect(
      await screen.findAllByText('Too many devices selected for bulk backup.'),
    ).not.toHaveLength(0);
  });
});

describe('BulkBackupPanel — uses persistent backend bulk runs', () => {
  it('shows the fetched persistent run batch size in the idle summary', async () => {
    const { fetchBulkOperationStatus } = await import('../../api/client');
    (fetchBulkOperationStatus as ReturnType<typeof vi.fn>).mockResolvedValueOnce(
      mockBulkOperationStatus({ bulkBackupRun: { batch_size: 7 } }),
    );

    render(<BulkBackupPanel devices={[mockDevice()]} />);

    expect(await screen.findByText(/groups of 7/)).toBeInTheDocument();
  });

  it('blocks persistent runs that exceed the fetched device limit', async () => {
    const { fetchBulkOperationStatus, startBulkBackupRun } = await import('../../api/client');
    (fetchBulkOperationStatus as ReturnType<typeof vi.fn>).mockResolvedValueOnce(
      mockBulkOperationStatus({ bulkBackupRun: { batch_size: 7, max_devices: 2 } }),
    );
    const devices = Array.from({ length: 3 }, (_, index) =>
      mockDevice({
        id: `dev-${index + 1}`,
        sys_name: `router-${index + 1}`,
      }),
    );

    render(<BulkBackupPanel devices={devices} />);

    await screen.findByText(/groups of 7/);
    fireEvent.click(screen.getByText('Backup All Devices'));

    expect(startBulkBackupRun).not.toHaveBeenCalled();
    expect(
      await screen.findByText('Too many devices selected for bulk backup. Maximum 2, selected 3.'),
    ).toBeInTheDocument();
  });

  it('starts one persistent run and maps queued/skipped items', async () => {
    const { startBulkBackupRun, triggerBackup, triggerBulkBackup } = await import(
      '../../api/client'
    );
    (startBulkBackupRun as ReturnType<typeof vi.fn>).mockResolvedValueOnce(
      mockBulkRun({}, [
        mockRunItem({
          device_id: 'dev-1',
          device_name: 'router-01',
          status: 'queued',
          backup_job_id: 'job-1',
        }),
        mockRunItem({
          device_id: 'dev-2',
          device_name: 'router-02',
          status: 'skipped',
          reason: 'device unreachable',
        }),
      ]),
    );

    render(
      <BulkBackupPanel
        devices={[
          mockDevice({ id: 'dev-1', sys_name: 'router-01' }),
          mockDevice({ id: 'dev-2', sys_name: 'router-02' }),
        ]}
      />,
    );

    fireEvent.click(screen.getByText('Backup All Devices'));

    await waitFor(() => {
      expect(startBulkBackupRun).toHaveBeenCalledWith(['dev-1', 'dev-2']);
    });
    expect(triggerBackup).not.toHaveBeenCalled();
    expect(triggerBulkBackup).not.toHaveBeenCalled();
    expect(await screen.findByText('device unreachable')).toBeInTheDocument();
  });

  it('shows aggregate progress counts and the current running device', async () => {
    const { startBulkBackupRun } = await import('../../api/client');
    (startBulkBackupRun as ReturnType<typeof vi.fn>).mockResolvedValueOnce(
      mockBulkRun({}, [
        mockRunItem({
          device_id: 'dev-1',
          device_name: 'queued-router',
          status: 'queued',
        }),
        mockRunItem({
          device_id: 'dev-2',
          device_name: 'running-router',
          status: 'running',
          backup_job_id: 'job-2',
        }),
        mockRunItem({
          device_id: 'dev-3',
          device_name: 'ok-router',
          status: 'success',
        }),
        mockRunItem({
          device_id: 'dev-4',
          device_name: 'bad-router',
          status: 'failed',
        }),
        mockRunItem({
          device_id: 'dev-5',
          device_name: 'skipped-router',
          status: 'skipped',
        }),
      ]),
    );

    render(
      <BulkBackupPanel
        devices={[
          mockDevice({ id: 'dev-1', sys_name: 'queued-router' }),
          mockDevice({ id: 'dev-2', sys_name: 'running-router' }),
          mockDevice({ id: 'dev-3', sys_name: 'ok-router' }),
          mockDevice({ id: 'dev-4', sys_name: 'bad-router' }),
          mockDevice({ id: 'dev-5', sys_name: 'skipped-router' }),
        ]}
      />,
    );

    fireEvent.click(screen.getByText('Backup All Devices'));

    expect(
      await screen.findByText('Queued 1 · Running 1 · Completed 3 · Failed 1 · Skipped 1'),
    ).toBeInTheDocument();
    expect(screen.getByText('Current running-router · job job-2')).toBeInTheDocument();
  });

  it('shows persistent run file and byte totals', async () => {
    const { startBulkBackupRun } = await import('../../api/client');
    (startBulkBackupRun as ReturnType<typeof vi.fn>).mockResolvedValueOnce(
      mockBulkRun({ file_count: 3, byte_count: 1536 }, [
        mockRunItem({
          device_id: 'dev-1',
          device_name: 'router-01',
          status: 'success',
          backup_job_id: 'job-1',
          file_count: 2,
          byte_count: 1024,
        }),
        mockRunItem({
          device_id: 'dev-2',
          device_name: 'router-02',
          status: 'success',
          backup_job_id: 'job-2',
          file_count: 1,
          byte_count: 512,
        }),
      ]),
    );

    render(
      <BulkBackupPanel
        devices={[
          mockDevice({ id: 'dev-1', sys_name: 'router-01' }),
          mockDevice({ id: 'dev-2', sys_name: 'router-02' }),
        ]}
      />,
    );

    fireEvent.click(screen.getByText('Backup All Devices'));

    expect(await screen.findByText('Files 3 · 1.5 KB')).toBeInTheDocument();
    expect(screen.getByText('2 files · 1.0 KB')).toBeInTheDocument();
    expect(screen.getByText('1 file · 512 B')).toBeInTheDocument();
  });

  it('backs up only selected devices', async () => {
    const { startBulkBackupRun } = await import('../../api/client');

    render(
      <BulkBackupPanel
        devices={[
          mockDevice({ id: 'dev-1', sys_name: 'router-01' }),
          mockDevice({ id: 'dev-2', sys_name: 'router-02' }),
        ]}
      />,
    );

    fireEvent.click(screen.getByLabelText('Select router-02'));
    fireEvent.click(screen.getByText('Backup Selected Devices'));

    await waitFor(() => {
      expect(startBulkBackupRun).toHaveBeenCalledWith(['dev-1']);
    });
  });

  it('hydrates a running bulk backup after a page refresh', async () => {
    const { fetchLatestBulkBackupRun } = await import('../../api/client');
    (fetchLatestBulkBackupRun as ReturnType<typeof vi.fn>).mockResolvedValueOnce(
      mockBulkRun({ created_by: 'test-operator' }, [
        mockRunItem({
          device_id: 'dev-1',
          device_name: 'router-01',
          status: 'queued',
        }),
        mockRunItem({
          device_id: 'dev-2',
          device_name: 'router-02',
          status: 'checking',
        }),
      ]),
    );

    render(
      <BulkBackupPanel
        devices={[
          mockDevice({ id: 'dev-1', sys_name: 'router-01' }),
          mockDevice({ id: 'dev-2', sys_name: 'router-02' }),
        ]}
      />,
    );

    expect(await screen.findByText('Processing... 0/2')).toBeInTheDocument();
    expect(screen.getByText('Started by test-operator')).toBeInTheDocument();
    expect(screen.queryByText('Backup All Devices')).not.toBeInTheDocument();
    expect(screen.getByText('checking...')).toBeInTheDocument();
  });

  it('hydrates a paused bulk backup and exposes resume and stop controls', async () => {
    const { fetchLatestBulkBackupRun, resumeBulkBackupRun, cancelBulkBackupRun } = await import(
      '../../api/client'
    );
    (fetchLatestBulkBackupRun as ReturnType<typeof vi.fn>).mockResolvedValueOnce(
      mockBulkRun({ status: 'paused' }, [
        mockRunItem({
          device_id: 'dev-1',
          device_name: 'router-01',
          status: 'queued',
        }),
      ]),
    );
    (resumeBulkBackupRun as ReturnType<typeof vi.fn>).mockResolvedValueOnce(
      mockBulkRun({ status: 'running' }, [
        mockRunItem({
          device_id: 'dev-1',
          device_name: 'router-01',
          status: 'queued',
        }),
      ]),
    );
    (cancelBulkBackupRun as ReturnType<typeof vi.fn>).mockResolvedValueOnce(
      mockBulkRun({ status: 'cancelling', cancel_requested: true }, [
        mockRunItem({
          device_id: 'dev-1',
          device_name: 'router-01',
          status: 'queued',
        }),
      ]),
    );

    render(<BulkBackupPanel devices={[mockDevice({ id: 'dev-1', sys_name: 'router-01' })]} />);

    expect(await screen.findAllByText('paused')).toHaveLength(2);
    expect(screen.queryByText('checking...')).not.toBeInTheDocument();
    fireEvent.click(screen.getByText('Resume'));
    await waitFor(() => {
      expect(resumeBulkBackupRun).toHaveBeenCalledWith('run-1');
    });
    fireEvent.click(screen.getByText('Stop'));
    await waitFor(() => {
      expect(cancelBulkBackupRun).toHaveBeenCalledWith('run-1');
    });
  });

  it('pauses a running bulk backup from the active session', async () => {
    const { startBulkBackupRun, pauseBulkBackupRun } = await import('../../api/client');
    (startBulkBackupRun as ReturnType<typeof vi.fn>).mockResolvedValueOnce(
      mockBulkRun({ status: 'running' }, [
        mockRunItem({
          device_id: 'dev-1',
          device_name: 'router-01',
          status: 'queued',
        }),
      ]),
    );
    (pauseBulkBackupRun as ReturnType<typeof vi.fn>).mockResolvedValueOnce(
      mockBulkRun({ status: 'pausing' }, [
        mockRunItem({
          device_id: 'dev-1',
          device_name: 'router-01',
          status: 'active',
        }),
        mockRunItem({
          device_id: 'dev-2',
          device_name: 'router-02',
          status: 'queued',
        }),
      ]),
    );

    render(
      <BulkBackupPanel
        devices={[
          mockDevice({ id: 'dev-1', sys_name: 'router-01' }),
          mockDevice({ id: 'dev-2', sys_name: 'router-02' }),
        ]}
      />,
    );

    fireEvent.click(screen.getByText('Backup All Devices'));
    fireEvent.click(await screen.findByText('Pause'));

    await waitFor(() => {
      expect(pauseBulkBackupRun).toHaveBeenCalledWith('run-1');
    });
    expect(await screen.findByText('pausing')).toBeInTheDocument();
    expect(screen.getByText('1 completing; 1 will pause')).toBeInTheDocument();
    expect(screen.getByText('completing')).toBeInTheDocument();
    expect(screen.getByText('will pause')).toBeInTheDocument();
  });

  it('hides pause control when bulk run status reports pause disabled', async () => {
    const { fetchBulkOperationStatus, startBulkBackupRun, pauseBulkBackupRun } = await import(
      '../../api/client'
    );
    (fetchBulkOperationStatus as ReturnType<typeof vi.fn>).mockResolvedValueOnce(
      mockBulkOperationStatus({ bulkBackupRun: { can_pause: false } }),
    );
    (startBulkBackupRun as ReturnType<typeof vi.fn>).mockResolvedValueOnce(
      mockBulkRun({ status: 'running' }, [
        mockRunItem({
          device_id: 'dev-1',
          device_name: 'router-01',
          status: 'queued',
        }),
      ]),
    );

    render(<BulkBackupPanel devices={[mockDevice({ id: 'dev-1', sys_name: 'router-01' })]} />);

    fireEvent.click(screen.getByText('Backup All Devices'));

    await screen.findByText('Processing... 0/1');
    expect(screen.queryByText('Pause')).not.toBeInTheDocument();
    expect(pauseBulkBackupRun).not.toHaveBeenCalled();
  });

  it('distinguishes completing and will stop devices while stopping', async () => {
    const { fetchLatestBulkBackupRun } = await import('../../api/client');
    (fetchLatestBulkBackupRun as ReturnType<typeof vi.fn>).mockResolvedValueOnce(
      mockBulkRun({ status: 'cancelling', cancel_requested: true }, [
        mockRunItem({
          device_id: 'dev-1',
          device_name: 'router-01',
          status: 'active',
        }),
        mockRunItem({
          device_id: 'dev-2',
          device_name: 'router-02',
          status: 'checking',
        }),
        mockRunItem({
          device_id: 'dev-3',
          device_name: 'router-03',
          status: 'success',
        }),
      ]),
    );

    render(
      <BulkBackupPanel
        devices={[
          mockDevice({ id: 'dev-1', sys_name: 'router-01' }),
          mockDevice({ id: 'dev-2', sys_name: 'router-02' }),
          mockDevice({ id: 'dev-3', sys_name: 'router-03' }),
        ]}
      />,
    );

    expect(await screen.findByText('stopping')).toBeInTheDocument();
    expect(screen.getByText('1 completing; 1 will stop')).toBeInTheDocument();
    expect(screen.getByText('completing')).toBeInTheDocument();
    expect(screen.getByText('will stop')).toBeInTheDocument();
    expect(screen.queryByText('checking...')).not.toBeInTheDocument();
  });

  it('renders cancelled devices as stopped', async () => {
    const { startBulkBackupRun } = await import('../../api/client');
    (startBulkBackupRun as ReturnType<typeof vi.fn>).mockResolvedValueOnce(
      mockBulkRun({ status: 'cancelled' }, [
        mockRunItem({
          device_id: 'dev-1',
          device_name: 'router-01',
          status: 'cancelled',
        }),
      ]),
    );

    render(<BulkBackupPanel devices={[mockDevice({ id: 'dev-1', sys_name: 'router-01' })]} />);

    fireEvent.click(screen.getByText('Backup All Devices'));

    expect(await screen.findByText('stopped')).toBeInTheDocument();
    expect(
      screen.getByText('Complete — 0 succeeded, 0 failed, 0 skipped, 1 stopped'),
    ).toBeInTheDocument();
    expect(screen.queryByText('cancelled')).not.toBeInTheDocument();
    expect(screen.queryByText(/No devices were eligible/)).not.toBeInTheDocument();
  });

  it('does not hydrate a completed historical bulk backup after a page refresh', async () => {
    const { fetchLatestBulkBackupRun } = await import('../../api/client');
    (fetchLatestBulkBackupRun as ReturnType<typeof vi.fn>).mockResolvedValueOnce(
      mockBulkRun({ status: 'success' }, [
        mockRunItem({ device_id: 'dev-1', device_name: 'router-01', status: 'success' }),
      ]),
    );

    render(<BulkBackupPanel devices={[mockDevice({ id: 'dev-1', sys_name: 'router-01' })]} />);

    await waitFor(() => {
      expect(fetchLatestBulkBackupRun).toHaveBeenCalled();
    });
    expect(screen.getByText('Backup All Devices')).toBeInTheDocument();
    expect(screen.queryByText(/Complete/)).not.toBeInTheDocument();
  });

  it('shows offline feedback returned by the persistent run', async () => {
    const { startBulkBackupRun } = await import('../../api/client');
    (startBulkBackupRun as ReturnType<typeof vi.fn>).mockResolvedValueOnce(
      mockBulkRun({ status: 'partial' }, [
        mockRunItem({
          device_id: 'dev-1',
          device_name: 'router-01',
          status: 'skipped',
          reason: 'device offline',
        }),
        mockRunItem({
          device_id: 'dev-2',
          device_name: 'router-02',
          status: 'skipped',
          reason: 'device offline',
        }),
      ]),
    );

    render(
      <BulkBackupPanel
        devices={[
          mockDevice({ id: 'dev-1', sys_name: 'router-01', status: 'down' }),
          mockDevice({ id: 'dev-2', sys_name: 'router-02', status: 'down' }),
        ]}
      />,
    );

    fireEvent.click(screen.getByText('Backup All Devices'));

    expect(await screen.findAllByText('device offline')).toHaveLength(2);
    expect(screen.getByText('Complete — 0 succeeded, 0 failed, 2 skipped')).toBeInTheDocument();
  });

  it('submits large selections as one persistent run while backend performs batching', async () => {
    const { fetchBulkOperationStatus, startBulkBackupRun } = await import('../../api/client');
    (fetchBulkOperationStatus as ReturnType<typeof vi.fn>).mockResolvedValueOnce(
      mockBulkOperationStatus({ bulkBackupRun: { batch_size: 11, max_devices: 200 } }),
    );
    const devices = Array.from({ length: 105 }, (_, index) =>
      mockDevice({
        id: `dev-${index + 1}`,
        sys_name: `router-${index + 1}`,
      }),
    );

    render(<BulkBackupPanel devices={devices} />);

    await screen.findByText(/groups of 11/);
    fireEvent.click(screen.getByText('Backup All Devices'));

    await waitFor(() => {
      expect(startBulkBackupRun).toHaveBeenCalledTimes(1);
    });
    expect(startBulkBackupRun).toHaveBeenCalledWith(
      Array.from({ length: 105 }, (_, index) => `dev-${index + 1}`),
    );
  });

  it('splits successful devices into bulk download batches of 100', async () => {
    const { fetchBulkBackupRun, startBulkBackupRun, triggerBulkDownload } = await import(
      '../../api/client'
    );
    const successItems = Array.from({ length: 101 }, (_, index) =>
      mockRunItem({
        device_id: `dev-${index + 1}`,
        device_name: `router-${index + 1}`,
        status: 'success',
      }),
    );
    (startBulkBackupRun as ReturnType<typeof vi.fn>).mockResolvedValueOnce(
      mockBulkRun(
        { status: 'running' },
        successItems.map((item) => ({ ...item, status: 'queued' })),
      ),
    );
    (fetchBulkBackupRun as ReturnType<typeof vi.fn>).mockResolvedValueOnce(
      mockBulkRun({ status: 'success' }, successItems),
    );
    const devices = Array.from({ length: 101 }, (_, index) =>
      mockDevice({
        id: `dev-${index + 1}`,
        sys_name: `router-${index + 1}`,
      }),
    );

    render(<BulkBackupPanel devices={devices} />);

    fireEvent.click(screen.getByText('Backup All Devices'));

    await screen.findByText('Download 2 ZIP files', {}, { timeout: 4000 });
    expect(
      screen.getByText('Downloads will be split into 2 ZIP files of up to 100 devices each.'),
    ).toBeInTheDocument();
    fireEvent.click(screen.getByText('Download 2 ZIP files'));

    await waitFor(() => {
      expect(triggerBulkDownload).toHaveBeenCalledTimes(2);
    });
    expect(triggerBulkDownload).toHaveBeenNthCalledWith(
      1,
      Array.from({ length: 100 }, (_, index) => `dev-${index + 1}`),
      {
        filename: expect.stringMatching(/^THEIA_BACKUPS_batch-1-of-2_.*\.zip$/),
      },
    );
    expect(triggerBulkDownload).toHaveBeenNthCalledWith(2, ['dev-101'], {
      filename: expect.stringMatching(/^THEIA_BACKUPS_batch-2-of-2_.*\.zip$/),
    });
  }, 10000);

  it('uses the fetched bulk download device limit for ZIP batches', async () => {
    const {
      fetchBulkOperationStatus,
      fetchBulkBackupRun,
      startBulkBackupRun,
      triggerBulkDownload,
    } = await import('../../api/client');
    const successItems = Array.from({ length: 81 }, (_, index) =>
      mockRunItem({
        device_id: `dev-${index + 1}`,
        device_name: `router-${index + 1}`,
        status: 'success',
      }),
    );
    (fetchBulkOperationStatus as ReturnType<typeof vi.fn>).mockResolvedValueOnce(
      mockBulkOperationStatus({
        bulkBackupRun: { batch_size: 7 },
        bulkDownload: { max_devices: 40 },
      }),
    );
    (startBulkBackupRun as ReturnType<typeof vi.fn>).mockResolvedValueOnce(
      mockBulkRun(
        { status: 'running' },
        successItems.map((item) => ({ ...item, status: 'queued' })),
      ),
    );
    (fetchBulkBackupRun as ReturnType<typeof vi.fn>).mockResolvedValueOnce(
      mockBulkRun({ status: 'success' }, successItems),
    );
    const devices = Array.from({ length: 81 }, (_, index) =>
      mockDevice({
        id: `dev-${index + 1}`,
        sys_name: `router-${index + 1}`,
      }),
    );

    render(<BulkBackupPanel devices={devices} />);

    await screen.findByText(/groups of 7/);
    fireEvent.click(screen.getByText('Backup All Devices'));

    await screen.findByText('Download 3 ZIP files', {}, { timeout: 4000 });
    expect(
      screen.getByText('Downloads will be split into 3 ZIP files of up to 40 devices each.'),
    ).toBeInTheDocument();
    fireEvent.click(screen.getByText('Download 3 ZIP files'));

    await waitFor(() => {
      expect(triggerBulkDownload).toHaveBeenCalledTimes(3);
    });
    expect(triggerBulkDownload).toHaveBeenNthCalledWith(
      1,
      Array.from({ length: 40 }, (_, index) => `dev-${index + 1}`),
      {
        filename: expect.stringMatching(/^THEIA_BACKUPS_batch-1-of-3_.*\.zip$/),
      },
    );
    expect(triggerBulkDownload).toHaveBeenNthCalledWith(
      2,
      Array.from({ length: 40 }, (_, index) => `dev-${index + 41}`),
      {
        filename: expect.stringMatching(/^THEIA_BACKUPS_batch-2-of-3_.*\.zip$/),
      },
    );
    expect(triggerBulkDownload).toHaveBeenNthCalledWith(3, ['dev-81'], {
      filename: expect.stringMatching(/^THEIA_BACKUPS_batch-3-of-3_.*\.zip$/),
    });
  }, 10000);

  it('uses the fetched bulk download file limit for ZIP batches', async () => {
    const {
      fetchBackupJob,
      fetchBulkOperationStatus,
      fetchBulkBackupRun,
      startBulkBackupRun,
      triggerBulkDownload,
    } = await import('../../api/client');
    const successItems = [
      mockRunItem({
        device_id: 'dev-1',
        device_name: 'router-1',
        status: 'success',
        backup_job_id: 'job-1',
      }),
      mockRunItem({
        device_id: 'dev-2',
        device_name: 'router-2',
        status: 'success',
        backup_job_id: 'job-2',
      }),
    ];
    (fetchBulkOperationStatus as ReturnType<typeof vi.fn>).mockResolvedValueOnce(
      mockBulkOperationStatus({
        bulkDownload: { max_devices: 10, max_files: 3, max_bytes: 1024 },
      }),
    );
    (startBulkBackupRun as ReturnType<typeof vi.fn>).mockResolvedValueOnce(
      mockBulkRun(
        { status: 'running' },
        successItems.map((item) => ({ ...item, status: 'queued' })),
      ),
    );
    (fetchBulkBackupRun as ReturnType<typeof vi.fn>).mockResolvedValueOnce(
      mockBulkRun({ status: 'success' }, successItems),
    );
    (fetchBackupJob as ReturnType<typeof vi.fn>)
      .mockResolvedValueOnce({
        id: 'job-1',
        status: 'success',
        error_message: '',
        files: [
          { id: 'file-1', file_name: 'a.rsc', download_url: '', size_bytes: 100 },
          { id: 'file-2', file_name: 'b.rsc', download_url: '', size_bytes: 100 },
        ],
      })
      .mockResolvedValueOnce({
        id: 'job-2',
        status: 'success',
        error_message: '',
        files: [
          { id: 'file-3', file_name: 'c.rsc', download_url: '', size_bytes: 100 },
          { id: 'file-4', file_name: 'd.rsc', download_url: '', size_bytes: 100 },
        ],
      });

    render(
      <BulkBackupPanel
        devices={[
          mockDevice({ id: 'dev-1', sys_name: 'router-1' }),
          mockDevice({ id: 'dev-2', sys_name: 'router-2' }),
        ]}
      />,
    );

    fireEvent.click(screen.getByText('Backup All Devices'));

    await screen.findByText('Download All as ZIP', {}, { timeout: 4000 });
    fireEvent.click(screen.getByText('Download All as ZIP'));

    await waitFor(() => {
      expect(triggerBulkDownload).toHaveBeenCalledTimes(2);
    });
    expect(triggerBulkDownload).toHaveBeenNthCalledWith(1, ['dev-1'], {
      filename: expect.stringMatching(/^THEIA_BACKUPS_batch-1-of-2_.*\.zip$/),
    });
    expect(triggerBulkDownload).toHaveBeenNthCalledWith(2, ['dev-2'], {
      filename: expect.stringMatching(/^THEIA_BACKUPS_batch-2-of-2_.*\.zip$/),
    });
  }, 10000);

  it('blocks a bulk ZIP when one successful device exceeds the fetched byte limit', async () => {
    const {
      fetchBackupJob,
      fetchBulkOperationStatus,
      fetchBulkBackupRun,
      startBulkBackupRun,
      triggerBulkDownload,
    } = await import('../../api/client');
    const successItems = [
      mockRunItem({
        device_id: 'dev-1',
        device_name: 'router-1',
        status: 'success',
        backup_job_id: 'job-1',
      }),
    ];
    (fetchBulkOperationStatus as ReturnType<typeof vi.fn>).mockResolvedValueOnce(
      mockBulkOperationStatus({
        bulkDownload: { max_devices: 10, max_files: 10, max_bytes: 150 },
      }),
    );
    (startBulkBackupRun as ReturnType<typeof vi.fn>).mockResolvedValueOnce(
      mockBulkRun(
        { status: 'running' },
        successItems.map((item) => ({ ...item, status: 'queued' })),
      ),
    );
    (fetchBulkBackupRun as ReturnType<typeof vi.fn>).mockResolvedValueOnce(
      mockBulkRun({ status: 'success' }, successItems),
    );
    (fetchBackupJob as ReturnType<typeof vi.fn>).mockResolvedValueOnce({
      id: 'job-1',
      status: 'success',
      error_message: '',
      files: [{ id: 'file-1', file_name: 'large.rsc', download_url: '', size_bytes: 200 }],
    });

    render(<BulkBackupPanel devices={[mockDevice({ id: 'dev-1', sys_name: 'router-1' })]} />);

    fireEvent.click(screen.getByText('Backup All Devices'));

    await screen.findByText('Download All as ZIP', {}, { timeout: 4000 });
    fireEvent.click(screen.getByText('Download All as ZIP'));

    await screen.findByText('Bulk download is too large. Maximum 150 bytes, selected 200 bytes.');
    expect(triggerBulkDownload).not.toHaveBeenCalled();
  }, 10000);

  it('stops downloading remaining batches when the save dialog is cancelled', async () => {
    const { fetchBulkBackupRun, startBulkBackupRun, triggerBulkDownload } = await import(
      '../../api/client'
    );
    const successItems = Array.from({ length: 101 }, (_, index) =>
      mockRunItem({
        device_id: `dev-${index + 1}`,
        device_name: `router-${index + 1}`,
        status: 'success',
      }),
    );
    (startBulkBackupRun as ReturnType<typeof vi.fn>).mockResolvedValueOnce(
      mockBulkRun(
        { status: 'running' },
        successItems.map((item) => ({ ...item, status: 'queued' })),
      ),
    );
    (fetchBulkBackupRun as ReturnType<typeof vi.fn>).mockResolvedValueOnce(
      mockBulkRun({ status: 'success' }, successItems),
    );
    (triggerBulkDownload as ReturnType<typeof vi.fn>).mockResolvedValueOnce('cancelled');
    const devices = Array.from({ length: 101 }, (_, index) =>
      mockDevice({
        id: `dev-${index + 1}`,
        sys_name: `router-${index + 1}`,
      }),
    );

    render(<BulkBackupPanel devices={devices} />);

    fireEvent.click(screen.getByText('Backup All Devices'));

    await screen.findByText('Download 2 ZIP files', {}, { timeout: 4000 });
    fireEvent.click(screen.getByText('Download 2 ZIP files'));

    await waitFor(() => {
      expect(triggerBulkDownload).toHaveBeenCalledTimes(1);
    });
  }, 10000);
});
