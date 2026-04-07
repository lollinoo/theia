import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, fireEvent, waitFor } from '@testing-library/react';
import { DeviceConfigPanel } from './DeviceConfigPanel';
import type { Device } from '../types/api';
import { ValidationError, ServerError } from '../api/errors';

// Mock all API calls used in useEffect
vi.mock('../api/client', () => ({
  fetchSNMPProfiles: vi.fn().mockResolvedValue([]),
  fetchSSHProfiles: vi.fn().mockResolvedValue([]),
  fetchAreas: vi.fn().mockResolvedValue([]),
  fetchSettings: vi.fn().mockResolvedValue({}),
  checkPrometheusHealth: vi.fn().mockResolvedValue({ available: false, url: '' }),
  updateSetting: vi.fn().mockResolvedValue(undefined),
  updateDevice: vi.fn().mockResolvedValue({}),
  deleteDevice: vi.fn().mockResolvedValue(undefined),
  testSNMPConnection: vi.fn().mockResolvedValue({ success: true }),
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

describe('DeviceConfigPanel', () => {
  it('renders device config content', () => {
    render(
      <DeviceConfigPanel
        device={mockDevice()}
        onDeviceUpdated={vi.fn()}
        onDeviceDeleted={vi.fn()}
      />,
    );

    // Should show Polling Override section
    expect(screen.getByText('Polling Override')).toBeInTheDocument();
    // Should show Edit Device section
    expect(screen.getByText('Edit Device')).toBeInTheDocument();
    // Should show Save Changes button
    expect(screen.getByText('Save Changes')).toBeInTheDocument();
  });

  it('renders Grafana dashboard URL field', () => {
    render(
      <DeviceConfigPanel
        device={mockDevice()}
        onDeviceUpdated={vi.fn()}
        onDeviceDeleted={vi.fn()}
      />,
    );

    expect(screen.getByText('Custom Grafana Dashboard URL')).toBeInTheDocument();
    expect(screen.getByPlaceholderText('Leave blank to use default')).toBeInTheDocument();
  });

  it('renders device IP in edit form', () => {
    render(
      <DeviceConfigPanel
        device={mockDevice({ ip: '192.168.1.1' })}
        onDeviceUpdated={vi.fn()}
        onDeviceDeleted={vi.fn()}
      />,
    );

    expect(screen.getByDisplayValue('192.168.1.1')).toBeInTheDocument();
  });

  it('renders SNMP test button', () => {
    render(
      <DeviceConfigPanel
        device={mockDevice()}
        onDeviceUpdated={vi.fn()}
        onDeviceDeleted={vi.fn()}
      />,
    );

    expect(screen.getByText('Test SNMP Connectivity')).toBeInTheDocument();
  });

  it('renders delete device button', () => {
    render(
      <DeviceConfigPanel
        device={mockDevice()}
        onDeviceUpdated={vi.fn()}
        onDeviceDeleted={vi.fn()}
      />,
    );

    expect(screen.getByText('Delete Device')).toBeInTheDocument();
  });

  it('shows auto-discovered hostname when sys_name exists', () => {
    render(
      <DeviceConfigPanel
        device={mockDevice({ sys_name: 'my-router' })}
        onDeviceUpdated={vi.fn()}
        onDeviceDeleted={vi.fn()}
      />,
    );

    expect(screen.getByText('Auto-discovered Hostname')).toBeInTheDocument();
    expect(screen.getByText('my-router')).toBeInTheDocument();
  });

  it('renders without crashing', () => {
    const { container } = render(
      <DeviceConfigPanel
        device={mockDevice()}
        onDeviceUpdated={vi.fn()}
        onDeviceDeleted={vi.fn()}
      />,
    );
    expect(container.firstChild).toBeTruthy();
  });

  it('renders Areas section with select dropdown', async () => {
    // Mock fetchAreas to return at least one area so the dropdown renders
    const { fetchAreas } = await import('../api/client');
    (fetchAreas as ReturnType<typeof vi.fn>).mockResolvedValueOnce([
      { id: 'area-1', name: 'Backbone', description: '', color: '#00E676', device_count: 0 },
    ]);

    render(
      <DeviceConfigPanel
        device={mockDevice()}
        onDeviceUpdated={vi.fn()}
        onDeviceDeleted={vi.fn()}
      />,
    );

    // Areas label should be present
    await waitFor(() => {
      expect(screen.getByText('Areas')).toBeInTheDocument();
    });
    // Unassigned select option should be available when device has no areas
    await waitFor(() => {
      expect(screen.getByText('Unassigned - select area...')).toBeInTheDocument();
    });
  });

  it('renders areas section between IP and Vendor fields', () => {
    render(
      <DeviceConfigPanel
        device={mockDevice()}
        onDeviceUpdated={vi.fn()}
        onDeviceDeleted={vi.fn()}
      />,
    );

    const areaLabel = screen.getByText('Areas');
    expect(areaLabel).toBeInTheDocument();
  });
});

// --- Gap 2: DeviceConfigPanel blur+submit validation ---

describe('DeviceConfigPanel — onBlur IP validation', () => {
  it('shows error text when IP input is blurred with an invalid value', async () => {
    render(
      <DeviceConfigPanel
        device={mockDevice({ ip: '10.0.0.1' })}
        onDeviceUpdated={vi.fn()}
        onDeviceDeleted={vi.fn()}
      />,
    );

    const ipInput = screen.getByDisplayValue('10.0.0.1');
    fireEvent.change(ipInput, { target: { value: 'not valid!!' } });
    fireEvent.blur(ipInput);

    await waitFor(() => {
      expect(screen.getByText('Invalid IP address or hostname')).toBeInTheDocument();
    });
  });

  it('applies border-status-down class to IP field on invalid blur', async () => {
    render(
      <DeviceConfigPanel
        device={mockDevice({ ip: '10.0.0.1' })}
        onDeviceUpdated={vi.fn()}
        onDeviceDeleted={vi.fn()}
      />,
    );

    const ipInput = screen.getByDisplayValue('10.0.0.1');
    fireEvent.change(ipInput, { target: { value: '' } });
    fireEvent.blur(ipInput);

    await waitFor(() => {
      expect(ipInput.className).toContain('border-status-down');
    });
  });

  it('clears IP error when user types valid value', async () => {
    render(
      <DeviceConfigPanel
        device={mockDevice({ ip: '10.0.0.1' })}
        onDeviceUpdated={vi.fn()}
        onDeviceDeleted={vi.fn()}
      />,
    );

    const ipInput = screen.getByDisplayValue('10.0.0.1');
    fireEvent.change(ipInput, { target: { value: '' } });
    fireEvent.blur(ipInput);

    await waitFor(() => {
      expect(screen.getByText('IP address or hostname is required')).toBeInTheDocument();
    });

    fireEvent.change(ipInput, { target: { value: '192.168.1.1' } });

    await waitFor(() => {
      expect(screen.queryByText('IP address or hostname is required')).not.toBeInTheDocument();
    });
  });
});

describe('DeviceConfigPanel — submit validates before API call', () => {
  it('does not call updateDevice when IP is invalid on submit', async () => {
    const { updateDevice } = await import('../api/client');

    render(
      <DeviceConfigPanel
        device={mockDevice({ ip: '10.0.0.1' })}
        onDeviceUpdated={vi.fn()}
        onDeviceDeleted={vi.fn()}
      />,
    );

    const ipInput = screen.getByDisplayValue('10.0.0.1');
    fireEvent.change(ipInput, { target: { value: '' } });

    fireEvent.click(screen.getByText('Save Changes'));

    await waitFor(() => {
      expect(screen.getByText('IP address or hostname is required')).toBeInTheDocument();
    });
    expect(updateDevice).not.toHaveBeenCalled();
  });
});

// --- Gap 6 (continued): DeviceConfigPanel typed error display ---

describe('DeviceConfigPanel — backend typed error display', () => {
  it('shows ServerError ref when updateDevice throws ServerError', async () => {
    const { updateDevice } = await import('../api/client');
    (updateDevice as ReturnType<typeof vi.fn>).mockRejectedValueOnce(
      new ServerError('internal error, ref: dc001', 'dc001'),
    );

    render(
      <DeviceConfigPanel
        device={mockDevice({ ip: '10.0.0.1' })}
        onDeviceUpdated={vi.fn()}
        onDeviceDeleted={vi.fn()}
      />,
    );

    fireEvent.click(screen.getByText('Save Changes'));

    await waitFor(() => {
      expect(screen.getByText('Something went wrong (ref: dc001)')).toBeInTheDocument();
    });
  });

  it('shows ValidationError message when updateDevice throws ValidationError', async () => {
    const { updateDevice } = await import('../api/client');
    (updateDevice as ReturnType<typeof vi.fn>).mockRejectedValueOnce(
      new ValidationError('IP address already in use'),
    );

    render(
      <DeviceConfigPanel
        device={mockDevice({ ip: '10.0.0.1' })}
        onDeviceUpdated={vi.fn()}
        onDeviceDeleted={vi.fn()}
      />,
    );

    fireEvent.click(screen.getByText('Save Changes'));

    await waitFor(() => {
      expect(screen.getByText('IP address already in use')).toBeInTheDocument();
    });
  });

  it('renders Grafana URL input field for physical devices', () => {
    render(
      <DeviceConfigPanel
        device={mockDevice()}
        onDeviceUpdated={vi.fn()}
        onDeviceDeleted={vi.fn()}
      />,
    );

    // The Grafana URL field must be present so blur validation can fire on it
    expect(screen.getByPlaceholderText('Leave blank to use default')).toBeInTheDocument();
    expect(screen.getByText('Custom Grafana Dashboard URL')).toBeInTheDocument();
  });
});
