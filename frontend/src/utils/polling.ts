/**
 * Provides polling utility behavior shared by frontend workflows.
 * Keeps non-UI policy and formatting rules reusable across components.
 */
import type { Device, DevicePollClass } from '../types/api';

const DEFAULT_POLLING_INTERVAL_SECONDS_BY_CLASS: Record<DevicePollClass, number> = {
  core: 30,
  standard: 60,
  low: 300,
};

/** Returns default polling interval seconds for the shared frontend utility layer. */
export function getDefaultPollingIntervalSeconds(pollClass: DevicePollClass | undefined): number {
  if (!pollClass) {
    return DEFAULT_POLLING_INTERVAL_SECONDS_BY_CLASS.standard;
  }

  return (
    DEFAULT_POLLING_INTERVAL_SECONDS_BY_CLASS[pollClass] ??
    DEFAULT_POLLING_INTERVAL_SECONDS_BY_CLASS.standard
  );
}

/** Returns effective polling interval seconds for the shared frontend utility layer. */
export function getEffectivePollingIntervalSeconds(
  device: Partial<Pick<Device, 'poll_class' | 'poll_interval_override'>>,
): number {
  return device.poll_interval_override ?? getDefaultPollingIntervalSeconds(device.poll_class);
}
