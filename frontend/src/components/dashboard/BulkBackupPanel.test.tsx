import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { ServerError, ValidationError } from '../../api/errors';
import type { Device } from '../../types/api';
import { BulkBackupPanel } from './BulkBackupPanel';

// Mock API calls — triggerBackup resolves by default; individual tests override as needed.
// fetchDeviceCredentialProfiles returns one profile by default (device is eligible).
vi.mock('../../api/client', () => ({
  triggerBackup: vi.fn().mockResolvedValue({ id: 'job-1', status: 'queued' }),
  triggerBulkBackup: vi
    .fn()
    .mockResolvedValue([
      { device_id: 'dev-1', device_name: 'router-01', status: 'queued', job_id: 'job-1' },
    ]),
  triggerBulkDownload: vi.fn().mockResolvedValue(undefined),
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
});
