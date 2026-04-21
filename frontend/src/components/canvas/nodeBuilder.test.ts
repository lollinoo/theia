import { describe, expect, it, vi } from 'vitest';

import { buildTopologyNodes } from './nodeBuilder';
import type { Device, Link } from '../../types/api';
import type { AlertDTO, SnapshotPayload } from '../../types/metrics';

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
      { x: 120, y: 180 },
      false,
      vi.fn(),
      mockSnapshot(),
      [mockAlert()],
    );

    expect(nodes[0].data.device.status).toBe('down');
    expect(nodes[0].data.alertStatus).toBe('degraded');
    expect(nodes[0].data.metrics).toMatchObject({
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
      { x: 120, y: 180 },
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
    expect(nodes[0].data.metrics).toMatchObject({
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
      { x: 120, y: 180 },
      false,
      vi.fn(),
      mockSnapshot(),
      [],
    );

    expect(nodes[0].data.monitoringState).toBe('unmonitored');
    expect(nodes[0].data.metrics).toBeNull();
    expect(nodes[0].data.device.status).toBe('down');
  });

  it('hydrates status from normalized snapshot device runtime', () => {
    const nodes = buildTopologyNodes(
      [mockDevice({ status: 'up' })],
      new Map(),
      new Map(),
      { x: 120, y: 180 },
      false,
      vi.fn(),
      mockSnapshot(),
      [],
    );

    expect(nodes[0].data.device.status).toBe('down');
    expect(nodes[0].data.metrics).toMatchObject({
      health: 'warning',
      last_polled_at: '2026-04-13T11:59:45Z',
      expected_poll_interval_seconds: 60,
    });
  });

  it('prefers normalized alert state over the alert feed when runtime exists', () => {
    const nodes = buildTopologyNodes(
      [mockDevice({ status: 'up' })],
      new Map(),
      new Map(),
      { x: 120, y: 180 },
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

    expect(nodes[0].data.alertStatus).toBe('normal');
  });

  it('preserves normalized unmonitored device state when building nodes', () => {
    const nodes = buildTopologyNodes(
      [mockDevice({ status: 'up', ip: '10.0.0.1', device_type: 'router' })],
      new Map(),
      new Map(),
      { x: 120, y: 180 },
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

    expect(nodes[0].data.monitoringState).toBe('unmonitored');
    expect(nodes[0].data.metrics).toBeNull();
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
