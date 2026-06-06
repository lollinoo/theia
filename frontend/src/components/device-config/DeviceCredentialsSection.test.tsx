/**
 * Exercises device credentials section device configuration behavior so refactors preserve the documented contract.
 */
import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import type { Device, DeviceCredentialProfile } from '../../types/api';
import { DeviceCredentialsSection } from './DeviceCredentialsSection';

vi.mock('../../api/client', () => ({
  assignCredentialProfile: vi.fn().mockResolvedValue(undefined),
  clearWinBoxProfile: vi.fn().mockResolvedValue(undefined),
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
  setWinBoxProfile: vi.fn().mockResolvedValue(undefined),
  unassignCredentialProfile: vi.fn().mockResolvedValue(undefined),
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
    polling_enabled: true,
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

function createDeferredAssignments(assignments: DeviceCredentialProfile[]) {
  let resolve: (value: DeviceCredentialProfile[]) => void;
  const promise = new Promise<DeviceCredentialProfile[]>((promiseResolve) => {
    resolve = promiseResolve;
  });
  return {
    promise,
    resolve: () => resolve(assignments),
  };
}

beforeEach(() => {
  vi.clearAllMocks();
});

describe('DeviceCredentialsSection', () => {
  it('loads credential profiles and assigned profiles for a physical device', async () => {
    const { fetchCredentialProfiles, fetchDeviceCredentialProfiles } = await import(
      '../../api/client'
    );
    (fetchDeviceCredentialProfiles as ReturnType<typeof vi.fn>).mockResolvedValueOnce([
      { profile_id: 'p1', name: 'Admin SSH', role: 'Admin', is_winbox: true },
    ]);
    const onWinBoxAvailabilityChange = vi.fn();

    render(
      <DeviceCredentialsSection
        device={mockDevice()}
        onWinBoxAvailabilityChange={onWinBoxAvailabilityChange}
      />,
    );

    await waitFor(() => {
      expect(screen.getByText('Admin SSH')).toBeInTheDocument();
    });

    expect(fetchCredentialProfiles).toHaveBeenCalledTimes(1);
    expect(fetchDeviceCredentialProfiles).toHaveBeenCalledWith('dev-1');
    expect(screen.getByText('Admin')).toBeInTheDocument();
    expect(screen.getByTitle('Clear WinBox designation')).toBeInTheDocument();
    expect(onWinBoxAvailabilityChange).toHaveBeenLastCalledWith(true);
  });

  it('assigns a selected profile, hides the add select, and reloads assignments', async () => {
    const { assignCredentialProfile, fetchDeviceCredentialProfiles } = await import(
      '../../api/client'
    );
    (fetchDeviceCredentialProfiles as ReturnType<typeof vi.fn>)
      .mockResolvedValueOnce([])
      .mockResolvedValueOnce([
        { profile_id: 'p2', name: 'Read SSH', role: 'Read', is_winbox: false },
      ]);

    render(<DeviceCredentialsSection device={mockDevice()} />);

    await waitFor(() => {
      expect(
        screen.getByText('No credentials assigned. Add a profile to enable WinBox launch.'),
      ).toBeInTheDocument();
    });

    fireEvent.click(screen.getByText('+ Add'));
    fireEvent.change(screen.getByDisplayValue('Select a profile...'), {
      target: { value: 'p2' },
    });

    await waitFor(() => {
      expect(assignCredentialProfile).toHaveBeenCalledWith('dev-1', 'p2');
      expect(screen.getByText('Read SSH')).toBeInTheDocument();
    });
    expect(screen.queryByText('Select a profile...')).not.toBeInTheDocument();
  });

  it('unassigns after delete confirmation and reloads assignments', async () => {
    const { fetchDeviceCredentialProfiles, unassignCredentialProfile } = await import(
      '../../api/client'
    );
    (fetchDeviceCredentialProfiles as ReturnType<typeof vi.fn>)
      .mockResolvedValueOnce([
        { profile_id: 'p1', name: 'Admin SSH', role: 'Admin', is_winbox: true },
      ])
      .mockResolvedValueOnce([]);

    render(<DeviceCredentialsSection device={mockDevice()} />);

    fireEvent.click(await screen.findByTitle('Remove assignment'));
    expect(screen.getByText('Delete this profile?')).toBeInTheDocument();
    expect(screen.getByText('Keep Profile')).toBeInTheDocument();
    fireEvent.click(screen.getByText('Delete'));

    await waitFor(() => {
      expect(unassignCredentialProfile).toHaveBeenCalledWith('dev-1', 'p1');
      expect(
        screen.getByText('No credentials assigned. Add a profile to enable WinBox launch.'),
      ).toBeInTheDocument();
    });
    expect(screen.queryByText('Delete this profile?')).not.toBeInTheDocument();
  });

  it('toggles WinBox designation and notifies from the reloaded assignments', async () => {
    const { fetchDeviceCredentialProfiles, setWinBoxProfile } = await import('../../api/client');
    (fetchDeviceCredentialProfiles as ReturnType<typeof vi.fn>)
      .mockResolvedValueOnce([
        { profile_id: 'p1', name: 'Admin SSH', role: 'Admin', is_winbox: false },
      ])
      .mockResolvedValueOnce([
        { profile_id: 'p1', name: 'Admin SSH', role: 'Admin', is_winbox: true },
      ]);
    const onWinBoxAvailabilityChange = vi.fn();

    render(
      <DeviceCredentialsSection
        device={mockDevice()}
        onWinBoxAvailabilityChange={onWinBoxAvailabilityChange}
      />,
    );

    fireEvent.click(await screen.findByTitle('Designate as WinBox profile'));

    await waitFor(() => {
      expect(setWinBoxProfile).toHaveBeenCalledWith('dev-1', 'p1');
      expect(screen.getByTitle('Clear WinBox designation')).toBeInTheDocument();
    });
    expect(onWinBoxAvailabilityChange).toHaveBeenLastCalledWith(true);
  });

  it('clears assignments and reports WinBox unavailable for virtual devices', async () => {
    const { fetchDeviceCredentialProfiles } = await import('../../api/client');
    const onWinBoxAvailabilityChange = vi.fn();

    render(
      <DeviceCredentialsSection
        device={mockDevice({ device_type: 'virtual' })}
        isVirtual
        onWinBoxAvailabilityChange={onWinBoxAvailabilityChange}
      />,
    );

    await waitFor(() => {
      expect(onWinBoxAvailabilityChange).toHaveBeenCalledWith(false);
    });
    expect(fetchDeviceCredentialProfiles).not.toHaveBeenCalled();
    expect(screen.queryByText('Credentials')).not.toBeInTheDocument();
  });

  it('does not show stale assignments after switching devices', async () => {
    const { fetchDeviceCredentialProfiles } = await import('../../api/client');
    const staleAssignments = createDeferredAssignments([
      { profile_id: 'p1', name: 'Admin SSH', role: 'Admin', is_winbox: true },
    ]);
    (fetchDeviceCredentialProfiles as ReturnType<typeof vi.fn>)
      .mockReturnValueOnce(staleAssignments.promise)
      .mockResolvedValueOnce([
        { profile_id: 'p2', name: 'Read SSH', role: 'Read', is_winbox: false },
      ]);

    const { rerender } = render(<DeviceCredentialsSection device={mockDevice()} />);

    rerender(
      <DeviceCredentialsSection device={mockDevice({ id: 'dev-2', hostname: 'router-02' })} />,
    );

    await waitFor(() => {
      expect(screen.getByText('Read SSH')).toBeInTheDocument();
    });

    staleAssignments.resolve();

    await waitFor(() => {
      expect(screen.queryByText('Admin SSH')).not.toBeInTheDocument();
      expect(screen.getByText('Read SSH')).toBeInTheDocument();
    });
  });

  it('keeps read-only controls inert', async () => {
    const {
      assignCredentialProfile,
      fetchDeviceCredentialProfiles,
      setWinBoxProfile,
      unassignCredentialProfile,
    } = await import('../../api/client');
    (fetchDeviceCredentialProfiles as ReturnType<typeof vi.fn>).mockResolvedValueOnce([
      { profile_id: 'p1', name: 'Admin SSH', role: 'Admin', is_winbox: false },
    ]);

    render(<DeviceCredentialsSection device={mockDevice()} readOnly />);

    fireEvent.click(await screen.findByTitle('Designate as WinBox profile'));
    fireEvent.click(screen.getByTitle('Remove assignment'));
    fireEvent.click(screen.getByText('+ Add'));
    fireEvent.change(screen.getByDisplayValue('Select a profile...'), {
      target: { value: 'p2' },
    });

    expect(setWinBoxProfile).not.toHaveBeenCalled();
    expect(unassignCredentialProfile).not.toHaveBeenCalled();
    expect(assignCredentialProfile).not.toHaveBeenCalled();
  });
});
