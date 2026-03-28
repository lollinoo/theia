import { useMemo } from 'react';
import type { Device, Link } from '../../types/api';

export interface FilteredTopology {
  filteredDevices: Device[];
  filteredLinks: Link[];
  ghostDevices: Device[];
}

/**
 * Filters devices and links by the selected area, identifying ghost devices
 * for cross-area links.
 *
 * - Global view (selectedAreaId is null): returns all devices and links, no ghosts
 * - Area view: returns only area devices and links with at least one area endpoint
 * - Ghost devices: remote endpoints of cross-area links (not in selected area)
 * - Unassigned devices (no area_id) are excluded from area views (per D-14)
 */
export function useAreaFilteredTopology(
  devices: Device[],
  links: Link[],
  selectedAreaId: string | null,
): FilteredTopology {
  return useMemo(() => {
    // No filter = show everything (Global view)
    if (!selectedAreaId) {
      return { filteredDevices: devices, filteredLinks: links, ghostDevices: [] };
    }

    // Devices in the selected area (per D-14: unassigned devices excluded)
    const areaDeviceIds = new Set(
      devices
        .filter((d) => d.area_ids?.includes(selectedAreaId))
        .map((d) => d.id),
    );
    const filteredDevices = devices.filter((d) => areaDeviceIds.has(d.id));

    // Links where at least one endpoint is in the area
    const filteredLinks = links.filter(
      (l) => areaDeviceIds.has(l.source_device_id) || areaDeviceIds.has(l.target_device_id),
    );

    // Ghost devices: remote endpoints of cross-area links
    const ghostDeviceIds = new Set<string>();
    for (const link of filteredLinks) {
      if (!areaDeviceIds.has(link.source_device_id)) {
        ghostDeviceIds.add(link.source_device_id);
      }
      if (!areaDeviceIds.has(link.target_device_id)) {
        ghostDeviceIds.add(link.target_device_id);
      }
    }
    const ghostDevices = devices.filter((d) => ghostDeviceIds.has(d.id));

    return { filteredDevices, filteredLinks, ghostDevices };
  }, [devices, links, selectedAreaId]);
}
