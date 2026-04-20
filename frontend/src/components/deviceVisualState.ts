import type { CSSProperties } from 'react';
import type { Device, DeviceStatus } from '../types/api';
import type { DeviceMetricsDTO } from '../types/metrics';
import { getEffectivePollingIntervalSeconds } from '../utils/polling';

export type DeviceMonitoringState = 'monitorable' | 'unmonitored';
export type DeviceVisualStatus = DeviceStatus | 'degraded' | 'critical' | 'unmonitored';

type DeviceVisualLabel = 'Up' | 'Down' | 'Probing' | 'Unknown' | 'Warning' | 'Critical' | 'Unmonitored';

type DeviceMonitoringInput = Pick<Device, 'device_type' | 'ip'> & Partial<Pick<Device, 'poll_class' | 'poll_interval_override'>>;
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

export interface DeviceNodeStatusStyles {
  badgeClass: string;
  badgeStyle?: CSSProperties;
  panelClass: string;
  panelStyle?: CSSProperties;
  frameClass?: string;
  frameStyle: CSSProperties;
}

export interface DeviceStatusDotStyles {
  className: string;
  style: CSSProperties;
}

interface DeviceFrameTone {
  borderColor?: string;
  shadowLayers: string[];
  focusRingSize: number;
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
  if (!isDeviceMonitorable(device) || !metrics) {
    return null;
  }

  return {
    ...metrics,
    temp_celsius: metrics.temp_celsius ?? null,
    uptime_secs: metrics.uptime_secs ?? null,
    last_polled_at: metrics.last_polled_at ?? metrics.collected_at,
    expected_poll_interval_seconds: getEffectivePollingIntervalSeconds(device),
  };
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

export function resolveDeviceOperationalStatusState(
  device: DeviceVisualInput,
): DeviceVisualState {
  if (!isDeviceMonitorable(device)) {
    return {
      dotStatus: 'unmonitored',
      label: 'Unmonitored',
      labelClass: unmonitoredLabelClass(),
    };
  }

  return {
    dotStatus: device.status,
    label: statusLabel(device.status),
    labelClass: statusLabelClass(device.status),
  };
}

function badgeClassForStatus(status: DeviceVisualStatus): string {
  switch (status) {
    case 'up':
      return 'border-status-up/30 bg-status-up/10 text-status-up';
    case 'critical':
      return 'text-status-critical';
    case 'down':
      return 'text-status-down';
    case 'degraded':
    case 'probing':
      return 'border-warning/30 bg-warning/10 text-warning';
    case 'unmonitored':
      return 'border-outline-strong bg-surface-container text-on-bg-secondary';
    default:
      return 'border-outline bg-surface-container text-on-bg-secondary';
  }
}

function badgeStyleForStatus(status: DeviceVisualStatus): CSSProperties | undefined {
  switch (status) {
    case 'critical':
      return {
        borderColor: 'var(--nt-node-critical-badge-border)',
        backgroundColor: 'var(--nt-node-critical-badge-bg)',
      };
    case 'down':
      return {
        borderColor: 'var(--nt-node-down-badge-border)',
        backgroundColor: 'var(--nt-node-down-badge-bg)',
      };
    default:
      return undefined;
  }
}

function panelClassForStatus(status: DeviceVisualStatus): string {
  switch (status) {
    case 'up':
      return 'border-status-up/30 bg-status-up/10';
    case 'critical':
      return '';
    case 'down':
      return '';
    case 'degraded':
    case 'probing':
      return 'border-warning/30 bg-warning/10';
    default:
      return 'border-outline bg-surface-container';
  }
}

function panelStyleForStatus(status: DeviceVisualStatus): CSSProperties | undefined {
  switch (status) {
    case 'critical':
      return {
        borderColor: 'var(--nt-node-critical-panel-border)',
        backgroundColor: 'var(--nt-node-critical-panel-bg)',
      };
    case 'down':
      return {
        borderColor: 'var(--nt-node-down-panel-border)',
        backgroundColor: 'var(--nt-node-down-panel-bg)',
      };
    default:
      return undefined;
  }
}

function frameToneForStatus(status: DeviceVisualStatus): DeviceFrameTone {
  switch (status) {
    case 'down':
      return {
        borderColor: 'var(--nt-node-down-border)',
        shadowLayers: [
          '0 0 0 1px var(--nt-node-down-border)',
          '0 0 0 6px var(--nt-node-down-ring)',
          '0 0 28px var(--nt-node-down-glow)',
        ],
        focusRingSize: 8,
      };
    case 'critical':
      return {
        borderColor: 'var(--nt-node-critical-border)',
        shadowLayers: ['0 0 0 1px var(--nt-node-critical-border)'],
        focusRingSize: 4,
      };
    case 'degraded':
    case 'probing':
      return {
        borderColor: 'var(--color-status-warning)',
        shadowLayers: ['0 0 0 1px var(--color-status-warning)'],
        focusRingSize: 4,
      };
    default:
      return {
        shadowLayers: [],
        focusRingSize: 4,
      };
  }
}

export function resolveDeviceNodeStatusStyles({
  status,
  selected = false,
  highlighted = false,
}: {
  status: DeviceVisualStatus;
  selected?: boolean;
  highlighted?: boolean;
}): DeviceNodeStatusStyles {
  const tone = frameToneForStatus(status);
  const focusVisible = selected || highlighted;
  const shadowLayers = [...tone.shadowLayers];

  if (focusVisible && !tone.borderColor) {
    shadowLayers.unshift('0 0 0 1px var(--color-node-selected)');
  }

  if (focusVisible) {
    shadowLayers.push(`0 0 0 ${tone.focusRingSize}px var(--color-focus-ring)`);
  }

  shadowLayers.push('var(--nt-node-shadow)');

  return {
    badgeClass: badgeClassForStatus(status),
    badgeStyle: badgeStyleForStatus(status),
    panelClass: panelClassForStatus(status),
    panelStyle: panelStyleForStatus(status),
    frameClass: status === 'down' ? 'topology-node-down-fade' : undefined,
    frameStyle: {
      ...(tone.borderColor
        ? { borderColor: tone.borderColor }
        : focusVisible
          ? { borderColor: 'var(--color-node-selected)' }
          : {}),
      boxShadow: shadowLayers.join(', '),
    },
  };
}

function dotClassForStatus(status: DeviceVisualStatus): string {
  switch (status) {
    case 'up':
      return 'bg-status-up';
    case 'critical':
      return 'bg-status-critical';
    case 'down':
      return 'bg-status-down motion-reduce:animate-none animate-pulse';
    case 'degraded':
      return 'bg-warning motion-reduce:animate-none animate-pulse';
    case 'probing':
      return 'bg-status-probing motion-reduce:animate-none animate-pulse';
    case 'unknown':
      return 'bg-status-unknown';
    case 'unmonitored':
      return 'border border-outline-strong bg-surface-container-high';
  }
}

function dotStyleForStatus(status: DeviceVisualStatus): CSSProperties {
  switch (status) {
    case 'up':
      return { boxShadow: 'var(--nt-glow-status-ok)' };
    case 'critical':
      return { boxShadow: '0 0 0 1px var(--nt-node-critical-badge-border)' };
    case 'down':
      return { boxShadow: 'var(--nt-glow-status-down)' };
    case 'degraded':
    case 'probing':
      return { boxShadow: 'var(--nt-glow-status-warning)' };
    case 'unknown':
      return { boxShadow: 'var(--nt-glow-status-unknown)' };
    case 'unmonitored':
      return { boxShadow: 'none' };
  }
}

export function resolveDeviceStatusDotStyles(status: DeviceVisualStatus): DeviceStatusDotStyles {
  return {
    className: dotClassForStatus(status),
    style: dotStyleForStatus(status),
  };
}

export function resolveDeviceVisualState(
  device: DeviceVisualInput,
  metrics?: DeviceVisualMetrics,
): DeviceVisualState {
  const operationalStatus = resolveDeviceOperationalStatusState(device);

  if (
    operationalStatus.dotStatus === 'unmonitored' ||
    device.status !== 'up' ||
    device.device_type === 'virtual'
  ) {
    return operationalStatus;
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
