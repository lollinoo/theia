import type { Device, DeviceStatus } from '../types/api';
import type { DeviceMetricsDTO } from '../types/metrics';

export type DeviceMonitoringState = 'monitorable' | 'unmonitored';
export type DeviceVisualStatus = DeviceStatus | 'degraded' | 'critical' | 'unmonitored';

type DeviceVisualLabel = 'Up' | 'Down' | 'Probing' | 'Unknown' | 'Warning' | 'Critical' | 'Unmonitored';

type DeviceMonitoringInput = Pick<Device, 'device_type' | 'ip'>;
type DeviceVisualInput = Pick<Device, 'device_type' | 'ip' | 'status'>;
type DeviceVisualMetrics = DeviceMetricsDTO | null | undefined;
export type DeviceAddressState = 'address' | 'missing' | 'unmonitored';

export interface DeviceVisualState {
  dotStatus: DeviceVisualStatus;
  label: DeviceVisualLabel;
  labelClass: string;
}

export interface DeviceOperationalReadouts {
  cpuPercent: number | null;
  memPercent: number | null;
  uptimeSecs: number | null;
  isDeviceDown: boolean;
}

export function resolveDeviceMonitoringState(device: DeviceMonitoringInput): DeviceMonitoringState {
  const hasIp = device.ip.trim().length > 0;
  return device.device_type === 'virtual' && !hasIp ? 'unmonitored' : 'monitorable';
}

export function isDeviceMonitorable(device: DeviceMonitoringInput): boolean {
  return resolveDeviceMonitoringState(device) === 'monitorable';
}

export function sanitizeDeviceMetricsForDisplay(
  device: DeviceMonitoringInput,
  metrics?: DeviceVisualMetrics,
): DeviceMetricsDTO | null {
  return isDeviceMonitorable(device) ? metrics ?? null : null;
}

export function resolveDeviceAddressState(device: DeviceMonitoringInput): DeviceAddressState {
  if (device.ip.trim().length > 0) {
    return 'address';
  }

  return isDeviceMonitorable(device) ? 'missing' : 'unmonitored';
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

function unmonitoredLabelClass(): string {
  return 'text-[12px] font-semibold text-on-bg-secondary';
}

export function resolveDeviceVisualState(
  device: DeviceVisualInput,
  metrics?: DeviceVisualMetrics,
): DeviceVisualState {
  if (!isDeviceMonitorable(device)) {
    return {
      dotStatus: 'unmonitored',
      label: 'Unmonitored',
      labelClass: unmonitoredLabelClass(),
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
        dotStatus: 'critical',
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

export function resolveDeviceOperationalReadouts(
  device: DeviceVisualInput,
  metrics?: DeviceVisualMetrics,
): DeviceOperationalReadouts {
  const sanitizedMetrics = sanitizeDeviceMetricsForDisplay(device, metrics);
  const isDeviceDown = isDeviceMonitorable(device) && device.status === 'down';

  return {
    cpuPercent: isDeviceDown ? null : sanitizedMetrics?.cpu_percent ?? null,
    memPercent: isDeviceDown ? null : sanitizedMetrics?.mem_percent ?? null,
    uptimeSecs: isDeviceDown ? null : sanitizedMetrics?.uptime_secs ?? null,
    isDeviceDown,
  };
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
    case 'critical':
      return 'var(--color-status-critical)';
    case 'down':
      return 'var(--color-status-down)';
    case 'probing':
      return 'var(--color-status-probing)';
    case 'degraded':
      return 'var(--color-status-probing)';
    case 'unmonitored':
      return 'var(--nt-on-bg-muted)';
    default:
      return 'var(--color-status-unknown)';
  }
}
