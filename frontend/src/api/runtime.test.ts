/**
 * Exercises the compact runtime recovery API boundary.
 */
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { ServerError } from './errors';
import { fetchRuntimeOverview } from './runtime';

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

function validRuntimeOverview() {
  return {
    schema_version: 1,
    runtime_stream_id: 'runtime-stream-5',
    runtime_version: 5,
    runtime_identity: 'rt-sha256:exact',
    runtime_snapshot: { devices: {}, links: {} },
  };
}

beforeEach(() => {
  vi.restoreAllMocks();
});

describe('fetchRuntimeOverview', () => {
  it('fetches and strictly parses the runtime-only snapshot without cache reuse', async () => {
    const fetchMock = vi.fn().mockResolvedValue(mockResponse(validRuntimeOverview()));
    vi.stubGlobal('fetch', fetchMock);

    await expect(fetchRuntimeOverview()).resolves.toEqual(validRuntimeOverview());
    expect(fetchMock).toHaveBeenCalledWith('/api/v1/runtime/overview', {
      cache: 'no-store',
      headers: {
        Accept: 'application/json',
      },
    });
  });

  it('forwards the recovery abort signal to the transport request', async () => {
    const fetchMock = vi.fn().mockResolvedValue(mockResponse(validRuntimeOverview()));
    vi.stubGlobal('fetch', fetchMock);
    const controller = new AbortController();

    await expect(fetchRuntimeOverview(controller.signal)).resolves.toEqual(validRuntimeOverview());
    expect(fetchMock).toHaveBeenCalledWith('/api/v1/runtime/overview', {
      cache: 'no-store',
      signal: controller.signal,
      headers: {
        Accept: 'application/json',
      },
    });
  });

  it('maps backend error payloads with the response status', async () => {
    vi.stubGlobal(
      'fetch',
      vi
        .fn()
        .mockResolvedValue(
          mockResponse(
            { error: 'runtime temporarily unavailable' },
            { ok: false, status: 503, statusText: 'Service Unavailable' },
          ),
        ),
    );

    await expect(fetchRuntimeOverview()).rejects.toThrow(
      '/api/v1/runtime/overview failed: 503 runtime temporarily unavailable',
    );
  });

  it('reuses the shared server error mapping for internal failures', async () => {
    vi.stubGlobal(
      'fetch',
      vi
        .fn()
        .mockResolvedValue(
          mockResponse(
            { error: 'runtime unavailable (ref: runtime-abc-123)' },
            { ok: false, status: 500, statusText: 'Internal Server Error' },
          ),
        ),
    );

    const request = fetchRuntimeOverview();
    await expect(request).rejects.toBeInstanceOf(ServerError);
    await expect(request).rejects.toEqual(
      expect.objectContaining({
        correlationId: 'runtime-abc-123',
        message: 'Something went wrong (ref: runtime-abc-123)',
      }),
    );
  });

  it('rejects malformed successful payloads through the strict parser', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn().mockResolvedValue(mockResponse({ ...validRuntimeOverview(), runtime_version: -1 })),
    );

    await expect(fetchRuntimeOverview()).rejects.toThrow('invalid runtime overview response');
  });
});
