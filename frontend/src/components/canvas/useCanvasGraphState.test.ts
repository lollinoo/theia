import { act, renderHook } from '@testing-library/react';
import type { EdgeChange, NodeChange } from '@xyflow/react';
import { describe, expect, it } from 'vitest';

import type { DeviceNode } from '../DeviceCard';
import type { LinkEdgeType } from '../LinkEdge';
import { useCanvasGraphState } from './useCanvasGraphState';

function node(id: string): DeviceNode {
  return {
    id,
    type: 'device',
    position: { x: 0, y: 0 },
    data: {},
  } as DeviceNode;
}

function edge(id: string, source: string, target: string): LinkEdgeType {
  return {
    id,
    type: 'link',
    source,
    target,
    data: {},
  } as LinkEdgeType;
}

describe('useCanvasGraphState', () => {
  it('preserves array references when updaters return the same arrays', () => {
    const { result } = renderHook(() => useCanvasGraphState());
    const nextNodes = [node('node-a'), node('node-b')];
    const nextEdges = [edge('edge-a', 'node-a', 'node-b')];

    act(() => {
      result.current.setNodes(nextNodes);
      result.current.setEdges(nextEdges);
    });

    const previousNodes = result.current.nodes;
    const previousEdges = result.current.edges;
    const previousNodeIndex = result.current.nodeIndexByIdRef.current;
    const previousEdgeIndex = result.current.edgeIndexByIdRef.current;

    act(() => {
      result.current.setNodes((current) => current);
      result.current.setEdges((current) => current);
    });

    expect(result.current.nodes).toBe(previousNodes);
    expect(result.current.edges).toBe(previousEdges);
    expect(result.current.nodeIndexByIdRef.current).toBe(previousNodeIndex);
    expect(result.current.edgeIndexByIdRef.current).toBe(previousEdgeIndex);
  });

  it('updates node and edge indexes after array replacement', () => {
    const { result } = renderHook(() => useCanvasGraphState());

    act(() => {
      result.current.setNodes([node('node-a'), node('node-b')]);
      result.current.setEdges([
        edge('edge-a', 'node-a', 'node-b'),
        edge('edge-b', 'node-b', 'node-a'),
      ]);
    });

    expect(result.current.nodeIndexByIdRef.current).toEqual(
      new Map([
        ['node-a', 0],
        ['node-b', 1],
      ]),
    );
    expect(result.current.edgeIndexByIdRef.current).toEqual(
      new Map([
        ['edge-a', 0],
        ['edge-b', 1],
      ]),
    );
  });

  it('applies node changes while refreshing the node index', () => {
    const { result } = renderHook(() => useCanvasGraphState());

    act(() => {
      result.current.setNodes([node('node-a'), node('node-b')]);
    });

    const previousNodeIndex = result.current.nodeIndexByIdRef.current;
    const changes: NodeChange<DeviceNode>[] = [{ id: 'node-a', type: 'remove' }];

    act(() => {
      result.current.onNodesChange(changes);
    });

    expect(result.current.nodes.map((current) => current.id)).toEqual(['node-b']);
    expect(result.current.nodeIndexByIdRef.current).not.toBe(previousNodeIndex);
    expect(result.current.nodeIndexByIdRef.current).toEqual(new Map([['node-b', 0]]));
  });

  it('applies edge changes while refreshing the edge index', () => {
    const { result } = renderHook(() => useCanvasGraphState());

    act(() => {
      result.current.setEdges([
        edge('edge-a', 'node-a', 'node-b'),
        edge('edge-b', 'node-b', 'node-a'),
      ]);
    });

    const previousEdgeIndex = result.current.edgeIndexByIdRef.current;
    const changes: EdgeChange<LinkEdgeType>[] = [{ id: 'edge-a', type: 'remove' }];

    act(() => {
      result.current.onEdgesChange(changes);
    });

    expect(result.current.edges.map((current) => current.id)).toEqual(['edge-b']);
    expect(result.current.edgeIndexByIdRef.current).not.toBe(previousEdgeIndex);
    expect(result.current.edgeIndexByIdRef.current).toEqual(new Map([['edge-b', 0]]));
  });
});
