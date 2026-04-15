import type { Device, DeviceStatus } from '../types/api';
import type { DeviceMetricsDTO } from '../types/metrics';

export type DeviceVisualStatus = DeviceStatus | 'degraded';

type DeviceVisualLabel = 'Up' | 'Down' | 'Probing' | 'Unknown' | 'Warning' | 'Critical';

type DeviceVisualInput = Pick<Device, 'device_type' | 'ip' | 'status'>;
type DeviceVisualMetrics = Pick<DeviceMetricsDTO, 'health'> | null | undefined;

export interface DeviceVisualState {
  dotStatus: DeviceVisualStatus;
  label: DeviceVisualLabel;
  labelClass: string;
}

function healthLabelClass(health: string | undefined): string {
  switch (health) {
    case 'healthy':
      return 'text-[12px] font-semibold text-status-up';
    case 'warning':
      return 'text-[12px] font-semibold text-warning';
    case 'critical':
      return 'text-[12px] font-semibold text-critical';
    default:
      return 'text-[12px] font-semibold text-status-unknown';
  }
}

function statusLabel(status: DeviceStatus): Exclude<DeviceVisualLabel, 'Warning' | 'Critical'> {
  switch (status) {
    case 'up':
      return 'Up';
    case 'down':
      return 'Down';
    case 'probing':
      return 'Probing';
    default:
      return 'Unknown';
  }
}

function statusLabelClass(status: DeviceStatus): string {
  switch (status) {
    case 'up':
      return 'text-[12px] font-semibold text-status-up';
    case 'down':
      return 'text-[12px] font-semibold text-status-down';
    case 'probing':
      return 'text-[12px] font-semibold text-status-probing';
    default:
      return 'text-[12px] font-semibold text-status-unknown';
  }
}

export function resolveDeviceVisualState(
  device: DeviceVisualInput,
  metrics?: DeviceVisualMetrics,
): DeviceVisualState {
  if (device.device_type === 'virtual' && device.ip.trim().length === 0) {
    return {
      dotStatus: 'unknown',
      label: 'Unknown',
      labelClass: statusLabelClass('unknown'),
    };
  }

  if (device.status !== 'up') {
    return {
      dotStatus: device.status,
      label: statusLabel(device.status),
      labelClass: statusLabelClass(device.status),
    };
  }

  switch (metrics?.health) {
    case 'healthy':
      return {
        dotStatus: 'up',
        label: 'Up',
        labelClass: healthLabelClass('healthy'),
      };
    case 'warning':
      return {
        dotStatus: 'degraded',
        label: 'Warning',
        labelClass: healthLabelClass('warning'),
      };
    case 'critical':
      return {
        dotStatus: 'down',
        label: 'Critical',
        labelClass: healthLabelClass('critical'),
      };
    default:
      return {
        dotStatus: 'unknown',
        label: 'Unknown',
        labelClass: healthLabelClass(undefined),
      };
  }
}

export function minimapColorForDevice({
  device,
  metrics,
  isGhost = false,
}: {
  device: DeviceVisualInput;
  metrics?: DeviceVisualMetrics;
  isGhost?: boolean;
}): string {
  if (isGhost) {
    return 'var(--nt-on-bg-muted)';
  }

  switch (resolveDeviceVisualState(device, metrics).dotStatus) {
    case 'up':
      return 'var(--color-status-up)';
    case 'down':
      return 'var(--color-status-down)';
    case 'probing':
      return 'var(--color-status-probing)';
    case 'degraded':
      return 'var(--color-status-probing)';
    default:
      return 'var(--color-status-unknown)';
  }
}
