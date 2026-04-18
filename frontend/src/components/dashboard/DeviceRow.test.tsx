import { describe, it, expect, vi } from 'vitest';
import { render, screen } from '@testing-library/react';
import { DeviceRow } from './DeviceRow';
import type { Device, Area } from '../../types/api';
import type { DeviceMetricsDTO } from '../../types/metrics';

// Mock StatusDot as a simple stub
vi.mock('../StatusDot', () => ({
  StatusDot: ({ status }: { status: string }) => (
    <span data-testid="status-dot" data-status={status} />
  ),
}));

// Mock MaterialIcon as a simple stub
vi.mock('../MaterialIcon', () => ({
  MaterialIcon: ({ name }: { name: string }) => (
    <span data-testid={`material-icon-${name}`}>{name}</span>
  ),
}));

// Mock VendorIcon as a simple stub
vi.mock('../icons/VendorIcon', () => ({
  VendorIcon: ({ vendor }: { vendor: string }) => (
    <span data-testid="vendor-icon" data-vendor={vendor} />
  ),
}));

// Mock ThemeContext (adaptAreaColor returning the input color)
vi.mock('../../contexts/ThemeContext', () => ({
  adaptAreaColor: (hex: string) => hex,
  useTheme: () => ({ resolvedTheme: 'dark' as const }),
}));

function mockDevice(overrides: Partial<Device> = {}): Device {
  return {
    id: 'dev-1',
    hostname: 'router-01',
    ip: '10.0.0.1',
    device_type: 'router',
    status: 'up',
    sys_name: 'router-01',
    sys_descr: 'RouterOS 7.14.3',
    hardware_model: 'RB4011',
    vendor: 'mikrotik',
    managed: true,
    interfaces: [],
    backup_supported: true,
    metrics_source: 'prometheus',
    prometheus_label_name: 'instance',
    prometheus_label_value: '10.0.0.1:9100',
    area_ids: [],
    ...overrides,
  };
}

function mockAreaMap(entries?: [string, Area][]): Map<string, Area> {
  const map = new Map<string, Area>();
  if (entries) {
    for (const [k, v] of entries) map.set(k, v);
  }
  return map;
}

const noop = () => {};

function renderRow(
  deviceOverrides: Partial<Device> = {},
  metrics: DeviceMetricsDTO | null = null,
  areaEntries?: [string, Area][]
) {
  return render(
    <table>
      <tbody>
        <DeviceRow
          device={mockDevice(deviceOverrides)}
          areaMap={mockAreaMap(areaEntries)}
          resolvedTheme="dark"
          deviceMetrics={metrics}
          onSSHCredentials={noop}
          onBackup={noop}
          onBackupHistory={noop}
          onViewConfig={noop}
        />
      </tbody>
    </table>
  );
}

describe('DeviceRow', () => {
  it('renders icon action buttons with correct titles', () => {
    renderRow();

    const buttons = screen.getAllByRole('button');
    const titles = buttons.map(b => b.getAttribute('title'));
    expect(titles).toContain('SSH Credentials');
    expect(titles).toContain('Backup Now');
    expect(titles).toContain('Backup History');
    expect(titles).toContain('View Config');
    expect(buttons.length).toBe(4);
  });

  it('renders StatusDot component', () => {
    renderRow({ status: 'up' });

    const dot = screen.getByTestId('status-dot');
    expect(dot).toBeInTheDocument();
    expect(dot.getAttribute('data-status')).toBe('up');
  });

  it('renders no-ip virtual nodes as unmonitored instead of down', () => {
    renderRow({ device_type: 'virtual', ip: '', status: 'down' });

    const dot = screen.getByTestId('status-dot');
    expect(dot.getAttribute('data-status')).toBe('unmonitored');
    expect(screen.getByText('Unmonitored')).toBeInTheDocument();
    expect(screen.queryByText(/^down$/i)).not.toBeInTheDocument();
  });

  it('renders area color dot when device has area_ids and area is in areaMap', () => {
    const area: Area = {
      id: 'area-1',
      name: 'Core',
      description: 'Core network',
      color: '#00E676',
      device_count: 5,
      created_at: '2024-01-01T00:00:00Z',
      updated_at: '2024-01-01T00:00:00Z',
    };

    renderRow({ area_ids: ['area-1'] }, null, [['area-1', area]]);

    expect(screen.getByText('Core')).toBeInTheDocument();
  });

  it('renders em dash when device has no area', () => {
    renderRow({ area_ids: [] });

    // Multiple em dashes may be present (area, uptime, etc.) -- just verify at least one exists
    const emDashes = screen.getAllByText('\u2014');
    expect(emDashes.length).toBeGreaterThan(0);
  });

  it('renders vendor name as text', () => {
    renderRow({ vendor: 'mikrotik' });

    expect(screen.getByText('mikrotik')).toBeInTheDocument();
  });

  it('renders uptime value from deviceMetrics when available', () => {
    const metrics: DeviceMetricsDTO = {
      device_id: 'dev-1',
      cpu_percent: 15,
      mem_percent: 40,
      temp_celsius: 45,
      uptime_secs: 259200, // 3 days
      collected_at: '2024-01-01T00:00:00Z',
    };

    renderRow({}, metrics);

    expect(screen.getByText('3d')).toBeInTheDocument();
  });

  it('renders OS version parsed from sys_descr when available', () => {
    renderRow({ sys_descr: 'RouterOS 7.14.3' });

    expect(screen.getByText('RouterOS 7.14.3')).toBeInTheDocument();
  });

  it('renders RouterOS version when sys_descr includes model text before the version', () => {
    renderRow({ sys_descr: 'RouterOS RB4011iGS+5HacQ2HnD-IN 7.13.5 (stable)' });

    expect(screen.getByText('RouterOS 7.13.5 (stable)')).toBeInTheDocument();
  });

  it('renders trailing firmware version when sys_descr ends with a dotted version', () => {
    renderRow({ sys_descr: 'airMAX Wireless Router CPE LiteBeam 5AC Gen2 8.7.4' });

    expect(screen.getByText('8.7.4')).toBeInTheDocument();
  });

  it('prefers an explicit device os_version over parsing sys_descr', () => {
    renderRow({ sys_descr: 'RouterOS RB1100x4', os_version: '7.22.1' });

    expect(screen.getByText('7.22.1')).toBeInTheDocument();
  });

  it('renders em dash for uptime when no metrics available', () => {
    renderRow({}, null);

    // There should be em dashes for missing data
    const allCells = screen.getAllByText('\u2014');
    expect(allCells.length).toBeGreaterThan(0);
  });
});
