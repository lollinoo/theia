import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen } from '@testing-library/react';
import { AddDevicePanel } from './AddDevicePanel';

// Mock API calls that fire in useEffect
vi.mock('../api/client', () => ({
  fetchSNMPProfiles: vi.fn().mockResolvedValue([]),
  fetchSSHProfiles: vi.fn().mockResolvedValue([]),
  checkPrometheusHealth: vi.fn().mockResolvedValue({ available: false, url: '' }),
  createDevice: vi.fn().mockResolvedValue({
    id: 'new-dev',
    hostname: 'test',
    ip: '10.0.0.1',
    device_type: 'unknown',
    status: 'probing',
    sys_name: '',
    sys_descr: '',
    hardware_model: '',
    vendor: 'default',
    managed: true,
    interfaces: [],
    backup_supported: false,
    metrics_source: 'snmp',
    prometheus_label_name: 'instance',
    prometheus_label_value: '',
  }),
}));

beforeEach(() => {
  vi.clearAllMocks();
});

describe('AddDevicePanel', () => {
  it('renders form fields', () => {
    render(<AddDevicePanel onDeviceAdded={vi.fn()} />);

    // IP Address field
    expect(screen.getByPlaceholderText('192.168.1.1')).toBeInTheDocument();
    // Submit button
    expect(screen.getByText('Add Device')).toBeInTheDocument();
  });

  it('renders SNMP version selector', () => {
    render(<AddDevicePanel onDeviceAdded={vi.fn()} />);

    // The SNMP version select should have v2c and v3 options
    expect(screen.getByDisplayValue('v2c')).toBeInTheDocument();
  });

  it('renders community field for v2c', () => {
    render(<AddDevicePanel onDeviceAdded={vi.fn()} />);

    // Community input with placeholder
    expect(screen.getByPlaceholderText('public')).toBeInTheDocument();
  });

  it('renders IP Address label', () => {
    render(<AddDevicePanel onDeviceAdded={vi.fn()} />);

    expect(screen.getByText('IP Address')).toBeInTheDocument();
  });

  it('renders metrics source selector', () => {
    render(<AddDevicePanel onDeviceAdded={vi.fn()} />);

    expect(screen.getByText('Metrics Source')).toBeInTheDocument();
    expect(screen.getByText('SNMP Direct')).toBeInTheDocument();
  });

  it('renders optional fields', () => {
    render(<AddDevicePanel onDeviceAdded={vi.fn()} />);

    // Custom name field
    expect(screen.getByPlaceholderText('Auto-discovered from SNMP / Prometheus')).toBeInTheDocument();
    // Vendor field
    expect(screen.getByText('Vendor')).toBeInTheDocument();
  });

  it('renders without crashing', () => {
    const { container } = render(<AddDevicePanel onDeviceAdded={vi.fn()} />);
    expect(container.querySelector('form')).toBeInTheDocument();
  });
});
