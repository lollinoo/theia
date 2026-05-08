import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { fetchAreas, updateCanvasMapDeviceAreas, updateDevice } from '../api/client';
import { ServerError, ValidationError } from '../api/errors';
import type { Device } from '../types/api';
import { BulkEditPanel } from './BulkEditPanel';

// Mock API calls
vi.mock('../api/client', () => ({
  fetchAreas: vi.fn().mockResolvedValue([]),
  fetchCredentialProfiles: vi.fn().mockResolvedValue([]),
  updateDevice: vi.fn().mockResolvedValue({}),
  updateCanvasMapDeviceAreas: vi.fn().mockResolvedValue({}),
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
      <BulkEditPanel devices={devices} onDevicesUpdated={vi.fn()} onDevicesDeleted={vi.fn()} />,
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
      <BulkEditPanel devices={devices} onDevicesUpdated={vi.fn()} onDevicesDeleted={vi.fn()} />,
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
      <BulkEditPanel devices={devices} onDevicesUpdated={vi.fn()} onDevicesDeleted={vi.fn()} />,
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
      <BulkEditPanel devices={devices} onDevicesUpdated={vi.fn()} onDevicesDeleted={vi.fn()} />,
    );

    // The apply button is disabled when hasChanges is false
    const applyBtn = screen.getByText('Apply to 1 Devices');
    expect(applyBtn).toBeDisabled();
  });
});

describe('BulkEditPanel — bulk save behavior', () => {
  it('preserves the current update payload shape for each selected device', async () => {
    const updateDeviceMock = vi.mocked(updateDevice);
    updateDeviceMock.mockImplementation(async (id, payload) => mockDevice({ id, ...payload }));

    render(
      <BulkEditPanel
        devices={[
          mockDevice({ id: 'dev-1', hostname: 'router-01', ip: '10.0.0.1' }),
          mockDevice({ id: 'dev-2', hostname: 'router-02', ip: '10.0.0.2' }),
        ]}
        onDevicesUpdated={vi.fn()}
        onDevicesDeleted={vi.fn()}
      />,
    );

    const [vendorSelect, metricsSourceSelect] = screen.getAllByRole('combobox');

    fireEvent.change(vendorSelect, { target: { value: '' } });
    fireEvent.change(metricsSourceSelect, { target: { value: 'prometheus' } });
    fireEvent.click(screen.getByText('Apply to 2 Devices'));

    await waitFor(() => {
      expect(updateDeviceMock).toHaveBeenCalledTimes(2);
    });

    expect(updateDeviceMock).toHaveBeenNthCalledWith('1', 'dev-1', {
      hostname: 'router-01',
      vendor: '',
      metrics_source: 'prometheus',
    });
    expect(updateDeviceMock).toHaveBeenNthCalledWith('2', 'dev-2', {
      hostname: 'router-02',
      vendor: '',
      metrics_source: 'prometheus',
    });
  });

  it('restores keep-current metrics source to a no-op state for mixed devices', async () => {
    const updateDeviceMock = vi.mocked(updateDevice);
    updateDeviceMock.mockImplementation(async (id, payload) => mockDevice({ id, ...payload }));

    render(
      <BulkEditPanel
        devices={[
          mockDevice({ id: 'dev-1', hostname: 'router-01', metrics_source: 'snmp' }),
          mockDevice({
            id: 'dev-2',
            hostname: 'router-02',
            ip: '10.0.0.2',
            metrics_source: 'prometheus',
          }),
        ]}
        onDevicesUpdated={vi.fn()}
        onDevicesDeleted={vi.fn()}
      />,
    );

    const [, metricsSourceSelect] = screen.getAllByRole('combobox');
    const applyButton = screen.getByText('Apply to 2 Devices');

    fireEvent.change(metricsSourceSelect, { target: { value: 'snmp' } });
    fireEvent.change(metricsSourceSelect, { target: { value: '' } });

    await waitFor(() => {
      expect(applyButton).toBeDisabled();
    });

    fireEvent.click(applyButton);

    expect(updateDeviceMock).not.toHaveBeenCalled();
  });

  it('shows mixed areas without preselected chips and saves only the user-edited areas', async () => {
    const updateDeviceMock = vi.mocked(updateDevice);
    const fetchAreasMock = vi.mocked(fetchAreas);
    updateDeviceMock.mockImplementation(async (id, payload) => mockDevice({ id, ...payload }));
    fetchAreasMock.mockResolvedValue([
      { id: 'area-1', name: 'Area 1', color: '#111111' },
      { id: 'area-2', name: 'Area 2', color: '#222222' },
      { id: 'area-3', name: 'Area 3', color: '#333333' },
    ]);

    render(
      <BulkEditPanel
        devices={[
          mockDevice({ id: 'dev-1', hostname: 'router-01', area_ids: ['area-1'] }),
          mockDevice({ id: 'dev-2', hostname: 'router-02', ip: '10.0.0.2', area_ids: ['area-2'] }),
        ]}
        onDevicesUpdated={vi.fn()}
        onDevicesDeleted={vi.fn()}
      />,
    );

    await waitFor(() => {
      expect(screen.getByText('Mixed')).toBeInTheDocument();
      expect(screen.queryByText('Area 1', { selector: 'span' })).not.toBeInTheDocument();
      expect(screen.queryByText('Area 2', { selector: 'span' })).not.toBeInTheDocument();
    });

    const [areaSelect] = screen.getAllByRole('combobox');
    fireEvent.change(areaSelect, { target: { value: 'area-3' } });
    fireEvent.click(screen.getByText('Apply to 2 Devices'));

    await waitFor(() => {
      expect(updateDeviceMock).toHaveBeenCalledTimes(2);
    });

    expect(updateDeviceMock).toHaveBeenNthCalledWith('1', 'dev-1', {
      hostname: 'router-01',
      area_ids: ['area-3'],
    });
    expect(updateDeviceMock).toHaveBeenNthCalledWith('2', 'dev-2', {
      hostname: 'router-02',
      area_ids: ['area-3'],
    });
  });

  it('updates area memberships through the map-scoped endpoint when bulk editing a saved map', async () => {
    const updateDeviceMock = vi.mocked(updateDevice);
    const updateCanvasMapDeviceAreasMock = vi.mocked(updateCanvasMapDeviceAreas);
    const onDevicesUpdated = vi.fn();

    render(
      <BulkEditPanel
        devices={[
          mockDevice({ id: 'dev-1', hostname: 'router-01', area_ids: ['area-1'] }),
          mockDevice({ id: 'dev-2', hostname: 'router-02', ip: '10.0.0.2', area_ids: ['area-1'] }),
        ]}
        areas={[
          { id: 'area-1', name: 'Original Area', color: '#111111' },
          { id: 'area-2', name: 'Duplicated Map Area', color: '#222222' },
        ]}
        mapContext={{ mapId: 'map-copy', mapName: 'Copy' }}
        onDevicesUpdated={onDevicesUpdated}
        onDevicesDeleted={vi.fn()}
      />,
    );

    const [areaSelect] = screen.getAllByRole('combobox');
    fireEvent.change(areaSelect, { target: { value: 'area-2' } });
    fireEvent.click(screen.getByText('Apply to 2 Devices'));

    await waitFor(() => {
      expect(updateCanvasMapDeviceAreasMock).toHaveBeenCalledTimes(1);
    });

    expect(updateCanvasMapDeviceAreasMock).toHaveBeenCalledWith('map-copy', {
      device_ids: ['dev-1', 'dev-2'],
      area_ids: ['area-1', 'area-2'],
    });
    expect(updateDeviceMock).not.toHaveBeenCalled();
    expect(onDevicesUpdated).toHaveBeenCalledWith([
      expect.objectContaining({ id: 'dev-1', area_ids: ['area-1', 'area-2'] }),
      expect.objectContaining({ id: 'dev-2', area_ids: ['area-1', 'area-2'] }),
    ]);
  });
});
