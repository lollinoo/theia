import type { Device, Link } from '../../types/api';
import { normalizeInterfaceName } from './canvasHelpers';

interface PositionLike {
  x: number;
  y: number;
  pinned?: boolean;
}

export interface TopologyIdentity {
  deviceKeys: string[];
  linkKeys: string[];
  signature: string;
}

function hasUsablePosition(position: PositionLike | undefined): boolean {
  return position !== undefined && Number.isFinite(position.x) && Number.isFinite(position.y);
}

export function topologyDeviceKey(deviceId: string): string {
  return deviceId.trim().toLowerCase();
}

export function topologyLinkKey(link: Link): string {
  const sourceDeviceId = topologyDeviceKey(link.source_device_id);
  const targetDeviceId = topologyDeviceKey(link.target_device_id);
  const sourceIfName = normalizeInterfaceName(link.source_if_name);
  const targetIfName = normalizeInterfaceName(link.target_if_name);

  const endpointKeys = [
    `${sourceDeviceId}:${sourceIfName}`,
    `${targetDeviceId}:${targetIfName}`,
  ].sort();

  if (sourceDeviceId && targetDeviceId && (sourceIfName || targetIfName)) {
    return endpointKeys.join('|');
  }

  const fallbackPair = [sourceDeviceId, targetDeviceId].sort().join('|');
  return `${fallbackPair}|id:${link.id.trim().toLowerCase()}`;
}

export function buildTopologyIdentity(devices: Device[], links: Link[]): TopologyIdentity {
  const deviceKeys = [...new Set(devices.map((device) => topologyDeviceKey(device.id)))].sort();
  const linkKeys = [...new Set(links.map((link) => topologyLinkKey(link)))].sort();

  return {
    deviceKeys,
    linkKeys,
    signature: JSON.stringify({ deviceKeys, linkKeys }),
  };
}

export function collectPlacementDeviceIds(
  devices: Device[],
  currentPositions: Map<string, PositionLike>,
  savedPositions: Map<string, PositionLike>,
  existingDeviceIds: Iterable<string> = [],
): Set<string> {
  const existingKeys = new Set(
    Array.from(existingDeviceIds, (deviceId) => topologyDeviceKey(deviceId)),
  );
  const placementDeviceIds = new Set<string>();

  for (const device of devices) {
    const deviceKey = topologyDeviceKey(device.id);
    const isNewToCanvas = !existingKeys.has(deviceKey);
    const hasCurrentPosition = hasUsablePosition(currentPositions.get(device.id));
    const hasSavedPosition = hasUsablePosition(savedPositions.get(device.id));

    if (isNewToCanvas || (!hasCurrentPosition && !hasSavedPosition)) {
      if (!hasCurrentPosition && !hasSavedPosition) {
        placementDeviceIds.add(device.id);
      }
    }
  }

  return placementDeviceIds;
}
