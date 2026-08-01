import { describe, expect, it } from 'vitest';

import { scoreTopologyLayoutCandidate } from './topologyImportLayoutGeometry';

describe('scoreTopologyLayoutCandidate', () => {
  it('counts a crossing introduced by links with distinct endpoints', () => {
    const positions = new Map([
      ['a', { x: 0, y: 0 }],
      ['b', { x: 466, y: 256 }],
      ['c', { x: 0, y: 256 }],
      ['d', { x: 466, y: 0 }],
    ]);
    const score = scoreTopologyLayoutCandidate('b', positions.get('b')!, positions, [
      { source: 'a', target: 'b' },
      { source: 'c', target: 'd' },
    ]);

    expect(score.nodeLinkIntersections).toBe(0);
    expect(score.linkCrossings).toBe(1);
  });

  it('counts a routed crossing away from a shared endpoint', () => {
    const positions = new Map([
      ['moving', { x: 0, y: 0 }],
      ['shared', { x: 932, y: 0 }],
      ['fixed', { x: 932, y: 512 }],
    ]);
    const score = scoreTopologyLayoutCandidate('moving', positions.get('moving')!, positions, [
      { source: 'moving', target: 'shared' },
      {
        source: 'shared',
        target: 'fixed',
        waypoints: [
          { x: 1147, y: 326 },
          { x: 500, y: 326 },
          { x: 500, y: -100 },
        ],
      },
    ]);

    expect(score.linkCrossings).toBe(1);
  });

  it('ignores non-finite waypoints instead of losing the direct link corridor', () => {
    const positions = new Map([
      ['left', { x: 0, y: 0 }],
      ['right', { x: 932, y: 0 }],
      ['imported', { x: 466, y: 0 }],
    ]);
    const score = scoreTopologyLayoutCandidate('imported', positions.get('imported')!, positions, [
      {
        source: 'left',
        target: 'right',
        waypoints: [{ x: Number.NaN, y: 70 }],
      },
    ]);

    expect(score.nodeLinkIntersections).toBe(1);
  });
});
