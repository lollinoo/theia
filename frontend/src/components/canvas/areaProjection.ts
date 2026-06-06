/**
 * Defines area projection behavior for the topology canvas.
 * Documents how canonical topology data is projected into the interactive view layer.
 */
import type { Device, Link } from '../../types/api';
import { projectTopologyView } from './topologyViewProjection';

/** Describes the area topology projection contract used by the topology canvas. */
export interface AreaTopologyProjection {
  filteredDevices: Device[];
  filteredLinks: Link[];
  ghostDevices: Device[];
}

/** Describes the project area topology input contract used by the topology canvas. */
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

  return projectTopologyView({
    devices,
    links,
    filter: {
      areaId: selectedAreaId,
      includeCrossAreaLinks: true,
      includeGhostDevices: true,
    },
  });
}
