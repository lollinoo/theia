/**
 * Exercises topology composer topology canvas behavior so refactors preserve the documented contract.
 */
import type { SnapGrid } from '@xyflow/react';
import { describe, expect, it, vi } from 'vitest';

import type { Device, Link } from '../../types/api';
import type { AlertDTO, SnapshotPayload } from '../../types/metrics';
import { buildRuntimeState } from './runtimeAdapters';
import { composeCanvasTopology } from './topologyComposer';

/**
 * Builds a device fixture with complete API defaults and per-test overrides.
 */
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

/**
 * Builds a link fixture with complete API defaults and per-test overrides.
 */
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

/**
 * Builds a runtime snapshot fixture with device and link telemetry defaults.
 */
function mockSnapshot(overrides: Partial<SnapshotPayload> = {}): SnapshotPayload {
  return {
    devices: {
      'dev-1': {
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
        last_collected_at: '2026-04-20T12:00:00Z',
        last_polled_at: '2026-04-20T12:00:00Z',
        expected_poll_interval_seconds: 60,
        cpu_percent: 12,
        mem_percent: 44,
        temp_celsius: null,
        uptime_secs: 900,
      },
      'dev-2': {
        device_id: 'dev-2',
        operational_status: 'up',
        primary_health: 'up_fresh',
        runtime_flags: [],
        field_states: { uptime: 'ok', cpu: 'ok', memory: 'ok' },
        network_reachable: 'true',
        snmp_reachable: 'true',
        reachability: 'up',
        health: 'warning',
        freshness: 'fresh',
        primary_reason: 'ok',
        metrics_status: 'available',
        metrics_reason: 'ok',
        alert_status: 'degraded',
        firing_alert_count: 1,
        last_collected_at: '2026-04-20T12:00:00Z',
        last_polled_at: '2026-04-20T12:00:00Z',
        expected_poll_interval_seconds: 60,
        cpu_percent: 18,
        mem_percent: 50,
        temp_celsius: null,
        uptime_secs: 800,
      },
    },
    links: {
      'link-1': {
        link_id: 'link-1',
        source_device_id: 'dev-1',
        target_device_id: 'dev-2',
        source_if_name: 'ether1',
        target_if_name: 'ether2',
        metrics_status: 'available',
        metrics_reason: 'ok',
        last_collected_at: '2026-04-20T12:00:00Z',
        tx_bps: 1200,
        rx_bps: 2400,
        utilization: 0.15,
      },
    },
    ...overrides,
  };
}

/**
 * Composes the topology under test with representative devices, links,
 * positions, and optional runtime inputs.
 */
function buildSubject(options: {
  snapshot?: SnapshotPayload | null;
  alerts?: AlertDTO[];
  snapGrid?: SnapGrid | null;
}) {
  const devices = [
    mockDevice(),
    mockDevice({
      id: 'dev-2',
      hostname: 'switch-01',
      ip: '10.0.0.2',
      sys_name: 'switch-01',
    }),
  ];
  const links = [mockLink()];
  const runtimeState = buildRuntimeState({
    devices,
    links,
    snapshot: options.snapshot ?? mockSnapshot(),
    alerts: options.alerts ?? [],
    prometheusStatus: null,
  });

  return composeCanvasTopology({
    devices,
    links,
    runtimeState,
    savedPositions: new Map(),
    computedPositions: new Map([
      ['dev-1', { x: 100, y: 120 }],
      ['dev-2', { x: 320, y: 120 }],
    ]),
    currentPositions: new Map(),
    explicitPositions: new Map(),
    editMode: false,
    openDeviceMenu: vi.fn(),
    openEdgeMenu: vi.fn(),
    placementDeviceIds: new Set(['dev-1', 'dev-2']),
    alerts: options.alerts ?? [],
    snapGrid: options.snapGrid ?? null,
  });
}

describe('composeCanvasTopology', () => {
  it('projects a saved self-link route while retaining its node annotation shortcut', () => {
    const device = mockDevice();
    const selfLink = mockLink({
      id: 'self-link-1',
      source_device_id: device.id,
      source_if_name: 'ether1',
      target_device_id: device.id,
      target_if_name: 'ether2',
    });
    const route = { version: 1 as const, waypoints: [{ x: 260, y: 80 }] };
    const owner = { mapId: 'map-a', generation: 1 } as const;
    const routeEditToken = { owner, actionEpoch: 0 } as const;
    const onLinkRouteCommit = vi.fn();
    const runtimeState = buildRuntimeState({
      devices: [device],
      links: [selfLink],
      snapshot: null,
      alerts: [],
      prometheusStatus: null,
    });

    const { nodes, edges } = composeCanvasTopology({
      devices: [device],
      links: [selfLink],
      linkRoutes: { [selfLink.id]: route },
      onLinkRouteCommit,
      getLinkRouteEditToken: () => routeEditToken,
      runtimeState,
      savedPositions: new Map(),
      computedPositions: new Map([[device.id, { x: 100, y: 120 }]]),
      currentPositions: new Map(),
      explicitPositions: new Map(),
      editMode: true,
      openDeviceMenu: vi.fn(),
      openEdgeMenu: vi.fn(),
      openSelfLinkDetails: vi.fn(),
      placementDeviceIds: new Set([device.id]),
      alerts: [],
      snapGrid: null,
    });

    expect(edges).toHaveLength(1);
    expect(edges[0]).toMatchObject({
      id: selfLink.id,
      source: device.id,
      target: device.id,
      data: {
        route,
        routeEditable: true,
        routeEditToken,
        onRouteCommit: onLinkRouteCommit,
      },
    });
    expect(edges[0]?.data.route).not.toBe(route);
    expect(nodes[0]?.data.selfLinks?.map((link) => link.id)).toEqual([selfLink.id]);
  });

  it('threads the enabled grid through topology node composition', () => {
    const { nodes } = buildSubject({ snapGrid: [30, 30] });

    expect(nodes.map((node) => node.position)).toEqual([
      { x: 90, y: 120 },
      { x: 330, y: 120 },
    ]);
  });

  it('hydrates node runtime status from runtime models without mutating static device status', () => {
    const { nodes } = buildSubject({
      snapshot: mockSnapshot({
        devices: {
          ...mockSnapshot().devices,
          'dev-1': {
            ...mockSnapshot().devices['dev-1'],
            operational_status: 'down',
          },
        },
      }),
    });

    const node = nodes.find((candidate) => candidate.id === 'dev-1');
    expect(node?.data.device.status).toBe('up');
    expect(node?.data.runtime.status).toBe('down');
  });

  it('hydrates edge endpoint status from runtime models', () => {
    const { edges } = buildSubject({
      snapshot: mockSnapshot({
        devices: {
          'dev-1': {
            ...mockSnapshot().devices['dev-1'],
            operational_status: 'probing',
          },
          'dev-2': {
            ...mockSnapshot().devices['dev-2'],
            operational_status: 'down',
          },
        },
      }),
    });

    expect(edges[0]?.data).toMatchObject({
      sourceDeviceStatus: 'probing',
      targetDeviceStatus: 'down',
    });
  });

  it('hydrates edge endpoint reachability from runtime device models', () => {
    const baseSnapshot = mockSnapshot();
    const { edges } = buildSubject({
      snapshot: mockSnapshot({
        devices: {
          ...baseSnapshot.devices,
          'dev-1': {
            ...baseSnapshot.devices['dev-1'],
            health: 'unknown',
            primary_health: 'snmp_degraded',
            network_reachable: 'true',
            snmp_reachable: 'false',
            reachability: 'up',
            metrics_status: 'unavailable',
            metrics_reason: 'no_data',
            cpu_percent: null,
            mem_percent: null,
            uptime_secs: null,
          },
        },
      }),
    });

    expect(edges[0]?.data).toMatchObject({
      sourceDeviceHealth: 'unknown',
      sourceDevicePrimaryHealth: 'snmp_degraded',
      sourceDeviceReachability: 'up',
      sourceDeviceNetworkReachable: 'true',
      sourceDeviceSnmpReachable: 'false',
      targetDeviceHealth: 'warning',
      targetDevicePrimaryHealth: 'up_fresh',
      targetDeviceReachability: 'up',
      targetDeviceNetworkReachable: 'true',
      targetDeviceSnmpReachable: 'true',
    });
  });

  it('hydrates positive throughput data from runtime link models', () => {
    const { edges } = buildSubject({});

    expect(edges[0]?.data).toMatchObject({
      throughputLabel: 'TX: 1K / RX: 2K',
      utilization: 0.15,
      metrics: expect.objectContaining({
        tx_bps: 1200,
        rx_bps: 2400,
      }),
    });
  });

  it('removes throughput when runtime marks link telemetry unusable', () => {
    const { edges } = buildSubject({
      snapshot: mockSnapshot({
        links: {
          'link-1': {
            ...mockSnapshot().links['link-1'],
            metrics_status: 'unavailable',
            metrics_reason: 'upstream_unavailable',
            tx_bps: null,
            rx_bps: null,
            utilization: null,
          },
        },
      }),
    });

    expect(edges[0]?.data).toMatchObject({
      metrics: null,
      throughputLabel: undefined,
      utilization: null,
    });
  });
});
