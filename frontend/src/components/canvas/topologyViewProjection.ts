/**
 * Defines topology view projection behavior for the topology canvas.
 * Documents how canonical topology data is projected into the interactive view layer.
 */
import type { Device, Link } from '../../types/api';

/** Describes the topology view filter contract used by the topology canvas. */
export interface TopologyViewFilter {
  areaId?: string | null;
  deviceIds?: string[];
  includeCrossAreaLinks?: boolean;
  includeGhostDevices?: boolean;
  tagFilter?: Record<string, string>;
}

/** Describes the topology view projection contract used by the topology canvas. */
export interface TopologyViewProjection {
  filteredDevices: Device[];
  filteredLinks: Link[];
  ghostDevices: Device[];
}

/** Project topology view for the topology canvas. */
export function projectTopologyView(input: {
  devices: Device[];
  links: Link[];
  filter?: TopologyViewFilter;
}): TopologyViewProjection {
  const { devices, links } = input;
  const filter = input.filter ?? {};
  const hasDeviceIds = filter.deviceIds !== undefined && filter.deviceIds.length > 0;
  const areaId = filter.areaId ?? null;
  const hasAreaId = areaId !== null;
  const tagFilter = filter.tagFilter ?? {};
  const selectedDeviceIds = new Set(filter.deviceIds ?? []);
  const knownIds = new Set(devices.map((device) => device.id));

  const baseIds = new Set<string>();
  for (const device of devices) {
    let baseDevice = true;
    if (hasDeviceIds) {
      baseDevice = selectedDeviceIds.has(device.id);
    } else if (hasAreaId) {
      baseDevice = device.area_ids?.includes(areaId) === true;
    }

    if (baseDevice && deviceMatchesTags(device, tagFilter)) {
      baseIds.add(device.id);
    }
  }

  const filteredDevices = devices.filter((device) => baseIds.has(device.id));
  const includeCrossAreaLinks = filter.includeCrossAreaLinks === true;
  const includeGhostDevices = filter.includeGhostDevices === true;
  const ghostIds = new Set<string>();
  const filteredLinks = links.filter((link) => {
    if (!knownIds.has(link.source_device_id) || !knownIds.has(link.target_device_id)) {
      return false;
    }

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
    ghostDevices: devices.filter((device) => !baseIds.has(device.id) && ghostIds.has(device.id)),
  };
}

function deviceMatchesTags(device: Device, tagFilter: Record<string, string>): boolean {
  for (const [key, expected] of Object.entries(tagFilter)) {
    const tags = device.tags;
    if (
      tags === undefined ||
      tags === null ||
      // biome-ignore lint/suspicious/noPrototypeBuiltins: Object.hasOwn is unavailable under this package's current TypeScript lib target.
      !Object.prototype.hasOwnProperty.call(tags, key) ||
      tags[key] !== expected
    ) {
      return false;
    }
  }

  return true;
}
