import { describe, expect, it } from 'vitest';

import { buildImportedNodePlacementRequest } from './importedNodePlacementRequest';

describe('buildImportedNodePlacementRequest', () => {
  it('deduplicates and sorts imported device IDs into a stable request', () => {
    const reverse = buildImportedNodePlacementRequest({
      fileDigest: 'sha256:import',
      mapId: 'map-1',
      deviceIds: ['device-b', 'device-a', 'device-b'],
    });
    const forward = buildImportedNodePlacementRequest({
      fileDigest: 'sha256:import',
      mapId: 'map-1',
      deviceIds: ['device-a', 'device-b'],
    });

    expect(reverse).toEqual(forward);
    expect(reverse).toMatchObject({
      mapId: 'map-1',
      deviceIds: ['device-a', 'device-b'],
    });
  });

  it('changes request identity when the imported result changes', () => {
    const first = buildImportedNodePlacementRequest({
      fileDigest: 'sha256:import',
      mapId: 'map-1',
      deviceIds: ['device-a'],
    });
    const second = buildImportedNodePlacementRequest({
      fileDigest: 'sha256:import',
      mapId: 'map-1',
      deviceIds: ['device-b'],
    });

    expect(first?.requestId).not.toBe(second?.requestId);
  });

  it('carries durable topology-run layout ownership into the canvas request', () => {
    const request = buildImportedNodePlacementRequest({
      fileDigest: 'sha256:import',
      mapId: 'map-1',
      deviceIds: ['device-a'],
      topologyRunId: 'run-1',
      topologyLayoutScope: 'reorganize',
    });

    expect(request).toMatchObject({
      topologyRunId: 'run-1',
      topologyLayoutScope: 'reorganize',
    });
    expect(request?.requestId).toContain('run-1');
  });

  it('returns null when no usable created device IDs exist', () => {
    expect(
      buildImportedNodePlacementRequest({
        fileDigest: 'sha256:import',
        mapId: 'map-1',
        deviceIds: ['', '   '],
      }),
    ).toBeNull();
  });
});
