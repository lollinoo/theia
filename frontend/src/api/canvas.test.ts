/**
 * Exercises canvas API client helpers and bootstrap cache invalidation.
 */
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { setDocumentCookie } from '../test/documentCookie';
import { mockResponse } from '../test/mockResponse';
import {
  addDeviceToCanvasMap,
  createCanvasMapArea,
  deleteCanvasMap,
  deleteCanvasMapArea,
  deleteCanvasMapLinkRoute,
  duplicateCanvasMap,
  fetchCanvasBootstrap,
  fetchCanvasMapBootstrap,
  getCanvasBootstrapRequestKey,
  removeDeviceFromCanvasMap,
  resetCanvasBootstrapRequestCache,
  saveCanvasMapLinkRoute,
  updateCanvasMap,
  updateCanvasMapArea,
  updateCanvasMapDeviceAreas,
  updateCanvasMapDeviceVisualColor,
} from './canvas';

function sampleTopologyPayload() {
  return {
    devices: [],
    links: [],
    areas: [],
    link_routes: {},
  };
}

function sampleMapPayload(id: string) {
  return {
    data: {
      id,
      name: 'Map 1',
      description: 'Test map',
      is_default: false,
      is_primary: false,
      device_count: 1,
      devices: [{ id: 'dev-1', name: 'router-1' }],
      created_at: '',
      updated_at: '',
    },
  };
}

beforeEach(() => {
  vi.restoreAllMocks();
  setDocumentCookie('theia_csrf=canvas-csrf');
  resetCanvasBootstrapRequestCache();
});

afterEach(() => {
  resetCanvasBootstrapRequestCache();
});

describe('canvas API client', () => {
  it('generates expected cache keys', () => {
    expect(getCanvasBootstrapRequestKey()).toBe('default:__default__');
    expect(getCanvasBootstrapRequestKey('map-123')).toBe('map:map-123');
  });

  it('reuses cached bootstrap response within the reuse window', async () => {
    const fetchMock = vi
      .fn()
      .mockResolvedValueOnce(mockResponse(sampleTopologyPayload()))
      .mockResolvedValueOnce(mockResponse(sampleTopologyPayload()));
    vi.stubGlobal('fetch', fetchMock);

    const first = await fetchCanvasMapBootstrap('map-1');
    const second = await fetchCanvasMapBootstrap('map-1');

    expect(fetchMock).toHaveBeenCalledTimes(1);
    expect(second).toEqual(first);
  });

  it('invalidates cached bootstrap when addDeviceToCanvasMap or removeDeviceFromCanvasMap is called', async () => {
    const fetchMock = vi
      .fn()
      .mockResolvedValueOnce(mockResponse(sampleTopologyPayload()))
      .mockResolvedValueOnce(mockResponse(sampleMapPayload('map-1')))
      .mockResolvedValueOnce(mockResponse(sampleTopologyPayload()))
      .mockResolvedValueOnce(mockResponse(null, { status: 204 }))
      .mockResolvedValueOnce(mockResponse(sampleTopologyPayload()));
    vi.stubGlobal('fetch', fetchMock);

    // Initial load
    await fetchCanvasMapBootstrap('map-1');
    expect(fetchMock).toHaveBeenCalledTimes(1);

    // Mutation 1: addDeviceToCanvasMap
    await addDeviceToCanvasMap('map-1', 'dev-2');
    expect(fetchMock).toHaveBeenCalledTimes(2);

    // Bootstrap load should fetch fresh data
    await fetchCanvasMapBootstrap('map-1');
    expect(fetchMock).toHaveBeenCalledTimes(3);

    // Mutation 2: removeDeviceFromCanvasMap
    await removeDeviceFromCanvasMap('map-1', 'dev-2');
    expect(fetchMock).toHaveBeenCalledTimes(4);

    // Bootstrap load should fetch fresh data again
    await fetchCanvasMapBootstrap('map-1');
    expect(fetchMock).toHaveBeenCalledTimes(5);
  });

  it('invalidates cached bootstrap on device areas and visual color updates', async () => {
    const fetchMock = vi
      .fn()
      .mockResolvedValueOnce(mockResponse(sampleTopologyPayload()))
      .mockResolvedValueOnce(mockResponse(sampleMapPayload('map-1')))
      .mockResolvedValueOnce(mockResponse(sampleTopologyPayload()))
      .mockResolvedValueOnce(mockResponse(sampleMapPayload('map-1')))
      .mockResolvedValueOnce(mockResponse(sampleTopologyPayload()));
    vi.stubGlobal('fetch', fetchMock);

    await fetchCanvasMapBootstrap('map-1');
    expect(fetchMock).toHaveBeenCalledTimes(1);

    await updateCanvasMapDeviceAreas('map-1', { device_ids: ['dev-1'], area_ids: ['area-1'] });
    expect(fetchMock).toHaveBeenCalledTimes(2);

    await fetchCanvasMapBootstrap('map-1');
    expect(fetchMock).toHaveBeenCalledTimes(3);

    await updateCanvasMapDeviceVisualColor('map-1', 'dev-1', { visual_color: '#ff0000' });
    expect(fetchMock).toHaveBeenCalledTimes(4);

    await fetchCanvasMapBootstrap('map-1');
    expect(fetchMock).toHaveBeenCalledTimes(5);
  });

  it('invalidates cached bootstrap on link route updates and deletions', async () => {
    const routePayload = {
      data: {
        version: 1,
        waypoints: [{ x: 10, y: 20 }],
      },
    };
    const fetchMock = vi
      .fn()
      .mockResolvedValueOnce(mockResponse(sampleTopologyPayload()))
      .mockResolvedValueOnce(mockResponse(routePayload))
      .mockResolvedValueOnce(mockResponse(sampleTopologyPayload()))
      .mockResolvedValueOnce(mockResponse(null, { status: 204 }))
      .mockResolvedValueOnce(mockResponse(sampleTopologyPayload()));
    vi.stubGlobal('fetch', fetchMock);

    await fetchCanvasMapBootstrap('map-1');
    expect(fetchMock).toHaveBeenCalledTimes(1);

    await saveCanvasMapLinkRoute('map-1', 'link-1', { version: 1, waypoints: [{ x: 10, y: 20 }] });
    expect(fetchMock).toHaveBeenCalledTimes(2);

    await fetchCanvasMapBootstrap('map-1');
    expect(fetchMock).toHaveBeenCalledTimes(3);

    await deleteCanvasMapLinkRoute('map-1', 'link-1');
    expect(fetchMock).toHaveBeenCalledTimes(4);

    await fetchCanvasMapBootstrap('map-1');
    expect(fetchMock).toHaveBeenCalledTimes(5);
  });

  it('invalidates cached bootstrap on area creation, update, and deletion', async () => {
    const areaPayload = {
      data: {
        id: 'area-1',
        name: 'Zone A',
        description: 'Zone A area',
        color: '#00ff00',
        created_at: '',
        updated_at: '',
      },
    };
    const fetchMock = vi
      .fn()
      .mockResolvedValueOnce(mockResponse(sampleTopologyPayload()))
      .mockResolvedValueOnce(mockResponse(areaPayload))
      .mockResolvedValueOnce(mockResponse(sampleTopologyPayload()))
      .mockResolvedValueOnce(mockResponse(areaPayload))
      .mockResolvedValueOnce(mockResponse(sampleTopologyPayload()))
      .mockResolvedValueOnce(mockResponse(null, { status: 204 }))
      .mockResolvedValueOnce(mockResponse(sampleTopologyPayload()));
    vi.stubGlobal('fetch', fetchMock);

    await fetchCanvasMapBootstrap('map-1');
    expect(fetchMock).toHaveBeenCalledTimes(1);

    await createCanvasMapArea('map-1', {
      name: 'Zone A',
      description: 'Zone A area',
      color: '#00ff00',
    });
    expect(fetchMock).toHaveBeenCalledTimes(2);

    await fetchCanvasMapBootstrap('map-1');
    expect(fetchMock).toHaveBeenCalledTimes(3);

    await updateCanvasMapArea('map-1', 'area-1', {
      name: 'Zone A Renamed',
      description: 'Zone A area',
      color: '#00ff00',
    });
    expect(fetchMock).toHaveBeenCalledTimes(4);

    await fetchCanvasMapBootstrap('map-1');
    expect(fetchMock).toHaveBeenCalledTimes(5);

    await deleteCanvasMapArea('map-1', 'area-1');
    expect(fetchMock).toHaveBeenCalledTimes(6);

    await fetchCanvasMapBootstrap('map-1');
    expect(fetchMock).toHaveBeenCalledTimes(7);
  });

  it('clears all bootstrap caches when deleteCanvasMap is called', async () => {
    const fetchMock = vi
      .fn()
      .mockResolvedValueOnce(mockResponse(sampleTopologyPayload()))
      .mockResolvedValueOnce(mockResponse(sampleTopologyPayload()))
      .mockResolvedValueOnce(mockResponse(null, { status: 204 }))
      .mockResolvedValueOnce(mockResponse(sampleTopologyPayload()))
      .mockResolvedValueOnce(mockResponse(sampleTopologyPayload()));
    vi.stubGlobal('fetch', fetchMock);

    // Warm cache for map-1 and default bootstrap
    await fetchCanvasMapBootstrap('map-1');
    await fetchCanvasBootstrap();
    expect(fetchMock).toHaveBeenCalledTimes(2);

    // Deleting any map clears all bootstrap caches
    await deleteCanvasMap('map-1');
    expect(fetchMock).toHaveBeenCalledTimes(3);

    // Both should fetch fresh
    await fetchCanvasMapBootstrap('map-1');
    expect(fetchMock).toHaveBeenCalledTimes(4);

    await fetchCanvasBootstrap();
    expect(fetchMock).toHaveBeenCalledTimes(5);
  });

  it('invalidates source map bootstrap cache on duplicateCanvasMap and updateCanvasMap', async () => {
    const fetchMock = vi
      .fn()
      .mockResolvedValueOnce(mockResponse(sampleTopologyPayload()))
      .mockResolvedValueOnce(mockResponse(sampleMapPayload('map-2')))
      .mockResolvedValueOnce(mockResponse(sampleTopologyPayload()))
      .mockResolvedValueOnce(mockResponse(sampleMapPayload('map-1')))
      .mockResolvedValueOnce(mockResponse(sampleTopologyPayload()));
    vi.stubGlobal('fetch', fetchMock);

    await fetchCanvasMapBootstrap('map-1');
    expect(fetchMock).toHaveBeenCalledTimes(1);

    await duplicateCanvasMap('map-1', { name: 'Map 1 Copy' });
    expect(fetchMock).toHaveBeenCalledTimes(2);

    await fetchCanvasMapBootstrap('map-1');
    expect(fetchMock).toHaveBeenCalledTimes(3);

    await updateCanvasMap('map-1', { name: 'Map 1 Renamed' });
    expect(fetchMock).toHaveBeenCalledTimes(4);

    await fetchCanvasMapBootstrap('map-1');
    expect(fetchMock).toHaveBeenCalledTimes(5);
  });
});
