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
      metrics_source: 'prometheus',
      prometheus_label_name: 'instance',
      prometheus_label_value: `${id}.example.test:9100`,
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
      data: [
        deviceResource('ap-1', 'access_point'),
        deviceResource('fw-1', 'firewall'),
      ],
    });

    expect(devices[0].device_type).toBe('ap');
    expect(devices[1].device_type).toBe('firewall');
    expect(devices[0].notes).toBe('Managed by NOC');
    expect(devices[1].notes).toBeNull();
  });
});
