import { beforeEach, describe, expect, it, vi } from 'vitest';
import {
  createCredentialProfile,
  deleteCredentialProfile,
  fetchCredentialProfiles,
  updateCredentialProfile,
} from './credentials';

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

function credentialProfile(id: string) {
  return {
    id,
    name: 'Admin SSH',
    description: '',
    username: 'admin',
    port: 22,
    auth_method: 'password',
    secret_set: true,
    role: 'admin',
    created_at: '',
    updated_at: '',
  };
}

beforeEach(() => {
  vi.restoreAllMocks();
  document.cookie = 'theia_csrf=credential-csrf';
});

describe('credential client', () => {
  it('fetches and parses credential profiles', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn().mockResolvedValue(mockResponse({ data: [credentialProfile('profile-1')] })),
    );

    await expect(fetchCredentialProfiles()).resolves.toMatchObject([
      { id: 'profile-1', username: 'admin', port: 22 },
    ]);
  });

  it('creates and updates credential profiles with CSRF', async () => {
    const fetchMock = vi
      .fn()
      .mockResolvedValue(mockResponse({ data: credentialProfile('profile-1') }));
    vi.stubGlobal('fetch', fetchMock);
    const payload = {
      name: 'Admin SSH',
      description: '',
      username: 'admin',
      port: 22,
      auth_method: 'password',
      secret: 'secret',
      role: 'admin',
    };

    await createCredentialProfile(payload);
    await updateCredentialProfile('profile-1', payload);

    expect(fetchMock.mock.calls[0][0]).toBe('/api/v1/credential-profiles');
    expect(fetchMock.mock.calls[1][0]).toBe('/api/v1/credential-profiles/profile-1');
    for (const [, options] of fetchMock.mock.calls) {
      expect(options.headers).toEqual(
        expect.objectContaining({ 'X-CSRF-Token': 'credential-csrf' }),
      );
    }
  });

  it('deletes credential profiles through the encoded profile endpoint', async () => {
    const fetchMock = vi.fn().mockResolvedValue(mockResponse(null, { status: 204 }));
    vi.stubGlobal('fetch', fetchMock);

    await deleteCredentialProfile('profile/1');

    expect(fetchMock.mock.calls[0][0]).toBe('/api/v1/credential-profiles/profile%2F1');
    expect(fetchMock.mock.calls[0][1].method).toBe('DELETE');
  });
});
