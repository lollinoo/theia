/**
 * Defines canvas map state behavior for the topology hub.
 * Keeps saved-map and area workflows separate from the live canvas surface.
 */
import type { Area, CanvasMap, CanvasMapFilter } from '../../types/api';

/** Builds canvas map error message for the topology hub. */
export function canvasMapErrorMessage(error: unknown, fallback: string): string {
  return error instanceof Error ? error.message : fallback;
}

/** Maps filter for area for the topology hub. */
export function mapFilterForArea(area: Area): CanvasMapFilter {
  return {
    area_id: area.id,
    include_cross_area_links: true,
    include_ghost_devices: true,
  };
}

/** Upserts canvas map for the topology hub. */
export function upsertCanvasMap(maps: CanvasMap[], map: CanvasMap): CanvasMap[] {
  const existingIndex = maps.findIndex((candidate) => candidate.id === map.id);
  if (existingIndex === -1) {
    return [...maps, map];
  }

  return maps.map((candidate, index) => (index === existingIndex ? map : candidate));
}

/** Selects fallback canvas map for the topology hub. */
export function fallbackCanvasMap(maps: CanvasMap[]): CanvasMap | null {
  return maps.find((map) => map.is_default) ?? maps[0] ?? null;
}

/** Sets primary canvas map for the topology hub. */
export function setPrimaryCanvasMap(maps: CanvasMap[], primaryMap: CanvasMap): CanvasMap[] {
  let found = false;
  const nextMaps = maps.map((map) => {
    if (map.id === primaryMap.id) {
      found = true;
      return { ...primaryMap, is_default: true };
    }
    return map.is_default ? { ...map, is_default: false } : map;
  });

  if (!found) {
    return [...nextMaps, { ...primaryMap, is_default: true }];
  }
  return nextMaps;
}
