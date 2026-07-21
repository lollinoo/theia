/**
 * Defines canvas topology source behavior for the topology canvas.
 * Documents how canonical topology data is projected into the interactive view layer.
 */
import {
  fetchCanvasBootstrap,
  fetchCanvasMapBootstrap,
  fetchCanvasMapTopology,
  fetchCanvasTopology,
  fetchDevices,
  fetchLinks,
} from '../../api/client';
import type { PositionState } from '../../hooks/usePositions';
import type { Area, Device, DevicePosition, Link, LinkRouteMap } from '../../types/api';
import type { SnapshotPayload } from '../../types/metrics';

/** Describes the canvas topology source contract used by the topology canvas. */
export type CanvasTopologySource =
  | {
      status: 'ok';
      devices: Device[];
      links: Link[];
      linkRoutes: LinkRouteMap;
      areas: Area[];
      positions: Map<string, PositionState>;
      etag?: string;
      topologyVersion?: string;
      runtimeStreamId?: string;
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

// canvasMapKey returns the stable ownership key used to discard stale map loads.
export function canvasMapKey(mapId: string | null): string {
  return mapId === null ? 'default:' : `map:${mapId}`;
}

// topologyPositionsToPositionMap converts API position rows into hook position state.
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

// isCanvasTopologyUnsupported identifies backends that need the legacy topology fallback.
export function isCanvasTopologyUnsupported(error: unknown): boolean {
  if (typeof error !== 'object' || error === null || !('status' in error)) {
    return false;
  }

  const status = (error as { status?: unknown }).status;
  return status === 404 || status === 405 || status === 501;
}

// etagFromTopologyVersion converts topology versions to the quoted ETag format used by fetch.
function etagFromTopologyVersion(topologyVersion: string | undefined): string | undefined {
  const version = topologyVersion?.trim();
  return version ? JSON.stringify(version) : undefined;
}

// loadCanvasTopologySource loads default or saved-map topology with ETag and bootstrap handling.
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
        linkRoutes: topology.link_routes ?? {},
        areas: topology.areas,
        positions: topologyPositionsToPositionMap(Object.values(topology.positions)),
        etag: etagFromTopologyVersion(topology.topology_version),
        topologyVersion: topology.topology_version,
        runtimeStreamId: topology.runtime_stream_id,
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
      linkRoutes: topology.link_routes ?? {},
      areas: topology.areas,
      positions: topologyPositionsToPositionMap(Object.values(topology.positions)),
      etag: result.etag,
      topologyVersion: topology.topology_version,
      runtimeStreamId: topology.runtime_stream_id,
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
    linkRoutes: {},
    areas: [],
    positions,
  };
}
