/**
 * Coordinates area filtered topology state for the topology canvas.
 * Keeps canvas lifecycle, projected graph state, and cleanup behavior explicit for callers.
 */
import { useMemo } from 'react';
import type { Device, Link } from '../../types/api';
import { projectAreaTopology } from './areaProjection';

/** Describes the filtered topology contract used by the topology canvas. */
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
  return useMemo(
    () => projectAreaTopology({ devices, links, selectedAreaId }),
    [devices, links, selectedAreaId],
  );
}
