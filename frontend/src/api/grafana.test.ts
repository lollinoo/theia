import { beforeEach, describe, expect, it, vi } from 'vitest';
import {
  checkPrometheusHealth,
  createGrafanaDashboardProfile,
  fetchGrafanaDashboardConfig,
  saveDeviceGrafanaDashboardOverride,
} from './grafana';

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

function grafanaConfigPayload() {
  return {
    data: {
      profiles: [
        {
          id: 'profile-1',
          name: 'Grafana',
          url_template: 'https://grafana/d/${device_id}',
          variable_source: 'device',
          is_default: true,
          created_at: '2026-01-01T00:00:00Z',
          updated_at: '2026-01-01T00:00:00Z',
        },
      ],
      default_profile_id: 'profile-1',
      device_overrides: {},
    },
  };
}

beforeEach(() => {
  vi.restoreAllMocks();
  document.cookie = 'theia_csrf=grafana-csrf';
});

describe('grafana client', () => {
  it('fetches and parses dashboard config', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(mockResponse(grafanaConfigPayload())));

    const config = await fetchGrafanaDashboardConfig();

    expect(config.profiles).toHaveLength(1);
    expect(config.profiles[0]).toMatchObject({ id: 'profile-1', variable_source: 'hostname' });
    expect(config.default_profile_id).toBe('profile-1');
  });

  it('posts dashboard profile changes with CSRF', async () => {
    const fetchMock = vi.fn().mockResolvedValue(mockResponse(grafanaConfigPayload()));
    vi.stubGlobal('fetch', fetchMock);

    await createGrafanaDashboardProfile({
      name: 'Grafana',
      url_template: 'https://grafana/d/${device_id}',
      variable_source: 'device',
      is_default: true,
    });

    expect(fetchMock).toHaveBeenCalledWith(
      '/api/v1/grafana/dashboard-profiles',
      expect.objectContaining({
        method: 'POST',
        headers: expect.objectContaining({ 'X-CSRF-Token': 'grafana-csrf' }),
      }),
    );
  });

  it('saves device dashboard overrides through the device override endpoint', async () => {
    const fetchMock = vi.fn().mockResolvedValue(mockResponse(grafanaConfigPayload()));
    vi.stubGlobal('fetch', fetchMock);

    await saveDeviceGrafanaDashboardOverride('device/1', {
      profile_id: 'profile-1',
      custom_url: '',
    });

    expect(fetchMock.mock.calls[0][0]).toBe('/api/v1/grafana/device-overrides/device%2F1');
    expect(JSON.parse(fetchMock.mock.calls[0][1].body)).toEqual({
      profile_id: 'profile-1',
      custom_url: '',
    });
  });

  it('normalizes Prometheus health failures to unavailable results', async () => {
    vi.stubGlobal(
      'fetch',
      vi
        .fn()
        .mockResolvedValue(
          mockResponse({ error: 'offline' }, { ok: false, status: 503, statusText: 'Unavailable' }),
        ),
    );

    await expect(checkPrometheusHealth()).resolves.toEqual({
      available: false,
      url: '',
      error: '/api/v1/prometheus/health failed: 503 offline',
    });
  });
});
