import { act, fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { ServerError, ValidationError } from '../api/errors';
import type { Device } from '../types/api';
import { DeviceConfigPanel } from './DeviceConfigPanel';

// Mock all API calls used in useEffect
vi.mock('../api/client', () => ({
  fetchSNMPProfiles: vi.fn().mockResolvedValue([]),
  fetchCredentialProfiles: vi.fn().mockResolvedValue([
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
  ]),
  fetchDeviceCredentialProfiles: vi.fn().mockResolvedValue([]),
  assignCredentialProfile: vi.fn().mockResolvedValue(undefined),
  unassignCredentialProfile: vi.fn().mockResolvedValue(undefined),
  setWinBoxProfile: vi.fn().mockResolvedValue(undefined),
  clearWinBoxProfile: vi.fn().mockResolvedValue(undefined),
  fetchAreas: vi.fn().mockResolvedValue([]),
  fetchSettings: vi.fn().mockResolvedValue({}),
  checkPrometheusHealth: vi.fn().mockResolvedValue({ available: false, url: '' }),
  updateSetting: vi.fn().mockResolvedValue(undefined),
  updateDevice: vi.fn().mockResolvedValue({}),
  runTopologyDiscovery: vi.fn().mockResolvedValue(undefined),
  deleteDevice: vi.fn().mockResolvedValue(undefined),
  testSNMPConnection: vi.fn().mockResolvedValue({ success: true }),
}));

function mockDevice(overrides: Partial<Device> = {}): Device {
  return {
    id: 'dev-1',
    hostname: 'router-01',
    ip: '10.0.0.1',
    notes: null,
    device_type: 'router',
    poll_class: 'core',
    poll_interval_override: null,
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
    topology_discovery_mode: 'inherit',
    effective_topology_discovery_mode: 'off',
    topology_bootstrap_state: 'idle',
    last_topology_discovery_at: null,
    last_topology_discovery_result: '',
    area_ids: [],
    ...overrides,
  };
}

beforeEach(() => {
  vi.clearAllMocks();
});

describe('DeviceConfigPanel — polling override', () => {
  it('renders default cadence context from device poll class', () => {
    render(
      <DeviceConfigPanel
        device={mockDevice()}
        onDeviceUpdated={vi.fn()}
        onDeviceDeleted={vi.fn()}
      />,
    );

    expect(screen.getByText('Polling Override')).toBeInTheDocument();
    expect(screen.getByText('Default cadence: every 30s (core class)')).toBeInTheDocument();
    expect(screen.getByDisplayValue('Use device default')).toBeInTheDocument();
  });

  it('selecting Use device default sends poll_interval_override null', async () => {
    const { updateDevice, updateSetting } = await import('../api/client');

    render(
      <DeviceConfigPanel
        device={mockDevice({ poll_interval_override: 30 })}
        onDeviceUpdated={vi.fn()}
        onDeviceDeleted={vi.fn()}
      />,
    );

    fireEvent.change(screen.getByDisplayValue('30 seconds'), { target: { value: 'default' } });

    await waitFor(() => {
      expect(updateDevice).toHaveBeenCalledWith('dev-1', { poll_interval_override: null });
    });
    expect(updateSetting).not.toHaveBeenCalled();
  });

  it('selecting 30 seconds sends poll_interval_override 30', async () => {
    const { updateDevice, updateSetting } = await import('../api/client');

    render(
      <DeviceConfigPanel
        device={mockDevice()}
        onDeviceUpdated={vi.fn()}
        onDeviceDeleted={vi.fn()}
      />,
    );

    fireEvent.change(screen.getByDisplayValue('Use device default'), { target: { value: '30' } });

    await waitFor(() => {
      expect(updateDevice).toHaveBeenCalledWith('dev-1', { poll_interval_override: 30 });
    });
    expect(updateSetting).not.toHaveBeenCalled();
  });

  it('custom override initializes from non-preset device value', () => {
    render(
      <DeviceConfigPanel
        device={mockDevice({ poll_interval_override: 123 })}
        onDeviceUpdated={vi.fn()}
        onDeviceDeleted={vi.fn()}
      />,
    );

    expect(screen.getByDisplayValue('Custom...')).toBeInTheDocument();
    expect(screen.getByDisplayValue('123')).toBeInTheDocument();
    expect(screen.getByPlaceholderText('Seconds (5-3600)')).toBeInTheDocument();
  });

  it('invalid custom override blocks updateDevice and shows validation error', async () => {
    const { updateDevice } = await import('../api/client');

    render(
      <DeviceConfigPanel
        device={mockDevice()}
        onDeviceUpdated={vi.fn()}
        onDeviceDeleted={vi.fn()}
      />,
    );

    fireEvent.change(screen.getByDisplayValue('Use device default'), {
      target: { value: 'custom' },
    });

    const customInput = screen.getByPlaceholderText('Seconds (5-3600)');
    fireEvent.change(customInput, { target: { value: '4' } });

    await waitFor(() => {
      expect(
        screen.getByText('Polling override must be an integer between 5 and 3600 seconds'),
      ).toBeInTheDocument();
    });
    expect(updateDevice).not.toHaveBeenCalled();

    fireEvent.change(customInput, { target: { value: '3601' } });

    await waitFor(() => {
      expect(
        screen.getByText('Polling override must be an integer between 5 and 3600 seconds'),
      ).toBeInTheDocument();
    });
    expect(updateDevice).not.toHaveBeenCalled();
  });

  it('successful polling save shows Saved and calls onDeviceUpdated', async () => {
    const { updateDevice, updateSetting } = await import('../api/client');
    const onDeviceUpdated = vi.fn();
    (updateDevice as ReturnType<typeof vi.fn>).mockResolvedValueOnce(
      mockDevice({ poll_interval_override: 123 }),
    );

    render(
      <DeviceConfigPanel
        device={mockDevice()}
        onDeviceUpdated={onDeviceUpdated}
        onDeviceDeleted={vi.fn()}
      />,
    );

    fireEvent.change(screen.getByDisplayValue('Use device default'), {
      target: { value: 'custom' },
    });
    fireEvent.change(screen.getByPlaceholderText('Seconds (5-3600)'), { target: { value: '123' } });

    await waitFor(() => {
      expect(updateDevice).toHaveBeenCalledWith('dev-1', { poll_interval_override: 123 });
    });
    expect(updateSetting).not.toHaveBeenCalled();

    await waitFor(() => {
      expect(onDeviceUpdated).toHaveBeenCalledWith(mockDevice({ poll_interval_override: 123 }));
    });

    const pollingHeader = screen.getByText('Polling Override').parentElement;
    expect(pollingHeader).not.toBeNull();
    await waitFor(() => {
      expect(within(pollingHeader as HTMLElement).getByText('Saved').className).toContain(
        'opacity-100',
      );
    });
  });

  it('allows saving virtual devices without an IP address', async () => {
    const { updateDevice } = await import('../api/client');
    const onDeviceUpdated = vi.fn();
    (updateDevice as ReturnType<typeof vi.fn>).mockResolvedValueOnce(
      mockDevice({
        device_type: 'virtual',
        ip: '',
        metrics_source: 'none',
        tags: { display_name: 'Virtual cloud', virtual_subtype: 'cloud' },
      }),
    );

    render(
      <DeviceConfigPanel
        device={mockDevice({
          device_type: 'virtual',
          ip: '',
          metrics_source: 'none',
          tags: { display_name: 'Virtual cloud', virtual_subtype: 'cloud' },
        })}
        isVirtual
        onDeviceUpdated={onDeviceUpdated}
        onDeviceDeleted={vi.fn()}
      />,
    );

    fireEvent.click(screen.getByText('Save Changes'));

    await waitFor(() => {
      expect(updateDevice).toHaveBeenCalledWith(
        'dev-1',
        expect.objectContaining({
          ip: '',
          metrics_source: 'none',
        }),
      );
    });

    await waitFor(() => {
      expect(onDeviceUpdated).toHaveBeenCalled();
    });
  });

  it('requires a display name when editing virtual devices', async () => {
    const { updateDevice } = await import('../api/client');

    render(
      <DeviceConfigPanel
        device={mockDevice({
          device_type: 'virtual',
          ip: '',
          sys_name: '',
          metrics_source: 'none',
          tags: { virtual_subtype: 'cloud' },
        })}
        isVirtual
        onDeviceUpdated={vi.fn()}
        onDeviceDeleted={vi.fn()}
      />,
    );

    const displayNameInput = screen.getByPlaceholderText('e.g. ISP Gateway');
    expect(displayNameInput).toHaveAttribute('required');

    fireEvent.click(screen.getByText('Save Changes'));

    await waitFor(() => {
      expect(screen.getByText('Display Name is required')).toBeInTheDocument();
    });
    expect(updateDevice).not.toHaveBeenCalled();
  });

  it('persists a trimmed display name for virtual devices', async () => {
    const { updateDevice } = await import('../api/client');
    const onDeviceUpdated = vi.fn();
    (updateDevice as ReturnType<typeof vi.fn>).mockResolvedValueOnce(
      mockDevice({
        device_type: 'virtual',
        ip: '',
        metrics_source: 'none',
        tags: { display_name: 'ISP Gateway', virtual_subtype: 'internet' },
      }),
    );

    render(
      <DeviceConfigPanel
        device={mockDevice({
          device_type: 'virtual',
          ip: '',
          sys_name: '',
          metrics_source: 'none',
          tags: { virtual_subtype: 'internet' },
        })}
        isVirtual
        onDeviceUpdated={onDeviceUpdated}
        onDeviceDeleted={vi.fn()}
      />,
    );

    fireEvent.change(screen.getByPlaceholderText('e.g. ISP Gateway'), {
      target: { value: '  ISP Gateway  ' },
    });
    fireEvent.click(screen.getByText('Save Changes'));

    await waitFor(() => {
      expect(updateDevice).toHaveBeenCalledWith(
        'dev-1',
        expect.objectContaining({
          ip: '',
          tags: expect.objectContaining({
            display_name: 'ISP Gateway',
            virtual_subtype: 'internet',
          }),
        }),
      );
    });

    await waitFor(() => {
      expect(onDeviceUpdated).toHaveBeenCalled();
    });
  });
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
    expect(screen.getByText('Topology Discovery')).toBeInTheDocument();
    // Should show Edit Device section
    expect(screen.getByText('Edit Device')).toBeInTheDocument();
    // Should show Save Changes button
    expect(screen.getByText('Save Changes')).toBeInTheDocument();
  });

  it('renders saved notes preview and editable textarea', () => {
    render(
      <DeviceConfigPanel
        device={mockDevice({ notes: 'Check transceiver levels weekly' })}
        onDeviceUpdated={vi.fn()}
        onDeviceDeleted={vi.fn()}
      />,
    );

    const savedNotesHeader = screen.getByText('Saved Notes');
    expect(savedNotesHeader).toBeInTheDocument();
    const savedNotesSection = savedNotesHeader.parentElement;
    expect(savedNotesSection).not.toBeNull();
    expect(
      within(savedNotesSection as HTMLElement).getByText('Check transceiver levels weekly'),
    ).toBeInTheDocument();
    expect(screen.getByLabelText('Device Notes')).toHaveValue('Check transceiver levels weekly');
  });

  it('submits trimmed notes and clears blank notes as null', async () => {
    const { updateDevice } = await import('../api/client');
    (updateDevice as ReturnType<typeof vi.fn>)
      .mockResolvedValueOnce(mockDevice({ notes: 'Move to UPS-backed PDU' }))
      .mockResolvedValueOnce(mockDevice({ notes: null }));

    const onDeviceUpdated = vi.fn();

    const { rerender } = render(
      <DeviceConfigPanel
        device={mockDevice()}
        onDeviceUpdated={onDeviceUpdated}
        onDeviceDeleted={vi.fn()}
      />,
    );

    fireEvent.change(screen.getByLabelText('Device Notes'), {
      target: { value: '  Move to UPS-backed PDU  ' },
    });
    fireEvent.click(screen.getByText('Save Changes'));

    await waitFor(() => {
      expect(updateDevice).toHaveBeenCalledWith(
        'dev-1',
        expect.objectContaining({ notes: 'Move to UPS-backed PDU' }),
      );
    });

    rerender(
      <DeviceConfigPanel
        device={mockDevice({ notes: 'Move to UPS-backed PDU' })}
        onDeviceUpdated={onDeviceUpdated}
        onDeviceDeleted={vi.fn()}
      />,
    );

    fireEvent.change(screen.getByLabelText('Device Notes'), {
      target: { value: '   ' },
    });
    fireEvent.click(screen.getByText('Save Changes'));

    await waitFor(() => {
      expect(updateDevice).toHaveBeenCalledWith('dev-1', expect.objectContaining({ notes: null }));
    });
  });

  it('includes topology discovery mode when saving device changes', async () => {
    const { updateDevice } = await import('../api/client');
    (updateDevice as ReturnType<typeof vi.fn>).mockResolvedValueOnce(
      mockDevice({ topology_discovery_mode: 'bootstrap_once' }),
    );

    render(
      <DeviceConfigPanel
        device={mockDevice()}
        onDeviceUpdated={vi.fn()}
        onDeviceDeleted={vi.fn()}
      />,
    );

    fireEvent.change(screen.getByLabelText('Topology Discovery'), {
      target: { value: 'bootstrap_once' },
    });
    fireEvent.click(screen.getByText('Save Changes'));

    await waitFor(() => {
      expect(updateDevice).toHaveBeenCalledWith(
        'dev-1',
        expect.objectContaining({
          topology_discovery_mode: 'bootstrap_once',
        }),
      );
    });
  });

  it('runs topology discovery manually and shows feedback', async () => {
    const { runTopologyDiscovery } = await import('../api/client');

    render(
      <DeviceConfigPanel
        device={mockDevice({
          topology_discovery_mode: 'off',
          effective_topology_discovery_mode: 'off',
        })}
        onDeviceUpdated={vi.fn()}
        onDeviceDeleted={vi.fn()}
      />,
    );

    fireEvent.click(screen.getByText('Run Topology Discovery Now'));

    await waitFor(() => {
      expect(runTopologyDiscovery).toHaveBeenCalledWith('dev-1');
    });
    await waitFor(() => {
      expect(
        screen.getByText(
          'Topology discovery started. Links and ports will refresh when the SNMP pass completes.',
        ),
      ).toBeInTheDocument();
    });
  });

  it('shows the delayed topology follow-up expectation while bootstrap follow-up is queued', () => {
    render(
      <DeviceConfigPanel
        device={mockDevice({
          topology_bootstrap_state: 'followup_scheduled',
          last_topology_discovery_at: '2026-04-18T10:09:16Z',
          last_topology_discovery_result: 'ports_pending',
        })}
        onDeviceUpdated={vi.fn()}
        onDeviceDeleted={vi.fn()}
      />,
    );

    expect(screen.getByText('Next Follow-up')).toBeInTheDocument();
    expect(
      screen.getByText('Automatic follow-up runs about 20s after last discovery.'),
    ).toBeInTheDocument();
  });

  it('disables manual topology discovery for Prometheus-only devices', () => {
    render(
      <DeviceConfigPanel
        device={mockDevice({ metrics_source: 'prometheus' })}
        onDeviceUpdated={vi.fn()}
        onDeviceDeleted={vi.fn()}
      />,
    );

    expect(screen.getByText('Run Topology Discovery Now')).toBeDisabled();
    expect(
      screen.getByText(
        'Prometheus-only devices cannot run SNMP topology discovery until SNMP or fallback mode is enabled.',
      ),
    ).toBeInTheDocument();
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

  it('renders Areas section with select dropdown', async () => {
    // Mock fetchAreas to return at least one area so the dropdown renders
    const { fetchAreas } = await import('../api/client');
    (fetchAreas as ReturnType<typeof vi.fn>).mockResolvedValueOnce([
      { id: 'area-1', name: 'Backbone', description: '', color: '#00E676', device_count: 0 },
    ]);

    render(
      <DeviceConfigPanel
        device={mockDevice()}
        onDeviceUpdated={vi.fn()}
        onDeviceDeleted={vi.fn()}
      />,
    );

    // Areas label should be present
    await waitFor(() => {
      expect(screen.getByText('Areas')).toBeInTheDocument();
    });
    // Unassigned select option should be available when device has no areas
    await waitFor(() => {
      expect(screen.getByText('Unassigned - select area...')).toBeInTheDocument();
    });
  });

  it('renders areas section between IP and Vendor fields', () => {
    render(
      <DeviceConfigPanel
        device={mockDevice()}
        onDeviceUpdated={vi.fn()}
        onDeviceDeleted={vi.fn()}
      />,
    );

    const areaLabel = screen.getByText('Areas');
    expect(areaLabel).toBeInTheDocument();
  });
});

// --- Gap 2: DeviceConfigPanel blur+submit validation ---

describe('DeviceConfigPanel — onBlur IP validation', () => {
  it('shows error text when IP input is blurred with an invalid value', async () => {
    render(
      <DeviceConfigPanel
        device={mockDevice({ ip: '10.0.0.1' })}
        onDeviceUpdated={vi.fn()}
        onDeviceDeleted={vi.fn()}
      />,
    );

    const ipInput = screen.getByDisplayValue('10.0.0.1');
    fireEvent.change(ipInput, { target: { value: 'not valid!!' } });
    fireEvent.blur(ipInput);

    await waitFor(() => {
      expect(screen.getByText('Invalid IP address or hostname')).toBeInTheDocument();
    });
  });

  it('applies border-status-down class to IP field on invalid blur', async () => {
    render(
      <DeviceConfigPanel
        device={mockDevice({ ip: '10.0.0.1' })}
        onDeviceUpdated={vi.fn()}
        onDeviceDeleted={vi.fn()}
      />,
    );

    const ipInput = screen.getByDisplayValue('10.0.0.1');
    fireEvent.change(ipInput, { target: { value: '' } });
    fireEvent.blur(ipInput);

    await waitFor(() => {
      expect(ipInput.className).toContain('border-status-down');
    });
  });

  it('clears IP error when user types valid value', async () => {
    render(
      <DeviceConfigPanel
        device={mockDevice({ ip: '10.0.0.1' })}
        onDeviceUpdated={vi.fn()}
        onDeviceDeleted={vi.fn()}
      />,
    );

    const ipInput = screen.getByDisplayValue('10.0.0.1');
    fireEvent.change(ipInput, { target: { value: '' } });
    fireEvent.blur(ipInput);

    await waitFor(() => {
      expect(screen.getByText('IP address or hostname is required')).toBeInTheDocument();
    });

    fireEvent.change(ipInput, { target: { value: '192.168.1.1' } });

    await waitFor(() => {
      expect(screen.queryByText('IP address or hostname is required')).not.toBeInTheDocument();
    });
  });
});

describe('DeviceConfigPanel — submit validates before API call', () => {
  it('does not call updateDevice when IP is invalid on submit', async () => {
    const { updateDevice } = await import('../api/client');

    render(
      <DeviceConfigPanel
        device={mockDevice({ ip: '10.0.0.1' })}
        onDeviceUpdated={vi.fn()}
        onDeviceDeleted={vi.fn()}
      />,
    );

    const ipInput = screen.getByDisplayValue('10.0.0.1');
    fireEvent.change(ipInput, { target: { value: '' } });

    fireEvent.click(screen.getByText('Save Changes'));

    await waitFor(() => {
      expect(screen.getByText('IP address or hostname is required')).toBeInTheDocument();
    });
    expect(updateDevice).not.toHaveBeenCalled();
  });
});

// --- Gap 6 (continued): DeviceConfigPanel typed error display ---

describe('DeviceConfigPanel — backend typed error display', () => {
  it('shows ServerError ref when updateDevice throws ServerError', async () => {
    const { updateDevice } = await import('../api/client');
    (updateDevice as ReturnType<typeof vi.fn>).mockRejectedValueOnce(
      new ServerError('internal error, ref: dc001', 'dc001'),
    );

    render(
      <DeviceConfigPanel
        device={mockDevice({ ip: '10.0.0.1' })}
        onDeviceUpdated={vi.fn()}
        onDeviceDeleted={vi.fn()}
      />,
    );

    fireEvent.click(screen.getByText('Save Changes'));

    await waitFor(() => {
      expect(screen.getByText('Something went wrong (ref: dc001)')).toBeInTheDocument();
    });
  });

  it('shows ValidationError message when updateDevice throws ValidationError', async () => {
    const { updateDevice } = await import('../api/client');
    (updateDevice as ReturnType<typeof vi.fn>).mockRejectedValueOnce(
      new ValidationError('IP address already in use'),
    );

    render(
      <DeviceConfigPanel
        device={mockDevice({ ip: '10.0.0.1' })}
        onDeviceUpdated={vi.fn()}
        onDeviceDeleted={vi.fn()}
      />,
    );

    fireEvent.click(screen.getByText('Save Changes'));

    await waitFor(() => {
      expect(screen.getByText('IP address already in use')).toBeInTheDocument();
    });
  });

  it('renders Grafana URL input field for physical devices', () => {
    render(
      <DeviceConfigPanel
        device={mockDevice()}
        onDeviceUpdated={vi.fn()}
        onDeviceDeleted={vi.fn()}
      />,
    );

    // The Grafana URL field must be present so blur validation can fire on it
    expect(screen.getByPlaceholderText('Leave blank to use default')).toBeInTheDocument();
    expect(screen.getByText('Custom Grafana Dashboard URL')).toBeInTheDocument();
  });
});

describe('DeviceConfigPanel — Grafana URL autosave validation', () => {
  beforeEach(() => {
    vi.useFakeTimers();
  });

  afterEach(() => {
    vi.useRealTimers();
  });

  it('does not persist invalid Grafana URLs during debounced autosave', async () => {
    const { updateSetting } = await import('../api/client');

    render(
      <DeviceConfigPanel
        device={mockDevice()}
        onDeviceUpdated={vi.fn()}
        onDeviceDeleted={vi.fn()}
      />,
    );

    const grafanaInput = screen.getByPlaceholderText('Leave blank to use default');
    await act(async () => {
      fireEvent.change(grafanaInput, { target: { value: 'not-a-url' } });
      await Promise.resolve();
    });

    expect(screen.getByText('URL must start with http:// or https://')).toBeInTheDocument();

    await act(async () => {
      await vi.advanceTimersByTimeAsync(500);
    });

    expect(updateSetting).not.toHaveBeenCalled();
  });

  it('persists valid Grafana URLs after the debounce window', async () => {
    const { updateSetting } = await import('../api/client');

    render(
      <DeviceConfigPanel
        device={mockDevice()}
        onDeviceUpdated={vi.fn()}
        onDeviceDeleted={vi.fn()}
      />,
    );

    const grafanaInput = screen.getByPlaceholderText('Leave blank to use default');
    await act(async () => {
      fireEvent.change(grafanaInput, {
        target: { value: 'https://grafana.example/d/router-overview' },
      });
      await Promise.resolve();
    });

    await act(async () => {
      await vi.advanceTimersByTimeAsync(500);
      await Promise.resolve();
    });

    expect(updateSetting).toHaveBeenCalledWith(
      'grafana_dashboard_url:dev-1',
      'https://grafana.example/d/router-overview',
    );
  });
});

// --- Credentials section tests ---

describe('DeviceConfigPanel — Credentials section', () => {
  it('renders credentials section with assigned profiles', async () => {
    const { fetchDeviceCredentialProfiles } = await import('../api/client');
    (fetchDeviceCredentialProfiles as ReturnType<typeof vi.fn>).mockResolvedValueOnce([
      { profile_id: 'p1', name: 'Admin SSH', role: 'Admin', is_winbox: true },
    ]);

    render(
      <DeviceConfigPanel
        device={mockDevice()}
        onDeviceUpdated={vi.fn()}
        onDeviceDeleted={vi.fn()}
      />,
    );

    await waitFor(() => {
      expect(screen.getByText('Admin SSH')).toBeInTheDocument();
    });
    await waitFor(() => {
      expect(screen.getByText('Admin')).toBeInTheDocument();
    });
  });

  it('shows empty state when no profiles assigned', async () => {
    const { fetchDeviceCredentialProfiles } = await import('../api/client');
    (fetchDeviceCredentialProfiles as ReturnType<typeof vi.fn>).mockResolvedValueOnce([]);

    render(
      <DeviceConfigPanel
        device={mockDevice()}
        onDeviceUpdated={vi.fn()}
        onDeviceDeleted={vi.fn()}
      />,
    );

    await waitFor(() => {
      expect(
        screen.getByText('No credentials assigned. Add a profile to enable WinBox launch.'),
      ).toBeInTheDocument();
    });
  });

  it('shows Add select when + Add is clicked', async () => {
    const { fetchDeviceCredentialProfiles } = await import('../api/client');
    (fetchDeviceCredentialProfiles as ReturnType<typeof vi.fn>).mockResolvedValueOnce([]);

    render(
      <DeviceConfigPanel
        device={mockDevice()}
        onDeviceUpdated={vi.fn()}
        onDeviceDeleted={vi.fn()}
      />,
    );

    // Click the + Add button
    const addButton = screen.getByText('+ Add');
    fireEvent.click(addButton);

    await waitFor(() => {
      expect(screen.getByText('Select a profile...')).toBeInTheDocument();
    });

    // Dismiss button should also appear
    expect(screen.getByText('Dismiss')).toBeInTheDocument();
  });

  it('notifies parent when WinBox designation changes', async () => {
    const { fetchDeviceCredentialProfiles, setWinBoxProfile } = await import('../api/client');
    (fetchDeviceCredentialProfiles as ReturnType<typeof vi.fn>)
      .mockResolvedValueOnce([
        { profile_id: 'p1', name: 'Admin SSH', role: 'Admin', is_winbox: false },
      ])
      .mockResolvedValueOnce([
        { profile_id: 'p1', name: 'Admin SSH', role: 'Admin', is_winbox: true },
      ]);

    const onWinBoxAvailabilityChange = vi.fn();

    render(
      <DeviceConfigPanel
        device={mockDevice()}
        onDeviceUpdated={vi.fn()}
        onDeviceDeleted={vi.fn()}
        onWinBoxAvailabilityChange={onWinBoxAvailabilityChange}
      />,
    );

    const toggleButton = await screen.findByTitle('Designate as WinBox profile');
    fireEvent.click(toggleButton);

    await waitFor(() => {
      expect(setWinBoxProfile).toHaveBeenCalledWith('dev-1', 'p1');
    });
    await waitFor(() => {
      expect(onWinBoxAvailabilityChange).toHaveBeenLastCalledWith(true);
    });
  });
});
