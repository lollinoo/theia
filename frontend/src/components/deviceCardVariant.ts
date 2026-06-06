/**
 * Renders device card variant UI behavior for the Theia frontend.
 * Keeps this component's state and interaction boundary explicit for maintainers.
 */
import type { Device } from '../types/api';
import {
  type DeviceAddressState,
  type DeviceMonitoringState,
  resolveDeviceMonitoringState,
} from './deviceVisualState';

/** Describes the device card variant contract used by the UI component boundary. */
export type DeviceCardVariant = 'physical' | 'virtual-monitorable' | 'virtual-unmonitored';

type DeviceCardVariantInput = Pick<Device, 'device_type' | 'ip'>;

/** Describes the device card render model contract used by the UI component boundary. */
export interface DeviceCardRenderModel {
  variant: DeviceCardVariant;
  showFreshnessMeta: boolean;
  showOperationalReadouts: boolean;
  showVirtualStatusBadge: boolean;
  showVirtualAddressChip: boolean;
}

/** Resolves device card variant for the UI component boundary. */
export function resolveDeviceCardVariant(
  device: DeviceCardVariantInput,
  monitoringState?: DeviceMonitoringState,
): DeviceCardVariant {
  if (device.device_type !== 'virtual') {
    return 'physical';
  }

  const effectiveMonitoringState = monitoringState ?? resolveDeviceMonitoringState(device);
  return effectiveMonitoringState === 'unmonitored' ? 'virtual-unmonitored' : 'virtual-monitorable';
}

/** Resolves device card render model for the UI component boundary. */
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
        showVirtualStatusBadge: false,
        showVirtualAddressChip: false,
      };
    case 'virtual-monitorable':
      return {
        variant,
        showFreshnessMeta: hasFreshnessMeta,
        showOperationalReadouts: false,
        showVirtualStatusBadge: true,
        showVirtualAddressChip: addressState === 'address',
      };
    case 'virtual-unmonitored':
      return {
        variant,
        showFreshnessMeta: false,
        showOperationalReadouts: false,
        showVirtualStatusBadge: false,
        showVirtualAddressChip: false,
      };
  }
}
