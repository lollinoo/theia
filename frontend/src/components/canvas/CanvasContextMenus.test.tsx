/**
 * Exercises canvas context menu behavior so external dashboards open without caller access.
 */
import { act, fireEvent, render, screen } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

import type { Device, Link } from '../../types/api';
import type { LinkEdgeType } from '../LinkEdge';
import { CanvasContextMenus } from './CanvasContextMenus';

function mockDevice(overrides: Partial<Device> = {}): Device {
  return {
    id: 'dev-a',
    hostname: 'router-a',
    ip: '10.0.0.1',
    addresses: [],
    probe_ports: null,
    device_type: 'router',
    poll_class: 'core',
    poll_interval_override: null,
    polling_enabled: true,
    status: 'up',
    sys_name: 'router-a',
    sys_descr: 'RouterOS',
    hardware_model: 'RB4011',
    vendor: 'mikrotik',
    managed: true,
    interfaces: [],
    area_ids: [],
    backup_supported: true,
    metrics_source: 'snmp',
    prometheus_label_name: 'instance',
    prometheus_label_value: '10.0.0.1:9100',
    ...overrides,
  };
}

function mockLink(): Link {
  return {
    id: 'link-a-b',
    source_device_id: 'dev-a',
    source_if_name: 'ether1',
    target_device_id: 'dev-b',
    target_if_name: 'ether2',
    discovery_protocol: 'lldp',
    source_if_speed: 1_000_000_000,
    source_if_oper_status: 'up',
    target_if_speed: 1_000_000_000,
    target_if_oper_status: 'up',
  };
}

describe('CanvasContextMenus Grafana actions', () => {
  let openSpy: ReturnType<typeof vi.spyOn>;

  beforeEach(() => {
    openSpy = vi.spyOn(window, 'open').mockImplementation(() => null);
  });

  afterEach(() => {
    openSpy.mockRestore();
  });

  it('opens a device Grafana dashboard without retaining window.opener and closes the menu', () => {
    const setDeviceMenu = vi.fn();

    render(
      <CanvasContextMenus
        deviceMenu={{ deviceId: 'dev-a', x: 20, y: 30 }}
        edgeMenu={null}
        devices={[mockDevice()]}
        edges={[]}
        bridgeChecked={true}
        bridgeRunning={true}
        deviceWinboxState={{}}
        launchWinbox={vi.fn()}
        grafanaUrl={() => 'https://grafana.example/d/device-a'}
        setDeviceMenu={setDeviceMenu}
        setEdgeMenu={vi.fn()}
        setPanelContent={vi.fn()}
      />,
    );

    act(() => {
      fireEvent.click(screen.getByRole('button', { name: 'Open in Grafana' }));
    });

    expect(openSpy).toHaveBeenCalledWith(
      'https://grafana.example/d/device-a',
      '_blank',
      'noopener,noreferrer',
    );
    expect(setDeviceMenu).toHaveBeenCalledWith(null);
  });

  it('opens an edge Grafana dashboard without retaining window.opener and closes the menu', () => {
    const setEdgeMenu = vi.fn();
    const link = mockLink();
    const edge = {
      id: link.id,
      source: link.source_device_id,
      target: link.target_device_id,
      data: { link },
    } as LinkEdgeType;

    render(
      <CanvasContextMenus
        deviceMenu={null}
        edgeMenu={{ edgeID: link.id, x: 20, y: 30 }}
        devices={[mockDevice(), mockDevice({ id: 'dev-b', hostname: 'router-b' })]}
        edges={[edge]}
        bridgeChecked={true}
        bridgeRunning={true}
        deviceWinboxState={{}}
        launchWinbox={vi.fn()}
        grafanaUrl={(device) =>
          device?.id === 'dev-a' ? 'https://grafana.example/d/device-a' : ''
        }
        setDeviceMenu={vi.fn()}
        setEdgeMenu={setEdgeMenu}
        setPanelContent={vi.fn()}
      />,
    );

    act(() => {
      fireEvent.click(screen.getByRole('button', { name: 'Open in Grafana' }));
    });

    expect(openSpy).toHaveBeenCalledWith(
      'https://grafana.example/d/device-a',
      '_blank',
      'noopener,noreferrer',
    );
    expect(setEdgeMenu).toHaveBeenCalledWith(null);
  });
});
