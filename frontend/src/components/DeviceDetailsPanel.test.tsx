import { fireEvent, render, screen } from '@testing-library/react';
import { describe, expect, it } from 'vitest';

import type { Device } from '../types/api';
import type { DeviceMetricsDTO } from '../types/metrics';
import { DeviceDetailsPanel } from './DeviceDetailsPanel';

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
    expect(screen.getByText('10.0.0.1')).toBeInTheDocument();
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
});
