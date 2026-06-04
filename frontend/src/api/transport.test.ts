import { beforeEach, describe, expect, it, vi } from 'vitest';
import { ServerError, ValidationError } from './errors';
import { headersWithCsrf, requestJSON, requestJSONWithBody } from './transport';

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

beforeEach(() => {
  vi.restoreAllMocks();
  Object.defineProperty(document, 'cookie', {
    configurable: true,
    value: '',
  });
});

describe('API transport', () => {
  it('adds the CSRF cookie value to mutating headers', () => {
    Object.defineProperty(document, 'cookie', {
      configurable: true,
      value: 'theme=dark; theia_csrf=csrf-token-123',
    });

    expect(headersWithCsrf({ Accept: 'application/json' })).toEqual({
      Accept: 'application/json',
      'X-CSRF-Token': 'csrf-token-123',
    });
  });

  it('leaves headers unchanged when the CSRF cookie is missing or malformed', () => {
    expect(headersWithCsrf({ Accept: 'application/json' })).toEqual({
      Accept: 'application/json',
    });

    Object.defineProperty(document, 'cookie', {
      configurable: true,
      value: 'theia_csrf=%E0%A4%A',
    });

    expect(headersWithCsrf({ Accept: 'application/json' })).toEqual({
      Accept: 'application/json',
    });
  });

  it('sends Accept headers for JSON reads and maps backend errors', async () => {
    const fetchMock = vi
      .fn()
      .mockResolvedValueOnce(mockResponse({ data: true }))
      .mockResolvedValueOnce(
        mockResponse({ error: 'not allowed' }, { ok: false, status: 403, statusText: 'Forbidden' }),
      );
    vi.stubGlobal('fetch', fetchMock);

    await expect(requestJSON('/api/v1/settings')).resolves.toEqual({ data: true });
    await expect(requestJSON('/api/v1/settings')).rejects.toThrow(
      '/api/v1/settings failed: 403 not allowed',
    );
    expect(fetchMock).toHaveBeenNthCalledWith(1, '/api/v1/settings', {
      headers: { Accept: 'application/json' },
    });
  });

  it('serializes JSON bodies, handles 204, and maps validation errors', async () => {
    Object.defineProperty(document, 'cookie', {
      configurable: true,
      value: 'theia_csrf=csrf-token-123',
    });
    const fetchMock = vi
      .fn()
      .mockResolvedValueOnce(
        mockResponse(null, { ok: true, status: 204, statusText: 'No Content' }),
      )
      .mockResolvedValueOnce(
        mockResponse({ error: 'bad value' }, { ok: false, status: 400, statusText: 'Bad Request' }),
      );
    vi.stubGlobal('fetch', fetchMock);

    await expect(requestJSONWithBody('/api/v1/settings/key', 'PUT', { value: 'x' })).resolves.toBe(
      null,
    );
    await expect(requestJSONWithBody('/api/v1/settings/key', 'PUT', { value: '' })).rejects.toThrow(
      ValidationError,
    );
    expect(fetchMock).toHaveBeenNthCalledWith(
      1,
      '/api/v1/settings/key',
      expect.objectContaining({
        method: 'PUT',
        headers: expect.objectContaining({
          Accept: 'application/json',
          'Content-Type': 'application/json',
          'X-CSRF-Token': 'csrf-token-123',
        }),
        body: JSON.stringify({ value: 'x' }),
      }),
    );
  });

  it('redacts internal errors behind ServerError correlation messages', async () => {
    vi.stubGlobal(
      'fetch',
      vi
        .fn()
        .mockResolvedValue(
          mockResponse(
            { error: 'internal error, ref: abc12345' },
            { ok: false, status: 500, statusText: 'Internal Server Error' },
          ),
        ),
    );

    await expect(requestJSONWithBody('/api/v1/devices', 'POST', {})).rejects.toEqual(
      new ServerError('Something went wrong (ref: abc12345)', 'abc12345'),
    );
  });
});
