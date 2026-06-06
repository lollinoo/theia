/**
 * Exercises device visual state component behavior so refactors preserve the documented contract.
 */
import { describe, expect, it } from 'vitest';

import type { Device } from '../types/api';
import type { DeviceMetricsDTO } from '../types/metrics';
import {
  minimapColorForDevice,
  resolveDeviceMonitoringState,
  resolveDeviceNodeStatusStyles,
  resolveDeviceStatusDotStyles,
  resolveDeviceVisualState,
  sanitizeDeviceMetricsForDisplay,
} from './deviceVisualState';

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

function mockMetrics(overrides: Partial<DeviceMetricsDTO> = {}): DeviceMetricsDTO {
  return {
    device_id: 'dev-1',
    operational_status: 'up',
    primary_health: 'up_fresh',
    runtime_flags: [],
    field_states: { uptime: 'missing', cpu: 'ok', memory: 'ok' },
    network_reachable: 'true',
    snmp_reachable: 'true',
    reachability: 'up',
    cpu_percent: 42,
    mem_percent: 68,
    health: 'healthy',
    freshness: 'fresh',
    primary_reason: 'ok',
    metrics_status: 'available',
    metrics_reason: 'ok',
    alert_status: 'normal',
    firing_alert_count: 0,
    last_collected_at: '2026-04-13T11:59:45Z',
    last_polled_at: '2026-04-13T11:59:45Z',
    expected_poll_interval_seconds: 30,
    temp_celsius: null,
    uptime_secs: null,
    ...overrides,
  };
}

describe('deviceVisualState', () => {
  it('maps warning health on up devices to degraded minimap status', () => {
    const device = mockDevice();
    const metrics = mockMetrics({ health: 'warning' });

    expect(resolveDeviceVisualState(device, metrics)).toMatchObject({
      dotStatus: 'degraded',
      label: 'Warning',
    });
    expect(minimapColorForDevice({ device, metrics })).toBe('var(--color-status-probing)');
  });

  it('maps critical health on up devices to a dedicated critical minimap status', () => {
    const device = mockDevice();
    const metrics = mockMetrics({ health: 'critical' });

    expect(resolveDeviceVisualState(device, metrics)).toMatchObject({
      dotStatus: 'critical',
      label: 'Critical',
    });
    expect(minimapColorForDevice({ device, metrics })).toBe('var(--color-status-critical)');
  });

  it('uses primary health when reachable SNMP devices report unknown legacy health', () => {
    const device = mockDevice({ status: 'up' });
    const metrics = mockMetrics({
      operational_status: 'up',
      primary_health: 'snmp_degraded',
      field_states: { uptime: 'error', cpu: 'missing', memory: 'missing' },
      network_reachable: 'true',
      snmp_reachable: 'false',
      reachability: 'up',
      health: 'unknown',
      metrics_status: 'unavailable',
      metrics_reason: 'no_data',
      cpu_percent: null,
      mem_percent: null,
      uptime_secs: null,
    });

    expect(resolveDeviceVisualState(device, metrics)).toMatchObject({
      dotStatus: 'degraded',
      label: 'Warning',
    });
    expect(minimapColorForDevice({ device, metrics })).toBe('var(--color-status-probing)');
  });

  it('uses SNMP reachability before cached healthy metrics', () => {
    const device = mockDevice({ status: 'up' });
    const metrics = mockMetrics({
      operational_status: 'up',
      primary_health: 'snmp_degraded',
      field_states: { uptime: 'error', cpu: 'ok', memory: 'ok' },
      network_reachable: 'true',
      snmp_reachable: 'false',
      reachability: 'up',
      health: 'healthy',
      metrics_status: 'available',
      metrics_reason: 'ok',
      cpu_percent: 0,
      mem_percent: 4.5,
      uptime_secs: 15143983,
    });

    expect(resolveDeviceVisualState(device, metrics)).toMatchObject({
      dotStatus: 'degraded',
      label: 'Warning',
    });
    expect(minimapColorForDevice({ device, metrics })).toBe('var(--color-status-probing)');
  });

  it('uses primary up health when reachable SNMP devices only have partial telemetry', () => {
    const device = mockDevice({ status: 'up' });
    const metrics = mockMetrics({
      operational_status: 'up',
      primary_health: 'up_fresh',
      field_states: { uptime: 'ok', cpu: 'missing', memory: 'missing' },
      network_reachable: 'true',
      snmp_reachable: 'true',
      reachability: 'up',
      health: 'unknown',
      metrics_status: 'partial',
      metrics_reason: 'ok',
      cpu_percent: null,
      mem_percent: null,
      uptime_secs: 12345,
    });

    expect(resolveDeviceVisualState(device, metrics)).toMatchObject({
      dotStatus: 'up',
      label: 'Up',
    });
    expect(minimapColorForDevice({ device, metrics })).toBe('var(--color-status-up)');
  });

  it('keeps down devices on the dedicated down color', () => {
    const device = mockDevice({ status: 'down' });
    const metrics = mockMetrics({ health: 'critical' });

    expect(resolveDeviceVisualState(device, metrics)).toMatchObject({
      dotStatus: 'down',
      label: 'Down',
    });
    expect(minimapColorForDevice({ device, metrics })).toBe('var(--color-status-down)');
  });

  it('uses runtime operational down before unknown metric health', () => {
    const device = mockDevice({ status: 'up' });
    const metrics = mockMetrics({
      operational_status: 'down',
      primary_health: 'unreachable',
      network_reachable: 'false',
      snmp_reachable: 'false',
      reachability: 'hard_down',
      health: 'unknown',
      metrics_status: 'unavailable',
      metrics_reason: 'device_unreachable',
      cpu_percent: null,
      mem_percent: null,
      uptime_secs: null,
    });

    expect(resolveDeviceVisualState(device, metrics)).toMatchObject({
      dotStatus: 'down',
      label: 'Down',
    });
    expect(minimapColorForDevice({ device, metrics })).toBe('var(--color-status-down)');
  });

  it('gives down nodes a dedicated frame glow and card fill without reusing them for critical', () => {
    expect(resolveDeviceNodeStatusStyles({ status: 'down' }).frameStyle.boxShadow).toContain(
      'var(--nt-node-down-ring)',
    );
    expect(resolveDeviceNodeStatusStyles({ status: 'down' }).frameStyle.boxShadow).toContain(
      'var(--nt-node-down-glow)',
    );
    expect(resolveDeviceNodeStatusStyles({ status: 'down' }).frameStyle.backgroundColor).toBe(
      'var(--nt-node-down-card-bg)',
    );
    expect(resolveDeviceNodeStatusStyles({ status: 'down' }).frameClass).toBe(
      'topology-node-down-pulse',
    );
    expect(
      resolveDeviceNodeStatusStyles({ status: 'critical' }).frameStyle.boxShadow,
    ).not.toContain('var(--nt-node-down-ring)');
    expect(
      resolveDeviceNodeStatusStyles({ status: 'critical' }).frameStyle.boxShadow,
    ).not.toContain('var(--nt-node-down-glow)');
    expect(resolveDeviceNodeStatusStyles({ status: 'critical' }).frameStyle.backgroundColor).toBe(
      undefined,
    );
    expect(resolveDeviceNodeStatusStyles({ status: 'critical' }).frameClass).toBeUndefined();
  });

  it('preserves the down glow when selected so failure semantics stay visible', () => {
    const selectedDown = resolveDeviceNodeStatusStyles({ status: 'down', selected: true });

    expect(selectedDown.frameStyle.boxShadow).toContain('var(--color-focus-ring)');
    expect(selectedDown.frameStyle.boxShadow).toContain('var(--nt-node-down-glow)');
    expect(selectedDown.frameStyle.backgroundColor).toBe('var(--nt-node-down-card-bg)');
  });

  it('keeps critical dots static while down dots keep the stronger active emphasis', () => {
    expect(resolveDeviceStatusDotStyles('critical').className).not.toContain('animate-pulse');
    expect(resolveDeviceStatusDotStyles('down').className).toContain('animate-pulse');
  });

  it('keeps virtual nodes without IP inert instead of rendering them offline', () => {
    const device = mockDevice({
      device_type: 'virtual',
      ip: '',
      status: 'down',
    });
    const metrics = mockMetrics({ health: 'critical' });

    expect(resolveDeviceVisualState(device, metrics)).toMatchObject({
      dotStatus: 'unmonitored',
      label: 'Unmonitored',
    });
    expect(resolveDeviceMonitoringState(device)).toBe('unmonitored');
    expect(sanitizeDeviceMetricsForDisplay(device, metrics)).toBeNull();
    expect(minimapColorForDevice({ device, metrics })).toBe('var(--nt-on-bg-muted)');
  });

  it('renders polling-disabled devices as muted and suppresses cached metrics', () => {
    const device = mockDevice({ polling_enabled: false });
    const metrics = mockMetrics({
      cpu_percent: 42,
      mem_percent: 68,
      uptime_secs: 86400,
      health: 'healthy',
      freshness: 'fresh',
    });

    expect(resolveDeviceVisualState(device, metrics)).toMatchObject({
      dotStatus: 'polling_disabled',
      label: 'Polling off',
    });
    expect(sanitizeDeviceMetricsForDisplay(device, metrics)).toBeNull();
    expect(minimapColorForDevice({ device, metrics })).toBe('var(--nt-on-bg-muted)');
  });

  it('uses operational status directly for virtual nodes with IP even without health metrics', () => {
    const device = mockDevice({
      device_type: 'virtual',
      ip: '127.0.0.1',
      status: 'up',
    });
    const metrics = mockMetrics({ health: undefined });

    expect(resolveDeviceVisualState(device, metrics)).toMatchObject({
      dotStatus: 'up',
      label: 'Up',
    });
  });

  it('preserves normalized runtime cadence fields without backfilling from inventory', () => {
    const device = mockDevice({ poll_class: 'core', poll_interval_override: 15 });

    expect(sanitizeDeviceMetricsForDisplay(device, mockMetrics())).toMatchObject({
      temp_celsius: null,
      uptime_secs: null,
      last_polled_at: '2026-04-13T11:59:45Z',
      expected_poll_interval_seconds: 30,
    });
  });

  it('does not infer last_polled_at from last_collected_at when runtime omits it', () => {
    const device = mockDevice({ poll_class: 'core', poll_interval_override: 15 });

    expect(
      sanitizeDeviceMetricsForDisplay(
        device,
        mockMetrics({
          expected_poll_interval_seconds: null,
          last_polled_at: null,
        }),
      ),
    ).toMatchObject({
      last_polled_at: null,
      expected_poll_interval_seconds: null,
    });
  });
});
