import type { Area, CanvasMap, CanvasMapFilter } from '../../types/api';

export function canvasMapErrorMessage(error: unknown, fallback: string): string {
  return error instanceof Error ? error.message : fallback;
}

export function mapFilterForArea(area: Area): CanvasMapFilter {
  return {
    area_id: area.id,
    include_cross_area_links: true,
    include_ghost_devices: true,
  };
}

export function upsertCanvasMap(maps: CanvasMap[], map: CanvasMap): CanvasMap[] {
  const existingIndex = maps.findIndex((candidate) => candidate.id === map.id);
  if (existingIndex === -1) {
    return [...maps, map];
  }

  return maps.map((candidate, index) => (index === existingIndex ? map : candidate));
}

export function fallbackCanvasMap(maps: CanvasMap[]): CanvasMap | null {
  return maps.find((map) => map.is_default) ?? maps[0] ?? null;
}

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
