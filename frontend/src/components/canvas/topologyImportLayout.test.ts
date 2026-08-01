import { describe, expect, it } from 'vitest';

import { computeTopologyImportLayout } from './topologyImportLayout';

const existingPositions = new Map([
  ['core', { x: 0, y: 0, pinned: true }],
  ['edge-a', { x: 500, y: 0, pinned: false }],
]);

const CARD_WIDTH = 430;
const CARD_HEIGHT = 140;

function segmentIntersectsCard(
  source: { x: number; y: number },
  target: { x: number; y: number },
  card: { x: number; y: number },
): boolean {
  const minimum = { x: card.x, y: card.y };
  const maximum = { x: card.x + CARD_WIDTH, y: card.y + CARD_HEIGHT };
  const delta = { x: target.x - source.x, y: target.y - source.y };
  let lower = 0;
  let upper = 1;

  for (const axis of ['x', 'y'] as const) {
    if (Math.abs(delta[axis]) < Number.EPSILON) {
      if (source[axis] <= minimum[axis] || source[axis] >= maximum[axis]) return false;
      continue;
    }
    const first = (minimum[axis] - source[axis]) / delta[axis];
    const second = (maximum[axis] - source[axis]) / delta[axis];
    lower = Math.max(lower, Math.min(first, second));
    upper = Math.min(upper, Math.max(first, second));
    if (lower >= upper) return false;
  }

  return upper > 0 && lower < 1;
}

function segmentsCross(
  leftSource: { x: number; y: number },
  leftTarget: { x: number; y: number },
  rightSource: { x: number; y: number },
  rightTarget: { x: number; y: number },
): boolean {
  const orientation = (
    first: { x: number; y: number },
    second: { x: number; y: number },
    third: { x: number; y: number },
  ) => (second.x - first.x) * (third.y - first.y) - (second.y - first.y) * (third.x - first.x);
  const leftToSource = orientation(leftSource, leftTarget, rightSource);
  const leftToTarget = orientation(leftSource, leftTarget, rightTarget);
  const rightToSource = orientation(rightSource, rightTarget, leftSource);
  const rightToTarget = orientation(rightSource, rightTarget, leftTarget);

  return leftToSource * leftToTarget < 0 && rightToSource * rightToTarget < 0;
}

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

  it('keeps automatic links out of unrelated cards in a connected topology', () => {
    const deviceIds = ['a', 'b', 'c', 'd', 'e'];
    const links = [
      { id: 'link-ab', source: 'a', target: 'b' },
      { id: 'link-ac', source: 'a', target: 'c' },
      { id: 'link-ad', source: 'a', target: 'd' },
      { id: 'link-ae', source: 'a', target: 'e' },
    ];
    const result = computeTopologyImportLayout({
      deviceIds,
      links,
      positions: new Map(),
      importedDeviceIds: new Set(deviceIds),
      attentionDeviceIds: new Set<string>(),
      scope: 'reorganize',
    });
    const positions = new Map(
      result.positions.map((position) => [position.device_id, position] as const),
    );
    const crossingLinks = links.filter((link) => {
      const source = positions.get(link.source)!;
      const target = positions.get(link.target)!;
      return deviceIds.some(
        (deviceId) =>
          deviceId !== link.source &&
          deviceId !== link.target &&
          segmentIntersectsCard(
            { x: source.x + CARD_WIDTH / 2, y: source.y + CARD_HEIGHT / 2 },
            { x: target.x + CARD_WIDTH / 2, y: target.y + CARD_HEIGHT / 2 },
            positions.get(deviceId)!,
          ),
      );
    });

    expect(crossingLinks).toEqual([]);
  });

  it('keeps an imported node out of a fixed manual route in preserve mode', () => {
    const source = { x: 0 + CARD_WIDTH / 2, y: 256 + CARD_HEIGHT / 2 };
    const target = { x: 932 + CARD_WIDTH / 2, y: 256 + CARD_HEIGHT / 2 };
    const waypoints = [
      { x: source.x, y: 582 },
      { x: 2000, y: 582 },
      { x: target.x, y: 582 },
    ];
    const links = [{ id: 'fixed-link', source: 'left', target: 'right', waypoints }];
    const result = computeTopologyImportLayout({
      deviceIds: ['left', 'right', 'imported'],
      links,
      positions: new Map([
        ['left', { x: 0, y: 256, pinned: true }],
        ['right', { x: 932, y: 256, pinned: true }],
        ['imported', { x: 466, y: 256, pinned: false }],
      ]),
      importedDeviceIds: new Set(['imported']),
      attentionDeviceIds: new Set<string>(),
      scope: 'preserve',
    });
    const imported = result.positions.find((position) => position.device_id === 'imported');
    const routePoints = [source, ...waypoints, target];

    expect(imported).toBeDefined();
    expect(
      routePoints
        .slice(1)
        .some((point, index) => segmentIntersectsCard(routePoints[index], point, imported!)),
    ).toBe(false);
  });

  it('removes an avoidable crossing while preserving fixed endpoints', () => {
    const fixedPositions = new Map([
      ['a', { x: 0, y: 0, pinned: true }],
      ['b', { x: 466, y: 256, pinned: false }],
      ['c', { x: 0, y: 256, pinned: true }],
      ['d', { x: 466, y: 0, pinned: true }],
    ]);
    const result = computeTopologyImportLayout({
      deviceIds: ['a', 'b', 'c', 'd'],
      links: [
        { id: 'moving-link', source: 'a', target: 'b' },
        { id: 'fixed-link', source: 'c', target: 'd' },
      ],
      positions: fixedPositions,
      importedDeviceIds: new Set(['b']),
      attentionDeviceIds: new Set<string>(),
      scope: 'preserve',
    });
    const positions = new Map(fixedPositions);
    for (const position of result.positions) positions.set(position.device_id, position);
    const center = (deviceId: string) => {
      const position = positions.get(deviceId)!;
      return { x: position.x + CARD_WIDTH / 2, y: position.y + CARD_HEIGHT / 2 };
    };

    expect(segmentsCross(center('a'), center('b'), center('c'), center('d'))).toBe(false);
    expect(positions.get('a')).toEqual(fixedPositions.get('a'));
    expect(positions.get('c')).toEqual(fixedPositions.get('c'));
    expect(positions.get('d')).toEqual(fixedPositions.get('d'));
  });

  it('avoids link crossings when a connected topology has a clear arrangement', () => {
    const deviceIds = ['a', 'b', 'c', 'd', 'e'];
    const links = [
      { id: 'link-ac', source: 'a', target: 'c' },
      { id: 'link-ae', source: 'a', target: 'e' },
      { id: 'link-bc', source: 'b', target: 'c' },
      { id: 'link-bd', source: 'b', target: 'd' },
      { id: 'link-be', source: 'b', target: 'e' },
    ];
    const result = computeTopologyImportLayout({
      deviceIds,
      links,
      positions: new Map(),
      importedDeviceIds: new Set(deviceIds),
      attentionDeviceIds: new Set<string>(),
      scope: 'reorganize',
    });
    const positions = new Map(
      result.positions.map((position) => [position.device_id, position] as const),
    );
    const center = (deviceId: string) => {
      const position = positions.get(deviceId)!;
      return { x: position.x + CARD_WIDTH / 2, y: position.y + CARD_HEIGHT / 2 };
    };
    const crossingPairs: string[] = [];

    for (let left = 0; left < links.length; left += 1) {
      for (let right = left + 1; right < links.length; right += 1) {
        const leftLink = links[left];
        const rightLink = links[right];
        if (
          [leftLink.source, leftLink.target].some(
            (endpoint) => endpoint === rightLink.source || endpoint === rightLink.target,
          )
        ) {
          continue;
        }
        if (
          segmentsCross(
            center(leftLink.source),
            center(leftLink.target),
            center(rightLink.source),
            center(rightLink.target),
          )
        ) {
          crossingPairs.push(`${leftLink.id}:${rightLink.id}`);
        }
      }
    }

    expect(crossingPairs).toEqual([]);
  });

  it('keeps links to attention nodes out of normal cards', () => {
    const deviceIds = ['a', 'b', 'c', 'd', 'e'];
    const links = [
      { id: 'link-ab', source: 'a', target: 'b' },
      { id: 'link-ac', source: 'a', target: 'c' },
      { id: 'link-ad', source: 'a', target: 'd' },
      { id: 'link-ae', source: 'a', target: 'e' },
    ];
    const result = computeTopologyImportLayout({
      deviceIds,
      links,
      positions: new Map(),
      importedDeviceIds: new Set(deviceIds),
      attentionDeviceIds: new Set(['e']),
      scope: 'reorganize',
    });
    const positions = new Map(
      result.positions.map((position) => [position.device_id, position] as const),
    );
    const source = positions.get('a')!;
    const target = positions.get('e')!;
    const crossedNormalNodes = ['b', 'c', 'd'].filter((deviceId) =>
      segmentIntersectsCard(
        { x: source.x + CARD_WIDTH / 2, y: source.y + CARD_HEIGHT / 2 },
        { x: target.x + CARD_WIDTH / 2, y: target.y + CARD_HEIGHT / 2 },
        positions.get(deviceId)!,
      ),
    );

    expect(crossedNormalNodes).toEqual([]);
    expect(positions.get('e')!.y).toBeGreaterThan(
      Math.max(...['a', 'b', 'c', 'd'].map((deviceId) => positions.get(deviceId)!.y)),
    );
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

  it('keeps refinement bounded for a large linked import', () => {
    const deviceIds = Array.from({ length: 48 }, (_, index) => `device-${index}`);
    const links = deviceIds.flatMap((source, index) =>
      [index + 1, index + 4]
        .filter((targetIndex) => targetIndex < deviceIds.length)
        .map((targetIndex) => ({
          id: `link-${index}-${targetIndex}`,
          source,
          target: deviceIds[targetIndex],
        })),
    );
    const result = computeTopologyImportLayout({
      deviceIds,
      links,
      positions: new Map(),
      importedDeviceIds: new Set(deviceIds),
      attentionDeviceIds: new Set<string>(),
      scope: 'reorganize',
    });

    expect(result.positions).toHaveLength(deviceIds.length);
    for (const position of result.positions) {
      expect(Number.isFinite(position.x)).toBe(true);
      expect(Number.isFinite(position.y)).toBe(true);
    }
    for (let left = 0; left < result.positions.length; left += 1) {
      for (let right = left + 1; right < result.positions.length; right += 1) {
        const a = result.positions[left];
        const b = result.positions[right];
        expect(Math.abs(a.x - b.x) >= 466 || Math.abs(a.y - b.y) >= 256).toBe(true);
      }
    }
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
