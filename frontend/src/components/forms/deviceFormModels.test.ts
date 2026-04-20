import { describe, expect, it } from 'vitest';

import type { Device } from '../../types/api';
import {
  createAddDeviceFormModel,
  createDeviceConfigFormModel,
  resetDeviceFormMode,
} from './deviceFormModels';

function mockDevice(overrides: Partial<Device> = {}): Device {
  return {
    id: 'dev-1',
    hostname: 'router-01',
    ip: '10.0.0.1',
    notes: null,
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
    topology_discovery_mode: 'inherit',
    ...overrides,
  };
}

describe('deviceFormModels', () => {
  it('builds add-device UI state without backend payload shape leaking into local state', () => {
    const form = createAddDeviceFormModel();

    expect(form.mode).toBe('physical');
    expect(form.prometheus.labelName).toBe('instance');
    expect(form.virtual.subtype).toBe('internet');
  });

  it('initializes edit state from the current device without reusing the raw DTO as mutable state', () => {
    const form = createDeviceConfigFormModel(
      mockDevice({
        notes: 'rack A',
        metrics_source: 'prometheus_snmp_fallback',
        area_ids: ['area-1'],
      }),
      false,
    );

    expect(form.notes).toBe('rack A');
    expect(form.metricsMode).toBe('prometheus_snmp_fallback');
    expect(form.areaIds).toEqual(['area-1']);
  });

  it('does not inherit add-form v2c community defaults into edit state', () => {
    const form = createDeviceConfigFormModel(mockDevice({ metrics_source: 'snmp' }), false);

    expect(form.snmp.version).toBe('2c');
    expect(form.snmp.community).toBe('');
  });

  it('resets physical-only fields when switching to virtual mode', () => {
    const next = resetDeviceFormMode(
      {
        ...createAddDeviceFormModel(),
        hostname: '10.0.0.1',
        prometheus: { labelName: 'instance', labelValue: '10.0.0.1:9100' },
      },
      'virtual',
    );

    expect(next.mode).toBe('virtual');
    expect(next.hostname).toBe('');
    expect(next.prometheus.labelValue).toBe('');
  });
});
