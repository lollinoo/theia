import { render, screen, waitFor } from '@testing-library/react';
import { describe, expect, it, vi } from 'vitest';

import { fetchDeviceInterfaces } from '../../api/client';
import type { Device, InterfaceInfo, Link } from '../../types/api';
import {
  DeviceInterfaceStatsPanelRoute,
  LinkInterfaceStatsPanelRoute,
} from './InterfaceStatsPanelRoutes';
import { buildRuntimeState } from './runtimeAdapters';

vi.mock('../../api/client', () => ({
  fetchDeviceInterfaces: vi.fn(),
}));

vi.mock('../InterfaceStatsPanel', () => ({
  DeviceInterfaceStatsPanel: (props: { model: { sections: Array<{ ifName: string }> } }) => (
    <div>{props.model.sections.map((section) => section.ifName).join(',') || 'empty'}</div>
  ),
  InterfaceStatsPanel: () => <div>link route</div>,
}));

function deferred<T>() {
  let resolve!: (value: T) => void;
  let reject!: (error: unknown) => void;
  const promise = new Promise<T>((res, rej) => {
    resolve = res;
    reject = rej;
  });
  return { promise, resolve, reject };
}

function mockDevice(overrides: Partial<Device> = {}): Device {
  return {
    id: 'dev-1',
    hostname: 'router-01',
    ip: '10.0.0.1',
    device_type: 'router',
    poll_class: 'standard',
    poll_interval_override: null,
    status: 'up',
    sys_name: 'router-01',
    sys_descr: 'RouterOS',
    hardware_model: 'RB4011',
    vendor: 'mikrotik',
    managed: true,
    interfaces: [],
    area_ids: [],
    backup_supported: true,
    metrics_source: 'prometheus',
    prometheus_label_name: 'instance',
    prometheus_label_value: '10.0.0.1:9100',
    ...overrides,
  };
}

function mockInterface(overrides: Partial<InterfaceInfo> = {}): InterfaceInfo {
  return {
    if_name: 'ether1',
    if_descr: 'Uplink',
    speed: 1_000_000_000,
    oper_status: 'up',
    admin_status: 'up',
    in_use: true,
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

describe('DeviceInterfaceStatsPanelRoute', () => {
  it('clears stale interfaces when switching devices before the next fetch resolves', async () => {
    const dev1 = mockDevice();
    const dev2 = mockDevice({
      id: 'dev-2',
      hostname: 'router-02',
      ip: '10.0.0.2',
      sys_name: 'router-02',
    });
    const runtimeState = buildRuntimeState({
      devices: [dev1, dev2],
      links: [],
      snapshot: null,
      alerts: [],
      prometheusStatus: null,
    });
    const firstFetch = deferred<InterfaceInfo[]>();
    const secondFetch = deferred<InterfaceInfo[]>();
    vi.mocked(fetchDeviceInterfaces)
      .mockReturnValueOnce(firstFetch.promise)
      .mockReturnValueOnce(secondFetch.promise);

    const { rerender } = render(
      <DeviceInterfaceStatsPanelRoute device={dev1} runtimeState={runtimeState} />,
    );

    firstFetch.resolve([mockInterface({ if_name: 'ether1' })]);
    await screen.findByText('ether1');

    rerender(<DeviceInterfaceStatsPanelRoute device={dev2} runtimeState={runtimeState} />);

    expect(screen.queryByText('ether1')).not.toBeInTheDocument();

    await waitFor(() => {
      expect(screen.queryByText('ether1')).not.toBeInTheDocument();
    });

    secondFetch.resolve([mockInterface({ if_name: 'ether9' })]);
    await screen.findByText('ether9');
  });

  it('renders an explicit error state when interface fetch fails', async () => {
    const dev1 = mockDevice();
    const runtimeState = buildRuntimeState({
      devices: [dev1],
      links: [],
      snapshot: null,
      alerts: [],
      prometheusStatus: null,
    });
    vi.mocked(fetchDeviceInterfaces).mockRejectedValueOnce(new Error('boom'));

    render(<DeviceInterfaceStatsPanelRoute device={dev1} runtimeState={runtimeState} />);

    expect(await screen.findByText('Unable to load interface details.')).toBeInTheDocument();
  });

  it('renders an explicit error state for link routes when either endpoint fetch fails', async () => {
    const dev1 = mockDevice();
    const dev2 = mockDevice({
      id: 'dev-2',
      hostname: 'router-02',
      ip: '10.0.0.2',
      sys_name: 'router-02',
    });
    const link = mockLink();
    const runtimeState = buildRuntimeState({
      devices: [dev1, dev2],
      links: [link],
      snapshot: null,
      alerts: [],
      prometheusStatus: null,
    });
    vi.mocked(fetchDeviceInterfaces)
      .mockRejectedValueOnce(new Error('boom'))
      .mockResolvedValueOnce([mockInterface({ if_name: 'ether2' })]);

    render(
      <LinkInterfaceStatsPanelRoute
        link={link}
        sourceDevice={dev1}
        targetDevice={dev2}
        runtimeState={runtimeState}
      />,
    );

    expect(await screen.findByText('Unable to load interface details.')).toBeInTheDocument();
  });

  it('renders a loading placeholder for link routes while endpoint interfaces are still loading', () => {
    const dev1 = mockDevice();
    const dev2 = mockDevice({
      id: 'dev-2',
      hostname: 'router-02',
      ip: '10.0.0.2',
      sys_name: 'router-02',
    });
    const link = mockLink();
    const runtimeState = buildRuntimeState({
      devices: [dev1, dev2],
      links: [link],
      snapshot: null,
      alerts: [],
      prometheusStatus: null,
    });
    const sourceFetch = deferred<InterfaceInfo[]>();
    const targetFetch = deferred<InterfaceInfo[]>();
    vi.mocked(fetchDeviceInterfaces)
      .mockReturnValueOnce(sourceFetch.promise)
      .mockReturnValueOnce(targetFetch.promise);

    render(
      <LinkInterfaceStatsPanelRoute
        link={link}
        sourceDevice={dev1}
        targetDevice={dev2}
        runtimeState={runtimeState}
      />,
    );

    expect(screen.getByText('Loading interface details...')).toBeInTheDocument();
  });
});
