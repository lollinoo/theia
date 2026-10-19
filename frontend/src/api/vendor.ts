/**
 * Provides frontend API helpers for vendor endpoints.
 * Keeps request construction and backend response handling out of UI components.
 */
import { type VendorConfig } from '../types/api';
import { requestJSON, requestJSONWithBody } from './transport';

function parseVendorConfigRecord(value: unknown): VendorConfig {
  const rec = (value as Record<string, unknown>) ?? {};
  return {
    name: typeof rec.name === 'string' ? rec.name : '',
    display_name: typeof rec.display_name === 'string' ? rec.display_name : '',
    config: rec.config as VendorConfig['config'],
  };
}

// fetchVendorConfigs loads all vendor configuration records and keeps absent lists empty.
export async function fetchVendorConfigs(): Promise<VendorConfig[]> {
  const payload = await requestJSON('/api/v1/vendors');
  const data = (payload as Record<string, unknown>)?.data;
  if (!Array.isArray(data)) return [];
  return data.map(parseVendorConfigRecord);
}

// fetchVendorConfig loads one vendor configuration record by vendor name.
export async function fetchVendorConfig(name: string): Promise<VendorConfig> {
  const payload = await requestJSON(`/api/v1/vendors/${encodeURIComponent(name)}`);
  return parseVendorConfigRecord((payload as Record<string, unknown>)?.data);
}

// updateVendorConfig saves one vendor configuration and returns the normalized record.
export async function updateVendorConfig(
  name: string,
  config: VendorConfig['config'],
): Promise<VendorConfig> {
  const response = await requestJSONWithBody(
    `/api/v1/vendors/${encodeURIComponent(name)}`,
    'PUT',
    config,
  );
  return parseVendorConfigRecord((response as Record<string, unknown>)?.data);
}
