/**
 * Exercises edge badge anchors component behavior so refactors preserve the documented contract.
 */
import { afterEach, describe, expect, it, vi } from 'vitest';
import { computeLinkBadgeAnchor, resolveBadgePathLengths } from './edgeBadgeAnchors';

describe('edgeBadgeAnchors', () => {
  afterEach(() => {
    vi.restoreAllMocks();
  });

  it('distributes multiple badge anchors symmetrically along the edge length', () => {
    expect(resolveBadgePathLengths(300, 3)).toEqual([58, 150, 242]);
    expect(resolveBadgePathLengths(240, 2)).toEqual([74, 166]);
  });

  it('samples the badge-group anchor from the real path geometry instead of a shared midpoint', () => {
    const createElementNSSpy = vi.spyOn(document, 'createElementNS');
    createElementNSSpy.mockImplementation((_ns, tagName) => {
      if (tagName !== 'path') {
        return document.createElement(tagName);
      }

      return {
        setAttribute: vi.fn(),
        getTotalLength: () => 240,
        getPointAtLength: (length: number) => ({ x: length, y: 120 }),
      } as unknown as SVGPathElement;
    });

    const anchor = computeLinkBadgeAnchor({
      path: 'M0 120 C80 120 160 120 240 120',
      fallbackX: 999,
      fallbackY: 999,
    });

    expect(Math.round(anchor.x)).toBe(120);
    expect(Math.round(anchor.y)).toBe(120);
  });

  it('keeps parallel-edge badge groups attached to the path while moving to a deterministic slot', () => {
    const createElementNSSpy = vi.spyOn(document, 'createElementNS');
    createElementNSSpy.mockImplementation((_ns, tagName) => {
      if (tagName !== 'path') {
        return document.createElement(tagName);
      }

      return {
        setAttribute: vi.fn(),
        getTotalLength: () => 240,
        getPointAtLength: (length: number) => ({ x: length, y: 120 }),
      } as unknown as SVGPathElement;
    });

    const base = computeLinkBadgeAnchor({
      path: 'M0 120 C80 120 160 120 240 120',
      fallbackX: 0,
      fallbackY: 0,
      parallelIndex: 0,
    });

    const parallel = computeLinkBadgeAnchor({
      path: 'M0 120 C80 120 160 120 240 120',
      fallbackX: 0,
      fallbackY: 0,
      parallelIndex: 1,
    });

    expect(Math.round(base.y)).toBe(120);
    expect(Math.round(parallel.y)).toBe(120);
    expect(base.x).not.toBe(parallel.x);
  });
});
