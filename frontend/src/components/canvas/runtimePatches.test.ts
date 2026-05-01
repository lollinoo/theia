import { describe, expect, it, vi } from 'vitest';

import type { Device, Link } from '../../types/api';
import type { DeviceRuntimeDTO, LinkRuntimeDTO, SnapshotPayload } from '../../types/metrics';
import type { DeviceNode } from '../DeviceCard';
import type { LinkEdgeType } from '../LinkEdge';
import { buildEdgeData } from './edgeBuilder';
import { buildRuntimeState } from './runtimeAdapters';
import {
  buildRuntimePatchPlan,
  patchRuntimeDevices,
  patchRuntimeEdges,
  patchRuntimeNodes,
} from './runtimePatches';

function mockDevice(id: string, overrides: Partial<Device> = {}): Device {
  return {
    id,
    hostname: id,
    ip: `10.0.0.${id.slice(-1)}`,
    device_type: 'router',
    poll_class: 'standard',
    poll_interval_override: null,
    status: 'up',
    sys_name: id,
    sys_descr: 'RouterOS',
    hardware_model: 'RB4011',
    vendor: 'mikrotik',
    managed: true,
    interfaces: [],
    area_ids: [],
    backup_supported: true,
    metrics_source: 'prometheus',
    prometheus_label_name: 'instance',
    prometheus_label_value: `10.0.0.${id.slice(-1)}:9100`,
    ...overrides,
  };
}

function mockLink(id: string, sourceDeviceId: string, targetDeviceId: string): Link {
  return {
    id,
    source_device_id: sourceDeviceId,
    source_if_name: 'ether1',
    source_if_mac: '',
    source_if_speed: 1_000_000_000,
    source_if_oper_status: 'up',
    target_device_id: targetDeviceId,
    target_if_name: 'ether2',
    target_if_mac: '',
    target_if_speed: 1_000_000_000,
    target_if_oper_status: 'up',
    discovery_protocol: 'lldp',
  };
}

function mockDeviceRuntime(
  deviceId: string,
  overrides: Partial<DeviceRuntimeDTO> = {},
): DeviceRuntimeDTO {
  return {
    device_id: deviceId,
    operational_status: 'up',
    reachability: 'up',
    health: 'ok',
    freshness: 'fresh',
    primary_health: 'up_fresh',
    primary_reason: 'ok',
    metrics_status: 'available',
    metrics_reason: 'ok',
    alert_status: 'normal',
    firing_alert_count: 0,
    runtime_flags: [],
    last_collected_at: '2026-04-30T10:00:00Z',
    last_polled_at: '2026-04-30T10:00:00Z',
    expected_poll_interval_seconds: 60,
    cpu_percent: 12,
    mem_percent: 34,
    temp_celsius: null,
    uptime_secs: 1234,
    network_reachable: 'true',
    snmp_reachable: 'true',
    ...overrides,
  };
}

function mockLinkRuntime(
  linkId: string,
  sourceDeviceId: string,
  targetDeviceId: string,
  overrides: Partial<LinkRuntimeDTO> = {},
): LinkRuntimeDTO {
  return {
    link_id: linkId,
    source_device_id: sourceDeviceId,
    source_if_name: 'ether1',
    target_device_id: targetDeviceId,
    target_if_name: 'ether2',
    metrics_status: 'available',
    metrics_reason: 'ok',
    bitrate_bps: 1_000_000,
    rx_bps: 500_000,
    tx_bps: 500_000,
    utilization: 0.1,
    updated_at: '2026-04-30T10:00:00Z',
    ...overrides,
  };
}

function snapshot(
  devices: Record<string, DeviceRuntimeDTO>,
  links: Record<string, LinkRuntimeDTO> = {},
): SnapshotPayload {
  return { devices, links };
}

function nodeFor(device: Device): DeviceNode {
  return {
    id: device.id,
    type: 'device',
    position: { x: 0, y: 0 },
    data: {
      kind: 'device',
      device,
      pinned: false,
      highlighted: false,
      metrics: null,
      alertStatus: 'normal',
      isVirtual: false,
      monitoringState: 'monitorable',
    },
  };
}

function edgeFor(link: Link, devicesById: Map<string, Device>): LinkEdgeType {
  return {
    id: link.id,
    source: link.source_device_id,
    target: link.target_device_id,
    type: 'link',
    sourceHandle: 'right',
    targetHandle: 'left',
    data: buildEdgeData(link, devicesById, undefined, vi.fn()),
  };
}

describe('runtime canvas patching', () => {
  it('targets only changed runtime devices and their adjacent edges', () => {
    const links = [mockLink('link-1', 'dev-1', 'dev-2'), mockLink('link-2', 'dev-2', 'dev-3')];
    const previousDeviceRuntime = mockDeviceRuntime('dev-1');
    const previous = snapshot({
      'dev-1': previousDeviceRuntime,
      'dev-2': mockDeviceRuntime('dev-2'),
      'dev-3': mockDeviceRuntime('dev-3'),
    });
    const next = snapshot({
      ...previous.devices,
      'dev-1': mockDeviceRuntime('dev-1', {
        operational_status: 'down',
        health: 'critical',
        primary_health: 'unreachable',
      }),
    });

    const plan = buildRuntimePatchPlan({
      previousSnapshot: previous,
      nextSnapshot: next,
      links,
    });

    expect([...plan.deviceIds]).toEqual(['dev-1']);
    expect([...plan.directLinkIds]).toEqual([]);
    expect([...plan.edgeIds]).toEqual(['link-1']);
  });

  it('targets direct link telemetry changes without touching unrelated nodes', () => {
    const links = [mockLink('link-1', 'dev-1', 'dev-2'), mockLink('link-2', 'dev-2', 'dev-3')];
    const previousLinkRuntime = mockLinkRuntime('link-2', 'dev-2', 'dev-3');
    const previous = snapshot(
      {
        'dev-1': mockDeviceRuntime('dev-1'),
        'dev-2': mockDeviceRuntime('dev-2'),
        'dev-3': mockDeviceRuntime('dev-3'),
      },
      {
        'link-2': previousLinkRuntime,
      },
    );
    const next = snapshot(previous.devices, {
      ...previous.links,
      'link-2': mockLinkRuntime('link-2', 'dev-2', 'dev-3', { utilization: 0.82 }),
    });

    const plan = buildRuntimePatchPlan({
      previousSnapshot: previous,
      nextSnapshot: next,
      links,
    });

    expect([...plan.deviceIds]).toEqual([]);
    expect([...plan.directLinkIds]).toEqual(['link-2']);
    expect([...plan.edgeIds]).toEqual(['link-2']);
  });

  it('does not target cloned runtime records when their values are unchanged', () => {
    const links = [mockLink('link-1', 'dev-1', 'dev-2')];
    const previous = snapshot(
      {
        'dev-1': mockDeviceRuntime('dev-1'),
        'dev-2': mockDeviceRuntime('dev-2'),
      },
      {
        'link-1': mockLinkRuntime('link-1', 'dev-1', 'dev-2'),
      },
    );
    const next = snapshot(
      {
        'dev-1': { ...previous.devices['dev-1'], runtime_flags: [] },
        'dev-2': { ...previous.devices['dev-2'] },
      },
      {
        'link-1': { ...previous.links['link-1'] },
      },
    );

    const plan = buildRuntimePatchPlan({
      previousSnapshot: previous,
      nextSnapshot: next,
      links,
    });

    expect([...plan.deviceIds]).toEqual([]);
    expect([...plan.directLinkIds]).toEqual([]);
    expect([...plan.edgeIds]).toEqual([]);
  });

  it('patches only impacted node data and runtime-aware device records', () => {
    const devices = [mockDevice('dev-1'), mockDevice('dev-2')];
    const nodes = devices.map(nodeFor);
    const runtimeState = buildRuntimeState({
      devices,
      links: [],
      snapshot: snapshot({
        'dev-1': mockDeviceRuntime('dev-1', {
          operational_status: 'down',
          health: 'critical',
          cpu_percent: 91,
        }),
        'dev-2': mockDeviceRuntime('dev-2'),
      }),
      alerts: [],
      prometheusStatus: null,
    });
    const plan = {
      deviceIds: new Set(['dev-1']),
      directLinkIds: new Set<string>(),
      edgeIds: new Set<string>(),
    };

    const patchedNodes = patchRuntimeNodes({ nodes, runtimeState, plan });
    const patchedDevices = patchRuntimeDevices({ devices, runtimeState, plan });

    expect(patchedNodes).not.toBe(nodes);
    expect(patchedNodes[0]).not.toBe(nodes[0]);
    expect(patchedNodes[1]).toBe(nodes[1]);
    expect(patchedNodes[0].data.device.status).toBe('down');
    expect(patchedNodes[0].data.metrics?.cpu_percent).toBe(91);
    expect(patchedDevices[0]).not.toBe(devices[0]);
    expect(patchedDevices[0].status).toBe('down');
    expect(patchedDevices[1]).toBe(devices[1]);
  });

  it('keeps node references stable when an included runtime patch does not alter render data', () => {
    const devices = [mockDevice('dev-1')];
    const runtimeState = buildRuntimeState({
      devices,
      links: [],
      snapshot: snapshot({
        'dev-1': mockDeviceRuntime('dev-1'),
      }),
      alerts: [],
      prometheusStatus: null,
    });
    const runtimeDevice = runtimeState.devicesById.get('dev-1')!;
    const node = nodeFor(runtimeDevice.device);
    node.data.metrics = runtimeDevice.metrics;
    node.data.alertStatus = runtimeDevice.alertStatus;
    node.data.monitoringState = runtimeDevice.monitoringState;
    node.data.isVirtual = false;
    node.data.subtype = undefined;
    const nodes = [node];
    const plan = {
      deviceIds: new Set(['dev-1']),
      directLinkIds: new Set<string>(),
      edgeIds: new Set<string>(),
    };

    const patchedNodes = patchRuntimeNodes({ nodes, runtimeState, plan });

    expect(patchedNodes).toBe(nodes);
    expect(patchedNodes[0]).toBe(node);
  });

  it('keeps node references stable when only non-rendered runtime fields change', () => {
    const devices = [mockDevice('dev-1')];
    const baseRuntime = mockDeviceRuntime('dev-1', {
      temp_celsius: 45,
      last_polled_at: '2026-04-30T10:00:00Z',
    });
    const currentRuntimeState = buildRuntimeState({
      devices,
      links: [],
      snapshot: snapshot({ 'dev-1': baseRuntime }),
      alerts: [],
      prometheusStatus: null,
    });
    const currentRuntimeDevice = currentRuntimeState.devicesById.get('dev-1')!;
    const node = nodeFor(currentRuntimeDevice.device);
    node.data.metrics = currentRuntimeDevice.metrics;
    node.data.alertStatus = currentRuntimeDevice.alertStatus;
    node.data.monitoringState = currentRuntimeDevice.monitoringState;
    node.data.isVirtual = false;

    const nextRuntimeState = buildRuntimeState({
      devices,
      links: [],
      snapshot: snapshot({
        'dev-1': {
          ...baseRuntime,
          temp_celsius: 51,
          last_polled_at: '2026-04-30T10:01:00Z',
        },
      }),
      alerts: [],
      prometheusStatus: null,
    });
    const plan = {
      deviceIds: new Set(['dev-1']),
      directLinkIds: new Set<string>(),
      edgeIds: new Set<string>(),
    };

    const nodes = [node];
    const patchedNodes = patchRuntimeNodes({ nodes, runtimeState: nextRuntimeState, plan });

    expect(patchedNodes).toBe(nodes);
    expect(patchedNodes[0]).toBe(node);
  });

  it('patches only impacted edge data with link and endpoint runtime', () => {
    const devices = [mockDevice('dev-1'), mockDevice('dev-2'), mockDevice('dev-3')];
    const links = [mockLink('link-1', 'dev-1', 'dev-2'), mockLink('link-2', 'dev-2', 'dev-3')];
    const devicesById = new Map(devices.map((device) => [device.id, device]));
    const edges = links.map((link) => edgeFor(link, devicesById));
    const runtimeState = buildRuntimeState({
      devices,
      links,
      snapshot: snapshot(
        {
          'dev-1': mockDeviceRuntime('dev-1', {
            operational_status: 'down',
            health: 'critical',
            primary_health: 'unreachable',
          }),
          'dev-2': mockDeviceRuntime('dev-2'),
          'dev-3': mockDeviceRuntime('dev-3'),
        },
        {
          'link-1': mockLinkRuntime('link-1', 'dev-1', 'dev-2', {
            utilization: 0.77,
          }),
        },
      ),
      alerts: [],
      prometheusStatus: null,
    });
    const plan = {
      deviceIds: new Set(['dev-1']),
      directLinkIds: new Set<string>(),
      edgeIds: new Set(['link-1']),
    };

    const patchedEdges = patchRuntimeEdges({
      edges,
      links,
      runtimeState,
      alerts: [],
      onEdgeContextMenu: vi.fn(),
      plan,
    });

    expect(patchedEdges).not.toBe(edges);
    expect(patchedEdges[0]).not.toBe(edges[0]);
    expect(patchedEdges[1]).toBe(edges[1]);
    expect(patchedEdges[0].data?.sourceDeviceStatus).toBe('down');
    expect(patchedEdges[0].data?.sourceDeviceHealth).toBe('critical');
    expect(patchedEdges[0].data?.utilization).toBe(0.77);
  });
});
