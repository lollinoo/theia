import { beforeEach, describe, expect, it, vi } from 'vitest';
import { fetchVendorConfig, fetchVendorConfigs, updateVendorConfig } from './vendor';

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

function vendorConfig(name: string) {
  return {
    name,
    display_name: 'MikroTik',
    config: { backup: { enabled: true } },
  };
}

beforeEach(() => {
  vi.restoreAllMocks();
  document.cookie = 'theia_csrf=vendor-csrf';
});

describe('vendor client', () => {
  it('fetches and parses vendor config lists', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn().mockResolvedValue(mockResponse({ data: [vendorConfig('mikrotik')] })),
    );

    await expect(fetchVendorConfigs()).resolves.toEqual([vendorConfig('mikrotik')]);
  });

  it('fetches individual vendor configs through encoded names', async () => {
    const fetchMock = vi.fn().mockResolvedValue(mockResponse({ data: vendorConfig('acme/vendor') }));
    vi.stubGlobal('fetch', fetchMock);

    await expect(fetchVendorConfig('acme/vendor')).resolves.toEqual(vendorConfig('acme/vendor'));
    expect(fetchMock.mock.calls[0][0]).toBe('/api/v1/vendors/acme%2Fvendor');
  });

  it('updates vendor configs with raw config payload and CSRF', async () => {
    const fetchMock = vi.fn().mockResolvedValue(mockResponse({ data: vendorConfig('mikrotik') }));
    vi.stubGlobal('fetch', fetchMock);

    await updateVendorConfig('mikrotik', { backup: { enabled: true } });

    expect(fetchMock).toHaveBeenCalledWith(
      '/api/v1/vendors/mikrotik',
      expect.objectContaining({
        method: 'PUT',
        headers: expect.objectContaining({ 'X-CSRF-Token': 'vendor-csrf' }),
        body: JSON.stringify({ backup: { enabled: true } }),
      }),
    );
  });
});
