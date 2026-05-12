import { fireEvent, render, screen } from '@testing-library/react';
import { describe, expect, it, vi } from 'vitest';
import type { Area, Device } from '../../types/api';
import type { DeviceMetricsDTO } from '../../types/metrics';
import type { SnapshotPayload } from '../../types/metrics';
import { DeviceRow } from './DeviceRow';
import { buildRuntimeDeviceRows } from './runtimeDeviceRows';
import type { RuntimeDeviceRow } from './runtimeDeviceRows';

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

function makeRow(overrides: Partial<RuntimeDeviceRow> = {}): RuntimeDeviceRow {
  return {
    ...buildRuntimeDeviceRows({ devices: [mockDevice()], snapshot: null })[0],
    ...overrides,
  };
}

function renderRow(
  deviceOverrides: Partial<Device> = {},
  metrics: DeviceMetricsDTO | null = null,
  areaEntries?: [string, Area][],
) {
  const device = mockDevice(deviceOverrides);
  const snapshot: SnapshotPayload | null = metrics
    ? {
        devices: {
          [device.id]: {
            operational_status: device.status,
            reachability: 'up',
            health: 'healthy',
            freshness: 'fresh',
            primary_reason: 'ok',
            metrics_status: 'available',
            metrics_reason: 'ok',
            alert_status: 'normal',
            firing_alert_count: 0,
            last_collected_at: '2024-01-01T00:00:00Z',
            last_polled_at: '2024-01-01T00:00:00Z',
            expected_poll_interval_seconds: 60,
            ...metrics,
          },
        },
        links: {},
      }
    : null;
  const row = buildRuntimeDeviceRows({ devices: [device], snapshot })[0];

  return render(
    <table>
      <tbody>
        <DeviceRow
          row={row}
          areaMap={mockAreaMap(areaEntries)}
          resolvedTheme="dark"
          onSSHCredentials={noop}
          onBackup={noop}
          onBackupHistory={noop}
          onViewConfig={noop}
        />
      </tbody>
    </table>,
  );
}

describe('DeviceRow', () => {
  it('does not keep the Name cell sticky during horizontal table scrolling', () => {
    renderRow();

    const nameCell = screen.getByText('router-01').closest('td');
    expect(nameCell).toBeTruthy();
    expect(nameCell?.className).not.toContain('sticky');
    expect(nameCell?.className).not.toContain('left-0');
  });

  it('renders icon action buttons with correct titles', () => {
    renderRow();

    const buttons = screen.getAllByRole('button');
    const titles = buttons.map((b) => b.getAttribute('title'));
    expect(titles).toContain('SSH Credentials');
    expect(titles).toContain('Backup Now');
    expect(titles).toContain('Backup History');
    expect(titles).toContain('View Config');
    expect(buttons.length).toBe(4);
  });

  it('renders a permanent delete action only when an orphan delete handler is provided', () => {
    const onDeletePermanently = vi.fn();
    const row = makeRow();
    const { rerender } = render(
      <table>
        <tbody>
          <DeviceRow
            row={row}
            areaMap={mockAreaMap()}
            resolvedTheme="dark"
            onSSHCredentials={noop}
            onBackup={noop}
            onBackupHistory={noop}
            onViewConfig={noop}
          />
        </tbody>
      </table>,
    );

    expect(screen.queryByTitle('Delete permanently')).toBeNull();

    rerender(
      <table>
        <tbody>
          <DeviceRow
            row={row}
            areaMap={mockAreaMap()}
            resolvedTheme="dark"
            onSSHCredentials={noop}
            onBackup={noop}
            onBackupHistory={noop}
            onViewConfig={noop}
            onDeletePermanently={onDeletePermanently}
          />
        </tbody>
      </table>,
    );

    fireEvent.click(screen.getByTitle('Delete permanently'));

    expect(onDeletePermanently).toHaveBeenCalledTimes(1);
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
      operational_status: 'up',
      reachability: 'up',
      cpu_percent: 15,
      mem_percent: 40,
      temp_celsius: 45,
      uptime_secs: 259200, // 3 days
      health: 'healthy',
      freshness: 'fresh',
      primary_reason: 'ok',
      metrics_status: 'available',
      metrics_reason: 'ok',
      alert_status: 'normal',
      firing_alert_count: 0,
      last_collected_at: '2024-01-01T00:00:00Z',
      last_polled_at: '2024-01-01T00:00:00Z',
      expected_poll_interval_seconds: 60,
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

  it('renders presentation fields from the row model instead of raw device values', () => {
    const row = makeRow({
      displayName: 'Display From Row',
      sysName: 'System From Row',
      ip: '198.51.100.10',
      vendor: 'row-vendor',
      modelLabel: 'Row Model Label',
      osVersion: 'row-os',
      uptimeLabel: '9d',
    });

    row.device = {
      ...row.device,
      hostname: 'device-hostname',
      sys_name: 'Device System',
      ip: '203.0.113.5',
      vendor: 'device-vendor',
      hardware_model: 'Device Model',
      sys_descr: 'Device Description',
    };

    render(
      <table>
        <tbody>
          <DeviceRow
            row={row}
            areaMap={mockAreaMap()}
            resolvedTheme="dark"
            onSSHCredentials={noop}
            onBackup={noop}
            onBackupHistory={noop}
            onViewConfig={noop}
          />
        </tbody>
      </table>,
    );

    expect(screen.getByText('Display From Row')).toBeInTheDocument();
    expect(screen.getByText('System From Row')).toBeInTheDocument();
    expect(screen.getByText('198.51.100.10')).toBeInTheDocument();
    expect(screen.getByText('row-vendor')).toBeInTheDocument();
    expect(screen.getByText('Row Model Label')).toBeInTheDocument();
    expect(screen.getByText('row-os')).toBeInTheDocument();
    expect(screen.getByText('9d')).toBeInTheDocument();
    expect(screen.queryByText('device-vendor')).not.toBeInTheDocument();
    expect(screen.queryByText('203.0.113.5')).not.toBeInTheDocument();
  });
});
