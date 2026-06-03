import {
  fetchCanvasBootstrap,
  fetchCanvasMapBootstrap,
  fetchCanvasMapTopology,
  fetchCanvasTopology,
  fetchDevices,
  fetchLinks,
} from '../../api/client';
import type { PositionState } from '../../hooks/usePositions';
import type { Area, Device, DevicePosition, Link } from '../../types/api';
import type { SnapshotPayload } from '../../types/metrics';

export type CanvasTopologySource =
  | {
      status: 'ok';
      devices: Device[];
      links: Link[];
      areas: Area[];
      positions: Map<string, PositionState>;
      etag?: string;
      topologyVersion?: string;
      runtimeVersion?: number;
      runtimeIdentity?: string;
      runtimeSnapshot?: SnapshotPayload;
      schemaVersion?: number;
    }
  | {
      status: 'not-modified';
      etag?: string;
    };

interface LoadCanvasTopologySourceOptions {
  mapId: string | null;
  fetchPositions: () => Promise<Map<string, PositionState>>;
  etag: string | null;
  includeRuntimeBootstrap?: boolean;
  forceRuntimeBootstrap?: boolean;
}

export function canvasMapKey(mapId: string | null): string {
  return mapId === null ? 'default:' : `map:${mapId}`;
}

export function topologyPositionsToPositionMap(
  positions: Iterable<DevicePosition>,
): Map<string, PositionState> {
  return new Map(
    Array.from(positions).map((position) => [
      position.device_id,
      {
        x: position.x,
        y: position.y,
        pinned: position.pinned,
      },
    ]),
  );
}

export function isCanvasTopologyUnsupported(error: unknown): boolean {
  if (typeof error !== 'object' || error === null || !('status' in error)) {
    return false;
  }

  const status = (error as { status?: unknown }).status;
  return status === 404 || status === 405 || status === 501;
}

function etagFromTopologyVersion(topologyVersion: string | undefined): string | undefined {
  const version = topologyVersion?.trim();
  return version ? JSON.stringify(version) : undefined;
}

export async function loadCanvasTopologySource({
  mapId,
  fetchPositions,
  etag,
  includeRuntimeBootstrap = false,
  forceRuntimeBootstrap = false,
}: LoadCanvasTopologySourceOptions): Promise<CanvasTopologySource> {
  try {
    if (includeRuntimeBootstrap) {
      const result =
        mapId === null
          ? await fetchCanvasBootstrap({ force: forceRuntimeBootstrap })
          : await fetchCanvasMapBootstrap(mapId, { force: forceRuntimeBootstrap });
      const topology = result.topology;
      return {
        status: 'ok',
        devices: topology.devices,
        links: topology.links,
        areas: topology.areas,
        positions: topologyPositionsToPositionMap(Object.values(topology.positions)),
        etag: etagFromTopologyVersion(topology.topology_version),
        topologyVersion: topology.topology_version,
        runtimeVersion: topology.runtime_version,
        runtimeIdentity: topology.runtime_identity,
        runtimeSnapshot: topology.runtime_snapshot,
        schemaVersion: topology.schema_version,
      };
    }

    const result =
      mapId === null
        ? await fetchCanvasTopology(etag ?? undefined)
        : await fetchCanvasMapTopology(mapId, etag ?? undefined);
    if (result.status === 'not-modified') {
      return {
        status: 'not-modified',
        etag: result.etag,
      };
    }

    const topology = result.topology;
    return {
      status: 'ok',
      devices: topology.devices,
      links: topology.links,
      areas: topology.areas,
      positions: topologyPositionsToPositionMap(Object.values(topology.positions)),
      etag: result.etag,
      topologyVersion: topology.topology_version,
      runtimeVersion: topology.runtime_version,
      runtimeIdentity: topology.runtime_identity,
      runtimeSnapshot: topology.runtime_snapshot,
      schemaVersion: topology.schema_version,
    };
  } catch (error) {
    if (includeRuntimeBootstrap && isCanvasTopologyUnsupported(error)) {
      return loadCanvasTopologySource({
        mapId,
        fetchPositions,
        etag,
      });
    }
    if (!isCanvasTopologyUnsupported(error)) {
      throw error;
    }
    if (mapId !== null) {
      throw error;
    }
  }

  const [devices, links, positions] = await Promise.all([
    fetchDevices(),
    fetchLinks(),
    fetchPositions(),
  ]);

  return {
    status: 'ok',
    devices,
    links,
    areas: [],
    positions,
  };
}
