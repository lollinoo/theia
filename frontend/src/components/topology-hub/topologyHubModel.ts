/**
 * Defines topology hub model behavior for the topology hub.
 * Keeps saved-map and area workflows separate from the live canvas surface.
 */
import type { Area, CanvasMap, Device, Link } from '../../types/api';
import type { SnapshotPayload } from '../../types/metrics';
import {
  type DeviceVisualStatus,
  resolveDeviceMonitoringState,
  resolveDeviceOperationalStatusState,
  resolveDeviceVisualState,
} from '../deviceVisualState';

/** Describes the topology hub aggregate contract used by the topology hub. */
export interface TopologyHubAggregate {
  totalDevices: number;
  activeLinks: number;
  degradedDevices: number;
  healthPercentage: number;
}

/** Describes the topology hub area model contract used by the topology hub. */
export interface TopologyHubAreaModel {
  area: Area;
  deviceCount: number;
  activeLinkCount: number;
  degradedDeviceCount: number;
  healthPercentage: number;
  healthLabel: 'Healthy' | 'Needs attention';
}

/** Describes the topology hub model contract used by the topology hub. */
export interface TopologyHubModel {
  aggregate: TopologyHubAggregate;
  areas: TopologyHubAreaModel[];
  attentionDevices: Device[];
  unassignedDevices: Device[];
  maps: CanvasMap[];
}

/** Describes the build topology hub model input contract used by the topology hub. */
export interface BuildTopologyHubModelInput {
  devices: Device[];
  areas: Area[];
  links: Link[];
  snapshot: SnapshotPayload | null;
  maps: CanvasMap[];
}

const attentionVisualStatuses = new Set<DeviceVisualStatus>([
  'critical',
  'degraded',
  'down',
  'probing',
  'unknown',
]);

function isDeviceDegraded(device: Device, snapshot: SnapshotPayload | null): boolean {
  const runtimeDevice = snapshot?.devices?.[device.id];
  const monitoringState = resolveDeviceMonitoringState(device);
  const visualState = runtimeDevice
    ? resolveDeviceVisualState(device, runtimeDevice, monitoringState)
    : resolveDeviceOperationalStatusState(device, monitoringState);

  return attentionVisualStatuses.has(visualState.dotStatus);
}

function healthPercentage(deviceCount: number, degradedDeviceCount: number): number {
  if (deviceCount === 0) {
    return 100;
  }

  return Math.round(((deviceCount - degradedDeviceCount) / deviceCount) * 100);
}

function countAreaLinks(links: Link[], areaDeviceIds: Set<string>): number {
  return links.filter(
    (link) => areaDeviceIds.has(link.source_device_id) || areaDeviceIds.has(link.target_device_id),
  ).length;
}

/** Builds topology hub model for the topology hub. */
export function buildTopologyHubModel({
  devices,
  areas,
  links,
  snapshot,
  maps,
}: BuildTopologyHubModelInput): TopologyHubModel {
  const degradedDevices = devices.filter((device) => isDeviceDegraded(device, snapshot));

  const areaModels = areas.map((area) => {
    const areaDevices = devices.filter((device) => device.area_ids?.includes(area.id));
    const areaDeviceIds = new Set(areaDevices.map((device) => device.id));
    const degradedDeviceCount = areaDevices.filter((device) =>
      isDeviceDegraded(device, snapshot),
    ).length;
    const healthLabel: TopologyHubAreaModel['healthLabel'] =
      degradedDeviceCount > 0 ? 'Needs attention' : 'Healthy';

    return {
      area,
      deviceCount: areaDevices.length,
      activeLinkCount: countAreaLinks(links, areaDeviceIds),
      degradedDeviceCount,
      healthPercentage: healthPercentage(areaDevices.length, degradedDeviceCount),
      healthLabel,
    };
  });

  return {
    aggregate: {
      totalDevices: devices.length,
      activeLinks: links.length,
      degradedDevices: degradedDevices.length,
      healthPercentage: healthPercentage(devices.length, degradedDevices.length),
    },
    areas: areaModels,
    attentionDevices: degradedDevices,
    unassignedDevices: devices.filter((device) => !device.area_ids || device.area_ids.length === 0),
    maps,
  };
}
