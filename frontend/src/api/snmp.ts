/**
 * Provides frontend API helpers for snmp endpoints.
 * Keeps request construction and backend response handling out of UI components.
 */
import {
  parseSNMPProfileResponse,
  parseSNMPProfilesResponse,
  type SNMPProfile,
} from '../types/api';
import { type SNMPPayload } from './device';
import { requestJSON, requestJSONWithBody } from './transport';

/** Describes the snmpprofile payload contract used by the frontend API boundary. */
export interface SNMPProfilePayload {
  name: string;
  description?: string;
  snmp: SNMPPayload;
}

// fetchSNMPProfiles loads SNMP profiles with secret fields omitted by the backend.
export async function fetchSNMPProfiles(): Promise<SNMPProfile[]> {
  return parseSNMPProfilesResponse(await requestJSON('/api/v1/snmp-profiles'));
}

// createSNMPProfile creates an SNMP credential profile through the shared JSON transport.
export async function createSNMPProfile(payload: SNMPProfilePayload): Promise<SNMPProfile> {
  return parseSNMPProfileResponse(
    await requestJSONWithBody('/api/v1/snmp-profiles', 'POST', payload),
  );
}

// updateSNMPProfile replaces one SNMP profile and normalizes the returned DTO.
export async function updateSNMPProfile(
  id: string,
  payload: SNMPProfilePayload,
): Promise<SNMPProfile> {
  return parseSNMPProfileResponse(
    await requestJSONWithBody(`/api/v1/snmp-profiles/${encodeURIComponent(id)}`, 'PUT', payload),
  );
}

// deleteSNMPProfile removes one SNMP profile by ID.
export async function deleteSNMPProfile(id: string): Promise<void> {
  await requestJSONWithBody(`/api/v1/snmp-profiles/${encodeURIComponent(id)}`, 'DELETE');
}

// revealSNMPProfile requests a privileged secret reveal with the audit reason included.
export async function revealSNMPProfile(id: string, reason: string): Promise<SNMPProfile> {
  return parseSNMPProfileResponse(
    await requestJSONWithBody(`/api/v1/snmp-profiles/${encodeURIComponent(id)}/reveal`, 'POST', {
      reason,
    }),
  );
}
