import {
  type SNMPProfile,
  parseSNMPProfileResponse,
  parseSNMPProfilesResponse,
} from '../types/api';
import { type SNMPPayload } from './device';
import { requestJSON, requestJSONWithBody } from './transport';

export interface SNMPProfilePayload {
  name: string;
  description?: string;
  snmp: SNMPPayload;
}

export async function fetchSNMPProfiles(): Promise<SNMPProfile[]> {
  return parseSNMPProfilesResponse(await requestJSON('/api/v1/snmp-profiles'));
}

export async function createSNMPProfile(payload: SNMPProfilePayload): Promise<SNMPProfile> {
  return parseSNMPProfileResponse(
    await requestJSONWithBody('/api/v1/snmp-profiles', 'POST', payload),
  );
}

export async function updateSNMPProfile(
  id: string,
  payload: SNMPProfilePayload,
): Promise<SNMPProfile> {
  return parseSNMPProfileResponse(
    await requestJSONWithBody(`/api/v1/snmp-profiles/${encodeURIComponent(id)}`, 'PUT', payload),
  );
}

export async function deleteSNMPProfile(id: string): Promise<void> {
  await requestJSONWithBody(`/api/v1/snmp-profiles/${encodeURIComponent(id)}`, 'DELETE');
}

export async function revealSNMPProfile(id: string, reason: string): Promise<SNMPProfile> {
  return parseSNMPProfileResponse(
    await requestJSONWithBody(`/api/v1/snmp-profiles/${encodeURIComponent(id)}/reveal`, 'POST', {
      reason,
    }),
  );
}
