import {
  type Area,
  type CanvasMap,
  type CanvasMapFilter,
  type CanvasTopologyResponse,
  parseAreaResponse,
  parseAreasResponse,
  parseCanvasMapResponse,
  parseCanvasMapsResponse,
  parseCanvasTopologyResponse,
} from '../types/api';
import { type ErrorPayload, requestJSON, requestJSONWithBody } from './transport';

export class CanvasTopologyFetchError extends Error {
  status: number;

  constructor(status: number, message: string) {
    super(message);
    this.name = 'CanvasTopologyFetchError';
    this.status = status;
  }
}

export type CanvasTopologyFetchResult =
  | {
      status: 'ok';
      topology: CanvasTopologyResponse;
      etag?: string;
    }
  | {
      status: 'not-modified';
      etag?: string;
    };

const canvasBootstrapReuseWindowMs = 2000;

type CanvasBootstrapCacheKey = string;

const defaultCanvasBootstrapCacheKey = '__default__';

const canvasBootstrapRequests = new Map<
  CanvasBootstrapCacheKey,
  Promise<{ topology: CanvasTopologyResponse }>
>();
const recentCanvasBootstraps = new Map<
  CanvasBootstrapCacheKey,
  { value: { topology: CanvasTopologyResponse }; expiresAt: number }
>();

type FetchCanvasBootstrapOptions = {
  force?: boolean;
};

// resetCanvasBootstrapRequestCache clears in-flight and short-lived bootstrap reuse state for tests.
export function resetCanvasBootstrapRequestCache(): void {
  canvasBootstrapRequests.clear();
  recentCanvasBootstraps.clear();
}

// fetchCanvasBootstrap loads the default canvas topology bootstrap with request reuse.
export async function fetchCanvasBootstrap(
  options: FetchCanvasBootstrapOptions = {},
): Promise<{ topology: CanvasTopologyResponse }> {
  return fetchCanvasBootstrapWithCache(
    `default:${defaultCanvasBootstrapCacheKey}`,
    '/api/v1/canvas',
    options,
  );
}

// fetchCanvasBootstrapWithCache deduplicates concurrent bootstrap loads and short reuse windows.
function fetchCanvasBootstrapWithCache(
  cacheKey: CanvasBootstrapCacheKey,
  path: string,
  options: FetchCanvasBootstrapOptions = {},
): Promise<{ topology: CanvasTopologyResponse }> {
  const recentBootstrap = recentCanvasBootstraps.get(cacheKey);
  if (options.force !== true && recentBootstrap && Date.now() < recentBootstrap.expiresAt) {
    return Promise.resolve(recentBootstrap.value);
  }

  const pendingRequest = canvasBootstrapRequests.get(cacheKey);
  if (options.force !== true && pendingRequest) {
    return pendingRequest;
  }

  const request = fetchCanvasBootstrapUncached(path)
    .then((result) => {
      recentCanvasBootstraps.set(cacheKey, {
        value: result,
        expiresAt: Date.now() + canvasBootstrapReuseWindowMs,
      });
      return result;
    })
    .finally(() => {
      if (canvasBootstrapRequests.get(cacheKey) === request) {
        canvasBootstrapRequests.delete(cacheKey);
      }
    });
  canvasBootstrapRequests.set(cacheKey, request);
  return request;
}

// fetchCanvasBootstrapUncached performs one bootstrap HTTP request and maps topology errors.
async function fetchCanvasBootstrapUncached(
  path: string,
): Promise<{ topology: CanvasTopologyResponse }> {
  const response = await fetch(path, {
    headers: {
      Accept: 'application/json',
    },
  });

  const payload = (await response.json().catch(() => null)) as ErrorPayload | unknown;

  if (!response.ok) {
    const errorMessage =
      typeof payload === 'object' &&
      payload !== null &&
      'error' in payload &&
      typeof payload.error === 'string'
        ? payload.error
        : response.statusText;
    throw new CanvasTopologyFetchError(
      response.status,
      `${path} failed: ${response.status} ${errorMessage}`,
    );
  }

  return {
    topology: parseCanvasTopologyResponse(payload),
  };
}

// fetchCanvasTopology loads the default topology and preserves 304 ETag semantics.
export async function fetchCanvasTopology(
  ifNoneMatch?: string,
): Promise<CanvasTopologyFetchResult> {
  return fetchCanvasTopologyFromPath('/api/v1/topology/canvas', ifNoneMatch);
}

// fetchCanvasTopologyFromPath performs topology fetches for default and saved-map endpoints.
async function fetchCanvasTopologyFromPath(
  path: string,
  ifNoneMatch?: string,
): Promise<CanvasTopologyFetchResult> {
  const headers: Record<string, string> = {
    Accept: 'application/json',
  };
  if (ifNoneMatch) {
    headers['If-None-Match'] = ifNoneMatch;
  }

  const response = await fetch(path, { headers });
  const etag = response.headers.get('ETag') ?? undefined;

  if (response.status === 304) {
    return {
      status: 'not-modified',
      etag,
    };
  }

  const payload = (await response.json().catch(() => null)) as ErrorPayload | unknown;

  if (!response.ok) {
    const errorMessage =
      typeof payload === 'object' &&
      payload !== null &&
      'error' in payload &&
      typeof payload.error === 'string'
        ? payload.error
        : response.statusText;
    throw new CanvasTopologyFetchError(
      response.status,
      `${path} failed: ${response.status} ${errorMessage}`,
    );
  }

  return {
    status: 'ok',
    topology: parseCanvasTopologyResponse(payload),
    etag,
  };
}

// fetchCanvasMaps lists saved canvas maps through the compatibility client barrel.
export async function fetchCanvasMaps(): Promise<CanvasMap[]> {
  return parseCanvasMapsResponse(await requestJSON('/api/v1/canvas/maps'));
}

// createCanvasMap creates a saved map from direct filters, source areas, or source maps.
export async function createCanvasMap(payload: {
  name: string;
  description?: string;
  source_area_id?: string | null;
  source_map_id?: string | null;
  filter?: CanvasMapFilter;
}): Promise<CanvasMap> {
  return parseCanvasMapResponse(await requestJSONWithBody('/api/v1/canvas/maps', 'POST', payload));
}

// updateCanvasMap patches saved-map metadata while preserving nullable source and filter fields.
export async function updateCanvasMap(
  id: string,
  payload: Partial<{
    name: string;
    description: string;
    source_area_id: string | null;
    filter: CanvasMapFilter;
  }>,
): Promise<CanvasMap> {
  return parseCanvasMapResponse(
    await requestJSONWithBody(`/api/v1/canvas/maps/${encodeURIComponent(id)}`, 'PATCH', payload),
  );
}

// deleteCanvasMap deletes a saved map and lets the backend enforce default-map conflicts.
export async function deleteCanvasMap(id: string): Promise<void> {
  await requestJSONWithBody(`/api/v1/canvas/maps/${encodeURIComponent(id)}`, 'DELETE');
}

// setCanvasMapPrimary promotes one saved map to primary and returns the updated map DTO.
export async function setCanvasMapPrimary(id: string): Promise<CanvasMap> {
  return parseCanvasMapResponse(
    await requestJSONWithBody(`/api/v1/canvas/maps/${encodeURIComponent(id)}/primary`, 'POST'),
  );
}

// removeDeviceFromCanvasMap removes one materialized device membership from a saved map.
export async function removeDeviceFromCanvasMap(mapId: string, deviceId: string): Promise<void> {
  await requestJSONWithBody(
    `/api/v1/canvas/maps/${encodeURIComponent(mapId)}/devices/${encodeURIComponent(deviceId)}`,
    'DELETE',
  );
}

// addDeviceToCanvasMap adds a device and optionally asks the backend to include missing links.
export async function addDeviceToCanvasMap(
  mapId: string,
  deviceId: string,
  payload: { include_connected_links?: boolean } = {},
): Promise<CanvasMap> {
  return parseCanvasMapResponse(
    await requestJSONWithBody(
      `/api/v1/canvas/maps/${encodeURIComponent(mapId)}/devices/${encodeURIComponent(deviceId)}`,
      'POST',
      payload,
    ),
  );
}

// updateCanvasMapDeviceAreas replaces saved-map area assignments for selected devices.
export async function updateCanvasMapDeviceAreas(
  mapId: string,
  payload: { device_ids: string[]; area_ids: string[] },
): Promise<CanvasMap> {
  return parseCanvasMapResponse(
    await requestJSONWithBody(
      `/api/v1/canvas/maps/${encodeURIComponent(mapId)}/device-areas`,
      'PUT',
      payload,
    ),
  );
}

// updateCanvasMapDeviceVisualColor applies map-local visual color metadata to a device.
export async function updateCanvasMapDeviceVisualColor(
  mapId: string,
  deviceId: string,
  payload: { visual_color: string | null },
): Promise<CanvasMap> {
  return parseCanvasMapResponse(
    await requestJSONWithBody(
      `/api/v1/canvas/maps/${encodeURIComponent(mapId)}/devices/${encodeURIComponent(deviceId)}`,
      'PATCH',
      payload,
    ),
  );
}

// fetchCanvasMapAreas loads saved-map areas with imported device counts.
export async function fetchCanvasMapAreas(mapId: string): Promise<Area[]> {
  return parseAreasResponse(
    await requestJSON(`/api/v1/canvas/maps/${encodeURIComponent(mapId)}/areas`),
  );
}

// createCanvasMapArea creates a map-local area snapshot.
export async function createCanvasMapArea(
  mapId: string,
  payload: { name: string; description: string; color: string },
): Promise<Area> {
  return parseAreaResponse(
    await requestJSONWithBody(
      `/api/v1/canvas/maps/${encodeURIComponent(mapId)}/areas`,
      'POST',
      payload,
    ),
  );
}

// updateCanvasMapArea replaces one map-local area snapshot.
export async function updateCanvasMapArea(
  mapId: string,
  areaId: string,
  payload: { name: string; description: string; color: string },
): Promise<Area> {
  return parseAreaResponse(
    await requestJSONWithBody(
      `/api/v1/canvas/maps/${encodeURIComponent(mapId)}/areas/${encodeURIComponent(areaId)}`,
      'PUT',
      payload,
    ),
  );
}

// deleteCanvasMapArea removes one map-local area from a saved map.
export async function deleteCanvasMapArea(mapId: string, areaId: string): Promise<void> {
  await requestJSONWithBody(
    `/api/v1/canvas/maps/${encodeURIComponent(mapId)}/areas/${encodeURIComponent(areaId)}`,
    'DELETE',
  );
}

// duplicateCanvasMap creates a copy that preserves backend-managed memberships and positions.
export async function duplicateCanvasMap(
  id: string,
  payload: { name: string },
): Promise<CanvasMap> {
  return parseCanvasMapResponse(
    await requestJSONWithBody(
      `/api/v1/canvas/maps/${encodeURIComponent(id)}/duplicate`,
      'POST',
      payload,
    ),
  );
}

// fetchCanvasMapBootstrap loads a saved-map topology bootstrap with map-specific cache identity.
export async function fetchCanvasMapBootstrap(
  mapId: string,
  options: FetchCanvasBootstrapOptions = {},
): Promise<{ topology: CanvasTopologyResponse }> {
  return fetchCanvasBootstrapWithCache(
    `map:${mapId}`,
    `/api/v1/canvas/maps/${encodeURIComponent(mapId)}/bootstrap`,
    options,
  );
}

// fetchCanvasMapTopology loads a saved-map topology and preserves 304 ETag semantics.
export async function fetchCanvasMapTopology(
  mapId: string,
  ifNoneMatch?: string,
): Promise<CanvasTopologyFetchResult> {
  return fetchCanvasTopologyFromPath(
    `/api/v1/canvas/maps/${encodeURIComponent(mapId)}/topology`,
    ifNoneMatch,
  );
}
