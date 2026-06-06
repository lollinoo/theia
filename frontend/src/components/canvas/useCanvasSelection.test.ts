/**
 * Exercises use canvas selection topology canvas behavior so refactors preserve the documented contract.
 */
import { describe, expect, it } from 'vitest';

import type { DeviceNode } from '../DeviceCard';
import { resolveSelectedRealNodeIds } from './useCanvasSelection';

describe('resolveSelectedRealNodeIds', () => {
  it('includes selected real nodes and excludes ghost nodes', () => {
    const nodes = [
      { id: 'real-selected', selected: true, data: { isGhost: false } },
      { id: 'real-unselected', selected: false, data: { isGhost: false } },
      { id: 'ghost-selected', selected: true, data: { kind: 'ghost-device', isGhost: true } },
    ] as DeviceNode[];

    expect(Array.from(resolveSelectedRealNodeIds(nodes))).toEqual(['real-selected']);
  });
});
