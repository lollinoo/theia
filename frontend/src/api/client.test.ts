import { describe, it, expect, vi, beforeEach } from 'vitest';
import {
  fetchDevices,
  fetchLinks,
  fetchSettings,
  createDevice,
  deleteDevice,
  fetchBackupJobs,
  type CreateDevicePayload,
} from './client';

// Helper to create a mock Response
function mockResponse(body: unknown, init: { ok?: boolean; status?: number; statusText?: string } = {}) {
  const { ok = true, status = 200, statusText = 'OK' } = init;
  return {
    ok,
    status,
    statusText,
    json: () => Promise.resolve(body),
    headers: new Headers(),
  } as unknown as Response;
}

// Valid JSON:API device resource matching parseDevicesResponse expectations
function deviceResource(id: string, hostname: string, ip: string) {
  return {
    id,
    attributes: {
      hostname,
      ip,
      device_type: 'router',
      status: 'up',
      sys_name: hostname,
      sys_descr: 'RouterOS',
      hardware_model: 'RB4011',
      vendor: 'mikrotik',
      managed: true,
      backup_supported: true,
      metrics_source: 'prometheus',
      prometheus_label_name: 'instance',
      prometheus_label_value: `${ip}:9100`,
    },
    relationships: {
      interfaces: { data: [] },
    },
  };
}

beforeEach(() => {
  vi.restoreAllMocks();
});

describe('fetchDevices', () => {
  it('parses valid device list response', async () => {
    const payload = { data: [deviceResource('uuid-1', 'router-01', '10.0.0.1')] };
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(mockResponse(payload)));

    const result = await fetchDevices();

    expect(result).toHaveLength(1);
    expect(result[0].hostname).toBe('router-01');
    expect(result[0].ip).toBe('10.0.0.1');
    expect(result[0].id).toBe('uuid-1');
    expect(result[0].device_type).toBe('router');
    expect(result[0].status).toBe('up');
  });

  it('throws on HTTP error', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn().mockResolvedValue(
        mockResponse({ error: 'Internal Server Error' }, { ok: false, status: 500, statusText: 'Internal Server Error' }),
      ),
    );

    await expect(fetchDevices()).rejects.toThrow('Failed to fetch devices');
  });

  it('constructs correct URL', async () => {
    const fetchMock = vi.fn().mockResolvedValue(mockResponse({ data: [] }));
    vi.stubGlobal('fetch', fetchMock);

    await fetchDevices();

    expect(fetchMock).toHaveBeenCalledTimes(1);
    expect(fetchMock.mock.calls[0][0]).toBe('/api/v1/devices');
  });

  it('returns empty array for empty data', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(mockResponse({ data: [] })));

    const result = await fetchDevices();
    expect(result).toEqual([]);
  });
});

describe('createDevice', () => {
  const payload: CreateDevicePayload = {
    hostname: 'new-router',
    ip: '10.0.0.2',
    snmp: { version: '2c', community: 'public' },
  };

  it('sends POST with correct body', async () => {
    // createDevice uses requestJSONWithBody which calls fetch with method+body
    // The response has { data: { id, attributes, ... } } (single resource, not array)
    const fetchMock = vi.fn().mockResolvedValue(
      mockResponse({ data: deviceResource('uuid-2', 'new-router', '10.0.0.2') }),
    );
    vi.stubGlobal('fetch', fetchMock);

    await createDevice(payload);

    expect(fetchMock).toHaveBeenCalledTimes(1);
    const [url, options] = fetchMock.mock.calls[0];
    expect(url).toBe('/api/v1/devices');
    expect(options.method).toBe('POST');
    expect(JSON.parse(options.body)).toEqual(payload);
  });

  it('throws on validation error', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn().mockResolvedValue(
        mockResponse({ error: 'hostname required' }, { ok: false, status: 400, statusText: 'Bad Request' }),
      ),
    );

    await expect(createDevice(payload)).rejects.toThrow();
  });
});

describe('fetchSettings', () => {
  it('parses settings response', async () => {
    const payload = {
      data: {
        backup_interval: '3600',
        retention_days: '30',
      },
    };
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(mockResponse(payload)));

    const result = await fetchSettings();

    expect(result).toEqual({
      backup_interval: '3600',
      retention_days: '30',
    });
  });

  it('returns empty object for invalid data shape', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(mockResponse({ unexpected: true })));

    const result = await fetchSettings();
    expect(result).toEqual({});
  });

  it('throws on HTTP error', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn().mockResolvedValue(
        mockResponse({ error: 'Unauthorized' }, { ok: false, status: 401, statusText: 'Unauthorized' }),
      ),
    );

    await expect(fetchSettings()).rejects.toThrow('Failed to fetch settings');
  });
});

describe('deleteDevice', () => {
  it('sends DELETE request with correct URL', async () => {
    const fetchMock = vi.fn().mockResolvedValue(
      mockResponse(null, { ok: true, status: 204, statusText: 'No Content' }),
    );
    vi.stubGlobal('fetch', fetchMock);

    await deleteDevice('device-123');

    expect(fetchMock).toHaveBeenCalledTimes(1);
    const [url, options] = fetchMock.mock.calls[0];
    expect(url).toBe('/api/v1/devices/device-123');
    expect(options.method).toBe('DELETE');
  });
});

describe('fetchLinks', () => {
  it('parses valid links response', async () => {
    const payload = {
      data: [
        {
          id: 'link-1',
          source_device_id: 'dev-a',
          source_if_name: 'ether1',
          target_device_id: 'dev-b',
          target_if_name: 'ether2',
          discovery_protocol: 'lldp',
        },
      ],
    };
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(mockResponse(payload)));

    const result = await fetchLinks();

    expect(result).toHaveLength(1);
    expect(result[0].id).toBe('link-1');
    expect(result[0].source_if_name).toBe('ether1');
    expect(result[0].discovery_protocol).toBe('lldp');
  });

  it('constructs correct URL', async () => {
    const fetchMock = vi.fn().mockResolvedValue(mockResponse({ data: [] }));
    vi.stubGlobal('fetch', fetchMock);

    await fetchLinks();
    expect(fetchMock.mock.calls[0][0]).toBe('/api/v1/links');
  });
});

describe('fetchBackupJobs', () => {
  it('parses backup jobs response', async () => {
    const payload = {
      data: [
        {
          id: 'job-1',
          device_id: 'dev-1',
          status: 'success',
          error_message: '',
          created_at: '2026-01-01T00:00:00Z',
          files: [
            {
              id: 'file-1',
              job_id: 'job-1',
              file_type: 'rsc',
              file_name: 'export.rsc',
              file_hash: 'abc123',
              size_bytes: 1024,
              created_at: '2026-01-01T00:00:00Z',
            },
          ],
        },
      ],
    };
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(mockResponse(payload)));

    const result = await fetchBackupJobs('dev-1');

    expect(result).toHaveLength(1);
    expect(result[0].id).toBe('job-1');
    expect(result[0].status).toBe('success');
    expect(result[0].files).toHaveLength(1);
    expect(result[0].files[0].file_name).toBe('export.rsc');
  });

  it('returns empty array when data is not an array', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(mockResponse({ data: null })));

    const result = await fetchBackupJobs('dev-1');
    expect(result).toEqual([]);
  });
});
