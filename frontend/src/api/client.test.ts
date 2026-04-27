import { beforeEach, describe, expect, it, vi } from 'vitest';
import {
  type CreateDevicePayload,
  createDevice,
  deleteDevice,
  fetchBackupJobs,
  fetchDevices,
  fetchLinks,
  fetchSettings,
  restoreInstanceBackup,
  runTopologyDiscovery,
  updateDevice,
} from './client';
import { ServerError, ValidationError } from './errors';

// Helper to create a mock Response
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

// Valid JSON:API device resource matching parseDevicesResponse expectations
function deviceResource(id: string, hostname: string, ip: string) {
  return {
    id,
    attributes: {
      hostname,
      ip,
      notes: null,
      device_type: 'router',
      status: 'up',
      sys_name: hostname,
      sys_descr: 'RouterOS',
      hardware_model: 'RB4011',
      vendor: 'mikrotik',
      managed: true,
      backup_supported: true,
      poll_class: 'standard',
      poll_interval_override: null,
      polling_enabled: true,
      metrics_source: 'prometheus',
      prometheus_label_name: 'instance',
      prometheus_label_value: `${ip}:9100`,
      topology_discovery_mode: 'inherit',
      effective_topology_discovery_mode: 'off',
      topology_bootstrap_state: 'idle',
      last_topology_discovery_at: null,
      last_topology_discovery_result: '',
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
      vi
        .fn()
        .mockResolvedValue(
          mockResponse(
            { error: 'Internal Server Error' },
            { ok: false, status: 500, statusText: 'Internal Server Error' },
          ),
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

  it('round-trips poll classification fields from the API resource', async () => {
    const payload = {
      data: [
        {
          ...deviceResource('uuid-1', 'router-01', '10.0.0.1'),
          attributes: {
            ...deviceResource('uuid-1', 'router-01', '10.0.0.1').attributes,
            poll_class: 'core',
            poll_interval_override: 30,
          },
        },
      ],
    };
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(mockResponse(payload)));

    const result = await fetchDevices();

    expect(result[0].poll_class).toBe('core');
    expect(result[0].poll_interval_override).toBe(30);
  });

  it('parses nullable notes from the API resource', async () => {
    const payload = {
      data: [
        {
          ...deviceResource('uuid-1', 'router-01', '10.0.0.1'),
          attributes: {
            ...deviceResource('uuid-1', 'router-01', '10.0.0.1').attributes,
            notes: 'Installed in rack 7',
          },
        },
      ],
    };
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(mockResponse(payload)));

    const result = await fetchDevices();

    expect(result[0].notes).toBe('Installed in rack 7');
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
    const fetchMock = vi
      .fn()
      .mockResolvedValue(
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
      vi
        .fn()
        .mockResolvedValue(
          mockResponse(
            { error: 'hostname required' },
            { ok: false, status: 400, statusText: 'Bad Request' },
          ),
        ),
    );

    await expect(createDevice(payload)).rejects.toThrow();
  });

  it('throws ValidationError on conflict error', async () => {
    vi.stubGlobal(
      'fetch',
      vi
        .fn()
        .mockResolvedValue(
          mockResponse(
            { error: 'a device with IP/host "10.0.0.2" already exists' },
            { ok: false, status: 409, statusText: 'Conflict' },
          ),
        ),
    );

    await expect(createDevice(payload)).rejects.toThrow(ValidationError);
    await expect(createDevice(payload)).rejects.toThrow(
      'a device with IP/host "10.0.0.2" already exists',
    );
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
      vi
        .fn()
        .mockResolvedValue(
          mockResponse(
            { error: 'Unauthorized' },
            { ok: false, status: 401, statusText: 'Unauthorized' },
          ),
        ),
    );

    await expect(fetchSettings()).rejects.toThrow('Failed to fetch settings');
  });
});

describe('deleteDevice', () => {
  it('sends DELETE request with correct URL', async () => {
    const fetchMock = vi
      .fn()
      .mockResolvedValue(mockResponse(null, { ok: true, status: 204, statusText: 'No Content' }));
    vi.stubGlobal('fetch', fetchMock);

    await deleteDevice('device-123');

    expect(fetchMock).toHaveBeenCalledTimes(1);
    const [url, options] = fetchMock.mock.calls[0];
    expect(url).toBe('/api/v1/devices/device-123');
    expect(options.method).toBe('DELETE');
  });
});

describe('updateDevice', () => {
  it('sends null poll_interval_override unchanged', async () => {
    const fetchMock = vi
      .fn()
      .mockResolvedValue(mockResponse({ data: deviceResource('uuid-1', 'router-01', '10.0.0.1') }));
    vi.stubGlobal('fetch', fetchMock);

    await updateDevice('uuid-1', { poll_interval_override: null });

    expect(fetchMock).toHaveBeenCalledTimes(1);
    const [url, options] = fetchMock.mock.calls[0];
    expect(url).toBe('/api/v1/devices/uuid-1');
    expect(options.method).toBe('PUT');
    expect(JSON.parse(options.body)).toEqual({ poll_interval_override: null });
  });

  it('sends numeric poll_interval_override unchanged', async () => {
    const fetchMock = vi
      .fn()
      .mockResolvedValue(mockResponse({ data: deviceResource('uuid-1', 'router-01', '10.0.0.1') }));
    vi.stubGlobal('fetch', fetchMock);

    await updateDevice('uuid-1', { poll_interval_override: 30 });

    expect(fetchMock).toHaveBeenCalledTimes(1);
    const [, options] = fetchMock.mock.calls[0];
    expect(JSON.parse(options.body)).toEqual({ poll_interval_override: 30 });
  });

  it('sends boolean polling_enabled unchanged', async () => {
    const fetchMock = vi
      .fn()
      .mockResolvedValue(mockResponse({ data: deviceResource('uuid-1', 'router-01', '10.0.0.1') }));
    vi.stubGlobal('fetch', fetchMock);

    await updateDevice('uuid-1', { polling_enabled: false });

    const [, options] = fetchMock.mock.calls[0];
    expect(JSON.parse(options.body)).toEqual({ polling_enabled: false });
  });

  it('sends nullable notes unchanged', async () => {
    const fetchMock = vi
      .fn()
      .mockResolvedValue(mockResponse({ data: deviceResource('uuid-1', 'router-01', '10.0.0.1') }));
    vi.stubGlobal('fetch', fetchMock);

    await updateDevice('uuid-1', { notes: null });
    await updateDevice('uuid-1', { notes: 'Needs maintenance window' });

    expect(JSON.parse(fetchMock.mock.calls[0][1].body)).toEqual({ notes: null });
    expect(JSON.parse(fetchMock.mock.calls[1][1].body)).toEqual({
      notes: 'Needs maintenance window',
    });
  });

  it('passes topology discovery mode through unchanged', async () => {
    const fetchMock = vi
      .fn()
      .mockResolvedValue(mockResponse({ data: deviceResource('uuid-1', 'router-01', '10.0.0.1') }));
    vi.stubGlobal('fetch', fetchMock);

    await updateDevice('uuid-1', { topology_discovery_mode: 'bootstrap_once' });

    expect(JSON.parse(fetchMock.mock.calls[0][1].body)).toEqual({
      topology_discovery_mode: 'bootstrap_once',
    });
  });
});

describe('runTopologyDiscovery', () => {
  it('posts to the device topology discovery endpoint', async () => {
    const fetchMock = vi
      .fn()
      .mockResolvedValue(mockResponse({ status: 'topology_discovery_started' }));
    vi.stubGlobal('fetch', fetchMock);

    await runTopologyDiscovery('uuid-1');

    expect(fetchMock).toHaveBeenCalledTimes(1);
    const [url, options] = fetchMock.mock.calls[0];
    expect(url).toBe('/api/v1/devices/uuid-1/topology-discovery');
    expect(options.method).toBe('POST');
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

describe('restoreInstanceBackup', () => {
  const mockRestoreReportPayload = {
    data: {
      valid: true,
      app_version: '1.4.0',
      git_commit: 'abc1234',
      migration_version: 5,
      created_at: '2026-01-01T00:00:00Z',
      db_size_bytes: 102400,
      backup_file_count: 3,
      total_size_bytes: 204800,
      needs_migration: false,
      current_migration_version: 5,
      message: 'OK',
    },
  };

  it('throws ValidationError on 400', async () => {
    vi.stubGlobal(
      'fetch',
      vi
        .fn()
        .mockResolvedValue(
          mockResponse(
            { error: 'encryption key mismatch' },
            { ok: false, status: 400, statusText: 'Bad Request' },
          ),
        ),
    );
    const file = new File(['test'], 'backup.tar.gz');
    try {
      await restoreInstanceBackup(file, true);
      expect.fail('should have thrown');
    } catch (err) {
      expect(err).toBeInstanceOf(ValidationError);
      expect((err as ValidationError).message).toBe('encryption key mismatch');
    }
  });

  it('throws ServerError with correlationId on 500 with ref', async () => {
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
    const file = new File(['test'], 'backup.tar.gz');
    try {
      await restoreInstanceBackup(file, true);
      expect.fail('should have thrown');
    } catch (err) {
      expect(err).toBeInstanceOf(ServerError);
      expect((err as ServerError).correlationId).toBe('abc12345');
      expect((err as ServerError).message).toBe('Something went wrong (ref: abc12345)');
    }
  });

  it('throws ServerError without correlationId on 500 with no ref', async () => {
    vi.stubGlobal(
      'fetch',
      vi
        .fn()
        .mockResolvedValue(
          mockResponse(
            { error: 'unexpected failure' },
            { ok: false, status: 500, statusText: 'Internal Server Error' },
          ),
        ),
    );
    const file = new File(['test'], 'backup.tar.gz');
    try {
      await restoreInstanceBackup(file, true);
      expect.fail('should have thrown');
    } catch (err) {
      expect(err).toBeInstanceOf(ServerError);
      expect((err as ServerError).correlationId).toBeUndefined();
      expect((err as ServerError).message).toBe('Something went wrong');
    }
  });

  it('throws plain Error for non-400/500 error status (413)', async () => {
    vi.stubGlobal(
      'fetch',
      vi
        .fn()
        .mockResolvedValue(
          mockResponse(
            { error: 'payload too large' },
            { ok: false, status: 413, statusText: 'Payload Too Large' },
          ),
        ),
    );
    const file = new File(['test'], 'backup.tar.gz');
    await expect(restoreInstanceBackup(file, true)).rejects.toThrow(/413/);
    // Must not be ValidationError or ServerError
    try {
      await restoreInstanceBackup(file, true);
      expect.fail('should have thrown');
    } catch (err) {
      expect(err).not.toBeInstanceOf(ValidationError);
      expect(err).not.toBeInstanceOf(ServerError);
    }
  });

  it('returns parsed RestoreReport on 200 success', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(mockResponse(mockRestoreReportPayload)));
    const file = new File(['test'], 'backup.tar.gz');
    const result = await restoreInstanceBackup(file, true);
    expect(result.valid).toBe(true);
    expect(result.app_version).toBe('1.4.0');
    expect(result.migration_version).toBe(5);
    expect(result.backup_file_count).toBe(3);
  });
});
