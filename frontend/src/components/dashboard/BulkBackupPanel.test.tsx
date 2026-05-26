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
      mockBulkRun({}, [
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
    expect(screen.queryByText('Backup All Devices')).not.toBeInTheDocument();
    expect(screen.getByText('checking...')).toBeInTheDocument();
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
    const { startBulkBackupRun } = await import('../../api/client');
    const devices = Array.from({ length: 105 }, (_, index) =>
      mockDevice({
        id: `dev-${index + 1}`,
        sys_name: `router-${index + 1}`,
      }),
    );

    render(<BulkBackupPanel devices={devices} />);

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
