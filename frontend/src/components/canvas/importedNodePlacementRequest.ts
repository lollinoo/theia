/** Identifies one import batch that must be positioned once on its destination map. */
export interface ImportedNodePlacementRequest {
  requestId: string;
  mapId: string;
  deviceIds: string[];
  topologyRunId?: string;
  topologyLayoutScope?: 'preserve' | 'reorganize';
}

interface BuildImportedNodePlacementRequestInput {
  fileDigest: string;
  mapId: string;
  deviceIds: Iterable<string>;
  topologyRunId?: string;
  topologyLayoutScope?: 'preserve' | 'reorganize';
}

/** Normalizes import results into a stable one-shot canvas placement request. */
export function buildImportedNodePlacementRequest({
  fileDigest,
  mapId,
  deviceIds,
  topologyRunId,
  topologyLayoutScope,
}: BuildImportedNodePlacementRequestInput): ImportedNodePlacementRequest | null {
  const normalizedDeviceIds = [
    ...new Set([...deviceIds].map((id) => id.trim()).filter(Boolean)),
  ].sort((left, right) => left.localeCompare(right));
  if (!fileDigest || !mapId || normalizedDeviceIds.length === 0) return null;

  const normalizedTopologyRunId = topologyRunId?.trim() ?? '';
  const normalizedTopologyLayoutScope = topologyLayoutScope ?? 'preserve';

  return {
    requestId: JSON.stringify([
      fileDigest,
      mapId,
      normalizedDeviceIds,
      normalizedTopologyRunId,
      normalizedTopologyLayoutScope,
    ]),
    mapId,
    deviceIds: normalizedDeviceIds,
    ...(normalizedTopologyRunId
      ? {
          topologyRunId: normalizedTopologyRunId,
          topologyLayoutScope: normalizedTopologyLayoutScope,
        }
      : {}),
  };
}
