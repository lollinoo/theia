import { describe, it, expect, vi } from 'vitest';
import { fireEvent, render, screen } from '@testing-library/react';
import { DeviceTable } from './DeviceTable';
import type { Device, Area } from '../../types/api';
import { buildRuntimeDeviceRows } from './runtimeDeviceRows';
import type { RuntimeDeviceRow } from './runtimeDeviceRows';

// Mock DeviceRow as a simple <tr> stub
vi.mock('./DeviceRow', () => ({
  DeviceRow: ({ row }: { row: RuntimeDeviceRow }) => (
    <tr data-testid={`device-row-${row.id}`}>
      <td>{row.hostname}</td>
    </tr>
  ),
}));

// Mock ThemeContext
vi.mock('../../contexts/ThemeContext', () => ({
  useTheme: () => ({ resolvedTheme: 'dark' as const }),
  adaptAreaColor: (hex: string) => hex,
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

function mockAreaMap(): Map<string, Area> {
  const map = new Map<string, Area>();
  map.set('area-1', {
    id: 'area-1',
    name: 'Core',
    description: 'Core network',
    color: '#00E676',
    device_count: 5,
    created_at: '2024-01-01T00:00:00Z',
    updated_at: '2024-01-01T00:00:00Z',
  });
  return map;
}

const noop = () => {};

function renderTable(devices = [mockDevice()]) {
  const rows = buildRuntimeDeviceRows({ devices, snapshot: null });
  return render(
    <DeviceTable
      rows={rows}
      areaMap={mockAreaMap()}
      resolvedTheme="dark"
      onSSHCredentials={noop}
      onBackup={noop}
      onBackupHistory={noop}
      onViewConfig={noop}
    />
  );
}

describe('DeviceTable', () => {
  it('renders all 9 column headers', () => {
    renderTable();

    expect(screen.getByText('Name')).toBeInTheDocument();
    expect(screen.getByText('IP Address')).toBeInTheDocument();
    expect(screen.getByText('Status')).toBeInTheDocument();
    expect(screen.getByText('Area')).toBeInTheDocument();
    expect(screen.getByText('Model')).toBeInTheDocument();
    expect(screen.getByText('Vendor')).toBeInTheDocument();
    expect(screen.getByText('Uptime')).toBeInTheDocument();
    expect(screen.getByText('OS Version')).toBeInTheDocument();
    expect(screen.getByText('Actions')).toBeInTheDocument();
  });

  it('thead has sticky class', () => {
    const { container } = renderTable();

    const thead = container.querySelector('thead');
    expect(thead).toBeTruthy();
    expect(thead?.className).toMatch(/sticky/);
    expect(thead?.className).toMatch(/top-0/);
  });

  it('renders correct number of DeviceRow components', () => {
    const devices = [
      mockDevice({ id: 'dev-1', hostname: 'router-01' }),
      mockDevice({ id: 'dev-2', hostname: 'switch-01' }),
      mockDevice({ id: 'dev-3', hostname: 'ap-01' }),
    ];

    renderTable(devices);

    expect(screen.getByTestId('device-row-dev-1')).toBeInTheDocument();
    expect(screen.getByTestId('device-row-dev-2')).toBeInTheDocument();
    expect(screen.getByTestId('device-row-dev-3')).toBeInTheDocument();
  });

  it('area column header is present and clickable for sorting', () => {
    renderTable();

    const areaHeader = screen.getByText('Area');
    expect(areaHeader).toBeInTheDocument();
    expect(areaHeader.closest('th')).toBeTruthy();
    expect(areaHeader.closest('th')?.className).toMatch(/cursor-pointer/);
  });

  it('uptime column header is present', () => {
    renderTable();

    expect(screen.getByText('Uptime')).toBeInTheDocument();
  });

  it('OS Version column header is present', () => {
    renderTable();

    expect(screen.getByText('OS Version')).toBeInTheDocument();
  });

  it('sorts unmonitored no-ip virtual nodes separately from down nodes', () => {
    const devices = [
      mockDevice({ id: 'dev-virtual', hostname: 'virtual-cloud', device_type: 'virtual', ip: '', status: 'down' }),
      mockDevice({ id: 'dev-down', hostname: 'router-down', status: 'down' }),
    ];

    const { container } = renderTable(devices);

    fireEvent.click(screen.getByText('Status'));

    const rows = Array.from(container.querySelectorAll('tbody tr'));
    expect(rows[0]?.textContent).toContain('router-down');
    expect(rows[1]?.textContent).toContain('virtual-cloud');
  });

  it('sorts by row-model hostname instead of raw device hostname', () => {
    const rows: RuntimeDeviceRow[] = [
      {
        ...buildRuntimeDeviceRows({ devices: [mockDevice({ id: 'dev-1', hostname: 'zzz-device' })], snapshot: null })[0],
        hostname: 'bbb-row',
      },
      {
        ...buildRuntimeDeviceRows({ devices: [mockDevice({ id: 'dev-2', hostname: 'aaa-device' })], snapshot: null })[0],
        hostname: 'aaa-row',
      },
    ];

    const { container } = render(
      <DeviceTable
        rows={rows}
        areaMap={mockAreaMap()}
        resolvedTheme="dark"
        onSSHCredentials={noop}
        onBackup={noop}
        onBackupHistory={noop}
        onViewConfig={noop}
      />,
    );

    const renderedRows = Array.from(container.querySelectorAll('tbody tr'));
    expect(renderedRows[0]?.textContent).toContain('aaa-row');
    expect(renderedRows[1]?.textContent).toContain('bbb-row');
  });


});
