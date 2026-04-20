import { describe, expect, it } from 'vitest';

import type { Device } from '../../types/api';
import { buildBulkUpdatePayload, createBulkEditModel } from './bulkEditModels';

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
    backup_supported: true,
    metrics_source: 'snmp',
    prometheus_label_name: 'instance',
    prometheus_label_value: '10.0.0.1:9100',
    area_ids: [],
    ...overrides,
  };
}

describe('bulkEditModels', () => {
  it('records mixed values separately from unchanged values', () => {
    const model = createBulkEditModel([
      mockDevice({ vendor: 'mikrotik' }),
      mockDevice({ id: 'dev-2', vendor: '' }),
    ]);

    expect(model.vendor.mixed).toBe(true);
    expect(model.vendor.dirty).toBe(false);
  });

  it('initializes mixed area selections with an empty UI value', () => {
    const model = createBulkEditModel([
      mockDevice({ area_ids: ['area-1'] }),
      mockDevice({ id: 'dev-2', area_ids: ['area-2'] }),
    ]);

    expect(model.areaIds.mixed).toBe(true);
    expect(model.areaIds.dirty).toBe(false);
    expect(model.areaIds.value).toEqual([]);
  });

  it('omits unchanged fields from the per-device update payload', () => {
    const model = createBulkEditModel([mockDevice()]);
    const payload = buildBulkUpdatePayload(mockDevice(), model);

    expect(payload).toEqual({ hostname: 'router-01' });
  });

  it('includes changed fields in the per-device update payload', () => {
    const model = {
      ...createBulkEditModel([mockDevice()]),
      vendor: { value: '', mixed: false, dirty: true },
      metricsSource: { value: 'prometheus', mixed: false, dirty: true },
      areaIds: { value: ['area-1'], mixed: false, dirty: true },
    };

    expect(buildBulkUpdatePayload(mockDevice(), model)).toMatchObject({
      hostname: 'router-01',
      vendor: '',
      metrics_source: 'prometheus',
      area_ids: ['area-1'],
    });
  });

  it('omits metrics_source when keep-current is reselected after a mixed initial state', () => {
    const model = {
      ...createBulkEditModel([
        mockDevice({ metrics_source: 'snmp' }),
        mockDevice({ id: 'dev-2', metrics_source: 'prometheus' }),
      ]),
      metricsSource: { value: '', mixed: false, dirty: true },
    };

    expect(buildBulkUpdatePayload(mockDevice(), model)).toEqual({
      hostname: 'router-01',
    });
  });

  it('builds payloads from user-edited mixed area selections only', () => {
    const model = {
      ...createBulkEditModel([
        mockDevice({ area_ids: ['area-1'] }),
        mockDevice({ id: 'dev-2', area_ids: ['area-2'] }),
      ]),
      areaIds: { value: ['area-3'], mixed: false, dirty: true },
    };

    expect(buildBulkUpdatePayload(mockDevice(), model)).toEqual({
      hostname: 'router-01',
      area_ids: ['area-3'],
    });
  });
});
