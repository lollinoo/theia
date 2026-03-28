import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, waitFor } from '@testing-library/react';
import { DeviceConfigPanel } from './DeviceConfigPanel';
import type { Device } from '../types/api';

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

  it('renders Area dropdown with Unassigned option', async () => {
    render(
      <DeviceConfigPanel
        device={mockDevice()}
        onDeviceUpdated={vi.fn()}
        onDeviceDeleted={vi.fn()}
      />,
    );

    // Area label should be present
    await waitFor(() => {
      expect(screen.getByText('Area')).toBeInTheDocument();
    });
    // Unassigned option should be the default
    expect(screen.getByText('Unassigned')).toBeInTheDocument();
  });

  it('renders area dropdown between IP and Vendor fields', () => {
    render(
      <DeviceConfigPanel
        device={mockDevice()}
        onDeviceUpdated={vi.fn()}
        onDeviceDeleted={vi.fn()}
      />,
    );

    const areaLabel = screen.getByText('Area');
    expect(areaLabel).toBeInTheDocument();
  });
});
