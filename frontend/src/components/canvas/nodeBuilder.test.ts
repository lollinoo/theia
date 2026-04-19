import { describe, expect, it, vi } from 'vitest';

import { buildTopologyNodes } from './nodeBuilder';
import type { Device, Link } from '../../types/api';
import type { SnapshotPayload } from '../../types/metrics';

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

function mockSnapshot(): SnapshotPayload {
  return {
    device_metrics: {
      'dev-1': {
        device_id: 'dev-1',
        cpu_percent: 42,
        mem_percent: 68,
        temp_celsius: 55,
        uptime_secs: 86400,
        collected_at: '2026-04-13T11:59:45Z',
        health: 'warning',
        stale: false,
        last_polled_at: '2026-04-13T11:59:30Z',
        expected_poll_interval_seconds: 30,
      },
    },
    link_metrics: {},
    alerts: [],
    device_statuses: {
      'dev-1': 'down',
    },
    device_hostnames: {},
    device_models: {},
  };
}

function mockLink(overrides: Partial<Link> = {}): Link {
  return {
    id: 'link-1',
    source_device_id: 'dev-1',
    source_if_name: 'ether1',
    target_device_id: 'dev-1',
    target_if_name: 'ether9',
    discovery_protocol: 'lldp',
    source_if_speed: 0,
    source_if_oper_status: 'up',
    target_if_speed: 0,
    target_if_oper_status: 'up',
    ...overrides,
  };
}

describe('buildTopologyNodes', () => {
  it('keeps overview metadata on down physical nodes instead of nulling metrics', () => {
    const nodes = buildTopologyNodes(
      [mockDevice()],
      new Map(),
      new Map(),
      { x: 120, y: 180 },
      false,
      vi.fn(),
      mockSnapshot(),
    );

    expect(nodes[0].data.device.status).toBe('down');
    expect(nodes[0].data.metrics).toMatchObject({
      health: 'warning',
      last_polled_at: '2026-04-13T11:59:30Z',
      expected_poll_interval_seconds: 30,
    });
  });

  it('passes overview metadata into virtual nodes instead of forcing metrics null', () => {
    const nodes = buildTopologyNodes(
      [
        mockDevice({
          device_type: 'virtual',
          ip: '192.168.1.1',
          tags: { display_name: 'Cloud VPN', virtual_subtype: 'cloud' },
        }),
      ],
      new Map(),
      new Map(),
      { x: 120, y: 180 },
      false,
      vi.fn(),
      {
        ...mockSnapshot(),
        device_statuses: {
          'dev-1': 'up',
        },
      },
    );

    expect(nodes[0].data.isVirtual).toBe(true);
    expect(nodes[0].data.metrics).toMatchObject({
      health: 'warning',
      last_polled_at: '2026-04-13T11:59:30Z',
      expected_poll_interval_seconds: 30,
    });
  });

  it('sanitizes no-ip virtual placeholders into unmonitored nodes during mapping', () => {
    const nodes = buildTopologyNodes(
      [
        mockDevice({
          device_type: 'virtual',
          ip: '',
          status: 'down',
          tags: { display_name: 'Internet', virtual_subtype: 'internet' },
        }),
      ],
      new Map(),
      new Map(),
      { x: 120, y: 180 },
      false,
      vi.fn(),
      mockSnapshot(),
    );

    expect(nodes[0].data.monitoringState).toBe('unmonitored');
    expect(nodes[0].data.metrics).toBeNull();
    expect(nodes[0].data.device.status).toBe('down');
  });

  it('attaches visible self-links to matching device nodes', () => {
    const onSelfLinkClick = vi.fn();
    const nodes = buildTopologyNodes(
      [
        mockDevice(),
        mockDevice({
          id: 'dev-2',
          hostname: 'router-02',
          ip: '10.0.0.2',
          sys_name: 'router-02',
        }),
      ],
      new Map(),
      new Map(),
      { x: 120, y: 180 },
      false,
      vi.fn(),
      mockSnapshot(),
      [
        mockLink(),
        mockLink({
          id: 'link-2',
          source_device_id: 'dev-1',
          target_device_id: 'dev-2',
          target_if_name: 'ether2',
        }),
      ],
      onSelfLinkClick,
    );

    expect(nodes[0].data.selfLinks?.map((link) => link.id)).toEqual(['link-1']);
    expect(nodes[0].data.onSelfLinkClick).toBe(onSelfLinkClick);
    expect(nodes[1].data.selfLinks).toBeUndefined();
  });

  it('prefers current positions and limits computed placement to placementDeviceIds', () => {
    const nodes = buildTopologyNodes(
      [
        mockDevice(),
        mockDevice({
          id: 'dev-2',
          hostname: 'router-02',
          ip: '10.0.0.2',
          sys_name: 'router-02',
        }),
      ],
      new Map([
        ['dev-1', { x: 120, y: 180, pinned: false }],
      ]),
      new Map([
        ['dev-1', { x: 900, y: 910 }],
        ['dev-2', { x: 320, y: 420 }],
      ]),
      { x: 50, y: 60 },
      false,
      vi.fn(),
      null,
      [],
      undefined,
      new Map([
        ['dev-1', { x: 25, y: 35, pinned: true }],
      ]),
      new Set(['dev-2']),
    );

    expect(nodes[0].position).toEqual({ x: 25, y: 35 });
    expect(nodes[0].data.pinned).toBe(true);
    expect(nodes[1].position).toEqual({ x: 50, y: 60 });
  });
});
