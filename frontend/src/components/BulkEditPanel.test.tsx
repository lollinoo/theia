import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, fireEvent, waitFor } from '@testing-library/react';
import { BulkEditPanel } from './BulkEditPanel';
import type { Device } from '../types/api';
import { ValidationError, ServerError } from '../api/errors';

// Mock API calls
vi.mock('../api/client', () => ({
  fetchAreas: vi.fn().mockResolvedValue([]),
  fetchSSHProfiles: vi.fn().mockResolvedValue([]),
  updateDevice: vi.fn().mockResolvedValue({}),
  deleteDevice: vi.fn().mockResolvedValue(undefined),
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

// --- Gap 11: BulkEditPanel typed errors ---

describe('BulkEditPanel — per-device error loop handles ServerError', () => {
  it('shows server error ref when a device update returns ServerError', async () => {
    const { updateDevice } = await import('../api/client');
    (updateDevice as ReturnType<typeof vi.fn>).mockRejectedValueOnce(
      new ServerError('internal error, ref: bulk001', 'bulk001'),
    );

    const devices = [mockDevice()];
    render(
      <BulkEditPanel
        devices={devices}
        onDevicesUpdated={vi.fn()}
        onDevicesDeleted={vi.fn()}
      />,
    );

    // Make a change so the save button is enabled: change the vendor
    const vendorSelect = screen.getByDisplayValue('MikroTik');
    fireEvent.change(vendorSelect, { target: { value: '' } });

    // Button text is "Apply to N Devices"
    fireEvent.click(screen.getByText('Apply to 1 Devices'));

    await waitFor(() => {
      expect(screen.getByText(/server error \(ref: bulk001\)/)).toBeInTheDocument();
    });
  });

  it('shows ValidationError message when a device update returns ValidationError', async () => {
    const { updateDevice } = await import('../api/client');
    (updateDevice as ReturnType<typeof vi.fn>).mockRejectedValueOnce(
      new ValidationError('invalid vendor name'),
    );

    const devices = [mockDevice()];
    render(
      <BulkEditPanel
        devices={devices}
        onDevicesUpdated={vi.fn()}
        onDevicesDeleted={vi.fn()}
      />,
    );

    const vendorSelect = screen.getByDisplayValue('MikroTik');
    fireEvent.change(vendorSelect, { target: { value: '' } });

    fireEvent.click(screen.getByText('Apply to 1 Devices'));

    await waitFor(() => {
      expect(screen.getByText(/invalid vendor name/)).toBeInTheDocument();
    });
  });
});

describe('BulkEditPanel — outer catch handles typed errors for multiple devices', () => {
  it('collects per-device ServerError refs and shows combined error message', async () => {
    const { updateDevice } = await import('../api/client');
    (updateDevice as ReturnType<typeof vi.fn>).mockRejectedValue(
      new ServerError('global server error, ref: g001', 'g001'),
    );

    const devices = [
      mockDevice({ id: 'dev-1' }),
      mockDevice({ id: 'dev-2', hostname: 'router-02', ip: '10.0.0.2' }),
    ];
    render(
      <BulkEditPanel
        devices={devices}
        onDevicesUpdated={vi.fn()}
        onDevicesDeleted={vi.fn()}
      />,
    );

    const vendorSelect = screen.getByDisplayValue('MikroTik');
    fireEvent.change(vendorSelect, { target: { value: '' } });

    fireEvent.click(screen.getByText('Apply to 2 Devices'));

    await waitFor(() => {
      // Errors are collected per-device and shown as combined message
      expect(screen.getByText(/server error \(ref: g001\)/)).toBeInTheDocument();
    });
  });
});

describe('BulkEditPanel — save button is disabled without changes', () => {
  it('save button is disabled when no fields have been changed', () => {
    const devices = [mockDevice()];
    render(
      <BulkEditPanel
        devices={devices}
        onDevicesUpdated={vi.fn()}
        onDevicesDeleted={vi.fn()}
      />,
    );

    // The apply button is disabled when hasChanges is false
    const applyBtn = screen.getByText('Apply to 1 Devices');
    expect(applyBtn).toBeDisabled();
  });
});
