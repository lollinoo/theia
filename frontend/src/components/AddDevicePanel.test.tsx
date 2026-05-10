import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import {
  addDeviceToCanvasMap,
  assignCredentialProfile,
  createDevice,
  setWinBoxProfile,
  updateCanvasMapDeviceAreas,
} from '../api/client';
import { ServerError, ValidationError } from '../api/errors';
import { AddDevicePanel } from './AddDevicePanel';

// Mock API calls that fire in useEffect
vi.mock('../api/client', () => ({
  fetchSNMPProfiles: vi.fn().mockResolvedValue([]),
  revealSNMPProfile: vi.fn(),
  fetchCredentialProfiles: vi.fn().mockResolvedValue([]),
  assignCredentialProfile: vi.fn().mockResolvedValue(undefined),
  setWinBoxProfile: vi.fn().mockResolvedValue(undefined),
  addDeviceToCanvasMap: vi.fn().mockResolvedValue({
    id: 'map-1',
    name: 'Backbone',
    description: '',
    source_area_id: null,
    filter: {},
    is_default: false,
    device_count: 1,
    link_count: 0,
    position_count: 0,
    created_at: '',
    updated_at: '',
  }),
  updateCanvasMapDeviceAreas: vi.fn().mockResolvedValue({
    id: 'map-1',
    name: 'Backbone',
    description: '',
    source_area_id: null,
    filter: {},
    is_default: false,
    device_count: 1,
    link_count: 0,
    position_count: 0,
    created_at: '',
    updated_at: '',
  }),
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
    topology_discovery_mode: 'inherit',
    effective_topology_discovery_mode: 'off',
    topology_bootstrap_state: 'idle',
    last_topology_discovery_at: null,
    last_topology_discovery_result: '',
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

  it('renders topology discovery selector', () => {
    render(<AddDevicePanel onDeviceAdded={vi.fn()} />);

    const topologySelect = screen.getByLabelText('Topology Discovery');
    expect(topologySelect).toBeInTheDocument();
    expect(topologySelect).toHaveValue('inherit');
  });

  it('renders optional fields', () => {
    render(<AddDevicePanel onDeviceAdded={vi.fn()} />);

    // Custom name field
    expect(
      screen.getByPlaceholderText('Auto-discovered from SNMP / Prometheus'),
    ).toBeInTheDocument();
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

  it('adds a newly created virtual node to the selected saved map', async () => {
    render(<AddDevicePanel mapContext={{ mapId: 'map-1' }} onDeviceAdded={vi.fn()} />);

    fireEvent.click(screen.getByText('Virtual Node'));
    fireEvent.change(screen.getByPlaceholderText('e.g. ISP Gateway'), {
      target: { value: 'Internet Edge' },
    });
    fireEvent.click(screen.getByText('Add Virtual Node'));

    await waitFor(() => {
      expect(addDeviceToCanvasMap).toHaveBeenCalledWith('map-1', 'new-dev', {
        include_connected_links: true,
      });
    });
  });

  it('submits physical device with topology discovery mode override', async () => {
    render(<AddDevicePanel onDeviceAdded={vi.fn()} />);

    fireEvent.change(screen.getByPlaceholderText('192.168.1.1'), {
      target: { value: '10.0.0.1' },
    });
    fireEvent.change(screen.getByLabelText('Topology Discovery'), {
      target: { value: 'bootstrap_once' },
    });
    fireEvent.click(screen.getByText('Add Device'));

    await waitFor(() => {
      expect(createDevice).toHaveBeenCalledWith(
        expect.objectContaining({
          topology_discovery_mode: 'bootstrap_once',
        }),
      );
    });
  });

  it('adds a newly created physical device to the selected saved map', async () => {
    const onDeviceAdded = vi.fn();
    render(
      <AddDevicePanel
        areas={[
          {
            id: 'map-area-1',
            name: 'Map Area',
            description: '',
            color: '#00E676',
            device_count: 0,
            created_at: '',
            updated_at: '',
          },
        ]}
        mapContext={{ mapId: 'map-1' }}
        onDeviceAdded={onDeviceAdded}
      />,
    );

    fireEvent.change(screen.getByPlaceholderText('192.168.1.1'), {
      target: { value: '10.0.0.1' },
    });
    fireEvent.change(screen.getByDisplayValue('Unassigned - select area...'), {
      target: { value: 'map-area-1' },
    });
    fireEvent.click(screen.getByText('Add Device'));

    await waitFor(() => {
      expect(addDeviceToCanvasMap).toHaveBeenCalledWith('map-1', 'new-dev', {
        include_connected_links: true,
      });
    });
    expect(updateCanvasMapDeviceAreas).toHaveBeenCalledWith('map-1', {
      device_ids: ['new-dev'],
      area_ids: ['map-area-1'],
    });
    expect(createDevice).toHaveBeenCalledWith(
      expect.not.objectContaining({ area_ids: ['map-area-1'] }),
    );
    expect(onDeviceAdded).toHaveBeenCalled();
  });

  it('assigns selected credentials after creating a physical device', async () => {
    const { fetchCredentialProfiles } = await import('../api/client');
    (fetchCredentialProfiles as ReturnType<typeof vi.fn>).mockResolvedValueOnce([
      {
        id: 'p1',
        name: 'Admin SSH',
        description: '',
        username: 'admin',
        port: 22,
        auth_method: 'password',
        role: 'Admin',
        created_at: '',
        updated_at: '',
      },
      {
        id: 'p2',
        name: 'Read SSH',
        description: '',
        username: 'read',
        port: 22,
        auth_method: 'password',
        role: 'Read',
        created_at: '',
        updated_at: '',
      },
    ]);
    const onDeviceAdded = vi.fn();

    render(<AddDevicePanel onDeviceAdded={onDeviceAdded} />);

    await waitFor(() => {
      expect(screen.getByText('Admin SSH')).toBeInTheDocument();
    });
    fireEvent.click(screen.getByLabelText('Assign Admin SSH'));
    fireEvent.click(screen.getByLabelText('Use Admin SSH for WinBox'));
    fireEvent.change(screen.getByPlaceholderText('192.168.1.1'), {
      target: { value: '10.0.0.1' },
    });
    fireEvent.click(screen.getByText('Add Device'));

    await waitFor(() => {
      expect(createDevice).toHaveBeenCalled();
      expect(assignCredentialProfile).toHaveBeenCalledWith('new-dev', 'p1');
      expect(setWinBoxProfile).toHaveBeenCalledWith('new-dev', 'p1');
      expect(onDeviceAdded).toHaveBeenCalled();
    });
  });

  it('hides SSH credentials selection for virtual nodes', async () => {
    const { fetchCredentialProfiles } = await import('../api/client');
    (fetchCredentialProfiles as ReturnType<typeof vi.fn>).mockResolvedValueOnce([
      {
        id: 'p1',
        name: 'Admin SSH',
        description: '',
        username: 'admin',
        port: 22,
        auth_method: 'password',
        role: 'Admin',
        created_at: '',
        updated_at: '',
      },
    ]);

    render(<AddDevicePanel onDeviceAdded={vi.fn()} />);

    await waitFor(() => {
      expect(screen.getByText('Admin SSH')).toBeInTheDocument();
    });
    fireEvent.click(screen.getByText('Virtual Node'));

    expect(screen.queryByText('Admin SSH')).not.toBeInTheDocument();
  });
});

// --- Gap 1: AddDevicePanel blur+submit validation ---

describe('AddDevicePanel — onBlur hostname validation', () => {
  it('shows error text when IP/hostname input is blurred with an empty value', async () => {
    render(<AddDevicePanel onDeviceAdded={vi.fn()} />);

    const ipInput = screen.getByPlaceholderText('192.168.1.1');
    fireEvent.blur(ipInput);

    await waitFor(() => {
      expect(screen.getByText('IP address or hostname is required')).toBeInTheDocument();
    });
  });

  it('shows error text when IP/hostname input is blurred with an invalid value', async () => {
    render(<AddDevicePanel onDeviceAdded={vi.fn()} />);

    const ipInput = screen.getByPlaceholderText('192.168.1.1');
    fireEvent.change(ipInput, { target: { value: 'not valid!!' } });
    fireEvent.blur(ipInput);

    await waitFor(() => {
      expect(screen.getByText('Invalid IP address or hostname')).toBeInTheDocument();
    });
  });

  it('applies border-status-down class to IP field on invalid blur', async () => {
    render(<AddDevicePanel onDeviceAdded={vi.fn()} />);

    const ipInput = screen.getByPlaceholderText('192.168.1.1');
    fireEvent.blur(ipInput);

    await waitFor(() => {
      expect(ipInput.className).toContain('border-status-down');
    });
  });

  it('clears hostname error when user types in the field', async () => {
    render(<AddDevicePanel onDeviceAdded={vi.fn()} />);

    const ipInput = screen.getByPlaceholderText('192.168.1.1');
    fireEvent.blur(ipInput);

    await waitFor(() => {
      expect(screen.getByText('IP address or hostname is required')).toBeInTheDocument();
    });

    fireEvent.change(ipInput, { target: { value: '10.0.0.1' } });

    await waitFor(() => {
      expect(screen.queryByText('IP address or hostname is required')).not.toBeInTheDocument();
    });
  });
});

describe('AddDevicePanel — submit validates all fields before API call', () => {
  it('does not call createDevice when IP is empty on submit', async () => {
    const { container } = render(<AddDevicePanel onDeviceAdded={vi.fn()} />);

    // Use fireEvent.submit on the form to bypass HTML5 required constraint in jsdom
    const form = container.querySelector('form')!;
    fireEvent.submit(form);

    await waitFor(() => {
      expect(screen.getByText('IP address or hostname is required')).toBeInTheDocument();
    });
    expect(createDevice).not.toHaveBeenCalled();
  });

  it('does not call createDevice when IP is invalid on submit', async () => {
    const { container } = render(<AddDevicePanel onDeviceAdded={vi.fn()} />);

    const ipInput = screen.getByPlaceholderText('192.168.1.1');
    fireEvent.change(ipInput, { target: { value: '999' } });

    const form = container.querySelector('form')!;
    fireEvent.submit(form);

    await waitFor(() => {
      expect(screen.getByText('Invalid IP address or hostname')).toBeInTheDocument();
    });
    expect(createDevice).not.toHaveBeenCalled();
  });
});

// --- Gap 6: Backend typed error display ---

describe('AddDevicePanel — backend typed error display', () => {
  it('shows ServerError ref message when createDevice throws ServerError', async () => {
    (createDevice as ReturnType<typeof vi.fn>).mockRejectedValueOnce(
      new ServerError('internal error, ref: srv001', 'srv001'),
    );

    render(<AddDevicePanel onDeviceAdded={vi.fn()} />);

    const ipInput = screen.getByPlaceholderText('192.168.1.1');
    fireEvent.change(ipInput, { target: { value: '10.0.0.1' } });
    fireEvent.click(screen.getByText('Add Device'));

    await waitFor(() => {
      expect(screen.getByText('Something went wrong (ref: srv001)')).toBeInTheDocument();
    });
  });

  it('shows ValidationError message when createDevice throws ValidationError', async () => {
    (createDevice as ReturnType<typeof vi.fn>).mockRejectedValueOnce(
      new ValidationError('IP already exists'),
    );

    render(<AddDevicePanel onDeviceAdded={vi.fn()} />);

    const ipInput = screen.getByPlaceholderText('192.168.1.1');
    fireEvent.change(ipInput, { target: { value: '10.0.0.1' } });
    fireEvent.click(screen.getByText('Add Device'));

    await waitFor(() => {
      expect(screen.getByText('IP already exists')).toBeInTheDocument();
    });
  });

  it('shows plain Error message when createDevice throws plain Error', async () => {
    (createDevice as ReturnType<typeof vi.fn>).mockRejectedValueOnce(new Error('network failure'));

    render(<AddDevicePanel onDeviceAdded={vi.fn()} />);

    const ipInput = screen.getByPlaceholderText('192.168.1.1');
    fireEvent.change(ipInput, { target: { value: '10.0.0.1' } });
    fireEvent.click(screen.getByText('Add Device'));

    await waitFor(() => {
      expect(screen.getByText('network failure')).toBeInTheDocument();
    });
  });
});
