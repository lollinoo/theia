import { describe, expect, it } from 'vitest';

import type { CanvasMap } from '../../types/api';
import {
  fallbackCanvasMap,
  mapFilterForArea,
  setPrimaryCanvasMap,
  upsertCanvasMap,
} from './canvasMapState';

function map(id: string, isDefault = false): CanvasMap {
  return {
    id,
    name: id,
    is_default: isDefault,
    filter: {},
    created_at: '',
    updated_at: '',
  };
}

describe('canvas map state helpers', () => {
  it('builds an area-scoped map filter that includes cross-area context', () => {
    expect(mapFilterForArea({ id: 'area-1', name: 'Area 1', color: '#fff' })).toEqual({
      area_id: 'area-1',
      include_cross_area_links: true,
      include_ghost_devices: true,
    });
  });

  it('upserts new and existing maps without changing order for replacements', () => {
    const maps = [map('a'), map('b')];

    expect(upsertCanvasMap(maps, map('c')).map((candidate) => candidate.id)).toEqual([
      'a',
      'b',
      'c',
    ]);
    expect(
      upsertCanvasMap(maps, { ...map('b'), name: 'renamed' }).map((candidate) => candidate.name),
    ).toEqual(['a', 'renamed']);
  });

  it('selects the default fallback and marks a primary map exclusively', () => {
    const maps = [map('a'), map('b', true)];

    expect(fallbackCanvasMap(maps)?.id).toBe('b');
    expect(setPrimaryCanvasMap(maps, map('a')).map((candidate) => candidate.is_default)).toEqual([
      true,
      false,
    ]);
  });
});
