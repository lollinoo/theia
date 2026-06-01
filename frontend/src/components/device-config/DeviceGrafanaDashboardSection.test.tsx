import { act, fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import type { Device, GrafanaDashboardConfig } from '../../types/api';
import { DeviceGrafanaDashboardSection } from './DeviceGrafanaDashboardSection';

vi.mock('../../api/client', () => ({
  fetchGrafanaDashboardConfig: vi.fn().mockResolvedValue({
    profiles: [
      {
        id: 'grafana-profile-1',
        name: 'RouterBoard shared',
        url_template: 'https://grafana.example/d/router?var-device={{hostname}}',
        variable_source: 'hostname',
      },
    ],
    default_profile_id: '',
    device_overrides: {},
  }),
  fetchSettings: vi.fn().mockResolvedValue({}),
  saveDeviceGrafanaDashboardOverride: vi.fn().mockResolvedValue({
    profiles: [
      {
        id: 'grafana-profile-1',
        name: 'RouterBoard shared',
        url_template: 'https://grafana.example/d/router?var-device={{hostname}}',
        variable_source: 'hostname',
      },
    ],
    default_profile_id: '',
    device_overrides: {},
  }),
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

function mockGrafanaConfig(): GrafanaDashboardConfig {
  return {
    profiles: [
      {
        id: 'grafana-profile-1',
        name: 'RouterBoard shared',
        url_template: 'https://grafana.example/d/router?var-device={{hostname}}',
        variable_source: 'hostname',
      },
    ],
    default_profile_id: '',
    device_overrides: {},
  };
}

function createDeferredSave() {
  let resolve: (value: GrafanaDashboardConfig) => void;
  const promise = new Promise<GrafanaDashboardConfig>((promiseResolve) => {
    resolve = promiseResolve;
  });
  return {
    promise,
    resolve: () => resolve(mockGrafanaConfig()),
  };
}

beforeEach(() => {
  vi.clearAllMocks();
});

describe('DeviceGrafanaDashboardSection', () => {
  it('initializes from device override before legacy settings fallback', async () => {
    const { fetchGrafanaDashboardConfig, fetchSettings } = await import('../../api/client');
    (fetchSettings as ReturnType<typeof vi.fn>).mockResolvedValueOnce({
      'grafana_dashboard_url:dev-1': 'https://legacy.example/d/router',
    });
    (fetchGrafanaDashboardConfig as ReturnType<typeof vi.fn>).mockResolvedValueOnce({
      profiles: [
        {
          id: 'grafana-profile-1',
          name: 'RouterBoard shared',
          url_template: 'https://grafana.example/d/router?var-device={{hostname}}',
          variable_source: 'hostname',
        },
      ],
      default_profile_id: '',
      device_overrides: {
        'dev-1': {
          profile_id: 'grafana-profile-1',
          custom_url: 'https://override.example/d/router',
        },
      },
    });

    render(<DeviceGrafanaDashboardSection device={mockDevice()} />);

    expect(screen.getByText('Grafana Dashboard')).toBeInTheDocument();
    await waitFor(() => {
      expect(screen.getByPlaceholderText('Optional custom URL override')).toHaveValue(
        'https://override.example/d/router',
      );
    });
    expect(screen.getByRole('combobox', { name: /dashboard profile/i })).toHaveValue(
      'grafana-profile-1',
    );
  });

  it('debounces valid URL saves with the selected profile and shows Saved', async () => {
    const { saveDeviceGrafanaDashboardOverride } = await import('../../api/client');
    const onSettingsChange = vi.fn();

    render(
      <DeviceGrafanaDashboardSection device={mockDevice()} onSettingsChange={onSettingsChange} />,
    );

    await waitFor(() => {
      expect(screen.getByRole('combobox', { name: /dashboard profile/i })).toBeInTheDocument();
    });

    vi.useFakeTimers();
    try {
      fireEvent.change(screen.getByRole('combobox', { name: /dashboard profile/i }), {
        target: { value: 'grafana-profile-1' },
      });
      fireEvent.change(screen.getByPlaceholderText('Optional custom URL override'), {
        target: { value: 'https://grafana.example/d/router-overview' },
      });

      await act(async () => {
        await vi.advanceTimersByTimeAsync(500);
      });

      expect(saveDeviceGrafanaDashboardOverride).toHaveBeenLastCalledWith('dev-1', {
        profile_id: 'grafana-profile-1',
        custom_url: 'https://grafana.example/d/router-overview',
      });
      expect(onSettingsChange).toHaveBeenCalled();

      const grafanaHeader = screen.getByText('Grafana Dashboard').parentElement;
      expect(grafanaHeader).not.toBeNull();
      expect(within(grafanaHeader as HTMLElement).getByText('Saved').className).toContain(
        'opacity-100',
      );
    } finally {
      vi.useRealTimers();
    }
  });

  it('cancels pending URL autosave when switching devices', async () => {
    const { saveDeviceGrafanaDashboardOverride } = await import('../../api/client');
    const { rerender } = render(<DeviceGrafanaDashboardSection device={mockDevice()} />);

    await waitFor(() => {
      expect(screen.getByPlaceholderText('Optional custom URL override')).toBeInTheDocument();
    });

    vi.useFakeTimers();
    try {
      fireEvent.change(screen.getByPlaceholderText('Optional custom URL override'), {
        target: { value: 'https://grafana.example/d/stale-router' },
      });

      rerender(
        <DeviceGrafanaDashboardSection
          device={mockDevice({ id: 'dev-2', hostname: 'router-02' })}
        />,
      );

      await act(async () => {
        await vi.advanceTimersByTimeAsync(500);
      });

      expect(saveDeviceGrafanaDashboardOverride).not.toHaveBeenCalled();
    } finally {
      vi.useRealTimers();
    }
  });

  it('cancels pending URL autosave when controls become read-only', async () => {
    const { saveDeviceGrafanaDashboardOverride } = await import('../../api/client');
    const { rerender } = render(<DeviceGrafanaDashboardSection device={mockDevice()} />);

    await waitFor(() => {
      expect(screen.getByPlaceholderText('Optional custom URL override')).toBeInTheDocument();
    });

    vi.useFakeTimers();
    try {
      fireEvent.change(screen.getByPlaceholderText('Optional custom URL override'), {
        target: { value: 'https://grafana.example/d/read-only-router' },
      });

      rerender(<DeviceGrafanaDashboardSection device={mockDevice()} readOnly />);

      await act(async () => {
        await vi.advanceTimersByTimeAsync(500);
      });

      expect(saveDeviceGrafanaDashboardOverride).not.toHaveBeenCalled();
    } finally {
      vi.useRealTimers();
    }
  });

  it('ignores an in-flight save after switching devices', async () => {
    const { saveDeviceGrafanaDashboardOverride } = await import('../../api/client');
    const onSettingsChange = vi.fn();
    const deferredSave = createDeferredSave();
    (saveDeviceGrafanaDashboardOverride as ReturnType<typeof vi.fn>).mockReturnValueOnce(
      deferredSave.promise,
    );
    const { rerender } = render(
      <DeviceGrafanaDashboardSection device={mockDevice()} onSettingsChange={onSettingsChange} />,
    );

    await waitFor(() => {
      expect(screen.getByRole('combobox', { name: /dashboard profile/i })).toBeInTheDocument();
    });

    fireEvent.change(screen.getByRole('combobox', { name: /dashboard profile/i }), {
      target: { value: 'grafana-profile-1' },
    });
    expect(saveDeviceGrafanaDashboardOverride).toHaveBeenCalledWith('dev-1', {
      profile_id: 'grafana-profile-1',
      custom_url: '',
    });

    rerender(
      <DeviceGrafanaDashboardSection
        device={mockDevice({ id: 'dev-2', hostname: 'router-02' })}
        onSettingsChange={onSettingsChange}
      />,
    );

    await act(async () => {
      deferredSave.resolve();
    });

    expect(onSettingsChange).not.toHaveBeenCalled();
    const grafanaHeader = screen.getByText('Grafana Dashboard').parentElement;
    expect(grafanaHeader).not.toBeNull();
    expect(within(grafanaHeader as HTMLElement).getByText('Saved').className).toContain(
      'opacity-0',
    );
  });

  it('ignores an in-flight save after controls become read-only', async () => {
    const { saveDeviceGrafanaDashboardOverride } = await import('../../api/client');
    const onSettingsChange = vi.fn();
    const deferredSave = createDeferredSave();
    (saveDeviceGrafanaDashboardOverride as ReturnType<typeof vi.fn>).mockReturnValueOnce(
      deferredSave.promise,
    );
    const { rerender } = render(
      <DeviceGrafanaDashboardSection device={mockDevice()} onSettingsChange={onSettingsChange} />,
    );

    await waitFor(() => {
      expect(screen.getByRole('combobox', { name: /dashboard profile/i })).toBeInTheDocument();
    });

    fireEvent.change(screen.getByRole('combobox', { name: /dashboard profile/i }), {
      target: { value: 'grafana-profile-1' },
    });
    expect(saveDeviceGrafanaDashboardOverride).toHaveBeenCalledWith('dev-1', {
      profile_id: 'grafana-profile-1',
      custom_url: '',
    });

    rerender(
      <DeviceGrafanaDashboardSection
        device={mockDevice()}
        readOnly
        onSettingsChange={onSettingsChange}
      />,
    );

    await act(async () => {
      deferredSave.resolve();
    });

    expect(onSettingsChange).not.toHaveBeenCalled();
    const grafanaHeader = screen.getByText('Grafana Dashboard').parentElement;
    expect(grafanaHeader).not.toBeNull();
    expect(within(grafanaHeader as HTMLElement).getByText('Saved').className).toContain(
      'opacity-0',
    );
  });

  it('ignores an in-flight save after becoming hidden for a virtual device without IP', async () => {
    const { saveDeviceGrafanaDashboardOverride } = await import('../../api/client');
    const onSettingsChange = vi.fn();
    const deferredSave = createDeferredSave();
    (saveDeviceGrafanaDashboardOverride as ReturnType<typeof vi.fn>).mockReturnValueOnce(
      deferredSave.promise,
    );
    const { rerender } = render(
      <DeviceGrafanaDashboardSection device={mockDevice()} onSettingsChange={onSettingsChange} />,
    );

    await waitFor(() => {
      expect(screen.getByRole('combobox', { name: /dashboard profile/i })).toBeInTheDocument();
    });

    fireEvent.change(screen.getByRole('combobox', { name: /dashboard profile/i }), {
      target: { value: 'grafana-profile-1' },
    });
    expect(saveDeviceGrafanaDashboardOverride).toHaveBeenCalledWith('dev-1', {
      profile_id: 'grafana-profile-1',
      custom_url: '',
    });

    rerender(
      <DeviceGrafanaDashboardSection
        device={mockDevice({ device_type: 'virtual', ip: '' })}
        isVirtual
        onSettingsChange={onSettingsChange}
      />,
    );

    await act(async () => {
      deferredSave.resolve();
    });

    expect(onSettingsChange).not.toHaveBeenCalled();
    expect(screen.queryByText('Grafana Dashboard')).not.toBeInTheDocument();
  });

  it('hides virtual devices without IP and disables read-only controls', () => {
    const { rerender } = render(
      <DeviceGrafanaDashboardSection
        device={mockDevice({ device_type: 'virtual', ip: '' })}
        isVirtual
      />,
    );

    expect(screen.queryByText('Grafana Dashboard')).not.toBeInTheDocument();

    rerender(<DeviceGrafanaDashboardSection device={mockDevice()} readOnly />);

    expect(screen.getByRole('combobox', { name: /dashboard profile/i })).toBeDisabled();
    expect(screen.getByPlaceholderText('Optional custom URL override')).toBeDisabled();
  });
});
