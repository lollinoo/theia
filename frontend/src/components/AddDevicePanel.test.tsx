/**
 * Exercises add device panel component behavior so refactors preserve the documented contract.
 */
import { act, fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import {
  addDeviceToCanvasMap,
  assignCredentialProfile,
  createDevice,
  fetchDevices,
  setWinBoxProfile,
  updateCanvasMapDeviceAreas,
  updateCanvasMapDeviceVisualColor,
} from '../api/client';
import { ServerError, ValidationError } from '../api/errors';
import type { Device } from '../types/api';
import { AddDevicePanel } from './AddDevicePanel';

// Mock API calls that fire in useEffect
vi.mock('../api/client', () => {
  const pendingApiCall = () => new Promise<never>(() => {});

  return {
    fetchSNMPProfiles: vi.fn().mockImplementation(pendingApiCall),
    revealSNMPProfile: vi.fn(),
    fetchCredentialProfiles: vi.fn().mockImplementation(pendingApiCall),
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
    updateCanvasMapDeviceVisualColor: vi.fn().mockResolvedValue({
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
    fetchDevices: vi.fn().mockResolvedValue([]),
    fetchAreas: vi.fn().mockImplementation(pendingApiCall),
    checkPrometheusHealth: vi.fn().mockImplementation(pendingApiCall),
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
  };
});

beforeEach(() => {
  vi.clearAllMocks();
  (fetchDevices as ReturnType<typeof vi.fn>).mockResolvedValue([]);
});

function mapDevice(overrides: Partial<Device>): Device {
  return {
    id: 'map-device',
    hostname: 'map-device',
    ip: '10.0.0.1',
    device_type: 'router',
    poll_class: 'standard',
    poll_interval_override: null,
    polling_enabled: true,
    status: 'up',
    sys_name: '',
    sys_descr: '',
    hardware_model: '',
    vendor: 'default',
    managed: true,
    interfaces: [],
    area_ids: [],
    backup_supported: false,
    metrics_source: 'snmp',
    prometheus_label_name: 'instance',
    prometheus_label_value: '',
    ...overrides,
  };
}

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
      expect(createDevice).toHaveBeenCalledWith(
        expect.objectContaining({ skip_primary_map_membership: true }),
      );
      expect(addDeviceToCanvasMap).toHaveBeenCalledWith('map-1', 'new-dev', {
        include_connected_links: true,
      });
    });
  });

  it('applies selected visual color after adding a virtual node to the selected saved map', async () => {
    render(<AddDevicePanel mapContext={{ mapId: 'map-1' }} onDeviceAdded={vi.fn()} />);

    fireEvent.click(screen.getByText('Virtual Node'));
    fireEvent.change(screen.getByPlaceholderText('e.g. ISP Gateway'), {
      target: { value: 'Internet Edge' },
    });
    fireEvent.change(screen.getByLabelText('Virtual node color'), {
      target: { value: '#123abc' },
    });
    fireEvent.click(screen.getByText('Add Virtual Node'));

    await waitFor(() => {
      expect(addDeviceToCanvasMap).toHaveBeenCalledWith('map-1', 'new-dev', {
        include_connected_links: true,
      });
      expect(updateCanvasMapDeviceVisualColor).toHaveBeenCalledWith('map-1', 'new-dev', {
        visual_color: '#123ABC',
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

  it('submits legacy physical payload when no additional address rows exist', async () => {
    render(<AddDevicePanel onDeviceAdded={vi.fn()} />);

    fireEvent.change(screen.getByPlaceholderText('192.168.1.1'), {
      target: { value: '10.0.0.1' },
    });
    fireEvent.click(screen.getByText('Add Device'));

    await waitFor(() => {
      expect(createDevice).toHaveBeenCalledWith(expect.not.objectContaining({ addresses: [] }));
      const payload = (createDevice as ReturnType<typeof vi.fn>).mock.calls[0][0];
      expect(payload).not.toHaveProperty('addresses');
    });
  });

  it('submits backup address rows for physical devices', async () => {
    render(<AddDevicePanel onDeviceAdded={vi.fn()} />);

    fireEvent.change(screen.getByPlaceholderText('192.168.1.1'), {
      target: { value: '10.0.0.1' },
    });
    fireEvent.click(screen.getByRole('button', { name: 'Add address' }));
    fireEvent.change(screen.getByLabelText('Additional address 1'), {
      target: { value: '192.0.2.10' },
    });
    fireEvent.change(screen.getByLabelText('Address role 1'), {
      target: { value: 'backup' },
    });
    fireEvent.change(screen.getByLabelText('Address label 1'), {
      target: { value: 'OOB' },
    });
    fireEvent.click(screen.getByText('Add Device'));

    await waitFor(() => {
      expect(createDevice).toHaveBeenCalledWith(
        expect.objectContaining({
          addresses: [
            {
              address: '10.0.0.1',
              role: 'primary',
              is_primary: true,
              priority: 0,
            },
            {
              address: '192.0.2.10',
              role: 'backup',
              label: 'OOB',
              is_primary: false,
              priority: 10,
            },
          ],
        }),
      );
    });
  });

  it('stacks additional address controls vertically with compact visible labels', () => {
    render(<AddDevicePanel onDeviceAdded={vi.fn()} />);

    fireEvent.click(screen.getByRole('button', { name: 'Add address' }));

    const row = screen.getByTestId('additional-address-row-1');
    expect(row).toHaveClass('space-y-3');
    expect(within(row).getByText('Address')).toBeInTheDocument();
    expect(within(row).getByText('Role')).toBeInTheDocument();
    expect(within(row).getByText('Label')).toBeInTheDocument();
    expect(within(row).getByText('Probe ports')).toBeInTheDocument();
    expect(within(row).queryByText('Address label 1')).not.toBeInTheDocument();
    expect(screen.getByLabelText('Address label 1')).toBeInTheDocument();
    expect(screen.getByLabelText('Address probe ports 1')).toBeInTheDocument();
  });

  it('submits physical device and address probe ports', async () => {
    render(<AddDevicePanel onDeviceAdded={vi.fn()} />);

    await act(async () => {
      fireEvent.change(screen.getByPlaceholderText('192.168.1.1'), {
        target: { value: '10.0.0.1' },
      });
      fireEvent.change(screen.getByLabelText('Probe ports'), {
        target: { value: '22,8291' },
      });
      fireEvent.click(screen.getByRole('button', { name: 'Add address' }));
    });

    await act(async () => {
      fireEvent.change(screen.getByLabelText('Additional address 1'), {
        target: { value: '192.0.2.10' },
      });
      fireEvent.change(screen.getByLabelText('Address probe ports 1'), {
        target: { value: '2222' },
      });
      fireEvent.click(screen.getByText('Add Device'));
    });

    await waitFor(() => {
      expect(createDevice).toHaveBeenCalledWith(
        expect.objectContaining({
          probe_ports: [22, 8291],
          addresses: expect.arrayContaining([
            expect.objectContaining({
              address: '192.0.2.10',
              probe_ports: [2222],
            }),
          ]),
        }),
      );
    });
  });

  it('blocks submit when physical probe ports are invalid', async () => {
    render(<AddDevicePanel onDeviceAdded={vi.fn()} />);

    await act(async () => {
      fireEvent.change(screen.getByPlaceholderText('192.168.1.1'), {
        target: { value: '10.0.0.1' },
      });
      fireEvent.change(screen.getByLabelText('Probe ports'), {
        target: { value: '0,443' },
      });
      fireEvent.click(screen.getByText('Add Device'));
    });

    expect(await screen.findByText('Ports must be between 1 and 65535')).toBeInTheDocument();
    expect(createDevice).not.toHaveBeenCalled();
  });

  it('blocks submit when a blank additional address row has invalid probe ports', async () => {
    render(<AddDevicePanel onDeviceAdded={vi.fn()} />);

    await act(async () => {
      fireEvent.change(screen.getByPlaceholderText('192.168.1.1'), {
        target: { value: '10.0.0.1' },
      });
      fireEvent.click(screen.getByRole('button', { name: 'Add address' }));
    });

    await act(async () => {
      fireEvent.change(screen.getByLabelText('Address probe ports 1'), {
        target: { value: '0' },
      });
      fireEvent.click(screen.getByText('Add Device'));
    });

    expect(await screen.findByText('Ports must be between 1 and 65535')).toBeInTheDocument();
    expect(createDevice).not.toHaveBeenCalled();
  });

  it('blocks submit when an additional address is invalid', async () => {
    render(<AddDevicePanel onDeviceAdded={vi.fn()} />);

    fireEvent.change(screen.getByPlaceholderText('192.168.1.1'), {
      target: { value: '10.0.0.1' },
    });
    fireEvent.click(screen.getByRole('button', { name: 'Add address' }));
    fireEvent.change(screen.getByLabelText('Additional address 1'), {
      target: { value: 'not valid!!' },
    });
    fireEvent.click(screen.getByText('Add Device'));

    expect(await screen.findByText('Invalid IP address or hostname')).toBeInTheDocument();
    expect(createDevice).not.toHaveBeenCalled();
  });

  it('blocks submit when additional addresses duplicate the primary address', async () => {
    render(<AddDevicePanel onDeviceAdded={vi.fn()} />);

    fireEvent.change(screen.getByPlaceholderText('192.168.1.1'), {
      target: { value: '10.0.0.1' },
    });
    fireEvent.click(screen.getByRole('button', { name: 'Add address' }));
    fireEvent.change(screen.getByLabelText('Additional address 1'), {
      target: { value: '10.0.0.1' },
    });
    fireEvent.click(screen.getByText('Add Device'));

    expect(await screen.findByText('Duplicate device address')).toBeInTheDocument();
    expect(createDevice).not.toHaveBeenCalled();
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
      expect.objectContaining({ skip_primary_map_membership: true }),
    );
    expect(createDevice).toHaveBeenCalledWith(
      expect.not.objectContaining({ area_ids: ['map-area-1'] }),
    );
    expect(onDeviceAdded).toHaveBeenCalledWith('new-dev');
    expect(onDeviceAdded).toHaveBeenCalledTimes(1);
  });

  it('adds an existing physical device to the selected saved map when create reports duplicate address', async () => {
    const onDeviceAdded = vi.fn();
    (createDevice as ReturnType<typeof vi.fn>).mockRejectedValueOnce(
      new ValidationError('a device with IP/host "10.0.0.1" already exists'),
    );
    (fetchDevices as ReturnType<typeof vi.fn>).mockResolvedValueOnce([
      {
        id: 'existing-dev',
        hostname: 'existing-router',
        ip: '10.0.0.1',
        device_type: 'router',
        poll_class: 'standard',
        poll_interval_override: null,
        polling_enabled: true,
        status: 'up',
        sys_name: 'existing-router',
        sys_descr: '',
        hardware_model: '',
        vendor: 'mikrotik',
        managed: true,
        interfaces: [],
        backup_supported: true,
        metrics_source: 'snmp',
        prometheus_label_name: 'instance',
        prometheus_label_value: '',
        area_ids: [],
      },
    ]);

    render(<AddDevicePanel mapContext={{ mapId: 'map-1' }} onDeviceAdded={onDeviceAdded} />);

    fireEvent.change(screen.getByPlaceholderText('192.168.1.1'), {
      target: { value: '10.0.0.1' },
    });
    fireEvent.click(screen.getByText('Add Device'));

    await waitFor(() => {
      expect(addDeviceToCanvasMap).toHaveBeenCalledWith('map-1', 'existing-dev', {
        include_connected_links: true,
      });
    });

    expect(fetchDevices).toHaveBeenCalled();
    expect(assignCredentialProfile).not.toHaveBeenCalled();
    expect(onDeviceAdded).toHaveBeenCalledWith('existing-dev');
    expect(onDeviceAdded).toHaveBeenCalledTimes(1);
    expect(screen.queryByText(/already exists/i)).not.toBeInTheDocument();
  });

  it('keeps the panel open when the existing physical device is already in the selected saved map', async () => {
    const onDeviceAdded = vi.fn();
    (createDevice as ReturnType<typeof vi.fn>).mockRejectedValueOnce(
      new ValidationError('a device with IP/host "10.0.0.1" already exists'),
    );
    (fetchDevices as ReturnType<typeof vi.fn>).mockResolvedValueOnce([
      {
        id: 'existing-dev',
        hostname: 'existing-router',
        ip: '10.0.0.1',
        device_type: 'router',
        poll_class: 'standard',
        poll_interval_override: null,
        polling_enabled: true,
        status: 'up',
        sys_name: 'existing-router',
        sys_descr: '',
        hardware_model: '',
        vendor: 'mikrotik',
        managed: true,
        interfaces: [],
        backup_supported: true,
        metrics_source: 'snmp',
        prometheus_label_name: 'instance',
        prometheus_label_value: '',
        area_ids: [],
      },
    ]);
    (addDeviceToCanvasMap as ReturnType<typeof vi.fn>).mockRejectedValueOnce(
      new ValidationError('device already exists in this map'),
    );

    render(<AddDevicePanel mapContext={{ mapId: 'map-1' }} onDeviceAdded={onDeviceAdded} />);

    fireEvent.change(screen.getByPlaceholderText('192.168.1.1'), {
      target: { value: '10.0.0.1' },
    });
    fireEvent.click(screen.getByText('Add Device'));

    expect(await screen.findByText('device already exists in this map')).toBeInTheDocument();
    expect(addDeviceToCanvasMap).toHaveBeenCalledWith('map-1', 'existing-dev', {
      include_connected_links: true,
    });
    expect(onDeviceAdded).not.toHaveBeenCalled();
    expect(screen.getByText('Add Device')).toBeInTheDocument();
  });

  it('blocks a physical device when the selected saved map already has a virtual node with the same IP', async () => {
    const onDeviceAdded = vi.fn();
    render(
      <AddDevicePanel
        devices={[
          mapDevice({
            id: 'virtual-dev',
            hostname: 'virtual-edge',
            ip: '10.0.0.1',
            device_type: 'virtual',
          }),
        ]}
        mapContext={{ mapId: 'map-1' }}
        onDeviceAdded={onDeviceAdded}
      />,
    );

    fireEvent.change(screen.getByPlaceholderText('192.168.1.1'), {
      target: { value: '10.0.0.1' },
    });
    fireEvent.click(screen.getByText('Add Device'));

    expect(
      await screen.findByText('a device with IP/host "10.0.0.1" already exists in this map'),
    ).toBeInTheDocument();
    expect(createDevice).not.toHaveBeenCalled();
    expect(addDeviceToCanvasMap).not.toHaveBeenCalled();
    expect(onDeviceAdded).not.toHaveBeenCalled();
  });

  it('blocks selected-map add when the new primary address matches an existing secondary address', async () => {
    const onDeviceAdded = vi.fn();
    render(
      <AddDevicePanel
        devices={[
          mapDevice({
            id: 'map-dev',
            hostname: 'router-01',
            ip: '10.0.0.1',
            addresses: [
              {
                id: 'addr-primary',
                device_id: 'map-dev',
                address: '10.0.0.1',
                label: '',
                role: 'primary',
                is_primary: true,
                priority: 0,
              },
              {
                id: 'addr-backup',
                device_id: 'map-dev',
                address: '192.0.2.10',
                label: 'OOB',
                role: 'backup',
                is_primary: false,
                priority: 10,
              },
            ],
          }),
        ]}
        mapContext={{ mapId: 'map-1' }}
        onDeviceAdded={onDeviceAdded}
      />,
    );

    fireEvent.change(screen.getByPlaceholderText('192.168.1.1'), {
      target: { value: '192.0.2.10' },
    });
    fireEvent.click(screen.getByText('Add Device'));

    expect(
      await screen.findByText('a device with IP/host "192.0.2.10" already exists in this map'),
    ).toBeInTheDocument();
    expect(createDevice).not.toHaveBeenCalled();
    expect(addDeviceToCanvasMap).not.toHaveBeenCalled();
    expect(onDeviceAdded).not.toHaveBeenCalled();
  });

  it('blocks a virtual node with IP when the selected saved map already has a physical device with the same IP', async () => {
    const onDeviceAdded = vi.fn();
    render(
      <AddDevicePanel
        devices={[
          mapDevice({
            id: 'physical-dev',
            hostname: 'physical-edge',
            ip: '10.0.0.1',
            device_type: 'router',
          }),
        ]}
        mapContext={{ mapId: 'map-1' }}
        onDeviceAdded={onDeviceAdded}
      />,
    );

    fireEvent.click(screen.getByText('Virtual Node'));
    fireEvent.change(screen.getByPlaceholderText('e.g. ISP Gateway'), {
      target: { value: 'Internet Edge' },
    });
    fireEvent.change(screen.getByPlaceholderText('e.g. 203.0.113.1'), {
      target: { value: '10.0.0.1' },
    });
    fireEvent.click(screen.getByText('Add Virtual Node'));

    expect(
      await screen.findByText('a device with IP/host "10.0.0.1" already exists in this map'),
    ).toBeInTheDocument();
    expect(createDevice).not.toHaveBeenCalled();
    expect(addDeviceToCanvasMap).not.toHaveBeenCalled();
    expect(onDeviceAdded).not.toHaveBeenCalled();
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
      expect(onDeviceAdded).toHaveBeenCalledWith('new-dev');
      expect(onDeviceAdded).toHaveBeenCalledTimes(1);
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
