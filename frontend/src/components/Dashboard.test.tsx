import { describe, it, expect, vi } from 'vitest';
import { render, screen } from '@testing-library/react';
import { Dashboard } from './Dashboard';
import type { Device } from '../types/api';

// Mock sub-components that have their own complex dependencies
vi.mock('./dashboard/DeviceTable', () => ({
  DeviceTable: ({ devices }: { devices: Device[] }) => (
    <table data-testid="device-table">
      <tbody>
        {devices.map((d) => (
          <tr key={d.id}>
            <td>{d.hostname}</td>
            <td>{d.ip}</td>
            <td>{d.status}</td>
          </tr>
        ))}
      </tbody>
    </table>
  ),
}));

vi.mock('./dashboard/SSHCredentialForm', () => ({
  SSHCredentialForm: () => <div data-testid="ssh-form" />,
}));

vi.mock('./dashboard/BackupPanel', () => ({
  BackupPanel: () => <div data-testid="backup-panel" />,
}));

vi.mock('./dashboard/BackupHistoryTable', () => ({
  BackupHistoryTable: () => <div data-testid="backup-history" />,
}));

vi.mock('./dashboard/BulkBackupPanel', () => ({
  BulkBackupPanel: () => <div data-testid="bulk-backup" />,
}));

vi.mock('./dashboard/ConfigViewer', () => ({
  ConfigViewer: () => <div data-testid="config-viewer" />,
}));

vi.mock('./dashboard/VendorSettingsPanel', () => ({
  VendorSettingsPanel: () => <div data-testid="vendor-settings" />,
}));

vi.mock('./SidePanel', () => ({
  SidePanel: ({ children, open }: { children: React.ReactNode; open: boolean }) =>
    open ? <div data-testid="side-panel">{children}</div> : null,
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
    metrics_source: 'prometheus',
    prometheus_label_name: 'instance',
    prometheus_label_value: '10.0.0.1:9100',
    ...overrides,
  };
}

describe('Dashboard', () => {
  it('renders device information when devices provided', () => {
    const devices = [
      mockDevice({ id: 'dev-1', hostname: 'router-01', ip: '10.0.0.1' }),
      mockDevice({ id: 'dev-2', hostname: 'switch-01', ip: '10.0.0.2', device_type: 'switch' }),
    ];

    render(<Dashboard devices={devices} />);

    expect(screen.getByText('router-01')).toBeInTheDocument();
    expect(screen.getByText('10.0.0.1')).toBeInTheDocument();
    expect(screen.getByText('switch-01')).toBeInTheDocument();
    expect(screen.getByText('10.0.0.2')).toBeInTheDocument();
  });

  it('shows loading state when no devices', () => {
    render(<Dashboard devices={[]} />);

    expect(screen.getByText('Loading devices...')).toBeInTheDocument();
  });

  it('renders without crashing with minimal props', () => {
    const { container } = render(<Dashboard devices={[]} />);
    expect(container.firstChild).toBeTruthy();
  });

  it('displays device count', () => {
    const devices = [mockDevice()];
    render(<Dashboard devices={devices} />);

    expect(screen.getByText('1 / 1 devices')).toBeInTheDocument();
  });

  it('renders filter controls', () => {
    render(<Dashboard devices={[mockDevice()]} />);

    expect(screen.getByPlaceholderText('Search devices...')).toBeInTheDocument();
    expect(screen.getByText('Backup All')).toBeInTheDocument();
    expect(screen.getByText('Vendor Settings')).toBeInTheDocument();
  });

  it('renders DeviceTable when devices exist', () => {
    render(<Dashboard devices={[mockDevice()]} />);

    expect(screen.getByTestId('device-table')).toBeInTheDocument();
  });
});
