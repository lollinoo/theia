import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, fireEvent, waitFor } from '@testing-library/react';
import { AddDevicePanel } from './AddDevicePanel';
import { createDevice } from '../api/client';

// Mock API calls that fire in useEffect
vi.mock('../api/client', () => ({
  fetchSNMPProfiles: vi.fn().mockResolvedValue([]),
  fetchSSHProfiles: vi.fn().mockResolvedValue([]),
  fetchAreas: vi.fn().mockResolvedValue([]),
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

describe('virtual mode', () => {
  it('renders Physical Device and Virtual Node toggle', () => {
    render(<AddDevicePanel onDeviceAdded={vi.fn()} />);
    expect(screen.getByText('Physical Device')).toBeInTheDocument();
    expect(screen.getByText('Virtual Node')).toBeInTheDocument();
  });

  it('shows subtype cards in virtual mode', () => {
    render(<AddDevicePanel onDeviceAdded={vi.fn()} />);
    fireEvent.click(screen.getByText('Virtual Node'));
    expect(screen.getByText('Internet')).toBeInTheDocument();
    expect(screen.getByText('Cloud')).toBeInTheDocument();
    expect(screen.getByText('Server')).toBeInTheDocument();
    expect(screen.getByText('Generic')).toBeInTheDocument();
  });

  it('hides SNMP fields in virtual mode', () => {
    render(<AddDevicePanel onDeviceAdded={vi.fn()} />);
    // Physical mode has SNMP version selector
    expect(screen.getByDisplayValue('v2c')).toBeInTheDocument();
    fireEvent.click(screen.getByText('Virtual Node'));
    expect(screen.queryByDisplayValue('v2c')).not.toBeInTheDocument();
  });

  it('shows Display Name field in virtual mode', () => {
    render(<AddDevicePanel onDeviceAdded={vi.fn()} />);
    fireEvent.click(screen.getByText('Virtual Node'));
    expect(screen.getByPlaceholderText('e.g. ISP Gateway')).toBeInTheDocument();
  });

  it('resets fields when switching modes', () => {
    render(<AddDevicePanel onDeviceAdded={vi.fn()} />);
    const ipInput = screen.getByPlaceholderText('192.168.1.1');
    fireEvent.change(ipInput, { target: { value: '10.0.0.1' } });
    expect(ipInput).toHaveValue('10.0.0.1');
    // Switch to virtual and back
    fireEvent.click(screen.getByText('Virtual Node'));
    fireEvent.click(screen.getByText('Physical Device'));
    expect(screen.getByPlaceholderText('192.168.1.1')).toHaveValue('');
  });

  it('submits virtual device with correct payload', async () => {
    const onDeviceAdded = vi.fn();
    render(<AddDevicePanel onDeviceAdded={onDeviceAdded} />);
    fireEvent.click(screen.getByText('Virtual Node'));
    // Fill display name
    fireEvent.change(screen.getByPlaceholderText('e.g. ISP Gateway'), {
      target: { value: 'My ISP' },
    });
    // Submit
    fireEvent.click(screen.getByText('Add Virtual Node'));
    await waitFor(() => {
      expect(createDevice).toHaveBeenCalledWith(
        expect.objectContaining({
          device_type: 'virtual',
          hostname: 'My ISP',
          tags: expect.objectContaining({
            display_name: 'My ISP',
            virtual_subtype: 'internet',
          }),
        }),
      );
    });
  });

  it('submits virtual device without snmp field', async () => {
    render(<AddDevicePanel onDeviceAdded={vi.fn()} />);
    fireEvent.click(screen.getByText('Virtual Node'));
    fireEvent.change(screen.getByPlaceholderText('e.g. ISP Gateway'), {
      target: { value: 'Cloud Node' },
    });
    fireEvent.click(screen.getByText('Add Virtual Node'));
    await waitFor(() => {
      const callArg = (createDevice as ReturnType<typeof vi.fn>).mock.calls[0][0];
      expect(callArg).not.toHaveProperty('snmp');
    });
  });
});
