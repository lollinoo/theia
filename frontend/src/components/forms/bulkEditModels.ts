import type { Device, MetricsSource } from '../../types/api';

export interface BulkField<T> {
  value: T;
  mixed: boolean;
  dirty: boolean;
}

export interface BulkEditModel {
  areaIds: BulkField<string[]>;
  metricsSource: BulkField<MetricsSource | ''>;
  vendor: BulkField<string>;
}

function commonValue<T>(
  devices: Device[],
  extract: (device: Device) => T,
): { value: T; mixed: boolean } {
  const first = extract(devices[0]);
  const firstJSON = JSON.stringify(first);

  return {
    value: first,
    mixed: devices.some((device) => JSON.stringify(extract(device)) !== firstJSON),
  };
}

export function createBulkEditModel(devices: Device[]): BulkEditModel {
  const areaIds = commonValue(devices, (device) => [...(device.area_ids ?? [])].sort());
  const metricsSource = commonValue(devices, (device) => device.metrics_source || 'snmp');
  const vendor = commonValue(devices, (device) => device.vendor || '');

  return {
    areaIds: { value: areaIds.mixed ? [] : areaIds.value, mixed: areaIds.mixed, dirty: false },
    metricsSource: {
      value: metricsSource.mixed ? '' : metricsSource.value,
      mixed: metricsSource.mixed,
      dirty: false,
    },
    vendor: {
      value: vendor.mixed ? '' : vendor.value,
      mixed: vendor.mixed,
      dirty: false,
    },
  };
}

export function buildBulkUpdatePayload(device: Device, model: BulkEditModel) {
  return {
    hostname: device.hostname,
    ...(model.areaIds.dirty ? { area_ids: model.areaIds.value } : {}),
    ...(model.metricsSource.dirty && model.metricsSource.value !== ''
      ? { metrics_source: model.metricsSource.value }
      : {}),
    ...(model.vendor.dirty ? { vendor: model.vendor.value } : {}),
  };
}
