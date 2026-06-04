import {
  type CredentialProfile,
  parseCredentialProfileResponse,
  parseCredentialProfilesResponse,
} from '../types/api';
import { requestJSON, requestJSONWithBody } from './transport';

// fetchCredentialProfiles loads saved credential profiles without revealing secrets.
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

// createCredentialProfile sends a new credential profile payload through the CSRF-aware transport.
export async function createCredentialProfile(
  payload: CredentialProfilePayload,
): Promise<CredentialProfile> {
  return parseCredentialProfileResponse(
    await requestJSONWithBody('/api/v1/credential-profiles', 'POST', payload),
  );
}

// updateCredentialProfile replaces one credential profile while keeping response parsing centralized.
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

// deleteCredentialProfile removes one credential profile by ID.
export async function deleteCredentialProfile(id: string): Promise<void> {
  await requestJSONWithBody(`/api/v1/credential-profiles/${encodeURIComponent(id)}`, 'DELETE');
}
