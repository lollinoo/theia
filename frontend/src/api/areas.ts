/**
 * Provides frontend API helpers for areas endpoints.
 * Keeps request construction and backend response handling out of UI components.
 */
import { type Area, parseAreaResponse, parseAreasResponse } from '../types/api';
import { requestJSON, requestJSONWithBody } from './transport';

// fetchAreas loads the global area catalog used outside saved canvas maps.
export async function fetchAreas(): Promise<Area[]> {
  return parseAreasResponse(await requestJSON('/api/v1/areas'));
}

// createArea creates a global area and preserves the parser's default field behavior.
export async function createArea(payload: {
  name: string;
  description: string;
  color: string;
}): Promise<Area> {
  return parseAreaResponse(await requestJSONWithBody('/api/v1/areas', 'POST', payload));
}

// updateArea replaces editable metadata for one global area.
export async function updateArea(
  id: string,
  payload: { name: string; description: string; color: string },
): Promise<Area> {
  return parseAreaResponse(
    await requestJSONWithBody(`/api/v1/areas/${encodeURIComponent(id)}`, 'PUT', payload),
  );
}

// deleteArea removes one global area through the mutating JSON transport.
export async function deleteArea(id: string): Promise<void> {
  await requestJSONWithBody(`/api/v1/areas/${encodeURIComponent(id)}`, 'DELETE');
}
