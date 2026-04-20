import { describe, expect, it } from 'vitest';

import type { Device } from '../../types/api';
import type { SnapshotPayload } from '../../types/metrics';
import { buildRuntimeDeviceRows, computeAreaHealthSummary } from './runtimeDeviceRows';

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
    sys_descr: 'RouterOS 7.14.3',
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

describe('runtimeDeviceRows', () => {
  it('builds row uptime and status from runtime-aware devices plus snapshot metrics', () => {
    const rows = buildRuntimeDeviceRows({
      devices: [mockDevice({ status: 'down' })],
      snapshot: {
        device_metrics: {
          'dev-1': {
            device_id: 'dev-1',
            cpu_percent: 10,
            mem_percent: 22,
            uptime_secs: 7200,
            collected_at: '2026-04-20T12:00:00Z',
          },
        },
        link_metrics: {},
        device_statuses: { 'dev-1': 'down' },
      } satisfies SnapshotPayload,
    });

    expect(rows[0]?.statusState.label).toBe('Down');
    expect(rows[0]?.uptimeLabel).toBeNull();
  });

  it('uses snapshot metrics without overriding the incoming runtime-aware status', () => {
    const rows = buildRuntimeDeviceRows({
      devices: [mockDevice({ status: 'down' })],
      snapshot: {
        device_metrics: {
          'dev-1': {
            device_id: 'dev-1',
            cpu_percent: 10,
            mem_percent: 22,
            uptime_secs: 7200,
            collected_at: '2026-04-20T12:00:00Z',
          },
        },
        link_metrics: {},
        device_statuses: { 'dev-1': 'up' },
      } satisfies SnapshotPayload,
    });

    expect(rows[0]?.statusState.label).toBe('Down');
    expect(rows[0]?.uptimeLabel).toBeNull();
  });

  it('preserves incoming runtime-aware status when snapshot status disagrees', () => {
    const rows = buildRuntimeDeviceRows({
      devices: [mockDevice({ status: 'down' })],
      snapshot: {
        device_metrics: {
          'dev-1': {
            device_id: 'dev-1',
            cpu_percent: 10,
            mem_percent: 22,
            uptime_secs: 7200,
            collected_at: '2026-04-20T12:00:00Z',
          },
        },
        link_metrics: {},
        device_statuses: { 'dev-1': 'up' },
      } satisfies SnapshotPayload,
    });

    expect(rows[0]?.device.status).toBe('down');
    expect(rows[0]?.statusState.label).toBe('Down');
    expect(rows[0]?.uptimeLabel).toBeNull();
  });

  it('builds presentation and sort fields on the row model', () => {
    const rows = buildRuntimeDeviceRows({
      devices: [mockDevice({
        hostname: 'edge-01',
        ip: '10.10.10.1',
        device_type: 'switch',
        sys_name: 'edge-core-01',
        sys_descr: 'RouterOS RB5009 7.15.1',
        hardware_model: 'Unknown',
        vendor: 'mikrotik',
        area_ids: ['area-1'],
      })],
      snapshot: null,
    });

    expect(rows[0]).toMatchObject({
      id: 'dev-1',
      hostname: 'edge-01',
      ip: '10.10.10.1',
      deviceType: 'switch',
      sysName: 'edge-core-01',
      vendor: 'mikrotik',
      areaIds: ['area-1'],
      modelLabel: 'RouterOS RB5009 7.15.1',
      areaSortName: '',
      statusSortLabel: 'up',
      searchText: 'edge-01 10.10.10.1 edge-core-01 edge-core-01',
    });
    expect(rows[0]?.osVersion).toBe('RouterOS 7.15.1');
  });

  it('computes aggregate area health from runtime-aware status labels only', () => {
    const summary = computeAreaHealthSummary([
      { statusState: { label: 'Up', dotStatus: 'up', labelClass: '' } },
      { statusState: { label: 'Up', dotStatus: 'up', labelClass: '' } },
      { statusState: { label: 'Down', dotStatus: 'down', labelClass: '' } },
    ]);

    expect(summary.percentage).toBeCloseTo(66.67, 1);
    expect(summary.label).toBe('Critical');
  });
});
