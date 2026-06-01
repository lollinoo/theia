import { beforeEach, describe, expect, it, vi } from 'vitest';

import {
  fetchCanvasBootstrap,
  fetchCanvasMapBootstrap,
  fetchCanvasMapTopology,
  fetchCanvasTopology,
  fetchDevices,
  fetchLinks,
} from '../../api/client';
import type { PositionState } from '../../hooks/usePositions';
import type { CanvasTopologyResponse, Device, DevicePosition, Link } from '../../types/api';
import type { SnapshotPayload } from '../../types/metrics';
import {
  canvasMapKey,
  isCanvasTopologyUnsupported,
  loadCanvasTopologySource,
  topologyPositionsToPositionMap,
} from './canvasTopologySource';

vi.mock('../../api/client', () => ({
  fetchCanvasBootstrap: vi.fn(),
  fetchCanvasMapBootstrap: vi.fn(),
  fetchCanvasMapTopology: vi.fn(),
  fetchCanvasTopology: vi.fn(),
  fetchDevices: vi.fn(),
  fetchLinks: vi.fn(),
}));

const testDevice = { id: 'dev-1' } as Device;
const testLink = { id: 'link-1' } as Link;

function canvasTopologyResponse(
  overrides: Partial<CanvasTopologyResponse> = {},
): CanvasTopologyResponse {
  return {
    schema_version: 1,
    topology_version: 'topo-1',
    generated_at: '2026-04-30T12:00:00Z',
    devices: [testDevice],
    links: [testLink],
    positions: {},
    areas: [],
    capabilities: {
      supports_topology_delta: false,
      supports_position_revision: false,
      supports_area_filtering: true,
    },
    settings: { layout: { version: 1 } },
    ...overrides,
  };
}

describe('canvasTopologySource', () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it('scopes default and saved map keys distinctly', () => {
    expect(canvasMapKey(null)).toBe('default:');
    expect(canvasMapKey('map-1')).toBe('map:map-1');
    expect(canvasMapKey('__default__')).toBe('map:__default__');
  });

  it('converts topology positions to saved position state', () => {
    const positions: DevicePosition[] = [
      {
        device_id: 'dev-1',
        x: 12.5,
        y: 48.25,
        pinned: true,
        updated_at: '2026-04-30T12:00:00Z',
      },
    ];

    expect(topologyPositionsToPositionMap(positions)).toEqual(
      new Map<string, PositionState>([['dev-1', { x: 12.5, y: 48.25, pinned: true }]]),
    );
  });

  it('classifies unsupported topology endpoints by status', () => {
    expect(isCanvasTopologyUnsupported({ status: 404 })).toBe(true);
    expect(isCanvasTopologyUnsupported({ status: 405 })).toBe(true);
    expect(isCanvasTopologyUnsupported({ status: 501 })).toBe(true);
    expect(isCanvasTopologyUnsupported({ status: 500 })).toBe(false);
    expect(isCanvasTopologyUnsupported(new Error('network failed'))).toBe(false);
    expect(isCanvasTopologyUnsupported(null)).toBe(false);
  });

  it('loads default bootstrap topology with runtime metadata', async () => {
    const runtimeSnapshot = { devices: {}, links: {} } as SnapshotPayload;
    vi.mocked(fetchCanvasBootstrap).mockResolvedValueOnce({
      topology: canvasTopologyResponse({
        positions: {
          'dev-1': { device_id: 'dev-1', x: 10, y: 20, pinned: true },
        },
        runtime_version: 7,
        runtime_identity: 'rt-sha256:abc',
        runtime_snapshot: runtimeSnapshot,
      }),
    });
    const fetchPositions = vi.fn(async () => new Map<string, PositionState>());

    const result = await loadCanvasTopologySource({
      mapId: null,
      fetchPositions,
      etag: '"stale"',
      includeRuntimeBootstrap: true,
      forceRuntimeBootstrap: true,
    });

    expect(fetchCanvasBootstrap).toHaveBeenCalledWith({ force: true });
    expect(fetchCanvasMapBootstrap).not.toHaveBeenCalled();
    expect(fetchCanvasTopology).not.toHaveBeenCalled();
    expect(fetchPositions).not.toHaveBeenCalled();
    expect(result).toMatchObject({
      status: 'ok',
      devices: [testDevice],
      links: [testLink],
      areas: [],
      topologyVersion: 'topo-1',
      runtimeVersion: 7,
      runtimeIdentity: 'rt-sha256:abc',
      runtimeSnapshot,
      schemaVersion: 1,
    });
    expect(result.status === 'ok' ? result.positions : null).toEqual(
      new Map([['dev-1', { x: 10, y: 20, pinned: true }]]),
    );
  });

  it('returns saved map not-modified responses with the response ETag', async () => {
    vi.mocked(fetchCanvasMapTopology).mockResolvedValueOnce({
      status: 'not-modified',
      etag: '"map-etag"',
    });
    const fetchPositions = vi.fn(async () => new Map<string, PositionState>());

    await expect(
      loadCanvasTopologySource({
        mapId: 'map-1',
        fetchPositions,
        etag: '"map-etag"',
      }),
    ).resolves.toEqual({
      status: 'not-modified',
      etag: '"map-etag"',
    });
    expect(fetchCanvasMapTopology).toHaveBeenCalledWith('map-1', '"map-etag"');
    expect(fetchCanvasTopology).not.toHaveBeenCalled();
  });

  it('falls back to legacy default topology when the default read model is unsupported', async () => {
    const savedPositions = new Map<string, PositionState>([
      ['dev-1', { x: 90, y: 120, pinned: false }],
    ]);
    vi.mocked(fetchCanvasTopology).mockRejectedValueOnce({ status: 404 });
    vi.mocked(fetchDevices).mockResolvedValueOnce([testDevice]);
    vi.mocked(fetchLinks).mockResolvedValueOnce([testLink]);
    const fetchPositions = vi.fn(async () => savedPositions);

    const result = await loadCanvasTopologySource({
      mapId: null,
      fetchPositions,
      etag: '"stale"',
    });

    expect(fetchCanvasTopology).toHaveBeenCalledWith('"stale"');
    expect(fetchDevices).toHaveBeenCalledTimes(1);
    expect(fetchLinks).toHaveBeenCalledTimes(1);
    expect(fetchPositions).toHaveBeenCalledTimes(1);
    expect(result).toMatchObject({
      status: 'ok',
      devices: [testDevice],
      links: [testLink],
      areas: [],
    });
    expect(result.status === 'ok' ? result.positions : null).toBe(savedPositions);
  });

  it('does not fall back to legacy topology for unsupported saved map endpoints', async () => {
    const unsupportedError = { status: 404 };
    vi.mocked(fetchCanvasMapTopology).mockRejectedValueOnce(unsupportedError);
    const fetchPositions = vi.fn(async () => new Map<string, PositionState>());

    await expect(
      loadCanvasTopologySource({
        mapId: 'map-1',
        fetchPositions,
        etag: null,
      }),
    ).rejects.toBe(unsupportedError);
    expect(fetchDevices).not.toHaveBeenCalled();
    expect(fetchLinks).not.toHaveBeenCalled();
    expect(fetchPositions).not.toHaveBeenCalled();
  });
});
