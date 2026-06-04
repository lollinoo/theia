import { beforeEach, describe, expect, it, vi } from 'vitest';
import { createArea, deleteArea, fetchAreas, updateArea } from './areas';

function mockResponse(
  body: unknown,
  init: { ok?: boolean; status?: number; statusText?: string } = {},
) {
  const { ok = true, status = 200, statusText = 'OK' } = init;
  return {
    ok,
    status,
    statusText,
    json: () => Promise.resolve(body),
    headers: new Headers(),
  } as unknown as Response;
}

function areaPayload(id: string) {
  return {
    data: {
      id,
      name: 'Core',
      description: 'Core routers',
      color: '#3366ff',
      created_at: '',
      updated_at: '',
    },
  };
}

beforeEach(() => {
  vi.restoreAllMocks();
  document.cookie = 'theia_csrf=areas-csrf';
});

describe('areas client', () => {
  it('fetches and parses areas', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(mockResponse({ data: [areaPayload('a1').data] })));

    await expect(fetchAreas()).resolves.toEqual([
      {
        id: 'a1',
        name: 'Core',
        description: 'Core routers',
        color: '#3366ff',
        device_count: 0,
        created_at: '',
        updated_at: '',
      },
    ]);
  });

  it('creates and updates areas with encoded URLs and CSRF', async () => {
    const fetchMock = vi.fn().mockResolvedValue(mockResponse(areaPayload('area/1')));
    vi.stubGlobal('fetch', fetchMock);
    const payload = { name: 'Core', description: 'Core routers', color: '#3366ff' };

    await createArea(payload);
    await updateArea('area/1', payload);

    expect(fetchMock.mock.calls[0][0]).toBe('/api/v1/areas');
    expect(fetchMock.mock.calls[1][0]).toBe('/api/v1/areas/area%2F1');
    for (const [, options] of fetchMock.mock.calls) {
      expect(options.headers).toEqual(expect.objectContaining({ 'X-CSRF-Token': 'areas-csrf' }));
    }
  });

  it('deletes areas through the encoded endpoint', async () => {
    const fetchMock = vi.fn().mockResolvedValue(mockResponse(null, { status: 204 }));
    vi.stubGlobal('fetch', fetchMock);

    await deleteArea('area/1');

    expect(fetchMock.mock.calls[0][0]).toBe('/api/v1/areas/area%2F1');
    expect(fetchMock.mock.calls[0][1].method).toBe('DELETE');
  });
});
