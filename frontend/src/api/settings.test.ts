/**
 * Exercises settings API helpers and in-flight request deduplication.
 */
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { setDocumentCookie } from '../test/documentCookie';
import { mockResponse } from '../test/mockResponse';
import {
  fetchSettings,
  fetchSettingsWithMetadata,
  resetSettingsRequestCacheForTests,
  updateSetting,
} from './settings';

function sampleSettingsPayload() {
  return {
    data: {
      site_name: 'Theia Production',
      retention_days: '30',
    },
    secrets: {
      api_token: {
        present: true,
        redacted: true,
      },
    },
  };
}

beforeEach(() => {
  vi.restoreAllMocks();
  setDocumentCookie('theia_csrf=settings-csrf');
  resetSettingsRequestCacheForTests();
});

afterEach(() => {
  resetSettingsRequestCacheForTests();
});

describe('settings client', () => {
  it('deduplicates concurrent fetchSettingsWithMetadata requests', async () => {
    let resolveFetch!: (res: Response) => void;
    const pendingFetch = new Promise<Response>((resolve) => {
      resolveFetch = resolve;
    });

    const fetchMock = vi.fn().mockImplementation(() => pendingFetch);
    vi.stubGlobal('fetch', fetchMock);

    const call1 = fetchSettingsWithMetadata();
    const call2 = fetchSettingsWithMetadata();
    const call3 = fetchSettings();

    expect(fetchMock).toHaveBeenCalledTimes(1);

    resolveFetch(mockResponse(sampleSettingsPayload()));

    const [res1, res2, res3] = await Promise.all([call1, call2, call3]);
    expect(res1.data.site_name).toBe('Theia Production');
    expect(res2).toBe(res1);
    expect(res3.site_name).toBe('Theia Production');
  });

  it('issues a fresh request after the previous in-flight request finishes', async () => {
    const fetchMock = vi
      .fn()
      .mockResolvedValueOnce(mockResponse(sampleSettingsPayload()))
      .mockResolvedValueOnce(
        mockResponse({
          data: { site_name: 'Updated Site' },
          secrets: {},
        }),
      );
    vi.stubGlobal('fetch', fetchMock);

    const first = await fetchSettingsWithMetadata();
    expect(first.data.site_name).toBe('Theia Production');
    expect(fetchMock).toHaveBeenCalledTimes(1);

    const second = await fetchSettingsWithMetadata();
    expect(second.data.site_name).toBe('Updated Site');
    expect(fetchMock).toHaveBeenCalledTimes(2);
  });

  it('resets in-flight request on error so retries can proceed', async () => {
    const fetchMock = vi
      .fn()
      .mockResolvedValueOnce(
        mockResponse({ error: 'service unavailable' }, { ok: false, status: 503 }),
      )
      .mockResolvedValueOnce(mockResponse(sampleSettingsPayload()));
    vi.stubGlobal('fetch', fetchMock);

    await expect(fetchSettingsWithMetadata()).rejects.toThrow(/Failed to fetch settings/);
    expect(fetchMock).toHaveBeenCalledTimes(1);

    const recovered = await fetchSettingsWithMetadata();
    expect(recovered.data.site_name).toBe('Theia Production');
    expect(fetchMock).toHaveBeenCalledTimes(2);
  });

  it('clears in-flight cache on resetSettingsRequestCacheForTests', async () => {
    let resolveFirst!: (res: Response) => void;
    const firstPromise = new Promise<Response>((resolve) => {
      resolveFirst = resolve;
    });
    const fetchMock = vi
      .fn()
      .mockImplementationOnce(() => firstPromise)
      .mockResolvedValueOnce(mockResponse(sampleSettingsPayload()));
    vi.stubGlobal('fetch', fetchMock);

    void fetchSettingsWithMetadata();
    expect(fetchMock).toHaveBeenCalledTimes(1);

    resetSettingsRequestCacheForTests();

    void fetchSettingsWithMetadata();
    expect(fetchMock).toHaveBeenCalledTimes(2);

    resolveFirst(mockResponse(sampleSettingsPayload()));
  });

  it('clears in-flight cache when updateSetting is called', async () => {
    const fetchMock = vi
      .fn()
      .mockResolvedValueOnce(mockResponse(null, { status: 200 }))
      .mockResolvedValueOnce(mockResponse(sampleSettingsPayload()));
    vi.stubGlobal('fetch', fetchMock);

    await updateSetting('site_name', 'New Name');
    expect(fetchMock).toHaveBeenCalledWith(
      '/api/v1/settings/site_name',
      expect.objectContaining({
        method: 'PUT',
        body: JSON.stringify({ value: 'New Name' }),
      }),
    );

    const fresh = await fetchSettingsWithMetadata();
    expect(fresh.data.site_name).toBe('Theia Production');
  });
});
