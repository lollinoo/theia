import { describe, expect, it } from 'vitest';

import type { Device, Link } from '../../types/api';
import type { AlertDTO, PrometheusStatusPayload, SnapshotPayload } from '../../types/metrics';
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
        cpu_percent: 17,
        mem_percent: 55,
        uptime_secs: 900,
        collected_at: '2026-04-20T12:00:00Z',
        health: 'warning',
      },
      'dev-2': {
        device_id: 'dev-2',
        cpu_percent: 9,
        mem_percent: 34,
        uptime_secs: 1200,
        collected_at: '2026-04-20T12:00:00Z',
        health: 'healthy',
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

describe('buildRuntimeState', () => {
  it('forces Prometheus and fallback devices down during an enabled outage', () => {
    const devices = [
      mockDevice(),
      mockDevice({
        id: 'dev-2',
        hostname: 'switch-01',
        ip: '10.0.0.2',
        sys_name: 'switch-01',
        metrics_source: 'prometheus_snmp_fallback',
      }),
      mockDevice({
        id: 'dev-3',
        hostname: 'ap-01',
        ip: '10.0.0.3',
        sys_name: 'ap-01',
        metrics_source: 'snmp',
      }),
    ];

    const runtime = buildRuntimeState({
      devices,
      links: [mockLink({ target_device_id: 'dev-2' })],
      snapshot: mockSnapshot({
        device_statuses: { 'dev-1': 'up', 'dev-2': 'up', 'dev-3': 'up' },
      }),
      alerts: [],
      prometheusStatus: { enabled: true, available: false },
    });

    expect(runtime.prometheusDown).toBe(true);
    expect(runtime.devicesById.get('dev-1')?.device.status).toBe('down');
    expect(runtime.devicesById.get('dev-2')?.device.status).toBe('down');
    expect(runtime.devicesById.get('dev-3')?.device.status).toBe('up');
    expect(runtime.devicesById.get('dev-1')?.prometheusOutageMode).toBe('offline');
    expect(runtime.devicesById.get('dev-2')?.prometheusOutageMode).toBe('fallback');
    expect(runtime.devicesById.get('dev-3')?.prometheusOutageMode).toBe('none');
  });

  it('keeps normalized device metrics in the runtime model', () => {
    const runtime = buildRuntimeState({
      devices: [mockDevice()],
      links: [],
      snapshot: mockSnapshot({
        device_metrics: {
          'dev-1': {
            device_id: 'dev-1',
            cpu_percent: 17,
            mem_percent: 55,
            uptime_secs: 900,
            collected_at: '2026-04-20T12:00:00Z',
            health: 'warning',
          },
        },
      }),
      alerts: [],
      prometheusStatus: null,
    });

    expect(runtime.devicesById.get('dev-1')?.metrics).toEqual({
      device_id: 'dev-1',
      cpu_percent: 17,
      mem_percent: 55,
      uptime_secs: 900,
      collected_at: '2026-04-20T12:00:00Z',
      health: 'warning',
      temp_celsius: null,
      last_polled_at: '2026-04-20T12:00:00Z',
      expected_poll_interval_seconds: 60,
    });
  });

  it('derives usable link telemetry fields from snapshot metrics', () => {
    const runtime = buildRuntimeState({
      devices: [
        mockDevice(),
        mockDevice({
          id: 'dev-2',
          hostname: 'switch-01',
          ip: '10.0.0.2',
          sys_name: 'switch-01',
        }),
      ],
      links: [mockLink({ target_device_id: 'dev-2' })],
      snapshot: mockSnapshot(),
      alerts: [],
      prometheusStatus: null,
    });

    expect(runtime.linksById.get('link-1')).toMatchObject({
      sourceMetrics: {
        device_id: 'dev-1',
        if_name: 'ether1',
        tx_bps: 1200,
        rx_bps: 2400,
        utilization: 0.15,
        collected_at: '2026-04-20T12:00:00Z',
      },
      targetMetrics: null,
      metrics: {
        device_id: 'dev-1',
        if_name: 'ether1',
        tx_bps: 1200,
        rx_bps: 2400,
        utilization: 0.15,
        collected_at: '2026-04-20T12:00:00Z',
      },
      metricsUsable: true,
      throughputLabel: 'TX: 1K / RX: 2K',
      utilization: 0.15,
    });
  });

  it('preserves source and target endpoint metrics separately when both are present', () => {
    const runtime = buildRuntimeState({
      devices: [
        mockDevice(),
        mockDevice({
          id: 'dev-2',
          hostname: 'switch-01',
          ip: '10.0.0.2',
          sys_name: 'switch-01',
        }),
      ],
      links: [mockLink({ target_device_id: 'dev-2' })],
      snapshot: mockSnapshot({
        link_metrics: {
          'dev-1': [{
            device_id: 'dev-1',
            if_name: 'ether1',
            tx_bps: 1500,
            rx_bps: 2500,
            utilization: 0.42,
            collected_at: '2026-04-20T12:00:00Z',
          }],
          'dev-2': [{
            device_id: 'dev-2',
            if_name: 'ether2',
            tx_bps: 3500,
            rx_bps: 4500,
            utilization: 0.91,
            collected_at: '2026-04-20T12:00:00Z',
          }],
        },
      }),
      alerts: [],
      prometheusStatus: null,
    });

    expect(runtime.linksById.get('link-1')).toMatchObject({
      sourceMetrics: {
        device_id: 'dev-1',
        if_name: 'ether1',
        tx_bps: 1500,
        rx_bps: 2500,
        utilization: 0.42,
        collected_at: '2026-04-20T12:00:00Z',
      },
      targetMetrics: {
        device_id: 'dev-2',
        if_name: 'ether2',
        tx_bps: 3500,
        rx_bps: 4500,
        utilization: 0.91,
        collected_at: '2026-04-20T12:00:00Z',
      },
      metrics: {
        device_id: 'dev-1',
        if_name: 'ether1',
        tx_bps: 1500,
        rx_bps: 2500,
        utilization: 0.42,
        collected_at: '2026-04-20T12:00:00Z',
      },
      metricsUsable: true,
      throughputLabel: 'TX: 2K / RX: 3K',
      utilization: 0.42,
    });
  });

  it('falls back to target-side link metrics with normalized interface names', () => {
    const runtime = buildRuntimeState({
      devices: [
        mockDevice({
          interfaces: [],
        }),
        mockDevice({
          id: 'dev-2',
          hostname: 'switch-01',
          ip: '10.0.0.2',
          sys_name: 'switch-01',
        }),
      ],
      links: [mockLink({
        source_if_name: 'missing0',
        target_device_id: 'dev-2',
        target_if_name: ' Ether2 ',
      })],
      snapshot: mockSnapshot({
        link_metrics: {
          'dev-1': [],
          'dev-2': [{
            device_id: 'dev-2',
            if_name: 'ether2',
            tx_bps: 5000,
            rx_bps: 7000,
            utilization: 0.42,
            collected_at: '2026-04-20T12:00:00Z',
          }],
        },
      }),
      alerts: [],
      prometheusStatus: null,
    });

    expect(runtime.linksById.get('link-1')).toMatchObject({
      metrics: {
        device_id: 'dev-2',
        if_name: 'ether2',
        tx_bps: 5000,
        rx_bps: 7000,
        utilization: 0.42,
        collected_at: '2026-04-20T12:00:00Z',
      },
      metricsUsable: true,
      throughputLabel: 'TX: 5K / RX: 7K',
      utilization: 0.42,
    });
  });

  it('drops link telemetry when both endpoints are effectively down', () => {
    const devices = [
      mockDevice(),
      mockDevice({
        id: 'dev-2',
        hostname: 'switch-01',
        ip: '10.0.0.2',
        sys_name: 'switch-01',
      }),
    ];

    const runtime = buildRuntimeState({
      devices,
      links: [mockLink({ target_device_id: 'dev-2' })],
      snapshot: mockSnapshot({
        device_statuses: { 'dev-1': 'down', 'dev-2': 'down' },
      }),
      alerts: [{
        device_id: 'dev-1',
        alert_name: 'DeviceDown',
        severity: 'critical',
        state: 'firing',
        summary: 'router unreachable',
      } satisfies AlertDTO],
      prometheusStatus: null,
    });

    expect(runtime.linksById.get('link-1')?.metrics).toBeNull();
    expect(runtime.linksById.get('link-1')?.metricsUsable).toBe(false);
    expect(runtime.devicesById.get('dev-1')?.alertStatus).toBe('down');
  });

  it('ignores invalid snapshot device statuses', () => {
    const runtime = buildRuntimeState({
      devices: [mockDevice({ status: 'up' })],
      links: [],
      snapshot: mockSnapshot({
        device_statuses: { 'dev-1': 'definitely-bad-status' },
      }),
      alerts: [],
      prometheusStatus: null,
    });

    expect(runtime.devicesById.get('dev-1')?.device.status).toBe('up');
  });

  it('does not apply outage semantics when Prometheus is disabled', () => {
    const runtime = buildRuntimeState({
      devices: [mockDevice()],
      links: [],
      snapshot: mockSnapshot(),
      alerts: [],
      prometheusStatus: { enabled: false, available: false } satisfies PrometheusStatusPayload,
    });

    expect(runtime.prometheusDown).toBe(false);
    expect(runtime.devicesById.get('dev-1')?.device.status).toBe('up');
  });

  it('indexes snapshot interface metrics even without a topology link', () => {
    const runtime = buildRuntimeState({
      devices: [mockDevice()],
      links: [],
      snapshot: mockSnapshot({
        link_metrics: {
          'dev-1': [{
            device_id: 'dev-1',
            if_name: 'ether7',
            tx_bps: 7_500,
            rx_bps: 8_500,
            utilization: 0.11,
            collected_at: '2026-04-20T12:00:00Z',
          }],
        },
      }),
      alerts: [],
      prometheusStatus: null,
    });

    expect(runtime.interfaceMetricsByDeviceId.get('dev-1')?.get('ether7')).toEqual({
      device_id: 'dev-1',
      if_name: 'ether7',
      tx_bps: 7_500,
      rx_bps: 8_500,
      utilization: 0.11,
      collected_at: '2026-04-20T12:00:00Z',
    });
  });
});
