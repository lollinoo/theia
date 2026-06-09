/**
 * Exercises client API boundary behavior so refactors preserve the documented contract.
 */
import { beforeEach, describe, expect, it, vi } from 'vitest';
import {
  addDeviceToCanvasMap,
  assignAdminUserRole,
  type CreateDevicePayload,
  cancelInstanceBackup,
  changePassword,
  checkDeviceAddressReachability,
  createAdminPasswordReset,
  createAdminUser,
  createBridgeLaunchRequest,
  createCanvasMap,
  createCanvasMapArea,
  createDevice,
  deleteCanvasMap,
  deleteCanvasMapArea,
  deleteDevice,
  duplicateCanvasMap,
  fetchAdminAuditLogs,
  fetchAdminDashboard,
  fetchAdminPermissions,
  fetchAdminRoles,
  fetchAdminUsers,
  fetchBackupFileContent,
  fetchBackupJobs,
  fetchBridgeConnectorConfig,
  fetchBulkBackupRun,
  fetchBulkOperationStatus,
  fetchCanvasBootstrap,
  fetchCanvasMapAreas,
  fetchCanvasMapBootstrap,
  fetchCanvasMaps,
  fetchCanvasMapTopology,
  fetchCanvasTopology,
  fetchCurrentUser,
  fetchDevices,
  fetchInstanceBackups,
  fetchLatestBulkBackupRun,
  fetchLinks,
  fetchOrphanDevices,
  fetchSettings,
  fetchSettingsWithMetadata,
  loginUser,
  logoutUser,
  pauseBulkBackupRun,
  removeAdminUserRole,
  removeDeviceFromCanvasMap,
  resetCanvasBootstrapRequestCache,
  resetPasswordWithToken,
  restoreInstanceBackup,
  resumeBulkBackupRun,
  revealSNMPProfile,
  runTopologyDiscovery,
  setAdminUserStatus,
  setCanvasMapPrimary,
  startBulkBackupRun,
  triggerBulkDownload,
  updateAdminUser,
  updateCanvasMap,
  updateCanvasMapArea,
  updateCanvasMapDeviceAreas,
  updateCanvasMapDeviceVisualColor,
  updateDevice,
  ValidationError,
} from './client';
import { ServerError } from './errors';

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

function emptyTopologyPayload() {
  return {
    schema_version: 1,
    topology_version: 'topo-empty',
    generated_at: '2026-05-07T00:00:00Z',
    devices: [],
    links: [],
    positions: {},
    areas: [],
    capabilities: {
      supports_topology_delta: false,
      supports_position_revision: false,
      supports_area_filtering: true,
    },
    settings: { layout: { version: 1 } },
  };
}

beforeEach(() => {
  vi.restoreAllMocks();
});

describe('password sessions', () => {
  it('fetches the current password session without bearer authorization', async () => {
    const fetchMock = vi.fn().mockResolvedValue(
      mockResponse({
        authenticated: true,
        user: {
          id: 'user-1',
          username: 'alice',
          email: 'alice@example.test',
          display_name: 'Alice',
          status: 'active',
          must_change_password: false,
          roles: ['operator'],
          permissions: ['topology:read'],
          password_hash: 'must-not-leak',
          token_hash: 'must-not-leak',
        },
      }),
    );
    vi.stubGlobal('fetch', fetchMock);

    const result = await fetchCurrentUser();

    expect(result).toEqual({
      authenticated: true,
      user: {
        id: 'user-1',
        username: 'alice',
        email: 'alice@example.test',
        display_name: 'Alice',
        status: 'active',
        must_change_password: false,
        roles: ['operator'],
        permissions: ['topology:read'],
      },
    });
    expect(fetchMock.mock.calls[0][0]).toBe('/api/v1/auth/me');
    expect(fetchMock.mock.calls[0][1]?.headers).not.toHaveProperty('Authorization');
  });

  it('logs in with identifier and password without exposing credentials in the URL', async () => {
    const fetchMock = vi.fn().mockResolvedValue(
      mockResponse({
        authenticated: true,
        user: {
          id: 'user-1',
          username: 'alice',
          email: '',
          display_name: '',
          status: 'active',
          must_change_password: false,
          roles: [],
          permissions: [],
        },
      }),
    );
    vi.stubGlobal('fetch', fetchMock);

    const result = await loginUser({
      identifier: 'alice',
      password: 'secret-password',
    });

    expect(result.authenticated).toBe(true);
    expect(result.user?.username).toBe('alice');
    expect(fetchMock.mock.calls[0][0]).toBe('/api/v1/auth/login');
    expect(fetchMock.mock.calls[0][1]).toMatchObject({
      method: 'POST',
      body: JSON.stringify({
        identifier: 'alice',
        password: 'secret-password',
      }),
    });
    expect(fetchMock.mock.calls[0][1]?.headers).not.toHaveProperty('Authorization');
  });

  it('sends the CSRF cookie value on password mutations and logout', async () => {
    Object.defineProperty(document, 'cookie', {
      configurable: true,
      value: 'theme=dark; theia_csrf=csrf-token-123',
    });
    const fetchMock = vi
      .fn()
      .mockResolvedValueOnce(
        mockResponse({
          authenticated: true,
          user: {
            id: 'user-1',
            username: 'alice',
            email: '',
            display_name: '',
            status: 'active',
            must_change_password: false,
            roles: [],
            permissions: [],
          },
        }),
      )
      .mockResolvedValueOnce(mockResponse({ authenticated: false }));
    vi.stubGlobal('fetch', fetchMock);

    await changePassword({ current_password: 'old', new_password: 'new' });
    await logoutUser();

    expect(fetchMock).toHaveBeenNthCalledWith(
      1,
      '/api/v1/auth/password/change',
      expect.objectContaining({
        method: 'POST',
        headers: expect.objectContaining({ 'X-CSRF-Token': 'csrf-token-123' }),
      }),
    );
    expect(fetchMock).toHaveBeenNthCalledWith(
      2,
      '/api/v1/auth/logout',
      expect.objectContaining({
        method: 'POST',
        headers: expect.objectContaining({ 'X-CSRF-Token': 'csrf-token-123' }),
      }),
    );
    expect(fetchMock.mock.calls[0][1]?.headers).not.toHaveProperty('Authorization');
  });

  it('ignores malformed CSRF cookie values on password mutations', async () => {
    Object.defineProperty(document, 'cookie', {
      configurable: true,
      value: 'theme=dark; theia_csrf=%E0%A4%A',
    });
    const fetchMock = vi.fn().mockResolvedValue(
      mockResponse({
        authenticated: true,
        user: {
          id: 'user-1',
          username: 'alice',
          email: '',
          display_name: '',
          status: 'active',
          must_change_password: false,
          roles: [],
          permissions: [],
        },
      }),
    );
    vi.stubGlobal('fetch', fetchMock);

    await expect(changePassword({ current_password: 'old', new_password: 'new' })).resolves.toEqual(
      expect.objectContaining({ authenticated: true }),
    );

    expect(fetchMock).toHaveBeenCalledWith(
      '/api/v1/auth/password/change',
      expect.objectContaining({
        method: 'POST',
        headers: expect.not.objectContaining({
          'X-CSRF-Token': expect.any(String),
        }),
      }),
    );
  });

  it('completes password reset with token without bearer authorization', async () => {
    const fetchMock = vi
      .fn()
      .mockResolvedValue(mockResponse(null, { ok: true, status: 204, statusText: 'No Content' }));
    vi.stubGlobal('fetch', fetchMock);

    await resetPasswordWithToken({
      token: 'one-time-reset-token',
      new_password: 'Correct Horse Battery Staple Reset 2026!',
    });

    expect(fetchMock).toHaveBeenCalledWith(
      '/api/v1/auth/password/reset',
      expect.objectContaining({
        method: 'POST',
        body: JSON.stringify({
          token: 'one-time-reset-token',
          new_password: 'Correct Horse Battery Staple Reset 2026!',
        }),
        headers: expect.not.objectContaining({
          Authorization: expect.any(String),
        }),
      }),
    );
  });
});

describe('admin API', () => {
  it('fetches dashboard, users, roles, permissions, and audit logs defensively', async () => {
    const fetchMock = vi
      .fn()
      .mockResolvedValueOnce(
        mockResponse({
          stats: {
            total_users: 2,
            active_users: 1,
            disabled_users: 1,
            locked_users: 0,
            recent_logins: 4,
            recent_failed_login_attempts: 1,
          },
          recent_audit_logs: [
            {
              id: 'audit-1',
              actor_user_id: 'admin-user-1',
              action: 'login',
              target_user_id: 'user-1',
              resource: 'user',
              resource_id: 'user-1',
              metadata: { source: 'session' },
              ip_address: '192.0.2.1',
              user_agent: 'Vitest',
              created_at: '2026-05-21T10:00:00Z',
              token_hash: 'must-not-leak',
            },
          ],
        }),
      )
      .mockResolvedValueOnce(
        mockResponse({
          users: [
            {
              id: 'user-1',
              username: 'alice',
              email: 'alice@example.test',
              display_name: 'Alice',
              status: 'active',
              must_change_password: false,
              roles: ['admin'],
              permissions: ['admin:dashboard:read'],
              password_hash: 'must-not-leak',
            },
          ],
        }),
      )
      .mockResolvedValueOnce(
        mockResponse({
          roles: [
            {
              id: 'role-1',
              name: 'admin',
              description: 'Administrators',
              is_system_role: true,
              permissions: [
                {
                  key: 'admin:dashboard:read',
                  description: 'Dashboard access',
                },
              ],
            },
          ],
        }),
      )
      .mockResolvedValueOnce(
        mockResponse({
          permissions: [
            {
              id: 'permission-1',
              key: 'admin:dashboard:read',
              description: 'Dashboard access',
              resource: 'admin',
              action: 'dashboard:read',
            },
          ],
        }),
      )
      .mockResolvedValueOnce(
        mockResponse({
          audit_logs: [
            {
              id: 'audit-2',
              actor_user_id: 'admin-user-1',
              action: 'user.create',
              target_user_id: 'user-2',
              resource: 'user',
              resource_id: 'user-2',
              metadata: { created_username: 'bob' },
              ip_address: '192.0.2.2',
              user_agent: 'Vitest',
              created_at: '2026-05-21T11:00:00Z',
            },
          ],
        }),
      );
    vi.stubGlobal('fetch', fetchMock);

    await expect(fetchAdminDashboard()).resolves.toMatchObject({
      stats: { total_users: 2 },
      recent_audit_logs: [
        {
          id: 'audit-1',
          actor_user_id: 'admin-user-1',
          action: 'login',
          target_user_id: 'user-1',
          resource: 'user',
          resource_id: 'user-1',
          metadata: { source: 'session' },
          ip_address: '192.0.2.1',
          user_agent: 'Vitest',
        },
      ],
    });
    await expect(fetchAdminUsers()).resolves.toEqual([
      expect.objectContaining({ id: 'user-1', username: 'alice' }),
    ]);
    await expect(fetchAdminRoles()).resolves.toEqual([
      expect.objectContaining({
        id: 'role-1',
        name: 'admin',
        permissions: ['admin:dashboard:read'],
      }),
    ]);
    await expect(fetchAdminPermissions()).resolves.toEqual(['admin:dashboard:read']);
    await expect(fetchAdminAuditLogs()).resolves.toEqual([
      expect.objectContaining({
        id: 'audit-2',
        actor_user_id: 'admin-user-1',
        action: 'user.create',
        target_user_id: 'user-2',
        resource: 'user',
        resource_id: 'user-2',
        metadata: { created_username: 'bob' },
        ip_address: '192.0.2.2',
        user_agent: 'Vitest',
      }),
    ]);
  });

  it('sends CSRF on admin mutations and parses one-time reset tokens', async () => {
    Object.defineProperty(document, 'cookie', {
      configurable: true,
      value: 'theia_csrf=admin-csrf',
    });
    const userPayload = {
      user: {
        id: 'user-1',
        username: 'alice',
        email: '',
        display_name: '',
        status: 'active',
        must_change_password: true,
        roles: [],
        permissions: [],
      },
    };
    const fetchMock = vi
      .fn()
      .mockResolvedValueOnce(mockResponse(userPayload))
      .mockResolvedValueOnce(mockResponse(userPayload))
      .mockResolvedValueOnce(mockResponse(userPayload))
      .mockResolvedValueOnce(mockResponse(userPayload))
      .mockResolvedValueOnce(
        mockResponse(null, { ok: true, status: 204, statusText: 'No Content' }),
      )
      .mockResolvedValueOnce(mockResponse({ reset_token: 'one-time-token' }));
    vi.stubGlobal('fetch', fetchMock);

    await createAdminUser({ username: 'alice', password: 'initial-password' });
    await updateAdminUser('user-1', { display_name: 'Alice Admin' });
    await setAdminUserStatus('user-1', 'disabled');
    await assignAdminUserRole('user-1', 'role-1');
    await removeAdminUserRole('user-1', 'role-1');
    const reset = await createAdminPasswordReset('user-1');

    expect(reset.reset_token).toBe('one-time-token');
    for (const [, options] of fetchMock.mock.calls) {
      expect(options?.headers).toEqual(expect.objectContaining({ 'X-CSRF-Token': 'admin-csrf' }));
      expect(options?.headers).not.toHaveProperty('Authorization');
    }
  });
});

describe('fetchDevices', () => {
  it('parses valid device list response', async () => {
    const payload = {
      data: [deviceResource('uuid-1', 'router-01', '10.0.0.1')],
    };
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

describe('fetchOrphanDevices', () => {
  it('fetches and parses orphan device list response', async () => {
    const payload = {
      data: [deviceResource('uuid-orphan', 'orphan-01', '10.0.0.32')],
    };
    const fetchMock = vi.fn().mockResolvedValue(mockResponse(payload));
    vi.stubGlobal('fetch', fetchMock);

    const result = await fetchOrphanDevices();

    expect(fetchMock).toHaveBeenCalledTimes(1);
    expect(fetchMock.mock.calls[0][0]).toBe('/api/v1/devices/orphans');
    expect(result).toHaveLength(1);
    expect(result[0].id).toBe('uuid-orphan');
    expect(result[0].hostname).toBe('orphan-01');
  });

  it('wraps HTTP errors with orphan device context', async () => {
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

    await expect(fetchOrphanDevices()).rejects.toThrow('Failed to fetch orphan devices');
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
      mockResponse({
        data: deviceResource('uuid-2', 'new-router', '10.0.0.2'),
      }),
    );
    vi.stubGlobal('fetch', fetchMock);

    await createDevice(payload);

    expect(fetchMock).toHaveBeenCalledTimes(1);
    const [url, options] = fetchMock.mock.calls[0];
    expect(url).toBe('/api/v1/devices');
    expect(options.method).toBe('POST');
    expect(JSON.parse(options.body)).toEqual(payload);
  });

  it('sends device and address probe_ports in create payloads', async () => {
    const fetchMock = vi.fn().mockResolvedValue(
      mockResponse({
        data: deviceResource('uuid-2', 'new-router', '10.0.0.2'),
      }),
    );
    vi.stubGlobal('fetch', fetchMock);

    const payloadWithProbePorts: CreateDevicePayload = {
      ...payload,
      probe_ports: [22, 8291],
      addresses: [
        {
          address: '10.0.0.2',
          role: 'primary',
          is_primary: true,
          probe_ports: [22],
        },
      ],
    };

    await createDevice(payloadWithProbePorts);

    const [, options] = fetchMock.mock.calls[0];
    expect(JSON.parse(options.body)).toEqual(payloadWithProbePorts);
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

describe('fetchSettingsWithMetadata', () => {
  it('preserves redacted secret metadata without putting secret values in data', async () => {
    const payload = {
      data: {
        bridge_port: '1337',
      },
      meta: {
        secrets: {
          external_token: { present: true, redacted: true },
        },
      },
    };
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(mockResponse(payload)));

    const result = await fetchSettingsWithMetadata();

    expect(result.data.bridge_port).toBe('1337');
    expect(result.data.external_token).toBeUndefined();
    expect(result.secrets.external_token).toEqual({
      present: true,
      redacted: true,
    });
  });
});

describe('fetchBridgeConnectorConfig', () => {
  it('parses connector config and available download targets', async () => {
    const fetchMock = vi.fn().mockResolvedValue(
      mockResponse({
        config: {
          theia_base_url: 'http://localhost:3000',
          theia_origin: 'http://localhost:3000',
        },
        downloads: [
          {
            label: 'Linux x64',
            os: 'linux',
            arch: 'amd64',
            url: '/api/v1/settings/bridge/connector/download/linux/amd64',
            available: true,
          },
          {
            label: '',
            os: 'broken',
            arch: 'amd64',
            url: '/api/v1/settings/bridge/connector/download/broken/amd64',
            available: true,
          },
        ],
      }),
    );
    vi.stubGlobal('fetch', fetchMock);

    const result = await fetchBridgeConnectorConfig();

    expect(result).toEqual({
      config: {
        theia_base_url: 'http://localhost:3000',
        theia_origin: 'http://localhost:3000',
      },
      downloads: [
        {
          label: 'Linux x64',
          os: 'linux',
          arch: 'amd64',
          url: '/api/v1/settings/bridge/connector/download/linux/amd64',
          available: true,
        },
      ],
    });
    expect(fetchMock).toHaveBeenCalledWith('/api/v1/settings/bridge/connector/config', {
      headers: { Accept: 'application/json' },
    });
  });
});

describe('createBridgeLaunchRequest', () => {
  it('requests a user-scoped launch token without sending connector secrets in the body', async () => {
    const fetchMock = vi.fn().mockResolvedValue(
      mockResponse({
        launch_token: 'launch-token',
        expires_at: '2026-05-04T19:15:00Z',
      }),
    );
    vi.stubGlobal('fetch', fetchMock);

    const result = await createBridgeLaunchRequest('device-1');

    expect(result.launch_token).toBe('launch-token');
    expect(fetchMock).toHaveBeenCalledTimes(1);
    const [url, options] = fetchMock.mock.calls[0];
    expect(url).toBe('/api/v1/bridge/launch-requests/device-1');
    expect(options.method).toBe('POST');
    expect(options.body).toBeUndefined();
  });
});

describe('revealSNMPProfile', () => {
  it('posts an explicit reveal reason and parses revealed credentials', async () => {
    const fetchMock = vi.fn().mockResolvedValue(
      mockResponse({
        data: {
          id: 'profile-1',
          name: 'Office SNMP',
          description: '',
          snmp: {
            version: '2c',
            community: 'profile-community',
            community_set: true,
            auth_password_set: false,
            priv_password_set: false,
          },
          created_at: '',
          updated_at: '',
        },
      }),
    );
    vi.stubGlobal('fetch', fetchMock);

    const profile = await revealSNMPProfile('profile-1', 'apply profile');

    expect(profile.snmp.community).toBe('profile-community');
    const [url, options] = fetchMock.mock.calls[0];
    expect(url).toBe('/api/v1/snmp-profiles/profile-1/reveal');
    expect(options.method).toBe('POST');
    expect(JSON.parse(options.body)).toEqual({ reason: 'apply profile' });
  });
});

describe('fetchCanvasTopology', () => {
  it('fetches the canvas read model with an optional ETag validator', async () => {
    const payload = {
      schema_version: 1,
      topology_version: 'topo-abc123',
      generated_at: '2026-04-30T12:00:00Z',
      devices: [deviceResource('uuid-1', 'router-01', '10.0.0.1')],
      links: [],
      positions: {},
      areas: [],
      capabilities: {
        supports_topology_delta: false,
        supports_position_revision: false,
        supports_area_filtering: true,
      },
      settings: { layout: { version: 1 } },
    };
    const fetchMock = vi.fn().mockResolvedValue({
      ...mockResponse(payload),
      headers: new Headers({ ETag: '"canvas-topology-1"' }),
    });
    vi.stubGlobal('fetch', fetchMock);

    const result = await fetchCanvasTopology('"previous"');

    expect(fetchMock).toHaveBeenCalledWith('/api/v1/topology/canvas', {
      headers: {
        Accept: 'application/json',
        'If-None-Match': '"previous"',
      },
    });
    expect(result).toMatchObject({
      status: 'ok',
      etag: '"canvas-topology-1"',
      topology: {
        schema_version: 1,
        topology_version: 'topo-abc123',
      },
    });
  });

  it('returns not-modified when the backend responds with 304', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn().mockResolvedValue({
        ok: false,
        status: 304,
        statusText: 'Not Modified',
        json: () => Promise.resolve(null),
        headers: new Headers({ ETag: '"canvas-topology-1"' }),
      }),
    );

    await expect(fetchCanvasTopology('"canvas-topology-1"')).resolves.toEqual({
      status: 'not-modified',
      etag: '"canvas-topology-1"',
    });
  });
});

describe('fetchCanvasBootstrap', () => {
  beforeEach(() => {
    resetCanvasBootstrapRequestCache();
  });

  it('fetches the full canvas bootstrap including runtime state', async () => {
    const payload = {
      schema_version: 1,
      topology_version: 'topo-abc123',
      runtime_version: 42,
      runtime_identity: 'rt-sha256:abc',
      runtime_snapshot: {
        devices: {
          'uuid-1': {
            device_id: 'uuid-1',
            operational_status: 'down',
            primary_health: 'unreachable',
            runtime_flags: [],
            field_states: {
              cpu: 'missing',
              memory: 'missing',
              uptime: 'error',
            },
            network_reachable: 'false',
            snmp_reachable: 'false',
            reachability: 'hard_down',
            health: 'unknown',
            freshness: 'fresh',
            primary_reason: 'device_unreachable',
            metrics_status: 'unavailable',
            metrics_reason: 'device_unreachable',
            alert_status: 'normal',
            firing_alert_count: 0,
            last_collected_at: null,
            last_polled_at: null,
            expected_poll_interval_seconds: null,
            cpu_percent: null,
            mem_percent: null,
            temp_celsius: null,
            uptime_secs: null,
          },
        },
        links: {},
      },
      generated_at: '2026-04-30T12:00:00Z',
      devices: [deviceResource('uuid-1', 'router-01', '10.0.0.1')],
      links: [],
      positions: {},
      areas: [],
      capabilities: {
        supports_topology_delta: false,
        supports_position_revision: false,
        supports_area_filtering: true,
      },
      settings: { layout: { version: 1 } },
    };
    const fetchMock = vi.fn().mockResolvedValue(mockResponse(payload));
    vi.stubGlobal('fetch', fetchMock);

    const result = await fetchCanvasBootstrap();

    expect(fetchMock).toHaveBeenCalledWith('/api/v1/canvas', {
      headers: {
        Accept: 'application/json',
      },
    });
    expect(result.topology.runtime_version).toBe(42);
    expect(result.topology.runtime_identity).toBe('rt-sha256:abc');
    expect(result.topology.runtime_snapshot?.devices['uuid-1'].operational_status).toBe('down');
  });

  it('deduplicates concurrent full canvas bootstrap requests', async () => {
    const payload = {
      schema_version: 1,
      topology_version: 'topo-abc123',
      runtime_version: 42,
      runtime_identity: 'rt-sha256:abc',
      runtime_snapshot: {
        devices: {},
        links: {},
      },
      generated_at: '2026-04-30T12:00:00Z',
      devices: [],
      links: [],
      positions: {},
      areas: [],
      capabilities: {
        supports_topology_delta: false,
        supports_position_revision: false,
        supports_area_filtering: true,
      },
      settings: { layout: { version: 1 } },
    };
    const fetchMock = vi.fn().mockResolvedValue(mockResponse(payload));
    vi.stubGlobal('fetch', fetchMock);

    const [first, second] = await Promise.all([fetchCanvasBootstrap(), fetchCanvasBootstrap()]);

    expect(fetchMock).toHaveBeenCalledTimes(1);
    expect(first.topology.runtime_version).toBe(42);
    expect(second.topology.runtime_version).toBe(42);
  });

  it('reuses a fresh completed full canvas bootstrap request', async () => {
    const payload = {
      schema_version: 1,
      topology_version: 'topo-abc123',
      runtime_version: 42,
      runtime_identity: 'rt-sha256:abc',
      runtime_snapshot: {
        devices: {},
        links: {},
      },
      generated_at: '2026-04-30T12:00:00Z',
      devices: [],
      links: [],
      positions: {},
      areas: [],
      capabilities: {
        supports_topology_delta: false,
        supports_position_revision: false,
        supports_area_filtering: true,
      },
      settings: { layout: { version: 1 } },
    };
    const fetchMock = vi.fn().mockResolvedValue(mockResponse(payload));
    vi.stubGlobal('fetch', fetchMock);

    const first = await fetchCanvasBootstrap();
    const second = await fetchCanvasBootstrap();

    expect(fetchMock).toHaveBeenCalledTimes(1);
    expect(first.topology.runtime_identity).toBe('rt-sha256:abc');
    expect(second.topology.runtime_identity).toBe('rt-sha256:abc');
  });

  it('bypasses a fresh completed full canvas bootstrap request when forced', async () => {
    const firstPayload = {
      schema_version: 1,
      topology_version: 'topo-abc123',
      runtime_version: 42,
      runtime_identity: 'rt-sha256:abc',
      runtime_snapshot: { devices: {}, links: {} },
      generated_at: '2026-04-30T12:00:00Z',
      devices: [],
      links: [],
      positions: {},
      areas: [],
      capabilities: {
        supports_topology_delta: false,
        supports_position_revision: false,
        supports_area_filtering: true,
      },
      settings: { layout: { version: 1 } },
    };
    const secondPayload = {
      ...firstPayload,
      runtime_version: 43,
      runtime_identity: 'rt-sha256:def',
    };
    const fetchMock = vi
      .fn()
      .mockResolvedValueOnce(mockResponse(firstPayload))
      .mockResolvedValueOnce(mockResponse(secondPayload));
    vi.stubGlobal('fetch', fetchMock);

    const first = await fetchCanvasBootstrap();
    const second = await fetchCanvasBootstrap({ force: true });

    expect(fetchMock).toHaveBeenCalledTimes(2);
    expect(first.topology.runtime_version).toBe(42);
    expect(second.topology.runtime_version).toBe(43);
  });
});

describe('canvas map client', () => {
  beforeEach(() => {
    resetCanvasBootstrapRequestCache();
  });

  it('fetches canvas maps from map list endpoint', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn().mockResolvedValue(
        mockResponse({
          data: [{ id: 'default', name: 'Default', is_default: true, filter: {} }],
        }),
      ),
    );

    await expect(fetchCanvasMaps()).resolves.toHaveLength(1);
    expect(fetch).toHaveBeenCalledWith(
      '/api/v1/canvas/maps',
      expect.objectContaining({ headers: { Accept: 'application/json' } }),
    );
  });

  it('normalizes create conflicts as ValidationError', async () => {
    vi.stubGlobal(
      'fetch',
      vi
        .fn()
        .mockResolvedValue(
          mockResponse(
            { error: 'name exists' },
            { ok: false, status: 409, statusText: 'Conflict' },
          ),
        ),
    );

    await expect(createCanvasMap({ name: 'Default' })).rejects.toBeInstanceOf(ValidationError);
  });

  it('uses map-specific topology and bootstrap endpoints', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(mockResponse(emptyTopologyPayload())));

    await fetchCanvasMapBootstrap('map-1');
    await fetchCanvasMapTopology('map-1', 'etag-1');

    expect(fetch).toHaveBeenNthCalledWith(
      1,
      '/api/v1/canvas/maps/map-1/bootstrap',
      expect.any(Object),
    );
    expect(fetch).toHaveBeenNthCalledWith(
      2,
      '/api/v1/canvas/maps/map-1/topology',
      expect.objectContaining({
        headers: expect.objectContaining({ 'If-None-Match': 'etag-1' }),
      }),
    );
  });

  it('isolates default bootstrap cache from saved map bootstrap entries', async () => {
    const fetchMock = vi
      .fn()
      .mockResolvedValueOnce(
        mockResponse({
          ...emptyTopologyPayload(),
          topology_version: 'default-bootstrap',
        }),
      )
      .mockResolvedValueOnce(
        mockResponse({
          ...emptyTopologyPayload(),
          topology_version: 'map-bootstrap',
        }),
      );
    vi.stubGlobal('fetch', fetchMock);

    const defaultBootstrap = await fetchCanvasBootstrap();
    const mapBootstrap = await fetchCanvasMapBootstrap('__default__');

    expect(fetchMock).toHaveBeenCalledTimes(2);
    expect(fetchMock).toHaveBeenNthCalledWith(
      1,
      '/api/v1/canvas',
      expect.objectContaining({ headers: { Accept: 'application/json' } }),
    );
    expect(fetchMock).toHaveBeenNthCalledWith(
      2,
      '/api/v1/canvas/maps/__default__/bootstrap',
      expect.objectContaining({ headers: { Accept: 'application/json' } }),
    );
    expect(defaultBootstrap.topology.topology_version).toBe('default-bootstrap');
    expect(mapBootstrap.topology.topology_version).toBe('map-bootstrap');
  });

  it('isolates saved map bootstrap cache entries by map id', async () => {
    const fetchMock = vi
      .fn()
      .mockResolvedValueOnce(mockResponse({ ...emptyTopologyPayload(), topology_version: 'map-a' }))
      .mockResolvedValueOnce(
        mockResponse({ ...emptyTopologyPayload(), topology_version: 'map-b' }),
      );
    vi.stubGlobal('fetch', fetchMock);

    const mapA = await fetchCanvasMapBootstrap('map-a');
    const mapB = await fetchCanvasMapBootstrap('map-b');

    expect(fetchMock).toHaveBeenCalledTimes(2);
    expect(fetchMock).toHaveBeenNthCalledWith(
      1,
      '/api/v1/canvas/maps/map-a/bootstrap',
      expect.any(Object),
    );
    expect(fetchMock).toHaveBeenNthCalledWith(
      2,
      '/api/v1/canvas/maps/map-b/bootstrap',
      expect.any(Object),
    );
    expect(mapA.topology.topology_version).toBe('map-a');
    expect(mapB.topology.topology_version).toBe('map-b');
  });

  it('bypasses the saved map bootstrap cache when forced', async () => {
    const fetchMock = vi
      .fn()
      .mockResolvedValueOnce(
        mockResponse({
          ...emptyTopologyPayload(),
          topology_version: 'map-1-initial',
        }),
      )
      .mockResolvedValueOnce(
        mockResponse({
          ...emptyTopologyPayload(),
          topology_version: 'map-1-forced',
        }),
      );
    vi.stubGlobal('fetch', fetchMock);

    const initial = await fetchCanvasMapBootstrap('map-1');
    const forced = await fetchCanvasMapBootstrap('map-1', { force: true });

    expect(fetchMock).toHaveBeenCalledTimes(2);
    expect(initial.topology.topology_version).toBe('map-1-initial');
    expect(forced.topology.topology_version).toBe('map-1-forced');
  });

  it('clears saved map bootstrap cache entries on reset', async () => {
    const fetchMock = vi
      .fn()
      .mockResolvedValueOnce(
        mockResponse({
          ...emptyTopologyPayload(),
          topology_version: 'map-1-before-reset',
        }),
      )
      .mockResolvedValueOnce(
        mockResponse({
          ...emptyTopologyPayload(),
          topology_version: 'map-1-after-reset',
        }),
      );
    vi.stubGlobal('fetch', fetchMock);

    const beforeReset = await fetchCanvasMapBootstrap('map-1');
    resetCanvasBootstrapRequestCache();
    const afterReset = await fetchCanvasMapBootstrap('map-1');

    expect(fetchMock).toHaveBeenCalledTimes(2);
    expect(beforeReset.topology.topology_version).toBe('map-1-before-reset');
    expect(afterReset.topology.topology_version).toBe('map-1-after-reset');
  });

  it('updates and duplicates canvas maps through their map endpoints', async () => {
    const response = mockResponse({
      data: {
        id: 'map-1',
        name: 'Backbone',
        description: '',
        source_area_id: null,
        filter: {},
        is_default: false,
        created_at: '2026-05-07T00:00:00Z',
        updated_at: '2026-05-07T00:00:00Z',
      },
    });
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(response));

    await updateCanvasMap('map-1', { source_area_id: null });
    await duplicateCanvasMap('map-1', { name: 'Backbone Copy' });

    expect(fetch).toHaveBeenNthCalledWith(
      1,
      '/api/v1/canvas/maps/map-1',
      expect.objectContaining({ method: 'PATCH' }),
    );
    expect(fetch).toHaveBeenNthCalledWith(
      2,
      '/api/v1/canvas/maps/map-1/duplicate',
      expect.objectContaining({ method: 'POST' }),
    );
  });

  it('deletes canvas maps through the map endpoint', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn().mockResolvedValue(
        mockResponse(null, {
          ok: true,
          status: 204,
          statusText: 'No Content',
        }),
      ),
    );

    await deleteCanvasMap('map-1');

    expect(fetch).toHaveBeenCalledWith(
      '/api/v1/canvas/maps/map-1',
      expect.objectContaining({ method: 'DELETE' }),
    );
  });

  it('sets a canvas map as primary through the primary endpoint', async () => {
    const response = mockResponse({
      data: {
        id: 'map-1',
        name: 'Backbone',
        description: '',
        source_area_id: null,
        filter: {},
        is_default: true,
        created_at: '2026-05-07T00:00:00Z',
        updated_at: '2026-05-07T00:00:00Z',
      },
    });
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(response));

    const map = await setCanvasMapPrimary('map-1');

    expect(map.is_default).toBe(true);
    expect(fetch).toHaveBeenCalledWith(
      '/api/v1/canvas/maps/map-1/primary',
      expect.objectContaining({ method: 'POST' }),
    );
  });

  it('removes a device from a canvas map without calling the global device endpoint', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn().mockResolvedValue(
        mockResponse(null, {
          ok: true,
          status: 204,
          statusText: 'No Content',
        }),
      ),
    );

    await removeDeviceFromCanvasMap('map/a', 'device 1');

    expect(fetch).toHaveBeenCalledTimes(1);
    expect(fetch).toHaveBeenCalledWith(
      '/api/v1/canvas/maps/map%2Fa/devices/device%201',
      expect.objectContaining({ method: 'DELETE' }),
    );
    expect(fetch).not.toHaveBeenCalledWith('/api/v1/devices/device%201', expect.anything());
  });

  it('adds an existing device to a canvas map through the map membership endpoint', async () => {
    const response = mockResponse({
      data: {
        id: 'map-1',
        name: 'Backbone',
        description: '',
        source_area_id: null,
        filter: {},
        is_default: false,
        device_count: 2,
        link_count: 1,
        position_count: 0,
        created_at: '2026-05-07T00:00:00Z',
        updated_at: '2026-05-07T00:00:00Z',
      },
    });
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(response));

    const map = await addDeviceToCanvasMap('map/a', 'device 1', {
      include_connected_links: true,
    });

    expect(map.device_count).toBe(2);
    expect(fetch).toHaveBeenCalledTimes(1);
    expect(fetch).toHaveBeenCalledWith(
      '/api/v1/canvas/maps/map%2Fa/devices/device%201',
      expect.objectContaining({
        method: 'POST',
        body: JSON.stringify({ include_connected_links: true }),
      }),
    );
    expect(fetch).not.toHaveBeenCalledWith('/api/v1/devices/device%201', expect.anything());
  });

  it('updates canvas map device area membership through a map-scoped endpoint', async () => {
    const response = mockResponse({
      data: {
        id: 'map-1',
        name: 'Backbone',
        description: '',
        source_area_id: null,
        filter: {},
        is_default: false,
        device_count: 2,
        link_count: 1,
        position_count: 0,
        created_at: '2026-05-07T00:00:00Z',
        updated_at: '2026-05-07T00:00:00Z',
      },
    });
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(response));

    const map = await updateCanvasMapDeviceAreas('map/a', {
      device_ids: ['device 1', 'device 2'],
      area_ids: ['area 1'],
    });

    expect(map.device_count).toBe(2);
    expect(fetch).toHaveBeenCalledTimes(1);
    expect(fetch).toHaveBeenCalledWith(
      '/api/v1/canvas/maps/map%2Fa/device-areas',
      expect.objectContaining({
        method: 'PUT',
        body: JSON.stringify({
          device_ids: ['device 1', 'device 2'],
          area_ids: ['area 1'],
        }),
      }),
    );
    expect(fetch).not.toHaveBeenCalledWith('/api/v1/devices/device%201', expect.anything());
  });

  it('updates canvas map device visual color through the map device endpoint', async () => {
    const response = mockResponse({
      data: {
        id: 'map-1',
        name: 'Backbone',
        description: '',
        source_area_id: null,
        filter: {},
        is_default: false,
        device_count: 1,
        link_count: 0,
        position_count: 0,
        created_at: '2026-05-07T00:00:00Z',
        updated_at: '2026-05-07T00:00:00Z',
      },
    });
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(response));

    const map = await updateCanvasMapDeviceVisualColor('map/a', 'device 1', {
      visual_color: '#123ABC',
    });

    expect(map.id).toBe('map-1');
    expect(fetch).toHaveBeenCalledTimes(1);
    expect(fetch).toHaveBeenCalledWith(
      '/api/v1/canvas/maps/map%2Fa/devices/device%201',
      expect.objectContaining({
        method: 'PATCH',
        body: JSON.stringify({ visual_color: '#123ABC' }),
      }),
    );
    expect(fetch).not.toHaveBeenCalledWith('/api/v1/devices/device%201', expect.anything());
  });

  it('manages canvas map areas through map-scoped endpoints', async () => {
    const areaPayload = {
      data: {
        id: 'area-1',
        name: 'Backbone',
        description: 'Core',
        color: '#2979FF',
        device_count: 0,
        created_at: '2026-05-07T00:00:00Z',
        updated_at: '2026-05-07T00:00:00Z',
      },
    };
    const fetchMock = vi
      .fn()
      .mockResolvedValueOnce(mockResponse({ data: [areaPayload.data] }))
      .mockResolvedValueOnce(mockResponse(areaPayload))
      .mockResolvedValueOnce(mockResponse(areaPayload))
      .mockResolvedValueOnce(
        mockResponse(null, { ok: true, status: 204, statusText: 'No Content' }),
      );
    vi.stubGlobal('fetch', fetchMock);

    await expect(fetchCanvasMapAreas('map/a')).resolves.toHaveLength(1);
    await createCanvasMapArea('map/a', {
      name: 'Backbone',
      description: 'Core',
      color: '#2979FF',
    });
    await updateCanvasMapArea('map/a', 'area 1', {
      name: 'Backbone',
      description: 'Core',
      color: '#2979FF',
    });
    await deleteCanvasMapArea('map/a', 'area 1');

    expect(fetchMock).toHaveBeenNthCalledWith(
      1,
      '/api/v1/canvas/maps/map%2Fa/areas',
      expect.objectContaining({ headers: { Accept: 'application/json' } }),
    );
    expect(fetchMock).toHaveBeenNthCalledWith(
      2,
      '/api/v1/canvas/maps/map%2Fa/areas',
      expect.objectContaining({
        method: 'POST',
        body: JSON.stringify({
          name: 'Backbone',
          description: 'Core',
          color: '#2979FF',
        }),
      }),
    );
    expect(fetchMock).toHaveBeenNthCalledWith(
      3,
      '/api/v1/canvas/maps/map%2Fa/areas/area%201',
      expect.objectContaining({
        method: 'PUT',
        body: JSON.stringify({
          name: 'Backbone',
          description: 'Core',
          color: '#2979FF',
        }),
      }),
    );
    expect(fetchMock).toHaveBeenNthCalledWith(
      4,
      '/api/v1/canvas/maps/map%2Fa/areas/area%201',
      expect.objectContaining({ method: 'DELETE' }),
    );
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
    const fetchMock = vi.fn().mockResolvedValue(
      mockResponse({
        data: deviceResource('uuid-1', 'router-01', '10.0.0.1'),
      }),
    );
    vi.stubGlobal('fetch', fetchMock);

    await updateDevice('uuid-1', { poll_interval_override: null });

    expect(fetchMock).toHaveBeenCalledTimes(1);
    const [url, options] = fetchMock.mock.calls[0];
    expect(url).toBe('/api/v1/devices/uuid-1');
    expect(options.method).toBe('PUT');
    expect(JSON.parse(options.body)).toEqual({ poll_interval_override: null });
  });

  it('sends numeric poll_interval_override unchanged', async () => {
    const fetchMock = vi.fn().mockResolvedValue(
      mockResponse({
        data: deviceResource('uuid-1', 'router-01', '10.0.0.1'),
      }),
    );
    vi.stubGlobal('fetch', fetchMock);

    await updateDevice('uuid-1', { poll_interval_override: 30 });

    expect(fetchMock).toHaveBeenCalledTimes(1);
    const [, options] = fetchMock.mock.calls[0];
    expect(JSON.parse(options.body)).toEqual({ poll_interval_override: 30 });
  });

  it('sends boolean polling_enabled unchanged', async () => {
    const fetchMock = vi.fn().mockResolvedValue(
      mockResponse({
        data: deviceResource('uuid-1', 'router-01', '10.0.0.1'),
      }),
    );
    vi.stubGlobal('fetch', fetchMock);

    await updateDevice('uuid-1', { polling_enabled: false });

    const [, options] = fetchMock.mock.calls[0];
    expect(JSON.parse(options.body)).toEqual({ polling_enabled: false });
  });

  it('sends nullable notes unchanged', async () => {
    const fetchMock = vi.fn().mockResolvedValue(
      mockResponse({
        data: deviceResource('uuid-1', 'router-01', '10.0.0.1'),
      }),
    );
    vi.stubGlobal('fetch', fetchMock);

    await updateDevice('uuid-1', { notes: null });
    await updateDevice('uuid-1', { notes: 'Needs maintenance window' });

    expect(JSON.parse(fetchMock.mock.calls[0][1].body)).toEqual({
      notes: null,
    });
    expect(JSON.parse(fetchMock.mock.calls[1][1].body)).toEqual({
      notes: 'Needs maintenance window',
    });
  });

  it('passes topology discovery mode through unchanged', async () => {
    const fetchMock = vi.fn().mockResolvedValue(
      mockResponse({
        data: deviceResource('uuid-1', 'router-01', '10.0.0.1'),
      }),
    );
    vi.stubGlobal('fetch', fetchMock);

    await updateDevice('uuid-1', { topology_discovery_mode: 'bootstrap_once' });

    expect(JSON.parse(fetchMock.mock.calls[0][1].body)).toEqual({
      topology_discovery_mode: 'bootstrap_once',
    });
  });

  it('sends device and address probe_ports in update payloads', async () => {
    const fetchMock = vi.fn().mockResolvedValue(
      mockResponse({
        data: deviceResource('uuid-1', 'router-01', '10.0.0.1'),
      }),
    );
    vi.stubGlobal('fetch', fetchMock);

    await updateDevice('uuid-1', {
      probe_ports: [22, 8291],
      addresses: [
        {
          address: '10.0.0.1',
          role: 'primary',
          is_primary: true,
          probe_ports: [22],
        },
      ],
    });

    expect(JSON.parse(fetchMock.mock.calls[0][1].body)).toEqual({
      probe_ports: [22, 8291],
      addresses: [
        {
          address: '10.0.0.1',
          role: 'primary',
          is_primary: true,
          probe_ports: [22],
        },
      ],
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

describe('checkDeviceAddressReachability', () => {
  it('posts to the device address reachability endpoint and returns address results', async () => {
    const fetchMock = vi.fn().mockResolvedValue(
      mockResponse({
        data: [
          {
            address_id: 'addr-1',
            address: '10.0.0.1',
            role: 'primary',
            label: 'Primary',
            is_primary: true,
            probe_ports: [22],
            reachable: true,
            error: '',
          },
        ],
      }),
    );
    vi.stubGlobal('fetch', fetchMock);

    const result = await checkDeviceAddressReachability('dev-1');

    expect(result).toEqual([
      expect.objectContaining({
        address: '10.0.0.1',
        reachable: true,
        probe_ports: [22],
      }),
    ]);
    expect(fetchMock).toHaveBeenCalledTimes(1);
    const [url, options] = fetchMock.mock.calls[0];
    expect(url).toBe('/api/v1/devices/dev-1/addresses/reachability');
    expect(options.method).toBe('POST');
  });

  it('filters invalid address reachability results and falls back to an empty array', async () => {
    const fetchMock = vi
      .fn()
      .mockResolvedValueOnce(
        mockResponse({
          data: [
            {
              address_id: 'addr-1',
              address: '10.0.0.1',
              role: 'primary',
              label: 'Primary',
              is_primary: true,
              probe_ports: [22],
              reachable: true,
              error: '',
            },
            {
              address_id: 'addr-2',
              address: '198.51.100.10',
              role: 'backup',
              label: 'Backup',
              is_primary: false,
              probe_ports: ['2222'],
              reachable: false,
              error: 'invalid port shape',
            },
          ],
        }),
      )
      .mockResolvedValueOnce(mockResponse({ data: null }));
    vi.stubGlobal('fetch', fetchMock);

    await expect(checkDeviceAddressReachability('dev-1')).resolves.toEqual([
      expect.objectContaining({
        address_id: 'addr-1',
        probe_ports: [22],
      }),
    ]);
    await expect(checkDeviceAddressReachability('dev-1')).resolves.toEqual([]);
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

describe('fetchBackupFileContent', () => {
  it('defaults missing inline metadata to inline with a download URL fallback', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(mockResponse({ data: { content: '' } })));

    const result = await fetchBackupFileContent('file-1');

    expect(result).toEqual({
      content: '',
      inline: true,
      download_url: '/api/v1/backup-files/file-1/download',
      size_bytes: 0,
      max_inline_size_bytes: 0,
    });
  });
});

describe('triggerBulkDownload', () => {
  it('sends the CSRF token without bearer authorization', async () => {
    Object.defineProperty(document, 'cookie', {
      configurable: true,
      value: 'theia_csrf=download-csrf',
    });
    const clickMock = vi.fn();
    const appendChild = vi.spyOn(document.body, 'appendChild');
    const removeChild = vi.spyOn(document.body, 'removeChild');
    const createElement = vi.spyOn(document, 'createElement');
    let downloadAnchor: HTMLAnchorElement | undefined;
    vi.stubGlobal('URL', {
      createObjectURL: vi.fn(() => 'blob:download'),
      revokeObjectURL: vi.fn(),
    });
    createElement.mockImplementation((tagName: string) => {
      const element = document.createElementNS('http://www.w3.org/1999/xhtml', tagName);
      if (tagName === 'a') {
        downloadAnchor = element as HTMLAnchorElement;
        Object.defineProperty(element, 'click', {
          configurable: true,
          value: clickMock,
        });
      }
      return element as HTMLElement;
    });
    const fetchMock = vi.fn().mockResolvedValue({
      ...mockResponse(null),
      headers: new Headers({
        'Content-Disposition': 'attachment; filename="backups.zip"',
      }),
      blob: () => Promise.resolve(new Blob(['zip'])),
    });
    vi.stubGlobal('fetch', fetchMock);

    const result = await triggerBulkDownload(['dev-1'], {
      filename: 'custom-backups.zip',
    });

    expect(fetchMock).toHaveBeenCalledWith(
      '/api/v1/backups/bulk-download',
      expect.objectContaining({
        method: 'POST',
        headers: expect.objectContaining({ 'X-CSRF-Token': 'download-csrf' }),
      }),
    );
    expect(fetchMock.mock.calls[0][1]?.headers).not.toHaveProperty('Authorization');
    expect(clickMock).toHaveBeenCalled();
    expect(downloadAnchor?.download).toBe('custom-backups.zip');
    expect(result).toBe('saved');
    appendChild.mockRestore();
    removeChild.mockRestore();
    createElement.mockRestore();
  });

  it('uses the File System Access API stream when available', async () => {
    Object.defineProperty(document, 'cookie', {
      configurable: true,
      value: 'theia_csrf=download-csrf',
    });
    const writable = new WritableStream<Uint8Array>();
    const createWritable = vi.fn().mockResolvedValue(writable);
    const showSaveFilePicker = vi.fn().mockResolvedValue({ createWritable });
    const blob = vi.fn();
    vi.stubGlobal('showSaveFilePicker', showSaveFilePicker);
    vi.stubGlobal(
      'fetch',
      vi.fn().mockResolvedValue({
        ok: true,
        status: 200,
        statusText: 'OK',
        headers: new Headers({
          'Content-Disposition': 'attachment; filename="backups.zip"',
        }),
        body: new ReadableStream<Uint8Array>({
          start(controller) {
            controller.enqueue(new Uint8Array([1, 2, 3]));
            controller.close();
          },
        }),
        blob,
      } as unknown as Response),
    );

    const result = await triggerBulkDownload(['dev-1'], {
      filename: 'custom-backups.zip',
    });

    expect(showSaveFilePicker).toHaveBeenCalledWith(
      expect.objectContaining({ suggestedName: 'custom-backups.zip' }),
    );
    expect(createWritable).toHaveBeenCalled();
    expect(blob).not.toHaveBeenCalled();
    expect(result).toBe('saved');
  });

  it('cancels the response stream when the save picker is cancelled', async () => {
    const cancel = vi.fn().mockResolvedValue(undefined);
    const showSaveFilePicker = vi
      .fn()
      .mockRejectedValue(new DOMException('cancelled', 'AbortError'));
    const blob = vi.fn();
    vi.stubGlobal('showSaveFilePicker', showSaveFilePicker);
    vi.stubGlobal(
      'fetch',
      vi.fn().mockResolvedValue({
        ok: true,
        status: 200,
        statusText: 'OK',
        headers: new Headers({
          'Content-Disposition': 'attachment; filename="backups.zip"',
        }),
        body: { cancel },
        blob,
      } as unknown as Response),
    );

    const result = await triggerBulkDownload(['dev-1']);

    expect(cancel).toHaveBeenCalled();
    expect(blob).not.toHaveBeenCalled();
    expect(result).toBe('cancelled');
  });

  it('cancels the response stream when the writable file cannot be created', async () => {
    const cancel = vi.fn().mockResolvedValue(undefined);
    const createWritable = vi.fn().mockRejectedValue(new Error('permission denied'));
    const showSaveFilePicker = vi.fn().mockResolvedValue({ createWritable });
    vi.stubGlobal('showSaveFilePicker', showSaveFilePicker);
    vi.stubGlobal(
      'fetch',
      vi.fn().mockResolvedValue({
        ok: true,
        status: 200,
        statusText: 'OK',
        headers: new Headers({
          'Content-Disposition': 'attachment; filename="backups.zip"',
        }),
        body: { cancel },
        blob: vi.fn(),
      } as unknown as Response),
    );

    await expect(triggerBulkDownload(['dev-1'])).rejects.toThrow('permission denied');

    expect(createWritable).toHaveBeenCalled();
    expect(cancel).toHaveBeenCalled();
  });

  it('normalizes 413 responses to a readable bulk download limit error', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn().mockResolvedValue(
        mockResponse(
          {
            error: 'bulk download exceeds files limit: requested 501, maximum 500',
          },
          { ok: false, status: 413, statusText: 'Payload Too Large' },
        ),
      ),
    );

    await expect(triggerBulkDownload(['dev-1'])).rejects.toThrow(
      'Too many backup files selected for bulk download. Maximum 500, requested 501.',
    );
  });

  it('does not open the save picker when the bulk download is rejected before streaming', async () => {
    const showSaveFilePicker = vi.fn().mockResolvedValue({
      createWritable: vi.fn(),
    });
    vi.stubGlobal('showSaveFilePicker', showSaveFilePicker);
    vi.stubGlobal(
      'fetch',
      vi.fn().mockResolvedValue(
        mockResponse(
          {
            error: 'bulk download already in progress for this user',
          },
          { ok: false, status: 429, statusText: 'Too Many Requests' },
        ),
      ),
    );

    await expect(triggerBulkDownload(['dev-1'])).rejects.toThrow(
      'bulk download already in progress for this user',
    );
    expect(showSaveFilePicker).not.toHaveBeenCalled();
  });
});

describe('bulk backup runs', () => {
  const runPayload = {
    id: 'run-1',
    status: 'running',
    batch_size: 10,
    total_count: 2,
    queued_count: 1,
    running_count: 1,
    completed_count: 1,
    success_count: 0,
    failed_count: 0,
    skipped_count: 1,
    cancelled_count: 0,
    file_count: 2,
    byte_count: 579,
    current_device_id: 'dev-1',
    current_device_name: 'router-01',
    current_job_id: 'job-1',
    error_message: '',
    cancel_requested: false,
    created_by: '',
    created_at: '2026-05-26T10:00:00Z',
    items: [
      {
        id: 'item-1',
        run_id: 'run-1',
        device_id: 'dev-1',
        device_name: 'router-01',
        status: 'queued',
        backup_job_id: 'job-1',
        file_count: 2,
        byte_count: 579,
        created_at: '2026-05-26T10:00:00Z',
        updated_at: '2026-05-26T10:00:00Z',
      },
      {
        id: 'item-2',
        run_id: 'run-1',
        device_id: 'dev-2',
        device_name: 'router-02',
        status: 'skipped',
        reason: 'device offline',
        file_count: 0,
        byte_count: 0,
        created_at: '2026-05-26T10:00:00Z',
        updated_at: '2026-05-26T10:00:00Z',
      },
    ],
  };

  it('starts a persistent bulk backup run and parses item progress', async () => {
    Object.defineProperty(document, 'cookie', {
      configurable: true,
      value: 'theia_csrf=bulk-run-csrf',
    });
    const fetchMock = vi.fn().mockResolvedValue(mockResponse({ data: runPayload }));
    vi.stubGlobal('fetch', fetchMock);

    const run = await startBulkBackupRun(['dev-1', 'dev-2']);

    expect(fetchMock).toHaveBeenCalledWith(
      '/api/v1/backups/bulk-runs',
      expect.objectContaining({
        method: 'POST',
        headers: expect.objectContaining({ 'X-CSRF-Token': 'bulk-run-csrf' }),
        body: JSON.stringify({ device_ids: ['dev-1', 'dev-2'] }),
      }),
    );
    expect(run.id).toBe('run-1');
    expect(run).toMatchObject({
      running_count: 1,
      completed_count: 1,
      current_device_id: 'dev-1',
      current_device_name: 'router-01',
      current_job_id: 'job-1',
      file_count: 2,
      byte_count: 579,
    });
    expect(run.items).toEqual([
      expect.objectContaining({
        device_id: 'dev-1',
        status: 'queued',
        backup_job_id: 'job-1',
        file_count: 2,
        byte_count: 579,
      }),
      expect.objectContaining({
        device_id: 'dev-2',
        status: 'skipped',
        reason: 'device offline',
      }),
    ]);
  });

  it('returns the active run from a 409 response so the UI cannot spam starts', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn().mockResolvedValue(
        mockResponse(
          {
            code: 'bulk_backup_run_active',
            error: 'bulk backup run already active',
            data: runPayload,
          },
          { ok: false, status: 409, statusText: 'Conflict' },
        ),
      ),
    );

    const run = await startBulkBackupRun(['dev-1']);

    expect(run.id).toBe('run-1');
    expect(run.status).toBe('running');
  });

  it('normalizes 413 responses from persistent runs to a readable limit error', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn().mockResolvedValue(
        mockResponse(
          {
            error: 'bulk backup run exceeds devices limit: requested 101, maximum 100',
          },
          { ok: false, status: 413, statusText: 'Payload Too Large' },
        ),
      ),
    );

    await expect(startBulkBackupRun(['dev-1'])).rejects.toThrow(
      'Too many devices selected for bulk backup. Maximum 100, requested 101.',
    );
  });

  it('fetches bulk operation status limits and capabilities', async () => {
    const fetchMock = vi.fn().mockResolvedValue(
      mockResponse({
        data: {
          bulk_backup_run: {
            max_devices: 303,
            max_queued_jobs: 404,
            batch_size: 7,
            max_active_runs: 1,
            configurable_concurrency: false,
            distributed: true,
            distributed_max_active_runs: 1,
            can_pause: true,
            can_resume: true,
            can_cancel: true,
          },
          bulk_download: {
            max_devices: 40,
            max_files: 500,
            max_bytes: 104857600,
            max_concurrent_per_actor: 2,
            max_concurrent_global: 3,
            distributed: true,
            distributed_max_concurrent_per_actor: 1,
            distributed_max_concurrent_global: 3,
          },
        },
      }),
    );
    vi.stubGlobal('fetch', fetchMock);

    await expect(fetchBulkOperationStatus()).resolves.toEqual({
      bulk_backup_run: {
        max_devices: 303,
        max_queued_jobs: 404,
        batch_size: 7,
        max_active_runs: 1,
        configurable_concurrency: false,
        distributed: true,
        distributed_max_active_runs: 1,
        can_pause: true,
        can_resume: true,
        can_cancel: true,
      },
      bulk_download: {
        max_devices: 40,
        max_files: 500,
        max_bytes: 104857600,
        max_concurrent_per_actor: 2,
        max_concurrent_global: 3,
        distributed: true,
        distributed_max_concurrent_per_actor: 1,
        distributed_max_concurrent_global: 3,
      },
    });
    expect(fetchMock).toHaveBeenCalledWith(
      '/api/v1/backups/bulk/status',
      expect.objectContaining({
        headers: expect.objectContaining({ Accept: 'application/json' }),
      }),
    );
  });

  it('fetches latest and specific persistent bulk backup runs', async () => {
    const fetchMock = vi
      .fn()
      .mockResolvedValueOnce(mockResponse({ data: null }))
      .mockResolvedValueOnce(mockResponse({ data: runPayload }));
    vi.stubGlobal('fetch', fetchMock);

    await expect(fetchLatestBulkBackupRun()).resolves.toBeNull();
    await expect(fetchBulkBackupRun('run-1')).resolves.toEqual(
      expect.objectContaining({ id: 'run-1', total_count: 2 }),
    );
    expect(fetchMock).toHaveBeenNthCalledWith(
      1,
      '/api/v1/backups/bulk-runs/latest',
      expect.objectContaining({
        headers: expect.objectContaining({ Accept: 'application/json' }),
      }),
    );
    expect(fetchMock).toHaveBeenNthCalledWith(
      2,
      '/api/v1/backups/bulk-runs/run-1',
      expect.objectContaining({
        headers: expect.objectContaining({ Accept: 'application/json' }),
      }),
    );
  });

  it('parses paused persistent bulk backup runs', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn().mockResolvedValue(mockResponse({ data: { ...runPayload, status: 'paused' } })),
    );

    await expect(fetchBulkBackupRun('run-1')).resolves.toEqual(
      expect.objectContaining({ status: 'paused' }),
    );
  });

  it('parses active persistent bulk backup run items', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn().mockResolvedValue(
        mockResponse({
          data: {
            ...runPayload,
            items: [{ ...runPayload.items[0], status: 'active' }],
          },
        }),
      ),
    );

    await expect(fetchBulkBackupRun('run-1')).resolves.toEqual(
      expect.objectContaining({
        items: [expect.objectContaining({ status: 'active' })],
      }),
    );
  });

  it('pauses and resumes persistent bulk backup runs with CSRF', async () => {
    Object.defineProperty(document, 'cookie', {
      configurable: true,
      value: 'theia_csrf=bulk-control-csrf',
    });
    const fetchMock = vi
      .fn()
      .mockResolvedValueOnce(mockResponse({ data: { ...runPayload, status: 'pausing' } }))
      .mockResolvedValueOnce(mockResponse({ data: { ...runPayload, status: 'running' } }));
    vi.stubGlobal('fetch', fetchMock);

    await expect(pauseBulkBackupRun('run-1')).resolves.toEqual(
      expect.objectContaining({ status: 'pausing' }),
    );
    await expect(resumeBulkBackupRun('run-1')).resolves.toEqual(
      expect.objectContaining({ status: 'running' }),
    );
    expect(fetchMock).toHaveBeenNthCalledWith(
      1,
      '/api/v1/backups/bulk-runs/run-1/pause',
      expect.objectContaining({
        method: 'POST',
        headers: expect.objectContaining({ 'X-CSRF-Token': 'bulk-control-csrf' }),
      }),
    );
    expect(fetchMock).toHaveBeenNthCalledWith(
      2,
      '/api/v1/backups/bulk-runs/run-1/resume',
      expect.objectContaining({
        method: 'POST',
        headers: expect.objectContaining({ 'X-CSRF-Token': 'bulk-control-csrf' }),
      }),
    );
  });
});

describe('fetchInstanceBackups', () => {
  it('parses cancelled status and progress metadata', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn().mockResolvedValue(
        mockResponse({
          data: [
            {
              id: 'backup-1',
              file_name: '',
              size_bytes: 0,
              sha256: '',
              migration_version: 0,
              status: 'cancelled',
              error_message: 'cancelled by user',
              trigger: 'manual',
              created_at: '2026-01-01T00:00:00Z',
              progress: {
                phase: 'cancelling',
                message: 'Cancellation requested',
                current: 1,
                total: 2,
              },
            },
          ],
        }),
      ),
    );

    const result = await fetchInstanceBackups();

    expect(result[0].status).toBe('cancelled');
    expect(result[0].progress?.phase).toBe('cancelling');
    expect(result[0].progress?.current).toBe(1);
  });
});

describe('cancelInstanceBackup', () => {
  it('posts to the cancel endpoint and parses the returned backup', async () => {
    const fetchMock = vi.fn().mockResolvedValue(
      mockResponse({
        data: {
          id: 'backup-1',
          file_name: '',
          status: 'cancelled',
          error_message: 'cancelled by user',
          trigger: 'manual',
          created_at: '2026-01-01T00:00:00Z',
        },
      }),
    );
    vi.stubGlobal('fetch', fetchMock);

    const result = await cancelInstanceBackup('backup-1');

    expect(fetchMock.mock.calls[0][0]).toBe('/api/v1/instance-backups/backup-1/cancel');
    expect(fetchMock.mock.calls[0][1]?.method).toBe('POST');
    expect(result.status).toBe('cancelled');
  });
});

describe('restoreInstanceBackup', () => {
  const mockRestoreReportPayload = {
    data: {
      valid: true,
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
    Object.defineProperty(document, 'cookie', {
      configurable: true,
      value: 'theia_csrf=restore-csrf',
    });
    const fetchMock = vi.fn().mockResolvedValue(mockResponse(mockRestoreReportPayload));
    vi.stubGlobal('fetch', fetchMock);
    const file = new File(['test'], 'backup.tar.gz');
    const result = await restoreInstanceBackup(file, true);
    expect(result.valid).toBe(true);
    expect(result.migration_version).toBe(5);
    expect(result.backup_file_count).toBe(3);
    expect(fetchMock).toHaveBeenCalledWith(
      '/api/v1/instance-backups/restore?dry_run=true',
      expect.objectContaining({
        method: 'POST',
        headers: expect.objectContaining({ 'X-CSRF-Token': 'restore-csrf' }),
      }),
    );
    expect(fetchMock.mock.calls[0][1]?.headers).not.toHaveProperty('Authorization');
  });
});
