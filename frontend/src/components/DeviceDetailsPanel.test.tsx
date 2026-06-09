/**
 * Exercises device details panel component behavior so refactors preserve the documented contract.
 */
import { act, fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import { describe, expect, it, vi } from 'vitest';

import type { Device } from '../types/api';
import type { DeviceMetricsDTO } from '../types/metrics';
import { DeviceDetailsPanel } from './DeviceDetailsPanel';

type AddressReachabilityResults = Awaited<
  ReturnType<
    NonNullable<React.ComponentProps<typeof DeviceDetailsPanel>['onCheckAddressReachability']>
  >
>;

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
    area_ids: [],
    addresses: [
      {
        id: 'addr-1',
        device_id: 'dev-1',
        address: '10.0.0.1',
        label: 'Primary',
        role: 'primary',
        is_primary: true,
        priority: 0,
        probe_ports: [22],
      },
    ],
    ...overrides,
  };
}

function mockDeviceMetrics(overrides: Partial<DeviceMetricsDTO> = {}): DeviceMetricsDTO {
  return {
    device_id: 'dev-1',
    operational_status: 'up',
    primary_health: 'up_fresh',
    runtime_flags: [],
    field_states: { uptime: 'ok', cpu: 'ok', memory: 'ok' },
    network_reachable: 'true',
    snmp_reachable: 'true',
    reachability: 'up',
    health: 'healthy',
    freshness: 'fresh',
    primary_reason: 'ok',
    metrics_status: 'available',
    metrics_reason: 'ok',
    alert_status: 'normal',
    firing_alert_count: 0,
    last_collected_at: '2026-04-25T10:00:00Z',
    last_polled_at: '2026-04-25T10:00:00Z',
    expected_poll_interval_seconds: 30,
    cpu_percent: 12,
    mem_percent: 34,
    temp_celsius: 45,
    uptime_secs: 3660,
    ...overrides,
  };
}

describe('DeviceDetailsPanel', () => {
  it('renders read-only live telemetry for the selected device', () => {
    render(<DeviceDetailsPanel device={mockDevice()} detailMetrics={mockDeviceMetrics()} />);

    expect(screen.getByText('Live Detail Telemetry')).toBeInTheDocument();
    expect(screen.getByText('router-01')).toBeInTheDocument();
    expect(screen.getAllByText('10.0.0.1').length).toBeGreaterThan(0);
    expect(screen.getByText('Operational status')).toBeInTheDocument();
    expect(screen.getAllByText('up').length).toBeGreaterThan(0);
    expect(screen.getByText('Network reachable')).toBeInTheDocument();
    expect(screen.getByText('SNMP reachable')).toBeInTheDocument();
    expect(screen.getByText('30s')).toBeInTheDocument();
    expect(screen.getByText('1h 1m')).toBeInTheDocument();
  });

  it('renders saved device notes for read-only viewing', () => {
    render(
      <DeviceDetailsPanel
        device={mockDevice({ notes: 'Check transceiver levels weekly' })}
        detailMetrics={mockDeviceMetrics()}
      />,
    );

    expect(screen.getByText('Device Notes')).toBeInTheDocument();
    expect(screen.getByText('Check transceiver levels weekly')).toBeInTheDocument();
  });

  it('shows probing state when address reachability is in progress', () => {
    render(
      <DeviceDetailsPanel
        device={mockDevice()}
        detailMetrics={mockDeviceMetrics()}
        onCheckAddressReachability={vi.fn()}
        addressReachabilityState={{
          results: [],
          loading: true,
          error: null,
        }}
      />,
    );

    expect(screen.getByText('Checking address reachability')).toBeInTheDocument();
    expect(screen.getByText('probing')).toBeInTheDocument();
  });

  it('formats memory and last poll values for operators', () => {
    render(
      <DeviceDetailsPanel
        device={mockDevice()}
        detailMetrics={mockDeviceMetrics({
          mem_percent: 46.25244140625,
          last_polled_at: '2026-04-25T20:05:57Z',
        })}
      />,
    );

    expect(screen.getByText('46.25%')).toBeInTheDocument();
    expect(screen.getByText('Apr 25, 2026, 20:05:57 UTC')).toBeInTheDocument();
    expect(screen.queryByText('2026-04-25T20:05:57Z')).not.toBeInTheDocument();
  });

  it('shows metric health when threshold warnings exist without active alerts', () => {
    render(
      <DeviceDetailsPanel
        device={mockDevice()}
        detailMetrics={mockDeviceMetrics({
          primary_health: 'up_fresh',
          health: 'warning',
          temp_celsius: 70,
          firing_alert_count: 0,
        })}
      />,
    );

    expect(screen.getByText('Metric health')).toBeInTheDocument();
    expect(screen.getByText('warning')).toBeInTheDocument();
    expect(screen.getByText('70 C')).toBeInTheDocument();
    expect(screen.getByText('Active alerts')).toBeInTheDocument();
    expect(screen.getByText('0')).toBeInTheDocument();
  });

  it('groups interface statistics in a collapsible disclosure', () => {
    render(
      <DeviceDetailsPanel
        device={mockDevice()}
        detailMetrics={mockDeviceMetrics()}
        interfaceStats={<div>Interface statistics content</div>}
      />,
    );

    const toggle = screen.getByRole('button', { name: 'Interfaces Show' });

    expect(screen.queryByLabelText('Interface visibility')).not.toBeInTheDocument();
    expect(toggle).toHaveAttribute('aria-expanded', 'false');
    expect(screen.queryByText('Interface statistics content')).not.toBeInTheDocument();
    expect(screen.queryByText('chevron_right')).not.toBeInTheDocument();

    fireEvent.click(toggle);

    expect(screen.getByRole('button', { name: 'Interfaces Hide' })).toHaveAttribute(
      'aria-expanded',
      'true',
    );
    expect(screen.getByText('Interface statistics content')).toBeInTheDocument();
  });

  it('shows an empty telemetry state without exposing edit actions', () => {
    render(<DeviceDetailsPanel device={mockDevice()} detailMetrics={null} />);

    expect(screen.getByText('No live telemetry available for this device.')).toBeInTheDocument();
    expect(screen.queryByText('Save Changes')).not.toBeInTheDocument();
    expect(screen.queryByText('Delete Device')).not.toBeInTheDocument();
  });

  it('renders device addresses and checks reachability', async () => {
    const onCheckAddressReachability = vi.fn().mockResolvedValue([
      {
        address_id: 'addr-1',
        address: '10.0.0.1',
        role: 'primary',
        label: 'Primary',
        is_primary: true,
        probe_ports: [22],
        reachable_ports: [{ port: 22, reachable: true, error: '' }],
        reachable: true,
        error: '',
      },
      {
        address_id: 'addr-2',
        address: '198.51.100.10',
        role: 'backup',
        label: 'Backup',
        is_primary: false,
        probe_ports: [2222],
        reachable_ports: [{ port: 2222, reachable: false, error: 'connection refused' }],
        reachable: false,
        error: 'connection refused',
      },
    ]);

    render(
      <DeviceDetailsPanel
        device={mockDevice({
          addresses: [
            {
              id: 'addr-1',
              device_id: 'dev-1',
              address: '10.0.0.1',
              label: 'Primary',
              role: 'primary',
              is_primary: true,
              priority: 0,
              probe_ports: [22],
            },
            {
              id: 'addr-2',
              device_id: 'dev-1',
              address: '198.51.100.10',
              label: 'Backup',
              role: 'backup',
              is_primary: false,
              priority: 1,
              probe_ports: [2222],
            },
          ],
        })}
        detailMetrics={mockDeviceMetrics()}
        onCheckAddressReachability={onCheckAddressReachability}
      />,
    );

    expect(screen.getByText('Addresses')).toBeInTheDocument();

    await act(async () => {
      fireEvent.click(screen.getByRole('button', { name: 'Check address reachability' }));
    });

    await waitFor(() => {
      expect(screen.getByText('reachable')).toBeInTheDocument();
      expect(screen.getByText('unreachable')).toBeInTheDocument();
    });
    const port22Row = screen.getByText('Port 22').closest('div');
    const port2222Row = screen.getByText('Port 2222').closest('div');
    expect(port22Row).not.toBeNull();
    expect(port2222Row).not.toBeNull();
    expect(within(port22Row!).getByText('up')).toBeInTheDocument();
    expect(within(port2222Row!).getByText('down')).toBeInTheDocument();
    expect(screen.getByText('connection refused')).toBeInTheDocument();
    expect(onCheckAddressReachability).toHaveBeenCalledWith('dev-1');
  });

  it('promotes an address to primary', async () => {
    const onPromoteAddress = vi.fn().mockResolvedValue(undefined);

    render(
      <DeviceDetailsPanel
        device={mockDevice({
          addresses: [
            {
              id: 'addr-1',
              device_id: 'dev-1',
              address: '10.0.0.1',
              label: 'Primary',
              role: 'primary',
              is_primary: true,
              priority: 0,
              probe_ports: [22],
            },
            {
              id: 'addr-2',
              device_id: 'dev-1',
              address: '198.51.100.10',
              label: 'Backup',
              role: 'backup',
              is_primary: false,
              priority: 1,
              probe_ports: [2222],
            },
          ],
        })}
        detailMetrics={mockDeviceMetrics()}
        onPromoteAddress={onPromoteAddress}
      />,
    );

    await act(async () => {
      fireEvent.click(screen.getByRole('button', { name: 'Use 198.51.100.10 as primary' }));
    });

    expect(onPromoteAddress).toHaveBeenCalledWith('addr-2');
  });

  it('shows address action failures without leaving unhandled promise rejections', async () => {
    const onCheckAddressReachability = vi.fn().mockRejectedValue(new Error('probe failed'));
    const onPromoteAddress = vi.fn().mockRejectedValue(new Error('promotion failed'));

    render(
      <DeviceDetailsPanel
        device={mockDevice({
          addresses: [
            {
              id: 'addr-1',
              device_id: 'dev-1',
              address: '10.0.0.1',
              label: 'Primary',
              role: 'primary',
              is_primary: true,
              priority: 0,
              probe_ports: [22],
            },
            {
              id: 'addr-2',
              device_id: 'dev-1',
              address: '198.51.100.10',
              label: 'Backup',
              role: 'backup',
              is_primary: false,
              priority: 1,
              probe_ports: [2222],
            },
          ],
        })}
        detailMetrics={mockDeviceMetrics()}
        onCheckAddressReachability={onCheckAddressReachability}
        onPromoteAddress={onPromoteAddress}
      />,
    );

    await act(async () => {
      fireEvent.click(screen.getByRole('button', { name: 'Check address reachability' }));
    });

    expect(screen.getByText('probe failed')).toBeInTheDocument();

    await act(async () => {
      fireEvent.click(screen.getByRole('button', { name: 'Use 198.51.100.10 as primary' }));
    });

    expect(screen.getByText('promotion failed')).toBeInTheDocument();
  });

  it('shows promotion failures while address reachability state is controlled', async () => {
    const onPromoteAddress = vi.fn().mockRejectedValue(new Error('promotion failed'));

    render(
      <DeviceDetailsPanel
        device={mockDevice({
          addresses: [
            {
              id: 'addr-1',
              device_id: 'dev-1',
              address: '10.0.0.1',
              label: 'Primary',
              role: 'primary',
              is_primary: true,
              priority: 0,
              probe_ports: [22],
            },
            {
              id: 'addr-2',
              device_id: 'dev-1',
              address: '198.51.100.10',
              label: 'Backup',
              role: 'backup',
              is_primary: false,
              priority: 1,
              probe_ports: [2222],
            },
          ],
        })}
        detailMetrics={mockDeviceMetrics()}
        addressReachabilityState={{ results: [], loading: false, error: null }}
        onPromoteAddress={onPromoteAddress}
      />,
    );

    await act(async () => {
      fireEvent.click(screen.getByRole('button', { name: 'Use 198.51.100.10 as primary' }));
    });

    expect(screen.getByText('promotion failed')).toBeInTheDocument();
  });

  it('clears stale address reachability when the selected device changes', async () => {
    let resolveProbe: (results: AddressReachabilityResults) => void = () => {};
    const onCheckAddressReachability = vi.fn(
      () =>
        new Promise<AddressReachabilityResults>((resolve) => {
          resolveProbe = resolve;
        }),
    );

    const { rerender } = render(
      <DeviceDetailsPanel
        device={mockDevice()}
        detailMetrics={mockDeviceMetrics()}
        onCheckAddressReachability={onCheckAddressReachability}
      />,
    );

    fireEvent.click(screen.getByRole('button', { name: 'Check address reachability' }));

    rerender(
      <DeviceDetailsPanel
        device={mockDevice({ id: 'dev-2', hostname: 'core-2' })}
        detailMetrics={mockDeviceMetrics()}
        onCheckAddressReachability={onCheckAddressReachability}
      />,
    );

    await act(async () => {
      resolveProbe([
        {
          address_id: 'addr-1',
          address: '10.0.0.1',
          role: 'primary',
          label: 'Primary',
          is_primary: true,
          probe_ports: [22],
          reachable_ports: [{ port: 22, reachable: true, error: '' }],
          reachable: true,
          error: '',
        },
      ]);
    });

    expect(screen.queryByText('reachable')).not.toBeInTheDocument();
    expect(screen.getByText('not checked')).toBeInTheDocument();
  });

  it('ignores stale promotion failures when the selected device changes', async () => {
    let rejectPromotion: (error: Error) => void = () => {};
    const onPromoteAddress = vi.fn(
      () =>
        new Promise<void>((_resolve, reject) => {
          rejectPromotion = reject;
        }),
    );

    const { rerender } = render(
      <DeviceDetailsPanel
        device={mockDevice({
          addresses: [
            {
              id: 'addr-1',
              device_id: 'dev-1',
              address: '10.0.0.1',
              label: 'Primary',
              role: 'primary',
              is_primary: true,
              priority: 0,
              probe_ports: [22],
            },
            {
              id: 'addr-2',
              device_id: 'dev-1',
              address: '198.51.100.10',
              label: 'Backup',
              role: 'backup',
              is_primary: false,
              priority: 1,
              probe_ports: [2222],
            },
          ],
        })}
        detailMetrics={mockDeviceMetrics()}
        onPromoteAddress={onPromoteAddress}
      />,
    );

    fireEvent.click(screen.getByRole('button', { name: 'Use 198.51.100.10 as primary' }));

    rerender(
      <DeviceDetailsPanel
        device={mockDevice({ id: 'dev-2', hostname: 'core-2' })}
        detailMetrics={mockDeviceMetrics()}
        onPromoteAddress={onPromoteAddress}
      />,
    );

    await act(async () => {
      rejectPromotion(new Error('stale promotion failed'));
    });

    expect(screen.queryByText('stale promotion failed')).not.toBeInTheDocument();
  });
});
