import { describe, it, expect, vi } from 'vitest';
import { fireEvent, render, screen } from '@testing-library/react';
import { CanvasPanels } from './CanvasPanels';
import type { Device, Link } from '../../types/api';

vi.mock('../DeviceConfigPanel', () => ({
  DeviceConfigPanel: (props: { onWinBoxAvailabilityChange?: (hasWinboxProfile: boolean) => void }) => (
    <button type="button" onClick={() => props.onWinBoxAvailabilityChange?.(true)}>
      Notify WinBox
    </button>
  ),
}));

vi.mock('../LinkDetailsPanel', () => ({
  LinkDetailsPanel: (props: { readOnly?: boolean; link: { target_if_name: string } }) => (
    <div>
      {props.readOnly ? 'Link Details Read Only' : 'Link Details Editable'}:{props.link.target_if_name}
    </div>
  ),
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
    target_if_speed: 1_000_000_000,
    target_if_oper_status: 'up',
    ...overrides,
  };
}

describe('CanvasPanels', () => {
  it('forwards WinBox availability updates for the open device config panel', () => {
    const onWinBoxAvailabilityChange = vi.fn();
    const device = mockDevice();

    render(
      <CanvasPanels
        panelContent={{ type: 'deviceConfig', data: { device } }}
        setPanelContent={vi.fn()}
        snapshot={null}
        devices={[device]}
        topologyLinks={[]}
        loadTopology={vi.fn().mockResolvedValue(undefined)}
        setDevices={vi.fn()}
        setNodes={vi.fn()}
        reactFlow={{} as never}
        prometheusStatus={null}
        onWinBoxAvailabilityChange={onWinBoxAvailabilityChange}
      />,
    );

    fireEvent.click(screen.getByRole('button', { name: 'Notify WinBox' }));

    expect(onWinBoxAvailabilityChange).toHaveBeenCalledWith(device.id, true);
  });

  it('renders link details in read-only mode when requested by panel content', () => {
    const sourceDevice = mockDevice();
    const targetDevice = mockDevice({
      id: 'dev-2',
      hostname: 'router-02',
      ip: '10.0.0.2',
      sys_name: 'router-02',
    });

    render(
      <CanvasPanels
        panelContent={{ type: 'link-details', data: { link: mockLink(), readOnly: true } }}
        setPanelContent={vi.fn()}
        snapshot={null}
        devices={[sourceDevice, targetDevice]}
        topologyLinks={[mockLink()]}
        loadTopology={vi.fn().mockResolvedValue(undefined)}
        setDevices={vi.fn()}
        setNodes={vi.fn()}
        reactFlow={{} as never}
        prometheusStatus={null}
      />,
    );

    expect(screen.getByText('Link Details Read Only:ether2')).toBeInTheDocument();
  });

  it('prefers the live topology link over stale panel data for link details', () => {
    const sourceDevice = mockDevice();
    const targetDevice = mockDevice({
      id: 'dev-2',
      hostname: 'router-02',
      ip: '10.0.0.2',
      sys_name: 'router-02',
    });

    render(
      <CanvasPanels
        panelContent={{ type: 'link-details', data: { link: mockLink({ target_if_name: '' }), readOnly: true } }}
        setPanelContent={vi.fn()}
        snapshot={null}
        devices={[sourceDevice, targetDevice]}
        topologyLinks={[mockLink({ target_if_name: 'ether2' })]}
        loadTopology={vi.fn().mockResolvedValue(undefined)}
        setDevices={vi.fn()}
        setNodes={vi.fn()}
        reactFlow={{} as never}
        prometheusStatus={null}
      />,
    );

    expect(screen.getByText('Link Details Read Only:ether2')).toBeInTheDocument();
  });
});
