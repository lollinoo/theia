import { fireEvent, render, screen } from '@testing-library/react';
import { describe, expect, it, vi } from 'vitest';
import type { Device } from '../types/api';
import { Dashboard } from './Dashboard';
import type { RuntimeDeviceRow } from './dashboard/runtimeDeviceRows';

// Mock sub-components that have their own complex dependencies
vi.mock('./dashboard/DeviceTable', () => ({
  DeviceTable: ({ rows }: { rows: RuntimeDeviceRow[] }) => (
    <table data-testid="device-table">
      <tbody>
        {rows.map((row) => (
          <tr key={row.id}>
            <td>{row.hostname}</td>
            <td>{row.ip}</td>
            <td>{row.statusState.label}</td>
          </tr>
        ))}
      </tbody>
    </table>
  ),
}));

vi.mock('./dashboard/FilterSelect', () => ({
  FilterSelect: ({
    label,
    value,
    onChange,
    options,
  }: {
    label: string;
    value: string;
    onChange: (v: string) => void;
    options: Array<{ value: string; label: string }>;
  }) => (
    <div data-testid={`filter-select-${label.toLowerCase()}`} data-value={value}>
      <button type="button">
        {label}: {value}
      </button>
      {options.map((option) => (
        <button
          key={`${label}-${option.value}`}
          type="button"
          onClick={() => onChange(option.value)}
        >
          {label} option {option.label}
        </button>
      ))}
    </div>
  ),
}));

vi.mock('./MaterialIcon', () => ({
  MaterialIcon: ({ name }: { name: string }) => (
    <span data-testid={`material-icon-${name}`}>{name}</span>
  ),
}));

vi.mock('../contexts/ThemeContext', () => ({
  useTheme: () => ({ resolvedTheme: 'dark' as const }),
  adaptAreaColor: (hex: string) => hex,
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
    area_ids: [],
    ...overrides,
  };
}

describe('Dashboard', () => {
  it('renders device information when devices provided', () => {
    const devices = [
      mockDevice({ id: 'dev-1', hostname: 'router-01', ip: '10.0.0.1' }),
      mockDevice({ id: 'dev-2', hostname: 'switch-01', ip: '10.0.0.2', device_type: 'switch' }),
    ];

    render(<Dashboard devices={devices} areas={[]} snapshot={null} />);

    expect(screen.getByText('router-01')).toBeInTheDocument();
    expect(screen.getByText('10.0.0.1')).toBeInTheDocument();
    expect(screen.getByText('switch-01')).toBeInTheDocument();
    expect(screen.getByText('10.0.0.2')).toBeInTheDocument();
  });

  it('preserves incoming runtime-aware device status when snapshot status disagrees', () => {
    const devices = [mockDevice({ status: 'down' })];

    render(
      <Dashboard
        devices={devices}
        areas={[]}
        snapshot={{
          device_metrics: {},
          link_metrics: {},
          device_statuses: { 'dev-1': 'up' },
        }}
      />,
    );

    expect(screen.getByText('Down')).toBeInTheDocument();
  });

  it('shows loading state with skeleton table when no devices', () => {
    const { container } = render(<Dashboard devices={[]} areas={[]} snapshot={null} />);

    // Skeleton table should show animate-pulse elements
    const pulseElements = container.querySelectorAll('.animate-pulse');
    expect(pulseElements.length).toBeGreaterThan(0);
  });

  it('shows an explicit empty-map state after a loaded map has no devices', () => {
    const { container } = render(
      <Dashboard devices={[]} areas={[]} snapshot={null} loading={false} />,
    );

    expect(screen.getByText('No devices in this map')).toBeInTheDocument();
    expect(container.querySelectorAll('.animate-pulse')).toHaveLength(0);
  });

  it('renders without crashing with minimal props', () => {
    const { container } = render(<Dashboard devices={[]} areas={[]} snapshot={null} />);
    expect(container.firstChild).toBeTruthy();
  });

  it('displays device count in font-mono badge', () => {
    const devices = [mockDevice()];
    const { container } = render(<Dashboard devices={devices} areas={[]} snapshot={null} />);

    // Device count badge with font-mono
    const badge = container.querySelector('.font-mono');
    expect(badge).toBeTruthy();
    expect(badge?.textContent).toContain('1 / 1');
  });

  it('renders FilterSelect controls (no native selects)', () => {
    render(<Dashboard devices={[mockDevice()]} areas={[]} snapshot={null} />);

    expect(screen.getByTestId('filter-select-status')).toBeInTheDocument();
    expect(screen.getByTestId('filter-select-type')).toBeInTheDocument();
    expect(screen.getByTestId('filter-select-area')).toBeInTheDocument();
    expect(screen.getByPlaceholderText('Search devices...')).toBeInTheDocument();
    expect(screen.getByText('Backup All')).toBeInTheDocument();
    expect(screen.getByText('Open map')).toBeInTheDocument();
    expect(screen.getByText('Vendor Settings')).toBeInTheDocument();
  });

  it('calls onOpenMap from the Devices return-to-map action', () => {
    const onOpenMap = vi.fn();

    render(<Dashboard devices={[mockDevice()]} areas={[]} snapshot={null} onOpenMap={onOpenMap} />);

    const returnButton = screen.getByText('Open map').closest('button');
    expect(returnButton).not.toBeNull();
    fireEvent.click(returnButton as HTMLButtonElement);

    expect(onOpenMap).toHaveBeenCalledTimes(1);
  });

  it('renders DeviceTable when devices exist', () => {
    render(<Dashboard devices={[mockDevice()]} areas={[]} snapshot={null} />);

    expect(screen.getByTestId('device-table')).toBeInTheDocument();
  });

  it('filter bar does not render with border-b class (no-line rule)', () => {
    const { container } = render(<Dashboard devices={[mockDevice()]} areas={[]} snapshot={null} />);
    const filterBar = container.querySelector('.bg-surface\\/50');
    expect(filterBar).toBeTruthy();
    expect(filterBar?.className).not.toMatch(/border-b/);
  });

  it('gives the Devices search textbox priority space in the filter row', () => {
    const { container } = render(<Dashboard devices={[mockDevice()]} areas={[]} snapshot={null} />);

    const filterBar = container.querySelector('.bg-surface\\/50');
    const searchInput = screen.getByPlaceholderText('Search devices...');
    const searchField = screen.getByTestId('devices-search-field');

    expect(filterBar?.className).toContain('flex-wrap');
    expect(searchField).toContainElement(searchInput);
    expect(searchField.className).toContain('min-w-[min(26rem,100%)]');
    expect(searchField.className).toContain('flex-[2_1_24rem]');
  });

  it('adds enough top padding so the fixed navigation pill does not cover controls', () => {
    const { container } = render(<Dashboard devices={[mockDevice()]} areas={[]} snapshot={null} />);

    expect(container.firstElementChild?.className).toContain('pt-[86px]');
    expect(container.firstElementChild?.className).not.toContain('pt-10');
    expect(container.firstElementChild?.className).not.toContain('pt-24');
  });

  it('shows no-filter-matches message when filters exclude all devices', () => {
    const devices = [mockDevice({ status: 'up' })];

    render(<Dashboard devices={devices} areas={[]} snapshot={null} />);

    // Type in search to filter out all devices
    const searchInput = screen.getByPlaceholderText('Search devices...');
    fireEvent.change(searchInput, { target: { value: 'nonexistent-device-xyz' } });

    expect(screen.getByText('No devices match your filters')).toBeInTheDocument();
    expect(screen.getByText('Clear filters')).toBeInTheDocument();
  });

  it('shows skeleton headers when no devices are available yet', () => {
    const { container } = render(<Dashboard devices={[]} areas={[]} snapshot={null} />);
    expect(container.querySelector('thead')).toBeTruthy();
  });

  it('shows MaterialIcon search in search input area', () => {
    render(<Dashboard devices={[mockDevice()]} areas={[]} snapshot={null} />);

    expect(screen.getByTestId('material-icon-search')).toBeInTheDocument();
  });

  it('renders without errors as a smoke test (THEME-05)', () => {
    const devices = [
      mockDevice({ id: 'dev-1', status: 'up' }),
      mockDevice({ id: 'dev-2', status: 'down' }),
      mockDevice({ id: 'dev-3', status: 'probing' }),
      mockDevice({ id: 'dev-4', status: 'unknown' }),
    ];
    const { container } = render(<Dashboard devices={devices} areas={[]} snapshot={null} />);
    expect(container.firstChild).toBeTruthy();
    expect(container.querySelector('.transition-colors')).toBeTruthy();
  });

  it('does not keep no-ip virtual nodes in the down filter bucket', () => {
    const devices = [
      mockDevice({ id: 'dev-down', hostname: 'router-down', status: 'down' }),
      mockDevice({
        id: 'dev-virtual',
        hostname: 'virtual-cloud',
        device_type: 'virtual',
        ip: '',
        status: 'down',
      }),
    ];

    render(<Dashboard devices={devices} areas={[]} snapshot={null} />);

    fireEvent.click(screen.getByText('Status option Down'));

    expect(screen.getByText('router-down')).toBeInTheDocument();
    expect(screen.queryByText('virtual-cloud')).toBeNull();
  });

  it('searches using row-model fields instead of raw device fields', () => {
    const devices = [mockDevice({ hostname: 'router-01', sys_name: 'router-01' })];

    render(<Dashboard devices={devices} areas={[]} snapshot={null} />);

    fireEvent.change(screen.getByPlaceholderText('Search devices...'), {
      target: { value: '10.0.0.1' },
    });

    expect(screen.getByText('router-01')).toBeInTheDocument();
  });

  it('uses the selected map area from navigation as the active table area filter', () => {
    const devices = [
      mockDevice({ id: 'dev-1', hostname: 'router-01', area_ids: ['map-area-1'] }),
      mockDevice({
        id: 'dev-2',
        hostname: 'switch-01',
        ip: '10.0.0.2',
        area_ids: ['map-area-2'],
      }),
    ];

    render(
      <Dashboard
        devices={devices}
        areas={[
          {
            id: 'map-area-1',
            name: 'Map Local Area',
            description: '',
            color: '#2979FF',
            device_count: 1,
            created_at: '2026-01-01T00:00:00Z',
            updated_at: '2026-01-01T00:00:00Z',
          },
          {
            id: 'map-area-2',
            name: 'Access Ring',
            description: '',
            color: '#00E676',
            device_count: 1,
            created_at: '2026-01-01T00:00:00Z',
            updated_at: '2026-01-01T00:00:00Z',
          },
        ]}
        snapshot={null}
        selectedAreaId="map-area-1"
        onAreaSelect={vi.fn()}
      />,
    );

    expect(screen.getByTestId('filter-select-area')).toHaveAttribute('data-value', 'map-area-1');
    expect(screen.getByText('router-01')).toBeInTheDocument();
    expect(screen.queryByText('switch-01')).toBeNull();
  });
});
