import { describe, expect, it } from 'vitest';

import type { Device } from '../../types/api';
import {
  applySNMPProfile,
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

  it('applies revealed SNMP profile credentials to add-device state', () => {
    const form = createAddDeviceFormModel();
    const next = applySNMPProfile(form, {
      id: 'profile-1',
      name: 'Office',
      description: '',
      snmp: {
        version: '3',
        username: 'snmp-user',
        auth_protocol: 'SHA',
        auth_password: 'auth-pass',
        priv_protocol: 'AES',
        priv_password: 'priv-pass',
        security_level: 'authPriv',
        auth_password_set: true,
        priv_password_set: true,
      },
      created_at: '',
      updated_at: '',
    });

    expect(next.snmp.version).toBe('3');
    expect(next.snmp.username).toBe('snmp-user');
    expect(next.snmp.authPassword).toBe('auth-pass');
    expect(next.snmp.privPassword).toBe('priv-pass');
  });

  it('does not overwrite current secrets from redacted SNMP profile metadata', () => {
    const form = {
      ...createAddDeviceFormModel(),
      snmp: {
        ...createAddDeviceFormModel().snmp,
        community: 'current-community',
        authPassword: 'current-auth',
        privPassword: 'current-priv',
      },
    };
    const next = applySNMPProfile(form, {
      id: 'profile-1',
      name: 'Office',
      description: '',
      snmp: {
        version: '2c',
        community_set: true,
        auth_password_set: false,
        priv_password_set: false,
      },
      created_at: '',
      updated_at: '',
    });

    expect(next.snmp.community).toBe('current-community');
    expect(next.snmp.authPassword).toBe('current-auth');
    expect(next.snmp.privPassword).toBe('current-priv');
  });
});
