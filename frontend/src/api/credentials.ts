import {
  type CredentialProfile,
  parseCredentialProfileResponse,
  parseCredentialProfilesResponse,
} from '../types/api';
import { requestJSON, requestJSONWithBody } from './transport';

export async function fetchCredentialProfiles(): Promise<CredentialProfile[]> {
  return parseCredentialProfilesResponse(await requestJSON('/api/v1/credential-profiles'));
}

export interface CredentialProfilePayload {
  name: string;
  description?: string;
  username: string;
  port: number;
  auth_method: string;
  secret: string;
  role: string;
}

export async function createCredentialProfile(
  payload: CredentialProfilePayload,
): Promise<CredentialProfile> {
  return parseCredentialProfileResponse(
    await requestJSONWithBody('/api/v1/credential-profiles', 'POST', payload),
  );
}

export async function updateCredentialProfile(
  id: string,
  payload: CredentialProfilePayload,
): Promise<CredentialProfile> {
  return parseCredentialProfileResponse(
    await requestJSONWithBody(
      `/api/v1/credential-profiles/${encodeURIComponent(id)}`,
      'PUT',
      payload,
    ),
  );
}

export async function deleteCredentialProfile(id: string): Promise<void> {
  await requestJSONWithBody(`/api/v1/credential-profiles/${encodeURIComponent(id)}`, 'DELETE');
}
