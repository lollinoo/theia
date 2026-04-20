import { describe, expect, it, vi } from 'vitest';

import type { Device, Link } from '../../types/api';
import type { AlertDTO, SnapshotPayload } from '../../types/metrics';
import { composeCanvasTopology } from './topologyComposer';
import { buildRuntimeState } from './runtimeAdapters';

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

function mockSnapshot(overrides: Partial<SnapshotPayload> = {}): SnapshotPayload {
  return {
    device_metrics: {
      'dev-1': {
        device_id: 'dev-1',
        cpu_percent: 12,
        mem_percent: 44,
        uptime_secs: 900,
        collected_at: '2026-04-20T12:00:00Z',
        health: 'healthy',
      },
      'dev-2': {
        device_id: 'dev-2',
        cpu_percent: 18,
        mem_percent: 50,
        uptime_secs: 800,
        collected_at: '2026-04-20T12:00:00Z',
        health: 'warning',
      },
    },
    link_metrics: {
      'dev-1': [{
        device_id: 'dev-1',
        if_name: 'ether1',
        tx_bps: 1200,
        rx_bps: 2400,
        utilization: 0.15,
        collected_at: '2026-04-20T12:00:00Z',
      }],
    },
    device_statuses: {
      'dev-1': 'up',
      'dev-2': 'up',
    },
    ...overrides,
  };
}

function buildSubject(options: {
  snapshot?: SnapshotPayload | null;
  alerts?: AlertDTO[];
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
    defaultPosition: undefined,
    editMode: false,
    openDeviceMenu: vi.fn(),
    openEdgeMenu: vi.fn(),
    placementDeviceIds: new Set(['dev-1', 'dev-2']),
    alerts: options.alerts ?? [],
  });
}

describe('composeCanvasTopology', () => {
  it('hydrates node device status from runtime models', () => {
    const { nodes } = buildSubject({
      snapshot: mockSnapshot({
        device_statuses: {
          'dev-1': 'down',
          'dev-2': 'up',
        },
      }),
    });

    expect(nodes.find((node) => node.id === 'dev-1')?.data.device.status).toBe('down');
  });

  it('hydrates edge endpoint status from runtime models', () => {
    const { edges } = buildSubject({
      snapshot: mockSnapshot({
        device_statuses: {
          'dev-1': 'probing',
          'dev-2': 'down',
        },
      }),
    });

    expect(edges[0]?.data).toMatchObject({
      sourceDeviceStatus: 'probing',
      targetDeviceStatus: 'down',
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
        device_statuses: {
          'dev-1': 'down',
          'dev-2': 'down',
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
