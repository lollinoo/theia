import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { ServerError, ValidationError } from '../../api/errors';
import type { Device } from '../../types/api';
import { BulkBackupPanel, __resetBulkBackupSessionForTests } from './BulkBackupPanel';

// Mock API calls — triggerBackup resolves by default; individual tests override as needed.
// fetchDeviceCredentialProfiles returns one profile by default (device is eligible).
vi.mock('../../api/client', () => ({
  triggerBackup: vi.fn().mockResolvedValue({ id: 'job-1', status: 'queued' }),
  triggerBulkBackup: vi
    .fn()
    .mockResolvedValue([
      { device_id: 'dev-1', device_name: 'router-01', status: 'queued', job_id: 'job-1' },
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

beforeEach(() => {
  vi.clearAllMocks();
  __resetBulkBackupSessionForTests();
});

// --- Gap 12: BulkBackupPanel typed errors ---

describe('BulkBackupPanel — triggerBulkBackup .catch handles ServerError', () => {
  it('shows server error ref when triggerBulkBackup throws ServerError', async () => {
    const { triggerBulkBackup } = await import('../../api/client');
    (triggerBulkBackup as ReturnType<typeof vi.fn>).mockRejectedValueOnce(
      new ServerError('internal error, ref: bk001', 'bk001'),
    );

    const devices = [mockDevice()];
    render(<BulkBackupPanel devices={devices} />);

    fireEvent.click(screen.getByText('Backup All Devices'));

    expect(await screen.findAllByText(/server error \(ref: bk001\)/i)).not.toHaveLength(0);
  });

  it('shows server error without ref when triggerBulkBackup throws ServerError without correlationId', async () => {
    const { triggerBulkBackup } = await import('../../api/client');
    (triggerBulkBackup as ReturnType<typeof vi.fn>).mockRejectedValueOnce(
      new ServerError('internal error', undefined),
    );

    const devices = [mockDevice()];
    render(<BulkBackupPanel devices={devices} />);

    fireEvent.click(screen.getByText('Backup All Devices'));

    expect(await screen.findAllByText('server error')).not.toHaveLength(0);
  });
});

describe('BulkBackupPanel — triggerBulkBackup .catch handles ValidationError', () => {
  it('shows ValidationError message when triggerBulkBackup throws ValidationError', async () => {
    const { triggerBulkBackup } = await import('../../api/client');
    (triggerBulkBackup as ReturnType<typeof vi.fn>).mockRejectedValueOnce(
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

describe('BulkBackupPanel — skips devices without credential profile assigned', () => {
  it('marks device as skipped with "no credential profile assigned" reason', async () => {
    const { fetchDeviceCredentialProfiles } = await import('../../api/client');
    (fetchDeviceCredentialProfiles as ReturnType<typeof vi.fn>).mockResolvedValueOnce([]);

    const device = mockDevice();
    render(<BulkBackupPanel devices={[device]} />);

    fireEvent.click(screen.getByText('Backup All Devices'));

    await waitFor(() => {
      expect(screen.getByText('no credential profile assigned')).toBeInTheDocument();
    });
  });
});

describe('BulkBackupPanel — skips devices where backup_supported is false', () => {
  it('does not call triggerBulkBackup for unsupported devices', async () => {
    const { triggerBulkBackup } = await import('../../api/client');
    const device = mockDevice({ backup_supported: false });
    render(<BulkBackupPanel devices={[device]} />);

    fireEvent.click(screen.getByText('Backup All Devices'));

    // No eligible devices — triggerBulkBackup never called
    await new Promise((r) => setTimeout(r, 20));
    expect(triggerBulkBackup).not.toHaveBeenCalled();
  });
});

describe('BulkBackupPanel — uses backend bulk backup endpoint', () => {
  it('sends all eligible devices in one bulk request and maps queued jobs', async () => {
    const { triggerBackup, triggerBulkBackup } = await import('../../api/client');
    (triggerBulkBackup as ReturnType<typeof vi.fn>).mockResolvedValueOnce([
      { device_id: 'dev-1', device_name: 'router-01', status: 'queued', job_id: 'job-1' },
      {
        device_id: 'dev-2',
        device_name: 'router-02',
        status: 'skipped',
        reason: 'device unreachable',
      },
    ]);

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
      expect(triggerBulkBackup).toHaveBeenCalledWith(['dev-1', 'dev-2']);
    });
    expect(triggerBackup).not.toHaveBeenCalled();
    expect(await screen.findByText('device unreachable')).toBeInTheDocument();
  });

  it('backs up only selected devices', async () => {
    const { triggerBulkBackup } = await import('../../api/client');
    (triggerBulkBackup as ReturnType<typeof vi.fn>).mockResolvedValueOnce([
      { device_id: 'dev-1', device_name: 'router-01', status: 'queued', job_id: 'job-1' },
    ]);

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
      expect(triggerBulkBackup).toHaveBeenCalledWith(['dev-1']);
    });
  });

  it('keeps a running bulk backup visible after the panel is reopened', async () => {
    const { triggerBulkBackup } = await import('../../api/client');
    (triggerBulkBackup as ReturnType<typeof vi.fn>).mockReturnValueOnce(new Promise(() => {}));

    const { unmount } = render(
      <BulkBackupPanel
        devices={[
          mockDevice({ id: 'dev-1', sys_name: 'router-01' }),
          mockDevice({ id: 'dev-2', sys_name: 'router-02' }),
        ]}
      />,
    );

    fireEvent.click(screen.getByText('Backup All Devices'));

    await screen.findAllByText('checking...');
    unmount();

    render(
      <BulkBackupPanel
        devices={[
          mockDevice({ id: 'dev-1', sys_name: 'router-01' }),
          mockDevice({ id: 'dev-2', sys_name: 'router-02' }),
        ]}
      />,
    );

    expect(screen.queryByText('Backup All Devices')).not.toBeInTheDocument();
    expect(screen.getByText('Processing... 0/2')).toBeInTheDocument();
    expect(screen.getAllByText('checking...')).toHaveLength(2);
    expect(triggerBulkBackup).toHaveBeenCalledTimes(1);
  });

  it('skips offline devices immediately without calling the bulk endpoint', async () => {
    const { fetchDeviceCredentialProfiles, triggerBulkBackup } = await import('../../api/client');
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
    expect(fetchDeviceCredentialProfiles).not.toHaveBeenCalled();
    expect(triggerBulkBackup).not.toHaveBeenCalled();
  });

  it('shows offline feedback immediately while online credential checks are still pending', async () => {
    const { fetchDeviceCredentialProfiles, triggerBulkBackup } = await import('../../api/client');
    (fetchDeviceCredentialProfiles as ReturnType<typeof vi.fn>).mockReturnValueOnce(
      new Promise(() => {}),
    );

    render(
      <BulkBackupPanel
        devices={[
          mockDevice({ id: 'dev-1', sys_name: 'router-01', status: 'down' }),
          mockDevice({ id: 'dev-2', sys_name: 'router-02', status: 'up' }),
        ]}
      />,
    );

    fireEvent.click(screen.getByText('Backup All Devices'));

    expect(await screen.findByText('device offline')).toBeInTheDocument();
    expect(screen.getByText('checking...')).toBeInTheDocument();
    expect(triggerBulkBackup).not.toHaveBeenCalled();
  });

  it('splits selected devices into operational bulk backup groups of 10', async () => {
    const { triggerBulkBackup } = await import('../../api/client');
    (triggerBulkBackup as ReturnType<typeof vi.fn>).mockImplementation((ids: string[]) =>
      Promise.resolve(
        ids.map((id) => ({
          device_id: id,
          device_name: id,
          status: 'queued',
          job_id: `job-${id}`,
        })),
      ),
    );
    const devices = Array.from({ length: 105 }, (_, index) =>
      mockDevice({
        id: `dev-${index + 1}`,
        sys_name: `router-${index + 1}`,
      }),
    );

    render(<BulkBackupPanel devices={devices} />);

    fireEvent.click(screen.getByText('Backup All Devices'));

    await waitFor(() => {
      expect(triggerBulkBackup).toHaveBeenCalledTimes(11);
    });
    expect(triggerBulkBackup).toHaveBeenNthCalledWith(
      1,
      Array.from({ length: 10 }, (_, index) => `dev-${index + 1}`),
    );
    expect(triggerBulkBackup).toHaveBeenNthCalledWith(
      11,
      Array.from({ length: 5 }, (_, index) => `dev-${index + 101}`),
    );
  });

  it('splits successful devices into bulk download batches of 100', async () => {
    const { fetchBackupJob, triggerBulkBackup, triggerBulkDownload } = await import(
      '../../api/client'
    );
    (triggerBulkBackup as ReturnType<typeof vi.fn>).mockImplementation((ids: string[]) =>
      Promise.resolve(
        ids.map((id) => ({
          device_id: id,
          device_name: id,
          status: 'queued',
          job_id: `job-${id}`,
        })),
      ),
    );
    (fetchBackupJob as ReturnType<typeof vi.fn>).mockResolvedValue({
      id: 'job',
      status: 'success',
      error_message: '',
    });
    const devices = Array.from({ length: 101 }, (_, index) =>
      mockDevice({
        id: `dev-${index + 1}`,
        sys_name: `router-${index + 1}`,
      }),
    );

    render(<BulkBackupPanel devices={devices} />);

    fireEvent.click(screen.getByText('Backup All Devices'));

    await waitFor(() => {
      expect(triggerBulkBackup).toHaveBeenCalledTimes(11);
    });
    await new Promise((resolve) => setTimeout(resolve, 2100));
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
      { filename: expect.stringMatching(/^THEIA_BACKUPS_batch-1-of-2_.*\.zip$/) },
    );
    expect(triggerBulkDownload).toHaveBeenNthCalledWith(2, ['dev-101'], {
      filename: expect.stringMatching(/^THEIA_BACKUPS_batch-2-of-2_.*\.zip$/),
    });
  }, 10000);

  it('stops downloading remaining batches when the save dialog is cancelled', async () => {
    const { fetchBackupJob, triggerBulkBackup, triggerBulkDownload } = await import(
      '../../api/client'
    );
    (triggerBulkBackup as ReturnType<typeof vi.fn>).mockImplementation((ids: string[]) =>
      Promise.resolve(
        ids.map((id) => ({
          device_id: id,
          device_name: id,
          status: 'queued',
          job_id: `job-${id}`,
        })),
      ),
    );
    (fetchBackupJob as ReturnType<typeof vi.fn>).mockResolvedValue({
      id: 'job',
      status: 'success',
      error_message: '',
    });
    (triggerBulkDownload as ReturnType<typeof vi.fn>).mockResolvedValueOnce('cancelled');
    const devices = Array.from({ length: 101 }, (_, index) =>
      mockDevice({
        id: `dev-${index + 1}`,
        sys_name: `router-${index + 1}`,
      }),
    );

    render(<BulkBackupPanel devices={devices} />);

    fireEvent.click(screen.getByText('Backup All Devices'));

    await waitFor(() => {
      expect(triggerBulkBackup).toHaveBeenCalledTimes(11);
    });
    await new Promise((resolve) => setTimeout(resolve, 2100));
    await screen.findByText('Download 2 ZIP files', {}, { timeout: 4000 });
    fireEvent.click(screen.getByText('Download 2 ZIP files'));

    await waitFor(() => {
      expect(triggerBulkDownload).toHaveBeenCalledTimes(1);
    });
  }, 10000);
});
