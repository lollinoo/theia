/**
 * Exercises device API boundary behavior so refactors preserve the documented contract.
 */
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { setDocumentCookie } from '../test/documentCookie';
import {
  type CreateDevicePayload,
  createBridgeLaunchRequest,
  createDevice,
  fetchDevices,
  fetchLinks,
  testSNMPConnection,
  testSSHConnection,
} from './device';
import { ValidationError } from './errors';

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
  setDocumentCookie('theia_csrf=device-csrf');
});

describe('device client', () => {
  it('fetches and parses devices from the device resource module', async () => {
    vi.stubGlobal(
      'fetch',
      vi
        .fn()
        .mockResolvedValue(mockResponse({ data: [deviceResource('dev-1', 'r1', '10.0.0.1')] })),
    );

    const devices = await fetchDevices();

    expect(devices).toHaveLength(1);
    expect(devices[0]).toMatchObject({
      id: 'dev-1',
      hostname: 'r1',
      ip: '10.0.0.1',
      status: 'up',
    });
  });

  it('keeps createDevice validation error behavior', async () => {
    const payload: CreateDevicePayload = {
      hostname: 'r1',
      ip: '10.0.0.1',
      snmp: { version: '2c', community: 'public' },
    };
    vi.stubGlobal(
      'fetch',
      vi
        .fn()
        .mockResolvedValue(
          mockResponse(
            { error: 'a device with IP/host "10.0.0.1" already exists' },
            { ok: false, status: 409, statusText: 'Conflict' },
          ),
        ),
    );

    await expect(createDevice(payload)).rejects.toThrow(ValidationError);
    await expect(createDevice(payload)).rejects.toThrow(
      'a device with IP/host "10.0.0.1" already exists',
    );
  });

  it('fetches and parses topology links', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn().mockResolvedValue(
        mockResponse({
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
        }),
      ),
    );

    const links = await fetchLinks();

    expect(links).toHaveLength(1);
    expect(links[0]).toMatchObject({
      id: 'link-1',
      source_if_name: 'ether1',
      discovery_protocol: 'lldp',
    });
  });

  it('requests bridge launch tokens without a body', async () => {
    const fetchMock = vi.fn().mockResolvedValue(
      mockResponse({
        launch_token: 'launch-token',
        expires_at: '2026-05-04T19:15:00Z',
      }),
    );
    vi.stubGlobal('fetch', fetchMock);

    const result = await createBridgeLaunchRequest('device-1');

    expect(result.launch_token).toBe('launch-token');
    expect(fetchMock).toHaveBeenCalledWith(
      '/api/v1/bridge/launch-requests/device-1',
      expect.objectContaining({
        method: 'POST',
        headers: expect.objectContaining({ 'X-CSRF-Token': 'device-csrf' }),
      }),
    );
    expect(fetchMock.mock.calls[0][1]?.body).toBeUndefined();
  });

  it('parses SNMP test defaults and optional fields', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn().mockResolvedValue(
        mockResponse({
          success: true,
          sys_name: 'router-1',
          sys_descr: 'RouterOS',
          target_ip: '10.0.0.1',
          snmp_version: '2c',
        }),
      ),
    );

    await expect(testSNMPConnection('device-1')).resolves.toEqual({
      success: true,
      sys_name: 'router-1',
      sys_descr: 'RouterOS',
      target_ip: '10.0.0.1',
      snmp_version: '2c',
      error: undefined,
    });
  });

  it('parses SSH test host-key mismatch error codes', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn().mockResolvedValue(
        mockResponse({
          success: false,
          error: 'SSH connection to 10.8.20.1 failed: SSH host key mismatch for 10.8.20.1:22',
          error_code: 'ssh_host_key_mismatch',
        }),
      ),
    );

    await expect(testSSHConnection('device-1')).resolves.toEqual({
      success: false,
      error: 'SSH connection to 10.8.20.1 failed: SSH host key mismatch for 10.8.20.1:22',
      error_code: 'ssh_host_key_mismatch',
    });
  });
});
