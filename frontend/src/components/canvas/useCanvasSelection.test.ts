/**
 * Exercises use canvas selection topology canvas behavior so refactors preserve the documented contract.
 */
import { renderHook } from '@testing-library/react';
import type { ReactFlowInstance } from '@xyflow/react';
import { describe, expect, it, vi } from 'vitest';

import type { DeviceNode } from '../DeviceCard';
import type { LinkEdgeType } from '../LinkEdge';
import { resolveSelectedRealNodeIds, useCanvasSelection } from './useCanvasSelection';

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

describe('useCanvasSelection', () => {
  it('preserves the selectedRealNodeIds Set reference when non-selection node properties change', () => {
    const nodeA: DeviceNode = {
      id: 'node-a',
      type: 'device',
      position: { x: 0, y: 0 },
      selected: true,
      data: { isGhost: false } as DeviceNode['data'],
    };
    const nodeB: DeviceNode = {
      id: 'node-b',
      type: 'device',
      position: { x: 100, y: 100 },
      selected: false,
      data: { isGhost: false } as DeviceNode['data'],
    };
    const nodes = [nodeA, nodeB];
    const reactFlow = {
      getNodes: () => nodes,
    } as unknown as ReactFlowInstance<DeviceNode, LinkEdgeType>;
    const setPanelContent = vi.fn();

    const { result, rerender } = renderHook(
      ({ currentNodes }) =>
        useCanvasSelection({
          nodes: currentNodes,
          editMode: false,
          reactFlow,
          setPanelContent,
        }),
      {
        initialProps: { currentNodes: nodes },
      },
    );

    const initialSet = result.current.selectedRealNodeIds;
    expect(Array.from(initialSet)).toEqual(['node-a']);

    // Change node position (e.g. during dragging) without changing selection
    const movedNodes: DeviceNode[] = [
      { ...nodeA, position: { x: 10, y: 25 } },
      { ...nodeB, position: { x: 110, y: 125 } },
    ];
    rerender({ currentNodes: movedNodes });

    expect(result.current.selectedRealNodeIds).toBe(initialSet);

    // Change selection to node-b
    const newSelectionNodes: DeviceNode[] = [
      { ...nodeA, selected: false },
      { ...nodeB, selected: true },
    ];
    rerender({ currentNodes: newSelectionNodes });

    expect(result.current.selectedRealNodeIds).not.toBe(initialSet);
    expect(Array.from(result.current.selectedRealNodeIds)).toEqual(['node-b']);
  });
});
