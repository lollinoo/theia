import { act, fireEvent, render, screen, within } from '@testing-library/react';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { ServerError, ValidationError } from '../../api/errors';
import type { Device } from '../../types/api';
import { DevicePollingSection } from './DevicePollingSection';

vi.mock('../../api/client', () => ({
  updateDevice: vi.fn().mockResolvedValue({}),
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

beforeEach(() => {
  vi.clearAllMocks();
});

describe('DevicePollingSection', () => {
  it('renders default cadence context from device poll class', () => {
    render(<DevicePollingSection device={mockDevice()} onDeviceUpdated={vi.fn()} />);

    expect(screen.getByText('Polling Override')).toBeInTheDocument();
    expect(screen.getByText('Default cadence: every 30s (core class)')).toBeInTheDocument();
    expect(screen.getByDisplayValue('Use device default')).toBeInTheDocument();
  });

  it('debounces a custom override save and shows Saved', async () => {
    vi.useFakeTimers();
    try {
      const { updateDevice } = await import('../../api/client');
      const onDeviceUpdated = vi.fn();
      (updateDevice as ReturnType<typeof vi.fn>).mockResolvedValueOnce(
        mockDevice({ poll_interval_override: 123 }),
      );

      render(<DevicePollingSection device={mockDevice()} onDeviceUpdated={onDeviceUpdated} />);

      fireEvent.change(screen.getByDisplayValue('Use device default'), {
        target: { value: 'custom' },
      });
      fireEvent.change(screen.getByPlaceholderText('Seconds (5-3600)'), {
        target: { value: '123' },
      });

      expect(updateDevice).not.toHaveBeenCalled();

      await act(async () => {
        await vi.advanceTimersByTimeAsync(500);
      });

      expect(updateDevice).toHaveBeenCalledWith('dev-1', { poll_interval_override: 123 });
      expect(onDeviceUpdated).toHaveBeenCalledWith(mockDevice({ poll_interval_override: 123 }));

      const pollingHeader = screen.getByText('Polling Override').parentElement;
      expect(pollingHeader).not.toBeNull();
      expect(within(pollingHeader as HTMLElement).getByText('Saved').className).toContain(
        'opacity-100',
      );
    } finally {
      vi.useRealTimers();
    }
  });

  it('blocks invalid custom overrides before updateDevice', async () => {
    const { updateDevice } = await import('../../api/client');

    render(<DevicePollingSection device={mockDevice()} onDeviceUpdated={vi.fn()} />);

    fireEvent.change(screen.getByDisplayValue('Use device default'), {
      target: { value: 'custom' },
    });
    fireEvent.change(screen.getByPlaceholderText('Seconds (5-3600)'), {
      target: { value: '3601' },
    });

    expect(
      screen.getByText('Polling override must be an integer between 5 and 3600 seconds'),
    ).toBeInTheDocument();
    expect(updateDevice).not.toHaveBeenCalled();
  });

  it('cancels pending cadence save when continuous polling is suspended', async () => {
    vi.useFakeTimers();
    try {
      const { updateDevice } = await import('../../api/client');
      (updateDevice as ReturnType<typeof vi.fn>).mockResolvedValue(
        mockDevice({ polling_enabled: false }),
      );

      render(<DevicePollingSection device={mockDevice()} onDeviceUpdated={vi.fn()} />);

      fireEvent.change(screen.getByDisplayValue('Use device default'), { target: { value: '30' } });
      fireEvent.click(screen.getByRole('switch', { name: 'Continuous Polling' }));

      expect(updateDevice).toHaveBeenCalledTimes(1);
      expect(updateDevice).toHaveBeenCalledWith('dev-1', { polling_enabled: false });

      await act(async () => {
        await vi.advanceTimersByTimeAsync(500);
      });

      expect(updateDevice).toHaveBeenCalledTimes(1);
    } finally {
      vi.useRealTimers();
    }
  });

  it('shows typed backend errors on polling saves', async () => {
    vi.useFakeTimers();
    try {
      const { updateDevice } = await import('../../api/client');
      (updateDevice as ReturnType<typeof vi.fn>).mockRejectedValueOnce(
        new ValidationError('Polling override is invalid'),
      );

      render(<DevicePollingSection device={mockDevice()} onDeviceUpdated={vi.fn()} />);

      fireEvent.change(screen.getByDisplayValue('Use device default'), { target: { value: '30' } });

      await act(async () => {
        await vi.advanceTimersByTimeAsync(500);
      });

      expect(screen.getByText('Polling override is invalid')).toBeInTheDocument();

      (updateDevice as ReturnType<typeof vi.fn>).mockRejectedValueOnce(
        new ServerError('internal error, ref: poll001', 'poll001'),
      );

      fireEvent.change(screen.getByDisplayValue('30 seconds'), { target: { value: '60' } });

      await act(async () => {
        await vi.advanceTimersByTimeAsync(500);
      });

      expect(screen.getByText('internal error, ref: poll001')).toBeInTheDocument();
    } finally {
      vi.useRealTimers();
    }
  });
});
