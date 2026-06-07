/**
 * Exercises SNMP API boundary behavior so refactors preserve the documented contract.
 */
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { setDocumentCookie } from '../test/documentCookie';
import { createSNMPProfile, fetchSNMPProfiles, revealSNMPProfile } from './snmp';

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

function snmpProfile(id: string, community?: string) {
  return {
    id,
    name: 'Office SNMP',
    description: '',
    snmp: {
      version: '2c',
      community,
      community_set: Boolean(community),
      auth_password_set: false,
      priv_password_set: false,
    },
    created_at: '',
    updated_at: '',
  };
}

beforeEach(() => {
  vi.restoreAllMocks();
  setDocumentCookie('theia_csrf=snmp-csrf');
});

describe('snmp client', () => {
  it('fetches and parses SNMP profiles', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn().mockResolvedValue(mockResponse({ data: [snmpProfile('profile-1')] })),
    );

    await expect(fetchSNMPProfiles()).resolves.toMatchObject([
      {
        id: 'profile-1',
        snmp: { version: '2c', community_set: false },
      },
    ]);
  });

  it('creates SNMP profiles with CSRF and payload body', async () => {
    const fetchMock = vi
      .fn()
      .mockResolvedValue(mockResponse({ data: snmpProfile('profile-1', 'public') }));
    vi.stubGlobal('fetch', fetchMock);

    await createSNMPProfile({
      name: 'Office SNMP',
      description: '',
      snmp: { version: '2c', community: 'public' },
    });

    expect(fetchMock).toHaveBeenCalledWith(
      '/api/v1/snmp-profiles',
      expect.objectContaining({
        method: 'POST',
        headers: expect.objectContaining({ 'X-CSRF-Token': 'snmp-csrf' }),
      }),
    );
    expect(JSON.parse(fetchMock.mock.calls[0][1].body)).toEqual({
      name: 'Office SNMP',
      description: '',
      snmp: { version: '2c', community: 'public' },
    });
  });

  it('reveals SNMP profile credentials with an explicit reason', async () => {
    const fetchMock = vi
      .fn()
      .mockResolvedValue(mockResponse({ data: snmpProfile('profile-1', 'private') }));
    vi.stubGlobal('fetch', fetchMock);

    await expect(revealSNMPProfile('profile-1', 'apply profile')).resolves.toMatchObject({
      snmp: { community: 'private' },
    });
    expect(fetchMock.mock.calls[0][0]).toBe('/api/v1/snmp-profiles/profile-1/reveal');
    expect(JSON.parse(fetchMock.mock.calls[0][1].body)).toEqual({ reason: 'apply profile' });
  });
});
