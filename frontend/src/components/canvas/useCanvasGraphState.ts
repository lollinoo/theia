import * as ReactFlow from '@xyflow/react';
import type { EdgeChange, NodeChange } from '@xyflow/react';
import { useCallback, useMemo, useRef, useState } from 'react';
import type React from 'react';

import type { DeviceNode } from '../DeviceCard';
import type { LinkEdgeType } from '../LinkEdge';

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

export function useCanvasGraphState(): CanvasGraphState {
  const [nodes, setNodes] = ReactFlow.useNodesState<DeviceNode>([]);
  const [edges, setEdges] = useState<LinkEdgeType[]>([]);

  const nodeIndexById = useMemo(() => buildIndexById(nodes), [nodes]);
  const edgeIndexById = useMemo(() => buildIndexById(edges), [edges]);
  const nodeIndexByIdRef = useRef(nodeIndexById);
  const edgeIndexByIdRef = useRef(edgeIndexById);

  nodeIndexByIdRef.current = nodeIndexById;
  edgeIndexByIdRef.current = edgeIndexById;

  const onNodesChange = useCallback(
    (changes: NodeChange<DeviceNode>[]) => {
      setNodes((currentNodes) => ReactFlow.applyNodeChanges<DeviceNode>(changes, currentNodes));
    },
    [setNodes],
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
