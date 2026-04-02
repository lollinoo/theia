import { describe, it, expect } from 'vitest';
import { buildEdgeData } from './edgeBuilder';
import type { Device, Link } from '../../types/api';

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
    ...overrides,
  };
}

describe('buildEdgeData', () => {
  it('physical-physical link with speed mismatch sets speedMismatch=true', () => {
    const source = mockDevice({
      id: 'dev-1',
      interfaces: [
        { id: 'if-1', if_index: 1, if_name: 'ether1', if_descr: 'ether1', speed: 1_000_000_000, admin_status: 'up', oper_status: 'up' },
      ],
    });
    const target = mockDevice({
      id: 'dev-2',
      interfaces: [
        { id: 'if-2', if_index: 1, if_name: 'ether2', if_descr: 'ether2', speed: 100_000_000, admin_status: 'up', oper_status: 'up' },
      ],
    });
    const devicesByID = new Map([
      ['dev-1', source],
      ['dev-2', target],
    ]);
    const link = mockLink();

    const result = buildEdgeData(link, devicesByID);

    expect(result.speedMismatch).toBe(true);
    expect(result.bandwidthLabel).toContain('(!)');
    expect(result.bandwidthLabel).toContain('100 Mbps');
  });

  it('source is virtual device: speedMismatch=false, bandwidthLabel uses target speed', () => {
    const source = mockDevice({
      id: 'dev-1',
      device_type: 'virtual',
      interfaces: [],
    });
    const target = mockDevice({
      id: 'dev-2',
      device_type: 'router',
      interfaces: [
        { id: 'if-2', if_index: 1, if_name: 'ether2', if_descr: 'ether2', speed: 1_000_000_000, admin_status: 'up', oper_status: 'up' },
      ],
    });
    const devicesByID = new Map([
      ['dev-1', source],
      ['dev-2', target],
    ]);
    const link = mockLink();

    const result = buildEdgeData(link, devicesByID);

    expect(result.speedMismatch).toBe(false);
    expect(result.bandwidthLabel).toBe('1 Gbps');
  });

  it('target is virtual device: speedMismatch=false, bandwidthLabel uses source speed', () => {
    const source = mockDevice({
      id: 'dev-1',
      device_type: 'router',
      interfaces: [
        { id: 'if-1', if_index: 1, if_name: 'ether1', if_descr: 'ether1', speed: 1_000_000_000, admin_status: 'up', oper_status: 'up' },
      ],
    });
    const target = mockDevice({
      id: 'dev-2',
      device_type: 'virtual',
      interfaces: [],
    });
    const devicesByID = new Map([
      ['dev-1', source],
      ['dev-2', target],
    ]);
    const link = mockLink();

    const result = buildEdgeData(link, devicesByID);

    expect(result.speedMismatch).toBe(false);
    expect(result.bandwidthLabel).toBe('1 Gbps');
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
  });

  it('virtual link preserves existing throughputLabel and metrics from existingData', () => {
    const source = mockDevice({
      id: 'dev-1',
      device_type: 'virtual',
      interfaces: [],
    });
    const target = mockDevice({
      id: 'dev-2',
      device_type: 'router',
      interfaces: [
        { id: 'if-2', if_index: 1, if_name: 'ether2', if_descr: 'ether2', speed: 1_000_000_000, admin_status: 'up', oper_status: 'up' },
      ],
    });
    const devicesByID = new Map([
      ['dev-1', source],
      ['dev-2', target],
    ]);
    const link = mockLink();

    const existingData = {
      throughputLabel: 'TX: 500M / RX: 300M',
      metrics: { device_id: 'dev-2', if_name: 'ether2', tx_bps: 500_000_000, rx_bps: 300_000_000, utilization: 50, collected_at: '' },
      utilization: 50,
    };

    const result = buildEdgeData(link, devicesByID, existingData);

    expect(result.throughputLabel).toBe('TX: 500M / RX: 300M');
    expect(result.metrics).toEqual(existingData.metrics);
    expect(result.utilization).toBe(50);
  });

  it('virtual source: sourceIfStatus is undefined, targetIfStatus shows real status', () => {
    const source = mockDevice({
      id: 'dev-1',
      device_type: 'virtual',
      interfaces: [],
    });
    const target = mockDevice({
      id: 'dev-2',
      device_type: 'router',
      interfaces: [
        { id: 'if-2', if_index: 1, if_name: 'ether2', if_descr: 'ether2', speed: 1_000_000_000, admin_status: 'up', oper_status: 'up' },
      ],
    });
    const devicesByID = new Map([
      ['dev-1', source],
      ['dev-2', target],
    ]);
    const link = mockLink();

    const result = buildEdgeData(link, devicesByID);

    expect(result.sourceIfStatus).toBeUndefined();
    expect(result.targetIfStatus).toBe('up');
  });

  it('virtual target: targetIfStatus is undefined, sourceIfStatus shows real status', () => {
    const source = mockDevice({
      id: 'dev-1',
      device_type: 'router',
      interfaces: [
        { id: 'if-1', if_index: 1, if_name: 'ether1', if_descr: 'ether1', speed: 1_000_000_000, admin_status: 'up', oper_status: 'up' },
      ],
    });
    const target = mockDevice({
      id: 'dev-2',
      device_type: 'virtual',
      interfaces: [],
    });
    const devicesByID = new Map([
      ['dev-1', source],
      ['dev-2', target],
    ]);
    const link = mockLink();

    const result = buildEdgeData(link, devicesByID);

    expect(result.sourceIfStatus).toBe('up');
    expect(result.targetIfStatus).toBeUndefined();
  });
});
