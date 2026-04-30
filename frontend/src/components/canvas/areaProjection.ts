import type { Device, Link } from '../../types/api';

export interface AreaTopologyProjection {
  filteredDevices: Device[];
  filteredLinks: Link[];
  ghostDevices: Device[];
}

export interface ProjectAreaTopologyInput {
  devices: Device[];
  links: Link[];
  selectedAreaId: string | null;
}

/**
 * Pure area projection used by the Canvas hook and by performance benchmarks.
 * It returns the canonical devices/links visible in the selected area plus
 * remote endpoints represented as ghost devices by the React Flow adapter.
 */
export function projectAreaTopology({
  devices,
  links,
  selectedAreaId,
}: ProjectAreaTopologyInput): AreaTopologyProjection {
  if (!selectedAreaId) {
    return { filteredDevices: devices, filteredLinks: links, ghostDevices: [] };
  }

  const areaDeviceIds = new Set(
    devices
      .filter((device) => device.area_ids?.includes(selectedAreaId))
      .map((device) => device.id),
  );
  const filteredDevices = devices.filter((device) => areaDeviceIds.has(device.id));
  const filteredLinks = links.filter(
    (link) => areaDeviceIds.has(link.source_device_id) || areaDeviceIds.has(link.target_device_id),
  );

  const ghostDeviceIds = new Set<string>();
  for (const link of filteredLinks) {
    if (!areaDeviceIds.has(link.source_device_id)) {
      ghostDeviceIds.add(link.source_device_id);
    }
    if (!areaDeviceIds.has(link.target_device_id)) {
      ghostDeviceIds.add(link.target_device_id);
    }
  }

  return {
    filteredDevices,
    filteredLinks,
    ghostDevices: devices.filter((device) => ghostDeviceIds.has(device.id)),
  };
}
