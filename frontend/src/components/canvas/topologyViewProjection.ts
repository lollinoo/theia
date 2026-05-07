import type { Device, Link } from '../../types/api';

export interface TopologyViewFilter {
  areaId?: string | null;
  deviceIds?: string[];
  includeCrossAreaLinks?: boolean;
  includeGhostDevices?: boolean;
  tagFilter?: Record<string, string>;
}

export interface TopologyViewProjection {
  filteredDevices: Device[];
  filteredLinks: Link[];
  ghostDevices: Device[];
}

export function projectTopologyView(input: {
  devices: Device[];
  links: Link[];
  filter?: TopologyViewFilter;
}): TopologyViewProjection {
  const { devices, links } = input;
  const filter = input.filter ?? {};
  const hasDeviceIds = filter.deviceIds !== undefined && filter.deviceIds.length > 0;
  const hasAreaId = filter.areaId !== undefined && filter.areaId !== null;
  const hasTagFilter =
    filter.tagFilter !== undefined && Object.keys(filter.tagFilter).length > 0;

  if (!hasDeviceIds && !hasAreaId && !hasTagFilter) {
    return { filteredDevices: devices, filteredLinks: links, ghostDevices: [] };
  }

  const baseIds = new Set<string>();
  if (hasDeviceIds) {
    for (const deviceId of filter.deviceIds ?? []) {
      baseIds.add(deviceId);
    }
  } else if (hasAreaId) {
    for (const device of devices) {
      if (device.area_ids?.includes(filter.areaId as string)) {
        baseIds.add(device.id);
      }
    }
  } else {
    for (const device of devices) {
      baseIds.add(device.id);
    }
  }

  if (hasTagFilter) {
    for (const device of devices) {
      if (!baseIds.has(device.id)) {
        continue;
      }

      const matches = Object.entries(filter.tagFilter ?? {}).every(
        ([key, value]) => device.tags?.[key] === value,
      );
      if (!matches) {
        baseIds.delete(device.id);
      }
    }
  }

  const filteredDevices = devices.filter((device) => baseIds.has(device.id));
  const includeCrossAreaLinks = filter.includeCrossAreaLinks === true;
  const includeGhostDevices = filter.includeGhostDevices === true;
  const ghostIds = new Set<string>();
  const filteredLinks = links.filter((link) => {
    const sourceInBase = baseIds.has(link.source_device_id);
    const targetInBase = baseIds.has(link.target_device_id);
    const include = includeCrossAreaLinks
      ? sourceInBase || targetInBase
      : sourceInBase && targetInBase;

    if (include && includeGhostDevices) {
      if (sourceInBase && !targetInBase) {
        ghostIds.add(link.target_device_id);
      }
      if (targetInBase && !sourceInBase) {
        ghostIds.add(link.source_device_id);
      }
    }

    return include;
  });

  return {
    filteredDevices,
    filteredLinks,
    ghostDevices: devices.filter((device) => ghostIds.has(device.id)),
  };
}
