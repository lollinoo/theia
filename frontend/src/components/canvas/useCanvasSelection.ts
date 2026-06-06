import type { ReactFlowInstance } from '@xyflow/react';
import type { Dispatch, SetStateAction } from 'react';
import { useCallback, useMemo, useState } from 'react';

import type { DeviceNode } from '../DeviceCard';
import type { LinkEdgeType } from '../LinkEdge';
import { isGhostDeviceNode } from './canvasHelpers';
import type { CanvasPanelContent } from './useCanvasMenus';

export function resolveSelectedRealNodeIds(nodes: DeviceNode[]): Set<string> {
  return new Set(
    nodes.filter((node) => node.selected && !isGhostDeviceNode(node)).map((node) => node.id),
  );
}

interface UseCanvasSelectionParams {
  nodes: DeviceNode[];
  editMode: boolean;
  reactFlow: ReactFlowInstance<DeviceNode, LinkEdgeType>;
  setPanelContent: Dispatch<SetStateAction<CanvasPanelContent | null>>;
}

export function useCanvasSelection({
  nodes,
  editMode,
  reactFlow,
  setPanelContent,
}: UseCanvasSelectionParams): {
  selectedNodeCount: number;
  selectedRealNodeIds: Set<string>;
  setSelectedNodeCount: Dispatch<SetStateAction<number>>;
  handleSelectionChange: (selection: { nodes: DeviceNode[] }) => void;
  openBulkEditPanel: () => void;
} {
  const [selectedNodeCount, setSelectedNodeCount] = useState(0);
  const selectedRealNodeIds = useMemo(() => resolveSelectedRealNodeIds(nodes), [nodes]);

  const openBulkEditPanel = useCallback(() => {
    if (!editMode) return;
    const selectedNodes = reactFlow.getNodes().filter((node) => node.selected);
    if (selectedNodes.length > 1) {
      setPanelContent({
        type: 'bulkEdit',
        data: { deviceIds: selectedNodes.map((node) => node.id) },
      });
    }
  }, [editMode, reactFlow, setPanelContent]);

  const handleSelectionChange = useCallback(
    ({ nodes: selectedNodes }: { nodes: DeviceNode[] }) => {
      setSelectedNodeCount(selectedNodes.length);
      if (selectedNodes.length > 1 && editMode) {
        setPanelContent({
          type: 'bulkEdit',
          data: { deviceIds: selectedNodes.map((node) => node.id) },
        });
      }
    },
    [editMode, setPanelContent],
  );

  return {
    selectedNodeCount,
    selectedRealNodeIds,
    setSelectedNodeCount,
    handleSelectionChange,
    openBulkEditPanel,
  };
}
