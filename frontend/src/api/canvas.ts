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

export function resetCanvasBootstrapRequestCache(): void {
  canvasBootstrapRequests.clear();
  recentCanvasBootstraps.clear();
}

export async function fetchCanvasBootstrap(
  options: FetchCanvasBootstrapOptions = {},
): Promise<{ topology: CanvasTopologyResponse }> {
  return fetchCanvasBootstrapWithCache(
    `default:${defaultCanvasBootstrapCacheKey}`,
    '/api/v1/canvas',
    options,
  );
}

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

export async function fetchCanvasTopology(
  ifNoneMatch?: string,
): Promise<CanvasTopologyFetchResult> {
  return fetchCanvasTopologyFromPath('/api/v1/topology/canvas', ifNoneMatch);
}

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

export async function fetchCanvasMaps(): Promise<CanvasMap[]> {
  return parseCanvasMapsResponse(await requestJSON('/api/v1/canvas/maps'));
}

export async function createCanvasMap(payload: {
  name: string;
  description?: string;
  source_area_id?: string | null;
  source_map_id?: string | null;
  filter?: CanvasMapFilter;
}): Promise<CanvasMap> {
  return parseCanvasMapResponse(await requestJSONWithBody('/api/v1/canvas/maps', 'POST', payload));
}

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

export async function deleteCanvasMap(id: string): Promise<void> {
  await requestJSONWithBody(`/api/v1/canvas/maps/${encodeURIComponent(id)}`, 'DELETE');
}

export async function setCanvasMapPrimary(id: string): Promise<CanvasMap> {
  return parseCanvasMapResponse(
    await requestJSONWithBody(`/api/v1/canvas/maps/${encodeURIComponent(id)}/primary`, 'POST'),
  );
}

export async function removeDeviceFromCanvasMap(mapId: string, deviceId: string): Promise<void> {
  await requestJSONWithBody(
    `/api/v1/canvas/maps/${encodeURIComponent(mapId)}/devices/${encodeURIComponent(deviceId)}`,
    'DELETE',
  );
}

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

export async function fetchCanvasMapAreas(mapId: string): Promise<Area[]> {
  return parseAreasResponse(
    await requestJSON(`/api/v1/canvas/maps/${encodeURIComponent(mapId)}/areas`),
  );
}

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

export async function deleteCanvasMapArea(mapId: string, areaId: string): Promise<void> {
  await requestJSONWithBody(
    `/api/v1/canvas/maps/${encodeURIComponent(mapId)}/areas/${encodeURIComponent(areaId)}`,
    'DELETE',
  );
}

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

export async function fetchCanvasMapTopology(
  mapId: string,
  ifNoneMatch?: string,
): Promise<CanvasTopologyFetchResult> {
  return fetchCanvasTopologyFromPath(
    `/api/v1/canvas/maps/${encodeURIComponent(mapId)}/topology`,
    ifNoneMatch,
  );
}
