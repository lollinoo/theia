import { describe, expect, it } from 'vitest';

import type { Device, Link } from '../../types/api';
import { projectTopologyView } from './topologyViewProjection';

const devices = [
  { id: 'local', area_ids: ['area-a'], tags: { role: 'core' } },
  { id: 'remote', area_ids: ['area-b'], tags: { role: 'edge' } },
] as Device[];

const links = [
  { id: 'link-1', source_device_id: 'local', target_device_id: 'remote' },
] as Link[];

describe('projectTopologyView', () => {
  it('returns global topology without a filter', () => {
    const result = projectTopologyView({ devices, links, filter: {} });

    expect(result.filteredDevices.map((device) => device.id)).toEqual(['local', 'remote']);
    expect(result.filteredLinks.map((link) => link.id)).toEqual(['link-1']);
    expect(result.ghostDevices).toEqual([]);
  });

  it('projects area views with optional ghosts', () => {
    const result = projectTopologyView({
      devices,
      links,
      filter: {
        areaId: 'area-a',
        includeCrossAreaLinks: true,
        includeGhostDevices: true,
      },
    });

    expect(result.filteredDevices.map((device) => device.id)).toEqual(['local']);
    expect(result.filteredLinks.map((link) => link.id)).toEqual(['link-1']);
    expect(result.ghostDevices.map((device) => device.id)).toEqual(['remote']);
  });

  it('lets deviceIds take precedence over areaId and intersects tags', () => {
    const result = projectTopologyView({
      devices,
      links,
      filter: {
        areaId: 'area-a',
        deviceIds: ['remote'],
        tagFilter: { role: 'edge' },
      },
    });

    expect(result.filteredDevices.map((device) => device.id)).toEqual(['remote']);
  });
});
