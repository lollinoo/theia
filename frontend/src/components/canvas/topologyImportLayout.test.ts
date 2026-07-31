import { describe, expect, it } from 'vitest';

import { computeTopologyImportLayout } from './topologyImportLayout';

const existingPositions = new Map([
  ['core', { x: 0, y: 0, pinned: true }],
  ['edge-a', { x: 500, y: 0, pinned: false }],
]);

describe('computeTopologyImportLayout', () => {
  it('is deterministic across input order and collapses parallel links', () => {
    const input = {
      deviceIds: ['core', 'edge-b', 'edge-a'],
      links: [
        { id: 'link-2', source: 'core', target: 'edge-a' },
        { id: 'link-1', source: 'edge-a', target: 'core' },
        { id: 'link-3', source: 'core', target: 'edge-b' },
      ],
      positions: existingPositions,
      importedDeviceIds: new Set(['edge-a', 'edge-b']),
      attentionDeviceIds: new Set<string>(),
      scope: 'reorganize' as const,
    };

    const forward = computeTopologyImportLayout(input);
    const reverse = computeTopologyImportLayout({
      ...input,
      deviceIds: [...input.deviceIds].reverse(),
      links: [...input.links].reverse(),
    });

    expect(reverse).toEqual(forward);
    expect(forward.positions.map((position) => position.device_id)).toEqual([
      'core',
      'edge-a',
      'edge-b',
    ]);
  });

  it('preserves existing nodes and places imported nodes without overlap', () => {
    const result = computeTopologyImportLayout({
      deviceIds: ['core', 'edge-a', 'edge-b'],
      links: [
        { id: 'link-a', source: 'core', target: 'edge-a' },
        { id: 'link-b', source: 'core', target: 'edge-b' },
      ],
      positions: existingPositions,
      importedDeviceIds: new Set(['edge-a', 'edge-b']),
      attentionDeviceIds: new Set<string>(),
      scope: 'preserve',
    });

    expect(result.positions.map((position) => position.device_id)).toEqual(['edge-a', 'edge-b']);
    const rectangles = [
      { id: 'core', x: 0, y: 0 },
      ...result.positions.map((position) => ({
        id: position.device_id,
        x: position.x,
        y: position.y,
      })),
    ];
    for (let left = 0; left < rectangles.length; left += 1) {
      for (let right = left + 1; right < rectangles.length; right += 1) {
        const a = rectangles[left];
        const b = rectangles[right];
        expect(Math.abs(a.x - b.x) >= 466 || Math.abs(a.y - b.y) >= 256).toBe(true);
      }
    }
  });

  it('places failed or unresolved nodes in a deterministic attention band', () => {
    const result = computeTopologyImportLayout({
      deviceIds: ['core', 'edge-a', 'edge-b'],
      links: [{ id: 'link-a', source: 'core', target: 'edge-a' }],
      positions: new Map(),
      importedDeviceIds: new Set(['core', 'edge-a', 'edge-b']),
      attentionDeviceIds: new Set(['edge-b']),
      scope: 'reorganize',
    });
    const attention = result.positions.find((position) => position.device_id === 'edge-b');
    const normalY = result.positions
      .filter((position) => position.device_id !== 'edge-b')
      .map((position) => position.y);

    expect(attention).toBeDefined();
    expect(attention!.y).toBeGreaterThan(Math.max(...normalY));
    expect(result.attentionBandY).toBe(attention!.y);
  });

  it('resets waypoints only for links with an endpoint that actually moved', () => {
    const result = computeTopologyImportLayout({
      deviceIds: ['core', 'edge-a', 'outside'],
      links: [
        { id: 'moved-link', source: 'core', target: 'edge-a' },
        { id: 'unmoved-link', source: 'core', target: 'outside' },
      ],
      positions: new Map([
        ['core', { x: 0, y: 0, pinned: true }],
        ['edge-a', { x: 0, y: 0, pinned: false }],
        ['outside', { x: 1000, y: 0, pinned: true }],
      ]),
      importedDeviceIds: new Set(['edge-a']),
      attentionDeviceIds: new Set<string>(),
      scope: 'preserve',
    });

    expect(result.resetLinkRouteIds).toEqual(['moved-link']);
  });
});
