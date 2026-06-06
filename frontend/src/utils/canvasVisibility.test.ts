/**
 * Exercises canvas visibility utility behavior so refactors preserve the documented contract.
 */
import { describe, expect, it } from 'vitest';

import { isNodeVisibleInViewport } from './canvasVisibility';

describe('canvasVisibility helpers', () => {
  it('treats nodes inside the viewport as visible', () => {
    expect(
      isNodeVisibleInViewport({
        nodeX: 100,
        nodeY: 120,
        nodeWidth: 260,
        nodeHeight: 160,
        viewportWidth: 1200,
        viewportHeight: 800,
        transform: [0, 0, 1],
      }),
    ).toBe(true);
  });

  it('treats nearby offscreen nodes within the margin as visible', () => {
    expect(
      isNodeVisibleInViewport({
        nodeX: -200,
        nodeY: 120,
        nodeWidth: 260,
        nodeHeight: 160,
        viewportWidth: 1200,
        viewportHeight: 800,
        transform: [0, 0, 1],
      }),
    ).toBe(true);
  });

  it('treats distant offscreen nodes as not visible', () => {
    expect(
      isNodeVisibleInViewport({
        nodeX: -500,
        nodeY: 120,
        nodeWidth: 260,
        nodeHeight: 160,
        viewportWidth: 1200,
        viewportHeight: 800,
        transform: [0, 0, 1],
      }),
    ).toBe(false);
  });

  it('accounts for viewport translation and zoom', () => {
    expect(
      isNodeVisibleInViewport({
        nodeX: 400,
        nodeY: 300,
        nodeWidth: 260,
        nodeHeight: 160,
        viewportWidth: 1200,
        viewportHeight: 800,
        transform: [-350, -250, 0.8],
      }),
    ).toBe(true);
  });
});
