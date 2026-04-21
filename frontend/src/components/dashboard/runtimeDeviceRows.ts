import type { Device } from '../../types/api';
import { type SnapshotPayload, formatUptime } from '../../types/metrics';
import {
  resolveDeviceMonitoringState,
  resolveDeviceOperationalReadouts,
  resolveDeviceOperationalStatusState,
} from '../deviceVisualState';
import { resolveOsVersion } from './parseOsVersion';

export interface RuntimeDeviceRow {
  id: string;
  device: Device;
  displayName: string;
  hostname: string;
  ip: string;
  sysName: string;
  deviceType: Device['device_type'];
  vendor: string;
  areaIds: string[];
  modelLabel: string;
  searchText: string;
  statusSortLabel: string;
  areaSortName: string;
  statusState: ReturnType<typeof resolveDeviceOperationalStatusState>;
  uptimeSecs: number | null;
  uptimeLabel: string | null;
  osVersion: string;
}

function runtimeAwareDevice(device: Device, snapshot: SnapshotPayload | null): Device {
  const runtime = snapshot?.devices?.[device.id];
  if (!runtime) {
    return device;
  }

  return {
    ...device,
    status:
      runtime.operational_status === 'unmonitored' ? device.status : runtime.operational_status,
  };
}

function runtimeMonitoringState(device: Device, snapshot: SnapshotPayload | null) {
  return snapshot?.devices?.[device.id]?.operational_status === 'unmonitored'
    ? 'unmonitored'
    : resolveDeviceMonitoringState(device);
}

export function buildRuntimeDeviceRows({
  devices,
  snapshot,
}: {
  devices: Device[];
  snapshot: SnapshotPayload | null;
}): RuntimeDeviceRow[] {
  return devices.map((device) => {
    const runtimeDevice = runtimeAwareDevice(device, snapshot);
    const monitoringState = runtimeMonitoringState(device, snapshot);
    const metrics = snapshot?.devices?.[device.id] ?? null;
    const readouts = resolveDeviceOperationalReadouts(runtimeDevice, metrics, monitoringState);
    const displayName =
      device.tags?.display_name || device.sys_name || device.hostname || device.ip;
    const modelLabel =
      device.hardware_model && device.hardware_model !== 'Unknown'
        ? device.hardware_model
        : device.sys_descr || '';
    const statusState = resolveDeviceOperationalStatusState(runtimeDevice, monitoringState);

    return {
      id: device.id,
      device: runtimeDevice,
      displayName,
      hostname: device.hostname,
      ip: device.ip,
      sysName: device.sys_name,
      deviceType: device.device_type,
      vendor: device.vendor,
      areaIds: device.area_ids ?? [],
      modelLabel,
      searchText: [device.hostname, device.ip, device.sys_name, displayName]
        .filter((value): value is string => Boolean(value))
        .join(' ')
        .toLowerCase(),
      statusSortLabel: statusState.label.toLowerCase(),
      areaSortName: '',
      statusState,
      uptimeSecs: readouts.uptimeSecs,
      uptimeLabel: readouts.uptimeSecs === null ? null : formatUptime(readouts.uptimeSecs),
      osVersion: resolveOsVersion(device.os_version, device.sys_descr),
    };
  });
}

export function computeAreaHealthSummary(rows: Array<Pick<RuntimeDeviceRow, 'statusState'>>): {
  percentage: number;
  label: string;
  color: string;
} {
  if (rows.length === 0) {
    return { percentage: 100, label: 'N/A', color: 'text-on-bg-secondary' };
  }

  const upCount = rows.filter((row) => row.statusState.dotStatus === 'up').length;
  const percentage = (upCount / rows.length) * 100;

  if (percentage >= 95) return { percentage, label: 'Optimal', color: 'text-status-up' };
  if (percentage >= 80) return { percentage, label: 'Degraded', color: 'text-warning' };
  return { percentage, label: 'Critical', color: 'text-status-down' };
}
