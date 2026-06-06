/**
 * Exercises runtime adapters topology canvas behavior so refactors preserve the documented contract.
 */
import { describe, expect, it } from 'vitest';

import type { Device, Link } from '../../types/api';
import type { AlertDTO, PrometheusStatusPayload, SnapshotPayload } from '../../types/metrics';
import { buildRuntimeState, primaryHealthPriority } from './runtimeAdapters';

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
        cpu_percent: 17,
        mem_percent: 55,
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
        cpu_percent: 9,
        mem_percent: 34,
        temp_celsius: null,
        uptime_secs: 1200,
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

describe('buildRuntimeState', () => {
  it('uses normalized device runtime even during a Prometheus outage', () => {
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
        devices: {
          ...mockSnapshot().devices,
          'dev-3': {
            device_id: 'dev-3',
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
            mem_percent: 24,
            temp_celsius: null,
            uptime_secs: 1800,
          },
        },
      }),
      alerts: [],
      prometheusStatus: { enabled: true, available: false },
    });

    expect(runtime.prometheusDown).toBe(true);
    expect(runtime.devicesById.get('dev-1')?.device.status).toBe('up');
    expect(runtime.devicesById.get('dev-2')?.device.status).toBe('up');
    expect(runtime.devicesById.get('dev-3')?.device.status).toBe('up');
  });

  it('keeps normalized device metrics in the runtime model', () => {
    const runtime = buildRuntimeState({
      devices: [mockDevice()],
      links: [],
      snapshot: mockSnapshot(),
      alerts: [],
      prometheusStatus: null,
    });

    expect(runtime.devicesById.get('dev-1')?.metrics).toEqual({
      device_id: 'dev-1',
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
      cpu_percent: 17,
      mem_percent: 55,
      temp_celsius: null,
      uptime_secs: 900,
    });
  });

  it('preserves the device object reference when normalized runtime status is unchanged', () => {
    const device = mockDevice({ status: 'up' });

    const runtime = buildRuntimeState({
      devices: [device],
      links: [],
      snapshot: mockSnapshot({
        devices: {
          'dev-1': {
            ...mockSnapshot().devices['dev-1'],
            operational_status: 'up',
          },
        },
      }),
      alerts: [],
      prometheusStatus: null,
    });

    expect(runtime.devicesById.get('dev-1')?.device).toBe(device);
  });

  it('exposes primary health and runtime flags on device models', () => {
    const runtime = buildRuntimeState({
      devices: [mockDevice()],
      links: [],
      snapshot: mockSnapshot({
        devices: {
          'dev-1': {
            ...mockSnapshot().devices['dev-1'],
            primary_health: 'snmp_degraded',
            runtime_flags: ['deadline_missed', 'partial_telemetry'],
          },
        },
      }),
      alerts: [],
      prometheusStatus: null,
    });

    expect(runtime.devicesById.get('dev-1')).toMatchObject({
      primaryHealth: 'snmp_degraded',
      runtimeFlags: ['deadline_missed', 'partial_telemetry'],
    });
  });

  it('orders primary health states from most urgent to least urgent', () => {
    expect([
      primaryHealthPriority('quarantined'),
      primaryHealthPriority('unreachable'),
      primaryHealthPriority('snmp_degraded'),
      primaryHealthPriority('up_stale'),
      primaryHealthPriority('up_fresh'),
      primaryHealthPriority('probing'),
      primaryHealthPriority(null),
    ]).toEqual([0, 1, 2, 3, 4, 5, 6]);
  });

  it('keeps shared link telemetry on the link model without inventing endpoint copies', () => {
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
      metrics: {
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
      metricsUsable: true,
      throughputLabel: 'TX: 1K / RX: 2K',
      utilization: 0.15,
    });
  });

  it('uses snapshot link runtime by link id instead of endpoint maps', () => {
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
        links: {
          'link-1': {
            link_id: 'link-1',
            source_device_id: 'dev-1',
            target_device_id: 'dev-2',
            source_if_name: 'ether1',
            target_if_name: 'ether2',
            metrics_status: 'partial',
            metrics_reason: 'ok',
            last_collected_at: '2026-04-20T12:00:00Z',
            tx_bps: 1500,
            rx_bps: 2500,
            utilization: 0.42,
          },
        },
      }),
      alerts: [],
      prometheusStatus: null,
    });

    expect(runtime.linksById.get('link-1')).toMatchObject({
      metrics: {
        link_id: 'link-1',
        source_device_id: 'dev-1',
        target_device_id: 'dev-2',
        source_if_name: 'ether1',
        target_if_name: 'ether2',
        metrics_status: 'partial',
        metrics_reason: 'ok',
        last_collected_at: '2026-04-20T12:00:00Z',
        tx_bps: 1500,
        rx_bps: 2500,
        utilization: 0.42,
      },
      metricsUsable: true,
      throughputLabel: 'TX: 2K / RX: 3K',
      utilization: 0.42,
    });
  });

  it('drops link telemetry when normalized link runtime marks metrics unavailable', () => {
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
        links: {
          'link-1': {
            link_id: 'link-1',
            source_device_id: 'dev-1',
            target_device_id: 'dev-2',
            source_if_name: 'ether1',
            target_if_name: 'ether2',
            metrics_status: 'unavailable',
            metrics_reason: 'upstream_unavailable',
            last_collected_at: null,
            tx_bps: null,
            rx_bps: null,
            utilization: null,
          },
        },
      }),
      alerts: [],
      prometheusStatus: null,
    });

    expect(runtime.linksById.get('link-1')).toMatchObject({
      metrics: null,
      metricsUsable: false,
      throughputLabel: undefined,
      utilization: null,
    });
  });

  it('keeps normalized link telemetry when both endpoints are effectively down', () => {
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
        devices: {
          'dev-1': {
            ...mockSnapshot().devices['dev-1'],
            operational_status: 'down',
            primary_reason: 'device_unreachable',
          },
          'dev-2': {
            ...mockSnapshot().devices['dev-2'],
            operational_status: 'down',
            primary_reason: 'device_unreachable',
          },
        },
      }),
      alerts: [
        {
          device_id: 'dev-1',
          alert_name: 'DeviceDown',
          severity: 'critical',
          state: 'firing',
          summary: 'router unreachable',
        } satisfies AlertDTO,
      ],
      prometheusStatus: null,
    });

    expect(runtime.linksById.get('link-1')?.metrics).toEqual(mockSnapshot().links['link-1']);
    expect(runtime.linksById.get('link-1')?.metricsUsable).toBe(true);
    expect(runtime.devicesById.get('dev-1')?.alertStatus).toBe('degraded');
  });

  it('trusts normalized link metrics_status even when both endpoints are down', () => {
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
        devices: {
          'dev-1': {
            ...mockSnapshot().devices['dev-1'],
            operational_status: 'down',
            primary_reason: 'device_unreachable',
          },
          'dev-2': {
            ...mockSnapshot().devices['dev-2'],
            operational_status: 'down',
            primary_reason: 'device_unreachable',
          },
        },
        links: {
          'link-1': {
            ...mockSnapshot().links['link-1'],
            metrics_status: 'available',
            metrics_reason: 'ok',
            tx_bps: 12_000,
            rx_bps: 24_000,
            utilization: 0.5,
          },
        },
      }),
      alerts: [],
      prometheusStatus: null,
    });

    expect(runtime.linksById.get('link-1')).toMatchObject({
      metricsUsable: true,
      throughputLabel: 'TX: 12K / RX: 24K',
      utilization: 0.5,
    });
  });

  it('prefers normalized alert state over the alert feed when runtime exists', () => {
    const runtime = buildRuntimeState({
      devices: [mockDevice()],
      links: [],
      snapshot: mockSnapshot({
        devices: {
          'dev-1': {
            ...mockSnapshot().devices['dev-1'],
            alert_status: 'normal',
            firing_alert_count: 0,
          },
        },
      }),
      alerts: [
        {
          device_id: 'dev-1',
          alert_name: 'DeviceDown',
          severity: 'critical',
          state: 'firing',
          summary: 'legacy alert feed still firing',
        } satisfies AlertDTO,
      ],
      prometheusStatus: null,
    });

    expect(runtime.devicesById.get('dev-1')?.alertStatus).toBe('normal');
  });

  it('preserves normalized unmonitored device state in the runtime model', () => {
    const runtime = buildRuntimeState({
      devices: [mockDevice()],
      links: [],
      snapshot: mockSnapshot({
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
      }),
      alerts: [],
      prometheusStatus: null,
    });

    expect(runtime.devicesById.get('dev-1')).toMatchObject({
      monitoringState: 'unmonitored',
      metrics: null,
      runtimeStatus: 'unmonitored',
    });
  });

  it('falls back to inventory status when normalized runtime omits a device', () => {
    const runtime = buildRuntimeState({
      devices: [mockDevice({ status: 'up' })],
      links: [],
      snapshot: { devices: {}, links: {} },
      alerts: [],
      prometheusStatus: null,
    });

    expect(runtime.devicesById.get('dev-1')?.device.status).toBe('up');
  });

  it('keeps Prometheus outage diagnostic state separate from runtime semantics when disabled', () => {
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

  it('indexes snapshot interface metrics for both source and target endpoints', () => {
    const runtime = buildRuntimeState({
      devices: [mockDevice()],
      links: [],
      snapshot: mockSnapshot({
        links: {
          'link-7': {
            link_id: 'link-7',
            source_device_id: 'dev-1',
            target_device_id: 'dev-9',
            source_if_name: 'ether7',
            target_if_name: 'ether8',
            metrics_status: 'available',
            metrics_reason: 'ok',
            last_collected_at: '2026-04-20T12:00:00Z',
            tx_bps: 7_500,
            rx_bps: 8_500,
            utilization: 0.11,
          },
        },
      }),
      alerts: [],
      prometheusStatus: null,
    });

    expect(runtime.interfaceMetricsByDeviceId.get('dev-1')?.get('ether7')).toEqual({
      link_id: 'link-7',
      source_device_id: 'dev-1',
      target_device_id: 'dev-9',
      source_if_name: 'ether7',
      target_if_name: 'ether8',
      metrics_status: 'available',
      metrics_reason: 'ok',
      last_collected_at: '2026-04-20T12:00:00Z',
      tx_bps: 7_500,
      rx_bps: 8_500,
      utilization: 0.11,
    });
    expect(runtime.interfaceMetricsByDeviceId.get('dev-9')?.get('ether8')).toEqual({
      link_id: 'link-7',
      source_device_id: 'dev-1',
      target_device_id: 'dev-9',
      source_if_name: 'ether7',
      target_if_name: 'ether8',
      metrics_status: 'available',
      metrics_reason: 'ok',
      last_collected_at: '2026-04-20T12:00:00Z',
      tx_bps: 7_500,
      rx_bps: 8_500,
      utilization: 0.11,
    });
  });

  it('does not index unusable normalized link telemetry into interface lookups', () => {
    const runtime = buildRuntimeState({
      devices: [mockDevice()],
      links: [],
      snapshot: mockSnapshot({
        links: {
          'link-7': {
            link_id: 'link-7',
            source_device_id: 'dev-1',
            target_device_id: 'dev-9',
            source_if_name: 'ether7',
            target_if_name: 'ether8',
            metrics_status: 'unavailable',
            metrics_reason: 'upstream_unavailable',
            last_collected_at: '2026-04-20T12:00:00Z',
            tx_bps: 7_500,
            rx_bps: 8_500,
            utilization: 0.11,
          },
        },
      }),
      alerts: [],
      prometheusStatus: null,
    });

    expect(runtime.interfaceMetricsByDeviceId.get('dev-1')?.get('ether7')).toBeUndefined();
  });
});
