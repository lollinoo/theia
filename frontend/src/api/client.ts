import {
  type Area,
  type CredentialProfile,
  type VendorConfig,
  parseAreaResponse,
  parseAreasResponse,
  parseCredentialProfileResponse,
  parseCredentialProfilesResponse,
} from '../types/api';
import { ServerError, ValidationError } from './errors';
import { requestJSON, requestJSONWithBody } from './transport';

export { ValidationError, ServerError };
export * from './admin';
export * from './auth';
export * from './backup';
export * from './canvas';
export * from './device';
export * from './grafana';
export * from './instanceBackup';
export * from './settings';
export * from './snmp';
export { headersWithCsrf } from './transport';

// --- Credential Profiles ---

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

// --- Areas ---

export async function fetchAreas(): Promise<Area[]> {
  return parseAreasResponse(await requestJSON('/api/v1/areas'));
}

export async function createArea(payload: {
  name: string;
  description: string;
  color: string;
}): Promise<Area> {
  return parseAreaResponse(await requestJSONWithBody('/api/v1/areas', 'POST', payload));
}

export async function updateArea(
  id: string,
  payload: { name: string; description: string; color: string },
): Promise<Area> {
  return parseAreaResponse(
    await requestJSONWithBody(`/api/v1/areas/${encodeURIComponent(id)}`, 'PUT', payload),
  );
}

export async function deleteArea(id: string): Promise<void> {
  await requestJSONWithBody(`/api/v1/areas/${encodeURIComponent(id)}`, 'DELETE');
}

// --- Vendor Configs ---

export async function fetchVendorConfigs(): Promise<VendorConfig[]> {
  const payload = await requestJSON('/api/v1/vendors');
  const data = (payload as Record<string, unknown>)?.data;
  if (!Array.isArray(data)) return [];
  return data.map((item) => {
    const rec = item as Record<string, unknown>;
    return {
      name: typeof rec.name === 'string' ? rec.name : '',
      display_name: typeof rec.display_name === 'string' ? rec.display_name : '',
      config: rec.config as VendorConfig['config'],
    };
  });
}

export async function fetchVendorConfig(name: string): Promise<VendorConfig> {
  const payload = await requestJSON(`/api/v1/vendors/${encodeURIComponent(name)}`);
  const rec = (payload as Record<string, unknown>)?.data as Record<string, unknown>;
  return {
    name: typeof rec.name === 'string' ? rec.name : '',
    display_name: typeof rec.display_name === 'string' ? rec.display_name : '',
    config: rec.config as VendorConfig['config'],
  };
}

export async function updateVendorConfig(
  name: string,
  config: VendorConfig['config'],
): Promise<VendorConfig> {
  const response = await requestJSONWithBody(
    `/api/v1/vendors/${encodeURIComponent(name)}`,
    'PUT',
    config,
  );
  const rec = (response as Record<string, unknown>)?.data as Record<string, unknown>;
  return {
    name: typeof rec.name === 'string' ? rec.name : '',
    display_name: typeof rec.display_name === 'string' ? rec.display_name : '',
    config: rec.config as VendorConfig['config'],
  };
}
