import { describe, expect, it } from 'vitest';
import { buildSelfLoopPathModel } from './linkEdgeGeometry';

describe('buildSelfLoopPathModel', () => {
  it('places self-loop apex and fallback label above the device card', () => {
    const result = buildSelfLoopPathModel({
      sourceX: 236,
      sourceY: 120,
      targetX: 76,
      targetY: 120,
    });

    expect(result.edgePath).toMatch(/^M 236,120 C /);
    expect(result.edgePath).toContain(' 76,120');
    expect(result.labelX).toBe(156);
    expect(result.labelY).toBeLessThan(120);
  });

  it('expands successive self-loop lanes away from the node', () => {
    const base = buildSelfLoopPathModel({
      sourceX: 236,
      sourceY: 120,
      targetX: 76,
      targetY: 120,
      parallelIndex: 0,
    });
    const secondLane = buildSelfLoopPathModel({
      sourceX: 236,
      sourceY: 120,
      targetX: 76,
      targetY: 120,
      parallelIndex: 1,
    });

    expect(secondLane.labelY).toBeLessThan(base.labelY);
    expect(secondLane.edgePath).not.toBe(base.edgePath);
  });
});
