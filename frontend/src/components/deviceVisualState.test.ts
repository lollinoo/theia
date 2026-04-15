import { describe, expect, it } from 'vitest';

import type { Device } from '../types/api';
import type { DeviceMetricsDTO } from '../types/metrics';
import { minimapColorForDevice, resolveDeviceVisualState } from './deviceVisualState';

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
    cpu_percent: 42,
    mem_percent: 68,
    temp_celsius: 55,
    uptime_secs: 86400,
    collected_at: '2026-04-13T11:59:45Z',
    health: 'healthy',
    stale: false,
    last_polled_at: '2026-04-13T11:59:30Z',
    expected_poll_interval_seconds: 30,
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

  it('keeps down devices on the dedicated down color', () => {
    const device = mockDevice({ status: 'down' });
    const metrics = mockMetrics({ health: 'critical' });

    expect(resolveDeviceVisualState(device, metrics)).toMatchObject({
      dotStatus: 'down',
      label: 'Down',
    });
    expect(minimapColorForDevice({ device, metrics })).toBe('var(--color-status-down)');
  });

  it('keeps virtual nodes without IP inert instead of rendering them offline', () => {
    const device = mockDevice({
      device_type: 'virtual',
      ip: '',
      status: 'down',
    });
    const metrics = mockMetrics({ health: 'critical' });

    expect(resolveDeviceVisualState(device, metrics)).toMatchObject({
      dotStatus: 'unknown',
      label: 'Unknown',
    });
    expect(minimapColorForDevice({ device, metrics })).toBe('var(--color-status-unknown)');
  });
});
