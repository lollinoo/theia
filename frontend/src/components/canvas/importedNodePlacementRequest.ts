/** Identifies one import batch that must be positioned once on its destination map. */
export interface ImportedNodePlacementRequest {
  requestId: string;
  mapId: string;
  deviceIds: string[];
}

interface BuildImportedNodePlacementRequestInput {
  fileDigest: string;
  mapId: string;
  deviceIds: Iterable<string>;
}

/** Normalizes import results into a stable one-shot canvas placement request. */
export function buildImportedNodePlacementRequest({
  fileDigest,
  mapId,
  deviceIds,
}: BuildImportedNodePlacementRequestInput): ImportedNodePlacementRequest | null {
  const normalizedDeviceIds = [
    ...new Set([...deviceIds].map((id) => id.trim()).filter(Boolean)),
  ].sort((left, right) => left.localeCompare(right));
  if (!fileDigest || !mapId || normalizedDeviceIds.length === 0) return null;

  return {
    requestId: JSON.stringify([fileDigest, mapId, normalizedDeviceIds]),
    mapId,
    deviceIds: normalizedDeviceIds,
  };
}
