/**
 * Exercises node builder topology canvas behavior so refactors preserve the documented contract.
 */
import type { SnapGrid } from '@xyflow/react';
import { describe, expect, it, vi } from 'vitest';

import type { Device, Link } from '../../types/api';
import type { AlertDTO, SnapshotPayload } from '../../types/metrics';
import { buildTopologyNodes } from './nodeBuilder';

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
    devices: {
      'dev-1': {
        device_id: 'dev-1',
        operational_status: 'down',
        reachability: 'hard_down',
        health: 'warning',
        freshness: 'fresh',
        primary_reason: 'device_unreachable',
        metrics_status: 'available',
        metrics_reason: 'ok',
        alert_status: 'degraded',
        firing_alert_count: 1,
        last_collected_at: '2026-04-13T11:59:45Z',
        last_polled_at: '2026-04-13T11:59:45Z',
        expected_poll_interval_seconds: 60,
        cpu_percent: 42,
        mem_percent: 68,
        temp_celsius: null,
        uptime_secs: null,
      },
    },
    links: {},
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

function mockAlert(overrides: Partial<AlertDTO> = {}): AlertDTO {
  return {
    device_id: 'dev-1',
    severity: 'critical',
    alert_name: 'DeviceDown',
    state: 'firing',
    summary: 'router unreachable',
    ...overrides,
  };
}

describe('buildTopologyNodes', () => {
  it('keeps overview metadata on down physical nodes instead of nulling metrics', () => {
    const nodes = buildTopologyNodes(
      [mockDevice()],
      new Map(),
      new Map(),
      new Map([['dev-1', { x: 120, y: 180 }]]),
      false,
      vi.fn(),
      mockSnapshot(),
      [mockAlert()],
    );

    expect(nodes[0].data.device.status).toBe('up');
    expect(nodes[0].data.runtime.status).toBe('down');
    expect(nodes[0].data.runtime.alertStatus).toBe('degraded');
    expect(nodes[0].data.runtime.metrics).toMatchObject({
      health: 'warning',
      last_polled_at: '2026-04-13T11:59:45Z',
      expected_poll_interval_seconds: 60,
    });
  });

  it('passes overview metadata into virtual nodes instead of forcing metrics null', () => {
    const nodes = buildTopologyNodes(
      [
        mockDevice({
          device_type: 'virtual',
          ip: '192.168.1.1',
          poll_interval_override: 15,
          tags: { display_name: 'Cloud VPN', virtual_subtype: 'cloud' },
        }),
      ],
      new Map(),
      new Map(),
      new Map([['dev-1', { x: 120, y: 180 }]]),
      false,
      vi.fn(),
      {
        ...mockSnapshot(),
        devices: {
          'dev-1': {
            ...mockSnapshot().devices['dev-1'],
            operational_status: 'up',
          },
        },
      },
      [],
    );

    expect(nodes[0].data.isVirtual).toBe(true);
    expect(nodes[0].data.runtime.metrics).toMatchObject({
      health: 'warning',
      last_polled_at: '2026-04-13T11:59:45Z',
      expected_poll_interval_seconds: 60,
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
      new Map([['dev-1', { x: 120, y: 180 }]]),
      false,
      vi.fn(),
      mockSnapshot(),
      [],
    );

    expect(nodes[0].data.runtime.monitoringState).toBe('unmonitored');
    expect(nodes[0].data.runtime.metrics).toBeNull();
    expect(nodes[0].data.device.status).toBe('down');
  });

  it('hydrates status into node runtime from normalized snapshot device runtime', () => {
    const nodes = buildTopologyNodes(
      [mockDevice({ status: 'up' })],
      new Map(),
      new Map(),
      new Map([['dev-1', { x: 120, y: 180 }]]),
      false,
      vi.fn(),
      mockSnapshot(),
      [],
    );

    expect(nodes[0].data.device.status).toBe('up');
    expect(nodes[0].data.runtime.status).toBe('down');
    expect(nodes[0].data.runtime.metrics).toMatchObject({
      health: 'warning',
      last_polled_at: '2026-04-13T11:59:45Z',
      expected_poll_interval_seconds: 60,
    });
  });

  it('keeps static device status separate from runtime status when hydrating nodes', () => {
    const nodes = buildTopologyNodes(
      [mockDevice({ status: 'up' })],
      new Map(),
      new Map(),
      new Map([['dev-1', { x: 120, y: 180 }]]),
      false,
      vi.fn(),
      mockSnapshot(),
      [],
    );

    expect(nodes[0].data.device.status).toBe('up');
    expect(nodes[0].data.runtime).toMatchObject({
      status: 'down',
      metrics: expect.objectContaining({
        health: 'warning',
        expected_poll_interval_seconds: 60,
      }),
    });
  });

  it('prefers normalized alert state over the alert feed when runtime exists', () => {
    const nodes = buildTopologyNodes(
      [mockDevice({ status: 'up' })],
      new Map(),
      new Map(),
      new Map([['dev-1', { x: 120, y: 180 }]]),
      false,
      vi.fn(),
      {
        ...mockSnapshot(),
        devices: {
          'dev-1': {
            ...mockSnapshot().devices['dev-1'],
            alert_status: 'normal',
            firing_alert_count: 0,
          },
        },
      },
      [mockAlert()],
    );

    expect(nodes[0].data.runtime.alertStatus).toBe('normal');
  });

  it('preserves normalized unmonitored device state when building nodes', () => {
    const nodes = buildTopologyNodes(
      [mockDevice({ status: 'up', ip: '10.0.0.1', device_type: 'router' })],
      new Map(),
      new Map(),
      new Map([['dev-1', { x: 120, y: 180 }]]),
      false,
      vi.fn(),
      {
        ...mockSnapshot(),
        devices: {
          'dev-1': {
            ...mockSnapshot().devices['dev-1'],
            operational_status: 'unmonitored',
            reachability: 'unmonitored',
            freshness: 'unmonitored',
            metrics_status: 'unmonitored',
            metrics_reason: 'unmonitored',
            primary_reason: 'unmonitored',
            last_collected_at: null,
            last_polled_at: null,
            cpu_percent: null,
            mem_percent: null,
            uptime_secs: null,
          },
        },
      },
      [],
    );

    expect(nodes[0].data.runtime.monitoringState).toBe('unmonitored');
    expect(nodes[0].data.runtime.metrics).toBeNull();
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
      new Map([
        ['dev-1', { x: 120, y: 180 }],
        ['dev-2', { x: 120, y: 180 }],
      ]),
      false,
      vi.fn(),
      mockSnapshot(),
      [],
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
      new Map([['dev-1', { x: 120, y: 180, pinned: false }]]),
      new Map([
        ['dev-1', { x: 900, y: 910 }],
        ['dev-2', { x: 320, y: 420 }],
      ]),
      new Map([['dev-2', { x: 50, y: 60 }]]),
      false,
      vi.fn(),
      null,
      [],
      [],
      undefined,
      new Map([['dev-1', { x: 25, y: 35, pinned: true }]]),
      new Set(['dev-2']),
    );

    expect(nodes[0].position).toEqual({ x: 25, y: 35 });
    expect(nodes[0].data.pinned).toBe(true);
    expect(nodes[1].position).toEqual({ x: 50, y: 60 });
  });

  it('preserves saved and current positions while snapping new placements when enabled', () => {
    const devices = [
      mockDevice({ id: 'saved' }),
      mockDevice({ id: 'current' }),
      mockDevice({ id: 'computed' }),
      mockDevice({ id: 'explicit' }),
    ];
    const buildNodes = (snapGrid: SnapGrid | null) =>
      buildTopologyNodes(
        devices,
        new Map([['saved', { x: 44, y: 46, pinned: true }]]),
        new Map([['computed', { x: 104, y: 106 }]]),
        new Map([['explicit', { x: 136, y: 164 }]]),
        false,
        vi.fn(),
        null,
        [],
        [],
        undefined,
        new Map([['current', { x: 74, y: -16, pinned: true }]]),
        new Set(['computed']),
        snapGrid,
      );

    const nodesById = new Map(buildNodes([30, 30]).map((node) => [node.id, node]));

    expect(nodesById.get('saved')?.position).toEqual({ x: 44, y: 46 });
    expect(nodesById.get('current')?.position).toEqual({ x: 74, y: -16 });
    expect(nodesById.get('computed')?.position).toEqual({ x: 90, y: 120 });
    expect(nodesById.get('explicit')?.position).toEqual({ x: 150, y: 150 });
  });

  it('preserves exact resolved positions when the grid is disabled', () => {
    const devices = [
      mockDevice({ id: 'saved' }),
      mockDevice({ id: 'current' }),
      mockDevice({ id: 'computed' }),
      mockDevice({ id: 'explicit' }),
    ];
    const nodes = buildTopologyNodes(
      devices,
      new Map([['saved', { x: 44, y: 46, pinned: true }]]),
      new Map([['computed', { x: 104, y: 106 }]]),
      new Map([['explicit', { x: 136, y: 164 }]]),
      false,
      vi.fn(),
      null,
      [],
      [],
      undefined,
      new Map([['current', { x: 74, y: -16, pinned: true }]]),
      new Set(['computed']),
      null,
    );
    const nodesById = new Map(nodes.map((node) => [node.id, node]));

    expect(nodesById.get('saved')?.position).toEqual({ x: 44, y: 46 });
    expect(nodesById.get('current')?.position).toEqual({ x: 74, y: -16 });
    expect(nodesById.get('computed')?.position).toEqual({ x: 104, y: 106 });
    expect(nodesById.get('explicit')?.position).toEqual({ x: 136, y: 164 });
  });

  it('applies only the keyed explicit override ahead of current positions', () => {
    const explicitPositions = new Map([['dev-new', { x: 450, y: 275 }]]);
    const currentPositions = new Map([
      ['dev-old', { x: 10, y: 20, pinned: true }],
      ['dev-new', { x: 9000, y: 9000, pinned: true }],
    ]);
    const nodes = buildTopologyNodes(
      [
        mockDevice({ id: 'dev-old' }),
        mockDevice({
          id: 'dev-new',
          hostname: 'router-new',
          ip: '10.0.0.2',
          sys_name: 'router-new',
        }),
      ],
      new Map(),
      new Map(),
      explicitPositions,
      false,
      vi.fn(),
      null,
      [],
      [],
      undefined,
      currentPositions,
      new Set(),
    );

    const nodesById = new Map(nodes.map((node) => [node.id, node]));
    expect(nodesById.get('dev-old')?.position).toEqual({ x: 10, y: 20 });
    expect(nodesById.get('dev-old')?.data.pinned).toBe(true);
    expect(nodesById.get('dev-new')?.position).toEqual({ x: 450, y: 275 });
    expect(nodesById.get('dev-new')?.data.pinned).toBe(false);
  });

  it('ignores non-finite keyed overrides without changing current or saved priority', () => {
    const nodes = buildTopologyNodes(
      [
        mockDevice({ id: 'dev-current' }),
        mockDevice({
          id: 'dev-saved',
          hostname: 'router-saved',
          ip: '10.0.0.3',
          sys_name: 'router-saved',
        }),
      ],
      new Map([
        ['dev-current', { x: 100, y: 120, pinned: false }],
        ['dev-saved', { x: 30, y: 40, pinned: true }],
      ]),
      new Map(),
      new Map([
        ['dev-current', { x: Number.POSITIVE_INFINITY, y: 275 }],
        ['dev-saved', { x: 450, y: Number.NaN }],
      ]),
      false,
      vi.fn(),
      null,
      [],
      [],
      undefined,
      new Map([['dev-current', { x: 10, y: 20, pinned: true }]]),
      new Set(),
    );

    const nodesById = new Map(nodes.map((node) => [node.id, node]));
    expect(nodesById.get('dev-current')?.position).toEqual({ x: 10, y: 20 });
    expect(nodesById.get('dev-current')?.data.pinned).toBe(true);
    expect(nodesById.get('dev-saved')?.position).toEqual({ x: 30, y: 40 });
    expect(nodesById.get('dev-saved')?.data.pinned).toBe(true);
  });
});
