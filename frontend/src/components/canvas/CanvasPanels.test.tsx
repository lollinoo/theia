import { describe, it, expect, vi } from 'vitest';
import { fireEvent, render, screen } from '@testing-library/react';
import { CanvasPanels } from './CanvasPanels';
import type { Device } from '../../types/api';

vi.mock('../DeviceConfigPanel', () => ({
  DeviceConfigPanel: (props: { onWinBoxAvailabilityChange?: (hasWinboxProfile: boolean) => void }) => (
    <button type="button" onClick={() => props.onWinBoxAvailabilityChange?.(true)}>
      Notify WinBox
    </button>
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
});
