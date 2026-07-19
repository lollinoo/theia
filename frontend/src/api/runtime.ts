/**
 * Provides the compact runtime-only recovery API boundary.
 */
import { parseRuntimeOverviewResponse, type RuntimeOverviewResponse } from '../types/api';
import { requestJSON } from './transport';

const runtimeOverviewPath = '/api/v1/runtime/overview';

/** Fetches one uncached atomic runtime overview snapshot for recovery. */
export async function fetchRuntimeOverview(signal?: AbortSignal): Promise<RuntimeOverviewResponse> {
  return parseRuntimeOverviewResponse(
    await requestJSON(runtimeOverviewPath, {
      cache: 'no-store',
      signal,
    }),
  );
}
