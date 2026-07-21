/**
 * Coordinates canvas graph state state for the topology canvas.
 * Keeps canvas lifecycle, projected graph state, and cleanup behavior explicit for callers.
 */

import type { EdgeChange, NodeChange, SnapGrid } from '@xyflow/react';
import * as ReactFlow from '@xyflow/react';
import type React from 'react';
import { useCallback, useLayoutEffect, useMemo, useRef, useState } from 'react';

import type { DeviceNode } from '../DeviceCard';
import type { LinkEdgeType } from '../LinkEdge';
import { canvasSnapGrid, snapNodeChangesToGrid } from './canvasGrid';

/** Configures optional controlled-state snapping for the topology canvas. */
export interface UseCanvasGraphStateOptions {
  snapToGrid?: boolean;
  snapGrid?: SnapGrid;
}

/** Describes the canvas graph state contract used by the topology canvas. */
export interface CanvasGraphState {
  nodes: DeviceNode[];
  edges: LinkEdgeType[];
  setNodes: React.Dispatch<React.SetStateAction<DeviceNode[]>>;
  setEdges: React.Dispatch<React.SetStateAction<LinkEdgeType[]>>;
  onNodesChange: (changes: NodeChange<DeviceNode>[]) => void;
  onEdgesChange: (changes: EdgeChange<LinkEdgeType>[]) => void;
  nodeIndexByIdRef: React.MutableRefObject<Map<string, number>>;
  edgeIndexByIdRef: React.MutableRefObject<Map<string, number>>;
}

function buildIndexById(items: { id: string }[]): Map<string, number> {
  const indexById = new Map<string, number>();
  items.forEach((item, index) => {
    indexById.set(item.id, index);
  });
  return indexById;
}

/** Coordinates canvas graph state behavior for the topology canvas. */
export function useCanvasGraphState({
  snapToGrid = true,
  snapGrid = canvasSnapGrid,
}: UseCanvasGraphStateOptions = {}): CanvasGraphState {
  const [nodes, setNodes] = useState<DeviceNode[]>([]);
  const [edges, setEdges] = useState<LinkEdgeType[]>([]);

  const nodeIndexById = useMemo(() => buildIndexById(nodes), [nodes]);
  const edgeIndexById = useMemo(() => buildIndexById(edges), [edges]);
  const nodeIndexByIdRef = useRef<Map<string, number>>(new Map());
  const edgeIndexByIdRef = useRef<Map<string, number>>(new Map());

  useLayoutEffect(() => {
    nodeIndexByIdRef.current = nodeIndexById;
  }, [nodeIndexById]);

  useLayoutEffect(() => {
    edgeIndexByIdRef.current = edgeIndexById;
  }, [edgeIndexById]);

  const onNodesChange = useCallback(
    (changes: NodeChange<DeviceNode>[]) => {
      const normalizedChanges = snapToGrid ? snapNodeChangesToGrid(changes, snapGrid) : changes;
      setNodes((currentNodes) =>
        ReactFlow.applyNodeChanges<DeviceNode>(normalizedChanges, currentNodes),
      );
    },
    [setNodes, snapGrid, snapToGrid],
  );

  const onEdgesChange = useCallback(
    (changes: EdgeChange<LinkEdgeType>[]) => {
      setEdges((currentEdges) => ReactFlow.applyEdgeChanges<LinkEdgeType>(changes, currentEdges));
    },
    [setEdges],
  );

  return {
    nodes,
    edges,
    setNodes,
    setEdges,
    onNodesChange,
    onEdgesChange,
    nodeIndexByIdRef,
    edgeIndexByIdRef,
  };
}
