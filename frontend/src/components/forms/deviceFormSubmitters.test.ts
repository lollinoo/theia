/**
 * Exercises device form submitters component behavior so refactors preserve the documented contract.
 */
import { describe, expect, it } from 'vitest';

import type { Device } from '../../types/api';
import { createAddDeviceFormModel, createDeviceConfigFormModel } from './deviceFormModels';
import { buildCreateDevicePayload, buildUpdateDevicePayload } from './deviceFormSubmitters';

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

describe('deviceFormSubmitters', () => {
  it('maps add-device UI state back to the current create payload contract', () => {
    const payload = buildCreateDevicePayload({
      ...createAddDeviceFormModel(),
      hostname: '10.0.0.1',
      ip: '10.0.0.1',
      metricsMode: 'prometheus',
      prometheus: { labelName: 'instance', labelValue: '' },
    });

    expect(payload).toMatchObject({
      hostname: '10.0.0.1',
      ip: '10.0.0.1',
      metrics_source: 'prometheus',
      prometheus_label_name: 'instance',
      prometheus_label_value: '10.0.0.1',
    });
  });

  it('maps edit-device UI state back to the current update payload contract', () => {
    const payload = buildUpdateDevicePayload(mockDevice(), {
      ...createDeviceConfigFormModel(mockDevice(), false),
      notes: 'rack A',
      vendor: 'mikrotik',
      metricsMode: 'snmp',
    });

    expect(payload).toMatchObject({
      hostname: 'router-01',
      ip: '10.0.0.1',
      notes: 'rack A',
      vendor: 'mikrotik',
      metrics_source: 'snmp',
    });
    expect(payload).not.toHaveProperty('prometheus_label_name');
  });

  it('sends blank physical display name and vendor when clearing existing values', () => {
    const device = mockDevice({
      vendor: 'mikrotik',
      tags: { display_name: 'Core Router', owner: 'netops' },
    });

    const payload = buildUpdateDevicePayload(device, {
      ...createDeviceConfigFormModel(device, false),
      displayName: '',
      vendor: '',
    });

    expect(payload.vendor).toBe('');
    expect(payload.tags).toEqual({ display_name: '', owner: 'netops' });
  });

  it('does not send v2c SNMP credentials for unchanged edit forms', () => {
    const payload = buildUpdateDevicePayload(
      mockDevice({ metrics_source: 'snmp' }),
      createDeviceConfigFormModel(mockDevice({ metrics_source: 'snmp' }), false),
    );

    expect(payload).not.toHaveProperty('snmp');
  });

  it('only includes SNMPv3 fields required by the selected security level', () => {
    const noAuthPayload = buildCreateDevicePayload({
      ...createAddDeviceFormModel(),
      hostname: '10.0.0.1',
      ip: '10.0.0.1',
      snmp: {
        ...createAddDeviceFormModel().snmp,
        version: '3',
        username: 'snmp-user',
        securityLevel: 'noAuthNoPriv',
        authProtocol: 'SHA',
        authPassword: 'auth-secret',
        privProtocol: 'AES',
        privPassword: 'priv-secret',
      },
    });

    const authNoPrivPayload = buildCreateDevicePayload({
      ...createAddDeviceFormModel(),
      hostname: '10.0.0.1',
      ip: '10.0.0.1',
      snmp: {
        ...createAddDeviceFormModel().snmp,
        version: '3',
        username: 'snmp-user',
        securityLevel: 'authNoPriv',
        authProtocol: 'SHA',
        authPassword: 'auth-secret',
        privProtocol: 'AES',
        privPassword: 'priv-secret',
      },
    });

    expect(noAuthPayload.snmp).toMatchObject({
      version: '3',
      username: 'snmp-user',
      security_level: 'noAuthNoPriv',
    });
    expect(noAuthPayload.snmp).not.toHaveProperty('auth_protocol');
    expect(noAuthPayload.snmp).not.toHaveProperty('auth_password');
    expect(noAuthPayload.snmp).not.toHaveProperty('priv_protocol');
    expect(noAuthPayload.snmp).not.toHaveProperty('priv_password');

    expect(authNoPrivPayload.snmp).toMatchObject({
      version: '3',
      username: 'snmp-user',
      security_level: 'authNoPriv',
      auth_protocol: 'SHA',
      auth_password: 'auth-secret',
    });
    expect(authNoPrivPayload.snmp).not.toHaveProperty('priv_protocol');
    expect(authNoPrivPayload.snmp).not.toHaveProperty('priv_password');
  });
});
