/**
 * Exercises edge builder topology canvas behavior so refactors preserve the documented contract.
 */
import { describe, expect, it, vi } from 'vitest';
import type { Device, Link } from '../../types/api';
import type { AlertDTO } from '../../types/metrics';
import type { DeviceNode } from '../DeviceCard';
import { buildEdgeData, buildTopologyEdges } from './edgeBuilder';

function mockDevice(overrides: Partial<Device> = {}): Device {
  return {
    id: 'dev-1',
    hostname: 'router-01',
    ip: '10.0.0.1',
    device_type: 'router',
    status: 'up',
    sys_name: 'router-01',
    sys_descr: 'RouterOS',
    hardware_model: 'RB4011',
    vendor: 'mikrotik',
    managed: true,
    interfaces: [],
    backup_supported: false,
    metrics_source: 'prometheus',
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
    source_if_speed: 0,
    source_if_oper_status: '',
    target_if_speed: 0,
    target_if_oper_status: '',
    ...overrides,
  };
}

function mockNode(id: string, x: number, y: number): DeviceNode {
  return {
    id,
    type: 'device',
    position: { x, y },
    data: {
      device: mockDevice({ id, ip: id === 'dev-1' ? '10.0.0.1' : '10.0.0.2', sys_name: id }),
      pinned: false,
    },
  } as DeviceNode;
}

function mockAlert(overrides: Partial<AlertDTO> = {}): AlertDTO {
  return {
    device_id: 'dev-1',
    severity: 'critical',
    alert_name: 'LinkDown',
    state: 'firing',
    summary: 'ether1 down',
    ...overrides,
  };
}

describe('buildEdgeData', () => {
  it.each([
    { label: 'physical', deviceType: 'router' as const },
    { label: 'virtual', deviceType: 'virtual' as const },
  ])('preserves saved route controls for $label links', ({ deviceType }) => {
    const source = mockDevice({ id: 'dev-1', device_type: deviceType });
    const target = mockDevice({ id: 'dev-2' });
    const devicesByID = new Map([
      ['dev-1', source],
      ['dev-2', target],
    ]);
    const route = { version: 1 as const, waypoints: [{ x: 12.5, y: -8 }] };
    const onRouteCommit = vi.fn();

    const result = buildEdgeData(mockLink(), devicesByID, {
      route,
      routeEditable: true,
      onRouteCommit,
    });

    expect(result.route).toBe(route);
    expect(result.routeEditable).toBe(true);
    expect(result.onRouteCommit).toBe(onRouteCommit);
  });

  it('physical-physical link with speed mismatch sets speedMismatch=true', () => {
    const source = mockDevice({ id: 'dev-1' });
    const target = mockDevice({ id: 'dev-2' });
    const devicesByID = new Map([
      ['dev-1', source],
      ['dev-2', target],
    ]);
    const link = mockLink({
      source_if_speed: 1_000_000_000,
      source_if_oper_status: 'up',
      target_if_speed: 100_000_000,
      target_if_oper_status: 'up',
    });

    const result = buildEdgeData(link, devicesByID);

    expect(result.speedMismatch).toBe(true);
    expect(result.bandwidthLabel).toBe('100 Mbps');
    expect(result.speedLabel).toBe('SPD 1 Gbps');
    expect(result.negotiationState).toBe('mismatch');
  });

  it('source is virtual device: speedMismatch=false, bandwidthLabel uses target speed', () => {
    const source = mockDevice({ id: 'dev-1', device_type: 'virtual' });
    const target = mockDevice({ id: 'dev-2', device_type: 'router' });
    const devicesByID = new Map([
      ['dev-1', source],
      ['dev-2', target],
    ]);
    const link = mockLink({
      target_if_speed: 1_000_000_000,
      target_if_oper_status: 'up',
    });

    const result = buildEdgeData(link, devicesByID);

    expect(result.speedMismatch).toBe(false);
    expect(result.bandwidthLabel).toBe('1 Gbps');
    expect(result.speedLabel).toBe('SPD 1 Gbps');
    expect(result.negotiationState).toBe('not_applicable');
  });

  it('target is virtual device: speedMismatch=false, bandwidthLabel uses source speed', () => {
    const source = mockDevice({ id: 'dev-1', device_type: 'router' });
    const target = mockDevice({ id: 'dev-2', device_type: 'virtual' });
    const devicesByID = new Map([
      ['dev-1', source],
      ['dev-2', target],
    ]);
    const link = mockLink({
      source_if_speed: 1_000_000_000,
      source_if_oper_status: 'up',
    });

    const result = buildEdgeData(link, devicesByID);

    expect(result.speedMismatch).toBe(false);
    expect(result.bandwidthLabel).toBe('1 Gbps');
    expect(result.speedLabel).toBe('SPD 1 Gbps');
    expect(result.negotiationState).toBe('not_applicable');
  });

  it('virtual link with no real interface speed returns undefined bandwidthLabel', () => {
    const source = mockDevice({
      id: 'dev-1',
      device_type: 'virtual',
      interfaces: [],
    });
    const target = mockDevice({
      id: 'dev-2',
      device_type: 'router',
      interfaces: [],
    });
    const devicesByID = new Map([
      ['dev-1', source],
      ['dev-2', target],
    ]);
    const link = mockLink();

    const result = buildEdgeData(link, devicesByID);

    expect(result.bandwidthLabel).toBeUndefined();
    expect(result.speedMismatch).toBe(false);
    expect(result.speedLabel).toBeUndefined();
    expect(result.negotiationState).toBe('not_applicable');
  });

  it('virtual link preserves existing throughputLabel and metrics from existingData', () => {
    const source = mockDevice({ id: 'dev-1', device_type: 'virtual' });
    const target = mockDevice({ id: 'dev-2', device_type: 'router' });
    const devicesByID = new Map([
      ['dev-1', source],
      ['dev-2', target],
    ]);
    const link = mockLink({
      target_if_speed: 1_000_000_000,
      target_if_oper_status: 'up',
    });

    const existingData = {
      throughputLabel: 'TX: 500M / RX: 300M',
      metrics: {
        device_id: 'dev-2',
        if_name: 'ether2',
        tx_bps: 500_000_000,
        rx_bps: 300_000_000,
        utilization: 50,
        collected_at: '',
      },
      utilization: 50,
    };

    const result = buildEdgeData(link, devicesByID, existingData);

    expect(result.throughputLabel).toBe('TX: 500M / RX: 300M');
    expect(result.metrics).toEqual(existingData.metrics);
    expect(result.utilization).toBe(50);
  });

  it('physical links with one-sided speed still expose stacked rate and speed telemetry on the canvas', () => {
    const source = mockDevice({ id: 'dev-1' });
    const target = mockDevice({ id: 'dev-2' });
    const devicesByID = new Map([
      ['dev-1', source],
      ['dev-2', target],
    ]);
    const link = mockLink({
      source_if_speed: 1_000_000_000,
      source_if_oper_status: 'up',
    });

    const result = buildEdgeData(link, devicesByID);

    expect(result.bandwidthLabel).toBe('1 Gbps');
    expect(result.speedLabel).toBe('SPD 1 Gbps');
    expect(result.negotiationState).toBe('partial');
  });

  it('physical links with no negotiated speed still expose the primary rate signal without inventing AUTO pills', () => {
    const source = mockDevice({ id: 'dev-1' });
    const target = mockDevice({ id: 'dev-2' });
    const devicesByID = new Map([
      ['dev-1', source],
      ['dev-2', target],
    ]);
    const link = mockLink();

    const result = buildEdgeData(link, devicesByID);

    expect(result.bandwidthLabel).toBe('SPD ?');
    expect(result.speedLabel).toBeUndefined();
    expect(result.negotiationState).toBe('unknown');
  });

  it('virtual source: sourceIfStatus is undefined, targetIfStatus shows real status', () => {
    const source = mockDevice({ id: 'dev-1', device_type: 'virtual' });
    const target = mockDevice({ id: 'dev-2', device_type: 'router' });
    const devicesByID = new Map([
      ['dev-1', source],
      ['dev-2', target],
    ]);
    const link = mockLink({
      target_if_speed: 1_000_000_000,
      target_if_oper_status: 'up',
    });

    const result = buildEdgeData(link, devicesByID);

    expect(result.sourceIfStatus).toBeUndefined();
    expect(result.targetIfStatus).toBe('up');
  });

  it('marks links to no-IP virtual nodes as inert for live color telemetry', () => {
    const source = mockDevice({ id: 'dev-1', device_type: 'virtual', ip: '' });
    const target = mockDevice({ id: 'dev-2', device_type: 'router' });
    const devicesByID = new Map([
      ['dev-1', source],
      ['dev-2', target],
    ]);
    const link = mockLink({
      target_if_speed: 1_000_000_000,
      target_if_oper_status: 'up',
    });

    const result = buildEdgeData(link, devicesByID);

    expect(result.inertVirtualLink).toBe(true);
  });

  it('suppresses the no-IP virtual endpoint status while preserving the physical endpoint status', () => {
    const source = mockDevice({ id: 'dev-1', device_type: 'router', status: 'down' });
    const target = mockDevice({ id: 'dev-2', device_type: 'virtual', ip: '', status: 'down' });
    const devicesByID = new Map([
      ['dev-1', source],
      ['dev-2', target],
    ]);
    const link = mockLink({
      source_if_speed: 1_000_000_000,
      source_if_oper_status: 'up',
    });

    const result = buildEdgeData(link, devicesByID);

    expect(result.sourceIsVirtual).toBe(false);
    expect(result.targetIsVirtual).toBe(true);
    expect(result.sourceDeviceStatus).toBe('down');
    expect(result.targetDeviceStatus).toBeUndefined();
  });

  it('virtual target: targetIfStatus is undefined, sourceIfStatus shows real status', () => {
    const source = mockDevice({ id: 'dev-1', device_type: 'router' });
    const target = mockDevice({ id: 'dev-2', device_type: 'virtual' });
    const devicesByID = new Map([
      ['dev-1', source],
      ['dev-2', target],
    ]);
    const link = mockLink({
      source_if_speed: 1_000_000_000,
      source_if_oper_status: 'up',
    });

    const result = buildEdgeData(link, devicesByID);

    expect(result.sourceIfStatus).toBe('up');
    expect(result.targetIfStatus).toBeUndefined();
  });

  it('keeps IP-bearing virtual links live for telemetry coloring', () => {
    const source = mockDevice({ id: 'dev-1', device_type: 'virtual', ip: '192.168.1.1' });
    const target = mockDevice({ id: 'dev-2', device_type: 'router' });
    const devicesByID = new Map([
      ['dev-1', source],
      ['dev-2', target],
    ]);
    const link = mockLink({
      target_if_speed: 1_000_000_000,
      target_if_oper_status: 'up',
    });

    const result = buildEdgeData(link, devicesByID);

    expect(result.inertVirtualLink).toBe(false);
  });
});

describe('buildTopologyEdges', () => {
  it('derives edge alert status from separate alert state', () => {
    const dev1 = mockDevice({ id: 'dev-1', ip: '10.0.0.1', sys_name: 'dev-1' });
    const dev2 = mockDevice({ id: 'dev-2', ip: '10.0.0.2', sys_name: 'dev-2' });
    const devicesByID = new Map([
      ['dev-1', dev1],
      ['dev-2', dev2],
    ]);
    const nodes = [mockNode('dev-1', 0, 0), mockNode('dev-2', 300, 0)];

    const edges = buildTopologyEdges([mockLink()], devicesByID, nodes, undefined, undefined, [
      mockAlert(),
    ]);

    expect(edges).toHaveLength(1);
    expect(edges[0].data.alertStatus).toBe('down');
  });

  it('keeps distinct parallel uplinks between the same devices visible', () => {
    const dev1 = mockDevice({ id: 'dev-1', ip: '10.0.0.1', sys_name: 'dev-1' });
    const dev2 = mockDevice({ id: 'dev-2', ip: '10.0.0.2', sys_name: 'dev-2' });
    const devicesByID = new Map([
      ['dev-1', dev1],
      ['dev-2', dev2],
    ]);
    const nodes = [mockNode('dev-1', 0, 0), mockNode('dev-2', 300, 0)];
    const links = [
      mockLink({ id: 'link-1', source_if_name: 'sfp1', target_if_name: 'ether1' }),
      mockLink({ id: 'link-2', source_if_name: 'sfp2', target_if_name: 'ether2' }),
    ];

    const edges = buildTopologyEdges(links, devicesByID, nodes);

    expect(edges).toHaveLength(2);
    expect(edges.map((edge) => edge.id)).toEqual(['link-1', 'link-2']);
    expect(edges.map((edge) => edge.data.parallelIndex)).toEqual([0, 1]);
  });

  it('hides lower-quality duplicate links when a physical link exists for the same device pair', () => {
    const dev1 = mockDevice({ id: 'dev-1', ip: '10.0.0.1', sys_name: 'dev-1' });
    const dev2 = mockDevice({ id: 'dev-2', ip: '10.0.0.2', sys_name: 'dev-2' });
    const devicesByID = new Map([
      ['dev-1', dev1],
      ['dev-2', dev2],
    ]);
    const nodes = [mockNode('dev-1', 0, 0), mockNode('dev-2', 300, 0)];
    const links = [
      mockLink({ id: 'link-vlan', source_if_name: '', target_if_name: 'VLAN-99-MGMT-ETH6' }),
      mockLink({
        id: 'link-incomplete',
        source_if_name: '',
        target_if_name: 'ether6-link_new_apparati',
      }),
      mockLink({
        id: 'link-physical',
        source_if_name: 'ether2-verso-border-botte',
        target_if_name: 'ether6-link_new_apparati',
      }),
    ];

    const edges = buildTopologyEdges(links, devicesByID, nodes);

    expect(edges).toHaveLength(1);
    expect(edges[0].id).toBe('link-physical');
  });

  it('deduplicates only reverse-direction discovery of the same physical link', () => {
    const dev1 = mockDevice({ id: 'dev-1', ip: '10.0.0.1', sys_name: 'dev-1' });
    const dev2 = mockDevice({ id: 'dev-2', ip: '10.0.0.2', sys_name: 'dev-2' });
    const devicesByID = new Map([
      ['dev-1', dev1],
      ['dev-2', dev2],
    ]);
    const nodes = [mockNode('dev-1', 0, 0), mockNode('dev-2', 300, 0)];
    const links = [
      mockLink({ id: 'link-1', source_if_name: 'sfp1', target_if_name: 'ether1' }),
      mockLink({
        id: 'link-1-reverse',
        source_device_id: 'dev-2',
        source_if_name: 'ether1',
        target_device_id: 'dev-1',
        target_if_name: 'sfp1',
      }),
    ];

    const edges = buildTopologyEdges(links, devicesByID, nodes);

    expect(edges).toHaveLength(1);
    expect(edges[0].id).toBe('link-1');
    expect(edges[0].data.parallelIndex).toBe(0);
  });

  it('omits self-links from edge rendering so they can be shown as node annotations', () => {
    const dev1 = mockDevice({ id: 'dev-1', ip: '10.0.0.1', sys_name: 'dev-1' });
    const devicesByID = new Map([['dev-1', dev1]]);
    const nodes = [mockNode('dev-1', 120, 180)];
    const links = [
      mockLink({
        id: 'link-self',
        target_device_id: 'dev-1',
        target_if_name: 'ether9',
      }),
    ];

    const edges = buildTopologyEdges(links, devicesByID, nodes);

    expect(edges).toHaveLength(0);
  });

  it('drops incomplete same-pair edges when a richer edge already provides link speed metadata', () => {
    const dev1 = mockDevice({ id: 'dev-1', ip: '10.0.0.1', sys_name: 'dev-1' });
    const dev2 = mockDevice({ id: 'dev-2', ip: '10.0.0.2', sys_name: 'dev-2' });
    const devicesByID = new Map([
      ['dev-1', dev1],
      ['dev-2', dev2],
    ]);
    const nodes = [mockNode('dev-1', 0, 0), mockNode('dev-2', 300, 0)];
    const links = [
      mockLink({
        id: 'link-rich',
        source_if_name: 'ether1',
        target_if_name: 'ether2',
        source_if_speed: 1_000_000_000,
        target_if_speed: 1_000_000_000,
      }),
      mockLink({
        id: 'link-incomplete',
        source_if_name: '',
        target_if_name: 'lag-member',
      }),
    ];

    const edges = buildTopologyEdges(links, devicesByID, nodes);

    expect(edges).toHaveLength(1);
    expect(edges[0].id).toBe('link-rich');
    expect(edges[0].data.bandwidthLabel).toBe('1 Gbps');
  });
});
