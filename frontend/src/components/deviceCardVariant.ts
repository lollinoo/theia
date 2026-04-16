import type { Device } from '../types/api';
import {
  type DeviceAddressState,
  resolveDeviceMonitoringState,
  type DeviceMonitoringState,
} from './deviceVisualState';

export type DeviceCardVariant =
  | 'physical'
  | 'virtual-monitorable'
  | 'virtual-unmonitored';

type DeviceCardVariantInput = Pick<Device, 'device_type' | 'ip'>;

export interface DeviceCardRenderModel {
  variant: DeviceCardVariant;
  showFreshnessMeta: boolean;
  showOperationalReadouts: boolean;
  showVirtualStatusPanel: boolean;
  showVirtualIdentityTag: boolean;
  showVirtualAddressChip: boolean;
  showVirtualCategoryBadge: boolean;
}

export function resolveDeviceCardVariant(
  device: DeviceCardVariantInput,
  monitoringState?: DeviceMonitoringState,
): DeviceCardVariant {
  if (device.device_type !== 'virtual') {
    return 'physical';
  }

  const effectiveMonitoringState = monitoringState ?? resolveDeviceMonitoringState(device);
  return effectiveMonitoringState === 'unmonitored'
    ? 'virtual-unmonitored'
    : 'virtual-monitorable';
}

export function resolveDeviceCardRenderModel({
  device,
  monitoringState,
  addressState,
  hasFreshnessMeta,
}: {
  device: DeviceCardVariantInput;
  monitoringState?: DeviceMonitoringState;
  addressState: DeviceAddressState;
  hasFreshnessMeta: boolean;
}): DeviceCardRenderModel {
  const variant = resolveDeviceCardVariant(device, monitoringState);

  switch (variant) {
    case 'physical':
      return {
        variant,
        showFreshnessMeta: hasFreshnessMeta,
        showOperationalReadouts: true,
        showVirtualStatusPanel: false,
        showVirtualIdentityTag: false,
        showVirtualAddressChip: false,
        showVirtualCategoryBadge: false,
      };
    case 'virtual-monitorable':
      return {
        variant,
        showFreshnessMeta: hasFreshnessMeta,
        showOperationalReadouts: false,
        showVirtualStatusPanel: true,
        showVirtualIdentityTag: false,
        showVirtualAddressChip: addressState === 'address',
        showVirtualCategoryBadge: true,
      };
    case 'virtual-unmonitored':
      return {
        variant,
        showFreshnessMeta: false,
        showOperationalReadouts: false,
        showVirtualStatusPanel: false,
        showVirtualIdentityTag: true,
        showVirtualAddressChip: false,
        showVirtualCategoryBadge: false,
      };
  }
}
