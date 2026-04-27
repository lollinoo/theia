import { describe, expect, it } from 'vitest';

import { parseDevicesResponse } from './api';

function deviceResource(id: string, deviceType: string) {
  return {
    id,
    attributes: {
      hostname: `${id}.example.test`,
      ip: `10.0.0.${id === 'ap-1' ? '1' : '2'}`,
      notes: id === 'ap-1' ? 'Managed by NOC' : null,
      device_type: deviceType,
      status: 'up',
      sys_name: `${id}.example.test`,
      sys_descr: 'Test device',
      hardware_model: 'Model X',
      vendor: 'mikrotik',
      managed: true,
      backup_supported: true,
      poll_class: 'standard',
      poll_interval_override: null,
      polling_enabled: true,
      metrics_source: 'prometheus',
      prometheus_label_name: 'instance',
      prometheus_label_value: `${id}.example.test:9100`,
      topology_discovery_mode: 'inherit',
      effective_topology_discovery_mode: 'off',
      topology_bootstrap_state: 'idle',
      last_topology_discovery_at: null,
      last_topology_discovery_result: '',
    },
    relationships: {
      interfaces: {
        data: [],
      },
    },
  };
}

describe('parseDevicesResponse', () => {
  it('maps backend access-point values to ap and preserves firewall devices', () => {
    const devices = parseDevicesResponse({
      data: [deviceResource('ap-1', 'access_point'), deviceResource('fw-1', 'firewall')],
    });

    expect(devices[0].device_type).toBe('ap');
    expect(devices[1].device_type).toBe('firewall');
    expect(devices[0].notes).toBe('Managed by NOC');
    expect(devices[1].notes).toBeNull();
  });

  it('parses topology discovery fields from the device payload', () => {
    const devices = parseDevicesResponse({
      data: [
        {
          ...deviceResource('router-1', 'router'),
          attributes: {
            ...deviceResource('router-1', 'router').attributes,
            topology_discovery_mode: 'bootstrap_once',
            effective_topology_discovery_mode: 'bootstrap_once',
            topology_bootstrap_state: 'followup_scheduled',
            last_topology_discovery_at: '2026-04-18T12:34:56Z',
            last_topology_discovery_result: 'ports_pending',
          },
        },
      ],
    });

    expect(devices[0].topology_discovery_mode).toBe('bootstrap_once');
    expect(devices[0].effective_topology_discovery_mode).toBe('bootstrap_once');
    expect(devices[0].topology_bootstrap_state).toBe('followup_scheduled');
    expect(devices[0].last_topology_discovery_at).toBe('2026-04-18T12:34:56Z');
    expect(devices[0].last_topology_discovery_result).toBe('ports_pending');
  });

  it('defaults polling_enabled to true when omitted', () => {
    const resource = deviceResource('router-2', 'router');
    delete (resource.attributes as Record<string, unknown>).polling_enabled;

    const devices = parseDevicesResponse({ data: [resource] });

    expect(devices[0].polling_enabled).toBe(true);
  });

  it('preserves explicit polling_enabled false', () => {
    const devices = parseDevicesResponse({
      data: [
        {
          ...deviceResource('router-3', 'router'),
          attributes: {
            ...deviceResource('router-3', 'router').attributes,
            polling_enabled: false,
          },
        },
      ],
    });

    expect(devices[0].polling_enabled).toBe(false);
  });
});
