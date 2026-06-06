/**
 * Defines topology position state behavior for the topology canvas.
 * Documents how canonical topology data is projected into the interactive view layer.
 */
import type { PositionState } from '../../hooks/usePositions';
import type { Device } from '../../types/api';
import type { DeviceNode } from '../DeviceCard';
import { buildPositionPayload } from './canvasHelpers';
import type { CanvasMeasurementTrigger } from './canvasInstrumentation';

interface TopologyCompositionPositionPlanInput {
  trigger: CanvasMeasurementTrigger;
  savedPositions: Map<string, PositionState>;
  currentNodePositions: Map<string, PositionState>;
}

interface TopologyCompositionPositionPlan {
  effectivePositions: Map<string, PositionState>;
  currentPositionsForComposition: Map<string, PositionState>;
}

interface TopologyPositionSavePlan {
  shouldSave: boolean;
  payload: ReturnType<typeof buildPositionPayload>;
}

// hasUsablePosition accepts only finite positions for composition and save decisions.
function hasUsablePosition(position: PositionState | undefined): position is PositionState {
  return position !== undefined && Number.isFinite(position.x) && Number.isFinite(position.y);
}

// buildTopologyCompositionPositionPlan merges saved and current positions for one topology compose pass.
export function buildTopologyCompositionPositionPlan({
  trigger,
  savedPositions,
  currentNodePositions,
}: TopologyCompositionPositionPlanInput): TopologyCompositionPositionPlan {
  const effectivePositions = new Map(savedPositions);
  for (const [deviceId, position] of currentNodePositions.entries()) {
    if (!effectivePositions.has(deviceId)) {
      effectivePositions.set(deviceId, position);
    }
  }

  return {
    effectivePositions,
    currentPositionsForComposition:
      trigger === 'backend_reconnected' ? new Map<string, PositionState>() : currentNodePositions,
  };
}

// buildUsablePositionState creates a stable signature of devices that already have usable positions.
export function buildUsablePositionState(
  devices: Device[],
  currentPositions: Map<string, PositionState>,
  savedPositions: Map<string, PositionState>,
): string {
  return devices
    .map((device) => {
      const currentPosition = currentPositions.get(device.id);
      const savedPosition = savedPositions.get(device.id);

      if (hasUsablePosition(currentPosition) || hasUsablePosition(savedPosition)) {
        return device.id;
      }

      return null;
    })
    .filter((deviceId): deviceId is string => deviceId !== null)
    .sort()
    .join('|');
}

// positionsChanged compares a save payload with the saved backend position map.
export function positionsChanged(
  nextPositions: ReturnType<typeof buildPositionPayload>,
  savedPositions: Map<string, PositionState>,
): boolean {
  if (nextPositions.length !== savedPositions.size) {
    return true;
  }

  for (const position of nextPositions) {
    const savedPosition = savedPositions.get(position.device_id);
    if (
      !savedPosition ||
      savedPosition.x !== position.x ||
      savedPosition.y !== position.y ||
      savedPosition.pinned !== position.pinned
    ) {
      return true;
    }
  }

  return false;
}

// buildTopologyPositionSavePlan decides whether rendered node positions need persistence.
export function buildTopologyPositionSavePlan(
  nodes: DeviceNode[],
  savedPositions: Map<string, PositionState>,
): TopologyPositionSavePlan {
  const payload = buildPositionPayload(nodes);
  return {
    payload,
    shouldSave: positionsChanged(payload, savedPositions),
  };
}

// nodePositionsToPositionMap converts React Flow node positions into persisted position state.
export function nodePositionsToPositionMap(nodes: DeviceNode[]): Map<string, PositionState> {
  return new Map(
    nodes.map((node) => [
      node.id,
      {
        x: node.position.x,
        y: node.position.y,
        pinned: node.data.pinned ?? false,
      },
    ]),
  );
}

// mergeNodePresentationState preserves transient React Flow presentation fields across recomposition.
export function mergeNodePresentationState(
  nextNodes: DeviceNode[],
  currentNodes: DeviceNode[],
): DeviceNode[] {
  const currentNodesById = new Map(currentNodes.map((node) => [node.id, node]));

  return nextNodes.map((node) => {
    const currentNode = currentNodesById.get(node.id);
    if (!currentNode) {
      return node;
    }

    return {
      ...node,
      selected: currentNode.selected,
      dragging: currentNode.dragging,
      width: currentNode.width,
      height: currentNode.height,
      initialWidth: currentNode.initialWidth,
      initialHeight: currentNode.initialHeight,
      measured: currentNode.measured,
      data: {
        ...node.data,
        highlighted: currentNode.data.highlighted,
      },
    };
  });
}
