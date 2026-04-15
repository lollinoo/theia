import { render, screen } from '@testing-library/react';
import { describe, expect, it, vi } from 'vitest';

import { LinkDetailsPanel } from './LinkDetailsPanel';
import type { Device, Link } from '../types/api';

vi.mock('../api/client', () => ({
  fetchDeviceInterfaces: vi.fn(),
  updateLink: vi.fn(),
  deleteLink: vi.fn(),
}));

function mockDevice(overrides: Partial<Device> = {}): Device {
  return {
    id: 'dev-1',
    hostname: 'router-01',
    ip: '10.0.0.1',
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

function mockLink(overrides: Partial<Link> = {}): Link {
  return {
    id: 'link-1',
    source_device_id: 'dev-1',
    source_if_name: 'ether1',
    target_device_id: 'dev-2',
    target_if_name: 'ether2',
    discovery_protocol: 'lldp',
    source_if_speed: 1_000_000_000,
    source_if_oper_status: 'up',
    target_if_speed: 100_000_000,
    target_if_oper_status: 'down',
    ...overrides,
  };
}

describe('LinkDetailsPanel', () => {
  const devices = [
    mockDevice(),
    mockDevice({
      id: 'dev-2',
      hostname: 'router-02',
      ip: '10.0.0.2',
      sys_name: 'router-02',
    }),
  ];

  it('renders a read-only details view without edit or delete actions', () => {
    render(
      <LinkDetailsPanel
        link={mockLink()}
        devices={devices}
        readOnly
        onUpdated={vi.fn()}
        onDeleted={vi.fn()}
        onClose={vi.fn()}
      />,
    );

    expect(screen.getByText('Source Endpoint')).toBeInTheDocument();
    expect(screen.getByText('Target Endpoint')).toBeInTheDocument();
    expect(screen.getByText('Enter edit mode to change ports or delete this link.')).toBeInTheDocument();
    expect(screen.queryByRole('button', { name: 'Edit Ports' })).not.toBeInTheDocument();
    expect(screen.queryByRole('button', { name: 'Delete Link' })).not.toBeInTheDocument();
  });

  it('keeps edit actions available in editable mode', () => {
    render(
      <LinkDetailsPanel
        link={mockLink()}
        devices={devices}
        onUpdated={vi.fn()}
        onDeleted={vi.fn()}
        onClose={vi.fn()}
      />,
    );

    expect(screen.getByRole('button', { name: 'Edit Ports' })).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Delete Link' })).toBeInTheDocument();
  });
});
