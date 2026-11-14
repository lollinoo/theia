/**
 * Test fixtures for canvas models (Device, Link) shared across canvas unit tests.
 */
import type { Device, Link } from '../types/api';

/**
 * Builds a mock Device with standard defaults and optional overrides or custom id.
 */
export function mockDevice(
  idOrOverrides?: string | Partial<Device>,
  maybeOverrides?: Partial<Device>,
): Device {
  if (typeof idOrOverrides === 'string') {
    const id = idOrOverrides;
    const ipSuffix = id.slice(-1);
    return {
      id,
      hostname: id,
      ip: `10.0.0.${ipSuffix}`,
      addresses: [],
      probe_ports: null,
      device_type: 'router',
      poll_class: 'standard',
      poll_interval_override: null,
      polling_enabled: true,
      status: 'up',
      sys_name: id,
      sys_descr: 'RouterOS',
      hardware_model: 'RB4011',
      vendor: 'mikrotik',
      managed: true,
      interfaces: [],
      area_ids: [],
      backup_supported: true,
      metrics_source: 'prometheus',
      prometheus_label_name: 'instance',
      prometheus_label_value: `10.0.0.${ipSuffix}:9100`,
      ...maybeOverrides,
    };
  }

  return {
    id: 'dev-1',
    hostname: 'router-01',
    ip: '10.0.0.1',
    addresses: [],
    probe_ports: null,
    device_type: 'router',
    poll_class: 'standard',
    poll_interval_override: null,
    polling_enabled: true,
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
    ...idOrOverrides,
  };
}

/**
 * Builds a mock Link with standard defaults and optional overrides or positional arguments.
 */
export function mockLink(
  idOrOverrides?: string | Partial<Link>,
  sourceDeviceId?: string,
  targetDeviceId?: string,
  overrides?: Partial<Link>,
): Link {
  if (typeof idOrOverrides === 'string') {
    return {
      id: idOrOverrides,
      source_device_id: sourceDeviceId ?? 'dev-1',
      source_if_name: 'ether1',
      source_if_speed: 1_000_000_000,
      source_if_oper_status: 'up',
      target_device_id: targetDeviceId ?? 'dev-2',
      target_if_name: 'ether2',
      target_if_speed: 1_000_000_000,
      target_if_oper_status: 'up',
      discovery_protocol: 'lldp',
      ...overrides,
    };
  }

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
    ...idOrOverrides,
  };
}
