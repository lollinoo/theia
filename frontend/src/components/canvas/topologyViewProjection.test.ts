/**
 * Exercises topology view projection topology canvas behavior so refactors preserve the documented contract.
 */
import { describe, expect, it } from 'vitest';

import type { Device, Link } from '../../types/api';
import { projectTopologyView } from './topologyViewProjection';

const devices = [
  { id: 'local', area_ids: ['area-a'], tags: { role: 'core' } },
  { id: 'remote', area_ids: ['area-b'], tags: { role: 'edge' } },
] as Device[];

const links = [{ id: 'link-1', source_device_id: 'local', target_device_id: 'remote' }] as Link[];

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

  it('ignores unknown explicit device IDs', () => {
    const result = projectTopologyView({
      devices,
      links: [
        { id: 'missing-link', source_device_id: 'missing', target_device_id: 'local' },
      ] as Link[],
      filter: {
        deviceIds: ['missing'],
        includeCrossAreaLinks: true,
        includeGhostDevices: true,
      },
    });

    expect(result.filteredDevices).toEqual([]);
    expect(result.filteredLinks).toEqual([]);
    expect(result.ghostDevices).toEqual([]);
  });

  it('excludes cross-area links with unknown endpoints without creating ghosts', () => {
    const result = projectTopologyView({
      devices,
      links: [
        { id: 'missing-link', source_device_id: 'local', target_device_id: 'missing' },
      ] as Link[],
      filter: {
        areaId: 'area-a',
        includeCrossAreaLinks: true,
        includeGhostDevices: true,
      },
    });

    expect(result.filteredDevices.map((device) => device.id)).toEqual(['local']);
    expect(result.filteredLinks).toEqual([]);
    expect(result.ghostDevices).toEqual([]);
  });

  it('requires both endpoints to be base devices when cross links are disabled', () => {
    const result = projectTopologyView({
      devices,
      links,
      filter: {
        areaId: 'area-a',
        includeCrossAreaLinks: false,
        includeGhostDevices: true,
      },
    });

    expect(result.filteredDevices.map((device) => device.id)).toEqual(['local']);
    expect(result.filteredLinks).toEqual([]);
    expect(result.ghostDevices).toEqual([]);
  });

  it('keeps duplicate cross links while deduping ghost devices', () => {
    const result = projectTopologyView({
      devices,
      links: [
        { id: 'link-1', source_device_id: 'local', target_device_id: 'remote' },
        { id: 'link-2', source_device_id: 'local', target_device_id: 'remote' },
      ] as Link[],
      filter: {
        areaId: 'area-a',
        includeCrossAreaLinks: true,
        includeGhostDevices: true,
      },
    });

    expect(result.filteredLinks.map((link) => link.id)).toEqual(['link-1', 'link-2']);
    expect(result.ghostDevices.map((device) => device.id)).toEqual(['remote']);
  });

  it('preserves device input order for ghosts', () => {
    const orderedDevices = [
      { id: 'local', area_ids: ['area-a'] },
      { id: 'remote-2', area_ids: ['area-b'] },
      { id: 'remote-1', area_ids: ['area-b'] },
    ] as Device[];
    const result = projectTopologyView({
      devices: orderedDevices,
      links: [
        { id: 'link-1', source_device_id: 'local', target_device_id: 'remote-1' },
        { id: 'link-2', source_device_id: 'local', target_device_id: 'remote-2' },
      ] as Link[],
      filter: {
        areaId: 'area-a',
        includeCrossAreaLinks: true,
        includeGhostDevices: true,
      },
    });

    expect(result.ghostDevices.map((device) => device.id)).toEqual(['remote-2', 'remote-1']);
  });

  it('matches empty-string tags only when the key is present', () => {
    const taggedDevices = [
      { id: 'empty', tags: { owner: '' }, area_ids: [] },
      { id: 'other', tags: { owner: 'ops' }, area_ids: [] },
      { id: 'missing', tags: {}, area_ids: [] },
      { id: 'nil', tags: null, area_ids: [] },
      { id: 'undefined', area_ids: [] },
    ] as unknown as Device[];

    const result = projectTopologyView({
      devices: taggedDevices,
      links: [],
      filter: {
        tagFilter: { owner: '' },
      },
    });

    expect(result.filteredDevices.map((device) => device.id)).toEqual(['empty']);
  });
});
